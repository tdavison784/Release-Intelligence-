package render

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RenderedChangeValue is the three-valued outcome of a `rendered-change`
// leaf (DESIGN.md §1.3): true / false / unknown. The applicability lane maps
// it onto its ConditionResult.
type RenderedChangeValue string

const (
	RenderedTrue    RenderedChangeValue = "true"
	RenderedFalse   RenderedChangeValue = "false"
	RenderedUnknown RenderedChangeValue = "unknown"
)

// RenderedChangeResult is what one `rendered-change` condition evaluated to
// over the environment's From/To render pairs (the customer's values).
//
// A result of RenderedFalse is evidence: it requires every pair to have both
// renders succeeded with complete values. A failed or values-incomplete pair
// keeps the whole leaf unknown (an absence of change decides nothing). The
// evidence records are environment renders: they back impact findings, never
// knowledge or prompts (the rendered-diff validator uses release pairs).
//
// The leaf says nothing about consequences: a render difference alone never
// produces ACTION REQUIRED (RENDER-MISSION R11) — that is the trust ladder's
// job, above this predicate.
type RenderedChangeResult struct {
	Value  RenderedChangeValue `json:"value"`
	Detail string              `json:"detail,omitempty"`
	// Reason is set when Value is unknown (DESIGN.md unknownReason).
	Reason domain.UnknownReason `json:"reason,omitempty"`
	// Evidence cites the rendered-diff records a true/false rests on.
	Evidence []domain.Evidence `json:"evidence,omitempty"`
	// Targets names the render targets the condition was evaluated against.
	Targets []string `json:"targets,omitempty"`
}

// EvaluateRenderedChange evaluates one `rendered-change` condition against
// the environment's render pairs (customer configuration): does the field at
// cond.Path of the objects matching cond.Group/Kind/Name differ between the
// From and To renders the way cond.State says (changed / unchanged / added /
// removed), with the To value in cond.Values when given?
//
// cond.Path uses the `[]` pattern syntax (spec.template.spec.containers[].image)
// and also accepts the diff's keyed selectors (containers[name=controller].image);
// the diff's Change.Pattern is the same syntax, so conditions match diff
// changes exactly.
func EvaluateRenderedChange(cond domain.Condition, pairs []*Pair) RenderedChangeResult {
	res := RenderedChangeResult{Value: RenderedUnknown, Reason: domain.UnknownEnvironmentVisibilityGap}
	for _, p := range pairs {
		res.Targets = append(res.Targets, p.Target.ID)
	}
	if cond.Op != domain.OpRenderedChange {
		res.Detail = fmt.Sprintf("not a rendered-change condition (op %s)", cond.Op)
		return res
	}
	if cond.Kind == "" || cond.Path == "" {
		res.Detail = "rendered-change needs kind and path"
		return res
	}
	segs, err := parsePatternPath(cond.Path)
	if err != nil {
		res.Detail = fmt.Sprintf("path %q: %v", cond.Path, err)
		return res
	}
	var unknowns []string
	decidedFalse := false
	for _, p := range pairs {
		v, detail, ev := evaluatePair(cond, segs, p)
		switch v {
		case RenderedTrue:
			return RenderedChangeResult{Value: RenderedTrue,
				Detail:   fmt.Sprintf("%s (%s): %s", p.Target.ID, p.Target.Origin, detail),
				Targets:  res.Targets,
				Evidence: ev}
		case RenderedFalse:
			decidedFalse = true
		default:
			what := "render unavailable"
			if p.Status == PairOK && !p.Target.ValuesComplete {
				what = "values incomplete: " + p.Target.IncompleteReason
			}
			unknowns = append(unknowns, fmt.Sprintf("%s: %s (%s)", p.Target.ID, detail, what))
		}
	}
	if len(pairs) == 0 {
		res.Detail = "render unavailable: no environment renders (--render)"
		return res
	}
	if decidedFalse && len(unknowns) == 0 {
		return RenderedChangeResult{Value: RenderedFalse, Targets: res.Targets, Detail: fmt.Sprintf(
			"every complete render of %s %s shows %s, not %s", cond.Kind, cond.Path,
			strings.Join(pairStates(pairs, cond, segs), ", "), cond.State)}
	}
	res.Detail = "not decidable from the renders: " + strings.Join(unknowns, "; ")
	return res
}

// pairStates names what each deciding pair actually shows at the path (the
// false result's detail); pairs that did not decide are skipped.
func pairStates(pairs []*Pair, cond domain.Condition, segs []pathSeg) []string {
	var out []string
	for _, p := range pairs {
		if p.Status != PairOK || !p.Target.ValuesComplete {
			continue
		}
		seen := map[string]bool{}
		for _, st := range objectStates(cond, segs, p) {
			if st.state != "" {
				seen[st.state] = true
			}
		}
		if len(seen) > 0 {
			out = append(out, p.Target.ID+"="+strings.Join(sortedKeys(seen), "/"))
		}
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// evaluatePair decides the condition against one pair. unknown covers failed
// renders, incomplete values, absent objects and paths absent in both renders.
func evaluatePair(cond domain.Condition, segs []pathSeg, p *Pair) (RenderedChangeValue, string, []domain.Evidence) {
	if p.Status != PairOK {
		why := "render failed"
		if p.Failure != nil {
			why = fmt.Sprintf("render %s: %s", p.Failure.Reason, p.Failure.Detail)
		}
		return RenderedUnknown, why, nil
	}
	states := objectStates(cond, segs, p)
	if len(states) == 0 {
		return RenderedUnknown, fmt.Sprintf("no %s object in either render", cond.Kind), nil
	}
	decisive := false
	for _, st := range states {
		if st.state == "" {
			continue // the path is absent in both renders: decides nothing
		}
		decisive = true
		if st.state == string(cond.State) && valuesOK(cond, st.toVals) {
			return RenderedTrue, st.detail(cond.Path), st.evidence(p, cond.Path)
		}
	}
	if !decisive {
		return RenderedUnknown, fmt.Sprintf("%s has no %s in either render", cond.Kind, cond.Path), nil
	}
	if !p.Target.ValuesComplete {
		return RenderedUnknown, "values incomplete — an absence of change here decides nothing", nil
	}
	return RenderedFalse, "", nil
}

// valuesOK applies the optional Values filter: the To value must be one of
// cond.Values (JSON, string, or image-tag equality — the validator's rules).
// Resolved values are JSON-encoded; a JSON string is compared by its decoded
// form so `…image` matches Values ["v1.1.0"] by tag suffix.
func valuesOK(cond domain.Condition, to []string) bool {
	if len(cond.Values) == 0 {
		return true
	}
	plain := make([]string, len(to))
	for i, v := range to {
		var s string
		if json.Unmarshal([]byte(v), &s) == nil {
			plain[i] = s
		} else {
			plain[i] = v
		}
	}
	for _, want := range cond.Values {
		if valueMatches(want, plain) {
			return true
		}
	}
	return false
}

// objectState is the resolved state of the path in one object pair.
type objectState struct {
	from, to         []Object
	fromVals, toVals []string
	state            string // added | removed | changed | unchanged | "" absent in both
}

func (s objectState) detail(path string) string {
	obj := "object"
	if len(s.to) > 0 {
		obj = s.to[0].ID.Kind + "/" + s.to[0].ID.Name
	} else if len(s.from) > 0 {
		obj = s.from[0].ID.Kind + "/" + s.from[0].ID.Name
	}
	return fmt.Sprintf("%s %s is %s", obj, path, s.state)
}

// evidence cites the diff change at the condition path when the diff has one
// (a changed/added/removed value always has), else state records for both
// sides. A keyed selector (containers[name=controller].image) must match the
// change's Path exactly; a `[]` pattern matches every element's change.
func (s objectState) evidence(p *Pair, condPath string) []domain.Evidence {
	// a keyed selector must match the change's Path exactly (that one
	// element); a `[]` pattern matches every element's change
	keyed := false
	if segs, err := parsePatternPath(condPath); err == nil {
		for _, sg := range segs {
			keyed = keyed || (sg.sel != "" && sg.sel != "[]")
		}
	}
	pattern := patternOfPath(condPath)
	var out []domain.Evidence
	seen := map[domain.EvidenceID]bool{}
	add := func(e domain.Evidence) {
		if !seen[e.ID] {
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	objKeys := map[string]bool{}
	for _, o := range append(append([]Object{}, s.from...), s.to...) {
		objKeys[o.ID.key()] = true
	}
	for _, c := range p.Diff.Changes {
		if !objKeys[c.Object.key()] || c.Pattern != pattern {
			continue
		}
		if keyed && c.Path != condPath {
			continue
		}
		add(p.Evidence(c, false))
	}
	subject := "field " + condPath
	add(p.stateEvidence(p.FromResult, p.From, subject, s.occ(s.from, s.fromVals)))
	add(p.stateEvidence(p.ToResult, p.To, subject, s.occ(s.to, s.toVals)))
	return out
}

func (s objectState) occ(objs []Object, vals []string) []Occurrence {
	var out []Occurrence
	for _, o := range objs {
		if len(vals) == 0 {
			out = append(out, Occurrence{Object: o.ID})
			continue
		}
		for _, v := range vals {
			out = append(out, Occurrence{Object: o.ID, Value: v})
		}
	}
	out = sortOcc(out)
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// objectStates resolves the condition path in every object (matched by
// group/kind/name, paired across renders by identity) of one pair.
func objectStates(cond domain.Condition, segs []pathSeg, p *Pair) []objectState {
	from := matchObjects(p.FromResult.Objects, cond)
	to := matchObjects(p.ToResult.Objects, cond)
	fm, tm := map[string]Object{}, map[string]Object{}
	for _, o := range from {
		fm[o.ID.key()] = o
	}
	for _, o := range to {
		tm[o.ID.key()] = o
	}
	var out []objectState
	for _, k := range sortedObjKeys(from, to) {
		f, t := fm[k], tm[k]
		var fvals, tvals []string
		if f.ID.Kind != "" {
			fvals = resolveSegs(f.Body, segs)
		}
		if t.ID.Kind != "" {
			tvals = resolveSegs(t.Body, segs)
		}
		st := objectState{fromVals: fvals, toVals: tvals}
		switch {
		case len(fvals) == 0 && len(tvals) == 0:
			st.state = "" // absent in both: decides nothing
		case len(fvals) == 0:
			st.state = string(domain.StateAdded)
		case len(tvals) == 0:
			st.state = string(domain.StateRemoved)
		case sameStrings(fvals, tvals):
			st.state = string(domain.StateUnchanged)
		default:
			st.state = string(domain.StateChanged)
		}
		if f.ID.Kind != "" {
			st.from = []Object{f}
		}
		if t.ID.Kind != "" {
			st.to = []Object{t}
		}
		out = append(out, st)
	}
	return out
}

func sortedObjKeys(a, b []Object) []string {
	seen := map[string]bool{}
	for _, o := range append(append([]Object{}, a...), b...) {
		seen[o.ID.key()] = true
	}
	return sortedKeys(seen)
}

// matchObjects selects the render's objects by the condition's group/kind/name.
func matchObjects(objs []Object, cond domain.Condition) []Object {
	var out []Object
	for _, o := range objs {
		if o.ID.Kind != cond.Kind {
			continue
		}
		if cond.Group != "" && o.ID.Group != cond.Group {
			continue
		}
		if cond.Name != "" && o.ID.Name != cond.Name {
			continue
		}
		out = append(out, o)
	}
	return out
}

// --- the pattern-path resolver ----------------------------------------------------------------

// pathSeg is one segment of a condition path: a map key with an optional
// element selector ("" none, "[]" every element, "k=v" one element).
type pathSeg struct {
	key string
	sel string
}

// parsePatternPath parses the condition path syntax: dotted keys, quoted map
// keys (labels["app.kubernetes.io/name"]), all-elements selectors
// (containers[]) and keyed selectors (containers[name=controller],
// ports[port=443/TCP]). A quoted bracket is a map key and becomes its own
// segment, so labels["a.b"].value is [labels, a.b, value].
func parsePatternPath(p string) ([]pathSeg, error) {
	var segs []pathSeg
	appendSeg := func(s pathSeg) { segs = append(segs, s) }
	readKey := func(i int) (key string, next int, err error) {
		if p[i] == '"' {
			j := strings.IndexByte(p[i+1:], '"')
			if j < 0 {
				return "", i, fmt.Errorf("unterminated quoted key")
			}
			k, uerr := strconv.Unquote(p[i : i+j+2])
			if uerr != nil {
				return "", i, fmt.Errorf("quoted key: %v", uerr)
			}
			return k, i + j + 2, nil
		}
		j := i
		for j < len(p) && p[j] != '.' && p[j] != '[' {
			j++
		}
		if j == i {
			return "", i, fmt.Errorf("empty path segment at %d", i)
		}
		return p[i:j], j, nil
	}
	for i := 0; i < len(p); {
		key, next, err := readKey(i)
		if err != nil {
			return nil, err
		}
		i = next
		sel := ""
		if i < len(p) && p[i] == '[' {
			j := strings.IndexByte(p[i:], ']')
			if j < 0 {
				return nil, fmt.Errorf("unterminated selector")
			}
			raw := p[i+1 : i+j]
			i += j + 1
			switch {
			case raw == "":
				sel = "[]"
			case raw[0] == '"' || raw[0] == '`':
				// a quoted bracket is a map key of the node just read
				k, uerr := strconv.Unquote(raw)
				if uerr != nil {
					return nil, fmt.Errorf("quoted key: %v", uerr)
				}
				appendSeg(pathSeg{key: key})
				appendSeg(pathSeg{key: k})
				sel = "\x00" // already appended both segments
			default:
				if !strings.Contains(raw, "=") {
					return nil, fmt.Errorf("selector [%s] is neither [], a quoted map key nor key=value", raw)
				}
				sel = raw
			}
		}
		if sel != "\x00" {
			appendSeg(pathSeg{key: key, sel: sel})
		}
		if i < len(p) {
			if p[i] != '.' && p[i] != '[' {
				return nil, fmt.Errorf("expected '.' after %q", segString(key, sel))
			}
			if p[i] == '.' {
				i++
				if i == len(p) {
					return nil, fmt.Errorf("path ends after '.'")
				}
			}
		}
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	return segs, nil
}

// patternOfPath reduces a condition path to the diff's Pattern form: every
// keyed selector becomes [] (spec.template.spec.containers[].image), and keys
// holding separators are quoted the way the diff quotes them.
func patternOfPath(p string) string {
	segs, err := parsePatternPath(p)
	if err != nil {
		return p
	}
	var b strings.Builder
	for _, s := range segs {
		key := s.key
		if strings.ContainsAny(key, "./[]\"") {
			key = "[" + strconv.Quote(key) + "]"
		} else if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(key)
		if s.sel != "" {
			b.WriteString("[]")
		}
	}
	return b.String()
}

func segString(key, sel string) string {
	if sel == "" {
		return key
	}
	return key + "[" + sel + "]"
}

// resolveSegs returns every value the path addresses, JSON-encoded (the diff's
// Before/After encoding). A path that addresses nothing returns nil.
func resolveSegs(body map[string]any, segs []pathSeg) []string {
	var out []string
	var walk func(v any, i int)
	walk = func(v any, i int) {
		if i == len(segs) {
			if v != nil {
				out = append(out, encode(v))
			}
			return
		}
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		x := m[segs[i].key]
		if x == nil {
			return
		}
		switch segs[i].sel {
		case "":
			walk(x, i+1)
		case "[]":
			l, ok := x.([]any)
			if !ok {
				walk(x, i+1) // a bare "key[]" over a non-list names the value itself
				return
			}
			for _, e := range l {
				walk(e, i+1)
			}
		default: // "k=v"
			k, want, _ := strings.Cut(segs[i].sel, "=")
			l, ok := x.([]any)
			if !ok {
				return
			}
			for _, e := range l {
				em, ok := e.(map[string]any)
				if !ok {
					continue
				}
				if elemSelectorMatch(k, want, em) {
					walk(e, i+1)
				}
			}
		}
	}
	walk(body, 0)
	sort.Strings(out)
	return out
}

// elemSelectorMatch compares a keyed selector with an element, reproducing the
// diff's port keys (port=443/TCP carries the protocol; TCP is the default).
func elemSelectorMatch(key, want string, m map[string]any) bool {
	got := fmt.Sprint(m[key])
	if got == want {
		return true
	}
	if key == "port" || key == "containerPort" {
		proto, _ := m["protocol"].(string)
		if proto == "" {
			proto = "TCP"
		}
		return got+"/"+proto == want
	}
	return false
}
