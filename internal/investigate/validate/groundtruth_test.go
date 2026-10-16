package validate

// The validators over REAL bundles: the groundtruth environments'
// input.yaml + env/ (never expected.yaml, NOTES.md or observed/), collected
// by the bundle lane and rendered by the renderdiff lane, offline from a warm
// state directory, with the impact report and verified knowledge the
// wiring would pass. Opt-in:
//
//	RI_VALIDATE_STATE=/path/to/.ri go test ./internal/investigate/validate -run Groundtruth -v
//
// It asserts generic behaviour only — every record of every validator on
// every in-scope item conforms to the contract (ValidateAgainst), a pin only
// on a passing override inspection, nothing decisive without inputs — and
// logs the outcome table (it never compares with expected classes).

import (
	"context"
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
		ID   string `yaml:"id"`
		Root string `yaml:"root"`
	} `yaml:"environment"`
}

func TestGroundtruthBundles(t *testing.T) {
	state := os.Getenv("RI_VALIDATE_STATE")
	if state == "" {
		t.Skip("set RI_VALIDATE_STATE to a warm state directory (e.g. the primary checkout's .ri) to validate real bundles offline")
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
			change, err := bundle.NewChangeCollector(a, st).CollectChange(ctx, investigate.ChangeRequest{Product: in.Product, From: in.From.Version, To: in.To.Version, Render: true})
			if err != nil {
				t.Skipf("change evidence not available offline: %v", err)
			}
			snap, err := bundle.NewSnapshotBuilderWithHints(st, env.HintsFromCatalog(a.Catalog)).BuildSnapshot(ctx, investigate.EnvironmentInputs{Name: in.Environment.ID, Repo: envDir})
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
			tr, err := relations.NewTraverser().Traverse(ctx, g, roots, domain.DefaultTraversalBound)
			if err != nil {
				t.Fatal(err)
			}
			vals := []investigate.Validator{JoinVerdicts(rep, WithStore(st)), RenderDiff(WithStore(st)),
				OverrideInspection(WithStore(st), WithFacts(facts...)), SchemaCompat(WithStore(st)), StaticReference(WithStore(st))}
			counts := map[string]int{}
			var lines []string
			for _, it := range b.Change.Items {
				if !it.InScope {
					continue
				}
				for _, v := range vals {
					if !v.Applies(it.Family) {
						continue
					}
					out := run(t, v, investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, Report: rep, ChangeItem: it.ID})
					for _, r := range out.Records {
						counts[fmt.Sprintf("%s %s", v.Name(), r.Outcome)]++
						if r.Outcome.Decisive() || os.Getenv("RI_VALIDATE_VERBOSE") != "" {
							lines = append(lines, fmt.Sprintf("%-32s %-4s %-20s %s — %s", v.Name(), r.Outcome, it.Manifestation, trim(it.Title, 60), trim(strings.Join(strings.Fields(r.Detail), " "), 400)))
						}
					}
				}
			}
			var keys []string
			for k := range counts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				t.Logf("%-48s %d", k, counts[k])
			}
			for _, l := range lines {
				t.Log(l)
			}
		})
	}
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(s, "\n")
	return s
}
