package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// ExerciseProducer names the rule exercise (component@vN).
const ExerciseProducer = "verify.exercise@v1"

// ControllerRenderer renders the product's controller at one version with
// the fixture's values (the wiring uses the render engine and its cache).
type ControllerRenderer interface {
	RenderController(ctx context.Context, product, version string, c ControllerFixture) (runtime.RawRender, error)
}

// FixtureRunner runs a fixture in a throwaway cluster (runtime.Validator).
type FixtureRunner interface {
	RunFixture(ctx context.Context, spec runtime.FixtureSpec) (*runtime.FixtureResult, error)
}

// ExerciseInput is everything a rule exercise needs.
type ExerciseInput struct {
	Product string
	Rule    domain.SemanticAssertion
	Bounds  *domain.ProofBounds
	// Fixtures, Exercises and Relationships are the product's declarations.
	Fixtures      *FixtureSet
	Exercises     []runtime.DeclaredExercise
	Relationships []catalog.Relationship
	// KubernetesVersion selects the node image ("" = kind's default).
	KubernetesVersion string
	Renderer          ControllerRenderer
	Runner            FixtureRunner
}

// armPlan is an arm with the objects observed for it.
type armPlan struct {
	Arm
	subject string   // live id of the subject object
	objects []string // subject + produced (+ consuming objects and theirs)
	// consumer is the arm's consumer check (consumer observation only).
	consumer *runtime.ConsumerCheck
}

// ExerciseRule runs the two-arm exercise of a rule. It never runs against an
// evaluation environment: only the controller and the synthetic objects are
// installed. Missing declarations or an unsynthesisable rule ⇒ not-run.
func ExerciseRule(ctx context.Context, in ExerciseInput) (domain.ProofMethod, []string) {
	m := domain.ProofMethod{Method: domain.ProofRuleExercise, Producer: ExerciseProducer, Level: domain.LevelObserved}
	notRun := func(format string, args ...any) (domain.ProofMethod, []string) {
		m.Outcome, m.Detail = domain.MethodNotRun, fmt.Sprintf(format, args...)
		return m, nil
	}
	switch {
	case in.Bounds == nil:
		return notRun("the rule states no release bounds: there is no FROM and TO to install")
	case in.Rule.Subject == nil:
		return notRun("the rule names no subject")
	case in.Fixtures == nil:
		return notRun("no fixtures are declared for %s (products/exercises/fixtures/%s.yaml)", in.Product, in.Product)
	case in.Renderer == nil || in.Runner == nil:
		return notRun("no controller renderer or cluster runner configured")
	}
	fx, ok := in.Fixtures.Subject(in.Rule.Subject.Group, in.Rule.Subject.Kind)
	if !ok {
		return notRun("no fixture is declared for the subject kind %s/%s", in.Rule.Subject.Group, in.Rule.Subject.Kind)
	}
	var ex *runtime.DeclaredExercise
	for i := range in.Exercises {
		x := in.Exercises[i]
		if x.Product == in.Product && x.Exercise.ID == fx.Exercise {
			ex = &in.Exercises[i]
		}
	}
	switch {
	case ex == nil:
		return notRun("the fixture's exercise %s/%s is not declared", in.Product, fx.Exercise)
	case ex.Exercise.Target.Group != fx.Target.Group || ex.Exercise.Target.Kind != fx.Target.Kind:
		return notRun("exercise %s targets %s/%s, not the subject kind", ex.Key(), ex.Exercise.Target.Group, ex.Exercise.Target.Kind)
	case ex.Exercise.Reaction == nil:
		return notRun("exercise %s declares no controller reaction: it can never show that the controller acted", ex.Key())
	}
	cf, why := consumerFor(in.Rule, fx)
	if why != "" {
		return notRun("%s", why)
	}
	var arms []consumerArm
	var limitations []string
	if cf != nil {
		if arms, limitations, why = BuildConsumerArms(in.Rule, fx, cf); why != "" {
			return notRun("the rule cannot be synthesised on the consumer observation %s: %s", cf.ID, why)
		}
	} else {
		plain, why := BuildArms(in.Rule, fx)
		if why != "" {
			return notRun("the rule cannot be synthesised on a fixture: %s", why)
		}
		for _, a := range plain {
			arms = append(arms, consumerArm{Arm: a})
		}
	}
	var plans []armPlan
	var consumers []runtime.ConsumerCheck
	var stream strings.Builder
	write := func(obj map[string]any) error {
		b, err := yaml.Marshal(obj)
		if err != nil {
			return err
		}
		stream.WriteString("---\n")
		stream.Write(b)
		return nil
	}
	for _, p := range fx.Prerequisites {
		obj := deepCopy(p).(map[string]any)
		if meta, _ := obj["metadata"].(map[string]any); meta != nil && meta["namespace"] == nil {
			meta["namespace"] = fx.Namespace
		}
		if err := write(obj); err != nil {
			return notRun("fixture prerequisite: %v", err)
		}
	}
	var observe []string
	for _, a := range arms {
		p := armPlan{Arm: a.Arm, subject: objectID(a.Object)}
		for _, obj := range append([]map[string]any{a.Object}, a.consumers...) {
			if err := write(obj); err != nil {
				return notRun("fixture object: %v", err)
			}
			p.objects = append(p.objects, objectID(obj))
			if obj["kind"] != fx.Target.Kind || !strings.HasPrefix(objectID(obj), groupOr(fx.Target.Group)+"/") {
				continue // a consuming object of another kind: its outputs are not declared
			}
			produced, notes := Produced(in.Relationships, fx.Target, obj)
			limitations = append(limitations, notes...)
			observe = append(observe, produced...)
			p.objects = append(p.objects, produced...)
		}
		if cf != nil {
			c := consumerCheckOf(a, cf, fx, ex.Key())
			p.consumer = &c
			consumers = append(consumers, c)
		}
		plans = append(plans, p)
	}
	from, err := in.Renderer.RenderController(ctx, in.Product, in.Bounds.From, in.Fixtures.Controller)
	if err != nil {
		return notRun("the controller at %s cannot be rendered: %v", in.Bounds.From, err)
	}
	to, err := in.Renderer.RenderController(ctx, in.Product, in.Bounds.To, in.Fixtures.Controller)
	if err != nil {
		return notRun("the controller at %s cannot be rendered: %v", in.Bounds.To, err)
	}
	objects := stream.String()
	idParts := []string{in.Product, in.Bounds.From, in.Bounds.To, in.Fixtures.Digest(), objects}
	if len(consumers) > 0 {
		b, _ := json.Marshal(consumers)
		idParts = append(idParts, string(b))
	}
	spec := runtime.FixtureSpec{ID: "fx-" + domain.ShortHash(idParts...),
		Product: in.Product, From: in.Bounds.From, To: in.Bounds.To, KubernetesVersion: in.KubernetesVersion,
		FromController: from, ToController: to, Objects: []byte(objects), Namespace: fx.Namespace, Observe: observe, Consumers: consumers}
	res, err := in.Runner.RunFixture(ctx, spec)
	if err != nil {
		return notRun("the fixture run could not start: %v", err)
	}
	if cf != nil {
		return EvaluateConsumerArms(res, plans), limitations
	}
	m = EvaluateArms(res, plans, ex.Key())
	return m, limitations
}

// EvaluateArms decides the exercise from a fixture run: confirmed (pass) only
// when both arms answered the exercise at FROM and at TO, the
// condition-present arm behaved at TO as at FROM, and the condition-absent
// arm did not. The condition-present arm changing refutes the rule (fail);
// neither arm changing means the change itself was not demonstrated, so
// neither is the mitigation (inconclusive); a run that did not complete, an
// unanswered exercise or a sandbox problem is inconclusive or not-run.
func EvaluateArms(res *runtime.FixtureResult, plans []armPlan, exercise string) domain.ProofMethod {
	m := domain.ProofMethod{Method: domain.ProofRuleExercise, Producer: ExerciseProducer, Level: domain.LevelObserved}
	r := res.Run
	summary := fmt.Sprintf("fixture %s %s → %s in %s (%s, %.0fs)", r.Product, r.From, r.To, r.Cluster, r.Status, r.DurationSeconds)
	for _, a := range res.Artifacts {
		m.Evidence = append(m.Evidence, domain.NewEvidence(domain.EvidenceStructured, a.ID, a.URI, a.Path, "runtime "+a.Path, a.Digest, time.Time{}))
	}
	switch {
	case r.Status == runtime.RunNotRun:
		m.Outcome, m.Detail, m.Evidence = domain.MethodNotRun, summary+": "+r.Reason, nil
		return m
	case r.Status != runtime.RunCompleted:
		m.Outcome, m.Detail, m.Evidence = domain.MethodInconclusive, summary+": "+r.Reason, nil
		return m
	case !r.UpgradeCompleted:
		m.Outcome, m.Detail, m.Evidence = domain.MethodInconclusive, summary+": the upgrade did not complete: "+strings.Join(r.UpgradeProblems, "; "), nil
		return m
	}
	reacted := map[string]map[string]bool{"from": {}, "to": {}}
	for _, er := range r.Exercises {
		if er.Exercise != exercise {
			continue
		}
		for _, id := range er.Reacted {
			reacted[er.Phase][id] = true
		}
	}
	infra := false
	var armsOut []domain.ProofArm
	byName := map[string]domain.ProofArm{}
	for _, p := range plans {
		a := domain.ProofArm{Name: p.Name, Objects: p.objects, Condition: p.Condition, Predicted: p.Predicted,
			ReactedFrom: reacted["from"][p.subject], ReactedTo: reacted["to"][p.subject]}
		in := map[string]bool{}
		for _, o := range p.objects {
			in[o] = true
		}
		for _, d := range r.Degradations {
			if !in[d.Object] {
				continue
			}
			if d.Infrastructure {
				infra = true
				continue
			}
			a.Changed = true
			a.Signatures = append(a.Signatures, d.Object+" "+d.Signature)
		}
		sort.Strings(a.Signatures)
		switch {
		case !a.ReactedFrom || !a.ReactedTo:
			a.Observed = domain.BehaviourUndetermined
		case a.Changed:
			a.Observed = domain.BehaviourNew
		default:
			a.Observed = domain.BehaviourPrevious
		}
		armsOut = append(armsOut, a)
		byName[a.Name] = a
		m.Checks = append(m.Checks, domain.ProofCheck{Subject: a.Name, Outcome: armOutcome(a), Detail: armDetail(a)})
	}
	m.Arms = armsOut
	present, okP := byName[domain.ArmConditionPresent]
	absent, okA := byName[domain.ArmConditionAbsent]
	switch {
	case !okP || !okA:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": both arms are required"
	case infra:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": the sandbox could not run an arm's objects (image pull)"
	case present.Observed == domain.BehaviourUndetermined || absent.Observed == domain.BehaviourUndetermined:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+fmt.Sprintf(": exercise %s was not answered on every arm at both versions — a quiet window proves nothing", exercise)
	case present.Changed:
		m.Outcome, m.Detail = domain.MethodFail, summary+": with the mitigating condition the behaviour still changed at TO ("+strings.Join(present.Signatures, "; ")+"): the condition does not preserve the previous behaviour"
	case !absent.Changed:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": without the condition the behaviour did not change either: the upstream change was not demonstrated, so neither is the mitigation"
	default:
		m.Outcome, m.Detail = domain.MethodPass, summary+": with the condition the behaviour at TO matched FROM; without it, it changed ("+strings.Join(absent.Signatures, "; ")+")"
	}
	if m.Outcome == domain.MethodPass && len(m.Evidence) == 0 {
		m.Outcome, m.Detail = domain.MethodInconclusive, m.Detail+"; but no observation artifacts were stored, so the result cannot be audited"
	}
	if m.Outcome != domain.MethodPass && m.Outcome != domain.MethodFail {
		m.Evidence = nil
	}
	return m
}

func groupOr(g string) string {
	if g == "" {
		return "core"
	}
	return g
}

func armOutcome(a domain.ProofArm) domain.MethodOutcome {
	switch {
	case a.Observed == domain.BehaviourUndetermined:
		return domain.MethodInconclusive
	case a.Observed == a.Predicted:
		return domain.MethodPass
	case a.Name == domain.ArmConditionPresent:
		return domain.MethodFail
	}
	return domain.MethodInconclusive
}

func armDetail(a domain.ProofArm) string {
	d := fmt.Sprintf("%s (%s): predicted %s, observed %s; exercise answered at FROM %v, at TO %v",
		a.Name, a.Condition, a.Predicted, a.Observed, a.ReactedFrom, a.ReactedTo)
	if len(a.Signatures) > 0 {
		d += "; differences: " + strings.Join(a.Signatures, "; ")
	}
	return d
}
