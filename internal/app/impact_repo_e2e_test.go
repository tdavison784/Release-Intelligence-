package app

// End-to-end test of `ri impact --repo` (directory mode): the recorded
// cert-manager v1.17.0 → v1.18.0 edge of e2e_test.go joined with the
// miniature customer repository in internal/env/testdata/customer-repo,
// discovered purely by convention. Replayed offline with the fixed clock;
// goldens re-derived with -update like every other e2e golden.

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

func mustImpactRepo(t *testing.T, a *App) *domain.ImpactReport {
	t.Helper()
	in := ImpactOptions{Environment: env.Inputs{
		KubernetesVersion: "1.28",
		Repo:              filepathRepoFixture,
	}}
	rep, err := a.Impact(context.Background(), "cert-manager", "v1.17.0", "v1.18.0", in)
	if err != nil {
		t.Fatalf("impact --repo: %v", err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
	return rep
}

// filepathRepoFixture is relative to this package (internal/app), keeping
// evidence URIs in the goldens stable regardless of where the test runs from.
const filepathRepoFixture = "../env/testdata/customer-repo"

func TestE2EImpactRepo(t *testing.T) {
	rep := mustImpactRepo(t, newReplayApp(t, fixedNow))

	// the same recorded edge; the repo discovery must not change it
	if rep.Summary.UpstreamChanges != 51 {
		t.Errorf("upstream changes = %d, want 51", rep.Summary.UpstreamChanges)
	}
	if rep.Summary.AffectEnvironment == 0 {
		t.Errorf("findings = %d", rep.Summary.AffectEnvironment)
	}

	byRule := map[string][]domain.ImpactFinding{}
	for _, f := range rep.Findings {
		byRule[f.Rule] = append(byRule[f.Rule], f)
	}

	// cluster 1.28 below the recorded support range 1.29–1.33
	if fs := byRule[impact.RuleKubernetesBelow]; len(fs) != 1 || fs[0].Classification != domain.ImpactActionRequired {
		t.Fatalf("kubernetes-below = %+v", byRule[impact.RuleKubernetesBelow])
	}
	// the prod values file pins prometheus.servicemonitor.targetPort
	fs := byRule[impact.RuleValuesPinned]
	if len(fs) != 1 || !slicesContains(matchSubjectsOf(fs[0]), "prometheus.servicemonitor.targetPort") {
		t.Fatalf("values-pinned = %+v", fs)
	}
	// global.rbac.disableHTTPChallengesRole is new in v1.18.0 and set in the
	// discovered values file
	found := false
	for _, f := range byRule[impact.RuleValuesNewKey] {
		if slicesContains(matchSubjectsOf(f), "global.rbac.disableHTTPChallengesRole") {
			found = true
		}
	}
	if !found {
		t.Errorf("values-new-key on global.rbac.disableHTTPChallengesRole missing: %+v", byRule[impact.RuleValuesNewKey])
	}
	// the deployment/workflow/kustomization pins of the old controller image
	pinned := 0
	for _, f := range byRule[impact.RuleImageChanged] {
		if strings.Contains(f.Title, "quay.io/jetstack/cert-manager-controller:v1.17.0") {
			pinned++
		}
	}
	if pinned == 0 {
		t.Errorf("no image finding for the pinned controller reference: %+v", byRule[impact.RuleImageChanged])
	}

	// environment evidence points into the discovered repo. Both chains are
	// mandatory for affected findings (the ACTION/review/informational set);
	// not-affected/unknown carry their evaluation records instead
	// (docs/ACTION_CLASSIFICATION.md).
	for _, f := range rep.Findings {
		if f.Classification.Affected() {
			if len(f.UpstreamEvidence) == 0 || len(f.EnvironmentEvidence) == 0 {
				t.Errorf("affected finding %s (%s) lacks a chain", f.ID, f.Classification)
			}
			continue
		}
		if len(f.Checks) == 0 && len(f.NeededToDetermine) == 0 {
			t.Errorf("non-affected finding %s (%s) has neither checks nor neededToDetermine", f.ID, f.Classification)
		}
	}
	var citesRepo bool
	for _, e := range rep.EnvironmentEvidence {
		if strings.Contains(e.URI, "customer-repo") {
			citesRepo = true
		}
	}
	if !citesRepo {
		t.Error("no environment evidence cites the discovered repository")
	}

	// repo-mode warnings travel into the report
	joined := strings.Join(rep.Warnings, "\n")
	if !strings.Contains(joined, "is not built with kustomize") || !strings.Contains(joined, "values files") {
		t.Errorf("repo warnings missing from the report: %v", rep.Warnings)
	}

	var text strings.Builder
	if err := impact.RenderText(&text, rep, impact.RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	checkImpactGolden(t, "cert-manager_repo_v1.17.0_v1.18.0.txt", []byte(text.String()))
	checkImpactGolden(t, "cert-manager_repo_v1.17.0_v1.18.0.json", []byte(impactJSON(t, rep)))
}

// TestE2EImpactRepoDeterminism: discovery and the join are pure functions of
// the fixture files and the clock.
func TestE2EImpactRepoDeterminism(t *testing.T) {
	a := newReplayApp(t, fixedNow)
	first := impactJSON(t, mustImpactRepo(t, a))
	if got := impactJSON(t, mustImpactRepo(t, a)); got != first {
		t.Fatalf("second run differs:\n%s", firstDiff([]byte(got), []byte(first)))
	}
}

func slicesContains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
