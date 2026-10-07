package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
	"github.com/tdavison784/release-intelligence/internal/proxyreview"
)

// answer writes a proxy-review response for a request, as scripts/proxy-review.sh does.
func answer(t *testing.T, xdir, item string, output any, callID string) string {
	t.Helper()
	var req proxyreview.Request
	b, err := os.ReadFile(filepath.Join(xdir, item+".request.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &req); err != nil {
		t.Fatal(err)
	}
	out, _ := json.Marshal(output)
	start := time.Now().UTC().Add(-3 * time.Second)
	resp := proxyreview.Response{Format: proxyreview.ResponseFormat, ItemID: item, PromptDigest: req.PromptDigest,
		Model: "claude-opus-5-5", ModelVersion: "claude-opus-5-5", CallID: callID, StartedAt: start, DecidedAt: start.Add(2 * time.Second),
		DurationMs: 2000, CostUSD: 0.05, Output: out}
	rb, _ := json.Marshal(resp)
	path := filepath.Join(xdir, item+".response.json")
	if err := os.WriteFile(path, rb, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestKnowledgeProxyPromptDecideReport(t *testing.T) {
	dir, _ := seedKnowledge(t)
	if _, err := kcli(t, "knowledge", "route", "-dir", dir); err != nil {
		t.Fatal(err)
	}
	xdir := filepath.Join(t.TempDir(), "x")
	out, err := kcli(t, "knowledge", "proxy-prompt", "-dir", dir, "-o", xdir, "-priority", "all")
	if err != nil {
		t.Fatalf("proxy-prompt: %v", err)
	}
	snap, _ := knowledge.NewFileStore(dir).Load(context.Background(), knowledge.Query{})
	var pending []domain.ReviewItem
	for _, it := range snap.ReviewItems {
		if it.Status == domain.ReviewPending {
			pending = append(pending, it)
		}
	}
	if len(pending) == 0 || !strings.Contains(out, "requests written") {
		t.Fatalf("nothing to review: %q", out)
	}
	// re-running writes nothing new
	if out, _ := kcli(t, "knowledge", "proxy-prompt", "-dir", dir, "-o", xdir, "-priority", "all"); !strings.Contains(out, "0 requests written") {
		t.Errorf("second run rewrote requests: %q", out)
	}
	// the prompt is blind: no model names, no environment, no eval data
	for _, it := range pending {
		b, _ := os.ReadFile(filepath.Join(xdir, it.ID+".request.json"))
		var req proxyreview.Request
		_ = json.Unmarshal(b, &req)
		user := req.Request.Messages[0].Content
		for _, banned := range []string{"glm", "eval/", "case.yaml"} {
			if strings.Contains(strings.ToLower(user), banned) {
				t.Errorf("%s prompt mentions %q", it.ID, banned)
			}
		}
		if len(req.Proposals) == 0 || req.Proposals[0].Model != "glm-5.3-flash" {
			t.Errorf("authors not recorded on the request: %+v", req.Proposals)
		}
	}
	ledger := filepath.Join(t.TempDir(), "ledger.jsonl")
	item := pending[0]
	reqPath := filepath.Join(xdir, item.ID+".request.json")

	// a verdict citing evidence it was not shown is refused and recorded, not dropped
	bad := answer(t, xdir, item.ID, map[string]any{"action": "accept", "reason": "ok", "citations": []string{"ev-nope"}, "confidence": "medium"}, "call-bad")
	if _, err := kcli(t, "knowledge", "decide", "-dir", dir, "-reviewer-kind", "proxy", "-reviewer", "x",
		"-proxy-request", reqPath, "-proxy-response", bad, "-proxy-ledger", ledger, item.ID); err == nil {
		t.Fatal("a verdict outside the schema was recorded")
	}
	// a proxy-response without -reviewer-kind proxy is a usage error
	if _, err := kcli(t, "knowledge", "decide", "-dir", dir, "-reviewer", "x", "-proxy-request", reqPath, "-proxy-response", bad, item.ID); err == nil {
		t.Fatal("proxy-response accepted as a human decision")
	}

	var req proxyreview.Request
	b, _ := os.ReadFile(reqPath)
	_ = json.Unmarshal(b, &req)
	verdict := map[string]any{"action": "accept", "reason": "[" + string(req.InputEvidence[0]) + "] states it.", "citations": []string{string(req.InputEvidence[0])}, "confidence": "medium"}
	if item.QuestionType == domain.QuestionEvidenceSufficiency {
		verdict["action"] = proxyreview.ActionEvidenceSufficient
	}
	good := answer(t, xdir, item.ID, verdict, "call-good")
	out, err = kcli(t, "knowledge", "decide", "-dir", dir, "-reviewer-kind", "proxy", "-reviewer", "ignored",
		"-proxy-request", reqPath, "-proxy-response", good, "-proxy-ledger", ledger, item.ID)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	snap, _ = knowledge.NewFileStore(dir).Load(context.Background(), knowledge.Query{})
	var d *domain.ReviewDecision
	for i := range snap.Decisions {
		if snap.Decisions[i].ReviewItemID == item.ID {
			d = &snap.Decisions[i]
		}
	}
	if d == nil {
		t.Fatalf("no decision recorded: %q", out)
	}
	pp := d.ProxyProvenance
	if d.ReviewerKind != domain.ReviewerProxy || pp == nil || pp.CallID != "call-good" || pp.PromptVersion != proxyreview.PromptVersion ||
		pp.PromptDigest != req.PromptDigest || pp.Provider != "anthropic" || d.Reviewer != "proxy:claude-opus-5-5" {
		t.Errorf("decision attribution: %+v %+v", d, pp)
	}
	if !d.DecidedAt.After(d.StartedAt) {
		t.Errorf("call times not recorded: %v %v", d.StartedAt, d.DecidedAt)
	}
	// the same verdict cannot be recorded twice (the item is no longer pending)
	if _, err := kcli(t, "knowledge", "decide", "-dir", dir, "-reviewer-kind", "proxy", "-reviewer", "x",
		"-proxy-request", reqPath, "-proxy-response", good, "-proxy-ledger", ledger, item.ID); err == nil {
		t.Error("a verdict was recorded twice")
	}
	entries, err := proxyreview.ReadLedger(ledger)
	if err != nil || len(entries) != 3 {
		t.Fatalf("ledger: %d entries, %v", len(entries), err)
	}
	if entries[0].Outcome != proxyreview.OutcomeFailed || entries[1].Outcome != proxyreview.OutcomeDecided || entries[2].Outcome != proxyreview.OutcomeFailed {
		t.Errorf("ledger outcomes: %+v", entries)
	}
	if entries[1].SelfFamily || entries[1].SelfModel || entries[1].ProposalFamilies[0] != "glm" {
		t.Errorf("authorship: %+v", entries[1])
	}
	out, err = kcli(t, "knowledge", "proxy-report", "-dir", dir, "-ledger", ledger)
	if err != nil || !strings.Contains(out, "Decisions recorded: 1") || !strings.Contains(out, "glm-5.3-flash") || !strings.Contains(out, "other-family") {
		t.Errorf("report: %v\n%s", err, out)
	}
}
