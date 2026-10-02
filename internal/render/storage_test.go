package render

import (
	"context"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// crdPair renders a CRD serving v1beta1 and v1 on both sides whose storage
// flag moves v1beta1 → v1 (the karpenter NodePool shape of
// VALIDATOR-AUDIT.md).
func crdPair(t *testing.T) *Pair {
	t.Helper()
	crd := func(storageV1 bool) []Object {
		st := "false"
		if storageV1 {
			st = "true"
		}
		other := map[string]string{"true": "false", "false": "true"}[st]
		o, err := ParseObjects([]byte(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata: {name: nodepools.karpenter.sh}
spec:
  group: karpenter.sh
  names: {kind: NodePool, plural: nodepools}
  scope: Cluster
  versions:
    - {name: v1, served: true, storage: ` + st + `}
    - {name: v1beta1, served: true, storage: ` + other + `}
`))
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	from, to := crd(false), crd(true)
	d := Diff(from, to, DiffOptions{})
	res := func(version string, objs []Object) *Result {
		return &Result{Status: StatusSucceeded, Objects: objs, Provenance: Provenance{Scope: domain.RenderRelease, Tool: ToolHelm,
			ToolVersion: "v3.16.1", Chart: "karpenter", ChartVersion: version, ArtifactDigest: "sha256:" + version, OutputDigest: "sha256:o" + version, RenderedAt: fixedNow}}
	}
	return &Pair{Product: "demo", From: "1.0.0", To: "1.1.0", Scope: domain.RenderRelease, Target: DefaultsTarget("demo"), Status: PairOK,
		FromResult: res("0.37.8", from), ToResult: res("1.0.0", to), Diff: &d}
}

// VALIDATOR-AUDIT.md: a storage-version move is invisible in rendered
// objects (the render shows served versions; which one stores is a CRD flag
// the API server applies), so rendered-diff must neither refute nor confirm
// it: render-not-applicable, inconclusive, and the render is not consulted.
func TestRenderedDiffStorageVersionIsNotApplicable(t *testing.T) {
	src := &fixedPairs{p: crdPair(t)}
	claim := assertOn(&domain.Subject{Family: domain.SubjectGVK, Product: "demo", Group: "karpenter.sh", Version: "v1", Kind: "NodePool"},
		&domain.ChangeSpec{Type: domain.ChangeKindValueChanged, Before: sptr(`"v1beta1"`), After: sptr(`"v1"`)})
	vs, err := NewValidator(src).Validate(context.Background(), vInput(claim))
	if err != nil || len(vs) != 1 {
		t.Fatalf("validate: %v %d", err, len(vs))
	}
	v := vs[0]
	if v.RenderRelation != domain.RenderNotApplicable {
		t.Errorf("relation = %s, want render-not-applicable", v.RenderRelation)
	}
	for _, c := range v.Checks {
		if c.Outcome != domain.OutcomeInconclusive {
			t.Errorf("%s: %s (%s); a storage move must stay inconclusive", c.Aspect, c.Outcome, c.Detail)
		}
	}
	if src.calls != 0 {
		t.Errorf("the render was consulted %d times for a non-renderable claim", src.calls)
	}
	if r := domain.RenderabilityOf(domain.SubjectGVK, domain.ChangeKindValueChanged); r != domain.RenderNotVerifiable {
		t.Errorf("gvk value-changed renderability = %s", r)
	}
	// served-version changes stay render-verifiable: a version removed from the
	// CRD (or an object moving apiVersion) is visible in the render
	if r := domain.RenderabilityOf(domain.SubjectGVK, domain.ChangeKindRemoved); r != domain.RenderVerifiable {
		t.Errorf("gvk removed renderability = %s", r)
	}
}
