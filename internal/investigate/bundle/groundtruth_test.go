package bundle_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/bundle"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

// groundtruthInput is the subset of eval/investigations/<case>/input.yaml the
// collectors need. Only input.yaml is ever read (FLEET.md "Evaluation
// integrity"): never expected.yaml, NOTES.md or observed/.
type groundtruthInput struct {
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

// warmState locates a warm upstream cache: RI_STATE, or the primary
// checkout's .ri. The test skips when none exists (no network is used: the
// app runs offline).
func warmState(t *testing.T) string {
	t.Helper()
	for _, p := range []string{os.Getenv("RI_STATE"), "/Users/tommydavison/repos/Release-Intelligence-/.ri"} {
		if p == "" {
			continue
		}
		if fi, err := os.Stat(filepath.Join(p, "cache")); err == nil && fi.IsDir() {
			return p
		}
	}
	t.Skip("no warm upstream cache (set RI_STATE to a state dir populated by `ri upgrade`)")
	return ""
}

func loadCases(t *testing.T) []groundtruthInput {
	t.Helper()
	root := filepath.Join("..", "..", "..", "eval", "investigations")
	dirs, _ := filepath.Glob(filepath.Join(root, "*", "input.yaml"))
	if len(dirs) == 0 {
		t.Skip("no groundtruth cases")
	}
	var out []groundtruthInput
	for _, f := range dirs {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var in groundtruthInput
		if err := yaml.Unmarshal(raw, &in); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		in.Environment.Root = filepath.Join(filepath.Dir(f), in.Environment.Root)
		out = append(out, in)
	}
	return out
}

func buildBundle(t *testing.T, a *app.App, st *store.Store, in groundtruthInput, render bool) *domain.EvidenceBundle {
	t.Helper()
	ctx := context.Background()
	cc := bundle.NewChangeCollector(a, st)
	change, err := cc.CollectChange(ctx, investigate.ChangeRequest{Product: in.Product, From: in.From.Version, To: in.To.Version, Render: render})
	if err != nil {
		t.Fatalf("%s: change: %v", in.ID, err)
	}
	snap, err := bundle.NewSnapshotBuilderWithHints(st, env.HintsFromCatalog(a.Catalog)).BuildSnapshot(ctx, investigate.EnvironmentInputs{Name: in.Environment.ID, Repo: in.Environment.Root})
	if err != nil {
		t.Fatalf("%s: snapshot: %v", in.ID, err)
	}
	b, err := bundle.NewAssembler(st).Assemble(ctx, change, snap, nil)
	if err != nil {
		t.Fatalf("%s: assemble: %v", in.ID, err)
	}
	return b
}

// TestGroundtruthInputsBundle: a bundle for every groundtruth case's
// input.yaml validates, is storable, and a second run yields the same id.
func TestGroundtruthInputsBundle(t *testing.T) {
	state := warmState(t)
	a, err := app.New(app.Config{ProductsDir: filepath.Join("..", "..", "..", "products"), StateDir: state, Offline: true})
	if err != nil {
		t.Fatal(err)
	}
	_, helmErr := exec.LookPath("helm")
	for _, in := range loadCases(t) {
		t.Run(in.ID, func(t *testing.T) {
			st, err := store.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			b := buildBundle(t, a, st, in, helmErr == nil)
			if err := b.Validate(); err != nil {
				t.Fatal(err)
			}
			if err := st.PutBundle(context.Background(), b); err != nil {
				t.Fatal(err)
			}
			for _, k := range domain.ChangeSourceKinds {
				s, _ := b.Change.SourceStatus(k)
				t.Logf("source %-18s %-14s %d artifact(s) %s", k, s.Status, len(s.Artifacts), s.Reason)
			}
			t.Logf("bundle %s: %d items (%d in scope), %d artifacts, %d evidence", b.ID, len(b.Change.Items), len(b.Change.InScope()), len(b.Manifest.Artifacts), len(b.Evidence))
			if testing.Verbose() {
				for _, it := range b.Change.Items {
					if it.InScope {
						t.Logf("  in-scope %s %-10s %-22s %-34s %-32s %s", it.ID, it.Family, it.ScopeRule, it.ManifestationRule, it.Manifestation, it.Title)
					}
				}
				for _, p := range b.Change.Renders {
					n, corr := 0, 0
					for _, c := range p.Changes {
						n++
						if len(c.ChangeItems) > 0 {
							corr++
						}
					}
					t.Logf("  release render %s: %s, %d changes (%d correlated) %s", p.Target, p.Status, n, corr, p.Reason)
				}
			}
			// determinism: a second, independent run over a fresh store
			st2, _ := store.New(t.TempDir())
			if b2 := buildBundle(t, a, st2, in, helmErr == nil); b2.ID != b.ID {
				t.Fatalf("two runs, two bundle ids: %s vs %s", b.ID, b2.ID)
			}
		})
	}
}
