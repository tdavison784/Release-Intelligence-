package synthesize

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// target computes the target options and the PD-3 recommendation from the
// edge's security section and the release facts:
//
//   - minimum: the lowest safe release, the edge's target-line candidate,
//     else its any-line candidate; the proposed target itself when it has no
//     regression;
//   - recommended: the latest known patch on the minimum's line when it is
//     safe (affected by no assessed advisory the current version is free of)
//     and adds no assessed advisory the minimum is free of; otherwise the
//     minimum.
//
// Safe needs a complete advisory assessment: while a security source is
// unread, an advisory it lists is unknown, not absent (mission A2), so no
// release is established as safe and none is recommended. The proposed
// target is always an option (unsafe when it regresses or its status is not
// established). The edge's clean candidate is an option when it differs. The
// current-line candidate is never a target.
func (p *pre) target() {
	up := p.out.Upgrade
	t := domain.JudgementTarget{Current: up.From, Proposed: up.To}
	s := p.sec
	complete := s != nil && s.Status == domain.SecurityComplete
	if s != nil {
		t.SecurityStatus = s.Status
	}
	proposed := domain.TargetOption{Role: domain.TargetProposed, Version: up.To, Line: up.To.Line()}
	switch {
	case s == nil:
		proposed.Statement = fmt.Sprintf("%s is the proposed target; without an advisory assessment its security status is not established.", up.To)
	case s.Status == domain.SecurityUnavailable:
		proposed.Statement = fmt.Sprintf("%s is the proposed target; no advisory source was read, so its security status is not established.", up.To)
	default:
		proposed.Safe = complete && len(s.Regressions) == 0
		for _, a := range s.Regressions {
			proposed.Introduces = append(proposed.Introduces, a.ID)
			proposed.Evidence = append(proposed.Evidence, p.cite(a.Evidence, 2)...)
		}
		for _, list := range [][]domain.SecurityAdvisory{s.Regressions, s.Persisting} {
			for _, a := range list {
				proposed.Leaves = append(proposed.Leaves, a.ID)
			}
		}
		for _, a := range s.Unassessed {
			proposed.Unassessed = append(proposed.Unassessed, a.ID)
		}
		switch {
		case len(proposed.Introduces) > 0:
			proposed.Statement = fmt.Sprintf("%s is the proposed target; it introduces %s, which %s is free of.", up.To, strings.Join(proposed.Introduces, ", "), up.From)
		case !complete:
			proposed.Statement = fmt.Sprintf("%s is the proposed target; no advisory of the sources read affects it beyond %s, but the advisory assessment is %s (an unread source may list one), so its security status is not established.", up.To, up.From, s.Status)
		default:
			proposed.Statement = fmt.Sprintf("%s is the proposed target; it is affected by no assessed advisory %s is free of.", up.To, up.From)
		}
	}
	t.Options = append(t.Options, proposed)

	var minimum *domain.TargetOption
	if proposed.Safe {
		minimum = &t.Options[0]
	} else if s != nil {
		for _, kind := range []domain.CandidateKind{domain.CandidateTargetLine, domain.CandidateAnyLine} {
			if c := candidate(s, kind); c != nil && c.Version != nil {
				o := p.candidateOption(domain.TargetMinimum, *c)
				t.Options = append(t.Options, o)
				if o.Safe {
					minimum = &t.Options[len(t.Options)-1]
				}
				break
			}
		}
	}
	if minimum != nil {
		v := minimum.Version
		t.Minimum = &v
	}
	if s != nil {
		if c := candidate(s, domain.CandidateClean); c != nil && c.Version != nil && (minimum == nil || !c.Version.Equal(minimum.Version)) && !c.Version.Equal(up.To) {
			t.Options = append(t.Options, p.candidateOption(domain.TargetClean, *c))
		}
	}

	switch {
	case minimum == nil && s != nil && s.Status == domain.SecurityIncomplete:
		t.Rule = "The advisory assessment is incomplete: no release is established as safe until every security source is read, so none is recommended."
	case minimum == nil:
		t.Rule = "No release is established as safe, so none is recommended."
	default:
		latest, why := p.latestPatch(*minimum)
		if latest != nil {
			t.Options = append(t.Options, *latest)
			v := latest.Version
			t.Recommended = &v
			t.Rule = fmt.Sprintf("%s is the latest known patch on line %s: it removes every regression and adds no assessed advisory %s is free of, so it is recommended (PD-3); %s is the minimum.", latest.Version, latest.Line, minimum.Version, minimum.Version)
		} else {
			v := minimum.Version
			t.Recommended = &v
			t.Rule = fmt.Sprintf("%s is recommended as the minimum safe release: %s.", minimum.Version, why)
		}
	}
	p.out.Target = t
}

func candidate(s *domain.EdgeSecurity, kind domain.CandidateKind) *domain.VersionCandidate {
	for i := range s.Candidates {
		if s.Candidates[i].Kind == kind {
			return &s.Candidates[i]
		}
	}
	return nil
}

func (p *pre) candidateOption(role string, c domain.VersionCandidate) domain.TargetOption {
	o := domain.TargetOption{Role: role, Version: *c.Version, Line: c.Line, Safe: p.sec.Status == domain.SecurityComplete, Unassessed: c.Unassessed, Statement: c.Statement}
	if !o.Safe {
		o.Statement += fmt.Sprintf(" The advisory assessment is %s, so this is not established as safe.", p.sec.Status)
	}
	if o.Line == "" {
		o.Line = c.Version.Line()
	}
	for _, r := range c.Leaves {
		o.Leaves = append(o.Leaves, r.ID)
	}
	o.Evidence = p.cite(c.Evidence, 0)
	for _, ch := range c.Charts {
		ch.Evidence = p.cite(ch.Evidence, 0)
		o.Charts = append(o.Charts, ch)
	}
	return o
}

// latestPatch evaluates the latest known stable patch on the minimum's line
// against every assessed advisory of the product (the edge's reading,
// upgrade.AdvisoryReading). It returns the option when it qualifies, or why
// not. The release facts must agree with the edge: the same regressions of
// the target, read the same way; otherwise nothing is computed.
func (p *pre) latestPatch(min domain.TargetOption) (*domain.TargetOption, string) {
	r := p.in.Releases
	line := min.Version.Line()
	if r == nil || len(r.Versions) == 0 {
		return nil, fmt.Sprintf("the latest patch on line %s was not computed (no release list was supplied)", line)
	}
	var latest *domain.Version
	for _, v := range r.Versions {
		if v.IsPrerelease() || v.Line() != line || v.Compare(min.Version) <= 0 {
			continue
		}
		if latest == nil || v.Compare(*latest) > 0 {
			v := v
			latest = &v
		}
	}
	if latest == nil {
		return nil, fmt.Sprintf("no known release on line %s is later than %s", line, min.Version)
	}
	from, to := p.out.Upgrade.From, p.out.Upgrade.To
	if p.sec != nil {
		want := map[string]bool{}
		for _, a := range p.sec.Regressions {
			want[a.ID] = true
		}
		got := map[string]bool{}
		for _, a := range r.Advisories {
			atTo, ok := upgrade.AdvisoryReading(a, to)
			atFrom, _ := upgrade.AdvisoryReading(a, from)
			if ok && atTo && !atFrom {
				got[a.ID] = true
			}
		}
		if fmt.Sprint(sortedKeys(want)) != fmt.Sprint(sortedKeys(got)) {
			return nil, fmt.Sprintf("the release facts do not reproduce the edge's regressions (%v vs %v), so %s was not assessed", sortedKeys(got), sortedKeys(want), latest)
		}
	}
	o := domain.TargetOption{Role: domain.TargetLatestPatch, Version: *latest, Line: line, Safe: true}
	var adds []string
	var ev []domain.EvidenceID
	for _, a := range r.Advisories {
		atV, ok := upgrade.AdvisoryReading(a, *latest)
		if !ok {
			o.Unassessed = append(o.Unassessed, a.ID)
			continue
		}
		if !atV {
			continue
		}
		o.Leaves = append(o.Leaves, a.ID)
		ev = append(ev, a.Evidence...)
		if atFrom, _ := upgrade.AdvisoryReading(a, from); !atFrom {
			o.Introduces = append(o.Introduces, a.ID)
		}
		if atMin, _ := upgrade.AdvisoryReading(a, min.Version); !atMin {
			adds = append(adds, a.ID)
		}
	}
	o.Safe = len(o.Introduces) == 0
	switch {
	case !o.Safe:
		return nil, fmt.Sprintf("the latest known patch on line %s, %s, is affected by %s, which %s is free of", line, latest, strings.Join(o.Introduces, ", "), from)
	case len(adds) > 0:
		return nil, fmt.Sprintf("the latest known patch on line %s, %s, is affected by %s, which %s is free of", line, latest, strings.Join(adds, ", "), min.Version)
	}
	o.Evidence = p.cite(ev, 0)
	o.Charts = p.charts(latest.Semver)
	n := 0
	for _, a := range r.Advisories {
		if _, ok := upgrade.AdvisoryReading(a, *latest); ok {
			n++
		}
	}
	o.Statement = fmt.Sprintf("%s is the latest known patch on line %s; checked against %d assessed advisories of the product, it is affected by none %s or %s is free of.", latest, line, n, from, min.Version)
	return &o, ""
}

func (p *pre) charts(semver string) []domain.CandidateChart {
	r := p.in.Releases
	if r == nil {
		return nil
	}
	var out []domain.CandidateChart
	for _, ch := range r.Charts[semver] {
		ch.Evidence = p.cite(ch.Evidence, 0)
		if len(ch.Evidence) > 0 {
			out = append(out, ch)
		}
	}
	return out
}
