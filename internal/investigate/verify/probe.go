package verify

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/applicability"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
)

// Default probes and observed defaults (PD-5, lane verify3, D-0026). A scope
// refinement compares the catalog role's fields by value, and real objects
// often leave such a field unset. Without an evidenced default those objects
// stay undecidable (boundary.go). PD-5 lets the verifier establish the
// default by observation on the real controller instead:
//
//   - For a role field F the rule cannot predict when it is omitted, each of
//     up to two base arms that set F explicitly (one exposed arm in the role,
//     one role-absent arm, when built) gets two probes in the base arm's own
//     observation: the object without F ("-unset"), and the object with F
//     set to the role's other value ("-other").
//   - Probes are run and recorded beside the other arms and never decide the
//     method.
//   - On a confirmed proof, F's default is recorded as observed
//     (domain.ObservedDefault) with value V when the unset probe observed the
//     same consumer outcome, at FROM and at TO, as the arm with F = V, and
//     the arm with the other value observed a different one. When the two
//     values behave alike, the observation does not distinguish them and
//     nothing is recorded. When two observations disagree, nothing is
//     recorded either (a contradiction).
//
// The distinguishing condition is this lane's reading of PD-5's "the
// omitted-field arm shows an unset field behaving as a given value": an
// observation in which the explicit values behave alike shows nothing about
// the default, and recording one from it would let the evaluator decide
// objects it cannot decide.

// maxProbeBases bounds the base arms probed per field.
const maxProbeBases = 2

// Probe arm name parts ("default-probe-<n>-unset" / "-other").
const (
	probePrefix      = "default-probe-"
	probeUnsetSuffix = "-unset"
	probeOtherSuffix = "-other"
)

// DefaultProbes builds the default probes of a scope refinement's role fields
// that the rule cannot predict for an object that omits them (see the comment
// above). arms are the exercise's arms (principal and outside-role); pool is
// the rule's upstream evidence.
func DefaultProbes(rule domain.SemanticAssertion, fx *SubjectFixture, arms []consumerArm, cf *ConsumerFixture,
	pool map[domain.EvidenceID]domain.Evidence) (out []consumerArm, limitations []string) {
	if cf == nil {
		return nil, nil
	}
	values := map[string][]any{} // role field → the values the catalog gives it
	addValue := func(f FieldValue) {
		for _, v := range values[f.Path] {
			if encode(v) == encode(f.Value) {
				return
			}
		}
		values[f.Path] = append(values[f.Path], f.Value)
	}
	for _, f := range cf.Role {
		addValue(f)
	}
	if cf.Outside != nil {
		for _, f := range cf.Outside.Role {
			addValue(f)
		}
	}
	compared := map[string]bool{}
	for _, f := range valueFields(rule) {
		compared[f] = true
	}
	var fields []string
	for f := range values {
		if compared[f] {
			fields = append(fields, f)
		}
	}
	sort.Strings(fields)
	n := 0
	for _, f := range fields {
		keys, err := catalog.SplitFieldPath(f)
		if err != nil {
			continue
		}
		var inRole, outside []consumerArm
		for _, a := range arms {
			if _, present := getPath(a.Object, keys); !present || isBoundary(a.Name) && !isOutsideRole(a.Name) {
				continue
			}
			obj := deepCopy(a.Object).(map[string]any)
			omitPath(obj, keys)
			if _, undecided := predict(rule, fx.Target, obj, pool); undecided == "" {
				continue // the rule states what an unset field means: nothing to observe
			}
			switch {
			case isOutsideRole(a.Name):
				outside = append(outside, a)
			case a.Predicted == domain.BehaviourNew:
				inRole = append([]consumerArm{a}, inRole...) // an exposed arm first
			default:
				inRole = append(inRole, a)
			}
		}
		var bases []consumerArm
		if len(inRole) > 0 {
			bases = append(bases, inRole[0])
		}
		if len(outside) > 0 {
			bases = append(bases, outside[0])
		}
		if len(bases) > maxProbeBases {
			bases = bases[:maxProbeBases]
		}
		for _, b := range bases {
			v, _ := getPath(b.Object, keys)
			var other any
			found := false
			for _, x := range values[f] {
				if encode(x) != encode(v) {
					other, found = x, true
					break
				}
			}
			if !found {
				limitations = append(limitations, fmt.Sprintf("the default of %s is not probed on arm %s: the catalog gives it no other value to contrast with", f, b.Name))
				continue
			}
			n++
			base := domain.ArmProbe{Field: f, Base: b.Name, BaseValue: encode(v)}
			unset := deepCopy(b.Object).(map[string]any)
			omitPath(unset, keys)
			set := deepCopy(b.Object).(map[string]any)
			setPath(set, keys, deepCopy(other))
			for _, p := range []struct {
				suffix string
				obj    map[string]any
				probe  domain.ArmProbe
				text   string
			}{
				{probeUnsetSuffix, unset, withOmitted(base), "without " + f},
				{probeOtherSuffix, set, withValue(base, encode(other)), fmt.Sprintf("with %s = %s", f, encode(other))},
			} {
				name := fmt.Sprintf("%s%d%s", probePrefix, n, p.suffix)
				objName := fmt.Sprintf("ri-arm-probe-%d%s", n, p.suffix)
				pr := p.probe
				a := consumerArm{Arm: Arm{Name: name, Object: renameArm(p.obj, b.Object, objName), Predicted: domain.BehaviourUndetermined, Probe: &pr,
					Condition: fmt.Sprintf("default probe of %s: arm %s (%s) %s, in the same observation; never decisive", f, b.Name, b.Condition, p.text)},
					outside: isOutsideRole(b.Name)}
				for _, c := range b.consumers {
					a.consumers = append(a.consumers, renameArm(deepCopy(c).(map[string]any), b.Object, objName))
				}
				out = append(out, a)
			}
		}
	}
	return out, limitations
}

func withOmitted(p domain.ArmProbe) domain.ArmProbe {
	p.Omitted = true
	return p
}

func withValue(p domain.ArmProbe, v string) domain.ArmProbe {
	p.Value = v
	return p
}

// isProbe reports whether an arm name is a default probe's.
func isProbe(name string) bool { return strings.HasPrefix(name, probePrefix) }

// consumerKey is what an arm's consumer observed at FROM and at TO ("" when
// either is undetermined).
func consumerKey(a domain.ProofArm) string {
	if a.Consumer == nil || a.Consumer.From == nil || a.Consumer.To == nil {
		return ""
	}
	from, to := a.Consumer.From.Outcome, a.Consumer.To.Outcome
	decided := func(o string) bool { return o == runtime.ConsumerVerified || o == runtime.ConsumerFailed }
	if !decided(from) || !decided(to) {
		return ""
	}
	return "FROM " + from + ", TO " + to
}

// ObservationFixture is what an observed default records about the run's
// controller configuration.
type ObservationFixture struct {
	Bounds   *domain.ProofBounds
	Fixtures *FixtureSet
}

// ObservedDefaults derives the observed defaults of a rule exercise's default
// probes (see the comment above). Call it only for a confirmed proof; notes
// say why a probed field got none.
func ObservedDefaults(rule domain.SemanticAssertion, ex domain.ProofMethod, fx ObservationFixture) (out []domain.ObservedDefault, notes []string) {
	if ex.Method != domain.ProofRuleExercise || rule.Subject == nil || fx.Bounds == nil || fx.Fixtures == nil {
		return nil, nil
	}
	byName := map[string]domain.ProofArm{}
	type group struct {
		base         string
		unset, other []domain.ProofArm
	}
	groups := map[string]map[string]*group{} // field → base → probes
	var fields []string
	for _, a := range ex.Arms {
		byName[a.Name] = a
		p := a.Probe
		if p == nil {
			continue
		}
		if groups[p.Field] == nil {
			groups[p.Field] = map[string]*group{}
			fields = append(fields, p.Field)
		}
		g := groups[p.Field][p.Base]
		if g == nil {
			g = &group{base: p.Base}
			groups[p.Field][p.Base] = g
		}
		if p.Omitted {
			g.unset = append(g.unset, a)
		} else {
			g.other = append(g.other, a)
		}
	}
	sort.Strings(fields)
	values, _ := json.Marshal(fx.Fixtures.Controller.Values)
	if fx.Fixtures.Controller.Values == nil {
		values = []byte("{}")
	}
	for _, f := range fields {
		var value string
		var arms, seen []string
		conflict := false
		var bases []string
		for b := range groups[f] {
			bases = append(bases, b)
		}
		sort.Strings(bases)
		for _, b := range bases {
			g := groups[f][b]
			base, ok := byName[b]
			if !ok || len(g.unset) != 1 || len(g.other) != 1 {
				notes = append(notes, fmt.Sprintf("default probe of %s on arm %s is incomplete", f, b))
				continue
			}
			u, o := g.unset[0], g.other[0]
			kb, ku, ko := consumerKey(base), consumerKey(u), consumerKey(o)
			var v string
			switch {
			case kb == "" || ku == "" || ko == "":
				notes = append(notes, fmt.Sprintf("default probe of %s on arm %s: not every arm was observed at both versions (%s: %s; %s: %s; %s: %s)", f, b, b, orNone(kb), u.Name, orNone(ku), o.Name, orNone(ko)))
				continue
			case kb == ko:
				notes = append(notes, fmt.Sprintf("default probe of %s on arm %s: the observation does not distinguish %s = %s from %s = %s (both %s), so it shows nothing about an unset %s", f, b, f, u.Probe.BaseValue, f, o.Probe.Value, kb, f))
				continue
			case ku == kb:
				v = u.Probe.BaseValue
			case ku == ko:
				v = o.Probe.Value
			default:
				notes = append(notes, fmt.Sprintf("default probe of %s on arm %s: an unset %s behaved like neither value (%s; %s = %s: %s; %s = %s: %s)", f, b, f, ku, f, u.Probe.BaseValue, kb, f, o.Probe.Value, ko))
				continue
			}
			if value != "" && !applicability.SameValue(value, v) {
				conflict = true
			}
			if value == "" {
				value = v
			}
			arms = append(arms, b, u.Name, o.Name)
			seen = append(seen, fmt.Sprintf("in the observation of arm %s an unset %s behaved as %s = %s (%s: %s), not as the other value (%s: %s)", b, f, f, v, u.Name, ku, o.Name, ko))
		}
		switch {
		case conflict:
			notes = append(notes, fmt.Sprintf("the default probes of %s contradict each other (%s): no observed default is recorded", f, strings.Join(seen, "; ")))
		case value != "":
			out = append(out, domain.ObservedDefault{Group: rule.Subject.Group, Kind: rule.Subject.Kind, Path: f, Value: value, Level: domain.LevelObserved,
				Arms: arms, Observation: strings.Join(seen, "; "), Versions: []string{fx.Bounds.From, fx.Bounds.To}, ControllerValues: string(values),
				Fixture: fx.Fixtures.Digest(), Evidence: append([]domain.Evidence(nil), ex.Evidence...), Limitation: domain.ObservedDefaultLimitation,
				RuleDigest: rule.Digest()})
		}
	}
	return out, notes
}

func orNone(k string) string {
	if k == "" {
		return "undetermined"
	}
	return k
}
