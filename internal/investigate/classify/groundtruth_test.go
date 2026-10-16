package classify

// The classifier over REAL bundles and the real wave-1 stages: the
// groundtruth environments' input.yaml + env/ (never expected.yaml, NOTES.md
// or observed/), collected by lane bundle, rendered by lane renderdiff,
// traversed by lane relations and validated by lane validate, offline from a
// warm state directory. Opt-in:
//
//	RI_CLASSIFY_STATE=/path/to/.ri go test ./internal/investigate/classify -run Groundtruth -v
//
// It asserts generic behaviour only — every classification is recorded on a
// run record that ValidatesAgainst its bundle, re-classifying the stored run
// reproduces it byte for byte, nothing is approval-eligible without the agent
// stage — and logs the class table (it never compares with expected classes).

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
	"github.com/tdavison784/release-intelligence/internal/investigate/validate"
)

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
		ID   string `yaml:"id"`
		Root string `yaml:"root"`
	} `yaml:"environment"`
}

func TestGroundtruthClassification(t *testing.T) {
	state := os.Getenv("RI_CLASSIFY_STATE")
	if state == "" {
		t.Skip("set RI_CLASSIFY_STATE to a warm state directory (e.g. the primary checkout's .ri) to classify real bundles offline")
	}
	for _, bin := range []string{"helm", "kubectl"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not on PATH; real environment renders need it", bin)
		}
	}
	root := filepath.Join("..", "..", "..")
	a, err := app.New(app.Config{ProductsDir: filepath.Join(root, "products"), StateDir: state, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	var facts []domain.VerifiedFact
	if ks, err := app.LoadKnowledge(filepath.Join(root, "knowledge")); err == nil {
		facts = ks.Facts
	}
	inputs, _ := filepath.Glob(filepath.Join(root, "eval", "investigations", "*", "input.yaml"))
	if len(inputs) == 0 {
		t.Skip("no groundtruth inputs")
	}
	for _, path := range inputs {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var in gtInput
		if err := yaml.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		envDir, _ := filepath.Abs(filepath.Join(filepath.Dir(path), in.Environment.Root))
		t.Run(in.ID, func(t *testing.T) {
			ctx := context.Background()
			st, err := store.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			change, err := bundle.NewChangeCollector(a, st, bundle.WithKnowledge(facts...)).CollectChange(ctx, investigate.ChangeRequest{Product: in.Product, From: in.From.Version, To: in.To.Version, Render: true})
			if err != nil {
				t.Skipf("change evidence not available offline: %v", err)
			}
			snap, err := bundle.NewSnapshotBuilderWithHints(st, env.HintsFromCatalog(a.Catalog)).BuildSnapshot(ctx, investigate.EnvironmentInputs{Name: in.Environment.ID, Repo: envDir})
			if err != nil {
				t.Fatal(err)
			}
			cfg := investigate.Config{Policy: Policy, Bound: domain.DefaultTraversalBound, Render: true}
			renders, err := renderdiff.New(a, st).RenderEnvironment(ctx, change, snap, cfg)
			if err != nil {
				t.Fatal(err)
			}
			b, err := bundle.NewAssembler(st).Assemble(ctx, change, snap, renders)
			if err != nil {
				t.Fatal(err)
			}
			rep, err := a.Impact(ctx, in.Product, in.From.Version, in.To.Version, app.ImpactOptions{
				Environment: env.Inputs{Repo: envDir}, Facts: facts, MinVerification: domain.VerifiedHuman,
				Render: &app.RenderOptions{Repo: envDir}})
			if err != nil {
				t.Logf("no impact report: %v", err)
				rep = nil
			}
			g, err := relations.NewBuilder(st, relations.WithCatalog(a.Catalog)).BuildGraph(ctx, b)
			if err != nil {
				t.Fatal(err)
			}
			roots, err := relations.NewExposure().Exposure(ctx, b, g, rep)
			if err != nil {
				t.Fatal(err)
			}
			tr, err := relations.NewTraverser().Traverse(ctx, g, roots, cfg.Bound)
			if err != nil {
				t.Fatal(err)
			}
			validators := []investigate.Validator{validate.JoinVerdicts(rep, validate.WithStore(st)), validate.RenderDiff(validate.WithStore(st)),
				validate.OverrideInspection(validate.WithStore(st), validate.WithFacts(facts...)), validate.SchemaCompat(validate.WithStore(st)),
				validate.StaticReference(validate.WithStore(st))}
			var vals []domain.ValidationRecord
			var vEv []domain.Evidence
			var vArts []domain.ArtifactRef
			seenEv, seenArt := map[domain.EvidenceID]bool{}, map[string]bool{}
			unvalidated := 0
			for _, id := range b.InScope() {
				it, _ := b.Item(id)
				for _, v := range validators {
					if !v.Applies(it.Family) {
						continue
					}
					out, err := v.Validate(ctx, investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, Report: rep, ChangeItem: id})
					if err != nil && errors.Is(err, investigate.ErrNotFound) && isEnvItem(b, id) {
						// Known cross-lane gap (status/classify.md): validate looks items
						// up in the change record only, so environment-scope items
						// (scope:rendered-unexplained) get no records — they stay open.
						unvalidated++
						continue
					}
					if err != nil {
						t.Fatalf("%s: %v", v.Name(), err)
					}
					vals = append(vals, out.Records...)
					for _, e := range out.Evidence {
						if !seenEv[e.ID] {
							seenEv[e.ID] = true
							vEv = append(vEv, e)
						}
					}
					for _, ar := range out.Artifacts {
						if !seenArt[ar.ID] {
							seenArt[ar.ID] = true
							vArts = append(vArts, ar)
						}
					}
				}
			}
			cin := investigate.ClassifyInput{Bundle: b, Traversal: tr, Report: rep, Validations: vals, Config: cfg}
			cls, err := New().Classify(ctx, cin)
			if err != nil {
				t.Fatalf("classify: %v", err)
			}
			inv := &domain.Investigation{SchemaVersion: domain.InvestigationSchemaVersion, Bundle: b.ID, Snapshot: b.Snapshot.ID,
				ChangeRecord: b.Change.ID, Environment: b.Snapshot.Environment, Config: cfg.Digest(), Status: domain.InvestigationRunning,
				Traversal: tr, Validations: vals, Evidence: vEv, Artifacts: vArts, Review: domain.HumanReview{State: domain.ReviewNotRequired}, CreatedAt: testNow}
			inv.ID = domain.InvestigationID(inv.Bundle, inv.Config)
			inv.AppendAudit(domain.AuditEvent{At: testNow, Stage: domain.StageValidate, Event: "completed", Actor: "system"})
			if err := Apply(inv, cls, testNow.Add(time.Minute)); err != nil {
				t.Fatalf("apply: %v", err)
			}
			if err := inv.ValidateAgainst(b); err != nil {
				t.Fatalf("classification does not ValidateAgainst its bundle: %v", err)
			}
			again, err := New().Classify(ctx, investigate.ClassifyInput{Bundle: b, Traversal: inv.Traversal, Report: rep, Validations: inv.Validations, Config: cfg})
			if err != nil || !bytes.Equal(mustJSON(t, again), mustJSON(t, cls)) {
				t.Fatalf("re-classification does not reproduce the findings (%v)", err)
			}
			if cls.Decision.Outcome == domain.PolicyAutoApproveEligible {
				t.Fatal("auto-approve-eligible without the agent stage")
			}

			counts := map[string]int{}
			var lines []string
			for _, f := range cls.Findings {
				it, _ := b.Item(f.ChangeItem)
				counts[fmt.Sprintf("%-22s %s", f.Class, f.Manifestation)]++
				lines = append(lines, fmt.Sprintf("%-22s %-20s %-26s %s", f.Class, f.Manifestation, it.ScopeRule, clip(it.Title, 70)))
			}
			var keys []string
			for k := range counts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				t.Logf("%-48s %d", k, counts[k])
			}
			if unvalidated > 0 {
				t.Logf("%d validator run(s) on environment-scope items returned ErrNotFound (validate looks items up in the change record only)", unvalidated)
			}
			t.Logf("decision %s: %s", cls.Decision.Outcome, strings.Join(cls.Decision.Reasons, " | "))
			if os.Getenv("RI_CLASSIFY_VERBOSE") != "" {
				for _, l := range lines {
					t.Log(l)
				}
				var buf bytes.Buffer
				_ = WriteText(&buf, inv, b)
				t.Log("\n" + buf.String())
			}
		})
	}
}

func isEnvItem(b *domain.EvidenceBundle, id string) bool {
	for _, it := range b.EnvironmentItems {
		if it.ID == id {
			return true
		}
	}
	return false
}
