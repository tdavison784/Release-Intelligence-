package upgrade

import (
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// securityAssessment collects, while advisoryChanges classifies the
// advisories, what the edge's security section reports, and computes the
// candidate versions when the classification is done (finish).
//
// The section is deterministic: an advisory is in Regressions when its
// machine-readable range covers the target and not the current version, in
// Persisting when it covers both, in Fixed when it covers only the current
// version, and in Unassessed when it has no machine-readable range. A
// security source that could not be read makes the section incomplete (or
// unavailable when none was read), with the reason as a gap: unread is never
// "no advisories".
type securityAssessment struct {
	b   *builder
	sec *domain.EdgeSecurity
	// ranges are the assessed advisories' constraints, by id; order keeps
	// the classification order (advisory id).
	ranges map[string]*semver.Constraints
	order  []string
	// entries are the section entries by id (for change ids and
	// candidate references).
	entries   map[string]*domain.SecurityAdvisory
	fromAff   map[string]bool
	toAff     map[string]bool
	changeIDs map[string]string
}

func newSecurityAssessment(b *builder) *securityAssessment {
	s := &securityAssessment{
		b: b,
		sec: &domain.EdgeSecurity{
			SchemaVersion: domain.EdgeSecuritySchemaVersion,
			Sources:       []domain.SourceStatus{},
		},
		ranges:    map[string]*semver.Constraints{},
		entries:   map[string]*domain.SecurityAdvisory{},
		fromAff:   map[string]bool{},
		toAff:     map[string]bool{},
		changeIDs: map[string]string{},
	}
	read, unread := 0, 0
	for _, st := range b.in.ExtraSources {
		if !hasRole(st, domain.RoleSecurity) {
			continue
		}
		s.sec.Sources = append(s.sec.Sources, st)
		switch st.State {
		case domain.SourceOK:
			read++
		case domain.SourceSkipped:
			// a fallback member whose group another source satisfied, or a
			// kind without an advisory adapter: not a read, not a failure
		default:
			unread++
			s.gapf("security source not read (%s): the advisories it lists are unknown, not absent", statusPhrase(st))
		}
	}
	switch {
	case read == 0 && unread == 0:
		s.sec.Status = domain.SecurityUnavailable
		s.gapf("no security source was read for %s: whether advisories affect %s or %s is unknown", b.edge.Product.ID, b.from.Version, b.to.Version)
	case read == 0:
		s.sec.Status = domain.SecurityUnavailable
	case unread > 0:
		s.sec.Status = domain.SecurityIncomplete
	default:
		s.sec.Status = domain.SecurityComplete
	}
	return s
}

func (s *securityAssessment) gapf(format string, args ...any) {
	s.sec.Gaps = append(s.sec.Gaps, fmt.Sprintf(format, args...))
}

func (s *securityAssessment) entry(a domain.Advisory, vul string, ev []domain.EvidenceID) domain.SecurityAdvisory {
	return domain.SecurityAdvisory{
		ID: a.ID, Aliases: a.Aliases, Severity: a.Severity, Summary: strings.TrimSpace(firstLine(a.Summary)), URL: a.URL,
		Vulnerable: vul, Patched: a.Patched, SourceID: a.SourceID, Sources: a.Sources,
		PublishedAt: a.PublishedAt, UpdatedAt: a.UpdatedAt, Evidence: ev,
	}
}

// unassessed records an advisory without a machine-readable range. It is not
// matched against any version, so (like any unmatched advisory) its evidence
// record does not join the edge: the entry is located by its URL and source.
func (s *securityAssessment) unassessed(a domain.Advisory) {
	s.sec.Unassessed = append(s.sec.Unassessed, s.entry(a, "", []domain.EvidenceID{}))
	s.gapf("advisory %s has no machine-readable affected range: whether it affects %s, %s or a candidate is not assessed (check it manually: %s)",
		advisoryLabel(a), s.b.from.Version, s.b.to.Version, where(a))
}

// assessed records an advisory's constraint (used for the candidates).
func (s *securityAssessment) assessed(a domain.Advisory, cs *semver.Constraints) {
	if _, ok := s.ranges[a.ID]; !ok {
		s.order = append(s.order, a.ID)
	}
	s.ranges[a.ID] = cs
}

// classified records an advisory affecting the current version, the target,
// or both, with its resolved evidence.
func (s *securityAssessment) classified(a domain.Advisory, ev []domain.EvidenceID, fromAff, toAff bool) {
	s.fromAff[a.ID], s.toAff[a.ID] = fromAff, toAff
	if len(ev) == 0 {
		// advisoryChanges warns; without evidence nothing can be cited
		s.gapf("advisory %s affects %s but no evidence record is available: not reported", advisoryLabel(a), map[bool]string{true: s.b.to.Version.String(), false: s.b.from.Version.String()}[toAff])
		return
	}
	e := s.entry(a, strings.TrimSpace(a.Vulnerable), ev)
	switch {
	case toAff && !fromAff:
		s.sec.Regressions = append(s.sec.Regressions, e)
	case toAff:
		s.sec.Persisting = append(s.sec.Persisting, e)
	default:
		s.sec.Fixed = append(s.sec.Fixed, e)
	}
}

// changed links the advisory to the change advisoryChanges just added.
func (s *securityAssessment) changed(id string, added bool) {
	if added && len(s.b.changes) > 0 {
		s.changeIDs[id] = s.b.changes[len(s.b.changes)-1].ID
	}
}

func (s *securityAssessment) finish() {
	for _, list := range []*[]domain.SecurityAdvisory{&s.sec.Regressions, &s.sec.Persisting, &s.sec.Fixed} {
		for i := range *list {
			a := &(*list)[i]
			a.ChangeID = s.changeIDs[a.ID]
			s.entries[a.ID] = a
		}
	}
	s.candidates()
	s.b.edge.Security = s.sec
}

// candidates computes the candidate versions when the target is affected by
// an assessed advisory. Candidates are upgrade targets: they are chosen from
// every known stable release at or above the target (Input.Versions), never
// from a later patch of the line the upgrade leaves. The predicate of a
// "removes every regression" candidate is that it is affected by no assessed
// advisory the current version is free of, so it removes the target's
// regressions and introduces none of its own; "clean" is affected by no
// assessed advisory at all. When advisories persist into the target, the
// current-line entry names the lowest later patch of the current version's
// line below the target that removes them: information for a defer or stay
// decision, not a target. Unassessed advisories cannot be cleared and are
// listed on every candidate. Candidates are never computed when no source
// was read.
func (s *securityAssessment) candidates() {
	if len(s.sec.Regressions) == 0 && len(s.sec.Persisting) == 0 {
		return
	}
	if s.sec.Status == domain.SecurityUnavailable {
		return
	}
	from, to := s.b.from.Version, s.b.to.Version
	// pool: the upgrade targets (at or above To); current: the later patches
	// of From's line below To
	var pool, current []domain.Version
	for _, v := range s.b.in.Versions {
		sv, err := semver.NewVersion(v.Semver)
		if err != nil || sv.Prerelease() != "" {
			continue
		}
		switch {
		case v.Compare(to) >= 0:
			pool = append(pool, v)
		case v.Compare(from) > 0 && v.Line() == from.Line():
			current = append(current, v)
		}
	}
	domain.SortVersions(pool)
	domain.SortVersions(current)
	s.sec.VersionsConsidered = len(pool)
	if len(pool) == 0 {
		s.gapf("no known release list at or above %s was supplied: candidate versions were not computed", to)
		return
	}
	introducesNone := func(v domain.Version) bool {
		for _, id := range s.order {
			if affected(s.ranges[id], v) && !s.fromAff[id] {
				return false
			}
		}
		return true
	}
	clean := func(v domain.Version) bool {
		for _, id := range s.order {
			if affected(s.ranges[id], v) {
				return false
			}
		}
		return true
	}
	lowest := func(vs []domain.Version, pred func(domain.Version) bool) *domain.Version {
		for _, v := range vs {
			if pred(v) {
				v := v
				return &v
			}
		}
		return nil
	}
	newest := pool[len(pool)-1]
	if len(s.sec.Regressions) > 0 {
		line := to.Line()
		v := lowest(pool, func(v domain.Version) bool { return v.Line() == line && introducesNone(v) })
		c := s.candidate(domain.CandidateTargetLine, v)
		c.Line = line
		if v == nil {
			c.Statement = fmt.Sprintf("No known release on line %s at or above %s removes every regression of %s.", line, to, to)
		} else {
			c.Statement = fmt.Sprintf("%s is the lowest known release on line %s at or above %s that removes every regression of %s.", v, line, to, to)
		}
		s.sec.Candidates = append(s.sec.Candidates, c)

		v = lowest(pool, introducesNone)
		c = s.candidate(domain.CandidateAnyLine, v)
		if v == nil {
			c.Statement = fmt.Sprintf("No known release at or above %s (up to %s) removes every regression of %s.", to, newest, to)
		} else {
			c.Statement = fmt.Sprintf("%s is the lowest known release on any line at or above %s that removes every regression of %s.", v, to, to)
		}
		s.sec.Candidates = append(s.sec.Candidates, c)
	}
	v := lowest(pool, clean)
	c := s.candidate(domain.CandidateClean, v)
	if v == nil {
		c.Statement = fmt.Sprintf("No known release at or above %s (up to %s) is free of every assessed advisory.", to, newest)
	} else {
		c.Statement = fmt.Sprintf("%s is the lowest known release at or above %s free of every assessed advisory.", v, to)
	}
	s.sec.Candidates = append(s.sec.Candidates, c)

	if len(s.sec.Persisting) > 0 {
		removesPersisting := func(v domain.Version) bool {
			for _, id := range s.order {
				if s.fromAff[id] && s.toAff[id] && affected(s.ranges[id], v) {
					return false
				}
			}
			return introducesNone(v)
		}
		line := from.Line()
		v := lowest(current, removesPersisting)
		c := s.candidate(domain.CandidateCurrentLine, v)
		c.Line = line
		if v == nil {
			c.Statement = fmt.Sprintf("No known later patch of %s on line %s below %s removes every advisory persisting into %s.", from, line, to, to)
		} else {
			c.Statement = fmt.Sprintf("%s is the lowest known later patch of %s on line %s that removes every advisory persisting into %s. It is below %s: information for a defer or stay decision, not an upgrade target.", v, from, line, to, to)
		}
		s.sec.Candidates = append(s.sec.Candidates, c)
	}
}

// candidate fills in what v removes and leaves (relative to the target), the
// unassessed advisories, the chart versions and the evidence.
func (s *securityAssessment) candidate(kind domain.CandidateKind, v *domain.Version) domain.VersionCandidate {
	c := domain.VersionCandidate{Kind: kind, Version: v, Evidence: []domain.EvidenceID{}}
	for _, a := range s.sec.Unassessed {
		c.Unassessed = append(c.Unassessed, a.ID)
	}
	if v == nil {
		return c
	}
	ref := func(id string) (domain.AdvisoryRef, bool) {
		e, ok := s.entries[id]
		if !ok {
			return domain.AdvisoryRef{}, false
		}
		c.Evidence = appendUniqueIDs(c.Evidence, e.Evidence...)
		return domain.AdvisoryRef{ID: id, Evidence: e.Evidence}, true
	}
	for _, id := range s.order {
		aff := affected(s.ranges[id], *v)
		switch {
		case s.toAff[id] && !aff:
			if r, ok := ref(id); ok {
				c.Removes = append(c.Removes, r)
			}
		case aff:
			if r, ok := ref(id); ok {
				c.Leaves = append(c.Leaves, r)
			} else {
				// classified without an evidence record (already a gap)
				c.Unassessed = append(c.Unassessed, id)
			}
		}
	}
	charts, ok := s.b.in.Charts[v.Semver]
	switch {
	case ok && len(charts.Charts) > 0:
		for _, ch := range charts.Charts {
			ev := s.b.resolveOrAdd(ch.Evidence)
			c.Charts = append(c.Charts, domain.CandidateChart{Artifact: ch.Artifact, Name: ch.Name, Version: ch.Version, Evidence: ev})
			c.Evidence = appendUniqueIDs(c.Evidence, ev...)
		}
		c.ChartGaps = charts.Gaps
	case ok:
		c.ChartGaps = charts.Gaps
	}
	return c
}

// where locates an advisory for a human: its page, else its source.
func where(a domain.Advisory) string {
	if a.URL != "" {
		return a.URL
	}
	return a.SourceID
}
