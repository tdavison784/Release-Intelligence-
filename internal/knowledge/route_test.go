package knowledge

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func hasSig(it domain.ReviewItem, s domain.RoutingSignal) bool { return hasSignal(it, s) }

func TestRouteTable(t *testing.T) {
	c := fixtureCandidate()
	behavior := rotationAssertion(domain.ConsequenceBehaviorChange)
	failing := rotationAssertion(domain.ConsequenceSettingIgnored)
	val := validation(c, behavior, domain.AspectSubject, domain.AspectChange, domain.AspectApplicability)

	t.Run("models agree + validation confirmed rest = low", func(t *testing.T) {
		r := Route(c, []domain.SemanticProposal{
			proposal(c, "zai", "glm", behavior), proposal(c, "anthropic", "sonnet", behavior)}, []domain.ValidationResult{val})
		if len(r.ReviewItems) != 1 {
			t.Fatalf("items = %d", len(r.ReviewItems))
		}
		it := r.ReviewItems[0]
		if it.QuestionType != domain.QuestionConsequence || it.Routing.Priority != domain.PriorityLow || !hasSig(it, domain.SignalModelsAgree) {
			t.Fatalf("item = %+v", it.Routing)
		}
	})
	t.Run("models agree, no validation = normal", func(t *testing.T) {
		r := Route(c, []domain.SemanticProposal{
			proposal(c, "zai", "glm", behavior), proposal(c, "anthropic", "sonnet", behavior)}, nil)
		for _, it := range r.ReviewItems {
			if it.Routing.Priority != domain.PriorityNormal || it.Routing.Route != domain.RouteReview {
				t.Fatalf("%s routing = %+v", it.QuestionType, it.Routing)
			}
		}
		if len(r.ReviewItems) != 3 { // mapping, applicability, consequence
			t.Fatalf("items = %d, want 3", len(r.ReviewItems))
		}
	})
	t.Run("disagreement = normal, action-eligible = high", func(t *testing.T) {
		both := Route(c, []domain.SemanticProposal{
			proposal(c, "zai", "glm", behavior), proposal(c, "anthropic", "sonnet", failing)}, []domain.ValidationResult{val})
		it := both.ReviewItems[0]
		if it.Routing.Priority != domain.PriorityHigh || !hasSig(it, domain.SignalModelsDisagree) || !hasSig(it, domain.SignalHighImpact) {
			t.Fatalf("routing = %+v", it.Routing)
		}
		two := Route(c, []domain.SemanticProposal{
			proposal(c, "zai", "glm", behavior), proposal(c, "anthropic", "sonnet", rotationAssertionWithBehaviorSeverity())}, []domain.ValidationResult{val})
		if p := two.ReviewItems[0].Routing.Priority; p != domain.PriorityNormal {
			t.Fatalf("plain disagreement priority = %s, want normal", p)
		}
	})
	t.Run("single model never counts as agreement", func(t *testing.T) {
		r := Route(c, []domain.SemanticProposal{proposal(c, "zai", "glm", behavior)}, []domain.ValidationResult{val})
		it := r.ReviewItems[0]
		if hasSig(it, domain.SignalModelsAgree) || !hasSig(it, domain.SignalSingleModel) {
			t.Fatalf("signals = %v", it.Routing.Signals)
		}
	})
	t.Run("one call answering twice is not two calls", func(t *testing.T) {
		p1 := proposal(c, "zai", "glm", behavior)
		p2 := proposal(c, "zai", "glm", behavior)
		p2.Provenance.PromptDigest = "sha256:other"
		p2.ID = domain.ProposalID(p2.CandidateID, p2.Task, p2.Provider, p2.Provenance)
		r := Route(c, []domain.SemanticProposal{p1, p2}, nil)
		if hasSig(r.ReviewItems[0], domain.SignalModelsAgree) {
			t.Fatal("one model agreeing with itself counted as agreement")
		}
	})
	t.Run("two separate calls of one model agree (PO-1)", func(t *testing.T) {
		p := proposal(c, "zai", "glm", behavior)
		r := Route(c, []domain.SemanticProposal{call(p, "call-a", ""), call(p, "call-b", "")}, nil)
		if !hasSig(r.ReviewItems[0], domain.SignalModelsAgree) || hasSig(r.ReviewItems[0], domain.SignalSingleModel) {
			t.Fatalf("signals = %v", r.ReviewItems[0].Routing.Signals)
		}
	})
	t.Run("everyone abstains = missing-evidence", func(t *testing.T) {
		r := Route(c, []domain.SemanticProposal{
			proposal(c, "zai", "glm", domain.SemanticAssertion{}, domain.Aspects...)}, nil)
		if len(r.ReviewItems) != 1 {
			t.Fatalf("items = %d", len(r.ReviewItems))
		}
		it := r.ReviewItems[0]
		if it.Routing.Route != domain.RouteMissingEvidence || it.Routing.Priority != domain.PriorityLow || it.QuestionType != domain.QuestionEvidenceSufficiency {
			t.Fatalf("item = %+v", it)
		}
	})
	t.Run("deterministic when every aspect is confirmed", func(t *testing.T) {
		none := rotationAssertion(domain.ConsequenceNone)
		none.Consequence.Severity = ""
		v := validation(c, none, domain.Aspects...)
		r := Route(c, nil, []domain.ValidationResult{v})
		if r.Fact == nil || len(r.ReviewItems) != 0 {
			t.Fatalf("route = %+v", r)
		}
		if r.Fact.Level() != domain.VerifiedDeterministic {
			t.Fatalf("level = %s", r.Fact.Level())
		}
	})
	t.Run("routing is pure", func(t *testing.T) {
		ps := []domain.SemanticProposal{proposal(c, "zai", "glm", behavior), proposal(c, "anthropic", "sonnet", failing)}
		a := Route(c, ps, []domain.ValidationResult{val})
		b := Route(c, ps, []domain.ValidationResult{val})
		if a.ReviewItems[0].ID != b.ReviewItems[0].ID {
			t.Fatal("not deterministic")
		}
	})
	t.Run("policy seam can auto-verify", func(t *testing.T) {
		policy := func(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult, ag []AspectAgreement) *AutoApproval {
			return &AutoApproval{Aspects: map[domain.Aspect]AutoApprovedAspect{
				domain.AspectConsequence: {Part: partOf(behavior, domain.AspectConsequence), Level: domain.VerifiedDeterministic, Basis: []string{val.ID}},
			}}
		}
		// the basis would not re-prove (val does not confirm consequence), but the
		// seam composes; the store would refuse it. Routing itself only composes.
		r := RouteWith(policy, c, nil, []domain.ValidationResult{val})
		if r.Fact == nil {
			t.Fatal("policy covering the open aspect did not yield a fact")
		}
	})
}

func rotationAssertionWithBehaviorSeverity() domain.SemanticAssertion {
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	a.Consequence.Severity = domain.SeverityLow
	return a
}

// Mixing a subject from one proposal with a change from another can build an
// invalid assertion (requirement-changed needs a compatibility-boundary or
// product-relationship subject). Items take the open aspects coherently from one proposal.
func TestItemsComposeCoherentAssertions(t *testing.T) {
	c := fixtureCandidate()
	a1 := domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectProtocolBehavior, Product: "cert-manager", Name: "grpc-alpn"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindBehaviorChanged},
	}
	a2 := domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCompatibilityBoundary, Product: "cert-manager", Name: "kubernetes"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindRequirementChanged, After: str(">=1.29")},
	}
	for i := 0; i < 8; i++ { // vary tie-breaking by model/call names
		p1 := call(proposal(c, "zai", "glm", a1, domain.AspectApplicability, domain.AspectConsequence), "c1", "")
		p2 := call(proposal(c, "anthropic", "sonnet", a2, domain.AspectApplicability, domain.AspectConsequence), "c2", "")
		p1.Task, p2.Task = domain.TaskFull, domain.TaskFull
		r := Route(c, []domain.SemanticProposal{p1, p2}, nil)
		for _, it := range r.ReviewItems {
			if err := it.Validate(); err != nil {
				t.Fatalf("incoherent item: %v", err)
			}
			if it.QuestionType == domain.QuestionSemanticMapping {
				ok1 := it.Proposed.Subject.Key() == a1.Subject.Key() && it.Proposed.Change.Type == a1.Change.Type
				ok2 := it.Proposed.Subject.Key() == a2.Subject.Key() && it.Proposed.Change.Type == a2.Change.Type
				if !ok1 && !ok2 {
					t.Fatalf("mapping mixes proposals: %+v", it.Proposed)
				}
			}
		}
	}
}
