package render

import (
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
	argo := byKind[TargetArgoCD]
	if len(argo) != 1 || argo[0].ReleaseName != "cert-manager" || argo[0].Namespace != "cert-manager" || len(argo[0].Values) != 2 || !argo[0].ValuesComplete {
		t.Errorf("argo target: %+v", argo)
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
