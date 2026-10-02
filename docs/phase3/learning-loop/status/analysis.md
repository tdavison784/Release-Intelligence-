# Lane `analysis` — status

**State: DONE.** Deliverable: `docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md` (complete: all four sections).

## Done
- Rebuilt `bin/ri` and reran `ri eval -o json` offline against the primary checkout's warm cache. The
  baseline reproduces exactly: applicabilityAccuracy 0.095 (2/21), 47 confusion cells, unknownRate 0.731.
- Ran `ri impact -o json` and `ri upgrade -o json` per environment case, with exactly the inputs that
  `internal/eval/run.go` assembles. Rebuilt all 47 cells per (change × item), and they match the
  evaluator cell for cell.
- Per-link analysis of all 21 links: cause class, missing semantic structure, deciding env evidence,
  validation possibility, honest reachable class, tier.
- Compact tables for the 28 ACTION→UNKNOWN cells and the 5 ACTION→NOT-AFFECTED cells.
- Aggregates: cause classes, subject families, env extensions X1–X6, validators V1–V6, ceilings.
- 14 dataset/evaluator observations (D1–D14), plus design inputs for `contract` (§3.5).

## Headline findings (for the commander)
- Ceilings: (a) deterministic 0.24 (0.38 with new artifact capture) · (b) + verified facts **0.76** ·
  (c) + cross-product inventory **0.90**. **0.80 needs a new environment input**: (b) tops out at 16/21.
- In cilium-1.15 E6 and cilium-1.16 E3 the engine *already* emits ACTION REQUIRED (`impact:values-removed`
  on `bgp.*`), but the item matchers can't see that change (measurement artefact, D1).
- Two links are not honestly reachable (cilium-1.15 E1: precondition met; strimzi E6: no OPA in the
  fixture). Their honest class is NOT AFFECTED.
- None of the 5 ACTION→NOT-AFFECTED cells is the engine clearing an item's own subject: 3 are matcher
  artefacts, and 2 are a missing `replacedBy` relationship (tls.secretsBackend).
- Both current hits are class-wrong (INFORMATIONAL "does not apply to you").
- MISSION demo vs dataset: rotationPolicy is labelled `review`. An ACTION finding there would count as a
  false action (D11, §3.5-4).

## Decisions taken
- Kept UNKNOWN cause classes to the six Goal-18 values. "Measurement artefact" and "label not supported"
  are reported as eval/dataset issues, never as engine UNKNOWN reasons.
- Tier (a) counts only artifacts already ingested. New artifact capture is reported separately as (a\*).

## Files touched outside ownership
None.

## Open questions (commander / groundtruth)
- D1: whether to add `subject: bgp.enabled` matchers. This is integrity-sensitive, because the motive
  comes from seeing pipeline output.
- D2/D3: whether to relabel cilium-1.15 E1 and strimzi E6, and on what upstream grounds.
- D11: should the demo's step 10 be REVIEW REQUIRED, or should cert-manager-1.17 E1 be relabelled from
  upstream?

## Test status
Docs-only lane; no Go code changed. `go build ./...` was used to build the binary; no `go vet`/`go test`
impact.
