package renderdiff

// The groundtruth environments (eval/investigations/<case>/input.yaml and
// env/ — never expected.yaml, NOTES.md or observed/), through the real
// pipeline: lane bundle's change collector and snapshot builder, this
// renderer, the bundle assembler — helm and kubectl on PATH, upstream charts
// and releases served OFFLINE from a warm state directory. Opt-in, because
// the checked-in fixtures do not carry the charts:
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
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

const groundtruthDir = "../../../eval/investigations"

// gtInput is the part of input.yaml the pipeline needs.
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
			st, err := store.New(t.TempDir())
			must(t, err)
			change, err := bundle.NewChangeCollector(a, st).CollectChange(ctx, investigate.ChangeRequest{Product: in.Product, From: in.From.Version, To: in.To.Version, Render: true})
			if err != nil {
				t.Skipf("change evidence not available offline from %s: %v", state, err)
			}
			snap, err := bundle.NewSnapshotBuilderWithHints(st, env.HintsFromCatalog(a.Catalog)).BuildSnapshot(ctx, investigate.EnvironmentInputs{Name: in.Environment.ID, Repo: envDir})
			must(t, err)
			r := New(a, st)
			cfg := investigate.Config{Render: true, KubeVersion: "1.31"}
			res, err := r.RenderEnvironment(ctx, change, snap, cfg)
			must(t, err)
			b, err := bundle.NewAssembler(st).Assemble(ctx, change, snap, res)
			if err != nil {
				t.Fatalf("assemble: %v", err)
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
				t.Logf("%-60s %-14s changes=%-3d %s", p.Target, p.Status, len(p.Changes), trunc(p.Reason, 140))
			}
			for _, n := range res.Notes {
				t.Logf("note: %s", n)
			}
			items := map[string]domain.ChangeItem{}
			for _, it := range b.Items() {
				items[it.ID] = it
			}
			byRule := map[string]int{}
			for _, p := range res.Pairs {
				for _, c := range p.Changes {
					rule := "uncorrelated"
					for _, id := range c.ChangeItems {
						if it := items[id]; it.RenderedChange != "" {
							rule = it.ScopeRule // anchored: on itself (environment) or on its release counterpart
						} else if rule == "uncorrelated" || rule == "explained" {
							rule = "explained"
						}
					}
					byRule[rule]++
					if os.Getenv("RI_RENDERDIFF_DUMP") != "" {
						t.Logf("  %-28s %s %s %s", rule, c.Class, c.Object, c.Path)
					}
				}
			}
			m := Measure(res.Pairs)
			var reasons []string
			for k, v := range m.Reasons {
				reasons = append(reasons, fmt.Sprintf("%s×%d", k, v))
			}
			sort.Strings(reasons)
			table = append(table, fmt.Sprintf("| %s | %d | %d | %d | %d | %d | %s | %d | %d | %d | %d | %d | %d | %d |", in.ID, m.Targets, m.OK, m.Partial,
				m.Unavailable, m.NotApplicable, strings.Join(reasons, ", "), m.Changes, byRule["explained"], byRule[domain.ScopeRenderedStructural],
				byRule[domain.ScopeRoutineSelfArtifact], byRule[domain.ScopeRenderedUnexplained], len(res.Items), len(b.Uncorrelated())))
		})
	}
	t.Log("\n| case | targets | ok | partial | unavailable | not-applicable | non-ok reasons | env rendered changes | explained by an upstream change | structural item | self-artifact item | unexplained item | environment items | uncorrelated |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n" + strings.Join(table, "\n"))
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
