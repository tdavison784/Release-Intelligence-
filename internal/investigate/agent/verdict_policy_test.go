package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/classify"
)

// A provisional verdict is never a policy input (PROTOCOL §8): findings,
// decision and approval eligibility are byte-identical with and without
// verdicts — including an adversarial not-affected verdict on an item a
// failing validation makes affected, and on an item the policy leaves
// undecided. The opposite (fail-open) implementation — a classifier that
// lets a not-affected verdict lower a class or count as a validation — fails
// this test.
func TestVerdictsNeverReachThePolicy(t *testing.T) {
	f := newWidgetFixture(t, widgetOpts{})
	ctx := context.Background()
	var rcEv domain.EvidenceID
	for _, p := range f.b.Renders {
		for _, c := range p.Changes {
			if c.ID == f.rc {
				rcEv = c.Evidence[0]
			}
		}
	}
	fail := domain.ValidationRecord{Validator: "fake.reference@v1", Kind: domain.ValidationStaticReference, Level: domain.LevelDeterministic,
		ChangeItem: f.other, Property: "every reference resolves at TO", Inputs: []string{f.toRender.ID}, Outcome: domain.ValidationFail,
		Detail: "a reference dangles at TO", Evidence: []domain.EvidenceID{rcEv},
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "fake.reference@v1", Confidence: domain.ConfidenceHigh}, CheckedAt: time.Now()}
	fail.ID = domain.ValidationRecordID(fail.Validator, fail.ChangeItem, nil, nil, nil, fail.Inputs)
	must(t, fail.Validate())

	// The agent's run: a not-affected verdict on both items.
	cite := f.upstreamCite()
	legacy := []map[string]any{{"artifact": f.notes.ID, "quote": "The values key `controller.legacyMode` was removed."}}
	adversarial := func(c []map[string]any) map[string]any {
		return map[string]any{"class": "not-affected", "confidence": "high", "mechanism": "nothing breaks",
			"environmentReasoning": "trust me", "citations": c, "wouldOverturn": "nothing"}
	}
	d := &Scripted{Model: "test-model", ModelVersion: "v", ByItem: map[string][]Step{
		f.item: {{Call: "call-1", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.item,
			"disposition": "examined-no-concern", "summary": "fine", "citations": cite, "verdict": adversarial(cite)})}},
		f.other: {{Call: "call-2", Tool: investigate.ToolSubmitFindings, Args: submitArgs(map[string]any{"changeItem": f.other,
			"disposition": "examined-no-concern", "summary": "fine", "citations": legacy, "verdict": adversarial(legacy)})}},
	}}
	out := widgetRun(t, f, d)
	if len(out.Verdicts) != 2 {
		t.Fatalf("verdicts: %+v (%v)", out.Verdicts, d.Results)
	}
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v6Config()}
	run := func(agent *investigate.AgentOutcome) []byte {
		cls, err := classify.New().Classify(ctx, investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: agent,
			Validations: append([]domain.ValidationRecord{fail}, out.Validations...), Config: cfg})
		must(t, err)
		b, err := json.Marshal(cls)
		must(t, err)
		return b
	}
	without := *out
	without.Verdicts = nil
	a, b := run(&without), run(out)
	if !bytes.Equal(a, b) {
		t.Fatalf("verdicts changed the classification:\nwithout: %s\nwith:    %s", a, b)
	}
	var cls investigate.Classification
	must(t, json.Unmarshal(b, &cls))
	if len(cls.Findings) != 2 {
		t.Fatalf("findings: %+v", cls.Findings)
	}
	for _, fd := range cls.Findings {
		switch fd.ChangeItem {
		case f.other:
			if fd.Class != domain.FindingAffected {
				t.Errorf("the failing validation's item is %s, want affected", fd.Class)
			}
		case f.item:
			if fd.Class == domain.FindingNotAffected || fd.Approval.Eligible {
				t.Errorf("an agent verdict cleared %s: %+v", fd.ChangeItem, fd)
			}
		}
	}
	if cls.Decision.Outcome == domain.PolicyAutoApproveEligible {
		t.Fatal("an agent verdict made the investigation approval-eligible")
	}
	// The stored run keeps both, independently.
	inv := widgetRecord(t, f, out, fail)
	inv.Findings, inv.Decision = cls.Findings, &cls.Decision
	if len(inv.AgentVerdicts) != 2 {
		t.Fatal("the run record lost the verdicts")
	}
}

// A natural assessment is never a policy input either (PLAN-M1 §4): under v9,
// an adversarial not-affected verdict with an assessment whose verified
// entries all resolve changes neither the findings, the decision nor approval
// eligibility, and the run record keeps the assessment beside the verdict.
func TestAssessmentsNeverReachThePolicy(t *testing.T) {
	f := newConsumerFixture(t)
	ctx := context.Background()
	confident := f.assessment()
	confident["likelyImpact"] = "none: nothing here depends on the key"
	_, out := v9Run(t, f, Step{Call: "call-9", Tool: investigate.ToolTraceConsumers, Args: map[string]any{"changeItem": f.item}},
		f.v9Submit("not-affected", confident))
	if len(out.Assessments) != 1 || len(out.Verdicts) != 1 || out.Verdicts[0].Class != domain.FindingNotAffected {
		t.Fatalf("assessments %+v verdicts %+v", out.Assessments, out.Verdicts)
	}
	cfg := investigate.Config{Policy: "classify.policy@v1", Bound: domain.DefaultTraversalBound, Agent: v9Config()}
	run := func(agent *investigate.AgentOutcome) []byte {
		cls, err := classify.New().Classify(ctx, investigate.ClassifyInput{Bundle: f.b, Traversal: f.trav, Agent: agent, Validations: out.Validations, Config: cfg})
		must(t, err)
		b, err := json.Marshal(cls)
		must(t, err)
		return b
	}
	without := *out
	without.Assessments, without.Verdicts = nil, nil
	if a, b := run(&without), run(out); !bytes.Equal(a, b) {
		t.Fatalf("assessments changed the classification:\nwithout: %s\nwith:    %s", a, b)
	}
	var cls investigate.Classification
	must(t, json.Unmarshal(run(out), &cls))
	for _, fd := range cls.Findings {
		if fd.Class == domain.FindingNotAffected || fd.Approval.Eligible {
			t.Errorf("an agent assessment cleared %s: %+v", fd.ChangeItem, fd)
		}
	}
	if cls.Decision.Outcome == domain.PolicyAutoApproveEligible {
		t.Fatal("an agent assessment made the investigation approval-eligible")
	}
	inv := widgetLikeRecord(t, f, out)
	inv.Findings, inv.Decision = cls.Findings, &cls.Decision
	must(t, inv.Validate())
	if len(inv.AgentAssessments) != 1 || inv.AgentAssessments[0].Verdict != inv.AgentVerdicts[0].ID {
		t.Fatal("the run record lost the assessment or its verdict link")
	}
}
