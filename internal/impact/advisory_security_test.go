package impact

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// The edge's security section reaches the report: copied, joined to the
// advisory change's finding by changeId, its evidence cited in the report.
// The advisory change keeps its unknown class (nothing becomes less strict),
// and its finding says what decides version applicability.
func TestReportCarriesSecuritySection(t *testing.T) {
	eb := newEdge()
	c := eb.change(upgrade.RuleAdvisoryRegression, "GHSA-reg affects target v1.1.0: injection", "GHSA-reg")
	c.Category, c.ActionRequired = domain.CategorySecurity, true
	eb.edge.Changes[len(eb.edge.Changes)-1] = c
	chartEv := eb.ev("https://charts.example/index.yaml", "version: 4.1.1, appVersion: 1.1.1")
	v := domain.MustVersion("v1.1.1", "1.1.1")
	eb.edge.Security = &domain.EdgeSecurity{
		SchemaVersion: domain.EdgeSecuritySchemaVersion, Status: domain.SecurityComplete,
		Sources:     []domain.SourceStatus{{SourceID: "global-advisories", State: domain.SourceOK, Roles: []domain.SourceRole{domain.RoleSecurity}}},
		Regressions: []domain.SecurityAdvisory{{ID: "GHSA-reg", Severity: "critical", Summary: "injection", Vulnerable: ">= 1.1.0, < 1.1.1", SourceID: "global-advisories", ChangeID: c.ID, Evidence: c.Evidence}},
		Candidates: []domain.VersionCandidate{{Kind: domain.CandidateTargetLine, Line: "1.1", Version: &v, Statement: "v1.1.1 removes it.",
			Removes:  []domain.AdvisoryRef{{ID: "GHSA-reg", Evidence: c.Evidence}},
			Charts:   []domain.CandidateChart{{Artifact: "chart", Name: "example", Version: "4.1.1", Evidence: []domain.EvidenceID{chartEv}}},
			Evidence: append(append([]domain.EvidenceID{}, c.Evidence...), chartEv)}},
		VersionsConsidered: 1,
	}
	rep := buildReport(t, eb.edge, envWithValues(t))
	if rep.Security == nil || rep.Security.Regressions[0].ChangeID != c.ID || rep.Security.Candidates[0].Charts[0].Version != "4.1.1" {
		t.Fatalf("security = %+v", rep.Security)
	}
	cited := map[domain.EvidenceID]bool{}
	for _, e := range rep.Evidence {
		cited[e.ID] = true
	}
	if !cited[chartEv] || !cited[c.Evidence[0]] {
		t.Errorf("security evidence must be part of the report's evidence: %v", rep.Evidence)
	}
	fs := findingsByRule(rep, RuleNotJoined)
	if len(fs) != 1 || fs[0].ChangeID != c.ID || fs[0].Classification != domain.ImpactUnknown ||
		!strings.Contains(strings.Join(fs[0].NeededToDetermine, " "), "security section") {
		t.Fatalf("advisory finding = %+v", fs)
	}
	var buf bytes.Buffer
	if err := RenderText(&buf, rep, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"Security advisories (version-based", "Regressions: affect v1.1.0, not v1.0.0 (1):", "target-line 1.1: v1.1.1", "chart example 4.1.1"} {
		if !strings.Contains(out, want) {
			t.Errorf("render lacks %q:\n%s", want, out)
		}
	}

	// A self-contradicting advisory and a candidate's alternative are
	// carried, cited and rendered.
	altEv := eb.ev("https://github.com/advisories/GHSA-self", "patched 1.1.0")
	eb.edge.Security.Inconsistencies = []domain.AdvisoryInconsistency{{ID: "GHSA-self", Vulnerable: ">= 1.0.0, <= 1.1.2", Patched: []string{"1.1.0"},
		Contradicting: []string{"1.1.0"}, SourceID: "global-advisories", Evidence: []domain.EvidenceID{altEv},
		Statement: "GHSA-self lists 1.1.0 as patched but its range >= 1.0.0, <= 1.1.2 includes it; check the advisory."}}
	alt := domain.MustVersion("v1.1.0", "1.1.0")
	eb.edge.Security.Candidates[0].Alternatives = []domain.CandidateAlternative{{Version: &alt, Advisories: []string{"GHSA-self"},
		Reason: "If GHSA-self's patched list is right, v1.1.0 also qualifies; it is not chosen because the stated range includes it.", Evidence: []domain.EvidenceID{altEv}}}
	rep = buildReport(t, eb.edge, envWithValues(t))
	if len(rep.Security.Inconsistencies) != 1 || len(rep.Security.Candidates[0].Alternatives) != 1 || rep.Security.Candidates[0].Version.Semver != "1.1.1" {
		t.Fatalf("security = %+v", rep.Security)
	}
	found := false
	for _, e := range rep.Evidence {
		found = found || e.ID == altEv
	}
	if !found {
		t.Errorf("inconsistency evidence must be part of the report's evidence: %v", rep.Evidence)
	}
	buf.Reset()
	if err := RenderText(&buf, rep, RenderOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Inconsistent: patched versions inside the advisory's own range, the range decides (1):",
		"GHSA-self lists 1.1.0 as patched but its range >= 1.0.0, <= 1.1.2 includes it; check the advisory.",
		"alternative, not chosen (if the patched list of GHSA-self is right): v1.1.0"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("render lacks %q:\n%s", want, buf.String())
		}
	}

	// Without a section on the edge: none on the report, and an unavailable
	// one is not rendered as a section (the warnings say it).
	eb.edge.Security = nil
	if rep := buildReport(t, eb.edge, envWithValues(t)); rep.Security != nil {
		t.Errorf("security without an edge section: %+v", rep.Security)
	}
}
