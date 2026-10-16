package run_test

import (
	"context"
	"testing"

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
