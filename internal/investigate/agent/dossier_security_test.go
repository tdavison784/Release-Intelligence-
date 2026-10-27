package agent

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

func TestSecuritySummary(t *testing.T) {
	if securitySummary(nil) != nil {
		t.Fatal("an edge without a security section adds nothing to the dossier")
	}
	v := domain.MustVersion("v1.1.1", "1.1.1")
	s := &domain.EdgeSecurity{
		SchemaVersion: domain.EdgeSecuritySchemaVersion, Status: domain.SecurityIncomplete,
		Regressions: []domain.SecurityAdvisory{{ID: "GHSA-reg", Severity: "critical", Summary: "injection", Vulnerable: ">= 1.1.0, < 1.1.1",
			Patched: []string{"1.1.1"}, SourceID: "global-advisories", Evidence: []domain.EvidenceID{"ev-a"}}},
		Candidates: []domain.VersionCandidate{
			{Kind: domain.CandidateTargetLine, Line: "1.1", Version: &v, Removes: []domain.AdvisoryRef{{ID: "GHSA-reg"}},
				Charts: []domain.CandidateChart{{Name: "example", Version: "4.1.1"}}, Evidence: []domain.EvidenceID{"ev-a", "ev-c"}},
			{Kind: domain.CandidateClean, Statement: "No known release above v1.0.0 (up to v1.1.1) is free of every assessed advisory."},
		},
		Gaps: []string{"security source not read (repo: unavailable — offline)"},
	}
	for i := 0; i < 12; i++ {
		s.Fixed = append(s.Fixed, domain.SecurityAdvisory{ID: "GHSA-f", SourceID: "x", Evidence: []domain.EvidenceID{"ev-f"}})
	}
	d := securitySummary(s)
	if d.Status != "incomplete" || len(d.Regressions) != 1 || len(d.Gaps) != 1 {
		t.Fatalf("summary %+v", d)
	}
	for _, want := range []string{"GHSA-reg (critical): injection", "affected >= 1.1.0, < 1.1.1", "patched 1.1.1", "[ev-a]"} {
		if !strings.Contains(d.Regressions[0], want) {
			t.Errorf("regression line %q lacks %q", d.Regressions[0], want)
		}
	}
	if c := d.Candidates[0]; !strings.Contains(c, "target-line 1.1: v1.1.1. removes [GHSA-reg]") || !strings.Contains(c, "chart example 4.1.1") || !strings.Contains(c, "[ev-a, ev-c]") {
		t.Errorf("candidate line %q", c)
	}
	if !strings.HasPrefix(d.Candidates[1], "clean: none.") {
		t.Errorf("no-candidate line %q", d.Candidates[1])
	}
	if len(d.Fixed) != dossierSecurityLines+1 || !strings.Contains(d.Fixed[dossierSecurityLines], "2 more") {
		t.Errorf("fixed list must be bounded: %d %v", len(d.Fixed), d.Fixed[len(d.Fixed)-1])
	}
}
