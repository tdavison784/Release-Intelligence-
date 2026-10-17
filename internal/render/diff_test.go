package render

import (
	"github.com/tdavison784/release-intelligence/internal/domain"
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

// crdObjects is a CustomResourceDefinition whose schema carries description
// text (root and per-property) and, when deprecated, one more schema field.
func crdObjects(t *testing.T, schemaDesc, modeDesc, modeExtra string) []Object {
	t.Helper()
	extra := ""
	if modeExtra != "" {
		extra = "          " + modeExtra + "\n"
	}
	return objs(t, `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: widgets.demo.example.org}
spec:
  group: demo.example.org
  names: {kind: Widget, plural: widgets}
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          description: `+schemaDesc+`
          properties:
            spec:
              type: object
              properties:
                mode:
                  type: string
                  description: `+modeDesc+`
`+extra)
}

// OpenAPI description text inside a CRD schema is its own declared class — it
// documents the API, it configures nothing. Everything else in the schema
// keeps its ordinary class, and a "description" field of a non-CRD object is
// an ordinary field change.
func TestCRDDescriptionIsItsOwnClass(t *testing.T) {
	d := Diff(crdObjects(t, `"Widgets v1"`, `"defaults to safe"`, ""),
		crdObjects(t, `"The Widget API"`, `"defaults to fast"`, ""), DiffOptions{})
	if len(d.Changes) != 2 {
		t.Fatalf("changes = %s", changeSummaries(d))
	}
	for _, c := range d.Changes {
		if c.Class != CRDDescriptionChanged || c.Name != "description" {
			t.Errorf("description diff = %s, want crd-description-changed", c.Summary(true))
		}
		if !strings.Contains(c.Path, "openAPIV3Schema") {
			t.Errorf("description class applied outside the schema: %s", c.Path)
		}
	}
}

// A real schema difference beside a reworded description keeps its ordinary
// class: the description class must swallow documentation, not structure.
func TestCRDDescriptionDoesNotSwallowSchemaChanges(t *testing.T) {
	d := Diff(crdObjects(t, `"Widgets"`, `"safe"`, ""),
		crdObjects(t, `"Widgets"`, `"fast"`, "deprecated: true"), DiffOptions{})
	classes := map[ChangeClass]int{}
	for _, c := range d.Changes {
		classes[c.Class]++
	}
	if classes[CRDDescriptionChanged] != 1 || classes[FieldAdded] != 1 || len(d.Changes) != 2 {
		t.Fatalf("classes = %v (%s)", classes, changeSummaries(d))
	}
}

// crdWithDescriptionField is a CRD whose spec declares a real FIELD named
// "description" (schema node fieldSchema) next to documented text.
func crdWithDescriptionField(t *testing.T, fieldSchema string) []Object {
	t.Helper()
	field := ""
	if fieldSchema != "" {
		field = "                description: " + fieldSchema + "\n"
	}
	return objs(t, `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: widgets.demo.example.org}
spec:
  group: demo.example.org
  names: {kind: Widget, plural: widgets}
  scope: Namespaced
  versions:
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
`+field+`                mode: {type: string}
`)
}

// A schema property NAMED "description" is API structure, not documentation:
// adding, removing or retyping the field keeps the ordinary field classes —
// only the `description` keyword with text (or absent) values is the
// documentation class.
func TestCRDFieldNamedDescriptionIsStructure(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     ChangeClass
	}{
		{"field added", "", "{type: string, description: \"what the widget is for\"}", FieldAdded},
		{"field removed", "{type: string}", "", FieldRemoved},
		{"field retyped", "{type: string}", "{type: integer}", FieldChanged},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := Diff(crdWithDescriptionField(t, c.from), crdWithDescriptionField(t, c.to), DiffOptions{})
			if len(d.Changes) != 1 || d.Changes[0].Class != c.want {
				t.Fatalf("changes = %s, want one %s", changeSummaries(d), c.want)
			}
		})
	}
	// …while the field's own documentation keyword is still documentation
	d := Diff(crdWithDescriptionField(t, "{type: string, description: old}"), crdWithDescriptionField(t, "{type: string, description: new}"), DiffOptions{})
	if len(d.Changes) != 1 || d.Changes[0].Class != CRDDescriptionChanged || DocumentedField(d.Changes[0]) != "description" {
		t.Fatalf("documentation of a field named description = %s", changeSummaries(d))
	}
}

// DocumentedField names the property a description keyword documents,
// through list items and map values; "" where no named property owns it.
func TestDocumentedField(t *testing.T) {
	const root = "spec.versions[].schema.openAPIV3Schema."
	cases := map[string]string{
		root + "properties.spec.properties.privateKey.properties.rotationPolicy.description": "rotationPolicy",
		root + "properties.spec.properties.solvers.items.description":                        "solvers",
		root + "properties.spec.properties.labels.additionalProperties.description":          "labels",
		root + "properties.spec.description":                                                 "spec",
		root + "description":                                                                 "",
		root + "properties.spec.allOf[].description":                                         "",
	}
	for pattern, want := range cases {
		if got := DocumentedField(Change{Class: CRDDescriptionChanged, Pattern: pattern}); got != want {
			t.Errorf("%s: %q, want %q", pattern, got, want)
		}
	}
	if got := DocumentedField(Change{Class: FieldChanged, Pattern: root + "properties.spec.properties.mode.description"}); got != "" {
		t.Errorf("not a documentation change: %q", got)
	}
}

// The domain names the documentation class without importing render.
func TestDocumentationClassMatchesDomain(t *testing.T) {
	if string(CRDDescriptionChanged) != domain.RenderedClassDocumentation {
		t.Fatalf("render %q != domain %q", CRDDescriptionChanged, domain.RenderedClassDocumentation)
	}
}

// A "description" field of a non-CRD object is data, not schema documentation.
func TestDescriptionFieldOfNonCRDStaysAFieldChange(t *testing.T) {
	from := objs(t, "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: c}\ndata:\n  description: old\n")
	to := objs(t, "apiVersion: v1\nkind: ConfigMap\nmetadata: {name: c}\ndata:\n  description: new\n")
	d := Diff(from, to, DiffOptions{})
	if len(d.Changes) != 1 || d.Changes[0].Class != FieldChanged {
		t.Fatalf("changes = %s", changeSummaries(d))
	}
}

func changeSummaries(d DiffResult) string {
	var out []string
	for _, c := range d.Changes {
		out = append(out, c.Summary(true))
	}
	return strings.Join(out, "; ")
}
