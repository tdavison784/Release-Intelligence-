package runtest

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// TickClock is a Deps.Now that advances by a second on every call, so a run
// record's audit times are strictly increasing in the order they are stamped
// (a fixed clock hides an event stamped before the events appended after it).
func TickClock() func() time.Time {
	var mu sync.Mutex
	t := FixedNow
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		t = t.Add(time.Second)
		return t
	}
}

// RequestingAgent is a scripted investigator that makes one evidence request
// and one validation request, so the investigate stage appends `requested`
// audit events (actor agent:<model>) before its `completed` event.
type RequestingAgent struct {
	St investigate.ArtifactStore
}

// Investigate implements investigate.Investigator.
func (a *RequestingAgent) Investigate(ctx context.Context, in investigate.InvestigateInput) (*investigate.AgentOutcome, error) {
	ref, err := a.St.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactAgentTranscript, Path: "derived/agent/transcript.jsonl",
		Origin: domain.OriginDerived, MediaType: "application/x-ndjson", URI: "ri:agent@v1", Producer: "agent@v1"}, []byte("{}\n"))
	if err != nil {
		return nil, err
	}
	if len(in.Bundle.Change.Items) == 0 || len(in.Bundle.Evidence) == 0 {
		return nil, fmt.Errorf("fixture has no change item or evidence to cite")
	}
	item := in.Bundle.Change.Items[0].ID
	now := FixedNow
	prov := domain.Provenance{Method: domain.MethodAI, Producer: "agent@v1", Confidence: domain.ConfidenceMedium, Model: "scripted", ModelVersion: "1",
		Provider: "test", PromptVersion: in.Config.PromptVersion, PromptDigest: domain.Digest([]byte("p")),
		InputEvidence: []domain.EvidenceID{in.Bundle.Evidence[0].ID}, GeneratedAt: &now, CallID: "call-1"}
	er := domain.EvidenceRequest{ChangeItem: item, Kind: domain.RequestArtifact, Target: "https://example.test/notes.md",
		Reason: "the notes link a PR that is not in the bundle", Status: domain.RequestUnavailable, Resolution: "not fetched in test", Provenance: prov}
	er.ID = domain.AgentRecordID(domain.EvidenceRequestIDPrefix, er.ChangeItem, string(er.Kind)+"\x00"+er.Target, prov.CallID)
	vr := domain.ValidationRequest{ChangeItem: item, Kind: domain.ValidationRenderDiff, Status: domain.RequestRejected,
		Resolution: "no validator for it in test", Provenance: prov}
	vr.ID = domain.AgentRecordID(domain.ValidationRequestIDPrefix, vr.ChangeItem, string(vr.Kind)+"\x00", prov.CallID)
	return &investigate.AgentOutcome{
		Run: domain.AgentRun{Status: domain.AgentSubmitted, Model: "scripted", Provider: "test", PromptVersion: in.Config.PromptVersion,
			PromptDigest: prov.PromptDigest, CallID: "call-1", Budget: in.Config.Budget, Turns: 1, Transcript: ref.ID},
		EvidenceRequests: []domain.EvidenceRequest{er}, ValidationRequests: []domain.ValidationRequest{vr},
		Artifacts: []domain.ArtifactRef{ref},
	}, nil
}

// CheckAuditMonotonic returns an error unless the record validates, has the
// agent's two `requested` events, and its audit times never go backwards.
func CheckAuditMonotonic(inv *domain.Investigation) error {
	if err := inv.Validate(); err != nil {
		return err
	}
	requested, last := 0, time.Time{}
	for _, e := range inv.Audit {
		if e.At.Before(last) {
			return fmt.Errorf("audit event %d goes back in time", e.Seq)
		}
		last = e.At
		if e.Event == "requested" {
			requested++
		}
	}
	if requested != 2 {
		return fmt.Errorf("want 2 requested events, got %d", requested)
	}
	return nil
}
