package knowledge

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// auditedConsensusFact reproduces the 2026-10-16 situation: an auto-approved
// fact (subject + change render-confirmed, applicability and consequence by
// consensus) whose audit (classification) item a human accepted, so only the
// consequence became human and the fact stays consensus by its weakest aspect.
func auditedConsensusFact(t *testing.T) (Store, string, domain.VerifiedFact, domain.ReviewDecision) {
	t.Helper()
	dir := t.TempDir()
	s := NewFileStore(dir)
	c := fixtureCandidate()
	c.Renderability = domain.RenderVerifiable
	mustPut(t, s, c)
	ctx := context.Background()
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	base := proposal(c, "anthropic", "claude-sonnet-5-5", a)
	mustPut(t, s, call(base, "call-1", ""))
	mustPut(t, s, call(base, "call-2", ""))
	mustPut(t, s, renderConfirmed(c, a))
	if _, err := RouteStore(ctx, s, RouteOptions{Policy: DefaultAutoApprove, AuditEvery: 1, Now: func() time.Time { return t0.Add(time.Hour) }}, Query{}); err != nil {
		t.Fatal(err)
	}
	snap, _ := s.Load(ctx, Query{})
	audit := decision(snap.ReviewItems[0], "product-owner", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))
	out, err := NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{audit})
	if err != nil {
		t.Fatal(err)
	}
	f := *out[0].Fact
	if f.Level() != domain.VerifiedConsensus || f.AspectLevel(domain.AspectConsequence) != domain.VerifiedHuman ||
		f.AspectLevel(domain.AspectApplicability) != domain.VerifiedConsensus {
		t.Fatalf("fixture fact: level %s, %+v", f.Level(), f.Verification)
	}
	return s, dir, f, audit
}

func decisionFile(t *testing.T, dir, id string) []byte {
	t.Helper()
	var found []byte
	_ = filepath.WalkDir(dir, func(p string, _ os.DirEntry, _ error) error {
		if filepath.Base(p) == id+".json" {
			found, _ = os.ReadFile(p)
		}
		return nil
	})
	if found == nil {
		t.Fatalf("decision %s not stored", id)
	}
	return found
}

func activeFact(t *testing.T, s Store) domain.VerifiedFact {
	t.Helper()
	snap, _ := s.Load(context.Background(), Query{})
	for _, f := range snap.Facts {
		if f.Status == domain.FactActive {
			return f
		}
	}
	t.Fatal("no active fact")
	return domain.VerifiedFact{}
}

func TestOpenWholeFactReviewIsIdempotentAndTouchesNothingElse(t *testing.T) {
	s, _, f, _ := auditedConsensusFact(t)
	ctx := context.Background()
	it, err := OpenWholeFactReview(ctx, s, f.ID, t0.Add(3*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if it.QuestionType != domain.QuestionFactReview || it.Proposed.Digest() != f.Assertion.Digest() ||
		!slices.Equal(it.Aspects(), domain.Aspects) || !strings.Contains(it.Question, "applicability consensus") {
		t.Fatalf("item = %+v", it)
	}
	again, err := OpenWholeFactReview(ctx, s, f.ID, t0.Add(9*time.Hour))
	if err != nil || again.ID != it.ID || !again.CreatedAt.Equal(it.CreatedAt) {
		t.Fatalf("re-open: %v, %+v", err, again)
	}
	if got := activeFact(t, s); got.Level() != domain.VerifiedConsensus {
		t.Fatalf("opening an item verified something: %+v", got.Verification)
	}
}

func TestWholeFactAcceptRaisesDisplayedAspectsOnly(t *testing.T) {
	ctx := context.Background()
	t.Run("every aspect displayed", func(t *testing.T) {
		s, dir, f, audit := auditedConsensusFact(t)
		before := decisionFile(t, dir, audit.ID)
		it, _ := OpenWholeFactReview(ctx, s, f.ID, t0.Add(3*time.Hour))
		d := decision(*it, "product-owner", domain.ReviewerHuman, domain.ActionAccept, t0.Add(4*time.Hour))
		d.Aspects = domain.Aspects
		if _, err := NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{d}); err != nil {
			t.Fatal(err)
		}
		got := activeFact(t, s)
		if got.ID != f.ID || got.Level() != domain.VerifiedDeterministic && got.Level() != domain.VerifiedHuman {
			t.Fatalf("fact after whole-fact accept: %s %+v", got.Level(), got.Verification)
		}
		want := map[domain.Aspect]domain.VerificationLevel{
			domain.AspectSubject: domain.VerifiedDeterministic, domain.AspectChange: domain.VerifiedDeterministic,
			domain.AspectApplicability: domain.VerifiedHuman, domain.AspectConsequence: domain.VerifiedHuman,
		}
		for x, l := range want {
			if got.AspectLevel(x) != l {
				t.Errorf("%s = %s, want %s", x, got.AspectLevel(x), l)
			}
		}
		// past decisions untouched: the product owner's audit stays exactly as
		// recorded and remains the basis of the consequence it verified
		if !bytes.Equal(before, decisionFile(t, dir, audit.ID)) {
			t.Error("the earlier decision record changed")
		}
		for _, v := range got.Verification {
			if v.Aspect == domain.AspectConsequence && !slices.Equal(v.Basis, []string{audit.ID}) {
				t.Errorf("consequence basis = %v, want the earlier decision %s", v.Basis, audit.ID)
			}
			if v.Aspect == domain.AspectApplicability && !slices.Equal(v.Basis, []string{d.ID}) {
				t.Errorf("applicability basis = %v, want %s", v.Basis, d.ID)
			}
		}
	})
	t.Run("an aspect not displayed is not raised", func(t *testing.T) {
		s, _, f, _ := auditedConsensusFact(t)
		it, _ := OpenWholeFactReview(ctx, s, f.ID, t0.Add(3*time.Hour))
		d := decision(*it, "product-owner", domain.ReviewerHuman, domain.ActionAccept, t0.Add(4*time.Hour))
		d.Aspects = []domain.Aspect{domain.AspectSubject, domain.AspectChange, domain.AspectConsequence}
		if _, err := NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{d}); err != nil {
			t.Fatal(err)
		}
		if got := activeFact(t, s); got.AspectLevel(domain.AspectApplicability) != domain.VerifiedConsensus || got.Level() != domain.VerifiedConsensus {
			t.Fatalf("applicability raised without being shown: %+v", got.Verification)
		}
	})
}

func TestProxyWholeFactAcceptIsNeverHuman(t *testing.T) {
	s, _, f, _ := auditedConsensusFact(t)
	ctx := context.Background()
	it, _ := OpenWholeFactReview(ctx, s, f.ID, t0.Add(3*time.Hour))
	d := decision(*it, "claude-proxy", domain.ReviewerProxy, domain.ActionAccept, t0.Add(4*time.Hour))
	d.Aspects = domain.Aspects
	if _, err := NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{d}); err != nil {
		t.Fatal(err)
	}
	got := activeFact(t, s)
	if got.AspectLevel(domain.AspectApplicability) != domain.VerifiedConsensus || got.Level() != domain.VerifiedConsensus {
		t.Fatalf("proxy whole-fact accept: %+v", got.Verification)
	}
	for _, v := range got.Verification {
		if v.Level == domain.VerifiedHuman && slices.Contains(v.Basis, d.ID) {
			t.Fatalf("a proxy decision backs a human aspect: %+v", v)
		}
	}
}

func TestWholeFactCorrectAndReject(t *testing.T) {
	ctx := context.Background()
	t.Run("correct supersedes with per-aspect labels", func(t *testing.T) {
		s, _, f, _ := auditedConsensusFact(t)
		it, _ := OpenWholeFactReview(ctx, s, f.ID, t0.Add(3*time.Hour))
		d := decision(*it, "product-owner", domain.ReviewerHuman, domain.ActionCorrect, t0.Add(4*time.Hour))
		fixed := it.Proposed
		app := *fixed.Applicability
		app.Exposure = certificates(domain.Condition{Op: domain.OpField, Path: "spec.privateKey.rotationPolicy", State: domain.StateSet})
		fixed.Applicability = &app
		d.Corrected, d.Reason, d.Aspects = &fixed, "every Certificate is exposed", domain.Aspects
		d.Labels = []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongApplicability}
		if _, err := NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{d}); err != nil {
			t.Fatal(err)
		}
		got := activeFact(t, s)
		if got.ID == f.ID || !slices.Contains(got.Supersedes, f.ID) || got.AspectLevel(domain.AspectApplicability) != domain.VerifiedHuman {
			t.Fatalf("corrected fact = %+v", got)
		}
	})
	t.Run("reject retracts", func(t *testing.T) {
		s, _, f, _ := auditedConsensusFact(t)
		it, _ := OpenWholeFactReview(ctx, s, f.ID, t0.Add(3*time.Hour))
		d := decision(*it, "product-owner", domain.ReviewerHuman, domain.ActionReject, t0.Add(4*time.Hour))
		if _, err := NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{d}); err != nil {
			t.Fatal(err)
		}
		snap, _ := s.Load(ctx, Query{})
		if snap.Facts[0].Status != domain.FactRetracted {
			t.Fatalf("fact status %s", snap.Facts[0].Status)
		}
	})
}

func TestRecordedAspectsMustFitTheQuestion(t *testing.T) {
	s, q, _, item := seeded(t) // a consequence item
	full := rotationAssertion(domain.ConsequenceBehaviorChange)
	d := decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))
	d.Original = &full // states the subject, which a consequence question does not verify
	d.Aspects = []domain.Aspect{domain.AspectSubject, domain.AspectConsequence}
	if _, err := q.Decide(context.Background(), []domain.ReviewDecision{d}); err == nil || !strings.Contains(err.Error(), "does not verify") {
		t.Fatalf("aspect outside the question accepted: %v", err)
	}
	snap, _ := s.Load(context.Background(), Query{})
	if len(snap.Decisions) != 0 {
		t.Fatal("a refused decision was written")
	}
	for _, bad := range []domain.ReviewDecision{
		func() domain.ReviewDecision {
			x := decision(item, "e", domain.ReviewerHuman, domain.ActionReject, t0.Add(2*time.Hour))
			x.Aspects = []domain.Aspect{domain.AspectConsequence}
			return x
		}(),
		func() domain.ReviewDecision {
			x := decision(item, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))
			x.Aspects = []domain.Aspect{domain.AspectConsequence, domain.AspectConsequence}
			return x
		}(),
	} {
		if err := bad.Validate(); err == nil {
			t.Errorf("decision %+v validated", bad.Aspects)
		}
	}
}

func TestFactBasisMustBeAnAspectTheDecisionRecorded(t *testing.T) {
	s, _, f, _ := auditedConsensusFact(t)
	ctx := context.Background()
	it, _ := OpenWholeFactReview(ctx, s, f.ID, t0.Add(3*time.Hour))
	d := decision(*it, "product-owner", domain.ReviewerHuman, domain.ActionAccept, t0.Add(4*time.Hour))
	d.Aspects = []domain.Aspect{domain.AspectConsequence}
	forged := f
	forged.Verification = slices.Clone(f.Verification)
	for i := range forged.Verification {
		if forged.Verification[i].Aspect == domain.AspectApplicability {
			forged.Verification[i] = domain.AspectVerification{Aspect: domain.AspectApplicability, Level: domain.VerifiedHuman, Basis: []string{d.ID}}
		}
	}
	snap, _ := s.Load(ctx, Query{})
	recs := domain.FactRecords{Validations: map[string]domain.ValidationResult{}, Decisions: map[string]domain.ReviewDecision{d.ID: d},
		Items: map[string]domain.ReviewItem{it.ID: *it}, Proposals: map[string]domain.SemanticProposal{}}
	for _, v := range snap.Validations {
		recs.Validations[v.ID] = v
	}
	for _, p := range snap.Proposals {
		recs.Proposals[p.ID] = p
	}
	for _, x := range snap.Decisions {
		recs.Decisions[x.ID] = x
	}
	for _, x := range snap.ReviewItems {
		recs.Items[x.ID] = x
	}
	err := domain.ValidateFactRecords(forged, recs)
	if err == nil || !strings.Contains(err.Error(), "records that it verified only") {
		t.Fatalf("basis outside the recorded aspects accepted: %v", err)
	}
}
