package upgrade

import (
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Values diff rules.
const (
	RuleValuesRemoved        = "values:removed"
	RuleValuesSectionRemoved = "values:section-removed"
	RuleValuesDefaultChanged = "values:default-changed"
	RuleValuesAdded          = "values:added"
)

// maxDetailLines bounds enumerations inside a Detail (Subjects keep everything).
const maxDetailLines = 50

func topKey(k string) string {
	if i := strings.IndexByte(k, '.'); i >= 0 {
		return k[:i]
	}
	return k
}

// prefixes returns the ancestors of a dotted key, shallowest first, followed
// by the key itself: "a.b.c" → ["a", "a.b", "a.b.c"].
func prefixes(k string) []string {
	var out []string
	for i := 0; i < len(k); i++ {
		if k[i] == '.' {
			out = append(out, k[:i])
		}
	}
	return append(out, k)
}

func prefixSet(entries map[string]string) map[string]bool {
	out := map[string]bool{}
	for k := range entries {
		for _, p := range prefixes(k) {
			out[p] = true
		}
	}
	return out
}

// valuesChanges diffs the Helm values snapshots of every chart present on both sides.
func (b *builder) valuesChanges() {
	ids, from, to := b.snapshotPairs(domain.SnapshotHelmValues, "Helm values", func(s *domain.Snapshot) bool { return s.Values != nil })
	for _, id := range ids {
		b.diffValues(id, from[id], to[id], len(ids) > 1)
	}
}

func (b *builder) diffValues(artifactID string, f, t *domain.Snapshot, multi bool) {
	fe, te := f.Values.Entries, t.Values.Entries
	chart := t.Values.Chart
	if chart == "" {
		chart = artifactID
	}
	suffix := ""
	if multi {
		suffix = " (chart " + chart + ")"
	}
	evidence := append(append([]domain.EvidenceID{}, f.Evidence...), t.Evidence...)
	fromTag, toTag := b.from.Version.String(), b.to.Version.String()

	// prefixes present in To ("a", "a.b" for key "a.b.c" and the key itself)
	toPrefixes := prefixSet(te)
	// number of From keys at or below each prefix
	fromBelow := map[string]int{}
	for k := range fe {
		for _, p := range prefixes(k) {
			fromBelow[p]++
		}
	}

	// removed keys, grouped by the shallowest ancestor section that
	// disappeared completely; a removed top-level section is breaking.
	groups := map[string][]string{}
	for _, k := range sortedKeys(fe) {
		if toPrefixes[k] {
			continue // still present (or turned from a leaf into a section)
		}
		group := k
		for i, p := range prefixes(k) {
			if p == k {
				break
			}
			// a vanished top-level section always groups; deeper sections
			// only when they held more than one key
			if !toPrefixes[p] && (i == 0 || fromBelow[p] > 1) {
				group = p + ".*"
				break
			}
		}
		groups[group] = append(groups[group], k)
	}
	for _, g := range sortedKeys(groups) {
		keys := groups[g]
		if !strings.HasSuffix(g, ".*") {
			k := keys[0]
			detail := fmt.Sprintf("Values set at this key no longer take effect after the upgrade (or are rejected when the chart validates values against a schema). Default in %s: %s.", fromTag, fe[k])
			if c := strings.TrimSpace(f.Values.Comments[k]); c != "" {
				detail += "\nDocumentation in " + fromTag + ": " + c
			}
			b.addChange(domain.Change{
				Category:       domain.CategoryHelmValues,
				ActionRequired: true,
				Title:          fmt.Sprintf("Helm value %s removed%s", code(k), suffix),
				Detail:         detail,
				Subjects:       []string{k},
				Provenance:     computed(RuleValuesRemoved),
				Evidence:       evidence,
			}, RuleValuesRemoved, artifactID, k)
			continue
		}
		section := strings.TrimSuffix(g, ".*")
		var lines []string
		for i, k := range keys {
			if i == maxDetailLines {
				lines = append(lines, fmt.Sprintf("… and %d more", len(keys)-i))
				break
			}
			lines = append(lines, fmt.Sprintf("%s (default %s)", k, fe[k]))
		}
		topLevel := !strings.Contains(section, ".")
		rule, kind := RuleValuesRemoved, "Helm values"
		if topLevel {
			rule, kind = RuleValuesSectionRemoved, "Helm values section"
		}
		b.addChange(domain.Change{
			Category:       domain.CategoryHelmValues,
			Breaking:       topLevel,
			ActionRequired: true,
			Title:          fmt.Sprintf("%s %s removed (%s)%s", kind, code(g), plural(len(keys), "key", "keys"), suffix),
			Detail: fmt.Sprintf("%s of chart %s no longer exists in %s. Remove these keys from your values and migrate the configuration they controlled; values set here no longer take effect (or are rejected when the chart validates values against a schema).\nRemoved keys:\n%s",
				code(section), chart, toTag, strings.Join(lines, "\n")),
			Subjects:   keys,
			Provenance: computed(rule),
			Evidence:   evidence,
		}, rule, artifactID, g)
	}

	// changed defaults
	for _, k := range sortedKeys(fe) {
		nv, ok := te[k]
		if !ok || nv == fe[k] {
			continue
		}
		ov := fe[k]
		b.addChange(domain.Change{
			Category:   domain.CategoryHelmValues,
			Title:      fmt.Sprintf("Default of Helm value %s changed: %s → %s%s", code(k), shorten(ov, 50), shorten(nv, 50), suffix),
			Detail:     fmt.Sprintf("%s → %s", ov, nv),
			Subjects:   []string{k},
			Provenance: computed(RuleValuesDefaultChanged),
			Evidence:   evidence,
		}, RuleValuesDefaultChanged, artifactID, k)
	}

	// added keys, one change per top-level key
	topsFrom := map[string]bool{}
	for k := range fe {
		topsFrom[topKey(k)] = true
	}
	addedByTop := map[string][]string{}
	for _, k := range sortedKeys(te) {
		if _, ok := fe[k]; ok {
			continue
		}
		addedByTop[topKey(k)] = append(addedByTop[topKey(k)], k)
	}
	for _, top := range sortedKeys(addedByTop) {
		keys := addedByTop[top]
		sort.Strings(keys)
		var title string
		switch {
		case len(keys) == 1:
			title = fmt.Sprintf("New Helm value %s", code(keys[0]))
		case !topsFrom[top]:
			title = fmt.Sprintf("New Helm values section %s (%d values)", code(top+".*"), len(keys))
		default:
			title = fmt.Sprintf("%d new Helm values under %s", len(keys), code(top+".*"))
		}
		var lines []string
		for i, k := range keys {
			if i == maxDetailLines {
				lines = append(lines, fmt.Sprintf("… and %d more", len(keys)-i))
				break
			}
			line := fmt.Sprintf("%s = %s", k, te[k])
			if c := strings.TrimSpace(t.Values.Comments[k]); c != "" {
				line += "  # " + firstLine(c)
			}
			lines = append(lines, line)
		}
		b.addChange(domain.Change{
			Category:   domain.CategoryHelmValues,
			Title:      title + suffix,
			Detail:     "Defaults in " + toTag + ":\n" + strings.Join(lines, "\n"),
			Subjects:   keys,
			Provenance: computed(RuleValuesAdded),
			Evidence:   evidence,
		}, RuleValuesAdded, artifactID, top)
	}
}
