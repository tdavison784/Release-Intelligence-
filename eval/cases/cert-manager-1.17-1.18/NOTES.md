# Research notes: cert-manager v1.17.0 → v1.18.0 (with environment)

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- Upgrading v1.17 to v1.18 guide:
  https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md
  Two numbered breaking changes: RotationPolicy default Never→Always and
  RevisionHistoryLimit default nil→1.
- Release 1.18 notes:
  https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md
  Adds the ACME HTTP01 `PathType: Exact` breaking change (with the
  ingress-nginx >= 1.12.0 interaction and the OpenShift Route interaction),
  ACME certificate profiles, and the OperatorHub discontinuation restated.
- Support table (Kubernetes ranges per release):
  https://github.com/cert-manager/website/blob/master/content/docs/releases/README.md
  1.17 and 1.18 both support Kubernetes "1.29 → 1.33", so a 1.31 cluster must
  end up "in range", not flagged.
- Chart values at both endpoints (for the environment fixture):
  https://github.com/cert-manager/cert-manager/blob/v1.17.0/deploy/charts/cert-manager/values.yaml
  (`prometheus.servicemonitor.targetPort: 9402`, no
  `global.rbac.disableHTTPChallengesRole`) and
  https://github.com/cert-manager/cert-manager/blob/v1.18.0/deploy/charts/cert-manager/values.yaml
  (`targetPort: http-metrics`, `disableHTTPChallengesRole: false`).

## Judgement calls

- E3 (PathType Exact) is critical because the workaround decision is forced:
  disable the feature gate, relax ingress-nginx validation, or upgrade
  ingress-nginx to v1.12.6/v1.13.2. The ingress-nginx side is corroborated by
  https://github.com/kubernetes/ingress-nginx/pull/11819 (validation enabled
  by default in >= 1.12.0) — the same PR that motivates the separate
  ingress-nginx-1.11-1.12 case in this dataset.
- The environment pins `targetPort: 9402` and sets
  `disableHTTPChallengesRole: true`: one pin over a CHANGED default (should be
  informational: the pin keeps winning) and one value that only exists from
  v1.18.0 (starts taking effect).
- expectedImpact E1 is deliberately a hard join: the Certificate manifests do
  NOT set rotationPolicy, so the environment is affected precisely by what it
  does NOT say. Recording it as `review` keeps the expectation honest; a
  tool that only joins on explicitly-set fields will miss it.
- notExpectedFindings: cert-manager v1.18.0 still ships every CRD v1.17.0
  shipped (checked against the two chart values/CRD trees), so a
  crd-removed finding would be a false positive.
