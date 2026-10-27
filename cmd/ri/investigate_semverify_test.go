package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// `ri investigate semcheck` re-checks a stored run's upstream behaviour
// entries against their cited passages (report-only).
func TestInvestigateSemcheckStoredRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "R-1")
	caseDir := filepath.Join(dir, "cases", "c1")
	if err := os.MkdirAll(caseDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inv := domain.Investigation{ID: "inv-000000000001",
		Evidence: []domain.Evidence{{ID: "ev-src", Excerpt: "var validPathType = regexp.MustCompile(`^[A-Za-z0-9\\-\\._~/]+$`)"}},
		AgentAssessments: []domain.AgentAssessment{{ID: "aas-1", ChangeItem: "chi-1", Behaviour: []domain.AssessmentBehaviour{
			{Behaviour: "the webhook accepts `*` in a path", Source: domain.BehaviourUpstream, Citations: []domain.EvidenceID{"ev-src"}},
			{Behaviour: "the webhook rejects `*` in a path", Source: domain.BehaviourUpstream, Citations: []domain.EvidenceID{"ev-src"}},
			{Behaviour: "paths are parsed", Source: domain.BehaviourNotChecked},
		}}}}
	b, _ := json.Marshal(inv)
	for p, data := range map[string][]byte{filepath.Join(dir, "run.json"): []byte(`{"runId":"R-1"}`), filepath.Join(caseDir, "investigation.json"): b} {
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	out, _, err := runCLI(t, "investigate", "semcheck", "-store", dir, "R-1/c1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "2 upstream behaviour entries") || !strings.Contains(out, "unsupported  the webhook accepts `*` in a path") ||
		!strings.Contains(out, "supported    the webhook rejects `*` in a path") {
		t.Errorf("output:\n%s", out)
	}
}
