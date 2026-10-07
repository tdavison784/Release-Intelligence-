# Lane `capture` status

**State: done** (branch `p3ll/capture`). `go build ./... && go vet ./... && go test ./...` pass; schemas regenerated
(`schemas/release.schema.json`), `schemas/product-definition.schema.json` extended by hand for the new fields.

## Done
- **V3 CRD schema enrichment.** `domain.CRDVersionInfo.Fields []CRDFieldSchema{Path, Type, Default, Enum, Required}`
  (one entry per `SchemaPaths` path; default/enum as canonical JSON; array enums read from `items`; `required` from the
  parent schema). `upgrade.Build` diffs paths present on both sides into four aggregated computed changes per CRD
  version (never one per field): `crd:default-changed`, `crd:enum-changed` (breaking), `crd:field-required` (breaking),
  `crd:field-type-changed` (breaking). Titles/details follow the `crd:fields-removed` identity wording ("<Kind> <ver>
  schema: …", "in the <group/ver> schema of <crd>") so the applicability lane can recover the GVK the same way.
  None sets ActionRequired. Old snapshots without `Fields` produce no diff. Real example: Karpenter 1.13→1.14
  `NodePool v1 schema: allowed values changed: spec.disruption.consolidationPolicy`.
- **V4 karpenter.** `products/karpenter.yaml` only: `contents: [helm-values, chart-metadata]` (no locator → the `oci`
  channel, representation `published-oci-chart`) on `karpenter-chart` and `karpenter-chart-legacy`. Verified live:
  1.14.1 → 81 values keys from the published OCI chart; 1.13.0→1.14.1 values diff works
  (`settings.featureGates.capacityBuffer` added, image tag/digest defaults). No Go change was needed. The in-repo chart
  lags a release, which is why it was never declared from source.
- **V5 strimzi.** Existing constructs could not express a *set* of supported operand versions, so two generic
  constructs were added (documented in the ARCHITECTURE constructs table with their motivating case):
  `extract.collect` + `extract.where` (yaml-records: merge all records passing `where`; cells = distinct values joined
  with ", ") and `columns[].reduce: major|minor`. `products/strimzi.yaml` gains source `kafka-versions`
  (>= 0.25.0). Result for 0.45.0→0.46.0: `Kafka support narrowed: 3.8–3.9 → 3.9, 4.0 (drops 3.8)` (computed,
  citing kafka-versions.yaml at both tags).
- Tests: normalize (CRD field schemas, collect/where, reduce), upgrade (attribute diff, one-sided snapshot),
  catalog (collect/reduce validation), stats guard (map-valued catalog field now counted as a construct).
- Docs: ARCHITECTURE (diff list + constructs table), ARTIFACTS (CRD schema capture, registry-only charts).

## Eval before/after (live, warm cache)
Gates identical: all pass except applicabilityAccuracy 0.095 (unchanged, 2/21). criticalRecall/importantRecall 1.00,
falseActionRate 0.00, unsupported 0, pipelineFailures 0. Deltas:
- changes 2831 → 2851, matched 212 → 214, raw precision 0.64 → 0.65, labeled precision 0.57 → 0.58; unknown rate 0.73
  and classification accuracy 0.46 unchanged. karpenter: 43 → 56 changes (mostly values diff from the OCI chart),
  matched 13 → 15.
- **ACTION findings 3 → 4, still 0 wrong:** karpenter-0.37.8-1.0.0 gains "Helm values section `logConfig.*` removed
  (7 keys)". Existing `values:removed` join; the case environment's values.yaml sets `logConfig`, which 1.0.0 removed,
  so this is a correct ACTION that only became visible once the OCI chart values were captured. No new join rule.
- **`ri eval` reports 2 regressions vs eval/results (duplicateGroups): cilium-1.15-1.17 6→8, cilium-1.16-1.17 2→4.**
  Cause: two new `crd:field-required` changes (`CiliumEnvoyConfig` and `CiliumClusterwideEnvoyConfig`, both
  `spec.resources[]` newly required in cilium.io/v2) have the same category + subject set, which the evaluator's
  duplicate heuristic (`findDuplicates`, subject-key and near-title groups) counts as a duplicate pair although they
  belong to different CRDs. Not a real duplicate. I did NOT run `-update`; commander decides (options: accept via
  `-update`, or have the evaluator key duplicates on CRD identity too — an evaluator change, owned elsewhere).
- New computed changes are `unknown / not-joined` in impact (listed in `unimplementedDiffRules`, an additive edit to
  `internal/impact/build.go`), so they add UNKNOWN cells but no verdict change.

## Outside ownership (for merge-conflict planning)
`internal/catalog/{definition,validate}.go` (new Extract/ColumnSpec fields), `internal/normalize/{api,tables}.go`,
`internal/ingest/docs.go` (selector), `internal/stats/constructs{,_test}.go` (map support), `internal/impact/build.go`
(additive map entries), `schemas/product-definition.schema.json`, `internal/domain/release.go` (additive fields).

## Findings / notes for others
- `applicability` lane: the four new rules carry paths as `Subjects`, GVK in title/detail; consider a join rule for
  `crd:default-changed` (field unset on GVK → REVIEW) and `crd:enum-changed` (value in use). Only karpenter E2 /
  cm-1.17 E1 "if the schema states the default" can use these.
- The `fetch` client does not retry HTTP 429. ECR Public rate-limits anonymous manifest GETs in bursts (seen twice during
  `validate`/`ingest`); caching makes a re-run succeed. A bounded retry honouring `Retry-After` in `internal/fetch`
  would make live OCI ingestion of many releases smoother (not done: outside ownership).
- Kafka support is compared per minor line; a line with only some patch versions supported counts as supported.
- Strimzi E5 (3.8→4.0): the fact is now computed, but it needs a join rule on a non-values subject (operand version
  vs `spec.version`) — `applicability`/`contract` lanes.

## capture-2 (branch p3ll/capture-2)
- **Istio values eras** (`products/istio.yaml`): one `helm-values` content per era for each of the five charts —
  `availability: "< 1.24.0"` with `stripPrefix: defaults`, `">= 1.24.0"` with `_internal_defaults_do_not_set` (wrapper
  verified at upstream 1.22.0, 1.23.0, 1.23.4, 1.24.0). Existing constructs only (`contents.availability`,
  `stripPrefix`); documented in the ARCHITECTURE constructs row. The 1.23→1.24 diff is now the real restructuring
  (`pilot.*`/`cni.*` keys moved to the chart top level, `global.*` additions), no `defaults.*` removal/re-addition pairs.
- **Cilium CRDs** (`products/cilium.yaml`): the `crds` artifact read only `crds/v2`; the generated manifests are one
  directory per API version and `repo-dir` recurses, so the artifact now reads the parent
  (`pkg/k8s/apis/cilium.io/client/crds`): 22 CRDs at 1.15.6, 1.16.1 and 1.17.0 instead of 10/11/11, now including
  `CiliumLoadBalancerIPPool`, `CiliumBGP*`, `CiliumCIDRGroup`, `CiliumL2AnnouncementPolicy`, `CiliumPodIPPool`,
  `CiliumEndpointSlice`. No gap left to record (verified against the upstream listings of all three tags).
- **Eval before/after** (base = p3-learning-loop b102dd9 with a copy of the warm state, offline; after = this branch).
  `ri eval -update` not run. Note the base already fails 2 hard gates (applicabilityAccuracy 0.486, falseActionRate
  0.067 = 1 wrong of 15) — pre-existing, from lanes merged before this one.
  - istio-1.23-1.24: changes 230 → 170, duplicate groups 26 → 10 (better), E1 now hit (`cni.* removed`: the
    environment sets `cni.ambient.dnsCapture`, which the 1.24 chart reads at top level; matched by the pipeline, not
    tuned). Confusion ACTION→NOTAFF 26→25 overall.
  - gates: applicabilityAccuracy 0.486 → 0.495 (still < 0.80), falseActionRate 0.067 → 0.059 (1 wrong of 17 ACTION;
    the wrong one is unchanged; still ≥ 0.05), recall/precision unchanged apart from raw precision 0.73, labeled
    precision +2 true / −2 false; unsupported 0.
  - cilium-1.15-1.17 / 1.16-1.17: +15 / +7 computed changes (the 12 newly captured CRDs), matched +2 (the
    `CiliumLoadBalancerIPPool` field changes now exist as evidence for the expected items). **Two eval/results
    regressions: duplicateGroups 8 → 10 and 4 → 6** — `status.conditions[]` is added to several CRDs and the evaluator's
    duplicate heuristic keys on category + subject set, so identical subjects on different CRDs count as duplicates
    (the same evaluator limitation reported in capture-1). Decision for the commander (accept via `-update` or key
    duplicates on CRD identity).

## capture-3 (branch p3ll/capture-3): the 6 "not in the edge" links of LOOP-DIAGNOSIS stage 0
Each fact is captured because the upstream artifact states it; nothing was motivated by an eval expectation, no case
or result was edited, `ri eval -update` not run. One generic construct was added: **`contents: lines`** (a snapshot of
the lines of a document matching a declared RE2 pattern, diffed as sets between the releases: `lines:added` /
`lines:removed`, one computed change per line, ≤ 25 per direction + one summary; ARTIFACTS.md, ARCHITECTURE constructs
table, catalog validation, JSON schema, schemagen, tests in catalog/ingest/upgrade). Everything else is configuration.

| link | upstream statement | capture |
|---|---|---|
| crossplane E8 | `cmd/crossplane/core/core.go` retires flags in error/log messages and comments ("… flag will be removed", "… removed support for … returns an error when you enable them") | new artifact `core-flags` with a `lines` content (pattern on the retirement wording): 5 lines added/removed in the edge, incl. the `--enable-composition-webhook-schema-validation` error |
| external-secrets E7 | the commit subject "feat: make kubernetes auth prefer service account tokens over secrets (#4596)"; the v0.16.0 release body is a hand-written upgrade guide, so the commit-log source (declared a *fallback* of the release body) never ran | `commit-log` is now read for every release (no `fallbackGroup`); +37 changes for the edge (also picks up E8's RBAC commit) |
| flux E5 | the CRD description in `install.yaml`: "Note: The `Updated` template field has been removed. Use `Changed` instead." | `lines` content on `install-bundle` (pattern: "has/have been removed", "Deprecated:", "will be removed") — 4 line changes in the edge |
| kyverno E10 | the website's per-minor branch carries the table "Kyverno Version / Kubernetes Min / Kubernetes Max" | three `compatibility` sources (one per page era: `<= 1.12`, `1.13–1.16`, `1.17`) with `markdown-table` columns minimum/maximum, ref `release-{{.Major}}-{{.Minor}}-0`. Edge: "Kubernetes minimum ≥ 1.26 → ≥ 1.28", "maximum ≤ 1.29 → ≤ 1.31". **Gap, recorded in the definition:** from 1.18 the matrix is no longer a table in the page, so releases ≥ 1.18 have no window. **Captured, not matched:** the case's matcher wants one sentence naming 1.28 and 1.31, the differ reports min and max as two changes (a combined "supported window" change would be a compat-differ decision, not done) |
| prometheus-operator E3 | `pkg/operator/defaults.go`: the default Alertmanager/Thanos version constants and the Prometheus version list whose last entry is the default | new artifact `operand-defaults` with a `lines` content: `"v3.6.0",` added in the edge |
| prometheus-operator E8 | CHANGELOG 0.86.0 bullet "[ENHANCEMENT] Add `app.kubernetes.io/managed-by: prometheus-operator` label …" — folded away because the section opens with `> [!NOTE]` callouts | existing `extract.listItems: true` on the changelog source (same mechanism as Karpenter's upgrade guide): 19 → 38 changes for the edge |

### Eval before/after
Base = `p3-learning-loop` 7d39b28 (offline, copy of the warm state); after = this branch merged with it.
Gates: unchanged verdicts (the base already fails applicabilityAccuracy 0.49 and falseActionRate 0.06 — pre-existing).
- recall 0.95 → **0.98**, criticalRecall 0.98 → **1.00** (0 critical missed), importantRecall 0.95 → 0.97; missed 9 → 4.
- crossplane-1.20-2.0: E8 now found (critical), recall 0.90 → 1.00. external-secrets-0.15-0.16: E7, E8 found (recall
  0.78 → 1.00; changes 71 → 108 from the commit log, precision 0.64 → 0.67). flux-2.6-2.7: E5 found. 
  prometheus-operator-0.85-0.86: E8 found; changes 19 → 38 and **one more false positive** (the adjudicated
  "config-reloader init container port rename" bullet, now an item of its own instead of being folded into the callout
  item) — the only `eval/results` regression (`falsePositives 1 → 2`); decision for the commander (accept via
  `-update`).
- kyverno-1.12-1.13: +2 changes (the window), no new matched item (see above).
- applicabilityAccuracy 0.49 → 0.49, affected links hit 20/75 → 20/75, ACTION findings 17 → 17: none of the six links
  moves to a decided class — they leave stage 0 and now wait for the knowledge loop (unknown/not-joined), as intended.
- precision/volume: changes 3768 → 3839 (+71), unknown rate 0.79 → 0.80, raw precision 0.73 → 0.74, labeled precision
  unchanged 0.58.
