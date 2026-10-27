package orchestrate_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

// The synthesis activity runs after classify and before the review when the
// input asks for it; the record matches the in-process runner's (findings,
// decision and judgement), and without the flag no synthesis runs.
func TestWorkflowSynthesis(t *testing.T) {
	for _, on := range []bool{true, false} {
		h := newHarness(t)
		client, _ := runtest.SynthClient(t)
		h.f.Deps.Synthesizer = runtest.Synth{Client: client}
		h.signalAt(2*time.Hour, runtest.Human(investigate.ReviewApprove))
		in := h.input()
		in.Synthesize = on
		h.start(in)
		res := h.result(t)
		if res.Outcome != run.OutcomeApproved {
			t.Fatalf("result = %+v", res.Result)
		}
		inv, err := h.f.St.GetInvestigation(context.Background(), res.Investigation)
		if err != nil {
			t.Fatal(err)
		}
		synthAudit := 0
		for _, e := range res.Audit {
			if e.Stage == string(domain.StageSynthesize) {
				synthAudit++
			}
		}
		if !on {
			if inv.Judgement != nil || synthAudit != 0 {
				t.Errorf("synthesis ran without the flag")
			}
			continue
		}
		if inv.Judgement == nil || inv.Judgement.Status != domain.JudgementComplete || synthAudit != 2 {
			t.Fatalf("judgement %v, %d workflow audit entries", inv.Judgement != nil, synthAudit)
		}
		// the same request in-process gives the same findings, decision and judgement
		f := runtest.New(t)
		c2, _ := runtest.SynthClient(t)
		f.Deps.Synthesizer = runtest.Synth{Client: c2}
		r := &run.Runner{P: run.New(f.Deps), Synthesize: true, Sleep: func(time.Duration) {}}
		ires, err := r.Run(context.Background(), f.Request())
		if err != nil {
			t.Fatal(err)
		}
		local, _ := f.St.GetInvestigation(context.Background(), ires.Investigation)
		for what, pair := range map[string][2]any{"findings": {inv.Findings, local.Findings}, "decision": {inv.Decision, local.Decision}, "judgement": {inv.Judgement, local.Judgement}} {
			a, _ := json.Marshal(pair[0])
			b, _ := json.Marshal(pair[1])
			if string(a) != string(b) {
				t.Errorf("%s differ between Temporal and in-process", what)
			}
		}
	}
}

// A synthesis without a judgement is audited and the workflow continues to
// the review.
func TestWorkflowSynthesisFailureContinues(t *testing.T) {
	h := newHarness(t)
	h.signalAt(2*time.Hour, runtest.Human(investigate.ReviewApprove))
	in := h.input()
	in.Synthesize = true // no synthesizer configured: a permanent activity error
	h.start(in)
	res := h.result(t)
	failed := false
	for _, e := range res.Audit {
		failed = failed || (e.Stage == string(domain.StageSynthesize) && e.Status == "failed")
	}
	if res.Outcome != run.OutcomeApproved || !failed {
		t.Fatalf("outcome %s, failed entry %v", res.Outcome, failed)
	}
}
