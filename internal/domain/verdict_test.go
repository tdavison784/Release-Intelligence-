package domain

import (
	"testing"
	"time"
)

func strp(s string) *string { return &s }

// testRule is a synthetic rule: widgets of example.dev whose
// spec.cleanup.policy is unset are purged after the upgrade.
func testRule() ProposedRule {
	return ProposedRule{
		Assertion: SemanticAssertion{
			Subject: &Subject{Family: SubjectCRDField, Product: "widgets", Group: "example.dev", Kind: "Widget", Path: "spec.cleanup.policy"},
			Change:  &ChangeSpec{Type: ChangeKindDefaultChanged, Before: strp(`"Retain"`), After: strp(`"Purge"`)},
			Applicability: &Applicability{Exposure: Condition{Op: OpResource, Group: "example.dev", Kind: "Widget",
				Of: []Condition{{Op: OpField, Path: "spec.cleanup.policy", State: StateUnset}}}},
			Consequence: &Consequence{Kind: ConsequenceBehaviorChange, ExposedClass: ImpactReviewRequired},
			Statement:   "Widgets that leave spec.cleanup.policy unset are purged after the upgrade.",
		},
		Release: ReleaseBounds{From: "1.4.0", To: "1.5.0"},
	}
}

func TestMitigationSpecValidate(t *testing.T) {
	ok := MitigationSpec{Item: "ci-x", ProposedRule: testRule(), EnvironmentBasis: []string{"clm-1"}, RequestedVerification: "exercise both arms"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("well-formed spec rejected: %v", err)
	}
	cases := map[string]func(m *MitigationSpec){
		"incomplete rule":   func(m *MitigationSpec) { m.ProposedRule.Assertion.Consequence = nil },
		"bad grammar":       func(m *MitigationSpec) { m.ProposedRule.Assertion.Applicability.Exposure.Of[0].State = StateEnabled },
		"bounds unparsable": func(m *MitigationSpec) { m.ProposedRule.Release.To = "next" },
		"bounds reversed":   func(m *MitigationSpec) { m.ProposedRule.Release = ReleaseBounds{From: "1.5.0", To: "1.4.0"} },
		"no basis":          func(m *MitigationSpec) { m.EnvironmentBasis = nil },
		"no verification":   func(m *MitigationSpec) { m.RequestedVerification = " " },
		"no item":           func(m *MitigationSpec) { m.Item = "" },
		"undecidable": func(m *MitigationSpec) {
			m.ProposedRule.Assertion.Applicability.Exposure = Condition{Op: OpUndecidable, Reason: UnknownRuntimeBehaviorGap, Needed: "a runtime read"}
		},
		"undecidable nested": func(m *MitigationSpec) {
			m.ProposedRule.Assertion.Applicability.Exposure.Of = append(m.ProposedRule.Assertion.Applicability.Exposure.Of, Condition{Op: OpUndecidable, Reason: UnknownRuntimeBehaviorGap, Needed: "x"})
		},
	}
	for name, mut := range cases {
		m := ok
		m.ProposedRule = testRule()
		mut(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestReleaseBoundsContains(t *testing.T) {
	r := ReleaseBounds{From: "1.4.0", To: "1.5.0"}
	for v, want := range map[string]bool{"1.4.0": false, "1.4.1": true, "1.5.0": true, "v1.5.0": true, "controller-v1.5.0": true, "chart-1.4.0": false, "1.5.1": false, "x": false} {
		if got := r.Contains(v); got != want {
			t.Errorf("Contains(%s) = %v, want %v", v, got, want)
		}
	}
	a, b := testRule(), testRule()
	b.Release.To = "1.6.0"
	if a.Digest() == b.Digest() {
		t.Error("the rule digest ignores the release bounds")
	}
	b = testRule()
	b.Assertion.Statement = "reworded"
	if a.Digest() != b.Digest() {
		t.Error("the rule digest depends on prose")
	}
}

func TestHypothesisRule(t *testing.T) {
	r := HypothesisRule("hyp-abc", "inv-def")
	h, i, ok := ParseHypothesisRule(r)
	if !ok || h != "hyp-abc" || i != "inv-def" {
		t.Fatalf("round trip: %q → %q %q %v", r, h, i, ok)
	}
	for _, bad := range []string{"provider:anthropic", "hypothesis:hyp-1", "hypothesis:x@inv-1", "hypothesis:hyp-1@y"} {
		if _, _, ok := ParseHypothesisRule(bad); ok {
			t.Errorf("parsed %q", bad)
		}
	}
}

func TestAgentVerdictValidate(t *testing.T) {
	at := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	prov := Provenance{Method: MethodAI, Producer: "investigate.agent@v1", Confidence: ConfidenceLow, Model: "m", ModelVersion: "m-1",
		PromptVersion: "investigate-agent/v6", PromptDigest: "d", InputEvidence: []EvidenceID{"ev-1"}, GeneratedAt: &at, CallID: "call-1"}
	v := AgentVerdict{ChangeItem: "ci-1", Class: FindingNotAffected, Confidence: ConfidenceMedium, Mechanism: "default flips",
		EnvironmentReasoning: "every widget sets the policy", Citations: []EvidenceID{"ev-1"}, VerifiedFacts: []string{"clm-1", "vr-2"},
		Mitigations: []string{"hyp-3"}, WouldOverturn: "a widget without the setting", Provenance: prov}
	v.ID = AgentVerdictID(v.ChangeItem, v.Class, v.Mechanism, prov.CallID)
	if err := v.Validate(); err != nil {
		t.Fatal(err)
	}
	cases := map[string]func(x *AgentVerdict){
		"not ai":            func(x *AgentVerdict) { x.Provenance.Method = MethodComputed },
		"class":             func(x *AgentVerdict) { x.Class = "safe" },
		"confidence":        func(x *AgentVerdict) { x.Confidence = "" },
		"uncited":           func(x *AgentVerdict) { x.Citations = nil },
		"no overturn":       func(x *AgentVerdict) { x.WouldOverturn = "" },
		"fact ref":          func(x *AgentVerdict) { x.VerifiedFacts = []string{"asr-1"} },
		"mitigation ref":    func(x *AgentVerdict) { x.Mitigations = []string{"clm-1"} },
		"id not derived":    func(x *AgentVerdict) { x.Mechanism = "other" },
		"negative dropping": func(x *AgentVerdict) { x.DroppedCitations = -1 },
	}
	for name, mut := range cases {
		x := v
		mut(&x)
		if err := x.Validate(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// insufficient evidence may cite nothing
	x := v
	x.Class, x.Citations = FindingInsufficientEvidence, nil
	x.ID = AgentVerdictID(x.ChangeItem, x.Class, x.Mechanism, prov.CallID)
	if err := x.Validate(); err != nil {
		t.Fatalf("insufficient-evidence without citations rejected: %v", err)
	}
}
