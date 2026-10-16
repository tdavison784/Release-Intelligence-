package run_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

type scriptedAgent struct {
	calls int
	st    investigate.ArtifactStore
}

func (a *scriptedAgent) Investigate(_ context.Context, in investigate.InvestigateInput) (*investigate.AgentOutcome, error) {
	a.calls++
	ref, err := a.st.Put(context.Background(), investigate.ArtifactSpec{Kind: domain.ArtifactAgentTranscript, Path: "derived/agent/transcript.jsonl",
		Origin: domain.OriginDerived, MediaType: "application/x-ndjson", URI: "ri:agent@v1", Producer: "agent@v1"}, []byte("{}\n"))
	if err != nil {
		return nil, err
	}
	return &investigate.AgentOutcome{Run: domain.AgentRun{Status: domain.AgentSubmitted, Model: "scripted", Provider: "test",
		PromptVersion: in.Config.PromptVersion, PromptDigest: domain.Digest([]byte("p")), CallID: "call-1", Budget: in.Config.Budget, Turns: 1,
		Transcript: ref.ID}, Artifacts: []domain.ArtifactRef{ref}}, nil
}

func TestAgentStageRunsOnceAndIsRecorded(t *testing.T) {
	f := runtest.New(t)
	ag := &scriptedAgent{st: f.St}
	f.Deps.Investigator = ag
	req := f.Request()
	req.Config.Agent.Enabled = true
	res, err := runner(f, nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := f.St.GetInvestigation(context.Background(), res.Investigation)
	if inv.Agent == nil || inv.Agent.Status != domain.AgentSubmitted || ag.calls != 1 {
		t.Fatalf("agent = %+v calls %d", inv.Agent, ag.calls)
	}
	// Re-running the stage with the same key must not call the (costly,
	// non-idempotent) agent again.
	if _, err := runner(f, nil).Run(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if ag.calls != 1 {
		t.Fatalf("agent ran %d times", ag.calls)
	}
}

func TestMergeProposedAddsOnlyInferredEdges(t *testing.T) {
	n := func(k string) domain.GraphNode {
		x := domain.GraphNode{Kind: domain.NodeWorkload, Key: k}
		x.ID = domain.NodeID(x.Kind, x.Key)
		return x
	}
	a, b := n("a"), n("b")
	g := &domain.RelationshipGraph{Bundle: "bnd-x", Builder: "t@v1", Nodes: []domain.GraphNode{a, b}}
	if a.ID > b.ID {
		g.Nodes = []domain.GraphNode{b, a}
	}
	g.ID = g.DerivedID()
	if g.Validate() != nil {
		t.Fatal(g.Validate())
	}
	e := domain.GraphEdge{Kind: domain.EdgeKinds[0], From: a.ID, To: "gn-unknown"}
	as := []domain.AgentAssertion{{ID: "asr-1", ProposedEdge: &e}, {ID: "asr-2"}}
	m, notes := run.MergeProposed(g, as)
	if len(notes) != 0 || len(m.Edges) != 1 || len(m.Nodes) != 3 {
		t.Fatalf("merged = %+v notes %v", m, notes)
	}
	got := m.Edges[0]
	if got.Status != domain.EdgeInferred || got.Assertion != "asr-1" || got.Status.Resolved() {
		t.Fatalf("edge = %+v", got)
	}
	if m.ID == g.ID || m.Validate() != nil {
		t.Fatalf("merged graph invalid or unchanged: %v", m.Validate())
	}
	if same, _ := run.MergeProposed(g, nil); same.ID != g.ID {
		t.Fatal("no proposals must keep the graph")
	}
}

// passValidator yields a deterministic passing record for the first in-scope
// item (so there is a record of the deterministic pass to link).
type passValidator struct{}

func (passValidator) Name() string                         { return "validate.render-diff@v1" }
func (passValidator) Kind() domain.ValidationKind          { return domain.ValidationRenderDiff }
func (passValidator) Applies(domain.ChangeItemFamily) bool { return true }
func (passValidator) Validate(_ context.Context, in investigate.ValidationInput) (*investigate.ValidationOutput, error) {
	r := domain.ValidationRecord{
		Validator: "validate.render-diff@v1", Kind: domain.ValidationRenderDiff, Level: domain.LevelDeterministic,
		ChangeItem: in.ChangeItem, Property: "the render shows no effect", Inputs: []string{in.Bundle.Change.Edge},
		Outcome: domain.ValidationPass, Evidence: []domain.EvidenceID{in.Bundle.Evidence[0].ID},
		Provenance: domain.Provenance{Method: domain.MethodComputed, Producer: "validate.render-diff@v1", Rule: "test", Confidence: domain.ConfidenceHigh},
	}
	r.ID = domain.ValidationRecordID(r.Validator, r.ChangeItem, r.Subjects, r.Paths, r.RenderedChanges, r.Inputs)
	return &investigate.ValidationOutput{Records: []domain.ValidationRecord{r}}, nil
}

// concernAgent records a raises-concern assertion and, when link is set,
// links it to the deterministic pass's record (what request_validation does
// when the requested validation duplicates an existing record).
type concernAgent struct {
	st   investigate.ArtifactStore
	link bool
}

func (a *concernAgent) Investigate(_ context.Context, in investigate.InvestigateInput) (*investigate.AgentOutcome, error) {
	ref, err := a.st.Put(context.Background(), investigate.ArtifactSpec{Kind: domain.ArtifactAgentTranscript, Path: "derived/agent/transcript.jsonl",
		Origin: domain.OriginDerived, MediaType: "application/x-ndjson", URI: "ri:agent@v1", Producer: "agent@v1"}, []byte("{}\n"))
	if err != nil {
		return nil, err
	}
	if len(in.Validations) == 0 || len(in.Bundle.Evidence) == 0 {
		return nil, fmt.Errorf("fixture has no validation or evidence to cite")
	}
	item, pass := in.Validations[0].ChangeItem, in.Validations[0]
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	cite := in.Bundle.Evidence[0].ID
	prov := domain.Provenance{Method: domain.MethodAI, Producer: "agent@v1", Confidence: domain.ConfidenceMedium, Model: "scripted", ModelVersion: "1",
		Provider: "test", PromptVersion: "investigate-agent/v1", PromptDigest: domain.Digest([]byte("p")), InputEvidence: []domain.EvidenceID{cite},
		GeneratedAt: &now, CallID: "call-1"}
	asr := domain.AgentAssertion{ChangeItem: item, Stance: domain.StanceRaisesConcern, Statement: "the flag change may break the controller",
		Citations: []domain.EvidenceID{cite}, Provenance: prov}
	asr.ID = domain.AgentRecordID(domain.AssertionIDPrefix, item, string(asr.Stance)+"\x00"+asr.Statement, prov.CallID)
	out := &investigate.AgentOutcome{
		Run: domain.AgentRun{Status: domain.AgentSubmitted, Model: "scripted", Provider: "test", PromptVersion: "investigate-agent/v1",
			PromptDigest: prov.PromptDigest, CallID: "call-1", Budget: in.Config.Budget, Turns: 1, Transcript: ref.ID},
		Assertions: []domain.AgentAssertion{asr}, Artifacts: []domain.ArtifactRef{ref},
	}
	if a.link {
		out.AnswerLinks = []investigate.AnswerLink{{Validation: pass.ID, Request: "vrq-1", Assertions: []string{asr.ID}}}
	}
	return out, nil
}

type spyClassifier struct {
	inner investigate.Classifier
	seen  []domain.ValidationRecord
}

func (s *spyClassifier) Classify(ctx context.Context, in investigate.ClassifyInput) (*investigate.Classification, error) {
	s.seen = in.Validations
	return s.inner.Classify(ctx, in)
}

// A link from the agent's requested validation to a record of the
// deterministic pass is applied by the runner before classify, so the
// passing record answers the concern (R2.6 lifts it); without the link the
// concern stays unanswered.
func TestAnswerLinkReachesClassifyAndRecord(t *testing.T) {
	for _, link := range []bool{true, false} {
		f := runtest.New(t)
		spy := &spyClassifier{inner: f.Deps.Classifier}
		f.Deps.Classifier = spy
		f.Deps.Validators = func(*domain.ImpactReport) []investigate.Validator { return []investigate.Validator{passValidator{}} }
		f.Deps.Investigator = &concernAgent{st: f.St, link: link}
		req := f.Request()
		req.Config.Validators = []string{"validate.render-diff@v1"}
		req.Config.Agent.Enabled = true
		res, err := runner(f, nil).Run(context.Background(), req)
		if err != nil {
			t.Fatalf("link=%t: %v", link, err)
		}
		inv, _ := f.St.GetInvestigation(context.Background(), res.Investigation)
		if len(inv.Assertions) != 1 {
			t.Fatalf("link=%t: assertions = %d", link, len(inv.Assertions))
		}
		answered := func(rs []domain.ValidationRecord) bool {
			for _, r := range rs {
				for _, a := range r.Answers {
					if a == inv.Assertions[0].ID && r.Outcome == domain.ValidationPass {
						return true
					}
				}
			}
			return false
		}
		if got := answered(spy.seen); got != link {
			t.Errorf("link=%t: classifier saw the concern answered = %t", link, got)
		}
		if got := answered(inv.Validations); got != link {
			t.Errorf("link=%t: record answers the concern = %t", link, got)
		}
	}
}
