package impact

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Role-membership defaults (PD-6, lane verify4, D-0027): "spec.signer unset ⇒
// outside role signer-trust (observed)" decides only the refinement's
// membership leaves (spec.signer equals the role's value or the outside
// value), only when the fact carries it from a confirmed proof of exactly
// its rule; never another condition on the same field; a contradiction with
// a schema, cited or observed default blocks it.
func TestRoleMembershipDefaults(t *testing.T) {
	noCRD := badgeEnv(t, false)
	edge := newEdge().edge
	pool := []domain.Evidence{upstreamDoc}
	signer := func(v string) domain.Condition { return field("spec.signer", domain.StateEquals, v) }
	rule := domain.SemanticAssertion{Statement: "refined"}
	membership := func() domain.RoleMembershipDefault {
		return domain.RoleMembershipDefault{Group: "acme.example", Kind: "Badge", Path: "spec.signer", Role: "signer-trust", RoleValue: "true", OutsideValue: "false",
			Level: domain.LevelObserved, Arms: []string{"condition-absent", "default-probe-1-unset", "default-probe-1-other"},
			Observation: "with spec.signer unset the relationship could not form", Versions: []string{"1.9.4", "2.0.0"}, ControllerValues: "{}",
			Fixture: "sha256:fx", Evidence: []domain.Evidence{upstreamDoc}, Limitation: domain.ObservedDefaultLimitation, RuleDigest: rule.Digest(), Proof: "pf-0123456789ab"}
	}
	type facts struct {
		mds []domain.RoleMembershipDefault
		ods []domain.ObservedDefault
	}
	evalOn := func(c domain.Condition, f facts) ConditionResult {
		fact := domain.VerifiedFact{Assertion: rule, RoleMembershipDefaults: f.mds, ObservedDefaults: f.ods}
		mem, _ := fact.UsableRoleMembershipDefaults()
		obs, _ := fact.UsableObservedDefaults()
		return EvaluateConditionWith(c, noCRD, edge, ConditionOptions{Evidence: pool, Membership: mem, Observed: obs})
	}
	eval := func(c domain.Condition, mds ...domain.RoleMembershipDefault) ConditionResult {
		return evalOn(c, facts{mds: mds})
	}

	// without it the membership leaves are undecidable on an unset field
	for _, v := range []string{"true", "false"} {
		if r := eval(badgeScope("plain", signer(v))); r.Value != Unknown {
			t.Fatalf("no membership default, %s: %v", v, r.Value)
		}
	}
	// with it: an unset spec.signer is outside the role (role leaf false,
	// outside leaf true), naming the proof, the role and the limitation
	if r := eval(badgeScope("plain", signer("true")), membership()); r.Value != False {
		t.Fatalf("role leaf: %v %v", r.Value, r.Needed)
	}
	r := eval(badgeScope("plain", signer("false")), membership())
	subjects := strings.Join(matchSubjects(r), "|")
	if r.Value != True || !strings.Contains(subjects, "outside role signer-trust") || !strings.Contains(subjects, "pf-0123456789ab") ||
		!strings.Contains(subjects, domain.ObservedDefaultLimitation) || strings.Contains(subjects, "defaults to") {
		t.Fatalf("outside leaf: %v %v", r.Value, matchSubjects(r))
	}
	// the refined rule's shape: exposure resource(K, E, role), overlap
	// resource(K, any(all(O), all(outside))) on an object that leaves both unset
	exposure := badgeScope("plain", field("spec.key.policy", domain.StateUnset), signer("true"))
	overlap := badgeScope("plain", domain.Condition{Op: domain.OpAny, Of: []domain.Condition{
		{Op: domain.OpAll, Of: []domain.Condition{field("spec.key.policy", domain.StateEquals, `"Keep"`)}},
		{Op: domain.OpAll, Of: []domain.Condition{signer("false")}}}})
	if r := eval(exposure); r.Value != Unknown {
		t.Fatalf("refined exposure without membership: %v", r.Value)
	}
	if r := eval(exposure, membership()); r.Value != False {
		t.Fatalf("refined exposure with membership: %v", r.Value)
	}
	if r := eval(overlap, membership()); r.Value != True {
		t.Fatalf("refined overlap with membership: %v", r.Value)
	}
	// a stated value wins
	if r := eval(badgeScope("signer", signer("false")), membership()); r.Value != False {
		t.Fatalf("stated value: %v", r.Value)
	}

	// only for membership: any other condition on the same field stays undecidable
	for name, c := range map[string]domain.Condition{
		"another value":  signer(`"maybe"`),
		"not-equals":     field("spec.signer", domain.StateNotEquals, "true"),
		"matches":        {Op: domain.OpField, Path: "spec.signer", State: domain.StateMatches, Pattern: "^f"},
		"several values": field("spec.signer", domain.StateEquals, "true", "false"),
		"another field":  field("spec.mode", domain.StateEquals, `"Strict"`),
		"a similar path": {Op: domain.OpField, Path: "spec.signers", State: domain.StateEquals, Values: []string{"false"}},
	} {
		if r := eval(badgeScope("plain", c), membership()); r.Value != Unknown {
			t.Errorf("%s: %v (a membership default must not decide it)", name, r.Value)
		}
	}
	// never a field default: the PD-5 field-default lookup does not see it
	if r := evalOn(badgeScope("plain", field("spec.signer", domain.StateNotEquals, "false")), facts{mds: []domain.RoleMembershipDefault{membership()}}); r.Value != Unknown {
		t.Fatalf("not-equals outside value: %v", r.Value)
	}

	// the guards: merely proposed (no proof), another rule, invalid shape
	proposed := membership()
	proposed.Proof = ""
	other := membership()
	other.RuleDigest = "sha256:other"
	unlimited := membership()
	unlimited.Limitation = ""
	for name, md := range map[string]domain.RoleMembershipDefault{"no proof": proposed, "another rule": other, "no limitation": unlimited} {
		if r := eval(badgeScope("plain", signer("false")), md); r.Value != Unknown {
			t.Errorf("%s: %v", name, r.Value)
		}
	}

	// contradictions block: a cited default putting unset in the role
	cited := withDefault(signer("false"), "true", domain.DefaultController, upstreamDoc.ID)
	r = eval(badgeScope("plain", cited), membership())
	if r.Value != Unknown || r.Reason != domain.UnknownSemanticAmbiguity || !strings.Contains(strings.Join(r.Needed, "|"), "contradiction") {
		t.Fatalf("contradiction with a cited default: %v %v", r.Value, r.Needed)
	}
	// an agreeing cited default decides by its value
	agree := withDefault(signer("false"), "false", domain.DefaultController, upstreamDoc.ID)
	if r := eval(badgeScope("plain", agree), membership()); r.Value != True {
		t.Fatalf("agreeing cited default: %v", r.Value)
	}
	// an observed field default (PD-5) putting unset in the role
	od := domain.ObservedDefault{Group: "acme.example", Kind: "Badge", Path: "spec.signer", Value: "true", Level: domain.LevelObserved,
		Arms: []string{"a", "b", "c"}, Observation: "o", Versions: []string{"2.0.0"}, ControllerValues: "{}", Fixture: "sha256:fx",
		Evidence: []domain.Evidence{upstreamDoc}, Limitation: domain.ObservedDefaultLimitation, RuleDigest: rule.Digest(), Proof: "pf-0123456789ab"}
	if r := evalOn(badgeScope("plain", signer("false")), facts{mds: []domain.RoleMembershipDefault{membership()}, ods: []domain.ObservedDefault{od}}); r.Value != Unknown ||
		!strings.Contains(strings.Join(r.Needed, "|"), "contradiction") {
		t.Fatalf("contradiction with an observed default: %v %v", r.Value, r.Needed)
	}
	// a contradiction already found (cited vs observed) stays blocked
	od.Value = "false"
	if r := evalOn(badgeScope("plain", cited), facts{mds: []domain.RoleMembershipDefault{membership()}, ods: []domain.ObservedDefault{od}}); r.Value != Unknown {
		t.Fatalf("an earlier contradiction was resolved: %v", r.Value)
	}
	// two membership defaults that disagree on the role's values
	second := membership()
	second.Proof, second.RoleValue, second.OutsideValue = "pf-ba9876543210", "false", "true"
	if r := eval(badgeScope("plain", signer("false")), membership(), second); r.Value != Unknown || !strings.Contains(strings.Join(r.Needed, "|"), "contradiction") {
		t.Fatalf("disagreeing membership defaults: %v %v", r.Value, r.Needed)
	}
}

// A schema default that puts an unset field in the role contradicts the
// membership default; one outside the role agrees.
func TestRoleMembershipDefaultAgainstSchema(t *testing.T) {
	edge := newEdge().edge
	rule := domain.SemanticAssertion{Statement: "refined"}
	md := domain.RoleMembershipDefault{Group: "acme.example", Kind: "Badge", Path: "spec.signer", Role: "signer-trust", RoleValue: "true", OutsideValue: "false",
		Level: domain.LevelObserved, Arms: []string{"a", "b", "c"}, Observation: "o", Versions: []string{"2.0.0"}, ControllerValues: "{}", Fixture: "sha256:fx",
		Evidence: []domain.Evidence{upstreamDoc}, Limitation: domain.ObservedDefaultLimitation, RuleDigest: rule.Digest(), Proof: "pf-0123456789ab"}
	for _, tc := range []struct {
		schemaDefault string
		want          Truth
	}{{"true", Unknown}, {"false", True}} {
		crd := strings.Replace(defaultBadgeCRD, "signer: {type: boolean}", "signer: {type: boolean, default: "+tc.schemaDefault+"}", 1)
		e := condEnv(t, "", map[string]string{"badges.yaml": defaultBadges, "badge-crd.yaml": crd})
		f := domain.VerifiedFact{Assertion: rule, RoleMembershipDefaults: []domain.RoleMembershipDefault{md}}
		mem, _ := f.UsableRoleMembershipDefaults()
		r := EvaluateConditionWith(badgeScope("plain", field("spec.signer", domain.StateEquals, "false")), e, edge, ConditionOptions{Membership: mem})
		if r.Value != tc.want {
			t.Errorf("schema default %s: %v %v", tc.schemaDefault, r.Value, r.Needed)
		}
	}
}

// A schema default and a cited default that contradict each other block the
// field; an observed default (PD-5) or a membership default (PD-6) never
// settles it afterwards (before lane verify4 an observed default did).
func TestObservedDefaultKeepsAnEarlierContradiction(t *testing.T) {
	edge := newEdge().edge
	rule := domain.SemanticAssertion{Statement: "r"}
	crd := strings.Replace(defaultBadgeCRD, "signer: {type: boolean}", "signer: {type: boolean, default: true}", 1)
	e := condEnv(t, "", map[string]string{"badges.yaml": defaultBadges, "badge-crd.yaml": crd})
	od := domain.ObservedDefault{Group: "acme.example", Kind: "Badge", Path: "spec.signer", Value: "false", Level: domain.LevelObserved,
		Arms: []string{"a", "b", "c"}, Observation: "o", Versions: []string{"2.0.0"}, ControllerValues: "{}", Fixture: "sha256:fx",
		Evidence: []domain.Evidence{upstreamDoc}, Limitation: domain.ObservedDefaultLimitation, RuleDigest: rule.Digest(), Proof: "pf-0123456789ab"}
	cited := withDefault(field("spec.signer", domain.StateEquals, "false"), "false", domain.DefaultController, upstreamDoc.ID)
	r := EvaluateConditionWith(badgeScope("plain", cited), e, edge, ConditionOptions{Evidence: []domain.Evidence{upstreamDoc}, Observed: []domain.ObservedDefault{od}})
	if r.Value != Unknown || !strings.Contains(strings.Join(r.Needed, "|"), "contradicts") {
		t.Fatalf("an observed default settled a schema/cited contradiction: %v %v", r.Value, r.Needed)
	}
}
