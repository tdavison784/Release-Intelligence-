package main

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const schemaDialect = "https://json-schema.org/draft/2020-12/schema"

// target is one generated schema file.
type target struct {
	file string       // file name under schemas/
	id   string       // $id
	root reflect.Type // root Go type
}

var targets = []target{
	{"upgrade-edge.schema.json", "https://ri.dev/schemas/upgrade-edge/v1alpha1.json", reflect.TypeOf(domain.UpgradeEdge{})},
	{"release.schema.json", "https://ri.dev/schemas/release/v1alpha1.json", reflect.TypeOf(domain.Release{})},
	{"impact-report.schema.json", "https://ri.dev/schemas/impact-report/v1alpha1.json", reflect.TypeOf(domain.ImpactReport{})},
}

var timeType = reflect.TypeOf(time.Time{})

// generate builds every schema file in memory (file name to content). It
// fails when the hand-maintained tables in meta.go refer to types or fields
// that no longer exist.
func generate() (files map[string][]byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("schemagen: %v", r)
		}
	}()
	used := map[string]bool{}
	files = map[string][]byte{}
	for _, t := range targets {
		g := &gen{defs: map[string]obj{}, used: used}
		g.ref(t.root)
		g.addProvenanceVariants()
		doc := o(
			"$schema", schemaDialect,
			"$id", t.id,
			"title", t.root.Name(),
			"description", descriptions[t.root.Name()],
			"$ref", "#/$defs/"+t.root.Name(),
			"$defs", g.sortedDefs(),
		)
		files[t.file] = pretty(doc)
	}
	if err := checkUsed(used); err != nil {
		return nil, err
	}
	return files, nil
}

// checkUsed fails when a key of the meta tables was never consumed.
func checkUsed(used map[string]bool) error {
	var stale []string
	check := func(table string, keys []string) {
		for _, k := range keys {
			if !used[table+":"+k] {
				stale = append(stale, table+" "+k)
			}
		}
	}
	keys := func(m map[string]string) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	objKeys := func(m map[string]obj) []string {
		var out []string
		for k := range m {
			out = append(out, k)
		}
		return out
	}
	check("description", keys(descriptions))
	check("fieldPatch", objKeys(fieldPatches))
	check("typePatch", objKeys(typePatches))
	var enumNames []string
	for _, e := range enums {
		enumNames = append(enumNames, e.typ.Name())
	}
	check("enum", enumNames)
	if len(stale) == 0 {
		return nil
	}
	sort.Strings(stale)
	return fmt.Errorf("schemagen: meta.go entries match no type or field in the schemas (renamed or removed in domain?): %s", strings.Join(stale, "; "))
}

// gen accumulates the $defs of one schema document.
type gen struct {
	defs map[string]obj
	used map[string]bool // shared across documents, see checkUsed
}

func (g *gen) sortedDefs() obj {
	names := make([]string, 0, len(g.defs))
	for n := range g.defs {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make(obj, 0, len(names))
	for _, n := range names {
		out = append(out, kv{n, g.defs[n]})
	}
	return out
}

// describe adds the hand-written description for key to a field schema.
func (g *gen) describe(key string, s obj) obj {
	d, ok := descriptions[key]
	if !ok {
		return s
	}
	g.used["description:"+key] = true
	return s.with("description", d)
}

// describeDef adds the description of a $defs entry; it comes first, right
// after the type, so the definition reads top-down.
func (g *gen) describeDef(key string, s obj) obj {
	d, ok := descriptions[key]
	if !ok {
		return s
	}
	g.used["description:"+key] = true
	out := make(obj, 0, len(s)+1)
	for i, p := range s {
		out = append(out, p)
		if i == 0 && p.K == "type" {
			out = append(out, kv{"description", d})
		}
	}
	if len(out) == len(s) { // no leading type: put the description first
		out = append(obj{{"description", d}}, s...)
	}
	return out
}

var (
	marshalerType = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
	textType      = reflect.TypeOf((*encoding.TextMarshaler)(nil)).Elem()
)

// ref returns the schema to use where type t occurs: a $ref to a $defs entry
// for named types, an inline schema for everything else. Struct types are
// generated into $defs on first use.
func (g *gen) ref(t reflect.Type) obj {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == timeType {
		return o("type", "string", "format", "date-time")
	}
	if t.PkgPath() != "" && (t.Implements(marshalerType) || reflect.PointerTo(t).Implements(marshalerType) ||
		t.Implements(textType) || reflect.PointerTo(t).Implements(textType)) {
		panic(fmt.Sprintf("type %s has a custom JSON marshaler; teach the generator about it", t))
	}
	switch t.Kind() {
	case reflect.Struct:
		g.structDef(t)
		return o("$ref", "#/$defs/"+t.Name())
	case reflect.String:
		if t.PkgPath() == "" {
			return o("type", "string")
		}
		g.stringDef(t)
		return o("$ref", "#/$defs/"+t.Name())
	case reflect.Bool:
		return o("type", "boolean")
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return o("type", "integer")
	case reflect.Float32, reflect.Float64:
		return o("type", "number")
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return o("type", "string", "contentEncoding", "base64")
		}
		return o("type", "array", "items", g.ref(t.Elem()))
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			panic(fmt.Sprintf("map %s: only string keys are supported", t))
		}
		return o("type", "object", "additionalProperties", g.ref(t.Elem()))
	case reflect.Interface:
		return obj{}
	}
	panic(fmt.Sprintf("unsupported type %s (%s)", t, t.Kind()))
}

// stringDef emits the $defs entry of a named string type: an enum when the
// constants are a registered closed set, otherwise a plain string.
func (g *gen) stringDef(t reflect.Type) {
	name := t.Name()
	if _, done := g.defs[name]; done {
		return
	}
	def := o("type", "string")
	for _, e := range enums {
		if e.typ == t {
			g.used["enum:"+name] = true
			vals := make([]any, len(e.values))
			for i, v := range e.values {
				vals[i] = v
			}
			def = def.with("enum", vals)
		}
	}
	g.defs[name] = g.describeDef(name, def)
}

type fieldOpts struct {
	name      string
	omitEmpty bool
	omitZero  bool
}

func parseTag(f reflect.StructField) (fieldOpts, bool) {
	tag, ok := f.Tag.Lookup("json")
	if tag == "-" || !f.IsExported() {
		return fieldOpts{}, false
	}
	if f.Anonymous {
		panic(fmt.Sprintf("embedded field %s: not supported by the generator", f.Name))
	}
	parts := strings.Split(tag, ",")
	opts := fieldOpts{name: parts[0]}
	if !ok || opts.name == "" {
		opts.name = f.Name
	}
	for _, p := range parts[1:] {
		switch p {
		case "omitempty":
			opts.omitEmpty = true
		case "omitzero":
			opts.omitZero = true
		}
	}
	return opts, true
}

// optional reports whether encoding/json may leave the field out. Note that
// omitempty has no effect on struct values (including time.Time and Version):
// those are always emitted, so such fields are required in the schema.
func (fo fieldOpts) optional(t reflect.Type) bool {
	if fo.omitZero {
		return true
	}
	if !fo.omitEmpty {
		return false
	}
	return t.Kind() != reflect.Struct
}

func (g *gen) structDef(t reflect.Type) {
	name := t.Name()
	if _, done := g.defs[name]; done {
		return
	}
	g.defs[name] = nil // guard against recursion
	var props obj
	var required []string
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		opts, ok := parseTag(f)
		if !ok {
			continue
		}
		key := name + "." + opts.name
		s := g.ref(f.Type)
		if p, ok := fieldPatches[key]; ok {
			g.used["fieldPatch:"+key] = true
			s = s.merge(p)
		}
		s = g.describe(key, s)
		optional := opts.optional(f.Type)
		if f.Type.Kind() == reflect.Pointer && !optional {
			// A nil pointer without omitempty encodes as null.
			s = o("anyOf", []any{s, o("type", "null")})
		}
		props = append(props, kv{opts.name, s})
		if !optional {
			required = append(required, opts.name)
		}
	}
	def := o("type", "object")
	if len(required) > 0 {
		def = def.with("required", required)
	}
	def = def.with("properties", props).with("additionalProperties", false)
	if p, ok := typePatches[name]; ok {
		g.used["typePatch:"+name] = true
		def = def.merge(p)
	}
	g.defs[name] = g.describeDef(name, def)
}

// addProvenanceVariants derives the two refinements of Provenance that carry
// the AI / deterministic boundary. Both are layered on the generated base
// definition with allOf, so they cannot drift from the Go struct.
func (g *gen) addProvenanceVariants() {
	if _, ok := g.defs[defProvenance]; !ok {
		return
	}
	base := []any{o("$ref", "#/$defs/"+defProvenance)}
	det := []any{string(domain.MethodDeclared), string(domain.MethodComputed), string(domain.MethodHeuristic)}
	g.defs[defDeterminist] = g.describeDef(defDeterminist, o(
		"allOf", base,
		"properties", o("method", o("enum", det)),
		"not", o("anyOf", []any{
			o("required", []string{"model"}),
			o("required", []string{"promptDigest"}),
			o("required", []string{"inputEvidence"}),
			o("required", []string{"generatedAt"}),
			o("required", []string{"modelVersion"}),
			o("required", []string{"promptVersion"}),
		}),
	))
	g.defs[defAI] = g.describeDef(defAI, o(
		"allOf", base,
		"required", []string{"model", "modelVersion", "promptVersion", "promptDigest", "inputEvidence", "generatedAt"},
		"properties", o(
			"method", o("const", string(domain.MethodAI)),
			"model", o("minLength", 1),
			"modelVersion", o("minLength", 1),
			"promptVersion", o("minLength", 1),
			"promptDigest", o("minLength", 1),
			"inputEvidence", o("minItems", 1),
		),
	))
}
