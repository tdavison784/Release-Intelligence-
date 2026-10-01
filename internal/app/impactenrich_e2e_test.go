package app

// End-to-end test of the impact AI enrichment step over the recorded
// cert-manager edge and the checked-in environment (the same fixture as the
// impact e2e tests), with the live model's answers committed as llm-cache
// fixtures so `go test` replays them offline deterministically.
//
// The fixtures under testdata/impact-llm-cache were recorded from the Z.AI
// Anthropic-compatible gateway (model glm-5.3-flash) with
//
//	RI_LIVE_LLM=1 ANTHROPIC_API_KEY=… ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic \
//	  go test ./internal/app -run TestImpactEnrichLiveRecord
//
// Re-record only deliberately: a changed prompt (new prompt version, other
// model) is a new digest and stays pending offline.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/impact"
	"github.com/tdavison784/release-intelligence/internal/impactenrich"
	"github.com/tdavison784/release-intelligence/internal/llm"
)

// liveImpactModel is the model the committed answers were recorded from. It
// is part of every prompt digest, so a replay must request the same one.
const liveImpactModel = "glm-5.3-flash"

// replayMax bounds the prompts of the recorded run (the candidate cap of the
// smoke run is 30; the fixtures cover the first 20, applicability first, to
// keep the live recording volume modest).
const replayMax = 20

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// impactLLMCacheDir is the committed answer cache for this step.
func impactLLMCacheDir() string { return filepath.Join("testdata", "impact-llm-cache") }

// impactEnrichFixture builds the deterministic report over the recorded edge
// and runs the enrichment with the given backend.
func impactEnrichFixture(t *testing.T, client llm.Client) (*domain.ImpactReport, *impactenrich.Result, error) {
	t.Helper()
	a := newReplayApp(t, fixedNow)
	rep, edge, e, err := a.ImpactParts(context.Background(), "cert-manager", "v1.17.0", "v1.18.0",
		ImpactOptions{Environment: impactEnvironment()})
	if err != nil {
		t.Fatalf("impact: %v", err)
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("deterministic report invalid: %v", err)
	}
	res, err := impactenrich.Run(context.Background(), rep, edge, e,
		impactenrich.Options{Client: client, Model: liveImpactModel, Max: replayMax, Clock: func() time.Time { return fixedNow }})
	if err != nil {
		return nil, nil, err
	}
	if err := impactenrich.Apply(rep, res, e); err != nil {
		return nil, nil, err
	}
	if err := rep.Validate(); err != nil {
		t.Fatalf("enriched report invalid: %v", err)
	}
	return rep, res, nil
}

// TestImpactEnrichReplayOffline runs the enrichment purely from the committed
// answers (no network, no key). It pins the offline contract: suggestions
// only ever review-required, provenance complete, citations grounded, and the
// funnel movement visible in the summary.
func TestImpactEnrichReplayOffline(t *testing.T) {
	rep, res, err := impactEnrichFixture(t, &llm.Cache{Dir: impactLLMCacheDir()})
	if err != nil {
		t.Fatalf("offline replay: %v", err)
	}
	run := res.Run
	if run.Pending != 0 {
		t.Fatalf("%d prompts have no committed answer; re-record with RI_LIVE_LLM=1 (digests changed?)", run.Pending)
	}
	if run.Requests != replayMax {
		t.Fatalf("requests = %d, want %d (the candidate selection changed)", run.Requests, replayMax)
	}
	// the enriched report also honours the published JSON Schema
	schema, err := jsonschema.NewCompiler().Compile(mustAbs(t, filepath.Join("..", "..", "schemas", "impact-report.schema.json")))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(rep, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(inst); err != nil {
		t.Fatalf("enriched report does not match schemas/impact-report.schema.json: %v", err)
	}
	t.Logf("funnel: %d candidates, %d prompts, %d accepted, %d rejected, %d failed; suggestedReview=%d unknown=%d",
		run.CandidateGroups, run.Requests, run.Accepted, len(run.Rejected), run.Failed, rep.Summary.SuggestedReview, rep.Summary.Unknown)

	if run.Requests == 0 || run.Accepted == 0 {
		t.Fatalf("empty enrichment run: %+v", run)
	}
	// the deterministic verdicts are untouched and every suggestion is a
	// review-required suggestion on an unknown finding
	suggestions := 0
	for _, f := range rep.Findings {
		if f.Provenance.Method == domain.MethodAI {
			t.Fatalf("finding %s carries AI provenance", f.ID)
		}
		if f.SuggestedClassification != "" {
			suggestions++
			if f.Classification != domain.ImpactUnknown || f.SuggestedClassification != domain.ImpactReviewRequired {
				t.Fatalf("suggestion on %s: %s → %s", f.ID, f.Classification, f.SuggestedClassification)
			}
		}
	}
	if suggestions != rep.Summary.SuggestedReview {
		t.Errorf("suggestions = %d, summary says %d", suggestions, rep.Summary.SuggestedReview)
	}
	// provenance is complete on every enrichment and cites only shown evidence
	byID := map[domain.EvidenceID]domain.Evidence{}
	for _, x := range append(append([]domain.Evidence{}, rep.Evidence...), rep.EnvironmentEvidence...) {
		byID[x.ID] = x
	}
	for _, en := range rep.Enrichments {
		p := en.Provenance
		if p.Method != domain.MethodAI || p.Model == "" || p.ModelVersion == "" || p.PromptVersion != impactenrich.PromptVersion ||
			p.PromptDigest == "" || len(p.InputEvidence) == 0 || p.GeneratedAt == nil {
			t.Errorf("enrichment %s: incomplete provenance %+v", en.ID, p)
		}
		if p.Confidence == domain.ConfidenceHigh {
			t.Errorf("enrichment %s: confidence high escapes the cap", en.ID)
		}
		input := map[domain.EvidenceID]bool{}
		for _, id := range p.InputEvidence {
			input[id] = true
			if _, ok := byID[id]; !ok {
				t.Errorf("enrichment %s: input evidence %s not in the report", en.ID, id)
			}
		}
		if len(en.Citations) == 0 {
			t.Errorf("enrichment %s cites nothing", en.ID)
		}
		for _, id := range en.Citations {
			if !input[id] {
				t.Errorf("enrichment %s cites unshown evidence %s", en.ID, id)
			}
		}
		// a suggestion must carry the why: what overlaps and what to inspect
		if en.Kind == domain.EnrichmentPlausiblyApplies && len(en.Content) < 40 {
			t.Errorf("plausibly-applies content too thin to act on: %q", en.Content)
		}
	}
	// the replay is byte-stable: a second identical run produces identical
	// enrichments
	rep2, res2, err := impactEnrichFixture(t, &llm.Cache{Dir: impactLLMCacheDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep2.Enrichments) != len(rep.Enrichments) || rep2.Summary.SuggestedReview != rep.Summary.SuggestedReview {
		t.Fatal("offline replay is not deterministic")
	}
	for i := range rep2.Enrichments {
		if rep2.Enrichments[i].ID != rep.Enrichments[i].ID || rep2.Enrichments[i].Content != rep.Enrichments[i].Content {
			t.Fatalf("enrichment %d differs between replays", i)
		}
	}
	_ = res2
}

// TestImpactEnrichLiveRecord re-records the committed answers from the live
// gateway. Skipped unless RI_LIVE_LLM is set (with ANTHROPIC_API_KEY).
func TestImpactEnrichLiveRecord(t *testing.T) {
	if os.Getenv("RI_LIVE_LLM") == "" {
		t.Skip("set RI_LIVE_LLM=1 (and ANTHROPIC_API_KEY / ANTHROPIC_BASE_URL) to re-record")
	}
	key := os.Getenv("ANTHROPIC_API_KEY")
	if key == "" {
		t.Skip("RI_LIVE_LLM is set but ANTHROPIC_API_KEY is empty")
	}
	c := llm.NewAnthropic(key)
	c.Model = liveImpactModel
	if b := os.Getenv("ANTHROPIC_BASE_URL"); b != "" {
		c.BaseURL = b
		c.ServerFallback = false
	}
	c.Thinking = os.Getenv("ANTHROPIC_THINKING")
	cache := &llm.Cache{Dir: impactLLMCacheDir(), Inner: c, Clock: func() time.Time { return fixedNow }}
	rep, res, err := impactEnrichFixture(t, cache)
	if err != nil {
		t.Fatalf("live record: %v", err)
	}
	run := res.Run
	t.Logf("live: %d candidates, %d prompts, %d accepted, %d rejected, %d pending, %d failed; suggestedReview=%d",
		run.CandidateGroups, run.Requests, run.Accepted, len(run.Rejected), run.Pending, run.Failed, rep.Summary.SuggestedReview)
	var out strings.Builder
	if err := impact.RenderText(&out, rep, impact.RenderOptions{ShowNotAffected: true}); err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + out.String())
}
