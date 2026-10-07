package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

var testHints = []ProductHint{
	{ID: "ingress-nginx", Repositories: []string{"registry.k8s.io/ingress-nginx/controller"}},
	{ID: "cert-manager", Repositories: []string{"quay.io/jetstack/cert-manager-controller", "quay.io/jetstack/cert-manager-webhook"}},
}

func find(e *Environment, product, source string) (ProductInstance, bool) {
	for _, p := range e.Products {
		if p.Product == product && p.Source == source {
			return p, true
		}
	}
	return ProductInstance{}, false
}

func TestInventoryDeclared(t *testing.T) {
	inv := writeTemp(t, "inventory.yaml", `
- product: Ingress-NGINX
  version: v1.12.1
  note: from the runbook
- product: cilium
  version: "1.16"
- product: istio-csr
  version: 0.12.0
`)
	e, err := Load(Inputs{KubernetesVersion: "1.31", Inventory: inv, ProductHints: testHints})
	if err != nil {
		t.Fatal(err)
	}
	if e.Health(DimProducts) != HealthOK {
		t.Fatalf("products health = %s, warnings %v", e.Health(DimProducts), e.ProductsStatus().Warnings)
	}
	cases := []struct {
		product, version, raw string
		catalog               bool
	}{
		{"ingress-nginx", "1.12.1", "v1.12.1", true}, // id normalized to the catalog id, version to semver
		{"cilium", "1.16", "1.16", false},            // a line stays a line: no patch-level claim is invented
		{"istio-csr", "0.12.0", "0.12.0", false},     // not in the catalog: normalized name
		{"kubernetes", "1.31", "1.31", false},        // --kubernetes is part of the inventory
	}
	for _, c := range cases {
		p, ok := find(e, c.product, ProductDeclared)
		if !ok {
			t.Errorf("%s: not in inventory %+v", c.product, e.Products)
			continue
		}
		if p.Version != c.version || p.RawVersion != c.raw || p.Catalog != c.catalog || len(p.Evidence) != 1 {
			t.Errorf("%s: got %+v, want version %q raw %q catalog %v with one evidence", c.product, p, c.version, c.raw, c.catalog)
		}
	}
	ing, _ := e.Product("INGRESS-nginx")
	if ing.Note != "from the runbook" {
		t.Errorf("note lost: %+v", ing)
	}
	// evidence resolves to the file line
	for _, ev := range e.Evidence {
		if ev.ID == ing.Evidence[0] && !strings.Contains(ev.Locator, "L2") {
			t.Errorf("evidence locator = %q, want the entry's line", ev.Locator)
		}
	}
}

func TestInventoryAbsentVersusNotListed(t *testing.T) {
	// no inventory, only a cluster version: the dimension is absent — that
	// is not a statement that no other products run.
	e, _ := Load(Inputs{KubernetesVersion: "1.31"})
	if e.Health(DimProducts) != HealthAbsent {
		t.Errorf("health = %s, want absent", e.Health(DimProducts))
	}
	if r := e.ProductInRange("ingress-nginx", &domain.CompatibilityConstraint{Constraint: ">=1.12.6"}); r.Present {
		t.Errorf("absent dimension reported a present product: %+v", r)
	}
	// a supplied inventory that does not list a product: dimension ok, product not present
	inv := writeTemp(t, "inventory.yaml", "- product: cert-manager\n  version: 1.17.0\n")
	e, _ = Load(Inputs{Inventory: inv})
	if e.Health(DimProducts) != HealthOK {
		t.Errorf("health = %s, want ok", e.Health(DimProducts))
	}
	if _, ok := e.Product("ingress-nginx"); ok {
		t.Error("unlisted product reported present")
	}
	// an empty inventory file is supplied, and says nothing was listed
	empty := writeTemp(t, "inventory.yaml", "# nothing yet\n")
	e, _ = Load(Inputs{Inventory: empty})
	if e.Health(DimProducts) != HealthOK || len(e.Products) != 0 {
		t.Errorf("empty inventory: health %s products %v", e.Health(DimProducts), e.Products)
	}
}

func TestInventoryUnparsableAndMalformed(t *testing.T) {
	inv := writeTemp(t, "inventory.yaml", `
- product: ingress-nginx
  version: latest
- version: 1.2.3
- product: cilium
`)
	e, err := Load(Inputs{Inventory: inv, ProductHints: testHints})
	if err != nil {
		t.Fatal(err)
	}
	if e.Health(DimProducts) != HealthPartial {
		t.Fatalf("health = %s, want partial", e.Health(DimProducts))
	}
	p, ok := e.Product("ingress-nginx")
	if !ok || p.Version != "" || p.RawVersion != "latest" {
		t.Errorf("unparsable version must be kept raw with no normalized form: %+v", p)
	}
	if c, ok := e.Product("cilium"); !ok || c.Version != "" || c.RawVersion != "" {
		t.Errorf("an unstated version is present-without-version: %+v", c)
	}
	warns := strings.Join(e.ProductsStatus().Warnings, "\n")
	for _, want := range []string{`"latest" is not a parsable version`, "entry 2 has no product"} {
		if !strings.Contains(warns, want) {
			t.Errorf("warnings missing %q:\n%s", want, warns)
		}
	}
	// an unparsable version can never satisfy or violate a range
	r := e.ProductInRange("ingress-nginx", &domain.CompatibilityConstraint{Constraint: ">=1.12.6"})
	if !r.Present || r.Computable {
		t.Errorf("range check on unparsable version = %+v, want present, not computable", r)
	}
	// a file that is not a list is a warning, not an error
	bad := writeTemp(t, "inventory.yaml", "product: cilium\nversion: 1.16.1\n")
	e, err = Load(Inputs{Inventory: bad})
	if err != nil || e.Health(DimProducts) != HealthPartial {
		t.Errorf("non-list inventory: err %v health %s", err, e.Health(DimProducts))
	}
	// a missing file is an error: the caller named it
	if _, err := Load(Inputs{Inventory: filepath.Join(t.TempDir(), "nope.yaml")}); err == nil {
		t.Error("missing inventory file must fail Load")
	}
}

func TestInventoryDeclaredVersusDetectedConflict(t *testing.T) {
	inv := writeTemp(t, "inventory.yaml", "- product: ingress-nginx\n  version: v1.12.6\n")
	e, err := Load(Inputs{
		Inventory: inv, ProductHints: testHints,
		Images: []string{"registry.k8s.io/ingress-nginx/controller:v1.12.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	d, ok1 := find(e, "ingress-nginx", ProductDeclared)
	i, ok2 := find(e, "ingress-nginx", ProductDetectedImage)
	if !ok1 || !ok2 {
		t.Fatalf("both entries must be kept: %+v", e.Products)
	}
	if d.Version != "1.12.6" || i.Version != "1.12.1" {
		t.Errorf("versions = %q / %q", d.Version, i.Version)
	}
	if !strings.Contains(d.Conflict, "detected-image 1.12.1") || !strings.Contains(i.Conflict, "declared 1.12.6") {
		t.Errorf("conflict not visible on both: %q / %q", d.Conflict, i.Conflict)
	}
	if e.Health(DimProducts) != HealthPartial || !strings.Contains(strings.Join(e.ProductsStatus().Warnings, " "), "conflicting versions for ingress-nginx") {
		t.Errorf("conflict must degrade the dimension with a warning: %s %v", e.Health(DimProducts), e.ProductsStatus().Warnings)
	}
	// the engine must not pick a winner: the verdicts differ, so it is not computable
	r := e.ProductInRange("ingress-nginx", &domain.CompatibilityConstraint{Constraint: ">=1.12.6"})
	if !r.Present || !r.Conflict || r.Computable || r.InRange {
		t.Errorf("conflicting entries = %+v, want conflict and not computable", r)
	}
	// ...but when both sides land on the same side of the bound it is decidable
	r = e.ProductInRange("ingress-nginx", &domain.CompatibilityConstraint{Constraint: ">=1.10.0"})
	if !r.Computable || !r.InRange || r.Conflict {
		t.Errorf("agreeing verdicts = %+v", r)
	}
	// declared wins the single-entry accessor
	if p, _ := e.Product("ingress-nginx"); p.Source != ProductDeclared {
		t.Errorf("Product() = %+v, want the declared entry", p)
	}
}

func TestInventoryImageDetection(t *testing.T) {
	e, err := Load(Inputs{
		ProductHints: testHints,
		Images: []string{
			"quay.io/jetstack/cert-manager-controller:v1.17.0",
			"quay.io/jetstack/cert-manager-webhook:v1.17.0",              // same product+tag: one entry, two evidence
			"my.registry.local/jetstack/cert-manager-controller:v1.17.0", // mirror: path match, noted
			"registry.k8s.io/ingress-nginx/controller:latest",            // unparsable tag
			"registry.k8s.io/ingress-nginx/controller@sha256:" + strings.Repeat("a", 64),
			"nginx:1.25", // not a catalog image
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var cm, ing []ProductInstance
	for _, p := range e.Products {
		if p.Source != ProductDetectedImage {
			t.Errorf("unexpected non-image entry %+v", p)
		}
		switch p.Product {
		case "cert-manager":
			cm = append(cm, p)
		case "ingress-nginx":
			ing = append(ing, p)
		default:
			t.Errorf("unexpected product %+v", p)
		}
	}
	if len(cm) != 2 { // one exact-host entry (merged) and one mirror entry
		t.Fatalf("cert-manager entries = %+v", cm)
	}
	var exact, mirror ProductInstance
	for _, p := range cm {
		if p.Note != "" {
			mirror = p
		} else {
			exact = p
		}
	}
	if exact.Version != "1.17.0" || len(exact.Evidence) != 2 || !strings.Contains(mirror.Note, "path") {
		t.Errorf("exact %+v / mirror %+v", exact, mirror)
	}
	if len(ing) != 1 || ing[0].Version != "" || ing[0].RawVersion != "latest" {
		t.Errorf("ingress-nginx entries = %+v (digest-only references state no version)", ing)
	}
	if e.Health(DimProducts) != HealthPartial {
		t.Errorf("an unparsable tag must degrade the dimension, got %s", e.Health(DimProducts))
	}
}

func TestInventoryFromHelmDetectionKeepsChartVersionKind(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "d.yaml"), []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: x
  annotations:
    meta.helm.sh/release-name: my-cm
    helm.sh/chart: cert-manager-v1.17.0
  labels:
    app.kubernetes.io/name: cert-manager
    app.kubernetes.io/version: v1.17.1
`), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Load(Inputs{Manifests: []string{dir}, ProductHints: testHints})
	if err != nil {
		t.Fatal(err)
	}
	h, ok := find(e, "cert-manager", ProductDetectedHelm) // chart name wins over the arbitrary release name
	if !ok || h.Version != "1.17.0" || h.VersionOf != VersionOfChart || !h.Catalog {
		t.Errorf("helm entry = %+v", h)
	}
	l, ok := find(e, "cert-manager", ProductDetectedLabel)
	if !ok || l.Version != "1.17.1" || l.VersionOf != VersionOfApp {
		t.Errorf("label entry = %+v", l)
	}
	if h.Conflict != "" || l.Conflict != "" {
		t.Errorf("chart and app versions are different kinds and must not conflict: %q / %q", h.Conflict, l.Conflict)
	}
	if len(e.Installed) != 2 {
		t.Errorf("Installed must stay as it was: %+v", e.Installed)
	}
}

func TestProductInRange(t *testing.T) {
	inv := writeTemp(t, "inventory.yaml", `
- product: ingress-nginx
  version: v1.12.1
- product: cilium
  version: "1.16"
- product: karpenter
`)
	e, err := Load(Inputs{Inventory: inv, ProductHints: testHints})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, product           string
		c                       *domain.CompatibilityConstraint
		present, computable, in bool
	}{
		{"patch bound excludes 1.12.1", "ingress-nginx", &domain.CompatibilityConstraint{Constraint: ">=1.12.6"}, true, true, false},
		{"patch range includes", "ingress-nginx", &domain.CompatibilityConstraint{Constraint: ">=1.12.0, <1.13.0"}, true, true, true},
		{"minimum kind with patch bound", "ingress-nginx", &domain.CompatibilityConstraint{Kind: "minimum", Constraint: ">=1.12.6"}, true, true, false},
		{"line precision uses line semantics", "cilium", &domain.CompatibilityConstraint{Constraint: ">=1.16, <1.18"}, true, true, true},
		{"line below a line bound", "cilium", &domain.CompatibilityConstraint{Constraint: ">=1.17"}, true, true, false},
		{"line entry against patch bound is decided on the line", "cilium", &domain.CompatibilityConstraint{Kind: "minimum", Versions: []string{"1.16"}}, true, true, true},
		{"no version stated", "karpenter", &domain.CompatibilityConstraint{Constraint: ">=1.0.0"}, true, false, false},
		{"not listed", "istio", &domain.CompatibilityConstraint{Constraint: ">=1.0.0"}, false, false, false},
		{"nil constraint", "ingress-nginx", nil, true, false, false},
		{"garbage constraint", "ingress-nginx", &domain.CompatibilityConstraint{Constraint: "soonish"}, true, false, false},
	}
	for _, c := range cases {
		r := e.ProductInRange(c.product, c.c)
		if r.Present != c.present || r.Computable != c.computable || r.InRange != c.in {
			t.Errorf("%s: got present=%v computable=%v in=%v, want %v/%v/%v", c.name, r.Present, r.Computable, r.InRange, c.present, c.computable, c.in)
		}
	}
	var nilEnv *Environment
	if _, ok := nilEnv.Product("x"); ok {
		t.Error("nil environment has products")
	}
}

func TestInventoryRepoMode(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "inventory.yaml"), []byte("- product: ingress-nginx\n  version: 1.12.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "values.yaml"), []byte("replicaCount: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := Load(Inputs{Repo: repo, ProductHints: testHints})
	if err != nil {
		t.Fatal(err)
	}
	if p, ok := e.Product("ingress-nginx"); !ok || p.Version != "1.12.1" {
		t.Errorf("repo inventory.yaml not picked up: %+v", e.Products)
	}
	// an explicit --inventory overrides the repo's file
	other := writeTemp(t, "inv.yaml", "- product: ingress-nginx\n  version: 1.13.2\n")
	e, _ = Load(Inputs{Repo: repo, Inventory: other, ProductHints: testHints})
	if p, _ := e.Product("ingress-nginx"); p.Version != "1.13.2" || len(e.Products) != 1 {
		t.Errorf("explicit inventory must win: %+v", e.Products)
	}
}

func TestHintsFromCatalog(t *testing.T) {
	cat, err := catalog.LoadDir("../../products")
	if err != nil {
		t.Fatal(err)
	}
	hints := HintsFromCatalog(cat)
	got := map[string][]string{}
	for _, h := range hints {
		got[h.ID] = h.Repositories
	}
	found := false
	for _, r := range got["ingress-nginx"] {
		if r == "registry.k8s.io/ingress-nginx/controller" {
			found = true
		}
	}
	if !found {
		t.Errorf("ingress-nginx hints = %v", got["ingress-nginx"])
	}
	if HintsFromCatalog(nil) != nil {
		t.Error("nil catalog yields hints")
	}
}

func TestInventoryCompleteDeclaration(t *testing.T) {
	inv := writeTemp(t, "inventory.yaml", "complete: true\nproducts:\n  - product: ingress-nginx\n    version: v1.12.1\n")
	e, err := Load(Inputs{Inventory: inv})
	if err != nil {
		t.Fatal(err)
	}
	if !e.InventoryComplete || len(e.InventoryCompleteEvidence) != 1 || e.Health(DimProducts) != HealthOK {
		t.Fatalf("complete declaration not recorded: complete=%v evidence=%v health=%s", e.InventoryComplete, e.InventoryCompleteEvidence, e.Health(DimProducts))
	}
	if p, ok := e.Product("ingress-nginx"); !ok || p.Version != "1.12.1" {
		t.Errorf("products of the mapping form not read: %+v", p)
	}
	// the list form is never complete; complete: false neither
	for _, body := range []string{"- product: cilium\n  version: 1.16.1\n", "complete: false\nproducts:\n  - product: cilium\n    version: 1.16.1\n"} {
		e, err := Load(Inputs{Inventory: writeTemp(t, "inventory.yaml", body)})
		if err != nil || e.InventoryComplete {
			t.Errorf("%q: complete=%v err=%v, want not complete", body, e.InventoryComplete, err)
		}
	}
	// a malformed declaration is a warning and not complete
	e, err = Load(Inputs{Inventory: writeTemp(t, "inventory.yaml", "complete: yes-ish\nproducts: []\n")})
	if err != nil || e.InventoryComplete || e.Health(DimProducts) != HealthPartial {
		t.Errorf("malformed complete: complete=%v health=%s err=%v", e.InventoryComplete, e.Health(DimProducts), err)
	}
}
