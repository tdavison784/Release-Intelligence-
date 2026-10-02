package semvalidate

import (
	"context"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func unsetExposure(path string) domain.Applicability {
	return domain.Applicability{
		Exposure: domain.Condition{Op: domain.OpValuesKey, Path: path, State: domain.StateUnset},
		Overlap:  &domain.Condition{Op: domain.OpValuesKey, Path: path, State: domain.StateSet},
	}
}

func canon(t *testing.T, in domain.SemanticAssertion, from, to *rel) map[domain.Aspect]domain.ValidationOutcome {
	t.Helper()
	return run(t, newCanonical(), input(from, to, in))
}

func TestCanonicalApplicability(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "rotationPolicy", `"Never"`, "gone", "1", "same", "1")
	to := newRel("v1.1.0").values("chart", "demo", "rotationPolicy", `"Always"`, "same", "1", "fresh", "1")

	// default-changed + the canonical unset exposure: confirmed
	a := assertion(helmSubject("rotationPolicy"), change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`))
	ap := unsetExposure("rotationPolicy")
	a.Applicability = &ap
	if got := canon(t, a, from, to); got[domain.AspectApplicability] != domain.OutcomeConfirmed {
		t.Errorf("canonical default-changed: %v", got)
	}

	// a different (narrower) condition is inconclusive, never refuted
	narrow := unsetExposure("rotationPolicy")
	narrow.Exposure = domain.Condition{Op: domain.OpValuesKey, Path: "rotationPolicy", State: domain.StateEquals, Values: []string{`"Never"`}}
	a.Applicability = &narrow
	if got := canon(t, a, from, to); got[domain.AspectApplicability] != domain.OutcomeInconclusive {
		t.Errorf("non-canonical condition: %v", got)
	}

	// a missing overlap is not the canonical condition either
	noOverlap := domain.Applicability{Exposure: ap.Exposure}
	a.Applicability = &noOverlap
	if got := canon(t, a, from, to); got[domain.AspectApplicability] != domain.OutcomeInconclusive {
		t.Errorf("missing overlap: %v", got)
	}

	// the canonical condition over a WRONG default: the change is refuted by the
	// values validator, so the applicability cannot be confirmed
	wrong := assertion(helmSubject("rotationPolicy"), change(domain.ChangeKindDefaultChanged, `"Always"`, `"Never"`))
	wrong.Applicability = &ap
	if got := canon(t, wrong, from, to); got[domain.AspectApplicability] != domain.OutcomeInconclusive {
		t.Errorf("unproven change: %v", got)
	}

	// a path that does not exist
	ghost := assertion(helmSubject("ghost"), change(domain.ChangeKindRemoved, "", ""))
	g := domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "ghost", State: domain.StateSet}}
	ghost.Applicability = &g
	if got := canon(t, ghost, from, to); got[domain.AspectApplicability] != domain.OutcomeInconclusive {
		t.Errorf("nonexistent subject: %v", got)
	}

	// removed: values-key set
	rem := assertion(helmSubject("gone"), change(domain.ChangeKindRemoved, "", ""))
	r := domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "gone", State: domain.StateSet}}
	rem.Applicability = &r
	if got := canon(t, rem, from, to); got[domain.AspectApplicability] != domain.OutcomeConfirmed {
		t.Errorf("canonical removed: %v", got)
	}
}

func TestCanonicalCRDFieldAndGVK(t *testing.T) {
	from := certCRDs(newRel("v1.16.0"), crdVer("v1", true, true, fld("spec.rotationPolicy", "string", `"Never"`)))
	to := certCRDs(newRel("v1.17.0"), crdVer("v1", true, true, fld("spec.rotationPolicy", "string", `"Always"`)))
	s := crdSubject("cert-manager.io", "Certificate", "spec.rotationPolicy")
	field := func(state domain.FieldState) domain.Condition {
		return domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate",
			Of: []domain.Condition{{Op: domain.OpField, Path: "spec.rotationPolicy", State: state}}}
	}
	setOverlap := field(domain.StateSet)
	a := assertion(s, change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`))
	a.Applicability = &domain.Applicability{Exposure: field(domain.StateUnset), Overlap: &setOverlap}
	if got := canon(t, a, from, to); got[domain.AspectApplicability] != domain.OutcomeConfirmed {
		t.Errorf("crd-field default-changed: %v", got)
	}
	// the subject's version may be spelled in the resource scope
	s2 := *s
	s2.Version = "v1"
	exp := field(domain.StateUnset)
	exp.Version = "v1"
	ov := field(domain.StateSet)
	ov.Version = "v1"
	b := assertion(&s2, change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`))
	b.Applicability = &domain.Applicability{Exposure: exp, Overlap: &ov}
	if got := canon(t, b, from, to); got[domain.AspectApplicability] != domain.OutcomeConfirmed {
		t.Errorf("crd-field with version: %v", got)
	}
	// the wrong state (set instead of unset for a default change)
	c := assertion(s, change(domain.ChangeKindDefaultChanged, `"Never"`, `"Always"`))
	c.Applicability = &domain.Applicability{Exposure: field(domain.StateSet)}
	if got := canon(t, c, from, to); got[domain.AspectApplicability] != domain.OutcomeInconclusive {
		t.Errorf("wrong state: %v", got)
	}

	// gvk removed
	gfrom := newRel("v0.37.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool", crdVer("v1beta1", true, true)))
	gto := newRel("v1.0.0").crds("crds", crd("nodepools.karpenter.sh", "karpenter.sh", "NodePool", crdVer("v1", true, true)))
	g := assertion(&domain.Subject{Family: domain.SubjectGVK, Product: "karpenter", Group: "karpenter.sh", Version: "v1beta1", Kind: "NodePool"}, change(domain.ChangeKindRemoved, "", ""))
	g.Applicability = &domain.Applicability{Exposure: domain.Condition{Op: domain.OpGVKInUse, Group: "karpenter.sh", Version: "v1beta1", Kind: "NodePool"}}
	if got := canon(t, g, gfrom, gto); got[domain.AspectApplicability] != domain.OutcomeConfirmed {
		t.Errorf("gvk removed: %v", got)
	}
}

func TestCanonicalRequirementAndImage(t *testing.T) {
	from := newRel("v1.0.0").compat("kubernetes", "supported", "1.29 → 1.33").images("m", "quay.io/acme/old:1")
	to := newRel("v1.1.0").compat("kubernetes", "supported", "1.30 → 1.34").images("m", "quay.io/acme/new:1")
	a := assertion(boundary("kubernetes"), change(domain.ChangeKindRequirementChanged, "", ">=1.30.0-0, <1.35.0-0"))
	a.Applicability = &domain.Applicability{Exposure: domain.Condition{Op: domain.OpClusterVersion, Name: "kubernetes", State: domain.StateOutOfRange, Range: ">=1.30.0-0, <1.35.0-0"}}
	if got := canon(t, a, from, to); got[domain.AspectApplicability] != domain.OutcomeConfirmed {
		t.Errorf("cluster-version: %v", got)
	}
	img := assertion(&domain.Subject{Family: domain.SubjectImage, Product: "demo", Name: "quay.io/acme/old"}, change(domain.ChangeKindRemoved, "", ""))
	img.Applicability = &domain.Applicability{Exposure: domain.Condition{Op: domain.OpImageInUse, Name: "quay.io/acme/old"}}
	if got := canon(t, img, from, to); got[domain.AspectApplicability] != domain.OutcomeConfirmed {
		t.Errorf("image-in-use: %v", got)
	}
}

func TestCanonicalDeprecatedWithReplacement(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "old", "1")
	to := newRel("v1.1.0").values("chart", "demo", "old", "1", "new", "1")
	c := change(domain.ChangeKindDeprecated, "", "")
	c.ReplacedBy = helmSubject("new")
	a := assertion(helmSubject("old"), c)
	overlap := domain.Condition{Op: domain.OpValuesKey, Path: "new", State: domain.StateSet}
	a.Applicability = &domain.Applicability{
		Exposure: domain.Condition{Op: domain.OpAll, Of: []domain.Condition{
			{Op: domain.OpValuesKey, Path: "old", State: domain.StateSet},
			{Op: domain.OpValuesKey, Path: "new", State: domain.StateUnset}}},
		Overlap: &overlap,
	}
	// a deprecation is not provable from values, so the canonical condition
	// stays unconfirmed: a human confirms the change
	if got := canon(t, a, from, to); got[domain.AspectApplicability] != domain.OutcomeInconclusive {
		t.Errorf("deprecation: %v", got)
	}
}

func TestCanonicalConfirmsOnlyTheNoneConsequenceOfAnAddedSubject(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "a", "1")
	to := newRel("v1.1.0").values("chart", "demo", "a", "1", "fresh", "1")
	none := &domain.Consequence{Kind: domain.ConsequenceNone, ExposedClass: domain.ImpactInformational}
	added := assertion(helmSubject("fresh"), change(domain.ChangeKindAdded, "", ""))
	added.Consequence = none
	if got := canon(t, added, from, to); got[domain.AspectConsequence] != domain.OutcomeConfirmed {
		t.Errorf("added/none: %v", got)
	}
	// an added subject that is not actually new
	exists := assertion(helmSubject("a"), change(domain.ChangeKindAdded, "", ""))
	exists.Consequence = none
	if got := canon(t, exists, from, to); got[domain.AspectConsequence] != domain.OutcomeInconclusive {
		t.Errorf("unproven added: %v", got)
	}
	// a real consequence is a judgement: never confirmed, and not even checked
	fail := assertion(helmSubject("fresh"), change(domain.ChangeKindAdded, "", ""))
	fail.Consequence = &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired}
	if got := canon(t, fail, from, to); len(got) != 0 {
		t.Errorf("a real consequence must produce no check: %v", got)
	}
	// kind none on a removal is not the canonical case
	rem := assertion(helmSubject("a"), change(domain.ChangeKindRemoved, "", ""))
	rem.Consequence = none
	if got := canon(t, rem, from, to); len(got) != 0 {
		t.Errorf("none on removed: %v", got)
	}
}

func TestValidatorsRegistry(t *testing.T) {
	vs := Validators()
	want := []string{ProducerValues, ProducerCRD, ProducerCompat, ProducerImage, ProducerRestatement, ProducerCanonical}
	if len(vs) != len(want) {
		t.Fatalf("%d validators", len(vs))
	}
	for i, v := range vs {
		if v.Name() != want[i] {
			t.Errorf("validator %d = %s, want %s", i, v.Name(), want[i])
		}
	}
	// not applicable: an assertion without subject or change yields no result
	for _, v := range vs {
		rs, err := v.Validate(context.Background(), input(newRel("v1.0.0"), newRel("v1.1.0"), domain.SemanticAssertion{Statement: "x"}))
		if err != nil || len(rs) != 0 {
			t.Errorf("%s on an empty assertion: %v %v", v.Name(), rs, err)
		}
	}
	// a family none of them handles
	a := assertion(&domain.Subject{Family: domain.SubjectFeatureGate, Product: "demo", Name: "X"}, change(domain.ChangeKindRemoved, "", ""))
	for _, v := range vs {
		if rs, _ := v.Validate(context.Background(), input(newRel("v1.0.0"), newRel("v1.1.0"), a)); len(rs) != 0 {
			t.Errorf("%s handled a feature gate", v.Name())
		}
	}
}

func TestResultsAreDeterministic(t *testing.T) {
	from := newRel("v1.0.0").values("chart", "demo", "a", "1")
	to := newRel("v1.1.0").values("chart", "demo", "a", "2")
	a := assertion(helmSubject("a"), change(domain.ChangeKindDefaultChanged, "1", "2"))
	r1, _ := valuesValidator{}.Validate(context.Background(), input(from, to, a))
	r2, _ := valuesValidator{}.Validate(context.Background(), input(from, to, a))
	if r1[0].ID != r2[0].ID || r1[0].ID == "" {
		t.Errorf("ids differ: %s %s", r1[0].ID, r2[0].ID)
	}
}
