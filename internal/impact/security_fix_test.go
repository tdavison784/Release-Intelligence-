package impact

// The impact:security-fix rule (Round 4, the proxies' adoption-blocking
// finding): note-derived security remediation must be visible
// deterministically, not buried in UNKNOWN behind the optional AI layer —
// with the adversarial guard: a CVE fix that also carries a required
// migration (breaking or action-required) keeps its stronger class.

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/env"
	"github.com/tdavison784/release-intelligence/internal/normalize"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// noteSecurityChange appends a note-derived change citing a GHSA.
func noteSecurityChange(eb *edgeBuilder, title string, opts ...func(*domain.Change)) domain.Change {
	ev := eb.ev("https://example/release-notes.md", title)
	c := domain.Change{
		ID: "chg-" + domain.ShortHash("security", title), Category: domain.CategoryDependency,
		Title: title,
		Provenance: domain.Provenance{Method: domain.MethodDeclared, Producer: normalize.ProducerNotes,
			Rule: "section:deps", Confidence: domain.ConfidenceHigh},
		Evidence: []domain.EvidenceID{ev},
	}
	for _, o := range opts {
		o(&c)
	}
	eb.edge.Changes = append(eb.edge.Changes, c)
	return c
}

func envWithValues(t *testing.T) *env.Environment {
	t.Helper()
	val := writeFile(t, t.TempDir(), "values.yaml", "a: 1\n")
	return loadEnv(t, env.Inputs{ValuesFiles: []string{val}})
}

func TestSecurityFixIsInformationalWithUpstreamChainOnly(t *testing.T) {
	eb := newEdge()
	c := noteSecurityChange(eb, "Bump golang.org/x/crypto to fix GHSA-hcg3-q754-cr77")
	e := buildReport(t, eb.edge, envWithValues(t))

	fs := findingsByRule(e, RuleSecurityFix)
	if len(fs) != 1 {
		t.Fatalf("security-fix findings = %d (all: %+v)", len(fs), e.Findings)
	}
	f := fs[0]
	if f.Classification != domain.ImpactInformational {
		t.Errorf("classification = %s, want informational (applies, you appear safe)", f.Classification)
	}
	if f.Severity != domain.SeverityLow {
		t.Errorf("severity = %s, want low (a confirmed no-action overlap)", f.Severity)
	}
	if f.Provenance.Confidence != domain.ConfidenceHigh {
		t.Errorf("confidence = %q", f.Provenance.Confidence)
	}
	if len(f.Matches) != 0 || len(f.EnvironmentEvidence) != 0 {
		t.Errorf("the security-fix rule consults no environment dimension: matches %+v env %+v", f.Matches, f.EnvironmentEvidence)
	}
	if len(f.UpstreamEvidence) == 0 || f.ChangeID != c.ID {
		t.Errorf("the finding must cite its upstream evidence and join the change: %+v", f)
	}
	if !strings.Contains(f.Detail, "GHSA-hcg3-q754-cr77") {
		t.Errorf("the advisory id must be quoted in the detail: %q", f.Detail)
	}
	if !strings.Contains(f.Detail, "every environment that upgrades") {
		t.Errorf("the detail must state the universal applicability: %q", f.Detail)
	}
	// funnel: the item left UNKNOWN for INFORMATIONAL
	if e.Summary.Informational != 1 || e.Summary.Unknown != 0 {
		t.Errorf("summary = %+v", e.Summary)
	}
	// the wire value the domain contract pins (the Validate carve-out keys on
	// the literal); pinned here too so the two can never drift.
	if RuleSecurityFix != "impact:security-fix" {
		t.Errorf("rule = %q, want impact:security-fix", RuleSecurityFix)
	}
}

// TestSecurityFixGuard: a CVE fix that also carries a required migration
// (breaking, or action-required in the edge) must NOT be downgraded to
// informational — it keeps its stronger class (UNKNOWN: not evaluated here),
// and computed diffs citing a CVE stay out of the rule as well.
func TestSecurityFixGuard(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*domain.Change)
	}{
		{"breaking", func(c *domain.Change) { c.Breaking = true }},
		{"action-required", func(c *domain.Change) { c.ActionRequired = true }},
		{"computed-diff", func(c *domain.Change) {
			c.Provenance = domain.Provenance{Method: domain.MethodComputed, Producer: "upgrade@v1", Rule: "advisory:fixed", Confidence: domain.ConfidenceHigh}
		}},
		{"not-security", func(c *domain.Change) {
			c.Title = "Bump golang.org/x/crypto to v0.35.0"
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eb := newEdge()
			noteSecurityChange(eb, "Bump golang.org/x/crypto to fix GHSA-hcg3-q754-cr77", tc.mutate)
			e := buildReport(t, eb.edge, envWithValues(t))
			if fs := findingsByRule(e, RuleSecurityFix); len(fs) != 0 {
				t.Errorf("security-fix fired; it must keep its stronger class: %+v", fs)
			}
			fs := findingsByRule(e, RuleNotJoined)
			if len(fs) != 1 || fs[0].Classification != domain.ImpactUnknown {
				t.Fatalf("not-joined = %+v (all %+v)", fs, e.Findings)
			}
		})
	}
}

// TestSecurityFixValidateCarveOut: the contract's machine-enforced side — a
// security-fix finding may not claim environment evidence, and the domain
// literal stays pinned by Validate accepting the real shape.
func TestSecurityFixValidateCarveOut(t *testing.T) {
	eb := newEdge()
	noteSecurityChange(eb, "Bump golang.org/x/crypto to fix GHSA-hcg3-q754-cr77")
	e := buildReport(t, eb.edge, envWithValues(t))
	if err := e.Validate(); err != nil {
		t.Fatalf("the real security-fix shape must validate: %v", err)
	}
	// claiming an environment match contradicts universal applicability (the
	// id can stay unresolved — the carve-out fires before resolution)
	for i := range e.Findings {
		if e.Findings[i].Rule == RuleSecurityFix {
			e.Findings[i].Matches = []domain.ImpactMatch{{Kind: domain.MatchValuesKey, Subject: "a", Evidence: []domain.EvidenceID{"ev-dummy"}}}
			break
		}
	}
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "universal") {
		t.Errorf("a security-fix finding with matches must not validate, got %v", err)
	}
}

// TestUpgradeSecurityHelpersAreSharedWithRoutine: ONE definition of
// security-relevance — the predicate the routine carve-out uses is the one
// the impact rule fires on, and the advisory-id extraction serves the detail.
func TestUpgradeSecurityHelpersAreSharedWithRoutine(t *testing.T) {
	c := domain.Change{
		Title:  "Bump golang.org/x/crypto to v0.35.0",
		Detail: "Fixes GHSA-hcg3-q754-cr77 and CVE-2025-22868.",
	}
	if !upgrade.IsSecurityItem(c) {
		t.Error("a CVE/GHSA-citing bump is a security item")
	}
	if routine := func() bool { ok, _ := upgrade.ClassifyRoutine(c); return ok }(); routine {
		t.Error("IsSecurityItem and the routine carve-out must agree: not routine")
	}
	ids := upgrade.SecurityIDs(c)
	if len(ids) != 2 || ids[0] != "GHSA-hcg3-q754-cr77" || ids[1] != "CVE-2025-22868" {
		t.Errorf("SecurityIDs = %v", ids)
	}
	if upgrade.IsSecurityItem(domain.Change{Title: "Bump golang.org/x/crypto to v0.35.0"}) {
		t.Error("a bare bump is not a security item")
	}
}
