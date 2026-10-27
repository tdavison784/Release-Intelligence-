package run_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

// assessAgent submits one natural assessment (and its verdict) for the first
// in-scope item.
type assessAgent struct{ st investigate.ArtifactStore }

func (a *assessAgent) Investigate(ctx context.Context, in investigate.InvestigateInput) (*investigate.AgentOutcome, error) {
	out, err := (&scriptedAgent{st: a.st}).Investigate(ctx, in)
	if err != nil {
		return nil, err
	}
	var item string
	var inputs []domain.EvidenceID
	for _, it := range in.Bundle.Items() {
		if it.InScope && len(it.Evidence) > 0 {
			item, inputs = it.ID, it.Evidence
			break
		}
	}
	at := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	prov := domain.Provenance{Method: domain.MethodAI, Producer: "agent@v1", Confidence: domain.ConfidenceLow, Model: "scripted", ModelVersion: "v",
		PromptVersion: "investigate-agent/v9", PromptDigest: domain.Digest([]byte("p")), GeneratedAt: &at, CallID: "call-1", Provider: "test", InputEvidence: inputs}
	as := domain.AgentAssessment{ChangeItem: item, WhatChanged: "a default changes", Mechanism: "consumers see the new default",
		LikelyImpact: "none: trust me", StatementKind: domain.StatementUnverifiedProse, Provenance: prov,
		Headline: "Not affected within scope-claim-x, provided precondition-claim-y.", Scope: "scope-claim-x", Preconditions: []string{"precondition-claim-y"}}
	as.ID = domain.AgentAssessmentID(item, as.WhatChanged, as.Mechanism, prov.CallID)
	out.Assessments = []domain.AgentAssessment{as}
	return out, nil
}

// agentInputSpy records what the policy was given.
type agentInputSpy struct {
	runtest.Classifier
	agent []byte
}

func (s *agentInputSpy) Classify(ctx context.Context, in investigate.ClassifyInput) (*investigate.Classification, error) {
	s.agent, _ = json.Marshal(in.Agent)
	return s.Classifier.Classify(ctx, in)
}

// The runner stores the agent's assessments in the run record and never
// passes them to the classifier (agentOutcome omits them, like verdicts).
func TestAssessmentsAreStoredButNotClassified(t *testing.T) {
	f := runtest.New(t)
	spy := &agentInputSpy{}
	f.Deps.Classifier = spy
	f.Deps.Investigator = &assessAgent{st: f.St}
	req := f.Request()
	req.Config.Agent.Enabled = true
	res, err := runner(f, nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := f.St.GetInvestigation(context.Background(), res.Investigation)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.AgentAssessments) != 1 || inv.AgentAssessments[0].LikelyImpact != "none: trust me" || inv.AgentAssessments[0].Scope != "scope-claim-x" ||
		len(inv.AgentAssessments[0].Preconditions) != 1 {
		t.Fatalf("assessments %+v", inv.AgentAssessments)
	}
	// nor its scoped verdict (prompt v10): headline, scope, preconditions
	if len(spy.agent) == 0 || strings.Contains(string(spy.agent), "trust me") || strings.Contains(string(spy.agent), domain.AgentAssessmentIDPrefix) ||
		strings.Contains(string(spy.agent), "scope-claim-x") || strings.Contains(string(spy.agent), "precondition-claim-y") {
		t.Fatalf("the classifier received the assessment: %s", spy.agent)
	}
}
