# kube-prometheus-stack 89.2.4 → 90.2.0 — obs-prod-north (pair `control-plane-auth`, affected half)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and an
observed upgrade in a throwaway kind cluster. No Release Intelligence pipeline, agent or eval output
was run or read. Sibling: `kube-prometheus-stack-89-90--obs-south` (identical except one value).
Built on the commander's request for a pair whose breaking change **is** visible in the
environment's rendered FROM/TO diff.

## The environment

A GitOps repo for a cluster on a managed Kubernetes service: Argo CD app-of-apps, kube-prometheus-stack
from the upstream chart with two value files in order. `values.yaml` (shared): chart Grafana off
(central Grafana), scheduler/controller-manager/etcd/kube-proxy monitoring off (managed control
plane), `*SelectorNilUsesHelmValues: false` so rules and monitors from every namespace are picked up.
`apps/checkout` ships the team's `PrometheusRule checkout-alerts`: `CheckoutMemoryNearLimit`
(cAdvisor `container_memory_working_set_bytes`, scraped through the kubelet ServiceMonitor, divided
by kube-state-metrics' memory limit) and `CheckoutRestarting` (kube-state-metrics only).

`platform/monitoring/values-prod.yaml` sets `prometheus.prometheusSpec.ignoreNamespaceSelectors:
true` ("platform security baseline"); obs-south does not. That value (unchanged by the upgrade)
moves the kubelet and coredns ServiceMonitors to `kube-system` and the apiserver ServiceMonitor to
`default` in both chart versions.

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

- Chart 90 rewrites every control-plane ServiceMonitor endpoint from `bearerTokenFile` to
  `authorization.credentials` → Secret `kube-prometheus-stack-prometheus-token`, and renders that
  Secret into `monitoring`. The operator resolves the Secret in the ServiceMonitor's own namespace.
- Here those ServiceMonitors live in `kube-system`/`default`, so the lookup fails and the operator
  rejects them; their scrape jobs vanish. The checkout memory rule loses its cAdvisor input and goes
  silent; the restart rule (kube-state-metrics, ServiceMonitor in `monitoring`) is untouched.
- Nothing fails loudly: Helm succeeds, all pods are healthy, the rule still loads.

## Observed outcome (kind, 2026-10-16) — `observed/run.log`, script `observed/run.sh`

kind v0.24.0, Kubernetes v1.31.0. Argo CD resolved by hand (`helm upgrade --install` with both value
files in order; `kubectl apply -k apps/checkout/overlays/prod`). Prometheus queried from an
in-cluster curl pod. After the upgrade the script waits 6 minutes (past Prometheus' 5-minute lookback)
before observing. Cluster deleted at the end. Both cases ran in parallel from the same script.

| Check | chart 89.2.4 | chart 90.2.0 |
|---|---|---|
| control-plane ServiceMonitors | `default/…-apiserver`, `kube-system/…-coredns`, `kube-system/…-kubelet` | same placement |
| token Secret | none | `monitoring/kube-prometheus-stack-prometheus-token` only |
| scrape jobs | 12, incl. `serviceMonitor/kube-system/kube-prometheus-stack-kubelet/0..2`, `…/coredns/0`, `serviceMonitor/default/…-apiserver/0` | 7: **kubelet, coredns and apiserver jobs gone** |
| `up` by job | kubelet 3, coredns 2, apiserver 1, … | **no kubelet, coredns, apiserver** |
| events | none | **`"kube-system/kube-prometheus-stack-kubelet" was rejected due to invalid configuration: endpoints[0]: authorization: failed to get authorization token of type "Bearer": unable to get secret "kube-prometheus-stack-prometheus-token": secrets "kube-prometheus-stack-prometheus-token" not found`** (same for coredns and default/apiserver) |
| `count(container_memory_working_set_bytes{namespace="checkout",container="checkout"})` | 2 | **(no series)** |
| CheckoutMemoryNearLimit ratio (rule expr without threshold) | 0.018, 0.016 | **(no series)** — the alert can never fire |
| `kube_pod_container_status_restarts_total{namespace="checkout"}` (CheckoutRestarting) | 2 series | 2 series |
| `checkout.rules` loaded | true | true |

## What the rendered diff shows

`observed/render.diff` (helm template 89.2.4 vs 90.2.0 with both value files) is content-identical
to obs-south's: a new token Secret in `monitoring` (L62-L84), the operator's `--secret-field-selector`
(L247-L248), and `bearerTokenFile`/`caFile` → `authorization`/`ca` on the coredns (L681-L693),
apiserver (L694-L734) and kubelet (L747-L803) ServiceMonitors. **The namespaces those ServiceMonitors
render into are not in any hunk** (they do not change); the impact is decided by relating the
changed `authorization` reference to the unchanged `metadata.namespace` of the same object and to the
Secret's namespace. `expected.yaml#renderedChanges` labels each hunk.

## Judgement calls

- B1 `affected`, observed. Components: the checkout PrometheusRule (consumer that silently loses an
  input) and the three rejected ServiceMonitors. Default chart rules that depend on kubelet/apiserver
  metrics (e.g. KubeletDown) would eventually fire in a real cluster; not scored.
- The environment setting is a security hardening that the upgrade guide names explicitly; a correct
  investigation can find it from the values file + the guide alone, and confirm it from the render.
- B2 `not-affected` (observed: operator healthy). B3 `not-affected`, render-level (no removed value is
  set; nothing to observe at runtime).
