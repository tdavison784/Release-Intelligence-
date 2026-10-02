package render

import (
	"context"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// R11 adversarial: "RBAC verb removed" is confirmed by the release-level
// render, but a render difference alone never produces ACTION REQUIRED — the
// render proves the structural change, not that the workload depends on the
// permission. Followed end to end through validation and routing:
//
//  1. the rendered-diff validator confirms subject + change only; it never
//     checks applicability or consequence, even when the assertion carries an
//     action-eligible consequence;
//  2. with no verified consequence (one model call asking for action) no fact
//     is minted at all: the consequence goes to review;
//  3. with two separate calls agreeing on the consequence but not requesting
//     action, an auto-approved fact is minted at consensus level without
//     ConsensusAction: untrusted, so the trust ladder caps it at REVIEW;
//  4. only when every agreeing call requests action-required (PO-2) does the
//     fact carry ConsensusAction ("ACTION REQUIRED · model consensus", always
//     audited) — the models' consensus on the consequence, not the render.
func TestRenderDeltaAloneNeverYieldsAction(t *testing.T) {
	ctx := context.Background()
	up := domain.NewEvidence(domain.EvidenceDocument, "release-notes", "https://example.org/demo/releases/1.1.0",
		"## Changes", "The controller no longer updates widgets.", "sha256:notes", fixedNow)
	cand := domain.SemanticCandidate{Product: "demo", Release: "1.1.0", Grouping: "single", Category: domain.CategoryRemoval,
		Title: "The controller no longer updates widgets", Evidence: []domain.Evidence{up}, Producer: "test@v1", CreatedAt: fixedNow,
		Members: []domain.CandidateMember{{ChangeID: "chg-widgets", Computed: true}}}
	cand.ID = domain.CandidateID(cand.Product, cand.Release, cand.Members)
	if err := cand.Validate(); err != nil {
		t.Fatal(err)
	}
	statement := "the controller can no longer update widgets it reconciles"
	assertion := func() domain.SemanticAssertion {
		return domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectRBACPermission, Product: "demo", Group: "demo.example.org", Name: "widgets/update"},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindRemoved},
			Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpRenderedChange,
				Group: "rbac.authorization.k8s.io", Kind: "ClusterRole", Path: "rules", State: domain.StateChanged}},
			Consequence: &domain.Consequence{Kind: domain.ConsequencePermissionLost, ExposedClass: domain.ImpactActionRequired, Statement: statement},
		}
	}
	vs, err := NewValidator(&fixedPairs{p: goldenReleasePair(t)}).Validate(ctx, knowledge.ValidationInput{
		Candidate: cand, Assertion: assertion(), Now: fixedNow,
		Edge: &domain.UpgradeEdge{Product: domain.ProductRef{ID: "demo"}, From: domain.Version{Semver: "1.0.0"}, To: domain.Version{Semver: "1.1.0"}}})
	if err != nil || len(vs) != 1 {
		t.Fatalf("validate: %v %d", err, len(vs))
	}
	v := vs[0]
	if v.RenderRelation != domain.RenderConfirmed || !v.Confirms(domain.AspectSubject) || !v.Confirms(domain.AspectChange) {
		t.Fatalf("the render must confirm widgets/update removed: %+v", v.Checks)
	}
	for _, c := range v.Checks {
		if c.Aspect == domain.AspectConsequence || c.Aspect == domain.AspectApplicability {
			t.Fatalf("1. the render validator checked %s; it must judge only subject and change", c.Aspect)
		}
	}
	// routing derives the renderability from the render confirmation itself
	// (knowledge.RouteWith → domain.EffectiveRenderability); the candidate as
	// stored states none
	if cand.Renderability != "" || domain.EffectiveRenderability(cand, vs) != domain.RenderVerifiable {
		t.Fatalf("renderability: stored %q, effective %q", cand.Renderability, domain.EffectiveRenderability(cand, vs))
	}
	proposal := func(call string, class domain.ImpactClass) domain.SemanticProposal {
		at := fixedNow
		p := domain.SemanticProposal{CandidateID: cand.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: assertion(),
			SuggestedClass: class, Citations: []domain.EvidenceID{up.ID},
			Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "semantic.propose@v1", Confidence: domain.ConfidenceMedium,
				Model: "claude-opus-5-5", ModelVersion: "claude-opus-5-5", PromptVersion: "semantic/v1", PromptDigest: "sha256:p",
				InputEvidence: []domain.EvidenceID{up.ID}, GeneratedAt: &at, CallID: call}}
		p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
		if err := p.ValidateAgainst(cand); err != nil {
			t.Fatal(err)
		}
		return p
	}
	policy := knowledge.CombinePolicies(knowledge.AutoApproveRenderVerifiable, knowledge.AutoApproveConsensusAction)

	// 2. one model call asks for action: nothing verifies the consequence
	r := knowledge.RouteWith(policy, cand, []domain.SemanticProposal{proposal("call-1", domain.ImpactActionRequired)}, vs)
	if r.Fact != nil {
		t.Fatalf("2. a render delta plus one model call minted fact %s (level %s)", r.Fact.ID, r.Fact.Level())
	}
	if len(r.ReviewItems) == 0 {
		t.Fatal("2. the open consequence must go to review")
	}

	// 3. two separate calls agree on the consequence, neither requests action
	r = knowledge.RouteWith(policy, cand, []domain.SemanticProposal{
		proposal("call-1", domain.ImpactReviewRequired), proposal("call-2", domain.ImpactReviewRequired)}, vs)
	if r.Fact == nil {
		t.Fatalf("3. expected an auto-approved fact; items: %d", len(r.ReviewItems))
	}
	f := r.Fact
	if !f.AutoApproved || f.ConsensusAction || f.Level().Trusted() || f.AspectLevel(domain.AspectConsequence) != domain.VerifiedConsensus {
		t.Fatalf("3. fact must be auto-approved, consensus-level, without ConsensusAction: auto=%v action=%v level=%s",
			f.AutoApproved, f.ConsensusAction, f.Level())
	}
	if f.AspectLevel(domain.AspectSubject) != domain.VerifiedDeterministic || f.AspectLevel(domain.AspectChange) != domain.VerifiedDeterministic {
		t.Error("3. subject and change should rest on the render validation")
	}
	// the ladder: an untrusted fact without ConsensusAction can never back ACTION REQUIRED
	ref := domain.KnowledgeRef{Fact: f.ID, Verification: f.Level(), Consensus: domain.ConsensusSameModel, ConsensusAction: f.ConsensusAction}
	if ref.Verification.Trusted() || ref.ConsensusAction {
		t.Fatal("3. this fact would be eligible for ACTION REQUIRED")
	}

	// 4. PO-2: every agreeing call requested action ⇒ consensus action, labelled and audited
	r = knowledge.RouteWith(policy, cand, []domain.SemanticProposal{
		proposal("call-1", domain.ImpactActionRequired), proposal("call-2", domain.ImpactActionRequired)}, vs)
	if r.Fact == nil || !r.Fact.ConsensusAction || !r.Fact.AutoApproved {
		t.Fatalf("4. expected a consensus-action fact: %+v", r.Fact)
	}
	if (domain.KnowledgeRef{Verification: r.Fact.Level(), ConsensusAction: true}).ActionLabel() != "model consensus" {
		t.Error("4. consensus ACTION must be labelled model consensus")
	}
}

// The review context carries the validator's release-level render evidence
// with before/after values (R18) — what the dashboard's Rendered delta panel
// shows.
func TestRenderEvidenceReachesReviewContext(t *testing.T) {
	up := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.org/n", "L1", "widgets update removed", "sha256:n", fixedNow)
	cand := domain.SemanticCandidate{Product: "demo", Release: "1.1.0", Grouping: "single", Category: domain.CategoryRemoval,
		Title: "t", Evidence: []domain.Evidence{up}, Producer: "test@v1", CreatedAt: fixedNow,
		Members: []domain.CandidateMember{{ChangeID: "chg-w", Computed: true}}}
	cand.ID = domain.CandidateID(cand.Product, cand.Release, cand.Members)
	vs, err := NewValidator(&fixedPairs{p: goldenReleasePair(t)}).Validate(context.Background(), knowledge.ValidationInput{
		Candidate: cand, Now: fixedNow,
		Edge: &domain.UpgradeEdge{Product: domain.ProductRef{ID: "demo"}, From: domain.Version{Semver: "1.0.0"}, To: domain.Version{Semver: "1.1.0"}},
		Assertion: domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectRBACPermission, Product: "demo", Group: "demo.example.org", Name: "widgets/update"},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindRemoved}}})
	if err != nil {
		t.Fatal(err)
	}
	re := knowledge.RenderEvidenceOf(cand.ID, vs)
	if re == nil || re.Relation != domain.RenderConfirmed {
		t.Fatalf("render evidence: %+v", re)
	}
	found := false
	for _, e := range re.Evidence {
		if e.Render.Change == string(RBACPermissionRemoved) && e.Render.Before == `"demo.example.org/widgets:update"` && e.Render.After == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("no rbac-permission-removed record with its before value among %d records", len(re.Evidence))
	}
}
