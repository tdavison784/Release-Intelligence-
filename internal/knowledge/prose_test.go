package knowledge

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// proseCorrection turns d into a prose-only correction of the consequence.
func proseCorrection(d domain.ReviewDecision, statement, remediation string) domain.ReviewDecision {
	c := *d.Original
	cons := *c.Consequence
	cons.Statement, cons.Remediation = statement, remediation
	c.Consequence = &cons
	d.Action = domain.ActionCorrect
	d.Corrected = &c
	d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelImprovedStatement}
	d.Reason = "clearer wording"
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	return d
}

func TestProseOnlyCorrectionMintsTheFactWithTheCorrectedProse(t *testing.T) {
	_, q, _, item := seeded(t)
	d := proseCorrection(decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour)),
		"Renewals fail until the key is rotated by hand.", "Set rotationPolicy explicitly.")
	if !IsProseOnlyCorrection(d) {
		t.Fatal("not recognised as prose-only")
	}
	out, err := q.Decide(context.Background(), []domain.ReviewDecision{d})
	if err != nil {
		t.Fatal(err)
	}
	f := out[0].Fact
	if f == nil || f.Assertion.Consequence.Statement != "Renewals fail until the key is rotated by hand." || f.Assertion.Consequence.Remediation != "Set rotationPolicy explicitly." {
		t.Fatalf("fact consequence = %+v", f)
	}
	if out[0].Decision.Original == nil || out[0].Decision.Corrected == nil {
		t.Fatal("decision lost a value")
	}
}

func TestProseOnlyCorrectionUpdatesAnExistingFactUnderTheSameID(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	first, err := q.Decide(ctx, []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	old := first[0].Fact
	rv, err := OpenFactReview(ctx, s, old.ID, t0.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	d := proseCorrection(decision(*rv, "e2", domain.ReviewerHuman, domain.ActionAccept, t0.Add(3*time.Hour)),
		"Reworded statement.", "Reworded remediation.")
	out, err := q.Decide(ctx, []domain.ReviewDecision{d})
	if err != nil {
		t.Fatal(err)
	}
	nf := out[0].Fact
	if nf == nil || nf.ID != old.ID || nf.Status != domain.FactActive || len(nf.Supersedes) != 0 {
		t.Fatalf("a prose-only correction must keep the fact: %+v", nf)
	}
	rec, _ := s.Get(ctx, old.ID)
	if rec.Fact.Assertion.Consequence.Statement != "Reworded statement." || rec.Fact.Assertion.Consequence.Remediation != "Reworded remediation." {
		t.Fatalf("stored prose = %+v", rec.Fact.Assertion.Consequence)
	}
	if rec.Fact.Assertion.Digest() != old.Assertion.Digest() {
		t.Fatal("the typed assertion changed")
	}
	// both decisions justify the consequence aspect, and the basis re-proves
	var basis []string
	for _, v := range rec.Fact.Verification {
		if v.Aspect == domain.AspectConsequence {
			basis = v.Basis
		}
	}
	if len(basis) != 2 {
		t.Fatalf("consequence basis = %v, want both decisions", basis)
	}
}

func TestProxyProseCorrectionNeverRewritesAHumansProse(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	first, _ := q.Decide(ctx, []domain.ReviewDecision{proseCorrection(decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour)), "Human prose.", "")})
	fid := first[0].Fact.ID
	rv, _ := OpenFactReview(ctx, s, fid, t0.Add(2*time.Hour))
	d := proseCorrection(decision(*rv, "claude", domain.ReviewerProxy, domain.ActionAccept, t0.Add(3*time.Hour)), "Proxy prose.", "")
	if _, err := q.Decide(ctx, []domain.ReviewDecision{d}); err != nil {
		t.Fatal(err)
	}
	rec, _ := s.Get(ctx, fid)
	if got := rec.Fact.Assertion.Consequence.Statement; got != "Human prose." {
		t.Fatalf("proxy rewrote the human's prose: %q", got)
	}
	if rec.Fact.AspectLevel(domain.AspectConsequence) != domain.VerifiedHuman {
		t.Fatal("level weakened")
	}
}

func TestStoreAllowsOnlyTheConsequenceProseToChangeOnAFact(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	out, _ := q.Decide(ctx, []domain.ReviewDecision{decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour))})
	f := *out[0].Fact
	c := *f.Assertion.Consequence
	c.Statement = "edited"
	edited := f
	edited.Assertion.Consequence = &c
	mustPut(t, s, edited) // allowed: consequence prose
	other := f
	other.Assertion.Statement = "a different one-line statement"
	rec, _ := domain.NewRecord(other)
	if err := s.Put(ctx, rec); !errors.Is(err, ErrConflict) {
		t.Fatalf("changing the assertion statement = %v, want ErrConflict", err)
	}
}

func TestProseEditsAreReportedSeparately(t *testing.T) {
	s, q, _, item := seeded(t)
	ctx := context.Background()
	d := proseCorrection(decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(time.Hour)), "Better words.", "")
	if _, err := q.Decide(ctx, []domain.ReviewDecision{d}); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Load(ctx, Query{})
	m := ComputeMetrics(snap)
	if m.Review.Individual.ProseEdits != 1 || m.Review.Individual.Correct != 0 {
		t.Fatalf("decision stats = %+v", m.Review.Individual)
	}
	edits, asIs := 0, 0
	for _, mm := range m.Models {
		edits += mm.ProseEdits
		asIs += mm.AcceptedAsIs
	}
	if edits != 1 || asIs != 1 {
		t.Fatalf("model prose edits %d, accepted as is %d (the winner's typed value was right)", edits, asIs)
	}
}

func TestProposalFixturesStateTheProvider(t *testing.T) {
	c := fixtureCandidate()
	p := proposal(c, "zai", "glm", rotationAssertion(domain.ConsequenceBehaviorChange))
	if p.Provenance.Provider != "zai" || p.Provenance.ProviderName() != "zai" {
		t.Fatalf("provenance provider = %q", p.Provenance.Provider)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
}
