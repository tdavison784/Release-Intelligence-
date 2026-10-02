# Groundtruth sub-brief — author ONE transfer environment (MISSION G12)

A verified release-level fact must carry over to other clusters without re-review. You author a
**second, independently designed cluster** for an existing case's release transition, as a transfer
case: `eval/cases/<base-id>--<suffix>/` whose `case.yaml` declares `transferOf: <base-id>` and only an
environment. Read `eval/FORMAT.md` ("Semantic labels", "Transfer environments", "Expected
classifications") first.

## Integrity rules (non-negotiable)

- Same as `docs/phase3/learning-loop/briefs/groundtruth-newcase.md` "Integrity rules": blind, upstream
  sources only (quote them), never run `ri`, never read `eval/results/*`, `eval/REPORT.md`,
  `eval/adjudications/*`, `internal/*/testdata/*`, `.ri/`, or UNKNOWN-ANALYSIS §1–§2. Do not commit.
- Write only inside your new directory. **Do not edit the base case** (its items, `semantics` and
  environment are authored and committed). If you believe a base `semantics` label is wrong, say so in
  your report and NOTES.md — do not change it.
- Design the cluster from the upgrade notes, not from the base environment: it must be a different,
  realistic configuration, so links come out differently than in the base case.

## What to produce

`eval/cases/<base-id>--<suffix>/` (suffix: a short cluster name, e.g. `edge-cluster`):

- `case.yaml`:
  ```yaml
  id: <base-id>--<suffix>
  transferOf: <base-id>
  researchedAt: "2026-10-01"
  sources: [ … every upstream URL you used to ground the fixture (chart values at the tags, CRDs, docs) … ]
  environment:
    description: >-   # every fact the links rely on
    kubernetes: "1.xx"
    expectedImpact: [ … ]     # links with relevance, why, exposure, optional overlap, environmentEvidence
    undecidedImpact: [ … ]    # links whose honest answer is UNKNOWN: reason + needed (+ exposure)
  ```
  Do NOT restate product/from/to/expected/notExpected (inherited). `expectedFindings` /
  `notExpectedFindings` are optional; add only ones grounded in upstream artifacts.
- `environment/`: values.yaml and/or manifests/*.yaml, optional images.txt / crds/, and
  `inventory.yaml` (list of `{product, version, note?}` — only facts the description states, each with a
  comment quoting the sentence; catalog ids from `products/` where they exist; no `complete:` key).
- `NOTES.md`: how the cluster was designed and grounded, and every link's judgement.

## Link requirements

- At least 5 links over the base case's items, mixing: **≥2 affected** (action-required / review /
  informational), **≥2 not-affected** (the cluster touches the area but is clearly clear: sets the
  replacement key, runs a fixed version, uses the escape hatch, does not use the removed kind), and
  **≥1 undecided** (`undecidedImpact`, with a domain UNKNOWN reason: environment-visibility-gap,
  cross-product-context-gap, runtime-behavior-gap, evidence-gap, semantic-ambiguity, and `needed`).
- Each decided link's `exposure` must be consistent with the base item's `semantics` (same subject;
  the canonical shapes of DESIGN.md §1.3 where they apply), and its relevance must follow from the
  consequence's `exposedClass` when the exposure is true (overlap true → informational; exposure
  false → not-affected), unless you explain a deliberate difference in NOTES.md.
- `environmentEvidence` must point at the exact fixture lines (`<path>#Lx-Ly`); the loader checks the
  files and line ranges exist.
- Transfer cases do not score item classes, so there is nothing to set on the items.

## Validate

`go test ./internal/eval -run 'TestDatasetLoads|TestDatasetEntrySelectionLoads' -count=1` and
`go test ./internal/eval -count=1` must pass.

## Report back

≤20 lines: transfer id, cluster story, each link (item → relevance / undecided reason), anything in
the base labels you think is wrong.
