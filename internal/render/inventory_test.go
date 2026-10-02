package render

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// goldenFromTo parses the two golden streams once per test.
func goldenFromTo(t *testing.T) (from, to []Object) {
	t.Helper()
	f, g := goldenPair(t)
	return f, g
}

func TestInventoryRBACPermission(t *testing.T) {
	from, to := goldenFromTo(t)
	// the demo chart's ClusterRole (1.0.0): widgets get/list/watch/update,
	// events create/patch, secrets get/list/watch; 1.1.0 drops widgets/update
	// and adds coordination.k8s.io leases get/create.
	perm := domain.Subject{Family: domain.SubjectRBACPermission, Product: "demo", Group: "demo.example.org", Name: "widgets"}
	occ, ok := Inventory(from, perm)
	if !ok {
		t.Fatal("rbac-permission must be a family the inventory reads")
	}
	if len(occ) != 4 {
		t.Fatalf("widgets in 1.0.0 = %d occurrences (one per verb), want 4: %v", len(occ), occ)
	}
	for _, o := range occ {
		if o.Object.Kind != "ClusterRole" || o.Object.Name != "demo-controller" {
			t.Errorf("occurrence on %s, want ClusterRole/demo-controller", o.Object)
		}
	}
	occ, _ = Inventory(to, perm)
	if len(occ) != 3 {
		t.Fatalf("widgets in 1.1.0 = %d occurrences, want 3 (update removed): %v", len(occ), occ)
	}

	update := domain.Subject{Family: domain.SubjectRBACPermission, Product: "demo", Name: "widgets/update"}
	if occ, _ := Inventory(from, update); len(occ) != 1 {
		t.Errorf("widgets/update in 1.0.0 = %d occurrences, want 1", len(occ))
	}
	if occ, _ := Inventory(to, update); len(occ) != 0 {
		t.Errorf("widgets/update in 1.1.0 = %d occurrences, want 0", len(occ))
	}

	// Component narrows to the role; a non-matching role sees nothing.
	scoped := domain.Subject{Family: domain.SubjectRBACPermission, Product: "demo", Component: "demo-controller", Name: "secrets"}
	if occ, _ := Inventory(from, scoped); len(occ) != 3 {
		t.Errorf("secrets on demo-controller in 1.0.0 = %d occurrences, want 3", len(occ))
	}
	other := domain.Subject{Family: domain.SubjectRBACPermission, Product: "demo", Component: "other-role", Name: "secrets"}
	if occ, _ := Inventory(from, other); len(occ) != 0 {
		t.Errorf("a subject scoped to another role must have no occurrences, got %d", len(occ))
	}
}

func TestInventoryImage(t *testing.T) {
	from, to := goldenFromTo(t)
	subj := domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/demo/controller"}
	occ, ok := Inventory(from, subj)
	if !ok || len(occ) != 1 {
		t.Fatalf("controller image in 1.0.0 = %d occurrences (ok=%v), want 1", len(occ), ok)
	}
	if want := "example.org/demo/controller:v1.0.0"; occ[0].Value != want {
		t.Errorf("image value %q, want %q", occ[0].Value, want)
	}
	if occ[0].Path == "" || occ[0].Object.Kind != "Deployment" {
		t.Errorf("occurrence path/object not filled: %+v", occ[0])
	}
	occ, _ = Inventory(to, subj)
	if len(occ) != 1 || occ[0].Value != "example.org/demo/controller:v1.1.0" {
		t.Fatalf("controller image in 1.1.0 = %+v, want the v1.1.0 reference", occ)
	}
	absent := domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "example.org/other/controller"}
	if occ, _ := Inventory(from, absent); len(occ) != 0 {
		t.Errorf("an unused repository must have no occurrences, got %d", len(occ))
	}
}

func TestInventoryCLIFlagAndFeatureGate(t *testing.T) {
	from, to := goldenFromTo(t)
	led := domain.Subject{Family: domain.SubjectCLIFlag, Product: "demo", Name: "leader-election-namespace"}
	occ, ok := Inventory(from, led)
	if !ok || len(occ) != 1 || occ[0].Value != "kube-system" {
		t.Fatalf("leader-election-namespace in 1.0.0 = %+v (ok=%v), want kube-system", occ, ok)
	}
	if occ, _ := Inventory(to, led); len(occ) != 1 || occ[0].Value != "demo-system" {
		t.Fatalf("leader-election-namespace in 1.1.0 = %+v, want demo-system", occ)
	}
	gone := domain.Subject{Family: domain.SubjectCLIFlag, Product: "demo", Name: "enable-certificate-owner-ref"}
	if occ, _ := Inventory(from, gone); len(occ) != 1 || occ[0].Value != "false" {
		t.Errorf("enable-certificate-owner-ref in 1.0.0 = %+v, want value false", occ)
	}
	if occ, _ := Inventory(to, gone); len(occ) != 0 {
		t.Errorf("enable-certificate-owner-ref in 1.1.0 = %d occurrences, want 0", len(occ))
	}
	// a bare flag (no =value) is an occurrence with an empty value
	v := domain.Subject{Family: domain.SubjectCLIFlag, Product: "demo", Name: "v"}
	if occ, _ := Inventory(from, v); len(occ) != 1 || occ[0].Value != "2" {
		t.Errorf("--v=2 in 1.0.0 = %+v, want value 2", occ)
	}
	// component narrows to the container
	bad := domain.Subject{Family: domain.SubjectCLIFlag, Product: "demo", Name: "v", Component: "other"}
	if occ, _ := Inventory(from, bad); len(occ) != 0 {
		t.Errorf("a flag scoped to another container must have no occurrences, got %d", len(occ))
	}
	// feature gates are read from --feature-gates tokens; the demo chart has none
	gate := domain.Subject{Family: domain.SubjectFeatureGate, Product: "demo", Name: "SomeGate"}
	occ, ok = Inventory(from, gate)
	if !ok || len(occ) != 0 {
		t.Fatalf("a chart without --feature-gates must yield zero occurrences (ok=%v), got %v", ok, occ)
	}
}

func TestInventoryEnvVar(t *testing.T) {
	from, to := goldenFromTo(t)
	ns := domain.Subject{Family: domain.SubjectEnvVar, Product: "demo", Name: "POD_NAMESPACE"}
	occ, ok := Inventory(from, ns)
	if !ok || len(occ) != 1 {
		t.Fatalf("POD_NAMESPACE in 1.0.0 = %d occurrences (ok=%v), want 1", len(occ), ok)
	}
	if occ[0].Path == "" {
		t.Errorf("occurrence path not filled: %+v", occ[0])
	}
	gomaxprocs := domain.Subject{Family: domain.SubjectEnvVar, Product: "demo", Name: "GOMAXPROCS"}
	if occ, _ := Inventory(from, gomaxprocs); len(occ) != 0 {
		t.Errorf("GOMAXPROCS in 1.0.0 = %d occurrences, want 0", len(occ))
	}
	if occ, _ := Inventory(to, gomaxprocs); len(occ) != 1 || occ[0].Value != `"2"` {
		t.Errorf("GOMAXPROCS in 1.1.0 = %+v, want one occurrence of \"2\"", occ)
	}
}

func TestInventoryGVK(t *testing.T) {
	from, _ := goldenFromTo(t)
	svc := domain.Subject{Family: domain.SubjectGVK, Product: "demo", Version: "v1", Kind: "Service"}
	occ, ok := Inventory(from, svc)
	if !ok || len(occ) != 1 || occ[0].Object.Name != "demo" {
		t.Fatalf("v1 Service in 1.0.0 = %+v (ok=%v), want one object occurrence", occ, ok)
	}
	// a CRD's served version is an occurrence too
	objs, err := ParseObjects([]byte(`# Source: crds/x.yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: widgets.demo.example.org
spec:
  group: demo.example.org
  names:
    kind: Widget
  versions:
    - name: v1
      served: true
    - name: v2alpha1
      served: false
`))
	if err != nil {
		t.Fatal(err)
	}
	v1 := domain.Subject{Family: domain.SubjectGVK, Product: "demo", Group: "demo.example.org", Version: "v1", Kind: "Widget"}
	if occ, _ := Inventory(objs, v1); len(occ) != 1 {
		t.Errorf("served CRD version v1 = %d occurrences, want 1", len(occ))
	}
	v2 := domain.Subject{Family: domain.SubjectGVK, Product: "demo", Group: "demo.example.org", Version: "v2alpha1", Kind: "Widget"}
	if occ, _ := Inventory(objs, v2); len(occ) != 0 {
		t.Errorf("an unserved CRD version must have no occurrences, got %d", len(occ))
	}
}

func TestInventoryUnsupportedFamily(t *testing.T) {
	from, _ := goldenFromTo(t)
	if _, ok := Inventory(from, domain.Subject{Family: domain.SubjectHelmValue, Product: "demo", Path: "replicas"}); ok {
		t.Error("helm-value has no rendered representation the inventory reads")
	}
	if _, ok := Inventory(from, domain.Subject{Family: domain.SubjectProtocolBehavior, Product: "demo", Name: "x"}); ok {
		t.Error("protocol-behavior has no rendered representation")
	}
}
