package renderdiff

// The groundtruth environments (eval/investigations/<case>/input.yaml and
// env/ — never expected.yaml, NOTES.md or observed/), rendered for real:
// helm and kubectl on PATH, upstream charts and releases served OFFLINE from
// a warm state directory. Opt-in, because the checked-in fixtures do not
// carry the charts:
//
//	RI_RENDERDIFF_STATE=/path/to/.ri go test ./internal/investigate/renderdiff -run Groundtruth -v
//
// The test asserts generic behaviour only (every pair has a status; every
// indexed change resolves to the artifact bytes; the bundle validates; two
// runs are identical) and logs the render coverage the lane reports.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/render"
)

const groundtruthDir = "../../../eval/investigations"

// gtInput is the part of input.yaml the renderer needs.
type gtInput struct {
	ID      string `yaml:"id"`
	Product string `yaml:"product"`
	From    struct {
		Version string `yaml:"version"`
	} `yaml:"from"`
	To struct {
		Version string `yaml:"version"`
	} `yaml:"to"`
	Environment struct {
		ID       string `yaml:"id"`
		Root     string `yaml:"root"`
		Complete bool   `yaml:"complete"`
	} `yaml:"environment"`
}

func TestGroundtruthEnvironments(t *testing.T) {
	state := os.Getenv("RI_RENDERDIFF_STATE")
	if state == "" {
		t.Skip("set RI_RENDERDIFF_STATE to a warm state directory (e.g. the primary checkout's .ri) to render the groundtruth environments offline")
	}
	for _, bin := range []string{"helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not on PATH; the groundtruth renders need the real renderers", bin)
		}
	}
	a, err := app.New(app.Config{ProductsDir: "../../../products", StateDir: state, Offline: true})
	must(t, err)
	inputs, _ := filepath.Glob(filepath.Join(groundtruthDir, "*", "input.yaml"))
	if len(inputs) == 0 {
		t.Fatal("no groundtruth inputs")
	}
	var table []string
	for _, path := range inputs {
		raw, err := os.ReadFile(path)
		must(t, err)
		var in gtInput
		must(t, yaml.Unmarshal(raw, &in))
		envDir, _ := filepath.Abs(filepath.Join(filepath.Dir(path), in.Environment.Root))
		t.Run(in.ID, func(t *testing.T) {
			ctx := context.Background()
			edge, err := a.Upgrade(ctx, in.Product, in.From.Version, in.To.Version, app.UpgradeOptions{})
			if err != nil {
				t.Skipf("upgrade edge not available offline from %s: %v", state, err)
			}
			for _, anchored := range []bool{false, true} {
				st := newMemStore()
				change := gtChange(t, a, st, edge, anchored)
				snap := gtSnapshot(t, st, in.Environment.ID, envDir)
				r := New(a, st)
				cfg := investigate.Config{Render: true, KubeVersion: "1.31"}
				res, err := r.RenderEnvironment(ctx, change, snap, cfg)
				must(t, err)
				b := gtBundle(change, snap, res)
				if err := b.Validate(); err != nil {
					t.Fatalf("bundle: %v", err)
				}
				assertResolves(t, st, b)
				again, err := r.RenderEnvironment(ctx, change, snap, cfg)
				must(t, err)
				j1, _ := json.Marshal(res)
				j2, _ := json.Marshal(again)
				if string(j1) != string(j2) {
					t.Errorf("two renders of the same inputs differ")
				}
				for _, p := range res.Pairs {
					if p.Status == "" || p.Status != domain.CollectionOK && p.Reason == "" {
						t.Errorf("%s: status %q reason %q", p.Target, p.Status, p.Reason)
					}
					t.Logf("anchored=%v %-60s %-14s changes=%-3d %s", anchored, p.Target, p.Status, len(p.Changes), trunc(p.Reason, 140))
				}
				for _, n := range res.Notes {
					t.Logf("note: %s", n)
				}
				m := Measure(res.Pairs)
				reasons := make([]string, 0, len(m.Reasons))
				for k, v := range m.Reasons {
					reasons = append(reasons, fmt.Sprintf("%s×%d", k, v))
				}
				sort.Strings(reasons)
				table = append(table, fmt.Sprintf("| %s | %v | %d | %d | %d | %d | %d | %s | %d | %d | %d |", in.ID, anchored, m.Targets, m.OK, m.Partial,
					m.Unavailable, m.NotApplicable, strings.Join(reasons, ", "), m.Changes, m.Correlated, m.Uncorrelated))
				if os.Getenv("RI_RENDERDIFF_DUMP") != "" && anchored {
					for _, p := range res.Pairs {
						for _, c := range p.Changes {
							t.Logf("  %s %s %s %s items=%v rule=%s", p.Target, c.Class, c.Object, c.Path, c.ChangeItems, c.Correlation)
						}
					}
				}
			}
		})
	}
	t.Log("\n| case | release-anchored items | targets | ok | partial | unavailable | not-applicable | non-ok reasons | rendered changes | correlated | uncorrelated |\n|---|---|---|---|---|---|---|---|---|---|---|\n" + strings.Join(table, "\n"))
}

// gtChange is a minimal change result over the real edge: one item per
// non-routine change with evidence (the shape lane bundle's collector
// produces), every source stated. With anchored, it also renders the chart
// defaults (release scope) and anchors an item on every release-scope
// rendered change no edge change accounts for — the collector rule this
// lane proposes (status file), so environment image bumps correlate.
func gtChange(t *testing.T, a *app.App, st *memStore, edge *domain.UpgradeEdge, anchored bool) *investigate.ChangeResult {
	t.Helper()
	ctx := context.Background()
	eb, err := json.Marshal(edge)
	must(t, err)
	edgeRef, err := st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactUpgradeEdge, Path: "derived/upgrade/edge.json", Origin: domain.OriginDerived,
		MediaType: "application/json", URI: "ri:upgrade@v1", Producer: "upgrade@v1"}, eb)
	must(t, err)
	rec := domain.ChangeEvidenceRecord{Product: edge.Product, From: edge.From, To: edge.To, Edge: edgeRef.ID, Producer: "test.collect@v1"}
	for _, k := range domain.ChangeSourceKinds {
		rec.Sources = append(rec.Sources, domain.ChangeSourceStatus{Source: k, Status: domain.CollectionUnavailable, Reason: "not collected by this test"})
	}
	evidence := append([]domain.Evidence(nil), edge.Evidence...)
	item := func(it domain.ChangeItem) domain.ChangeItem {
		it.ID = domain.ChangeItemID(it.ChangeID, it.Platform, it.RenderedChange)
		it.Manifestation, it.ManifestationRule, it.InScope, it.ScopeRule = domain.ManifestUnknown, "manifest:unknown", true, "scope:test"
		return it
	}
	for _, c := range edge.Changes {
		if c.Routine || len(c.Evidence) == 0 {
			continue
		}
		rec.Items = append(rec.Items, item(domain.ChangeItem{ChangeID: c.ID, Family: domain.FamilyBehavior, Title: c.Title, Evidence: c.Evidence}))
	}
	arts := []domain.ArtifactRef{edgeRef}
	if anchored {
		kube := "1.31"
		p := a.RenderEngine().RenderPair(ctx, render.PairRequest{Product: string(edge.Product.ID), From: edge.From.String(), To: edge.To.String(),
			Target: render.DefaultsTarget(string(edge.Product.ID)), Scope: domain.RenderRelease, KubeVersion: kube})
		if p.Status != render.PairOK {
			t.Fatalf("release render: %s", p.Describe())
		}
		put := func(kind domain.ArtifactKind, name string, b []byte) string {
			ref, err := st.Put(ctx, investigate.ArtifactSpec{Kind: kind, Path: "derived/render/release/" + name, Origin: domain.OriginDerived,
				MediaType: "application/octet-stream", URI: "ri:test", Producer: "test.collect@v1"}, b)
			must(t, err)
			arts = append(arts, ref)
			return ref.ID
		}
		fromB, _ := EncodeObjects(p.FromResult.Objects)
		toB, _ := EncodeObjects(p.ToResult.Objects)
		diffB, _ := json.Marshal(p.Diff)
		mp := domain.ManifestPair{Scope: domain.RenderRelease, Target: "chart-defaults", From: put(domain.ArtifactRenderedManifests, "from.yaml", fromB),
			To: put(domain.ArtifactRenderedManifests, "to.yaml", toB), Diff: put(domain.ArtifactRenderDiff, "diff.json", diffB), Status: domain.CollectionOK}
		items := map[string]bool{}
		for _, it := range rec.Items {
			items[it.ID] = true
		}
		corr := render.Correlate(p.Diff, edge)
		seen := map[string]bool{}
		for i, c := range p.Diff.Changes {
			rc := domain.RenderedChange{Class: string(c.Class), Object: c.Object.String(), Path: changePath(c)}
			rc.ID = domain.RenderedChangeID(domain.RenderRelease, mp.Target, rc.Object, rc.Path, rc.Class)
			if seen[rc.ID] {
				continue
			}
			seen[rc.ID] = true
			for _, l := range corr.Changes[i].Links {
				if id := domain.ChangeItemID(l.ChangeID, "", ""); l.ChangeID != "" && items[id] {
					rc.ChangeItems, rc.Correlation = appendUnique(rc.ChangeItems, id), l.Rule
				}
			}
			if len(rc.ChangeItems) == 0 {
				ev := p.Evidence(c, true)
				evidence = append(evidence, ev)
				it := item(domain.ChangeItem{RenderedChange: rc.ID, Family: domain.FamilyRendered, Title: c.Summary(false), Evidence: []domain.EvidenceID{ev.ID}})
				it.Manifestation, it.ManifestationRule = domain.ManifestRenderedConfig, "manifest:rendered-anchor"
				rec.Items = append(rec.Items, it)
				rc.ChangeItems, rc.Correlation = []string{it.ID}, "rendered-anchor"
			}
			sort.Strings(rc.ChangeItems)
			mp.Changes = append(mp.Changes, rc)
		}
		sort.Slice(mp.Changes, func(i, j int) bool { return mp.Changes[i].ID < mp.Changes[j].ID })
		rec.Renders = []domain.ManifestPair{mp}
	}
	rec.ID = rec.DerivedID()
	must(t, rec.Validate())
	return &investigate.ChangeResult{Record: rec, Artifacts: arts, Evidence: uniqueEvidence(evidence)}
}

func gtSnapshot(t *testing.T, st *memStore, name, repo string) *investigate.SnapshotResult {
	t.Helper()
	facts, err := st.Put(context.Background(), investigate.ArtifactSpec{Kind: domain.ArtifactEnvironmentFacts, Path: "derived/env/facts.json",
		Origin: domain.OriginDerived, MediaType: "application/json", URI: "ri:test", Producer: "test.snapshot@v1"}, []byte(`{"repo":"`+filepath.Base(repo)+`"}`))
	must(t, err)
	s := domain.EnvironmentSnapshot{Environment: name, Repo: &domain.RepoRef{Path: repo, TreeDigest: domain.Digest([]byte(repo))},
		Inputs: []domain.SnapshotInput{{Kind: domain.InputGitOpsRepo, Path: repo, Status: domain.CollectionOK}}, Facts: facts.ID, Producer: "test.snapshot@v1"}
	s.ID = s.DerivedID()
	return &investigate.SnapshotResult{Snapshot: s, Artifacts: []domain.ArtifactRef{facts}}
}

func gtBundle(change *investigate.ChangeResult, snap *investigate.SnapshotResult, res *investigate.RenderResult) *domain.EvidenceBundle {
	arts := append(append(append([]domain.ArtifactRef(nil), change.Artifacts...), snap.Artifacts...), res.Artifacts...)
	sort.Slice(arts, func(i, j int) bool { return arts[i].Path < arts[j].Path })
	b := &domain.EvidenceBundle{SchemaVersion: domain.EvidenceBundleSchemaVersion, Change: change.Record, Snapshot: snap.Snapshot, Renders: res.Pairs,
		Manifest: domain.BundleManifest{Artifacts: arts}, Evidence: uniqueEvidence(append(append([]domain.Evidence(nil), change.Evidence...), res.Evidence...)),
		Producer: "test.assemble@v1"}
	b.ID = domain.BundleID(b.Change.ID, b.Snapshot.ID, b.Manifest.Digest(), b.Renders)
	return b
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
