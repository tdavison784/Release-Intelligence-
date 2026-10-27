package run_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

func runRecord(t *testing.T, synthesize bool, review *investigate.ReviewDecision) (*domain.Investigation, *run.Result) {
	t.Helper()
	f := runtest.New(t)
	if synthesize {
		client, _ := runtest.SynthClient(t)
		f.Deps.Synthesizer = runtest.Synth{Client: client}
	}
	r := runner(f, nil)
	if review != nil {
		r.Review = once(review)
	}
	r.Synthesize = synthesize
	res, err := r.Run(context.Background(), f.Request())
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.St.GetInvestigation(context.Background(), res.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := f.St.GetBundle(context.Background(), inv.Bundle)
	if err := inv.ValidateAgainst(b); err != nil {
		t.Fatalf("record does not validate: %v", err)
	}
	return inv, res
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Policy isolation: findings, decision and review are byte-identical with
// and without synthesis (before and after a human decision), and only the
// judgement and its audit events are added.
func TestSynthesisNeverChangesFindingsDecisionOrReview(t *testing.T) {
	for _, review := range []*investigate.ReviewDecision{nil, runtest.Human(investigate.ReviewApprove)} {
		without, _ := runRecord(t, false, review)
		with, res := runRecord(t, true, review)
		if without.ID != with.ID {
			t.Fatalf("synthesis changed the investigation id (%s vs %s)", without.ID, with.ID)
		}
		for what, pair := range map[string][2]any{
			"findings": {without.Findings, with.Findings}, "decision": {without.Decision, with.Decision},
			"review": {without.Review, with.Review}, "validations": {without.Validations, with.Validations},
			"agent": {without.AgentVerdicts, with.AgentVerdicts},
		} {
			if mustJSON(t, pair[0]) != mustJSON(t, pair[1]) {
				t.Errorf("%s differ with synthesis", what)
			}
		}
		if without.Judgement != nil || with.Judgement == nil {
			t.Fatalf("judgement without %v, with %v", without.Judgement != nil, with.Judgement != nil)
		}
		j := with.Judgement
		if j.Status != domain.JudgementComplete || j.Recommendation != domain.RecommendValidationRequired || j.Investigation != with.ID {
			t.Errorf("judgement = %s %s for %s", j.Status, j.Recommendation, j.Investigation)
		}
		// the fixture has no advisory assessment: an explicit gap, and no
		// version is safe to name
		gap := false
		for _, f := range j.Findings {
			gap = gap || f.Kind == domain.JFSecurityIncomplete
		}
		if !gap || j.Target.Minimum != nil {
			t.Errorf("findings %+v / minimum %v", j.Findings, j.Target.Minimum)
		}
		var synth []string
		for _, e := range with.Audit {
			if e.Stage == domain.StageSynthesize {
				synth = append(synth, e.Event)
			}
		}
		if mustJSON(t, synth) != `["started","completed"]` || len(with.Audit) != len(without.Audit)+2 {
			t.Errorf("synthesis audit = %v (%d vs %d events)", synth, len(with.Audit), len(without.Audit))
		}
		if res.RecordDigest != run.Digest(with) && review == nil {
			t.Error("the result's record digest is not the record's")
		}
	}
}

// The synthesis is idempotent per record: a second call is a memo hit (no
// model call), and a missing synthesizer or an unclassified run is refused.
func TestSynthesizeIsIdempotentAndGuarded(t *testing.T) {
	f := runtest.New(t)
	client, fake := runtest.SynthClient(t)
	f.Deps.Synthesizer = runtest.Synth{Client: client}
	p := run.New(f.Deps)
	res, err := (&run.Runner{P: p}).Run(context.Background(), f.Request())
	if err != nil {
		t.Fatal(err)
	}
	in := run.SynthesizeInput{Investigation: res.Investigation}
	a, err := p.Synthesize(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.Synthesize(context.Background(), in)
	if err != nil || a != b || len(fake.Requests()) != 1 {
		t.Errorf("second call: %v, %+v vs %+v, %d model calls", err, a, b, len(fake.Requests()))
	}
	f.Deps.Synthesizer = nil
	if _, err := run.New(f.Deps).Synthesize(context.Background(), in); err == nil || investigate.ClassOf(err) != investigate.Permanent {
		t.Errorf("no synthesizer: %v", err)
	}
}

type failingSynth struct{}

func (failingSynth) Synthesize(context.Context, run.SynthesisInput) (*domain.UpgradeJudgement, error) {
	return nil, errors.New("exchange pending")
}

// A synthesis without a judgement is audited as failed and the run goes on
// to the review.
func TestFailedSynthesisContinuesToReview(t *testing.T) {
	f := runtest.New(t)
	f.Deps.Synthesizer = failingSynth{}
	r := runner(f, once(runtest.Human(investigate.ReviewApprove)))
	r.Synthesize = true
	res, err := r.Run(context.Background(), f.Request())
	if err != nil || res.Outcome != run.OutcomeApproved {
		t.Fatalf("run = %+v, %v", res, err)
	}
	inv, _ := f.St.GetInvestigation(context.Background(), res.Investigation)
	failed := false
	for _, e := range inv.Audit {
		failed = failed || (e.Stage == domain.StageSynthesize && e.Event == "failed")
	}
	if inv.Judgement != nil || !failed {
		t.Errorf("judgement %v, failed event %v", inv.Judgement != nil, failed)
	}
}
