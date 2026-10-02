package impact

// Applicability conditions of verified knowledge, evaluated against the
// environment (docs/phase3/learning-loop/DESIGN.md §1.3).
//
// Evaluation is three-valued (Kleene): a node is true (exposed, always with
// environment evidence), false (checked against a supplied, healthy
// dimension and clear, always with an ImpactCheck) or unknown (with an
// UnknownReason and what is needed to decide). The rules this file keeps:
//
//  1. Absence is not knowledge: a leaf is false only when its deciding
//     dimension was supplied AND is healthy; under partial health a would-be
//     false is unknown (environment-visibility-gap). A true backed by
//     evidence stands under partial health.
//  2. Withheld values decide nothing: a predicate that needs a value marked
//     withheld (sensitive / oversize) or a withheld text line is unknown;
//     set/unset stay decidable (presence is not the value).
//  3. Scope is per resource: the operands of one `resource` node are
//     evaluated against the same resource.
//  4. Every true carries evidence and every false carries a check.
//  5. No product-specific code: every predicate is generic over its fields.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

// Truth is a three-valued (Kleene) truth value.
type Truth string

const (
	True    Truth = "true"
	False   Truth = "false"
	Unknown Truth = "unknown"
)

// ConditionResult is the value of a condition (or one node of it) for one
// environment.
type ConditionResult struct {
	Value Truth
	// Matches are the environment facts that make a true true (each with
	// environment evidence). Empty unless Value is True.
	Matches []domain.ImpactMatch
	// Checks are the evaluation record of a false (what was checked against
	// what); an unknown may carry the partial record.
	Checks []domain.ImpactCheck
	// Examined are the environment records a false examined: what `not`
	// turns into evidence ("no line matches" cites the lines it read).
	Examined []domain.EvidenceID
	// Reason and Needed explain an unknown.
	Reason domain.UnknownReason
	Needed []string
	// Records are evidence records the evaluation created that the
	// environment's own pool does not hold (the from-version input, the
	// values files an absence was read from). The engine copies the cited
	// ones into the report's environment evidence.
	Records []domain.Evidence

	// deps are the dimensions the value depended on (for `not`).
	deps []domain.EnvironmentDimension
}

// RenderedChangeEvaluator evaluates the rendered-change predicate: a field of
// a rendered object differs between the environment's From and To renders
// (customer values). The render lane (internal/render) implements it; the
// engine only depends on this interface. Without one, every rendered-change
// leaf is unknown (environment-visibility-gap, render unavailable).
type RenderedChangeEvaluator interface {
	EvaluateRenderedChange(c domain.Condition, e *env.Environment, edge *domain.UpgradeEdge) ConditionResult
}

// RenderUnavailable is the default RenderedChangeEvaluator: no renders exist,
// so a rendered-change leaf can never be decided.
type RenderUnavailable struct{}

// EvaluateRenderedChange always answers unknown.
func (RenderUnavailable) EvaluateRenderedChange(c domain.Condition, _ *env.Environment, _ *domain.UpgradeEdge) ConditionResult {
	return unknownResult(domain.UnknownEnvironmentVisibilityGap,
		fmt.Sprintf("render unavailable: the environment's From/To renders (customer values) are needed to decide %s", describe(c)))
}

// ConditionOptions tunes EvaluateConditionWith.
type ConditionOptions struct {
	// Render evaluates rendered-change leaves (nil: RenderUnavailable).
	Render RenderedChangeEvaluator
}

// EvaluateCondition evaluates an applicability condition against the
// environment (edge supplies the from-version for edge-from-version leaves).
func EvaluateCondition(c domain.Condition, e *env.Environment, edge *domain.UpgradeEdge) ConditionResult {
	return EvaluateConditionWith(c, e, edge, ConditionOptions{})
}

// EvaluateConditionWith is EvaluateCondition with options.
func EvaluateConditionWith(c domain.Condition, e *env.Environment, edge *domain.UpgradeEdge, opts ConditionOptions) ConditionResult {
	if opts.Render == nil {
		opts.Render = RenderUnavailable{}
	}
	ev := &evaluator{env: e, edge: edge, render: opts.Render, ix: map[domain.EvidenceID]domain.Evidence{}}
	if e == nil {
		return unknownResult(domain.UnknownEnvironmentVisibilityGap, "no environment supplied")
	}
	for _, x := range e.Evidence {
		ev.ix[x.ID] = x
	}
	if err := c.Validate(); err != nil {
		return unknownResult(domain.UnknownSemanticAmbiguity, fmt.Sprintf("the condition is not well-formed: %v", err))
	}
	r := ev.eval(c, nil)
	r.Matches = dedupMatches(r.Matches)
	r.Examined = appendUnique([]domain.EvidenceID(nil), r.Examined...)
	r.Needed = appendUnique([]string(nil), r.Needed...)
	return r
}

type evaluator struct {
	env    *env.Environment
	edge   *domain.UpgradeEdge
	render RenderedChangeEvaluator
	ix     map[domain.EvidenceID]domain.Evidence
}

// --- results ----------------------------------------------------------------------

func unknownResult(reason domain.UnknownReason, needed ...string) ConditionResult {
	return ConditionResult{Value: Unknown, Reason: reason, Needed: needed}
}

func trueResult(dim domain.EnvironmentDimension, matches ...domain.ImpactMatch) ConditionResult {
	return ConditionResult{Value: True, Matches: matches, deps: []domain.EnvironmentDimension{dim}}
}

func falseResult(dim domain.EnvironmentDimension, check domain.ImpactCheck, examined ...domain.EvidenceID) ConditionResult {
	return ConditionResult{Value: False, Checks: []domain.ImpactCheck{check}, Examined: examined, deps: []domain.EnvironmentDimension{dim}}
}

// reasonRank is the precedence of unknown reasons when several unknown leaves
// decide a combinator (DESIGN.md §1.5): the reason most actionable by the user
// first.
var reasonRank = map[domain.UnknownReason]int{
	domain.UnknownCrossProductContextGap:   0,
	domain.UnknownEnvironmentVisibilityGap: 1,
	domain.UnknownRuntimeBehaviorGap:       2,
	domain.UnknownEvidenceGap:              3,
	domain.UnknownSemanticAmbiguity:        4,
	domain.UnknownReleaseKnowledgeGap:      5,
}

func strongerReason(a, b domain.UnknownReason) domain.UnknownReason {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	if reasonRank[b] < reasonRank[a] {
		return b
	}
	return a
}

// merge folds the bookkeeping of rs into out (records and deps always;
// matches, checks, examined and needed per the caller's choice).
func merge(out *ConditionResult, r ConditionResult, matches, checks, examined, needed bool) {
	out.Records = appendRecords(out.Records, r.Records...)
	out.deps = appendUnique(out.deps, r.deps...)
	if matches {
		out.Matches = append(out.Matches, r.Matches...)
	}
	if checks {
		out.Checks = append(out.Checks, r.Checks...)
	}
	if examined {
		out.Examined = appendUnique(out.Examined, r.Examined...)
	}
	if needed {
		out.Needed = appendUnique(out.Needed, r.Needed...)
		out.Reason = strongerReason(out.Reason, r.Reason)
	}
}

func appendRecords(xs []domain.Evidence, ys ...domain.Evidence) []domain.Evidence {
	for _, y := range ys {
		dup := false
		for _, x := range xs {
			if x.ID == y.ID {
				dup = true
				break
			}
		}
		if !dup {
			xs = append(xs, y)
		}
	}
	return xs
}

// all is Kleene conjunction: any false → false; all true → true; else unknown.
func all(rs []ConditionResult) ConditionResult {
	var out ConditionResult
	nTrue, nFalse := 0, 0
	for _, r := range rs {
		switch r.Value {
		case True:
			nTrue++
		case False:
			nFalse++
		}
	}
	switch {
	case nFalse > 0:
		out.Value = False
		for _, r := range rs {
			merge(&out, r, false, r.Value == False, r.Value == False, false)
		}
	case nTrue == len(rs):
		out.Value = True
		for _, r := range rs {
			merge(&out, r, true, false, false, false)
		}
	default:
		out.Value = Unknown
		for _, r := range rs {
			merge(&out, r, false, false, false, r.Value == Unknown)
		}
	}
	return out
}

// any is Kleene disjunction: any true → true; all false → false; else unknown.
func anyOf(rs []ConditionResult) ConditionResult {
	var out ConditionResult
	nTrue, nFalse := 0, 0
	for _, r := range rs {
		switch r.Value {
		case True:
			nTrue++
		case False:
			nFalse++
		}
	}
	switch {
	case nTrue > 0:
		out.Value = True
		for _, r := range rs {
			merge(&out, r, r.Value == True, false, false, false)
		}
	case nFalse == len(rs):
		out.Value = False
		for _, r := range rs {
			merge(&out, r, false, true, true, false)
		}
	default:
		out.Value = Unknown
		for _, r := range rs {
			merge(&out, r, false, false, false, r.Value == Unknown)
		}
	}
	return out
}

// --- dimension health -----------------------------------------------------------------

// dimOK reports whether a would-be false of a leaf on this dimension may stand:
// the dimension was supplied and parsed completely. needed explains why not.
func (ev *evaluator) dimOK(dim domain.EnvironmentDimension) (ok bool, needed string) {
	e := ev.env
	switch dim {
	case domain.DimensionValues:
		switch {
		case !e.Supplied.Values:
			return false, "Helm values files (--values) not supplied"
		case e.Health(env.DimValues) != env.HealthOK:
			return false, "Helm values were only partially parsed; a key may be set in the unparsed part"
		}
	case domain.DimensionManifests:
		switch {
		case !e.Supplied.Manifests:
			return false, "Kubernetes manifests (--manifests) not supplied"
		case e.Health(env.DimManifests) != env.HealthOK:
			return false, "Kubernetes manifests were only partially parsed; the deciding resource may be in the unparsed part"
		}
	case domain.DimensionImages:
		if !(e.Supplied.Images || e.Supplied.Values || e.Supplied.Manifests) {
			return false, "no image references supplied (--images, --values or --manifests)"
		}
		for _, d := range []string{env.DimImages, env.DimValues, env.DimManifests} {
			if e.Health(d) == env.HealthPartial {
				return false, "an image-bearing input was only partially parsed; an image may be referenced in the unparsed part"
			}
		}
	case domain.DimensionProducts:
		switch e.Health(env.DimProducts) {
		case env.HealthAbsent:
			return false, "product inventory (--inventory) not supplied"
		case env.HealthPartial:
			return false, "the product inventory is partial (unparsable entries or conflicting versions)"
		}
	case domain.DimensionCluster:
		if e.Kubernetes == nil {
			return false, "Kubernetes cluster version (--kubernetes) not supplied"
		}
	}
	return true, ""
}

// decideFalse turns a would-be false into the false itself, or into unknown
// when the deciding dimension is absent or partial (rule 1).
func (ev *evaluator) decideFalse(dim domain.EnvironmentDimension, check domain.ImpactCheck, examined ...domain.EvidenceID) ConditionResult {
	if ok, needed := ev.dimOK(dim); !ok {
		r := unknownResult(domain.UnknownEnvironmentVisibilityGap, needed)
		r.deps = []domain.EnvironmentDimension{dim}
		return r
	}
	return falseResult(dim, check, examined...)
}

func (ev *evaluator) check(dim domain.EnvironmentDimension, facts int, c domain.Condition, evidence ...domain.EvidenceID) domain.ImpactCheck {
	chk := domain.ImpactCheck{Dimension: dim, Facts: facts, Subjects: []string{describe(c)}, Evidence: evidence}
	if dim == domain.DimensionCluster {
		chk.Platform = "kubernetes"
	}
	return chk
}

// --- the tree ---------------------------------------------------------------------------

// eval evaluates c; scope is the resource a scoped leaf reads (nil outside a
// resource/ref scope).
func (ev *evaluator) eval(c domain.Condition, scope *env.Resource) ConditionResult {
	switch c.Op {
	case domain.OpAll:
		return all(ev.operands(c.Of, scope))
	case domain.OpAny:
		return anyOf(ev.operands(c.Of, scope))
	case domain.OpNot:
		return ev.not(c, scope)
	case domain.OpResource:
		return ev.resource(c)
	case domain.OpField:
		return ev.field(c, scope)
	case domain.OpTextLine:
		return ev.textLine(c, scope)
	case domain.OpRef:
		return ev.ref(c, scope)
	case domain.OpValuesKey:
		return ev.valuesKey(c)
	case domain.OpGVKInUse:
		return ev.gvkInUse(c)
	case domain.OpImageInUse:
		return ev.imageInUse(c)
	case domain.OpCLIFlag:
		return ev.cliFlag(c)
	case domain.OpEnvVar:
		return ev.envVar(c)
	case domain.OpFeatureGate:
		return ev.featureGate(c)
	case domain.OpProductVersion:
		return ev.productVersion(c)
	case domain.OpClusterVersion:
		return ev.clusterVersion(c)
	case domain.OpEdgeFromVersion:
		return ev.edgeFromVersion(c)
	case domain.OpRenderedChange:
		// CONTRACT-CHANGE(render): the evaluator outside this package cannot
		// set deps; record the render dimension here so `not` labels its check
		// correctly. Health needs no gate: a rendered true rests on rendered
		// evidence, and the render evaluator returns false only for complete,
		// successful renders.
		r := ev.render.EvaluateRenderedChange(c, ev.env, ev.edge)
		if r.Value != Unknown {
			r.deps = []domain.EnvironmentDimension{domain.DimensionRender}
		}
		return r
	case domain.OpUndecidable:
		reason := c.Reason
		if !reason.Valid() {
			reason = domain.UnknownRuntimeBehaviorGap
		}
		return unknownResult(reason, c.Needed)
	}
	return unknownResult(domain.UnknownSemanticAmbiguity, fmt.Sprintf("unknown condition op %q", c.Op))
}

func (ev *evaluator) operands(cs []domain.Condition, scope *env.Resource) []ConditionResult {
	out := make([]ConditionResult, 0, len(cs))
	for _, c := range cs {
		out = append(out, ev.eval(c, scope))
	}
	return out
}

// not: not(true) = false; not(unknown) = unknown; not(false) = true only when
// the operand examined at least one environment record — that record becomes
// the evidence of the true. Absence of input is never negated into evidence.
func (ev *evaluator) not(c domain.Condition, scope *env.Resource) ConditionResult {
	r := ev.eval(c.Of[0], scope)
	out := ConditionResult{Records: r.Records, deps: r.deps}
	switch r.Value {
	case Unknown:
		out.Value, out.Reason, out.Needed = Unknown, r.Reason, r.Needed
	case True:
		// a false needs every dimension the operand read to be healthy
		for _, d := range r.deps {
			if ok, needed := ev.dimOK(d); !ok {
				out.Value, out.Reason = Unknown, domain.UnknownEnvironmentVisibilityGap
				out.Needed = appendUnique(out.Needed, needed)
			}
		}
		if out.Value == Unknown {
			return out
		}
		out.Value = False
		var evs []domain.EvidenceID
		for _, m := range r.Matches {
			evs = appendUnique(evs, m.Evidence...)
		}
		out.Examined = evs
		dims := r.deps
		if len(dims) == 0 {
			dims = []domain.EnvironmentDimension{domain.DimensionManifests}
		}
		for _, d := range dims {
			out.Checks = append(out.Checks, ev.check(d, len(r.Matches), c))
		}
	case False:
		if len(r.Examined) == 0 {
			out.Value, out.Reason = Unknown, domain.UnknownEnvironmentVisibilityGap
			out.Needed = []string{fmt.Sprintf("%s examined no environment record, so its negation has no evidence", describe(c.Of[0]))}
			return out
		}
		out.Value = True
		out.Matches = []domain.ImpactMatch{{Kind: domain.MatchAbsence, Subject: describe(c), Evidence: r.Examined}}
	}
	return out
}

// --- resource scope ---------------------------------------------------------------------

func selectorOf(c domain.Condition) env.GVKSelector {
	return env.GVKSelector{Group: c.Group, Version: c.Version, Kind: c.Kind}
}

func resourceLabel(r *env.Resource) string {
	s := apiVersionString(r.Group, r.Version) + " " + r.Kind
	if r.Name != "" {
		if r.Namespace != "" {
			s += " " + r.Namespace + "/" + r.Name
		} else {
			s += " " + r.Name
		}
	}
	return s
}

func apiVersionString(group, version string) string {
	if group == "" {
		return version
	}
	return group + "/" + version
}

// resource: some single resource of the kind satisfies all operands. False
// iff manifests are supplied and healthy and every resource of the kind is
// false (zero resources → false, the impact:crd-field-unset convention).
func (ev *evaluator) resource(c domain.Condition) ConditionResult {
	if ok, needed := ev.dimOK(domain.DimensionManifests); !ok && !ev.env.Supplied.Manifests {
		r := unknownResult(domain.UnknownEnvironmentVisibilityGap, needed)
		r.deps = []domain.EnvironmentDimension{domain.DimensionManifests}
		return r
	}
	var rs []*env.Resource
	for _, r := range ev.env.Select(selectorOf(c)) {
		if c.Name == "" || r.Name == c.Name {
			rs = append(rs, r)
		}
	}
	if len(rs) == 0 {
		return ev.decideFalse(domain.DimensionManifests, ev.check(domain.DimensionManifests, 0, c))
	}
	per := make([]ConditionResult, 0, len(rs))
	for _, r := range rs {
		x := all(ev.operands(c.Of, r))
		x.deps = appendUnique(x.deps, domain.DimensionManifests)
		switch x.Value {
		case True:
			x.Matches = append([]domain.ImpactMatch{{Kind: domain.MatchAPIVersion, Subject: resourceLabel(r), Evidence: []domain.EvidenceID{r.Evidence}}}, x.Matches...)
		case False:
			x.Examined = appendUnique(x.Examined, r.Evidence)
		}
		per = append(per, x)
	}
	out := anyOf(per)
	if out.Value == False {
		// one check for the scope, counting every resource examined, keeping
		// the evidence the inner checks cite (e.g. a completeness declaration)
		scope := ev.check(domain.DimensionManifests, len(rs), c)
		for _, x := range out.Checks {
			scope.Evidence = appendUnique(scope.Evidence, x.Evidence...)
		}
		out.Checks = []domain.ImpactCheck{scope}
	}
	return out
}

// fieldFacts returns the facts of r at path ("[]" markers cover every
// element; an explicit index selects one).
func fieldFacts(r *env.Resource, path string) []env.FieldFact {
	indexed := indexedPath.MatchString(path)
	var out []env.FieldFact
	for _, f := range r.Fields {
		if (!indexed && f.Path == path) || (indexed && f.Element == path) {
			out = append(out, f)
		}
	}
	return out
}

var indexedPath = regexp.MustCompile(`\[\d+\]`)

// vfact is one observed value of a value predicate.
type vfact struct {
	subject   string
	value     string // JSON-encoded scalar
	container bool
	withheld  string
	evidence  []domain.EvidenceID
	kind      domain.ImpactMatchKind
}

func (ev *evaluator) where(ids []domain.EvidenceID) string {
	for _, id := range ids {
		if e, ok := ev.ix[id]; ok {
			loc := e.Locator
			if m := lineRe.FindStringSubmatch(loc); m != nil {
				return e.URI + ":" + m[1]
			}
			return e.URI
		}
	}
	return "an environment file"
}

var lineRe = regexp.MustCompile(`\(L(\d+)\)`)

func (ev *evaluator) field(c domain.Condition, r *env.Resource) ConditionResult {
	if r == nil {
		return unknownResult(domain.UnknownSemanticAmbiguity, "a field predicate outside a resource scope")
	}
	var facts []vfact
	for _, f := range fieldFacts(r, c.Path) {
		facts = append(facts, vfact{
			subject: resourceLabel(r) + ": " + f.Element, value: f.Value, container: f.Container,
			withheld: f.Withheld, evidence: f.Evidence, kind: domain.MatchManifestField,
		})
	}
	absence := []domain.ImpactMatch{{Kind: domain.MatchAbsence, Subject: resourceLabel(r) + " leaves " + c.Path + " unset", Evidence: []domain.EvidenceID{r.Evidence}}}
	return ev.valueLeaf(c, domain.DimensionManifests, facts, absence, 1)
}

// valueLeaf evaluates the value states (unset, set, equals, not-equals,
// matches, has-token, has-token-key) over the observed facts of one subject.
// absence is the evidence an unset subject is read from (empty when the
// dimension offers none); n counts the facts compared for the check.
func (ev *evaluator) valueLeaf(c domain.Condition, dim domain.EnvironmentDimension, facts []vfact, absence []domain.ImpactMatch, n int) ConditionResult {
	set := len(facts) > 0
	var all []domain.EvidenceID
	for _, f := range facts {
		all = appendUnique(all, f.evidence...)
	}
	absenceEvidence := func() []domain.EvidenceID {
		var out []domain.EvidenceID
		for _, m := range absence {
			out = appendUnique(out, m.Evidence...)
		}
		return out
	}
	chk := ev.check(dim, n, c)
	matchOf := func(fs []vfact) []domain.ImpactMatch {
		out := make([]domain.ImpactMatch, 0, len(fs))
		for _, f := range fs {
			out = append(out, domain.ImpactMatch{Kind: f.kind, Subject: f.subject, Evidence: f.evidence})
		}
		return out
	}
	unsetFalse := func() ConditionResult { return ev.decideFalse(dim, chk, absenceEvidence()...) }
	switch c.State {
	case domain.StateSet:
		if set {
			return trueResult(dim, matchOf(facts)...)
		}
		return unsetFalse()
	case domain.StateUnset:
		if set {
			return ev.decideFalse(dim, chk, all...)
		}
		if ok, needed := ev.dimOK(dim); !ok {
			r := unknownResult(domain.UnknownEnvironmentVisibilityGap, needed)
			r.deps = []domain.EnvironmentDimension{dim}
			return r
		}
		if len(absence) == 0 {
			return unknownResult(domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("no environment record to read %s from", describe(c)))
		}
		return trueResult(dim, absence...)
	}
	if !set {
		return unsetFalse()
	}
	var scalars, hits, withheld []vfact
	for _, f := range facts {
		if f.container {
			continue
		}
		scalars = append(scalars, f)
		if f.withheld != "" {
			withheld = append(withheld, f)
			continue
		}
		if ev.valueHolds(c, f.value) {
			hits = append(hits, f)
		}
	}
	withheldUnknown := func() ConditionResult {
		var needed []string
		for _, f := range withheld {
			needed = append(needed, fmt.Sprintf("value withheld (%s) at %s", f.withheld, ev.where(f.evidence)))
		}
		r := unknownResult(domain.UnknownEnvironmentVisibilityGap, needed...)
		r.deps = []domain.EnvironmentDimension{dim}
		return r
	}
	if c.State == domain.StateNotEquals {
		switch {
		case len(hits) > 0: // set to one of Values
			return ev.decideFalse(dim, chk, all...)
		case len(withheld) > 0:
			return withheldUnknown()
		}
		return trueResult(dim, matchOf(facts)...)
	}
	switch {
	case len(hits) > 0:
		return trueResult(dim, matchOf(hits)...)
	case len(withheld) > 0:
		return withheldUnknown()
	}
	return ev.decideFalse(dim, chk, all...)
}

// valueHolds reports whether a JSON-encoded value satisfies the positive form
// of c's state (for not-equals: equals).
func (ev *evaluator) valueHolds(c domain.Condition, value string) bool {
	switch c.State {
	case domain.StateEquals, domain.StateNotEquals:
		for _, v := range c.Values {
			if jsonEqual(v, value) {
				return true
			}
		}
	case domain.StateMatches:
		re, err := regexp.Compile(c.Pattern)
		return err == nil && re.MatchString(plainValue(value))
	case domain.StateHasToken, domain.StateHasTokenKey:
		sep := c.Separator
		if sep == "" {
			sep = ","
		}
		for _, tok := range tokens(value, sep) {
			key := tok
			if c.State == domain.StateHasTokenKey {
				key, _, _ = strings.Cut(tok, "=")
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

// jsonEqual compares two JSON-encoded scalars semantically ("1" == "1.0" is
// not claimed: numbers compare by their canonical encoding).
func jsonEqual(a, b string) bool {
	if a == b {
		return true
	}
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	xb, _ := json.Marshal(x)
	yb, _ := json.Marshal(y)
	return string(xb) == string(yb)
}

// plainValue decodes a JSON string to its text; other values stay encoded.
func plainValue(v string) string {
	var s string
	if json.Unmarshal([]byte(v), &s) == nil {
		return s
	}
	return v
}

// tokens splits a JSON-encoded value into list tokens: a string is split by
// sep, an array contributes each element (strings split by sep as well).
func tokens(v, sep string) []string {
	var out []string
	split := func(s string) {
		for _, t := range strings.Split(s, sep) {
			if t = strings.TrimSpace(t); t != "" {
				out = append(out, t)
			}
		}
	}
	var s string
	if json.Unmarshal([]byte(v), &s) == nil {
		split(s)
		return out
	}
	var arr []any
	if json.Unmarshal([]byte(v), &arr) == nil {
		for _, x := range arr {
			switch t := x.(type) {
			case string:
				split(t)
			default:
				b, _ := json.Marshal(t)
				out = append(out, string(b))
			}
		}
	}
	return out
}

// textBlocks returns the text blocks of r at or below path (TextBlocks semantics).
func textBlocks(r *env.Resource, path string) []env.TextBlock {
	var out []env.TextBlock
	for _, b := range r.Texts {
		if path == "" || b.Path == path || strings.HasPrefix(b.Path, path+".") || strings.HasPrefix(b.Path, path+"[") {
			out = append(out, b)
		}
	}
	return out
}

// maxExamined caps the line evidence a text-line result carries.
const maxExamined = 20

func (ev *evaluator) textLine(c domain.Condition, r *env.Resource) ConditionResult {
	if r == nil {
		return unknownResult(domain.UnknownSemanticAmbiguity, "a text-line predicate outside a resource scope")
	}
	re, err := regexp.Compile(c.Pattern)
	if err != nil {
		return unknownResult(domain.UnknownSemanticAmbiguity, fmt.Sprintf("pattern %q: %v", c.Pattern, err))
	}
	blocks := textBlocks(r, c.Path)
	var hits []domain.ImpactMatch
	var examined []domain.EvidenceID
	incomplete := 0
	var withheldAt []string
	for _, b := range blocks {
		if f, ok := factByElement(r, b.Path); ok && f.Withheld != "" {
			// the field's value is withheld, so its lines are too, whatever
			// the line-level redaction decided (a credential-named key's
			// text never decides anything)
			b.Withheld, b.Lines = len(b.Lines), nil
		}
		for _, ln := range b.Match(re) {
			hits = append(hits, domain.ImpactMatch{Kind: domain.MatchTextLine, Subject: fmt.Sprintf("%s: %s line %d", resourceLabel(r), b.Path, ln.N), Evidence: []domain.EvidenceID{ln.Evidence}})
		}
		for _, ln := range b.Lines {
			if len(examined) < maxExamined {
				examined = append(examined, ln.Evidence)
			}
		}
		if b.Withheld > 0 || b.Truncated {
			incomplete++
			what := fmt.Sprintf("%d line(s) withheld (sensitive)", b.Withheld)
			if b.Truncated {
				what = "lines beyond the inventory cap"
			}
			withheldAt = append(withheldAt, fmt.Sprintf("%s in %s %s at %s", what, resourceLabel(r), b.Path, ev.where([]domain.EvidenceID{r.Evidence})))
		}
	}
	examined = appendUnique([]domain.EvidenceID{r.Evidence}, examined...)
	chk := ev.check(domain.DimensionManifests, len(examined)-1, c)
	unknownWithheld := func() ConditionResult {
		x := unknownResult(domain.UnknownEnvironmentVisibilityGap, withheldAt...)
		x.deps = []domain.EnvironmentDimension{domain.DimensionManifests}
		return x
	}
	switch c.State {
	case domain.StateExists:
		switch {
		case len(hits) > 0:
			return trueResult(domain.DimensionManifests, hits...)
		case incomplete > 0:
			return unknownWithheld() // a withheld line could be the match
		}
		return ev.decideFalse(domain.DimensionManifests, chk, examined...)
	case domain.StateNone:
		switch {
		case len(hits) > 0:
			var evs []domain.EvidenceID
			for _, h := range hits {
				evs = append(evs, h.Evidence...)
			}
			return ev.decideFalse(domain.DimensionManifests, chk, evs...)
		case incomplete > 0:
			return unknownWithheld()
		case len(blocks) == 0:
			// no text at the path at all: "no line matches" rests on the
			// absence of the block, so it needs a healthy dimension
			if ok, needed := ev.dimOK(domain.DimensionManifests); !ok {
				return unknownResult(domain.UnknownEnvironmentVisibilityGap, needed)
			}
		}
		return trueResult(domain.DimensionManifests, domain.ImpactMatch{
			Kind: domain.MatchAbsence, Subject: fmt.Sprintf("no line of %s %s matches /%s/", resourceLabel(r), c.Path, c.Pattern), Evidence: examined,
		})
	}
	return unknownResult(domain.UnknownSemanticAmbiguity, fmt.Sprintf("text-line state %q", c.State))
}

func (ev *evaluator) ref(c domain.Condition, r *env.Resource) ConditionResult {
	if r == nil {
		return unknownResult(domain.UnknownSemanticAmbiguity, "a ref predicate outside a resource scope")
	}
	var refs []env.Ref
	for _, x := range r.Refs {
		if x.Path == c.Path {
			refs = append(refs, x)
		}
	}
	chk := ev.check(domain.DimensionManifests, len(refs), c)
	if len(refs) == 0 {
		return ev.decideFalse(domain.DimensionManifests, chk, r.Evidence)
	}
	per := make([]ConditionResult, 0, len(refs))
	for _, ref := range refs {
		res := ev.env.ResolveRef(r, ref)
		switch res.Status {
		case env.RefAmbiguous:
			per = append(per, unknownResult(domain.UnknownSemanticAmbiguity, fmt.Sprintf("%s %s: %s", resourceLabel(r), ref.Element, res.Reason)))
		case env.RefUnresolved:
			if !res.ManifestsComplete {
				// absence is not knowledge: clean parsing is not completeness
				x := unknownResult(domain.UnknownEnvironmentVisibilityGap,
					fmt.Sprintf("the manifest of the referenced %s (%s %s points at it; it is not among the supplied manifests, which are not declared complete — supply it, or pass --manifests-complete if nothing else runs)",
						refTarget(ref, c), resourceLabel(r), ref.Element))
				x.deps = []domain.EnvironmentDimension{domain.DimensionManifests}
				per = append(per, x)
				continue
			}
			declared := chk
			declared.Evidence = append(append([]domain.EvidenceID{}, chk.Evidence...), ev.env.ManifestsCompleteEvidence...)
			per = append(per, falseResult(domain.DimensionManifests, declared, append(append([]domain.EvidenceID{}, ref.Evidence...), ev.env.ManifestsCompleteEvidence...)...))
		case env.RefResolved:
			t := res.Target
			if (c.Kind != "" && t.Kind != c.Kind) || (c.Group != "" && t.Group != c.Group) {
				per = append(per, falseResult(domain.DimensionManifests, chk, append(append([]domain.EvidenceID{}, ref.Evidence...), t.Evidence)...))
				continue
			}
			x := all(ev.operands(c.Of, t))
			switch x.Value {
			case True:
				x.Matches = append([]domain.ImpactMatch{{Kind: domain.MatchReference, Subject: fmt.Sprintf("%s %s → %s", resourceLabel(r), ref.Element, resourceLabel(t)),
					Evidence: append(append([]domain.EvidenceID{}, ref.Evidence...), t.Evidence)}}, x.Matches...)
			case False:
				x.Examined = appendUnique(x.Examined, ref.Evidence...)
			}
			per = append(per, x)
		}
	}
	return anyOf(per)
}

// refTarget names the object a reference points at ("Issuer letsencrypt").
func refTarget(ref env.Ref, c domain.Condition) string {
	kind := ref.Kind
	if kind == "" {
		kind = c.Kind
	}
	if kind == "" {
		kind = "object"
	}
	s := kind + " " + ref.Name
	if ref.Namespace != "" {
		s = kind + " " + ref.Namespace + "/" + ref.Name
	}
	return s
}

// --- environment-wide leaves -------------------------------------------------------------

// valuesAbsence is the evidence an unset values key is read from: one record
// per values file that was read (the file and its digest), or the --values
// input when no file yielded a key.
func (ev *evaluator) valuesAbsence(c domain.Condition) ([]domain.ImpactMatch, []domain.Evidence) {
	var recs []domain.Evidence
	seen := map[string]bool{}
	digest := map[string]string{}
	for _, f := range ev.env.Files {
		digest[f.Path] = f.Digest
	}
	for _, k := range ev.env.ValuesKeys {
		for _, id := range k.Evidence {
			e, ok := ev.ix[id]
			if !ok || seen[e.URI] {
				continue
			}
			seen[e.URI] = true
			recs = append(recs, domain.NewEvidence(domain.EvidenceLocalFile, "", e.URI, "",
				fmt.Sprintf("%s does not set %s (or anything below it)", e.URI, c.Path), digest[e.URI], time.Time{}))
		}
	}
	if len(recs) == 0 && ev.env.Supplied.Values {
		recs = append(recs, domain.NewEvidence(domain.EvidenceInput, "", "flag:--values", "",
			"the supplied values set no keys", domain.Digest([]byte("flag:--values:none")), time.Time{}))
	}
	var ids []domain.EvidenceID
	for _, r := range recs {
		ids = append(ids, r.ID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	return []domain.ImpactMatch{{Kind: domain.MatchAbsence, Subject: "values do not set " + c.Path, Evidence: ids}}, recs
}

func (ev *evaluator) valuesKey(c domain.Condition) ConditionResult {
	var exact *env.ValuesKey
	var facts []vfact
	for i, k := range ev.env.ValuesKeys {
		switch relate(c.Path, k.Path) {
		case relExact:
			exact = &ev.env.ValuesKeys[i] // the last file wins per key, as in Helm
		case relSubjectAncestor:
			facts = append(facts, vfact{subject: k.Path, container: true, evidence: k.Evidence, kind: domain.MatchValuesKey})
		}
	}
	if exact != nil {
		facts = append([]vfact{{subject: exact.Path, value: exact.Value, evidence: exact.Evidence, kind: domain.MatchValuesKey}}, facts...)
	}
	absence, recs := ev.valuesAbsence(c)
	r := ev.valueLeaf(c, domain.DimensionValues, facts, absence, len(ev.env.ValuesKeys))
	if r.Value != Unknown {
		r.Records = appendRecords(r.Records, recs...)
	}
	return r
}

func (ev *evaluator) gvkInUse(c domain.Condition) ConditionResult {
	var hits []domain.ImpactMatch
	var examined []domain.EvidenceID
	for _, u := range ev.env.GVKUsage {
		if len(u.FieldPaths) == 0 {
			continue // CRD-served versions, not manifest use
		}
		if len(examined) < maxExamined {
			examined = appendUnique(examined, u.Evidence...)
		}
		if u.Group != c.Group || u.Kind != c.Kind || (c.Version != "" && u.Version != c.Version) {
			continue
		}
		hits = append(hits, domain.ImpactMatch{Kind: domain.MatchAPIVersion, Subject: groupVersionOf(u) + " " + u.Kind, Evidence: u.Evidence})
	}
	if len(hits) > 0 {
		return trueResult(domain.DimensionManifests, hits...)
	}
	return ev.decideFalse(domain.DimensionManifests, ev.check(domain.DimensionManifests, len(ev.env.APIVersions), c), examined...)
}

func (ev *evaluator) imageInUse(c domain.Condition) ConditionResult {
	var per []ConditionResult
	var examined []domain.EvidenceID
	for _, u := range ev.env.Images {
		if len(examined) < maxExamined {
			examined = appendUnique(examined, u.Evidence...)
		}
		if u.Repository != c.Name {
			continue
		}
		m := domain.ImpactMatch{Kind: domain.MatchImage, Subject: u.Reference, Evidence: u.Evidence}
		if c.State == "" {
			per = append(per, trueResult(domain.DimensionImages, m))
			continue
		}
		in, ok := versionIn(u.Tag, c.Range)
		switch {
		case !ok:
			per = append(per, unknownResult(domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("image %s states no comparable version tag (range %s)", u.Reference, c.Range)))
		case in == (c.State == domain.StateInRange):
			per = append(per, trueResult(domain.DimensionImages, m))
		default:
			per = append(per, ev.decideFalse(domain.DimensionImages, ev.check(domain.DimensionImages, len(ev.env.Images), c), u.Evidence...))
		}
	}
	if len(per) == 0 {
		return ev.decideFalse(domain.DimensionImages, ev.check(domain.DimensionImages, len(ev.env.Images), c), examined...)
	}
	return anyOf(per)
}

// containerArgs are the args/command elements of workload containers.
var containerArgPath = regexp.MustCompile(`(?:^|\.)(?:containers|initContainers|ephemeralContainers)\[\]\.(?:args|command)\[\]$`)
var containerEnvNamePath = regexp.MustCompile(`(?:^|\.)(?:containers|initContainers|ephemeralContainers)\[\]\.env\[\]\.name$`)

// containerOf returns the element path of the container an element sits in
// ("spec.template.spec.containers[0].args[2]" → "spec.template.spec.containers[0]").
func containerOf(element string) string {
	for _, k := range []string{".args[", ".command[", ".env["} {
		if i := strings.LastIndex(element, k); i > 0 {
			return element[:i]
		}
	}
	return ""
}

func factByElement(r *env.Resource, element string) (env.FieldFact, bool) {
	for _, f := range r.Fields {
		if f.Element == element {
			return f, true
		}
	}
	return env.FieldFact{}, false
}

// containerName returns the name of the container holding element ("" if none).
func containerName(r *env.Resource, element string) string {
	if f, ok := factByElement(r, containerOf(element)+".name"); ok {
		return plainValue(f.Value)
	}
	return ""
}

var containerNamePath = regexp.MustCompile(`(?:^|\.)(?:containers|initContainers|ephemeralContainers)\[\]\.name$`)

// containersNamed returns the evidence of every workload container of that
// name in the supplied manifests (a condition's Component).
func (ev *evaluator) containersNamed(name string) []domain.EvidenceID {
	var out []domain.EvidenceID
	for i := range ev.env.Resources {
		for _, f := range ev.env.Resources[i].Fields {
			if !f.Container && f.Withheld == "" && containerNamePath.MatchString(f.Path) && plainValue(f.Value) == name {
				out = appendUnique(out, f.Evidence...)
			}
		}
	}
	return out
}

// componentAbsent decides a cli-flag / env-var / feature-gate leaf whose named
// component has no workload in the supplied manifests. Absence is not
// knowledge: Helm-installed controllers are rarely among the manifests a
// customer supplies, so a missing workload means "not shown", not "not
// running" — unknown (environment-visibility-gap). Only when the manifests are
// declared complete (`--manifests-complete`) and parsed healthily is the
// workload genuinely absent: false (nothing runs that could be exposed), with
// a check citing the declaration.
func (ev *evaluator) componentAbsent(c domain.Condition) ConditionResult {
	if ok, _ := ev.dimOK(domain.DimensionManifests); ok && ev.env.ManifestsDeclaredComplete {
		chk := ev.check(domain.DimensionManifests, len(ev.env.Resources), c, ev.env.ManifestsCompleteEvidence...)
		return falseResult(domain.DimensionManifests, chk, ev.env.ManifestsCompleteEvidence...)
	}
	r := unknownResult(domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf(
		"the manifests of the %s workload (no container named %q is among the supplied manifests, which are not declared complete; supply its Deployment/DaemonSet, or pass --manifests-complete if nothing else runs)",
		c.Component, c.Component))
	r.deps = []domain.EnvironmentDimension{domain.DimensionManifests}
	return r
}

// flagUse is one observed command-line flag.
type flagUse struct {
	r        *env.Resource
	name     string // without leading dashes
	value    string // JSON-encoded string; "\"true\"" for a bare flag
	withheld string
	evidence []domain.EvidenceID
	element  string
}

// flags parses the --flag[=value] / --flag value arguments of every workload
// container (component, when given, restricts to containers of that name).
func (ev *evaluator) flags(component string) (uses []flagUse, examined []domain.EvidenceID) {
	for i := range ev.env.Resources {
		r := &ev.env.Resources[i]
		byContainer := map[string][]env.FieldFact{}
		var order []string
		for _, f := range r.Fields {
			if f.Container || !containerArgPath.MatchString(f.Path) {
				continue
			}
			k := containerOf(f.Element)
			if component != "" && containerName(r, f.Element) != component {
				continue
			}
			if _, ok := byContainer[k]; !ok {
				order = append(order, k)
			}
			byContainer[k] = append(byContainer[k], f)
		}
		for _, k := range order {
			args := byContainer[k]
			for j, a := range args {
				if len(examined) < maxExamined {
					examined = appendUnique(examined, a.Evidence...)
				}
				if a.Withheld != "" {
					continue
				}
				s := plainValue(a.Value)
				if !strings.HasPrefix(s, "-") || s == "-" || s == "--" {
					continue
				}
				name, val, hasVal := strings.Cut(strings.TrimLeft(s, "-"), "=")
				u := flagUse{r: r, name: name, evidence: a.Evidence, element: a.Element}
				switch {
				case hasVal:
					u.value = jsonString(val)
				case j+1 < len(args) && !strings.HasPrefix(plainValue(args[j+1].Value), "-") && args[j+1].Withheld == "" && args[j+1].Value != "":
					u.value = jsonString(plainValue(args[j+1].Value))
					u.evidence = append(append([]domain.EvidenceID{}, a.Evidence...), args[j+1].Evidence...)
				case j+1 < len(args) && args[j+1].Withheld != "" && !strings.HasPrefix(plainValue(args[j+1].Value), "-"):
					u.withheld = args[j+1].Withheld
				default:
					u.value = jsonString("true")
				}
				uses = append(uses, u)
			}
		}
	}
	return uses, examined
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func (ev *evaluator) cliFlag(c domain.Condition) ConditionResult {
	want := strings.TrimLeft(c.Name, "-")
	uses, examined := ev.flags(c.Component)
	var facts []vfact
	for _, u := range uses {
		if u.name != want {
			continue
		}
		facts = append(facts, vfact{subject: fmt.Sprintf("%s: %s --%s", resourceLabel(u.r), u.element, u.name), value: u.value,
			withheld: u.withheld, evidence: u.evidence, kind: domain.MatchManifestField})
	}
	if c.Component != "" {
		present := ev.containersNamed(c.Component)
		if len(present) == 0 {
			return ev.componentAbsent(c)
		}
		examined = appendUnique(examined, present...)
	}
	var absence []domain.ImpactMatch
	if len(examined) > 0 {
		absence = []domain.ImpactMatch{{Kind: domain.MatchAbsence, Subject: "no workload container passes --" + want, Evidence: examined}}
	}
	if len(examined) == 0 && c.Component == "" {
		// No workload arguments at all: absence claims need examined evidence
		// (a named component absent from the manifests still decides false,
		// like an absent resource kind).
		return unknownResult(domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("no environment record to read %s from", describe(c)))
	}
	return ev.valueLeaf(c, domain.DimensionManifests, facts, absence, len(examined))
}

func (ev *evaluator) envVar(c domain.Condition) ConditionResult {
	var facts []vfact
	var examined []domain.EvidenceID
	for i := range ev.env.Resources {
		r := &ev.env.Resources[i]
		for _, f := range r.Fields {
			if f.Container || !containerEnvNamePath.MatchString(f.Path) {
				continue
			}
			if c.Component != "" && containerName(r, f.Element) != c.Component {
				continue
			}
			if len(examined) < maxExamined {
				examined = appendUnique(examined, f.Evidence...)
			}
			if plainValue(f.Value) != c.Name {
				continue
			}
			elem := strings.TrimSuffix(f.Element, ".name")
			vf := vfact{subject: fmt.Sprintf("%s: %s", resourceLabel(r), elem), evidence: f.Evidence, kind: domain.MatchManifestField}
			if v, ok := factByElement(r, elem+".value"); ok {
				vf.value, vf.withheld = v.Value, v.Withheld
				vf.evidence = append(append([]domain.EvidenceID{}, f.Evidence...), v.Evidence...)
			} else if _, ok := factByElement(r, elem+".valueFrom"); ok {
				vf.withheld = "indirect: valueFrom"
			} else {
				vf.value = `""`
			}
			facts = append(facts, vf)
		}
	}
	if c.Component != "" {
		present := ev.containersNamed(c.Component)
		if len(present) == 0 {
			return ev.componentAbsent(c)
		}
		examined = appendUnique(examined, present...)
	}
	var absence []domain.ImpactMatch
	if len(examined) > 0 {
		absence = []domain.ImpactMatch{{Kind: domain.MatchAbsence, Subject: "no workload container sets " + c.Name, Evidence: examined}}
	}
	if len(examined) == 0 && c.Component == "" {
		// No workload environment at all: absence claims need examined
		// evidence (a named component absent from the manifests still decides
		// false, like an absent resource kind).
		return unknownResult(domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("no environment record to read %s from", describe(c)))
	}
	return ev.valueLeaf(c, domain.DimensionManifests, facts, absence, len(examined))
}

// gateObservation is one place a feature gate is stated.
type gateObservation struct {
	enabled  bool
	match    domain.ImpactMatch
	withheld bool
}

func parseBool(s string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	return false, false
}

func (ev *evaluator) featureGate(c domain.Condition) ConditionResult {
	var obs []gateObservation
	var unparsable []string
	uses, examined := ev.flags(c.Component)
	for _, u := range uses {
		if u.name != "feature-gates" {
			continue
		}
		if u.withheld != "" {
			obs = append(obs, gateObservation{withheld: true})
			continue
		}
		for _, tok := range tokens(u.value, ",") {
			k, v, hasV := strings.Cut(tok, "=")
			if strings.TrimSpace(k) != c.Name {
				continue
			}
			on, ok := true, true
			if hasV {
				on, ok = parseBool(v)
			}
			if !ok {
				unparsable = append(unparsable, fmt.Sprintf("--feature-gates value %q at %s", tok, ev.where(u.evidence)))
				continue
			}
			obs = append(obs, gateObservation{enabled: on, match: domain.ImpactMatch{Kind: domain.MatchManifestField,
				Subject: fmt.Sprintf("%s: --feature-gates %s", resourceLabel(u.r), tok), Evidence: u.evidence}})
		}
	}
	dims := []domain.EnvironmentDimension{domain.DimensionManifests}
	if c.Path != "" {
		dims = append(dims, domain.DimensionValues)
		for _, k := range ev.env.ValuesKeys {
			switch {
			case k.Path == c.Path:
				examined = appendUnique(examined, k.Evidence...)
				for _, tok := range tokens(k.Value, ",") {
					kk, v, hasV := strings.Cut(tok, "=")
					if strings.TrimSpace(kk) != c.Name {
						continue
					}
					on, ok := true, true
					if hasV {
						on, ok = parseBool(v)
					}
					if ok {
						obs = append(obs, gateObservation{enabled: on, match: domain.ImpactMatch{Kind: domain.MatchValuesKey, Subject: k.Path + ": " + tok, Evidence: k.Evidence}})
					}
				}
			case relate(c.Path+"."+c.Name, k.Path) == relExact:
				if on, ok := parseBool(plainValue(k.Value)); ok {
					obs = append(obs, gateObservation{enabled: on, match: domain.ImpactMatch{Kind: domain.MatchValuesKey, Subject: k.Path + ": " + k.Value, Evidence: k.Evidence}})
				} else {
					unparsable = append(unparsable, fmt.Sprintf("values key %s = %s", k.Path, k.Value))
				}
			}
		}
	}
	var on, off []domain.ImpactMatch
	withheld := false
	for _, o := range obs {
		switch {
		case o.withheld:
			withheld = true
		case o.enabled:
			on = append(on, o.match)
		default:
			off = append(off, o.match)
		}
	}
	if c.Component != "" {
		present := ev.containersNamed(c.Component)
		if len(present) == 0 && len(obs) == 0 && len(unparsable) == 0 {
			// nothing states the gate and the named component's workload is
			// not in the manifests: silence is not absence
			return ev.componentAbsent(c)
		}
		examined = appendUnique(examined, present...)
	}
	decideFalse := func(evs []domain.EvidenceID) ConditionResult {
		var r ConditionResult
		for _, d := range dims {
			x := ev.decideFalse(d, ev.check(d, len(obs), c), evs...)
			if x.Value == Unknown {
				return x
			}
			if r.Value == "" {
				r = x
			} else {
				r.Checks = append(r.Checks, x.Checks...)
				r.deps = appendUnique(r.deps, x.deps...)
			}
		}
		return r
	}
	evidenceOf := func(ms []domain.ImpactMatch) []domain.EvidenceID {
		var out []domain.EvidenceID
		for _, m := range ms {
			out = appendUnique(out, m.Evidence...)
		}
		return out
	}
	undecided := func() ConditionResult {
		needed := append([]string{}, unparsable...)
		if withheld {
			needed = append(needed, "a --feature-gates argument value is withheld")
		}
		return unknownResult(domain.UnknownEnvironmentVisibilityGap, needed...)
	}
	switch c.State {
	case domain.StateEnabled, domain.StateDisabled:
		yes, no := on, off
		if c.State == domain.StateDisabled {
			yes, no = off, on
		}
		switch {
		case len(yes) > 0:
			r := trueResult(domain.DimensionManifests, yes...)
			r.deps = dims
			return r
		case withheld || len(unparsable) > 0:
			return undecided()
		case len(no) > 0:
			return decideFalse(evidenceOf(no))
		}
		for _, d := range dims {
			if ok, needed := ev.dimOK(d); !ok {
				r := unknownResult(domain.UnknownEnvironmentVisibilityGap, needed)
				r.deps = []domain.EnvironmentDimension{d}
				return r
			}
		}
		if len(examined) == 0 && c.Component == "" {
			// No argument or values key was ever read; the chart default may
			// still set the gate. Silence is not absence.
			return unknownResult(domain.UnknownEnvironmentVisibilityGap, "no workload container arguments (or values key) to read the feature gate from")
		}
		return decideFalse(examined)
	case domain.StateUnset:
		switch {
		case len(on)+len(off) > 0:
			return decideFalse(evidenceOf(append(on, off...)))
		case withheld || len(unparsable) > 0:
			return undecided()
		}
		for _, d := range dims {
			if ok, needed := ev.dimOK(d); !ok {
				return unknownResult(domain.UnknownEnvironmentVisibilityGap, needed)
			}
		}
		if len(examined) == 0 {
			return unknownResult(domain.UnknownEnvironmentVisibilityGap, "no workload container arguments (or values key) to read the feature gate from")
		}
		r := trueResult(domain.DimensionManifests, domain.ImpactMatch{Kind: domain.MatchAbsence, Subject: "feature gate " + c.Name + " is not set", Evidence: examined})
		r.deps = dims
		return r
	}
	return unknownResult(domain.UnknownSemanticAmbiguity, fmt.Sprintf("feature-gate state %q", c.State))
}

// --- versions ------------------------------------------------------------------------------

var (
	vPrefix  = regexp.MustCompile(`^[vV]`)
	lineOnly = regexp.MustCompile(`^\d+(\.\d+)?$`)
)

// versionIn reports whether version satisfies the semver constraint rng ("*"
// admits everything). A release line ("1.12") is decided only when every
// patch of the line gets the same verdict; otherwise ok is false — a line is
// never padded into a patch-level claim.
func versionIn(version, rng string) (in, ok bool) {
	version = vPrefix.ReplaceAllString(strings.TrimSpace(version), "")
	if strings.TrimSpace(rng) == "*" && version != "" {
		return true, true
	}
	c, err := semver.NewConstraint(rng)
	if err != nil || version == "" {
		return false, false
	}
	if lineOnly.MatchString(version) {
		lo, err1 := semver.NewVersion(version + strings.Repeat(".0", 2-strings.Count(version, ".")))
		hi, err2 := semver.NewVersion(version + strings.Repeat(".999999", 2-strings.Count(version, ".")))
		if err1 != nil || err2 != nil {
			return false, false
		}
		a, b := c.Check(lo), c.Check(hi)
		return a, a == b
	}
	v, err := semver.NewVersion(version)
	if err != nil {
		return false, false
	}
	return c.Check(v), true
}

func (ev *evaluator) productVersion(c domain.Condition) ConditionResult {
	e := ev.env
	health := e.Health(env.DimProducts)
	if health == env.HealthAbsent {
		return unknownResult(domain.UnknownCrossProductContextGap, "product inventory (--inventory) not supplied")
	}
	id := strings.ToLower(strings.TrimSpace(c.Name))
	all := e.ProductInstances(id)
	chk := domain.ImpactCheck{Dimension: domain.DimensionProducts, Facts: len(e.Products), Subjects: []string{describe(c)}}
	if len(all) == 0 {
		if e.InventoryComplete && health == env.HealthOK {
			chk.Evidence = e.InventoryCompleteEvidence
			return falseResult(domain.DimensionProducts, chk, e.InventoryCompleteEvidence...)
		}
		return unknownResult(domain.UnknownCrossProductContextGap, fmt.Sprintf(
			"%s is not listed in the product inventory, which is not declared complete: absence from a detected or partial inventory is not proof of absence (declare `complete: true` in the inventory file when it lists everything)", id))
	}
	entry := func(p env.ProductInstance) domain.ImpactMatch {
		v := p.RawVersion
		if v == "" {
			v = "(no version)"
		}
		return domain.ImpactMatch{Kind: domain.MatchProduct, Subject: fmt.Sprintf("%s %s (%s, %s version)", p.Product, v, p.Source, p.VersionOf), Evidence: p.Evidence}
	}
	if strings.TrimSpace(c.Range) == "*" {
		var ms []domain.ImpactMatch
		var evs []domain.EvidenceID
		for _, p := range all {
			ms = append(ms, entry(p))
			evs = appendUnique(evs, p.Evidence...)
		}
		if c.State == domain.StateInRange {
			return trueResult(domain.DimensionProducts, ms...)
		}
		chk.Evidence = evs
		return ev.decideFalse(domain.DimensionProducts, chk, evs...)
	}
	var app []env.ProductInstance
	for _, p := range all {
		if p.VersionOf == env.VersionOfApp {
			app = append(app, p)
		}
	}
	verdicts := map[bool][]env.ProductInstance{}
	var undecidable []string
	for _, p := range app {
		if p.Version == "" {
			continue
		}
		in, ok := versionIn(p.Version, c.Range)
		if !ok {
			undecidable = append(undecidable, fmt.Sprintf("%s %s (%s) cannot be compared with %s at that precision", p.Product, p.RawVersion, p.Source, c.Range))
			continue
		}
		verdicts[in] = append(verdicts[in], p)
	}
	switch {
	case len(verdicts) == 2:
		var names []string
		for _, ps := range verdicts {
			for _, p := range ps {
				names = append(names, fmt.Sprintf("%s %s (%s)", p.Product, p.RawVersion, p.Source))
			}
		}
		sort.Strings(names)
		return unknownResult(domain.UnknownCrossProductContextGap, fmt.Sprintf("the inventory entries of %s disagree about %s: %s", id, c.Range, strings.Join(names, " vs ")))
	case len(verdicts) == 0:
		needed := undecidable
		if len(needed) == 0 {
			needed = []string{fmt.Sprintf("%s is listed without an application version (only a chart/revision version or none); a chart version is never read as the app version — declare the app version in the inventory", id)}
		}
		return unknownResult(domain.UnknownCrossProductContextGap, needed...)
	case len(undecidable) > 0:
		return unknownResult(domain.UnknownCrossProductContextGap, undecidable...)
	}
	var in bool
	var ps []env.ProductInstance
	for k, v := range verdicts {
		in, ps = k, v
	}
	var ms []domain.ImpactMatch
	var evs []domain.EvidenceID
	for _, p := range ps {
		ms = append(ms, entry(p))
		evs = appendUnique(evs, p.Evidence...)
	}
	if in == (c.State == domain.StateInRange) {
		return trueResult(domain.DimensionProducts, ms...)
	}
	chk.Evidence = evs
	return ev.decideFalse(domain.DimensionProducts, chk, evs...)
}

func (ev *evaluator) clusterVersion(c domain.Condition) ConditionResult {
	if !strings.EqualFold(c.Name, "kubernetes") {
		return unknownResult(domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("%s cluster version not supplied (no input collects it; ri impact supports --kubernetes)", c.Name))
	}
	k := ev.env.Kubernetes
	if k == nil {
		return unknownResult(domain.UnknownEnvironmentVisibilityGap, "Kubernetes cluster version (--kubernetes) not supplied")
	}
	in, ok := versionIn(k.Version, c.Range)
	if !ok {
		return unknownResult(domain.UnknownEnvironmentVisibilityGap, fmt.Sprintf("cluster version %s cannot be compared with %s at that precision (supply the full version)", k.Version, c.Range))
	}
	if in == (c.State == domain.StateInRange) {
		return trueResult(domain.DimensionCluster, domain.ImpactMatch{Kind: domain.MatchKubernetes, Subject: k.Version, Evidence: k.Evidence})
	}
	return falseResult(domain.DimensionCluster, ev.check(domain.DimensionCluster, 1, c, k.Evidence...), k.Evidence...)
}

// FromVersionEvidence is the input record of the version the environment runs
// today (the `<from>` argument of ri impact).
func FromVersionEvidence(edge *domain.UpgradeEdge) domain.Evidence {
	v := edge.From.String()
	return domain.NewEvidence(domain.EvidenceInput, "", "arg:ri impact <from>", "", string(edge.Product.ID)+" "+v,
		domain.Digest([]byte("from:"+string(edge.Product.ID)+":"+v)), time.Time{})
}

func (ev *evaluator) edgeFromVersion(c domain.Condition) ConditionResult {
	if ev.edge == nil || ev.edge.From.IsZero() {
		return unknownResult(domain.UnknownEvidenceGap, "the upgrade's from-version is not known")
	}
	rec := FromVersionEvidence(ev.edge)
	from := ev.edge.From.Semver
	if from == "" {
		from = ev.edge.From.String()
	}
	in, ok := versionIn(from, c.Range)
	if !ok {
		return unknownResult(domain.UnknownEvidenceGap, fmt.Sprintf("from-version %s cannot be compared with %s", ev.edge.From.String(), c.Range))
	}
	var r ConditionResult
	if in == (c.State == domain.StateInRange) {
		r = trueResult(domain.DimensionFromVersion, domain.ImpactMatch{Kind: domain.MatchFromVersion, Subject: ev.edge.From.String(), Evidence: []domain.EvidenceID{rec.ID}})
	} else {
		r = falseResult(domain.DimensionFromVersion, ev.check(domain.DimensionFromVersion, 1, c, rec.ID), rec.ID)
	}
	r.Records = []domain.Evidence{rec}
	return r
}

// --- rendering ------------------------------------------------------------------------------

// describe renders a condition node for checks and needed-lists
// ("resource cert-manager.io Certificate [field spec.privateKey.rotationPolicy unset]").
func describe(c domain.Condition) string {
	var parts []string
	add := func(k, v string) {
		if v != "" {
			parts = append(parts, k+v)
		}
	}
	add("", c.Group)
	add("", c.Version)
	add("", c.Kind)
	add("", c.Name)
	add("", c.Path)
	add("component=", c.Component)
	add("", string(c.State))
	if len(c.Values) > 0 {
		parts = append(parts, strings.Join(c.Values, "|"))
	}
	add("/", c.Pattern)
	if c.Pattern != "" {
		parts[len(parts)-1] += "/"
	}
	add("", c.Range)
	s := string(c.Op)
	if len(parts) > 0 {
		s += " " + strings.Join(parts, " ")
	}
	if len(c.Of) > 0 {
		subs := make([]string, len(c.Of))
		for i, o := range c.Of {
			subs[i] = describe(o)
		}
		s += " [" + strings.Join(subs, "; ") + "]"
	}
	return s
}

func dedupMatches(ms []domain.ImpactMatch) []domain.ImpactMatch {
	var out []domain.ImpactMatch
	for _, m := range ms {
		out = appendUniqueMatches(out, m)
	}
	return out
}
