package relations

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// namespaceFrom: where a controller resolves a name a cluster-scoped object
// references without a namespace. The fixtures describe a FICTIONAL product
// ("gizmo": a cluster-scoped ClusterSigner reads spec.ca.secretName from the
// controller's --resource-namespace, default gizmo-system).

const (
	signerKey  = "signing.gizmo.example.io/v1/ClusterSigner//root"
	caSecret   = "core/v1/Secret/gizmo-system/root-ca"
	signerFile = `apiVersion: signing.gizmo.example.io/v1
kind: ClusterSigner
metadata: {name: root}
spec:
  ca: {secretName: root-ca}
`
	caSecrets = `apiVersion: v1
kind: Secret
metadata: {name: root-ca, namespace: gizmo-system}
---
apiVersion: v1
kind: Secret
metadata: {name: root-ca, namespace: apps}
`
)

var nsRefs = []catalog.ConfigReference{{URL: "https://example.invalid/gizmo/docs/flags", Quote: "--resource-namespace: where cluster signers read Secrets (default gizmo-system)"}}

func signerRule(mod func(*catalog.NamespaceSource)) catalog.Relationship {
	r := catalog.Relationship{ID: "cluster-signer-ca", Kind: catalog.RelReferences,
		From: catalog.ObjectType{Group: "signing.gizmo.example.io", Kind: "ClusterSigner"}, To: catalog.ObjectType{Kind: "Secret"},
		Summary: "a ClusterSigner reads its CA Secret from the controller's resource namespace", Path: "spec.ca.secretName",
		References: []catalog.ConfigReference{{URL: "https://example.invalid/gizmo/docs/api", Quote: "secretName of the CA key pair"}}}
	if mod != nil {
		r.NamespaceFrom = &catalog.NamespaceSource{Container: "gizmo-controller", Arg: "--resource-namespace", Default: "gizmo-system",
			ConfigFileArg: "--config", References: nsRefs}
		mod(r.NamespaceFrom)
	}
	return r
}

func keep(*catalog.NamespaceSource) {}

// controllerDeploy is the controller workload: namespace ns ("" = unstated),
// container gizmo-controller with args and an env block (YAML list items).
func controllerDeploy(name, ns string, args []string, env string) string {
	meta := "{name: " + name + "}"
	if ns != "" {
		meta = "{name: " + name + ", namespace: " + ns + "}"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "apiVersion: apps/v1\nkind: Deployment\nmetadata: %s\nspec:\n  template:\n    metadata: {labels: {app: gizmo}}\n    spec:\n      containers:\n        - name: gizmo-controller\n          image: example.invalid/gizmo:v2\n", meta)
	if len(args) > 0 {
		b.WriteString("          args:\n")
		for _, a := range args {
			fmt.Fprintf(&b, "            - %q\n", a)
		}
	}
	if env != "" {
		b.WriteString(env)
	}
	return b.String()
}

const podNamespaceEnv = `          env:
            - name: POD_NAMESPACE
              valueFrom: {fieldRef: {fieldPath: metadata.namespace}}
`

func signerFixture(t *testing.T, controllers ...string) *fixture {
	f := newFixture(t)
	f.file("signing/signer.yaml", signerFile)
	f.file("signing/secrets.yaml", caSecrets)
	if len(controllers) > 0 {
		f.render("helm-gizmo", strings.Join(controllers, "---\n"))
	}
	return f
}

// A decided namespace makes the reference established against the Secret in
// that namespace — never the same-named Secret elsewhere — and the edge cites
// the controller configuration it was read from.
func TestNamespaceFromDecided(t *testing.T) {
	cases := map[string]struct {
		controllers []string
		rule        func(*catalog.NamespaceSource)
	}{
		"literal flag":             {[]string{controllerDeploy("gizmo", "gizmo-system", []string{"--resource-namespace=gizmo-system"}, "")}, keep},
		"flag and separate value":  {[]string{controllerDeploy("gizmo", "gizmo-system", []string{"--resource-namespace", "gizmo-system"}, "")}, keep},
		"pod namespace via $(VAR)": {[]string{controllerDeploy("gizmo", "gizmo-system", []string{"--resource-namespace=$(POD_NAMESPACE)"}, podNamespaceEnv)}, keep},
		"literal env value": {[]string{controllerDeploy("gizmo", "elsewhere", []string{"--resource-namespace=$(RES_NS)"},
			"          env:\n            - {name: RES_NS, value: gizmo-system}\n")}, keep},
		"flag beats config file": {[]string{controllerDeploy("gizmo", "gizmo-system", []string{"--config=/etc/gizmo/config.yaml", "--resource-namespace=gizmo-system"}, "")}, keep},
		"documented default":     {[]string{controllerDeploy("gizmo", "elsewhere", []string{"--log-level=2"}, "")}, keep},
		"two controllers agree": {[]string{
			controllerDeploy("gizmo-a", "gizmo-system", []string{"--resource-namespace=$(POD_NAMESPACE)"}, podNamespaceEnv),
			controllerDeploy("gizmo-b", "elsewhere", []string{"--resource-namespace=gizmo-system"}, ""),
		}, keep},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := signerFixture(t, c.controllers...)
			v := view{t, f.build(WithRelationships("gizmo", signerRule(c.rule)))}
			e := v.wantEdge(domain.EdgeReferences, caSecret, signerKey, domain.EdgeEstablished)
			if e.Rule != "relationship:gizmo/cluster-signer-ca" {
				t.Errorf("rule = %q", e.Rule)
			}
			ctrl := f.b.Renders[0].To
			if !contains(e.Artifacts, ctrl) {
				t.Errorf("the edge must cite the controller render %s: %v", ctrl, e.Artifacts)
			}
			if _, ok := v.edge(domain.EdgeReferences, "core/v1/Secret/apps/root-ca", signerKey); ok {
				t.Error("the same-named Secret in another namespace must not be linked")
			}
		})
	}
}

// A decided namespace where the Secret does not exist is a missing
// reference (the controller will not find it), and says where it looked.
func TestNamespaceFromDecidedButAbsent(t *testing.T) {
	f := signerFixture(t, controllerDeploy("gizmo", "gizmo-system", []string{"--resource-namespace=vault"}, ""))
	v := view{t, f.build(WithRelationships("gizmo", signerRule(keep)))}
	e := v.wantEdge(domain.EdgeReferences, "core/v1/Secret/vault/root-ca", signerKey, domain.EdgeMissing)
	if !strings.Contains(e.Reason, "namespace vault") || !strings.Contains(e.Reason, "--resource-namespace=vault") {
		t.Errorf("reason must say where the namespace came from: %q", e.Reason)
	}
}

// The opposite (fail-open) implementation would pick a namespace when the
// snapshot does not decide it. Each undecidable shape leaves the reference
// matched by name only (inferred), with the reason.
func TestNamespaceFromUndecidedStaysInferred(t *testing.T) {
	ctrl := func(args []string, env string) []string {
		return []string{controllerDeploy("gizmo", "gizmo-system", args, env)}
	}
	cases := map[string]struct {
		controllers []string
		rule        func(*catalog.NamespaceSource)
		why         string
	}{
		"no citation": {ctrl([]string{"--resource-namespace=gizmo-system"}, ""),
			func(s *catalog.NamespaceSource) { s.References = nil }, "cites no upstream documentation"},
		"no controller in the environment": {nil, keep, `no workload of the environment runs a container named "gizmo-controller"`},
		"other container name": {[]string{strings.Replace(controllerDeploy("gizmo", "gizmo-system", []string{"--resource-namespace=gizmo-system"}, ""), "name: gizmo-controller", "name: sidecar", 1)},
			keep, "no workload of the environment runs"},
		"env from a ConfigMap key": {ctrl([]string{"--resource-namespace=$(RES_NS)"},
			"          env:\n            - name: RES_NS\n              valueFrom: {configMapKeyRef: {name: gizmo-env, key: ns}}\n"), keep, "valueFrom.configMapKeyRef"},
		"env from a Secret key": {ctrl([]string{"--resource-namespace=$(RES_NS)"},
			"          env:\n            - name: RES_NS\n              valueFrom: {secretKeyRef: {name: gizmo-env, key: ns}}\n"), keep, "valueFrom.secretKeyRef"},
		"var only from envFrom": {ctrl([]string{"--resource-namespace=$(RES_NS)"},
			"          envFrom:\n            - configMapRef: {name: gizmo-env}\n"), keep, "envFrom may supply it"},
		"var not defined": {ctrl([]string{"--resource-namespace=$(RES_NS)"}, ""), keep, "stays unexpanded"},
		"var defined twice": {ctrl([]string{"--resource-namespace=$(RES_NS)"},
			"          env:\n            - {name: RES_NS, value: gizmo-system}\n            - {name: RES_NS, value: apps}\n"), keep, "defined more than once"},
		"escaped reference":       {ctrl([]string{"--resource-namespace=$$(RES_NS)"}, ""), keep, "escapes"},
		"pod namespace not known": {[]string{controllerDeploy("gizmo", "", []string{"--resource-namespace=$(POD_NAMESPACE)"}, podNamespaceEnv)}, keep, "pod's namespace, which is not known"},
		"config file, no flag":    {ctrl([]string{"--config=/etc/gizmo/config.yaml"}, ""), keep, "names a configuration file that can set it"},
		"no flag, no default":     {ctrl([]string{"--log-level=2"}, ""), func(s *catalog.NamespaceSource) { s.Default = "" }, "documents no default"},
		"flag without a value":    {ctrl([]string{"--resource-namespace"}, ""), keep, "without a value"},
		"flag given twice":        {ctrl([]string{"--resource-namespace=gizmo-system", "--resource-namespace=apps"}, ""), keep, "more than once with different values"},
		"not a namespace name":    {ctrl([]string{"--resource-namespace=Gizmo_System"}, ""), keep, "is not a namespace name"},
		"controllers disagree": {[]string{
			controllerDeploy("gizmo-a", "gizmo-system", []string{"--resource-namespace=gizmo-system"}, ""),
			controllerDeploy("gizmo-b", "apps", []string{"--resource-namespace=$(POD_NAMESPACE)"}, podNamespaceEnv),
		}, keep, "controller containers disagree"},
		"one controller undecided": {[]string{
			controllerDeploy("gizmo-a", "gizmo-system", []string{"--resource-namespace=gizmo-system"}, ""),
			controllerDeploy("gizmo-b", "gizmo-system", []string{"--config=/etc/gizmo/config.yaml"}, ""),
		}, keep, "configuration file"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := signerFixture(t, c.controllers...)
			v := view{t, f.build(WithRelationships("gizmo", signerRule(c.rule)))}
			for _, s := range []string{caSecret, "core/v1/Secret/apps/root-ca"} {
				if e, ok := v.edge(domain.EdgeReferences, s, signerKey); ok && e.Status == domain.EdgeEstablished {
					t.Fatalf("an undecided namespace established %s → ClusterSigner", s)
				}
			}
			var e domain.GraphEdge
			for _, x := range v.g.Edges {
				if x.Kind == domain.EdgeReferences && x.To == v.mustNode(signerKey).ID {
					e = x
				}
			}
			if e.Status != domain.EdgeInferred {
				t.Fatalf("status %s, want inferred (reason %q)", e.Status, e.Reason)
			}
			if !strings.Contains(e.Reason, "is not decided") || !strings.Contains(e.Reason, c.why) {
				t.Fatalf("reason %q must say the namespace is not decided and why (%q)", e.Reason, c.why)
			}
		})
	}
}

// Without the declaration nothing changes: name-only matching, inferred.
func TestNamespaceFromAbsentKeepsNameOnlyMatch(t *testing.T) {
	f := signerFixture(t, controllerDeploy("gizmo", "gizmo-system", []string{"--resource-namespace=gizmo-system"}, ""))
	f.file("signing/crd.yaml", `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: clustersigners.signing.gizmo.example.io}
spec:
  group: signing.gizmo.example.io
  scope: Cluster
  names: {kind: ClusterSigner, plural: clustersigners}
  versions: [{name: v1, served: true, storage: true}]
`)
	v := view{t, f.build(WithRelationships("gizmo", signerRule(nil)))}
	v.wantEdge(domain.EdgeReferences, caSecret, signerKey, domain.EdgeInferred)
}

// A Secret a declared producer creates in the decided namespace: the whole
// path producer → Secret → ClusterSigner is established and the traversal of
// an item rooted at the producer is complete; undecided, it is not.
func TestNamespaceFromCompletesTraversal(t *testing.T) {
	produces := catalog.Relationship{ID: "gizmocert-secret", Kind: catalog.RelProduces,
		From: catalog.ObjectType{Group: "signing.gizmo.example.io", Kind: "GizmoCert"}, To: catalog.ObjectType{Kind: "Secret"},
		Summary: "a GizmoCert creates its Secret", Path: "spec.secretName",
		References: []catalog.ConfigReference{{URL: "https://example.invalid/gizmo/docs/api", Quote: "the Secret this GizmoCert creates"}}}
	const certKey = "signing.gizmo.example.io/v1/GizmoCert/gizmo-system/root"
	for _, decided := range []bool{true, false} {
		t.Run(map[bool]string{true: "decided", false: "undecided"}[decided], func(t *testing.T) {
			f := newFixture(t)
			f.file("signing/signer.yaml", signerFile)
			f.file("signing/cert.yaml", "apiVersion: signing.gizmo.example.io/v1\nkind: GizmoCert\nmetadata: {name: root, namespace: gizmo-system}\nspec: {secretName: root-ca}\n")
			args := []string{"--resource-namespace=gizmo-system"}
			if !decided {
				args = []string{"--config=/etc/gizmo/config.yaml"}
			}
			f.render("helm-gizmo", controllerDeploy("gizmo", "gizmo-system", args, ""),
				domain.RenderedChange{Class: "field-changed", Object: certKey, Path: "spec.duration", ChangeItems: []string{"ci-1"}, Correlation: "subject"})
			f.b.Change.Items = scoped("ci-1")
			g := f.build(WithRelationships("gizmo", produces, signerRule(keep)))
			v := view{t, g}
			v.wantEdge(domain.EdgeProduces, certKey, caSecret, domain.EdgeEstablished)
			want := domain.EdgeEstablished
			if !decided {
				want = domain.EdgeInferred
			}
			v.wantEdge(domain.EdgeReferences, caSecret, signerKey, want)
			roots, err := NewExposure().Exposure(context.Background(), f.b, g, nil)
			if err != nil {
				t.Fatal(err)
			}
			res := traverse(t, g, roots, domain.DefaultTraversalBound)
			if res.CompleteFor("ci-1") != decided {
				t.Fatalf("CompleteFor = %v, want %v (unresolved %v)", res.CompleteFor("ci-1"), decided, res.Unresolved)
			}
		})
	}
}

// The reference view (StaticReference's input) resolves with the same
// namespace semantics, side by side: the TO render has the controller, the
// FROM render does not (undecided there: unknown, never dangling).
func TestNamespaceFromView(t *testing.T) {
	f := signerFixture(t, controllerDeploy("gizmo", "gizmo-system", []string{"--resource-namespace=gizmo-system"}, ""))
	rule := signerRule(keep)
	to := f.view(SideTo, WithRelationships("gizmo", rule))
	r := ref(t, to, signerKey, "spec.ca.secretName")
	if r.Resolution != RefResolved || r.Namespace != "gizmo-system" || r.Target != caSecret {
		t.Fatalf("TO: %+v", r)
	}
	from := f.view(SideFrom, WithRelationships("gizmo", rule))
	r = ref(t, from, signerKey, "spec.ca.secretName")
	if r.Resolution != RefUnknown || r.Namespace != "" {
		t.Fatalf("FROM (no controller rendered): %+v", r)
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}
