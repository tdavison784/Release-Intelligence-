package impact

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
)

const condCerts = `apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: a, namespace: ns}
spec:
  secretName: a-tls
  issuerRef: {name: ca, kind: Issuer, group: cert-manager.io}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: b, namespace: ns}
spec:
  secretName: b-tls
  dnsNames: [b.example.com, c.example.com]
  privateKey:
    rotationPolicy: Never
  issuerRef: {name: acme, kind: Issuer}
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata: {name: ca, namespace: ns}
spec:
  ca: {secretName: root}
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata: {name: acme, namespace: ns}
spec:
  acme:
    solvers:
    - http01:
        ingress: {class: nginx}
`

const condConfigMaps = `apiVersion: v1
kind: ConfigMap
metadata: {name: rbac, namespace: argocd}
data:
  policy.csv: |
    p, role:admin, applications, *, */*, allow
    g, admins, role:admin
---
apiVersion: v1
kind: ConfigMap
metadata: {name: creds, namespace: argocd}
data:
  config.yaml: |
    url: https://example
    client_secret: s3cr3t
  password: hunter2
`

const condDeploy = `apiVersion: apps/v1
kind: Deployment
metadata: {name: ctrl, namespace: cm}
spec:
  template:
    spec:
      containers:
      - name: controller
        image: quay.io/jetstack/cert-manager-controller:v1.17.2
        args:
        - --enable-certificate-owner-ref=true
        - --feature-gates=ValidateCAA=true,Other=false
        - --v
        - "2"
        env:
        - name: LEADER_ELECT
          value: "false"
        - name: API_TOKEN
          valueFrom: {secretKeyRef: {name: x, key: y}}
`

const condValues = `tls:
  secretsBackend: k8s
installCRDs: true
featureGates: "A=true,B=false"
`

// condEnv loads the fixture environment; extra files are added to the
// manifests directory (a broken one makes the dimension partial).
func condEnv(t *testing.T, inventory string, extra map[string]string) *env.Environment {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "manifests/certs.yaml", condCerts)
	writeFile(t, dir, "manifests/configmaps.yaml", condConfigMaps)
	writeFile(t, dir, "manifests/deploy.yaml", condDeploy)
	for name, body := range extra {
		writeFile(t, dir, "manifests/"+name, body)
	}
	in := env.Inputs{
		KubernetesVersion: "1.30.4",
		ValuesFiles:       []string{writeFile(t, dir, "values.yaml", condValues)},
		Manifests:         []string{dir + "/manifests"},
	}
	if inventory != "" {
		in.Inventory = writeFile(t, dir, "inventory.yaml", inventory)
	}
	return loadEnv(t, in)
}

func certScope(of ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate", Of: of}
}

func field(path string, st domain.FieldState, values ...string) domain.Condition {
	return domain.Condition{Op: domain.OpField, Path: path, State: st, Values: values}
}

func TestEvaluateConditionLeaves(t *testing.T) {
	e := condEnv(t, "- product: ingress-nginx\n  version: v1.12.1\n", nil)
	edge := newEdge().edge
	cases := []struct {
		name   string
		c      domain.Condition
		want   Truth
		reason domain.UnknownReason
	}{
		{"field unset on some Certificate", certScope(field("spec.privateKey.rotationPolicy", domain.StateUnset)), True, ""},
		{"field equals on the same Certificate", certScope(field("spec.privateKey.rotationPolicy", domain.StateEquals, `"Never"`)), True, ""},
		{"field equals another value", certScope(field("spec.privateKey.rotationPolicy", domain.StateEquals, `"Always"`)), False, ""},
		{"field not-equals", certScope(field("spec.privateKey.rotationPolicy", domain.StateNotEquals, `"Always"`)), True, ""},
		{"list descent", certScope(field("spec.dnsNames[]", domain.StateEquals, `"c.example.com"`)), True, ""},
		{"matches", certScope(domain.Condition{Op: domain.OpField, Path: "spec.secretName", State: domain.StateMatches, Pattern: `^b-`}), True, ""},
		{"no resource of the kind is false", domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "ClusterIssuer", Of: []domain.Condition{field("spec", domain.StateSet)}}, False, ""},
		// per-resource scope: no single Certificate both sets Never and lacks secretName
		{"scope is per resource", certScope(field("spec.privateKey.rotationPolicy", domain.StateEquals, `"Never"`), field("spec.issuerRef.group", domain.StateSet)), False, ""},
		{"ref resolves to a CA issuer", certScope(domain.Condition{Op: domain.OpRef, Path: "spec.issuerRef", Kind: "Issuer", Of: []domain.Condition{field("spec.ca", domain.StateSet)}}), True, ""},
		{"ref of the wrong kind", certScope(domain.Condition{Op: domain.OpRef, Path: "spec.issuerRef", Kind: "ClusterIssuer", Of: []domain.Condition{field("spec", domain.StateSet)}}), False, ""},
		{"text-line exists", domain.Condition{Op: domain.OpResource, Kind: "ConfigMap", Of: []domain.Condition{{Op: domain.OpTextLine, Path: `data["policy.csv"]`, State: domain.StateExists, Pattern: `^p, role:admin, applications`}}}, True, ""},
		{"text-line none with a matching line", domain.Condition{Op: domain.OpResource, Kind: "ConfigMap", Name: "rbac", Of: []domain.Condition{{Op: domain.OpTextLine, Path: `data["policy.csv"]`, State: domain.StateNone, Pattern: `role:admin`}}}, False, ""},
		{"text-line none over withheld lines is unknown", domain.Condition{Op: domain.OpResource, Kind: "ConfigMap", Name: "creds", Of: []domain.Condition{{Op: domain.OpTextLine, Path: `data["config.yaml"]`, State: domain.StateNone, Pattern: `client_secret: s3cr3t`}}}, Unknown, domain.UnknownEnvironmentVisibilityGap},
		{"text of a withheld field never matches", domain.Condition{Op: domain.OpResource, Kind: "ConfigMap", Name: "creds", Of: []domain.Condition{{Op: domain.OpTextLine, Path: "data.password", State: domain.StateExists, Pattern: `hunter2`}}}, Unknown, domain.UnknownEnvironmentVisibilityGap},
		{"withheld value decides nothing", domain.Condition{Op: domain.OpResource, Kind: "ConfigMap", Name: "creds", Of: []domain.Condition{field("data.password", domain.StateEquals, `"hunter2"`)}}, Unknown, domain.UnknownEnvironmentVisibilityGap},
		{"presence of a withheld value is decidable", domain.Condition{Op: domain.OpResource, Kind: "ConfigMap", Name: "creds", Of: []domain.Condition{field("data.password", domain.StateSet)}}, True, ""},
		{"values-key unset", domain.Condition{Op: domain.OpValuesKey, Path: "tls.readSecretsOnlyFromSecretsNamespace", State: domain.StateUnset}, True, ""},
		{"values-key set", domain.Condition{Op: domain.OpValuesKey, Path: "tls.secretsBackend", State: domain.StateSet}, True, ""},
		{"values-key section set", domain.Condition{Op: domain.OpValuesKey, Path: "tls", State: domain.StateSet}, True, ""},
		{"values-key has-token-key", domain.Condition{Op: domain.OpValuesKey, Path: "featureGates", State: domain.StateHasTokenKey, Values: []string{"B"}}, True, ""},
		{"values-key has-token exact", domain.Condition{Op: domain.OpValuesKey, Path: "featureGates", State: domain.StateHasToken, Values: []string{"B=true"}}, False, ""},
		{"gvk-in-use", domain.Condition{Op: domain.OpGVKInUse, Group: "cert-manager.io", Version: "v1", Kind: "Issuer"}, True, ""},
		{"gvk-in-use absent", domain.Condition{Op: domain.OpGVKInUse, Group: "cert-manager.io", Version: "v1alpha2", Kind: "Issuer"}, False, ""},
		{"image-in-use", domain.Condition{Op: domain.OpImageInUse, Name: "quay.io/jetstack/cert-manager-controller"}, True, ""},
		{"image-in-use out-of-range", domain.Condition{Op: domain.OpImageInUse, Name: "quay.io/jetstack/cert-manager-controller", State: domain.StateOutOfRange, Range: ">=1.18.0"}, True, ""},
		{"cli-flag equals", domain.Condition{Op: domain.OpCLIFlag, Name: "--enable-certificate-owner-ref", State: domain.StateEquals, Values: []string{`"true"`}}, True, ""},
		{"cli-flag value in next arg", domain.Condition{Op: domain.OpCLIFlag, Name: "--v", State: domain.StateEquals, Values: []string{`"2"`}}, True, ""},
		// a named component whose workload is not among the (undeclared) manifests: not shown ≠ not running
		{"cli-flag of a component not in the manifests", domain.Condition{Op: domain.OpCLIFlag, Name: "--enable-certificate-owner-ref", Component: "webhook", State: domain.StateSet}, Unknown, domain.UnknownEnvironmentVisibilityGap},
		{"cli-flag of a present component that does not pass it", domain.Condition{Op: domain.OpCLIFlag, Name: "--nope", Component: "controller", State: domain.StateSet}, False, ""},
		{"env-var equals", domain.Condition{Op: domain.OpEnvVar, Name: "LEADER_ELECT", State: domain.StateEquals, Values: []string{`"false"`}}, True, ""},
		{"env-var via valueFrom is not decidable", domain.Condition{Op: domain.OpEnvVar, Name: "API_TOKEN", State: domain.StateEquals, Values: []string{`"x"`}}, Unknown, domain.UnknownEnvironmentVisibilityGap},
		{"feature-gate enabled", domain.Condition{Op: domain.OpFeatureGate, Name: "ValidateCAA", State: domain.StateEnabled}, True, ""},
		{"feature-gate disabled", domain.Condition{Op: domain.OpFeatureGate, Name: "Other", State: domain.StateDisabled}, True, ""},
		{"feature-gate unset", domain.Condition{Op: domain.OpFeatureGate, Name: "Nope", State: domain.StateUnset}, True, ""},
		{"feature-gate via values", domain.Condition{Op: domain.OpFeatureGate, Name: "B", Path: "featureGates", State: domain.StateDisabled}, True, ""},
		{"cluster-version in range", domain.Condition{Op: domain.OpClusterVersion, Name: "kubernetes", State: domain.StateInRange, Range: ">=1.29.0"}, True, ""},
		{"cluster-version of an unsuppliable platform", domain.Condition{Op: domain.OpClusterVersion, Name: "openshift", State: domain.StateInRange, Range: ">=4.14.0"}, Unknown, domain.UnknownEnvironmentVisibilityGap},
		{"edge-from-version", domain.Condition{Op: domain.OpEdgeFromVersion, State: domain.StateInRange, Range: ">=1.0.0"}, True, ""},
		{"edge-from-version false", domain.Condition{Op: domain.OpEdgeFromVersion, State: domain.StateInRange, Range: ">=1.0.1"}, False, ""},
		{"rendered-change without renders", domain.Condition{Op: domain.OpRenderedChange, Kind: "Deployment", Path: "spec.template.spec.containers[].args", State: domain.StateChanged}, Unknown, domain.UnknownEnvironmentVisibilityGap},
		{"undecidable", domain.Condition{Op: domain.OpUndecidable, Reason: domain.UnknownRuntimeBehaviorGap, Needed: "live traffic"}, Unknown, domain.UnknownRuntimeBehaviorGap},
		{"product-version declared app version", domain.Condition{Op: domain.OpProductVersion, Name: "ingress-nginx", State: domain.StateInRange, Range: ">=1.12.0"}, True, ""},
		{"product missing from a non-complete inventory", domain.Condition{Op: domain.OpProductVersion, Name: "istio", State: domain.StateInRange, Range: "*"}, Unknown, domain.UnknownCrossProductContextGap},
		// Kleene combinators
		{"all with an unknown and a false is false", domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpUndecidable, Reason: domain.UnknownRuntimeBehaviorGap, Needed: "x"},
			{Op: domain.OpGVKInUse, Group: "cert-manager.io", Version: "v1alpha2", Kind: "Issuer"}}}, False, ""},
		{"any with an unknown and a true is true", domain.Condition{Op: domain.OpAny, Of: []domain.Condition{
			{Op: domain.OpUndecidable, Reason: domain.UnknownRuntimeBehaviorGap, Needed: "x"},
			{Op: domain.OpGVKInUse, Group: "cert-manager.io", Version: "v1", Kind: "Issuer"}}}, True, ""},
		{"reason precedence: cross-product beats runtime", domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpUndecidable, Reason: domain.UnknownRuntimeBehaviorGap, Needed: "x"},
			{Op: domain.OpProductVersion, Name: "istio", State: domain.StateInRange, Range: "*"}}}, Unknown, domain.UnknownCrossProductContextGap},
		{"not(false) with examined evidence is true", domain.Condition{Op: domain.OpNot, Of: []domain.Condition{{Op: domain.OpValuesKey, Path: "foo", State: domain.StateSet}}}, True, ""},
		{"not(true) is false", domain.Condition{Op: domain.OpNot, Of: []domain.Condition{{Op: domain.OpValuesKey, Path: "tls.secretsBackend", State: domain.StateSet}}}, False, ""},
		{"not of a zero-resource false examined nothing", domain.Condition{Op: domain.OpNot, Of: []domain.Condition{{Op: domain.OpResource, Group: "cert-manager.io", Kind: "ClusterIssuer", Of: []domain.Condition{field("spec", domain.StateSet)}}}}, Unknown, domain.UnknownEnvironmentVisibilityGap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := EvaluateCondition(tc.c, e, edge)
			if r.Value != tc.want {
				t.Fatalf("value = %s, want %s (reason %s, needed %v)", r.Value, tc.want, r.Reason, r.Needed)
			}
			assertResultShape(t, r, e)
			if tc.want == Unknown && r.Reason != tc.reason {
				t.Errorf("reason = %s, want %s (needed %v)", r.Reason, tc.reason, r.Needed)
			}
		})
	}
}

// assertResultShape: every true carries resolving environment evidence, every
// false a check, every unknown a reason and what is needed.
func assertResultShape(t *testing.T, r ConditionResult, e *env.Environment) {
	t.Helper()
	pool := map[domain.EvidenceID]bool{}
	for _, x := range e.Evidence {
		pool[x.ID] = true
	}
	for _, x := range r.Records {
		pool[x.ID] = true
	}
	switch r.Value {
	case True:
		if len(r.Matches) == 0 {
			t.Fatal("a true without matches")
		}
		for _, m := range r.Matches {
			if len(m.Evidence) == 0 {
				t.Errorf("match %q without evidence", m.Subject)
			}
			for _, id := range m.Evidence {
				if !pool[id] {
					t.Errorf("match %q cites %s, which resolves nowhere", m.Subject, id)
				}
			}
		}
	case False:
		if len(r.Checks) == 0 {
			t.Fatal("a false without a check")
		}
	case Unknown:
		if !r.Reason.Valid() || len(r.Needed) == 0 {
			t.Fatalf("an unknown without reason/needed: %q %v", r.Reason, r.Needed)
		}
	}
}

func TestEvaluateConditionAbsenceIsNotKnowledge(t *testing.T) {
	edge := newEdge().edge
	// nothing but a cluster version: no values, no manifests, no inventory
	bare := loadEnv(t, env.Inputs{KubernetesVersion: "1.30"})
	for _, c := range []domain.Condition{
		certScope(field("spec.privateKey.rotationPolicy", domain.StateUnset)),
		certScope(field("spec.privateKey.rotationPolicy", domain.StateSet)),
		{Op: domain.OpValuesKey, Path: "a", State: domain.StateUnset},
		{Op: domain.OpValuesKey, Path: "a", State: domain.StateSet},
		{Op: domain.OpGVKInUse, Version: "v1", Kind: "Pod"},
		{Op: domain.OpImageInUse, Name: "x/y"},
		{Op: domain.OpCLIFlag, Name: "--x", State: domain.StateSet},
		{Op: domain.OpEnvVar, Name: "X", State: domain.StateUnset},
		{Op: domain.OpFeatureGate, Name: "X", State: domain.StateUnset},
		{Op: domain.OpProductVersion, Name: "ingress-nginx", State: domain.StateInRange, Range: "*"},
		{Op: domain.OpNot, Of: []domain.Condition{{Op: domain.OpValuesKey, Path: "a", State: domain.StateSet}}},
	} {
		r := EvaluateCondition(c, bare, edge)
		if r.Value != Unknown {
			t.Errorf("%s with the dimension absent = %s, want unknown", describe(c), r.Value)
		}
	}
	// "1.30" is a release line: a patch-level range cannot be decided
	r := EvaluateCondition(domain.Condition{Op: domain.OpClusterVersion, Name: "kubernetes", State: domain.StateInRange, Range: ">=1.30.2"}, bare, edge)
	if r.Value != Unknown {
		t.Errorf("a line version against a patch range = %s, want unknown", r.Value)
	}
	if r := EvaluateCondition(domain.Condition{Op: domain.OpClusterVersion, Name: "kubernetes", State: domain.StateInRange, Range: ">=1.29.0"}, bare, edge); r.Value != True {
		t.Errorf("a line inside a range = %s, want true", r.Value)
	}
}

// An environment whose workloads expose no container arguments and no
// environment variables (and whose values carry no featureGates key) must not
// turn that silence into a false: absence needs examined evidence, exactly as
// the dimension-absent case does (TestEvaluateConditionAbsenceIsNotKnowledge).
func TestEvaluateConditionArglessWorkloads(t *testing.T) {
	edge := newEdge().edge
	dir := t.TempDir()
	writeFile(t, dir, "manifests/deploy.yaml", `apiVersion: apps/v1
kind: Deployment
metadata: {name: ctrl, namespace: cm}
spec:
  template:
    spec:
      containers:
      - name: controller
        image: quay.io/jetstack/cert-manager-controller:v1.17.2
`)
	e := loadEnv(t, env.Inputs{
		KubernetesVersion: "1.30.4",
		ValuesFiles:       []string{writeFile(t, dir, "values.yaml", "tls:\n  secretsBackend: k8s\n")},
		Manifests:         []string{dir + "/manifests"},
	})
	for _, c := range []domain.Condition{
		{Op: domain.OpCLIFlag, Name: "--feature-gates", State: domain.StateSet},
		{Op: domain.OpCLIFlag, Name: "--feature-gates", State: domain.StateEquals, Values: []string{`"A=true"`}},
		{Op: domain.OpEnvVar, Name: "LEADER_ELECT", State: domain.StateSet},
		{Op: domain.OpFeatureGate, Name: "ValidateCAA", State: domain.StateEnabled},
		{Op: domain.OpFeatureGate, Name: "ValidateCAA", State: domain.StateDisabled},
		// the values path is examined, but the fixture sets no such key
		{Op: domain.OpFeatureGate, Name: "ValidateCAA", Path: "featureGates", State: domain.StateEnabled},
	} {
		r := EvaluateCondition(c, e, edge)
		if r.Value != Unknown || r.Reason != domain.UnknownEnvironmentVisibilityGap {
			t.Errorf("%s from an argless workload = %s (reason %s), want unknown environment-visibility-gap", describe(c), r.Value, r.Reason)
		}
		assertResultShape(t, r, e)
	}
}

func TestEvaluateConditionPartialManifests(t *testing.T) {
	edge := newEdge().edge
	e := condEnv(t, "", map[string]string{"broken.yaml": "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: x\n"})
	if e.Health(env.DimManifests) != env.HealthPartial {
		t.Fatalf("fixture: manifests health = %s, want partial", e.Health(env.DimManifests))
	}
	// a would-be false becomes unknown …
	for _, c := range []domain.Condition{
		certScope(field("spec.privateKey.rotationPolicy", domain.StateEquals, `"Always"`)),
		{Op: domain.OpResource, Group: "cert-manager.io", Kind: "ClusterIssuer", Of: []domain.Condition{field("spec", domain.StateSet)}},
		{Op: domain.OpGVKInUse, Group: "cert-manager.io", Version: "v1alpha2", Kind: "Issuer"},
		{Op: domain.OpNot, Of: []domain.Condition{certScope(field("spec.secretName", domain.StateSet))}},
	} {
		if r := EvaluateCondition(c, e, edge); r.Value != Unknown || r.Reason != domain.UnknownEnvironmentVisibilityGap {
			t.Errorf("%s under partial manifests = %s/%s, want unknown/environment-visibility-gap", describe(c), r.Value, r.Reason)
		}
	}
	// … while a true backed by evidence stands
	if r := EvaluateCondition(certScope(field("spec.privateKey.rotationPolicy", domain.StateEquals, `"Never"`)), e, edge); r.Value != True {
		t.Errorf("an evidenced true under partial health = %s, want true", r.Value)
	}
}

func TestEvaluateConditionProductVersionTable(t *testing.T) {
	edge := newEdge().edge
	in := func(name, rng string) domain.Condition {
		return domain.Condition{Op: domain.OpProductVersion, Name: name, State: domain.StateInRange, Range: rng}
	}
	out := func(name, rng string) domain.Condition {
		return domain.Condition{Op: domain.OpProductVersion, Name: name, State: domain.StateOutOfRange, Range: rng}
	}
	cases := []struct {
		name      string
		inventory string
		c         domain.Condition
		want      Truth
	}{
		{"app version in range", "- product: ingress-nginx\n  version: v1.12.1\n", in("ingress-nginx", ">=1.12.0"), True},
		{"app version out of range", "- product: ingress-nginx\n  version: v1.11.3\n", out("ingress-nginx", ">=1.12.0"), True},
		{"app version below: in-range is false", "- product: ingress-nginx\n  version: v1.11.3\n", in("ingress-nginx", ">=1.12.0"), False},
		{"listed, any version, presence asked", "- product: argo-cd\n", in("argo-cd", "*"), True},
		{"listed without a version", "- product: argo-cd\n", in("argo-cd", ">=2.0.0"), Unknown},
		{"line version too coarse", "- product: argo-cd\n  version: \"2.14\"\n", in("argo-cd", ">=2.14.3"), Unknown},
		{"line version decides a coarse range", "- product: argo-cd\n  version: \"2.14\"\n", in("argo-cd", ">=2.0.0"), True},
		{"not listed, inventory not complete", "- product: argo-cd\n  version: 2.14.1\n", in("ingress-nginx", "*"), Unknown},
		{"not listed, inventory complete", "complete: true\nproducts:\n  - product: argo-cd\n    version: 2.14.1\n", in("ingress-nginx", ">=1.12.0"), False},
		{"conflicting app versions", "- product: ingress-nginx\n  version: v1.12.1\n- product: ingress-nginx\n  version: v1.11.0\n", in("ingress-nginx", ">=1.12.0"), Unknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			e := loadEnv(t, env.Inputs{Inventory: writeFile(t, dir, "inventory.yaml", tc.inventory)})
			r := EvaluateCondition(tc.c, e, edge)
			if r.Value != tc.want {
				t.Fatalf("= %s, want %s (needed %v)", r.Value, tc.want, r.Needed)
			}
			assertResultShape(t, r, e)
			if r.Value == Unknown && r.Reason != domain.UnknownCrossProductContextGap {
				t.Errorf("reason = %s, want cross-product-context-gap", r.Reason)
			}
		})
	}
	// a chart version is never read as the app version
	dir := t.TempDir()
	writeFile(t, dir, "manifests/app.yaml", `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata: {name: ingress, namespace: argocd}
spec:
  source:
    repoURL: https://kubernetes.github.io/ingress-nginx
    chart: ingress-nginx
    targetRevision: 4.12.1
`)
	e := loadEnv(t, env.Inputs{Manifests: []string{dir + "/manifests"}})
	if ps := e.ProductInstances("ingress-nginx"); len(ps) == 0 || ps[0].VersionOf != env.VersionOfChart {
		t.Skipf("fixture does not detect a chart-kind entry: %+v", ps)
	}
	if r := EvaluateCondition(in("ingress-nginx", ">=1.0.0"), e, edge); r.Value != Unknown || r.Reason != domain.UnknownCrossProductContextGap {
		t.Errorf("chart-kind only = %s/%s, want unknown/cross-product-context-gap", r.Value, r.Reason)
	}
}

func TestEvaluateConditionNeverPanicsOnInvalid(t *testing.T) {
	e := condEnv(t, "", nil)
	r := EvaluateCondition(domain.Condition{Op: domain.OpField, Path: "x", State: domain.StateSet}, e, newEdge().edge)
	if r.Value != Unknown || r.Reason != domain.UnknownSemanticAmbiguity {
		t.Errorf("an unscoped field leaf = %s/%s, want unknown/semantic-ambiguity", r.Value, r.Reason)
	}
	if !strings.Contains(strings.Join(r.Needed, " "), "not well-formed") {
		t.Errorf("needed = %v", r.Needed)
	}
}

// A fact scoped to a named component must not clear an environment whose
// manifests simply do not include that workload (LOOP-DIAGNOSIS.md §7.8):
// unknown, unless the manifests are declared complete.
func TestEvaluateConditionNamedComponentAbsent(t *testing.T) {
	edge := newEdge().edge
	conds := []domain.Condition{
		{Op: domain.OpCLIFlag, Name: "--enable-certificate-owner-ref", Component: "webhook", State: domain.StateEquals, Values: []string{`"true"`}},
		{Op: domain.OpCLIFlag, Name: "--enable-certificate-owner-ref", Component: "webhook", State: domain.StateUnset},
		{Op: domain.OpEnvVar, Name: "LEADER_ELECT", Component: "webhook", State: domain.StateSet},
		{Op: domain.OpFeatureGate, Name: "ValidateCAA", Component: "webhook", State: domain.StateEnabled},
		{Op: domain.OpFeatureGate, Name: "ValidateCAA", Component: "webhook", State: domain.StateDisabled},
	}
	undeclared := condEnv(t, "", nil)
	dir := t.TempDir()
	writeFile(t, dir, "m/deploy.yaml", condDeploy)
	declared := loadEnv(t, env.Inputs{Manifests: []string{dir + "/m"}, ManifestsComplete: true})
	if !declared.ManifestsDeclaredComplete || len(declared.ManifestsCompleteEvidence) != 1 {
		t.Fatalf("declaration not recorded")
	}
	for _, c := range conds {
		r := EvaluateCondition(c, undeclared, edge)
		if r.Value != Unknown || r.Reason != domain.UnknownEnvironmentVisibilityGap || !strings.Contains(strings.Join(r.Needed, " "), "webhook workload") {
			t.Errorf("%s, workload not supplied: %s/%s %v, want unknown/environment-visibility-gap naming the workload", describe(c), r.Value, r.Reason, r.Needed)
		}
		r = EvaluateCondition(c, declared, edge)
		if r.Value != False || len(r.Checks) == 0 || len(r.Checks[0].Evidence) == 0 {
			t.Errorf("%s, manifests declared complete: %s, want false citing the declaration", describe(c), r.Value)
		}
	}
	// the declaration needs healthy manifests: partial ones stay unknown
	dir = t.TempDir()
	writeFile(t, dir, "m/deploy.yaml", condDeploy)
	writeFile(t, dir, "m/broken.yaml", "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: x\n")
	partial := loadEnv(t, env.Inputs{Manifests: []string{dir + "/m"}, ManifestsComplete: true})
	if r := EvaluateCondition(conds[0], partial, edge); r.Value != Unknown {
		t.Errorf("declared complete but partially parsed: %s, want unknown", r.Value)
	}
	// a values key stating the gate still decides, workload or not
	fg := domain.Condition{Op: domain.OpFeatureGate, Name: "B", Path: "featureGates", Component: "webhook", State: domain.StateDisabled}
	if r := EvaluateCondition(fg, undeclared, edge); r.Value != True {
		t.Errorf("gate stated in values for an unsupplied workload: %s, want true", r.Value)
	}
	// without manifests at all the declaration means nothing
	if e := loadEnv(t, env.Inputs{KubernetesVersion: "1.30", ManifestsComplete: true}); e.ManifestsDeclaredComplete {
		t.Error("a completeness declaration without manifests must not be recorded")
	}
}

// An unresolved reference is "not shown", not "not there": unknown unless the
// manifests are declared complete (clean parsing is not completeness).
func TestEvaluateConditionUnresolvedReference(t *testing.T) {
	edge := newEdge().edge
	orphan := `apiVersion: cert-manager.io/v1
kind: Certificate
metadata: {name: orphan, namespace: ns}
spec:
  secretName: orphan-tls
  issuerRef: {name: chart-installed-ca, kind: ClusterIssuer, group: cert-manager.io}
`
	cond := domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate", Name: "orphan",
		Of: []domain.Condition{{Op: domain.OpRef, Path: "spec.issuerRef", Kind: "ClusterIssuer", Of: []domain.Condition{field("spec.ca", domain.StateSet)}}}}

	undeclared := condEnv(t, "", map[string]string{"orphan.yaml": orphan})
	if undeclared.Health(env.DimManifests) != env.HealthOK {
		t.Fatalf("fixture must parse cleanly: %s", undeclared.Health(env.DimManifests))
	}
	r := EvaluateCondition(cond, undeclared, edge)
	if r.Value != Unknown || r.Reason != domain.UnknownEnvironmentVisibilityGap || !strings.Contains(strings.Join(r.Needed, " "), "ClusterIssuer chart-installed-ca") {
		t.Errorf("unresolved reference, cleanly parsed but undeclared manifests: %s/%s %v, want unknown naming the referenced object", r.Value, r.Reason, r.Needed)
	}
	// not(unresolved ref) cannot become evidence either
	if r := EvaluateCondition(domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate", Name: "orphan",
		Of: []domain.Condition{{Op: domain.OpNot, Of: cond.Of}}}, undeclared, edge); r.Value != Unknown {
		t.Errorf("not(unresolved ref) = %s, want unknown", r.Value)
	}

	dir := t.TempDir()
	writeFile(t, dir, "m/orphan.yaml", orphan)
	declared := loadEnv(t, env.Inputs{Manifests: []string{dir + "/m"}, ManifestsComplete: true})
	r = EvaluateCondition(cond, declared, edge)
	if r.Value != False {
		t.Fatalf("declared complete: %s %v, want false", r.Value, r.Needed)
	}
	cited := false
	for _, c := range r.Checks {
		for _, id := range c.Evidence {
			for _, d := range declared.ManifestsCompleteEvidence {
				cited = cited || id == d
			}
		}
	}
	if !cited {
		t.Errorf("a false from a declared-complete environment must cite the declaration: %+v", r.Checks)
	}
}
