package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// exportRun is a v6 run with one well-formed and one malformed mitigation.
func exportRun(t *testing.T, f *widgetFixture) *domain.Investigation {
	t.Helper()
	script := f.itemScript()
	script[f.item] = append(script[f.item], Step{Call: "call-w", Tool: "submit_findings", Args: submitArgs(map[string]any{"changeItem": f.item,
		"disposition": "claims-mitigation", "summary": "explicitly set widgets keep their storage", "citations": f.upstreamCite(),
		"verdict": map[string]any{"class": "potentially-affected", "confidence": "low", "mechanism": "default flips", "environmentReasoning": "one widget inherits it",
			"citations": f.upstreamCite(), "wouldOverturn": "every widget sets it"}})})
	d := &Scripted{Model: "test-model", ModelVersion: "test-model-1", Provider: "test", ByItem: script}
	out := widgetRun(t, f, d)
	for _, r := range d.Results {
		if r.Err != nil {
			t.Fatalf("%s: %v", r.Tool, r.Err)
		}
	}
	inv := widgetRecord(t, f, out)
	must(t, inv.ValidateAgainst(f.b))
	return inv
}

func TestExportHypotheses(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	inv := exportRun(t, f)
	ctx := context.Background()
	ks := knowledge.NewFileStore(t.TempDir())
	res, err := ExportHypotheses(ctx, inv, f.b, f.st, ks, fixedNow())
	must(t, err)
	if len(res) != 2 {
		t.Fatalf("results: %+v", res)
	}
	var exp, skip ExportResult
	for _, r := range res {
		if r.Status == ExportExported {
			exp = r
		} else {
			skip = r
		}
	}
	if exp.Proposal == "" || exp.ReviewItem == "" || exp.Candidate == "" || exp.RuleDigest == "" {
		t.Fatalf("exported: %+v", exp)
	}
	if skip.Status != ExportSkipped || !strings.Contains(skip.Reason, "not within this run's transition") {
		t.Fatalf("the malformed mitigation: %+v", skip)
	}
	snap, err := ks.Load(ctx, knowledge.Query{})
	must(t, err)
	if len(snap.Facts) != 0 || len(snap.Decisions) != 0 {
		t.Fatal("export wrote a fact or a decision: a hypothesis is never knowledge by itself")
	}
	if len(snap.Proposals) != 1 || len(snap.ReviewItems) != 1 || len(snap.Candidates) != 1 {
		t.Fatalf("records: %d proposals, %d items, %d candidates", len(snap.Proposals), len(snap.ReviewItems), len(snap.Candidates))
	}
	p, it, c := snap.Proposals[0], snap.ReviewItems[0], snap.Candidates[0]
	hyp, run, ok := domain.ParseHypothesisRule(p.Provenance.Rule)
	if !ok || run != inv.ID || hyp != exp.Hypothesis || p.Provenance.Method != domain.MethodAI || p.Task != domain.TaskFull {
		t.Fatalf("proposal provenance: %+v", p.Provenance)
	}
	must(t, p.ValidateAgainst(c))
	var h domain.Hypothesis
	for _, x := range inv.Hypotheses {
		if x.ID == hyp {
			h = x
		}
	}
	if it.QuestionType != domain.QuestionFactReview || it.Status != domain.ReviewPending || it.Proposed.Digest() != h.Mitigation.ProposedRule.Assertion.Digest() ||
		len(it.Proposals) != 1 || it.Proposals[0] != p.ID || it.Release != "v1.5.0" {
		t.Fatalf("review item: %+v", it)
	}
	for _, e := range c.Evidence {
		if e.Kind == domain.EvidenceLocalFile {
			t.Fatal("environment evidence entered knowledge")
		}
	}
	// idempotent
	again, err := ExportHypotheses(ctx, inv, f.b, f.st, ks, fixedNow().Add(3600e9))
	must(t, err)
	for _, r := range again {
		if r.Hypothesis == exp.Hypothesis && (r.Status != ExportExisting || r.ReviewItem != exp.ReviewItem || r.Proposal != exp.Proposal) {
			t.Fatalf("re-export: %+v", r)
		}
	}
	snap2, err := ks.Load(ctx, knowledge.Query{})
	must(t, err)
	if len(snap2.Proposals) != 1 || len(snap2.ReviewItems) != 1 || len(snap2.Candidates) != 1 || len(snap2.Candidates[0].Evidence) != len(c.Evidence) {
		t.Fatal("re-export wrote new records")
	}
}

// A hypothesis whose citations are all environment files is never exported:
// knowledge is release-level.
func TestExportRefusesEnvironmentOnlyCitations(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	inv := exportRun(t, f)
	for i := range inv.Hypotheses {
		if inv.Hypotheses[i].Mitigation != nil {
			var env []domain.EvidenceID
			for _, e := range inv.Evidence {
				if e.Kind == domain.EvidenceLocalFile {
					env = append(env, e.ID)
				}
			}
			if len(env) == 0 {
				t.Fatal("fixture: no environment evidence in the run")
			}
			inv.Hypotheses[i].Citations = env
		}
	}
	res, err := ExportHypotheses(context.Background(), inv, f.b, f.st, knowledge.NewFileStore(t.TempDir()), fixedNow())
	must(t, err)
	for _, r := range res {
		if r.Status != ExportSkipped {
			t.Fatalf("exported with environment-only citations: %+v", r)
		}
	}
}
