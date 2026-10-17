package verify

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

const acmeFixtures = `apiVersion: ri.dev/v1alpha1
kind: ProductFixtures
product: acme-ctl
controller:
  values: {crds: {install: true}}
  references:
  - {url: "https://example.test/acme/chart/values.yaml", quote: "install the CRDs"}
subjects:
- id: widget
  target: {group: acme.example, version: v1, kind: Widget}
  exercise: rollover
  namespace: ri-verify
  prerequisites:
  - {apiVersion: acme.example/v1, kind: Store, metadata: {name: local}, spec: {type: memory}}
  template:
    apiVersion: acme.example/v1
    kind: Widget
    metadata: {name: placeholder}
    spec:
      storeRef: local
      outputName: "{{name}}-data"
  references:
  - {url: "https://example.test/acme/docs/widget.md", quote: "a Widget needs a store"}
`

func acmeSet(t *testing.T) *FixtureSet {
	t.Helper()
	s, err := ParseFixtures([]byte(acmeFixtures))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func rolloverExercise(withReaction bool) runtime.DeclaredExercise {
	x := catalog.Exercise{ID: "rollover", Summary: "roll every Widget over", Target: catalog.ObjectType{Group: "acme.example", Kind: "Widget"},
		Action: catalog.ActionAnnotate, Annotation: "acme.example/rollover", Families: []string{"behavior"},
		References: []catalog.ConfigReference{{URL: "https://example.test/acme/docs/rollover.md", Quote: "roll over"}}}
	if withReaction {
		x.Reaction = &catalog.ExerciseReaction{Kind: catalog.ReactionFieldChanged, Path: "status.generation"}
	}
	return runtime.DeclaredExercise{Product: "acme-ctl", Exercise: x}
}

var acmeProduces = []catalog.Relationship{{ID: "widget-output", Kind: catalog.RelProduces, From: catalog.ObjectType{Group: "acme.example", Kind: "Widget"},
	To: catalog.ObjectType{Kind: "ConfigMap"}, Path: "spec.outputName"}}

func TestBuildArms(t *testing.T) {
	fx, _ := acmeSet(t).Subject("acme.example", "Widget")
	arms, why := BuildArms(widgetRule(), fx)
	if why != "" {
		t.Fatal(why)
	}
	present, absent := arms[0], arms[1]
	spec := func(a Arm) map[string]any { return a.Object["spec"].(map[string]any) }
	if present.Name != domain.ArmConditionPresent || spec(present)["retention"].(map[string]any)["mode"] != "Keep" || present.Predicted != domain.BehaviourPrevious {
		t.Fatalf("present arm: %+v", present)
	}
	if _, set := spec(absent)["retention"]; set || absent.Predicted != domain.BehaviourNew || spec(absent)["outputName"] != "ri-arm-absent-data" {
		t.Fatalf("absent arm: %+v", absent)
	}
	if ids, _ := Produced(acmeProduces, fx.Target, absent.Object); len(ids) != 1 || ids[0] != "core/v1/ConfigMap/ri-verify/ri-arm-absent-data" {
		t.Fatalf("produced = %v", ids)
	}

	// without an overlap, "exposed while unset" is mitigated by the previous value
	r := widgetRule()
	r.Applicability.Overlap = nil
	if arms, why := BuildArms(r, fx); why != "" || spec(arms[0])["retention"].(map[string]any)["mode"] != "Keep" {
		t.Fatalf("derived mitigation: %q %+v", why, arms)
	}

	for name, mutate := range map[string]func(*domain.SemanticAssertion){
		"an environment-wide condition": func(a *domain.SemanticAssertion) {
			a.Applicability.Overlap = &domain.Condition{Op: domain.OpCLIFlag, Name: "--legacy-retention", State: domain.StateSet}
		},
		"another kind": func(a *domain.SemanticAssertion) {
			a.Applicability.Overlap = &domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Store",
				Of: []domain.Condition{{Op: domain.OpField, Path: "spec.type", State: domain.StateSet}}}
		},
		"a mitigation the exposure still matches": func(a *domain.SemanticAssertion) {
			a.Applicability.Overlap = &domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget",
				Of: []domain.Condition{{Op: domain.OpField, Path: "spec.storeRef", State: domain.StateSet}}}
		},
		"a list path": func(a *domain.SemanticAssertion) {
			a.Applicability.Overlap = &domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget",
				Of: []domain.Condition{{Op: domain.OpField, Path: "spec.rules[].mode", State: domain.StateSet}}}
		},
	} {
		r := widgetRule()
		mutate(&r)
		if _, why := BuildArms(r, fx); why == "" {
			t.Errorf("%s: arms built; want not synthesisable", name)
		}
	}
}

func TestParseFixturesRejectsInvalidDeclarations(t *testing.T) {
	for name, edit := range map[string][2]string{
		"no exercise":       {"exercise: rollover", "exercise: \"\""},
		"wrong template":    {"kind: Widget\n    metadata", "kind: Gadget\n    metadata"},
		"evaluation source": {"https://example.test/acme/docs/widget.md", "https://example.test/eval/investigations/x"},
		"unknown field":     {"namespace: ri-verify", "namespace: ri-verify\n  extra: 1"},
	} {
		if _, err := ParseFixtures([]byte(strings.Replace(acmeFixtures, edit[0], edit[1], 1))); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestDeclaredFixturesLoad(t *testing.T) {
	sets, err := LoadFixtures("../../../products/exercises/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) == 0 {
		t.Fatal("no fixtures declared")
	}
}

// --- the four arm combinations ------------------------------------------------------------

func plans() []armPlan {
	return []armPlan{
		{Arm: Arm{Name: domain.ArmConditionPresent, Predicted: domain.BehaviourPrevious, Condition: "kept"}, subject: "acme.example/v1/Widget/ri-verify/ri-arm-present",
			objects: []string{"acme.example/v1/Widget/ri-verify/ri-arm-present", "core/v1/ConfigMap/ri-verify/ri-arm-present-data"}},
		{Arm: Arm{Name: domain.ArmConditionAbsent, Predicted: domain.BehaviourNew, Condition: "unset"}, subject: "acme.example/v1/Widget/ri-verify/ri-arm-absent",
			objects: []string{"acme.example/v1/Widget/ri-verify/ri-arm-absent", "core/v1/ConfigMap/ri-verify/ri-arm-absent-data"}},
	}
}

func fixtureResult(presentChanged, absentChanged bool) *runtime.FixtureResult {
	r := &runtime.Run{Product: "acme-ctl", From: "1.9.4", To: "2.0.0", Cluster: "ri-verify-00000000", Status: runtime.RunCompleted, UpgradeCompleted: true}
	for _, phase := range []string{"from", "to"} {
		r.Exercises = append(r.Exercises, runtime.ExerciseRun{Exercise: "acme-ctl/rollover", Phase: phase,
			Reacted: []string{"acme.example/v1/Widget/ri-verify/ri-arm-present", "acme.example/v1/Widget/ri-verify/ri-arm-absent"}})
	}
	if presentChanged {
		r.Degradations = append(r.Degradations, runtime.Degradation{Object: "core/v1/ConfigMap/ri-verify/ri-arm-present-data", Window: "exercise", Signature: "data:payload", Inconclusive: true})
	}
	if absentChanged {
		r.Degradations = append(r.Degradations, runtime.Degradation{Object: "core/v1/ConfigMap/ri-verify/ri-arm-absent-data", Window: "exercise", Signature: "data:payload", Inconclusive: true})
	}
	return &runtime.FixtureResult{Run: r, Artifacts: []domain.ArtifactRef{{ID: "art-1", Path: "derived/runtime/fx/run.json", URI: "ri:runtime/run.json", Digest: "sha256:run"}}}
}

func TestEvaluateArmsFourCombinations(t *testing.T) {
	for _, tc := range []struct {
		present, absent bool
		want            domain.MethodOutcome
		detail          string
	}{
		{false, true, domain.MethodPass, "with the condition the behaviour at TO matched FROM"},
		{true, true, domain.MethodFail, "does not preserve the previous behaviour"},
		{false, false, domain.MethodInconclusive, "the upstream change was not demonstrated"},
		{true, false, domain.MethodFail, "does not preserve the previous behaviour"},
	} {
		m := EvaluateArms(fixtureResult(tc.present, tc.absent), plans(), "acme-ctl/rollover")
		if m.Outcome != tc.want || !strings.Contains(m.Detail, tc.detail) {
			t.Errorf("present changed %v, absent changed %v: %s %s", tc.present, tc.absent, m.Outcome, m.Detail)
		}
		if len(m.Arms) != 2 {
			t.Errorf("arms: %+v", m.Arms)
		}
	}
}

func TestEvaluateArmsNeverPassesAQuietOrBrokenRun(t *testing.T) {
	unanswered := fixtureResult(false, true)
	unanswered.Run.Exercises[1].Reacted = unanswered.Run.Exercises[1].Reacted[:1] // absent arm silent at TO
	notRun := fixtureResult(false, true)
	notRun.Run.Status, notRun.Run.Reason = runtime.RunNotRun, "kind not on PATH"
	broken := fixtureResult(false, true)
	broken.Run.Status, broken.Run.Reason = runtime.RunError, "timed out"
	incomplete := fixtureResult(false, true)
	incomplete.Run.UpgradeCompleted, incomplete.Run.UpgradeProblems = false, []string{"controller not ready"}
	sandbox := fixtureResult(false, true)
	sandbox.Run.Degradations = append(sandbox.Run.Degradations, runtime.Degradation{Object: "acme.example/v1/Widget/ri-verify/ri-arm-present", Signature: "not-ready", Infrastructure: true})
	unstored := fixtureResult(false, true)
	unstored.Artifacts = nil
	otherExercise := fixtureResult(false, true)
	for i := range otherExercise.Run.Exercises {
		otherExercise.Run.Exercises[i].Exercise = "acme-ctl/other"
	}
	for name, tc := range map[string]struct {
		res  *runtime.FixtureResult
		want domain.MethodOutcome
	}{
		"exercise unanswered on an arm": {unanswered, domain.MethodInconclusive},
		"run not run":                   {notRun, domain.MethodNotRun},
		"run error":                     {broken, domain.MethodInconclusive},
		"upgrade incomplete":            {incomplete, domain.MethodInconclusive},
		"sandbox problem":               {sandbox, domain.MethodInconclusive},
		"no stored observations":        {unstored, domain.MethodInconclusive},
		"another exercise answered":     {otherExercise, domain.MethodInconclusive},
	} {
		if m := EvaluateArms(tc.res, plans(), "acme-ctl/rollover"); m.Outcome != tc.want {
			t.Errorf("%s: %s (%s)", name, m.Outcome, m.Detail)
		}
	}
}

// --- ExerciseRule wiring ------------------------------------------------------------------

type fakeRenderer struct{ versions []string }

func (r *fakeRenderer) RenderController(_ context.Context, product, version string, c ControllerFixture) (runtime.RawRender, error) {
	r.versions = append(r.versions, version)
	return runtime.RawRender{Manifests: []byte("apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: acme, namespace: acme}\n"), Namespace: "acme"}, nil
}

type fakeRunner struct {
	spec runtime.FixtureSpec
	res  *runtime.FixtureResult
}

func (r *fakeRunner) RunFixture(_ context.Context, spec runtime.FixtureSpec) (*runtime.FixtureResult, error) {
	r.spec = spec
	return r.res, nil
}

func TestExerciseRuleBuildsTheFixtureFromTheRule(t *testing.T) {
	rnd, run := &fakeRenderer{}, &fakeRunner{res: fixtureResult(false, true)}
	in := ExerciseInput{Product: "acme-ctl", Rule: widgetRule(), Bounds: bounds, Fixtures: acmeSet(t), Exercises: []runtime.DeclaredExercise{rolloverExercise(true)},
		Relationships: acmeProduces, Renderer: rnd, Runner: run}
	m, _ := ExerciseRule(context.Background(), in)
	if m.Outcome != domain.MethodPass {
		t.Fatalf("%s: %s", m.Outcome, m.Detail)
	}
	if strings.Join(rnd.versions, ",") != "1.9.4,2.0.0" || run.spec.From != "1.9.4" || run.spec.To != "2.0.0" || !strings.HasPrefix(run.spec.ID, "fx-") {
		t.Fatalf("controller versions %v, spec %+v", rnd.versions, run.spec)
	}
	objs := string(run.spec.Objects)
	for _, want := range []string{"kind: Store", "name: ri-arm-present", "mode: Keep", "name: ri-arm-absent", "outputName: ri-arm-absent-data"} {
		if !strings.Contains(objs, want) {
			t.Fatalf("fixture objects lack %q:\n%s", want, objs)
		}
	}
	if strings.Count(objs, "mode:") != 1 {
		t.Fatalf("the condition-absent arm must leave the field unset:\n%s", objs)
	}
	if len(run.spec.Observe) != 2 {
		t.Fatalf("observe = %v", run.spec.Observe)
	}

	for name, mutate := range map[string]func(*ExerciseInput){
		"no fixtures":           func(in *ExerciseInput) { in.Fixtures = nil },
		"no reaction declared":  func(in *ExerciseInput) { in.Exercises = []runtime.DeclaredExercise{rolloverExercise(false)} },
		"exercise not declared": func(in *ExerciseInput) { in.Exercises = nil },
		"no bounds":             func(in *ExerciseInput) { in.Bounds = nil },
	} {
		x := in
		mutate(&x)
		if m, _ := ExerciseRule(context.Background(), x); m.Outcome != domain.MethodNotRun {
			t.Errorf("%s: %s", name, m.Outcome)
		}
	}
}
