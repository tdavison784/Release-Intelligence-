package validate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/applicability"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/render"
)

// Pin-rule derivations. A pin rule is release-level, declarative knowledge
// "an explicit setting S pins the old behaviour of change item C". It comes
// from the upstream change record (through the deterministic join) or from a
// trusted verified fact — never from product code, never from a model, never
// from an evaluation case. The rule id names the derivation; the record cites
// the source (finding / fact and its upstream evidence) and the environment
// evidence that states the setting.
const (
	// RulePinJoin: the join's own pinned verdict on a computed default change
	// (impact:values-pinned, impact:crd-default-pinned) — the change record
	// says the default of S changed, the join found every relevant
	// environment record setting S explicitly.
	RulePinJoin = "pin:join-pinned@v1"
	// RulePinDefaultChanged: a trusted fact asserts change.type
	// default-changed of a settable subject; an explicit setting of the
	// subject keeps the default from applying. The value must equal the old
	// default, or be stated identically before and after the upgrade.
	RulePinDefaultChanged = "pin:knowledge-default-changed@v1"
	// RulePinExposure: a trusted fact's exposure condition is a conjunction
	// with a settable leaf in state unset (or not-equals V); the explicit
	// setting (or a value in V) falsifies the exposure. A leaf inside a
	// resource scope keeps the scope: only the objects the scope's other
	// conditions do not provably exclude must carry the setting.
	RulePinExposure = "pin:knowledge-exposure@v2"
)

// Reason codes of an override inspection that establishes no pin (the
// not-run record's provenance rule).
const (
	// ReasonPinNoRule: no declarative pin rule names a setting for the item.
	ReasonPinNoRule = "pin:no-rule"
	// ReasonPinNotEstablished: a rule names a setting, but it does not hold
	// everywhere it matters.
	ReasonPinNotEstablished = "pin:not-established"
	// ReasonPinApplicabilityUndecidable: a rule's setting is missing only on
	// objects whose membership in the rule's scope cannot be decided.
	ReasonPinApplicabilityUndecidable = "pin:applicability-undecidable"
	// ReasonPinContradictingFacts: two active facts about the subject of a
	// pin-source fact state different changes or applicability.
	ReasonPinContradictingFacts = "pin:contradicting-facts"
)

// Join rules whose verdict is "the environment pins the changed default".
const (
	joinValuesPinned     = "impact:values-pinned"
	joinCRDDefaultPinned = "impact:crd-default-pinned"
)

// pinSpec is one setting to look for.
type pinSpec struct {
	rule string
	kind domain.PinKind
	// container settings (flag, env-var, feature-gate): the subject as the
	// render inventory understands it.
	container *domain.Subject
	// object settings (field, annotation, config-key): which objects, which
	// path.
	group, objKind, name string
	path                 string
	// acceptance: before = the old default (default-changed); equals = the
	// values that pin (exposure not-equals); neither = any explicit value
	// stated identically before and after.
	before *string
	equals []string
	// scope: the resource scope an exposure leaf sits in (nil: every
	// selected object must carry the setting).
	scope *pinScope
	// source
	fact         *domain.VerifiedFact
	factArtifact string
}

// pinScope is a resource scope of an exposure condition and the operand
// index path of the pin leaf inside it.
type pinScope struct {
	cond domain.Condition
	leaf []int
}

func (s pinSpec) setting() string {
	switch {
	case s.container != nil && s.container.Family == domain.SubjectFeatureGate:
		return "feature gate " + s.container.Name + componentOf(s.container.Component)
	case s.container != nil:
		return fmt.Sprintf("%s %s%s", s.kind, s.container.Name, componentOf(s.container.Component))
	case s.kind == domain.PinConfigKey:
		return fmt.Sprintf("config-key %s of ConfigMap %s", s.path, orAny(s.name))
	}
	g := s.group
	if g == "" {
		g = "core"
	}
	sel := g + " " + s.objKind
	if s.name != "" {
		sel += " " + s.name
	}
	if s.scope != nil && !s.scope.trivial() {
		return fmt.Sprintf("%s %s on every %s in the rule's scope (%s)", s.kind, s.path, sel, describeCondition(s.scope.cond))
	}
	return fmt.Sprintf("%s %s on every %s", s.kind, s.path, sel)
}

// trivial: the scope's only condition is the pin leaf (every object of the
// kind is in scope).
func (p *pinScope) trivial() bool { return len(p.cond.Of) == 1 && len(p.leaf) == 1 }

func (s pinSpec) acceptance() string {
	switch {
	case len(s.equals) > 0:
		return "set to one of " + strings.Join(s.equals, ", ")
	case s.before != nil:
		return "set explicitly to the old default " + *s.before + " (or to a value stated identically before and after the upgrade)"
	}
	return "set explicitly, identically before and after the upgrade"
}

func componentOf(c string) string {
	if c == "" {
		return ""
	}
	return " (component " + c + ")"
}

func orAny(s string) string {
	if s == "" {
		return "(any)"
	}
	return s
}

// specsFromFact derives the pin specs a trusted fact supports, with a note
// when it supports none.
//
// A fact that states an applicability is decided by it: its exposure leaves
// are the specs (a leaf on the subject's own setting also accepts the old
// default of a default change), and the default change alone is never a
// pin — the fact may say that objects are exposed whatever the setting. A
// fact without applicability pins through its default change.
func specsFromFact(f domain.VerifiedFact, artifact string) ([]pinSpec, string) {
	a := f.Assertion
	var specs []pinSpec
	var notes []string
	var def *pinSpec
	if a.Subject != nil && a.Change != nil && a.Change.Type == domain.ChangeKindDefaultChanged {
		s, why := specFromSubject(*a.Subject)
		if why != "" {
			notes = append(notes, why)
		} else {
			s.rule, s.before = RulePinDefaultChanged, a.Change.Before
			def = &s
		}
	}
	if a.Applicability != nil {
		if err := a.Applicability.Exposure.Validate(); err != nil {
			return nil, "exposure condition is not well-formed: " + err.Error()
		}
		for _, s := range specsFromExposure(a.Applicability.Exposure) {
			s.rule = RulePinExposure
			if def != nil && len(s.equals) == 0 && sameSetting(s, *def) {
				s.before = def.before
			}
			specs = append(specs, s)
		}
		if len(specs) == 0 && def != nil {
			notes = append(notes, "the fact's exposure condition names no setting whose explicit value falsifies it, so its default change alone is no pin (the applicability decides who is exposed)")
		}
	} else if def != nil {
		specs = append(specs, *def)
	}
	for i := range specs {
		specs[i].fact, specs[i].factArtifact = &f, artifact
	}
	if len(specs) == 0 && len(notes) == 0 {
		notes = append(notes, "neither a default change of a settable subject nor an exposure leaf an explicit setting falsifies")
	}
	return specs, strings.Join(notes, "; ")
}

// sameSetting reports whether two specs name the same setting.
func sameSetting(a, b pinSpec) bool {
	if a.kind != b.kind {
		return false
	}
	if a.container != nil || b.container != nil {
		return a.container != nil && b.container != nil && a.container.Name == b.container.Name && a.container.Component == b.container.Component
	}
	return a.objKind == b.objKind && a.group == b.group && a.path == b.path
}

// specFromSubject maps a settable subject to a spec.
func specFromSubject(s domain.Subject) (pinSpec, string) {
	switch s.Family {
	case domain.SubjectCRDField:
		if s.Kind == "" || s.Path == "" {
			return pinSpec{}, "crd-field subject without kind and path"
		}
		return objectSpec(s.Group, s.Kind, s.Name, s.Path), ""
	case domain.SubjectCLIFlag:
		sub := s
		return pinSpec{kind: domain.PinFlag, container: &sub}, ""
	case domain.SubjectEnvVar:
		sub := s
		return pinSpec{kind: domain.PinEnvVar, container: &sub}, ""
	case domain.SubjectFeatureGate:
		sub := s
		return pinSpec{kind: domain.PinFlag, container: &sub}, ""
	case domain.SubjectConfigKey:
		key := s.Path
		if key == "" {
			key = s.Name
		}
		if key == "" {
			return pinSpec{}, "config-key subject without a key"
		}
		return pinSpec{kind: domain.PinConfigKey, objKind: "ConfigMap", name: s.Component, path: key}, ""
	case domain.SubjectHelmValue:
		return pinSpec{}, "a values default change is decided by the render and the join's values verdicts (impact:values-pinned)"
	}
	return pinSpec{}, fmt.Sprintf("subject family %s names no setting an environment can pin", s.Family)
}

func objectSpec(group, kind, name, path string) pinSpec {
	s := pinSpec{kind: domain.PinField, group: group, objKind: kind, name: name, path: path}
	if strings.HasPrefix(path, "metadata.annotations.") {
		s.kind = domain.PinAnnotation
	}
	if kind == "ConfigMap" && group == "" && strings.HasPrefix(path, "data") {
		s.kind = domain.PinConfigKey
		s.path = strings.TrimPrefix(strings.TrimPrefix(path, "data"), ".")
		s.path = strings.TrimSuffix(strings.TrimPrefix(s.path, `["`), `"]`)
	}
	return s
}

// specsFromExposure finds the settable leaves of an exposure conjunction
// whose explicit complement falsifies the whole exposure: a leaf in state
// unset (→ set) or not-equals V (→ one of V), at the top level or inside a
// resource scope (directly, or under all/any — never under not or ref, where
// setting it could not falsify the scope). A scoped leaf keeps its scope.
func specsFromExposure(c domain.Condition) []pinSpec {
	conj := []domain.Condition{c}
	if c.Op == domain.OpAll {
		conj = c.Of
	}
	var out []pinSpec
	leaf := func(state domain.FieldState, values []string, s pinSpec) {
		switch state {
		case domain.StateUnset:
			out = append(out, s)
		case domain.StateNotEquals:
			s.equals = append([]string(nil), values...)
			out = append(out, s)
		}
	}
	for _, x := range conj {
		switch x.Op {
		case domain.OpResource:
			for _, at := range pinLeaves(x.Of, nil) {
				y := leafAt(x, at)
				s := objectSpec(x.Group, x.Kind, x.Name, y.Path)
				s.scope = &pinScope{cond: x, leaf: at}
				leaf(y.State, y.Values, s)
			}
		case domain.OpCLIFlag:
			leaf(x.State, x.Values, pinSpec{kind: domain.PinFlag, container: &domain.Subject{Family: domain.SubjectCLIFlag, Name: x.Name, Component: x.Component}})
		case domain.OpEnvVar:
			leaf(x.State, x.Values, pinSpec{kind: domain.PinEnvVar, container: &domain.Subject{Family: domain.SubjectEnvVar, Name: x.Name, Component: x.Component}})
		case domain.OpFeatureGate:
			if x.State == domain.StateUnset {
				out = append(out, pinSpec{kind: domain.PinFlag, container: &domain.Subject{Family: domain.SubjectFeatureGate, Name: x.Name, Component: x.Component}})
			}
		}
	}
	return out
}

// pinLeaves returns the operand index paths of the field leaves in state
// unset / not-equals reachable from ops through all/any only.
func pinLeaves(ops []domain.Condition, prefix []int) [][]int {
	var out [][]int
	for i, c := range ops {
		at := append(append([]int(nil), prefix...), i)
		switch {
		case c.Op == domain.OpField && (c.State == domain.StateUnset || c.State == domain.StateNotEquals):
			out = append(out, at)
		case c.Op == domain.OpAll || c.Op == domain.OpAny:
			out = append(out, pinLeaves(c.Of, at)...)
		}
	}
	return out
}

func leafAt(c domain.Condition, at []int) domain.Condition {
	for _, i := range at {
		c = c.Of[i]
	}
	return c
}

// --- evaluation -----------------------------------------------------------------

// pinHit is one environment record that pins the setting (or, in
// pinResult.excluded, an object provably outside the rule's scope).
type pinHit struct {
	object   string // render object id
	path     string // where in the object
	value    string // JSON-encoded
	rendered bool   // found in an environment render (to side, or a version-independent overlay)
	view     string // which view of the environment the object came from
	evidence domain.Evidence
	why      string
}

// pinResult is the evaluation of one spec against the view.
type pinResult struct {
	held   bool
	hits   []pinHit
	misses []string // records that do not pin it
	// undecidable: objects whose scope membership (or effective state)
	// cannot be decided and that do not pin it.
	undecidable []string
	// excluded: objects provably outside the rule's scope.
	excluded []pinHit
	// schemaEvidence: the CRD defaults the scope decisions used.
	schemaEvidence []domain.Evidence
	undecided      string // why the spec cannot be decided
	inputs         []string
	replaced       []string // environment files seen through the renders that consumed them
}

// onlyUndecidable: the spec failed only on objects whose scope membership
// could not be decided.
func (r pinResult) onlyUndecidable() bool {
	return r.undecided == "" && !r.held && len(r.misses) == 0 && len(r.undecidable) > 0
}

func (s pinSpec) evaluate(v *envView) pinResult {
	if !v.complete {
		return pinResult{undecided: "needs every environment render complete to see every place the setting must hold — " + incompleteDetail(v.pairs)}
	}
	if s.container != nil {
		return s.evalContainers(v)
	}
	return s.evalObjects(v)
}

// accept decides whether a value pins the behaviour. fromVal/fromOK is the
// same record before the upgrade (files state the same value on both sides).
func (s pinSpec) accept(to, fromVal string, fromOK bool) (bool, string) {
	switch {
	case len(s.equals) > 0:
		for _, e := range s.equals {
			if sameValue(to, e) {
				return true, fmt.Sprintf("set to %s, a value the exposure condition excludes", to)
			}
		}
		return false, fmt.Sprintf("set to %s, not one of the pinning values %s", to, strings.Join(s.equals, ", "))
	case s.before != nil && sameValue(to, *s.before):
		return true, fmt.Sprintf("set to %s, the old default", to)
	case fromOK && sameValue(fromVal, to):
		return true, fmt.Sprintf("set to %s on both sides of the upgrade", to)
	case s.before != nil:
		if fromOK {
			return false, fmt.Sprintf("set to %s after the upgrade but %s before it", to, fromVal)
		}
		return false, fmt.Sprintf("set to %s only after the upgrade, which is not the old default %s: the behaviour changes", to, *s.before)
	case fromOK:
		return false, fmt.Sprintf("set to %s after the upgrade but %s before it", to, fromVal)
	}
	return false, fmt.Sprintf("set to %s only after the upgrade", to)
}

func (s pinSpec) evalContainers(v *envView) pinResult {
	res := pinResult{held: true, inputs: v.inputs(false)}
	for _, r := range v.renders {
		occ, _ := render.Inventory(r.to.objects, *s.container)
		if len(occ) == 0 {
			res.held = false
			res.misses = append(res.misses, fmt.Sprintf("render %s: the target render does not set %s", r.pair.Target, s.setting()))
			continue
		}
		from, _ := render.Inventory(r.from.objects, *s.container)
		for _, o := range occ {
			to := containerValue(s.container.Family, o.Value)
			fv, fok := "", false
			for _, f := range from {
				if sameObject(f.Object, o.Object) && f.Path == o.Path {
					fv, fok = containerValue(s.container.Family, f.Value), true
				}
			}
			ok, why := s.accept(to, fv, fok)
			where := o.Object.String() + " " + o.Path
			if !ok {
				res.held = false
				res.misses = append(res.misses, fmt.Sprintf("render %s: %s %s", r.pair.Target, where, why))
				continue
			}
			res.hits = append(res.hits, pinHit{object: o.Object.String(), path: o.Path, value: to, rendered: true, why: why,
				evidence: artifactEvidence(domain.EvidenceRendered, r.to.artifact, where,
					fmt.Sprintf("%s: %s = %s", where, s.container.Name, plainValue(to)), envRender(o.Object.String(), o.Path, to))})
		}
	}
	if len(res.hits) == 0 {
		res.held = false
	}
	return res
}

func containerValue(f domain.SubjectFamily, v string) string {
	if f == domain.SubjectCLIFlag && v == "" {
		return "true" // a bare flag
	}
	return encodeValue(v)
}

func sameObject(a, b render.ObjectID) bool {
	return a.Group == b.Group && a.Kind == b.Kind && a.Namespace == b.Namespace && a.Name == b.Name
}

func (s pinSpec) selects(o render.Object) bool {
	if o.ID.Kind != s.objKind || (s.group != "" && o.ID.Group != s.group) {
		return false
	}
	return s.name == "" || o.ID.Name == s.name || strings.HasSuffix(o.ID.Name, "-"+s.name)
}

func (s pinSpec) evalObjects(v *envView) pinResult {
	if len(v.gaps) > 0 {
		return pinResult{undecided: "environment not collected completely: " + strings.Join(v.gaps, "; ")}
	}
	if files := v.mentionsKind(s.objKind); len(files) > 0 {
		return pinResult{undecided: fmt.Sprintf("environment files that could not be parsed may declare %s objects: %s", s.objKind, strings.Join(files, ", "))}
	}
	path := s.path
	if s.kind == domain.PinConfigKey {
		path = `data["` + s.path + `"]`
	}
	res := pinResult{held: true, inputs: v.inputs(true), replaced: v.replaced()}
	objs := v.objects()
	sc := newScopeEval(objs, v.schemas, s.fact)
	seen := 0
	for _, e := range objs {
		if !s.selects(e.obj) {
			continue
		}
		seen++
		o, src := e.obj, e.src
		at := fmt.Sprintf("%s (%s)", o.ID, src.label())
		if e.unrendered != "" {
			res.held = false
			res.undecidable = append(res.undecidable, fmt.Sprintf("%s: %s", at, e.unrendered))
			continue
		}
		hits, miss := s.pinsOn(e, path)
		if s.scope != nil {
			full := sc.decide(s.scope, e, miss == "")
			res.schemaEvidence = append(res.schemaEvidence, full.evidence...)
			switch {
			case full.t == tUnknown:
				res.held = false
				reason := full.why
				if miss != "" {
					reason = miss + ", and whether it is in the rule's scope cannot be decided: " + full.why
				}
				res.undecidable = append(res.undecidable, fmt.Sprintf("%s: %s", at, reason))
				continue
			case full.t == tTrue:
				res.held = false
				if miss == "" {
					miss = "pinned, but in the rule's scope through another condition: " + full.why
				}
				res.misses = append(res.misses, fmt.Sprintf("%s: %s", at, miss))
				continue
			case miss != "":
				// false although the leaf does not pin: provably out of scope
				res.excluded = append(res.excluded, pinHit{object: o.ID.String(), rendered: e.rendered, view: src.label(), why: full.why,
					evidence: objectEvidence(e, o.ID.String(), "out of the rule's scope: "+full.why)})
				continue
			}
		}
		if miss != "" {
			res.held = false
			res.misses = append(res.misses, fmt.Sprintf("%s: %s", at, miss))
			continue
		}
		res.hits = append(res.hits, hits...)
	}
	if seen == 0 {
		return pinResult{undecided: fmt.Sprintf("no %s object is visible in the environment files or renders; a setting that is absent is never a pin", s.objKind)}
	}
	if res.held && len(res.hits) == 0 {
		res.held = false
		res.misses = append(res.misses, fmt.Sprintf("every visible %s object is outside the rule's scope: no setting pins anything (whether the environment is exposed at all is the join's decision, not a pin)", s.objKind))
	}
	return res
}

// pinsOn evaluates the setting on one object of the effective view: the
// pinning records, or why the object does not pin it.
func (s pinSpec) pinsOn(e effObj, path string) ([]pinHit, string) {
	o := e.obj
	vals, missing := lookup(o.Body, path)
	if missing > 0 || len(vals) == 0 {
		return nil, "leaves " + s.path + " unset"
	}
	var hits []pinHit
	for _, val := range vals {
		fv, fok := "", false
		switch {
		case e.sameSides:
			fv, fok = val, true // an environment file / overlay states the same value before and after
		case e.from != nil:
			if fvals, fmissing := lookup(e.from.Body, path); fmissing == 0 && len(fvals) == 1 {
				fv, fok = fvals[0], true
			}
		}
		ok, why := s.accept(val, fv, fok)
		if !ok {
			return nil, why
		}
		where := o.ID.String() + " " + s.path
		why += e.src.readBackNote(o.ID, s.path)
		hits = append(hits, pinHit{object: o.ID.String(), path: s.path, value: val, rendered: e.rendered, view: e.src.label(), why: why,
			evidence: objectEvidence(e, where, fmt.Sprintf("%s: %s%s", s.path, plainValue(val), e.src.readBackNote(o.ID, s.path)), s.path, val)})
	}
	return hits, ""
}

// objectEvidence cites an object of the effective view (with render
// provenance for rendered objects).
func objectEvidence(e effObj, locator, excerpt string, pathValue ...string) domain.Evidence {
	if !e.rendered {
		return artifactEvidence(domain.EvidenceLocalFile, e.src.artifact, locator, excerpt, nil)
	}
	var prov *domain.RenderProvenance
	if len(pathValue) == 2 {
		prov = envRender(e.obj.ID.String(), pathValue[0], pathValue[1])
	} else {
		prov = &domain.RenderProvenance{Scope: domain.RenderEnvironment, Tool: "ri:render", Object: e.obj.ID.String()}
	}
	return artifactEvidence(domain.EvidenceRendered, e.src.artifact, locator, excerpt, prov)
}

// pinOf turns the hits of a held spec into the PinnedSetting the record
// carries: the first pinning record (sorted) names subject, path and value;
// the evidence covers every pinning record.
func pinOf(kind domain.PinKind, path string, hits []pinHit) *domain.PinnedSetting {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].object != hits[j].object {
			return hits[i].object < hits[j].object
		}
		return hits[i].path < hits[j].path
	})
	p := &domain.PinnedSetting{Kind: kind, Subject: hits[0].object, Path: path, Value: hits[0].value}
	for _, h := range hits {
		p.Evidence = append(p.Evidence, h.evidence.ID)
		if h.rendered && p.RenderedObject == "" {
			p.RenderedObject = h.object
		}
	}
	p.Evidence = uniqEvidence(p.Evidence)
	return p
}

func describeHits(hits []pinHit) string {
	var b strings.Builder
	for _, h := range hits {
		where := h.view
		if where == "" {
			where = "environment file"
			if h.rendered {
				where = "environment render"
			}
		}
		fmt.Fprintf(&b, "- %s %s (%s): %s\n", h.object, h.path, where, h.why)
	}
	return b.String()
}

// describeScope lists the objects a scoped rule excluded and the files the
// view saw through renders.
func describeScope(res pinResult) string {
	var b strings.Builder
	if len(res.excluded) > 0 {
		b.WriteString("Out of the rule's scope (no setting required):\n")
		for _, h := range res.excluded {
			fmt.Fprintf(&b, "- %s (%s): %s\n", h.object, h.view, h.why)
		}
	}
	if len(res.replaced) > 0 {
		b.WriteString("Environment files seen through the renders that consumed them (effective object view):\n")
		for _, r := range res.replaced {
			fmt.Fprintf(&b, "- %s\n", r)
		}
	}
	return b.String()
}

// CanBePinSource reports whether a, once verified at a trusted level, would
// give OverrideInspection at least one setting to look for (a default change of
// a settable subject, or an exposure an explicit setting falsifies). The
// review UI uses it to order open questions by what they would unblock
// (CONTRACT-CHANGE(reviewfilter): additive export; same derivation the pin
// rules use, so the two cannot drift).
func CanBePinSource(a domain.SemanticAssertion) bool {
	specs, _ := specsFromFact(domain.VerifiedFact{Assertion: a}, "")
	return len(specs) > 0
}

// --- scope: the applicability condition, decided per object ---------------------

// truth is a three-valued (Kleene) truth value.
type truth int8

const (
	tFalse truth = iota
	tTrue
	tUnknown
)

// verdict is the value of a scope condition on one object.
type verdict struct {
	t   truth
	why string
	// evidence: the CRD schema defaults the value rests on.
	evidence []domain.Evidence
}

// scopeEval decides a pin rule's resource scope against single objects of
// the effective view, with the semantics of the impact join's condition
// evaluator (internal/impact): field states over the values the object
// states, Kleene all/any/not, a ref resolving to exactly one object. A value
// predicate on a field the object leaves unset is decided by the rule shared
// with the join (internal/applicability): the schema default the bundle's
// CRDs state (inside a present parent), or the rule's own default when its
// citations resolve to the fact's upstream evidence, and undecidable
// otherwise. One deliberate difference makes it fail closed for exclusion: a
// reference that resolves to nothing is undecidable (the view is never
// declared complete).
type scopeEval struct {
	objs    []effObj
	schemas []effObj
	// crds are the schemas in the shared helper's form (index i ↔
	// schemas[i]); rule is the fact's evidence (condition defaults cite it).
	crds []applicability.CRD
	rule map[domain.EvidenceID]domain.Evidence
	// observed are the fact's usable observed defaults (PD-5); membership
	// its usable role-membership defaults (PD-6).
	observed   []domain.ObservedDefault
	membership []domain.RoleMembershipDefault
}

func newScopeEval(objs, schemas []effObj, fact *domain.VerifiedFact) *scopeEval {
	sc := &scopeEval{objs: objs, schemas: schemas, rule: map[domain.EvidenceID]domain.Evidence{}}
	for _, s := range schemas {
		c := applicability.CRD{Group: yStr(s.obj.Body, "spec", "group"), Kind: yStr(s.obj.Body, "spec", "names", "kind"),
			Schemas: map[string]map[string]any{}, Where: fmt.Sprintf("CRD %s in %s", s.obj.ID.Name, s.src.label())}
		for _, ver := range ySeq(yGet(s.obj.Body, "spec", "versions")) {
			if m := yMap(yGet(ver, "schema", "openAPIV3Schema")); m != nil {
				c.Schemas[yStr(ver, "name")] = m
			} else {
				c.Schemas[yStr(ver, "name")] = nil
			}
		}
		sc.crds = append(sc.crds, c)
	}
	if fact != nil {
		for _, e := range fact.Evidence {
			sc.rule[e.ID] = e
		}
		sc.observed, _ = fact.UsableObservedDefaults()
		sc.membership, _ = fact.UsableRoleMembershipDefaults()
	}
	return sc
}

// decide evaluates the scope against e, the pin leaf taking the value the
// setting gives it (pinned ⇒ the leaf is false).
func (sc *scopeEval) decide(p *pinScope, e effObj, pinned bool) verdict {
	leaf := verdict{t: tTrue, why: "the setting does not pin it"}
	if pinned {
		leaf = verdict{t: tFalse, why: "the setting pins it"}
	}
	return sc.all(p.cond.Of, e, nil, p.leaf, leaf, 0)
}

func (sc *scopeEval) eval(c domain.Condition, e effObj, at, leafAt []int, leaf verdict, depth int) verdict {
	if leafAt != nil && sameIndex(at, leafAt) {
		return leaf
	}
	if depth > domain.MaxConditionDepth {
		return verdict{t: tUnknown, why: "condition too deep"}
	}
	switch c.Op {
	case domain.OpAll:
		return sc.all(c.Of, e, at, leafAt, leaf, depth+1)
	case domain.OpAny:
		return sc.any(c.Of, e, at, leafAt, leaf, depth+1)
	case domain.OpNot:
		r := sc.eval(c.Of[0], e, append(clone(at), 0), leafAt, leaf, depth+1)
		switch r.t {
		case tTrue:
			r.t, r.why = tFalse, "not ("+r.why+")"
		case tFalse:
			r.t, r.why = tTrue, "not ("+r.why+")"
		}
		return r
	case domain.OpField:
		return sc.field(c, e)
	case domain.OpTextLine:
		return sc.textLine(c, e)
	case domain.OpRef:
		return sc.ref(c, e, depth+1)
	case domain.OpUndecidable:
		return verdict{t: tUnknown, why: fmt.Sprintf("the rule states this is not statically decidable (%s): %s", c.Reason, c.Needed)}
	}
	return verdict{t: tUnknown, why: fmt.Sprintf("%s is not decidable on a single object", c.Op)}
}

func (sc *scopeEval) all(ops []domain.Condition, e effObj, at, leafAt []int, leaf verdict, depth int) verdict {
	var unknown *verdict
	var ev []domain.Evidence
	var whys []string
	for i, c := range ops {
		r := sc.eval(c, e, append(clone(at), i), leafAt, leaf, depth)
		ev = append(ev, r.evidence...)
		switch r.t {
		case tFalse:
			return verdict{t: tFalse, why: r.why, evidence: r.evidence}
		case tUnknown:
			if unknown == nil {
				u := r
				unknown = &u
			}
		default:
			whys = append(whys, r.why)
		}
	}
	if unknown != nil {
		return verdict{t: tUnknown, why: unknown.why, evidence: ev}
	}
	return verdict{t: tTrue, why: strings.Join(whys, " and "), evidence: ev}
}

func (sc *scopeEval) any(ops []domain.Condition, e effObj, at, leafAt []int, leaf verdict, depth int) verdict {
	var unknown *verdict
	var ev []domain.Evidence
	var whys []string
	for i, c := range ops {
		r := sc.eval(c, e, append(clone(at), i), leafAt, leaf, depth)
		ev = append(ev, r.evidence...)
		switch r.t {
		case tTrue:
			return verdict{t: tTrue, why: r.why, evidence: r.evidence}
		case tUnknown:
			if unknown == nil {
				u := r
				unknown = &u
			}
		default:
			whys = append(whys, r.why)
		}
	}
	if unknown != nil {
		return verdict{t: tUnknown, why: unknown.why, evidence: ev}
	}
	return verdict{t: tFalse, why: strings.Join(whys, " and "), evidence: ev}
}

func sameIndex(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func clone(xs []int) []int { return append([]int(nil), xs...) }

// withheld: the object's values are stored redacted (Secret data), so no
// value predicate on it decides anything.
func withheld(e effObj) bool { return e.obj.ID.Kind == "Secret" && e.obj.ID.Group == "" }

func (sc *scopeEval) field(c domain.Condition, e effObj) verdict {
	vals, missing := lookupAny(e.obj.Body, c.Path)
	present := len(vals) > 0
	leaf := describeCondition(c)
	switch c.State {
	case domain.StateSet:
		if present {
			return verdict{t: tTrue, why: c.Path + " is set"}
		}
		return verdict{t: tFalse, why: c.Path + " is unset"}
	case domain.StateUnset:
		if present {
			return verdict{t: tFalse, why: c.Path + " is set"}
		}
		return verdict{t: tTrue, why: c.Path + " is unset"}
	}
	if withheld(e) {
		return verdict{t: tUnknown, why: fmt.Sprintf("%s: the object's values are withheld (redacted)", leaf)}
	}
	var stated []string
	hit := false
	for _, v := range vals {
		switch v.(type) {
		case map[string]any, []any:
			continue // a predicate on a scalar never holds for a container
		}
		if redactedValue(v) {
			return verdict{t: tUnknown, why: fmt.Sprintf("%s: %s is withheld (redacted as %v) in %s", leaf, c.Path, v, e.src.label())}
		}
		enc := encodeJSON(v)
		stated = append(stated, plainValue(enc))
		hit = hit || valueHolds(c, enc)
	}
	is := func() string {
		if len(stated) == 0 {
			return c.Path + " is unset"
		}
		return c.Path + " is " + strings.Join(stated, ", ") + e.src.readBackNote(e.obj.ID, c.Path)
	}
	// what the object leaves unset holds its established default: needed
	// when nothing stated decides it
	needDefault := !present || missing > 0
	if c.State == domain.StateNotEquals {
		switch {
		case hit:
			return verdict{t: tFalse, why: fmt.Sprintf("%s (one of %s)", is(), strings.Join(c.Values, ", "))}
		case !needDefault:
			return verdict{t: tTrue, why: fmt.Sprintf("%s (none of %s)", is(), strings.Join(c.Values, ", "))}
		}
	} else {
		switch {
		case hit:
			return verdict{t: tTrue, why: is()}
		case !needDefault:
			return verdict{t: tFalse, why: fmt.Sprintf("%s (%s)", is(), leaf+" does not hold")}
		}
	}
	d, evidence := sc.fieldDefault(c, e)
	u := applicability.Unset(c, d, func(value string) bool { return valueHolds(c, value) })
	switch {
	case !u.Known:
		return verdict{t: tUnknown, why: fmt.Sprintf("%s: %s, and %s; needed: %s", leaf, is(), u.Why, applicability.Needed(c))}
	case u.Holds:
		return verdict{t: tTrue, why: fmt.Sprintf("%s, otherwise %s", is(), u.Why), evidence: evidence}
	}
	return verdict{t: tFalse, why: fmt.Sprintf("%s, otherwise %s: %s does not hold", is(), u.Why, leaf), evidence: evidence}
}

// fieldDefault is the established default of c's field on e (schema default
// inside a present parent, combined with the rule's evidenced default) and
// the evidence of the schemas it was read from.
func (sc *scopeEval) fieldDefault(c domain.Condition, e effObj) (applicability.Default, []domain.Evidence) {
	o := e.obj
	schema := applicability.SchemaDefault(sc.crds, o.ID.Group, o.ID.Version, o.ID.Kind, c.Path)
	if parent := applicability.ParentPath(c.Path); parent != "" {
		if pv, _ := lookupAny(o.Body, parent); len(pv) == 0 {
			schema = applicability.ParentAbsent(schema, c.Path)
		}
	}
	obs, conflict := applicability.ObservedFor(sc.observed, o.ID.Group, o.ID.Kind, c.Path)
	d := applicability.WithObserved(applicability.Resolve(schema, applicability.RuleDefault(c, sc.rule)), obs, conflict)
	m, mconflict := applicability.MembershipFor(sc.membership, o.ID.Group, o.ID.Kind, c)
	d = applicability.WithMembership(d, m, mconflict)
	var evs []domain.Evidence
	if d.State == applicability.Known {
		for _, i := range d.CRDs {
			s := sc.schemas[i]
			evs = append(evs, artifactEvidence(evidenceKindOf(s), s.src.artifact, fmt.Sprintf("%s %s %s", s.obj.ID, o.ID.Version, c.Path),
				fmt.Sprintf("schema of %s/%s %s: %s default %s", o.ID.Group, o.ID.Kind, o.ID.Version, c.Path, schemaDefaultText(sc.crds[i], o.ID.Version, c.Path)), nil))
		}
	}
	return d, evs
}

func schemaDefaultText(c applicability.CRD, version, path string) string {
	v, has, _ := applicability.DefaultAt(c.Schemas[version], path)
	if !has {
		return "(none)"
	}
	return plainValue(v)
}

// valueHolds reports whether a JSON-encoded scalar satisfies the positive
// form of c's state (for not-equals: equals) — impact's semantics.
func valueHolds(c domain.Condition, value string) bool {
	switch c.State {
	case domain.StateEquals, domain.StateNotEquals:
		for _, v := range c.Values {
			if sameValue(v, value) {
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
		for _, tok := range strings.Split(plainValue(value), sep) {
			tok = strings.TrimSpace(tok)
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

func (sc *scopeEval) textLine(c domain.Condition, e effObj) verdict {
	leaf := describeCondition(c)
	if withheld(e) {
		return verdict{t: tUnknown, why: leaf + ": the object's values are withheld (redacted)"}
	}
	re, err := regexp.Compile(c.Pattern)
	if err != nil {
		return verdict{t: tUnknown, why: fmt.Sprintf("pattern %q: %v", c.Pattern, err)}
	}
	vals, _ := lookupAny(e.obj.Body, c.Path)
	matched := false
	for _, v := range vals {
		s, ok := v.(string)
		if !ok {
			continue
		}
		if redactedValue(v) {
			return verdict{t: tUnknown, why: fmt.Sprintf("%s: %s is withheld (redacted) in %s", leaf, c.Path, e.src.label())}
		}
		for _, ln := range strings.Split(s, "\n") {
			matched = matched || re.MatchString(ln)
		}
	}
	switch {
	case c.State == domain.StateExists && matched, c.State == domain.StateNone && !matched:
		return verdict{t: tTrue, why: leaf + " holds"}
	case c.State == domain.StateExists, c.State == domain.StateNone:
		return verdict{t: tFalse, why: leaf + " does not hold"}
	}
	return verdict{t: tUnknown, why: fmt.Sprintf("text-line state %q", c.State)}
}

// ref: the reference at c.Path resolves to exactly one object of the
// effective view satisfying the operands (resolution as env.ResolveRef: by
// name, the kind/group/namespace the reference states, else the referring
// object's namespace).
func (sc *scopeEval) ref(c domain.Condition, e effObj, depth int) verdict {
	vals, _ := lookupAny(e.obj.Body, c.Path)
	var refs []map[string]any
	for _, v := range vals {
		if m, ok := v.(map[string]any); ok {
			if n, _ := m["name"].(string); n != "" {
				refs = append(refs, m)
			}
		}
	}
	if len(refs) == 0 {
		return verdict{t: tFalse, why: c.Path + " references nothing"}
	}
	per := make([]verdict, 0, len(refs))
	for _, m := range refs {
		name, _ := m["name"].(string)
		kind, _ := m["kind"].(string)
		group, _ := m["group"].(string)
		if av, _ := m["apiVersion"].(string); group == "" && strings.Contains(av, "/") {
			group, _, _ = strings.Cut(av, "/")
		}
		ns, _ := m["namespace"].(string)
		var cands []effObj
		for _, t := range sc.objs {
			id := t.obj.ID
			if id.Name != name || (kind != "" && id.Kind != kind) || (group != "" && id.Group != group) {
				continue
			}
			if ns != "" && id.Namespace != ns || ns == "" && id.Namespace != "" && id.Namespace != e.obj.ID.Namespace {
				continue
			}
			cands = append(cands, t)
		}
		target := fmt.Sprintf("%s %s", orAny(kind), name)
		switch len(cands) {
		case 0:
			per = append(per, verdict{t: tUnknown, why: fmt.Sprintf("%s references %s, which is not in the environment's effective view (absence is not knowledge)", c.Path, target)})
			continue
		case 1:
		default:
			per = append(per, verdict{t: tUnknown, why: fmt.Sprintf("%s references %s, which %d objects match", c.Path, target, len(cands))})
			continue
		}
		t := cands[0]
		if t.unrendered != "" {
			per = append(per, verdict{t: tUnknown, why: fmt.Sprintf("%s references %s, whose effective state is unknown: %s", c.Path, t.obj.ID, t.unrendered)})
			continue
		}
		if (c.Kind != "" && t.obj.ID.Kind != c.Kind) || (c.Group != "" && t.obj.ID.Group != c.Group) {
			per = append(per, verdict{t: tFalse, why: fmt.Sprintf("%s references %s, not a %s", c.Path, t.obj.ID, c.Kind)})
			continue
		}
		r := sc.all(c.Of, t, nil, nil, verdict{}, depth)
		r.why = fmt.Sprintf("%s → %s: %s", c.Path, t.obj.ID, r.why)
		per = append(per, r)
	}
	var unknown *verdict
	var whys []string
	for _, r := range per {
		switch r.t {
		case tTrue:
			return r
		case tUnknown:
			if unknown == nil {
				u := r
				unknown = &u
			}
		default:
			whys = append(whys, r.why)
		}
	}
	if unknown != nil {
		return *unknown
	}
	return verdict{t: tFalse, why: strings.Join(whys, "; ")}
}

func evidenceKindOf(e effObj) domain.EvidenceKind {
	switch e.src.side {
	case "file":
		return domain.EvidenceLocalFile
	case "schema":
		return domain.EvidenceDocument
	}
	return domain.EvidenceRendered
}

// describeCondition renders a condition compactly for details.
func describeCondition(c domain.Condition) string {
	switch c.Op {
	case domain.OpAll, domain.OpAny:
		parts := make([]string, 0, len(c.Of))
		for _, x := range c.Of {
			parts = append(parts, describeCondition(x))
		}
		return string(c.Op) + "(" + strings.Join(parts, ", ") + ")"
	case domain.OpNot:
		return "not(" + describeCondition(c.Of[0]) + ")"
	case domain.OpResource:
		parts := make([]string, 0, len(c.Of))
		for _, x := range c.Of {
			parts = append(parts, describeCondition(x))
		}
		return c.Kind + " where " + strings.Join(parts, " and ")
	case domain.OpRef:
		parts := make([]string, 0, len(c.Of))
		for _, x := range c.Of {
			parts = append(parts, describeCondition(x))
		}
		return fmt.Sprintf("%s refers to a %s where %s", c.Path, orAny(c.Kind), strings.Join(parts, " and "))
	case domain.OpField:
		s := c.Path + " " + string(c.State)
		if len(c.Values) > 0 {
			s += " " + strings.Join(c.Values, "|")
		}
		if c.Pattern != "" {
			s += " /" + c.Pattern + "/"
		}
		return s
	case domain.OpTextLine:
		return fmt.Sprintf("%s line /%s/ %s", c.Path, c.Pattern, c.State)
	case domain.OpUndecidable:
		return "undecidable(" + c.Needed + ")"
	}
	return string(c.Op)
}
