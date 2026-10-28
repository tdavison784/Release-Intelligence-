package verify

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// tierRule: retention.mode's default changes, but only gold-tier Widgets are
// exposed; a Widget that keeps its mode, or a silver one, is shielded. It
// compares spec.tier by value, so what an unset tier means decides every
// Widget that omits it (mission E2).
func tierRule(def *domain.FieldDefault) domain.SemanticAssertion {
	r := widgetRule()
	gold := domain.Condition{Op: domain.OpField, Path: "spec.tier", State: domain.StateEquals, Values: []string{`"gold"`}, Default: def}
	r.Applicability = &domain.Applicability{
		Exposure: domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget", Of: []domain.Condition{
			{Op: domain.OpField, Path: "spec.retention.mode", State: domain.StateUnset}, gold}},
		Overlap: &domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget", Of: []domain.Condition{{Op: domain.OpAny, Of: []domain.Condition{
			{Op: domain.OpField, Path: "spec.retention.mode", State: domain.StateSet},
			{Op: domain.OpField, Path: "spec.tier", State: domain.StateEquals, Values: []string{`"silver"`}}}}}},
	}
	return r
}

// tierRole is a consumer observation whose role is spec.tier (no
// role-absent observation declared).
var tierRole = &ConsumerFixture{ID: "tier", Role: []FieldValue{{Path: "spec.tier", Value: "gold"}}}

var tierEvidence = cite("https://example.test/acme/v2.0.0/api/types.go", "Tier defaults to silver when omitted.", "// Tier defaults to silver when omitted.")

func tierDefault(value string, cites ...domain.EvidenceID) *domain.FieldDefault {
	return &domain.FieldDefault{Value: value, Source: domain.DefaultController, Citations: cites}
}

// plainArms are the rule's produced-object arms with every alternative of the
// overlap as its own shielded arm.
func tierArms(t *testing.T, rule domain.SemanticAssertion) ([]consumerArm, *SubjectFixture) {
	t.Helper()
	fx, _ := acmeSet(t).Subject("acme.example", "Widget")
	var out []consumerArm
	for i, leaves := range [][]domain.Condition{
		{{Op: domain.OpField, Path: "spec.retention.mode", State: domain.StateSet}},
		{{Op: domain.OpField, Path: "spec.tier", State: domain.StateEquals, Values: []string{`"silver"`}}},
	} {
		name := domain.ArmConditionPresent
		if i > 0 {
			name += "-2"
		}
		obj, err := buildObject(fx, "ri-arm-p"+name[len(name)-1:], leaves, rule.Change.Before)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, consumerArm{Arm: Arm{Name: name, Object: obj, Condition: describe(leaves, rule.Change.Before), Predicted: domain.BehaviourPrevious}})
	}
	exposed := []domain.Condition{{Op: domain.OpField, Path: "spec.retention.mode", State: domain.StateUnset},
		{Op: domain.OpField, Path: "spec.tier", State: domain.StateEquals, Values: []string{`"gold"`}}}
	obj, err := buildObject(fx, armAbsentName, exposed, rule.Change.Before)
	if err != nil {
		t.Fatal(err)
	}
	out = append(out, consumerArm{Arm: Arm{Name: domain.ArmConditionAbsent, Object: obj, Condition: describe(exposed, nil), Predicted: domain.BehaviourNew}})
	return out, fx
}

func TestBoundaryArms(t *testing.T) {
	pool := rulePool([]domain.Evidence{tierEvidence})
	t.Run("explicit-only rule fails", func(t *testing.T) {
		rule := tierRule(nil)
		arms, fx := tierArms(t, rule)
		_, _, why := BoundaryArms(rule, fx, arms, nil, pool, false)
		if !strings.Contains(why, "omits spec.tier") || !strings.Contains(why, "only works when every field") {
			t.Fatalf("why %q", why)
		}
	})
	t.Run("unevidenced default fails", func(t *testing.T) {
		rule := tierRule(tierDefault(`"silver"`, "ev-000000000000"))
		arms, fx := tierArms(t, rule)
		_, _, why := BoundaryArms(rule, fx, arms, nil, pool, false)
		if !strings.Contains(why, "not evidenced") {
			t.Fatalf("why %q", why)
		}
	})
	t.Run("evidenced default predicts the omitted object shielded", func(t *testing.T) {
		rule := tierRule(tierDefault(`"silver"`, tierEvidence.ID))
		arms, fx := tierArms(t, rule)
		extra, _, why := BoundaryArms(rule, fx, arms, nil, pool, false)
		if why != "" || len(extra) != 1 {
			t.Fatalf("extra %+v why %q", extra, why)
		}
		b := extra[0]
		spec := b.Object["spec"].(map[string]any)
		if _, set := spec["tier"]; set || b.Predicted != domain.BehaviourPrevious || !isBoundary(b.Name) || !shielded(b.Name) || b.outsideRole {
			t.Fatalf("omitted arm: %+v", b.Arm)
		}
		if b.Object["metadata"].(map[string]any)["name"] != "ri-arm-omit-1" || spec["outputName"] != "ri-arm-omit-1-data" {
			t.Fatalf("the omitted arm is renamed throughout: %+v", b.Object)
		}
	})
	t.Run("evidenced default predicts the omitted object exposed", func(t *testing.T) {
		rule := tierRule(tierDefault(`"gold"`, tierEvidence.ID))
		arms, fx := tierArms(t, rule)
		extra, _, why := BoundaryArms(rule, fx, arms, nil, pool, false)
		if why != "" || len(extra) != 1 || extra[0].Predicted != domain.BehaviourNew || shielded(extra[0].Name) {
			t.Fatalf("extra %+v why %q", extra, why)
		}
	})
	t.Run("omitting a role field takes the object outside the role", func(t *testing.T) {
		rule := tierRule(tierDefault(`"silver"`, tierEvidence.ID))
		arms, fx := tierArms(t, rule)
		extra, _, why := BoundaryArms(rule, fx, arms, tierRole, pool, false)
		if why != "" || len(extra) != 1 || !isOutsideRole(extra[0].Name) || !extra[0].outsideRole {
			t.Fatalf("extra %+v why %q", extra, why)
		}
		// exposed outside the role: the consumer cannot observe it
		rule = tierRule(tierDefault(`"gold"`, tierEvidence.ID))
		arms, fx = tierArms(t, rule)
		extra, lims, why := BoundaryArms(rule, fx, arms, tierRole, pool, false)
		if why != "" || len(extra) != 0 || len(lims) != 1 || !strings.Contains(lims[0], "outside the consumer role") {
			t.Fatalf("extra %+v lims %v why %q", extra, lims, why)
		}
	})
	t.Run("a rule whose conditions disagree on an arm fails", func(t *testing.T) {
		// the overlap shields silver, but the default makes an unset tier
		// gold and the shielded silver arm stays decidable; declare the
		// silver arm exposed by construction to see the disagreement
		rule := tierRule(tierDefault(`"silver"`, tierEvidence.ID))
		arms, fx := tierArms(t, rule)
		arms[1].Predicted = domain.BehaviourNew
		if _, _, why := BoundaryArms(rule, fx, arms, nil, pool, false); !strings.Contains(why, "disagree") {
			t.Fatalf("why %q", why)
		}
	})
}

// The exercise fails on its boundary fixtures before any controller is
// rendered or any cluster runs; with an evidenced default the omitted-field
// arm joins the fixture.
func TestExerciseRuleBoundary(t *testing.T) {
	// produced-object arms: shielded silver, exposed gold (both leave mode unset)
	plainRule := func(def *domain.FieldDefault) domain.SemanticAssertion {
		r := tierRule(def)
		r.Applicability.Overlap = &domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget", Of: []domain.Condition{
			{Op: domain.OpField, Path: "spec.tier", State: domain.StateEquals, Values: []string{`"silver"`}}}}
		return r
	}
	rnd, run := &fakeRenderer{}, &fakeRunner{res: fixtureResult(false, true)}
	in := ExerciseInput{Product: "acme-ctl", Rule: plainRule(nil), Bounds: bounds, Fixtures: acmeSet(t), Exercises: []runtime.DeclaredExercise{rolloverExercise(true)},
		Relationships: acmeProduces, Evidence: []domain.Evidence{tierEvidence}}
	m, _ := ExerciseRule(context.Background(), in)
	if m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "before any cluster ran") || !strings.Contains(m.Detail, "spec.tier") {
		t.Fatalf("%s: %s", m.Outcome, m.Detail)
	}
	in.Rule = plainRule(tierDefault(`"silver"`, tierEvidence.ID))
	in.Renderer, in.Runner = rnd, run
	m, _ = ExerciseRule(context.Background(), in)
	objs := string(run.spec.Objects)
	if !strings.Contains(objs, "name: ri-arm-omit-1") || strings.Count(objs, "tier:") != 2 {
		t.Fatalf("the omitted-field arm is not in the fixture (%s: %s):\n%s", m.Outcome, m.Detail, objs)
	}
	if strings.Join(rnd.versions, ",") != "1.9.4,2.0.0" {
		t.Fatalf("renders %v", rnd.versions)
	}
}

func TestEvaluateArmsWithBoundaryArms(t *testing.T) {
	omitted := armPlan{Arm: Arm{Name: domain.ArmConditionPresent + omittedSuffix + "-1", Predicted: domain.BehaviourPrevious, Condition: "tier omitted"},
		subject: "acme.example/v1/Widget/ri-verify/ri-arm-omit-1",
		objects: []string{"acme.example/v1/Widget/ri-verify/ri-arm-omit-1", "core/v1/ConfigMap/ri-verify/ri-arm-omit-1-data"}}
	res := func(changed bool) *runtime.FixtureResult {
		r := fixtureResult(false, true)
		for i := range r.Run.Exercises {
			r.Run.Exercises[i].Reacted = append(r.Run.Exercises[i].Reacted, omitted.subject)
		}
		if changed {
			r.Run.Degradations = append(r.Run.Degradations, runtime.Degradation{Object: "core/v1/ConfigMap/ri-verify/ri-arm-omit-1-data", Window: "exercise", Signature: "data:payload"})
		}
		return r
	}
	if m := EvaluateArms(res(false), append(plans(), omitted), "acme-ctl/rollover"); m.Outcome != domain.MethodPass || len(m.Arms) != 3 {
		t.Fatalf("predicted previous, kept: %s %s", m.Outcome, m.Detail)
	}
	if m := EvaluateArms(res(true), append(plans(), omitted), "acme-ctl/rollover"); m.Outcome != domain.MethodFail || !strings.Contains(m.Detail, "omits a compared field") {
		t.Fatalf("predicted previous, changed: %s %s", m.Outcome, m.Detail)
	}
	quiet := fixtureResult(false, true) // the omitted arm never answered
	if m := EvaluateArms(quiet, append(plans(), omitted), "acme-ctl/rollover"); m.Outcome != domain.MethodInconclusive {
		t.Fatalf("unanswered omitted arm: %s %s", m.Outcome, m.Detail)
	}
}

func TestEvaluateConsumerOutsideRoleArms(t *testing.T) {
	const v, f, u = runtime.ConsumerVerified, runtime.ConsumerFailed, runtime.ConsumerUndetermined
	present, absent, outside := domain.ArmConditionPresent, domain.ArmConditionAbsent, domain.ArmConditionPresent+outsideRoleSuffix+"-1"
	for name, tc := range map[string]struct {
		res  map[string][2]string
		want domain.MethodOutcome
	}{
		"outside the role, no relationship":  {map[string][2]string{present: {v, v}, absent: {v, f}, outside: {u, u}}, domain.MethodPass},
		"outside the role, never verified":   {map[string][2]string{present: {v, v}, absent: {v, f}}, domain.MethodPass},
		"plays the role and keeps verifying": {map[string][2]string{present: {v, v}, absent: {v, f}, outside: {v, v}}, domain.MethodPass},
		"plays the role and breaks":          {map[string][2]string{present: {v, v}, absent: {v, f}, outside: {v, f}}, domain.MethodFail},
	} {
		m := EvaluateConsumerArms(consumerRun(tc.res), consumerPlans(present, outside, absent), nil)
		if m.Outcome != tc.want {
			t.Errorf("%s: %s (%s)", name, m.Outcome, m.Detail)
		}
	}
}
