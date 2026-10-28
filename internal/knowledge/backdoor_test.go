package knowledge

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Lane backdoor (D-0029): every route that mints or changes an active fact
// names the other active facts of its product, release and subject. The
// route-B reproduction of the M4 negative-control build: after the narrow
// rule is promoted (superseding the broad fact), a human correct on the
// ordinary item behind the broad fact used to mint a second, contradicting
// active fact that named neither.

// promotedFixture promotes the narrow rule through the gate (superseding the
// broad fact) and returns the fixture, the promoted fact and the ordinary
// item the broad fact was decided on.
func promotedFixture(t *testing.T) (*gateFixture, domain.VerifiedFact, domain.ReviewItem) {
	t.Helper()
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds))
	d := humanAccept(f.item, t0.Add(2*time.Hour))
	d.Supersedes = []string{f.broad.ID}
	out, err := f.decide(t, d)
	if err != nil || out[0].Fact == nil {
		t.Fatalf("promotion: %v", err)
	}
	snap, _ := f.s.Load(context.Background(), Query{})
	var ordinary *domain.ReviewItem
	for i, it := range snap.ReviewItems {
		if it.ID != f.item.ID && it.CandidateID == f.c.ID {
			ordinary = &snap.ReviewItems[i]
		}
	}
	if ordinary == nil {
		t.Fatal("no ordinary item")
	}
	if _, _, hyp := HypothesisSource(snap, *ordinary); hyp {
		t.Fatal("the broad fact's item must be an ordinary item")
	}
	return f, *out[0].Fact, *ordinary
}

// contradictingRule is the route-B correction: the same exposure, but every
// unset Widget now fails (workload-failure / action-required).
func contradictingRule() domain.SemanticAssertion {
	a := broadRule()
	a.Consequence = &domain.Consequence{Kind: domain.ConsequenceWorkloadFailure, ExposedClass: domain.ImpactActionRequired, Severity: domain.SeverityHigh,
		Statement: "every Widget that leaves the field unset loses its data at rollover", Remediation: "set spec.retention.mode: Keep"}
	return a
}

// compatibleRule states the narrow rule's change, exposure and consequence
// kind, with another severity and without the overlap.
func compatibleRule() domain.SemanticAssertion {
	a := broadRule()
	a.Consequence.Severity = domain.SeverityMedium
	return a
}

func humanCorrect(it domain.ReviewItem, to domain.SemanticAssertion, at time.Time) domain.ReviewDecision {
	d := decision(it, "product-owner", domain.ReviewerHuman, domain.ActionCorrect, at)
	d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongConsequence}
	d.Reason = "M4 negative control (synthetic)"
	d.Corrected = &to
	return d
}

func activeFacts(t *testing.T, s Store) map[string]bool {
	t.Helper()
	snap, err := s.Load(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, f := range snap.Facts {
		if f.Status == domain.FactActive {
			out[f.ID] = true
		}
	}
	return out
}

func TestRouteBOrdinaryCorrectionMustNameTheActiveFact(t *testing.T) {
	f, narrow, ordinary := promotedFixture(t)
	before := activeFacts(t, f.s)
	_, err := f.decide(t, humanCorrect(ordinary, contradictingRule(), t0.Add(3*time.Hour)))
	refused(t, err, narrow.ID)
	if !strings.Contains(err.Error(), "no fact silently coexists") {
		t.Fatalf("err = %v", err)
	}
	if after := activeFacts(t, f.s); len(after) != len(before) {
		t.Fatalf("a refused decision changed the active facts: %v → %v", before, after)
	}
}

func TestRouteBNamingTheFactIsAccepted(t *testing.T) {
	f, narrow, ordinary := promotedFixture(t)
	d := humanCorrect(ordinary, contradictingRule(), t0.Add(3*time.Hour))
	d.Supersedes = []string{narrow.ID}
	out, err := f.decide(t, d)
	if err != nil || out[0].Fact == nil {
		t.Fatalf("named supersede refused: %v", err)
	}
	active := activeFacts(t, f.s)
	if active[narrow.ID] || !active[out[0].Fact.ID] || len(active) != 1 {
		t.Fatalf("active = %v; want only the correction", active)
	}
}

func TestCoexistenceOfContradictingFactsIsRefused(t *testing.T) {
	f, narrow, ordinary := promotedFixture(t)
	d := humanCorrect(ordinary, contradictingRule(), t0.Add(3*time.Hour))
	d.Coexists = []string{narrow.ID}
	_, err := f.decide(t, d)
	refused(t, err, "cannot coexist with "+narrow.ID)
	if !strings.Contains(err.Error(), "consequence") {
		t.Fatalf("err = %v; want the disagreement named", err)
	}
}

func TestCompatibleCoexistenceIsAllowed(t *testing.T) {
	f, narrow, ordinary := promotedFixture(t)
	d := humanCorrect(ordinary, compatibleRule(), t0.Add(3*time.Hour))
	d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongConsequence}
	d.Coexists = []string{narrow.ID}
	out, err := f.decide(t, d)
	if err != nil || out[0].Fact == nil {
		t.Fatalf("compatible coexistence refused: %v", err)
	}
	if active := activeFacts(t, f.s); !active[narrow.ID] || !active[out[0].Fact.ID] {
		t.Fatalf("active = %v; want both", active)
	}
}

// The duplicate route changes the named fact (anchors): it, too, names the
// other active facts of that fact's subject, and may only keep them.
func TestDuplicateMustNameTheOtherActiveFacts(t *testing.T) {
	f, narrow, ordinary := promotedFixture(t)
	d := humanCorrect(ordinary, compatibleRule(), t0.Add(3*time.Hour))
	d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongConsequence}
	d.Coexists = []string{narrow.ID}
	out, err := f.decide(t, d)
	if err != nil {
		t.Fatal(err)
	}
	other := out[0].Fact.ID
	it := domain.ReviewItem{CandidateID: f.c.ID, Product: gateProduct, Release: "2.0.0", QuestionType: domain.QuestionDuplicate,
		Question: "Is this the same change as the known fact?", Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityLow},
		Status: domain.ReviewPending, CreatedAt: t0.Add(4 * time.Hour)}
	it.ID = domain.ReviewItemID(it.CandidateID, it.QuestionType, it.Proposed)
	mustPut(t, f.s, it)
	dup := decision(it, "product-owner", domain.ReviewerHuman, domain.ActionReject, t0.Add(5*time.Hour))
	dup.Labels, dup.DuplicateOf, dup.Reason = []domain.FeedbackLabel{domain.LabelDuplicate}, narrow.ID, "same change, restated"
	_, err = f.decide(t, dup)
	refused(t, err, other)
	dup.Coexists = []string{other}
	if _, err := f.decide(t, dup); err != nil {
		t.Fatalf("named duplicate refused: %v", err)
	}
	dup.Supersedes = []string{other}
	if err := dup.Validate(); err == nil {
		t.Fatal("a duplicate cannot supersede")
	}
}

// Ordinary items without another active fact on the subject are unchanged.
func TestOrdinaryDecisionWithoutOtherFactsIsUnchanged(t *testing.T) {
	_, q, _, item := seeded(t)
	if _, err := q.Decide(context.Background(), []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))}); err != nil {
		t.Fatal(err)
	}
}
