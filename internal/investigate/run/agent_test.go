package run_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
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

// edgeAgent proposes a relationship to a node the graph does not have,
// naming the placeholder node in AgentOutcome.ProposedNodes.
type edgeAgent struct{ st investigate.ArtifactStore }

func (a *edgeAgent) Investigate(ctx context.Context, in investigate.InvestigateInput) (*investigate.AgentOutcome, error) {
	out, err := (&concernAgent{st: a.st}).Investigate(ctx, in)
	if err != nil {
		return nil, err
	}
	node := domain.GraphNode{Kind: domain.NodeExternal, Key: "external:billing-api", Placeholder: true}
	node.ID = domain.NodeID(node.Kind, node.Key)
	src := domain.GraphNode{Kind: domain.NodeExternal, Key: "external:checkout", Placeholder: true}
	src.ID = domain.NodeID(src.Kind, src.Key)
	asr := out.Assertions[0]
	asr.Stance = domain.StanceExplains
	asr.ID = domain.AgentRecordID(domain.AssertionIDPrefix, asr.ChangeItem, string(asr.Stance)+"\x00"+asr.Statement, asr.Provenance.CallID)
	e := domain.GraphEdge{Kind: domain.EdgeDependsOn, From: src.ID, To: node.ID, Status: domain.EdgeInferred,
		Rule: "agent.proposed-edge@v1", Reason: "the agent believes the workload calls the billing API", Assertion: asr.ID}
	e.ID = domain.EdgeID(e.Kind, e.From, e.To, e.Detail)
	asr.ProposedEdge = &e
	out.Assertions = []domain.AgentAssertion{asr}
	out.ProposedNodes = []domain.GraphNode{node, src}
	return out, nil
}

// The second traverse runs over the builder's graph plus the agent's
// proposed edge and its placeholder node (AgentOutcome.ProposedNodes): the
// run's graph artifact is the merged one, and the edge stays inferred.
func TestProposedEdgeAndNodesExtendTheRunGraph(t *testing.T) {
	f := runtest.New(t)
	f.Deps.Validators = func(*domain.ImpactReport) []investigate.Validator { return []investigate.Validator{passValidator{}} }
	f.Deps.Investigator = &edgeAgent{st: f.St}
	req := f.Request()
	req.Config.Validators = []string{"validate.render-diff@v1"}
	req.Config.Agent.Enabled = true
	res, err := runner(f, nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	inv, _ := f.St.GetInvestigation(context.Background(), res.Investigation)
	var g domain.RelationshipGraph
	for _, a := range inv.Artifacts {
		if a.ID == inv.Graph {
			rc, err := f.St.Open(context.Background(), a.Digest)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.NewDecoder(rc).Decode(&g); err != nil {
				t.Fatal(err)
			}
			rc.Close()
		}
	}
	var proposed, placeholder bool
	for _, e := range g.Edges {
		proposed = proposed || (e.Assertion != "" && e.Status == domain.EdgeInferred)
	}
	for _, n := range g.Nodes {
		placeholder = placeholder || (n.Placeholder && n.Key == "external:billing-api")
	}
	if !proposed || !placeholder || strings.Contains(g.Builder, "relations.builder@v1") && !strings.Contains(g.Builder, "+") {
		t.Fatalf("run graph %s (builder %s) lacks the proposed edge/node: proposed=%t placeholder=%t", g.ID, g.Builder, proposed, placeholder)
	}
}
