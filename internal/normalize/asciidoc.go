package normalize

import (
	"regexp"
	"strings"
)

// ASCIIDocToMarkdown renders AsciiDoc as markdown so that the markdown
// selectors and parsers apply to it. It exists for products whose upgrade
// intelligence is AsciiDoc in the repository (Elasticsearch's per-major
// migration guides docs/reference/migration/migrate_N_0(.asciidoc|.asciidoc/):
// "== Migrating to 9.0" with "=== Breaking changes" / "=== Deprecations"
// topics built from [[anchor]] / .Block title / [%collapsible] / ==== blocks;
// the same conventions serve OpenSearch, the fork of that repository, and the
// wider Asciidoctor ecosystem).
//
// The conversion is line-preserving: output line N renders input line N (the
// heading, anchor, attribute-list and block-delimiter lines that are consumed
// leave blank lines), so line ranges found in the markdown are line ranges of
// the original document and evidence locators stay valid.
//
//   - section titles become "#".."######" headings, in both AsciiDoc forms:
//     the one-line "= Title" prefix (level = number of '=') and the two-line
//     underline form, whose style-to-level mapping is FIXED in AsciiDoc
//     ("=" 1, "-" 2, "~" 3, "^" 4, "+" 5), unlike rst's first-use ordering;
//   - anchor ("[[id]]"), block attribute ("[discrete]", "[%collapsible]",
//     "[cols=...]"), macro ("include::x[]", "ifdef::[]", "tag::[]",
//     "image::x[]"), comment ("//") and standalone block-delimiter lines
//     ("====", "--", "****", "____") are blanked;
//   - a block title line (".Title", no space after the dot) becomes bold
//     text, so the topic of a collapsible breaking-change block survives as
//     the text of its own item;
//   - inline markup is flattened to its markdown equivalent: <<anchor,Text>>
//     and target[Text] link macros become [Text](target) (or their text when
//     the target is an attribute such as {ref}), kbd:[…]/btn:[…]/pass:[…]
//     become their text, and the " +" hard-line-break suffix is dropped;
//   - definition-list entry lines (":   text") lose their colon markers, so
//     the AsciiDoc-flavored definition lists of .md exports still read as
//     text;
//   - pipe tables ("|====" borders, "| cell" lines) become markdown pipe
//     tables: consecutive "| cell" lines are the cells of ONE row (AsciiDoc
//     rows span lines until a blank line or border), the first row is the
//     header and the blank line after it carries the separator, which keeps
//     every output on an input line.
//
// It is a renderer, not a validator: malformed input yields best-effort text.
func ASCIIDocToMarkdown(src []byte) []byte {
	s := strings.ReplaceAll(string(src), "\r\n", "\n")
	lines := strings.Split(s, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}

	// Pass 1: find headings and pipe tables in document order and mark the
	// lines they consume.
	type headingMark struct {
		line  int // line of the title
		level int // 1..6
	}
	var marks []headingMark
	tableAt := map[int]string{} // line -> rendered table line (row/separator)
	consumed := make([]bool, len(lines))
	for i := 0; i < len(lines); i++ {
		if consumed[i] {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if _, level, ok := adocPrefixHeading(trimmed); ok {
			consumed[i] = true
			marks = append(marks, headingMark{line: i, level: level})
			continue
		}
		if title, level, ok := adocUnderlineHeading(lines, i); ok {
			_ = title
			consumed[i], consumed[i+1] = true, true
			marks = append(marks, headingMark{line: i, level: level})
			i++
			continue
		}
		if isPipeTableBorder(trimmed) {
			end := markPipeTable(lines, i, consumed, tableAt)
			i = end
		}
	}
	markAt := map[int]headingMark{}
	for _, m := range marks {
		markAt[m.line] = m
	}

	// Pass 2: render line for line.
	out := make([]string, len(lines))
	for i := range lines {
		if m, ok := markAt[i]; ok {
			title := inlineASCIIDoc(strings.TrimSpace(lines[i]))
			title = strings.TrimLeft(title, "= ")
			out[i] = strings.Repeat("#", m.level) + " " + strings.TrimSpace(title)
			continue
		}
		if t, ok := tableAt[i]; ok {
			out[i] = t
			continue
		}
		if consumed[i] {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			out[i] = ""
			continue
		}
		// anchors, block attribute lists, macros and comments are structural
		if adocAnchorRe.MatchString(trimmed) || adocAttrListRe.MatchString(trimmed) ||
			adocMacroRe.MatchString(trimmed) || strings.HasPrefix(trimmed, "//") {
			continue
		}
		// standalone block delimiters and pass-through fences
		if isAdocDelimiter(trimmed) {
			continue
		}
		// block title: one leading dot, no space after it
		if rest, ok := strings.CutPrefix(trimmed, "."); ok && rest != "" && !strings.HasPrefix(rest, ".") {
			out[i] = "**" + inlineASCIIDoc(rest) + "**"
			continue
		}
		// definition-list entry lines lose their colon markers
		if m := adocDefEntryRe.FindStringSubmatch(lines[i]); m != nil {
			out[i] = inlineASCIIDoc(strings.TrimRight(m[2], " "))
			continue
		}
		out[i] = inlineASCIIDoc(lines[i])
	}
	return []byte(strings.Join(out, "\n"))
}

// adocLevelChars maps AsciiDoc two-line heading underline styles to levels
// (fixed by the format, unlike rst's first-use ordering).
const adocLevelChars = "=-~^+"

// adocPrefixHeading recognises the one-line form "== Title".
func adocPrefixHeading(trimmed string) (title string, level int, ok bool) {
	if !strings.HasPrefix(trimmed, "=") {
		return "", 0, false
	}
	n := 0
	for n < len(trimmed) && trimmed[n] == '=' {
		n++
	}
	if n >= len(trimmed) || trimmed[n] != ' ' {
		return "", 0, false // "====" delimiter or "====Title" (not a heading)
	}
	title = strings.TrimSpace(trimmed[n+1:])
	if title == "" || n > 6 {
		return "", 0, false
	}
	return title, n, true
}

// adocUnderlineHeading reports whether lines[i] is a section title underlined
// by lines[i+1] (underline at least as long as the title).
func adocUnderlineHeading(lines []string, i int) (title string, level int, ok bool) {
	if i+1 >= len(lines) {
		return "", 0, false
	}
	t := strings.TrimRight(lines[i], " \t")
	if t == "" || strings.TrimSpace(t) == "" {
		return "", 0, false
	}
	if leadingSpaces(lines[i]) > 3 { // indented text is never a title
		return "", 0, false
	}
	if listItemRe.MatchString(t) || strings.HasPrefix(strings.TrimSpace(t), "|") {
		return "", 0, false
	}
	u := strings.TrimSpace(lines[i+1])
	if len(u) < 2 || len(u) < runeLen(strings.TrimSpace(t)) {
		return "", 0, false
	}
	level = strings.IndexByte(adocLevelChars, u[0]) + 1
	if level == 0 {
		return "", 0, false
	}
	for k := 1; k < len(u); k++ {
		if u[k] != u[0] {
			return "", 0, false
		}
	}
	return t, level, true
}

// isAdocDelimiter reports whether the line is a standalone AsciiDoc block
// delimiter ("====", "--", "****", "____", "++++"), a pass-through fence or
// a ruler.
func isAdocDelimiter(s string) bool {
	if len(s) < 2 {
		return false
	}
	if s == `'''` {
		return true
	}
	c := s[0]
	if !strings.ContainsRune("=-*_^+~", rune(c)) {
		return false
	}
	for i := 1; i < len(s); i++ {
		if s[i] != c {
			return false
		}
	}
	return true
}

var (
	adocAnchorRe  = regexp.MustCompile(`^\[\[[^\]\n]*\]\]$`)
	adocAttrListRe = regexp.MustCompile(`^\[[^]\n]*\]$`)
	// standalone macro lines: include::dir/file.asciidoc[], ifdef::attr[],
	// image::x.png[], coming::[9.0.0], tag::name[] … (but not prose with "::")
	adocMacroRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*::[A-Za-z0-9_./%#-]*(\[[^\]\n]*\])?$`)
	// definition-list entry: 1-3 colon markers then text
	adocDefEntryRe = regexp.MustCompile(`^(:{1,3})[ \t]+(.*)$`)
)

var (
	adocXrefRe    = regexp.MustCompile("<<([^<>\n>,]+)(?:,([^<>\n>]+))?>>")
	adocLinkRe    = regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^\s\[]+|\{[a-z][a-z0-9_]*\}/[^\s\[]+)\[([^\]\n]*)\]`)
	adocWordLnkRe = regexp.MustCompile(`\b([A-Za-z][A-Za-z0-9_.-]*(?:/[A-Za-z0-9_.:#-]+)+)\[([^\]\n]*)\]`)
	adocRoleRe    = regexp.MustCompile(`\b(kbd|btn|menu|package|app|pass|term):\[([^\]\n]*)\]`)
	adocBreakRe   = regexp.MustCompile(`[ \t]\+{1,2}$`)
)

// inlineASCIIDoc flattens inline markup to its markdown equivalent.
func inlineASCIIDoc(line string) string {
	s := adocBreakRe.ReplaceAllString(line, "")
	// titled references first: <<anchor,Text>> -> Text (anchor when bare)
	s = adocXrefRe.ReplaceAllStringFunc(s, func(m string) string {
		g := adocXrefRe.FindStringSubmatch(m)
		if len(g) == 3 && strings.TrimSpace(g[2]) != "" {
			return strings.TrimSpace(g[2])
		}
		return g[1]
	})
	// URL and attribute-relative link macros: target[Text] -> [Text](target)
	s = adocLinkRe.ReplaceAllStringFunc(s, func(m string) string {
		g := adocLinkRe.FindStringSubmatch(m)
		target, text := g[1], strings.TrimSpace(g[2])
		if text == "" {
			return target
		}
		return "[" + text + "](" + target + ")"
	})
	// word-target links: {ref}/path.html[Text] -> [Text](target)
	s = adocWordLnkRe.ReplaceAllStringFunc(s, func(m string) string {
		g := adocWordLnkRe.FindStringSubmatch(m)
		target, text := g[1], strings.TrimSpace(g[2])
		if text == "" {
			return target
		}
		return "[" + text + "](" + target + ")"
	})
	s = adocRoleRe.ReplaceAllString(s, "$2")
	return s
}

// Pipe tables.

func isPipeTableBorder(trimmed string) bool {
	return strings.HasPrefix(trimmed, "|") && len(trimmed) > 1 &&
		strings.Trim(trimmed, "|=") == "" && strings.ContainsRune(trimmed, '=')
}

// isPipeCell reports whether the line is a table cell line ("| a | b |").
func isPipeCell(trimmed string) bool {
	return strings.HasPrefix(trimmed, "|") && !isPipeTableBorder(trimmed)
}

// pipeCells splits "| a | b |" (or "| a") into its trimmed, inline-flattened
// cells.
func pipeCells(trimmed string) []string {
	s := strings.TrimPrefix(trimmed, "|")
	parts := strings.Split(s, "|")
	cells := make([]string, 0, len(parts))
	for _, p := range parts {
		cells = append(cells, strings.TrimSpace(inlineASCIIDoc(strings.TrimSpace(p))))
	}
	return cells
}

// markPipeTable converts the pipe table starting at border line i: groups of
// consecutive cell lines are rows (the first is the header), the blank line
// after the header carries the markdown separator, and every consumed line is
// marked. It returns the index of the last line handled.
func markPipeTable(lines []string, i int, consumed []bool, tableAt map[int]string) int {
	consumed[i] = true // opening border
	headerDone := false
	firstCell := -1
	var cells []string
	flush := func() {
		if firstCell < 0 {
			return
		}
		tableAt[firstCell] = pipeRow(cells)
		firstCell, cells = -1, nil
	}
	end := i
	for j := i + 1; j < len(lines); j++ {
		trimmed := strings.TrimSpace(lines[j])
		switch {
		case isPipeCell(trimmed):
			if firstCell < 0 {
				firstCell = j
			}
			cells = append(cells, pipeCells(trimmed)...)
			consumed[j] = true
			end = j
		case trimmed == "":
			if !headerDone && firstCell >= 0 {
				// the header group is complete; the blank line after it
				// carries the markdown separator
				consumed[j] = true
				tableAt[firstCell] = pipeRow(cells)
				tableAt[j] = pipeRowSeparator(len(cells))
				headerDone = true
				firstCell, cells = -1, nil
				continue
			}
			flush()
			end = j
		case isPipeTableBorder(trimmed):
			flush()
			consumed[j] = true // closing border
			return j
		default:
			flush()
			return j - 1
		}
	}
	flush()
	return end
}
