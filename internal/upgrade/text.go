package upgrade

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// MaxTitle bounds Change titles (in runes).
const MaxTitle = 160

var (
	mdLinkRe    = regexp.MustCompile(`!?\[([^\]]*)\]\([^)]*\)`)
	mdRefLinkRe = regexp.MustCompile(`\[([^\]]+)\]\[[^\]]*\]`)
	bulletRe    = regexp.MustCompile(`^\s*(?:[-*+•]|\d+[.)])\s+`)
	spaceRe     = regexp.MustCompile(`\s+`)
	// trailing "(#1234, @user)" / "(https://github.com/o/r/pull/1)" groups
	// that differ between an original change and its backports.
	refGroupRe = regexp.MustCompile("(?i)\\s*\\(\\s*(?:`?(?:#\\d+|@[\\w.\\-]+|https?://[^\\s,)`]+|[\\w.\\-]+/[\\w.\\-]+#\\d+|(?:pr|issue|pull request)\\s*#?\\d+|by\\s+@[\\w.\\-]+)`?\\s*[,;]?\\s*)+\\)\\s*[.]?\\s*$")
)

// stripMarkdown removes link syntax and bold/italic markers, keeping text and code spans.
func stripMarkdown(s string) string {
	s = mdLinkRe.ReplaceAllString(s, "$1")
	s = mdRefLinkRe.ReplaceAllString(s, "$1")
	s = strings.ReplaceAll(s, "**", "")
	s = strings.ReplaceAll(s, "__", "")
	return s
}

func collapseSpace(s string) string { return strings.TrimSpace(spaceRe.ReplaceAllString(s, " ")) }

// firstLine returns the first non-blank line of s.
func firstLine(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			return strings.TrimSpace(l)
		}
	}
	return ""
}

var abbreviations = map[string]bool{"e.g": true, "i.e": true, "etc": true, "vs": true, "no": true, "approx": true, "incl": true, "cf": true, "ca": true}

// firstSentence cuts s after the first sentence terminator followed by
// whitespace, ignoring common abbreviations, single-character tokens ("a.",
// "1.") and terminators inside parentheses.
func firstSentence(s string) string {
	depth := 0
	for i := 0; i < len(s)-1; i++ {
		c := s[i]
		switch c {
		case '(':
			depth++
			continue
		case ')':
			if depth > 0 {
				depth--
			}
			continue
		case '.', '!', '?':
		default:
			continue
		}
		if depth > 0 || s[i+1] != ' ' || strings.TrimSpace(s[i+1:]) == "" {
			continue
		}
		if c == '.' {
			word := s[strings.LastIndexAny(s[:i], " (`")+1 : i]
			if abbreviations[strings.ToLower(word)] || utf8.RuneCountInString(word) <= 1 {
				continue
			}
		}
		return s[:i+1]
	}
	return s
}

// stripTrailingRefs removes trailing "(#1234, @user)" groups.
func stripTrailingRefs(s string) string {
	for {
		t := refGroupRe.ReplaceAllString(s, "")
		if t == s {
			return s
		}
		s = t
	}
}

// shorten bounds s to n runes, cutting at a word boundary when possible.
func shorten(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	rs := []rune(s)
	cut := n - 1
	for k := cut; k > cut-25 && k > 0; k-- {
		if rs[k] == ' ' {
			cut = k
			break
		}
	}
	return strings.TrimRight(string(rs[:cut]), " ,;:") + "…"
}

// noteTitle derives a one-line title from a release-note text and reports
// whether the full text carries more than the title.
func noteTitle(text string) (title string, more bool) {
	line := firstLine(text)
	line = bulletRe.ReplaceAllString(line, "")
	line = collapseSpace(stripMarkdown(line))
	sentence := strings.TrimSpace(stripTrailingRefs(firstSentence(line)))
	title = shorten(strings.TrimSuffix(sentence, "."), MaxTitle)
	whole := collapseSpace(stripMarkdown(bulletRe.ReplaceAllString(strings.TrimSpace(text), "")))
	more = strings.TrimSuffix(whole, ".") != title
	if title == "" {
		title = "(empty note)"
	}
	return title, more
}

// normalizeNoteText is the de-duplication key of a note item: markdown and
// trailing PR/author references removed, lower-cased, whitespace collapsed.
func normalizeNoteText(s string) string {
	s = bulletRe.ReplaceAllString(strings.TrimSpace(s), "")
	s = stripMarkdown(s)
	s = strings.NewReplacer("`", "", "*", "", " ", " ").Replace(s)
	s = stripTrailingRefs(strings.ToLower(collapseSpace(s)))
	return strings.TrimRight(strings.TrimSpace(s), ".:;, ")
}

// changeID returns a deterministic change id.
func changeID(parts ...string) string { return "chg-" + domain.ShortHash(parts...) }

// computed returns provenance for a deterministic diff rule.
func computed(rule string) domain.Provenance {
	return domain.Provenance{Method: domain.MethodComputed, Producer: Producer, Rule: rule, Confidence: domain.ConfidenceHigh}
}

func code(s string) string { return "`" + s + "`" }

// codeList renders up to max items as `a`, `b` and N more.
func codeList(items []string, max int) string {
	if len(items) == 0 {
		return ""
	}
	n := len(items)
	if max > 0 && n > max {
		parts := make([]string, max)
		for i := 0; i < max; i++ {
			parts[i] = code(items[i])
		}
		return strings.Join(parts, ", ") + " and " + strconv.Itoa(n-max) + " more"
	}
	parts := make([]string, n)
	for i, it := range items {
		parts[i] = code(it)
	}
	return strings.Join(parts, ", ")
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + many
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// appendUniqueIDs appends ids not yet present, preserving order.
func appendUniqueIDs(dst []domain.EvidenceID, ids ...domain.EvidenceID) []domain.EvidenceID {
	for _, id := range ids {
		dup := false
		for _, x := range dst {
			if x == id {
				dup = true
				break
			}
		}
		if !dup {
			dst = append(dst, id)
		}
	}
	return dst
}

// compareReleaseStrings orders semver strings ascending; "" sorts last.
func compareReleaseStrings(a, b string) int {
	switch {
	case a == b:
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}
	va, vb := domain.Version{Semver: a}, domain.Version{Semver: b}
	return va.Compare(vb)
}

var minorRe = regexp.MustCompile(`(\d+)\.(\d+)`)

// parseLine extracts major.minor from strings like "1.29", "v1.29.3".
func parseLine(s string) (lineKey, bool) {
	m := minorRe.FindStringSubmatch(s)
	if m == nil {
		return lineKey{}, false
	}
	a, err1 := strconv.ParseUint(m[1], 10, 64)
	b, err2 := strconv.ParseUint(m[2], 10, 64)
	if err1 != nil || err2 != nil {
		return lineKey{}, false
	}
	return lineKey{a, b}, true
}
