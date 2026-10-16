package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/env"
)

// TestDetectTargetsCustomerRepo: the repo-mode fixture declares cert-manager
// four ways; each becomes its own render target with the customer's values,
// names and completeness, and the overlay is picked with a reason.
func TestDetectTargetsCustomerRepo(t *testing.T) {
	repo := "../env/testdata/customer-repo"
	e, err := env.Load(env.Inputs{Repo: repo})
	if err != nil {
		t.Fatal(err)
	}
	ts, notes := DetectTargets(TargetOptions{Product: "cert-manager", Charts: []string{"cert-manager"}, Repo: repo, Files: RepoFilesFrom(e)})
	byKind := map[TargetKind][]Target{}
	for _, tg := range ts {
		byKind[tg.Kind] = append(byKind[tg.Kind], tg)
		t.Logf("%s %s release=%s ns=%s complete=%v (%s) layers=%d set=%d — %s", tg.Kind, tg.Origin, tg.ReleaseName, tg.Namespace, tg.ValuesComplete, tg.IncompleteReason, len(tg.Values), len(tg.Set), tg.Why)
	}
	for _, n := range notes {
		t.Logf("note: %s", n)
	}
	// the Application sets helm.values AND helm.valuesObject: Argo CD applies
	// valuesObject only ("valuesObject trumps values"), so one layer, and a
	// note says values was ignored
	argo := byKind[TargetArgoCD]
	if len(argo) != 1 || argo[0].ReleaseName != "cert-manager" || argo[0].Namespace != "cert-manager" || len(argo[0].Values) != 1 ||
		!strings.HasSuffix(argo[0].Values[0].Origin, ".helm.valuesObject") || !argo[0].ValuesComplete {
		t.Errorf("argo target: %+v", argo)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "helm.values is ignored") {
		t.Errorf("no note that helm.values is ignored: %v", notes)
	}
	flux := byKind[TargetFlux]
	if len(flux) != 1 || flux[0].ValuesComplete || !strings.Contains(flux[0].IncompleteReason, "valuesFrom") || flux[0].Namespace != "flux-system" {
		t.Errorf("flux target must be incomplete (valuesFrom): %+v", flux)
	}
	hf := byKind[TargetHelmfile]
	if len(hf) != 1 || len(hf[0].Values) != 1 || !strings.HasSuffix(hf[0].Values[0].Origin, "clusters/prod/values-cert-manager.yaml") || !hf[0].NamesAssumed {
		t.Errorf("helmfile target: %+v", hf)
	}
	vals := byKind[TargetValuesFiles]
	if len(vals) != 1 || !strings.HasSuffix(vals[0].Origin, "clusters/dev/values-cert-manager.yaml") {
		t.Errorf("standalone values target (dev only; prod belongs to the helmfile release): %+v", vals)
	}
	k := byKind[TargetKustomize]
	if len(k) != 1 || !strings.HasSuffix(k[0].Kustomize.Dir, "kustomize") || !strings.Contains(k[0].Why, "top-level") {
		t.Errorf("kustomize target: %+v", k)
	}
}

// TestDetectTargetsLayeredValuesFile: a repo-local values file an Argo CD
// Application already layers is not rendered again on its own (that
// configuration is deployed nowhere); a values file nothing layers still is.
func TestDetectTargetsLayeredValuesFile(t *testing.T) {
	repo := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(repo, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	app := write("clusters/prod/acme.yaml", `apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: acme
spec:
  destination:
    namespace: acme
  sources:
    - repoURL: https://charts.example.io
      chart: acme
      targetRevision: 1.0.0
      helm:
        valueFiles:
          - $values/platform/acme/values.yaml
          - $values/platform/acme/values-prod.yaml
    - repoURL: https://git.example.com/gitops.git
      ref: values
`)
	v1 := write("platform/acme/values.yaml", "replicaCount: 2\n")
	v2 := write("platform/acme/values-prod.yaml", "replicaCount: 3\n")
	v3 := write("platform/acme/values-dev.yaml", "replicaCount: 1\n")
	ts, notes := DetectTargets(TargetOptions{Product: "acme", Charts: []string{"acme"}, Repo: repo,
		Files: RepoFiles{ArgoCD: []string{app}, Values: []string{v1, v2, v3}}})
	var ids []string
	for _, tg := range ts {
		ids = append(ids, tg.ID)
	}
	if len(ts) != 2 || ts[0].Kind != TargetArgoCD || len(ts[0].Values) != 2 || ts[1].ID != "values:"+v3 {
		t.Fatalf("targets = %v", ids)
	}
	joined := strings.Join(notes, "\n")
	for _, f := range []string{v1, v2} {
		if !strings.Contains(joined, "values file "+f+": rendered as a values layer of "+ts[0].ID) {
			t.Errorf("no note for %s: %s", f, joined)
		}
	}
}
