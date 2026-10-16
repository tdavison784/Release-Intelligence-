# Lane `reviewfilter` — review UI shows only what needs review (product-owner request) — Sonnet

Branch `p4ei/reviewfilter`, worktree `.claude/worktrees/p4ei-reviewfilter`. Read FLEET.md, then
`docs/REVIEW_UI.md`, `docs/KNOWLEDGE.md`, `internal/reviewui/` and `internal/knowledge/` (the Phase 3 review
app, `ri review serve`). You own `internal/reviewui/` and `docs/REVIEW_UI.md` for this.

## The request (product owner, 2026-10-16, verbatim intent)

The product owner will now verify knowledge facts in the review UI, and wants it to **only show the data that
needs review — not all the other staged data**.

## What that means

1. **Default view = "Needs review"**: only items that require a human decision now — open review items whose
   subject is not yet verified at `human` or `deterministic` level and that have no human decision yet.
   Everything else is hidden from the default inbox and its counts: already decided (any decision), proxy-
   staged / proxy-decided material, consensus-only staging that has no open review item, retracted /
   superseded facts, demo data, and model proposals that are not attached to an open review item.
2. **Priority order inside "Needs review"**: first the items that currently **block investigations** — facts
   or review items attached to change items of the Phase 4 milestone products (cert-manager, ingress-nginx,
   kube-prometheus-stack / prometheus-operator) whose manifestation is controller-behaviour or unknown, i.e.
   the ones OverrideInspection could use as pin sources once human-verified (see DESIGN §3 R2.4 and
   `internal/investigate/validate` pin rules). Show *why* each is there in one plain sentence ("Verifying
   this lets investigations clear environments that pin the old default"). Then the rest of the open items.
   The ordering rule is generic (blocking-first by a computed signal), not a hard-coded list.
3. Hidden data stays reachable through one explicit, clearly labelled filter ("Show all staged data"), never
   mixed into the default view. Counts in the header reflect the default view.
4. No change to what a decision records (reviewer + reviewerKind human, labels, reasons — Phase 3 contract);
   proxy decisions never appear as human.
5. A tiny CLI helper or doc line telling the product owner exactly how to start it and how many items are
   waiting (`ri review serve …` and the URL).

## Done

Tests for the filter (each hidden category, priority order, the "show all" toggle, counts), browser check
as in `docs/REVIEW_UI.md` "Browser verification" (screenshot of the default view committed under
`docs/phase4/env-impact/review-ui/`), docs, build/vet/test green, cleanup (stop any server you started),
`LANE DONE:` with the exact start command and the number of items in the default view.
