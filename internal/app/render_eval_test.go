package app

// The render evaluation cases (eval/render/, R16/R17): expectations authored
// from upstream sources before the renderer ran on them, compared here
// against a live render. This test is the render-scoped part of R17's
// metrics: render success rate, precision and recall per case. It needs helm
// on PATH and network (charts are fetched through the product's channels) —
// it skips with a clear message otherwise. It never writes to eval/render/.

import (
	"context"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/render"
)

// evalRenderCase is one case.yaml (the fields the runner matches on).
type evalRenderCase struct {
	ID       string            `yaml:"id"`
	Product  string            `yaml:"product"`
	From     string            `yaml:"from"`
	To       string            `yaml:"to"`
	Level    string            `yaml:"level"` // release | environment
	Repo     string            `yaml:"repo"`
	Expected []evalExpectation `yaml:"expected"`
	Variants []evalVariant     `yaml:"variants"`
}

type evalExpectation struct {
	ID       string `yaml:"id"`
	Category string `yaml:"category"`
	Expect   string `yaml:"expect"` // present | absent
	Kind     string `yaml:"kind"`
	Name     string `yaml:"name"`
	Class    string `yaml:"class"`
	Path     string `yaml:"path"`
	Target   string `yaml:"target"`  // environment cases: target kind
	Failure  string `yaml:"failure"` // expected pair failure reason
	// KnownGap marks an expectation the authoring sources could not decide
	// (e.g. a CRD field, visible only if the renderer includes CRDs): a miss
	// is reported, not failed.
	KnownGap bool   `yaml:"knownGap"`
	Detail   string `yaml:"detail"`
}

type evalVariant struct {
	ID       string            `yaml:"id"`
	Values   string            `yaml:"values"`
	Expected []evalExpectation `yaml:"expected"`
}

// TestEvalRenderCases runs every eval/render case against a live render and
// reports precision/recall; a present expectation that no rendered change
// satisfies (and no failed render explains) fails the test.
func TestEvalRenderCases(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not on PATH; the render evaluation needs the real renderer")
	}
	cases := loadEvalRenderCases(t)
	if len(cases) == 0 {
		t.Fatal("no cases under eval/render/")
	}
	for _, c := range cases {
		t.Run(c.ID, func(t *testing.T) {
			a := newRenderEvalApp(t)
			opts := RenderOptions{KubeVersion: "1.31"}
			if c.Level == "environment" && c.Repo != "" {
				opts.Repo = filepath.Join("..", "..", c.Repo)
			} else {
				opts.Release = true
			}
			res, err := a.RenderDiff(context.Background(), c.Product, c.From, c.To, opts)
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			pairs := collectEvalPairs(t, res, c)
			for _, p := range pairs {
				if p.Status != render.PairOK {
					if p.Failure == nil || p.Failure.Reason == render.FailChartUnavailable || p.Failure.Reason == render.FailRendererUnavailable {
						t.Skipf("%s: %v — the evaluation needs fetchable charts and tools", p.Target.ID, p.Failure)
					}
				}
			}
			scoreEvalExpectations(t, c.Expected, pairs, c.ID, c.Level == "release")
			for _, v := range c.Variants {
				vres, err := a.RenderDiff(context.Background(), c.Product, c.From, c.To, RenderOptions{
					KubeVersion: "1.31", ValuesFiles: []string{filepath.Join("..", "..", "eval", "render", c.ID, v.Values)}})
				if err != nil {
					t.Fatalf("variant %s: %v", v.ID, err)
				}
				scoreEvalExpectations(t, v.Expected, collectEvalPairs(t, vres, c), c.ID+" / "+v.ID, false)
			}
		})
	}
}

// collectEvalPairs returns the pairs the case is about (release level: the
// chart-default pair; environment: every detected pair).
func collectEvalPairs(t *testing.T, res *RenderDiffResult, c evalRenderCase) []*render.Pair {
	t.Helper()
	if res.Release != nil {
		return []*render.Pair{res.Release}
	}
	if len(res.Pairs) == 0 {
		t.Fatalf("no render targets detected for %s", c.ID)
	}
	return res.Pairs
}

// scoreEvalExpectations matches expectations against the pairs' changes (and,
// for environment cases, their failures) and reports the R17 metrics.
// coversDelta says the expectation set aims at the whole rendered delta (the
// release-level case): only then is precision meaningful.
func scoreEvalExpectations(t *testing.T, exps []evalExpectation, pairs []*render.Pair, label string, coversDelta bool) {
	t.Helper()
	rendered, failed := 0, 0
	var changes []render.Change
	for _, p := range pairs {
		if p.Status == render.PairOK {
			rendered++
			if p.Diff != nil {
				changes = append(changes, p.Diff.Changes...)
			}
			continue
		}
		failed++
	}
	matched := map[string]bool{}
	matchedChanges := map[int]bool{}
	for i, ch := range changes {
		for _, e := range exps {
			if e.Expect == "present" && changeSatisfies(e, ch) {
				matched[e.ID] = true
				matchedChanges[i] = true
			}
		}
	}
	var misses []string
	for _, e := range exps {
		switch e.Expect {
		case "present":
			if e.Failure != "" { // a present failure expectation: the pair must have failed that way
				ok := false
				for _, p := range pairs {
					if p.Target.Kind == render.TargetKind(e.Target) && p.Failure != nil && string(p.Failure.Reason) == e.Failure {
						ok = true
					}
				}
				if !ok {
					misses = append(misses, e.ID+" ("+e.Category+")")
					continue
				}
				matched[e.ID] = true
			} else if !matched[e.ID] {
				if e.KnownGap {
					t.Logf("KNOWN GAP %s: %s — no rendered change satisfies it", e.ID, e.Detail)
					continue
				}
				misses = append(misses, e.ID+" ("+e.Category+")")
			}
		case "absent":
			ok := true
			for _, ch := range changes {
				if changeSatisfies(e, ch) {
					ok = false
				}
			}
			if !ok {
				misses = append(misses, e.ID+" (absent-"+e.Category+")")
			} else {
				matched[e.ID] = true
			}
		}
	}
	total := len(exps)
	if coversDelta {
		// precision counts only where the expectation set aims at the whole
		// delta; a variant adds two expectations over fifteen changes and
		// would report nonsense.
		t.Logf("%s: pairs rendered %d, failed %d; changes %d; expectations matched %d/%d; precision (changes explained) %d/%d",
			label, rendered, failed, len(changes), len(matched), total, len(matchedChanges), len(changes))
	} else {
		t.Logf("%s: pairs rendered %d, failed %d; changes %d; expectations matched %d/%d",
			label, rendered, failed, len(changes), len(matched), total)
	}
	if len(misses) > 0 {
		t.Errorf("recall: unmatched expectations: %s", strings.Join(misses, ", "))
	}
}

// changeSatisfies reports whether a rendered change satisfies an expectation's
// non-empty fields (kind, exact object name or glob, class, path substring).
// The path filter also looks at the change's subject name and its before/after
// values: upstream sources speak of arg flags and value keys ("targetPort")
// that the diff encodes in Name/Before/After, not in the path itself.
func changeSatisfies(e evalExpectation, ch render.Change) bool {
	if e.Kind != "" && ch.Object.Kind != e.Kind {
		return false
	}
	if e.Name != "" {
		if strings.ContainsAny(e.Name, "*?") {
			if ok, _ := filepath.Match(e.Name, ch.Object.Name); !ok {
				return false
			}
		} else if ch.Object.Name != e.Name {
			return false
		}
	}
	if e.Class != "" && string(ch.Class) != e.Class {
		return false
	}
	if e.Path != "" {
		in := ch.Path + " " + ch.Pattern + " " + ch.Name
		if ch.Before != nil {
			in += " " + *ch.Before
		}
		if ch.After != nil {
			in += " " + *ch.After
		}
		if !strings.Contains(in, e.Path) {
			return false
		}
	}
	return true
}

func loadEvalRenderCases(t *testing.T) []evalRenderCase {
	t.Helper()
	dir := filepath.Join("..", "..", "eval", "render")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Skipf("eval/render is not present: %v", err)
	}
	var out []evalRenderCase
	for _, en := range entries {
		if !en.IsDir() || en.Name() == "results" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, en.Name(), "case.yaml"))
		if err != nil {
			t.Fatalf("read %s: %v", en.Name(), err)
		}
		var c evalRenderCase
		if err := yaml.Unmarshal(b, &c); err != nil {
			t.Fatalf("parse %s: %v", en.Name(), err)
		}
		out = append(out, c)
	}
	return out
}

// newRenderEvalApp builds an app over a scratch copy of the e2e recording
// (its cache replays most fetches; missing pieces are fetched live).
func newRenderEvalApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	if err := copyTree(filepath.Join(e2eDir, "state"), dir, func(string, fs.DirEntry) bool { return false }); err != nil {
		t.Fatal(err)
	}
	a, err := New(Config{ProductsDir: productsDir, StateDir: dir, GitHubToken: GitHubTokenFromEnv(), Logf: t.Logf})
	if err != nil {
		t.Fatal(err)
	}
	return a
}
