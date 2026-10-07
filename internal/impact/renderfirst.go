package impact

// CONTRACT-CHANGE(renderfirst): PO-7a decision order for knowledge
// composition (briefs/renderfirst.md item 3): for a render-verifiable change
// with a decisive customer render, the rendered delta decides EXPOSURE — the
// fact's applicability condition only decides what rendering can't. The
// consequence still comes from knowledge (trust ladder unchanged: a render
// delta alone never yields ACTION), so a trusted fact may raise an
// attributable render to action-required, and a decisive no-effect stops a
// fact's exposure claim from overriding the render. Without --render, or when
// no render decides, composition is exactly today's.

import (
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/upgrade"
)

// renderExposure is the customer's render's exposure decision for one joined
// change, when it decisively decides one (ok). It mirrors the deterministic
// join's own dispatch per family:
//   - values:removed with the key matched (the customer sets it): the
//     evaluator's rejection/attributable → True, no-effect → False;
//   - values:default-changed / added with the key unmatched (the customer
//     leaves it unset): the PO-3 counterfactual's True/False;
//   - images:removed / moved / tag-changed with the repository referenced:
//     any attributable repo → True, every referenced repo no-effect → False
//     (one undecided repo leaves the change to the fact's condition).
func (b *builder) renderExposure(c domain.Change) (ConditionResult, bool) {
	switch valuesDiffKind(c.Provenance.Rule) {
	case "removed":
		if b.set == nil || !b.env.Supplied.Values || !b.valuesMatched(c.Subjects) {
			return ConditionResult{}, false
		}
		switch r := b.set.EvaluateSetValues(c); r.Outcome {
		case SetValuesRejected:
			return ConditionResult{Value: True, Matches: []domain.ImpactMatch{*r.Rejection}, Records: r.Records}, true
		case SetValuesAttributable:
			return ConditionResult{Value: True, Matches: r.Matches, Records: r.Records}, true
		case SetValuesNoEffect:
			return ConditionResult{Value: False, Checks: r.Checks, Records: r.Records}, true
		}
	case "default-changed", "added":
		if b.unset == nil || !b.env.Supplied.Values || b.valuesMatched(c.Subjects) {
			return ConditionResult{}, false
		}
		if r := b.unset.EvaluateUnsetValues(c, valuesDiffKind(c.Provenance.Rule)); r.Value == True || r.Value == False {
			return r, true
		}
	default:
		switch c.Provenance.Rule {
		case upgrade.RuleImageRemoved, upgrade.RuleImageMoved, upgrade.RuleImageTagsChanged:
		default:
			return ConditionResult{}, false
		}
		if b.image == nil || !b.imagesVisible() {
			return ConditionResult{}, false
		}
		var (
			matches []domain.ImpactMatch
			records []domain.Evidence
			checks  []domain.ImpactCheck
			refd    int
		)
		for _, repo := range c.Subjects {
			if len(b.imagesForRepo(repo)) == 0 {
				continue // not referenced: the join decided it (image-not-referenced)
			}
			refd++
			switch r := b.image.EvaluateImage(c, repo); r.Outcome {
			case ImageAttributable:
				matches = append(matches, r.Matches...)
				records = append(records, r.Records...)
			case ImageNoEffect:
				checks = append(checks, r.Checks...)
				records = append(records, r.Records...)
			default:
				return ConditionResult{}, false
			}
		}
		if refd == 0 {
			return ConditionResult{}, false
		}
		if len(matches) > 0 {
			return ConditionResult{Value: True, Matches: matches, Records: records}, true
		}
		if len(checks) > 0 {
			return ConditionResult{Value: False, Checks: checks, Records: records}, true
		}
	}
	return ConditionResult{}, false
}

// valuesMatched reports whether any subject of the change is at, over or
// under a key the customer's values files set (the join's own match
// predicate).
func (b *builder) valuesMatched(subjects []string) bool {
	for _, s := range subjects {
		for _, k := range b.env.ValuesKeys {
			if relate(s, k.Path) != relNone {
				return true
			}
		}
	}
	return false
}
