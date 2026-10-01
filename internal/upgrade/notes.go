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
	byTitle := map[string][]*noteGroup{} // release + heading title → candidate groups
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
			// The same change is sometimes published by two different
			// sources with different wording: a structured release-note entry
			// {title, content} and the hand-edited upgrade-notes section under
			// the same heading. Merge those by title (see titleMergeable).
			title := headingTitle(text)
			tkey := ""
			if title != "" {
				tkey = rel + "\x00" + title
				if g == nil {
					for _, cand := range byTitle[tkey] {
						if titleMergeable(cand, it, title) {
							g = cand
							break
						}
					}
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
			if tkey != "" && !containsGroup(byTitle[tkey], g) {
				byTitle[tkey] = append(byTitle[tkey], g)
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

func containsGroup(gs []*noteGroup, g *noteGroup) bool {
	for _, x := range gs {
		if x == g {
			return true
		}
	}
	return false
}

// sectionSep joins heading path elements in NoteItem.Section (the separator
// used by normalize).
const sectionSep = " › "

// sectionLeaf returns the alnum key of the last element of a Section path.
func sectionLeaf(section string) string {
	if i := strings.LastIndex(section, sectionSep); i >= 0 {
		section = section[i+len(sectionSep):]
	}
	return alnumKey(normalizeNoteText(section))
}

// titleMergeable reports whether it, whose "Title: body" prefix has the alnum
// key title, may join g by title instead of by identical text. A "Prefix:"
// title is common to unrelated items ("Helm chart: fixed X", "Potentially
// Breaking: ..."), so the merge is limited to the one legitimate case, the
// same change published by two different sources:
//
//   - never within one source: g must hold no item from it.SourceID, and
//   - the title must be the heading the change is filed under: it equals the
//     last Section path element of it or of the item of g it is merged with
//     (the upgrade-notes page has "## Title" while the structured release note
//     is filed under its area).
func titleMergeable(g *noteGroup, it domain.NoteItem, title string) bool {
	for _, x := range g.items {
		if x.SourceID == it.SourceID {
			return false
		}
	}
	itLeaf := sectionLeaf(it.Section)
	for _, x := range g.items {
		if headingTitle(strings.TrimSpace(x.Text)) != title {
			continue
		}
		if title == itLeaf || title == sectionLeaf(x.Section) {
			return true
		}
	}
	return false
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
