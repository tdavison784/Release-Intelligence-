package validate

import (
	"fmt"
	"sort"
	"strings"

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
	// setting (or a value in V) falsifies the exposure.
	RulePinExposure = "pin:knowledge-exposure@v1"
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
	// source
	fact         *domain.VerifiedFact
	factArtifact string
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
	return fmt.Sprintf("%s %s on every %s", s.kind, s.path, sel)
}

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
func specsFromFact(f domain.VerifiedFact, artifact string) ([]pinSpec, string) {
	a := f.Assertion
	var specs []pinSpec
	var notes []string
	if a.Subject != nil && a.Change != nil && a.Change.Type == domain.ChangeKindDefaultChanged {
		s, why := specFromSubject(*a.Subject)
		if why != "" {
			notes = append(notes, why)
		} else {
			s.rule, s.before = RulePinDefaultChanged, a.Change.Before
			specs = append(specs, s)
		}
	}
	if a.Applicability != nil {
		for _, s := range specsFromExposure(a.Applicability.Exposure) {
			s.rule = RulePinExposure
			specs = append(specs, s)
		}
	}
	for i := range specs {
		specs[i].fact, specs[i].factArtifact = &f, artifact
	}
	if len(specs) == 0 && len(notes) == 0 {
		notes = append(notes, "neither a default change of a settable subject nor an exposure leaf an explicit setting falsifies")
	}
	return specs, strings.Join(notes, "; ")
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
// unset (→ set) or not-equals V (→ one of V), at the top level or directly
// inside a resource scope.
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
			for _, y := range x.Of {
				if y.Op == domain.OpField {
					leaf(y.State, y.Values, objectSpec(x.Group, x.Kind, x.Name, y.Path))
				}
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

// --- evaluation -----------------------------------------------------------------

// pinHit is one environment record that pins the setting.
type pinHit struct {
	object   string // render object id
	path     string // where in the object
	value    string // JSON-encoded
	rendered bool   // found in an environment render (to side)
	evidence domain.Evidence
	why      string
}

// pinResult is the evaluation of one spec against the view.
type pinResult struct {
	held      bool
	hits      []pinHit
	misses    []string // records that do not pin it
	undecided string   // why the spec cannot be decided
	inputs    []string
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
	res := pinResult{held: true, inputs: v.inputs(true)}
	seen := 0
	check := func(src objSource, o render.Object, fromVal string, fromOK, rendered bool, kind domain.EvidenceKind, rp func(obj, path, after string) *domain.RenderProvenance) {
		seen++
		where := o.ID.String() + " " + s.path
		vals, missing := lookup(o.Body, path)
		if missing > 0 || len(vals) == 0 {
			res.held = false
			res.misses = append(res.misses, fmt.Sprintf("%s (%s) leaves %s unset", o.ID, src.artifact.Path, s.path))
			return
		}
		for _, val := range vals {
			fv, fok := fromVal, fromOK
			if !rendered {
				fv, fok = val, true // an environment file states the same value before and after
			}
			ok, why := s.accept(val, fv, fok)
			if !ok {
				res.held = false
				res.misses = append(res.misses, fmt.Sprintf("%s (%s): %s", o.ID, src.artifact.Path, why))
				continue
			}
			var prov *domain.RenderProvenance
			if rp != nil {
				prov = rp(o.ID.String(), s.path, val)
			}
			res.hits = append(res.hits, pinHit{object: o.ID.String(), path: s.path, value: val, rendered: rendered, why: why,
				evidence: artifactEvidence(kind, src.artifact, where, fmt.Sprintf("%s: %s", s.path, plainValue(val)), prov)})
		}
	}
	for _, f := range v.files {
		for _, o := range f.objects {
			if s.selects(o) {
				check(f, o, "", false, false, domain.EvidenceLocalFile, nil)
			}
		}
	}
	for _, r := range v.renders {
		for _, o := range r.to.objects {
			if !s.selects(o) {
				continue
			}
			fromVal, fromOK := "", false
			if fo, ok := r.fromObject(o.ID); ok {
				if vals, missing := lookup(fo.Body, path); missing == 0 && len(vals) == 1 {
					fromVal, fromOK = vals[0], true
				}
			}
			check(r.to, o, fromVal, fromOK, true, domain.EvidenceRendered, envRender)
		}
	}
	if seen == 0 {
		return pinResult{undecided: fmt.Sprintf("no %s object is visible in the environment files or renders; a setting that is absent is never a pin", s.objKind)}
	}
	return res
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
		where := "environment file"
		if h.rendered {
			where = "environment render"
		}
		fmt.Fprintf(&b, "- %s %s (%s): %s\n", h.object, h.path, where, h.why)
	}
	return b.String()
}
