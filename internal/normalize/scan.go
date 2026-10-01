package normalize

import (
	"regexp"
	"strings"
)

// This file contains the line scanner shared by every markdown consumer in
// the package (sections, notes, tables). It classifies each line of a document
// once so that later stages never have to track fences, comments or front
// matter themselves, and never lose the original 1-based line numbering.

type lineKind uint8

const (
	lkText          lineKind = iota // ordinary text (paragraph, list item, table row, quote, ...)
	lkFrontMatter                   // YAML/TOML front matter including its delimiters
	lkFence                         // fenced code block, delimiters and content
	lkShortcodeCode                 // Hugo {{< text >}} ... {{< /text >}} block, delimiters and content
	lkBlank                         // blank, comment-only, shortcode-only, link reference definition, thematic break
	lkHeading                       // ATX heading
)

// docLine is one classified line of a document.
type docLine struct {
	n    int // 1-based line number in the scanned content
	raw  string
	vis  string // raw without comments / shortcode tags (meaningful for lkText and lkHeading)
	kind lineKind

	level int    // heading level 1..6 (lkHeading)
	head  string // normalised heading text (lkHeading)
	hraw  string // heading text without leading/trailing '#' but otherwise verbatim (lkHeading)
}

type document struct {
	lines []docLine
}

var (
	atxHeadingRe   = regexp.MustCompile(`^ {0,3}(#{1,6})(?:[ \t]+(.*?))?[ \t]*$`)
	closingHashes  = regexp.MustCompile(`[ \t]+#+[ \t]*$`)
	fenceOpenRe    = regexp.MustCompile("^\\s*(`{3,}|~{3,})(.*)$")
	scCodeOpenRe   = regexp.MustCompile(`^\s*\{\{[<%]\s*text\b`)
	scCodeCloseRe  = regexp.MustCompile(`\{\{[<%]\s*/\s*text\b`)
	shortcodeTagRe = regexp.MustCompile(`\{\{[<%].*?[>%]\}\}`)
	tipOpenRe      = regexp.MustCompile(`\{\{[<%]\s*(?:tip|idea)\b[^>%]*[>%]\}\}`)
	tipCloseRe     = regexp.MustCompile(`\{\{[<%]\s*/\s*(?:tip|idea)\b[^>%]*[>%]\}\}`)
	tipSpanRe      = regexp.MustCompile(`\{\{[<%]\s*(?:tip|idea)\b[^>%]*[>%]\}\}.*?\{\{[<%]\s*/\s*(?:tip|idea)\b[^>%]*[>%]\}\}`)
	linkRefDefRe   = regexp.MustCompile(`^ {0,3}\[[^\]]+\]:\s+\S+`)
	tagOnlyLineRe  = regexp.MustCompile(`^\s*(?:</?[A-Za-z][^>]*>\s*)+$`)
	fmKeyRe        = regexp.MustCompile(`^[A-Za-z0-9_"'-][^:=]*[:=]`)
	anchorSuffixRe = regexp.MustCompile(`\s*\{[#:.][^}]*\}\s*$`)
)

// scanDocument classifies every line of md. Line numbers are those of md.
func scanDocument(md []byte) *document {
	text := string(md)
	text = strings.TrimPrefix(text, "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	raw := strings.Split(text, "\n")
	if len(raw) > 0 && raw[len(raw)-1] == "" {
		raw = raw[:len(raw)-1]
	}
	d := &document{lines: make([]docLine, len(raw))}
	for i, r := range raw {
		d.lines[i] = docLine{n: i + 1, raw: r, vis: r}
	}

	start := d.markFrontMatter()

	var (
		inFence    bool
		fenceCh    byte
		fenceLen   int
		inShortcut bool
		inComment  string // "" | "html" | "mdx"
		inTip      bool   // inside a Hugo {{< tip >}} aside, which is not release-note content
	)
	for i := start; i < len(d.lines); i++ {
		l := &d.lines[i]
		line := expandLeadingTabs(l.raw)

		if inFence {
			l.kind = lkFence
			t := strings.TrimSpace(line)
			if len(t) >= fenceLen && strings.Trim(t, string(fenceCh)) == "" {
				inFence = false
			}
			continue
		}
		if inShortcut {
			l.kind = lkShortcodeCode
			if scCodeCloseRe.MatchString(line) {
				inShortcut = false
			}
			continue
		}

		if inTip {
			l.kind, l.vis = lkBlank, ""
			if tipCloseRe.MatchString(line) {
				inTip = false
			}
			continue
		}
		if tipSpanRe.MatchString(line) {
			line = tipSpanRe.ReplaceAllString(line, "")
		} else if tipOpenRe.MatchString(line) {
			l.kind, l.vis = lkBlank, ""
			inTip = true
			continue
		}

		startedInComment := inComment != ""
		vis, still := stripComments(line, inComment)
		inComment = still
		l.vis = vis

		if !startedInComment && inComment == "" {
			if m := fenceOpenRe.FindStringSubmatch(line); m != nil && !(m[1][0] == '`' && strings.Contains(m[2], "`")) {
				inFence, fenceCh, fenceLen = true, m[1][0], len(m[1])
				l.kind = lkFence
				continue
			}
			if scCodeOpenRe.MatchString(line) {
				l.kind = lkShortcodeCode
				if !scCodeCloseRe.MatchString(line) {
					inShortcut = true
				}
				continue
			}
		}

		vis = shortcodeTagRe.ReplaceAllString(vis, "")
		l.vis = vis
		switch {
		case strings.TrimSpace(vis) == "":
			l.kind = lkBlank
		case linkRefDefRe.MatchString(vis), tagOnlyLineRe.MatchString(vis), isThematicBreak(vis):
			l.kind = lkBlank
		default:
			if m := atxHeadingRe.FindStringSubmatch(vis); m != nil {
				l.kind = lkHeading
				l.level = len(m[1])
				h := strings.TrimSpace(m[2])
				h = closingHashes.ReplaceAllString(h, "")
				h = strings.TrimSpace(h)
				if strings.Trim(h, "#") == "" {
					h = ""
				}
				l.hraw = h
				l.head = normalizeHeading(h)
			} else {
				l.kind = lkText
			}
		}
	}
	return d
}

// markFrontMatter marks a leading YAML ("---") or TOML ("+++") front matter
// block and returns the index of the first line after it.
func (d *document) markFrontMatter() int {
	if len(d.lines) == 0 {
		return 0
	}
	open := strings.TrimRight(d.lines[0].raw, " \t")
	var closers []string
	switch open {
	case "---":
		closers = []string{"---", "..."}
	case "+++":
		closers = []string{"+++"}
	default:
		return 0
	}
	for j := 1; j < len(d.lines); j++ {
		t := strings.TrimRight(d.lines[j].raw, " \t")
		for _, c := range closers {
			if t == c {
				// require something that looks like a key to avoid mistaking two
				// thematic breaks for front matter
				ok := j == 1
				for k := 1; k < j && !ok; k++ {
					ok = fmKeyRe.MatchString(d.lines[k].raw)
				}
				if !ok {
					return 0
				}
				for k := 0; k <= j; k++ {
					d.lines[k].kind = lkFrontMatter
				}
				return j + 1
			}
		}
	}
	return 0
}

// isThematicBreak reports whether the line is "---", "***" or "___" (three or
// more of one character, optionally separated by spaces).
func isThematicBreak(s string) bool {
	if leadingSpaces(s) > 3 {
		return false
	}
	t := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	if len(t) < 3 {
		return false
	}
	c := t[0]
	if c != '-' && c != '*' && c != '_' {
		return false
	}
	return strings.Trim(t, string(c)) == ""
}

func expandLeadingTabs(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	if !strings.Contains(s[:i], "\t") {
		return s
	}
	var b strings.Builder
	col := 0
	for _, c := range s[:i] {
		if c == '\t' {
			n := 4 - col%4
			b.WriteString(strings.Repeat(" ", n))
			col += n
		} else {
			b.WriteRune(c)
			col++
		}
	}
	b.WriteString(s[i:])
	return b.String()
}

// stripComments removes HTML (<!-- -->) and MDX ({/* */}) comments from one
// line. state is the comment kind still open from the previous line; the
// returned state is the one still open after this line.
func stripComments(line, state string) (string, string) {
	var b strings.Builder
	rest := line
	for {
		if state != "" {
			end := "-->"
			if state == "mdx" {
				end = "*/}"
			}
			i := strings.Index(rest, end)
			if i < 0 {
				return b.String(), state
			}
			rest = rest[i+len(end):]
			state = ""
			continue
		}
		h := strings.Index(rest, "<!--")
		m := strings.Index(rest, "{/*")
		switch {
		case h < 0 && m < 0:
			b.WriteString(rest)
			return b.String(), ""
		case m < 0 || (h >= 0 && h < m):
			b.WriteString(rest[:h])
			rest = rest[h+4:]
			state = "html"
		default:
			b.WriteString(rest[:m])
			rest = rest[m+3:]
			state = "mdx"
		}
	}
}

// normalizeHeading turns heading source text into the form used for matching
// and for Section.Heading: trailing {#anchor} removed, links replaced by their
// text, surrounding emphasis/backticks trimmed, whitespace collapsed.
func normalizeHeading(h string) string {
	h = strings.TrimSpace(h)
	for {
		n := anchorSuffixRe.ReplaceAllString(h, "")
		if n == h {
			break
		}
		h = n
	}
	h = convertLinks(h)
	h = collapseSpace(h)
	for {
		n := trimSurroundingMarks(h)
		if n == h {
			break
		}
		h = n
	}
	return h
}

// trimSurroundingMarks removes one layer of **, __, *, _ or ` that wraps the
// entire string.
func trimSurroundingMarks(s string) string {
	s = strings.TrimSpace(s)
	for _, m := range []string{"**", "__", "*", "_", "`"} {
		if len(s) > 2*len(m) && strings.HasPrefix(s, m) && strings.HasSuffix(s, m) {
			inner := s[len(m) : len(s)-len(m)]
			if m == "`" && strings.Contains(inner, "`") {
				continue
			}
			if (m == "*" || m == "_") && strings.HasPrefix(inner, m) {
				continue
			}
			return strings.TrimSpace(inner)
		}
	}
	return s
}

// section is one node of the heading tree of a document.
type section struct {
	heading string // normalised heading text ("" for the synthetic root)
	hraw    string
	level   int // 0 for the synthetic root
	path    []string
	parent  *section

	hLine     int // index in document.lines of the heading line (-1 for root)
	ownStart  int // first own content line index (after the heading)
	ownEnd    int // exclusive end index of own content (next heading of any level)
	treeEnd   int // exclusive end index of the whole subtree
	children  []*section
	kindBelow bool // some descendant heading is a category heading
	// container: the sub-sections hold the items. True for sections with
	// category-heading descendants, for level-1 titles with sub-sections and
	// for grouping headings ("Major Themes") with sub-sections.
	container bool
}

// buildSections builds the heading tree. The returned root has level 0 and
// covers the preamble before the first heading; flat lists every heading
// section in document order.
func (d *document) buildSections() (root *section, flat []*section) {
	root = &section{level: 0, hLine: -1}
	var stack []*section
	stack = append(stack, root)
	n := len(d.lines)
	for i := range d.lines {
		l := d.lines[i]
		if l.kind != lkHeading {
			continue
		}
		for len(stack) > 1 && stack[len(stack)-1].level >= l.level {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		s := &section{heading: l.head, hraw: l.hraw, level: l.level, parent: parent, hLine: i, ownStart: i + 1}
		s.path = append(append([]string{}, parent.path...), l.head)
		parent.children = append(parent.children, s)
		stack = append(stack, s)
		flat = append(flat, s)
	}
	// own and subtree ends
	root.ownStart = 0
	root.ownEnd = n
	root.treeEnd = n
	if len(flat) > 0 {
		root.ownEnd = flat[0].hLine
	}
	for idx, s := range flat {
		if idx+1 < len(flat) {
			s.ownEnd = flat[idx+1].hLine
		} else {
			s.ownEnd = n
		}
		s.treeEnd = n
		for j := idx + 1; j < len(flat); j++ {
			if flat[j].level <= s.level {
				s.treeEnd = flat[j].hLine
				break
			}
		}
	}
	var mark func(s *section) bool
	mark = func(s *section) bool {
		below := false
		for _, c := range s.children {
			if isCategoryHeading(c.heading) {
				below = true
			}
			if mark(c) {
				below = true
			}
		}
		s.kindBelow = below
		return below
	}
	mark(root)
	for _, s := range flat {
		s.container = s.kindBelow || (len(s.children) > 0 && (s.level == 1 || isGroupingHeading(s.heading)))
	}
	return root, flat
}

// lastContentLine returns the index of the last non-blank line in [lo, hi),
// or lo-1 when there is none.
func (d *document) lastContentLine(lo, hi int) int {
	for i := hi - 1; i >= lo; i-- {
		switch d.lines[i].kind {
		case lkBlank:
			continue
		case lkText:
			if strings.TrimSpace(d.lines[i].vis) == "" {
				continue
			}
		}
		return i
	}
	return lo - 1
}

func selectSection(md []byte, re *regexp.Regexp) (*Section, error) {
	if re == nil {
		return nil, ErrNoMatch
	}
	d := scanDocument(md)
	_, flat := d.buildSections()
	for _, s := range flat {
		l := d.lines[s.hLine]
		if !(re.MatchString(s.heading) || re.MatchString(s.hraw) || re.MatchString(strings.TrimSpace(l.vis))) {
			continue
		}
		last := d.lastContentLine(s.hLine, s.treeEnd)
		if last < s.hLine {
			last = s.hLine
		}
		rawLines := make([]string, 0, last-s.hLine+1)
		for i := s.hLine; i <= last; i++ {
			rawLines = append(rawLines, d.lines[i].raw)
		}
		return &Section{
			Heading:   s.heading,
			Level:     s.level,
			Path:      append([]string{}, s.path...),
			StartLine: d.lines[s.hLine].n,
			EndLine:   d.lines[last].n,
			Body:      strings.Join(rawLines, "\n"),
		}, nil
	}
	return nil, ErrNoMatch
}
