package main

import (
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// The B-arm verdict table reads the run record's agent verdicts.
func TestAgentVerdictInputs(t *testing.T) {
	if got := agentVerdictInputs(&invStored{}); got != nil {
		t.Fatalf("no record: %v", got)
	}
	inv := &domain.Investigation{AgentVerdicts: []domain.AgentVerdict{
		{ChangeItem: "ci-a", Class: domain.FindingNotAffected, Confidence: domain.ConfidenceMedium, Mechanism: "m"},
		{ChangeItem: "ci-b", Class: domain.FindingAffected, Confidence: domain.ConfidenceLow, Mechanism: "n"},
	}}
	got := agentVerdictInputs(&invStored{Inv: inv})
	if len(got) != 2 || got[0].Item != "ci-a" || got[0].Class != domain.FindingNotAffected || got[0].Confidence != "medium" || got[1].Mechanism != "n" {
		t.Fatalf("inputs: %+v", got)
	}
}
