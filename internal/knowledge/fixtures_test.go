package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// fixtures: the rotationPolicy demonstration (DESIGN.md §10), built from
// release-level facts only (no eval expectations).

var t0 = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func str(s string) *string { return &s }

func upEvidence() domain.Evidence {
	return domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example/upgrading-1.17-1.18.md", "L10-L12",
		"The default rotationPolicy is now Always", "sha256:doc", t0)
}

func crdEvidence() domain.Evidence {
	return domain.NewEvidence(domain.EvidenceStructured, "crds", "https://example/crds.yaml", "$.spec.versions[0]",
		"rotationPolicy: default Always", "sha256:crd", t0)
}

func certificates(of ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "cert-manager.io", Kind: "Certificate", Of: of}
}

func rotationAssertion(kind domain.ConsequenceKind) domain.SemanticAssertion {
	cons := &domain.Consequence{Kind: kind, ExposedClass: kind.ExposedClass(), Severity: domain.SeverityHigh}
	if kind.ActionEligible() {
		cons.Statement = "renewals fail"
	}
	return domain.SemanticAssertion{
		Subject: &domain.Subject{Family: domain.SubjectCRDField, Product: "cert-manager", Group: "cert-manager.io", Kind: "Certificate", Path: "spec.privateKey.rotationPolicy"},
		Change:  &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Never"`), After: str(`"Always"`)},
		Applicability: &domain.Applicability{
			Exposure: certificates(domain.Condition{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateUnset}),
		},
		Consequence: cons,
		Statement:   "Certificate.spec.privateKey.rotationPolicy default Never → Always",
	}
}

func fixtureCandidate() domain.SemanticCandidate {
	e := upEvidence()
	a := domain.ChangeAnchor{Release: "v1.18.0", EvidenceKeys: []string{domain.EvidenceKey(e)},
		StatementKeys: []string{domain.StatementKey(e)}, ChangeIDs: []string{"chg-rot"}}
	members := []domain.CandidateMember{{ChangeID: "chg-rot", Anchor: &a}}
	return domain.SemanticCandidate{
		ID: domain.CandidateID("cert-manager", a.Release, members), Product: "cert-manager", Release: a.Release,
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration,
		Title: "The default rotationPolicy is now Always", Evidence: []domain.Evidence{upEvidence(), crdEvidence()},
		Producer: "semantic.candidates@v1", CreatedAt: t0,
	}
}

func aiProv(model string) domain.Provenance {
	at := t0.Add(time.Minute)
	return domain.Provenance{Method: domain.MethodAI, Producer: "semantic.propose@v1", Confidence: domain.ConfidenceMedium,
		Model: model, ModelVersion: model + "-1", PromptVersion: "semantic/v1", PromptDigest: "sha256:" + model,
		InputEvidence: []domain.EvidenceID{upEvidence().ID, crdEvidence().ID}, GeneratedAt: &at}
}

// proposal builds a full-task proposal by model asserting a (nil aspects in
// undetermined are abstentions).
func proposal(c domain.SemanticCandidate, provider, model string, a domain.SemanticAssertion, undetermined ...domain.Aspect) domain.SemanticProposal {
	p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: provider, Assertion: a,
		Undetermined: undetermined, Provenance: aiProv(model)}
	if len(undetermined) > 0 {
		p.UndeterminedReason = "evidence does not say"
	}
	if !a.Empty() {
		p.Citations = []domain.EvidenceID{upEvidence().ID}
	}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

// validation confirms the given aspects of a.
func validation(c domain.SemanticCandidate, a domain.SemanticAssertion, aspects ...domain.Aspect) domain.ValidationResult {
	v := domain.ValidationResult{CandidateID: c.ID, Validator: "semvalidate.crd@v1", Assertion: a,
		Evidence: []domain.Evidence{crdEvidence()}, CheckedAt: t0.Add(2 * time.Minute)}
	for _, x := range aspects {
		v.Checks = append(v.Checks, domain.AspectCheck{Aspect: x, Outcome: domain.OutcomeConfirmed, Rule: "test:" + string(x)})
	}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	return v
}

func decision(item domain.ReviewItem, reviewer string, kind domain.ReviewerKind, action domain.DecisionAction, at time.Time) domain.ReviewDecision {
	orig := item.Proposed
	d := domain.ReviewDecision{ReviewItemID: item.ID, Action: action, Original: &orig, Reviewer: reviewer, ReviewerKind: kind,
		StartedAt: at.Add(-90 * time.Second), DecidedAt: at}
	switch action {
	case domain.ActionAccept:
		d.Labels = []domain.FeedbackLabel{domain.LabelAccepted}
	case domain.ActionReject:
		d.Labels = []domain.FeedbackLabel{domain.LabelRejected}
		d.Reason = "wrong"
	}
	if kind == domain.ReviewerProxy {
		p := aiProv("claude-sonnet-5-5")
		d.ProxyProvenance = &p
	}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	return d
}

func mustPut(t *testing.T, s Store, entity any) {
	t.Helper()
	rec, err := domain.NewRecord(entity)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatalf("put %T: %v", entity, err)
	}
}

// seeded builds a store with the candidate, two disagreeing proposals and a
// validation confirming subject, change and applicability; routes it and
// stores the resulting items. It returns the consequence item.
func seeded(t *testing.T) (Store, Queue, domain.SemanticCandidate, domain.ReviewItem) {
	t.Helper()
	s := NewFileStore(t.TempDir())
	q := NewQueue(s, func() time.Time { return t0.Add(time.Hour) })
	c := fixtureCandidate()
	mustPut(t, s, c)
	p1 := proposal(c, "zai", "glm-5.3-flash", rotationAssertion(domain.ConsequenceBehaviorChange))
	p2 := proposal(c, "anthropic", "claude-sonnet-5-5", rotationAssertion(domain.ConsequenceSettingIgnored))
	mustPut(t, s, p1)
	mustPut(t, s, p2)
	v := validation(c, rotationAssertion(domain.ConsequenceBehaviorChange), domain.AspectSubject, domain.AspectChange, domain.AspectApplicability)
	mustPut(t, s, v)
	res := Route(c, []domain.SemanticProposal{p1, p2}, []domain.ValidationResult{v})
	if res.Fact != nil || len(res.ReviewItems) != 1 {
		t.Fatalf("route = fact %v, %d items; want one consequence item", res.Fact != nil, len(res.ReviewItems))
	}
	for _, it := range res.ReviewItems {
		mustPut(t, s, it)
	}
	return s, q, c, res.ReviewItems[0]
}
