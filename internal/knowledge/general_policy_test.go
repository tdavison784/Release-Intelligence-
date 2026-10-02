package knowledge

import (
	"context"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

var general = CombinePolicies(DefaultAutoApprove, AutoApproveGeneral)

// twoCalls stores two separate calls of one model asserting a (class: the
// suggested class of both).
func twoCalls(t *testing.T, s Store, c domain.SemanticCandidate, a domain.SemanticAssertion, class domain.ImpactClass) {
	t.Helper()
	base := proposal(c, "anthropic", "claude-sonnet-5-5", a)
	mustPut(t, s, call(base, "call-1", class))
	mustPut(t, s, call(base, "call-2", class))
}

func routeGeneral(t *testing.T, s Store, every int) *RouteSummary {
	t.Helper()
	sum, err := RouteStore(context.Background(), s, RouteOptions{Policy: general, AuditEvery: every, Now: func() time.Time { return t0.Add(time.Hour) }}, Query{})
	if err != nil {
		t.Fatal(err)
	}
	return sum
}

func TestGeneralPolicyMintsAtConsensusWithADeterministicSubjectOrChange(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	twoCalls(t, s, c, a, "")
	mustPut(t, s, validation(c, a, domain.AspectSubject, domain.AspectChange))
	sum := routeGeneral(t, s, 1)
	if sum.Facts != 1 || len(sum.Audits) != 1 {
		t.Fatalf("summary = %+v", sum)
	}
	ctx := context.Background()
	snap, _ := s.Load(ctx, Query{})
	f := snap.Facts[0]
	if !f.AutoApproved || f.Level() != domain.VerifiedConsensus || f.ConsensusAction ||
		f.AspectLevel(domain.AspectSubject) != domain.VerifiedDeterministic || f.AspectLevel(domain.AspectApplicability) != domain.VerifiedConsensus {
		t.Fatalf("fact = %+v", f.Verification)
	}
	// the audit item records the policy decision for later testing against human outcomes
	sig := map[domain.RoutingSignal]bool{}
	for _, x := range snap.ReviewItems[0].Routing.Signals {
		sig[x] = true
	}
	for _, want := range []domain.RoutingSignal{domain.SignalPolicyGeneral, domain.SignalAutoApproved, domain.SignalConsensusSameModel} {
		if !sig[want] {
			t.Errorf("audit item lacks signal %s: %v", want, snap.ReviewItems[0].Routing.Signals)
		}
	}
	if sig[domain.SignalConsensusAction] || snap.ReviewItems[0].Routing.Priority == domain.PriorityHigh {
		t.Errorf("a plain consensus fact must not be consensus-action: %v", snap.ReviewItems[0].Routing)
	}
}

func TestGeneralPolicyIsOptIn(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	twoCalls(t, s, c, a, "")
	mustPut(t, s, validation(c, a, domain.AspectSubject, domain.AspectChange))
	sum, err := RouteStore(context.Background(), s, RouteOptions{Policy: DefaultAutoApprove}, Query{})
	if err != nil || sum.Facts != 0 {
		t.Fatalf("default policies approved a general-policy candidate: %+v, %v", sum, err)
	}
}

func TestGeneralPolicyRefusals(t *testing.T) {
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	cases := []struct {
		name  string
		setup func(t *testing.T, s Store, c domain.SemanticCandidate)
	}{
		{"zero aspects deterministically confirmed among subject/change", func(t *testing.T, s Store, c domain.SemanticCandidate) {
			twoCalls(t, s, c, a, "")
			mustPut(t, s, validation(c, a, domain.AspectApplicability)) // not subject/change
		}},
		{"no validation at all", func(t *testing.T, s Store, c domain.SemanticCandidate) { twoCalls(t, s, c, a, "") }},
		{"a validator refuted an aspect", func(t *testing.T, s Store, c domain.SemanticCandidate) {
			twoCalls(t, s, c, a, "")
			mustPut(t, s, validation(c, a, domain.AspectSubject, domain.AspectChange))
			v := validation(c, a, domain.AspectApplicability)
			v.Validator = "semvalidate.other@v1"
			v.Checks[0].Outcome = domain.OutcomeRefuted
			v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
			mustPut(t, s, v)
		}},
		{"an aspect is asserted by one call only", func(t *testing.T, s Store, c domain.SemanticCandidate) {
			mustPut(t, s, call(proposal(c, "anthropic", "claude-sonnet-5-5", a), "call-1", ""))
			mustPut(t, s, validation(c, a, domain.AspectSubject, domain.AspectChange))
		}},
		{"the calls disagree on an aspect", func(t *testing.T, s Store, c domain.SemanticCandidate) {
			mustPut(t, s, call(proposal(c, "anthropic", "claude-sonnet-5-5", a), "call-1", ""))
			mustPut(t, s, call(proposal(c, "zai", "glm-5.3-flash", rotationAssertion(domain.ConsequenceDeprecation)), "call-2", ""))
			mustPut(t, s, validation(c, a, domain.AspectSubject, domain.AspectChange))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewFileStore(t.TempDir())
			c := fixtureCandidate()
			mustPut(t, s, c)
			tc.setup(t, s, c)
			if sum := routeGeneral(t, s, 1); sum.Facts != 0 {
				t.Fatalf("minted a fact: %+v", sum)
			}
		})
	}
}

func TestGeneralPolicyConsensusActionIsAlwaysAuditedAndHighPriority(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceSettingIgnored)
	twoCalls(t, s, c, a, domain.ImpactActionRequired)
	mustPut(t, s, validation(c, a, domain.AspectSubject, domain.AspectChange))
	sum := routeGeneral(t, s, 0) // sampling off: consensus-action is audited anyway
	snap, _ := s.Load(context.Background(), Query{})
	if sum.Facts != 1 || len(sum.Audits) != 1 || !snap.Facts[0].ConsensusAction {
		t.Fatalf("summary %+v, facts %+v", sum, snap.Facts)
	}
	it := snap.ReviewItems[0]
	if it.Routing.Priority != domain.PriorityHigh || !hasSignal(it, domain.SignalConsensusAction) {
		t.Fatalf("audit routing = %+v", it.Routing)
	}
	// a consensus on an action-eligible consequence that did NOT all request action is not consensus-action
	s2 := NewFileStore(t.TempDir())
	mustPut(t, s2, c)
	mustPut(t, s2, call(proposal(c, "anthropic", "claude-sonnet-5-5", a), "call-1", domain.ImpactReviewRequired))
	mustPut(t, s2, call(proposal(c, "anthropic", "claude-sonnet-5-5", a), "call-2", domain.ImpactReviewRequired))
	mustPut(t, s2, validation(c, a, domain.AspectSubject, domain.AspectChange))
	snap2 := func() *Snapshot { routeGeneral(t, s2, 0); x, _ := s2.Load(context.Background(), Query{}); return x }()
	// not consensus-action (one call only asked for review) — but the consequence is still
	// action-eligible, so the auto-approved fact is audited (AuditRequired), at normal priority
	if len(snap2.Facts) != 1 || snap2.Facts[0].ConsensusAction || len(snap2.ReviewItems) != 1 ||
		snap2.ReviewItems[0].Routing.Priority == domain.PriorityHigh || hasSignal(snap2.ReviewItems[0], domain.SignalConsensusAction) {
		t.Fatalf("review-only consensus: facts %+v items %+v", snap2.Facts, snap2.ReviewItems)
	}
}

func TestProxyDecisionsNeverMakeAutoApprovedFacts(t *testing.T) {
	s, q, c, item := seeded(t) // subject/change/applicability validator-confirmed; consequence item open
	ctx := context.Background()
	if _, err := q.Decide(ctx, []domain.ReviewDecision{decision(item, "claude", domain.ReviewerProxy, domain.ActionAccept, t0.Add(time.Hour))}); err != nil {
		t.Fatal(err)
	}
	routeGeneral(t, s, 1)
	snap, _ := s.Load(ctx, Query{})
	if len(snap.Facts) != 1 {
		t.Fatalf("facts = %d", len(snap.Facts))
	}
	f := snap.Facts[0]
	if f.AutoApproved || f.Level() != domain.VerifiedProxy || f.ConsensusAction {
		t.Fatalf("proxy-decided fact: auto %v level %s consensusAction %v", f.AutoApproved, f.Level(), f.ConsensusAction)
	}
	_ = c
}

func TestDeriveRenderability(t *testing.T) {
	c := fixtureCandidate()
	cases := []struct {
		name string
		a    domain.SemanticAssertion
		want domain.Renderability
	}{
		{"rbac added is a rendered permission", domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectRBACPermission, Product: "cert-manager", Name: "events/create"},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindAdded}}, domain.RenderVerifiable},
		{"a helm default is only partly visible", rotationAssertion(domain.ConsequenceBehaviorChange), domain.RenderPartiallyVerifiable},
		{"protocol behaviour is not renderable", domain.SemanticAssertion{
			Subject: &domain.Subject{Family: domain.SubjectProtocolBehavior, Product: "cert-manager", Name: "grpc-alpn"},
			Change:  &domain.ChangeSpec{Type: domain.ChangeKindBehaviorChanged}}, domain.RenderNotVerifiable},
	}
	for _, tc := range cases {
		a := domain.SemanticAssertion{Subject: tc.a.Subject, Change: tc.a.Change}
		// a single model's claim alone assesses nothing
		one := []domain.SemanticProposal{call(proposal(c, "zai", "glm", tc.a), "c1", "")}
		if got := DeriveRenderability(c, one, nil); got != "" {
			t.Errorf("%s: one call derived %q", tc.name, got)
		}
		// two separate calls agreeing on subject+change
		two := []domain.SemanticProposal{call(proposal(c, "zai", "glm", tc.a), "c1", ""), call(proposal(c, "anthropic", "sonnet", tc.a), "c2", "")}
		if got := DeriveRenderability(c, two, nil); got != tc.want {
			t.Errorf("%s: consensus derived %q, want %q", tc.name, got, tc.want)
		}
		// a validator confirming subject+change
		v := validation(c, a, domain.AspectSubject, domain.AspectChange)
		if got := DeriveRenderability(c, nil, []domain.ValidationResult{v}); got != tc.want {
			t.Errorf("%s: validated derived %q, want %q", tc.name, got, tc.want)
		}
	}
	// the candidate's own assessment wins
	own := c
	own.Renderability = domain.RenderNotVerifiable
	if got := DeriveRenderability(own, nil, nil); got != domain.RenderNotVerifiable {
		t.Errorf("own = %q", got)
	}
}

func TestAutoApprovalSupersedesTheItemsRoutedBeforeIt(t *testing.T) {
	s := NewFileStore(t.TempDir())
	c := fixtureCandidate()
	mustPut(t, s, c)
	a := rotationAssertion(domain.ConsequenceBehaviorChange)
	twoCalls(t, s, c, a, "")
	mustPut(t, s, validation(c, a, domain.AspectSubject, domain.AspectChange))
	ctx := context.Background()
	// first pass without the policy: review items
	if sum, err := RouteStore(ctx, s, RouteOptions{}, Query{}); err != nil || sum.Items == 0 {
		t.Fatalf("first pass: %+v, %v", sum, err)
	}
	sum := routeGeneral(t, s, 0)
	if sum.Facts != 1 || sum.Superseded == 0 {
		t.Fatalf("second pass: %+v", sum)
	}
	snap, _ := s.Load(ctx, Query{})
	for _, it := range snap.ReviewItems {
		if it.Status == domain.ReviewPending && it.QuestionType != domain.QuestionClassification {
			t.Fatalf("a moot item is still pending: %s %s", it.QuestionType, it.Status)
		}
	}
}
