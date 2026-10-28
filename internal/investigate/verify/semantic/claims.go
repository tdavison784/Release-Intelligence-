package semantic

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// AssertionTokens returns the value tokens a structured rule asserts about
// upstream behaviour: its change's before and after values and the tokens
// of its statement. The before/after values assert presence only; the
// statement's tokens carry its clauses' polarity. The applicability
// condition's values are not among them: they scope the environment the
// rule applies to (often through a consumer role or an alternative that
// shields it), are not what an upstream passage states, and are tested by
// the rule exercise's two arms and the applicability checks instead.
func AssertionTokens(a domain.SemanticAssertion) []Token {
	var out []Token
	seen := map[string]bool{}
	add := func(t Token) {
		k := tokenKey(t)
		if t.Value != "" && !seen[k] {
			seen[k] = true
			out = append(out, t)
		}
	}
	plain := func(kind TokenKind, v string) {
		v = strings.TrimSpace(v)
		var s string
		if json.Unmarshal([]byte(v), &s) == nil {
			v = s
		}
		if b, ok := booleanValue(v); ok {
			if kind == TokenValue {
				kind = TokenBoolean
			}
			add(Token{Kind: kind, Value: b})
			return
		}
		if !isName(v) {
			add(Token{Kind: kind, Value: v})
		}
	}
	if c := a.Change; c != nil && c.Type != domain.ChangeKindRequirementChanged {
		if c.Before != nil {
			plain(TokenBefore, *c.Before)
		}
		if c.After != nil {
			plain(TokenAfter, *c.After)
		}
	}
	for _, t := range ExtractTokens(a.Statement) {
		add(t)
	}
	return out
}

// CheckAssertion checks a structured rule against its cited passages.
func CheckAssertion(a domain.SemanticAssertion, passages []string) Result {
	return CheckTokens(AssertionTokens(a), passages)
}

// Sourced is a part of a rule's statement that states conditions whose
// values do not come from the rule's upstream citation (lane verify3,
// D-0026): for example a scope refinement's clause, which names the catalog
// consumer role's field values, established by the role's consumer exercise.
type Sourced struct {
	// Text is the statement text that states the conditions, verbatim.
	Text string
	// Conditions are the conditions it states.
	Conditions []SourcedCondition
	// Source names where the values come from; Method what establishes the
	// conditions instead of the cited passages.
	Source, Method string
}

// SourcedCondition is one condition of a Sourced text: its description and
// its JSON-encoded value.
type SourcedCondition struct {
	Condition string
	Value     string
}

// Exclusion is a token the check does not compare with the cited passages,
// because it belongs to a condition another method establishes.
type Exclusion struct {
	Token     Token  `json:"token"`
	Condition string `json:"condition"`
	Source    string `json:"source"`
	Method    string `json:"method"`
}

func (x Exclusion) String() string {
	return fmt.Sprintf("%s belongs to condition %s (%s), established by %s, not by the cited passages", x.Token, x.Condition, x.Source, x.Method)
}

// CheckAssertionExcluding is CheckAssertion with the tokens of sourced
// conditions excluded: a Sourced text found verbatim in the statement is
// taken out of it; each of its tokens whose value is one of its conditions'
// values is recorded as excluded, any other token of it is checked as usual.
// Everything else is checked exactly as CheckAssertion does; a Sourced text
// the statement does not contain verbatim excludes nothing.
func CheckAssertionExcluding(a domain.SemanticAssertion, passages []string, sourced []Sourced) Result {
	rest := a
	var extra []Token
	var excluded []Exclusion
	for _, s := range sourced {
		text := strings.TrimSpace(s.Text)
		if text == "" || !strings.Contains(rest.Statement, text) {
			continue
		}
		rest.Statement = strings.Replace(rest.Statement, text, " ", 1)
		for _, t := range ExtractTokens(text) {
			if c, ok := sourcedCondition(t, s.Conditions); ok {
				excluded = append(excluded, Exclusion{Token: t, Condition: c.Condition, Source: s.Source, Method: s.Method})
				continue
			}
			extra = append(extra, t)
		}
	}
	tokens := AssertionTokens(rest)
	seen := map[string]bool{}
	for _, t := range tokens {
		seen[tokenKey(t)] = true
	}
	for _, t := range extra {
		if !seen[tokenKey(t)] {
			seen[tokenKey(t)] = true
			tokens = append(tokens, t)
		}
	}
	r := CheckTokens(tokens, passages)
	if len(excluded) > 0 {
		var xs []string
		for _, x := range excluded {
			xs = append(xs, x.String())
		}
		r.Excluded = excluded
		r.Detail += fmt.Sprintf("; %d token(s) excluded: %s", len(excluded), strings.Join(xs, "; "))
	}
	return r
}

func tokenKey(t Token) string { return string(t.Kind) + "\x00" + t.Value + "\x00" + string(t.Polarity) }

// sourcedCondition is the condition whose value the token states.
func sourcedCondition(t Token, cs []SourcedCondition) (SourcedCondition, bool) {
	for _, c := range cs {
		v := strings.TrimSpace(c.Value)
		var s string
		if json.Unmarshal([]byte(v), &s) == nil {
			v = s
		}
		if b, ok := booleanValue(v); ok {
			v = b
		}
		if t.Value == v {
			return c, true
		}
	}
	return SourcedCondition{}, false
}

// BehaviourCheck is the report-only check of one agent behaviour entry
// sourced upstream: its statement against the passages its citations quote.
// It is recorded and shown, and never changes a classification.
type BehaviourCheck struct {
	Assessment string              `json:"assessment"`
	ChangeItem string              `json:"changeItem"`
	Behaviour  string              `json:"behaviour"`
	Citations  []domain.EvidenceID `json:"citations"`
	// Unresolved are citations the evidence pool does not hold.
	Unresolved []domain.EvidenceID `json:"unresolved,omitempty"`
	Result
}

// CheckBehaviours checks every behaviour entry sourced upstream of the
// assessments; pool holds the run's evidence (the cited passages are the
// records' excerpts).
func CheckBehaviours(assessments []domain.AgentAssessment, pool map[domain.EvidenceID]domain.Evidence) []BehaviourCheck {
	var out []BehaviourCheck
	for _, a := range assessments {
		for _, b := range a.Behaviour {
			if b.Source != domain.BehaviourUpstream {
				continue
			}
			bc := BehaviourCheck{Assessment: a.ID, ChangeItem: a.ChangeItem, Behaviour: b.Behaviour, Citations: b.Citations}
			var passages []string
			for _, id := range b.Citations {
				e, ok := pool[id]
				if !ok {
					bc.Unresolved = append(bc.Unresolved, id)
					continue
				}
				passages = append(passages, strings.TrimSuffix(strings.TrimSpace(e.Excerpt), "…"))
			}
			bc.Result = Check(b.Behaviour, passages)
			out = append(out, bc)
		}
	}
	return out
}
