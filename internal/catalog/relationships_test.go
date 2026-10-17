package catalog

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

var relRef = []ConfigReference{{URL: "https://example.io/docs/api", Quote: "spec.secretName names the Secret."}}

func validProduces() Relationship {
	return Relationship{ID: "cert-secret", Kind: RelProduces, From: ObjectType{Group: "demo.io", Kind: "Cert"}, To: ObjectType{Kind: "Secret"},
		Summary: "a Cert creates its Secret", Path: "spec.secretName", References: relRef}
}

func relDef(rs ...Relationship) *ProductDefinition {
	d := lifecycleDef()
	d.Relationships = rs
	return d
}

func TestRelationshipsValid(t *testing.T) {
	sel := Relationship{ID: "mon", Kind: RelSelects, From: ObjectType{Group: "demo.io", Kind: "Mon"}, To: ObjectType{Kind: "Service"},
		Summary: "selects services", SelectorPath: "spec.selector", NamespaceSelectorPath: "spec.namespaceSelector", References: relRef}
	ref := Relationship{ID: "ann", Kind: RelReferences, From: ObjectType{Group: "networking.k8s.io", Kind: "Ingress"}, To: ObjectType{Kind: "Secret"},
		Summary: "annotation names a Secret", Path: "metadata.annotations[example.io/secret]", Format: RelFormatNamespaceName, References: relRef}
	cond := validProduces()
	cond.ID, cond.Kind, cond.Scope, cond.WhenPath, cond.WhenEquals = "cond", RelReferences, "cluster", "spec.kind", "Cluster"
	if rep := Validate(relDef(validProduces(), sel, ref, cond)); !rep.OK() {
		t.Fatalf("valid relationships rejected: %v", rep.Errors())
	}
}

func TestRelationshipsRejectBadRules(t *testing.T) {
	mut := func(f func(*Relationship)) Relationship {
		r := validProduces()
		f(&r)
		return r
	}
	sel := func(f func(*Relationship)) Relationship {
		r := Relationship{ID: "mon", Kind: RelSelects, From: ObjectType{Kind: "Mon"}, To: ObjectType{Kind: "Service"}, Summary: "s", SelectorPath: "spec.selector", References: relRef}
		f(&r)
		return r
	}
	cases := map[string]struct {
		rs   []Relationship
		want string
	}{
		"bad id":               {[]Relationship{mut(func(r *Relationship) { r.ID = "Bad_ID" })}, "lowercase DNS label"},
		"duplicate id":         {[]Relationship{validProduces(), validProduces()}, "duplicate relationship id"},
		"unknown kind":         {[]Relationship{mut(func(r *Relationship) { r.Kind = "owns" })}, "unknown kind"},
		"no from kind":         {[]Relationship{mut(func(r *Relationship) { r.From.Kind = "" })}, "from.kind"},
		"no to kind":           {[]Relationship{mut(func(r *Relationship) { r.To.Kind = "" })}, "to.kind"},
		"no summary":           {[]Relationship{mut(func(r *Relationship) { r.Summary = "" })}, "summary"},
		"no path":              {[]Relationship{mut(func(r *Relationship) { r.Path = "" })}, "required for kind produces"},
		"bad path":             {[]Relationship{mut(func(r *Relationship) { r.Path = "spec..x" })}, "empty key"},
		"unterminated bracket": {[]Relationship{mut(func(r *Relationship) { r.Path = "metadata.annotations[x" })}, "unterminated"},
		"bad format":           {[]Relationship{mut(func(r *Relationship) { r.Format = "ns:name" })}, "format"},
		"bad scope":            {[]Relationship{mut(func(r *Relationship) { r.Scope = "global" })}, "scope"},
		"selector on produces": {[]Relationship{mut(func(r *Relationship) { r.SelectorPath = "spec.selector" })}, "not a field of kind produces"},
		"when without equals":  {[]Relationship{mut(func(r *Relationship) { r.WhenPath = "spec.kind" })}, "go together"},
		"no references":        {[]Relationship{mut(func(r *Relationship) { r.References = nil })}, "upstream documentation reference"},
		"http reference":       {[]Relationship{mut(func(r *Relationship) { r.References = []ConfigReference{{URL: "http://x.io", Quote: "q"}} })}, "https URL"},
		"eval citation": {[]Relationship{mut(func(r *Relationship) {
			r.References = []ConfigReference{{URL: "https://github.com/o/r/blob/main/eval/investigations/x/expected.yaml", Quote: "q"}}
		})}, "evaluation data"},
		"empty quote":          {[]Relationship{mut(func(r *Relationship) { r.References = []ConfigReference{{URL: "https://x.io", Quote: " "}} })}, "quote"},
		"selects without path": {[]Relationship{sel(func(r *Relationship) { r.SelectorPath = "" })}, "required for kind selects"},
		"selects with path":    {[]Relationship{sel(func(r *Relationship) { r.Path = "spec.x" })}, "not fields of kind selects"},
		"selects wrong target": {[]Relationship{sel(func(r *Relationship) { r.To.Kind = "Secret" })}, "Service or Pod"},
	}
	for name, c := range cases {
		rep := Validate(relDef(c.rs...))
		if rep.OK() {
			t.Errorf("%s: accepted", name)
			continue
		}
		found := false
		for _, e := range rep.Errors() {
			found = found || strings.Contains(e.Path+" "+e.Message, c.want)
		}
		if !found {
			t.Errorf("%s: want an error mentioning %q, got %v", name, c.want, rep.Errors())
		}
	}
}

func TestSplitFieldPath(t *testing.T) {
	for in, want := range map[string]string{
		"spec.secretName": "spec|secretName",
		"metadata.annotations[example.io/auth-secret]": "metadata|annotations|example.io/auth-secret",
		"spec.a[b.c].d": "spec|a|b.c|d",
	} {
		got, err := SplitFieldPath(in)
		if err != nil || strings.Join(got, "|") != want {
			t.Errorf("%q = %v %v, want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"", " x", "a.", ".a", "a[]", "a[b", "a]b", "a[b]c"} {
		if _, err := SplitFieldPath(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSplitListFieldPath(t *testing.T) {
	for in, want := range map[string]string{
		"spec.secretName": "spec|secretName",
		"spec.endpoints[].tlsConfig.ca.secret.name": "spec|endpoints|[]|tlsConfig|ca|secret|name",
		"spec.a[].b[].c": "spec|a|[]|b|[]|c",
		"metadata.annotations[example.io/x]": "metadata|annotations|example.io/x",
	} {
		got, err := SplitListFieldPath(in)
		if err != nil || strings.Join(got, "|") != want {
			t.Errorf("%q = %v %v, want %s", in, got, err, want)
		}
	}
	for _, bad := range []string{"[]", "[].a", "a[][]", "a[]b", "a.[]", "a[", "a[]."} {
		if _, err := SplitListFieldPath(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	// only a relationship's path iterates lists
	if _, err := SplitFieldPath("spec.a[].b"); err == nil {
		t.Error("SplitFieldPath accepted []")
	}
}

func TestRelationshipsSchemaAndParse(t *testing.T) {
	schema := compileSchema(t, filepath.Join("..", "..", "schemas", "product-definition.schema.json"))
	doc := `apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: demo
name: demo
versioning: {scheme: semver, tagPrefix: v}
sources:
  - id: tags
    roles: [versions]
    locator: {kind: git-tags, repository: github.com/o/r}
artifacts: []
relationships:
  - id: cert-secret
    kind: produces
    from: {group: demo.io, kind: Cert}
    to: {kind: Secret}
    path: spec.secretName
    summary: a Cert creates its Secret
    references:
      - url: https://example.io/docs/api
        quote: spec.secretName names the Secret.
`
	check := func(doc string) error {
		var v any
		if err := yaml.Unmarshal([]byte(doc), &v); err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(v)
		inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
		if err != nil {
			t.Fatal(err)
		}
		return schema.Validate(inst)
	}
	if err := check(doc); err != nil {
		t.Fatalf("schema rejects a valid relationship: %v", err)
	}
	d, err := Parse([]byte(doc))
	if err != nil || len(d.Relationships) != 1 || d.Relationships[0].Path != "spec.secretName" {
		t.Fatalf("parse: %v %+v", err, d)
	}
	for name, bad := range map[string]string{
		"unknown kind":   strings.Replace(doc, "kind: produces", "kind: owns", 1),
		"no references":  strings.Replace(doc, "    references:\n      - url: https://example.io/docs/api\n        quote: spec.secretName names the Secret.\n", "", 1),
		"unknown field":  strings.Replace(doc, "path: spec.secretName", "path: spec.secretName\n    bogus: 1", 1),
		"http reference": strings.Replace(doc, "https://example.io", "http://example.io", 1),
	} {
		if check(bad) == nil {
			t.Errorf("schema accepts a relationship with %s", name)
		}
	}
}
