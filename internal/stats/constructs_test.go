package stats

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
)

func loadFixture(t *testing.T, id string) *catalog.ProductDefinition {
	t.Helper()
	d, err := catalog.LoadFile(filepath.Join("testdata", "products", id+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func has(set []string, want string) bool {
	for _, s := range set {
		if s == want {
			return true
		}
	}
	return false
}

func TestConstructsAlpha(t *testing.T) {
	got := Constructs(loadFixture(t, "alpha"))
	for _, want := range []string{
		"field:sources.fallbackGroup", "field:artifacts.optional", "field:artifacts.exceptions",
		"field:artifacts.contents.availability", "field:sources.classify.skip",
		"locator:git-tags", "locator:oci", "locator:helm-repo", "locator:github-releases",
		"extract:markdown-section", "version:template", "content:helm-values",
		"artifact:container-image", "artifact:helm-chart", "role:release-notes",
		"scheme:semver", "lineage:minor",
		"template-var:Tag", "template-var:Major", "template-func:regexQuote",
	} {
		if !has(got, want) {
			t.Errorf("missing construct %q in %v", want, got)
		}
	}
	for _, not := range []string{
		"field:id", "field:name", "field:description", "field:kind", "field:apiVersion",
		"field:sources.id", "field:sources.notes", "field:artifacts.name",
		"locator:helm-git", "version:lookup", "template-var:NotAConstruct", // only in a description
		"category:other", // classification vocabulary is data
	} {
		if has(got, not) {
			t.Errorf("unexpected construct %q", not)
		}
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("constructs must be sorted: %v", got)
	}
}

func TestConstructsDifferBetweenProducts(t *testing.T) {
	beta := Constructs(loadFixture(t, "beta"))
	for _, want := range []string{"extract:yaml-records", "extract:release-note-yaml", "locator:repo-dir", "field:sources.locator.baseRef", "template-var:PrevTag", "release-kind:minor", "column-kind:supported"} {
		if !has(beta, want) {
			t.Errorf("beta: missing %q", want)
		}
	}
	if has(beta, "field:artifacts.optional") {
		t.Error("beta does not use optional")
	}
}

// TestEveryCatalogFieldIsPickedUp is the guard for the data-driven inventory:
// it populates every field of the definition format and requires a construct
// for each yaml path, so a field added to the catalog later is counted (or
// this test fails and says which one the walker cannot see).
func TestEveryCatalogFieldIsPickedUp(t *testing.T) {
	def := &catalog.ProductDefinition{}
	fill(reflect.ValueOf(def).Elem())
	got := map[string]bool{}
	for _, c := range Constructs(def) {
		got[c] = true
	}
	want := map[string]bool{}
	fieldPaths(reflect.TypeOf(*def), "", true, want)
	if len(want) < 40 {
		t.Fatalf("suspiciously few catalog fields: %d", len(want))
	}
	var missing []string
	for p := range want {
		if !got["field:"+p] {
			missing = append(missing, p)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("catalog fields without a construct: %v", missing)
	}
}

// fill sets every field to a non-zero value.
func fill(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" {
				fill(v.Field(i))
			}
		}
	case reflect.Ptr:
		v.Set(reflect.New(v.Type().Elem()))
		fill(v.Elem())
	case reflect.Slice:
		el := reflect.New(v.Type().Elem()).Elem()
		fill(el)
		v.Set(reflect.Append(reflect.MakeSlice(v.Type(), 0, 1), el))
	case reflect.String:
		v.SetString("x")
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(1)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(1)
	}
}

// fieldPaths collects the yaml paths of every non-descriptive field.
func fieldPaths(t reflect.Type, path string, root bool, out map[string]bool) {
	for t.Kind() == reflect.Ptr || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := yamlName(f)
		if name == "" || descriptiveFields[name] || (root && rootDescriptive[name]) {
			continue
		}
		p := join(path, name)
		ft := f.Type
		for ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		switch {
		case ft.Kind() == reflect.Struct:
			fieldPaths(ft, p, false, out) // mandatory sub-document: only its fields
		case ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.Struct:
			out[p] = true
			fieldPaths(ft.Elem(), p, false, out)
		default:
			out[p] = true
		}
		if f.Type.Kind() == reflect.Ptr && ft.Kind() == reflect.Struct {
			out[p] = true // an optional sub-document is itself a construct
		}
	}
}

func TestEnumerationsAreDetectedByName(t *testing.T) {
	// A field not in the enumeration table but named like an enumeration is
	// treated as one, using the lower-cased struct name as the prefix.
	type Widget struct {
		Kind string `yaml:"kind"`
		Mode string `yaml:"mode"`
		Free string `yaml:"free"`
	}
	type doc struct {
		Widgets []Widget `yaml:"widgets"`
	}
	w := newWalker()
	w.walk(reflect.ValueOf(doc{Widgets: []Widget{{Kind: "a", Mode: "m", Free: "f"}}}), "", "", true)
	got := w.constructs()
	for _, want := range []string{"widget:a", "mode:m", "field:widgets", "field:widgets.kind", "field:widgets.free"} {
		if !has(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if has(got, "free:f") {
		t.Errorf("a free-text field is not an enumeration: %v", got)
	}
}

func TestTemplateExtraction(t *testing.T) {
	w := newWalker()
	w.template(`^{{regexQuote .Line}}(\s|$) and {{.Tag}} {{if .Prerelease}}x{{end}} {{ .A | lower }}`)
	got := w.constructs()
	for _, want := range []string{"template-func:regexQuote", "template-var:Line", "template-var:Tag", "template-var:Prerelease", "template-func:lower", "template-var:A"} {
		if !has(got, want) {
			t.Errorf("missing %q in %v", want, got)
		}
	}
	if w.tmplExprs != 5 { // every "{{", including the closing {{end}}
		t.Errorf("template expressions = %d, want 5", w.tmplExprs)
	}
	// text without expressions, and unparsable expressions, never panic
	w2 := newWalker()
	w2.template("plain {text} and {{ unterminated")
	if w2.tmplExprs != 1 || len(w2.constructs()) != 0 {
		t.Errorf("unparsable template: exprs=%d constructs=%v", w2.tmplExprs, w2.constructs())
	}
}

// TestCheckedInProductsHaveConstructs runs the inventory on the real
// definitions: no panics, and every product uses the base constructs.
func TestCheckedInProductsHaveConstructs(t *testing.T) {
	cat, err := catalog.LoadDir(filepath.Join("..", "..", "products"))
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range cat.List() {
		ps := Analyze(d, nil)
		if ps.YAMLLines == 0 || ps.Sources == 0 {
			t.Errorf("%s: empty size %+v", d.ID, ps.Size)
		}
		for _, base := range []string{"field:sources", "field:versioning.scheme", "role:versions"} {
			if !has(ps.Constructs, base) {
				t.Errorf("%s: missing base construct %q", d.ID, base)
			}
		}
		for _, c := range ps.Constructs {
			if strings.TrimSpace(c) == "" || !strings.Contains(c, ":") {
				t.Errorf("%s: malformed construct %q", d.ID, c)
			}
		}
	}
}
