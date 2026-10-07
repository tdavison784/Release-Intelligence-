package render

import (
	"sort"
	"strings"
	"testing"
)

// TestGoldenDiff checks the semantic diff of the checked-in golden render
// streams (produced by `helm template` from testdata/charts; regenerate with
// RI_RENDER_GOLDEN=1 go test -run TestHelmPairLive). No tools needed.
func TestGoldenDiff(t *testing.T) {
	from, to := goldenPair(t)
	// the golden streams carry a random token; suppress it like the live
	// nondeterminism probe would
	d := Diff(from, to, DiffOptions{Nondeterministic: []string{"core/v1/Secret/demo/demo-token|data.token"}})
	var got []string
	for _, c := range d.Changes {
		got = append(got, c.Summary(true))
	}
	sort.Strings(got)
	want := []string{
		`api-version-changed PodDisruptionBudget/demo-controller (ns demo) policy/v1beta1 → policy/v1`,
		`container-arg-changed Deployment/demo-controller (ns demo) spec.template.spec.containers[name=controller].args: "--leader-election-namespace=kube-system" → "--leader-election-namespace=demo-system"`,
		`container-arg-removed Deployment/demo-controller (ns demo) spec.template.spec.containers[name=controller].args: "--enable-certificate-owner-ref=false" → ∅`,
		`env-var-added Deployment/demo-controller (ns demo) spec.template.spec.containers[name=controller].env[name=GOMAXPROCS]: ∅ → "2"`,
		`image-changed Deployment/demo-controller (ns demo) spec.template.spec.containers[name=controller].image: "example.org/demo/controller:v1.0.0" → "example.org/demo/controller:v1.1.0"`,
		`label-added Deployment/demo-controller (ns demo) metadata.labels["app.kubernetes.io/component"]: ∅ → "controller"`,
		`rbac-permission-added ClusterRole/demo-controller rules coordination.k8s.io/leases:create`,
		`rbac-permission-added ClusterRole/demo-controller rules coordination.k8s.io/leases:get`,
		`rbac-permission-removed ClusterRole/demo-controller rules demo.example.org/widgets:update`,
		`resource-added ConfigMap/demo-feature-x (ns demo)`,
		`resource-added NetworkPolicy/demo-controller (ns demo)`,
		`resource-removed ConfigMap/demo-legacy-config (ns demo)`,
		`service-port-added Service/demo (ns demo) spec.ports[name=https]: ∅ → {"name":"https","port":443,"targetPort":10250}`,
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("diff:\n got:\n  %s\n want:\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
	// chart-version stamps and the checksum annotation are noise, counted
	if d.Suppressed["chart-metadata"] == 0 || d.Suppressed["checksum-annotation"] != 1 || d.Suppressed["nondeterministic"] != 1 {
		t.Errorf("suppressed = %v", d.Suppressed)
	}
	// the [] pattern form is the condition-language path
	c := findChange(d.Changes, ContainerArgRemoved, "--enable-certificate-owner-ref")
	if c == nil || c.Pattern != "spec.template.spec.containers[].args" || c.Container != "controller" {
		t.Fatalf("arg change: %+v", c)
	}
	r := findChange(d.Changes, RBACPermissionRemoved, "widgets/update")
	if r == nil || r.Permission.Group != "demo.example.org" || r.Source != "demo/templates/rbac.yaml" {
		t.Fatalf("rbac change: %+v", r)
	}
	img := findChange(d.Changes, ImageChanged, "example.org/demo/controller")
	if img == nil {
		t.Fatal("image change not named by repository")
	}
}

func objs(t *testing.T, y string) []Object {
	t.Helper()
	o, err := ParseObjects([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// TestNormalizationNoise: reorderings, empty-vs-omitted and map ordering are
// not changes; argument order is.
func TestNormalizationNoise(t *testing.T) {
	a := objs(t, `
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r, namespace: n, annotations: {}}
rules:
  - {apiGroups: [""], resources: [pods, secrets], verbs: [get, list]}
---
apiVersion: apps/v1
kind: Deployment
metadata: {name: d, namespace: n}
spec:
  template:
    spec:
      tolerations: [{key: a, operator: Exists}, {key: b, operator: Exists}]
      containers:
        - {name: a, image: x:1, args: [--one, --two], resources: {}}
        - {name: b, image: y:1, env: [{name: A, value: "1"}, {name: B, value: "2"}]}
`)
	b := objs(t, `
apiVersion: apps/v1
kind: Deployment
metadata: {namespace: n, name: d}
spec:
  template:
    spec:
      containers:
        - {name: b, env: [{name: B, value: "2"}, {name: A, value: "1"}], image: y:1}
        - {image: x:1, name: a, args: [--two, --one]}
      tolerations: [{key: b, operator: Exists}, {key: a, operator: Exists}]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata: {name: r, namespace: n}
rules:
  - {apiGroups: [""], resources: [secrets], verbs: [list, get]}
  - {apiGroups: [""], resources: [pods], verbs: [get, list]}
`)
	d := Diff(a, b, DiffOptions{})
	if len(d.Changes) != 1 || d.Changes[0].Class != ContainerArgChanged || d.Changes[0].Name != "(order)" {
		for _, c := range d.Changes {
			t.Logf("%s", c.Summary(true))
		}
		t.Fatalf("want exactly the argument reordering, got %d changes", len(d.Changes))
	}
}

// TestSensitiveValuesNeverSurface: Secret data and credential-named values
// are compared by digest; a change is visible, the value is not.
func TestSensitiveValuesNeverSurface(t *testing.T) {
	a := objs(t, `
apiVersion: v1
kind: Secret
metadata: {name: s}
stringData: {password: hunter2}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: c}
data: {apiToken: abc123, plain: one}
`)
	b := objs(t, `
apiVersion: v1
kind: Secret
metadata: {name: s}
stringData: {password: correct-horse}
---
apiVersion: v1
kind: ConfigMap
metadata: {name: c}
data: {apiToken: def456, plain: two}
`)
	d := Diff(a, b, DiffOptions{})
	if len(d.Changes) != 3 {
		t.Fatalf("changes = %d", len(d.Changes))
	}
	for _, c := range d.Changes {
		s := c.Summary(true) + deref(c.Before) + deref(c.After)
		for _, secret := range []string{"hunter2", "correct-horse", "abc123", "def456"} {
			if strings.Contains(s, secret) {
				t.Errorf("secret %q leaked: %s", secret, s)
			}
		}
		if strings.Contains(c.Path, "plain") == c.Sensitive {
			t.Errorf("sensitivity wrong for %s", c.Path)
		}
	}
}

func TestAPIVersionMigrationIsOneObject(t *testing.T) {
	a := objs(t, "apiVersion: autoscaling/v2beta2\nkind: HorizontalPodAutoscaler\nmetadata: {name: h, namespace: n}\nspec: {maxReplicas: 3}\n")
	b := objs(t, "apiVersion: autoscaling/v2\nkind: HorizontalPodAutoscaler\nmetadata: {name: h, namespace: n}\nspec: {maxReplicas: 5}\n")
	d := Diff(a, b, DiffOptions{})
	if len(d.Changes) != 2 || d.Changes[0].Class != APIVersionChanged || d.Changes[1].Class != FieldChanged || d.Changes[1].Path != "spec.maxReplicas" {
		for _, c := range d.Changes {
			t.Logf("%s", c.Summary(true))
		}
		t.Fatal("want api-version-changed + field-changed on one object")
	}
	if d.Changes[0].Object.String() != "autoscaling/v2/HorizontalPodAutoscaler/n/h" || d.Changes[0].FromObject.Version != "v2beta2" {
		t.Errorf("identity: %+v", d.Changes[0])
	}
}

func TestParseObjectsRejectsGarbage(t *testing.T) {
	if _, err := ParseObjects([]byte("---\n- a\n- b\n")); err == nil {
		t.Error("a list document must be unparsable")
	}
	o, err := ParseObjects([]byte("---\n# Source: x/templates/a.yaml\n# just a comment\n---\napiVersion: v1\nkind: List\nitems:\n- {apiVersion: v1, kind: ConfigMap, metadata: {name: a}}\n"))
	if err != nil || len(o) != 1 || o[0].ID.Kind != "ConfigMap" {
		t.Fatalf("list expansion: %v %+v", err, o)
	}
}
