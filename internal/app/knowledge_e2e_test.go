package app

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
)

// DESIGN.md §10 path 1 on real ingested data: the recorded cert-manager
// v1.17.0 → v1.18.0 edge states the rotationPolicy default change three times
// (release notes, the release notes' major themes, the upgrade guide). One
// human-verified fact — the contract's own example, anchored to the prose
// members of its cluster the way the semantic lane anchors them — turns all
// three not-joined unknowns into REVIEW REQUIRED for the fixture's
// Certificate, which leaves rotationPolicy unset (D11: behaviour change).
func TestE2EKnowledgeRotationPolicy(t *testing.T) {
	a := newReplayApp(t, fixedNow)
	ctx := context.Background()
	edge, err := a.Upgrade(ctx, "cert-manager", "v1.17.0", "v1.18.0", UpgradeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	lookup := func(id domain.EvidenceID) (domain.Evidence, bool) {
		for _, e := range edge.Evidence {
			if e.ID == id {
				return e, true
			}
		}
		return domain.Evidence{}, false
	}
	var anchors []domain.ChangeAnchor
	var restating []domain.Change
	release := ""
	for _, c := range edge.Changes {
		if strings.Contains(c.Title, "RotationPolicy") && c.Provenance.Method == domain.MethodDeclared && !domain.IsUmbrella(c, lookup) {
			anchors = append(anchors, domain.NewChangeAnchor(c, lookup))
			restating = append(restating, c)
			release = c.Release
		}
	}
	if len(restating) != 3 {
		t.Fatalf("the recorded edge should restate the change 3 times, found %d", len(restating))
	}
	for i := range anchors {
		anchors[i].Release = release
	}
	cert := func(st domain.FieldState) domain.Condition {
		return domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate",
			Of: []domain.Condition{{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: st}}}
	}
	overlap := cert(domain.StateSet)
	before, after := `"Never"`, `"Always"`
	f := domain.VerifiedFact{
		Product: "cert-manager", Release: release, Anchors: anchors, Candidates: []string{"sc-000000000001"},
		Assertion: domain.SemanticAssertion{
			Subject:       &domain.Subject{Family: domain.SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"},
			Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: &before, After: &after},
			Applicability: &domain.Applicability{Exposure: cert(domain.StateUnset), Overlap: &overlap},
			Consequence: &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired,
				Statement: "Private keys of Certificates without an explicit rotationPolicy are regenerated on every renewal."},
			Statement: "The default Certificate.spec.privateKey.rotationPolicy changed from Never to Always.",
		},
		Evidence: []domain.Evidence{mustEvidence(t, edge, restating[0])},
		Status:   domain.FactActive, CreatedAt: fixedNow,
		Verification: []domain.AspectVerification{
			{Aspect: domain.AspectSubject, Level: domain.VerifiedDeterministic, Basis: []string{"val-crd"}},
			{Aspect: domain.AspectChange, Level: domain.VerifiedHuman, Basis: []string{"rd-change"}},
			{Aspect: domain.AspectApplicability, Level: domain.VerifiedDeterministic, Basis: []string{"val-canonical"}},
			{Aspect: domain.AspectConsequence, Level: domain.VerifiedHuman, Basis: []string{"rd-consequence"}},
		},
	}
	f.ID = domain.VerifiedFactID(f.Product, f.Release, f.Assertion)
	if err := f.Validate(); err != nil {
		t.Fatalf("fact: %v", err)
	}

	base := mustImpact(t, a)
	rep, err := a.Impact(ctx, "cert-manager", "v1.17.0", "v1.18.0", ImpactOptions{Environment: impactEnvironment(), Facts: []domain.VerifiedFact{f}})
	if err != nil {
		t.Fatal(err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("report does not validate: %v", err)
	}
	for _, c := range restating {
		var got []domain.ImpactFinding
		for _, x := range rep.Findings {
			if x.ChangeID == c.ID {
				got = append(got, x)
			}
		}
		if len(got) != 1 || got[0].Rule != impact.RuleKnowledgeExposed || got[0].Classification != domain.ImpactReviewRequired {
			t.Errorf("%q: %+v", c.Title, got)
			continue
		}
		if !strings.Contains(got[0].Matches[0].Subject+got[0].Matches[len(got[0].Matches)-1].Subject, "example-com") {
			t.Errorf("%q: matches %+v, want the example-com Certificate", c.Title, got[0].Matches)
		}
	}
	if base.Summary.Unknown-rep.Summary.Unknown != 3 || rep.Summary.ReviewRequired-base.Summary.ReviewRequired != 3 {
		t.Errorf("funnel: unknown %d → %d, review %d → %d; want 3 unknowns to become reviews",
			base.Summary.Unknown, rep.Summary.Unknown, base.Summary.ReviewRequired, rep.Summary.ReviewRequired)
	}
}

// mustEvidence returns the first upstream record a change cites (the fact's
// own release-level evidence).
func mustEvidence(t *testing.T, edge *domain.UpgradeEdge, c domain.Change) domain.Evidence {
	t.Helper()
	for _, e := range edge.Evidence {
		if len(c.Evidence) > 0 && e.ID == c.Evidence[0] {
			return e
		}
	}
	t.Fatalf("change %s cites no resolvable evidence", c.ID)
	return domain.Evidence{}
}
