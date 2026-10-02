package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func renderable(t *testing.T) (Store, domain.SemanticCandidate) {
	t.Helper()
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	c.Renderability = domain.RenderVerifiable
	mustPut(t, s, c)
	return s, c
}

func TestConsensusRequiresIndependentFamilies(t *testing.T) {
	c := fixtureCandidate()
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	same := []domain.SemanticProposal{proposal(c, "anthropic", "claude-opus-5-5", a), proposal(c, "anthropic", "claude-sonnet-5-5", a)}
	if got := ConsensusAspects(same, nil); len(got) != 0 {
		t.Fatalf("Opus+Sonnet counted as consensus: %v", got)
	}
	indep := []domain.SemanticProposal{proposal(c, "anthropic", "claude-sonnet-5-5", a), proposal(c, "zai", "glm-5.3-flash", a)}
	if got := ConsensusAspects(indep, nil); len(got) != 4 {
		t.Fatalf("independent agreement gave %d aspects, want 4", len(got))
	}
	// a validator refutation removes the aspect from consensus
	v := validation(c, a, domain.AspectSubject)
	v.Checks[0].Outcome = domain.OutcomeRefuted
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	if got := ConsensusAspects(indep, []domain.ValidationResult{v}); len(got) != 3 {
		t.Fatalf("refuted aspect still consensus: %d", len(got))
	}
}

func TestAutoApprovalMintsMarkedFactAndAuditsASample(t *testing.T) {
	s, c := renderable(t)
	ctx := context.Background()
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	mustPut(t, s, proposal(c, "anthropic", "claude-sonnet-5-5", a))
	mustPut(t, s, proposal(c, "zai", "glm-5.3-flash", a))
	sum, err := RouteStore(ctx, s, RouteOptions{Policy: AutoApproveRenderVerifiable, AuditEvery: 1, Now: func() time.Time { return t0.Add(time.Hour) }}, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Facts != 1 || len(sum.Audits) != 1 || sum.Items != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	snap, _ := s.Load(ctx, Query{})
	f := snap.Facts[0]
	if !f.AutoApproved || f.Level() != domain.VerifiedConsensus {
		t.Fatalf("fact = auto %v level %s", f.AutoApproved, f.Level())
	}
	// idempotent
	if _, err := RouteStore(ctx, s, RouteOptions{Policy: AutoApproveRenderVerifiable, AuditEvery: 1}, Query{}); err != nil {
		t.Fatal(err)
	}

	// the human audit accepts: measured, but the fact is not upgraded
	q := NewQueue(s, nil)
	item := snap.ReviewItems[0]
	_ = item
	snap, _ = s.Load(ctx, Query{})
	it := snap.ReviewItems[0]
	out, err := q.Decide(ctx, []domain.ReviewDecision{decision(it, "engineer-1", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Fact == nil || !out[0].Fact.AutoApproved || out[0].Fact.Level() != domain.VerifiedConsensus {
		t.Fatalf("audit accept changed the fact: %+v", out[0].Fact)
	}
	snap, _ = s.Load(ctx, Query{})
	m := ComputeMetrics(snap).Facts
	if m.AutoApproved != 1 || m.AutoApprovedAudited != 1 || m.AutoApprovalAgreement != 1 || m.AutoApprovalAgreementBy[domain.SubjectCRDField] != 1 {
		t.Fatalf("audit metrics = %+v", m)
	}
	if m.ByLevel[domain.VerifiedConsensus] != 1 {
		t.Fatalf("by level = %v", m.ByLevel)
	}

	// a later reject retracts it and counts as disagreement
	if _, err := q.Decide(ctx, []domain.ReviewDecision{decision(it, "engineer-2", domain.ReviewerHuman, domain.ActionReject, t0.Add(3*time.Hour))}); err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Load(ctx, Query{})
	m = ComputeMetrics(snap).Facts
	if m.AutoApprovalAgreement != 0 || m.Retracted != 1 {
		t.Fatalf("after reject: %+v", m)
	}
}

func TestNotRenderVerifiableIsNeverAutoApproved(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate() // no renderability
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	mustPut(t, s, proposal(c, "anthropic", "claude-sonnet-5-5", a))
	mustPut(t, s, proposal(c, "zai", "glm-5.3-flash", a))
	sum, err := RouteStore(context.Background(), s, RouteOptions{Policy: AutoApproveRenderVerifiable}, Query{})
	if err != nil || sum.Facts != 0 || sum.Items == 0 {
		t.Fatalf("summary = %+v, %v", sum, err)
	}
}

func TestSampledForAudit(t *testing.T) {
	if SampledForAudit("vf-x", 0) || !SampledForAudit("vf-x", 1) {
		t.Fatal("bounds")
	}
	n := 0
	for i := 0; i < 400; i++ {
		if SampledForAudit(domain.ShortHash("f", string(rune(i))), 4) {
			n++
		}
	}
	if n < 60 || n > 140 {
		t.Fatalf("1-in-4 sample picked %d of 400", n)
	}
}
