# Lane `analysis` — status

**State: DONE (analysis brief); trustfix follow-on worked by handoff (see GLM handoff log).**
Deliverable: `docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md` (complete: all four sections).

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

## Trustfix follow-on (branch `p3ll/trustfix`)

The commander re-tasked this worktree past the analysis brief with impact-layer verdict-trust fixes
(branch `p3ll/trustfix` off `p3-learning-loop` @ 823b44c). Commit c804c91 (Claude agent) fixed the
crd-field-removed why-block to name only exposed resources. An uncommitted WIP was left mid-flight; the
GLM handoff finished it (details below).

## GLM handoff log

**Who/when:** glm-analysis (GLM-5.3), 2026-10-02, holding `.lane-lead`.

**What I did:**

1. Finished the uncommitted WIP as commit `3b42c1e` — *decide removed CRD fields below a set list
   element by element*. Design and tests are the Claude agent's; I verified the semantics against
   `internal/env` (field facts descend sequences in schema-path syntax; sensitive withholding keeps
   paths; any warning flips the manifests dimension to partial) and added one fix of my own: a CRD
   definition document stages the GVK usage entry of every version it serves (`internal/env/env.go`
   loadCRD), so the coverage check — "every usage document must have full-depth resource facts" —
   would always refuse on real environments (CRDs + manifests supplied), leaving the WIP dead in
   production while green in its manifests-only tests. `crdDefinitionDocs` exempts exactly those
   documents (a schema is not a resource of the kind it defines). Extended the regression test with
   the CRD-supplied shape and a manifests-only subtest.
2. Validated honestly: A/B eval (warm cache, primary `-state`) of c804c91 vs 3b42c1e —
   external-secrets-0.15-0.16 classificationMatched 3 → 4; applicabilityAccuracy 0.4571 → 0.4667
   (49/105); every other case byte-identical; falseActionRate unchanged 0.133. `go build ./...`,
   `go vet ./...`, `go test ./...` all green.

**Diagnosed, not fixed (other lanes / commander — the two false actions behind the failing
falseActionRate gate, both pre-existing on c804c91):**

- kyverno-1.12-1.13: the one ACTION is `impact:values-removed` on the removed cleanupJobs/chunkSize
  values (E9, expected review-required). Generic values-removed honestly says action; softening it for
  these values needs release-level knowledge (knowledge lane) or a relabel (groundtruth). Product-
  specific Go logic is forbidden by FLEET.
- crossplane-1.20-2.0: both ACTIONs (crd-enum-value-removed on spec.mode, crd-field-removed on
  spec.resources) are honest E1 verdicts (E1 expects action); the "false" accounting comes via
  dataset-FP changes / matcher joining — the D1-family measurement artefact. Editing the comparator to
  move a gate number is integrity-sensitive; left alone.

**Uncertainties / assumptions:**

- No written trustfix brief exists; I inferred the lane's mandate from the branch, c804c91, and the
  WIP. The analysis brief itself declared this lane read-only on code — the trustfix re-tasking
  supersedes that (the Claude agent's c804c91 already committed code here). Flagging in case the
  commander intended otherwise.
- Known bounds of the deep decision (documented in code comments): refuses on partial manifests
  dimension, on uncovered usage documents, on oversize-withheld subtrees, and (by construction) when a
  GVK exceeds 25 documents (a warning flips health to partial — conservative, maybe over-conservative
  for large fleets). Descent caps at depth 64 silently; a removed path nested deeper than 64 levels in
  a manifest would not be seen (pathological; same cap as the flattened paths).
- Dataset swept per-case (`ri impact` with exactly the evaluator's inputs, all 28 cases incl. the 4
  transfer environments): no further crd-fields-removed review/action verdicts remain — the family is
  decided everywhere the dataset exercises it.

**Files touched this handoff:** `internal/impact/crd_gvk.go`, `internal/impact/crd_gvk_test.go` (both
already touched by the trustfix re-tasking), this status file. No files outside; `.lane-lead` unread
beyond confirming my hold.
