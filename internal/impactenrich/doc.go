// Package impactenrich adds the optional, provenance-preserving AI
// enrichment step to `ri impact` (`ri impact … -enrich`). It sits between the
// deterministic impact join (internal/impact, pure, no model) and the human,
// and exists for exactly one block of the funnel: the unknown findings whose
// upstream change is note-derived — the ones the deterministic join cannot
// reason about (docs/ACTION_CLASSIFICATION.md: silence would read as
// "not affected", so they are recorded as unknown with the missing evidence).
//
// The step is bounded and deterministic first:
//
//  1. Candidates (no model calls): unknown findings whose change is
//     note-derived and not routine-maintenance (applicability questions);
//     known duplicate groups of findings (cluster questions); affected
//     findings eligible for migration synthesis (migration questions).
//     Candidates decide what is ASKED; they are not conclusions.
//  2. One bounded prompt per candidate: the finding, its upstream change,
//     evidence excerpts and — for applicability questions — a bounded summary
//     of the environment (dimensions + health, values key paths, GVK usage,
//     installed CRDs and products). No file dumps; values are never sent,
//     only key paths.
//  3. A validator that enforces the contract's forbidden transitions:
//     the model may never produce or suggest action-required, never delete or
//     downgrade a deterministic finding, never cite evidence it was not shown,
//     and never answer without complete AI provenance.
//  4. domain.Enrichments attached to the ImpactReport (never inside the
//     findings), plus at most one suggestion per unknown finding:
//     ImpactFinding.SuggestedClassification = "review-required" — the
//     deterministic classification is never overwritten.
//
// The three applicability answers and their (bounded) effect:
//
//	plausibly-applies → the unknown finding gains an AI review-required
//	                    suggestion (never higher) with the why;
//	not-applicable    → the finding stays unknown; the model's reasoning is
//	                    recorded as a note (the human decides not-affected);
//	undetermined      → the finding stays unknown; the reason is recorded.
//
// Without a model client, Run produces candidates only. The deterministic
// report is byte-identical with and without -enrich; enrichments are rendered
// in their own section and the run metadata records every prompt and
// rejection.
package impactenrich
