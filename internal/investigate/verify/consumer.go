package verify

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// Consumer observation (lane verify2; PROTOCOL amendment 2026-10-17 "B0-v8
// outcome"). The produced-object observation (exercise.go) can show that an
// explicit setting preserves an object's previous behaviour; it cannot test a
// rule whose claim is about consequence — that an object changes without any
// consumer observing it. A subject fixture may therefore declare consumer
// observations: the role the subject object plays for a consumer (fields it
// holds in that role, e.g. "it issues certificates"), the objects that
// consume it, the trust a consumer captures before re-issuance, the
// re-issuance order and the certificate verified afterwards
// (runtime.ConsumerCheck). A rule whose applicability names a role field
// distinguishes subjects by that role, so its exercise is the consumer
// observation; any other rule keeps the produced-object observation
// unchanged.
//
// Arms come from the rule, in the role: every alternative of the mitigating
// (overlap) condition that holds together with the role fields and does not
// meet the exposure is a shielded arm (predicted: the consumer still
// verifies at TO); every alternative of the exposure condition that holds in
// the role and does not meet the overlap is an exposed arm (predicted: it
// verifies at FROM and fails at TO). Alternatives that contradict the role
// are not exercised in the role. A shielded alternative that contradicts the
// role is exercised outside it instead (lane applicability, mission E2): an
// outside-role arm is the alternative's object with the same consuming
// objects, predicted to keep its consumers (as any shielded arm) or not to
// form the relationship at all; a consumer that verifies at FROM and fails at
// TO refutes the rule (the object plays the role after all, and is exposed),
// anything else is recorded and never counts toward a pass on its own.
// Exposed alternatives that contradict the role stay limitations. Outcome
// (fixed by the protocol): pass iff every shielded arm verifies at FROM and
// TO and every exposed arm verifies at FROM and fails at TO; fail if a
// shielded arm fails at either version (or an outside-role arm breaks);
// otherwise inconclusive — a quiet exposed arm is never a pass, and without
// an exposed arm the rule can be refuted but not confirmed.

// ConsumerExerciseProducer names the rule exercise when its consumer
// observation decides it (component@vN; the produced-object observation
// stays ExerciseProducer). v2 (lane verify2, D-0025): non-discriminating
// exposed arms never decide; the role-absent observation; scope refinements.
// v3 (lane verify3, D-0026): log lines and warning events are diagnostics;
// default probes (PD-5) are recorded and never decide. v4 (lane verify4,
// D-0027): an in-role default probe whose relationship cannot form is
// recorded role-cannot-form with its object-state evidence (PD-6).
const ConsumerExerciseProducer = "verify.exercise-consumer@v4"

// ConsumerFixture is one declared consumer observation of a subject kind.
type ConsumerFixture struct {
	ID      string `yaml:"id" json:"id"`
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
	// Role are the fields the subject object holds in this role.
	Role []FieldValue `yaml:"role" json:"role"`
	// Objects are the consuming objects, applied per arm (NamePlaceholder =
	// the arm's subject object name; the subject's namespace when unnamed).
	Objects []map[string]any `yaml:"objects" json:"objects"`
	// Trust is captured before re-issuance; Verify is checked after it.
	Trust  SecretRef `yaml:"trust" json:"trust"`
	Verify SecretRef `yaml:"verify" json:"verify"`
	// Reissue are names of objects of the subject kind (the arm's subject
	// object or a consuming object), re-issued in this order.
	Reissue    []string                  `yaml:"reissue" json:"reissue"`
	References []catalog.ConfigReference `yaml:"references" json:"references"`
	// Outside is the role-absent observation (optional; lane verify2,
	// D-0025): how a consumer uses the key material of a subject that does
	// not play the role. Without it an outside-role arm keeps the role's
	// consuming objects (whose relationship may then not form at all).
	Outside *OutsideRole `yaml:"outside,omitempty" json:"outside,omitempty"`
}

// OutsideRole is a consumer observation of a subject outside the role: the
// role fields such a subject holds (Role, the same fields with other values;
// a scope refinement's outside condition, refinement.go), fields set on the
// arm's subject for the observation (Subject, e.g. a reference to an issuer
// the Objects declare), the consuming objects, and the trust captured before
// re-issuance, the objects re-issued and the certificate verified after it.
type OutsideRole struct {
	Summary    string                    `yaml:"summary,omitempty" json:"summary,omitempty"`
	Role       []FieldValue              `yaml:"role" json:"role"`
	Subject    []FieldValue              `yaml:"subject,omitempty" json:"subject,omitempty"`
	Objects    []map[string]any          `yaml:"objects,omitempty" json:"objects,omitempty"`
	Trust      SecretRef                 `yaml:"trust" json:"trust"`
	Verify     SecretRef                 `yaml:"verify" json:"verify"`
	Reissue    []string                  `yaml:"reissue" json:"reissue"`
	References []catalog.ConfigReference `yaml:"references" json:"references"`
}

// FieldValue is a field and the value it holds.
type FieldValue struct {
	Path  string `yaml:"path" json:"path"`
	Value any    `yaml:"value" json:"value"`
}

// SecretRef names one key of a Secret (NamePlaceholder allowed).
type SecretRef struct {
	Secret string `yaml:"secret" json:"secret"`
	Key    string `yaml:"key" json:"key"`
}

func (cf ConsumerFixture) validate(x SubjectFixture) []error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	if len(cf.Role) == 0 {
		bad("role: at least one field is required (the consumer observation applies to rules that name it)")
	}
	for _, r := range cf.Role {
		if _, err := catalog.SplitFieldPath(r.Path); err != nil || strings.Contains(r.Path, "[]") || r.Value == nil {
			bad("role field %q needs a non-list path and a value", r.Path)
		}
	}
	names := map[string]bool{NamePlaceholder: true}
	for i, o := range cf.Objects {
		meta, _ := o["metadata"].(map[string]any)
		name, _ := meta["name"].(string)
		if o["apiVersion"] == nil || o["kind"] == nil || name == "" {
			bad("objects[%d] needs apiVersion, kind and metadata.name", i)
			continue
		}
		apiVersion := x.Target.Version
		if x.Target.Group != "" {
			apiVersion = x.Target.Group + "/" + x.Target.Version
		}
		if o["apiVersion"] == apiVersion && o["kind"] == x.Target.Kind {
			names[name] = true
		}
	}
	if len(cf.Reissue) == 0 {
		bad("reissue: at least one object is required")
	}
	for _, n := range cf.Reissue {
		if !names[n] {
			bad("reissue %q is neither the subject object (%s) nor a consuming object of the subject kind", n, NamePlaceholder)
		}
	}
	for _, s := range []SecretRef{cf.Trust, cf.Verify} {
		if s.Secret == "" || s.Key == "" {
			bad("trust and verify need a secret and a key")
		}
	}
	if o := cf.Outside; o != nil {
		for _, e := range o.validate(cf, x) {
			bad("outside: %v", e)
		}
	}
	return errs
}

func (o OutsideRole) validate(cf ConsumerFixture, x SubjectFixture) []error {
	var errs []error
	bad := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	role := map[string]any{}
	for _, r := range cf.Role {
		role[r.Path] = r.Value
	}
	if len(o.Role) == 0 {
		bad("role: the fields a subject outside the role holds are required")
	}
	for _, r := range o.Role {
		v, ok := role[r.Path]
		switch {
		case !ok:
			bad("role field %q is not a field of the role", r.Path)
		case r.Value == nil || encode(r.Value) == encode(v):
			bad("role field %q needs a value other than the role's", r.Path)
		}
	}
	for _, f := range o.Subject {
		if _, err := catalog.SplitFieldPath(f.Path); err != nil || strings.Contains(f.Path, "[]") || f.Value == nil {
			bad("subject field %q needs a non-list path and a value", f.Path)
		}
		if _, ok := role[f.Path]; ok {
			bad("subject field %q is a role field (the arm's condition sets it)", f.Path)
		}
	}
	shadow := cf
	shadow.Objects, shadow.Reissue, shadow.Trust, shadow.Verify, shadow.Outside = o.Objects, o.Reissue, o.Trust, o.Verify, nil
	for _, e := range shadow.validate(x) {
		bad("%v", e)
	}
	if len(o.References) == 0 {
		bad("at least one upstream reference is required")
	}
	return errs
}

// roleLeaves are the role fields as condition leaves.
func (cf ConsumerFixture) roleLeaves() ([]domain.Condition, error) {
	var out []domain.Condition
	for _, r := range cf.Role {
		b, err := json.Marshal(r.Value)
		if err != nil {
			return nil, err
		}
		out = append(out, domain.Condition{Op: domain.OpField, Path: r.Path, State: domain.StateEquals, Values: []string{string(b)}})
	}
	return out, nil
}

// consumerFor returns the consumer observation the rule's applicability
// selects: the one whose role field the rule names (nil: none does).
func consumerFor(rule domain.SemanticAssertion, fx *SubjectFixture) (*ConsumerFixture, string) {
	if rule.Applicability == nil {
		return nil, ""
	}
	paths := map[string]bool{}
	var walk func(domain.Condition)
	walk = func(c domain.Condition) {
		if c.Op == domain.OpField {
			paths[c.Path] = true
		}
		for _, x := range c.Of {
			walk(x)
		}
	}
	walk(rule.Applicability.Exposure)
	if rule.Applicability.Overlap != nil {
		walk(*rule.Applicability.Overlap)
	}
	var hit []*ConsumerFixture
	for i := range fx.Consumers {
		for _, r := range fx.Consumers[i].Role {
			if paths[r.Path] {
				hit = append(hit, &fx.Consumers[i])
				break
			}
		}
	}
	switch len(hit) {
	case 0:
		return nil, ""
	case 1:
		return hit[0], ""
	}
	return nil, "the rule names role fields of several consumer observations; which one tests it is ambiguous"
}

// maxAlternatives bounds the expansion of a condition into alternatives.
const maxAlternatives = 16

// alternatives expands a condition on the subject kind into alternatives,
// each a conjunction of field leaves of the one subject object (resource,
// all, any and field nodes; anything else cannot be synthesised).
func alternatives(c domain.Condition, t FixtureType, scoped bool) ([][]domain.Condition, string) {
	product := func(of []domain.Condition) ([][]domain.Condition, string) {
		out := [][]domain.Condition{nil}
		for _, x := range of {
			alts, why := alternatives(x, t, scoped)
			if why != "" {
				return nil, why
			}
			var next [][]domain.Condition
			for _, a := range out {
				for _, b := range alts {
					next = append(next, append(append([]domain.Condition(nil), a...), b...))
				}
			}
			if len(next) > maxAlternatives {
				return nil, fmt.Sprintf("the condition has more than %d alternatives", maxAlternatives)
			}
			out = next
		}
		return out, ""
	}
	switch c.Op {
	case domain.OpAll:
		return product(c.Of)
	case domain.OpAny:
		var out [][]domain.Condition
		for _, x := range c.Of {
			alts, why := alternatives(x, t, scoped)
			if why != "" {
				return nil, why
			}
			out = append(out, alts...)
		}
		if len(out) == 0 || len(out) > maxAlternatives {
			return nil, fmt.Sprintf("an any condition needs between 1 and %d alternatives", maxAlternatives)
		}
		return out, ""
	case domain.OpResource:
		switch {
		case scoped:
			return nil, "a resource condition nested in another cannot be synthesised on one object"
		case c.Group != t.Group || c.Kind != t.Kind:
			return nil, fmt.Sprintf("it scopes %s/%s, not the subject kind %s/%s", c.Group, c.Kind, t.Group, t.Kind)
		case c.Name != "":
			return nil, "it names one resource; a synthetic fixture cannot reproduce a name-specific condition"
		}
		scoped = true
		alts, why := product(c.Of)
		if why != "" {
			return nil, why
		}
		for _, a := range alts {
			if len(a) == 0 {
				return nil, "it states no field of the subject"
			}
		}
		return alts, ""
	case domain.OpField:
		switch {
		case !scoped:
			return nil, "a field condition outside a resource condition"
		case strings.Contains(c.Path, "[]"):
			return nil, fmt.Sprintf("the list path %s cannot be synthesised", c.Path)
		}
		switch c.State {
		case domain.StateUnset, domain.StateSet, domain.StateEquals, domain.StateNotEquals:
			return [][]domain.Condition{{c}}, ""
		}
		return nil, fmt.Sprintf("field state %s cannot be synthesised", c.State)
	}
	return nil, fmt.Sprintf("a %s condition is not a condition on the subject object", c.Op)
}

func holdsAny(alts [][]domain.Condition, obj map[string]any) (ok, known bool) {
	known = true
	for _, a := range alts {
		r, k := holds(a, obj)
		if r && k {
			return true, true
		}
		known = known && k
	}
	return false, known
}

// consumerArm is a shielded or exposed arm with its consuming objects.
type consumerArm struct {
	Arm
	consumers []map[string]any
	// outside: a default probe of an outside-role arm, observed by the
	// role-absent observation like its base arm (probe.go).
	outside bool
}

// BuildConsumerArms builds the arms of a rule's consumer observation (see
// the package comment above). limitations name alternatives not exercised;
// why is set when no arm can be built.
func BuildConsumerArms(rule domain.SemanticAssertion, fx *SubjectFixture, cf *ConsumerFixture) (arms []consumerArm, limitations []string, why string) {
	if rule.Subject == nil || rule.Applicability == nil || rule.Change == nil {
		return nil, nil, "the rule has no subject, change or applicability to build a fixture from"
	}
	role, err := cf.roleLeaves()
	if err != nil {
		return nil, nil, "consumer role: " + err.Error()
	}
	exposure, why := alternatives(rule.Applicability.Exposure, fx.Target, false)
	if why != "" {
		return nil, nil, "exposure condition: " + why
	}
	var overlap [][]domain.Condition
	if rule.Applicability.Overlap != nil {
		if overlap, why = alternatives(*rule.Applicability.Overlap, fx.Target, false); why != "" {
			return nil, nil, "mitigating (overlap) condition: " + why
		}
	} else if len(exposure) == 1 && len(exposure[0]) == 1 && exposure[0][0].State == domain.StateUnset {
		overlap = [][]domain.Condition{{{Op: domain.OpField, Path: exposure[0][0].Path, State: domain.StateSet}}}
	} else {
		return nil, nil, "the rule states no mitigating (overlap) condition and its exposure is not a single unset field"
	}
	roleText := describe(role, nil)
	build := func(kind string, alt, other [][]domain.Condition, otherName string) {
		for j, a := range alt {
			armName, objName, predicted := domain.ArmConditionPresent, armPresentName, domain.BehaviourPrevious
			if kind == "exposed" {
				armName, objName, predicted = domain.ArmConditionAbsent, armAbsentName, domain.BehaviourNew
			}
			if n := countOf(arms, armName); n > 0 {
				armName, objName = fmt.Sprintf("%s-%d", armName, n+1), fmt.Sprintf("%s-%d", objName, n+1)
			}
			text := describe(a, rule.Change.Before)
			outside := false
			obj, err := buildObject(fx, objName, append(append([]domain.Condition(nil), a...), role...), rule.Change.Before)
			if err != nil && kind == "shielded" {
				// exercised outside the role (see the comment above)
				n := 0
				for _, x := range arms {
					if isOutsideRole(x.Name) {
						n++
					}
				}
				armName, objName = fmt.Sprintf("%s%s-%d", domain.ArmConditionPresent, outsideRoleSuffix, n+1), fmt.Sprintf("ri-arm-outside-%d", n+1)
				obj, err = buildObject(fx, objName, a, rule.Change.Before)
				outside = err == nil
			}
			if err != nil {
				limitations = append(limitations, fmt.Sprintf("%s alternative %d (%s) is not exercised: it cannot hold in the consumer role (%s)", kind, j+1, text, roleText))
				continue
			}
			if outside {
				if err := cf.placeOutside(obj, objName, a); err != nil {
					limitations = append(limitations, fmt.Sprintf("%s alternative %d (%s) is not exercised outside the role: %v", kind, j+1, text, err))
					continue
				}
			}
			if ok, known := holdsAny(other, obj); !known || ok {
				limitations = append(limitations, fmt.Sprintf("%s alternative %d (%s) is not exercised: in the consumer role it also meets the %s condition", kind, j+1, text, otherName))
				continue
			}
			cons := cf.consumersFor(fx, objName, outside)
			cond := text + " (consumer role: " + roleText + ")"
			if outside {
				cond = text + " (outside the consumer role " + roleText + ")"
			}
			arms = append(arms, consumerArm{Arm: Arm{Name: armName, Object: obj, Condition: cond, Predicted: predicted}, consumers: cons})
		}
	}
	build("shielded", overlap, exposure, "exposure")
	build("exposed", exposure, overlap, "mitigating")
	if countOf(arms, domain.ArmConditionPresent) == 0 {
		return nil, limitations, "no alternative of the mitigating condition can be built in the consumer role (" + roleText + "): " + strings.Join(limitations, "; ")
	}
	if countOf(arms, domain.ArmConditionAbsent) == 0 {
		limitations = append(limitations, "no alternative of the exposure condition can be built in the consumer role ("+roleText+"): the rule can be refuted by this exercise but not confirmed")
	}
	return arms, limitations, ""
}

// countOf counts the in-role arms of a base name (outside-role and
// omitted-field arms are counted apart).
func countOf(arms []consumerArm, base string) int {
	n := 0
	for _, a := range arms {
		if isBoundary(a.Name) {
			continue
		}
		if a.Name == base || strings.HasPrefix(a.Name, base+"-") {
			n++
		}
	}
	return n
}

// shielded reports whether an arm name is a shielded (condition-present) arm.
func shielded(name string) bool {
	return name == domain.ArmConditionPresent || strings.HasPrefix(name, domain.ArmConditionPresent+"-")
}

// consumersFor are the consuming objects of an arm whose subject object is
// objName: the role's, or — for an outside-role arm when the catalog
// declares a role-absent observation — that observation's.
func (cf *ConsumerFixture) consumersFor(fx *SubjectFixture, objName string, outside bool) []map[string]any {
	objs := cf.Objects
	if outside && cf.Outside != nil {
		objs = cf.Outside.Objects
	}
	var out []map[string]any
	for _, o := range objs {
		c := substitute(deepCopy(o), objName).(map[string]any)
		if meta, _ := c["metadata"].(map[string]any); meta != nil && meta["namespace"] == nil {
			meta["namespace"] = fx.Namespace
		}
		out = append(out, c)
	}
	return out
}

// placeOutside sets the role-absent observation's subject fields on an
// outside-role arm's object (nothing without one); the arm's condition
// leaves must still hold.
func (cf *ConsumerFixture) placeOutside(obj map[string]any, objName string, leaves []domain.Condition) error {
	if cf.Outside == nil {
		return nil
	}
	for _, f := range cf.Outside.Subject {
		keys, err := catalog.SplitFieldPath(f.Path)
		if err != nil {
			return err
		}
		setPath(obj, keys, substitute(deepCopy(f.Value), objName))
	}
	if ok, known := holds(leaves, obj); !known || !ok {
		return fmt.Errorf("the role-absent observation's subject fields contradict %s", describe(leaves, nil))
	}
	return nil
}

// consumerCheckOf is the runtime check of one arm: the role's consumer, or
// for an outside-role arm the role-absent observation's when declared.
func consumerCheckOf(a consumerArm, cf *ConsumerFixture, fx *SubjectFixture, exercise string) runtime.ConsumerCheck {
	name := a.Object["metadata"].(map[string]any)["name"].(string)
	secret := func(s SecretRef) runtime.SecretKey {
		return runtime.SecretKey{Secret: "core/v1/Secret/" + fx.Namespace + "/" + strings.ReplaceAll(s.Secret, NamePlaceholder, name), Key: s.Key}
	}
	g := fx.Target.Group
	if g == "" {
		g = "core"
	}
	trust, verify, reissue := cf.Trust, cf.Verify, cf.Reissue
	if o := cf.Outside; o != nil && (isOutsideRole(a.Name) || a.outside) {
		trust, verify, reissue = o.Trust, o.Verify, o.Reissue
	}
	c := runtime.ConsumerCheck{ID: a.Name, Exercise: exercise, Trust: secret(trust), Verify: secret(verify)}
	for _, n := range reissue {
		c.Reissue = append(c.Reissue, g+"/"+fx.Target.Version+"/"+fx.Target.Kind+"/"+fx.Namespace+"/"+strings.ReplaceAll(n, NamePlaceholder, name))
	}
	return c
}

// EvaluateConsumerArms decides a consumer observation from a fixture run
// (the outcome rule in the package comment above). A non-discriminating
// exposed arm (discriminate.go) is recorded and never decides. For a scope
// refinement (refinement.go) a pass also needs a role-absent arm observed
// without consumer harm.
func EvaluateConsumerArms(res *runtime.FixtureResult, plans []armPlan, refinement *domain.ScopeRefinement) domain.ProofMethod {
	m := domain.ProofMethod{Method: domain.ProofRuleExercise, Producer: ConsumerExerciseProducer, Level: domain.LevelObserved}
	r := res.Run
	summary := fmt.Sprintf("fixture %s %s → %s in %s (%s, %.0fs), consumer observation", r.Product, r.From, r.To, r.Cluster, r.Status, r.DurationSeconds)
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
	results := map[string]map[string]*runtime.ConsumerResult{}
	for i := range r.Consumers {
		c := &r.Consumers[i]
		if results[c.Check] == nil {
			results[c.Check] = map[string]*runtime.ConsumerResult{}
		}
		results[c.Check][c.Phase] = c
	}
	infra := false
	var shieldedFail, outsideFail, undetermined, quiet, noBaseline, nonDisc, roleAbsentOpen []string
	exposed, roleAbsentKept := 0, 0
	for _, p := range plans {
		from, to := results[p.Name]["from"], results[p.Name]["to"]
		a := domain.ProofArm{Name: p.Name, Objects: p.objects, Condition: p.Condition, Predicted: p.Predicted,
			ReactedFrom: answered(from, p.consumer), ReactedTo: answered(to, p.consumer), NonDiscriminating: p.NonDiscriminating,
			Consumer: &domain.ProofConsumer{Check: p.consumer.ID, Trust: p.consumer.Trust.Secret + "[" + p.consumer.Trust.Key + "]",
				Reissue: p.consumer.Reissue, Verified: p.consumer.Verify.Secret + "[" + p.consumer.Verify.Key + "]",
				From: proofResult(from), To: proofResult(to)}}
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
			if d.Diagnostic() {
				a.Diagnostics = append(a.Diagnostics, d.Object+" "+d.Signature)
				continue
			}
			a.Signatures = append(a.Signatures, "data, not decisive for the consumer: "+d.Object+" "+d.Signature)
		}
		sort.Strings(a.Signatures)
		sort.Strings(a.Diagnostics)
		fo, too := outcomeOf(from), outcomeOf(to)
		if p.Probe != nil {
			// a default probe (probe.go): recorded, never decisive
			a.Probe = p.Probe
			var notFormed []string
			if !isOutsideRole(p.Probe.Base) {
				// in the role: the relationship may not form (formation.go)
				for _, v := range []struct {
					version string
					res     *runtime.ConsumerResult
					out     *string
					rec     *domain.ProofConsumerResult
				}{{"from", from, &fo, a.Consumer.From}, {"to", to, &too, a.Consumer.To}} {
					ev, why := RoleCannotForm(res, p, v.version, v.res)
					if ev == nil {
						if *v.out == runtime.ConsumerUndetermined {
							notFormed = append(notFormed, fmt.Sprintf("%s undetermined, not role-cannot-form: %s", v.version, why))
						}
						continue
					}
					*v.out = domain.ConsumerRoleCannotForm
					v.rec.Outcome, v.rec.Formation = domain.ConsumerRoleCannotForm, ev
					notFormed = append(notFormed, fmt.Sprintf("%s role-cannot-form: %s", v.version, strings.Join(ev, "; ")))
				}
			}
			switch {
			case fo == runtime.ConsumerVerified && too == runtime.ConsumerVerified:
				a.Observed = domain.BehaviourPrevious
			case fo == runtime.ConsumerVerified && too == runtime.ConsumerFailed:
				a.Observed, a.Changed = domain.BehaviourNew, true
			case fo == domain.ConsumerRoleCannotForm && too == domain.ConsumerRoleCannotForm:
				a.Observed = domain.BehaviourRoleCannotForm
			default:
				a.Observed = domain.BehaviourUndetermined
			}
			a.Signatures = append([]string{fmt.Sprintf("consumer: FROM %s, TO %s", fo, too)}, a.Signatures...)
			m.Arms = append(m.Arms, a)
			detail := consumerArmDetail(a, from, to)
			if len(notFormed) > 0 {
				detail += "; " + strings.Join(notFormed, "; ")
			}
			m.Checks = append(m.Checks, domain.ProofCheck{Subject: a.Name, Outcome: domain.MethodInconclusive, Detail: detail + "; a default probe, never decisive"})
			continue
		}
		if isOutsideRole(p.Name) {
			switch {
			case fo == runtime.ConsumerVerified && too == runtime.ConsumerFailed:
				a.Observed, a.Changed = domain.BehaviourNew, true
				outsideFail = append(outsideFail, fmt.Sprintf("%s (FROM %s, TO %s)", p.Name, fo, too))
			case fo == runtime.ConsumerVerified && too == runtime.ConsumerVerified:
				a.Observed = domain.BehaviourPrevious
				if p.Predicted == domain.BehaviourPrevious {
					roleAbsentKept++
				}
			default:
				a.Observed = domain.BehaviourUndetermined
				a.Signatures = append(a.Signatures, "outside the role: no consumer relationship verified at FROM")
				roleAbsentOpen = append(roleAbsentOpen, fmt.Sprintf("%s (FROM %s, TO %s)", p.Name, fo, too))
			}
			a.Signatures = append([]string{fmt.Sprintf("consumer: FROM %s, TO %s", fo, too)}, a.Signatures...)
			m.Arms = append(m.Arms, a)
			m.Checks = append(m.Checks, domain.ProofCheck{Subject: a.Name, Outcome: outsideArmOutcome(fo, too), Detail: consumerArmDetail(a, from, to)})
			continue
		}
		switch {
		case fo == runtime.ConsumerVerified && too == runtime.ConsumerVerified:
			a.Observed = domain.BehaviourPrevious
		case fo == runtime.ConsumerVerified && too == runtime.ConsumerFailed:
			a.Observed, a.Changed = domain.BehaviourNew, true
		default:
			a.Observed = domain.BehaviourUndetermined
		}
		a.Signatures = append([]string{fmt.Sprintf("consumer: FROM %s, TO %s", fo, too)}, a.Signatures...)
		if p.NonDiscriminating != "" {
			// recorded, never decisive (discriminate.go)
			nonDisc = append(nonDisc, p.Name+" ("+p.NonDiscriminating+")")
			m.Arms = append(m.Arms, a)
			m.Checks = append(m.Checks, domain.ProofCheck{Subject: a.Name, Outcome: domain.MethodInconclusive,
				Detail: consumerArmDetail(a, from, to) + "; non-discriminating, not decisive: " + p.NonDiscriminating})
			continue
		}
		if shielded(p.Name) {
			if fo == runtime.ConsumerFailed || too == runtime.ConsumerFailed {
				shieldedFail = append(shieldedFail, fmt.Sprintf("%s (FROM %s, TO %s)", p.Name, fo, too))
			}
		} else {
			exposed++
			switch {
			case fo == runtime.ConsumerFailed:
				noBaseline = append(noBaseline, p.Name)
			case too == runtime.ConsumerVerified:
				quiet = append(quiet, p.Name)
			}
		}
		if fo == runtime.ConsumerUndetermined || too == runtime.ConsumerUndetermined {
			undetermined = append(undetermined, p.Name)
		}
		m.Arms = append(m.Arms, a)
		m.Checks = append(m.Checks, domain.ProofCheck{Subject: a.Name, Outcome: consumerArmOutcome(p.Name, fo, too), Detail: consumerArmDetail(a, from, to)})
	}
	switch {
	case infra:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": the sandbox could not run an arm's objects (image pull)"
	case len(shieldedFail) > 0:
		m.Outcome, m.Detail = domain.MethodFail, summary+": with the mitigating condition the consumer does not verify ("+strings.Join(shieldedFail, "; ")+"): the condition does not shield its consumers"
	case len(outsideFail) > 0:
		m.Outcome, m.Detail = domain.MethodFail, summary+": an object the rule declares shielded outside the consumer role does play it, and its consumer breaks at TO ("+strings.Join(outsideFail, "; ")+"): the rule's shielding (or the default it states) is wrong"
	case len(undetermined) > 0:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": the consumer observation was not made on every arm at both versions ("+strings.Join(undetermined, ", ")+") — a quiet window proves nothing"
	case exposed == 0 && len(nonDisc) > 0:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": every shielded arm's consumer verified, but the only exposed arms cannot discriminate FROM from TO: the change was not shown, so neither is the mitigation"
	case exposed == 0:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": every shielded arm's consumer verified, but no exposed arm could be built: the rule is not refuted, and not confirmed"
	case len(noBaseline) > 0:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": the exposed arm's consumer does not verify even at FROM ("+strings.Join(noBaseline, ", ")+"): there is no baseline"
	case len(quiet) > 0:
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": without the condition the consumer still verifies at TO ("+strings.Join(quiet, ", ")+"): the change was not demonstrated, so neither is the mitigation"
	case refinement != nil && roleAbsentKept == 0:
		why := "no role-absent arm was built"
		if len(roleAbsentOpen) > 0 {
			why = "no consumer relationship was verified on the role-absent arms (" + strings.Join(roleAbsentOpen, "; ") + ")"
		}
		m.Outcome, m.Detail = domain.MethodInconclusive, summary+": in the role the consumer verifies with the condition and breaks without it, but the scope refinement (role "+refinement.Consumer+") is established only by a role-absent arm observed without consumer harm: "+why
	default:
		m.Outcome, m.Detail = domain.MethodPass, summary+": with the condition the consumer verifies at FROM and TO; without it, it verifies at FROM and fails at TO"
		if refinement != nil {
			m.Detail += fmt.Sprintf("; outside the role (%s) the consumer verifies at FROM and TO on %d arm(s): the harm is confined to the role", refinement.Consumer, roleAbsentKept)
		}
	}
	if len(nonDisc) > 0 {
		m.Detail += "; recorded, not decisive (non-discriminating): " + strings.Join(nonDisc, "; ")
	}
	if m.Outcome == domain.MethodPass && len(m.Evidence) == 0 {
		m.Outcome, m.Detail = domain.MethodInconclusive, m.Detail+"; but no observation artifacts were stored, so the result cannot be audited"
	}
	if m.Outcome != domain.MethodPass && m.Outcome != domain.MethodFail {
		m.Evidence = nil
	}
	return m
}

func outcomeOf(r *runtime.ConsumerResult) string {
	if r == nil {
		return runtime.ConsumerUndetermined
	}
	return r.Outcome
}

func answered(r *runtime.ConsumerResult, c *runtime.ConsumerCheck) bool {
	return r != nil && c != nil && len(r.Unanswered) == 0 && len(r.Reissued) == len(c.Reissue)
}

func proofResult(r *runtime.ConsumerResult) *domain.ProofConsumerResult {
	if r == nil {
		return nil
	}
	return &domain.ProofConsumerResult{Outcome: r.Outcome, Detail: r.Detail, TrustCertificates: r.TrustCertificates, TrustKeys: r.TrustKeys,
		Certificate: r.Certificate, CertificateKey: r.CertificateKey}
}

// outsideArmOutcome is the check outcome of an outside-role arm: refuted when
// its consumer verifies at FROM and fails at TO; a pass when it keeps
// verifying; otherwise undetermined (no relationship formed), which never
// decides the method.
func outsideArmOutcome(from, to string) domain.MethodOutcome {
	switch {
	case from == runtime.ConsumerVerified && to == runtime.ConsumerFailed:
		return domain.MethodFail
	case from == runtime.ConsumerVerified && to == runtime.ConsumerVerified:
		return domain.MethodPass
	}
	return domain.MethodInconclusive
}

func consumerArmOutcome(name, from, to string) domain.MethodOutcome {
	switch {
	case shielded(name) && (from == runtime.ConsumerFailed || to == runtime.ConsumerFailed):
		return domain.MethodFail
	case from == runtime.ConsumerUndetermined || to == runtime.ConsumerUndetermined:
		return domain.MethodInconclusive
	case shielded(name) || (from == runtime.ConsumerVerified && to == runtime.ConsumerFailed):
		return domain.MethodPass
	}
	return domain.MethodInconclusive
}

func consumerArmDetail(a domain.ProofArm, from, to *runtime.ConsumerResult) string {
	fo, too := outcomeOf(from), outcomeOf(to)
	if c := a.Consumer; c != nil && c.From != nil && c.To != nil {
		fo, too = c.From.Outcome, c.To.Outcome
	}
	d := fmt.Sprintf("%s (%s): predicted %s, observed %s; consumer at FROM %s, at TO %s",
		a.Name, a.Condition, a.Predicted, a.Observed, fo, too)
	for _, x := range []*runtime.ConsumerResult{from, to} {
		if x != nil && x.Detail != "" {
			d += "; " + x.Phase + ": " + x.Detail
		}
	}
	return d
}
