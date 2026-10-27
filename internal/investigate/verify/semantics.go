package verify

import (
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/verify/semantic"
)

// SemanticProducer names the semantic check (component@vN).
const SemanticProducer = semantic.Producer

// CheckSemantics runs the semantic check (Phase 6 M4, mission E4) over the
// passages the citation check established: the value tokens the rule
// asserts must each occur in one of them with the same polarity. Unsupported
// fails the method (the proof is refuted); a claim the check cannot decide is
// inconclusive and left to the human reviewer, never to a model. Without an
// established passage it does not run.
func CheckSemantics(rule domain.SemanticAssertion, evs []domain.Evidence, citations domain.ProofMethod) domain.ProofMethod {
	m := domain.ProofMethod{Method: domain.ProofSemanticCheck, Producer: SemanticProducer, Level: domain.LevelDeterministic}
	established := map[string]bool{}
	for _, c := range citations.Checks {
		if c.Outcome == domain.MethodPass {
			established[c.Subject] = true
		}
	}
	var passages []string
	var used []domain.Evidence
	for _, ev := range evs {
		if established[string(ev.ID)] {
			passages = append(passages, strings.TrimSuffix(strings.TrimSpace(ev.Excerpt), "…"))
			used = append(used, ev)
		}
	}
	if len(passages) == 0 {
		m.Outcome, m.Detail = domain.MethodNotRun, "no cited passage passed the citation check, so there is no established passage to compare the rule with"
		return m
	}
	r := semantic.CheckAssertion(rule, passages)
	for _, tc := range r.Tokens {
		c := domain.ProofCheck{Subject: tc.Token.String(), Detail: tc.Detail}
		switch tc.Verdict {
		case semantic.Supported:
			c.Outcome = domain.MethodPass
		case semantic.Unsupported:
			c.Outcome = domain.MethodFail
		default:
			c.Outcome = domain.MethodInconclusive
		}
		m.Checks = append(m.Checks, c)
	}
	switch r.Verdict {
	case semantic.Supported:
		m.Outcome, m.Detail, m.Evidence = domain.MethodPass, r.Detail, used
	case semantic.Unsupported:
		m.Outcome, m.Detail = domain.MethodFail, "the rule is unsupported by its cited passages: "+r.Detail
	default:
		m.Outcome, m.Detail = domain.MethodInconclusive, r.Detail+" (routed to the human reviewer, never to a model)"
	}
	return m
}
