package relations

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// pair adds a complete environment pair with both renders.
func (f *fixture) pair(target, from, to string) {
	f.t.Helper()
	fa := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, "derived/render/env/"+target+"/from.yaml", from)
	ta := f.put(domain.ArtifactRenderedManifests, domain.OriginDerived, "derived/render/env/"+target+"/to.yaml", to)
	da := f.put(domain.ArtifactRenderDiff, domain.OriginDerived, "derived/render/env/"+target+"/diff.json", "{}")
	f.b.Renders = append(f.b.Renders, domain.ManifestPair{Scope: domain.RenderEnvironment, Target: target, From: fa.ID, To: ta.ID, Diff: da.ID, Status: domain.CollectionOK})
}

func (f *fixture) view(side Side, opts ...Option) *View {
	f.t.Helper()
	v, err := NewViewer(f.st, opts...).View(context.Background(), f.b, side)
	if err != nil {
		f.t.Fatal(err)
	}
	return v
}

// ref finds the one reference of referrer at field (a suffix of the field path).
func ref(t *testing.T, v *View, referrer, field string) Reference {
	t.Helper()
	var hits []Reference
	for _, r := range v.References {
		if r.Referrer == referrer && strings.HasSuffix(r.Field, field) {
			hits = append(hits, r)
		}
	}
	if len(hits) != 1 {
		var all []string
		for _, r := range v.References {
			all = append(all, r.Referrer+" "+r.Field+" → "+r.TargetString()+" "+string(r.Resolution))
		}
		t.Fatalf("%d references of %s at …%s; all:\n  %s", len(hits), referrer, field, strings.Join(all, "\n  "))
	}
	return hits[0]
}

func wantRes(t *testing.T, r Reference, want Resolution, reason string) {
	t.Helper()
	if r.Resolution != want {
		t.Fatalf("%s %s → %s: %s, want %s (reason %q)", r.Referrer, r.Field, r.TargetString(), r.Resolution, want, r.Reason)
	}
	if reason != "" && !strings.Contains(r.Reason, reason) {
		t.Fatalf("reason %q does not mention %q", r.Reason, reason)
	}
}

const (
	webKey = "apps/v1/Deployment/apps/web"
)

func web(secret string, optional bool) string {
	s := `apiVersion: apps/v1
kind: Deployment
metadata: {name: web, namespace: apps}
spec:
  template:
    spec:
      serviceAccountName: default
      volumes:
        - name: creds
          secret:
            secretName: ` + secret + "\n"
	if optional {
		s += "            optional: true\n"
	}
	return s
}

func secret(name, ns string) string {
	return "apiVersion: v1\nkind: Secret\nmetadata: {name: " + name + ", namespace: " + ns + "}\n"
}

// Same-namespace semantics: a name-only reference resolves only in the
// referrer's namespace; the same name in another namespace leaves it dangling.
func TestViewSameNamespace(t *testing.T) {
	f := newFixture(t)
	f.pair("argocd:apps/web.yaml", web("db", false)+"---\n"+secret("db", "apps"), web("db", false)+"---\n"+secret("db", "other"))
	from, to := f.view(SideFrom), f.view(SideTo)
	r := ref(t, from, webKey, "secret.secretName")
	wantRes(t, r, RefResolved, "")
	if r.Target != "core/v1/Secret/apps/db" || r.Namespace != "apps" || !r.Rendered {
		t.Fatalf("resolved at FROM to %q in %q: %+v", r.Target, r.Namespace, r)
	}
	wantRes(t, ref(t, to, webKey, "secret.secretName"), RefDangling, "Secret apps/db is not defined")
	if from.Side != SideFrom || to.Side != SideTo || len(to.Objects) != 2 {
		t.Fatalf("view sides/objects: %s %s %d", from.Side, to.Side, len(to.Objects))
	}
	// the default ServiceAccount exists in every namespace
	wantRes(t, ref(t, to, webKey, "serviceAccountName"), RefResolved, "every namespace")
}

// An object the environment provides outside the rendered chart (an
// environment file) resolves; the namespace kustomize gives it counts.
func TestViewEnvironmentProvidedTarget(t *testing.T) {
	f := newFixture(t)
	f.file("apps/kustomization.yaml", "namespace: apps\nresources: [secret.yaml]\n")
	f.file("apps/secret.yaml", "apiVersion: v1\nkind: Secret\nmetadata: {name: db}\n")
	f.pair("argocd:apps/web.yaml", web("db", false), web("db", false))
	r := ref(t, f.view(SideTo), webKey, "secret.secretName")
	wantRes(t, r, RefResolved, "")
	if r.Target != "core/v1/Secret/apps/db" {
		t.Fatalf("target %q", r.Target)
	}
}

// Optional references and references whose namespace is not known are
// unknown, never dangling.
func TestViewUnknownTargets(t *testing.T) {
	f := newFixture(t)
	f.file("cluster/issuer.yaml", "apiVersion: gizmo.example.io/v1\nkind: ClusterGizmo\nmetadata: {name: root}\nspec:\n  caSecret: root-ca\n")
	f.pair("argocd:apps/web.yaml", web("db", true), web("db", true))
	rels := WithRelationships("test", catalog.Relationship{ID: "ca", Kind: catalog.RelReferences,
		From: catalog.ObjectType{Group: "gizmo.example.io", Kind: "ClusterGizmo"}, To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.caSecret"})
	v := f.view(SideTo, rels)
	wantRes(t, ref(t, v, webKey, "secret.secretName"), RefUnknown, "optional reference")
	wantRes(t, ref(t, v, "gizmo.example.io/v1/ClusterGizmo//root", "spec.caSecret"), RefUnknown, "namespace it is looked up in is not known")
}

// Declared rules: list paths give concrete per-element field paths, an
// explicit namespace/name format resolves in the named namespace, a
// cluster-scoped target resolves from any namespace, a declared producer
// resolves an object no input defines.
func TestViewDeclaredRules(t *testing.T) {
	const monitor = `apiVersion: gizmo.example.io/v1
kind: Probe
metadata: {name: p, namespace: mon}
spec:
  endpoints:
    - auth: {credentials: {name: tok, key: token}}
    - auth: {credentials: {name: gone, key: token}}
  caFrom: shared/ca
  classRef: fast
  certName: p-tls
`
	const objs = `apiVersion: v1
kind: Secret
metadata: {name: tok, namespace: mon}
---
apiVersion: v1
kind: Secret
metadata: {name: ca, namespace: shared}
---
apiVersion: v1
kind: Secret
metadata: {name: ca, namespace: mon}
---
apiVersion: gizmo.example.io/v1
kind: ProbeClass
metadata: {name: fast}
`
	f := newFixture(t)
	f.pair("argocd:apps/mon.yaml", monitor+"---\n"+objs, monitor+"---\n"+strings.Replace(objs, "namespace: shared", "namespace: elsewhere", 1))
	g := catalog.ObjectType{Group: "gizmo.example.io", Kind: "Probe"}
	rels := WithRelationships("test",
		catalog.Relationship{ID: "creds", Kind: catalog.RelReferences, From: g, To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.endpoints[].auth.credentials.name"},
		catalog.Relationship{ID: "ca", Kind: catalog.RelReferences, From: g, To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.caFrom", Format: catalog.RelFormatNamespaceName},
		catalog.Relationship{ID: "class", Kind: catalog.RelReferences, From: g, To: catalog.ObjectType{Group: "gizmo.example.io", Kind: "ProbeClass"}, Path: "spec.classRef", Scope: "cluster"},
		catalog.Relationship{ID: "cert", Kind: catalog.RelProduces, From: g, To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.certName"},
	)
	const p = "gizmo.example.io/v1/Probe/mon/p"
	from, to := f.view(SideFrom, rels), f.view(SideTo, rels)

	r0, r1 := ref(t, to, p, "endpoints[0].auth.credentials.name"), ref(t, to, p, "endpoints[1].auth.credentials.name")
	wantRes(t, r0, RefResolved, "")
	wantRes(t, r1, RefDangling, "Secret mon/gone")
	if r0.Rule != "relationship:test/creds" || r0.Field != "spec.endpoints[0].auth.credentials.name" {
		t.Fatalf("rule/field: %+v", r0)
	}
	// explicit namespace: resolves in "shared" at FROM, not in the referrer's namespace at TO
	wantRes(t, ref(t, from, p, "spec.caFrom"), RefResolved, "")
	wantRes(t, ref(t, to, p, "spec.caFrom"), RefDangling, "Secret shared/ca")
	c := ref(t, to, p, "spec.classRef")
	wantRes(t, c, RefResolved, "")
	if !c.Cluster || c.Namespace != "" {
		t.Fatalf("cluster-scoped target: %+v", c)
	}
	// the produced Secret is referenced by nothing here; a Pod mounting it would resolve
	if r0.Identity() == r1.Identity() || !r1.TargetMatches("core/v1/Secret/mon/gone") || r1.TargetMatches("core/v1/Secret/other/gone") {
		t.Fatalf("identity / target matching")
	}
}

// A reference to a Secret a declared produces relationship creates resolves.
func TestViewDeclaredProducerResolves(t *testing.T) {
	const cr = "apiVersion: gizmo.example.io/v1\nkind: Cert\nmetadata: {name: c, namespace: apps}\nspec: {secretName: db}\n"
	f := newFixture(t)
	f.pair("argocd:apps/web.yaml", web("db", false)+"---\n"+cr, web("db", false)+"---\n"+cr)
	declared := WithRelationships("test", catalog.Relationship{ID: "cert", Kind: catalog.RelProduces,
		From: catalog.ObjectType{Group: "gizmo.example.io", Kind: "Cert"}, To: catalog.ObjectType{Kind: "Secret"}, Path: "spec.secretName"})
	wantRes(t, ref(t, f.view(SideTo, declared), webKey, "secret.secretName"), RefResolved, "declared produces")
	// by the generic spec.secretName convention only: unknown
	wantRes(t, ref(t, f.view(SideTo), webKey, "secret.secretName"), RefUnknown, "presumed")
}

// The view never sees the graph's extra nodes for objects of rendered
// changes: a reference to an object the TO render removed dangles.
func TestViewIgnoresRenderedChangeNodes(t *testing.T) {
	f := newFixture(t)
	f.pair("argocd:apps/web.yaml", web("db", false)+"---\n"+secret("db", "apps"), web("db", false))
	f.b.Renders[0].Changes = []domain.RenderedChange{{ID: "rc-1", Class: "resource-removed", Object: "core/v1/Secret/apps/db"}}
	wantRes(t, ref(t, f.view(SideTo), webKey, "secret.secretName"), RefDangling, "")
	// the graph keeps the node (a root) and its reference established
	g := f.build()
	view{t, g}.wantEdge(domain.EdgeReferences, "core/v1/Secret/apps/db", webKey, domain.EdgeEstablished)
}

// Views are deterministic and reject a bad side.
func TestViewDeterministic(t *testing.T) {
	f := newFixture(t)
	f.pair("argocd:apps/web.yaml", web("db", false), web("db", false)+"---\n"+secret("db", "apps"))
	a, b := f.view(SideTo), f.view(SideTo)
	ids := func(v *View) []string {
		var out []string
		for _, r := range v.References {
			out = append(out, r.Identity()+string(r.Resolution))
		}
		return out
	}
	if strings.Join(ids(a), ";") != strings.Join(ids(b), ";") {
		t.Fatal("views differ")
	}
	if _, err := NewViewer(f.st).View(context.Background(), f.b, "middle"); err == nil {
		t.Fatal("bad side accepted")
	}
}
