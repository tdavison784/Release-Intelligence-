package impact

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// renderedCRDsEnv is an environment whose only CRD knowledge comes from the
// FROM render of the customer's install (PO-7a addendum 6): certificates
// with v1 and v1alpha2 served, behind the customer's open gate.
const renderedCRDDocs = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: certificates.example.io
spec:
  group: example.io
  names:
    kind: Certificate
    plural: certificates
  versions:
    - name: v1
      served: true
      storage: true
    - name: v1alpha2
      served: true
      storage: false
`

func renderedCRDEnv(t *testing.T) *env.Environment {
	t.Helper()
	dir := t.TempDir()
	return loadEnv(t, env.Inputs{
		ValuesFiles: []string{writeFile(t, dir, "values.yaml", "replicas: 2\n")},
		RenderedCRDs: []env.RenderedCRDSource{{Label: "chart example@1.0.0", Tool: "helm",
			ChartDigest: "sha256:art", ValuesDigest: "sha256:vals", Docs: []byte(renderedCRDDocs)}},
	})
}

// PO-7a addendum 6/7: rendered CRDs resolve the CRD-change UNKNOWNs that a
// missing --crds input leaves — review-required, never action (the render is
// not observed state; the trust ladder caps it), never not-affected (a
// separately installed CRD path is invisible to the render).
func TestRenderedCRDsResolveVersionUnknowns(t *testing.T) {
	eb := newEdge()
	removed := eb.change(upgrade.RuleCRDVersionRemoved, "API version example.io/v1alpha2 of Certificate removed", "certificates.example.io/v1alpha2")

	// without rendered CRDs: today's visibility-gap unknown
	base := buildEval(t, eb.edge, plainEnv(t), Input{})
	if f := findingsOf(base, removed.ID); len(f) != 1 || f[0].Classification != domain.ImpactUnknown || f[0].Rule != RuleInsufficientVisibility {
		t.Fatalf("baseline must stay the visibility-gap unknown: %+v", f)
	}

	// with the render of their install declaring the version: review-required
	rep := buildEval(t, eb.edge, renderedCRDEnv(t), Input{})
	fs := findingsOf(rep, removed.ID)
	if len(fs) != 1 || fs[0].Classification != domain.ImpactReviewRequired || fs[0].Rule != RuleCRDVersionRemoved {
		t.Fatalf("rendered CRD resolves the unknown upward: %+v", fs)
	}
	f := fs[0]
	if f.Severity != domain.SeverityCritical || f.Provenance.Confidence != domain.ConfidenceMedium {
		t.Errorf("critical but medium confidence (rendered, not observed): %s %s", f.Severity, f.Provenance.Confidence)
	}
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range rep.EnvironmentEvidence {
		pool[e.ID] = e
	}
	if len(f.Matches) != 1 || f.Matches[0].Kind != domain.MatchCRDVersion || f.Matches[0].Subject != "certificates.example.io/v1alpha2" {
		t.Fatalf("the match pins the rendered CRD version: %+v", f.Matches)
	}
	ev, ok := pool[f.Matches[0].Evidence[0]]
	if !ok || ev.Kind != domain.EvidenceRendered || ev.Render == nil || ev.Render.Scope != domain.RenderEnvironment ||
		ev.Render.ChartDigest != "sha256:art" || ev.Render.ValuesDigest != "sha256:vals" {
		t.Fatalf("chain 2 must cite the rendered CRD's evidence: %+v (render %+v)", ev, ev.Render)
	}

	// a version the rendered CRD does not declare resolves nothing: the
	// render says nothing about a separately installed path
	other := eb.change(upgrade.RuleCRDVersionRemoved, "API version example.io/v1alpha3 of Certificate removed", "certificates.example.io/v1alpha3")
	rep2 := buildEval(t, eb.edge, renderedCRDEnv(t), Input{})
	if f := findingsOf(rep2, other.ID); len(f) != 1 || f[0].Classification != domain.ImpactUnknown {
		t.Fatalf("an undeclared version keeps the unknown: %+v", f)
	}
}

// The whole-CRD removal resolves the same way; a deprecation resolves at
// review with deprecation severity.
func TestRenderedCRDsResolveRemovalAndDeprecation(t *testing.T) {
	eb := newEdge()
	removed := eb.change(upgrade.RuleCRDRemoved, "CRD certificates.example.io removed", "certificates.example.io")
	deprecated := eb.change(upgrade.RuleCRDVersionDeprecated, "API version example.io/v1 of Certificate deprecated", "certificates.example.io/v1")
	rep := buildEval(t, eb.edge, renderedCRDEnv(t), Input{})
	if f := findingsOf(rep, removed.ID); len(f) != 1 || f[0].Classification != domain.ImpactReviewRequired || f[0].Rule != RuleCRDRemoved {
		t.Fatalf("rendered CRD resolves the removal unknown: %+v", f)
	} else if f[0].Matches[0].Kind != domain.MatchCRD || f[0].Matches[0].Subject != "certificates.example.io" {
		t.Errorf("the match pins the rendered CRD: %+v", f[0].Matches)
	}
	if f := findingsOf(rep, deprecated.ID); len(f) != 1 || f[0].Classification != domain.ImpactReviewRequired ||
		f[0].Rule != RuleCRDVersionDeprecated || f[0].Severity != domain.SeverityMedium {
		t.Fatalf("a deprecation resolves at review/deprecation severity: %+v", f)
	}
}

// plainEnv is an environment without any CRD knowledge.
func plainEnv(t *testing.T) *env.Environment {
	t.Helper()
	dir := t.TempDir()
	return loadEnv(t, env.Inputs{ValuesFiles: []string{writeFile(t, dir, "values.yaml", "replicas: 2\n")}})
}
