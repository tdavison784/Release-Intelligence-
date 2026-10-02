package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

func kstr(s string) *string { return &s }

// seedKnowledge writes a candidate, two proposals and the routed items.
func seedKnowledge(t *testing.T) (dir string, item domain.ReviewItem) {
	t.Helper()
	dir = t.TempDir()
	s := knowledge.NewFileStore(dir)
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	ev := domain.NewEvidence(domain.EvidenceDocument, "notes", "https://example/upgrading.md", "L1", "rotationPolicy default is now Always", "sha256:d", t0)
	an := domain.ChangeAnchor{Release: "v1.18.0", EvidenceKeys: []string{domain.EvidenceKey(ev)}, StatementKeys: []string{domain.StatementKey(ev)}, ChangeIDs: []string{"chg-1"}}
	members := []domain.CandidateMember{{ChangeID: "chg-1", Anchor: &an}}
	c := domain.SemanticCandidate{ID: domain.CandidateID("cert-manager", "v1.18.0", members), Product: "cert-manager", Release: "v1.18.0",
		Members: members, Grouping: "single", Category: domain.CategoryConfiguration, Title: "rotationPolicy default is now Always",
		Evidence: []domain.Evidence{ev}, Producer: "semantic.candidates@v1", CreatedAt: t0}
	put := func(e any) {
		rec, err := domain.NewRecord(e)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Put(context.Background(), rec); err != nil {
			t.Fatal(err)
		}
	}
	put(c)
	at := t0.Add(time.Minute)
	a := domain.SemanticAssertion{
		Subject:       &domain.Subject{Family: domain.SubjectHelmValue, Product: "cert-manager", Path: "rotation.policy"},
		Change:        &domain.ChangeSpec{Type: domain.ChangeKindDefaultChanged, Before: kstr(`"Never"`), After: kstr(`"Always"`)},
		Applicability: &domain.Applicability{Exposure: domain.Condition{Op: domain.OpValuesKey, Path: "rotation.policy", State: domain.StateUnset}},
		Consequence:   &domain.Consequence{Kind: domain.ConsequenceBehaviorChange, ExposedClass: domain.ImpactReviewRequired},
	}
	p := domain.SemanticProposal{CandidateID: c.ID, Task: domain.TaskFull, Provider: "zai", Assertion: a, Citations: []domain.EvidenceID{ev.ID},
		Provenance: domain.Provenance{Method: domain.MethodAI, Producer: "semantic.propose@v1", Confidence: domain.ConfidenceMedium, Model: "glm-5.3-flash",
			ModelVersion: "1", PromptVersion: "v1", PromptDigest: "sha256:x", InputEvidence: []domain.EvidenceID{ev.ID}, GeneratedAt: &at,
			CallID: "call-1"}} // CONTRACT-CHANGE(contract-3): proposals carry a call id (PO-1)
	p.ID = domain.ProposalID(p.CandidateID, p.Task, p.Provider, p.Provenance)
	put(p)
	return dir, domain.ReviewItem{}
}

func kcli(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, errb bytes.Buffer
	err := run(context.Background(), args, &out, &errb)
	return out.String(), err
}

func TestKnowledgeCLIRouteProxyDecideMetricsExport(t *testing.T) {
	dir, _ := seedKnowledge(t)
	out, err := kcli(t, "knowledge", "route", "-dir", dir)
	if err != nil || !strings.Contains(out, "routed 1 candidates") {
		t.Fatalf("route: %q %v", out, err)
	}
	snap, _ := knowledge.NewFileStore(dir).Load(context.Background(), knowledge.Query{})
	if len(snap.ReviewItems) == 0 {
		t.Fatal("no items routed")
	}
	// proxy decisions need provenance
	item := snap.ReviewItems[0].ID
	if _, err := kcli(t, "knowledge", "decide", "-dir", dir, "-reviewer", "claude", "-reviewer-kind", "proxy", item); err == nil {
		t.Fatal("a proxy decision without provenance was accepted")
	}
	// answer every pending item as the proxy
	for _, it := range snap.ReviewItems {
		out, err = kcli(t, "knowledge", "decide", "-dir", dir, "-reviewer", "claude", "-reviewer-kind", "proxy",
			"-proxy-model", "claude-sonnet-5-5", "-proxy-model-version", "claude-sonnet-5-5", "-proxy-prompt-version", "proxy/v1",
			"-proxy-prompt-digest", "sha256:p", "-proxy-input-evidence", string(snap.Candidates[0].Evidence[0].ID), it.ID)
		if err != nil {
			t.Fatalf("decide %s: %v", it.ID, err)
		}
	}
	after, _ := knowledge.NewFileStore(dir).Load(context.Background(), knowledge.Query{})
	if len(after.Facts) != 1 || after.Facts[0].Level() != domain.VerifiedProxy {
		t.Fatalf("facts = %+v", after.Facts)
	}
	out, err = kcli(t, "knowledge", "metrics", "-dir", dir, "-o", "json")
	var m knowledge.LoopMetrics
	if err != nil || json.Unmarshal([]byte(out), &m) != nil || m.Facts.ByLevel[domain.VerifiedProxy] != 1 {
		t.Fatalf("metrics json: %v %q", err, out)
	}
	out, err = kcli(t, "knowledge", "metrics", "-dir", dir)
	if err != nil || !strings.Contains(out, "proxy 1") {
		t.Fatalf("metrics text: %v %q", err, out)
	}
	out, err = kcli(t, "knowledge", "export", "-dir", dir)
	if err != nil || strings.Count(out, "\n") != len(snap.ReviewItems) {
		t.Fatalf("export: %v %d lines for %d items", err, strings.Count(out, "\n"), len(snap.ReviewItems))
	}
	// candidates and propose are the semantic commands (usage error without arguments)
	var semErr, kErr error
	for _, sub := range []string{"candidates", "propose"} {
		_, semErr = kcli(t, "semantic", sub)
		_, kErr = kcli(t, "knowledge", sub)
		if kErr == nil || semErr == nil || kErr.Error() != semErr.Error() {
			t.Fatalf("knowledge %s = %v, semantic %s = %v; want the same command", sub, kErr, sub, semErr)
		}
	}
	if _, err := kcli(t, "knowledge", "validate"); err == nil || !strings.Contains(err.Error(), "-edge") {
		t.Fatalf("validate = %v", err)
	}
}
