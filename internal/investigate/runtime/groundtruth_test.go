package runtime

// REAL runs of the runtime validator: a throwaway kind cluster per
// groundtruth environment, built from the case's input.yaml + env/ only
// (never expected.yaml, NOTES.md or observed/), collected by the bundle lane,
// rendered by renderdiff, graph and traversal by relations, offline from a
// warm state directory. Opt-in (it pulls images and takes minutes per case):
//
//	RI_RUNTIME_STATE=/path/to/.ri RI_RUNTIME_CASES=cert-manager-1.17-1.18--payments-eu \
//	  go test ./internal/investigate/runtime -run Groundtruth -v -timeout 60m
//
// RI_RUNTIME_CASES is a comma list of case ids. RI_RUNTIME_TIMEOUT overrides
// the per-run budget (e.g. 20m). It asserts generic behaviour only — every
// record conforms, the cluster is deleted — and logs the outcome per item
// and the run's degradations (it never compares with expected classes).

import (
	"context"
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
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
	"github.com/tdavison784/release-intelligence/internal/investigate/renderdiff"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
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
		ID         string `yaml:"id"`
		Root       string `yaml:"root"`
		Kubernetes string `yaml:"kubernetes"`
	} `yaml:"environment"`
}

func TestGroundtruthRuntime(t *testing.T) {
	state, cases := os.Getenv("RI_RUNTIME_STATE"), os.Getenv("RI_RUNTIME_CASES")
	if state == "" || cases == "" {
		t.Skip("set RI_RUNTIME_STATE (a warm state directory) and RI_RUNTIME_CASES (comma-separated groundtruth case ids) to run real isolated upgrades")
	}
	for _, bin := range []string{"helm", "kubectl", "kind", "docker"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s is not on PATH; a real runtime validation needs it", bin)
		}
	}
	budget := 15 * time.Minute
	if s := os.Getenv("RI_RUNTIME_TIMEOUT"); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			t.Fatal(err)
		}
		budget = d
	}
	root := filepath.Join("..", "..", "..")
	a, err := app.New(app.Config{ProductsDir: filepath.Join(root, "products"), StateDir: state, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	sets, err := catalog.LoadExercises(filepath.Join(root, "products", "exercises"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range strings.Split(cases, ",") {
		id = strings.TrimSpace(id)
		path := filepath.Join(root, "eval", "investigations", id, "input.yaml")
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
			change, err := bundle.NewChangeCollector(a, st).CollectChange(ctx, investigate.ChangeRequest{Product: in.Product, From: in.From.Version, To: in.To.Version, Render: true})
			if err != nil {
				t.Skipf("change evidence not available offline: %v", err)
			}
			snap, err := bundle.NewSnapshotBuilderWithHints(st, env.HintsFromCatalog(a.Catalog)).BuildSnapshot(ctx,
				investigate.EnvironmentInputs{Name: in.Environment.ID, Repo: envDir, KubernetesVersion: in.Environment.Kubernetes})
			if err != nil {
				t.Fatal(err)
			}
			renders, err := renderdiff.New(a, st).RenderEnvironment(ctx, change, snap, investigate.Config{Render: true})
			if err != nil {
				t.Fatal(err)
			}
			b, err := bundle.NewAssembler(st).Assemble(ctx, change, snap, renders)
			if err != nil {
				t.Fatal(err)
			}
			g, err := relations.NewBuilder(st, relations.WithCatalog(a.Catalog)).BuildGraph(ctx, b)
			if err != nil {
				t.Fatal(err)
			}
			roots, err := relations.NewExposure().Exposure(ctx, b, g, nil)
			if err != nil {
				t.Fatal(err)
			}
			tr, err := relations.NewTraverser().Traverse(ctx, g, roots, domain.DefaultTraversalBound)
			if err != nil {
				t.Fatal(err)
			}
			v := New(WithStore(st), WithRenderer(NewAppRenderer(a)), WithExercises(FromSets(sets...)...), WithTimeout(budget),
				WithLogf(func(f string, args ...any) { t.Logf(f, args...) }))
			run, err := v.Run(ctx, b, g)
			if err != nil {
				t.Fatal(err)
			}
			assertNoRuntimeClusters(t)
			t.Logf("RUN %s: %s %s — cluster %s, node %s, %.0fs, upgrade completed=%v", in.ID, run.Status, run.Reason, run.Cluster, run.NodeImage, run.DurationSeconds, run.UpgradeCompleted)
			for _, p := range run.Phases {
				t.Logf("  phase %s: applied %d, retries %d, pruned %d, hooks %d, unserved %d, errors %d, not ready %d", p.Phase, p.Applied, p.Retries, len(p.Pruned), len(p.Hooks), len(p.Unserved), len(p.Errors), len(p.NotReady))
				for id, e := range p.Errors {
					t.Logf("    error %s: %s", id, e)
				}
				for _, n := range p.NotReady {
					t.Logf("    not ready %s", n)
				}
			}
			for _, x := range run.Exercises {
				t.Logf("  exercise %s @%s: %d targets, executed %d, failed %v", x.Exercise, x.Phase, len(x.Targets), len(x.Executed), x.Failed)
			}
			for _, p := range run.UpgradeProblems {
				t.Logf("  upgrade problem: %s", p)
			}
			for _, d := range run.Degradations {
				t.Logf("  DEGRADATION %s [%s] %s: %s", d.Object, d.Window, d.Signature, d.Detail)
			}
			for _, n := range run.Notes {
				t.Logf("  note: %s", n)
			}
			counts := map[string]int{}
			var lines []string
			arts := map[string]bool{}
			for _, x := range b.Manifest.Artifacts {
				arts[x.ID] = true
			}
			for _, it := range b.Items() {
				if !it.InScope {
					continue
				}
				out, err := v.Validate(ctx, investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: it.ID})
				if err != nil {
					t.Fatalf("%s: %v", it.ID, err)
				}
				for _, x := range out.Artifacts {
					arts[x.ID] = true
				}
				for _, r := range out.Records {
					conforms(t, r, arts, out.Evidence)
					counts[string(r.Outcome)+" "+r.Provenance.Rule]++
					lines = append(lines, fmt.Sprintf("%-7s %-26s %-9s %-20s %s — %s", r.Outcome, r.Provenance.Rule, it.Family, it.Manifestation,
						clip(it.Title, 70), clip(strings.Join(strings.Fields(r.Detail), " "), 700)))
				}
			}
			var keys []string
			for k := range counts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				t.Logf("%-45s %d", k, counts[k])
			}
			for _, l := range lines {
				t.Log(l)
			}
		})
	}
}

// conforms checks a record against the contract and that every id it cites
// resolves (bundle manifest + run artifacts; evidence returned with it).
func conforms(t *testing.T, r domain.ValidationRecord, arts map[string]bool, evs []domain.Evidence) {
	t.Helper()
	if err := r.Validate(); err != nil {
		t.Fatalf("record %s: %v", r.ID, err)
	}
	if r.Kind != domain.ValidationRuntime || r.Level != domain.LevelObserved {
		t.Fatalf("record %s: kind %s level %s", r.ID, r.Kind, r.Level)
	}
	for _, id := range append(append([]string(nil), r.Inputs...), r.Outputs...) {
		if !arts[id] {
			t.Fatalf("record %s cites artifact %s that does not resolve", r.ID, id)
		}
	}
	have := map[domain.EvidenceID]bool{}
	for _, e := range evs {
		have[e.ID] = true
	}
	for _, id := range r.Evidence {
		if !have[id] {
			t.Fatalf("record %s cites evidence %s not returned", r.ID, id)
		}
	}
}

func assertNoRuntimeClusters(t *testing.T) {
	t.Helper()
	names, err := NewKindDriver().Clusters(context.Background())
	if err != nil {
		t.Fatalf("kind get clusters: %v", err)
	}
	for _, n := range names {
		if strings.HasPrefix(n, ClusterPrefix) {
			t.Fatalf("cluster %s was left behind", n)
		}
	}
}
