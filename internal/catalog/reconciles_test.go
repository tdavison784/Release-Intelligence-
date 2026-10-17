package catalog

import (
	"path/filepath"
	"strings"
	"testing"
)

func validReconcile() Reconcile {
	return Reconcile{ID: "certs", Kinds: []ObjectType{{Group: "demo.io", Kind: "Cert"}}, Summary: "the controller issues every Cert",
		References: []ConfigReference{{URL: "https://example.io/docs/cert", Quote: "the controller issues every Cert"}}}
}

func recDef(rs ...Reconcile) *ProductDefinition {
	d := lifecycleDef()
	d.Reconciles = rs
	return d
}

func TestReconcilesValid(t *testing.T) {
	two := validReconcile()
	two.ID, two.Kinds = "core", []ObjectType{{Kind: "ConfigMap"}, {Group: "demo.io", Kind: "Issuer"}}
	d := recDef(validReconcile(), two)
	if rep := Validate(d); !rep.OK() {
		t.Fatalf("valid reconciles rejected: %v", rep.Errors())
	}
	if got := d.ReconciledKinds(); len(got) != 3 || got[1].Kind != "ConfigMap" {
		t.Fatalf("reconciled kinds %v", got)
	}
}

func TestReconcilesRejectBadEntries(t *testing.T) {
	mut := func(f func(*Reconcile)) Reconcile {
		r := validReconcile()
		f(&r)
		return r
	}
	other := validReconcile()
	other.ID = "again"
	cases := map[string]struct {
		rs   []Reconcile
		want string
	}{
		"bad id":         {[]Reconcile{mut(func(r *Reconcile) { r.ID = "Bad_ID" })}, "lowercase DNS label"},
		"duplicate id":   {[]Reconcile{validReconcile(), validReconcile()}, "duplicate reconciles id"},
		"no kinds":       {[]Reconcile{mut(func(r *Reconcile) { r.Kinds = nil })}, "at least one kind"},
		"empty kind":     {[]Reconcile{mut(func(r *Reconcile) { r.Kinds = []ObjectType{{Group: "demo.io"}} })}, "kinds[0].kind"},
		"duplicate kind": {[]Reconcile{validReconcile(), other}, "already declared"},
		"no summary":     {[]Reconcile{mut(func(r *Reconcile) { r.Summary = " " })}, "summary"},
		"no references":  {[]Reconcile{mut(func(r *Reconcile) { r.References = nil })}, "at least one upstream"},
		"http reference": {[]Reconcile{mut(func(r *Reconcile) { r.References = []ConfigReference{{URL: "http://x.io/a", Quote: "q"}} })}, "https URL"},
		"eval reference": {[]Reconcile{mut(func(r *Reconcile) {
			r.References = []ConfigReference{{URL: "https://github.com/o/r/blob/main/eval/investigations/x/env/a.yaml", Quote: "q"}}
		})}, "evaluation data"},
		"empty quote": {[]Reconcile{mut(func(r *Reconcile) { r.References = []ConfigReference{{URL: "https://x.io/a", Quote: ""}} })}, "verbatim quote"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rep := Validate(recDef(c.rs...))
			found := false
			for _, i := range rep.Errors() {
				found = found || strings.Contains(i.Path+": "+i.Message, c.want)
			}
			if rep.OK() || !found {
				t.Fatalf("errors %v do not mention %q", rep.Errors(), c.want)
			}
		})
	}
}

func TestReconcilesSchema(t *testing.T) {
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
reconciles:
  - id: certs
    kinds: [{group: demo.io, kind: Cert}]
    summary: the controller issues every Cert
    references:
      - url: https://example.io/docs/cert
        quote: the controller issues every Cert
`
	if err := schemaCheck(schema, []byte(doc)); err != nil {
		t.Fatalf("schema rejects valid reconciles: %v", err)
	}
	d, err := Parse([]byte(doc))
	if err != nil || len(d.Reconciles) != 1 || d.Reconciles[0].Kinds[0].Kind != "Cert" {
		t.Fatalf("parse: %v %+v", err, d)
	}
	for name, bad := range map[string]string{
		"no kinds":      strings.Replace(doc, "    kinds: [{group: demo.io, kind: Cert}]\n", "", 1),
		"empty kinds":   strings.Replace(doc, "[{group: demo.io, kind: Cert}]", "[]", 1),
		"unknown field": strings.Replace(doc, "    summary:", "    bogus: 1\n    summary:", 1),
	} {
		if schemaCheck(schema, []byte(bad)) == nil {
			t.Errorf("schema accepts reconciles with %s", name)
		}
	}
}
