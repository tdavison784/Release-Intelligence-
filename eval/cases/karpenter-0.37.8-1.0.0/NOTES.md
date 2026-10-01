# Research notes — karpenter v0.37.8 → v1.0.0

Authored blind on 2026-10-01 from the upstream documents below.

## Sources read

- upgrade-guide.md at v1.0.0, section "Upgrading to `1.0.0`+": the warning
  alert (v1 APIs, conversion webhooks, "Do not upgrade … without referencing
  the v1 Migration Upgrade Procedure") and the full v1 changelog copied from
  the migration procedure. Expectations E1-E10 map to that changelog:
  v1beta1→v1 (E1), WhenUnderutilized rename (E2), removed annotations/tags
  (E3), taint rename (E4), required fields (E5), httpPutResponseHopLimit
  default (E6), dropped/renamed env vars (E7), Ubuntu (E8), forceful
  expiration (E9), Helm topology spread + metrics port (E10).
- Chart values at v0.37.8 and v1.0.0, diffed for the environment findings:
  `logConfig` section and `assumeRoleARN`/`assumeRoleDuration` keys are gone
  in v1.0.0 (F1's rule), `controller.metrics.port` default 8000 → 8080 (F2).
  The tag line inside values.yaml still says 0.37.0 at both tags (the chart
  default template); the digest/tag the release ships differs — the image
  findings come from the controller-image artifact, not the values file.
- compatibility.yaml at v1.0.0: 0.37.x admits Kubernetes 1.23-1.30 — grounds
  F4 for the fixture's 1.29 cluster.

## Judgement calls

- E6/E8/E9/E10 review-required: each bites under workload-dependent
  conditions (IMDS consumers, Ubuntu pools, expiry headroom, single-zone
  control planes) the fixture does not uniformly assert.
- E7 action-required: the fixture's values file sets exactly the dropped
  `logConfig`/`assumeRoleARN` keys.
- The join cannot see the CRD-level breakings (renames, taints, annotations
  are note-derived): the v1beta1 manifests make the operator relevance of
  E1/E2 real, but the expected findings are limited to the chart values,
  image and cluster-version rules the join vocabulary actually has.
