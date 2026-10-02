package impact

// CONTRACT-CHANGE(render): product-owner decision PO-3 (DECISIONS.md;
// contract-4) — a changed chart default or a new key the customer leaves
// UNSET is decided by rendering. The rule ids, the render check
// (ImpactCheck.Render) and their shapes are the contract's
// (domain.RuleValuesDefault*); the render lane implements the evaluator
// (internal/render.UnsetValues).

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// PO-3 join rules (aliases of the domain constants, like every Rule*).
const (
	RuleValuesDefaultApplies    = domain.RuleValuesDefaultApplies
	RuleValuesDefaultNoEffect   = domain.RuleValuesDefaultNoEffect
	RuleValuesDefaultUnrendered = domain.RuleValuesDefaultUnrendered
)

// UnsetValuesEvaluator decides, for a values default-changed / added change
// whose keys the customer does not set, whether the new default changes the
// customer's rendered deployment:
//
//   - True: the customer From/To renders differ and the counterfactual (target
//     with the keys pinned to their previous state) attributes the difference
//     to them; Matches cite environment-render evidence, Records hold it.
//   - False: every render succeeded with complete values and nothing is
//     attributable; Checks hold the render check (Render.Outcome
//     no-attributable-change) citing environment-render evidence in Records.
//   - Unknown: no render was possible; Needed says why.
type UnsetValuesEvaluator interface {
	EvaluateUnsetValues(c domain.Change, kind string) ConditionResult
}

// unsetValues emits the PO-3 verdict of a default-changed / added change
// whose keys the customer leaves unset. Without an evaluator (no --render)
// the verdict is values-default-unrendered: not-affected as before, with the
// missing render visible on the finding.
func (b *builder) unsetValues(ev UnsetValuesEvaluator, c domain.Change, kind string) {
	keys := strings.Join(c.Subjects, ", ")
	var r ConditionResult
	if ev == nil {
		r = unknownResult(domain.UnknownEnvironmentVisibilityGap, "rendering was not requested (run ri impact with --render)")
	} else {
		r = ev.EvaluateUnsetValues(c, kind)
	}
	for _, rec := range r.Records {
		if _, ok := b.extraLocal[rec.ID]; !ok {
			b.extraLocal[rec.ID] = rec
			b.extraLocalOrder = append(b.extraLocalOrder, rec.ID)
		}
	}
	what := "default changed"
	if kind == "added" {
		what = "is new"
	}
	toTag := b.edge.To.String()
	switch r.Value {
	case True:
		var subs []string
		for _, m := range r.Matches {
			subs = append(subs, m.Subject)
		}
		title := fmt.Sprintf("You leave %s unset; its new default changes your rendered deployment", codeList(c.Subjects, 3))
		detail := fmt.Sprintf("Your values do not set %s, whose default %s in %s, so the new default applies to you. Rendering the target with your configuration, once as you will get it and once with the key pinned to its previous state, attributes these changes of your deployment to it:\n  - %s\nReview them before upgrading, or pin the key to keep the previous behaviour.",
			keys, what, toTag, strings.Join(subs, "\n  - "))
		b.add(RuleValuesDefaultApplies, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh, title, detail, c, r.Matches, c.Evidence...)
	case False:
		title := fmt.Sprintf("You leave %s unset; rendering shows its new default changes nothing for you", codeList(c.Subjects, 3))
		detail := fmt.Sprintf("Your values do not set %s, whose default %s in %s. Rendering the target with your configuration, once as you will get it and once with the key pinned to its previous state, shows no difference in your upgrade attributable to it.", keys, what, toTag)
		b.verdict(RuleValuesDefaultNoEffect, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			append([]domain.ImpactCheck{b.valuesCheck(c.Subjects)}, r.Checks...))
	default:
		reason := "render unavailable"
		if len(r.Needed) > 0 {
			reason = strings.Join(r.Needed, "; ")
		}
		title := fmt.Sprintf("You leave %s unset; not rendered, so its new default was not checked", codeList(c.Subjects, 3))
		if len(c.Subjects) == 1 {
			title = fmt.Sprintf("Your values do not set %s (new default not rendered)", code(c.Subjects[0]))
		}
		detail := fmt.Sprintf("Your values do not set %s, whose default %s in %s, so the new default applies to you. Whether it changes your deployment needs a render, which was not available (%s). Not affected is the product owner's default for this case; run with --render to decide it.", keys, what, toTag, reason)
		b.verdict(RuleValuesDefaultUnrendered, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence, []domain.ImpactCheck{
			b.valuesCheck(c.Subjects),
			{Dimension: domain.DimensionRender, Facts: 0, Subjects: c.Subjects,
				Render: &domain.RenderCheck{Outcome: domain.RenderUnavailable, Key: keys, Reason: reason}},
		})
	}
}
