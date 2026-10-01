package upgrade

import (
	"strings"
	"unicode"

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
	byTitle := map[string]*noteGroup{} // release + heading title → group
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
			key := alnumKey(normalizeNoteText(text))
			if key == "" {
				key = text
			}
			g := groups[key]
			// The same change often appears twice with different wording:
			// e.g. a structured release-note entry {title, content} and the
			// hand-edited upgrade-notes section under the same heading. Merge
			// heading-style items that share their title within a release.
			tkey := ""
			if t := headingTitle(text); t != "" {
				tkey = rel + "\x00" + t
				if g == nil {
					g = byTitle[tkey]
				}
			}
			if g == nil {
				g = &noteGroup{key: key, release: rel}
				groups[key] = g
				order = append(order, g)
			} else if compareReleaseStrings(rel, g.release) < 0 {
				g.release = rel
			}
			g.items = append(g.items, it)
			groups[key] = g
			if tkey != "" && byTitle[tkey] == nil {
				byTitle[tkey] = g
			}
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

// alnumKey reduces normalised text to lowercase letters and digits so that
// punctuation and markup differences do not defeat de-duplication.
func alnumKey(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// headingTitle returns the alnum key of a "Title: body" item's title when the
// title looks like a heading (2–12 words, no sentence punctuation), else "".
func headingTitle(text string) string {
	i := strings.Index(text, ": ")
	if i <= 0 || i > 140 {
		return ""
	}
	title := strings.TrimSpace(text[:i])
	if strings.ContainsAny(title, ".;!?") {
		return ""
	}
	if n := len(strings.Fields(title)); n < 2 || n > 12 {
		return ""
	}
	return alnumKey(normalizeNoteText(title))
}
