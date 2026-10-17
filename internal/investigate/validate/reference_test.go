package validate

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

const (
	secretKey   = "core/v1/Secret/gizmo/gizmo-ca"
	consumerKey = "apps/v1/Deployment/apps/web"
)

// refFx: a Secret the chart renders, referenced by a consumer Deployment.
func refFx(t *testing.T, changes ...domain.RenderedChange) (*fx, string) {
	f := newFx(t)
	item := f.change(domain.Change{ID: "chg-ca", Category: domain.CategoryRemoval, Breaking: true, Title: "the CA secret is no longer created",
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: "normalize@v1", Rule: "section:breaking", Confidence: domain.ConfidenceHigh}}, behaviour)
	f.pair("argocd:apps/gizmo.yaml", controller(), controller(), changes...)
	return f, item
}

func refGraph(t *testing.T, b *domain.EvidenceBundle, item string, status domain.EdgeStatus, placeholder bool) (*domain.RelationshipGraph, *domain.TraversalResult, string) {
	g := newGraph(t, b)
	sec := g.node(domain.NodeConfigObject, secretKey, placeholder)
	web := g.node(domain.NodeWorkload, consumerKey, false)
	g.edge(domain.EdgeReferences, sec, web, status)
	g.root(item, sec)
	p := g.path(item, sec, web)
	gr, tr := g.done()
	return gr, tr, p
}

func TestReferenceDangles(t *testing.T) {
	item := domain.ChangeItemID("chg-ca", "", "")
	f, _ := refFx(t, rc("resource-removed", secretKey, "", "", "", "subject", item))
	b := f.bundle()
	g, tr, p := refGraph(t, b, item, domain.EdgeEstablished, false)
	r := one(t, run(t, StaticReference(), investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationFail, "no longer rendered")
	if !contains(r.Paths, p) || len(r.RenderedChanges) != 1 {
		t.Fatalf("fail cites the path and the removal: %+v", r)
	}
}

func TestReferenceIntact(t *testing.T) {
	cases := []struct {
		name    string
		changes []domain.RenderedChange
	}{
		{"nothing removed", nil},
		// the referrer itself changes with the removal (a rename the chart carries through)
		{"referrer updated too", []domain.RenderedChange{
			rc("resource-removed", secretKey, "", "", "", ""),
			rc("field-changed", consumerKey, "spec.template.spec.volumes[name=ca].secret.secretName", `"gizmo-ca"`, `"gizmo-ca-v2"`, ""),
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, item := refFx(t, c.changes...)
			b := f.bundle()
			g, tr, p := refGraph(t, b, item, domain.EdgeEstablished, false)
			r := one(t, run(t, StaticReference(), investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}))
			wantOutcome(t, r, domain.ValidationPass, "")
			if !contains(r.Paths, p) || r.ClearsBehaviour() {
				t.Fatalf("pass covers the path and never clears a behaviour item: %+v", r)
			}
		})
	}
}

func TestReferenceUndecided(t *testing.T) {
	cases := []struct {
		name   string
		build  func(t *testing.T) investigate.ValidationInput
		detail string
	}{
		{"no graph", func(t *testing.T) investigate.ValidationInput {
			f, item := refFx(t)
			return investigate.ValidationInput{Bundle: f.bundle(), ChangeItem: item}
		}, "no relationship graph"},
		{"no paths", func(t *testing.T) investigate.ValidationInput {
			f, item := refFx(t)
			b := f.bundle()
			g, tr := newGraph(t, b).done()
			return investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}
		}, "no impact paths"},
		{"truncated", func(t *testing.T) investigate.ValidationInput {
			f, item := refFx(t)
			b := f.bundle()
			gb := newGraph(t, b)
			n := gb.node(domain.NodeConfigObject, secretKey, false)
			gb.root(item, n)
			gb.path(item, n)
			gb.trunc = true
			g, tr := gb.done()
			return investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}
		}, "truncated"},
		{"incomplete render", func(t *testing.T) investigate.ValidationInput {
			f, item := refFx(t)
			f.brokenPair("argocd:apps/other.yaml", "values-incomplete")
			b := f.bundle()
			g, tr, _ := refGraph(t, b, item, domain.EdgeEstablished, false)
			return investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}
		}, "values-incomplete"},
		{"inferred edge", func(t *testing.T) investigate.ValidationInput {
			f, item := refFx(t)
			b := f.bundle()
			g, tr, _ := refGraph(t, b, item, domain.EdgeInferred, false)
			return investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}
		}, "is inferred"},
		{"placeholder node", func(t *testing.T) investigate.ValidationInput {
			f, item := refFx(t)
			b := f.bundle()
			g, tr, _ := refGraph(t, b, item, domain.EdgeEstablished, true)
			return investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}
		}, "absent from the snapshot"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			in := c.build(t)
			r := one(t, run(t, StaticReference(), in))
			wantOutcome(t, r, domain.ValidationNotRun, c.detail)
		})
	}
}

func TestReferenceNeverPassesRenderedUnexplained(t *testing.T) {
	f := newFx(t)
	item, _ := f.unexplained("field-changed", secretKey, "type")
	f.pair("argocd:apps/gizmo.yaml", controller(), controller())
	b := f.bundle()
	g, tr, _ := refGraph(t, b, item, domain.EdgeEstablished, false)
	r := one(t, run(t, StaticReference(), investigate.ValidationInput{Bundle: b, Graph: g, Traversal: tr, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationNotRun, "does not show the unexplained rendered change harmless")
}

func TestReferenceRootOnlyPathIsNotPassed(t *testing.T) {
	f, item := refFx(t)
	b := f.bundle()
	g := newGraph(t, b)
	n := g.node(domain.NodeConfigObject, secretKey, false)
	g.root(item, n)
	g.path(item, n)
	gr, tr := g.done()
	r := one(t, run(t, StaticReference(), investigate.ValidationInput{Bundle: b, Graph: gr, Traversal: tr, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationNotRun, "no reference on it")
}

// --- render-level references (TO vs FROM) --------------------------------------

// webRefFx: the item removes Secret apps/db in the rendered deployment;
// Deployment apps/web mounts it. files are extra environment files.
func webRefFx(t *testing.T, files map[string]string, from, to string) (*domain.EvidenceBundle, *fx, string) {
	f, item := renderFx(t)
	for p, c := range files {
		f.file(p, c)
	}
	f.pair("argocd:apps/web.yaml", from, to, rc("resource-removed", "core/v1/Secret/apps/db", "", "", "", "subject", item))
	return f.bundle(), f, item
}

// Same-namespace semantics: the removal dangles the reference unless the
// environment provides the Secret in the referrer's namespace; a Secret of
// that name in another namespace does not count.
func TestReferenceRenderDanglingNamespaces(t *testing.T) {
	cases := []struct {
		name    string
		files   map[string]string
		outcome domain.ValidationOutcomeKind
		detail  string
	}{
		{"same namespace, nothing provides it", nil, domain.ValidationFail,
			webKey + " (the TO render) spec.template.spec.volumes[0:creds].secret.secretName → Secret apps/db [graph.k8s-refs@v1:volumes.secret]: resolved at FROM"},
		{"provided in another namespace", map[string]string{"other/kustomization.yaml": "namespace: other\nresources: [s.yaml]\n", "other/s.yaml": secretObj("db", "x")},
			domain.ValidationFail, "Secret apps/db"},
		{"environment provides it", map[string]string{"apps/kustomization.yaml": "namespace: apps\nresources: [s.yaml]\n", "apps/s.yaml": secretObj("db", "x")},
			domain.ValidationNotRun, "none newly dangling"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, f, item := webRefFx(t, c.files, stream(webDeploy("db"), secretObj("db", "apps")), webDeploy("db"))
			r := one(t, run(t, StaticReference(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, c.outcome, c.detail)
			if c.outcome == domain.ValidationFail && len(r.RenderedChanges) != 1 {
				t.Fatalf("the fail cites the removal: %+v", r)
			}
		})
	}
}

// With the graph: the fail covers the item's paths through the referrer;
// a path through an object whose reference cannot be decided is not passed.
func TestReferenceRenderWithGraph(t *testing.T) {
	t.Run("dangling", func(t *testing.T) {
		b, f, item := webRefFx(t, nil, stream(webDeploy("db"), secretObj("db", "apps")), webDeploy("db"))
		g := newGraph(t, b)
		sec := g.node(domain.NodeConfigObject, "core/v1/Secret/apps/db", false)
		web := g.node(domain.NodeWorkload, webKey, false)
		g.edge(domain.EdgeReferences, sec, web, domain.EdgeEstablished)
		g.root(item, sec)
		p := g.path(item, sec, web)
		gr, tr := g.done()
		r := one(t, run(t, StaticReference(WithStore(f.st)), investigate.ValidationInput{Bundle: b, Graph: gr, Traversal: tr, ChangeItem: item}))
		wantOutcome(t, r, domain.ValidationFail, "dangling in the TO render")
		if !contains(r.Paths, p) || !contains(r.Subjects, web) {
			t.Fatalf("the fail covers the path through the referrer: %+v", r)
		}
	})
	t.Run("unknown target", func(t *testing.T) {
		f, item := renderFx(t)
		opt := strings.Replace(webDeploy("db-v2"), "secretName: db-v2\n", "secretName: db-v2\n            optional: true\n", 1)
		f.pair("argocd:apps/web.yaml", webDeploy("db"), opt,
			rc("field-changed", webKey, "spec.template.spec.volumes[name=creds].secret.secretName", `"db"`, `"db-v2"`, "subject", item))
		b := f.bundle()
		g := newGraph(t, b)
		web := g.node(domain.NodeWorkload, webKey, false)
		svc := g.node(domain.NodeService, "core/v1/Service/apps/web", false)
		g.edge(domain.EdgeSelects, web, svc, domain.EdgeEstablished)
		g.root(item, web)
		g.path(item, web, svc)
		gr, tr := g.done()
		r := one(t, run(t, StaticReference(WithStore(f.st)), investigate.ValidationInput{Bundle: b, Graph: gr, Traversal: tr, ChangeItem: item}))
		wantOutcome(t, r, domain.ValidationNotRun, "cannot be decided")
		wantOutcome(t, r, domain.ValidationNotRun, "optional reference")
	})
}

// A reference that dangled before the upgrade too is not a failure; nor is a
// dangle on the path that no rendered change of the item causes.
func TestReferenceRenderPreexistingAndUnattributable(t *testing.T) {
	f, item := renderFx(t)
	f.pair("argocd:apps/web.yaml", webDeploy("external"), webDeploy("external"),
		rc("label-changed", webKey, "metadata.labels.tier", `"a"`, `"b"`, "subject", item))
	b := f.bundle()
	r := one(t, run(t, StaticReference(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
	wantOutcome(t, r, domain.ValidationNotRun, "dangled at FROM too")

	f2, item2 := renderFx(t)
	f2.pair("argocd:apps/web.yaml", stream(webDeploy("db"), secretObj("db", "apps")), webDeploy("db"),
		rc("resource-removed", "core/v1/Secret/apps/db", "", "", "", "")) // uncorrelated removal
	b2 := f2.bundle()
	g := newGraph(t, b2)
	web := g.node(domain.NodeWorkload, webKey, false)
	svc := g.node(domain.NodeService, "core/v1/Service/apps/web", false)
	g.edge(domain.EdgeSelects, web, svc, domain.EdgeEstablished)
	g.root(item2, web)
	g.path(item2, web, svc)
	gr, tr := g.done()
	r2 := one(t, run(t, StaticReference(WithStore(f2.st)), investigate.ValidationInput{Bundle: b2, Graph: gr, Traversal: tr, ChangeItem: item2}))
	wantOutcome(t, r2, domain.ValidationNotRun, "caused by no rendered change of this item")
}

// Explicit namespace fields (a *Ref with namespace) resolve where they say;
// a cluster-scoped target resolves from any namespace.
func TestReferenceRenderExplicitNamespaceAndClusterScope(t *testing.T) {
	const cr = `apiVersion: gizmo.example.io/v1
kind: Widget
metadata: {name: w1, namespace: apps}
spec:
  caRef: {kind: Secret, name: ca, namespace: %s}
  classRef: {kind: StorageClass, group: storage.k8s.io, name: fast}
`
	const objs = "apiVersion: v1\nkind: Secret\nmetadata: {name: ca, namespace: shared}\n---\napiVersion: storage.k8s.io/v1\nkind: StorageClass\nmetadata: {name: fast}\n"
	for _, c := range []struct {
		ns      string
		outcome domain.ValidationOutcomeKind
	}{{"shared", domain.ValidationNotRun}, {"apps", domain.ValidationFail}} {
		t.Run(c.ns, func(t *testing.T) {
			f, item := renderFx(t)
			f.pair("argocd:apps/w.yaml", stream(strings.Replace(cr, "%s", "shared", 1), objs), stream(strings.Replace(cr, "%s", c.ns, 1), objs),
				rc("field-changed", widgetKey, "spec.caRef.namespace", `"shared"`, `"`+c.ns+`"`, "subject", item))
			b := f.bundle()
			r := one(t, run(t, StaticReference(WithStore(f.st)), investigate.ValidationInput{Bundle: b, ChangeItem: item}))
			wantOutcome(t, r, c.outcome, "")
			if c.outcome == domain.ValidationFail && !strings.Contains(r.Detail, "spec.caRef → Secret apps/ca") {
				t.Fatalf("detail: %s", r.Detail)
			}
			if strings.Contains(r.Detail, "StorageClass") {
				t.Fatalf("a cluster-scoped target must resolve from any namespace: %s", r.Detail)
			}
		})
	}
}
