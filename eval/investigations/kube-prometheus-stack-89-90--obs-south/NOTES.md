# kube-prometheus-stack 89.2.4 → 90.2.0 — obs-prod-south (pair `control-plane-auth`, not-affected half)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and an
observed upgrade in a throwaway kind cluster. No Release Intelligence pipeline, agent or eval output
was run or read. Sibling: `kube-prometheus-stack-89-90--obs-north`.

## The environment

A GitOps repo for a cluster on a managed Kubernetes service: Argo CD app-of-apps, kube-prometheus-stack
from the upstream chart with two value files in order. `values.yaml` (shared): chart Grafana off
(central Grafana), scheduler/controller-manager/etcd/kube-proxy monitoring off (managed control
plane), `*SelectorNilUsesHelmValues: false` so rules and monitors from every namespace are picked up.
`apps/checkout` ships the team's `PrometheusRule checkout-alerts`: `CheckoutMemoryNearLimit`
(cAdvisor `container_memory_working_set_bytes`, scraped through the kubelet ServiceMonitor, divided
by kube-state-metrics' memory limit) and `CheckoutRestarting` (kube-state-metrics only).

**Difference from obs-north** (`diff -r` of the env trees shows only this plus names):
`platform/monitoring/values-prod.yaml` does not set `prometheus.prometheusSpec.ignoreNamespaceSelectors`,
so all ServiceMonitors render into the release namespace `monitoring`.

## Upstream sources

1. Chart upgrade guide at tag kube-prometheus-stack-90.2.0,
   https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/UPGRADE.md
   - L3-L10 "From 89.x to 90.x": control-plane ServiceMonitors (kubelet, kube-apiserver, … coredns)
     "no longer reference credentials on the scraper's filesystem … The bearer token now comes from a
     Secret through `authorization`, and the CA from the `kube-root-ca.crt` ConfigMap".
   - L12-L19: the chart creates the token Secret itself (`prometheus.serviceAccount.createTokenSecret`,
     default true); "The Secret must exist in the namespace of the ServiceMonitor: that is the release
     namespace, or `kube-system` (`default` for kube-apiserver) when
     `prometheus.prometheusSpec.ignoreNamespaceSelectors` is enabled — in the latter case the
     chart-created Secret is in the wrong namespace and you have to supply your own."
   - L43-L46: `prometheusOperator.secretFieldSelector` no longer excludes service-account-token Secrets (B2).
   - L48-L59: removed values table (B3).
2. Chart templates at the same tag: `templates/exporters/kubelet/servicemonitor.yaml` L5-L10 (namespace
   is `.Values.kubelet.namespace` — kube-system — when ignoreNamespaceSelectors is set, the release
   namespace otherwise; apiserver/coredns templates follow the same pattern) and
   `templates/prometheus/serviceaccount-token-secret.yaml` L1-L6 (the Secret is always rendered into
   the release namespace).
3. PR #7238 "[kube-prometheus-stack] Add secret-based auth to control-plane ServiceMonitors" (merge
   80e8c1f5, 2026-09-06; released as 90.0.0). Note: the PR description still says the new values
   "default to `{}`"; the shipped chart enables them by default (verified by rendering 90.0.0 and
   90.2.0), so the chart itself, not the PR text, is cited.
4. prometheus-operator v0.93.1 (shipped by both 89.2.4 and 90.2.0),
   `pkg/prometheus/resource_selector.go`: L237-L239 `AddSafeAuthorizationCredentials(ctx,
   sm.GetNamespace(), endpoint.Authorization)` — the Secret is looked up in the ServiceMonitor's
   namespace; L150-L155 — any error rejects the whole object ("skipping object", event "%q was
   rejected due to invalid configuration: %v").
5. Release notes 89.2.4…90.2.0 (GitHub releases): 90.0.0 is PR #7238 only; 90.1.0–90.2.0 are image
   bumps. Both chart versions ship app/operator v0.93.1 and byte-identical CRDs (checked).

Chart tarballs (https://prometheus-community.github.io/helm-charts): kube-prometheus-stack-89.2.4.tgz
sha256 `f4405216a734b182967ba8cc08169dd68a6e401688bb6885bffe0c07aad02b23`, 90.2.0
`e2b15d624c5f24170216b1b0ae28b71ed41bb33d012c668f62cff3eb905b5916` (match the repo index digests).

## Reasoning (before the run)

The rendered diff is the same as obs-north's, and every hunk of it is harmless here: the
ServiceMonitors that now reference the token Secret live in `monitoring`, where the chart creates
that Secret, so the operator resolves the credentials and keeps generating the scrape jobs.

## Observed outcome (kind, 2026-10-16) — `observed/run.log`, script `observed/run.sh`

kind v0.24.0, Kubernetes v1.31.0. Argo CD resolved by hand (`helm upgrade --install` with both value
files in order; `kubectl apply -k apps/checkout/overlays/prod`). Prometheus queried from an
in-cluster curl pod. After the upgrade the script waits 6 minutes (past Prometheus' 5-minute lookback)
before observing. Cluster deleted at the end. Both cases ran in parallel from the same script.

| Check | chart 89.2.4 | chart 90.2.0 |
|---|---|---|
| control-plane ServiceMonitors | all in `monitoring` | all in `monitoring` |
| token Secret | none | `monitoring/kube-prometheus-stack-prometheus-token` |
| scrape jobs | 12 | 12 (same jobs) |
| `up` by job | kubelet 3, coredns 2, apiserver 1, … | **kubelet 3, coredns 2, apiserver 1**, … |
| rejected ServiceMonitors (events) | none | none |
| CheckoutMemoryNearLimit inputs / ratio | 2 / 2 series, ratio ≈ 0.02 | 2 / 2 series, ratio ≈ 0.02 |
| CheckoutRestarting input | 2 series | 2 series |

`observed/render.diff`: content-identical to obs-north's (only hunk offsets differ); labelled in
`expected.yaml#renderedChanges`, all harmless here.

## Judgement calls

- B1 `not-affected`, observed; mitigation `feature-unused` (the setting that would make the chart's
  own Secret unreachable is absent). A diff-driven system that flags the authorization rewrite as
  breaking here is a false positive; one that clears it must cite that the ServiceMonitors and the
  Secret share the release namespace.
- B2, B3 as in obs-north.
