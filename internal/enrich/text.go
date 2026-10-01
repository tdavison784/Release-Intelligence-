package enrich

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

var (
	wordRe     = regexp.MustCompile(`[A-Za-z0-9]+`)
	codeSpanRe = regexp.MustCompile("`([^`\n]+)`")
	versionRe  = regexp.MustCompile(`^v?\d+(\.\d+)+([-+].*)?$`)
)

// stopwords are dropped from token sets: function words, and the boilerplate
// that release notes and upgrade guides put around every item.
var stopwords = toSet(
	"a", "an", "the", "of", "to", "in", "into", "for", "from", "and", "or", "is", "are", "was", "were", "be", "been",
	"it", "its", "this", "that", "these", "those", "with", "without", "by", "on", "as", "at", "we", "our", "you", "your",
	"has", "have", "had", "will", "can", "now", "not", "no", "more", "so", "if", "when", "which", "also", "all", "any",
	"read", "note", "information", "see", "please", "via", "using", "use", "used", "new", "same", "than", "then",
	"http", "https", "www", "com", "github", "io", "md",
)

// genericKeyParts never count as evidence that a diffed key is mentioned:
// they occur in almost every key.
var genericKeyParts = toSet("spec", "status", "enabled", "enable", "value", "values", "default", "name", "type",
	"config", "true", "false", "global", "image", "tag", "key", "keys", "item", "items", "list", "data", "field", "fields")

func toSet(ws ...string) map[string]bool {
	m := make(map[string]bool, len(ws))
	for _, w := range ws {
		m[w] = true
	}
	return m
}

// splitCamel splits "disableHTTPChallengesRole" into disable, HTTP,
// Challenges, Role. Words without case changes come back unchanged.
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
		return w[:len(w)-1] // changed → change, named → name
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
		if isDigits(w) && len(w) < 3 {
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

// keyTokens splits a structured key (Helm value path, CRD field, image) into
// its specific parts, dropping generic ones.
func keyTokens(key string) []string {
	var out []string
	seen := map[string]bool{}
	for _, w := range wordRe.FindAllString(key, -1) {
		for _, p := range splitCamel(w) {
			p = stem(strings.ToLower(p))
			if len(p) < 3 || genericKeyParts[p] || stopwords[p] || isDigits(p) || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// codeKeys returns the key-like code spans of a text (`Certificate.Spec.X`,
// `global.rbac.disableHTTPChallengesRole`, `PathType`), lowercased.
func codeKeys(s string) []string {
	var out []string
	for _, m := range codeSpanRe.FindAllStringSubmatch(s, -1) {
		if k := normKey(m[1]); keyLike(k, m[1]) {
			out = append(out, k)
		}
	}
	return out
}

func normKey(s string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(s), ".,;:()'\""))
}

// keyLike keeps identifiers that are specific enough to link two changes:
// dotted/dashed/slashed paths, camelCase identifiers and long words. Plain
// words (`Never`, `Exact`), versions, PR and user mentions are not keys.
func keyLike(norm, raw string) bool {
	switch {
	case norm == "", strings.ContainsAny(norm, " \t"), strings.HasPrefix(norm, "#"), strings.HasPrefix(norm, "@"),
		strings.Contains(norm, "://"), versionRe.MatchString(norm), isDigits(norm):
		return false
	case strings.ContainsAny(norm, "._/-[") && len(norm) >= 4:
		return true
	case len(splitCamel(strings.TrimSpace(raw))) > 1 && len(norm) >= 6:
		return true
	}
	return len(norm) >= 14
}

// containsWord reports whether needle occurs in hay delimited by non
// identifier characters (case-insensitive; both already lowercased).
func containsWord(hay, needle string) bool {
	for i := 0; ; {
		j := strings.Index(hay[i:], needle)
		if j < 0 {
			return false
		}
		j += i
		end := j + len(needle)
		if (j == 0 || !isIdent(hay[j-1])) && (end == len(hay) || !isIdent(hay[end])) {
			return true
		}
		i = j + 1
	}
}

func isIdent(c byte) bool {
	return c == '_' || c == '.' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// prefixMatch reports whether two tokens are the same word or one is a
// prefix (≥5 characters) of the other: "servicemon" ~ "servicemonitor".
func prefixMatch(a, b string) bool {
	if a == b {
		return true
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	return len(a) >= 5 && strings.HasPrefix(b, a)
}

// weightedOverlap is the IDF-weighted overlap coefficient of two token sets:
// shared weight divided by the weight of the smaller set. Common words
// ("default", "certificate") weigh little, rare ones ("rotationpolicy") a lot.
//
// Tokens are summed in sorted order: float addition is not associative, and
// map order would make the result (and so the grouping) vary between runs.
func weightedOverlap(a, b map[string]bool, idf func(string) float64) (sim float64, shared []string) {
	var wa, wb, ws float64
	for _, t := range sortedKeys(a) {
		wa += idf(t)
		if b[t] {
			ws += idf(t)
			shared = append(shared, t)
		}
	}
	for _, t := range sortedKeys(b) {
		wb += idf(t)
	}
	den := math.Min(wa, wb)
	if den == 0 {
		return 0, shared
	}
	return ws / den, shared
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
