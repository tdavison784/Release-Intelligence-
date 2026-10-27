package impact

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Observed defaults (PD-5, lane verify3, D-0026): a default a confirmed proof
// observed on the real controller decides an unset field nothing else
// establishes — only when the fact carries it from a confirmed proof of
// exactly its rule; a contradiction with a cited or schema default blocks
// both.
func TestObservedDefaults(t *testing.T) {
	withCRD, noCRD := badgeEnv(t, true), badgeEnv(t, false)
	edge := newEdge().edge
	pool := []domain.Evidence{upstreamDoc}
	signer := func(v string) domain.Condition { return field("spec.signer", domain.StateEquals, v) }
	rule := domain.SemanticAssertion{Statement: "r"}
	observed := func(value string) domain.ObservedDefault {
		return domain.ObservedDefault{Group: "acme.example", Kind: "Badge", Path: "spec.signer", Value: value, Level: domain.LevelObserved,
			Arms: []string{"condition-absent", "default-probe-1-unset", "default-probe-1-other"}, Observation: "an unset spec.signer behaved as false",
			Versions: []string{"1.9.4", "2.0.0"}, ControllerValues: "{}", Fixture: "sha256:fx", Evidence: []domain.Evidence{upstreamDoc},
			Limitation: domain.ObservedDefaultLimitation, RuleDigest: rule.Digest(), Proof: "pf-0123456789ab"}
	}
	eval := func(c domain.Condition, ods ...domain.ObservedDefault) ConditionResult {
		f := domain.VerifiedFact{Assertion: rule, ObservedDefaults: ods}
		usable, _ := f.UsableObservedDefaults()
		return EvaluateConditionWith(c, noCRD, edge, ConditionOptions{Evidence: pool, Observed: usable})
	}

	// without it the field is undecidable
	if r := eval(badgeScope("plain", signer("false"))); r.Value != Unknown {
		t.Fatalf("no observed default: %v", r.Value)
	}
	// through a confirmed proof of the same rule it decides, naming the proof and its limitation
	r := eval(badgeScope("plain", signer("false")), observed("false"))
	if r.Value != True || !strings.Contains(strings.Join(matchSubjects(r), "|"), "observed by confirmed proof pf-0123456789ab") ||
		!strings.Contains(strings.Join(matchSubjects(r), "|"), domain.ObservedDefaultLimitation) {
		t.Fatalf("observed default: %v %v", r.Value, matchSubjects(r))
	}
	if r := eval(badgeScope("plain", signer("true")), observed("false")); r.Value != False {
		t.Fatalf("observed default, other value: %v", r.Value)
	}
	// a stated value wins
	if r := eval(badgeScope("signer", signer("false")), observed("false")); r.Value != False {
		t.Fatalf("stated value: %v", r.Value)
	}
	// merely proposed (no proof) or observed for another rule: ignored
	proposed := observed("false")
	proposed.Proof = ""
	other := observed("false")
	other.RuleDigest = "sha256:other"
	for name, od := range map[string]domain.ObservedDefault{"no proof": proposed, "another rule": other} {
		if r := eval(badgeScope("plain", signer("false")), od); r.Value != Unknown {
			t.Errorf("%s: %v", name, r.Value)
		}
	}
	// a contradiction with the rule's cited default blocks both
	cited := withDefault(signer("false"), "true", domain.DefaultController, upstreamDoc.ID)
	if r := eval(badgeScope("plain", cited)); r.Value != False {
		t.Fatalf("cited default alone: %v", r.Value)
	}
	r = eval(badgeScope("plain", cited), observed("false"))
	if r.Value != Unknown || r.Reason != domain.UnknownSemanticAmbiguity || !strings.Contains(strings.Join(r.Needed, "|"), "contradiction") {
		t.Fatalf("contradiction with a cited default: %v %v", r.Value, r.Needed)
	}
	// agreeing with the cited default: decided, both named
	agree := withDefault(signer("false"), "false", domain.DefaultController, upstreamDoc.ID)
	if r := eval(badgeScope("plain", agree), observed("false")); r.Value != True {
		t.Fatalf("agreeing defaults: %v", r.Value)
	}
	// two observed defaults that disagree block the field
	second := observed("true")
	second.Proof = "pf-ba9876543210"
	if r := eval(badgeScope("plain", signer("false")), observed("false"), second); r.Value != Unknown || !strings.Contains(strings.Join(r.Needed, "|"), "contradiction") {
		t.Fatalf("disagreeing observed defaults: %v %v", r.Value, r.Needed)
	}
	// a contradiction with a schema default blocks both
	mode := domain.ObservedDefault{Group: "acme.example", Kind: "Badge", Path: "spec.mode", Value: `"Lax"`, Level: domain.LevelObserved,
		Arms: []string{"a", "b", "c"}, Observation: "o", Versions: []string{"2.0.0"}, ControllerValues: "{}", Fixture: "sha256:fx",
		Evidence: []domain.Evidence{upstreamDoc}, Limitation: domain.ObservedDefaultLimitation, RuleDigest: rule.Digest(), Proof: "pf-0123456789ab"}
	f := domain.VerifiedFact{Assertion: rule, ObservedDefaults: []domain.ObservedDefault{mode}}
	usable, _ := f.UsableObservedDefaults()
	if r := EvaluateConditionWith(badgeScope("plain", field("spec.mode", domain.StateEquals, `"Strict"`)), withCRD, edge, ConditionOptions{Evidence: pool, Observed: usable}); r.Value != Unknown ||
		!strings.Contains(strings.Join(r.Needed, "|"), "contradiction") {
		t.Fatalf("contradiction with a schema default: %v %v", r.Value, r.Needed)
	}
}

func matchSubjects(r ConditionResult) []string {
	var out []string
	for _, m := range r.Matches {
		out = append(out, m.Subject)
	}
	return out
}
