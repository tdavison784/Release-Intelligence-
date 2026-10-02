package render

import (
	"context"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func demoEdge() *domain.UpgradeEdge {
	ev := domain.NewEvidence(domain.EvidenceDocument, "release-notes", "https://example.org/demo/releases/1.1.0", "## Removed",
		"Removed the `--enable-certificate-owner-ref` flag.", "sha256:n", fixedNow)
	return &domain.UpgradeEdge{Product: domain.ProductRef{ID: "demo"}, From: domain.Version{Semver: "1.0.0"}, To: domain.Version{Semver: "1.1.0"},
		Evidence: []domain.Evidence{ev},
		Changes: []domain.Change{{ID: "chg-flag", Category: domain.CategoryRemoval, Title: "Removed the `--enable-certificate-owner-ref` flag",
			Release: "1.1.0", Provenance: domain.Provenance{Method: domain.MethodDeclared}, Evidence: []domain.EvidenceID{ev.ID}}}}
}

// Release-level rendered changes may be cited by proposals: they validate as
// candidate (knowledge) evidence, and the deterministic annotation picks the
// rendered changes a candidate's members restate.
func TestEdgeRenderedChangesForPrompts(t *testing.T) {
	er, err := EdgeRenderedChanges(context.Background(), &fixedPairs{p: goldenReleasePair(t)}, demoEdge())
	if err != nil {
		t.Fatal(err)
	}
	if len(er.Evidence) != len(er.Correlation.Changes) || len(er.Evidence) == 0 {
		t.Fatalf("evidence %d vs changes %d", len(er.Evidence), len(er.Correlation.Changes))
	}
	got := er.ForChanges([]string{"chg-flag"})
	if len(got) != 1 || !strings.Contains(got[0].Excerpt, "--enable-certificate-owner-ref") || got[0].Render.Change != string(ContainerArgRemoved) {
		t.Fatalf("for chg-flag: %+v", got)
	}
	if len(er.Undocumented()) != len(er.Evidence)-1 {
		t.Errorf("undocumented = %d of %d", len(er.Undocumented()), len(er.Evidence))
	}
	cand := domain.SemanticCandidate{Product: "demo", Release: "1.1.0", Grouping: "single", Category: domain.CategoryRemoval,
		Title: "flag removed", Producer: "test@v1", CreatedAt: fixedNow,
		Members:  []domain.CandidateMember{{ChangeID: "chg-flag", Computed: true}},
		Evidence: append([]domain.Evidence{demoEdge().Evidence[0]}, er.Evidence...)}
	cand.ID = domain.CandidateID(cand.Product, cand.Release, cand.Members)
	if err := cand.Validate(); err != nil {
		t.Fatalf("release-level rendered evidence must be valid knowledge evidence: %v", err)
	}
}

// Environment renders carry the customer's values: they never reach a prompt
// or knowledge/ (like TestPromptNeverSendsManifestValues). The prompt
// function refuses them, and the contract rejects their records anyway.
func TestEnvironmentRendersNeverReachPromptsOrKnowledge(t *testing.T) {
	env := envGoldenPair(t)
	if _, err := EdgeRenderedChanges(context.Background(), &fixedPairs{p: env}, demoEdge()); err == nil || !strings.Contains(err.Error(), "never enters a prompt") {
		t.Fatalf("an environment pair was accepted for prompts: %v", err)
	}
	// even a pair mislabelled release at the top but environment inside
	mixed := envGoldenPair(t)
	mixed.Scope = domain.RenderRelease
	if _, err := EdgeRenderedChanges(context.Background(), &fixedPairs{p: mixed}, demoEdge()); err == nil {
		t.Fatal("a pair whose renders are environment scope was accepted")
	}
	// and the contract rejects environment records in knowledge entities
	records := env.RenderedEvidence(false)
	cand := domain.SemanticCandidate{Product: "demo", Release: "1.1.0", Grouping: "single", Category: domain.CategoryRemoval,
		Title: "x", Producer: "test@v1", CreatedAt: fixedNow,
		Members: []domain.CandidateMember{{ChangeID: "chg-flag", Computed: true}}, Evidence: records}
	cand.ID = domain.CandidateID(cand.Product, cand.Release, cand.Members)
	if err := cand.Validate(); err == nil || !strings.Contains(err.Error(), "environment render") {
		t.Fatalf("knowledge accepted environment render evidence: %v", err)
	}
	// environment records hide values unless asked
	for _, e := range records {
		if strings.Contains(e.Excerpt, "→") && strings.Contains(e.Excerpt, `"--enable-certificate-owner-ref=false"`) {
			t.Errorf("environment excerpt shows a value without --show-values: %s", e.Excerpt)
		}
	}
}
