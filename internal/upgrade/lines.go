package upgrade

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// Lines diff rules.
const (
	RuleLinesAdded   = "lines:added"
	RuleLinesRemoved = "lines:removed"
)

// maxLineChanges bounds the changes one lines snapshot pair emits per
// direction; the rest is summarised in one more change (the lines themselves
// stay in the snapshots).
const maxLineChanges = 25

// lineChanges diffs the lines snapshots (the statements an artifact makes
// about itself, selected by a declared pattern) of every artifact present on
// both sides. A line only one release has is one change: a statement that
// stays put is no change.
func (b *builder) lineChanges() {
	ids, from, to := b.snapshotPairs(domain.SnapshotLines, "Matching lines", func(s *domain.Snapshot) bool { return s.Lines != nil })
	for _, id := range ids {
		f, t := from[id], to[id]
		evidence := append(append([]domain.EvidenceID{}, f.Evidence...), t.Evidence...)
		removed, added := diffStrings(f.Lines.Lines, t.Lines.Lines)
		label := t.Lines.Source
		fromTag, toTag := b.from.Version.String(), b.to.Version.String()
		emit := func(lines []string, rule, verb, where string) {
			for i, l := range lines {
				if i == maxLineChanges {
					b.addChange(domain.Change{
						Category:   domain.CategoryOther,
						Title:      fmt.Sprintf("%d more lines %s %s", len(lines)-i, verb, label),
						Detail:     strings.Join(lines[i:], "\n"),
						Subjects:   lines[i:],
						Provenance: computed(rule),
						Evidence:   evidence,
					}, rule, id, "more")
					return
				}
				b.addChange(domain.Change{
					Category:   domain.CategoryOther,
					Title:      fmt.Sprintf("Line %s %s %s: %s", verb, where, label, shorten(l, 160)),
					Detail:     fmt.Sprintf("%s\n(a line of %s matching `%s`, %s %s %s)", l, label, t.Lines.Pattern, verb, where, toTag),
					Subjects:   []string{l},
					Provenance: computed(rule),
					Evidence:   evidence,
				}, rule, id, l)
			}
		}
		_ = fromTag
		emit(added, RuleLinesAdded, "added", "to")
		emit(removed, RuleLinesRemoved, "removed", "from")
	}
}
