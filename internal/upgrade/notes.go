package upgrade

import (
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// noteGroup collects note items whose normalised text is identical (an
// original change and its backports, or the same item in release notes and
// the upgrade guide).
type noteGroup struct {
	key     string
	release string
	items   []domain.NoteItem
}

// noteRank prefers breaking, then action-required, then declared items.
func noteRank(it domain.NoteItem) int {
	r := 0
	if it.Breaking {
		r += 4
	}
	if it.ActionRequired {
		r += 2
	}
	if it.Classification.Method == domain.MethodDeclared {
		r++
	}
	return r
}

// noteChanges turns the note items of every path release (not From's) into
// Changes, merging duplicates.
func (b *builder) noteChanges() {
	groups := map[string]*noteGroup{}
	var order []*noteGroup
	for _, r := range b.releases {
		for _, it := range r.Notes {
			text := strings.TrimSpace(it.Text)
			if text == "" {
				continue
			}
			rel := it.Release
			if rel == "" {
				rel = r.Version.Semver
			}
			it.Release = rel
			key := normalizeNoteText(text)
			if key == "" {
				key = text
			}
			g := groups[key]
			if g == nil {
				g = &noteGroup{key: key, release: rel}
				groups[key] = g
				order = append(order, g)
			} else if compareReleaseStrings(rel, g.release) < 0 {
				g.release = rel
			}
			g.items = append(g.items, it)
		}
	}
	invalid := 0
	for _, g := range order {
		primary := g.items[0]
		for _, it := range g.items[1:] {
			if noteRank(it) > noteRank(primary) {
				primary = it
			}
		}
		if err := primary.Classification.Validate(); err != nil || !primary.Classification.Deterministic() {
			invalid++
			continue
		}
		c := domain.Change{
			Category:   primary.Category,
			Release:    g.release,
			Provenance: primary.Classification,
		}
		title, more := noteTitle(primary.Text)
		c.Title = title
		if more {
			c.Detail = strings.TrimSpace(primary.Text)
		}
		refSeen := map[string]bool{}
		for _, it := range g.items {
			c.Breaking = c.Breaking || it.Breaking
			c.ActionRequired = c.ActionRequired || it.ActionRequired
			c.Evidence = appendUniqueIDs(c.Evidence, it.Evidence...)
			for _, ref := range it.References {
				k := ref.Type + "\x00" + ref.ID
				if refSeen[k] {
					continue
				}
				refSeen[k] = true
				c.References = append(c.References, ref)
			}
		}
		b.addChange(c, "note", g.key)
	}
	if invalid > 0 {
		b.warnf("%s skipped because their classification provenance is invalid", plural(invalid, "release-note item was", "release-note items were"))
	}
}
