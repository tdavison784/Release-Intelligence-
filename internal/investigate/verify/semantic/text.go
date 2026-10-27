package semantic

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// --- normalization and clauses ------------------------------------------------------------

var normalizer = strings.NewReplacer(
	"‘", "'", "’", "'", "“", `"`, "”", `"`,
	"can't", "can not", "cannot", "can not", "won't", "will not", "n't", " not",
	" other than ", " except ", " ", " ",
)

func normalize(s string) string { return normalizer.Replace(s) }

// span is a quoted value (`x`, "x", 'x') or an unquoted bracket expression.
type span struct {
	start, end int // of the whole span in the text
	content    string
	quote      byte // '`', '"', '\'' or '[' (bracket expression)
}

// spans finds the quoted values and bracket expressions of a text, left to
// right, without overlap.
func spans(text string) []span {
	var out []span
	for i := 0; i < len(text); i++ {
		switch c := text[i]; c {
		case '`', '"':
			j := strings.IndexByte(text[i+1:], c)
			if j < 0 || strings.ContainsRune(text[i+1:i+1+j], '\n') || j > 200 {
				continue
			}
			out = append(out, span{start: i, end: i + 2 + j, content: text[i+1 : i+1+j], quote: c})
			i += 1 + j
		case '\'':
			if i > 0 && !strings.ContainsRune(" \t\n([{,:=/", rune(text[i-1])) {
				continue // an apostrophe
			}
			j := strings.IndexByte(text[i+1:], '\'')
			if j <= 0 || j > 64 || strings.ContainsAny(text[i+1:i+1+j], "\n") {
				continue
			}
			end := i + 2 + j
			if end < len(text) && !strings.ContainsRune(" \t\n)]},.;:!?", rune(text[end])) {
				continue
			}
			out = append(out, span{start: i, end: end, content: text[i+1 : i+1+j], quote: '\''})
			i = end - 1
		case '[':
			end := bracketEnd(text, i)
			if end < 0 {
				continue
			}
			if inner := text[i+1 : end]; isCharClass(inner) {
				out = append(out, span{start: i, end: end + 1, content: text[i : end+1], quote: '['})
			}
			i = end
		}
	}
	return out
}

// bracketEnd returns the index of the "]" closing the "[" at i (-1 when none
// on the line). A "]" right after "[" or "[^" is a literal.
func bracketEnd(s string, i int) int {
	j := i + 1
	if j < len(s) && s[j] == '^' {
		j++
	}
	if j < len(s) && s[j] == ']' {
		j++
	}
	for ; j < len(s); j++ {
		switch s[j] {
		case '\\':
			j++
		case '\n':
			return -1
		case ']':
			return j
		}
	}
	return -1
}

var rangeRe = regexp.MustCompile(`[A-Za-z0-9]-[A-Za-z0-9]`)

// isCharClass reports whether a bracket expression's inside reads as a
// regular-expression character class rather than an index or a link.
func isCharClass(inner string) bool {
	if inner == "" || len(inner) > 96 || strings.ContainsAny(inner, " \t") {
		return false
	}
	if strings.HasPrefix(inner, "^") || strings.Contains(inner, `\`) || rangeRe.MatchString(inner) {
		return true
	}
	for _, r := range inner {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return utf8.RuneCountInString(inner) >= 2
		}
	}
	return false
}

// clause is one sentence-like unit of a text with its polarity.
type clause struct {
	start, end int
	text       string
	// words are the clause's lower-case words (identifiers split at case
	// changes), quoted content included; cues excludes quoted values.
	words    []string
	polarity Polarity
	only     bool
	spans    []span
}

var clauseBreak = regexp.MustCompile(`[.;!?](\s|$)|\n|\s(but|whereas|however|while)\s`)

// clauses splits a text into clauses (quoted spans are never split).
func clauses(text string) []clause {
	sp := spans(text)
	masked := []byte(text)
	for _, s := range sp {
		for k := s.start; k < s.end; k++ {
			masked[k] = 'x'
		}
	}
	var out []clause
	start := 0
	cut := func(end int) {
		if strings.TrimSpace(text[start:end]) != "" {
			out = append(out, newClause(text, start, end, sp))
		}
	}
	for _, ix := range clauseBreak.FindAllIndex(masked, -1) {
		cut(ix[0] + 1)
		start = ix[0] + 1
	}
	cut(len(text))
	return out
}

func newClause(text string, start, end int, all []span) clause {
	c := clause{start: start, end: end, text: text[start:end]}
	var cueText strings.Builder
	last := start
	for _, s := range all {
		if s.start < start || s.end > end {
			continue
		}
		c.spans = append(c.spans, s)
		cueText.WriteString(text[last:s.start])
		cueText.WriteByte(' ')
		if s.quote == '"' && strings.ContainsAny(s.content, " \t") {
			cueText.WriteString(s.content) // a quoted message reads like prose
		}
		cueText.WriteByte(' ')
		last = s.end
	}
	cueText.WriteString(text[last:end])
	c.words = words(c.text)
	cues := words(cueText.String())
	c.polarity = polarityOf(cues)
	for _, w := range cues {
		c.only = c.only || w == "only"
	}
	return c
}

// words splits text into lower-case words, splitting identifiers at case
// changes (validPathType → valid path type).
func words(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = cur[:0]
		}
	}
	rs := []rune(s)
	for i, r := range rs {
		switch {
		case unicode.IsLetter(r):
			if unicode.IsUpper(r) && len(cur) > 0 && (unicode.IsLower(cur[len(cur)-1]) || (i+1 < len(rs) && unicode.IsLower(rs[i+1]) && unicode.IsUpper(cur[len(cur)-1]))) {
				flush()
			}
			cur = append(cur, r)
		case unicode.IsDigit(r):
			cur = append(cur, r)
		default:
			flush()
		}
	}
	flush()
	return out
}

var (
	positiveCues = set("accept", "accepts", "accepted", "accepting", "acceptable", "allow", "allows", "allowed", "allowing",
		"permit", "permits", "permitted", "valid", "admit", "admits", "admitted", "enable", "enables", "enabled",
		"support", "supports", "supported", "legal")
	weakPositiveCues = set("may", "can")
	negativeCues     = set("reject", "rejects", "rejected", "rejecting", "deny", "denies", "denied", "disallow", "disallows",
		"disallowed", "forbid", "forbids", "forbidden", "invalid", "illegal", "disable", "disables", "disabled", "refuse",
		"refuses", "refused", "prohibit", "prohibits", "prohibited", "unsupported", "block", "blocks", "blocked", "fail",
		"fails", "failed", "error", "errors")
	negators = set("not", "no", "never", "without", "outside", "except", "neither", "nor", "none")
)

func set(ws ...string) map[string]bool {
	m := map[string]bool{}
	for _, w := range ws {
		m[w] = true
	}
	return m
}

// polarityOf reads a clause's polarity from its cue words: affirming or
// denying verbs and adjectives, each negator flipping it. Both kinds of cue
// in one clause, or none, leave it unknown (a lone negator denies).
func polarityOf(ws []string) Polarity {
	pos, weak, neg, n := false, false, false, 0
	for _, w := range ws {
		switch {
		case positiveCues[w]:
			pos = true
		case weakPositiveCues[w]:
			weak = true
		case negativeCues[w]:
			neg = true
		case negators[w]:
			n++
		}
	}
	base := PolarityUnknown
	switch {
	case pos && neg:
		return PolarityUnknown
	case pos:
		base = PolarityPositive
	case neg:
		base = PolarityNegative
	case weak:
		base = PolarityPositive
	case n > 0:
		return PolarityNegative
	default:
		return PolarityUnknown
	}
	if n%2 == 1 {
		return base.opposite()
	}
	return base
}

// --- booleans ----------------------------------------------------------------------------

// booleanWord maps a word to the boolean it states ("true"/"false").
func booleanWord(w string) (string, bool) {
	switch strings.ToLower(w) {
	case "true", "enabled":
		return "true", true
	case "false", "disabled":
		return "false", true
	}
	return "", false
}

// booleanValue: a value token that is a boolean literal.
func booleanValue(v string) (string, bool) {
	switch strings.ToLower(strings.Trim(v, `"'`)) {
	case "true":
		return "true", true
	case "false":
		return "false", true
	}
	return "", false
}

// --- character sets ------------------------------------------------------------------------

// expandClass returns the characters of a bracket expression ("[^a-c\-]")
// and whether it is negated.
func expandClass(expr string) (map[rune]bool, bool) {
	inner := strings.TrimSuffix(strings.TrimPrefix(expr, "["), "]")
	neg := strings.HasPrefix(inner, "^")
	if neg {
		inner = inner[1:]
	}
	out := map[rune]bool{}
	rs := []rune(inner)
	addRange := func(a, b rune) {
		for r := a; r <= b && b-a < 256; r++ {
			out[r] = true
		}
	}
	for i := 0; i < len(rs); i++ {
		r := rs[i]
		if r == '\\' && i+1 < len(rs) {
			i++
			switch rs[i] {
			case 'w':
				addRange('a', 'z')
				addRange('A', 'Z')
				addRange('0', '9')
				out['_'] = true
			case 'd':
				addRange('0', '9')
			case 's':
				for _, c := range " \t\n\r\f\v" {
					out[c] = true
				}
			case 'n':
				out['\n'] = true
			case 't':
				out['\t'] = true
			default:
				out[rs[i]] = true
			}
			continue
		}
		if i+2 < len(rs) && rs[i+1] == '-' && rs[i+2] != '\\' && rs[i+2] >= r {
			addRange(r, rs[i+2])
			i += 2
			continue
		}
		out[r] = true
	}
	return out, neg
}

var namedClasses = map[string]string{
	"alphanumeric":  "a-zA-Z0-9",
	"alphanumerics": "a-zA-Z0-9",
	"letters":       "a-zA-Z",
	"digits":        "0-9",
	"lowercase":     "a-z",
	"uppercase":     "A-Z",
}

var bareChar = regexp.MustCompile(`(?:^|[\s,(])([^\w\s"'` + "`" + `])(?:$|[\s,)])`)

// charSetsOf returns the character sets a passage clause states: each
// bracket expression (an exhaustive set, quoted or not), and the single
// characters it lists (quoted, or bare between separators) with the named
// classes it mentions.
func charSetsOf(c clause) []occurrence {
	var out []occurrence
	for i := 0; i < len(c.text); i++ {
		if c.text[i] != '[' {
			continue
		}
		end := bracketEnd(c.text, i)
		if end < 0 || !isCharClass(c.text[i+1:end]) {
			continue
		}
		chars, neg := expandClass(c.text[i : end+1])
		p := c.polarity
		if neg {
			p = p.opposite()
		}
		out = append(out, occurrence{polarity: p, exhaustive: !neg, chars: chars, text: c.text[i : end+1]})
		i = end
	}
	listed := map[rune]bool{}
	for _, s := range c.spans {
		if s.quote != '[' && utf8.RuneCountInString(s.content) == 1 {
			r, _ := utf8.DecodeRuneInString(s.content)
			listed[r] = true
		}
	}
	for _, m := range bareChar.FindAllStringSubmatch(c.text, -1) {
		r, _ := utf8.DecodeRuneInString(m[1])
		listed[r] = true
	}
	for _, w := range c.words {
		if cls, ok := namedClasses[w]; ok {
			chars, _ := expandClass("[" + cls + "]")
			for r := range chars {
				listed[r] = true
			}
		}
	}
	if len(listed) > 0 {
		out = append(out, occurrence{polarity: c.polarity, exhaustive: c.only, chars: listed, text: c.text})
	}
	return out
}

// --- claim tokens --------------------------------------------------------------------------

var (
	versionLike = regexp.MustCompile(`^v?\d+\.\d+(\.\d+)?([-+][0-9A-Za-z.-]+)?$`)
	numberLike  = regexp.MustCompile(`^-?\d+(\.\d+)?([a-zA-Z%]{1,3})?$`)
	// defaultBefore ends right before a value a default names.
	defaultBefore = regexp.MustCompile(`(?i)\bdefaults?(\s+value)?(\s+(is|to|of|was|will\s+be|becomes|changes\s+to))?\s*[:=]?\s*$`)
	// byDefaultAfter starts right after a value "by default" qualifies.
	byDefaultAfter = regexp.MustCompile(`(?i)^\s+(by\s+default|is\s+the\s+default)\b`)
	bareDefault    = regexp.MustCompile(`(?i)\bdefaults?(?:\s+value)?(?:\s+(?:is|to|of|was|will\s+be|becomes|changes\s+to))?\s*[:=]?\s*([A-Za-z0-9][\w.%-]*)`)
	bareByDefault  = regexp.MustCompile(`(?i)\b([A-Za-z0-9][\w.%-]*)\s+by\s+default\b`)
	fromTo         = regexp.MustCompile(`(?i)\bfrom\s+(\S+)\s+to\s+(\S+)`)
	arrow          = regexp.MustCompile(`(\S+)\s*(?:->|→|=>)\s*(\S+)`)
)

// valueCues are the words after which a quoted span is a value.
var valueCues = set("is", "are", "be", "been", "being", "equals", "equal", "to", "of", "as", "accepts", "accept", "accepted",
	"rejects", "reject", "rejected", "allows", "allow", "allowed", "permits", "permit", "permitted", "contains", "contain",
	"containing", "contained", "includes", "include", "including", "uses", "use", "like", "disallows", "forbids", "denies",
	"with", "from", "than", "=", ":", "becomes", "became", "either", "only")

var fillers = set("only", "the", "a", "an", "value", "values", "set", "either", "literal", "character", "characters",
	"string", "strings", "exactly", "also", "still", "always", "now", "then")

var listJoiners = set(",", "or", "and", "nor", "/")

// prevWords returns up to n words (or the symbols , = : /) before i, nearest first.
func prevWords(text string, i, n int) []string {
	var out []string
	j := i
	for len(out) < n {
		for j > 0 && (text[j-1] == ' ' || text[j-1] == '\t') {
			j--
		}
		if j == 0 {
			break
		}
		if c := text[j-1]; c == ',' || c == '=' || c == ':' || c == '/' {
			out = append(out, string(c))
			j--
			continue
		}
		k := j
		for k > 0 && (identByteAt(text, k-1) || text[k-1] == '-') {
			k--
		}
		if k == j {
			break
		}
		out = append(out, strings.ToLower(text[k:j]))
		j = k
	}
	return out
}

// valuePosition reports whether the span at i follows a value cue (skipping
// fillers), or continues a list of values.
func valuePosition(text string, i int, prevWasValue bool) bool {
	for _, w := range prevWords(text, i, 4) {
		switch {
		case valueCues[w]:
			return true
		case listJoiners[w]:
			return prevWasValue
		case fillers[w]:
			continue
		default:
			return false
		}
	}
	return false
}

// isName reports a quoted span that names a setting, field, flag, path or
// piece of code rather than stating a value.
func isName(v string) bool {
	switch {
	case v == "" || len(v) > 64 || strings.ContainsAny(v, " \t(){}<>$"):
		return true
	case strings.HasPrefix(v, "-") && !numberLike.MatchString(v):
		return true // a flag
	case versionLike.MatchString(v):
		return true // a version: the citation check's concern
	}
	rs := []rune(v)
	if unicode.IsLower(rs[0]) {
		for _, r := range rs[1:] {
			if unicode.IsUpper(r) {
				return true // lowerCamel: a field
			}
		}
	}
	for i := 1; i+1 < len(rs); i++ {
		if (rs[i] == '.' || rs[i] == '/') && unicode.IsLetter(rs[i-1]) && unicode.IsLetter(rs[i+1]) {
			return true // a dotted path, an annotation or API group, a file
		}
	}
	if strings.Count(v, "-") >= 2 && strings.ToLower(v) == v {
		return true // a hyphenated option name
	}
	return false
}

// bareValue reports whether an unquoted word can be a stated value:
// a number, a boolean or a capitalised identifier.
func bareValue(v string) bool {
	v = strings.TrimRight(v, ".,;:)")
	if v == "" || versionLike.MatchString(v) {
		return false
	}
	if _, ok := booleanValue(v); ok {
		return true
	}
	if numberLike.MatchString(v) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(v)
	return unicode.IsUpper(r) && !strings.ContainsAny(v, "./") && len(v) <= 40
}

// ExtractTokens returns the value tokens a free-text claim asserts.
func ExtractTokens(claim string) []Token {
	text := normalize(claim)
	var out []Token
	seen := map[string]bool{}
	add := func(t Token) {
		k := string(t.Kind) + "\x00" + t.Value + "\x00" + string(t.Polarity)
		if t.Value == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, t)
	}
	value := func(kind TokenKind, v string, p Polarity) {
		v = strings.TrimSpace(v)
		if b, ok := booleanValue(v); ok {
			if kind == TokenValue {
				add(Token{Kind: TokenBoolean, Value: b})
			} else {
				add(Token{Kind: kind, Value: b})
			}
			return
		}
		if isName(v) {
			return
		}
		if kind != TokenValue {
			p = PolarityUnknown // a default or a before/after asserts presence
		}
		add(Token{Kind: kind, Value: v, Polarity: p})
	}
	inSpan := make([]bool, len(text))
	for _, c := range clauses(text) {
		prevValue := false
		for _, s := range c.spans {
			for k := s.start; k < s.end; k++ {
				inSpan[k] = true
			}
			kind := TokenValue
			before, after := text[c.start:s.start], text[s.end:c.end]
			if defaultBefore.MatchString(before) || byDefaultAfter.MatchString(after) {
				kind = TokenDefault
			}
			content := strings.TrimSpace(s.content)
			switch {
			case s.quote == '[' || (strings.HasPrefix(content, "[") && strings.HasSuffix(content, "]") && isCharClass(content[1:len(content)-1])):
				chars, neg := expandClass(content)
				p := c.polarity
				if neg {
					p = p.opposite()
				}
				add(Token{Kind: TokenCharacterClass, Value: content, Polarity: p, chars: sortedRunes(chars)})
				prevValue = true
			case utf8.RuneCountInString(content) == 1:
				r, _ := utf8.DecodeRuneInString(content)
				add(Token{Kind: TokenCharacter, Value: content, Polarity: c.polarity, chars: []rune{r}})
				prevValue = true
			case strings.Contains(content, "=") && !strings.HasPrefix(content, "="):
				value(kind, content[strings.LastIndex(content, "=")+1:], c.polarity)
				prevValue = true
			case strings.Contains(content, ": "):
				value(kind, content[strings.LastIndex(content, ": ")+2:], c.polarity)
				prevValue = true
			case kind != TokenValue || valuePosition(text, s.start, prevValue):
				value(kind, content, c.polarity)
				prevValue = true
			default:
				if _, ok := booleanValue(content); ok {
					value(TokenValue, content, c.polarity)
				}
				prevValue = false
			}
		}
	}
	// unquoted tokens: booleans, defaults, before → after
	bare := []byte(text)
	for i := range bare {
		if inSpan[i] {
			bare[i] = ' '
		}
	}
	plain := string(bare)
	for _, c := range clauses(plain) {
		for _, w := range strings.FieldsFunc(c.text, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if b, ok := booleanValue(w); ok {
				add(Token{Kind: TokenBoolean, Value: b})
			}
		}
		for _, m := range bareDefault.FindAllStringSubmatch(c.text, -1) {
			if v := strings.TrimRight(m[1], ".,;:"); bareValue(v) {
				value(TokenDefault, v, PolarityUnknown)
			} else if b, ok := booleanWord(v); ok {
				add(Token{Kind: TokenDefault, Value: b})
			}
		}
		for _, m := range bareByDefault.FindAllStringSubmatch(c.text, -1) {
			if b, ok := booleanWord(m[1]); ok {
				add(Token{Kind: TokenDefault, Value: b})
			} else if bareValue(m[1]) {
				value(TokenDefault, m[1], PolarityUnknown)
			}
		}
		for _, re := range []*regexp.Regexp{fromTo, arrow} {
			for _, m := range re.FindAllStringSubmatch(c.text, -1) {
				a, b := strings.TrimRight(m[1], ".,;:"), strings.TrimRight(m[2], ".,;:")
				if bareValue(a) && bareValue(b) {
					value(TokenBefore, a, PolarityUnknown)
					value(TokenAfter, b, PolarityUnknown)
				}
			}
		}
	}
	// quoted before → after pairs
	for _, re := range []*regexp.Regexp{fromTo, arrow} {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			a, b := unquote(m[1]), unquote(m[2])
			if a != m[1] && b != m[2] && !isName(a) && !isName(b) {
				value(TokenBefore, a, PolarityUnknown)
				value(TokenAfter, b, PolarityUnknown)
			}
		}
	}
	return out
}

// unquote strips one pair of quotes (and trailing punctuation) from a word.
func unquote(w string) string {
	w = strings.TrimRight(w, ".,;:)")
	if len(w) >= 2 {
		f, l := w[0], w[len(w)-1]
		if f == l && (f == '`' || f == '"' || f == '\'') {
			return w[1 : len(w)-1]
		}
	}
	return w
}
