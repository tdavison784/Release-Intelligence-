# Phase 2 outcomes

> The question this phase had to answer:
> **Can Release Intelligence become a generalised knowledge layer for third-party
> operational software, where adding new products is scalable and the release
> graph can be joined with a customer's real environment to say exactly what an
> upgrade means for them?**
>
> **Answer: yes, on the evidence of 28 onboarded products — with the remaining
> risk concentrated in upgrade-brief precision and environment-link coverage,
> not in the abstraction's ability to represent products.**

All numbers below are computed, not asserted: `ri stats` regenerates
[ONBOARDING.md](../ONBOARDING.md) from the definitions, records and saved
check reports; `ri eval` reproduces the quality numbers from the committed
dataset and regression-gates them.

## G1 — Catalog scalability: 28 products, five waves

| Wave | Products | New constructs / product | Reuse | Zero-new products | Median minutes |
|---|---|---|---|---|---|
| 0 (Phase 1) | 3 | 37.7 | 57% | 0/3 | n/a |
| 1 | 7 | 4.4 | 93% | 1/7 | 25 |
| 2 | 7 | 1.1 | 99% | 4/7 | 35 |
| 3 | 7 | **0.1** | **100%** | 6/7 | 24 |
| 4 (edge cases) | 4 | 2.0 | 97% | 1/4 | 28.5 |

- **Product-specific Go code: 0 across all 28 products.** This invariant never
  broke, in any wave, including the deliberately hostile edge-case wave.
- The wave-4 uptick (2.0/product) is the point of that wave: it sought
  unrepresentable shapes. What it produced were generic capability constructs —
  `format:html` (Go's HTML-only release notes; also Tomcat/Kafka/Maven-shaped),
  the first declarations of `compareWith` (Loki), `lineage:linear` and
  `template-func:replace` (MinIO) — not product logic. The extraction-format
  family itself (docbook → rst → adoc → html) is four fills of one slot created
  at wave 1.
- Later products really are `ProductDefinition + validation`: 12 of 28 products
  introduced nothing; Kyverno (17 min) and ARC (15 min) decomposed entirely into
  constructs from orders 1–15.
- Honest corrections the measurement forced: PostgreSQL's `format:adoc`
  justification named OpenSearch falsely (the fork has no AsciiDoc — measured
  zero adoc uses at order 25; record corrected); Go's component-group example
  was wrong in detail (`go1.22` never existed; one pattern still covers both
  tag eras).

## G2 — Measured onboarding

`ri stats` + per-product records ([PROTOCOL.md](../onboarding/PROTOCOL.md))
produce [ONBOARDING.md](../ONBOARDING.md) — per-product and per-wave: size,
constructs (used/new/reused, by reflection), Go changes, minutes, discovery
origin of every element, relationship-validation outcomes, unreachable hosts,
representability. Data-quality cross-checks reconcile records against
definitions; the only standing notes are the documented "migrated after they
appeared" adoptions.

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the G2 per-product columns "unverifiable", "unreachable sources" and
> the relationship-validation counts were measured in a network-restricted
> sandbox. Re-measured with network access: unverifiable checks fell to 0 for
> cert-manager (12), ingress-nginx (47) and postgresql (15); only falco (digest
> mismatch) and minio (anonymous pulls denied) keep unverifiable checks;
> karpenter's HTTP 429s were throttling, cleared by the sequential re-run after
> the throttling fixes. The saved baselines for 14 products (argo-cd with
> v3.4.1, karpenter, ingress-nginx included) were replaced by the live runs.
> G9 (`ri eval`) is unaffected: live, warm-cache
> offline and stored results are identical. The zero-product-specific-Go-code
> and construct-reuse numbers do not depend on reachability.

## G3 — Discovery

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** seven proposals (crossplane, external-secrets, falco, flux,
> ingress-nginx, linkerd, prometheus-operator) carried "Unverified by discovery
> (... HTTP 403 ...)" statuses that were sandbox rate limits; run live with the
> same binary they become `historically-validated`. The headline wrong-proposal
> and share-found numbers below are computed against final definitions and do
> not change.

`ri discover` proposes a definition with a five-status honesty vocabulary
(historically-validated / discovered / inferred / unverified / exception;
AI-sourced elements are capped at `inferred`). Measured against final
definitions ([IMPROVEMENTS.md](../onboarding/discovery/IMPROVEMENTS.md)):
wrong-proposal rate 9/25 → 4/28. Share of final definition elements found
automatically, by wave: 0% → 44% → 51% → 40% → 29%. **Not yet "most", and the
plateau is real** — recorded failure modes: release-lineage traps (ARC's dead
`v0.x` family vs the live one; Loki's inverse), chart monorepos, item-yield
blindness (a channel that validates but yields zero note items), and CalVer
invisibility (MinIO: "523 tags: 0 stable, 523 junk").

## G4 — Drift

`ri drift` re-validates a definition's relationships against releases newer
than its saved baseline and emits evidence-backed events from a closed
vocabulary, with proposals — never mutating `products/`. Unverifiable
(unreachable host) is never drift; already-failing subjects are notes.
Validated live: a deliberately stale definition against real upstream produced
a correct `availability-violated` event with registry evidence. The pipeline's
honesty here is corroborated by real anomalies onboarding already caught:
Falco's `9.0.0.tgz` re-published after its index entry (digest mismatch) and
MinIO's deleted `RELEASE.2025-09-06` GitHub release.

## G5 — Enrichment (pre-existing, boundaries held)

`ri upgrade -enrich` adds `method: ai` Enrichments with model, prompt digest,
input evidence and timestamp; deterministic facts are never replaced. No LLM
key existed in this environment, so nothing in this phase's onboarding or
validation depended on AI. The eval's 10 duplicate groups (cross-role
redundancy, e.g. cert-manager's upgrade guide quoting its release notes
verbatim) are exactly the consolidation work enrichment is for.

## G6/G7 — Environment-aware impact, dual provenance

`ri impact <product> <from> <to> --values/--manifests/--crds/--images/--kubernetes`
funnels all upstream changes → environment-relevant → action-required / review
/ informational, every finding citing **both** chains (upstream evidence IDs +
environment file:line/digest), enforced by `ImpactReport.Validate()`.
Example (offline fixture): 52 upstream changes → 5 relevant → 1 action
required; live: Istio 139 → 1 → 1, linking the "Kubernetes support narrowed"
change to the support-matrix rows.

## G8 — Published artifacts

Helm `.tgz` from indexes (digest-verified against the index), OCI chart layers
(digest-addressed), OCI image manifests/configs, release assets — every fact
carrying a `representation` from a closed vocabulary, derived from the locator,
never guessed. `compareWith` diffs two representations and records divergence
as a fact: Loki found that re-published pre-fork chart tags do **not**
reconstruct the published packages (+360/−143/~47 keys) while the current one
agrees exactly.

## G9 — Validation dataset

9 real historical upgrades across 8 products, expectations authored blind from
cited upstream sources; `ri eval` scores and regression-gates against committed
snapshots. Two eval-driven definition fixes already landed (cert-manager's 1.16
Breaking-changes section; the pinned-operator-notes source), each verified by
the loop: **recall 0.94 → 0.96 → 1.00** (53/53, zero misses), precision
0.48 → 0.49.

## The honest frontier (what is NOT yet solved)

1. **Precision 0.49.** Half of a raw upgrade brief is true-but-irrelevant.
   Precision tracks whether a product has a dedicated upgrade-guide channel
   (1.00) vs mixed channels (0.17–0.64). **Measured 2026-10-01** (first live
   enrichment run, GLM-5.3-Flash via Z.AI): as designed, enrichment does NOT
   move brief precision — 7% FP consolidation on the three noisiest cases
   ([eval/REPORT.md](../eval/REPORT.md)). The candidate groups consolidate
   duplicate statements, not routine-maintenance masses; the fix is a
   routine-classification capability (deterministic or candidate-side), not
   a better model.
2. **Environment-link accuracy 0.25.** The join is exact on what files show
   (findings 7/7, false findings 0) but blind to defaults you don't set.
3. **Discovery share plateau ~40–50%** with named failure modes (above).
4. **CalVer is partially representable** (MinIO: day-granularity works,
   same-day pairs collapse; no second product justifies `scheme: calver`).
5. **Cross-product edges are not joined into one view** — the KPS↔operator
   lookup works deterministically from both sides; a store/query layer is the
   missing piece.
6. Scattered per-product gaps (ES 8.x per-release notes channel, registry
   delistings, HTML-only support matrices) are inventoried in the records.

## What this buys the next phase

The plan's closing claim held: with the intelligence model validated at this
breadth, the next phase is designing the platform, API, data architecture,
customer integrations and commercial product **around** a working engine —
not more prototype development on the core model.
