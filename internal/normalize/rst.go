package normalize

import (
	"regexp"
	"strings"
)

// RSTToMarkdown renders reStructuredText as markdown so that the markdown
// selectors and parsers apply to it. It exists for products whose upgrade
// notes or changelogs are rst sources in the repository (Cilium's
// Documentation/operations/upgrade.rst and upgrade-current.inc: "X.Y Upgrade
// Notes" section with "Action Required" / "Removed Options" sub-sections;
// Cilium's README.rst uses the same markup).
//
// The conversion is line-preserving: output line N renders input line N (the
// heading underline, the directive lines and the table borders that are
// consumed leave blank lines), so line ranges found in the markdown are line
// ranges of the original document and evidence locators stay valid.
//
//   - section titles become "#".."######" headings. The level follows rst
//     semantics: a style is an adornment character used either as an
//     underline or as overline+underline, and styles are ordered by first
//     use in the document ("=" before "-" before "~" ...). The underline
//     must be at least as long as the title, and a title line is never a
//     list item, so setext-like text or stray rulers are not headings;
//   - bullet lists ("* ", "- ", "#. ", "N. ") are markdown already and are
//     kept line for line;
//   - inline roles are flattened: double-backtick literals, :role:`text` and
//     `Title <target>`_ references become their text;
//   - explicit markup blocks' marker lines (".. _label:", ".. include::",
//     ".. only::", comments) are blanked (their indented content remains);
//   - grid tables (+---+---+ borders, | cell | rows) become markdown pipe
//     tables: the first content row is the header, the border below it the
//     separator, every further content line a row of its own (a wrapped
//     cell line becomes a row with empty cells). Borders between data rows
//     end the converted table; rows after them are left as fragments. A
//     grid table with one logical data row — the shape of compatibility
//     matrices — converts exactly.
//
// It is a renderer, not a validator: malformed input yields best-effort text.
func RSTToMarkdown(src []byte) []byte {
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	// Pass 1: find the heading styles (adornment char + overline?) in
	// document order and mark heading positions.
	type headingMark struct {
		line  int // line of the title
		level int // 1..6
	}
	var marks []headingMark
	levels := map[string]int{} // "c" or "c+" -> level
	consumed := make([]bool, len(lines))
	nextLevel := func(key string) int {
		if l, ok := levels[key]; ok {
			return l
		}
		l := len(levels) + 1
		if l > 6 {
			l = 6
		}
		levels[key] = l
		return l
	}
	for i := 0; i < len(lines); i++ {
		if consumed[i] {
			continue
		}
		if _, u, ok := underlineHeading(lines, i); ok {
			consumed[i], consumed[i+1] = true, true
			marks = append(marks, headingMark{line: i, level: nextLevel(string(u[0]))})
			i++
			continue
		}
		if _, _, u2, ok := overlineHeading(lines, i); ok {
			consumed[i], consumed[i+1], consumed[i+2] = true, true, true
			marks = append(marks, headingMark{line: i + 1, level: nextLevel(string(u2[0]) + "+")})
			i += 2
			continue
		}
	}
	markAt := map[int]headingMark{}
	for _, m := range marks {
		markAt[m.line] = m
	}

	// Pass 2: render line for line.
	out := make([]string, len(lines))
	inTable := 0 // 0 outside, 1 header emitted, 2+ past the separator
	for i := range lines {
		if m, ok := markAt[i]; ok {
			title := inlineRST(lines[i])
			out[i] = strings.Repeat("#", m.level) + " " + strings.TrimSpace(title)
			continue
		}
		if consumed[i] {
			continue // underline / overline adornment line
		}
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			inTable = 0
			out[i] = ""
			continue
		}
		if isGridBorder(trimmed) || isGridRow(trimmed) {
			out[i] = renderGridLine(trimmed, &inTable)
			continue
		}
		if directiveRe.MatchString(lines[i]) {
			out[i] = "" // ".. _label:", ".. include:: x", ".. only:: ...", comment
			continue
		}
		out[i] = inlineRST(lines[i])
	}
	return []byte(strings.Join(out, "\n"))
}

// rstAdornChars are the characters docutils accepts as section adornments.
const rstAdornChars = "= - ` ~ ^ \" ' * + # . _ :"

func isAdornment(s string) (byte, bool) {
	t := strings.TrimSpace(s)
	if len(t) < 2 {
		return 0, false
	}
	c := t[0]
	if !strings.ContainsRune(rstAdornChars, rune(c)) {
		return 0, false
	}
	for i := 1; i < len(t); i++ {
		if t[i] != c {
			return 0, false
		}
	}
	return c, true
}

// listItemRe recognises lines that rst can never promote to titles.
var listItemRe = regexp.MustCompile(`^(\* |- |#.|\d+\. |\+ )`)

// underlineHeading reports whether lines[i] is a section title underlined by
// lines[i+1] (an underline at least as long as the title).
func underlineHeading(lines []string, i int) (title string, u string, ok bool) {
	if i+1 >= len(lines) {
		return "", "", false
	}
	t := strings.TrimRight(lines[i], " \t")
	if t == "" || strings.TrimSpace(t) == "" {
		return "", "", false
	}
	if leadingSpaces(lines[i]) > 3 { // indented text is never a title
		return "", "", false
	}
	if listItemRe.MatchString(t) {
		return "", "", false
	}
	c, ad := isAdornment(lines[i+1])
	if !ad || c == ' ' {
		return "", "", false
	}
	if len(strings.TrimSpace(lines[i+1])) < runeLen(t) {
		return "", "", false
	}
	return t, strings.TrimSpace(lines[i+1]), true
}

// overlineHeading reports whether lines[i..i+2] form an overline + title +
// underline group.
func overlineHeading(lines []string, i int) (o string, title string, u string, ok bool) {
	if i+2 >= len(lines) {
		return "", "", "", false
	}
	t := strings.TrimSpace(lines[i+1])
	if t == "" || leadingSpaces(lines[i+1]) > 3 || listItemRe.MatchString(t) {
		return "", "", "", false
	}
	c1, ad1 := isAdornment(lines[i])
	c2, ad2 := isAdornment(lines[i+2])
	if !ad1 || !ad2 || c1 != c2 {
		return "", "", "", false
	}
	if len(strings.TrimSpace(lines[i])) < runeLen(t) {
		return "", "", "", false
	}
	return strings.TrimSpace(lines[i]), t, strings.TrimSpace(lines[i+2]), true
}

// runeLen counts runes, tolerating invalid UTF-8 as 1 rune per byte fallback.
func runeLen(s string) int {
	n := 0
	for range s {
		n++
	}
	if n == 0 {
		n = len(s)
	}
	return n
}

var directiveRe = regexp.MustCompile(`^ {0,3}\.\.`)

var (
	rstLiteralRe = regexp.MustCompile("``([^`\n]+)``")
	rstRoleRe    = regexp.MustCompile(":[a-zA-Z][a-zA-Z0-9_-]*:`([^`\n]+)`")
	rstRefRe     = regexp.MustCompile("`([^`\n]+)`(__|_)")
	rstRefTgtRe  = regexp.MustCompile("`([^`\n<]+) <[^`\n<>]+>`(__|_)")
	rstRoleTgtRe = regexp.MustCompile("(:[a-zA-Z][a-zA-Z0-9_-]*:`[^`\n<]+) <[^`\n<>]+>`")
	rstLitBlock  = regexp.MustCompile(`::$`)
)

// inlineRST flattens inline markup to its markdown equivalent.
func inlineRST(line string) string {
	s := rstLiteralRe.ReplaceAllString(line, "`$1`")
	// titled references first: :ref:`Title <target>` and `Title <target>`_
	s = rstRefTgtRe.ReplaceAllString(s, "$1")
	s = rstRoleTgtRe.ReplaceAllString(s, "$1`")
	s = rstRoleRe.ReplaceAllString(s, "$1")
	s = rstRefRe.ReplaceAllString(s, "$1")
	s = rstLitBlock.ReplaceAllString(s, ":")
	return s
}

// Grid tables.

func isGridBorder(trimmed string) bool {
	return strings.HasPrefix(trimmed, "+") && strings.HasSuffix(trimmed, "+")
}

func isGridRow(trimmed string) bool {
	return strings.HasPrefix(trimmed, "|")
}

// renderGridLine converts one line of a grid table. state tracks how many
// non-border lines were seen (1 = header, 2 = separator follows the header's
// border, more = data rows).
func renderGridLine(trimmed string, state *int) string {
	switch {
	case isGridRow(trimmed):
		cells := gridCells(trimmed)
		*state++
		return pipeRow(cells)
	case isGridBorder(trimmed):
		if *state == 1 { // border directly under the header row
			*state = 2
			return pipeRowSeparator(gridCellsWidth(trimmed))
		}
		*state = 0 // border above the header or between data rows: ends the table
		return ""
	}
	return trimmed
}

// gridCells splits "| a | b |" into its trimmed cells.
func gridCells(trimmed string) []string {
	s := strings.TrimSuffix(strings.TrimPrefix(trimmed, "|"), "|")
	parts := strings.Split(s, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = inlineRST(strings.TrimSpace(p))
	}
	return cells
}

// gridCellsWidth returns the number of columns of a border line "+---+---+".
func gridCellsWidth(border string) int {
	s := strings.TrimSuffix(strings.TrimPrefix(border, "+"), "+")
	return strings.Count(s, "+") + 1
}

func pipeRow(cells []string) string {
	return "| " + strings.Join(cells, " | ") + " |"
}

func pipeRowSeparator(n int) string {
	if n < 1 {
		n = 1
	}
	dashes := make([]string, n)
	for i := range dashes {
		dashes[i] = "---"
	}
	return "|" + strings.Join(dashes, "|") + "|"
}
