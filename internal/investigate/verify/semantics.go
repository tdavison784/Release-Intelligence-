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
//
// For a scope refinement (refinement non-nil) the clause naming the catalog
// role's condition values is not compared with the cited passages (lane
// verify3, D-0026): those values come from the catalog role and are
// established by the role's consumer exercise, not by the base rule's
// upstream citation. Each such token is recorded as a not-run check naming
// the condition and the method that establishes it; every other token is
// checked as before.
func CheckSemantics(rule domain.SemanticAssertion, evs []domain.Evidence, citations domain.ProofMethod, refinement *domain.ScopeRefinement) domain.ProofMethod {
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
	r := semantic.CheckAssertionExcluding(rule, passages, roleSourced(refinement))
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
	for _, x := range r.Excluded {
		m.Checks = append(m.Checks, domain.ProofCheck{Subject: x.Token.String(), Outcome: domain.MethodNotRun, Detail: "excluded: " + x.String()})
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

// roleSourced is a scope refinement's clause with the role's and the
// outside-role condition values (none without a refinement).
func roleSourced(r *domain.ScopeRefinement) []semantic.Sourced {
	if r == nil {
		return nil
	}
	s := semantic.Sourced{Text: r.Clause(), Source: "the catalog's consumer role " + r.Consumer, Method: domain.RefinementMethodPrefix + r.Consumer + ")"}
	for _, f := range r.Role {
		s.Conditions = append(s.Conditions, semantic.SourcedCondition{Condition: f.Path + " = " + f.Value + " (in the role)", Value: f.Value})
	}
	for _, f := range r.Outside {
		s.Conditions = append(s.Conditions, semantic.SourcedCondition{Condition: f.Path + " = " + f.Value + " (outside the role)", Value: f.Value})
	}
	return []semantic.Sourced{s}
}
