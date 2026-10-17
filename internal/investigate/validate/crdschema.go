package validate

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// A custom resource checked against the CustomResourceDefinition the same
// side renders (or the environment files define): is its apiVersion served,
// and does it satisfy the version's structural schema (types, enums,
// required fields, fields the schema does not know — the API server rejects
// those under strict field validation and otherwise prunes them, so the
// setting is lost)? A kind whose CRD is not in the view decides nothing (the
// cluster may have it from elsewhere); an apiextensions.k8s.io/v1beta1 CRD or
// a schema construct not modelled here is not checked (no violation is ever
// invented).

// schemaViolation is one way an object fails its CRD at one side.
type schemaViolation struct {
	object string // object key
	crd    string // CRD object key
	path   string
	what   string // not-served | type | enum | required | unknown-field
	detail string
}

func (v schemaViolation) identity() string { return v.object + "|" + v.path + "|" + v.what }

func (v schemaViolation) line() string {
	return fmt.Sprintf("%s %s: %s (CRD %s)", v.object, v.path, v.detail, v.crd)
}

// crdIndex finds the CRD of a group/kind among a view's objects.
type crdIndex map[string]relations.Object

func newCRDIndex(view *relations.View) crdIndex {
	ix := crdIndex{}
	for _, o := range view.Objects {
		if o.ID.Kind != "CustomResourceDefinition" || o.ID.Group != "apiextensions.k8s.io" {
			continue
		}
		g, k := yStr(o.Body, "spec", "group"), yStr(o.Body, "spec", "names", "kind")
		if g != "" && k != "" {
			ix[g+"/"+k] = o
		}
	}
	return ix
}

// customResourcesOf returns the view's objects of the kind a CRD object
// (key) defines.
func customResourcesOf(view *relations.View, crdKey string) []relations.Object {
	var crd *relations.Object
	for i := range view.Objects {
		if view.Objects[i].Key == crdKey {
			crd = &view.Objects[i]
		}
	}
	if crd == nil {
		return nil
	}
	g, k := yStr(crd.Body, "spec", "group"), yStr(crd.Body, "spec", "names", "kind")
	var out []relations.Object
	for _, o := range view.Objects {
		if o.ID.Group == g && o.ID.Kind == k {
			out = append(out, o)
		}
	}
	return out
}

// check returns the violations of an object against its CRD in the index,
// and whether a CRD was found and checked.
func (ix crdIndex) check(o relations.Object) ([]schemaViolation, bool) {
	crd, ok := ix[o.ID.Group+"/"+o.ID.Kind]
	if !ok || crd.ID.Version != "v1" {
		return nil, false
	}
	var out []schemaViolation
	add := func(path, what, detail string) {
		out = append(out, schemaViolation{object: o.Key, crd: crd.Key, path: path, what: what, detail: detail})
	}
	var version map[string]any
	var names []string
	for _, v := range ySeq(yGet(crd.Body, "spec", "versions")) {
		n := yStr(v, "name")
		served, _ := yMap(v)["served"].(bool)
		if served {
			names = append(names, n)
		}
		if n == o.ID.Version {
			version = yMap(v)
		}
	}
	switch {
	case version == nil:
		add("apiVersion", "not-served", fmt.Sprintf("apiVersion %s/%s is not defined by the CRD (served versions: %s)", o.ID.Group, o.ID.Version, strings.Join(names, ", ")))
		return out, true
	case version["served"] != true:
		add("apiVersion", "not-served", fmt.Sprintf("apiVersion %s/%s is no longer served by the CRD (served versions: %s)", o.ID.Group, o.ID.Version, strings.Join(names, ", ")))
		return out, true
	}
	schema := yMap(yGet(version, "schema", "openAPIV3Schema"))
	if schema == nil {
		return out, true // no schema: everything is preserved
	}
	props := yMap(schema["properties"])
	keys := sortedKeys(o.Body)
	for _, k := range keys {
		switch k {
		case "apiVersion", "kind", "metadata", "status":
			continue
		}
		if s := yMap(props[k]); s != nil {
			walkSchema(o.Body[k], s, k, add)
		} else if !preserves(schema) {
			add(k, "unknown-field", "field "+k+" is not in the CRD schema")
		}
	}
	for _, r := range ySeq(schema["required"]) {
		if k, _ := r.(string); k != "" && k != "metadata" && k != "apiVersion" && k != "kind" && k != "status" {
			if _, has := o.Body[k]; !has {
				add(k, "required", "required field "+k+" is missing")
			}
		}
	}
	return out, true
}

func preserves(s map[string]any) bool {
	b, _ := s["x-kubernetes-preserve-unknown-fields"].(bool)
	return b
}

// walkSchema checks a value against a structural schema node.
func walkSchema(v any, s map[string]any, path string, add func(path, what, detail string)) {
	if v == nil {
		return // null: pruned or defaulted, never a rejection we can claim
	}
	if b, _ := s["x-kubernetes-int-or-string"].(bool); b {
		switch v.(type) {
		case string, int, int64, float64:
		default:
			add(path, "type", fmt.Sprintf("%s must be an integer or a string, got %s", path, typeName(v)))
		}
		return
	}
	if !enumAllows(s, v) {
		add(path, "enum", fmt.Sprintf("%s = %v is not one of the allowed values %v", path, v, ySeq(s["enum"])))
	}
	switch t, _ := s["type"].(string); t {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			add(path, "type", fmt.Sprintf("%s must be an object, got %s", path, typeName(v)))
			return
		}
		props := yMap(s["properties"])
		addl := s["additionalProperties"]
		embedded, _ := s["x-kubernetes-embedded-resource"].(bool)
		for _, k := range sortedKeys(m) {
			child := path + "." + k
			switch {
			case props[k] != nil:
				if ps := yMap(props[k]); ps != nil {
					walkSchema(m[k], ps, child, add)
				}
			case yMap(addl) != nil:
				walkSchema(m[k], yMap(addl), child, add)
			case addl == true, preserves(s), embedded && (k == "apiVersion" || k == "kind" || k == "metadata"):
			case len(props) == 0 && addl == nil && hasCombinator(s):
				// properties only under anyOf/oneOf/allOf: not modelled
			default:
				add(child, "unknown-field", "field "+child+" is not in the CRD schema")
			}
		}
		for _, r := range ySeq(s["required"]) {
			if k, _ := r.(string); k != "" {
				if _, has := m[k]; !has {
					add(path+"."+k, "required", "required field "+path+"."+k+" is missing")
				}
			}
		}
	case "array":
		l, ok := v.([]any)
		if !ok {
			add(path, "type", fmt.Sprintf("%s must be an array, got %s", path, typeName(v)))
			return
		}
		if items := yMap(s["items"]); items != nil {
			for i, e := range l {
				walkSchema(e, items, fmt.Sprintf("%s[%d]", path, i), add)
			}
		}
	case "string":
		if _, ok := v.(string); !ok {
			add(path, "type", fmt.Sprintf("%s must be a string, got %s", path, typeName(v)))
		}
	case "integer":
		switch n := v.(type) {
		case int, int64:
		case float64:
			if n != math.Trunc(n) {
				add(path, "type", fmt.Sprintf("%s must be an integer, got %v", path, n))
			}
		default:
			add(path, "type", fmt.Sprintf("%s must be an integer, got %s", path, typeName(v)))
		}
	case "number":
		switch v.(type) {
		case int, int64, float64:
		default:
			add(path, "type", fmt.Sprintf("%s must be a number, got %s", path, typeName(v)))
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			add(path, "type", fmt.Sprintf("%s must be a boolean, got %s", path, typeName(v)))
		}
	}
}

func hasCombinator(s map[string]any) bool {
	return s["anyOf"] != nil || s["oneOf"] != nil || s["allOf"] != nil
}

func enumAllows(s map[string]any, v any) bool {
	enum := ySeq(s["enum"])
	if len(enum) == 0 {
		return true
	}
	for _, e := range enum {
		if fmt.Sprint(e) == fmt.Sprint(v) {
			return true
		}
	}
	return false
}

func typeName(v any) string {
	switch v.(type) {
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	case string:
		return "a string"
	case bool:
		return "a boolean"
	case int, int64:
		return "an integer"
	case float64:
		return "a number"
	}
	return fmt.Sprintf("%T", v)
}

func sortedKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// newViolations checks the given TO objects against the TO CRDs and returns
// the violations the same object did not have at FROM (against the FROM
// CRDs) — what the upgrade breaks — plus how many objects had a CRD to be
// checked against.
func newViolations(from, to *relations.View, objects []relations.Object) (out []schemaViolation, checked int) {
	toIx, fromIx := newCRDIndex(to), newCRDIndex(from)
	before := map[string]relations.Object{}
	for _, o := range from.Objects {
		before[o.Key] = o
	}
	seen := map[string]bool{}
	for _, o := range objects {
		if seen[o.Key] {
			continue
		}
		seen[o.Key] = true
		vs, ok := toIx.check(o)
		if !ok {
			continue
		}
		checked++
		old := map[string]bool{}
		if fo, ok := before[o.Key]; ok {
			fvs, _ := fromIx.check(fo)
			for _, v := range fvs {
				old[v.identity()] = true
			}
		}
		for _, v := range vs {
			if !old[v.identity()] {
				out = append(out, v)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].identity() < out[j].identity() })
	return out, checked
}

// --- accessors over decoded YAML -------------------------------------------------

func yMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func ySeq(v any) []any {
	s, _ := v.([]any)
	return s
}

func yGet(v any, keys ...string) any {
	for _, k := range keys {
		m := yMap(v)
		if m == nil {
			return nil
		}
		v = m[k]
	}
	return v
}

func yStr(v any, keys ...string) string {
	s, _ := yGet(v, keys...).(string)
	return s
}
