package impact

// CONTRACT-CHANGE(render): product-owner decision PO-3 — render-backed
// classification of changed chart defaults and new keys the customer leaves
// UNSET. Without an evaluator (no `--render`) the values join is unchanged
// (byte-identical reports); the render lane implements the evaluator
// (internal/render.UnsetValues).

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// RuleValuesDefaultRendered: the customer leaves a key unset whose default
// changed (or which is new) in the target, and rendering the target with
// their configuration attributes a concrete rendered change to that key — the
// new default reaches their deployment. Review-required by default: the
// render proves the structural change, not its consequence (R11).
const RuleValuesDefaultRendered = "impact:values-default-rendered"

// UnsetValuesEvaluator decides, for a values default-changed / added change
// whose keys the customer does not set, whether the new default changes the
// customer's rendered deployment:
//
//   - True: a rendered change is attributable to the key (counterfactual:
//     the target rendered with the key pinned to its old default, or absent
//     for a new key, differs from the target as the customer gets it, and the
//     upgrade delta contains that difference). Matches cite environment-scope
//     rendered evidence (chain 2), Records hold those records.
//   - False: every render succeeded with complete values and nothing is
//     attributable; Checks carry the render evaluation record.
//   - Unknown: rendering was unavailable (failed, incomplete values, no Helm
//     deployment of the product); Needed says why. The join keeps today's
//     not-affected verdict and records "render unavailable" in its checks.
type UnsetValuesEvaluator interface {
	EvaluateUnsetValues(c domain.Change, kind string) ConditionResult
}

// unsetValues applies an UnsetValuesEvaluator to a default-changed / added
// change whose keys the customer leaves unset. It returns false when the
// caller should emit today's values-unset verdict (with extra checks).
func (b *builder) unsetValues(ev UnsetValuesEvaluator, c domain.Change, kind string) (handled bool, extraChecks []domain.ImpactCheck, extraDetail string) {
	r := ev.EvaluateUnsetValues(c, kind)
	switch r.Value {
	case True:
		for _, rec := range r.Records {
			if _, ok := b.extraLocal[rec.ID]; !ok {
				b.extraLocal[rec.ID] = rec
				b.extraLocalOrder = append(b.extraLocalOrder, rec.ID)
			}
		}
		what := "default changed"
		if kind == "added" {
			what = "new key"
		}
		title := fmt.Sprintf("You leave %s unset; its %s changes your rendered deployment", codeList(c.Subjects, 3), what)
		var subs []string
		for _, m := range r.Matches {
			subs = append(subs, m.Subject)
		}
		detail := fmt.Sprintf("Your values do not set %s, so %s's new default applies to you. Rendering the target with your configuration, once as you will get it and once with the key pinned to its previous state, attributes these changes of your deployment to it:\n%s\nReview them before upgrading, or pin the key to keep the previous behaviour.",
			strings.Join(c.Subjects, ", "), b.edge.To.String(), "  - "+strings.Join(subs, "\n  - "))
		before := len(b.findings)
		b.add(RuleValuesDefaultRendered, domain.ImpactReviewRequired, domain.SeverityMedium, domain.ConfidenceHigh, title, detail, c, r.Matches, c.Evidence...)
		return len(b.findings) > before, nil, ""
	case False:
		return false, r.Checks, " Rendering your configuration confirms it: nothing in your rendered deployment is attributable to the new default."
	default:
		needed := "render unavailable"
		if len(r.Needed) > 0 {
			needed = "render unavailable: " + strings.Join(r.Needed, "; ")
		}
		return false, []domain.ImpactCheck{{Dimension: domain.DimensionRender, Facts: 0, Subjects: []string{needed}}},
			" The render check could not run (" + needed + "), so this rests on the values comparison alone."
	}
}
