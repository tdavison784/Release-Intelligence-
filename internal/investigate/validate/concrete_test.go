package validate

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// Concrete-failure paths of RenderDiff: the same correlated rendered change
// fails only where it breaks something in the TO render.

const (
	webKey    = "apps/v1/Deployment/apps/web"
	widgetKey = "gizmo.example.io/v1/Widget/apps/w1"
)

func webDeploy(secret string) string {
	return `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: apps}
spec:
  template:
    spec:
      containers:
        - name: web
          image: example.invalid/web:1
      volumes:
        - name: creds
          secret:
            secretName: ` + secret + "\n"
}

func secretObj(name, ns string) string {
	return "apiVersion: v1\nkind: Secret\nmetadata: {name: " + name + ", namespace: " + ns + "}\n"
}

// widgetCRD renders the Widget CRD; served lists "name:served" versions,
// each with the schema spec.size (integer, required) and, when rotation is
// set, spec.rotation (enum Never|Always).
func widgetCRD(rotation bool, served ...string) string {
	s := `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: widgets.gizmo.example.io}
spec:
  group: gizmo.example.io
  names: {kind: Widget, plural: widgets}
  scope: Namespaced
  versions:
`
	for _, v := range served {
		name, on, _ := strings.Cut(v, ":")
		s += `    - name: ` + name + `
      served: ` + on + `
      storage: ` + on + `
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              required: [size]
              properties:
                size: {type: integer}
`
		if rotation {
			s += "                rotation: {type: string, enum: [Never, Always]}\n"
		}
	}
	return s
}

// A rendered change that removes an object a surviving object references:
// the reference resolved at FROM and dangles at TO ⇒ fail naming referrer,
// field path and target.
func TestRenderDiffRemovedReferencedObjectFails(t *testing.T) {
	f, item := renderFx(t)
	ids := f.pair("argocd:apps/web.yaml", stream(webDeploy("db"), secretObj("db", "apps")), webDeploy("db"),
		rc("resource-removed", "core/v1/Secret/apps/db", "", "", "", "subject", item))
	b := f.bundle()
	r := one(t, run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationFail, webKey+" (the TO render) spec.template.spec.volumes[0:creds].secret.secretName → Secret apps/db")
	wantOutcome(t, r, domain.ValidationFail, "resolved at FROM (to core/v1/Secret/apps/db)")
	if len(r.RenderedChanges) != 1 || r.RenderedChanges[0] != ids[0] {
		t.Fatalf("the fail cites the removal: %+v", r.RenderedChanges)
	}
}

// The same rendered change (a referrer now names another Secret) in two
// environments: where the environment provides the new Secret it is exposure
// only (error); where nothing provides it, the new reference dangles (fail).
// Same-namespace semantics: a Secret of that name in another namespace does
// not provide it.
func TestRenderDiffNewDanglingReferenceTellsEnvironmentsApart(t *testing.T) {
	cases := []struct {
		name    string
		file    string
		outcome domain.ValidationOutcomeKind
		detail  string
	}{
		{"environment provides it", "namespace: apps\nresources: [secret.yaml]\n", domain.ValidationError, "no concrete failure found"},
		{"other namespace only", "namespace: other\nresources: [secret.yaml]\n", domain.ValidationFail, "a reference the upgrade introduces"},
		{"nothing provides it", "", domain.ValidationFail, "Secret apps/db-v2"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, item := renderFx(t)
			if c.file != "" {
				f.file("apps/kustomization.yaml", c.file)
				f.file("apps/secret.yaml", "apiVersion: v1\nkind: Secret\nmetadata: {name: db-v2}\n")
			}
			f.pair("argocd:apps/web.yaml", stream(webDeploy("db"), secretObj("db", "apps")), stream(webDeploy("db-v2"), secretObj("db", "apps")),
				rc("field-changed", webKey, "spec.template.spec.volumes[name=creds].secret.secretName", `"db"`, `"db-v2"`, "values-default", item))
			b := f.bundle()
			r := one(t, run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, c.outcome, c.detail)
		})
	}
}

// A reference that dangled before the upgrade too is not the upgrade's
// doing; an optional one decides nothing.
func TestRenderDiffPreexistingAndOptionalReferencesDoNotFail(t *testing.T) {
	f, item := renderFx(t)
	f.pair("argocd:apps/web.yaml", webDeploy("external"), webDeploy("external")+"      serviceAccountName: web\n",
		rc("field-added", webKey, "spec.template.spec.serviceAccountName", "", `"web"`, "subject", item))
	b := f.bundle()
	r := one(t, run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	// the new serviceAccountName dangles: that one fails; the pre-existing Secret is reported as not the upgrade's
	wantOutcome(t, r, domain.ValidationFail, "ServiceAccount apps/web")
	if strings.Contains(r.Detail, "Secret apps/external") {
		t.Fatalf("a pre-existing dangling reference must not be a failure: %s", r.Detail)
	}

	f2, item2 := renderFx(t)
	opt := strings.Replace(webDeploy("db-v2"), "secretName: db-v2\n", "secretName: db-v2\n            optional: true\n", 1)
	f2.pair("argocd:apps/web.yaml", webDeploy("db"), opt,
		rc("field-changed", webKey, "spec.template.spec.volumes[name=creds].secret.secretName", `"db"`, `"db-v2"`, "subject", item2))
	b2 := f2.bundle()
	r2 := one(t, run(t, RenderDiff(WithStore(f2.st)), investigate.ValidationInput{Bundle: b2, ChangeItem: item2}))
	wantOutcome(t, r2, domain.ValidationError, "optional reference")
}

// A custom resource invalid against the CRD the TO render ships (a field the
// new schema drops, a type or enum it no longer accepts, a newly required
// field) ⇒ fail; a violation the FROM render already had is not new.
func TestRenderDiffCustomResourceInvalidAgainstRenderedCRD(t *testing.T) {
	const w = "apiVersion: gizmo.example.io/v1\nkind: Widget\nmetadata: {name: w1, namespace: apps}\nspec:\n"
	cases := []struct {
		name       string
		fromCR     string
		toCR       string
		toRotation bool
		outcome    domain.ValidationOutcomeKind
		detail     string
	}{
		{"field dropped by the new schema", w + "  size: 3\n  rotation: Always\n", w + "  size: 3\n  rotation: Always\n", false, domain.ValidationFail, "field spec.rotation is not in the CRD schema"},
		{"enum value not accepted", w + "  size: 3\n  rotation: Always\n", w + "  size: 3\n  rotation: Sometimes\n", true, domain.ValidationFail, "spec.rotation = Sometimes is not one of the allowed values"},
		{"wrong type", w + "  size: 3\n", w + "  size: three\n", true, domain.ValidationFail, "spec.size must be an integer, got a string"},
		{"required field missing", w + "  size: 3\n", w + "  rotation: Never\n", true, domain.ValidationFail, "required field spec.size is missing"},
		{"valid", w + "  size: 3\n", w + "  size: 4\n  rotation: Never\n", true, domain.ValidationError, "1 custom resource checked"},
		{"pre-existing violation", w + "  size: three\n", w + "  size: three\n  rotation: Never\n", true, domain.ValidationError, "no new violation"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, item := renderFx(t)
			f.pair("argocd:apps/w.yaml", stream(widgetCRD(true, "v1:true"), c.fromCR), stream(widgetCRD(c.toRotation, "v1:true"), c.toCR),
				rc("field-changed", widgetKey, "spec", "", "", "subject", item))
			b := f.bundle()
			r := one(t, run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, c.outcome, c.detail)
		})
	}
}

// A rendered change on the CRD itself checks every custom resource of its
// kind — including environment files the chart does not render — and an
// apiVersion the TO CRD no longer serves (or no longer defines) fails.
func TestRenderDiffAPIVersionNoLongerServed(t *testing.T) {
	for _, to := range []string{"v1:false", "v2:true"} {
		t.Run(to, func(t *testing.T) {
			f, item := renderFx(t)
			f.file("apps/kustomization.yaml", "resources: [w1.yaml]\n")
			f.file("apps/w1.yaml", widget("w1", "apps", ""))
			versions := []string{"v2:true"}
			if to == "v1:false" {
				versions = append([]string{to}, versions...)
			}
			f.pair("argocd:apps/w.yaml", widgetCRD(true, "v1:true", "v2:true"), widgetCRD(true, versions...),
				rc("field-changed", crdKey, "spec.versions[name=v1].served", "true", "false", "subject", item))
			b := f.bundle()
			r := one(t, run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, domain.ValidationFail, "apiVersion gizmo.example.io/v1 is no")
			wantOutcome(t, r, domain.ValidationFail, widgetKey+" apiVersion")
		})
	}
}

// A declared relationship (WithRelations) makes a monitor-style credentials
// reference checkable: the change that renames the Secret dangles; without
// the rule the same change is exposure only.
func TestRenderDiffDeclaredReferenceDangles(t *testing.T) {
	const probe = "apiVersion: gizmo.example.io/v1\nkind: Probe\nmetadata: {name: p, namespace: mon}\nspec:\n  endpoints:\n    - auth: {credentials: {name: %s, key: token}}\n"
	rule := relations.WithRelationships("gizmo", catalog.Relationship{ID: "probe-credentials", Kind: catalog.RelReferences,
		From: catalog.ObjectType{Group: "gizmo.example.io", Kind: "Probe"}, To: catalog.ObjectType{Kind: "Secret"},
		Path: "spec.endpoints[].auth.credentials.name"})
	for _, declared := range []bool{true, false} {
		t.Run(map[bool]string{true: "declared", false: "unknown to the rules"}[declared], func(t *testing.T) {
			f, item := renderFx(t)
			f.pair("argocd:apps/mon.yaml", stream(strings.Replace(probe, "%s", "tok", 1), secretObj("tok", "mon")),
				stream(strings.Replace(probe, "%s", "probe-token", 1), secretObj("tok", "mon")),
				rc("field-changed", "gizmo.example.io/v1/Probe/mon/p", "spec.endpoints[0].auth.credentials.name", `"tok"`, `"probe-token"`, "values-default", item))
			b := f.bundle()
			opts := []Option{WithStore(f.st)}
			if declared {
				opts = append(opts, WithRelations(rule))
			}
			r := one(t, run(t, RenderDiff(opts...), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			if declared {
				wantOutcome(t, r, domain.ValidationFail, "spec.endpoints[0].auth.credentials.name → Secret mon/probe-token [relationship:gizmo/probe-credentials]")
			} else {
				wantOutcome(t, r, domain.ValidationError, "exposure:")
			}
		})
	}
}

// A list reordering (a removed volume shifts the others) does not make a
// pre-existing dangle new: references are compared by target, and a target
// that dangled at FROM too (e.g. a Secret a hook Job creates at runtime) is
// not the upgrade's doing — also for a new referrer naming it.
func TestRenderDiffReorderedPreexistingReferenceDoesNotFail(t *testing.T) {
	two := strings.Replace(webDeploy("hook-made"), "      volumes:\n", "      volumes:\n        - name: otel\n          configMap: {name: otel}\n", 1)
	other := strings.Replace(webDeploy("hook-made"), "name: web,", "name: web2,", 1)
	f, item := renderFx(t)
	f.pair("argocd:apps/web.yaml", stream(two, configMap("otel", "apps", map[string]string{"a": "b"})), stream(webDeploy("hook-made"), other),
		rc("field-removed", webKey, "spec.template.spec.volumes[name=otel]", "", "", "values-default", item),
		rc("resource-added", "apps/v1/Deployment/apps/web2", "", "", "", "values-default", item))
	b := f.bundle()
	r := one(t, run(t, RenderDiff(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationError, "dangled at FROM too")
}
