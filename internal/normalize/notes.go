package normalize

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
)

// maxItemText bounds NoteItem.Text.
const maxItemText = 1200

// sectionSep joins heading path elements in NoteItem.Section.
const sectionSep = " › "

// rawItem is an atomic note item before classification.
type rawItem struct {
	startIdx, endIdx int // line indices (inclusive) in the scanned document
	path             []string
	text             string // cleaned text
	raw              string // markdown-intact text without code blocks (references, labels)
	label            string // first line without list marker, or the heading
	excerpt          string // verbatim lines
}

type noteParser struct {
	d     *document
	items []rawItem
}

func parseNotes(in DocInput, rules []catalog.ClassifyRule) ([]domain.NoteItem, []domain.Evidence, error) {
	cls, err := newClassifier(rules)
	if err != nil {
		return nil, nil, err
	}
	d := scanDocument(in.Content)
	root, flat := d.buildSections()
	p := &noteParser{d: d}
	p.walk(root, len(flat) == 0)

	b := newNoteBuilder(in, cls)
	for _, it := range p.items {
		b.add(it, d, classInput{})
	}
	return b.items, b.evidence, nil
}

// noteBuilder turns raw items into classified NoteItems with evidence,
// merging items that share an ID.
type noteBuilder struct {
	in       DocInput
	cls      *classifier
	items    []domain.NoteItem
	evidence []domain.Evidence
	byID     map[string]int
}

func newNoteBuilder(in DocInput, cls *classifier) *noteBuilder {
	return &noteBuilder{in: in, cls: cls, byID: map[string]int{}}
}

// add classifies one markdown item. base carries YAML specific input (unused
// for markdown).
func (b *noteBuilder) add(it rawItem, d *document, base classInput) {
	in := b.in
	section := strings.Join(it.path, sectionSep)
	locator := fmt.Sprintf("L%d-L%d", d.lines[it.startIdx].n+in.LineOffset, d.lines[it.endIdx].n+in.LineOffset)
	if section != "" {
		locator += " (" + section + ")"
	}
	b.addResolved(it.path, it.text, it.raw, it.label, locator, it.excerpt, base, nil)
}

// addResolved classifies and records one item whose evidence locator is
// already known. extraRefs are appended after the references found in raw.
func (b *noteBuilder) addResolved(path []string, text, raw, label, locator, excerpt string, base classInput, extraRefs []domain.Reference) {
	in := b.in
	if text == "" {
		return
	}
	ci := base
	ci.role = in.Role
	ci.path = path
	ci.text = text
	ci.raw = raw
	ci.label = label
	v := b.cls.classify(ci)
	if v.skip {
		return
	}
	section := strings.Join(path, sectionSep)
	ev := domain.NewEvidence(domain.EvidenceDocument, in.SourceID, in.URI, locator, excerpt, in.Digest, in.RetrievedAt)
	id := "note-" + domain.ShortHash(in.Release, in.SourceID, text)
	if idx, dup := b.byID[id]; dup {
		// identical text listed twice in the same release: keep the first
		// classification, remember the second location
		existing := &b.items[idx]
		have := false
		for _, e := range existing.Evidence {
			if e == ev.ID {
				have = true
			}
		}
		if !have {
			existing.Evidence = append(existing.Evidence, ev.ID)
			b.evidence = append(b.evidence, ev)
		}
		return
	}
	refs := ExtractReferences(raw, in.Repository)
	refs = mergeReferences(refs, extraRefs)
	b.byID[id] = len(b.items)
	b.items = append(b.items, domain.NoteItem{
		ID:             id,
		Release:        in.Release,
		SourceID:       in.SourceID,
		Role:           in.Role,
		Section:        section,
		Text:           text,
		Category:       v.category,
		Breaking:       v.breaking,
		ActionRequired: v.actionRequired,
		References:     refs,
		Classification: v.prov,
		Evidence:       []domain.EvidenceID{ev.ID},
	})
	b.evidence = append(b.evidence, ev)
}

func mergeReferences(a, b []domain.Reference) []domain.Reference {
	if len(b) == 0 {
		return a
	}
	seen := map[string]bool{}
	for _, r := range a {
		seen[r.Type+"\x00"+r.ID] = true
	}
	for _, r := range b {
		k := r.Type + "\x00" + r.ID
		if !seen[k] {
			seen[k] = true
			a = append(a, r)
		}
	}
	return a
}

var markerOnlyRe = regexp.MustCompile(`(?i)^[^\w]*(?:potentially\s+)?breaking(?:\s+changes?)?[.!]?$`)

func (b block) isLeadIn() bool {
	if b.kind != bPara {
		return false
	}
	t := strings.TrimSpace(b.text)
	if leadInRe.MatchString(t) {
		return true
	}
	// a short single sentence ending in a colon introduces what follows
	return (strings.HasSuffix(t, ":") || strings.HasSuffix(t, "\uff1a")) && len(t) <= 200 && !strings.Contains(t, ". ")
}

func (b block) isMarkerOnly() bool {
	return (b.kind == bQuote || b.kind == bPara) && markerOnlyRe.MatchString(strings.TrimSpace(b.text))
}

func (b block) isProse() bool { return (b.kind == bPara || b.kind == bQuote) && b.text != "" }

// walk emits the items of section s and recurses into its children.
//
//   - A section that starts with a list (ignoring lead-in lines such as
//     "Changes since v1.2.3:") is changelog-like: every top-level list item is
//     an item.
//   - A section with prose that has no category-heading descendants is
//     prose-like: ONE item whose text is "Heading: first paragraphs", folding
//     in its lists and all of its sub-sections (details such as "Detection" /
//     "Remediation").
//   - A section with category-heading descendants (Feature, Bug fixes, ...)
//     is a container: its sub-sections hold the items; its own prose, when
//     substantive, becomes an introductory item (not for level-1 titles).
func (p *noteParser) walk(s *section, noHeadings bool) {
	d := p.d
	blocks := d.blockify(s.ownStart, s.ownEnd)

	listLike, hasList, hasProse, hasSubstantive, decided := false, false, false, false, false
	leadIn, proseAfterList := false, false
	for _, b := range blocks {
		if b.kind == bList {
			hasList = true
		}
		if b.isProse() {
			hasProse = true
			if !b.isLeadIn() && !b.isMarkerOnly() {
				hasSubstantive = true
				if listLike {
					proseAfterList = true
				}
			}
		}
		if decided || b.kind == bCode || b.kind == bTable || b.isLeadIn() || b.isMarkerOnly() {
			if !decided && b.isLeadIn() && !leadInRe.MatchString(strings.TrimSpace(b.text)) {
				leadIn = true
			}
			continue
		}
		// the first block that is not a lead-in line decides the style
		listLike, decided = b.kind == bList, true
	}
	// "Explanation: - point - point  closing remarks" is one prose item, not a
	// changelog list
	if listLike && leadIn && proseAfterList {
		listLike = false
	}

	switch {
	case s.level == 0:
		if listLike || (noHeadings && hasList) {
			p.emitLists(s, blocks)
		} else if noHeadings && hasProse {
			p.emitProse(s, false, blocks)
		}
	case !s.container && !listLike && hasProse:
		p.emitProse(s, true, blocks)
		return
	case listLike:
		p.emitLists(s, blocks)
	case s.container && s.level >= 2 && hasSubstantive:
		p.emitProse(s, false, blocks)
	}
	for _, c := range s.children {
		p.walk(c, false)
	}
}

func (p *noteParser) emitLists(s *section, blocks []block) {
	for _, b := range blocks {
		if b.kind != bList || b.text == "" {
			continue
		}
		p.items = append(p.items, rawItem{
			startIdx: b.lo,
			endIdx:   b.last,
			path:     s.path,
			text:     truncateText(b.text, maxItemText),
			raw:      b.raw,
			label:    b.first,
			excerpt:  p.d.rawLines(b.lo, b.last),
		})
	}
}

// emitProse emits the single prose item of s. With absorb, the whole subtree
// of s is folded into it.
func (p *noteParser) emitProse(s *section, absorb bool, blocks []block) {
	d := p.d
	var parts, raws []string
	lastIdx := -1
	collect := func(bs []block, intro bool) {
		for _, b := range bs {
			switch {
			case b.kind == bPara || b.kind == bQuote:
				if b.text == "" {
					continue
				}
				raws = append(raws, b.raw)
				if b.isMarkerOnly() {
					lastIdx = maxInt(lastIdx, b.last)
					continue
				}
				if intro && b.isLeadIn() {
					continue
				}
				parts = append(parts, b.text)
				lastIdx = maxInt(lastIdx, b.last)
			case b.kind == bList:
				if b.text == "" {
					continue
				}
				raws = append(raws, b.raw)
				parts = append(parts, "- "+b.text)
				lastIdx = maxInt(lastIdx, b.last)
			}
		}
	}
	collect(blocks, !absorb && s.container)
	if absorb {
		var rec func(c *section)
		rec = func(c *section) {
			sub := d.blockify(c.ownStart, c.ownEnd)
			var sparts []string
			before := len(parts)
			parts = append(parts, "") // placeholder for the heading
			collect(sub, false)
			sparts = parts[before+1:]
			if len(sparts) == 0 {
				parts = parts[:before]
			} else {
				parts[before] = headingLabel(c.heading)
			}
			raws = append(raws, c.hraw)
			for _, cc := range c.children {
				rec(cc)
			}
		}
		for _, c := range s.children {
			rec(c)
		}
	}
	if len(parts) == 0 {
		return
	}
	text := joinHeading(s.heading, strings.Join(parts, " "))
	text = truncateText(collapseSpace(text), maxItemText)

	start := s.hLine
	if s.level == 0 {
		start = firstBlockLine(blocks)
	}
	end := lastIdx
	if absorb {
		end = d.lastContentLine(maxInt(start, 0), s.treeEnd)
	}
	if start < 0 || end < start {
		return
	}
	raw := collapseSpace(strings.Join(append([]string{s.hraw}, raws...), " "))
	p.items = append(p.items, rawItem{
		startIdx: start,
		endIdx:   end,
		path:     s.path,
		text:     text,
		raw:      raw,
		label:    s.hraw,
		excerpt:  d.rawLines(start, end),
	})
}

func firstBlockLine(bs []block) int {
	for _, b := range bs {
		return b.lo
	}
	return -1
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// headingLabel renders a sub-heading as a label inside folded text.
func headingLabel(h string) string {
	h = cleanInline(h)
	if h == "" || strings.ContainsAny(h[len(h)-1:], ".:!?") {
		return h
	}
	return h + ":"
}

// joinHeading renders "Heading: body", avoiding a doubled punctuation mark.
func joinHeading(heading, body string) string {
	heading = cleanInline(heading)
	if heading == "" {
		return body
	}
	if body == "" {
		return heading
	}
	if strings.ContainsAny(heading[len(heading)-1:], ".:!?") {
		return heading + " " + body
	}
	return heading + ": " + body
}

// rawLines returns the verbatim lines lo..hi (inclusive).
func (d *document) rawLines(lo, hi int) string {
	var out []string
	for i := lo; i <= hi && i < len(d.lines); i++ {
		out = append(out, d.lines[i].raw)
	}
	return strings.Join(out, "\n")
}

// flattenMarkdown converts a markdown fragment (a YAML release-note entry, an
// upgrade note body) into plain text, dropping code blocks.
func flattenMarkdown(src string) (text, raw, first string) {
	d := scanDocument([]byte(src))
	var texts, raws []string
	for _, b := range d.blockify(0, len(d.lines)) {
		switch b.kind {
		case bPara, bQuote, bList:
			if b.text == "" {
				continue
			}
			if first == "" {
				first = b.first
			}
			t := b.text
			if b.kind == bList {
				t = "- " + t
			}
			texts = append(texts, t)
			raws = append(raws, b.raw)
		}
	}
	return collapseSpace(strings.Join(texts, " ")), collapseSpace(strings.Join(raws, " ")), first
}
