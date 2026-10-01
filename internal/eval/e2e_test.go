package eval

// The ONE e2e test of the evaluator: the real dataset entry
// cert-manager-1.17-1.18 (edge + environment join) scored against the
// recorded upstream data of internal/app's e2e fixture, replayed completely
// offline with a fixed clock — the same mechanism as the e2e tests of
// internal/app. The full dataset (9 entries, 8 products) needs network and
// runs via `ri eval`; results and analysis live in eval/REPORT.md.
//
//	go test ./internal/eval                replay + score
//
// The test asserts mechanics (the entry runs, the scoring is internally
// consistent, the headline cert-manager items are found), never exact recall
// numbers: those are the dataset's job (eval/results + REPORT.md), and a
// test that pinned them would just duplicate the stored results.

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/fetch"
)

const (
	replayState  = "../../internal/app/testdata/e2e/state"
	replayProds  = "../../products"
	replayCaseID = "cert-manager-1.17-1.18"
)

var replayNow = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestE2EReplayedEntry(t *testing.T) {
	if _, err := os.Stat(replayState); err != nil {
		t.Fatalf("recording not found: %v", err)
	}
	// Private copy of the recording; empty PATH so any git/network use
	// fails loudly instead of silently going online.
	state := t.TempDir()
	if err := copyTreeForReplay(replayState, state); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	a, err := app.New(app.Config{
		ProductsDir: replayProds,
		StateDir:    state,
		Offline:     true,
		Now:         func() time.Time { return replayNow },
	})
	if err != nil {
		t.Fatal(err)
	}
	if hc, ok := a.Fetch.(*fetch.HTTPClient); ok {
		hc.HTTP = &http.Client{Transport: replayTripwire{t}}
	}

	r := &Runner{Pipeline: appPipelineForTest{a}, CasesDir: datasetRoot}
	cases, err := r.Load([]string{replayCaseID})
	if err != nil {
		t.Fatal(err)
	}
	results := r.Run(context.Background(), cases)
	if len(results) != 1 {
		t.Fatalf("results = %d", len(results))
	}
	res := results[0]
	if res.Error != "" {
		t.Fatalf("entry errored: %s", res.Error)
	}

	// scoring is internally consistent
	m := res.Metrics
	if m.Found+m.MissedCritical+m.MissedImportant+m.MissedMinor != m.Expected {
		t.Errorf("found+missed != expected: %+v", m)
	}
	if m.Changes == 0 || m.MatchedChanges == 0 {
		t.Errorf("edge produced no matchable output: %+v", m)
	}

	// the headline 1.18 changes (ground truth E1/E2) must be found by the
	// recorded edge: both are in the upstream upgrade guide, which the
	// definition ingests. This is a floor, not the metric.
	for _, id := range []string{"E1", "E2"} {
		am := matchByID(res.Matches, id)
		if am == nil || !am.Found {
			t.Errorf("%s (headline cert-manager 1.18 change) not found by the recorded edge", id)
		}
	}

	// the environment join ran and produced findings
	if res.Env == nil {
		t.Fatal("no environment metrics")
	}
	if res.Env.Findings == 0 {
		t.Error("impact join produced no findings for the fixture environment")
	}

	// audit trail is populated for both hits and misses
	for _, am := range res.Matches {
		if am.Found && len(am.Hits) == 0 {
			t.Errorf("%s: found without audit hits", am.ExpectedID)
		}
		if am.Found {
			for _, h := range am.Hits {
				if h.ChangeID == "" || h.MatchedBy == "" || h.Evidence == "" {
					t.Errorf("%s hit %+v lacks change/matcher/evidence", am.ExpectedID, h)
				}
			}
		}
	}
}

// appPipelineForTest adapts app.App the same way cmd/ri does; it lives here
// (not in api.go) because it is test plumbing, not evaluator API.
type appPipelineForTest struct{ a *app.App }

func (p appPipelineForTest) Upgrade(ctx context.Context, product, from, to string) (*domain.UpgradeEdge, error) {
	return p.a.Upgrade(ctx, product, from, to, app.UpgradeOptions{})
}

func (p appPipelineForTest) Impact(ctx context.Context, product, from, to string, inputs env.Inputs) (*domain.ImpactReport, error) {
	return p.a.Impact(ctx, product, from, to, app.ImpactOptions{Environment: inputs})
}

// replayTripwire fails the test on network access.
type replayTripwire struct{ t *testing.T }

func (w replayTripwire) RoundTrip(r *http.Request) (*http.Response, error) {
	w.t.Errorf("offline replay attempted network access: %s %s", r.Method, r.URL)
	return nil, errors.New("network access in offline replay")
}

func copyTreeForReplay(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dst, 0o755)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
}
