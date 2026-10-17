package verify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/runtime"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

var t0 = time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)

const (
	hypID = "hyp-0123456789ab"
	invID = "inv-0123456789ab"
)

type world struct {
	ks    knowledge.Store
	runs  string
	item  domain.ReviewItem
	res   mapResolver
	ev    domain.Evidence
	broad string
}

// newWorld stores what lane verdict's export writes: the candidate (with the
// hypothesis' upstream citation), the AI proposal with the hypothesis rule,
// the whole-fact review item; and the run record with the mitigation spec.
func newWorld(t *testing.T, spec domain.SemanticAssertion, specBounds *domain.ProofBounds) *world {
	t.Helper()
	w := &world{ks: knowledge.NewFileStore(t.TempDir()), runs: t.TempDir()}
	w.ev = cite("https://example.test/acme/blob/main/docs/release-notes-2.0.md", "The default value of spec.retention.mode changed from Keep to Purge.", notes200)
	w.res = mapResolver{w.ev.ContentDigest: notes200}
	a := domain.ChangeAnchor{Release: "2.0.0", EvidenceKeys: []string{domain.EvidenceKey(w.ev)}, StatementKeys: []string{domain.StatementKey(w.ev)}, ChangeIDs: []string{"chg-ret"}}
	members := []domain.CandidateMember{{ChangeID: "chg-ret", Anchor: &a}}
	c := domain.SemanticCandidate{ID: domain.CandidateID("acme-ctl", "2.0.0", members), Product: "acme-ctl", Release: "2.0.0", Members: members,
		Grouping: "single", Category: domain.CategoryConfiguration, Title: "retention default", Evidence: []domain.Evidence{w.ev},
		Producer: "semantic.candidates@v1", CreatedAt: t0}
	put(t, w.ks, c)
	at := t0.Add(time.Minute)
	p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "anthropic", Assertion: widgetRule(), Citations: []domain.EvidenceID{w.ev.ID},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "investigate.agent@v1", Rule: "hypothesis:" + hypID + "@" + invID, Confidence: domain.ConfidenceMedium,
			Model: "claude-opus-5-5", ModelVersion: "claude-opus-5-5", PromptVersion: "investigate/v6", PromptDigest: "sha256:p", InputEvidence: []domain.EvidenceID{w.ev.ID},
			GeneratedAt: &at, CallID: "call-1", Provider: "anthropic"}}
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	put(t, w.ks, p)
	w.item = domain.ReviewItem{CandidateID: c.ID, Product: "acme-ctl", Release: "2.0.0", QuestionType: domain.QuestionFactReview, Question: "whole rule?",
		Proposed: widgetRule(), Proposals: []string{p.ID}, Routing: domain.Routing{Route: domain.RouteReview, Priority: domain.PriorityHigh},
		Status: domain.ReviewPending, CreatedAt: t0.Add(2 * time.Minute)}
	w.item.ID = domain.ReviewItemID(w.item.CandidateID, w.item.QuestionType, w.item.Proposed)
	put(t, w.ks, w.item)
	run := map[string]any{"id": invID, "hypotheses": []any{map[string]any{"id": hypID, "mitigation": map[string]any{
		"item": "ci-1", "proposedRule": map[string]any{"assertion": spec, "release": specBounds}}}}}
	b, _ := json.Marshal(run)
	if err := os.WriteFile(filepath.Join(w.runs, invID+".json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	return w
}

func put(t *testing.T, s knowledge.Store, e any) {
	t.Helper()
	rec, err := domain.NewRecord(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), rec); err != nil {
		t.Fatal(err)
	}
}

func (w *world) options(t *testing.T, runtimeOn bool, run *runtime.FixtureResult) Options {
	return Options{Knowledge: w.ks, Resolver: w.res, Runs: StateRuns{Dir: w.runs}, Runtime: runtimeOn,
		Fixtures: map[string]*FixtureSet{"acme-ctl": acmeSet(t)}, Exercises: []runtime.DeclaredExercise{rolloverExercise(true)},
		Relationships: func(string) []catalog.Relationship { return acmeProduces },
		Renderer:      &fakeRenderer{}, Runner: &fakeRunner{res: run}, Now: func() time.Time { return t0.Add(time.Hour) }}
}

func (w *world) accept(t *testing.T, at time.Time) error {
	t.Helper()
	orig := w.item.Proposed
	d := domain.ReviewDecision{ReviewItemID: w.item.ID, Action: domain.ActionAccept, Labels: []domain.FeedbackLabel{domain.LabelAccepted}, Original: &orig,
		Reviewer: "product-owner", ReviewerKind: domain.ReviewerHuman, StartedAt: at.Add(-time.Minute), DecidedAt: at}
	d.ID = domain.DecisionID(d.ReviewItemID, d.Reviewer, d.DecidedAt)
	_, err := knowledge.NewQueue(w.ks, nil).Decide(context.Background(), []domain.ReviewDecision{d})
	return err
}

func TestHypothesesConfirmedProofOpensTheGate(t *testing.T) {
	w := newWorld(t, widgetRule(), bounds)
	if err := w.accept(t, t0.Add(30*time.Minute)); err == nil {
		t.Fatal("promotion before any proof was accepted")
	}
	rs, err := Hypotheses(context.Background(), w.options(t, true, fixtureResult(false, true)))
	if err != nil {
		t.Fatal(err)
	}
	if len(rs) != 1 || rs[0].Proof == nil || rs[0].Proof.Outcome != domain.ProofConfirmed || rs[0].Proof.Level != domain.LevelObserved {
		t.Fatalf("results: %+v", rs)
	}
	p := rs[0].Proof
	if p.Hypothesis != hypID || p.Investigation != invID || p.ProposedRuleDigest != widgetRule().Digest() || *p.VersionBounds != *bounds || len(p.Limitations) == 0 {
		t.Fatalf("proof: %+v", p)
	}
	if err := w.accept(t, t0.Add(2*time.Hour)); err != nil {
		t.Fatalf("promotion with a confirmed proof refused: %v", err)
	}
	// decisive proofs are not re-verified by default
	rs, _ = Hypotheses(context.Background(), w.options(t, true, fixtureResult(false, true)))
	if len(rs) != 0 {
		t.Fatalf("a decided item was re-verified: %+v", rs)
	}
}

func TestHypothesesWithoutRuntimeIsInconclusive(t *testing.T) {
	w := newWorld(t, widgetRule(), bounds)
	rs, err := Hypotheses(context.Background(), w.options(t, false, nil))
	if err != nil {
		t.Fatal(err)
	}
	p := rs[0].Proof
	if p == nil || p.Outcome != domain.ProofInconclusive || p.Level != domain.LevelDeterministic || p.Methods[0].Outcome != domain.MethodPass {
		t.Fatalf("proof: %+v", p)
	}
	if err := w.accept(t, t0.Add(2*time.Hour)); err == nil || !strings.Contains(err.Error(), "inconclusive") {
		t.Fatalf("promotion on an inconclusive proof: %v", err)
	}
	// an inconclusive proof is re-verified on the next pass
	rs, _ = Hypotheses(context.Background(), w.options(t, true, fixtureResult(false, true)))
	if len(rs) != 1 || rs[0].Proof == nil || rs[0].Proof.Outcome != domain.ProofConfirmed {
		t.Fatalf("re-verification: %+v", rs)
	}
}

func TestHypothesesRefutedWhenTheConditionDoesNotPreserve(t *testing.T) {
	w := newWorld(t, widgetRule(), bounds)
	rs, _ := Hypotheses(context.Background(), w.options(t, true, fixtureResult(true, true)))
	if p := rs[0].Proof; p == nil || p.Outcome != domain.ProofRefuted {
		t.Fatalf("proof: %+v", rs[0])
	}
	if err := w.accept(t, t0.Add(2*time.Hour)); err == nil || !strings.Contains(err.Error(), "refuted") {
		t.Fatalf("promotion of a refuted rule: %v", err)
	}
}

func TestHypothesesRefutedByCitationOutsideBounds(t *testing.T) {
	w := newWorld(t, widgetRule(), &domain.ProofBounds{From: "2.1.0", To: "2.2.0"})
	rs, _ := Hypotheses(context.Background(), w.options(t, true, fixtureResult(false, true)))
	p := rs[0].Proof
	if p == nil || p.Outcome != domain.ProofRefuted || p.Methods[0].Outcome != domain.MethodFail {
		t.Fatalf("proof: %+v", rs[0])
	}
}

func TestHypothesesSkipsAnItemWhoseRuleIsNotTheSpec(t *testing.T) {
	other := widgetRule()
	other.Applicability.Overlap = nil
	w := newWorld(t, other, bounds)
	rs, _ := Hypotheses(context.Background(), w.options(t, true, fixtureResult(false, true)))
	if len(rs) != 1 || rs[0].Proof != nil || !strings.Contains(rs[0].Skipped, "another rule") {
		t.Fatalf("results: %+v", rs)
	}
}

func TestHypothesesUnknownRunIsInconclusive(t *testing.T) {
	w := newWorld(t, widgetRule(), bounds)
	os.Remove(filepath.Join(w.runs, invID+".json"))
	rs, _ := Hypotheses(context.Background(), w.options(t, true, fixtureResult(false, true)))
	p := rs[0].Proof
	if p == nil || p.Outcome != domain.ProofInconclusive || p.VersionBounds != nil {
		t.Fatalf("proof: %+v", rs[0])
	}
}
