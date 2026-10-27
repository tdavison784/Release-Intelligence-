package upgrade

import (
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

var secSourceOK = domain.SourceStatus{SourceID: "global-advisories", Kind: "github-global-advisories", Roles: []domain.SourceRole{domain.RoleSecurity}, State: domain.SourceOK}

// securityInput is an upgrade 1.0.1 → 1.1.0 of a product whose known
// releases are 1.0.0 … 1.2.1, with the given advisories (each with its own
// evidence record).
func securityInput(advs ...domain.Advisory) Input {
	in := simpleInput(bare("v1.0.1"), bare("v1.1.0"))
	for _, t := range []string{"v1.0.0", "v1.0.1", "v1.1.0", "v1.1.1", "v1.1.2", "v1.2.0", "v1.2.1", "v1.3.0-rc.1"} {
		in.Versions = append(in.Versions, ver(t))
	}
	for i := range advs {
		ev := advisoryEvidence(advs[i].ID, "https://github.com/advisories/"+advs[i].ID, advs[i].ID)
		advs[i].Evidence = []domain.EvidenceID{ev.ID}
		advs[i].SourceID = "global-advisories"
		advs[i].URL = "https://github.com/advisories/" + advs[i].ID
		in.AdvisoryEvidence = append(in.AdvisoryEvidence, ev)
	}
	in.Advisories = advs
	in.ExtraSources = []domain.SourceStatus{secSourceOK}
	return in
}

func secIDs(as []domain.SecurityAdvisory) string {
	var out []string
	for _, a := range as {
		out = append(out, a.ID)
	}
	return strings.Join(out, ",")
}

func advRefs(rs []domain.AdvisoryRef) string {
	var out []string
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return strings.Join(out, ",")
}

func candidate(t *testing.T, s *domain.EdgeSecurity, k domain.CandidateKind) domain.VersionCandidate {
	t.Helper()
	for _, c := range s.Candidates {
		if c.Kind == k {
			return c
		}
	}
	t.Fatalf("no %s candidate in %+v", k, s.Candidates)
	return domain.VersionCandidate{}
}

func TestSecurityClassificationAndCandidates(t *testing.T) {
	in := securityInput(
		// regression through a prerelease lower bound: 1.0.1 not affected, 1.1.0 is
		domain.Advisory{ID: "GHSA-reg", Severity: "critical", Vulnerable: "< 1.0.1 || >= 1.1.0-beta.0, < 1.1.1", Patched: []string{"1.0.1", "1.1.1"}},
		// persisting: both affected, fixed in 1.2.0
		domain.Advisory{ID: "GHSA-per", Severity: "medium", Vulnerable: ">= 1.0.0, < 1.2.0"},
		// fixed: only 1.0.1 affected
		domain.Advisory{ID: "GHSA-fix", Vulnerable: "= 1.0.1"},
		// neither endpoint
		domain.Advisory{ID: "GHSA-old", Vulnerable: "< 1.0.1"},
		// no machine-readable range
		domain.Advisory{ID: "GHSA-unk", Summary: "free text"},
	)
	e := mustBuild(t, in)
	s := e.Security
	if s == nil || s.SchemaVersion != domain.EdgeSecuritySchemaVersion || s.Status != domain.SecurityComplete {
		t.Fatalf("security = %+v", s)
	}
	if secIDs(s.Regressions) != "GHSA-reg" || secIDs(s.Persisting) != "GHSA-per" || secIDs(s.Fixed) != "GHSA-fix" || secIDs(s.Unassessed) != "GHSA-unk" {
		t.Fatalf("reg %s per %s fix %s unk %s", secIDs(s.Regressions), secIDs(s.Persisting), secIDs(s.Fixed), secIDs(s.Unassessed))
	}
	reg := findChanges(e, RuleAdvisoryRegression)
	if len(reg) != 1 || !reg[0].ActionRequired || s.Regressions[0].ChangeID != reg[0].ID || !strings.Contains(reg[0].Detail, "Security regression") {
		t.Fatalf("regression change %+v", reg)
	}
	if per := findChanges(e, RuleAdvisoryAffected); len(per) != 1 || s.Persisting[0].ChangeID != per[0].ID {
		t.Errorf("persisting change %+v", per)
	}
	if len(s.Regressions[0].Evidence) != 1 || s.Regressions[0].SourceID != "global-advisories" {
		t.Errorf("regression provenance %+v", s.Regressions[0])
	}
	if len(s.Gaps) != 1 || !strings.Contains(s.Gaps[0], "GHSA-unk") {
		t.Errorf("gaps %v", s.Gaps)
	}
	// Releases above 1.0.1, stable only: 1.1.0 1.1.1 1.1.2 1.2.0 1.2.1.
	if s.VersionsConsidered != 5 || len(s.Candidates) != 3 {
		t.Fatalf("considered %d, candidates %+v", s.VersionsConsidered, s.Candidates)
	}
	tl := candidate(t, s, domain.CandidateTargetLine)
	if tl.Version == nil || tl.Version.Semver != "1.1.1" || tl.Line != "1.1" || advRefs(tl.Removes) != "GHSA-reg" || advRefs(tl.Leaves) != "GHSA-per" ||
		strings.Join(tl.Unassessed, ",") != "GHSA-unk" || len(tl.Evidence) != 2 {
		t.Errorf("target-line %+v", tl)
	}
	if al := candidate(t, s, domain.CandidateAnyLine); al.Version == nil || al.Version.Semver != "1.1.1" {
		t.Errorf("any-line %+v", al)
	}
	cl := candidate(t, s, domain.CandidateClean)
	if cl.Version == nil || cl.Version.Semver != "1.2.0" || advRefs(cl.Removes) != "GHSA-per,GHSA-reg" || len(cl.Leaves) != 0 ||
		!strings.Contains(cl.Statement, "1.2.0 is the lowest known release above v1.0.1 free of every assessed advisory") {
		t.Errorf("clean %+v", cl)
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

// No release on the target's line removes the regression; another line does;
// nothing is fully clean.
func TestSecurityCandidatesAcrossLines(t *testing.T) {
	e := mustBuild(t, securityInput(
		domain.Advisory{ID: "GHSA-line", Vulnerable: ">= 1.1.0-0, < 1.2.0"},
		domain.Advisory{ID: "GHSA-open", Vulnerable: ">= 1.0.0"},
	))
	s := e.Security
	tl := candidate(t, s, domain.CandidateTargetLine)
	if tl.Version != nil || !strings.Contains(tl.Statement, "No known release on line 1.1 above v1.0.1 removes every regression of v1.1.0") || len(tl.Evidence) != 0 {
		t.Errorf("target-line %+v", tl)
	}
	al := candidate(t, s, domain.CandidateAnyLine)
	if al.Version == nil || al.Version.Semver != "1.2.0" || advRefs(al.Removes) != "GHSA-line" || advRefs(al.Leaves) != "GHSA-open" {
		t.Errorf("any-line %+v", al)
	}
	if cl := candidate(t, s, domain.CandidateClean); cl.Version != nil || !strings.Contains(cl.Statement, "No known release above v1.0.1 (up to v1.2.1)") {
		t.Errorf("clean %+v", cl)
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestSecurityCandidateCharts(t *testing.T) {
	in := securityInput(domain.Advisory{ID: "GHSA-reg", Vulnerable: ">= 1.1.0-beta.0, < 1.1.1"})
	chartEv := domain.NewEvidence(domain.EvidenceRegistry, "chart", "https://charts.example/index.yaml", "entries.demo[version=4.1.1]", "version: 4.1.1, appVersion: 1.1.1", "sha256:"+strings.Repeat("a", 64), fixedNow)
	in.Charts = map[string]ChartLookup{
		"1.1.1": {Charts: []ChartVersion{{Artifact: "chart", Name: "demo", Version: "4.1.1", Evidence: []domain.Evidence{chartEv}}}},
	}
	e := mustBuild(t, in)
	tl := candidate(t, e.Security, domain.CandidateTargetLine)
	if len(tl.Charts) != 1 || tl.Charts[0].Version != "4.1.1" || tl.Charts[0].Evidence[0] != chartEv.ID {
		t.Fatalf("charts %+v", tl.Charts)
	}
	found := false
	for _, x := range e.Evidence {
		found = found || x.ID == chartEv.ID
	}
	if !found {
		t.Error("chart evidence must be part of the edge")
	}
	// CandidateVersions names what to look up: 1.1.1 (target-line, any-line, clean).
	if vs := CandidateVersions(in); len(vs) != 1 || vs[0].Semver != "1.1.1" {
		t.Errorf("CandidateVersions = %v", vs)
	}
}

// An unread source is a gap, never "no advisories"; nothing read means no
// candidates either.
func TestSecurityUnavailableSources(t *testing.T) {
	down := domain.SourceStatus{SourceID: "global-advisories", Kind: "github-global-advisories", Roles: []domain.SourceRole{domain.RoleSecurity},
		State: domain.SourceUnavailable, Detail: "fetch https://api.github.com/advisories: not in cache (offline mode)"}
	in := securityInput()
	in.ExtraSources = []domain.SourceStatus{down}
	e := mustBuild(t, in)
	s := e.Security
	if s.Status != domain.SecurityUnavailable || len(s.Candidates) != 0 || len(s.Gaps) != 1 ||
		!strings.Contains(s.Gaps[0], "not read") || !strings.Contains(s.Gaps[0], "unknown, not absent") {
		t.Errorf("offline miss: %+v", s)
	}
	if !hasWarning(e, "Security advisories were not fully retrieved") {
		t.Errorf("warning: %v", e.Warnings)
	}

	// One source read, one not: what was read is reported, and the section
	// says it is incomplete.
	in = securityInput(domain.Advisory{ID: "GHSA-reg", Vulnerable: ">= 1.1.0, < 1.1.1"})
	repo := secSourceOK
	repo.SourceID, repo.Kind = "repo-advisories", "github-advisories"
	in.ExtraSources = []domain.SourceStatus{repo, down}
	s = mustBuild(t, in).Security
	if s.Status != domain.SecurityIncomplete || secIDs(s.Regressions) != "GHSA-reg" || len(s.Candidates) != 3 || len(s.Gaps) != 1 {
		t.Errorf("incomplete: %+v", s)
	}

	// No security source at all.
	in = securityInput()
	in.ExtraSources = nil
	if s := mustBuild(t, in).Security; s.Status != domain.SecurityUnavailable || len(s.Gaps) != 1 || !strings.Contains(s.Gaps[0], "no security source was read") {
		t.Errorf("none declared: %+v", s)
	}

	// Read and nothing affects the endpoints: complete, no candidates.
	in = securityInput(domain.Advisory{ID: "GHSA-old", Vulnerable: "< 1.0.0"})
	if s := mustBuild(t, in).Security; s.Status != domain.SecurityComplete || len(s.Candidates) != 0 || len(s.Gaps) != 0 {
		t.Errorf("clean target: %+v", s)
	}
}
