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
