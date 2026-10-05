package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// call returns p as the answer of a specific stateless call.
func call(p domain.SemanticProposal, id string, class domain.ImpactClass) domain.SemanticProposal {
	p.Provenance.CallID = id
	p.SuggestedClass = class
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

func renderEvidence() domain.Evidence {
	e := domain.NewEvidence(domain.EvidenceStructured, "render", "https://example/chart/templates/crd.yaml", "$.spec", "rotationPolicy: Always", "sha256:r", t0)
	e.Render = &domain.RenderProvenance{Scope: domain.RenderRelease, Tool: "helm", ToolVersion: "v3.17.2", ChartDigest: "sha256:chart"}
	return e
}

// renderConfirmed confirms subject and change by the render.
func renderConfirmed(c domain.SemanticCandidate, a domain.SemanticAssertion) domain.ValidationResult {
	v := validation(c, a, domain.AspectSubject, domain.AspectChange)
	v.Evidence = []domain.Evidence{renderEvidence()}
	v.RenderRelation = domain.RenderConfirmed
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	return v
}

func renderable(t *testing.T) (Store, domain.SemanticCandidate) {
	t.Helper()
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	c.Renderability = domain.RenderVerifiable
	mustPut(t, s, c)
	return s, c
}

func TestConsensusIsSeparateCallsLabelledByScope(t *testing.T) {
	c := fixtureCandidate()
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	base := proposal(c, "anthropic", "claude-opus-5-5", a)

	// two separate calls of the SAME model agree: same-model consensus (PO-1)
	same := []domain.SemanticProposal{call(base, "c1", ""), call(base, "c2", "")}
	got := ConsensusAspects(same, nil)
	if len(got) != 4 || got[domain.AspectSubject].Consensus != domain.ConsensusSameModel {
		t.Fatalf("same-model: %d aspects, scope %q", len(got), got[domain.AspectSubject].Consensus)
	}
	// one call counted twice is not two calls
	dup := []domain.SemanticProposal{call(base, "c1", ""), call(withDigest(base, "other"), "c1", "")}
	if got := ConsensusAspects(dup, nil); len(got) != 0 {
		t.Fatalf("one call twice counted as consensus: %v", got)
	}
	// different model families: cross-model
	cross := []domain.SemanticProposal{call(base, "c1", ""), call(proposal(c, "zai", "glm-5.3-flash", a), "c2", "")}
	if got := ConsensusAspects(cross, nil); got[domain.AspectConsequence].Consensus != domain.ConsensusCrossModel {
		t.Fatalf("cross-model label = %q", got[domain.AspectConsequence].Consensus)
	}
	// a validator refutation removes the aspect
	v := validation(c, a, domain.AspectSubject)
	v.Checks[0].Outcome = domain.OutcomeRefuted
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	if got := ConsensusAspects(cross, []domain.ValidationResult{v}); len(got) != 3 {
		t.Fatalf("refuted aspect still consensus: %d", len(got))
	}
}

func withDigest(p domain.SemanticProposal, d string) domain.SemanticProposal {
	p.Provenance.PromptDigest = "sha256:" + d
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	return p
}

func TestRenderAutoApprovalMintsMarkedFactAndAuditsASample(t *testing.T) {
	s, c := renderable(t)
	ctx := context.Background()
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	base := proposal(c, "anthropic", "claude-sonnet-5-5", a)
	mustPut(t, s, call(base, "call-1", ""))
	mustPut(t, s, call(base, "call-2", "")) // two separate calls of one model
	mustPut(t, s, renderConfirmed(c, a))
	opts := RouteOptions{Policy: DefaultAutoApprove, AuditEvery: 1, Now: func() time.Time { return t0.Add(time.Hour) }}
	sum, err := RouteStore(ctx, s, opts, Query{})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Facts != 1 || len(sum.Audits) != 1 || sum.Items != 0 {
		t.Fatalf("summary = %+v", sum)
	}
	snap, _ := s.Load(ctx, Query{})
	f := snap.Facts[0]
	if !f.AutoApproved || f.Level() != domain.VerifiedConsensus || f.ConsensusAction {
		t.Fatalf("fact = auto %v level %s action %v", f.AutoApproved, f.Level(), f.ConsensusAction)
	}
	if f.AspectLevel(domain.AspectSubject) != domain.VerifiedDeterministic || f.Verification[3].Consensus != domain.ConsensusSameModel {
		t.Fatalf("verification = %+v", f.Verification)
	}
	if _, err := RouteStore(ctx, s, opts, Query{}); err != nil { // idempotent
		t.Fatal(err)
	}

	// the human audit accepts: the verified aspect is upgraded, the marker stays
	q := NewQueue(s, nil)
	snap, _ = s.Load(ctx, Query{})
	it := snap.ReviewItems[0]
	out, err := q.Decide(ctx, []domain.ReviewDecision{decision(it, "engineer-1", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))})
	if err != nil {
		t.Fatal(err)
	}
	af := out[0].Fact
	if af == nil || !af.AutoApproved || af.AspectLevel(domain.AspectConsequence) != domain.VerifiedHuman || af.AspectLevel(domain.AspectApplicability) != domain.VerifiedConsensus {
		t.Fatalf("audit accept: %+v", af)
	}
	snap, _ = s.Load(ctx, Query{})
	m := ComputeMetrics(snap).Facts
	if m.AutoApproved != 1 || m.AutoApprovedAudited != 1 || m.AutoApprovalAgreement != 1 || m.AutoApprovalAgreementBy[domain.SubjectCRDField] != 1 {
		t.Fatalf("audit metrics = %+v", m)
	}
	if m.ConsensusAgreementByScope[domain.ConsensusSameModel] != 1 {
		t.Fatalf("by scope = %v", m.ConsensusAgreementByScope)
	}
	// a later reject retracts it and counts as disagreement
	if _, err := q.Decide(ctx, []domain.ReviewDecision{decision(it, "engineer-2", domain.ReviewerHuman, domain.ActionReject, t0.Add(3*time.Hour))}); err != nil {
		t.Fatal(err)
	}
	snap, _ = s.Load(ctx, Query{})
	m = ComputeMetrics(snap).Facts
	if m.AutoApprovalAgreement != 0 || m.Retracted != 1 || m.ConsensusAgreementByScope[domain.ConsensusSameModel] != 0 {
		t.Fatalf("after reject: %+v", m)
	}
}

func TestRenderPolicyNeedsRenderConfirmationOfSubjectAndChange(t *testing.T) {
	s, c := renderable(t)
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	base := proposal(c, "anthropic", "claude-sonnet-5-5", a)
	mustPut(t, s, call(base, "call-1", ""))
	mustPut(t, s, call(base, "call-2", ""))
	// no render confirmation: consensus alone is not the render policy
	sum, err := RouteStore(context.Background(), s, RouteOptions{Policy: AutoApproveRenderVerifiable}, Query{})
	if err != nil || sum.Facts != 0 || sum.Items == 0 {
		t.Fatalf("summary = %+v, %v", sum, err)
	}
}

func TestConsensusActionFactIsAlwaysAudited(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate() // not render-verifiable: only PO-2 can approve it
	mustPut(t, s, c)
	ctx := context.Background()
	a := rotationAssertion(domain.ConsequenceSettingIgnored)
	base := proposal(c, "anthropic", "claude-opus-5-5", a)
	mustPut(t, s, call(base, "call-1", domain.ImpactActionRequired))
	mustPut(t, s, call(proposal(c, "zai", "glm-5.3-flash", a), "call-2", domain.ImpactActionRequired))
	sum, err := RouteStore(ctx, s, RouteOptions{Policy: DefaultAutoApprove, AuditEvery: 0}, Query{})
	if err != nil || sum.Facts != 1 || len(sum.Audits) != 1 {
		t.Fatalf("summary = %+v, %v (consensus-action facts are audited at 100%% even with sampling off)", sum, err)
	}
	snap, _ := s.Load(ctx, Query{})
	f := snap.Facts[0]
	if !f.ConsensusAction || !AuditRequired(f) || f.Verification[3].Consensus != domain.ConsensusCrossModel {
		t.Fatalf("fact = %+v", f)
	}
	if m := ComputeMetrics(snap).Facts; m.ConsensusAction != 1 || m.ConsensusActionAudited != 0 {
		t.Fatalf("metrics = %+v", m)
	}
	// the audit item: accept → consequence is human; consensusAction stays while other aspects are consensus
	q := NewQueue(s, nil)
	it := snap.ReviewItems[0]
	out, err := q.Decide(ctx, []domain.ReviewDecision{decision(it, "e", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))})
	if err != nil || out[0].Fact == nil || !out[0].Fact.ConsensusAction {
		t.Fatalf("audit accept = %+v, %v", out, err)
	}
	snap, _ = s.Load(ctx, Query{})
	if m := ComputeMetrics(snap).Facts; m.ConsensusActionAudited != 1 || m.ConsensusActionAgreement != 1 {
		t.Fatalf("metrics = %+v", m)
	}
}

func TestConsensusActionNeedsEveryAgreeingProposalToRequestAction(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceSettingIgnored)
	base := proposal(c, "anthropic", "claude-opus-5-5", a)
	mustPut(t, s, call(base, "call-1", domain.ImpactActionRequired))
	mustPut(t, s, call(base, "call-2", domain.ImpactReviewRequired)) // agrees on the digest, did not request action
	sum, err := RouteStore(context.Background(), s, RouteOptions{Policy: DefaultAutoApprove}, Query{})
	if err != nil || sum.Facts != 0 {
		t.Fatalf("a consensus where one call asked only for review was auto-approved: %+v, %v", sum, err)
	}
}

func TestNeverRenderableNeverActionIsNeverAutoApproved(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	base := proposal(c, "anthropic", "claude-sonnet-5-5", a)
	mustPut(t, s, call(base, "call-1", ""))
	mustPut(t, s, call(base, "call-2", ""))
	sum, err := RouteStore(context.Background(), s, RouteOptions{Policy: DefaultAutoApprove}, Query{})
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

// --- contract-6: PO-5 dissent, PO-6 audit -------------------------------------------

// dissentingCall answers the consequence like the others (same kind, a
// different severity: a different digest) but requests review — the flux E6
// shape (TRUST-AUDIT.md §2).
func dissentingCall(c domain.SemanticCandidate, id string) domain.SemanticProposal {
	a := rotationAssertion(domain.ConsequenceSettingIgnored)
	a.Consequence.Severity = domain.SeverityMedium
	return call(proposal(c, "anthropic", "claude-sonnet-5-5", a), id, domain.ImpactReviewRequired)
}

func TestConsensusActionBlockedByDissent(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceSettingIgnored)
	mustPut(t, s, call(proposal(c, "zai", "glm-5.3", a), "call-1", domain.ImpactActionRequired))
	mustPut(t, s, call(proposal(c, "zai", "glm-5.3", a), "call-2", domain.ImpactActionRequired))
	mustPut(t, s, dissentingCall(c, "call-3"))
	sum, err := RouteStore(context.Background(), s, RouteOptions{Policy: DefaultAutoApprove}, Query{})
	if err != nil || sum.Facts != 0 || sum.Items == 0 {
		t.Fatalf("two same-model calls requesting ACTION over a dissenting call were auto-approved: %+v, %v", sum, err)
	}
}

func TestRerouteRefreshesConsensusActionUnderDissent(t *testing.T) {
	ctx := context.Background()
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceSettingIgnored)
	mustPut(t, s, call(proposal(c, "zai", "glm-5.3", a), "call-1", domain.ImpactActionRequired))
	mustPut(t, s, call(proposal(c, "anthropic", "claude-opus-5-5", a), "call-2", domain.ImpactActionRequired))
	if sum, err := RouteStore(ctx, s, RouteOptions{Policy: DefaultAutoApprove}, Query{}); err != nil || sum.Facts != 1 {
		t.Fatalf("setup: %+v, %v", sum, err)
	}
	// a later call dissents: re-routing must clear the stored flag (PO-5)
	mustPut(t, s, dissentingCall(c, "call-3"))
	sum, err := RouteStore(ctx, s, RouteOptions{Policy: DefaultAutoApprove}, Query{})
	if err != nil || len(sum.Refreshed) != 1 || len(sum.Skipped) != 0 {
		t.Fatalf("re-route = %+v, %v", sum, err)
	}
	snap, _ := s.Load(ctx, Query{})
	if f := snap.Facts[0]; f.ConsensusAction || f.Status != domain.FactActive {
		t.Fatalf("fact after PO-5 refresh = consensusAction %v status %s (want false, active: REVIEW)", f.ConsensusAction, f.Status)
	}
	// idempotent
	if sum, err := RouteStore(ctx, s, RouteOptions{Policy: DefaultAutoApprove}, Query{}); err != nil || len(sum.Refreshed) != 0 {
		t.Fatalf("second re-route refreshed again: %+v, %v", sum, err)
	}
}

func TestOnlyAHumanAcceptAuditsAConsensusAction(t *testing.T) {
	for _, tc := range []struct {
		kind    domain.ReviewerKind
		audited bool
	}{{domain.ReviewerProxy, false}, {domain.ReviewerHuman, true}} {
		t.Run(string(tc.kind), func(t *testing.T) {
			ctx := context.Background()
			s := NewFileStore(t.TempDir())
			c := fixtureCandidate()
			mustPut(t, s, c)
			a := rotationAssertion(domain.ConsequenceSettingIgnored)
			mustPut(t, s, call(proposal(c, "anthropic", "claude-opus-5-5", a), "call-1", domain.ImpactActionRequired))
			mustPut(t, s, call(proposal(c, "zai", "glm-5.3-flash", a), "call-2", domain.ImpactActionRequired))
			if _, err := RouteStore(ctx, s, RouteOptions{Policy: DefaultAutoApprove}, Query{}); err != nil {
				t.Fatal(err)
			}
			snap, _ := s.Load(ctx, Query{})
			if snap.Facts[0].AuditedBy != "" {
				t.Fatal("a fresh consensus-action fact must be unaudited")
			}
			d := decision(snap.ReviewItems[0], "e", tc.kind, domain.ActionAccept, t0.Add(2*time.Hour))
			if _, err := NewQueue(s, nil).Decide(ctx, []domain.ReviewDecision{d}); err != nil {
				t.Fatal(err)
			}
			snap, _ = s.Load(ctx, Query{})
			var f domain.VerifiedFact
			for _, x := range snap.Facts {
				if x.Status == domain.FactActive {
					f = x
				}
			}
			if (f.AuditedBy == d.ID) != tc.audited || (f.AuditedBy != "") != tc.audited {
				t.Fatalf("%s accept: auditedBy = %q (want audited=%v)", tc.kind, f.AuditedBy, tc.audited)
			}
		})
	}
}
