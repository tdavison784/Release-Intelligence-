package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFiles(t *testing.T, files map[string]string) *Environment {
	t.Helper()
	root := t.TempDir()
	for p, body := range files {
		full := filepath.Join(root, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e, err := Load(Inputs{Repo: root})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func unparsable(e *Environment) []string {
	var out []string
	for _, w := range e.Warnings {
		if strings.Contains(w, "not a parsable version") {
			out = append(out, w)
		}
	}
	return out
}

const argoMultiSource = `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
  namespace: argocd
spec:
  sources:
    - repoURL: https://charts.jetstack.io
      chart: cert-manager
      targetRevision: v1.17.2
      helm:
        valueFiles:
          - $values/platform/cert-manager/values.yaml
    - repoURL: https://git.example.com/platform/gitops.git
      targetRevision: main
      ref: values
`

// A multi-source Argo CD Application: the chart source carries the deployed
// version; the values-only source (ref: values) tracking main is not it.
func TestArgoMultiSourceVersionComesFromTheChart(t *testing.T) {
	e := loadFiles(t, map[string]string{"apps/cert-manager.yaml": argoMultiSource, "platform/cert-manager/values.yaml": "replicaCount: 2\n"})
	var argo []InstalledProduct
	for _, p := range e.Installed {
		if p.Source == InstalledArgoCD {
			argo = append(argo, p)
		}
	}
	if len(argo) != 1 || argo[0].Product != "cert-manager" || argo[0].Version != "v1.17.2" || argo[0].Chart != "cert-manager" {
		t.Fatalf("argo installed = %+v, want one cert-manager v1.17.2", argo)
	}
	if w := unparsable(e); len(w) != 0 {
		t.Fatalf("the values repo's branch raised a version warning: %v", w)
	}
}

// A single-source Application tracking a branch for the product chart leaves
// the deployed version unknown: the warning (the gap) stays.
func TestArgoSingleSourceBranchStaysAGap(t *testing.T) {
	e := loadFiles(t, map[string]string{"apps/cert-manager.yaml": `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
  namespace: argocd
spec:
  source:
    repoURL: https://charts.jetstack.io
    chart: cert-manager
    targetRevision: main
`})
	if w := unparsable(e); len(w) != 1 || !strings.Contains(w[0], `"cert-manager"`) {
		t.Fatalf("warnings = %v, want one for cert-manager", w)
	}
}

// A values-only source (ref, no chart, no path) renders nothing: Argo CD
// applies its helm block to no chart, so those values reach no release.
func TestArgoValuesOnlySourceInlineValuesReachNoRelease(t *testing.T) {
	e := loadFiles(t, map[string]string{"apps/a.yaml": `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: cert-manager
spec:
  sources:
    - repoURL: https://charts.jetstack.io
      chart: cert-manager
      targetRevision: v1.17.2
    - repoURL: https://git.example.com/x.git
      targetRevision: main
      ref: values
      helm:
        valuesObject:
          replicaCount: 3
`})
	if len(e.ValuesKeys) != 0 {
		t.Fatalf("inline values of a values-only source were applied: %+v", e.ValuesKeys)
	}
	if len(e.ValuesReleases) != 1 || e.ValuesReleases[0].Chart != "cert-manager" || len(e.ValuesReleases[0].Layers) != 0 {
		t.Fatalf("releases = %+v, want the chart source with an empty stack", e.ValuesReleases)
	}
	if !e.Supplied.Values || e.Health(DimValues) != HealthOK {
		t.Errorf("a release with an empty, fully read stack sets no keys: supplied=%v health=%s", e.Supplied.Values, e.Health(DimValues))
	}
}

// Flux: the HelmRelease's chart version is the deployed version; a
// GitRepository (values or chart source) on a branch is not.
func TestFluxVersionIsTheChartVersion(t *testing.T) {
	e := loadFiles(t, map[string]string{"flux/hr.yaml": `apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: cert-manager
  namespace: cert-manager
spec:
  chart:
    spec:
      chart: cert-manager
      version: 1.17.2
      sourceRef:
        kind: HelmRepository
        name: jetstack
  values:
    replicaCount: 2
`, "flux/git.yaml": `apiVersion: source.toolkit.fluxcd.io/v1
kind: GitRepository
metadata:
  name: gitops
  namespace: flux-system
spec:
  url: https://git.example.com/gitops.git
  ref:
    branch: main
`})
	var flux []InstalledProduct
	for _, p := range e.Installed {
		if p.Source == InstalledFlux {
			flux = append(flux, p)
		}
	}
	if len(flux) != 1 || flux[0].Version != "1.17.2" {
		t.Fatalf("flux installed = %+v", flux)
	}
	if w := unparsable(e); len(w) != 0 {
		t.Fatalf("the GitRepository branch raised a version warning: %v", w)
	}
}
