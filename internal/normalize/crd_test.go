package normalize

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const crdV1 = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.example.io
spec:
  group: example.io
  scope: Namespaced
  names:
    kind: Widget
    plural: widgets
  versions:
    - name: v1alpha1
      served: true
      storage: false
      deprecated: true
      deprecationWarning: "use v1"
      schema:
        openAPIV3Schema:
          type: object
          properties:
            spec:
              type: object
              properties:
                old:
                  type: string
    - name: v1
      served: true
      storage: true
      schema:
        openAPIV3Schema:
          type: object
          x-kubernetes-preserve-unknown-fields: true
          properties:
            apiVersion: {type: string}
            spec:
              type: object
              properties:
                dnsNames:
                  type: array
                  items: {type: string}
                foo:
                  type: array
                  items:
                    type: object
                    properties:
                      bar: {type: string}
                      baz:
                        type: array
                        items:
                          type: object
                          properties:
                            qux: {type: integer}
                matrix:
                  type: array
                  items:
                    type: array
                    items: {type: string}
                issuerRef:
                  type: object
                  properties:
                    name: {type: string}
                    kind: {type: string}
                labels:
                  type: object
                  additionalProperties:
                    type: object
                    properties:
                      hidden: {type: string}
                x-kubernetes-thing: {type: string}
                intOrString:
                  x-kubernetes-int-or-string: true
            status:
              type: object
`

const crdV1beta1Versions = `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: gadgets.example.io
spec:
  group: example.io
  scope: Cluster
  names:
    kind: Gadget
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            shared: {type: string}
  versions:
    - name: v1beta1
      served: true
      storage: true
    - name: v1alpha1
      served: false
      storage: false
      schema:
        openAPIV3Schema:
          properties:
            spec:
              properties:
                own: {type: string}
`

const crdV1beta1Legacy = `apiVersion: apiextensions.k8s.io/v1beta1
kind: CustomResourceDefinition
metadata:
  name: legacy.example.io
spec:
  group: example.io
  version: v1
  names:
    kind: Legacy
  validation:
    openAPIV3Schema:
      properties:
        spec:
          properties:
            a: {type: string}
`

func TestCRDSnapshotV1(t *testing.T) {
	snap, err := CRDSnapshot([]byte(crdV1))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.CRDs) != 1 {
		t.Fatalf("crds = %d", len(snap.CRDs))
	}
	c := snap.CRDs[0]
	if c.Name != "widgets.example.io" || c.Group != "example.io" || c.Kind != "Widget" || c.Scope != "Namespaced" || len(c.Versions) != 2 {
		t.Fatalf("%+v", c)
	}
	v0, v1 := c.Versions[0], c.Versions[1]
	if v0.Name != "v1alpha1" || !v0.Served || v0.Storage || !v0.Deprecated || v0.DeprecationWarning != "use v1" {
		t.Errorf("v1alpha1: %+v", v0)
	}
	if !reflect.DeepEqual(v0.SchemaPaths, []string{"spec", "spec.old"}) {
		t.Errorf("v1alpha1 paths: %v", v0.SchemaPaths)
	}
	if v1.Name != "v1" || !v1.Served || !v1.Storage || v1.Deprecated {
		t.Errorf("v1: %+v", v1)
	}
	want := []string{
		"apiVersion",
		"spec",
		"spec.dnsNames[]",
		"spec.foo[]",
		"spec.foo[].bar",
		"spec.foo[].baz[]",
		"spec.foo[].baz[].qux",
		"spec.intOrString",
		"spec.issuerRef",
		"spec.issuerRef.kind",
		"spec.issuerRef.name",
		"spec.labels",
		"spec.matrix[][]",
		"status",
	}
	if !sort.StringsAreSorted(v1.SchemaPaths) || !reflect.DeepEqual(v1.SchemaPaths, want) {
		t.Errorf("v1 paths:\n got %v\nwant %v", v1.SchemaPaths, want)
	}
}

func TestCRDSnapshotBetaAndMixedStreams(t *testing.T) {
	list := `apiVersion: v1
kind: List
items:
  - apiVersion: apiextensions.k8s.io/v1
    kind: CustomResourceDefinition
    metadata: {name: inlist.example.io}
    spec:
      group: example.io
      names: {kind: InList}
      scope: Namespaced
      versions:
        - {name: v1, served: true, storage: true}
`
	stream1 := "# header comment\n---\napiVersion: v1\nkind: ConfigMap\nmetadata: {name: cm}\n---\n" + crdV1beta1Versions + "---\n" + crdV1 + "---\n"
	stream2 := crdV1beta1Legacy + "---\n" + list + "---\n" + crdV1 // a duplicate of widgets.example.io
	snap, err := CRDSnapshot([]byte(stream1), []byte(stream2))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range snap.CRDs {
		names = append(names, c.Name)
	}
	if want := []string{"gadgets.example.io", "inlist.example.io", "legacy.example.io", "widgets.example.io"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("names = %v, want %v", names, want)
	}
	byName := map[string]domain.CRDSummary{}
	for _, c := range snap.CRDs {
		byName[c.Name] = c
	}
	g := byName["gadgets.example.io"]
	if g.Scope != "Cluster" || g.Kind != "Gadget" || len(g.Versions) != 2 {
		t.Fatalf("gadget: %+v", g)
	}
	// v1beta1: the top-level validation applies to versions without own schema
	if !reflect.DeepEqual(g.Versions[0].SchemaPaths, []string{"spec", "spec.shared"}) || !g.Versions[0].Served || !g.Versions[0].Storage {
		t.Errorf("v1beta1 version: %+v", g.Versions[0])
	}
	if !reflect.DeepEqual(g.Versions[1].SchemaPaths, []string{"spec", "spec.own"}) || g.Versions[1].Served {
		t.Errorf("v1alpha1 version: %+v", g.Versions[1])
	}
	l := byName["legacy.example.io"]
	if len(l.Versions) != 1 || l.Versions[0].Name != "v1" || !l.Versions[0].Served || !l.Versions[0].Storage || !reflect.DeepEqual(l.Versions[0].SchemaPaths, []string{"spec", "spec.a"}) {
		t.Errorf("legacy: %+v", l)
	}
	if il := byName["inlist.example.io"]; len(il.Versions) != 1 || il.Versions[0].SchemaPaths != nil {
		t.Errorf("inlist: %+v", il)
	}
}

func TestCRDSnapshotEdgeCases(t *testing.T) {
	// no CRDs at all: empty, non-nil list
	snap, err := CRDSnapshot([]byte("apiVersion: v1\nkind: Namespace\nmetadata: {name: x}\n"), nil, []byte(""))
	if err != nil || snap == nil || snap.CRDs == nil || len(snap.CRDs) != 0 {
		t.Errorf("%+v %v", snap, err)
	}
	if snap, err = CRDSnapshot(); err != nil || len(snap.CRDs) != 0 {
		t.Errorf("no streams: %+v %v", snap, err)
	}
	// invalid YAML only: error; invalid after a valid CRD: the valid CRD is kept
	if _, err := CRDSnapshot([]byte("a: [unclosed")); err == nil {
		t.Error("expected parse error")
	}
	snap, err = CRDSnapshot([]byte(crdV1 + "---\nb: [unclosed\n"))
	if err != nil || len(snap.CRDs) != 1 {
		t.Errorf("%+v %v", snap, err)
	}
	// a CRD with the wrong apiVersion is not a CRD
	snap, _ = CRDSnapshot([]byte("apiVersion: v1\nkind: CustomResourceDefinition\nmetadata: {name: x}\nspec: {}\n"))
	if len(snap.CRDs) != 0 {
		t.Errorf("%+v", snap)
	}
}

func TestCRDSchemaPathCap(t *testing.T) {
	var b strings.Builder
	b.WriteString("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nmetadata: {name: big.example.io}\nspec:\n  group: example.io\n  names: {kind: Big}\n  scope: Namespaced\n  versions:\n    - name: v1\n      served: true\n      storage: true\n      schema:\n        openAPIV3Schema:\n          properties:\n")
	for i := 0; i < MaxSchemaPaths+500; i++ {
		fmt.Fprintf(&b, "            f%05d: {type: string}\n", i)
	}
	snap, err := CRDSnapshot([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	paths := snap.CRDs[0].Versions[0].SchemaPaths
	if len(paths) != MaxSchemaPaths || !sort.StringsAreSorted(paths) {
		t.Errorf("paths = %d, sorted=%v", len(paths), sort.StringsAreSorted(paths))
	}
	// the cap keeps the alphabetically first properties
	if paths[0] != "f00000" || paths[len(paths)-1] != fmt.Sprintf("f%05d", MaxSchemaPaths-1) {
		t.Errorf("first %q last %q", paths[0], paths[len(paths)-1])
	}
}

func TestCRDSnapshotCertManagerFixture(t *testing.T) {
	snap, err := CRDSnapshot(fixture(t, "certmanager/crds.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.CRDs) != 2 {
		t.Fatalf("crds = %d (the ServiceAccount must be ignored)", len(snap.CRDs))
	}
	if snap.CRDs[0].Name != "certificaterequests.cert-manager.io" || snap.CRDs[1].Name != "certificates.cert-manager.io" {
		t.Errorf("order: %s, %s", snap.CRDs[0].Name, snap.CRDs[1].Name)
	}
	cert := snap.CRDs[1]
	if cert.Group != "cert-manager.io" || cert.Kind != "Certificate" || cert.Scope != "Namespaced" {
		t.Errorf("%+v", cert)
	}
	if len(cert.Versions) != 1 {
		t.Fatalf("versions: %+v", cert.Versions)
	}
	v := cert.Versions[0]
	if v.Name != "v1" || !v.Served || !v.Storage || v.Deprecated {
		t.Errorf("v1: %+v", v)
	}
	has := map[string]bool{}
	for _, p := range v.SchemaPaths {
		has[p] = true
	}
	for _, p := range []string{
		"spec", "spec.issuerRef", "spec.issuerRef.name", "spec.issuerRef.kind", "spec.issuerRef.group",
		"spec.dnsNames[]", "spec.secretName", "spec.privateKey.rotationPolicy", "spec.additionalOutputFormats[].type",
		"status", "status.conditions[]", "status.conditions[].type",
	} {
		if !has[p] {
			t.Errorf("missing schema path %q (have %d paths)", p, len(v.SchemaPaths))
		}
	}
	for p := range has {
		if strings.Contains(p, "x-kubernetes") {
			t.Errorf("x-kubernetes key leaked: %s", p)
		}
	}
	if !sort.StringsAreSorted(v.SchemaPaths) {
		t.Error("paths not sorted")
	}
}

func TestCRDSnapshotFieldSchemas(t *testing.T) {
	const doc = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: knobs.example.io}
spec:
  group: example.io
  names: {kind: Knob}
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
              required: [mode]
              properties:
                mode:
                  type: string
                  enum: [Fast, Slow]
                  default: Fast
                rotationPolicy: {type: string, default: Never}
                replicas: {type: integer, default: 3}
                opts:
                  type: object
                  default: {b: 2, a: 1}
                kinds:
                  type: array
                  items: {type: string, enum: [A, B]}
`
	snap, err := CRDSnapshot([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]domain.CRDFieldSchema{}
	for _, f := range snap.CRDs[0].Versions[0].Fields {
		got[f.Path] = f
	}
	if n := len(snap.CRDs[0].Versions[0].Fields); n != len(snap.CRDs[0].Versions[0].SchemaPaths) {
		t.Errorf("fields = %d, paths = %d", n, len(snap.CRDs[0].Versions[0].SchemaPaths))
	}
	check := func(path string, want domain.CRDFieldSchema) {
		t.Helper()
		want.Path = path
		if !reflect.DeepEqual(got[path], want) {
			t.Errorf("%s: got %+v want %+v", path, got[path], want)
		}
	}
	check("spec", domain.CRDFieldSchema{Type: "object"})
	check("spec.mode", domain.CRDFieldSchema{Type: "string", Default: `"Fast"`, Enum: []string{`"Fast"`, `"Slow"`}, Required: true})
	check("spec.rotationPolicy", domain.CRDFieldSchema{Type: "string", Default: `"Never"`})
	check("spec.replicas", domain.CRDFieldSchema{Type: "integer", Default: `3`})
	check("spec.opts", domain.CRDFieldSchema{Type: "object", Default: `{"a":1,"b":2}`})
	check("spec.kinds[]", domain.CRDFieldSchema{Type: "array", Enum: []string{`"A"`, `"B"`}})
}
