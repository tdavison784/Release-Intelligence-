package verify

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/applicability"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Non-discriminating arms (Milestone 4 fix, lane verify2, D-0025). An arm
// predicted to take the new behaviour is evidence of the change only if its
// object can behave differently at FROM and at TO. Under a default-changed
// rule only an object that leaves the changed field unset meets the new
// default at TO; an object that sets the field explicitly keeps its explicit
// value at both versions — an explicit value equal to the new default already
// has the new behaviour at FROM. Such an arm cannot contrast FROM with TO by
// construction: it is run and recorded (ProofArm.NonDiscriminating says why)
// and never decides the exercise. A default-changed rule is then decided by
// its condition arm and its omitted-field arms (boundary.go), which leave the
// field unset; with no discriminating contrast arm the exercise stays
// inconclusive. Under any other kind of change an explicit value can
// discriminate (a value that is accepted at FROM and rejected at TO), so no
// arm is classified.

// nonDiscriminating says why an arm cannot contrast FROM with TO ("" when it
// can): the rule's change is a default change of a field of the fixture's
// subject kind, the arm is predicted to change, and its object sets that
// field explicitly.
func nonDiscriminating(rule domain.SemanticAssertion, t FixtureType, a Arm) string {
	s := rule.Subject
	switch {
	case a.Predicted != domain.BehaviourNew, rule.Change == nil, rule.Change.Type != domain.ChangeKindDefaultChanged, s == nil,
		s.Family != domain.SubjectCRDField, s.Group != t.Group, s.Kind != t.Kind, s.Path == "", strings.Contains(s.Path, "[]"):
		return ""
	}
	keys, err := catalog.SplitFieldPath(s.Path)
	if err != nil {
		return ""
	}
	v, present := getPath(a.Object, keys)
	if !present {
		return ""
	}
	value := encode(v)
	if after := rule.Change.After; after != nil && applicability.SameValue(*after, value) {
		return fmt.Sprintf("it sets %s explicitly to the new default %s: it has the new behaviour at FROM already, so FROM and TO cannot differ", s.Path, value)
	}
	return fmt.Sprintf("it sets %s explicitly (%s): a default change reaches only objects that leave the field unset, so FROM and TO cannot differ", s.Path, value)
}

// classify marks the non-discriminating arms of an exercise.
func classify(rule domain.SemanticAssertion, t FixtureType, arms []consumerArm) []consumerArm {
	for i := range arms {
		arms[i].NonDiscriminating = nonDiscriminating(rule, t, arms[i].Arm)
	}
	return arms
}
