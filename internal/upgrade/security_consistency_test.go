package upgrade

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/github"
)

func candidateVersion(c domain.VersionCandidate) string {
	if c.Version == nil {
		return "none"
	}
	return c.Version.Semver
}

// An advisory states one range across lines 1.1 and 1.2 but lists per-line
// fixes: its patched 1.1.1 lies inside its own range. The range decides: the
// candidates are those of the same advisory without the contradicting
// patched version. The finding is a gap and a warning, and each candidate
// whose answer the patched list would change says so as an alternative.
func TestSecuritySelfContradictingAdvisory(t *testing.T) {
	self := domain.Advisory{ID: "GHSA-self", Aliases: []string{"CVE-2026-0001"}, Vulnerable: ">= 1.1.0, <= 1.1.2", Patched: []string{"1.1.1", "1.2.0"}}
	e := mustBuild(t, securityInput(self))
	s := e.Security
	if secIDs(s.Regressions) != "GHSA-self" || len(s.Inconsistencies) != 1 {
		t.Fatalf("security %+v", s)
	}
	inc := s.Inconsistencies[0]
	wantStmt := "GHSA-self (CVE-2026-0001) lists 1.1.1 as patched but its range >= 1.1.0, <= 1.1.2 includes it; check the advisory."
	if inc.ID != "GHSA-self" || strings.Join(inc.Contradicting, ",") != "1.1.1" || strings.Join(inc.Patched, ",") != "1.1.1,1.2.0" ||
		inc.Vulnerable != ">= 1.1.0, <= 1.1.2" || inc.Statement != wantStmt || len(inc.Evidence) != 1 ||
		inc.ChangeID == "" || inc.ChangeID != s.Regressions[0].ChangeID || inc.SourceID != "global-advisories" {
		t.Errorf("inconsistency %+v", inc)
	}
	if len(s.Gaps) != 1 || !strings.Contains(s.Gaps[0], "advisory-consistency: GHSA-self (CVE-2026-0001) lists 1.1.1 as patched but its range >= 1.1.0, <= 1.1.2 includes it; check the advisory") ||
		!strings.Contains(s.Gaps[0], "https://github.com/advisories/GHSA-self") {
		t.Errorf("gaps %v", s.Gaps)
	}
	if !hasWarning(e, "Advisory GHSA-self (CVE-2026-0001) lists 1.1.1 as patched but its range >= 1.1.0, <= 1.1.2 includes it; check the advisory") {
		t.Errorf("warnings %v", e.Warnings)
	}
	if s.Status != domain.SecurityComplete {
		t.Errorf("status %s: an inconsistency is a finding, not an unread source", s.Status)
	}

	// Candidates stay strict: identical to the advisory without 1.1.1 in
	// its patched list.
	consistent := self
	consistent.Patched = []string{"1.2.0"}
	strict := mustBuild(t, securityInput(consistent)).Security
	if len(strict.Inconsistencies) != 0 || len(strict.Gaps) != 0 || len(strict.Candidates) != len(s.Candidates) {
		t.Fatalf("consistent advisory: %+v", strict)
	}
	for i, c := range s.Candidates {
		sc := strict.Candidates[i]
		if c.Kind != sc.Kind || candidateVersion(c) != candidateVersion(sc) || advRefs(c.Removes) != advRefs(sc.Removes) ||
			advRefs(c.Leaves) != advRefs(sc.Leaves) || !strings.HasPrefix(c.Statement, sc.Statement) || len(sc.Alternatives) != 0 {
			t.Errorf("%s: candidate changed by the patched list: %+v, strict %+v", c.Kind, c, sc)
		}
	}

	want := map[domain.CandidateKind]struct{ strict, reason string }{
		domain.CandidateTargetLine: {"none", "If GHSA-self's patched list is right, v1.1.1 qualifies; it is not chosen because the stated range includes it."},
		domain.CandidateAnyLine:    {"1.2.0", "If GHSA-self's patched list is right, v1.1.1 also qualifies; it is not chosen because the stated range includes it."},
		domain.CandidateClean:      {"1.2.0", "If GHSA-self's patched list is right, v1.1.1 also qualifies; it is not chosen because the stated range includes it."},
	}
	for k, w := range want {
		c := candidate(t, s, k)
		if candidateVersion(c) != w.strict || len(c.Alternatives) != 1 {
			t.Fatalf("%s %+v", k, c)
		}
		alt := c.Alternatives[0]
		if alt.Version == nil || alt.Version.Semver != "1.1.1" || strings.Join(alt.Advisories, ",") != "GHSA-self" || alt.Reason != w.reason ||
			len(alt.Evidence) != 1 || alt.Evidence[0] != inc.Evidence[0] || !strings.HasSuffix(c.Statement, " "+w.reason) {
			t.Errorf("%s alternative %+v, statement %q", k, alt, c.Statement)
		}
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
	// The alternative is not a candidate: no chart lookup for it.
	for _, v := range CandidateVersions(securityInput(self)) {
		if v.Semver == "1.1.1" {
			t.Errorf("CandidateVersions includes the alternative 1.1.1")
		}
	}

	text := strings.Join(SecurityLines(s, e.From, e.To, func(ids []domain.EvidenceID) string { return fmt.Sprint(len(ids)) }), "\n")
	for _, w := range []string{
		"Inconsistent: patched versions inside the advisory's own range, the range decides (1):",
		"• " + wantStmt,
		"• target-line 1.1: none. No known release on line 1.1 at or above v1.1.0 removes every regression of v1.1.0. If GHSA-self's patched list is right, v1.1.1 qualifies",
		"alternative, not chosen (if the patched list of GHSA-self is right): v1.1.1  (1)",
	} {
		if !strings.Contains(text, w) {
			t.Errorf("text lacks %q:\n%s", w, text)
		}
	}
}

// An inconsistent advisory that affects neither endpoint is still reported:
// it can decide a candidate.
func TestSecurityInconsistencyAwayFromEndpoints(t *testing.T) {
	e := mustBuild(t, securityInput(
		domain.Advisory{ID: "GHSA-reg", Vulnerable: ">= 1.1.0, < 1.1.1", Patched: []string{"1.1.1"}},
		domain.Advisory{ID: "GHSA-far", Vulnerable: ">= 1.2.0, <= 1.2.1", Patched: []string{"1.2.1"}},
	))
	s := e.Security
	if len(s.Inconsistencies) != 1 || s.Inconsistencies[0].ID != "GHSA-far" || s.Inconsistencies[0].ChangeID != "" || len(s.Inconsistencies[0].Evidence) != 1 {
		t.Fatalf("inconsistencies %+v", s.Inconsistencies)
	}
	// 1.1.1 is the answer under both readings: no alternative.
	for _, c := range s.Candidates {
		if len(c.Alternatives) != 0 {
			t.Errorf("%s: alternative %+v where the answer does not change", c.Kind, c.Alternatives)
		}
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}

// Consistent advisories produce no finding, including prerelease lower
// bounds whose patched version is the stable upper bound, and a patched
// prerelease below a stable upper bound (the range names no prerelease, so
// RI's reading does not include it).
func TestSecurityConsistentAdvisories(t *testing.T) {
	e := mustBuild(t, securityInput(
		domain.Advisory{ID: "GHSA-reg", Vulnerable: "< 1.0.1 || >= 1.1.0-beta.0, < 1.1.1", Patched: []string{"1.0.1", "1.1.1"}},
		domain.Advisory{ID: "GHSA-per", Vulnerable: ">= 1.0.0, < 1.2.0", Patched: []string{"v1.2.0"}},
		domain.Advisory{ID: "GHSA-rc", Vulnerable: "< 1.1.2", Patched: []string{"1.1.2-rc.1", "1.1.2"}},
		domain.Advisory{ID: "GHSA-text", Vulnerable: ">= 1.0.0, < 1.2.0", Patched: []string{"see the release notes"}},
		domain.Advisory{ID: "GHSA-unk", Summary: "free text", Patched: []string{"1.0.0"}},
	))
	if s := e.Security; len(s.Inconsistencies) != 0 || len(s.Gaps) != 1 || !strings.Contains(s.Gaps[0], "GHSA-unk") {
		t.Errorf("consistent advisories: inconsistencies %+v, gaps %v", s.Inconsistencies, s.Gaps)
	}
	for _, c := range e.Security.Candidates {
		if len(c.Alternatives) != 0 || strings.Contains(c.Statement, "patched list") {
			t.Errorf("%s: %+v", c.Kind, c)
		}
	}
}

// A prerelease patched version inside a range with a prerelease lower bound
// is a contradiction; the stable fix at its upper bound is not.
func TestSecurityInconsistentPrereleasePatched(t *testing.T) {
	e := mustBuild(t, securityInput(
		domain.Advisory{ID: "GHSA-pre", Vulnerable: ">= 1.1.0-beta.0, < 1.1.2", Patched: []string{"1.1.0-rc.1", "1.1.2"}},
	))
	s := e.Security
	if len(s.Inconsistencies) != 1 || strings.Join(s.Inconsistencies[0].Contradicting, ",") != "1.1.0-rc.1" {
		t.Fatalf("inconsistencies %+v", s.Inconsistencies)
	}
	// Under the patched list every 1.1 release from 1.1.0-rc.1 on is fixed,
	// so the target itself would qualify.
	tl := candidate(t, s, domain.CandidateTargetLine)
	if candidateVersion(tl) != "1.1.2" || len(tl.Alternatives) != 1 || tl.Alternatives[0].Version.Semver != "1.1.0" {
		t.Errorf("target-line %+v", tl)
	}
}

// An explicit version list that also names a patched version contradicts
// itself.
func TestSecurityInconsistentExplicitList(t *testing.T) {
	vul, err := github.ConvertVersionRange("v1.1.0,v1.1.1,v1.1.2")
	if err != nil {
		t.Fatal(err)
	}
	e := mustBuild(t, securityInput(
		domain.Advisory{ID: "GHSA-list", Vulnerable: vul, Patched: []string{"v1.1.2", "v1.2.0"}},
	))
	s := e.Security
	if len(s.Inconsistencies) != 1 || strings.Join(s.Inconsistencies[0].Contradicting, ",") != "v1.1.2" ||
		!strings.Contains(s.Inconsistencies[0].Statement, "lists v1.1.2 as patched but its range = 1.1.0 || = 1.1.1 || = 1.1.2 includes it") {
		t.Fatalf("inconsistencies %+v", s.Inconsistencies)
	}
	if tl := candidate(t, s, domain.CandidateTargetLine); candidateVersion(tl) != "none" || len(tl.Alternatives) != 1 || tl.Alternatives[0].Version.Semver != "1.1.2" {
		t.Errorf("target-line %+v", tl)
	}
	if sec := mustBuild(t, securityInput(domain.Advisory{ID: "GHSA-list", Vulnerable: vul, Patched: []string{"v1.2.0"}})).Security; len(sec.Inconsistencies) != 0 {
		t.Errorf("a list whose patched versions are outside it: %+v", sec.Inconsistencies)
	}
}

// When the current version is itself a contradicting patched version, the
// patched list makes it free of the advisory, so a strict candidate that
// keeps the advisory would introduce it: the alternative is stricter, and
// says so. A current-line patch the range includes would qualify under it.
func TestSecurityInconsistencyAtCurrentVersion(t *testing.T) {
	e := mustBuild(t, securityInput(
		domain.Advisory{ID: "GHSA-a", Vulnerable: ">= 1.0.0, <= 1.1.2", Patched: []string{"1.0.1", "1.2.0"}},
		domain.Advisory{ID: "GHSA-r", Vulnerable: ">= 1.1.0, < 1.1.1", Patched: []string{"1.1.1"}},
	))
	s := e.Security
	if secIDs(s.Regressions) != "GHSA-r" || secIDs(s.Persisting) != "GHSA-a" || len(s.Inconsistencies) != 1 {
		t.Fatalf("security %+v", s)
	}
	tl := candidate(t, s, domain.CandidateTargetLine)
	if candidateVersion(tl) != "1.1.1" || len(tl.Alternatives) != 1 || tl.Alternatives[0].Version != nil ||
		tl.Alternatives[0].Reason != "If GHSA-a's patched list is right, v1.1.1 does not qualify (it would introduce an advisory v1.0.1 is then free of) and no known release would qualify; the stated range decides." {
		t.Errorf("target-line %+v", tl)
	}
	al := candidate(t, s, domain.CandidateAnyLine)
	if candidateVersion(al) != "1.1.1" || len(al.Alternatives) != 1 || al.Alternatives[0].Version.Semver != "1.2.0" ||
		!strings.Contains(al.Alternatives[0].Reason, "and v1.2.0 would be the lowest that qualifies; the stated range decides.") {
		t.Errorf("any-line %+v", al)
	}
	if cl := candidate(t, s, domain.CandidateClean); candidateVersion(cl) != "1.2.0" || len(cl.Alternatives) != 0 {
		t.Errorf("clean %+v", cl)
	}
	cu := candidate(t, s, domain.CandidateCurrentLine)
	if candidateVersion(cu) != "none" || len(cu.Alternatives) != 1 || cu.Alternatives[0].Version.Semver != "1.0.2" ||
		cu.Alternatives[0].Reason != "If GHSA-a's patched list is right, v1.0.2 qualifies; it is not chosen because the stated range includes it." {
		t.Errorf("current-line %+v", cu)
	}
	if err := e.Validate(); err != nil {
		t.Fatal(err)
	}
}
