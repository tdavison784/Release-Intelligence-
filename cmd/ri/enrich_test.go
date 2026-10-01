package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// recordedState copies the offline recording of the end-to-end tests into a
// private state directory.
func recordedState(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "..", "internal", "app", "testdata", "e2e", "state")
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir()) // offline replay must not need git
	t.Setenv("ANTHROPIC_API_KEY", "")
	return dst
}

// TestUpgradeEnrichThroughExchange drives `ri upgrade -enrich` the way a batch
// user would: requests are written, answered by "some model" through files,
// picked up, and later replayed offline from the cache alone.
func TestUpgradeEnrichThroughExchange(t *testing.T) {
	state, exchange := recordedState(t), t.TempDir()
	base := []string{"-products", filepath.Join("..", "..", "products"), "-offline", "-state", state, "upgrade", "cert-manager", "v1.17.0", "v1.18.0"}

	out, errOut, err := runCLI(t, append(base, "-enrich", "-llm-exchange", exchange)...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Enriched conclusions (AI · verify against evidence) (0):") || !strings.Contains(out, "pending") ||
		!strings.Contains(errOut, "prompts are waiting in "+exchange) {
		t.Fatalf("first run:\n%s\n%s", out, errOut)
	}
	reqs, _ := filepath.Glob(filepath.Join(exchange, "*.request.json"))
	if len(reqs) == 0 {
		t.Fatal("no request files written")
	}

	// Answer the RotationPolicy group with a cluster.
	evRe := regexp.MustCompile(`(?m)^\[(chg-[^\]]+)\][^\n]*\n(?:  [^\n]*\n)*?  evidence: (ev-[0-9a-f]+)`)
	answered := 0
	for _, path := range reqs {
		b, _ := os.ReadFile(path)
		var xr llm.ExchangeRequest
		if err := json.Unmarshal(b, &xr); err != nil {
			t.Fatal(err)
		}
		u := xr.Request.Messages[0].Content
		if !strings.Contains(u, "RotationPolicy") {
			continue
		}
		var changes, cites []string
		for _, m := range evRe.FindAllStringSubmatch(u, -1) {
			changes, cites = append(changes, m[1]), append(cites, m[2])
		}
		output, _ := json.Marshal(map[string]any{"enrichments": []map[string]any{{"kind": "cluster", "title": "Rotation policy defaults to Always",
			"content": "Three sources state that RotationPolicy now defaults to Always.", "changes": changes, "citations": cites, "confidence": "medium"}}})
		resp, _ := json.Marshal(llm.ExchangeResponse{PromptDigest: xr.PromptDigest, Model: "batch-model", ModelVersion: "batch-model-1", Output: output})
		if err := os.WriteFile(filepath.Join(exchange, xr.ResponseFile), resp, 0o644); err != nil {
			t.Fatal(err)
		}
		answered++
	}
	if answered != 1 {
		t.Fatalf("answered %d groups", answered)
	}

	out, _, err = runCLI(t, append(base, "-enrich", "-llm-exchange", exchange, "-o", "json")...)
	if err != nil {
		t.Fatal(err)
	}
	var edge domain.UpgradeEdge
	if err := json.Unmarshal([]byte(out), &edge); err != nil {
		t.Fatal(err)
	}
	if len(edge.Enrichments) != 1 || edge.Enrichments[0].Kind != domain.EnrichmentCluster || edge.Enrichments[0].Provenance.Model != "batch-model" ||
		edge.EnrichmentRun == nil || edge.EnrichmentRun.DuplicatesConsolidated != 2 {
		t.Fatalf("json: %+v %+v", edge.Enrichments, edge.EnrichmentRun)
	}
	if err := edge.Validate(); err != nil {
		t.Fatal(err)
	}

	// Offline, without the exchange: replayed from the cache.
	out, _, err = runCLI(t, append(base, "-enrich")...)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Enriched conclusions (AI · verify against evidence) (1):", "◆ [cluster] Rotation policy defaults to Always",
		"(ai · batch-model batch-model-1 · medium)", "Duplicates: 3 changes consolidated into 1 cluster (2 duplicates)"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}

	// Without -enrich the report has no AI section at all.
	out, _, err = runCLI(t, base...)
	if err != nil || strings.Contains(out, "Enriched conclusions") || strings.Contains(out, "AI enrichment") {
		t.Fatalf("plain report: %v\n%s", err, out)
	}
}

func TestEnrichFlagsRequireEnrich(t *testing.T) {
	for _, flag := range [][]string{{"-model", "m"}, {"-llm-exchange", t.TempDir()}} {
		// The product does not exist: the usage error must come before any work.
		_, _, err := runCLI(t, append([]string{"-products", t.TempDir(), "-state", t.TempDir(), "upgrade", "x", "1", "2"}, flag...)...)
		if err == nil || !strings.Contains(err.Error(), "require -enrich") {
			t.Fatalf("%v: got %v", flag, err)
		}
	}
}
