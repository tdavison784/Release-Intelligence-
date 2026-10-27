package semantic

import (
	"encoding/json"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// AssertionTokens returns the value tokens a structured rule asserts: its
// change's before and after values, the values its applicability
// conditions compare with, and the tokens of its statement. Structured
// values assert presence only (their polarity is the rule's structure, not
// a word); the statement's tokens carry its clauses' polarity.
func AssertionTokens(a domain.SemanticAssertion) []Token {
	var out []Token
	seen := map[string]bool{}
	add := func(t Token) {
		k := string(t.Kind) + "\x00" + t.Value + "\x00" + string(t.Polarity)
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
	if ap := a.Applicability; ap != nil {
		conds := []domain.Condition{ap.Exposure}
		if ap.Overlap != nil {
			conds = append(conds, *ap.Overlap)
		}
		for len(conds) > 0 {
			c := conds[0]
			conds = append(conds[1:], c.Of...)
			switch c.State {
			case domain.StateEquals, domain.StateNotEquals:
				for _, v := range c.Values {
					var x any
					if json.Unmarshal([]byte(v), &x) != nil {
						continue
					}
					switch x := x.(type) {
					case string:
						plain(TokenValue, x)
					case bool, float64:
						plain(TokenValue, v)
					}
				}
			case domain.StateHasToken:
				for _, v := range c.Values {
					if k, val, ok := strings.Cut(v, "="); ok && k != "" {
						v = val
					}
					plain(TokenValue, v)
				}
			}
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
