// Package semantic is the semantic check (Phase 6 Milestone 4, mission E4,
// docs/phase6/decision-intel/PLAN-M4.md lane semverify): does a cited passage
// say what a claim says?
//
// The citation check (package verify) proves that a passage exists, lies in
// the rule's version bounds and names the subject. It does not read the
// passage. Here the value tokens a claim asserts (defaults, allowed or
// enumerated values, character classes, before/after values, booleans) are
// extracted deterministically and each must occur in a cited passage with
// the same polarity ("accepts" vs "rejects", "true" vs "false", "enabled"
// vs "disabled"):
//
//   - a token no passage carries, or one every passage states with the
//     opposite polarity, makes the claim unsupported;
//   - a token whose polarity cannot be read in the passage, or a claim that
//     asserts no token at all, is undetermined: a human judges it (never a
//     model);
//   - otherwise the claim is supported.
//
// The extraction is lexical and generic: no product, file or field names.
// It is deliberately narrow (values in value positions, never setting
// names), so that "unsupported" means a token is really absent or reversed.
// Pure Go, no I/O.
package semantic

import (
	"fmt"
	"sort"
	"strings"
)

// Producer names the check (component@vN).
const Producer = "verify.semantic-check@v1"

// Verdict is what the check concluded about a claim (or one token).
type Verdict string

const (
	Supported    Verdict = "supported"
	Unsupported  Verdict = "unsupported"
	Undetermined Verdict = "undetermined"
)

// TokenKind is the role of a value token in the claim.
type TokenKind string

const (
	TokenValue          TokenKind = "value"           // an allowed, enumerated or set value
	TokenDefault        TokenKind = "default"         // a default value
	TokenBefore         TokenKind = "before"          // the value before a change
	TokenAfter          TokenKind = "after"           // the value after a change
	TokenBoolean        TokenKind = "boolean"         // true / false (enabled / disabled)
	TokenCharacter      TokenKind = "character"       // one character
	TokenCharacterClass TokenKind = "character-class" // a set of characters ([A-Za-z0-9._-])
)

// Polarity is whether a clause affirms (accepts, allows, valid, enabled) or
// denies (rejects, forbids, invalid, disabled) what it names.
type Polarity string

const (
	PolarityUnknown  Polarity = ""
	PolarityPositive Polarity = "positive"
	PolarityNegative Polarity = "negative"
)

func (p Polarity) opposite() Polarity {
	switch p {
	case PolarityPositive:
		return PolarityNegative
	case PolarityNegative:
		return PolarityPositive
	}
	return PolarityUnknown
}

// Token is one value a claim asserts.
type Token struct {
	Kind  TokenKind `json:"kind"`
	Value string    `json:"value"`
	// Polarity is the claim's polarity for the token ("" when the claim
	// asserts none: presence is then enough). A boolean carries its
	// polarity in its value.
	Polarity Polarity `json:"polarity,omitempty"`
	// chars are a character or character-class token's characters.
	chars []rune
}

func (t Token) String() string {
	s := string(t.Kind) + " " + quote(t.Value)
	if t.Polarity != PolarityUnknown {
		s += " (" + string(t.Polarity) + ")"
	}
	return s
}

func quote(s string) string {
	if strings.Contains(s, "`") {
		return fmt.Sprintf("%q", s)
	}
	return "`" + s + "`"
}

// TokenCheck is the check of one token against the passages.
type TokenCheck struct {
	Token   Token   `json:"token"`
	Verdict Verdict `json:"verdict"`
	Detail  string  `json:"detail"`
}

// Result is the check of one claim.
type Result struct {
	Verdict Verdict      `json:"verdict"`
	Detail  string       `json:"detail"`
	Tokens  []TokenCheck `json:"tokens,omitempty"`
}

// Check checks a free-text claim against its cited passages.
func Check(claim string, passages []string) Result {
	return CheckTokens(ExtractTokens(claim), passages)
}

// CheckTokens checks extracted tokens against the passages: unsupported
// when any token is absent or reversed, undetermined when any cannot be
// decided or there is none, supported otherwise.
func CheckTokens(tokens []Token, passages []string) Result {
	var live []string
	for _, p := range passages {
		if strings.TrimSpace(p) != "" {
			live = append(live, p)
		}
	}
	if len(live) == 0 {
		return Result{Verdict: Undetermined, Detail: "no cited passage to compare the claim with: a human judges it"}
	}
	if len(tokens) == 0 {
		return Result{Verdict: Undetermined, Detail: "the claim asserts no value token (default, allowed value, character class, before/after value, boolean) a deterministic check can compare: a human judges whether the passage states it"}
	}
	ps := analyse(live)
	res := Result{}
	var unsupported, undetermined []string
	for _, t := range tokens {
		tc := ps.check(t)
		res.Tokens = append(res.Tokens, tc)
		switch tc.Verdict {
		case Unsupported:
			unsupported = append(unsupported, tc.Detail)
		case Undetermined:
			undetermined = append(undetermined, tc.Detail)
		}
	}
	switch {
	case len(unsupported) > 0:
		res.Verdict = Unsupported
		res.Detail = fmt.Sprintf("%d of %d asserted tokens are not supported by the cited passages: %s", len(unsupported), len(tokens), strings.Join(unsupported, "; "))
	case len(undetermined) > 0:
		res.Verdict = Undetermined
		res.Detail = fmt.Sprintf("%d of %d asserted tokens occur in the cited passages, but the check cannot decide their polarity there: %s — a human judges it", len(undetermined), len(tokens), strings.Join(undetermined, "; "))
	default:
		res.Verdict = Supported
		res.Detail = fmt.Sprintf("every asserted token (%d) occurs in the cited passages with the same polarity", len(tokens))
	}
	return res
}

// --- passages ------------------------------------------------------------------------------

// occurrence is one place a passage states a token: its clause's polarity.
type occurrence struct {
	polarity Polarity
	// exhaustive: the clause states a complete allowed set (a regex
	// character class, or a list introduced by "only").
	exhaustive bool
	chars      map[rune]bool
	text       string
}

type passages struct {
	clauses []clause
	// charSets are the character sets the passages state.
	charSets []occurrence
}

func analyse(texts []string) *passages {
	ps := &passages{}
	for _, t := range texts {
		for _, c := range clauses(normalize(t)) {
			ps.clauses = append(ps.clauses, c)
			ps.charSets = append(ps.charSets, charSetsOf(c)...)
		}
	}
	return ps
}

func (ps *passages) check(t Token) TokenCheck {
	switch t.Kind {
	case TokenCharacter, TokenCharacterClass:
		return ps.checkChars(t)
	case TokenBoolean:
		return ps.checkBoolean(t)
	}
	if b, ok := booleanValue(t.Value); ok && t.Kind != TokenValue {
		// a default / before / after that is a boolean compares like one
		tc := ps.checkBoolean(Token{Kind: TokenBoolean, Value: b})
		tc.Token = t
		return tc
	}
	var occ []Polarity
	for _, c := range ps.clauses {
		if containsWord(c.text, t.Value) {
			occ = append(occ, c.polarity)
		}
	}
	if len(occ) == 0 {
		return TokenCheck{Token: t, Verdict: Unsupported, Detail: fmt.Sprintf("%s does not occur in the cited passages", t)}
	}
	return polarityVerdict(t, occ)
}

// polarityVerdict compares the claim's polarity with the passages' at the
// token's occurrences.
func polarityVerdict(t Token, occ []Polarity) TokenCheck {
	if t.Polarity == PolarityUnknown {
		return TokenCheck{Token: t, Verdict: Supported, Detail: fmt.Sprintf("%s occurs in the cited passages", t)}
	}
	known, opposite := 0, 0
	for _, p := range occ {
		switch p {
		case t.Polarity:
			return TokenCheck{Token: t, Verdict: Supported, Detail: fmt.Sprintf("%s occurs in the cited passages with the same polarity", t)}
		case t.Polarity.opposite():
			opposite++
			known++
		}
	}
	if known > 0 && opposite == len(occ) {
		return TokenCheck{Token: t, Verdict: Unsupported, Detail: fmt.Sprintf("%s: the cited passages state it with the opposite polarity (%s)", t, t.Polarity.opposite())}
	}
	return TokenCheck{Token: t, Verdict: Undetermined, Detail: fmt.Sprintf("%s occurs in the cited passages, but its polarity there cannot be read", t)}
}

// checkBoolean: the value or a synonym must occur; only the opposite value
// is a flip.
func (ps *passages) checkBoolean(t Token) TokenCheck {
	want := t.Value
	other := "false"
	if want == "false" {
		other = "true"
	}
	found, flipped := false, false
	for _, c := range ps.clauses {
		for _, w := range c.words {
			if b, ok := booleanWord(w); ok {
				if b == want {
					found = true
				} else if b == other {
					flipped = true
				}
			}
		}
	}
	switch {
	case found:
		return TokenCheck{Token: t, Verdict: Supported, Detail: fmt.Sprintf("%s occurs in the cited passages", t)}
	case flipped:
		return TokenCheck{Token: t, Verdict: Unsupported, Detail: fmt.Sprintf("%s: the cited passages state %s, the opposite", t, other)}
	}
	return TokenCheck{Token: t, Verdict: Unsupported, Detail: fmt.Sprintf("%s does not occur in the cited passages", t)}
}

// checkChars: every character must be in a character set a passage
// states, with the same polarity. A character absent from an exhaustive
// allowed set is a negative statement about it.
func (ps *passages) checkChars(t Token) TokenCheck {
	var missing []string
	var undecided []string
	var flipped []string
	for _, r := range t.chars {
		var occ []Polarity
		exhaustivePositive := false
		for _, s := range ps.charSets {
			if s.chars[r] {
				occ = append(occ, s.polarity)
			} else if s.exhaustive && s.polarity == PolarityPositive {
				exhaustivePositive = true
			}
		}
		if len(occ) == 0 {
			if exhaustivePositive && t.Polarity == PolarityNegative {
				continue // not in the complete allowed set: rejected, as claimed
			}
			missing = append(missing, quote(string(r)))
			continue
		}
		switch polarityVerdict(t, occ).Verdict {
		case Unsupported:
			flipped = append(flipped, quote(string(r)))
		case Undetermined:
			undecided = append(undecided, quote(string(r)))
		}
	}
	switch {
	case len(missing) > 0 && len(flipped) > 0:
		return TokenCheck{Token: t, Verdict: Unsupported, Detail: fmt.Sprintf("%s: %s in no character set the cited passages state, and %s with the opposite polarity", t, strings.Join(missing, " "), strings.Join(flipped, " "))}
	case len(missing) > 0:
		return TokenCheck{Token: t, Verdict: Unsupported, Detail: fmt.Sprintf("%s: %s in no character set the cited passages state", t, strings.Join(missing, " "))}
	case len(flipped) > 0:
		return TokenCheck{Token: t, Verdict: Unsupported, Detail: fmt.Sprintf("%s: the cited passages state %s with the opposite polarity", t, strings.Join(flipped, " "))}
	case len(undecided) > 0:
		return TokenCheck{Token: t, Verdict: Undetermined, Detail: fmt.Sprintf("%s: %s occur in a character set of the cited passages, but its polarity cannot be read", t, strings.Join(undecided, " "))}
	}
	return TokenCheck{Token: t, Verdict: Supported, Detail: fmt.Sprintf("%s: every character occurs in a character set of the cited passages with the same polarity", t)}
}

// containsWord reports whether tok occurs in text, not as part of a longer
// identifier or number (case-sensitive: values are).
func containsWord(text, tok string) bool {
	if tok == "" {
		return false
	}
	for i := 0; ; {
		j := strings.Index(text[i:], tok)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(tok)
		if !identByteAt(text, start-1) && !identByteAt(text, end) {
			return true
		}
		i = start + 1
	}
}

func identByteAt(s string, i int) bool {
	if i < 0 || i >= len(s) {
		return false
	}
	b := s[i]
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func sortedRunes(m map[rune]bool) []rune {
	out := make([]rune, 0, len(m))
	for r := range m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
