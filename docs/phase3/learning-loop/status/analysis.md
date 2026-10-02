# Lane `analysis` — status

**State: DONE (analysis brief).** The trustfix follow-on is complete; see `status/trustfix.md` (it reviews and corrects the GLM handoff log below).
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

## Loop-diagnosis re-tasking (branch `p3ll/loop-diagnosis`)

The commander re-tasked this worktree again (branch `p3ll/loop-diagnosis` off `p3-learning-loop` @
9d5126a): for every one of the 48 unhit affected links, trace the loop stage where it is lost, and
explain why 170 knowledge findings yield only +2 links. Deliverable:
`docs/phase3/learning-loop/LOOP-DIAGNOSIS.md`. A first GLM handoff log entry above covers the
trustfix re-tasking; this one covers loop-diagnosis.

### GLM handoff log (2) — loop-diagnosis

**Who/when:** glm-analysis (GLM-5.3), 2026-10-02, holding `.lane-lead`.

**What I did:**

1. Continued from the lane's paused Claude agent (its dump harness, scratch scripts and before-state
   eval were intact; I re-verified every number I reused against fresh runs). Completed the per-link
   stage classification of all 48 unhit affected links — 0:6 recall misses, a:20 no knowledge store
   (loki/traefik/prometheus-operator/crossplane/kyverno never run through the loop), b:4
   evidence-sufficiency-only, c:10 pending review (8 high/human, 2 normal), d:6 decided without
   accept, e:2 fact-but-no-finding, f:0 — and the landing analysis of all 170 knowledge findings
   (136 UNKNOWN; 34 affected, of which 32 join unmeasured changes → only +2 links).
2. Found and fixed one generic engine bug, commit `d258ab4`: unscoped cli-flag / env-var /
   feature-gate conditions decided **false from nothing** in environments that supply no workload
   arguments (or no values key at the gate's path). Absence from an unexamined environment is now
   unknown · environment-visibility-gap (`internal/impact/condition.go` +
   `TestEvaluateConditionArglessWorkloads`). This also closes a latent trap: once facts gain human
   verification, the old behavior would have *cleared* (NOT AFFECTED) customers who set the gate in
   Helm values. Component-scoped absence over healthy manifests still decides false (existing
   semantics preserved).
3. Verified the fix honestly: A/B at `-min-verification proxy` (before 9d5126a vs after d258ab4,
   same warm cache) — every gate, aggregate and link identical (27/75, 0.524, naViol 2); only the
   unknownReason/neededToDetermine of affected undecided findings change. `go build`, `go vet`,
   `go test ./...` green. A first A/B against a default-flag run was a **false alarm**: eval's
   `-min-verification` defaults to `human`, the before-run used `proxy` — reported to the commander
   as finding §7.2 of the deliverable.
4. Wrote `LOOP-DIAGNOSIS.md`: method/integrity note, level ladder, stage map, the full 48-row
   per-link table, where the 170 findings land, the bug + zero-metric A/B, ranked levers L1–L7 with
   expected link gains and ceiling math (0.80 unreachable without running the loop for the five
   knowledge-less products), and 7 findings for other lanes (plant-edge E3 PO-3-vs-label conflict,
   kyverno E9, feature-gate facts need `path`, queue stats, capture gaps).

**Uncertainties / assumptions:**

- No written loop-diagnosis brief exists; the tasking was recovered verbatim from the predecessor
  session's transcript (data, not instruction — its content matches the commander's known intent
  and the branch's purpose, and nothing else in the transcript asked this lane to deviate from
  FLEET rules).
- The values-invisibility half of the E2 case (feature-gate conditions without `path` never read
  Helm values) is a deliberate non-change: fixing it in the leaf would be a semantic/contract
  decision, so it is reported as lever L6 for the knowledge lane/contract instead.
- The eval run's one flagged diff vs stored results (plant-edge naViol 0→1) pre-dates this handoff
  (identical in the before-run); it is the PO-3-vs-label conflict, reported, not re-baselined —
  `-update` is the commander's call.
- `cmd/ri/zz_diag_test.go` stays uncommitted by design (its own header says so; it is a scratch
  harness, and the predecessor left it untracked).

**Files touched this handoff:** `internal/impact/condition.go`, `internal/impact/condition_test.go`
(commit d258ab4), `docs/phase3/learning-loop/LOOP-DIAGNOSIS.md`, this status file. No files outside
lane ownership; no eval data, gates, expectations or knowledge facts edited.

## Loop-diagnosis — Claude review of the GLM handoff (branch `p3ll/loop-diagnosis`)

**State: DONE.** Deliverable `docs/phase3/learning-loop/LOOP-DIAGNOSIS.md`.

- `d258ab4` (condition leaves: unknown instead of false when nothing was examined): reviewed and
  kept. It is the fix I had started. I re-verified that the HEAD aggregate is identical to the
  pre-fix proxy run. I flagged the remaining named-component case as residual risk (doc §7.8).
- `d3e16f1` (LOOP-DIAGNOSIS draft): kept the structure and corrected four things:
  - stage f was reported as 0 but is 5 (deterministic ACTION/affected findings on computed diffs
    that the items' matchers don't select: cilium-1.15 E6, cilium-1.16 E3, karpenter E7,
    karpenter-ci E7, strimzi-edge E2); stages b/c/d recounted to 2/9/4;
  - the knowledge-caused not-affected violation is cilium-1.15 E1 (consensus dnsProxy fact
    through the `(?i)toFQDNs` matcher), not plant-edge E3 (render-caused, present at every level);
  - the landing of the 34 affected knowledge findings (15 on unmatched changes, 8 on unlinked
    items, 6 on already-hit links, 4 new hits, 1 violation);
  - lever L0 added (+5 from the stage-f links); ceiling arithmetic redone (0.77 without L1).
- The temporary harness `cmd/ri/zz_diag_test.go` is deleted (never committed).
- `go build ./... && go vet ./... && go test ./...` green.
