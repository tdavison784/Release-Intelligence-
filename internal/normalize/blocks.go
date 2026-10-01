package normalize

import (
	"regexp"
	"strings"
)

// Block segmentation of the own content of one markdown section: paragraphs,
// block quotes, top-level list items (with their continuation lines and nested
// items), code blocks and tables.

type blockKind uint8

const (
	bPara blockKind = iota
	bQuote
	bList // exactly one top-level list item
	bCode
	bTable
)

type block struct {
	kind   blockKind
	lo, hi int    // line index range [lo, hi) in document.lines
	text   string // cleaned inline text (paragraphs, quotes, list items)
	raw    string // text with markdown intact, code blocks removed (for label/reference detection)
	first  string // markdown-intact first line without list marker (labels)
	last   int    // index of the last non-blank line of the block
}

var (
	listMarkerRe = regexp.MustCompile(`^( {0,3})([-*+]|\d{1,9}[.)])( +|$)`)
	tableDelimRe = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(\|\s*:?-+:?\s*)*\|?\s*$`)
	quotePrefix  = regexp.MustCompile(`^ {0,3}>[ ]?`)
	leadInRe     = regexp.MustCompile(`(?i)^(changes|changelog) since\b`)
)

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

// blankLike reports whether the line separates blocks.
func blankLike(l docLine) bool {
	switch l.kind {
	case lkBlank, lkFrontMatter:
		return true
	case lkText:
		return strings.TrimSpace(l.vis) == ""
	}
	return false
}

// listStart parses a list marker at the start of the line.
func listStart(l docLine) (indent, contentCol int, ok bool) {
	if l.kind != lkText {
		return 0, 0, false
	}
	m := listMarkerRe.FindStringSubmatch(l.vis)
	if m == nil {
		return 0, 0, false
	}
	indent = len(m[1])
	pad := len(m[3])
	if pad > 4 {
		pad = 1 // indented code after the marker: content column is marker+1
	}
	if strings.TrimSpace(l.vis[len(m[0]):]) == "" && pad == 0 {
		pad = 1
	}
	return indent, indent + len(m[2]) + pad, true
}

func (d *document) tableStart(i, hi int) bool {
	if i+1 >= hi {
		return false
	}
	a, b := d.lines[i], d.lines[i+1]
	if a.kind != lkText || b.kind != lkText {
		return false
	}
	if !strings.Contains(a.vis, "|") || !strings.Contains(b.vis, "|") || !tableDelimRe.MatchString(b.vis) {
		return false
	}
	return len(splitTableRow(a.vis)) == len(splitTableRow(b.vis))
}

// blockify splits the lines [lo, hi) into blocks.
func (d *document) blockify(lo, hi int) []block {
	var out []block
	i := lo
	for i < hi {
		l := d.lines[i]
		if blankLike(l) {
			i++
			continue
		}
		switch l.kind {
		case lkFence, lkShortcodeCode:
			j := i
			for j < hi && (d.lines[j].kind == l.kind) {
				j++
			}
			out = append(out, block{kind: bCode, lo: i, hi: j, last: j - 1})
			i = j
			continue
		case lkHeading:
			// headings inside the range being blockified (used for flattened
			// fragments) become one-line paragraphs
			out = append(out, d.paragraphBlock(i, i+1, bPara))
			i++
			continue
		}
		if d.tableStart(i, hi) {
			j := i + 2
			for j < hi && d.lines[j].kind == lkText && strings.Contains(d.lines[j].vis, "|") {
				j++
			}
			out = append(out, block{kind: bTable, lo: i, hi: j, last: j - 1})
			i = j
			continue
		}
		if _, _, ok := listStart(l); ok {
			b, next := d.listItem(i, hi)
			out = append(out, b)
			i = next
			continue
		}
		// paragraph or quote
		isQuote := quotePrefix.MatchString(l.vis)
		j := i + 1
		for j < hi {
			n := d.lines[j]
			if n.kind != lkText || blankLike(n) {
				break
			}
			if _, _, ok := listStart(n); ok {
				break
			}
			if quotePrefix.MatchString(n.vis) != isQuote {
				break
			}
			if isQuote && strings.TrimSpace(quotePrefix.ReplaceAllString(n.vis, "")) == "" {
				break
			}
			j++
		}
		kind := bPara
		if isQuote {
			kind = bQuote
		}
		out = append(out, d.paragraphBlock(i, j, kind))
		i = j
	}
	return out
}

func (d *document) paragraphBlock(lo, hi int, kind blockKind) block {
	var parts []string
	for i := lo; i < hi; i++ {
		v := d.lines[i].vis
		if kind == bQuote {
			for quotePrefix.MatchString(v) {
				v = quotePrefix.ReplaceAllString(v, "")
			}
		}
		parts = append(parts, strings.TrimSpace(v))
	}
	raw := collapseSpace(strings.Join(parts, " "))
	return block{kind: kind, lo: lo, hi: hi, last: hi - 1, raw: raw, first: raw, text: cleanInline(raw)}
}

// listItem consumes one top-level list item starting at line i.
func (d *document) listItem(i, hi int) (block, int) {
	l := d.lines[i]
	indent, col, _ := listStart(l)
	nestedMin := indent + 2
	end := i + 1 // exclusive
	j := i + 1
	for j < hi {
		n := d.lines[j]
		if blankLike(n) {
			// look ahead to the next non-blank line: the item continues only
			// when that line is indented (or is a Hugo shortcode block, which
			// is conventionally written at column 0 even inside list items)
			k := j
			for k < hi && blankLike(d.lines[k]) {
				k++
			}
			if k >= hi {
				break
			}
			nl := d.lines[k]
			cont := false
			switch nl.kind {
			case lkShortcodeCode:
				cont = true
			case lkFence:
				cont = leadingSpaces(nl.raw) >= nestedMin
			case lkText:
				cont = leadingSpaces(nl.vis) >= minInt(col, nestedMin)
			}
			if !cont {
				break
			}
			j = k
			continue
		}
		switch n.kind {
		case lkHeading:
			j = hi
			continue
		case lkShortcodeCode:
			for j < hi && d.lines[j].kind == lkShortcodeCode {
				j++
			}
			end = j
			continue
		case lkFence:
			if leadingSpaces(n.raw) < nestedMin {
				j = hi
				continue
			}
			for j < hi && d.lines[j].kind == lkFence {
				j++
			}
			end = j
			continue
		}
		ind := leadingSpaces(n.vis)
		if _, _, ok := listStart(n); ok && ind < col && ind <= 3 {
			break
		}
		if ind < col && quotePrefix.MatchString(n.vis) && ind < nestedMin {
			break
		}
		if d.tableStart(j, hi) && ind < nestedMin {
			break
		}
		j++
		end = j
	}

	// assemble
	var raws, texts []string
	for k := i; k < end; k++ {
		ln := d.lines[k]
		if ln.kind != lkText || strings.TrimSpace(ln.vis) == "" {
			continue
		}
		v := ln.vis
		if k == i {
			v = v[len(listMarkerRe.FindString(v)):]
		}
		texts = append(texts, strings.TrimSpace(v))
		raws = append(raws, strings.TrimSpace(v))
	}
	raw := collapseSpace(strings.Join(raws, " "))
	first := ""
	if len(raws) > 0 {
		first = raws[0]
	}
	return block{
		kind: bList, lo: i, hi: end, last: d.lastContentLine(i, end),
		raw: raw, first: first, text: cleanInline(strings.Join(texts, " ")),
	}, end
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// splitTableRow splits one GFM table row into trimmed cells. Leading and
// trailing pipes are optional and "\|" is a literal pipe (unescaped in the
// result).
func splitTableRow(line string) []string {
	s := strings.TrimSpace(line)
	var cells []string
	var cur strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s) && s[i+1] == '|':
			cur.WriteByte('|')
			i++
		case c == '|':
			cells = append(cells, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteByte(c)
		}
	}
	cells = append(cells, strings.TrimSpace(cur.String()))
	if strings.HasPrefix(s, "|") && len(cells) > 0 {
		cells = cells[1:]
	}
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, `\|`) && len(cells) > 0 {
		cells = cells[:len(cells)-1]
	}
	return cells
}
