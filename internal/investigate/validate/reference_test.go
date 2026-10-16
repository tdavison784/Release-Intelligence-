package validate

import (
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
