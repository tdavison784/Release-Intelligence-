package semantic

import (
	"context"
	"os"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/knowledge"
)

// TestStoreIntegrity validates every record of a knowledge-layout store written by
// `ri semantic propose -out`: candidates validate, and every proposal validates against
// its stored candidate — domain validation plus the shown-subset check (citations and
// assertions may only rest on evidence the stored candidate actually shows).
//
// Skipped unless SEMANTIC_STORE names the store directory, so offline `go test ./...`
// is unaffected. Usage against a real run, e.g. the learning-loop real run:
//
//	SEMANTIC_STORE=.ri/semantic-run-v2/knowledge go test ./internal/semantic/ -run TestStoreIntegrity -v
func TestStoreIntegrity(t *testing.T) {
	dir := os.Getenv("SEMANTIC_STORE")
	if dir == "" {
		t.Skip("set SEMANTIC_STORE to a knowledge-layout store directory")
	}
	snap, err := knowledge.NewFileStore(dir).Load(context.Background(), knowledge.Query{})
	if err != nil {
		t.Fatal(err)
	}
	cands := map[string]domain.SemanticCandidate{}
	for _, c := range snap.Candidates {
		if err := c.Validate(); err != nil {
			t.Error(err)
		}
		cands[c.ID] = c
	}
	bad := 0
	for _, p := range snap.Proposals {
		if err := p.ValidateAgainst(cands[p.CandidateID]); err != nil {
			bad++
			t.Error(err)
		}
		if err := shownSubset(p, cands[p.CandidateID]); err != nil {
			bad++
			t.Error(err)
		}
	}
	t.Logf("candidates %d, proposals %d, invalid %d", len(snap.Candidates), len(snap.Proposals), bad)
}
