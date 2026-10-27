package verify

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// Non-discriminating arms (lane verify2, D-0025) on the synthetic Widget:
// retention.mode's default changes from Keep to Purge.

var modeEvidence = cite("https://example.test/acme/v2.0.0/api/types.go", "Mode defaults to Purge when omitted.", "// Mode defaults to Purge when omitted.")

// explicitDefaultRule is the rule shape that exposed the defect: "unset
// takes the new default Purge (exposed); an explicit Keep keeps the previous
// behaviour". Its exposure is mode = Purge with the evidenced default, so the
// condition-absent arm sets mode: Purge explicitly — which behaves the same
// at FROM and TO — and the omitted-field arm leaves it unset.
func explicitDefaultRule(change domain.ChangeKind, def string) domain.SemanticAssertion {
	r := widgetRule()
	r.Change.Type = change
	mode := func(v string) domain.Condition {
		c := domain.Condition{Op: domain.OpField, Path: "spec.retention.mode", State: domain.StateEquals, Values: []string{v},
			Default: &domain.FieldDefault{Value: def, Source: domain.DefaultController, Citations: []domain.EvidenceID{modeEvidence.ID}}}
		return domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget", Of: []domain.Condition{c}}
	}
	keep := mode(`"Keep"`)
	r.Applicability = &domain.Applicability{Exposure: mode(`"Purge"`), Overlap: &keep}
	return r
}

// armRun answers the exercise on every named arm at both versions; changed
// arms show a difference in their produced ConfigMap.
func armRun(arms []string, changed ...string) *runtime.FixtureResult {
	r := &runtime.Run{Product: "acme-ctl", From: "1.9.4", To: "2.0.0", Cluster: "ri-verify-00000000", Status: runtime.RunCompleted, UpgradeCompleted: true}
	var reacted []string
	for _, a := range arms {
		reacted = append(reacted, "acme.example/v1/Widget/ri-verify/"+a)
	}
	for _, phase := range []string{"from", "to"} {
		r.Exercises = append(r.Exercises, runtime.ExerciseRun{Exercise: "acme-ctl/rollover", Phase: phase, Reacted: reacted})
	}
	for _, a := range changed {
		r.Degradations = append(r.Degradations, runtime.Degradation{Object: "core/v1/ConfigMap/ri-verify/" + a + "-data", Window: "exercise", Signature: "data:payload"})
	}
	return &runtime.FixtureResult{Run: r, Artifacts: []domain.ArtifactRef{{ID: "art-1", Path: "derived/runtime/fx/run.json", URI: "ri:runtime/run.json", Digest: "sha256:run"}}}
}

func exerciseWith(t *testing.T, rule domain.SemanticAssertion, res *runtime.FixtureResult) (domain.ProofMethod, *fakeRunner) {
	t.Helper()
	run := &fakeRunner{res: res}
	in := ExerciseInput{Product: "acme-ctl", Rule: rule, Bounds: bounds, Fixtures: acmeSet(t), Exercises: []runtime.DeclaredExercise{rolloverExercise(true)},
		Relationships: acmeProduces, Renderer: &fakeRenderer{}, Runner: run, Evidence: []domain.Evidence{modeEvidence}}
	m, _ := ExerciseRule(context.Background(), in)
	return m, run
}

func armByName(m domain.ProofMethod, name string) *domain.ProofArm {
	for i := range m.Arms {
		if m.Arms[i].Name == name {
			return &m.Arms[i]
		}
	}
	return nil
}

func TestNonDiscriminatingArm(t *testing.T) {
	arms := []string{armPresentName, armAbsentName, "ri-arm-omit-1"}
	omitted := domain.ArmConditionAbsent + omittedSuffix + "-1"
	rule := explicitDefaultRule(domain.ChangeKindDefaultChanged, `"Purge"`)

	// the development-run shape: the explicit-Purge arm stays quiet (as it
	// must), the omitted arm changes; before this fix the whole rule was
	// inconclusive, now the explicit arm is recorded and the omitted arm decides
	m, run := exerciseWith(t, rule, armRun(arms, "ri-arm-omit-1"))
	if objs := string(run.spec.Objects); !strings.Contains(objs, "mode: Purge") || !strings.Contains(objs, "name: ri-arm-omit-1") {
		t.Fatalf("fixture objects:\n%s", objs)
	}
	if m.Outcome != domain.MethodPass || !strings.Contains(m.Detail, "non-discriminating") {
		t.Fatalf("%s: %s", m.Outcome, m.Detail)
	}
	if a := armByName(m, domain.ArmConditionAbsent); a == nil || !strings.Contains(a.NonDiscriminating, "new default") {
		t.Fatalf("condition-absent arm: %+v", a)
	}
	if a := armByName(m, omitted); a == nil || a.NonDiscriminating != "" || !a.Changed {
		t.Fatalf("omitted arm: %+v", a)
	}
	p := domain.ProofRecord{Hypothesis: hypID, ReviewItem: "ri-x", CandidateID: "sc-x", Product: "acme-ctl", ProposedRuleDigest: "d",
		Methods: []domain.ProofMethod{{Method: domain.ProofCitationCheck, Producer: CitationsProducer, Level: domain.LevelDeterministic, Outcome: domain.MethodPass,
			Detail: "ok", Evidence: []domain.Evidence{{ID: "ev-1"}}}, m}, VersionBounds: bounds, Limitations: []string{}, Level: domain.LevelObserved,
		Producer: Producer, CheckedAt: t0}
	p.Outcome = domain.ProofOutcomeOf(p.Methods)
	p.ID = domain.ProofRecordID(p)
	if err := p.Validate(); err != nil || p.Outcome != domain.ProofConfirmed {
		t.Fatalf("a pass decided by the omitted arm is a valid confirmation: %v", err)
	}

	// whatever the non-discriminating arm does, it never decides
	if m, _ := exerciseWith(t, rule, armRun(arms, armAbsentName, "ri-arm-omit-1")); m.Outcome != domain.MethodPass {
		t.Errorf("explicit arm changed too: %s %s", m.Outcome, m.Detail)
	}
	// the omitted arm quiet: the change is not shown
	if m, _ := exerciseWith(t, rule, armRun(arms)); m.Outcome != domain.MethodInconclusive || !strings.Contains(m.Detail, "did not change either") {
		t.Errorf("omitted arm quiet: %s %s", m.Outcome, m.Detail)
	}
	// the condition arm changed: refuted as before
	if m, _ := exerciseWith(t, rule, armRun(arms, armPresentName, "ri-arm-omit-1")); m.Outcome != domain.MethodFail {
		t.Errorf("condition arm changed: %s %s", m.Outcome, m.Detail)
	}

	// a rule whose only contrast arm is non-discriminating stays inconclusive:
	// with the default stated as Keep no omitted object is exposed
	keepRule := explicitDefaultRule(domain.ChangeKindDefaultChanged, `"Keep"`)
	m, _ = exerciseWith(t, keepRule, armRun([]string{armPresentName, armAbsentName, "ri-arm-omit-1"}, armAbsentName))
	if m.Outcome != domain.MethodInconclusive || !strings.Contains(m.Detail, "cannot discriminate") {
		t.Errorf("only contrast arm non-discriminating: %s %s", m.Outcome, m.Detail)
	}

	// an explicit value does discriminate under another kind of change (a
	// behaviour of the value Purge that changes between the versions): the
	// explicit arm decides as before
	behaviour := explicitDefaultRule(domain.ChangeKindBehaviorChanged, `"Keep"`)
	m, _ = exerciseWith(t, behaviour, armRun([]string{armPresentName, armAbsentName, "ri-arm-omit-1"}, armAbsentName))
	if m.Outcome != domain.MethodPass || armByName(m, domain.ArmConditionAbsent).NonDiscriminating != "" || strings.Contains(m.Detail, "non-discriminating") {
		t.Errorf("explicit value under a behaviour change: %s %s", m.Outcome, m.Detail)
	}
	if m, _ = exerciseWith(t, behaviour, armRun([]string{armPresentName, armAbsentName, "ri-arm-omit-1"})); m.Outcome != domain.MethodInconclusive {
		t.Errorf("explicit value quiet under a behaviour change: %s %s", m.Outcome, m.Detail)
	}
}

func TestNonDiscriminatingClassification(t *testing.T) {
	fx, _ := acmeSet(t).Subject("acme.example", "Widget")
	rule := explicitDefaultRule(domain.ChangeKindDefaultChanged, `"Purge"`)
	obj := func(mode any) map[string]any {
		spec := map[string]any{}
		if mode != nil {
			spec["retention"] = map[string]any{"mode": mode}
		}
		return map[string]any{"spec": spec}
	}
	for name, tc := range map[string]struct {
		arm  Arm
		want string
	}{
		"explicit new default":      {Arm{Predicted: domain.BehaviourNew, Object: obj("Purge")}, "to the new default"},
		"another explicit value":    {Arm{Predicted: domain.BehaviourNew, Object: obj("Archive")}, "only objects that leave the field unset"},
		"omitted":                   {Arm{Predicted: domain.BehaviourNew, Object: obj(nil)}, ""},
		"condition arm (no change)": {Arm{Predicted: domain.BehaviourPrevious, Object: obj("Keep")}, ""},
	} {
		if got := nonDiscriminating(rule, fx.Target, tc.arm); (tc.want == "") != (got == "") || !strings.Contains(got, tc.want) {
			t.Errorf("%s: %q", name, got)
		}
	}
	other := rule
	other.Subject = &domain.Subject{Family: domain.SubjectHelmValue, Product: "acme-ctl", Path: "retention.mode"}
	if got := nonDiscriminating(other, fx.Target, Arm{Predicted: domain.BehaviourNew, Object: obj("Purge")}); got != "" {
		t.Errorf("a subject that is not a field of the fixture object: %q", got)
	}
}

func TestEvaluateConsumerArmsNonDiscriminating(t *testing.T) {
	const v, f = runtime.ConsumerVerified, runtime.ConsumerFailed
	present, absent, omitted := domain.ArmConditionPresent, domain.ArmConditionAbsent, domain.ArmConditionAbsent+omittedSuffix+"-1"
	plansND := func(names ...string) []armPlan {
		ps := consumerPlans(names...)
		for i := range ps {
			if ps[i].Name == absent {
				ps[i].NonDiscriminating = "it sets the field explicitly to the new default"
			}
		}
		return ps
	}
	m := EvaluateConsumerArms(consumerRun(map[string][2]string{present: {v, v}, absent: {v, v}, omitted: {v, f}}), plansND(present, absent, omitted), nil)
	if m.Outcome != domain.MethodPass || !strings.Contains(m.Detail, "non-discriminating") {
		t.Errorf("omitted arm decides: %s %s", m.Outcome, m.Detail)
	}
	m = EvaluateConsumerArms(consumerRun(map[string][2]string{present: {v, v}, absent: {v, v}}), plansND(present, absent), nil)
	if m.Outcome != domain.MethodInconclusive || !strings.Contains(m.Detail, "cannot discriminate") {
		t.Errorf("only exposed arm non-discriminating: %s %s", m.Outcome, m.Detail)
	}
}
