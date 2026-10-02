package knowledge

import (
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ConsensusAspects finds, per aspect, a value that at least two SEPARATE model
// calls agree on (PO-1: any models, including two calls of the same model;
// domain.SeparateCalls) and that no validator refuted. The basis is one
// proposal per call, and the aspect is labelled cross-model or same-model
// (domain.ConsensusScopeOf) so the two can be measured against each other.
// An aspect with two competing agreed values is not consensus. For an
// action-eligible consequence the basis prefers the proposals that requested
// action-required (PO-2): ≥2 separate calls among them make a basis the
// consensus-action rule can use.
func ConsensusAspects(ps []domain.SemanticProposal, vs []domain.ValidationResult) map[domain.Aspect]AutoApprovedAspect {
	refuted := refutedDigests(vs)
	out := map[domain.Aspect]AutoApprovedAspect{}
	for _, x := range domain.Aspects {
		groups := map[string][]domain.SemanticProposal{}
		for _, p := range ps {
			if p.Assertion.Has(x) {
				d := p.Assertion.AspectDigest(x)
				if !refuted[x][d] {
					groups[d] = append(groups[d], p)
				}
			}
		}
		type win struct {
			basis []domain.SemanticProposal
		}
		var wins []win
		for _, g := range groups {
			basis := separateCalls(g)
			if x == domain.AspectConsequence && g[0].Assertion.Consequence.Kind.ActionEligible() {
				var req []domain.SemanticProposal
				for _, p := range g {
					if p.SuggestedClass == domain.ImpactActionRequired {
						req = append(req, p)
					}
				}
				if r := separateCalls(req); len(r) >= 2 {
					basis = r
				}
			}
			if len(basis) >= 2 {
				wins = append(wins, win{basis})
			}
		}
		if len(wins) != 1 {
			continue
		}
		b := wins[0].basis
		ids := make([]string, 0, len(b))
		for _, p := range b {
			ids = append(ids, p.ID)
		}
		sort.Strings(ids)
		out[x] = AutoApprovedAspect{Part: partOf(b[0].Assertion, x), Level: domain.VerifiedConsensus, Basis: ids, Consensus: domain.ConsensusScopeOf(b)}
	}
	return out
}

// separateCalls keeps one proposal per call id (first by proposal id) and
// returns them only when at least two calls are separate; otherwise nil.
func separateCalls(ps []domain.SemanticProposal) []domain.SemanticProposal {
	sorted := append([]domain.SemanticProposal(nil), ps...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	seen := map[string]bool{}
	var out []domain.SemanticProposal
	for _, p := range sorted {
		k := callKey(p)
		if !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func anyRefuted(vs []domain.ValidationResult) bool {
	for _, v := range vs {
		for _, c := range v.Checks {
			if c.Outcome == domain.OutcomeRefuted {
				return true
			}
		}
	}
	return false
}

// AutoApproveRenderVerifiable is the auto-approval policy of RENDER-MISSION
// Goal 10 (DESIGN §6): the candidate's class is render-verifiable, subject and
// change are confirmed by the render (a validation with renderRelation
// confirmed-by-render confirming both), and ≥2 separate calls agree on the
// remaining aspects. Anything else, and any refutation, is left to review.
func AutoApproveRenderVerifiable(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult, _ []AspectAgreement) *AutoApproval {
	if c.Renderability != domain.RenderVerifiable || anyRefuted(vs) {
		return nil
	}
	rendered := map[domain.Aspect]bool{}
	for _, v := range vs {
		if v.RenderRelation == domain.RenderConfirmed {
			for _, ch := range v.Checks {
				if ch.Outcome == domain.OutcomeConfirmed {
					rendered[ch.Aspect] = true
				}
			}
		}
	}
	if !rendered[domain.AspectSubject] || !rendered[domain.AspectChange] {
		return nil
	}
	cons := ConsensusAspects(ps, vs)
	delete(cons, domain.AspectSubject)
	delete(cons, domain.AspectChange)
	if len(cons) == 0 {
		return nil
	}
	return &AutoApproval{Aspects: cons}
}

// AutoApproveConsensusAction is the PO-2 row of DESIGN §6: whatever the
// candidate's class, when ≥2 separate calls agree on an action-eligible
// consequence and all requested action-required, every other aspect is
// validator-confirmed or agreed by consensus, and nothing is refuted, the fact
// is minted with ConsensusAction (buildFact sets it) and is always audited.
func AutoApproveConsensusAction(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult, _ []AspectAgreement) *AutoApproval {
	if anyRefuted(vs) {
		return nil
	}
	cons := ConsensusAspects(ps, vs)
	cq, ok := cons[domain.AspectConsequence]
	if !ok || cq.Part.Consequence == nil || !cq.Part.Consequence.Kind.ActionEligible() {
		return nil
	}
	byID := map[string]domain.SemanticProposal{}
	for _, p := range ps {
		byID[p.ID] = p
	}
	for _, id := range cq.Basis {
		if byID[id].SuggestedClass != domain.ImpactActionRequired {
			return nil
		}
	}
	confirmed := stateFromValidations(vs)
	out := &AutoApproval{Aspects: map[domain.Aspect]AutoApprovedAspect{}}
	for _, x := range domain.Aspects {
		if _, ok := confirmed[x]; ok {
			continue
		}
		a, ok := cons[x]
		if !ok {
			return nil
		}
		out.Aspects[x] = a
	}
	return out
}

// CombinePolicies tries each policy in order; the first that approves wins.
func CombinePolicies(policies ...AutoApprovePolicy) AutoApprovePolicy {
	return func(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult, ag []AspectAgreement) *AutoApproval {
		for _, p := range policies {
			if ap := p(c, ps, vs, ag); ap != nil {
				return ap
			}
		}
		return nil
	}
}

// DefaultAutoApprove is what `ri knowledge route` installs: the render
// auto-approval (Goal 10) and the consensus-action path (PO-2). RouteWith only
// honours a policy that completes all four aspects.
var DefaultAutoApprove = CombinePolicies(AutoApproveRenderVerifiable, AutoApproveConsensusAction)
