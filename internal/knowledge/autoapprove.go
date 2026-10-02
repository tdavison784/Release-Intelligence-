package knowledge

import (
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// ConsensusAspects finds, per aspect, a value that at least two proposals from
// SEPARATE model calls agree on (domain.SeparateCalls; any models, the same
// model twice included — PO-1) and that no validator refuted. The
// basis is the agreeing proposal ids. An aspect with two such competing values
// is not consensus.
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
		var winners []string
		for d, g := range groups {
			if hasSeparatePair(g) {
				winners = append(winners, d)
			}
		}
		if len(winners) != 1 {
			continue
		}
		g := groups[winners[0]]
		var ids []string
		for _, p := range g {
			ids = append(ids, p.ID)
		}
		sort.Strings(ids)
		out[x] = AutoApprovedAspect{Part: partOf(g[0].Assertion, x), Level: domain.VerifiedConsensus, Basis: ids}
	}
	return out
}

// CONTRACT-CHANGE(contract-3): PO-1 replaced IndependentModels (distinct
// model families) with SeparateCalls (distinct stateless calls).
func hasSeparatePair(ps []domain.SemanticProposal) bool {
	for i := range ps {
		for j := i + 1; j < len(ps); j++ {
			if domain.SeparateCalls(ps[i], ps[j]) {
				return true
			}
		}
	}
	return false
}

// AutoApproveRenderVerifiable is the auto-approval policy of RENDER-MISSION
// Goal 10: for a candidate whose class is render-verifiable, aspects no
// validator confirmed may still be verified by consensus of separate model
// calls; routing then mints the fact (marked auto-approved) once all four
// aspects are deterministic or consensus. Any other candidate, any aspect
// without separate-call agreement, and any aspect a validator refuted (e.g.
// contradicted by the render) is left to review.
func AutoApproveRenderVerifiable(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult, _ []AspectAgreement) *AutoApproval {
	if c.Renderability != domain.RenderVerifiable {
		return nil
	}
	cons := ConsensusAspects(ps, vs)
	if len(cons) == 0 {
		return nil
	}
	return &AutoApproval{Aspects: cons}
}
