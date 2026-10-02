package knowledge

import "github.com/tdavison784/release-intelligence/internal/domain"

// renderRank orders render relations by how decisive they are for a reviewer.
var renderRank = map[domain.RenderRelation]int{
	domain.RenderContradicted: 4, domain.RenderConfirmed: 3, domain.RenderNotVisible: 2, domain.RenderNotApplicable: 1,
}

// RenderEvidenceOf picks the render evidence of a candidate for its review
// context: the most decisive render-based validation of the candidate (ties:
// the one with the most rendered records, then the smallest id), with its
// release-scope rendered records. CONTRACT-CHANGE(render).
func RenderEvidenceOf(candidateID string, vs []domain.ValidationResult) *RenderEvidence {
	var best *domain.ValidationResult
	for i := range vs {
		v := &vs[i]
		if v.CandidateID != candidateID || v.RenderRelation == "" {
			continue
		}
		switch {
		case best == nil,
			renderRank[v.RenderRelation] > renderRank[best.RenderRelation],
			renderRank[v.RenderRelation] == renderRank[best.RenderRelation] && len(v.Evidence) > len(best.Evidence),
			renderRank[v.RenderRelation] == renderRank[best.RenderRelation] && len(v.Evidence) == len(best.Evidence) && v.ID < best.ID:
			best = v
		}
	}
	if best == nil {
		return nil
	}
	re := &RenderEvidence{Validation: best.ID, Relation: best.RenderRelation}
	for _, c := range best.Checks {
		if c.Detail != "" {
			re.Explanation = c.Detail
			break
		}
	}
	for _, e := range best.Evidence {
		if e.Render != nil && e.Render.Scope == domain.RenderRelease {
			re.Evidence = append(re.Evidence, e)
		}
	}
	return re
}
