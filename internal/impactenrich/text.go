package impactenrich

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	wordRe = regexp.MustCompile(`[A-Za-z0-9]+`)
)

// stopwords are dropped from duplicate-detection token sets: the function
// words and release-note boilerplate that surround every item.
var stopwords = toSet(
	"a", "an", "the", "of", "to", "in", "into", "for", "from", "and", "or", "is", "are", "was", "were", "be", "been",
	"it", "its", "this", "that", "these", "those", "with", "without", "by", "on", "as", "at", "we", "our", "you", "your",
	"has", "have", "had", "will", "can", "now", "not", "no", "more", "so", "if", "when", "which", "also", "all", "any",
	"read", "note", "information", "see", "please", "via", "using", "use", "used", "new", "same", "than", "then",
	"http", "https", "www", "com", "github", "io", "md",
)

func toSet(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}

// splitCamel splits "disableHTTPChallengesRole" into disable, HTTP,
// Challenges, Role.
func splitCamel(w string) []string {
	rs := []rune(w)
	var out []string
	start := 0
	for i := 1; i < len(rs); i++ {
		prev, cur := rs[i-1], rs[i]
		next := rune(0)
		if i+1 < len(rs) {
			next = rs[i+1]
		}
		boundary := (unicode.IsLower(prev) && unicode.IsUpper(cur)) ||
			(unicode.IsUpper(prev) && unicode.IsUpper(cur) && next != 0 && unicode.IsLower(next)) ||
			(unicode.IsLetter(prev) && unicode.IsDigit(cur)) || (unicode.IsDigit(prev) && unicode.IsLetter(cur))
		if boundary {
			out = append(out, string(rs[start:i]))
			start = i
		}
	}
	return append(out, string(rs[start:]))
}

// stem is a deliberately light suffix stripper: it only has to make
// "profiles"/"profile" and "changed"/"change" meet.
func stem(w string) string {
	switch {
	case len(w) > 5 && strings.HasSuffix(w, "ies"):
		return w[:len(w)-3] + "y"
	case len(w) > 6 && strings.HasSuffix(w, "ing"):
		return w[:len(w)-3]
	case len(w) > 5 && strings.HasSuffix(w, "ed"):
		return w[:len(w)-1]
	case len(w) > 4 && strings.HasSuffix(w, "ss"):
		return w
	case len(w) > 3 && strings.HasSuffix(w, "s"):
		return w[:len(w)-1]
	}
	return w
}

// tokens returns the normalised content tokens of s: lowercase, stemmed,
// stopwords and one-character tokens dropped; camelCase words contribute
// their parts as well as the whole word.
func tokens(s string) map[string]bool {
	out := map[string]bool{}
	add := func(w string) {
		w = strings.ToLower(w)
		if len(w) < 2 || stopwords[w] {
			return
		}
		out[stem(w)] = true
	}
	for _, w := range wordRe.FindAllString(s, -1) {
		add(w)
		if parts := splitCamel(w); len(parts) > 1 {
			for _, p := range parts {
				add(p)
			}
		}
	}
	return out
}

func jaccard(a, b map[string]bool) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for t := range a {
		if b[t] {
			shared++
		}
	}
	return float64(shared) / float64(len(a)+len(b)-shared)
}

// normalizeTitle flattens a title for exact-match duplicate detection.
func normalizeTitle(s string) string {
	ws := strings.Fields(strings.ToLower(s))
	return strings.Join(ws, " ")
}

func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return strings.TrimSpace(string(rs[:n])) + "…"
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
