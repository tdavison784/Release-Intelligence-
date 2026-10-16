package relations

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func scoped(ids ...string) []domain.ChangeItem {
	var out []domain.ChangeItem
	for _, id := range ids {
		out = append(out, domain.ChangeItem{ID: id, InScope: true})
	}
	return out
}

func TestExposureRootsFromRenderedChangesThenJoinMatches(t *testing.T) {
	f := newFixture(t)
	f.file("app/deploy.yaml", deployment)
	f.file("app/svc.yaml", service)
	f.render("argocd:clusters/a.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: other, namespace: shop}\n",
		domain.RenderedChange{Class: "field-changed", Object: "apps/v1/Deployment/shop/web", Path: "spec.replicas", ChangeItems: []string{"ci-A"}, Correlation: "subject"},
		domain.RenderedChange{Class: "field-changed", Object: "apps/v1/Deployment/shop/web", Path: "spec.paused", ChangeItems: []string{"ci-A"}, Correlation: "subject"},
		domain.RenderedChange{Class: "field-changed", Object: "core/v1/Service/shop/web", Path: "spec.type", ChangeItems: []string{"ci-OUT"}, Correlation: "subject"},
		domain.RenderedChange{Class: "field-changed", Object: "core/v1/ConfigMap/shop/other", Path: "data.x"}, // uncorrelated: no root
		domain.RenderedChange{Class: "field-changed", Object: "core/v1/Service/shop/unknown", Path: "spec", ChangeItems: []string{"ci-A"}},
	)
	f.b.Change.Items = append(scoped("ci-A", "ci-B"), domain.ChangeItem{ID: "ci-OUT", InScope: false})
	g := f.build()

	report := &domain.ImpactReport{Findings: []domain.ImpactFinding{
		// grounded by the join: Service reached by item B, and a node the render already rooted for A
		{ID: "imp-1", ChangeID: "chg-b", Matches: []domain.ImpactMatch{{Subject: "shop/Service/web"}, {Subject: "shop/Deployment/web"}, {Subject: "no such"}}},
		{ID: "imp-2", ChangeID: "chg-a", Matches: []domain.ImpactMatch{{Subject: "apps/v1/Deployment/shop/web"}}},
		{ID: "imp-3", ChangeID: "chg-b"}, // no matches: not an exposure
	}}
	f.b.Change.Items = append(f.b.Change.Items, domain.ChangeItem{ID: domain.ChangeItemID("chg-b", "", ""), InScope: true}, domain.ChangeItem{ID: domain.ChangeItemID("chg-a", "", ""), InScope: true})

	roots, err := NewExposure().Exposure(context.Background(), f.b, g, report)
	if err != nil {
		t.Fatal(err)
	}
	node := func(key string) string { return view{t, g}.mustNode(key).ID }
	type r struct{ item, node, rule string }
	var got []r
	for _, x := range roots {
		got = append(got, r{x.ChangeItem, x.Node, x.Rule})
		if (x.RenderedChange == "") == (x.ImpactFinding == "") {
			t.Errorf("exactly one grounding expected: %+v", x)
		}
	}
	want := map[r]bool{
		{"ci-A", node("core/v1/Service/shop/unknown"), RuleRenderedChange}:                         true, // changed by the render though absent from the TO render
		{"ci-A", node("apps/v1/Deployment/shop/web"), RuleRenderedChange}:                          true, // two rendered changes, one root
		{domain.ChangeItemID("chg-b", "", ""), node("core/v1/Service/shop/web"), RuleJoinMatch}:    true,
		{domain.ChangeItemID("chg-b", "", ""), node("apps/v1/Deployment/shop/web"), RuleJoinMatch}: true,
		{domain.ChangeItemID("chg-a", "", ""), node("apps/v1/Deployment/shop/web"), RuleJoinMatch}: true,
	}
	if len(got) != len(want) {
		t.Fatalf("roots = %+v", got)
	}
	for _, x := range got {
		if !want[x] {
			t.Errorf("unexpected root %+v", x)
		}
	}
	for i := 1; i < len(roots); i++ {
		a, b := roots[i-1], roots[i]
		if a.ChangeItem > b.ChangeItem || (a.ChangeItem == b.ChangeItem && a.Node >= b.Node) {
			t.Fatalf("roots not sorted/unique: %+v", roots)
		}
	}
	// The Service root reaches nothing downstream here; the Deployment is reached from the Secret... via paths in the traversal.
	res := traverse(t, g, roots, domain.DefaultTraversalBound)
	if len(res.Paths) == 0 {
		t.Error("expected paths")
	}
}

func TestExposureRenderedChangeWinsOverJoinMatch(t *testing.T) {
	f := newFixture(t)
	f.file("app/deploy.yaml", deployment)
	f.render("argocd:x.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: o}\n",
		domain.RenderedChange{Class: "field-changed", Object: "apps/v1/Deployment/shop/web", Path: "spec", ChangeItems: []string{domain.ChangeItemID("chg", "", "")}})
	f.b.Change.Items = scoped(domain.ChangeItemID("chg", "", ""))
	g := f.build()
	roots, _ := NewExposure().Exposure(context.Background(), f.b, g, &domain.ImpactReport{Findings: []domain.ImpactFinding{
		{ID: "imp-1", ChangeID: "chg", Matches: []domain.ImpactMatch{{Subject: "apps/v1/Deployment/shop/web"}}},
	}})
	if len(roots) != 1 || roots[0].Rule != RuleRenderedChange {
		t.Fatalf("roots = %+v", roots)
	}
}

func TestExposureNoSignalNoRoots(t *testing.T) {
	f := newFixture(t)
	f.file("app/deploy.yaml", deployment)
	g := f.build()
	roots, err := NewExposure().Exposure(context.Background(), f.b, g, nil)
	if err != nil || len(roots) != 0 {
		t.Fatalf("roots = %+v err = %v", roots, err)
	}
	if _, err := NewExposure().Exposure(context.Background(), nil, g, nil); err == nil {
		t.Fatal("nil bundle must be rejected")
	}
}

// End to end on the groundtruth environment: a rendered change on a Certificate reaches the
// Deployment that mounts the Secret it produces — as an unresolved (inferred) path.
func TestConsumerPathCertificateSecretDeployment(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "eval", "investigations", "cert-manager-1.17-1.18--payments-eu", "env")
	f := newFixture(t)
	if f.loadEnvDir(dir) == 0 {
		t.Skip("no fixture")
	}
	f.render("argocd:clusters/prod/cert-manager.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: x, namespace: cert-manager}\n",
		domain.RenderedChange{Class: "field-changed", Object: "cert-manager.io/v1/Certificate/payments/ledger-tls", Path: "spec.privateKey.rotationPolicy", ChangeItems: []string{"ci-B1"}, Correlation: "subject"})
	f.b.Change.Items = scoped("ci-B1")
	g := f.build()
	roots, err := NewExposure().Exposure(context.Background(), f.b, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := traverse(t, g, roots, domain.DefaultTraversalBound)
	v := view{t, g}
	secret, dep := v.mustNode("core/v1/Secret/payments/ledger-tls").ID, v.mustNode("apps/v1/Deployment/payments/ledger").ID
	found := false
	for _, p := range res.Paths {
		hasSecret, hasDep := false, false
		for _, n := range p.Nodes {
			hasSecret = hasSecret || n == secret
			hasDep = hasDep || n == dep
		}
		if hasSecret && hasDep {
			found = true
			if p.Status != domain.EdgeInferred {
				t.Errorf("path through the produced Secret must be inferred, got %s", p.Status)
			}
		}
	}
	if !found {
		t.Fatalf("no path Certificate → Secret → Deployment; paths: %d", len(res.Paths))
	}
	if res.Complete || res.CompleteFor("ci-B1") {
		t.Error("an inferred edge on a path must leave the traversal incomplete (fail closed)")
	}
}

// Controller-behaviour and unknown items — whose effect no render shows — are
// rooted at every environment object of a kind the product's controller
// reconciles; render-decisive and out-of-scope items are not, placeholders
// (referenced, undeclared objects) never are, and another product's
// declaration does not apply.
func TestExposureReconciledKinds(t *testing.T) {
	f := newFixture(t)
	f.file("app/deploy.yaml", deployment)
	f.file("app/svc.yaml", service)
	f.file("app/ing.yaml", ingress)
	beh := domain.ChangeItem{ID: "ci-beh", InScope: true, Manifestation: domain.ManifestControllerBehaviour}
	unk := domain.ChangeItem{ID: "ci-unk", InScope: true, Manifestation: domain.ManifestUnknown}
	ren := domain.ChangeItem{ID: "ci-ren", InScope: true, Manifestation: domain.ManifestRenderedConfig}
	out := domain.ChangeItem{ID: "ci-out", InScope: false, Manifestation: domain.ManifestControllerBehaviour}
	f.b.Change.Items = []domain.ChangeItem{beh, unk, ren, out}
	g := f.build()
	node := func(key string) string { return view{t, g}.mustNode(key).ID }
	secret := view{t, g}.mustNode("core/v1/Secret/shop/web-tls")
	if !secret.Placeholder {
		t.Fatal("fixture: the referenced TLS Secret is a placeholder")
	}
	m := NewExposure(
		WithReconciles("widget", catalog.ObjectType{Group: "networking.k8s.io", Kind: "Ingress"}, catalog.ObjectType{Kind: "Secret"}),
		WithReconciles("other", catalog.ObjectType{Kind: "Service"}))
	roots, err := m.Exposure(context.Background(), f.b, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, r := range roots {
		got[r.ChangeItem+" "+r.Node] = true
		if r.Rule != RuleReconciledKind || r.RenderedChange != "" || r.ImpactFinding != "" {
			t.Errorf("unexpected root %+v", r)
		}
	}
	ing := node("networking.k8s.io/v1/Ingress/shop/web")
	want := map[string]bool{"ci-beh " + ing: true, "ci-unk " + ing: true}
	if len(got) != len(want) {
		t.Fatalf("roots %v, want %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Fatalf("missing root %s in %v", k, got)
		}
	}
	res := traverse(t, g, roots, domain.DefaultTraversalBound)
	if len(res.PathsOf("ci-beh")) == 0 {
		t.Fatal("a reconciled-kind root starts impact paths")
	}
	// without the declaration, nothing changes
	if roots, _ := NewExposure().Exposure(context.Background(), f.b, g, nil); len(roots) != 0 {
		t.Fatalf("roots without reconciles: %v", roots)
	}
}

// Every checked-in product's reconciled kinds load through the catalog.
func TestExposureReconcilesFromCatalog(t *testing.T) {
	c, err := catalog.LoadDir(filepath.Join("..", "..", "..", "products"))
	if err != nil {
		t.Fatal(err)
	}
	m := NewExposure(WithReconcilesFrom(c)).(exposure)
	for _, p := range []string{"cert-manager", "ingress-nginx", "prometheus-operator", "kube-prometheus-stack"} {
		if len(m.reconciles[p]) == 0 {
			t.Errorf("%s declares no reconciled kinds", p)
		}
	}
	if !m.reconciles["ingress-nginx"]["networking.k8s.io|Ingress"] || !m.reconciles["cert-manager"]["cert-manager.io|Certificate"] {
		t.Fatalf("reconciles %v", m.reconciles)
	}
}
