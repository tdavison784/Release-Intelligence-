package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// rotationFake answers the candidate group of the RotationPolicy default
// change (stated by the upgrade guide and twice by the release notes) with
// one cluster citing one evidence record of each member, and every other
// group with nothing.
func rotationFake() *llm.Fake {
	changeRe := regexp.MustCompile(`(?m)^\[(chg-[^\]]+)\][^\n]*\n(?:  [^\n]*\n)*?  evidence: (ev-[0-9a-f]+)`)
	return &llm.Fake{Model: "fake-model", ModelVersion: "fake-model-v1", Respond: func(req llm.Request) (string, error) {
		u := req.Messages[0].Content
		if !strings.Contains(u, "RotationPolicy") {
			return `{"enrichments":[]}`, nil
		}
		var changes, citations []string
		for _, m := range changeRe.FindAllStringSubmatch(u, -1) {
			changes = append(changes, m[1])
			citations = append(citations, m[2])
		}
		b, _ := json.Marshal(map[string]any{"enrichments": []map[string]any{{
			"kind": "cluster", "title": "Private key rotation defaults to Always",
			"content": "The upgrade guide and the release notes state the same change: RotationPolicy now defaults to Always.",
			"changes": changes, "citations": citations, "confidence": "high"}}})
		return string(b), nil
	}}
}

func deterministicJSON(t *testing.T, e *domain.UpgradeEdge) string {
	t.Helper()
	c := *e
	c.Enrichments, c.EnrichmentRun = nil, nil
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// TestEnrichRecordedCertManager runs the real pipeline offline on the
// recorded cert-manager 1.17 → 1.18 upgrade, enriches it with a fake model,
// and replays the enrichment from the response cache without any model.
func TestEnrichRecordedCertManager(t *testing.T) {
	ctx := context.Background()
	a := newReplayApp(t, fixedNow)
	edge, err := a.Upgrade(ctx, "cert-manager", "v1.17.0", "v1.18.0", UpgradeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	plain := deterministicJSON(t, edge)

	res, err := a.Enrich(ctx, edge, EnrichOptions{Client: rotationFake()})
	if err != nil {
		t.Fatal(err)
	}
	if deterministicJSON(t, edge) != plain {
		t.Fatal("enrichment changed the deterministic edge")
	}
	if len(edge.Enrichments) != 1 || len(res.Run.Rejected) != 0 {
		t.Fatalf("enrichments %+v, rejected %+v", edge.Enrichments, res.Run.Rejected)
	}
	en := edge.Enrichments[0]
	if en.Kind != domain.EnrichmentCluster || len(en.RelatesTo) != 3 || len(en.Citations) != 3 {
		t.Fatalf("expected one cluster of the three RotationPolicy statements: %+v", en)
	}
	if r := edge.EnrichmentRun; r.Clusters != 1 || r.ClusteredChanges != 3 || r.DuplicatesConsolidated != 2 || r.CandidateGroups < 4 {
		t.Fatalf("run: %+v", r)
	}
	if want := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC); !en.Provenance.GeneratedAt.Equal(want) {
		t.Fatalf("generatedAt %v", en.Provenance.GeneratedAt)
	}

	// The enriched edge (as printed by `ri upgrade -enrich -o json`) matches
	// the published schema.
	schemaPath, _ := filepath.Abs(filepath.Join("..", "..", "schemas", "upgrade-edge.schema.json"))
	schema, err := jsonschema.NewCompiler().Compile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(edge)
	inst, err := jsonschema.UnmarshalJSON(strings.NewReader(string(b)))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(inst); err != nil {
		t.Fatalf("enriched edge does not match the schema: %v", err)
	}

	// Offline replay on the same state, without any model and with another
	// clock: the enrichments come back byte for byte from the cache.
	b2, err := New(Config{ProductsDir: productsDir, StateDir: a.cfg.StateDir, Offline: true, Now: func() time.Time { return fixedNow.AddDate(0, 1, 0) }})
	if err != nil {
		t.Fatal(err)
	}
	edge2, err := b2.Upgrade(ctx, "cert-manager", "v1.17.0", "v1.18.0", UpgradeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b2.Enrich(ctx, edge2, EnrichOptions{}); err != nil {
		t.Fatal(err)
	}
	got, _ := json.Marshal(edge2.Enrichments)
	want, _ := json.Marshal(edge.Enrichments)
	if string(got) != string(want) {
		t.Fatalf("offline replay differs:\n%s\n%s", got, want)
	}
	if b2.EnrichmentBackend(EnrichOptions{APIKey: "unused"}) == "" || !strings.Contains(b2.EnrichmentBackend(EnrichOptions{APIKey: "unused"}), "offline") {
		t.Error("offline runs never use the API, even with a key")
	}
}

// A non-default BaseURL must reach the client and disable server fallback
// (gateways reject the beta header).
func TestEnrichBackendBaseURL(t *testing.T) {
	a, err := New(Config{ProductsDir: productsDir, StateDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	opts := EnrichOptions{APIKey: "k", BaseURL: "https://gw.example/api/anthropic"}
	got, _ := a.enrichmentClient(opts)
	an, ok := got.(*llm.Cache)
	if !ok {
		t.Fatalf("backend = %T, want *llm.Cache", got)
	}
	inner, ok := an.Inner.(*llm.Anthropic)
	if !ok {
		t.Fatalf("cache inner = %T, want *llm.Anthropic", an.Inner)
	}
	if inner.BaseURL != "https://gw.example/api/anthropic" {
		t.Errorf("BaseURL = %q", inner.BaseURL)
	}
	if inner.ServerFallback {
		t.Error("ServerFallback not disabled for gateway base URL")
	}
}
