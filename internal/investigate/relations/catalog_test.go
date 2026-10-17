package relations

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func loadCatalog(t *testing.T) *catalog.Catalog {
	t.Helper()
	c, err := catalog.LoadDir(filepath.Join("..", "..", "..", "products"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Every relationship in the shipped catalog validates, so WithCatalog never applies a bad rule.
func TestShippedRelationshipsValidate(t *testing.T) {
	n := 0
	for _, d := range loadCatalog(t).List() {
		if rep := catalog.Validate(d); !rep.OK() {
			t.Errorf("%s: %v", d.ID, rep.Errors())
		}
		n += len(d.Relationships)
	}
	if n == 0 {
		t.Fatal("expected the in-scope products to declare relationships")
	}
}

// A declared produces relationship makes Certificate → Secret → Deployment established, so the
// traversal of an item rooted at the Certificate is complete (the system can clear a safe environment).
func TestCatalogProducesMakesCertificatePathComplete(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "eval", "investigations", "cert-manager-1.17-1.18--payments-eu", "env")
	f := newFixture(t)
	if f.loadEnvDir(dir) == 0 {
		t.Skip("no fixture")
	}
	f.render("argocd:clusters/prod/cert-manager.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: x, namespace: cert-manager}\n",
		domain.RenderedChange{Class: "field-changed", Object: "cert-manager.io/v1/Certificate/payments/ledger-tls", Path: "spec.privateKey.rotationPolicy", ChangeItems: []string{"ci-1"}, Correlation: "subject"})
	f.b.Change.Items = scoped("ci-1")

	// Without rules the convention only infers the edge.
	g := f.build()
	v := view{t, g}
	v.wantEdge(domain.EdgeProduces, "cert-manager.io/v1/Certificate/payments/ledger-tls", "core/v1/Secret/payments/ledger-tls", domain.EdgeInferred)

	g = f.build(WithCatalog(loadCatalog(t)))
	v = view{t, g}
	e := v.wantEdge(domain.EdgeProduces, "cert-manager.io/v1/Certificate/payments/ledger-tls", "core/v1/Secret/payments/ledger-tls", domain.EdgeEstablished)
	if e.Rule != "relationship:cert-manager/certificate-secret" {
		t.Errorf("rule = %q", e.Rule)
	}
	v.wantEdge(domain.EdgeReferences, "core/v1/Secret/payments/ledger-tls", "apps/v1/Deployment/payments/ledger", domain.EdgeEstablished)

	roots, err := NewExposure().Exposure(context.Background(), f.b, g, nil)
	if err != nil {
		t.Fatal(err)
	}
	res := traverse(t, g, roots, domain.DefaultTraversalBound)
	if !res.CompleteFor("ci-1") || !res.Complete {
		t.Fatalf("path through a declared produces edge must be complete: unresolved=%v", res.Unresolved)
	}
	reached := false
	dep := v.mustNode("apps/v1/Deployment/payments/ledger").ID
	for _, p := range res.Paths {
		for _, n := range p.Nodes {
			reached = reached || n == dep
		}
	}
	if !reached {
		t.Error("the Deployment mounting the produced Secret must lie on a path")
	}
	// The CA issuer reads the Secret a Certificate produces; its cluster resource namespace is not
	// stated on the issuer, so that link stays honestly inferred.
	v.wantEdge(domain.EdgeReferences, "core/v1/Secret/cert-manager/mesh-root-ca", "cert-manager.io/v1/ClusterIssuer//mesh-ca", domain.EdgeInferred)
}

func TestCatalogAnnotationReferences(t *testing.T) {
	f := newFixture(t)
	f.file("app/ing.yaml", `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: shop
  annotations:
    nginx.ingress.kubernetes.io/auth-secret: basic
    nginx.ingress.kubernetes.io/auth-tls-secret: certs/client-ca
    nginx.ingress.kubernetes.io/proxy-ssl-secret: "bad-no-slash"
`)
	f.file("app/secrets.yaml", `apiVersion: v1
kind: Secret
metadata: {name: basic, namespace: shop}
---
apiVersion: v1
kind: Secret
metadata: {name: client-ca, namespace: certs}
`)
	v := view{t, f.build(WithCatalog(loadCatalog(t)))}
	const ing = "networking.k8s.io/v1/Ingress/shop/web"
	v.wantEdge(domain.EdgeReferences, "core/v1/Secret/shop/basic", ing, domain.EdgeEstablished)
	e := v.wantEdge(domain.EdgeReferences, "core/v1/Secret/certs/client-ca", ing, domain.EdgeEstablished)
	if e.Rule != "relationship:ingress-nginx/auth-tls-secret" {
		t.Errorf("rule = %q", e.Rule)
	}
	// a value that does not have the documented namespace/name form yields no reference
	if _, ok := v.node("core/v1/Secret/shop/bad-no-slash"); ok {
		t.Error("a malformed namespace/name value must not create a reference")
	}
}

func TestCatalogMonitorSelectors(t *testing.T) {
	f := newFixture(t)
	f.file("app/svc.yaml", `apiVersion: v1
kind: Service
metadata: {name: api, namespace: shop, labels: {app: api}}
---
apiVersion: v1
kind: Service
metadata: {name: api, namespace: other, labels: {app: api}}
---
apiVersion: v1
kind: Service
metadata: {name: db, namespace: shop, labels: {app: db}}
`)
	f.file("app/deploy.yaml", deployment) // pod labels app=web in shop
	f.file("app/mon.yaml", `apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: {name: sm-local, namespace: shop}
spec:
  selector: {matchLabels: {app: api}}
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: {name: sm-any, namespace: shop}
spec:
  selector: {matchLabels: {app: api}}
  namespaceSelector: {any: true}
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: {name: sm-named, namespace: shop}
spec:
  selector: {matchLabels: {app: api}}
  namespaceSelector: {matchNames: [other]}
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: {name: sm-expr, namespace: shop}
spec:
  selector: {matchExpressions: [{key: app, operator: In, values: [api]}]}
---
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata: {name: pm, namespace: shop}
spec:
  selector: {matchLabels: {app: web}}
`)
	v := view{t, f.build(WithCatalog(loadCatalog(t)))}
	const mon = "monitoring.coreos.com/v1/ServiceMonitor/shop/"
	// default: the monitor's own namespace only
	v.wantEdge(domain.EdgeSelects, "core/v1/Service/shop/api", mon+"sm-local", domain.EdgeEstablished)
	if _, ok := v.edge(domain.EdgeSelects, "core/v1/Service/other/api", mon+"sm-local"); ok {
		t.Error("a ServiceMonitor without namespaceSelector must not select another namespace")
	}
	if _, ok := v.edge(domain.EdgeSelects, "core/v1/Service/shop/db", mon+"sm-local"); ok {
		t.Error("labels do not match")
	}
	// any: every namespace
	v.wantEdge(domain.EdgeSelects, "core/v1/Service/shop/api", mon+"sm-any", domain.EdgeEstablished)
	v.wantEdge(domain.EdgeSelects, "core/v1/Service/other/api", mon+"sm-any", domain.EdgeEstablished)
	// matchNames: only those
	v.wantEdge(domain.EdgeSelects, "core/v1/Service/other/api", mon+"sm-named", domain.EdgeEstablished)
	if _, ok := v.edge(domain.EdgeSelects, "core/v1/Service/shop/api", mon+"sm-named"); ok {
		t.Error("matchNames [other] must not select shop")
	}
	// matchExpressions are evaluated
	v.wantEdge(domain.EdgeSelects, "core/v1/Service/shop/api", mon+"sm-expr", domain.EdgeEstablished)
	if _, ok := v.edge(domain.EdgeSelects, "core/v1/Service/shop/db", mon+"sm-expr"); ok {
		t.Error("app In [api] must not select db")
	}
	// PodMonitor: workloads through their pod template labels
	v.wantEdge(domain.EdgeSelects, "apps/v1/Deployment/shop/web", "monitoring.coreos.com/v1/PodMonitor/shop/pm", domain.EdgeEstablished)
}

// The shipped prometheus-operator rules: a monitor endpoint's credentials and
// TLS Secrets/ConfigMaps resolve in the monitor's own namespace only.
func TestCatalogMonitorSecretReferences(t *testing.T) {
	const sm = `apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata: {name: api, namespace: mon}
spec:
  selector: {matchLabels: {app: api}}
  endpoints:
    - port: http
      authorization: {credentials: {name: api-token, key: token}}
    - port: https
      tlsConfig: {ca: {configMap: {name: api-ca, key: ca.crt}}, keySecret: {name: api-key, key: tls.key}}
---
apiVersion: monitoring.coreos.com/v1
kind: PodMonitor
metadata: {name: pods, namespace: mon}
spec:
  selector: {matchLabels: {app: api}}
  podMetricsEndpoints:
    - port: http
      basicAuth: {username: {name: basic, key: user}, password: {name: basic, key: pass}}
`
	const objs = `apiVersion: v1
kind: Secret
metadata: {name: api-token, namespace: mon}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: api-ca, namespace: mon}
---
apiVersion: v1
kind: Secret
metadata: {name: api-key, namespace: other}
---
apiVersion: v1
kind: Secret
metadata: {name: basic, namespace: mon}
`
	f := newFixture(t)
	f.pair("argocd:apps/mon.yaml", sm+"---\n"+objs, sm+"---\n"+objs)
	v := f.view(SideTo, WithCatalog(loadCatalog(t)))
	const smKey, pmKey = "monitoring.coreos.com/v1/ServiceMonitor/mon/api", "monitoring.coreos.com/v1/PodMonitor/mon/pods"
	r := ref(t, v, smKey, "spec.endpoints[0].authorization.credentials.name")
	wantRes(t, r, RefResolved, "")
	if r.Rule != "relationship:prometheus-operator/servicemonitor-authorization" {
		t.Errorf("rule %q", r.Rule)
	}
	wantRes(t, ref(t, v, smKey, "spec.endpoints[1].tlsConfig.ca.configMap.name"), RefResolved, "")
	wantRes(t, ref(t, v, smKey, "spec.endpoints[1].tlsConfig.keySecret.name"), RefDangling, "Secret mon/api-key")
	wantRes(t, ref(t, v, pmKey, "basicAuth.username.name"), RefResolved, "")
	wantRes(t, ref(t, v, pmKey, "basicAuth.password.name"), RefResolved, "")

	// the graph carries the same references
	g := f.build(WithCatalog(loadCatalog(t)))
	view{t, g}.wantEdge(domain.EdgeReferences, "core/v1/Secret/mon/api-token", smKey, domain.EdgeEstablished)
}

// ingress-nginx's auth-secret takes secretName or namespace/secretName;
// auth-tls-secret only namespace/secretName (a bare name is not a reference).
func TestCatalogIngressNginxSecretFormats(t *testing.T) {
	ing := func(name, annotations string) string {
		return "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: " + name + "\n  namespace: web\n  annotations:\n" + annotations
	}
	const objs = "apiVersion: v1\nkind: Secret\nmetadata: {name: basic, namespace: web}\n---\napiVersion: v1\nkind: Secret\nmetadata: {name: ca, namespace: shared}\n"
	docs := ing("a", "    nginx.ingress.kubernetes.io/auth-secret: basic\n") + "---\n" +
		ing("b", "    nginx.ingress.kubernetes.io/auth-secret: shared/ca\n") + "---\n" +
		ing("c", "    nginx.ingress.kubernetes.io/auth-tls-secret: shared/ca\n") + "---\n" +
		ing("d", "    nginx.ingress.kubernetes.io/auth-tls-secret: ca\n") + "---\n" + objs
	// environment files: the render withholds values of keys ending in "secret"
	f := newFixture(t)
	f.file("web/all.yaml", docs)
	v := f.view(SideTo, WithCatalog(loadCatalog(t)))
	key := func(n string) string { return "networking.k8s.io/v1/Ingress/web/" + n }
	wantRes(t, ref(t, v, key("a"), "auth-secret]"), RefResolved, "")
	b := ref(t, v, key("b"), "auth-secret]")
	wantRes(t, b, RefResolved, "")
	if b.Target != "core/v1/Secret/shared/ca" {
		t.Fatalf("namespace/name form: %+v", b)
	}
	wantRes(t, ref(t, v, key("c"), "auth-tls-secret]"), RefResolved, "")
	for _, r := range v.References {
		if r.Referrer == key("d") && strings.Contains(r.Field, "auth-tls-secret") {
			t.Fatalf("a bare name is no namespace/name reference: %+v", r)
		}
	}
}

// A rendered Ingress's Secret annotations are withheld by the render (keys
// ending in "secret" look like credentials): the reference is unknown, never
// dangling.
func TestCatalogRedactedNameIsUnknown(t *testing.T) {
	docs := "apiVersion: networking.k8s.io/v1\nkind: Ingress\nmetadata:\n  name: a\n  namespace: web\n  annotations:\n    nginx.ingress.kubernetes.io/auth-secret: basic\n"
	f := newFixture(t)
	f.pair("argocd:apps/web.yaml", docs, docs)
	r := ref(t, f.view(SideTo, WithCatalog(loadCatalog(t))), "networking.k8s.io/v1/Ingress/web/a", "auth-secret]")
	wantRes(t, r, RefUnknown, "withholds the name")
}
