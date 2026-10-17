package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Promotion-gate fixtures: a synthetic product ("acme-ctl", a Widget CRD whose
// spec.retention.mode default changes Keep → Purge), never an evaluation case.

const gateProduct = "acme-ctl"

func widgets(of ...domain.Condition) domain.Condition {
	return domain.Condition{Op: domain.OpResource, Group: "acme.example", Kind: "Widget", Of: of}
}

func retentionField(state domain.FieldState, values ...string) domain.Condition {
	return domain.Condition{Op: domain.OpField, Path: "spec.retention.mode", State: state, Values: values}
}

// broadRule is the release-level fact: every Widget without the field is exposed.
func broadRule() domain.SemanticAssertion {
	return domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectCRDField, Product: gateProduct, Group: "acme.example", Kind: "Widget", Path: "spec.retention.mode"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: str(`"Keep"`), After: str(`"Purge"`)},
		Applicability: &domain.Applicability{Exposure: widgets(retentionField(domain.StateUnset))},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired, Statement: "data is purged on rollover"},
		Statement:     "Widget.spec.retention.mode default Keep → Purge",
	}
}

// narrowRule is the agent's mitigation: an explicit Keep shields the Widget.
func narrowRule() domain.SemanticAssertion {
	a := broadRule()
	a.Applicability = &domain.Applicability{
		Exposure: widgets(retentionField(domain.StateUnset)),
		Overlap:  ptrCond(widgets(retentionField(domain.StateEquals, `"Keep"`))),
	}
	return a
}

func ptrCond(c domain.Condition) *domain.Condition { return &c }

func gateEvidence() domain.Evidence {
	return domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example.test/acme/releases/v2.0.0/notes.md", "L3",
		"The default retention mode is now Purge", "sha256:acme-notes", t0)
}

func gateCandidate() domain.SemanticCandidate {
	e := gateEvidence()
	a := domain.ChangeAnchor{Release: "2.0.0", EvidenceKeys: []string{domain.EvidenceKey(e)}, StatementKeys: []string{domain.StatementKey(e)}, ChangeIDs: []string{"chg-ret"}}
	members := []domain.CandidateMember{{ChangeID: "chg-ret", Anchor: &a}}
	return domain.SemanticCandidate{ID: domain.CandidateID(gateProduct, "2.0.0", members), Product: gateProduct, Release: "2.0.0",
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration, Title: "retention default is now Purge",
		Evidence: []domain.Evidence{e}, Producer: "semantic.candidates@v1", CreatedAt: t0}
}

func gateProv(rule string) domain.Provenance {
	at := t0.Add(time.Minute)
	return domain.Provenance{Method: domain.MethodAI, Producer: "investigate.agent@v1", Rule: rule, Confidence: domain.ConfidenceMedium,
		Model: "claude-opus-5-5", ModelVersion: "claude-opus-5-5", PromptVersion: "investigate/v6", PromptDigest: "sha256:p",
		InputEvidence: []domain.EvidenceID{gateEvidence().ID}, GeneratedAt: &at, CallID: "call-" + rule, Provider: "anthropic"}
}

type gateFixture struct {
	s     Store
	q     Queue
	c     domain.SemanticCandidate
	broad domain.VerifiedFact
	item  domain.ReviewItem
	hyp   domain.SemanticProposal
}

// newGateFixture stores the candidate, a validator-confirmed broad fact, the
// agent's hypothesis proposal and its whole-fact review item.
func newGateFixture(t *testing.T) *gateFixture {
	t.Helper()
	f := &gateFixture{s: NewFileStore(t.TempDir()), c: gateCandidate()}
	f.q = NewQueue(f.s, func() time.Time { return t0.Add(time.Hour) })
	mustPut(t, f.s, f.c)
	v := domain.ValidationResult{CandidateID: f.c.ID, Validator: "semvalidate.crd@v1", Assertion: broadRule(),
		Evidence: []domain.Evidence{gateEvidence()}, CheckedAt: t0.Add(2 * time.Minute)}
	for _, x := range []domain.Aspect{domain.AspectSubject, domain.AspectChange, domain.AspectApplicability} {
		v.Checks = append(v.Checks, domain.AspectCheck{Aspect: x, Outcome: domain.OutcomeConfirmed, Rule: "test:" + string(x)})
	}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	mustPut(t, f.s, v)
	semantic := domain.SemanticProposal{CandidateID: f.c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: broadRule(),
		Citations: []domain.EvidenceID{gateEvidence().ID}, Provenance: gateProv("")}
	semantic.Provenance.Producer = "semantic.propose@v1"
	semantic.ID = domain.ProposalID(semantic.CandidateID, semantic.Task, semantic.Provider, semantic.Provenance)
	mustPut(t, f.s, semantic)
	res := Route(f.c, []domain.SemanticProposal{semantic}, []domain.ValidationResult{v})
	if len(res.ReviewItems) != 1 {
		t.Fatalf("route: %+v", res)
	}
	mustPut(t, f.s, res.ReviewItems[0])
	out, err := f.q.Decide(context.Background(), []domain.ReviewDecision{humanAccept(res.ReviewItems[0], t0.Add(30*time.Minute))})
	if err != nil || out[0].Fact == nil {
		t.Fatalf("broad fact not minted: %v %+v", err, out)
	}
	f.broad = *out[0].Fact

	f.hyp = domain.SemanticProposal{CandidateID: f.c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: narrowRule(),
		Citations: []domain.EvidenceID{gateEvidence().ID}, Provenance: gateProv("hypothesis:hyp-0123456789ab@inv-0123456789ab")}
	f.hyp.ID = domain.ProposalID(f.hyp.CandidateID, f.hyp.Task, f.hyp.Provider, f.hyp.Provenance)
	mustPut(t, f.s, f.hyp)
	f.item = domain.ReviewItem{CandidateID: f.c.ID, Product: gateProduct, Release: "2.0.0", QuestionType: domain.QuestionFactReview,
		Question: "Is this agent-proposed rule correct as a whole?", Proposed: narrowRule(), Proposals: []string{f.hyp.ID},
		Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityHigh}, Status: domain.ReviewPending, CreatedAt: t0.Add(3 * time.Minute)}
	f.item.ID = domain.ReviewItemID(f.item.CandidateID, f.item.QuestionType, f.item.Proposed)
	mustPut(t, f.s, f.item)
	return f
}

// proofFor builds a valid proof of the item's rule with the given method outcomes.
func proofFor(it domain.ReviewItem, at time.Time, citation, exercise domain.MethodOutcome, bounds *domain.ProofBounds) domain.ProofRecord {
	ev := gateEvidence()
	cm := domain.ProofMethod{Method: domain.ProofCitationCheck, Producer: "verify.citations@v1", Level: domain.LevelDeterministic,
		Outcome: citation, Detail: "citation check", Evidence: []domain.Evidence{ev}}
	em := domain.ProofMethod{Method: domain.ProofRuleExercise, Producer: "verify.exercise@v1", Level: domain.LevelObserved,
		Outcome: exercise, Detail: "two-arm exercise"}
	if exercise == domain.MethodPass {
		em.Evidence = []domain.Evidence{ev}
		em.Arms = []domain.ProofArm{
			{Name: domain.ArmConditionPresent, ReactedFrom: true, ReactedTo: true, Predicted: domain.BehaviourPrevious, Observed: domain.BehaviourPrevious},
			{Name: domain.ArmConditionAbsent, ReactedFrom: true, ReactedTo: true, Changed: true, Predicted: domain.BehaviourNew, Observed: domain.BehaviourNew},
		}
	}
	p := domain.ProofRecord{Hypothesis: "hyp-0123456789ab", Investigation: "inv-0123456789ab", ReviewItem: it.ID, CandidateID: it.CandidateID,
		Product: it.Product, Release: it.Release, ProposedRuleDigest: it.Proposed.Digest(), Methods: []domain.ProofMethod{cm, em},
		VersionBounds: bounds, Conditions: it.Proposed.Applicability, Limitations: []string{"one synthetic object per arm"},
		Producer: "verify.hypothesis@v1", CheckedAt: at}
	p.Outcome = domain.ProofOutcomeOf(p.Methods)
	p.Level = domain.LevelDeterministic
	if exercise != domain.MethodNotRun {
		p.Level = domain.LevelObserved
	}
	p.ID = domain.ProofRecordID(p)
	return p
}

var gateBounds = &domain.ProofBounds{From: "1.9.4", To: "2.0.0"}

func humanAccept(it domain.ReviewItem, at time.Time) domain.ReviewDecision {
	return decision(it, "product-owner", domain.ReviewerHuman, domain.ActionAccept, at)
}

func (f *gateFixture) decide(t *testing.T, d domain.ReviewDecision) ([]DecisionOutcome, error) {
	t.Helper()
	return f.q.Decide(context.Background(), []domain.ReviewDecision{d})
}

func refused(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("decision accepted; want refusal mentioning %q", want)
	}
	if !errors.Is(err, ErrPromotionRefused) && !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v; want a promotion-gate refusal", err)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %v; want it to mention %q", err, want)
	}
}

// nothingWritten asserts a refused decision left no decision and no new fact.
func (f *gateFixture) nothingWritten(t *testing.T) {
	t.Helper()
	snap, err := f.s.Load(context.Background(), Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Decisions) != 1 {
		t.Fatalf("a refused decision was stored: %+v", snap.Decisions)
	}
	if len(snap.Facts) != 1 || snap.Facts[0].ID != f.broad.ID || snap.Facts[0].Status != domain.FactActive {
		t.Fatalf("facts changed: %+v", snap.Facts)
	}
}

func TestGateRefusesWithoutProof(t *testing.T) {
	f := newGateFixture(t)
	d := humanAccept(f.item, t0.Add(2*time.Hour))
	d.Supersedes = []string{f.broad.ID}
	_, err := f.decide(t, d)
	refused(t, err, "no proof record")
	f.nothingWritten(t)
}

func TestGateRefusesInconclusiveProof(t *testing.T) {
	for _, tc := range []struct {
		name               string
		citation, exercise domain.MethodOutcome
	}{
		{"exercise not run", domain.MethodPass, domain.MethodNotRun},
		{"exercise inconclusive", domain.MethodPass, domain.MethodInconclusive},
		{"citation not run", domain.MethodNotRun, domain.MethodPass},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGateFixture(t)
			mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), tc.citation, tc.exercise, gateBounds))
			d := humanAccept(f.item, t0.Add(2*time.Hour))
			d.Supersedes = []string{f.broad.ID}
			_, err := f.decide(t, d)
			refused(t, err, "is inconclusive")
			f.nothingWritten(t)
		})
	}
}

func TestGateRefusesRefutedProofEvenWithALaterConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name               string
		citation, exercise domain.MethodOutcome
	}{
		{"citation outside bounds", domain.MethodFail, domain.MethodPass},
		{"condition arm changed", domain.MethodPass, domain.MethodFail},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGateFixture(t)
			mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), tc.citation, tc.exercise, gateBounds))
			mustPut(t, f.s, proofFor(f.item, t0.Add(90*time.Minute), domain.MethodPass, domain.MethodPass, gateBounds))
			d := humanAccept(f.item, t0.Add(2*time.Hour))
			d.Supersedes = []string{f.broad.ID}
			_, err := f.decide(t, d)
			refused(t, err, "refuted")
			f.nothingWritten(t)
		})
	}
}

func TestGateRefusesProxyAndPartialReviews(t *testing.T) {
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds))
	proxy := decision(f.item, "proxy-reviewer", domain.ReviewerProxy, domain.ActionAccept, t0.Add(2*time.Hour))
	proxy.Supersedes = []string{f.broad.ID}
	_, err := f.decide(t, proxy)
	refused(t, err, "only a human")
	partial := humanAccept(f.item, t0.Add(2*time.Hour))
	partial.Aspects = []domain.Aspect{domain.AspectSubject, domain.AspectChange, domain.AspectConsequence}
	partial.Supersedes = []string{f.broad.ID}
	_, err = f.decide(t, partial)
	refused(t, err, "whole rule")
	f.nothingWritten(t)
}

func TestGateRefusesProofDigestMismatchAfterCorrection(t *testing.T) {
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds))
	// the reviewer widens the mitigation to "any explicit value": a different rule
	changed := narrowRule()
	changed.Applicability.Overlap = ptrCond(widgets(retentionField(domain.StateSet)))
	d := humanAccept(f.item, t0.Add(2*time.Hour))
	d.Action, d.Labels, d.Corrected = domain.ActionCorrect, []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelWrongApplicability}, &changed
	d.Reason = "any explicit value shields the Widget"
	d.Supersedes = []string{f.broad.ID}
	_, err := f.decide(t, d)
	refused(t, err, "needs its own proof")
	f.nothingWritten(t)

	// a proof of the corrected rule cannot even be stored on this item
	p := proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds)
	p.ProposedRuleDigest = changed.Digest()
	p.ID = domain.ProofRecordID(p)
	rec, _ := domain.NewRecord(p)
	if err := f.s.Put(context.Background(), rec); err == nil || !strings.Contains(err.Error(), "proposes") {
		t.Fatalf("storing a proof of another rule: err = %v", err)
	}
}

func TestGateRefusesBoundsThatDoNotCoverTheRelease(t *testing.T) {
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, &domain.ProofBounds{From: "2.0.0", To: "2.1.0"}))
	d := humanAccept(f.item, t0.Add(2*time.Hour))
	d.Supersedes = []string{f.broad.ID}
	_, err := f.decide(t, d)
	refused(t, err, "outside the proof's bounds")
	f.nothingWritten(t)
}

func TestGateRefusesSilentCoexistence(t *testing.T) {
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds))
	_, err := f.decide(t, humanAccept(f.item, t0.Add(2*time.Hour)))
	refused(t, err, "never silently coexists")
	f.nothingWritten(t)
}

func TestGateRefusesNamingAFactOfAnotherSubject(t *testing.T) {
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds))
	d := humanAccept(f.item, t0.Add(2*time.Hour))
	d.Supersedes = []string{"vf-000000000000"}
	_, err := f.decide(t, d)
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("err = %v", err)
	}
	f.nothingWritten(t)
}

func TestGatePromotesWithConfirmedProofAndSupersedes(t *testing.T) {
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds))
	d := humanAccept(f.item, t0.Add(2*time.Hour))
	d.Supersedes = []string{f.broad.ID}
	out, err := f.decide(t, d)
	if err != nil {
		t.Fatal(err)
	}
	nf := out[0].Fact
	if nf == nil || nf.Assertion.Digest() != narrowRule().Digest() || nf.Level() != domain.VerifiedHuman {
		t.Fatalf("promoted fact = %+v", nf)
	}
	snap, _ := f.s.Load(context.Background(), Query{})
	status := map[string]domain.FactStatus{}
	for _, x := range snap.Facts {
		status[x.ID] = x.Status
	}
	if status[f.broad.ID] != domain.FactSuperseded || status[nf.ID] != domain.FactActive {
		t.Fatalf("statuses = %v", status)
	}
	stored, _ := f.s.Get(context.Background(), nf.ID)
	if len(stored.Fact.Supersedes) != 1 || stored.Fact.Supersedes[0] != f.broad.ID {
		t.Fatalf("supersedes = %v", stored.Fact.Supersedes)
	}
}

func TestGatePromotesProseOnlyCorrectionAndExplicitCoexistence(t *testing.T) {
	f := newGateFixture(t)
	mustPut(t, f.s, proofFor(f.item, t0.Add(time.Hour), domain.MethodPass, domain.MethodPass, gateBounds))
	prose := narrowRule()
	prose.Consequence.Statement = "data is purged on rollover unless the Widget keeps it explicitly"
	d := humanAccept(f.item, t0.Add(2*time.Hour))
	d.Action, d.Labels, d.Corrected = domain.ActionCorrect, []domain.FeedbackLabel{domain.LabelCorrected, domain.LabelImprovedStatement}, &prose
	d.Reason = "clearer statement"
	d.Coexists = []string{f.broad.ID}
	out, err := f.decide(t, d)
	if err != nil {
		t.Fatal(err)
	}
	if out[0].Fact == nil {
		t.Fatal("no fact minted")
	}
	snap, _ := f.s.Load(context.Background(), Query{})
	active := 0
	for _, x := range snap.Facts {
		if x.Status == domain.FactActive {
			active++
		}
	}
	if active != 2 {
		t.Fatalf("want broad and narrow facts both active (explicit coexistence), got %d", active)
	}
}

func TestHypothesisSourceMatchesUnlistedProposalByRule(t *testing.T) {
	f := newGateFixture(t)
	it := f.item
	it.Proposals = nil
	snap, _ := f.s.Load(context.Background(), Query{})
	if hyp, inv, ok := HypothesisSource(snap, it); !ok || hyp != "hyp-0123456789ab" || inv != "inv-0123456789ab" {
		t.Fatalf("HypothesisSource = %q %q %v", hyp, inv, ok)
	}
	for _, rule := range []string{"", "provider:anthropic", "hypothesis:", "hypothesis:x@inv-1", "hypothesis:hyp-1@nope"} {
		if _, _, ok := ParseHypothesisRule(rule); ok {
			t.Errorf("ParseHypothesisRule(%q) accepted", rule)
		}
	}
}

func TestGateIgnoresOrdinaryItems(t *testing.T) {
	_, q, _, item := seeded(t)
	d := decision(item, "product-owner", domain.ReviewerHuman, domain.ActionAccept, t0.Add(2*time.Hour))
	if _, err := q.Decide(context.Background(), []domain.ReviewDecision{d}); err != nil {
		t.Fatalf("ordinary item refused: %v", err)
	}
}

// Agent-proposed rules never reach a fact through routing: two separate
// agent calls agreeing on the rule, with every other aspect validator
// confirmed, mint nothing and open no item (the exported whole-fact item and
// the gate are the only path).
func TestRoutingIgnoresHypothesisProposals(t *testing.T) {
	c := gateCandidate()
	v := domain.ValidationResult{CandidateID: c.ID, Validator: "semvalidate.crd@v1", Assertion: narrowRule(),
		Evidence: []domain.Evidence{gateEvidence()}, CheckedAt: t0.Add(2 * time.Minute)}
	for _, x := range []domain.Aspect{domain.AspectSubject, domain.AspectChange, domain.AspectApplicability} {
		v.Checks = append(v.Checks, domain.AspectCheck{Aspect: x, Outcome: domain.OutcomeConfirmed, Rule: "test:" + string(x)})
	}
	v.ID = domain.ValidationID(v.CandidateID, v.Validator, v.Assertion)
	var ps []domain.SemanticProposal
	for _, call := range []string{"hypothesis:hyp-0123456789ab@inv-0123456789ab", "hypothesis:hyp-ba9876543210@inv-ba9876543210"} {
		p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: narrowRule(),
			Citations: []domain.EvidenceID{gateEvidence().ID}, Provenance: gateProv(call)}
		p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
		ps = append(ps, p)
	}
	for _, policy := range []AutoApprovePolicy{nil, DefaultAutoApprove} {
		res := RouteWith(policy, c, ps, []domain.ValidationResult{v})
		if res.Fact != nil {
			t.Fatalf("a fact was minted from agent hypotheses: %+v", res.Fact)
		}
		for _, it := range res.ReviewItems {
			for _, id := range it.Proposals {
				if id == ps[0].ID || id == ps[1].ID {
					t.Fatalf("routing opened an item on an agent hypothesis: %+v", it)
				}
			}
		}
	}
}
