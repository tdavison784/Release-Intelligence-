# Proposal: an "unknown honesty" metric for undecided links

> Status: **proposal only, for the product owner's decision.** Author: `groundtruth` lane, 2026-10-02.
> Not pre-registered and not implemented. The author has already seen eval results for this dataset
> (status/groundtruth.md). That is why this document proposes a reported metric and leaves the
> decision to someone who has not shaped the labels. It does not quote any number from those results.

## What exists today

`environment.undecidedImpact` (eval/FORMAT.md) records links whose honest answer is UNKNOWN: the
fixture lacks or withholds the input that decides them. Each link names a domain `UnknownReason` and the
`needed` input. The dataset has **10 such links in 9 environments** (6 environment-visibility-gap,
3 runtime-behavior-gap, 1 cross-product-context-gap). Today they are loaded and validated but not
scored, and they never enter `applicabilityAccuracy`.

## Proposed metric

**`unknownHonesty`** = undecided links answered honestly / undecided links, over environment entries
(base and transfer cases alike).

An undecided link is **answered honestly** iff, for every change the item's matchers select, the
engine emits **no AFFECTED finding** (action-required / review-required / informational) and **no
NOT-AFFECTED finding**. UNKNOWN findings, or no finding at all, count as honest.

Optional secondary (also reported, never gated): **`unknownReasonAgreement`** = honest links whose
UNKNOWN findings carry the labelled `reason` / UNKNOWN-findings links. This needs the applicability
lane's `unknownReason`.

## Properties and choices

- **Reported, not gated.** It appears in the aggregate and the text report next to
  `applicabilityAccuracy`, never in `eval/gates.yaml`. A threshold, if ever wanted, would be
  pre-registered separately by someone who has not seen results.
- **Separate from applicabilityAccuracy.** Folding undecided links into that ratio would change a
  gated metric's definition after results were seen, and would mix "decided correctly" with "refused
  correctly". A separate number keeps both readable.
- **Both directions count as a failure.** A NOT-AFFECTED "checked and clear" on an undecided link is
  as dishonest as an AFFECTED claim: it asserts knowledge the inputs cannot support
  (docs/ACTION_CLASSIFICATION.md §2, "UNKNOWN must never silently become NOT_AFFECTED").
- **Vacuity is visible.** An engine that emits nothing at all scores 1.0. So the metric must always be
  shown with `applicabilityAccuracy` and the affected-link hit rate, and the report should say so (the
  same "vacuous pass" wording the gate panel uses).
- **Same matching as today.** It reuses the item matchers and `expIDForChange` attribution. No new
  matcher semantics, and D13 (last-write-wins attribution) applies unchanged.
- **Transfer subset.** It is computed on the transfer subset too (DESIGN.md §7), so "knowledge
  reviewed in environment A makes the engine overclaim in environment B" becomes visible.

## Implementation sketch (for whoever is assigned, after a decision)

`internal/eval/compare.go` `scoreReport`: for each `UndecidedImpact` link, collect the findings
joined to the item's matched changes and count the link honest iff none is AFFECTED or NOT_AFFECTED.
Add `EnvMetrics.UndecidedLinks` / `UndecidedHonest`, pool them in `AggregateResults`, and render them
in report.go. Add a unit test with a fake report covering honest (UNKNOWN), dishonest-affected and
dishonest-clear. This is roughly 40 lines plus a test, with no change to any gated metric.

## Decision requested

1. Adopt `unknownHonesty` as a reported, ungated metric: yes / no.
2. Adopt `unknownReasonAgreement`: yes / no / later (after `unknownReason` is mandatory).
3. Who implements it (the evaluator owner or the applicability lane), and when.
