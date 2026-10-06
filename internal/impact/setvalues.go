package impact

// CONTRACT-CHANGE(renderfirst): product-owner decision PO-7a (briefs/
// renderfirst.md, docs/RENDER-FIRST.md once written) — for a values key the
// customer SETS whose key the target release removes, the customer's render
// decides what today's blanket action-required verdict could only speculate
// about ("stop taking effect (or are rejected…)"): the target either refuses
// the configuration outright (render-target-rejects, the refusal names the
// key), or the key's contribution to the customer's rendered upgrade is
// attributed by the counterfactual render (values-removed with rendered
// evidence on chain 2), or nothing is attributable (values-set-no-effect).
// Without --render — or when no render decides — the verdict is exactly
// today's, so the report is byte-identical without --render.

import (
	"fmt"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

// PO-7a join rules (aliases of the domain constants, like every Rule*).
const (
	RuleRenderTargetRejects = domain.RuleRenderTargetRejects
	RuleValuesSetNoEffect   = domain.RuleValuesSetNoEffect
)

// SetValuesOutcome is what the customer's render said about values keys they
// set whose key the target release removes.
type SetValuesOutcome string

const (
	// SetValuesRejected: the target chart refuses the customer's values at
	// render time and the refusal names one of the removed keys (or its
	// ancestor) — the upgrade as configured fails before rendering anything.
	SetValuesRejected SetValuesOutcome = "rejected"
	// SetValuesAttributable: the renders succeeded and the key contributes
	// rendered changes to the customer's upgrade delta.
	SetValuesAttributable SetValuesOutcome = "attributable"
	// SetValuesNoEffect: every Helm deployment of the product rendered with
	// complete values and nothing of the customer's upgrade is attributable
	// to the key.
	SetValuesNoEffect SetValuesOutcome = "no-effect"
	// SetValuesUndecided: no render decided (none, unavailable, or the
	// refusal named no key of this change).
	SetValuesUndecided SetValuesOutcome = "undecided"
)

// SetValuesResult is the outcome of one removed-values-key change the
// customer sets, with the evidence of the deciding render.
type SetValuesResult struct {
	Outcome SetValuesOutcome
	// Rejection is the rendered-change match citing the environment-render
	// record of the target's refusal (Outcome rejected).
	Rejection *domain.ImpactMatch
	// Refusal is the renderer's refusal text (Outcome rejected), quoted by
	// the finding.
	Refusal string
	// Matches are the attributable rendered changes of the customer's
	// upgrade delta (Outcome attributable), each with environment evidence.
	Matches []domain.ImpactMatch
	// Checks is the no-attributable-change render check (Outcome no-effect).
	Checks []domain.ImpactCheck
	// Records are evidence records the evaluation created (the refusal
	// record; the counterfactual state records).
	Records []domain.Evidence
	// Needed says why no render decided (Outcome undecided).
	Needed []string
}

// SetValuesEvaluator decides a values key the customer sets whose key the
// target release removes (PO-7a): does the target refuse their configuration,
// does the key contribute rendered changes to their upgrade, or neither. The
// render lane implements it (internal/render.SetValues over the environment's
// pairs, `ri impact --render`); without one the verdict is today's
// values-removed.
type SetValuesEvaluator interface {
	EvaluateSetValues(c domain.Change) SetValuesResult
}

// setValues emits the PO-7a verdict of a values-removed change whose keys the
// customer sets. Without an evaluator (no --render), or when no render
// decided, this is exactly today's action-required values-removed verdict; a
// decisive render replaces it.
func (b *builder) setValues(ev SetValuesEvaluator, c domain.Change, matches []domain.ImpactMatch, exact, partial []string) {
	var r SetValuesResult
	if ev != nil {
		r = ev.EvaluateSetValues(c)
	}
	for _, rec := range r.Records {
		if _, ok := b.extraLocal[rec.ID]; !ok {
			b.extraLocal[rec.ID] = rec
			b.extraLocalOrder = append(b.extraLocalOrder, rec.ID)
		}
	}
	switch r.Outcome {
	case SetValuesRejected:
		toTag := b.edge.To.String()
		keys := strings.Join(append(append([]string{}, exact...), partial...), ", ")
		title := fmt.Sprintf("%s refuses your values at render time: you set %s, which it removed", toTag, pluralKeys(exact, partial))
		detail := fmt.Sprintf("%s no longer has these values keys, and rendering it with your configuration fails because the chart rejects the values: %s. The source release rendered the same configuration, so the upgrade as configured does not complete — remove the key(s) from your values and migrate the configuration they controlled.\nYou set: %s.",
			toTag, r.Refusal, keys)
		all := append(append([]domain.ImpactMatch{}, matches...), *r.Rejection)
		b.add(RuleRenderTargetRejects, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh, title, detail, c, all, c.Evidence...)
	case SetValuesNoEffect:
		toTag := b.edge.To.String()
		keys := strings.Join(append(append([]string{}, exact...), partial...), ", ")
		title := fmt.Sprintf("You set %s, which %s removed; rendering shows it changes nothing for you", pluralKeys(exact, partial), toTag)
		detail := fmt.Sprintf("Your values set %s, which %s removes. Rendering the source release with your configuration, once as you have it and once with the key unset, shows no difference attributable to it, and nothing in your upgrade's rendered delta traces to it: with complete values, the key never reached your rendered deployment. No action is needed for this key.",
			keys, toTag)
		b.verdict(RuleValuesSetNoEffect, domain.ImpactNotAffected, c.ID, title, detail, c, c.Evidence,
			append([]domain.ImpactCheck{b.valuesCheck(c.Subjects)}, r.Checks...))
	default: // attributable, undecided or no evaluator: today's verdict
		all := matches
		if len(r.Matches) > 0 { // the render attributed the key's contribution
			all = append(append([]domain.ImpactMatch{}, matches...), r.Matches...)
		}
		b.removedValues(c, all, exact, partial)
	}
}

// removedValues is the deterministic values-removed verdict (unchanged
// wording): keys the customer sets stop taking effect. With an attributable
// render, matches additionally cite the rendered changes on chain 2.
func (b *builder) removedValues(c domain.Change, matches []domain.ImpactMatch, exact, partial []string) {
	toTag := b.edge.To.String()
	title := fmt.Sprintf("You set %s that %s removed", pluralKeys(exact, partial), toTag)
	if len(exact)+len(partial) == 1 {
		title = fmt.Sprintf("You set %s, which %s removed", code(firstOf(exact, partial)), toTag)
	}
	detail := fmt.Sprintf("%s no longer has these values keys; keys you set here stop taking effect (or are rejected when the chart validates values against a schema). Remove them from your values and migrate the configuration they controlled.\nYou set: %s.",
		toTag, strings.Join(append(append([]string{}, exact...), partial...), ", "))
	b.add(RuleValuesRemoved, domain.ImpactActionRequired, domain.SeverityHigh, domain.ConfidenceHigh, title, detail, c, matches, c.Evidence...)
}
