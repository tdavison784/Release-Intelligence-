package run_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/run/runtest"
)

// upstreamAgent returns what a v11 session records for upstream evidence
// (briefs/upstream.md): a fulfilled upstream-file request with its fetched
// upstream-document run artifact and the evidence a verified statement cites
// from it, and an unavailable request stated as a gap in the assessment.
type upstreamAgent struct{ st investigate.ArtifactStore }

const fetchedBlob = "https://github.com/example/product/blob/v1.1.0/internal/check.go"

func (a *upstreamAgent) Investigate(ctx context.Context, in investigate.InvestigateInput) (*investigate.AgentOutcome, error) {
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
	fetched, err := a.st.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactUpstreamDocument, Origin: domain.OriginUpstream,
		Path: "upstream/product/v1.1.0/source/example/product/internal/check.go", MediaType: "text/plain", URI: fetchedBlob,
		Producer: "evidence-fetch@v1", Tags: []string{"upstream-fetch"}}, []byte("func check(s string) bool { return s != \"\" }\n"))
	if err != nil {
		return nil, err
	}
	at := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	prov := domain.Provenance{Method: domain.MethodAI, Producer: "investigate.agent@v1", Confidence: domain.ConfidenceLow, Model: "scripted", ModelVersion: "v",
		PromptVersion: "investigate-agent/v11", PromptDigest: domain.Digest([]byte("p")), GeneratedAt: &at, CallID: "call-1", Provider: "test", InputEvidence: inputs}
	req := func(kind domain.EvidenceRequestKind, target string, q domain.UpstreamQuery) domain.EvidenceRequest {
		r := domain.EvidenceRequest{ChangeItem: item, Kind: kind, Target: target, Reason: "what the check accepts", Upstream: &q, Provenance: prov}
		r.ID = domain.AgentRecordID(domain.EvidenceRequestIDPrefix, item, string(kind)+"\x00"+target, prov.CallID)
		return r
	}
	ok := req(domain.RequestUpstreamFile, fetchedBlob, domain.UpstreamQuery{Repository: "example/product", Ref: "v1.1.0", Path: "internal/check.go"})
	ok.Status, ok.Fulfilled = domain.RequestFulfilled, []string{fetched.ID}
	gone := req(domain.RequestUpstreamFile, "https://github.com/example/product/blob/v1.1.0/missing.go", domain.UpstreamQuery{Repository: "example/product", Ref: "v1.1.0", Path: "missing.go"})
	gone.Status, gone.Resolution = domain.RequestUnavailable, "not found at v1.1.0: missing.go in example/product"
	ev := domain.NewEvidence(domain.EvidenceDocument, fetched.ID, fetched.URI, "internal/check.go:1", "return s != \"\"", fetched.Digest, time.Time{})
	as := domain.AgentAssessment{ChangeItem: item, WhatChanged: "a check is enabled by default", Mechanism: "empty values are rejected",
		Verified: []domain.AssessmentFinding{{Statement: "at v1.1.0 the check rejects only the empty value", Citations: []domain.EvidenceID{ev.ID}}},
		Unknown:  []string{"investigation gap (recorded by the tool layer): behaviour at v1.1.0 unavailable: " + gone.Resolution},
		LikelyImpact: "provisional", StatementKind: domain.StatementUnverifiedProse, UnavailableEvidence: []string{gone.ID}, Provenance: prov}
	as.ID = domain.AgentAssessmentID(item, as.WhatChanged, as.Mechanism, prov.CallID)
	out.EvidenceRequests = []domain.EvidenceRequest{ok, gone}
	out.Assessments = []domain.AgentAssessment{as}
	out.Artifacts = append(out.Artifacts, fetched)
	out.Evidence = []domain.Evidence{ev}
	return out, nil
}

// The runner records the fetched artifact in the run (not the bundle), the
// fulfilled request resolves to it, the audit names how each upstream request
// ended, and none of it reaches the classifier.
func TestUpstreamEvidenceIsRecordedInTheRun(t *testing.T) {
	f := runtest.New(t)
	spy := &agentInputSpy{}
	f.Deps.Classifier = spy
	f.Deps.Investigator = &upstreamAgent{st: f.St}
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
	b, err := f.St.GetBundle(context.Background(), inv.Bundle)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.ValidateAgainst(b); err != nil {
		t.Fatal(err)
	}
	var fetched string
	for _, a := range inv.Artifacts {
		if a.URI == fetchedBlob && a.Origin == domain.OriginUpstream && a.Producer == "evidence-fetch@v1" {
			fetched = a.ID
		}
	}
	for _, a := range b.Manifest.Artifacts {
		if a.URI == fetchedBlob {
			t.Fatal("the fetched artifact entered the bundle")
		}
	}
	if fetched == "" || len(inv.EvidenceRequests) != 2 || inv.EvidenceRequests[0].Fulfilled[0] != fetched || inv.EvidenceRequests[1].Upstream.Path != "missing.go" {
		t.Fatalf("artifact %q requests %+v", fetched, inv.EvidenceRequests)
	}
	if len(inv.AgentAssessments) != 1 || len(inv.AgentAssessments[0].UnavailableEvidence) != 1 {
		t.Fatalf("assessment %+v", inv.AgentAssessments)
	}
	var details []string
	for _, e := range inv.Audit {
		if e.Event == "requested" {
			details = append(details, e.Detail)
		}
	}
	joined := strings.Join(details, "\n")
	if !strings.Contains(joined, "evidence: upstream-file "+fetchedBlob+" (fulfilled: "+fetched+")") ||
		!strings.Contains(joined, "(unavailable: not found at v1.1.0: missing.go in example/product)") {
		t.Fatalf("audit details:\n%s", joined)
	}
	if strings.Contains(string(spy.agent), "evidence-fetch@v1") || strings.Contains(string(spy.agent), "unavailableEvidence") {
		t.Fatalf("the classifier received fetched artifacts or gaps: %s", spy.agent)
	}
}
