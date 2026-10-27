package upgrade

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// SecurityShown reports whether a text report renders the security section:
// when at least one source was read. An unavailable section is already
// stated by the "not fully retrieved" warnings.
func SecurityShown(s *domain.EdgeSecurity) bool {
	return s != nil && s.Status != domain.SecurityUnavailable
}

// SecurityLines renders the security section of an edge (shared by the
// upgrade and impact text reports): status, regressions, persisting, fixed
// and unassessed advisories, candidate versions with charts, and gaps. cite
// turns evidence ids into the report's citation form.
func SecurityLines(s *domain.EdgeSecurity, from, to domain.Version, cite func([]domain.EvidenceID) string) []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	read := 0
	for _, st := range s.Sources {
		if st.State == domain.SourceOK {
			read++
		}
	}
	add("  Status: %s (%d of %d security sources read)", s.Status, read, len(s.Sources))
	list := func(title string, as []domain.SecurityAdvisory, detail bool) {
		if len(as) == 0 {
			return
		}
		add("  %s (%d):", title, len(as))
		for _, a := range as {
			label := a.ID
			if cves := cveIDs(a.Aliases); len(cves) > 0 {
				label += " (" + strings.Join(cves, ", ") + ")"
			}
			sev := a.Severity
			if sev == "" {
				sev = "unspecified"
			}
			if !detail {
				add("    • %s %s: %s  (%s)", label, sev, a.Summary, sourceOf(a))
				continue
			}
			patched := "unknown"
			if len(a.Patched) > 0 {
				patched = strings.Join(a.Patched, ", ")
			}
			add("    • %s %s: %s", label, sev, a.Summary)
			add("      affected %s; patched %s; %s  (%s)", a.Vulnerable, patched, sourceOf(a), cite(a.Evidence))
		}
	}
	list(fmt.Sprintf("Regressions: affect %s, not %s", to, from), s.Regressions, true)
	list(fmt.Sprintf("Persisting: affect %s and %s", from, to), s.Persisting, true)
	list(fmt.Sprintf("Fixed: affect %s, not %s", from, to), s.Fixed, false)
	list("Unassessed: no machine-readable range, check manually", s.Unassessed, false)
	if len(s.Candidates) > 0 {
		add("  Candidate versions (%d known releases at or above %s considered):", s.VersionsConsidered, to)
		for _, c := range s.Candidates {
			name := c.Label()
			if c.Version == nil {
				add("    • %s: none. %s", name, c.Statement)
				continue
			}
			add("    • %s: %s. %s", name, c.Version, c.Statement)
			add("      removes %s; leaves %s  (%s)", refIDs(c.Removes), refIDs(c.Leaves), cite(c.Evidence))
			if len(c.Unassessed) > 0 {
				add("      not assessed: %s", strings.Join(c.Unassessed, ", "))
			}
			for _, ch := range c.Charts {
				add("      chart %s %s  (%s)", ch.Name, ch.Version, cite(ch.Evidence))
			}
			for _, g := range c.ChartGaps {
				add("      chart: %s", g)
			}
		}
	}
	if len(s.Gaps) > 0 {
		add("  Gaps (%d):", len(s.Gaps))
		for _, g := range s.Gaps {
			add("    · %s", g)
		}
	}
	return out
}

func cveIDs(aliases []string) []string {
	var out []string
	for _, a := range aliases {
		if strings.HasPrefix(strings.ToUpper(a), "CVE-") {
			out = append(out, a)
		}
	}
	return out
}

func sourceOf(a domain.SecurityAdvisory) string {
	s := "source " + a.SourceID
	if len(a.Sources) > 1 {
		s = "sources " + strings.Join(a.Sources, ", ") + " (range from " + a.SourceID + ")"
	}
	if a.UpdatedAt != nil {
		s += ", updated " + a.UpdatedAt.Format("2006-01-02")
	} else if a.PublishedAt != nil {
		s += ", published " + a.PublishedAt.Format("2006-01-02")
	}
	return s
}

func refIDs(rs []domain.AdvisoryRef) string {
	if len(rs) == 0 {
		return "none"
	}
	ids := make([]string, len(rs))
	for i, r := range rs {
		ids[i] = r.ID
	}
	return strings.Join(ids, ", ")
}
