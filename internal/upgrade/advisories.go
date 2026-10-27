package upgrade

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Advisory rules.
const (
	RuleAdvisoryFixed = "advisory:fixed"
	// RuleAdvisoryAffected: the target and the current version are both
	// affected (a persisting exposure).
	RuleAdvisoryAffected = "advisory:affects-target"
	// RuleAdvisoryRegression: the target is affected and the current version
	// is not, so the upgrade introduces the vulnerability. Action required;
	// policy treats it at least as strictly as RuleAdvisoryAffected.
	RuleAdvisoryRegression = "advisory:regression"
)

func affected(c *semver.Constraints, v domain.Version) bool {
	sv, err := semver.NewVersion(v.Semver)
	if err != nil {
		return false
	}
	return c.Check(sv)
}

func advisoryRefs(a domain.Advisory) []domain.Reference {
	var out []domain.Reference
	seen := map[string]bool{}
	addRef := func(id, url string) {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		typ := "advisory"
		switch {
		case strings.HasPrefix(strings.ToUpper(id), "GHSA-"):
			typ = "ghsa"
		case strings.HasPrefix(strings.ToUpper(id), "CVE-"):
			typ = "cve"
		}
		out = append(out, domain.Reference{Type: typ, ID: id, URL: url})
	}
	addRef(a.ID, a.URL)
	for _, al := range a.Aliases {
		addRef(al, "")
	}
	return out
}

func cveAliases(a domain.Advisory) []string {
	var out []string
	for _, al := range a.Aliases {
		if strings.HasPrefix(strings.ToUpper(al), "CVE-") {
			out = append(out, al)
		}
	}
	return out
}

// advisoryLabel renders "GHSA-x (CVE-1, CVE-2)".
func advisoryLabel(a domain.Advisory) string {
	if cves := cveAliases(a); len(cves) > 0 {
		return a.ID + " (" + strings.Join(cves, ", ") + ")"
	}
	return a.ID
}

// advisoryChanges matches advisories against From, the path and To, and
// records the security section (security.go) from the same classification.
func (b *builder) advisoryChanges() {
	sec := newSecurityAssessment(b)
	defer sec.finish()
	advs := append([]domain.Advisory(nil), b.in.Advisories...)
	sort.SliceStable(advs, func(i, j int) bool { return advs[i].ID < advs[j].ID })
	pool := map[domain.EvidenceID]domain.Evidence{}
	for _, e := range b.in.AdvisoryEvidence {
		pool[e.ID] = e
	}
	fromTag, toTag := b.from.Version.String(), b.to.Version.String()
	for _, a := range advs {
		where := a.URL
		if where == "" {
			where = a.SourceID
		}
		vul := strings.TrimSpace(a.Vulnerable)
		if vul == "" {
			b.warnf("Advisory %s has no machine-readable affected-version range; check manually whether it applies (%s)", advisoryLabel(a), where)
			sec.unassessed(a)
			continue
		}
		cs, err := semver.NewConstraint(vul)
		if err != nil {
			b.warnf("Advisory %s has an unparsable affected-version range %q; check manually whether it applies (%s)", advisoryLabel(a), vul, where)
			sec.unassessed(a)
			continue
		}
		sec.assessed(a, cs)
		fromAff, toAff := affected(cs, b.from.Version), affected(cs, b.to.Version)
		if !fromAff && !toAff {
			continue
		}
		for _, id := range a.Evidence {
			if e, ok := pool[id]; ok {
				b.ev.Add(e)
			}
		}
		ev := b.resolve(a.Evidence...)
		patched := "unknown"
		if len(a.Patched) > 0 {
			patched = strings.Join(a.Patched, ", ")
		}
		severity := a.Severity
		if severity == "" {
			severity = "unspecified"
		}
		sec.classified(a, ev, fromAff, toAff)
		if len(ev) == 0 {
			if toAff {
				b.warnf("Target %s is affected by %s, severity %s; patched versions: %s (no evidence record available, not reported as a change)", toTag, advisoryLabel(a), severity, patched)
			} else {
				b.warnf("Advisory %s is fixed by this upgrade, but no evidence record is available for it", advisoryLabel(a))
			}
			continue
		}
		attrs := map[string]string{"vulnerable": vul}
		if a.Severity != "" {
			attrs["severity"] = a.Severity
		}
		if len(a.Patched) > 0 {
			attrs["patched"] = strings.Join(a.Patched, ",")
		}
		fact := domain.NewFact(domain.FactAdvisory, a.ID, "",
			fmt.Sprintf("Advisory %s declares affected versions %s", a.ID, vul), Producer, attrs, ev...)
		b.addFact(fact)
		summary := strings.TrimSpace(firstLine(a.Summary))
		detail := fmt.Sprintf("Severity: %s. Affected versions: %s. Patched: %s.", severity, vul, patched)
		if fromAff && !toAff {
			fixedIn := ""
			for _, s := range b.edge.Path {
				if !affected(cs, s.Version) {
					fixedIn = s.Version.Semver
					break
				}
			}
			sec.changed(a.ID, b.addChange(domain.Change{
				Category:   domain.CategorySecurity,
				Title:      shorten(fmt.Sprintf("Fixes %s: %s", advisoryLabel(a), summary), MaxTitle),
				Detail:     detail + fmt.Sprintf(" %s is affected; %s is not.", fromTag, toTag),
				Release:    fixedIn,
				Subjects:   []string{a.ID},
				References: advisoryRefs(a),
				Provenance: computed(RuleAdvisoryFixed),
				Facts:      []domain.FactID{fact.ID},
				Evidence:   ev,
			}, RuleAdvisoryFixed, a.ID))
			continue
		}
		if !fromAff {
			// The upgrade introduces the vulnerability: a regression.
			sec.changed(a.ID, b.addChange(domain.Change{
				Category:       domain.CategorySecurity,
				ActionRequired: true,
				Title:          shorten(fmt.Sprintf("%s affects target %s: %s", advisoryLabel(a), toTag, summary), MaxTitle),
				Detail:         detail + fmt.Sprintf(" Security regression: %s is not affected; %s is. Consider a release that removes the regression instead (see the security section's candidates).", fromTag, toTag),
				Subjects:       []string{a.ID},
				References:     advisoryRefs(a),
				Provenance:     computed(RuleAdvisoryRegression),
				Facts:          []domain.FactID{fact.ID},
				Evidence:       ev,
			}, RuleAdvisoryRegression, a.ID))
			b.warnf("Target %s is affected by %s, severity %s, and %s is not (a security regression); patched versions: %s", toTag, advisoryLabel(a), severity, fromTag, patched)
			continue
		}
		sec.changed(a.ID, b.addChange(domain.Change{
			Category:       domain.CategorySecurity,
			ActionRequired: true,
			Title:          shorten(fmt.Sprintf("%s still affects target %s: %s", advisoryLabel(a), toTag, summary), MaxTitle),
			Detail:         detail + " Consider upgrading to a patched release instead.",
			Subjects:       []string{a.ID},
			References:     advisoryRefs(a),
			Provenance:     computed(RuleAdvisoryAffected),
			Facts:          []domain.FactID{fact.ID},
			Evidence:       ev,
		}, RuleAdvisoryAffected, a.ID))
		b.warnf("Target %s is affected by %s, severity %s; patched versions: %s", toTag, advisoryLabel(a), severity, patched)
	}
}
