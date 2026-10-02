package semantic

import (
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Deterministic token pre-extraction. Hints are what a careful reader would
// underline in a release note before interpreting it: code spans, dotted key
// paths, CLI flags, environment variables, API versions, version constraints.
// They are typed tokens, never conclusions: a hint "path:tls.secretsBackend"
// says the text names that string, not that it is a Helm value or that it
// changed. The same extraction yields the subject signature the restatement
// clustering compares (subjectSignature).

var (
	codeSpanRe   = regexp.MustCompile("`([^`\n]{1,160})`")
	dottedPathRe = regexp.MustCompile(`(?:^|[^A-Za-z0-9_./-])([A-Za-z_][A-Za-z0-9_-]*(?:\[\])?(?:\.[A-Za-z_][A-Za-z0-9_-]*(?:\[\])?)+)`)
	flagRe       = regexp.MustCompile(`(?:^|[\s(` + "`" + `"'])(--[a-z0-9][a-z0-9-]{1,80})`)
	envVarRe     = regexp.MustCompile(`\b([A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+)\b`)
	apiVersionRe = regexp.MustCompile(`\b([a-z0-9-]+(?:\.[a-z0-9-]+)+/v[0-9]+(?:(?:alpha|beta)[0-9]+)?)\b`)
	constraintRe = regexp.MustCompile(`(?:>=|<=|>|<|~|\^)\s*v?[0-9]+(?:\.[0-9]+){0,2}`)
	versionRe    = regexp.MustCompile(`\bv?[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-[0-9A-Za-z.]+)?\b`)
)

// fileSuffixes mark dotted tokens that are file names or hosts, not key paths.
var fileSuffixes = []string{".md", ".yaml", ".yml", ".json", ".go", ".io", ".com", ".org", ".dev", ".sh", ".txt", ".html", ".tgz", ".net"}

// Hints returns the typed tokens of texts, deduplicated and sorted by kind
// then value, at most max (0 = no bound).
func Hints(max int, texts ...string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(kind, v string) {
		v = strings.TrimSpace(strings.Trim(v, ".,;:()[]{}\"'"))
		if v == "" || len(v) > 160 {
			return
		}
		h := kind + ":" + v
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	for _, t := range texts {
		for _, m := range codeSpanRe.FindAllStringSubmatch(t, -1) {
			if v := strings.TrimSpace(m[1]); v != "" && v[0] != '#' && v[0] != '@' { // PR numbers, handles
				add("code", v)
			}
		}
		for _, m := range dottedPathRe.FindAllStringSubmatch(t, -1) {
			if isKeyPath(m[1]) {
				add("path", m[1])
			}
		}
		for _, m := range flagRe.FindAllStringSubmatch(t, -1) {
			add("flag", m[1])
		}
		for _, m := range envVarRe.FindAllStringSubmatch(t, -1) {
			add("env", m[1])
		}
		for _, m := range apiVersionRe.FindAllStringSubmatch(t, -1) {
			add("apiVersion", m[1])
		}
		for _, m := range constraintRe.FindAllString(t, -1) {
			add("constraint", strings.ReplaceAll(m, " ", ""))
		}
		for _, m := range versionRe.FindAllString(t, -1) {
			if strings.Count(m, ".") >= 1 && !isKeyPath(m) {
				add("version", m)
			}
		}
	}
	// a path that is a strict prefix of another path hint is a truncation
	// ("a.b.RotationPo" of "a.b.RotationPolicy"), not a subject of its own
	kept := out[:0]
	for _, h := range out {
		trunc := false
		if v, ok := strings.CutPrefix(h, "path:"); ok {
			for _, o := range out {
				if ov, ok := strings.CutPrefix(o, "path:"); ok && len(ov) > len(v) && strings.HasPrefix(ov, v) && ov[len(v)] != '.' && ov[len(v)] != '[' {
					trunc = true
					break
				}
			}
		}
		if !trunc {
			kept = append(kept, h)
		}
	}
	out = kept
	sort.SliceStable(out, func(i, j int) bool { return hintRank(out[i]) < hintRank(out[j]) })
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}

var hintOrder = map[string]int{"code": 0, "path": 1, "flag": 2, "env": 3, "apiVersion": 4, "constraint": 5, "version": 6}

func hintRank(h string) int {
	k, _, _ := strings.Cut(h, ":")
	return hintOrder[k]
}

// isKeyPath reports whether a dotted token reads as a key/field path rather
// than a file name, host, version or sentence fragment ("e.g").
func isKeyPath(s string) bool {
	if len(s) < 3 || !strings.Contains(s, ".") {
		return false
	}
	low := strings.ToLower(s)
	for _, suf := range fileSuffixes {
		if strings.HasSuffix(low, suf) {
			return false
		}
	}
	switch low {
	case "e.g", "i.e", "etc.", "vs.":
		return false
	}
	// every segment starts with a letter (versions do not)
	for _, seg := range strings.Split(strings.TrimSuffix(s, "[]"), ".") {
		seg = strings.TrimSuffix(seg, "[]")
		if seg == "" || !unicode.IsLetter(rune(seg[0])) {
			return false
		}
	}
	return true
}

// subjectSignature is the set of subject-like identifiers a statement names
// in its lead (the title): code spans and bare tokens that look like a key or
// field path, a hyphenated or underscored name, a CLI flag or a multi-hump
// CamelCase identifier. Plain words and bare values (`Always`, `1`) are not
// identifiers. Lowercased so `rotationPolicy` and `RotationPolicy` meet.
// A trailing, unclosed code span (a truncated title) is ignored.
func subjectSignature(title string) map[string]bool {
	if strings.Count(title, "`")%2 == 1 {
		title = title[:strings.LastIndex(title, "`")]
	}
	sig := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(strings.Trim(s, ".,;:()\"'"))
		if subjectLike(s) {
			sig[strings.ToLower(s)] = true
		}
	}
	for _, m := range codeSpanRe.FindAllStringSubmatch(title, -1) {
		v := m[1]
		// "rotationPolicy: Always" names the key, not the value
		if k, _, ok := strings.Cut(v, ": "); ok {
			v = k
		}
		if k, _, ok := strings.Cut(v, "="); ok && strings.HasPrefix(v, "--") {
			v = k
		}
		add(v)
	}
	plain := codeSpanRe.ReplaceAllString(title, " ")
	for _, m := range dottedPathRe.FindAllStringSubmatch(plain, -1) {
		if isKeyPath(m[1]) && hasInnerUpperOrSep(m[1]) {
			add(m[1])
		}
	}
	for _, m := range flagRe.FindAllStringSubmatch(plain, -1) {
		add(m[1])
	}
	for _, m := range envVarRe.FindAllStringSubmatch(plain, -1) {
		add(m[1])
	}
	return sig
}

// subjectLike reports whether a token names a thing (a path, flag, variable,
// compound identifier) rather than being a plain word or value.
func subjectLike(s string) bool {
	if len(s) < 3 || len(s) > 120 || strings.ContainsAny(s, " \t") {
		return false
	}
	if strings.HasPrefix(s, "--") {
		return true
	}
	if !unicode.IsLetter(rune(s[0])) && s[0] != '_' {
		return false
	}
	if isKeyPath(s) || strings.ContainsAny(s, "_-/") {
		return true
	}
	return humps(s) >= 2
}

// humps counts lower→upper transitions ("rotationPolicy" 1, "ValidateCAA" 1,
// "UseDomainQualifiedFinalizer" 3); a PascalCase word with one internal hump
// also counts its leading capital.
func humps(s string) int {
	n := 0
	rs := []rune(s)
	for i := 1; i < len(rs); i++ {
		if unicode.IsLower(rs[i-1]) && unicode.IsUpper(rs[i]) {
			n++
		}
	}
	if n >= 1 && unicode.IsLower(rs[0]) {
		n++ // camelCase: "rotationPolicy" is a compound identifier
	} else if n >= 1 && unicode.IsUpper(rs[0]) {
		n++ // PascalCase compound: "PathType", "ValidateCAA"
	}
	return n
}

func hasInnerUpperOrSep(s string) bool {
	return strings.ContainsAny(s, "_-[") || humps(strings.ReplaceAll(s, ".", "")) >= 2 || strings.Count(s, ".") >= 2
}

func sameSet(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// --- title tokens (the duplicate-audit shape of internal/eval and
// internal/impactenrich: lowercase, light stemming, stopwords dropped,
// camelCase parts added) ---------------------------------------------------------

var wordRe = regexp.MustCompile(`[A-Za-z0-9]+`)

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
		if (unicode.IsLower(prev) && unicode.IsUpper(cur)) ||
			(unicode.IsUpper(prev) && unicode.IsUpper(cur) && next != 0 && unicode.IsLower(next)) ||
			(unicode.IsLetter(prev) && unicode.IsDigit(cur)) || (unicode.IsDigit(prev) && unicode.IsLetter(cur)) {
			out = append(out, string(rs[start:i]))
			start = i
		}
	}
	return append(out, string(rs[start:]))
}

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

func normalizeTitle(s string) string { return strings.Join(strings.Fields(strings.ToLower(s)), " ") }

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func shorten(s string, n int) string {
	s = strings.TrimSpace(s)
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return strings.TrimSpace(string(rs[:n])) + "…"
}
