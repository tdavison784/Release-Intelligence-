// Package applicability holds the one rule every applicability evaluator uses
// for a value predicate on a field an object leaves unset (mission E1, PLAN-M4
// lane applicability): the impact join (internal/impact) and the pin-rule
// scope (internal/investigate/validate) both decide such a leaf here, so they
// cannot disagree.
//
// An unset field holds whatever its default is. A default is established by
//
//   - the CRD schema: every CustomResourceDefinition of the input that serves
//     the object's kind and version describes the path and agrees on its
//     `default` (the API server fills it in); or
//   - the rule itself: a Condition.Default whose citations resolve to upstream
//     evidence in the pool the caller supplies (a controller default set in
//     code, or a schema default the rule cites).
//
// Anything else is unknown, with the reason and what would settle it: never
// false, never true. A schema default and a rule default that disagree are
// unknown too. The package is plain Go with no product knowledge.
package applicability

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// State says how far a default is established.
type State int8

const (
	// Unknown: nothing establishes what an unset field means.
	Unknown State = iota
	// None: the schema describes the field and states no default (a
	// controller may still default it in code).
	None
	// Known: the default is established (Value).
	Known
)

// Default is what an object that leaves a field unset holds.
type Default struct {
	State State
	// Value is the JSON-encoded default (Known).
	Value string
	// Source names what established it (or, for None, the schemas read).
	Source string
	// Why explains an Unknown.
	Why string
	// Citations are the rule-default evidence ids the value rests on.
	Citations []domain.EvidenceID
	// CRDs are the indexes (into the caller's CRD slice) of the schemas a
	// schema default was read from.
	CRDs []int
	// Contradiction marks an Unknown caused by two established defaults
	// that disagree: nothing may settle the field afterwards.
	Contradiction bool
	// Membership is the consumer role whose observed membership default
	// decided the leaf (PD-6, WithMembership): Value is then the role
	// field's outside value, standing for "outside the role", not a default
	// of the field.
	Membership string
}

// CRD is one CustomResourceDefinition: what it serves and its structural
// schemas per version.
type CRD struct {
	Group, Kind string
	// Schemas maps a served version to its openAPIV3Schema.
	Schemas map[string]map[string]any
	// Where names the CRD and its source in explanations.
	Where string
}

// SchemaDefault is the default the API server applies to path of an object of
// group/version/kind, read from every CRD of crds that serves the kind. Every
// such CRD must describe the path and agree; an ancestor default, a schema
// that does not describe the path, a version no CRD serves, or no CRD at all
// leave it Unknown.
func SchemaDefault(crds []CRD, group, version, kind, path string) Default {
	var srcs []string
	var used []int
	values := map[string]bool{}
	none := false
	for i, c := range crds {
		if c.Group != group || c.Kind != kind {
			continue
		}
		schema := c.Schemas[version]
		if schema == nil {
			return Default{Why: fmt.Sprintf("%s has no structural schema for version %s", c.Where, version)}
		}
		d, has, why := DefaultAt(schema, path)
		if why != "" {
			return Default{Why: c.Where + ": " + why}
		}
		srcs = append(srcs, c.Where)
		used = append(used, i)
		if has {
			values[d] = true
		} else {
			none = true
		}
	}
	src := strings.Join(uniq(srcs), "; ")
	switch {
	case len(srcs) == 0:
		g := group
		if g == "" {
			g = "(core)"
		}
		return Default{Why: fmt.Sprintf("no CustomResourceDefinition of %s/%s in the input states whether %s has a default", g, kind, path)}
	case len(values) > 1 || (len(values) == 1 && none):
		return Default{Why: fmt.Sprintf("the CRDs of the input disagree on the default of %s (%s)", path, src)}
	case none:
		return Default{State: None, Source: src, CRDs: used,
			Why: fmt.Sprintf("%s states no schema default for %s; a controller may default it in code", src, path)}
	}
	for v := range values {
		return Default{State: Known, Value: v, Source: src, CRDs: used}
	}
	return Default{}
}

// DefaultAt walks a structural schema along path ("[]" marks list elements)
// and returns the default at its end. why is set when the schema cannot
// decide it.
func DefaultAt(schema map[string]any, path string) (value string, has bool, why string) {
	node := schema
	for i, s := range env.SplitKeyPath(path) {
		name, list := strings.CutSuffix(s, "[]")
		if i > 0 {
			if _, ok := node["default"]; ok {
				return "", false, "an ancestor of " + path + " has a default of its own"
			}
		}
		if node["x-kubernetes-preserve-unknown-fields"] == true {
			return "", false, "the schema does not describe " + path + " (preserves unknown fields)"
		}
		next := asMap(asMap(node["properties"])[name])
		if next == nil {
			return "", false, "the schema does not describe " + path
		}
		node = next
		if list {
			if _, ok := node["default"]; ok {
				return "", false, "an ancestor of " + path + " has a default of its own"
			}
			node = asMap(node["items"])
			if node == nil {
				return "", false, "the schema does not describe the elements of " + name
			}
		}
	}
	d, ok := node["default"]
	if !ok {
		return "", false, ""
	}
	return EncodeJSON(d), true, ""
}

// CheckRuleDefault reports why a condition default is not evidenced by pool
// ("" when every citation resolves to upstream evidence in it): the check
// fact validation (domain.FieldDefault.Unevidenced) and the evaluators share.
func CheckRuleDefault(d domain.FieldDefault, pool map[domain.EvidenceID]domain.Evidence) string {
	return d.Unevidenced(pool)
}

// RuleDefault is the default c states for its field, when its citations
// resolve in pool; otherwise Unknown, saying why it was ignored.
func RuleDefault(c domain.Condition, pool map[domain.EvidenceID]domain.Evidence) Default {
	if c.Default == nil {
		return Default{Why: "the rule states no default for " + subjectOf(c)}
	}
	if why := CheckRuleDefault(*c.Default, pool); why != "" {
		return Default{Why: fmt.Sprintf("the rule's default %s for %s is not evidenced (%s), so it is ignored", plain(c.Default.Value), subjectOf(c), why)}
	}
	return Default{State: Known, Value: c.Default.Value, Citations: append([]domain.EvidenceID(nil), c.Default.Citations...),
		Source: fmt.Sprintf("the rule's %s default, citing %s", c.Default.Source, joinIDs(c.Default.Citations))}
}

// Resolve combines the schema default and the rule default of one field. A
// rule default decides a field the schema leaves without a default (the
// controller defaults it in code); two established defaults that disagree
// decide nothing.
func Resolve(schema, rule Default) Default {
	switch {
	case schema.State == Known && rule.State == Known:
		if !SameValue(schema.Value, rule.Value) {
			return Default{Why: fmt.Sprintf("the schema default %s (%s) contradicts the rule's default %s", plain(schema.Value), schema.Source, plain(rule.Value)), Contradiction: true}
		}
		out := schema
		out.Citations = rule.Citations
		out.Source = schema.Source + "; " + rule.Source
		return out
	case schema.State == Known:
		return schema
	case rule.State == Known:
		out := rule
		out.CRDs = schema.CRDs
		return out
	}
	why := schema.Why
	if rule.Why != "" {
		if why != "" {
			why += "; "
		}
		why += rule.Why
	}
	return Default{Why: why, CRDs: schema.CRDs}
}

// ObservedFor is the observed default of path on group/kind among ds (the
// fact's usable observed defaults, PD-5), nil when none. Two that disagree
// are a contradiction: nil, with why.
func ObservedFor(ds []domain.ObservedDefault, group, kind, path string) (d *domain.ObservedDefault, why string) {
	for i := range ds {
		x := &ds[i]
		if x.Group != group || x.Kind != kind || x.Path != path {
			continue
		}
		if d != nil && !SameValue(d.Value, x.Value) {
			return nil, fmt.Sprintf("the observed defaults of %s disagree (%s by proof %s, %s by proof %s): both are blocked", path, plain(d.Value), d.Proof, plain(x.Value), x.Proof)
		}
		d = x
	}
	return d, ""
}

// WithObserved combines an established default (schema and rule, Resolve)
// with a default observed on the real controller (PD-5: a confirmed proof's
// omitted-field arm, carried into the fact on human promotion). The
// observed default decides a field nothing else establishes; one that
// agrees with an established default is noted beside it; one that
// contradicts an upstream-cited or schema default blocks both — the field
// is unknown, the contradiction recorded, never silently resolved.
// conflict is a disagreement among the observed defaults themselves
// (ObservedFor's why): the field is unknown.
func WithObserved(d Default, obs *domain.ObservedDefault, conflict string) Default {
	if conflict != "" {
		return Default{Why: "contradiction: " + conflict, CRDs: d.CRDs, Contradiction: true}
	}
	if obs == nil {
		return d
	}
	src := fmt.Sprintf("observed by confirmed proof %s at controller %s (%s)", obs.Proof, strings.Join(obs.Versions, ", "), obs.Limitation)
	switch {
	case d.Contradiction:
		return d
	case d.State == Known && !SameValue(d.Value, obs.Value):
		return Default{Why: fmt.Sprintf("contradiction: the default %s %s contradicts the default %s %s: both are blocked", plain(obs.Value), src, plain(d.Value), "("+d.Source+")"), CRDs: d.CRDs, Contradiction: true}
	case d.State == Known:
		d.Source += "; agreeing with the default " + src
		return d
	}
	return Default{State: Known, Value: obs.Value, Source: "the default " + src, CRDs: d.CRDs}
}

// MembershipFor is the role-membership default (PD-6) that decides leaf c of
// an object of group/kind, nil when none does. A membership default decides
// only a membership leaf of its role: a field leaf on its path that equals
// the role's value or the outside value. Any other condition on the same
// field (another state, another value) gets none: the default says which
// objects are outside the role, not what the field holds. Two that disagree
// on the same leaf are a contradiction: nil, with why.
func MembershipFor(ds []domain.RoleMembershipDefault, group, kind string, c domain.Condition) (d *domain.RoleMembershipDefault, why string) {
	if c.Op != domain.OpField || c.State != domain.StateEquals || len(c.Values) != 1 {
		return nil, ""
	}
	for i := range ds {
		x := &ds[i]
		if x.Group != group || x.Kind != kind || x.Path != c.Path ||
			!(SameValue(c.Values[0], x.RoleValue) || SameValue(c.Values[0], x.OutsideValue)) {
			continue
		}
		if d != nil && (!SameValue(d.RoleValue, x.RoleValue) || !SameValue(d.OutsideValue, x.OutsideValue)) {
			return nil, fmt.Sprintf("the role-membership defaults of %s disagree (%s by proof %s, %s by proof %s): both are blocked", c.Path, d.Statement(), d.Proof, x.Statement(), x.Proof)
		}
		d = x
	}
	return d, ""
}

// WithMembership combines a field's established default (WithObserved's
// result) with the role-membership default MembershipFor found for one
// membership leaf. The membership default decides the leaf only when
// nothing else establishes the field: an unset field is outside the role,
// so the role leaf is false and the outside leaf true. An established
// default that puts an unset field in the role (the role's value)
// contradicts it and blocks both; one that agrees decides the leaf by its
// value, as before. A contradiction already found stays.
func WithMembership(d Default, m *domain.RoleMembershipDefault, conflict string) Default {
	if conflict != "" {
		return Default{Why: "contradiction: " + conflict, CRDs: d.CRDs, Contradiction: true}
	}
	if m == nil || d.Contradiction {
		return d
	}
	src := fmt.Sprintf("role-membership default %s, by confirmed proof %s at controller %s (%s)", m.Statement(), m.Proof, strings.Join(m.Versions, ", "), m.Limitation)
	switch {
	case d.State == Known && SameValue(d.Value, m.RoleValue):
		return Default{Why: fmt.Sprintf("contradiction: the default %s (%s) puts an unset %s in role %s, the %s says it is outside: both are blocked", plain(d.Value), d.Source, m.Path, m.Role, src), CRDs: d.CRDs, Contradiction: true}
	case d.State == Known:
		d.Source += "; agreeing with the " + src
		return d
	}
	return Default{State: Known, Value: m.OutsideValue, Source: "the " + src, CRDs: d.CRDs, Membership: m.Role}
}

// Needed is what would settle an unset field's value predicate.
func Needed(c domain.Condition) string {
	return fmt.Sprintf("what an unset %s means: a CRD schema default, or a rule default (condition default) citing upstream evidence of the controller's default", subjectOf(c))
}

// Verdict is the value of a value predicate on an unset field.
type Verdict struct {
	// Known is false when nothing establishes the default (Holds is then
	// meaningless).
	Known bool
	Holds bool
	// Why explains the verdict; Default is what decided it.
	Why     string
	Default Default
}

// Unset decides the value predicate c (equals, not-equals, matches,
// has-token, has-token-key) on a field the object leaves unset, given the
// field's default and holds, the predicate's positive form over one
// JSON-encoded value (for not-equals: equals). States unset/set are not value
// predicates: the caller decides them from presence alone.
func Unset(c domain.Condition, d Default, holds func(value string) bool) Verdict {
	leaf := subjectOf(c)
	if d.State != Known {
		why := d.Why
		if why == "" {
			why = "nothing establishes its default"
		}
		return Verdict{Why: fmt.Sprintf("no default of %s is established (%s), so %s %s is undecidable on an object that leaves it unset", leaf, why, leaf, c.State), Default: d}
	}
	hit := holds(d.Value)
	r := Verdict{Known: true, Holds: hit, Default: d}
	if c.State == domain.StateNotEquals {
		r.Holds = !hit
	}
	r.Why = fmt.Sprintf("%s is unset and defaults to %s (%s)", leaf, plain(d.Value), d.Source)
	if d.Membership != "" {
		r.Why = fmt.Sprintf("%s is unset, so the object is outside role %s (%s)", leaf, d.Membership, d.Source)
	}
	return r
}

// IsValuePredicate reports whether a value leaf's state compares its value
// (as opposed to testing presence).
func IsValuePredicate(s domain.FieldState) bool {
	switch s {
	case domain.StateEquals, domain.StateNotEquals, domain.StateMatches, domain.StateHasToken, domain.StateHasTokenKey:
		return true
	}
	return false
}

// EncodeJSON encodes a decoded value.
func EncodeJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

// SameValue compares two JSON-encoded scalars by their text ("true" and
// `"true"` differ only in encoding).
func SameValue(a, b string) bool { return plain(a) == plain(b) }

func plain(v string) string {
	var s string
	if json.Unmarshal([]byte(v), &s) == nil {
		return s
	}
	return v
}

func subjectOf(c domain.Condition) string {
	switch c.Op {
	case domain.OpCLIFlag:
		return "flag --" + strings.TrimLeft(c.Name, "-")
	case domain.OpEnvVar:
		return "env var " + c.Name
	case domain.OpValuesKey:
		return "values key " + c.Path
	}
	return c.Path
}

func joinIDs(ids []domain.EvidenceID) string {
	s := make([]string, 0, len(ids))
	for _, id := range ids {
		s = append(s, string(id))
	}
	return strings.Join(s, ", ")
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

// ParentPath is the path of the object that holds path's last field ("" for a
// top-level field): "spec.privateKey.rotationPolicy" → "spec.privateKey",
// "spec.containers[].image" → "spec.containers[]".
func ParentPath(path string) string {
	segs := env.SplitKeyPath(path)
	if len(segs) <= 1 {
		return ""
	}
	return strings.Join(segs[:len(segs)-1], ".")
}

// ParentAbsent is a schema default for a field whose parent object the object
// does not state: the API server defaults a field only inside a present
// parent, so the schema says nothing about it (a rule default still may).
func ParentAbsent(d Default, path string) Default {
	if d.State != Known && d.State != None {
		return d
	}
	return Default{Why: fmt.Sprintf("the object does not state %s, and the API server applies a schema default only inside a present parent", ParentPath(path)), CRDs: d.CRDs}
}
