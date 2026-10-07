package render

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// TestHelmPairLive renders the testdata chart at both versions for real and
// checks the semantic diff end to end (skipped without helm).
func TestHelmPairLive(t *testing.T) {
	needTool(t, "helm")
	e := testEngine(t)
	p := e.RenderPair(context.Background(), PairRequest{Product: "demo", From: "1.0.0", To: "1.1.0",
		Target: DefaultsTarget("demo"), Scope: domain.RenderRelease, KubeVersion: "1.31"})
	if p.Status != PairOK {
		t.Fatalf("pair: %s %+v", p.Status, p.Failure)
	}
	if os.Getenv("RI_RENDER_GOLDEN") != "" { // regenerate the golden streams
		_ = os.MkdirAll("testdata/golden", 0o755)
		_ = os.WriteFile("testdata/golden/demo-1.0.0.yaml", p.FromResult.Output, 0o644)
		_ = os.WriteFile("testdata/golden/demo-1.1.0.yaml", p.ToResult.Output, 0o644)
	}
	for _, c := range p.Diff.Changes {
		t.Logf("%s", c.Summary(true))
	}
	if p.Diff.Suppressed["nondeterministic"] == 0 {
		t.Errorf("the random Secret token was not detected as nondeterministic: %v", p.Diff.Suppressed)
	}
	if findChange(p.Diff.Changes, FieldChanged, "") != nil {
		for _, c := range p.Diff.Changes {
			if c.Class == FieldChanged && strings.Contains(c.Path, "data.token") {
				t.Errorf("random secret surfaced as a change: %s", c.Summary(false))
			}
		}
	}
	prov := p.ToResult.Provenance
	if prov.ToolVersion == "" || prov.ArtifactDigest == "" || prov.CacheKey == "" || prov.OutputDigest == "" {
		t.Errorf("incomplete provenance: %+v", prov)
	}
	if err := prov.Domain().Validate(); err != nil {
		t.Errorf("domain provenance: %v", err)
	}
	// a second pair is served from the cache with identical output
	p2 := e.RenderPair(context.Background(), PairRequest{Product: "demo", From: "1.0.0", To: "1.1.0",
		Target: DefaultsTarget("demo"), Scope: domain.RenderRelease, KubeVersion: "1.31"})
	if !p2.ToResult.Cached || p2.ToResult.Provenance.OutputDigest != prov.OutputDigest {
		t.Errorf("cache: cached=%v digest %s vs %s", p2.ToResult.Cached, p2.ToResult.Provenance.OutputDigest, prov.OutputDigest)
	}
}
