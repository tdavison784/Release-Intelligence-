package verify

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/applicability"
	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Boundary fixtures (mission E2, PLAN-M4 lane applicability change 3). The
// arms BuildArms and BuildConsumerArms synthesise state every field the rule
// compares by value explicitly; real environments mostly omit such fields. For
// each arm and each field the rule compares by value (equals / not-equals)
// that the arm states, an omitted-field arm is the same object without that
// field. Its prediction is the rule itself, evaluated three-valued on the
// object (internal/applicability): the field holds the rule's default when
// the rule states one whose citations resolve to the rule's upstream
// evidence, and is undecidable otherwise. A rule that cannot predict an
// object that omits a field it compares — a rule that only works when every
// field is explicit — fails its fixtures before any cluster runs; a rule
// that can is exercised on the omitted-field objects as well, so the
// controller's real behaviour checks the default the rule claims.
//
// A synthetic object is checked against the rule's own default only: the
// fixture does not consult the CRD schema, so a rule that relies on a schema
// default states it (Condition.Default, source crd-schema, citing the CRD).

// maxBoundaryArms bounds the omitted-field arms one exercise adds.
const maxBoundaryArms = 6

// Arm name parts of boundary and outside-role arms (the prefix stays
// domain.ArmConditionPresent / ArmConditionAbsent, whose role they share).
const (
	omittedSuffix     = "-omitted"
	outsideRoleSuffix = "-outside-role"
)

// valueFields are the subject fields the rule compares by value (equals /
// not-equals leaves of exposure and overlap), sorted.
func valueFields(rule domain.SemanticAssertion) []string {
	seen := map[string]bool{}
	var walk func(domain.Condition)
	walk = func(c domain.Condition) {
		if c.Op == domain.OpField && (c.State == domain.StateEquals || c.State == domain.StateNotEquals) && !strings.Contains(c.Path, "[]") {
			seen[c.Path] = true
		}
		for _, x := range c.Of {
			walk(x)
		}
	}
	if ap := rule.Applicability; ap != nil {
		walk(ap.Exposure)
		if ap.Overlap != nil {
			walk(*ap.Overlap)
		}
	}
	var out []string
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// truth3 is a three-valued truth value of a condition on one object.
type truth3 int8

const (
	t3False truth3 = iota
	t3True
	t3Unknown
)

// eval3 evaluates a condition on the subject kind against one synthetic
// object: resource (of the subject kind), all, any, not and field leaves;
// a value predicate on an absent field holds the rule's evidenced default
// (applicability.Unset), and is unknown without one.
func eval3(c domain.Condition, t FixtureType, obj map[string]any, pool map[domain.EvidenceID]domain.Evidence) (truth3, string) {
	switch c.Op {
	case domain.OpResource:
		if c.Group != t.Group || c.Kind != t.Kind || c.Name != "" {
			return t3Unknown, fmt.Sprintf("a %s/%s scope is not decidable on the subject object", c.Group, c.Kind)
		}
		return eval3(domain.Condition{Op: domain.OpAll, Of: c.Of}, t, obj, pool)
	case domain.OpAll, domain.OpAny:
		unknown := ""
		for _, x := range c.Of {
			r, why := eval3(x, t, obj, pool)
			switch {
			case r == t3Unknown:
				if unknown == "" {
					unknown = why
				}
			case (r == t3False) == (c.Op == domain.OpAll):
				return r, why
			}
		}
		if unknown != "" {
			return t3Unknown, unknown
		}
		if c.Op == domain.OpAll {
			return t3True, ""
		}
		return t3False, ""
	case domain.OpNot:
		r, why := eval3(c.Of[0], t, obj, pool)
		switch r {
		case t3True:
			return t3False, why
		case t3False:
			return t3True, why
		}
		return t3Unknown, why
	case domain.OpField:
		if strings.Contains(c.Path, "[]") {
			return t3Unknown, "the list path " + c.Path + " is not decidable on a synthetic object"
		}
		keys, err := catalog.SplitFieldPath(c.Path)
		if err != nil {
			return t3Unknown, err.Error()
		}
		v, present := getPath(obj, keys)
		switch c.State {
		case domain.StateSet:
			return bool3(present), ""
		case domain.StateUnset:
			return bool3(!present), ""
		}
		if !applicability.IsValuePredicate(c.State) {
			return t3Unknown, fmt.Sprintf("field state %s is not decidable on a synthetic object", c.State)
		}
		if present {
			hit := literalHolds(c, encode(v))
			if c.State == domain.StateNotEquals {
				hit = !hit
			}
			return bool3(hit), ""
		}
		d := applicability.Resolve(applicability.Default{Why: "the fixture consults no CRD schema"}, applicability.RuleDefault(c, pool))
		u := applicability.Unset(c, d, func(value string) bool { return literalHolds(c, value) })
		if !u.Known {
			return t3Unknown, u.Why
		}
		return bool3(u.Holds), ""
	}
	return t3Unknown, fmt.Sprintf("a %s condition is not decidable on the subject object", c.Op)
}

func bool3(b bool) truth3 {
	if b {
		return t3True
	}
	return t3False
}

func encode(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// literalHolds is the positive form of a value predicate over one
// JSON-encoded value (for not-equals: equals).
func literalHolds(c domain.Condition, value string) bool {
	switch c.State {
	case domain.StateEquals, domain.StateNotEquals:
		for _, v := range c.Values {
			if applicability.SameValue(v, value) {
				return true
			}
		}
	case domain.StateMatches:
		re, err := regexp.Compile(c.Pattern)
		var s string
		if json.Unmarshal([]byte(value), &s) != nil {
			s = value
		}
		return err == nil && re.MatchString(s)
	case domain.StateHasToken, domain.StateHasTokenKey:
		var s string
		if json.Unmarshal([]byte(value), &s) != nil {
			return false
		}
		sep := c.Separator
		if sep == "" {
			sep = ","
		}
		for _, tok := range strings.Split(s, sep) {
			key := strings.TrimSpace(tok)
			if c.State == domain.StateHasTokenKey {
				key, _, _ = strings.Cut(key, "=")
				key = strings.TrimSpace(key)
			}
			for _, v := range c.Values {
				if key == v {
					return true
				}
			}
		}
	}
	return false
}

// predict is the behaviour the rule predicts for one object: new when its
// exposure holds, previous when it does not; why is set when the rule cannot
// decide its exposure for the object.
func predict(rule domain.SemanticAssertion, t FixtureType, obj map[string]any, pool map[domain.EvidenceID]domain.Evidence) (string, string) {
	r, why := eval3(rule.Applicability.Exposure, t, obj, pool)
	switch r {
	case t3True:
		return domain.BehaviourNew, ""
	case t3False:
		return domain.BehaviourPrevious, ""
	}
	return "", why
}

// boundaryArm is an omitted-field arm: whether omitting the field takes the
// object out of the consumer role.
type boundaryArm struct {
	consumerArm
	outsideRole bool
}

// BoundaryArms builds the omitted-field arms of a rule from its arms (see
// the comment above). rolePaths are the consumer role's fields (nil without a
// consumer observation): omitting one takes the object out of the role, so a
// shielded prediction becomes an outside-role arm and an exposed one cannot be
// observed by the consumer (a limitation). why is set when the rule cannot
// predict an omitted-field object: the exercise then fails.
func BoundaryArms(rule domain.SemanticAssertion, fx *SubjectFixture, arms []consumerArm, rolePaths map[string]bool,
	pool map[domain.EvidenceID]domain.Evidence) (out []boundaryArm, limitations []string, why string) {
	fields := valueFields(rule)
	// every arm already built must be predicted by the rule itself, as its
	// construction says (an alternative the overlap shields but whose
	// exposure the rule cannot decide is the same explicit-only rule); an
	// omitted-field object identical (up to its name) to one of them is
	// exercised by it
	seen := map[string]bool{}
	for _, a := range arms {
		predicted, undecided := predict(rule, fx.Target, a.Object, pool)
		switch {
		case undecided != "":
			return nil, nil, fmt.Sprintf("the rule cannot predict arm %s (%s): %s — a rule that only works when every field it compares is explicit is not generally applicable (state the field's default with evidence)",
				a.Name, a.Condition, undecided)
		case predicted != a.Predicted:
			return nil, nil, fmt.Sprintf("arm %s (%s) is built as %s but the rule's exposure predicts %s: the rule's exposure and mitigating conditions disagree",
				a.Name, a.Condition, a.Predicted, predicted)
		}
		seen[canonical(a.Object)] = true
	}
	n := 0
	for _, a := range arms {
		for _, f := range fields {
			keys, err := catalog.SplitFieldPath(f)
			if err != nil {
				continue
			}
			if _, present := getPath(a.Object, keys); !present {
				continue // the arm already omits it
			}
			obj := deepCopy(a.Object).(map[string]any)
			omitPath(obj, keys)
			if seen[canonical(obj)] {
				continue
			}
			seen[canonical(obj)] = true
			predicted, undecided := predict(rule, fx.Target, obj, pool)
			if undecided != "" {
				return nil, nil, fmt.Sprintf("the rule cannot predict an object that omits %s (arm %s without it): %s — a rule that only works when every field it compares is explicit is not generally applicable (state the field's default with evidence, or what an unset %s means)",
					f, a.Name, undecided, f)
			}
			outside := rolePaths[f]
			if outside && predicted == domain.BehaviourNew {
				limitations = append(limitations, fmt.Sprintf("arm %s without %s is predicted exposed but outside the consumer role: the consumer observation cannot test it", a.Name, f))
				continue
			}
			if n >= maxBoundaryArms {
				limitations = append(limitations, fmt.Sprintf("arm %s without %s is not exercised: at most %d omitted-field arms run", a.Name, f, maxBoundaryArms))
				continue
			}
			n++
			base, suffix := domain.ArmConditionPresent, omittedSuffix
			if predicted == domain.BehaviourNew {
				base = domain.ArmConditionAbsent
			}
			if outside {
				suffix += outsideRoleSuffix
			}
			name := fmt.Sprintf("%s%s-%d", base, suffix, n)
			objName := fmt.Sprintf("ri-arm-omit-%d", n)
			renamed := renameArm(obj, a.Object, objName)
			b := boundaryArm{outsideRole: outside}
			b.Arm = Arm{Name: name, Object: renamed, Predicted: predicted,
				Condition: fmt.Sprintf("%s without %s (omitted; the rule predicts %s)", a.Condition, f, predicted)}
			for _, c := range a.consumers {
				b.consumers = append(b.consumers, renameArm(deepCopy(c).(map[string]any), a.Object, objName))
			}
			out = append(out, b)
		}
	}
	return out, limitations, ""
}

// omitPath removes the field at keys and every parent mapping the removal
// leaves empty (an omitted field, as an environment would write it).
func omitPath(obj map[string]any, keys []string) {
	unsetPath(obj, keys)
	for i := len(keys) - 1; i > 0; i-- {
		v, ok := getPath(obj, keys[:i])
		if m, isMap := v.(map[string]any); !ok || !isMap || len(m) > 0 {
			return
		}
		unsetPath(obj, keys[:i])
	}
}

// canonical is an arm object's content with its own name abstracted.
func canonical(obj map[string]any) string {
	return encode(renameArm(deepCopy(obj).(map[string]any), obj, "{{arm}}"))
}

// renameArm replaces the arm object's name in obj (its own metadata.name and
// every string that embeds it, as NamePlaceholder did).
func renameArm(obj, from map[string]any, to string) map[string]any {
	meta, _ := from["metadata"].(map[string]any)
	old, _ := meta["name"].(string)
	if old == "" {
		return obj
	}
	var walk func(v any) any
	walk = func(v any) any {
		switch x := v.(type) {
		case map[string]any:
			for k, y := range x {
				x[k] = walk(y)
			}
			return x
		case []any:
			for i, y := range x {
				x[i] = walk(y)
			}
			return x
		case string:
			return strings.ReplaceAll(x, old, to)
		}
		return v
	}
	return walk(obj).(map[string]any)
}

// isOutsideRole reports whether an arm name is an outside-role arm.
func isOutsideRole(name string) bool { return strings.Contains(name, outsideRoleSuffix) }

// isBoundary reports whether an arm is an extra arm (omitted-field or
// outside-role) beside the two principal arms.
func isBoundary(name string) bool {
	return strings.Contains(name, omittedSuffix) || strings.Contains(name, outsideRoleSuffix)
}

// rulePool indexes the rule's upstream evidence (the citations its
// condition defaults must resolve to).
func rulePool(evs []domain.Evidence) map[domain.EvidenceID]domain.Evidence {
	out := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range evs {
		out[e.ID] = e
	}
	return out
}
