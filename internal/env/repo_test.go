package env

// Repository-mode tests: the checked-in fixture testdata/customer-repo (a
// miniature customer repository) walked end to end, plus the GVK usage
// inventory and installed-product identification it exercises.

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const repoFixture = "testdata/customer-repo"

func discoveryOf(e *Environment) map[string]RepoDiscovery {
	out := map[string]RepoDiscovery{}
	for _, d := range e.Discovered {
		out[d.Path] = d
	}
	return out
}

func gvkOf(e *Environment, group, version, kind string) *GVKUsage {
	for i, u := range e.GVKUsage {
		if u.Group == group && u.Version == version && u.Kind == kind {
			return &e.GVKUsage[i]
		}
	}
	return nil
}

func TestRepoDiscovery(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	if e.RepoRoot != repoFixture {
		t.Errorf("repo root = %q", e.RepoRoot)
	}
	d := discoveryOf(e)
	for path, kind := range map[string]string{
		"clusters/prod/values-cert-manager.yaml": DiscValues,
		"clusters/dev/values-cert-manager.yaml":  DiscValues,
		"base/namespace.yaml":                    DiscManifests,
		"base/deployment.yaml":                   DiscManifests,
		"base/certificate.yaml":                  DiscManifests,
		"crds/certificates.cert-manager.io.yaml": DiscManifests,
		"apps/cert-manager-argocd.yaml":          DiscArgoCD,
		"flux/cert-manager-helmrelease.yaml":     DiscFlux,
		"helmfile.yaml":                          DiscHelmfile,
		"kustomize/kustomization.yaml":           DiscKustomization,
		".github/workflows/mirror.yaml":          DiscWorkflow,
		"terraform/main.tf":                      DiscTerraform,
	} {
		got, ok := d[filepath.Join(repoFixture, path)]
		if !ok {
			t.Errorf("%s: not discovered (have %d entries)", path, len(e.Discovered))
			continue
		}
		if got.Kind != kind {
			t.Errorf("%s: kind = %q, want %q (detail %q)", path, got.Kind, kind, got.Detail)
		}
		if len(got.Evidence) == 0 {
			t.Errorf("%s: discovery without evidence", path)
		}
	}
	// the .gitlab-ci.yml is YAML without apiVersion/kind: silently skipped
	if _, ok := d[filepath.Join(repoFixture, ".gitlab-ci.yml")]; ok {
		t.Error(".gitlab-ci.yml must not be classified")
	}
	// the vendored chart is skipped as a whole
	for path := range d {
		if strings.Contains(path, "charts"+string(filepath.Separator)) {
			t.Errorf("chart tree must be skipped, got %q", path)
		}
	}
}

func TestRepoSkipsVendoredChartValues(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range e.ValuesKeys {
		if k.Path == "decoyKey" {
			t.Fatal("a vendored chart's values.yaml must not reach the values inventory")
		}
	}
	joined := strings.Join(e.Warnings, "\n")
	if !strings.Contains(joined, "chart source tree") {
		t.Errorf("chart skip is not explained in warnings: %v", e.Warnings)
	}
}

func TestRepoGVKInventory(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	// group/version/kind split
	cert := gvkOf(e, "cert-manager.io", "v1", "Certificate")
	if cert == nil {
		t.Fatalf("Certificate usage missing (have %+v)", e.GVKUsage)
	}
	// the core group keeps an empty Group
	if ns := gvkOf(e, "", "v1", "Namespace"); ns == nil {
		t.Errorf("core v1/Namespace usage missing: %+v", e.GVKUsage)
	}
	if dep := gvkOf(e, "apps", "v1", "Deployment"); dep == nil {
		t.Errorf("apps/v1 Deployment usage missing: %+v", e.GVKUsage)
	}
	if app := gvkOf(e, "argoproj.io", "v1alpha1", "Application"); app == nil {
		t.Errorf("Argo CD Application usage missing: %+v", e.GVKUsage)
	}
	if hr := gvkOf(e, "helm.toolkit.fluxcd.io", "v2", "HelmRelease"); hr == nil {
		t.Errorf("Flux HelmRelease usage missing: %+v", e.GVKUsage)
	}
	// names: the using manifest and the installed CRD (served versions feed
	// the same inventory)
	wantNames := []ResourceName{
		{Name: "certificates.cert-manager.io"},
		{Name: "example-com", Namespace: "istio-system"},
	}
	if !reflect.DeepEqual(cert.Names, wantNames) {
		t.Errorf("Certificate names = %+v, want %+v", cert.Names, wantNames)
	}
	// field paths, values-path syntax, from the using document
	for _, want := range []string{"spec.secretName", "spec.dnsNames", "spec.issuerRef.kind"} {
		found := false
		for _, p := range cert.FieldPaths {
			if p == want {
				found = true
			}
		}
		if !found {
			t.Errorf("Certificate field path %q missing (have %v)", want, cert.FieldPaths)
		}
	}
	// raw document refs, so a matcher can re-inspect (a leading comment line
	// belongs to the document, so both start at L1)
	files := map[string]int{}
	for _, doc := range cert.Documents {
		files[doc.File] = doc.StartLine
	}
	certFile := filepath.Join(repoFixture, "base", "certificate.yaml")
	crdFile := filepath.Join(repoFixture, "crds", "certificates.cert-manager.io.yaml")
	if files[certFile] != 1 || files[crdFile] != 1 {
		t.Errorf("Certificate documents = %+v", cert.Documents)
	}
	// every usage carries evidence that resolves in the pool
	pool := map[domain.EvidenceID]bool{}
	for _, ev := range e.Evidence {
		pool[ev.ID] = true
	}
	for _, u := range e.GVKUsage {
		if len(u.Evidence) == 0 {
			t.Errorf("GVK %s/%s %s has no evidence", u.Group, u.Version, u.Kind)
		}
		for _, id := range u.Evidence {
			if !pool[id] {
				t.Errorf("GVK %s/%s %s cites unknown evidence %s", u.Group, u.Version, u.Kind, id)
			}
		}
	}
	ev := evURI(*e, cert.Evidence[0])
	if ev.Locator != "L1" || (ev.URI != crdFile && ev.URI != certFile) {
		t.Errorf("first Certificate evidence = %+v", ev)
	}
}

func TestRepoInstalledProducts(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	type want struct {
		source           string
		product, version string
		chart            string
	}
	wants := []want{
		{InstalledArgoCD, "cert-manager", "v1.17.0", "cert-manager"},
		{InstalledFlux, "cert-manager", "1.*", "cert-manager"},
		{InstalledHelmAnnotation, "cert-manager", "v1.17.0", "cert-manager-v1.17.0"},
		{InstalledHelmfile, "cert-manager", "v1.17.0", "jetstack/cert-manager"},
		{InstalledLabel, "cert-manager", "v1.17.0", ""},
	}
	if len(e.Installed) != len(wants) {
		t.Fatalf("installed = %+v, want %d entries", e.Installed, len(wants))
	}
	for i, w := range wants {
		got := e.Installed[i]
		if got.Source != w.source || got.Product != w.product || got.Version != w.version || got.Chart != w.chart {
			t.Errorf("installed[%d] = %+v, want %+v", i, got, w)
		}
		if len(got.Evidence) == 0 {
			t.Errorf("installed[%d] (%s) without evidence", i, w.source)
		}
	}
	// the label entry records the instance
	if e.Installed[4].Instance != "cert-manager" {
		t.Errorf("label instance = %q", e.Installed[4].Instance)
	}
	// every guess cites the field it grounded on
	ev := evURI(*e, e.Installed[2].Evidence[0])
	if !strings.Contains(ev.Excerpt, "meta.helm.sh/release-name") {
		t.Errorf("helm-annotation evidence = %+v", ev)
	}
}

func TestRepoInlineValuesBecomeValuesKeys(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	// Argo CD spec.source.helm.values (block scalar) — flattened like a file;
	// several sources set replicaCount, so find the argo one by its evidence
	var argoEV domain.Evidence
	for _, k := range e.ValuesKeys {
		if k.Path != "replicaCount" {
			continue
		}
		ev := evURI(*e, k.Evidence[0])
		if strings.Contains(ev.URI, "cert-manager-argocd.yaml") {
			argoEV = ev
		}
	}
	if argoEV.ID == "" || !strings.Contains(argoEV.Locator, "$.spec.source.helm.values.replicaCount") {
		t.Fatalf("argo inline values missing or mislocated: %+v", argoEV)
	}
	// Argo valuesObject keeps exact line numbers from the original node
	for _, k := range e.ValuesKeys {
		if k.Path != "webhook.timeoutSeconds" {
			continue
		}
		if k.Line != 22 || k.Value != "10" {
			t.Errorf("valuesObject key = %+v", k)
		}
	}
	// Flux spec.values (string leaves keep the JSON encoding of a parsed
	// values file)
	var fluxKey *ValuesKey
	for i, k := range e.ValuesKeys {
		if k.Path == "global.leaderElection.namespace" {
			fluxKey = &e.ValuesKeys[i]
		}
	}
	if fluxKey == nil || fluxKey.Value != `"cert-manager"` {
		t.Fatalf("flux values key = %+v", fluxKey)
	}
	fx := evURI(*e, fluxKey.Evidence[0])
	if !strings.Contains(fx.Locator, "$.spec.values.global.leaderElection.namespace (L21)") {
		t.Errorf("flux values evidence = %+v", fx)
	}
}

func TestRepoImages(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	type key struct{ ref, source string }
	byRef := map[string]map[string]bool{}
	for _, im := range e.Images {
		if byRef[im.Reference] == nil {
			byRef[im.Reference] = map[string]bool{}
		}
		byRef[im.Reference][im.Source] = true
	}
	controller := filepath.Base("quay.io/jetstack/cert-manager-controller:v1.17.0")
	_ = controller
	if s := byRef["quay.io/jetstack/cert-manager-controller:v1.17.0"]; !s["manifest"] || !s["workflow"] || !s["kustomization"] {
		t.Errorf("controller image sources = %v (have %v)", s, e.Images)
	}
	if s := byRef["quay.io/jetstack/cert-manager-webhook:v1.17.0"]; !s["workflow"] || !s["terraform"] {
		t.Errorf("webhook image sources = %v (have %v)", s, e.Images)
	}
	// the workflow evidence points at the image line
	for _, im := range e.Images {
		if im.Source == "workflow" && strings.Contains(im.Reference, "webhook") {
			ev := evURI(*e, im.Evidence[0])
			if ev.Locator != "L16" || !strings.Contains(ev.URI, "mirror.yaml") {
				t.Errorf("workflow webhook image evidence = %+v", ev)
			}
		}
	}
}

func TestRepoWarnings(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(e.Warnings, "\n")
	for _, want := range []string{
		"2 values files",
		"is not built with kustomize",
		"templating (go templates, environments) is not evaluated",
		"valuesFrom",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("warning %q missing in %v", want, e.Warnings)
		}
	}
}

// TestRepoPrecedence: explicit flags compose with discovery — discovered
// files load first, explicit entries after (so they win per key), and a file
// named by both is loaded once.
func TestRepoPrecedence(t *testing.T) {
	e, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	base, err := Load(Inputs{Repo: repoFixture})
	if err != nil {
		t.Fatal(err)
	}
	_ = base

	extra := filepath.Join(t.TempDir(), "extra.yaml")
	if err := os.WriteFile(extra, []byte("replicaCount: 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	merged, err := Load(Inputs{Repo: repoFixture, ValuesFiles: []string{extra}})
	if err != nil {
		t.Fatal(err)
	}
	// the explicit file's key is present and applied after the discovered
	// ones (the last replicaCount wins per Helm merge semantics)
	last := -1
	for i, k := range merged.ValuesKeys {
		if k.Path == "replicaCount" && k.Value == "9" {
			last = i
		}
	}
	if last < 0 {
		t.Fatal("explicit values file missing")
	}
	for i, k := range merged.ValuesKeys {
		if k.Path == "replicaCount" && k.Value == "1" && i > last {
			t.Errorf("explicit values file must apply after the discovered ones")
		}
	}

	// an explicit manifest directory that overlaps discovery loads each file
	// once
	baseDocs := e.ManifestDocCount
	if _, err := Load(Inputs{Repo: repoFixture, Manifests: []string{filepath.Join(repoFixture, "base")}}); err != nil {
		t.Fatal(err)
	}
	again, err := Load(Inputs{Repo: repoFixture, Manifests: []string{filepath.Join(repoFixture, "base")}})
	if err != nil {
		t.Fatal(err)
	}
	if again.ManifestDocCount != baseDocs {
		t.Errorf("overlapping explicit manifests double-loaded: %d docs, want %d", again.ManifestDocCount, baseDocs)
	}
}

func TestRepoDeterministic(t *testing.T) {
	in := Inputs{Repo: repoFixture, KubernetesVersion: "1.28"}
	first, err := Load(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("repo-mode Load must be deterministic")
	}
	if !(&Inputs{}).Empty() {
		t.Fatal("zero inputs must be empty")
	}
	if (&Inputs{Repo: "x"}).Empty() {
		t.Fatal("repo-only inputs must not be empty")
	}
}

func TestRepoMissingRoot(t *testing.T) {
	if _, err := Load(Inputs{Repo: filepath.Join(repoFixture, "nope")}); err == nil {
		t.Error("a missing repo root is an error")
	}
	empty := t.TempDir()
	e, err := Load(Inputs{Repo: empty})
	if err != nil {
		t.Fatal(err)
	}
	if len(e.ValuesKeys) != 0 || len(e.Discovered) != 0 {
		t.Errorf("empty repo produced facts: %+v", e)
	}
	if len(e.Warnings) == 0 || !strings.Contains(e.Warnings[0], "no values or manifest files") {
		t.Errorf("empty repo is a warning: %v", e.Warnings)
	}
}

func TestSplitGroupVersion(t *testing.T) {
	for apiVersion, want := range map[string][2]string{
		"cert-manager.io/v1":        {"cert-manager.io", "v1"},
		"apps/v1":                   {"apps", "v1"},
		"v1":                        {"", "v1"},
		"helm.toolkit.fluxcd.io/v2": {"helm.toolkit.fluxcd.io", "v2"},
	} {
		g, v := splitGroupVersion(apiVersion)
		if g != want[0] || v != want[1] {
			t.Errorf("splitGroupVersion(%q) = %q, %q; want %q, %q", apiVersion, g, v, want[0], want[1])
		}
	}
}

func TestChartVersionOf(t *testing.T) {
	for chart, want := range map[string]string{
		"cert-manager-v1.17.0":         "v1.17.0",
		"kube-prometheus-stack-70.4.1": "70.4.1",
		"1.17.0":                       "1.17.0",
		"v1.17.0":                      "v1.17.0",
		"jetstack-cert-manager":        "",
		"":                             "",
	} {
		if got := chartVersionOf(chart); got != want {
			t.Errorf("chartVersionOf(%q) = %q, want %q", chart, got, want)
		}
	}
}

func TestIsValuesName(t *testing.T) {
	for base, want := range map[string]bool{
		"values.yaml":        true,
		"values-prod.yaml":   true,
		"prod-values.yaml":   true,
		"staging_values.yml": true,
		"evaluation.yaml":    false,
		"values.md":          false,
		"deployment.yaml":    false,
	} {
		if got := isValuesName(base); got != want {
			t.Errorf("isValuesName(%q) = %v, want %v", base, got, want)
		}
	}
}
