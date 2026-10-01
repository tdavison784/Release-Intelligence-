// Package stats measures how product onboarding scales.
//
// The central question of the second phase is whether adding another product
// increasingly becomes configuration (a new products/<id>.yaml) instead of
// software development (a new generic construct in the catalog format or the
// ingestion code). This package derives the numbers:
//
//   - per product definition: size and complexity, and the set of catalog
//     constructs it uses (computed from the parsed definition by reflection,
//     so constructs added to the catalog later are picked up without editing
//     any list);
//   - per onboarding record (docs/onboarding/records/*.yaml): the constructs
//     the product introduced, the generic Go changes it needed, how much of
//     the definition discovery found, how long it took;
//   - per saved relationship check (docs/onboarding/checks/*.json): how many
//     declared relationships validated, failed or could not be verified;
//   - across products in onboarding order: the construct-introduction curve
//     and per-wave aggregates.
//
// Everything here is pure and offline; the only I/O is reading the three
// input directories.
package stats

import (
	"reflect"
	"sort"
	"strings"
	"text/template/parse"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

// Construct names. A construct is a string with a kind prefix:
//
//	field:sources.fallbackGroup       a catalog field that is set (yaml path;
//	                                  list elements do not appear in the path)
//	locator:git-log                   an enumerated value: locator kind,
//	extract:markdown-table            extract type, version strategy, content
//	version:lookup                    kind, artifact type, source role, ...
//	content:helm-values
//	template-var:PrevTag              a template variable ({{.PrevTag}})
//	template-func:regexQuote          a template function
const (
	prefixField        = "field:"
	prefixTemplateVar  = "template-var:"
	prefixTemplateFunc = "template-func:"
)

// descriptiveFields are free-text or bookkeeping fields (at any depth). They
// document a definition but do not drive ingestion, so they are not
// constructs and are not scanned for template expressions.
var descriptiveFields = map[string]bool{
	"id": true, "name": true, "description": true, "homepage": true, "notes": true,
	"apiVersion": true, "provenance": true, "validatedAgainst": true,
}

// rootDescriptive are descriptive only at the document root ("kind" at the
// root is the document kind; below it, "kind" selects a locator or content
// type and is a construct).
var rootDescriptive = map[string]bool{"kind": true}

// enumFields names the fields whose values are an enumeration, keyed by
// "<Go struct name>.<yaml field>", with the construct prefix for the values.
// An empty prefix means "not a construct" (the value is data). Fields not
// listed here are detected by name, see enumPrefix.
var enumFields = map[string]string{
	"Versioning.scheme":        "scheme",
	"Versioning.lineage":       "lineage",
	"Source.roles":             "role",
	"Source.releaseKinds":      "release-kind",
	"Locator.kind":             "locator",
	"Extract.type":             "extract",
	"ColumnSpec.kind":          "column-kind",
	"ColumnSpec.platform":      "", // data: which platform a column describes
	"ClassifyRule.category":    "", // data: the classification vocabulary
	"Artifact.type":            "artifact",
	"VersionRelation.strategy": "version",
	"VersionRelation.select":   "select",
	"Content.kind":             "content",
}

// enumNames are yaml field names that, in a struct missing from enumFields,
// are assumed to hold an enumeration (so values added to the catalog later,
// such as a new kind of something, are still detected). The prefix is the
// lower-cased struct name for kind and type, otherwise the field name.
var enumNames = map[string]bool{
	"kind": true, "type": true, "strategy": true, "mode": true,
	"scheme": true, "policy": true, "format": true, "lineage": true, "select": true,
}

// Constructs returns the sorted set of constructs a definition uses.
func Constructs(def *catalog.ProductDefinition) []string {
	w := newWalker()
	w.walk(reflect.ValueOf(def), "", "", true)
	return w.constructs()
}

// walker visits a definition and collects constructs and template data.
type walker struct {
	set       map[string]struct{}
	tmplExprs int
}

func newWalker() *walker { return &walker{set: map[string]struct{}{}} }

func (w *walker) add(s string) { w.set[s] = struct{}{} }

func (w *walker) constructs() []string {
	out := make([]string, 0, len(w.set))
	for s := range w.set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func join(path, name string) string {
	if path == "" {
		return name
	}
	return path + "." + name
}

// yamlName returns the yaml field name of a struct field ("" to skip).
func yamlName(f reflect.StructField) string {
	if f.PkgPath != "" { // unexported
		return ""
	}
	tag := f.Tag.Get("yaml")
	if tag == "-" {
		return ""
	}
	name := strings.Split(tag, ",")[0]
	if name == "" {
		name = strings.ToLower(f.Name[:1]) + f.Name[1:]
	}
	return name
}

// walk visits v. path is the yaml path of v; owner the Go name of the
// enclosing struct; root is true only for the document itself.
func (w *walker) walk(v reflect.Value, path, owner string, root bool) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			name := yamlName(f)
			if name == "" || descriptiveFields[name] || (root && rootDescriptive[name]) {
				continue
			}
			w.field(v.Field(i), join(path, name), t.Name(), name)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			w.walk(v.Index(i), path, owner, false)
		}
	}
}

// field handles one struct field: it records the field itself when set,
// its enumerated values and template data, and recurses into nested structs.
func (w *walker) field(fv reflect.Value, path, owner, name string) {
	ft := fv.Type()
	switch {
	case ft.Kind() == reflect.Ptr:
		if fv.IsNil() {
			return
		}
		// an optional sub-document (or tri-state bool) that is present
		w.add(prefixField + path)
		w.field(fv.Elem(), path, owner, name)
		// field() on the element would add the same field again: harmless.
	case ft.Kind() == reflect.Struct:
		// a mandatory sub-document: its own fields are the constructs
		w.walk(fv, path, owner, false)
	case ft.Kind() == reflect.Slice:
		if fv.Len() == 0 {
			return
		}
		w.add(prefixField + path)
		for i := 0; i < fv.Len(); i++ {
			el := fv.Index(i)
			switch el.Kind() {
			case reflect.Struct, reflect.Ptr:
				w.walk(el, path, owner, false)
			case reflect.String:
				w.value(el.String(), owner, name, ft.Elem())
			}
		}
	case ft.Kind() == reflect.String:
		if fv.String() == "" {
			return
		}
		w.add(prefixField + path)
		w.value(fv.String(), owner, name, ft)
	case ft.Kind() == reflect.Bool:
		if fv.Bool() {
			w.add(prefixField + path)
		}
	case ft.Kind() >= reflect.Int && ft.Kind() <= reflect.Uint64, ft.Kind() == reflect.Float32, ft.Kind() == reflect.Float64:
		if !fv.IsZero() {
			w.add(prefixField + path)
		}
	}
}

// value handles a string value: enumeration constructs and template data.
func (w *walker) value(s, owner, name string, t reflect.Type) {
	if prefix, ok := enumPrefix(owner, name, t); ok && prefix != "" {
		w.add(prefix + ":" + s)
	}
	w.template(s)
}

// enumPrefix reports whether owner.name holds an enumeration and the prefix
// of its value constructs.
func enumPrefix(owner, name string, t reflect.Type) (string, bool) {
	if p, ok := enumFields[owner+"."+name]; ok {
		return p, true
	}
	if enumNames[name] {
		if name == "kind" || name == "type" {
			return strings.ToLower(owner), true
		}
		return name, true
	}
	// a named string type (a Go enum) that the table does not know
	if t.Kind() == reflect.String && t.PkgPath() != "" && t.Name() != "string" {
		return strings.ToLower(name), true
	}
	return "", false
}

// template counts the template expressions in s and records the variables
// and functions they use.
func (w *walker) template(s string) {
	n := strings.Count(s, "{{")
	if n == 0 {
		return
	}
	w.tmplExprs += n
	t := parse.New("x")
	t.Mode = parse.SkipFuncCheck
	trees := map[string]*parse.Tree{}
	if _, err := t.Parse(s, "", "", trees, map[string]any{}); err != nil {
		return // still counted as expressions; names cannot be extracted
	}
	for _, tr := range trees {
		if tr != nil && tr.Root != nil {
			w.node(tr.Root)
		}
	}
}

func (w *walker) node(n parse.Node) {
	switch n := n.(type) {
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			w.node(c)
		}
	case *parse.ActionNode:
		w.node(n.Pipe)
	case *parse.PipeNode:
		if n == nil {
			return
		}
		for _, c := range n.Cmds {
			w.node(c)
		}
	case *parse.CommandNode:
		for _, a := range n.Args {
			w.node(a)
		}
	case *parse.FieldNode:
		if len(n.Ident) > 0 {
			w.add(prefixTemplateVar + n.Ident[0])
		}
	case *parse.ChainNode:
		w.node(n.Node)
	case *parse.IdentifierNode:
		w.add(prefixTemplateFunc + n.Ident)
	case *parse.IfNode:
		w.branch(&n.BranchNode)
	case *parse.RangeNode:
		w.branch(&n.BranchNode)
	case *parse.WithNode:
		w.branch(&n.BranchNode)
	}
}

func (w *walker) branch(b *parse.BranchNode) {
	w.node(b.Pipe)
	if b.List != nil {
		w.node(b.List)
	}
	if b.ElseList != nil {
		w.node(b.ElseList)
	}
}
