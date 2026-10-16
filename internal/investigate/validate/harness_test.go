package validate

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// The fixtures below describe a FICTIONAL product ("gizmo", API group
// gizmo.example.io, kind Widget, controller Deployment gizmo/gizmo-controller)
// so the tests assert generic behaviour, never an evaluation case's answer.

var testNow = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func clock() time.Time { return testNow }

// memStore is an in-memory investigate.ArtifactStore.
type memStore struct{ m map[string][]byte }

func newMemStore() *memStore { return &memStore{m: map[string][]byte{}} }

func (s *memStore) Put(_ context.Context, spec investigate.ArtifactSpec, content []byte) (domain.ArtifactRef, error) {
	d := domain.Digest(content)
	s.m[d] = append([]byte(nil), content...)
	return domain.ArtifactRef{
		ID: domain.ArtifactID(spec.Kind, spec.Path, d), Kind: spec.Kind, MediaType: spec.MediaType, Digest: d,
		Size: int64(len(content)), Path: spec.Path, Origin: spec.Origin, URI: spec.URI, Producer: spec.Producer,
		Redacted: spec.Redacted, Tags: spec.Tags, ChangeItems: spec.ChangeItems,
	}, nil
}

func (s *memStore) Open(_ context.Context, digest string) (io.ReadCloser, error) {
	b, ok := s.m[digest]
	if !ok {
		return nil, investigate.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// fx builds a valid synthetic evidence bundle.
type fx struct {
	t      *testing.T
	st     *memStore
	edge   domain.UpgradeEdge
	items  []domain.ChangeItem
	arts   []domain.ArtifactRef
	ev     []domain.Evidence
	pairs  []domain.ManifestPair
	files  []string
	inputs []domain.SnapshotInput
	kube   string
	up     domain.Evidence
}

func newFx(t *testing.T) *fx {
	t.Helper()
	f := &fx{t: t, st: newMemStore()}
	f.up = domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.invalid/gizmo/releases/v2.0.0", "L1-L3",
		"the default of mode changes from safe to fast", domain.Digest([]byte("notes")), testNow)
	f.ev = append(f.ev, f.up)
	f.edge = domain.UpgradeEdge{
		SchemaVersion: domain.UpgradeEdgeSchemaVersion,
		Product:       domain.ProductRef{ID: "gizmo", Name: "gizmo"},
		From:          domain.Version{Tag: "v1.0.0", Semver: "1.0.0"},
		To:            domain.Version{Tag: "v2.0.0", Semver: "2.0.0"},
		Evidence:      []domain.Evidence{f.up},
	}
	return f
}

func (f *fx) put(kind domain.ArtifactKind, origin domain.ArtifactOrigin, path, content string, items ...string) domain.ArtifactRef {
	f.t.Helper()
	media := "application/yaml"
	if strings.HasSuffix(path, ".json") {
		media = "application/json"
	}
	a, err := f.st.Put(context.Background(), investigate.ArtifactSpec{Kind: kind, Path: path, Origin: origin, MediaType: media,
		URI: "ri:test/" + path, Producer: "test@v1", ChangeItems: items}, []byte(content))
	if err != nil {
		f.t.Fatal(err)
	}
	f.arts = append(f.arts, a)
	return a
}

// change adds an edge change and a change item anchored on it.
func (f *fx) change(c domain.Change, mods ...func(*domain.ChangeItem)) string {
	if c.Provenance.Producer == "" {
		c.Provenance = domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: c.Provenance.Rule, Confidence: domain.ConfidenceHigh}
	}
	c.Evidence = []domain.EvidenceID{f.up.ID}
	f.edge.Changes = append(f.edge.Changes, c)
	it := domain.ChangeItem{ChangeID: c.ID, Family: domain.FamilyValues, Title: c.Title, Breaking: c.Breaking,
		Subjects: c.Subjects, Evidence: []domain.EvidenceID{f.up.ID}, InScope: true, ScopeRule: "scope:breaking",
		Manifestation: domain.ManifestRenderedConfig, ManifestationRule: "manifest:computed-values"}
	for _, m := range mods {
		m(&it)
	}
	it.ID = domain.ChangeItemID(it.ChangeID, it.Platform, it.RenderedChange)
	f.items = append(f.items, it)
	return it.ID
}

// platformItem adds a compatibility item anchored on a platform.
func (f *fx) platformItem(cc domain.CompatibilityChange) string {
	f.edge.Compatibility = append(f.edge.Compatibility, cc)
	it := domain.ChangeItem{Platform: cc.Platform, Family: domain.FamilyCompatibility, Title: cc.Summary,
		Evidence: []domain.EvidenceID{f.up.ID}, InScope: true, ScopeRule: "scope:computed-breaking",
		Manifestation: domain.ManifestRenderedConfig, ManifestationRule: "manifest:computed-compat"}
	it.ID = domain.ChangeItemID("", it.Platform, "")
	f.items = append(f.items, it)
	return it.ID
}

func behaviour(it *domain.ChangeItem) {
	it.Family, it.Manifestation, it.ManifestationRule = domain.FamilyBehavior, domain.ManifestControllerBehaviour, "manifest:declared-behaviour"
}

func notBreaking(it *domain.ChangeItem) { it.Breaking, it.ScopeRule = false, "scope:category" }

// file adds an environment file of the GitOps repository.
func (f *fx) file(path, content string) domain.ArtifactRef {
	a := f.put(domain.ArtifactEnvironmentFile, domain.OriginEnvironment, "env/"+path, content)
	f.files = append(f.files, a.ID)
	return a
}

// rc is a rendered change to put into a pair (id derived by pair()).
func rc(class, object, path, before, after, correlation string, items ...string) domain.RenderedChange {
	sort.Strings(items)
	c := domain.RenderedChange{Class: class, Object: object, Path: path, Before: before, After: after, ChangeItems: items}
	if len(items) > 0 {
		c.Correlation = correlation
	}
	return c
}

// pair adds a complete environment render pair.
func (f *fx) pair(target, from, to string, changes ...domain.RenderedChange) []string {
	dir := "derived/render/env/" + strings.NewReplacer(":", "-", "/", "-").Replace(target)
	fa := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, dir+"/from.yaml", from)
	ta := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, dir+"/to.yaml", to)
	da := f.put(domain.ArtifactRenderDiff, domain.OriginDerived, dir+"/diff.json", `{"target":"`+target+`"}`)
	var ids []string
	for i := range changes {
		changes[i].ID = domain.RenderedChangeID(domain.RenderEnvironment, target, changes[i].Object, changes[i].Path, changes[i].Class)
		ids = append(ids, changes[i].ID)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID < changes[j].ID })
	f.pairs = append(f.pairs, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: fa.ID, To: ta.ID, Diff: da.ID,
		Status: domain.CollectionOK, Changes: changes})
	return ids
}

// brokenPair adds an incomplete environment render pair.
func (f *fx) brokenPair(target, reason string) {
	f.pairs = append(f.pairs, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, Status: domain.CollectionPartial, Reason: reason})
}

func (f *fx) bundle() *domain.EvidenceBundle {
	f.t.Helper()
	edgeJSON, err := json.Marshal(f.edge)
	if err != nil {
		f.t.Fatal(err)
	}
	edgeArt := f.put(domain.ArtifactUpgradeEdge, domain.OriginDerived, "derived/upgrade-edge.json", string(edgeJSON))
	factsArt := f.put(domain.ArtifactEnvironmentFacts, domain.OriginDerived, "derived/environment-facts.json", "{}")
	notesArt := f.put(domain.ArtifactUpstreamDocument, domain.OriginUpstream, "upstream/gizmo/v2.0.0/release-notes.md", "notes")
	var sources []domain.ChangeSourceStatus
	for _, k := range domain.ChangeSourceKinds {
		s := domain.ChangeSourceStatus{Source: k, Status: domain.CollectionNotApplicable, Reason: "not in the fixture"}
		if k == domain.ChangeSourceReleaseNotes {
			s = domain.ChangeSourceStatus{Source: k, Status: domain.CollectionOK, Artifacts: []string{notesArt.ID}}
		}
		sources = append(sources, s)
	}
	cr := domain.ChangeEvidenceRecord{Product: f.edge.Product, From: f.edge.From, To: f.edge.To, Edge: edgeArt.ID,
		Items: f.items, Sources: sources, Producer: "test.change@v1"}
	cr.ID = cr.DerivedID()
	inputs := append([]domain.SnapshotInput{{Kind: domain.InputGitOpsRepo, Path: "./gitops", Status: domain.CollectionOK, Artifacts: f.files}}, f.inputs...)
	if f.kube != "" {
		inputs = append(inputs, domain.SnapshotInput{Kind: domain.InputClusterVersion, Value: f.kube, Status: domain.CollectionOK})
	}
	snap := domain.EnvironmentSnapshot{Environment: "test-env", Repo: &domain.RepoRef{Path: "./gitops", TreeDigest: domain.Digest([]byte("tree"))},
		KubernetesVersion: f.kube, Inputs: inputs, Facts: factsArt.ID, Producer: "test.snapshot@v1", CollectedAt: testNow}
	snap.ID = snap.DerivedID()
	man := domain.BundleManifest{Artifacts: append([]domain.ArtifactRef(nil), f.arts...)}
	sort.Slice(man.Artifacts, func(i, j int) bool { return man.Artifacts[i].Path < man.Artifacts[j].Path })
	b := &domain.EvidenceBundle{SchemaVersion: domain.EvidenceBundleSchemaVersion, Change: cr, Snapshot: snap, Renders: f.pairs,
		Manifest: man, Evidence: f.ev, Producer: "test.bundle@v1", CreatedAt: testNow}
	b.ID = domain.BundleID(cr.ID, snap.ID, man.Digest(), b.Renders)
	if err := b.Validate(); err != nil {
		f.t.Fatalf("fixture bundle invalid: %v", err)
	}
	return b
}

// --- graph and traversal ------------------------------------------------------

type gb struct {
	t     *testing.T
	g     domain.RelationshipGraph
	roots []domain.ExposureRoot
	paths []domain.ImpactPath
	ev    domain.EvidenceID
	trunc bool
}

func newGraph(t *testing.T, b *domain.EvidenceBundle) *gb {
	return &gb{t: t, g: domain.RelationshipGraph{Bundle: b.ID, Builder: "test.graph@v1"}, ev: b.Evidence[0].ID}
}

func (g *gb) node(kind domain.NodeKind, key string, placeholder bool) string {
	n := domain.GraphNode{ID: domain.NodeID(kind, key), Kind: kind, Key: key, Placeholder: placeholder}
	g.g.Nodes = append(g.g.Nodes, n)
	return n.ID
}

func (g *gb) edge(kind domain.EdgeKind, from, to string, status domain.EdgeStatus) string {
	e := domain.GraphEdge{ID: domain.EdgeID(kind, from, to, ""), Kind: kind, From: from, To: to, Status: status, Rule: "test.rule@v1"}
	if status == domain.EdgeEstablished {
		e.Evidence = []domain.EvidenceID{g.ev}
	} else {
		e.Reason = "not established in the fixture"
	}
	g.g.Edges = append(g.g.Edges, e)
	return e.ID
}

func (g *gb) root(item, node string) { g.roots = append(g.roots, domain.ExposureRoot{ChangeItem: item, Node: node, Rule: "exposure:test"}) }

// path adds the impact path along nodes (edges looked up between neighbours).
func (g *gb) path(item string, nodes ...string) string {
	p := domain.ImpactPath{ChangeItem: item, Nodes: nodes, Status: domain.EdgeEstablished}
	for i := 0; i+1 < len(nodes); i++ {
		for _, e := range g.g.Edges {
			if e.From == nodes[i] && e.To == nodes[i+1] {
				p.Edges = append(p.Edges, e.ID)
				if !e.Status.Resolved() {
					p.Status = e.Status
				}
			}
		}
	}
	p.ID = domain.ImpactPathID(item, p.Nodes, p.Edges)
	g.paths = append(g.paths, p)
	return p.ID
}

func (g *gb) done() (*domain.RelationshipGraph, *domain.TraversalResult) {
	g.t.Helper()
	sort.Slice(g.g.Nodes, func(i, j int) bool { return g.g.Nodes[i].ID < g.g.Nodes[j].ID })
	sort.Slice(g.g.Edges, func(i, j int) bool { return g.g.Edges[i].ID < g.g.Edges[j].ID })
	g.g.ID = g.g.DerivedID()
	if err := g.g.Validate(); err != nil {
		g.t.Fatalf("fixture graph invalid: %v", err)
	}
	tr := &domain.TraversalResult{Graph: g.g.ID, Algorithm: "graph.dfs@v1", Bound: domain.DefaultTraversalBound, Roots: g.roots, Paths: g.paths}
	visited := map[string]bool{}
	var un []string
	for _, p := range g.paths {
		for _, n := range p.Nodes {
			visited[n] = true
		}
		if len(p.Edges) > tr.DepthReached {
			tr.DepthReached = len(p.Edges)
		}
		for _, eid := range p.Edges {
			for _, e := range g.g.Edges {
				if e.ID == eid && !e.Status.Resolved() {
					un = append(un, eid)
				}
			}
		}
	}
	for _, r := range g.roots {
		visited[r.Node] = true
	}
	for n := range visited {
		tr.Visited = append(tr.Visited, n)
	}
	sort.Strings(tr.Visited)
	tr.Unresolved = uniq(un)
	if g.trunc {
		tr.Truncated, tr.TruncationReason = true, "max paths reached"
	}
	tr.Complete = !tr.Truncated && len(tr.Unresolved) == 0
	tr.ID = tr.DerivedID()
	if err := tr.ValidateAgainst(g.g); err != nil {
		g.t.Fatalf("fixture traversal invalid: %v", err)
	}
	g2 := g.g
	return &g2, tr
}

// --- running and conformance --------------------------------------------------

// run executes a validator and checks every record against the contract: it
// must Validate, be deterministic-level, carry the validator's name, and the
// whole output must ValidateAgainst the bundle inside a run record.
func run(t *testing.T, v investigate.Validator, in investigate.ValidationInput) *investigate.ValidationOutput {
	t.Helper()
	out, err := v.Validate(context.Background(), in)
	if err != nil {
		t.Fatalf("%s: %v", v.Name(), err)
	}
	for _, r := range out.Records {
		if r.Level != domain.LevelDeterministic || r.Validator != v.Name() || r.ChangeItem != in.ChangeItem {
			t.Fatalf("record %s: level %s validator %s item %s", r.ID, r.Level, r.Validator, r.ChangeItem)
		}
		if r.Pin != nil && !r.ClearsBehaviour() {
			t.Fatalf("record %s: a pin that does not clear", r.ID)
		}
	}
	conform(t, in, out)
	return out
}

func conform(t *testing.T, in investigate.ValidationInput, out *investigate.ValidationOutput) {
	t.Helper()
	b := in.Bundle
	inv := &domain.Investigation{SchemaVersion: domain.InvestigationSchemaVersion, Bundle: b.ID, Snapshot: b.Snapshot.ID,
		ChangeRecord: b.Change.ID, Environment: b.Snapshot.Environment, Config: domain.Digest([]byte("config")),
		Status: domain.InvestigationRunning, Traversal: in.Traversal, Validations: out.Records, Evidence: out.Evidence,
		Review: domain.HumanReview{State: domain.ReviewNotRequired}, CreatedAt: testNow}
	if r := in.Request; r != nil {
		inv.ValidationRequests = []domain.ValidationRequest{*r}
		if r.Hypothesis != "" {
			at := testNow
			h := domain.Hypothesis{ID: r.Hypothesis, ChangeItem: in.ChangeItem, Statement: "a hypothesis", Citations: []domain.EvidenceID{b.Evidence[0].ID},
				Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "test.agent@v1", Confidence: domain.ConfidenceLow, Model: "m", ModelVersion: "m",
					PromptVersion: "p/v1", PromptDigest: domain.Digest([]byte("p")), InputEvidence: []domain.EvidenceID{b.Evidence[0].ID}, GeneratedAt: &at, CallID: "call-1"}}
			inv.Hypotheses = []domain.Hypothesis{h}
		}
	}
	inv.ID = domain.InvestigationID(inv.Bundle, inv.Config)
	inv.AppendAudit(domain.AuditEvent{At: testNow, Stage: domain.StageValidate, Event: "completed", Actor: "system"})
	if err := inv.ValidateAgainst(b); err != nil {
		t.Fatalf("validator output does not conform to the contract: %v", err)
	}
}

func one(t *testing.T, out *investigate.ValidationOutput) domain.ValidationRecord {
	t.Helper()
	if len(out.Records) != 1 {
		t.Fatalf("want 1 record, got %d: %+v", len(out.Records), out.Records)
	}
	return out.Records[0]
}

func wantOutcome(t *testing.T, r domain.ValidationRecord, want domain.ValidationOutcomeKind, detail string) {
	t.Helper()
	if r.Outcome != want {
		t.Fatalf("outcome %s, want %s (detail: %s)", r.Outcome, want, r.Detail)
	}
	if detail != "" && !strings.Contains(r.Detail, detail) {
		t.Fatalf("detail %q does not mention %q", r.Detail, detail)
	}
}

// --- fictional product content ------------------------------------------------

const controllerFrom = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: gizmo-controller
  namespace: gizmo
spec:
  template:
    spec:
      containers:
        - name: controller
          image: example.invalid/gizmo:v1.0.0
          args:
            - --log-level=info
%s`

func controller(extraArgs ...string) string {
	var b strings.Builder
	for _, a := range extraArgs {
		b.WriteString("            - " + a + "\n")
	}
	return strings.Replace(controllerFrom, "%s", b.String(), 1)
}

func widget(name, ns, rotation string) string {
	s := "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata:\n  name: " + name + "\n  namespace: " + ns + "\nspec:\n  size: 3\n"
	if rotation != "" {
		s += "  rotation: " + rotation + "\n"
	}
	return s
}

func configMap(name, ns string, data map[string]string) string {
	s := "apiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: " + name + "\n  namespace: " + ns + "\ndata:\n"
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		s += "  " + k + ": " + data[k] + "\n"
	}
	return s
}

func stream(docs ...string) string { return strings.Join(docs, "---\n") }

const deployKey = "apps/v1/Deployment/gizmo/gizmo-controller"
