package reportui

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

// A run with an upgrade judgement shows it, labelled advisory, on the page
// and in the self-contained export; a run without one shows no section.
func TestJudgementSection(t *testing.T) {
	f := runtest.New(t)
	client, _ := runtest.SynthClient(t)
	f.Deps.Synthesizer = runtest.Synth{Client: client}
	res, err := (&run.Runner{P: run.New(f.Deps), Synthesize: true, Sleep: func(time.Duration) {}}).Run(context.Background(), f.Request())
	if err != nil {
		t.Fatal(err)
	}
	src, err := Open(filepath.Dir(f.St.Dir()))
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(src, Options{Reviewer: classify.NewReviewApplier(), Now: func() time.Time { return runtest.FixedNow.Add(time.Hour) }, Logf: t.Logf})
	for _, target := range []string{"/investigations/view/" + res.Investigation, "/investigations/export/" + res.Investigation + "?format=html"} {
		w := get(t, s.handler, target)
		if w.Code != http.StatusOK {
			t.Fatalf("%s: %d", target, w.Code)
		}
		body := w.Body.String()
		for _, want := range []string{`id="judgement"`, "AI JUDGEMENT — advisory", "validation-required", "disagreements and hard findings", "── backout ──"} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %q", target, want)
			}
		}
	}
	plain, _, id := liveServer(t)
	if body := get(t, plain.handler, "/investigations/view/"+id).Body.String(); strings.Contains(body, `id="judgement"`) {
		t.Error("a run without a judgement shows the section")
	}
}
