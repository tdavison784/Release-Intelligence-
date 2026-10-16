package relations

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const (
	deployment = `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: shop, labels: {app: web}}
spec:
  selector: {matchLabels: {app: web}}
  template:
    metadata: {labels: {app: web}}
    spec:
      serviceAccountName: web-sa
      containers:
        - name: web
          image: web:1
          env:
            - name: PW
              valueFrom: {secretKeyRef: {name: web-secret, key: pw}}
          envFrom:
            - configMapRef: {name: web-cfg}
      volumes:
        - name: tls
          secret: {secretName: web-tls}
`
	service = `apiVersion: v1
kind: Service
metadata: {name: web, namespace: shop}
spec:
  selector: {app: web}
`
	ingress = `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: {name: web, namespace: shop}
spec:
  ingressClassName: nginx
  tls: [{secretName: web-tls}]
  rules:
    - http:
        paths:
          - path: /
            backend: {service: {name: web, port: {number: 80}}}
`
)

func TestBuilderKubernetesReferences(t *testing.T) {
	f := newFixture(t)
	f.file("app/deploy.yaml", deployment)
	f.file("app/svc.yaml", service)
	f.file("app/ing.yaml", ingress)
	f.file("app/cfg.yaml", `apiVersion: v1
kind: ConfigMap
metadata: {name: web-cfg, namespace: shop}
`)
	f.file("app/tls.yaml", `apiVersion: v1
kind: Secret
metadata: {name: web-tls, namespace: shop}
`)
	v := view{t, f.build()}

	const dep = "apps/v1/Deployment/shop/web"
	v.wantEdge(domain.EdgeReferences, "core/v1/ConfigMap/shop/web-cfg", dep, domain.EdgeEstablished)
	e := v.wantEdge(domain.EdgeReferences, "core/v1/Secret/shop/web-tls", dep, domain.EdgeEstablished)
	if e.Detail == "" || e.Rule != "graph.k8s-refs@v1:volumes.secret" || len(e.Artifacts) == 0 {
		t.Errorf("volume edge lacks detail/rule/artifacts: %+v", e)
	}
	// Absent from the snapshot: placeholder + missing edge, never silently dropped.
	e = v.wantEdge(domain.EdgeReferences, "core/v1/Secret/shop/web-secret", dep, domain.EdgeMissing)
	if e.Reason == "" {
		t.Error("missing edge needs a reason")
	}
	if n := v.mustNode("core/v1/Secret/shop/web-secret"); !n.Placeholder {
		t.Error("absent Secret must be a placeholder node")
	}
	v.wantEdge(domain.EdgeReferences, "core/v1/ServiceAccount/shop/web-sa", dep, domain.EdgeMissing)

	// Ingress: backend Service, TLS Secret, class.
	const ing = "networking.k8s.io/v1/Ingress/shop/web"
	v.wantEdge(domain.EdgeReferences, "core/v1/Service/shop/web", ing, domain.EdgeEstablished)
	v.wantEdge(domain.EdgeReferences, "core/v1/Secret/shop/web-tls", ing, domain.EdgeEstablished)
	v.wantEdge(domain.EdgeReferences, "networking.k8s.io/v1/IngressClass//nginx", ing, domain.EdgeMissing)
	// Selector: workload → Service.
	v.wantEdge(domain.EdgeSelects, dep, "core/v1/Service/shop/web", domain.EdgeEstablished)
}

func TestBuilderIngressClassFromRender(t *testing.T) {
	f := newFixture(t)
	f.file("app/ing.yaml", ingress)
	f.render("argocd:clusters/ctl.yaml", `apiVersion: networking.k8s.io/v1
kind: IngressClass
metadata: {name: nginx}
`)
	v := view{t, f.build()}
	v.wantEdge(domain.EdgeReferences, "networking.k8s.io/v1/IngressClass//nginx", "networking.k8s.io/v1/Ingress/shop/web", domain.EdgeEstablished)
	// No Application owns the target: the product release is the render source.
	v.wantEdge(domain.EdgeRendersTo, "widget@v2.0.0", "networking.k8s.io/v1/IngressClass//nginx", domain.EdgeEstablished)
}

const certificate = `apiVersion: acme.example/v1
kind: Certificate
metadata: {name: web-tls, namespace: shop}
spec:
  secretName: web-tls
  issuerRef: {name: ca, kind: ClusterIssuer, group: acme.example}
`

func TestBuilderProducesConventionIsInferred(t *testing.T) {
	f := newFixture(t)
	f.file("app/deploy.yaml", deployment)
	f.file("app/cert.yaml", certificate)
	v := view{t, f.build()}
	const cert = "acme.example/v1/Certificate/shop/web-tls"
	e := v.wantEdge(domain.EdgeProduces, cert, "core/v1/Secret/shop/web-tls", domain.EdgeInferred)
	if e.Reason == "" {
		t.Error("inferred edge needs a reason")
	}
	// The Deployment's reference to the produced Secret is only as good as the convention.
	v.wantEdge(domain.EdgeReferences, "core/v1/Secret/shop/web-tls", "apps/v1/Deployment/shop/web", domain.EdgeInferred)
	// issuerRef points at an absent issuer: a known unknown. Its scope is not known
	// without a CRD in the snapshot, so it is looked for in the referrer's namespace.
	v.wantEdge(domain.EdgeReferences, "acme.example/*/ClusterIssuer/shop/ca", cert, domain.EdgeMissing)
	// No CRD in the snapshot: the type node is a placeholder, the edge still states what the object declares.
	if n := v.mustNode("acme.example/v1/Certificate"); !n.Placeholder {
		t.Error("api-type without a CRD must be a placeholder")
	}
	v.wantEdge(domain.EdgeConsumesAPI, "acme.example/v1/Certificate", cert, domain.EdgeEstablished)
}

func TestBuilderDeclaredProducesRuleIsEstablished(t *testing.T) {
	f := newFixture(t)
	f.file("app/deploy.yaml", deployment)
	f.file("app/cert.yaml", certificate)
	v := view{t, f.build(WithProducesRules(ProducesRule{Name: "acme/certificate-secret", Group: "acme.example", Kind: "Certificate", NamePath: "spec.secretName", ProducedKind: "Secret"}))}
	e := v.wantEdge(domain.EdgeProduces, "acme.example/v1/Certificate/shop/web-tls", "core/v1/Secret/shop/web-tls", domain.EdgeEstablished)
	if e.Rule != "relationship:acme/certificate-secret" {
		t.Errorf("rule = %q", e.Rule)
	}
	v.wantEdge(domain.EdgeReferences, "core/v1/Secret/shop/web-tls", "apps/v1/Deployment/shop/web", domain.EdgeEstablished)
}

func TestBuilderKustomizeNamespaceGeneratorsAndApplication(t *testing.T) {
	f := newFixture(t)
	f.file("clusters/app.yaml", `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata: {name: web, namespace: argocd}
spec:
  destination: {namespace: shop}
  source: {repoURL: x, path: apps/web/overlays/prod}
`)
	f.file("apps/web/overlays/prod/kustomization.yaml", "resources: [../../base]\npatches:\n  - path: patch.yaml\n")
	f.file("apps/web/overlays/prod/patch.yaml", "apiVersion: apps/v1\nkind: Deployment\nmetadata: {name: web}\nspec: {replicas: 3}\n")
	f.file("apps/web/base/kustomization.yaml", "namespace: shop\nresources: [deploy.yaml]\nconfigMapGenerator:\n  - name: web-cfg\n    literals: [a=b]\n")
	f.file("apps/web/base/deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: web}
spec:
  template:
    spec:
      containers:
        - name: web
          envFrom: [{configMapRef: {name: web-cfg}}]
`)
	g := f.build()
	v := view{t, g}
	const dep = "apps/v1/Deployment/shop/web" // namespace from the base kustomization
	v.mustNode(dep)
	// The patch document is not an object: no namespace-less duplicate Deployment.
	if _, ok := v.node("apps/v1/Deployment//web"); ok {
		t.Error("a kustomize patch file must not become an object")
	}
	v.wantEdge(domain.EdgeReferences, "core/v1/ConfigMap/shop/web-cfg", dep, domain.EdgeEstablished)
	v.wantEdge(domain.EdgeDeploys, "argoproj.io/v1alpha1/Application/argocd/web", "kustomize.config.k8s.io/v1beta1/Kustomization//env/apps/web/overlays/prod", domain.EdgeEstablished)
	v.wantEdge(domain.EdgeDeploys, "kustomize.config.k8s.io/v1beta1/Kustomization//env/apps/web/overlays/prod", "kustomize.config.k8s.io/v1beta1/Kustomization//env/apps/web/base", domain.EdgeEstablished)
	v.wantEdge(domain.EdgeRendersTo, "kustomize.config.k8s.io/v1beta1/Kustomization//env/apps/web/base", dep, domain.EdgeEstablished)
	v.wantEdge(domain.EdgeRendersTo, "kustomize.config.k8s.io/v1beta1/Kustomization//env/apps/web/base", "core/v1/ConfigMap/shop/web-cfg", domain.EdgeEstablished)
}

func TestBuilderRenderedObjectsAndAPITypes(t *testing.T) {
	f := newFixture(t)
	f.file("clusters/app.yaml", `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata: {name: acme, namespace: argocd}
spec:
  destination: {namespace: acme-system}
  source: {repoURL: x, chart: widget, targetRevision: 2.0.0}
`)
	f.file("app/cert.yaml", certificate)
	f.render("argocd:clusters/app.yaml", `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: certificates.acme.example}
spec:
  group: acme.example
  names: {kind: Certificate}
  versions: [{name: v1}]
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: controller}
`, domain.RenderedChange{Class: "field-changed", Object: "apiextensions.k8s.io/v1/CustomResourceDefinition//certificates.acme.example", Path: "spec.versions"},
		domain.RenderedChange{Class: "resource-removed", Object: "core/v1/ConfigMap/acme-system/gone"})
	v := view{t, f.build()}

	const crd = "apiextensions.k8s.io/v1/CustomResourceDefinition//certificates.acme.example"
	const app = "argoproj.io/v1alpha1/Application/argocd/acme"
	v.wantEdge(domain.EdgeRendersTo, app, crd, domain.EdgeEstablished)
	v.wantEdge(domain.EdgeServesAPI, crd, "acme.example/v1/Certificate", domain.EdgeEstablished)
	v.wantEdge(domain.EdgeConsumesAPI, "acme.example/v1/Certificate", "acme.example/v1/Certificate/shop/web-tls", domain.EdgeEstablished)
	if n := v.mustNode("acme.example/v1/Certificate"); n.Placeholder {
		t.Error("a CRD in the render defines the type: not a placeholder")
	}
	// A rendered change on an object absent from the TO render still has a node to root on.
	if n := v.mustNode("core/v1/ConfigMap/acme-system/gone"); n.Placeholder {
		t.Error("an object the render changed is not a placeholder")
	}
}

func TestBuilderIsDeterministic(t *testing.T) {
	build := func(reverse bool) *domain.RelationshipGraph {
		f := newFixture(t)
		f.reverse = reverse
		f.file("app/deploy.yaml", deployment)
		f.file("app/svc.yaml", service)
		f.file("app/ing.yaml", ingress)
		f.file("app/cert.yaml", certificate)
		return f.build()
	}
	a, b := build(false), build(true)
	if a.ID != b.ID || len(a.Edges) != len(b.Edges) {
		t.Fatalf("graph depends on artifact order: %s vs %s", a.ID, b.ID)
	}
	for i := range a.Edges {
		if a.Edges[i].ID != b.Edges[i].ID {
			t.Fatalf("edge %d differs", i)
		}
	}
}

func TestBuilderRejectsNilBundle(t *testing.T) {
	if _, err := NewBuilder(newMemStore()).BuildGraph(t.Context(), nil); err == nil {
		t.Fatal("want error")
	}
}

// Every groundtruth environment (inputs only: env/) yields a graph that validates.
func TestBuilderGroundtruthEnvironments(t *testing.T) {
	root := filepath.Join("..", "..", "..", "eval", "investigations")
	envs, _ := filepath.Glob(filepath.Join(root, "*", "env"))
	if len(envs) == 0 {
		t.Skip("no eval/investigations/*/env fixtures")
	}
	for _, dir := range envs {
		name := filepath.Base(filepath.Dir(dir))
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if f.loadEnvDir(dir) == 0 {
				t.Skip("empty env")
			}
			g := f.build()
			if len(g.Nodes) == 0 {
				t.Fatal("empty graph")
			}
			v := view{t, g}
			// Environments that run the payments mesh: Certificate → Secret → Deployment.
			if _, err := os.Stat(filepath.Join(dir, "apps", "payments", "base", "ledger-deployment.yaml")); err == nil {
				v.wantEdge(domain.EdgeProduces, "cert-manager.io/v1/Certificate/payments/ledger-tls", "core/v1/Secret/payments/ledger-tls", domain.EdgeInferred)
				v.wantEdge(domain.EdgeReferences, "core/v1/Secret/payments/ledger-tls", "apps/v1/Deployment/payments/ledger", domain.EdgeInferred)
				// The generated ConfigMap resolves through kustomize's configMapGenerator.
				v.wantEdge(domain.EdgeReferences, "core/v1/ConfigMap/payments/ledger-nginx", "apps/v1/Deployment/payments/ledger", domain.EdgeEstablished)
			}
			// Environments that route through Ingress objects: Service → Ingress.
			if _, err := os.Stat(filepath.Join(dir, "apps", "storefront", "base", "ingress.yaml")); err == nil {
				v.wantEdge(domain.EdgeReferences, "core/v1/Service/storefront/storefront", "networking.k8s.io/v1/Ingress/storefront/storefront", domain.EdgeEstablished)
			}
		})
	}
}
