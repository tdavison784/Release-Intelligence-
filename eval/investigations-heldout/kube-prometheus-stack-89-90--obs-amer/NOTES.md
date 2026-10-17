# kube-prometheus-stack-89-90--obs-amer — notes (ground truth; not for systems under test)

Pair `kube-prometheus-stack-89-90--token-secret` with `--obs-emea`. The two environments differ only in
the platform policy `deny-service-account-token-secrets` (absent here; the NodePort policy is in both),
plus names.

## Upstream sources

- Chart UPGRADE.md at tag `kube-prometheus-stack-90.2.0`, section "From 89.x to 90.x":
  [`UPGRADE.md#L3-L19`](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/UPGRADE.md#L3-L19) (B1):
  > The bearer token now comes from a Secret through `authorization`, and the CA from the `kube-root-ca.crt`
  > ConfigMap ... Because a ServiceMonitor can only authenticate through a Secret, and Kubernetes no longer
  > creates a token Secret for a service account automatically, the chart now creates one itself:
  > `prometheus.serviceAccount.createTokenSecret` (default `true`) renders a `kubernetes.io/service-account-token`
  > Secret named `<prometheus service account>-token`. That token is long-lived and does not expire. Disable the
  > value and set the `authorization` of each component if you would rather manage the credential yourself.

  [`#L43-L46`](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/UPGRADE.md#L43-L46) (B2):
  > `prometheusOperator.secretFieldSelector` no longer excludes `kubernetes.io/service-account-token`, so that
  > changes to that Secret trigger a reconciliation. The operator reads Secrets referenced by a ServiceMonitor
  > directly, so scraping also works with the previous value, but the generated configuration is then only
  > refreshed on the next unrelated reconciliation.

  [`#L48-L59`](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/UPGRADE.md#L48-L59) (B3): the table of removed values
  (`<component>.serviceMonitor.bearerTokenFile`, `kubelet.serviceMonitor.insecureSkipVerify`, the
  `kubeEtcd`/`kubeControllerManager`/`kubeScheduler`/`kubeProxy` TLS keys, `kubeApiServer.tlsConfig.caSecret`).
- Chart 90.2.0 `templates/prometheus/serviceaccount-token-secret.yaml` renders the Secret (`type:
  kubernetes.io/service-account-token`, annotation `kubernetes.io/service-account.name`) whenever
  `prometheus.enabled`, `prometheus.serviceAccount.create` and `createTokenSecret` are true (all defaults).
- ValidatingAdmissionPolicy (GA in Kubernetes 1.30; kind v1.31.0 here), kubernetes/website `c2f45dad`
  [`validating-admission-policy.md#L94-L98`](https://github.com/kubernetes/website/blob/c2f45dad1a42840c53d59e30b7e4d1706392ecea/content/en/docs/reference/access-authn-authz/validating-admission-policy.md#L94-L98):
  > The supported `validationActions` are: - `Deny`: Validation failure results in a denied request.
- Flux's helm-controller installs and upgrades the release with the Helm library; the observation uses the
  helm CLI with the same chart and values. The HelmRelease configures no upgrade remediation. (How often
  helm-controller would retry was not observed and does not change the outcome: every attempt creates the
  same Secret.)

## Reasoning in terms of the fixture

- Nothing in the environment objects to the chart-created token Secret, Prometheus runs in the release
  namespace with the chart's service account, and the ServiceMonitors live in the release namespace (no
  `ignoreNamespaceSelectors`). The large rendered change of B1 is **harmless: not affected**.
- B2: the selector is pinned to the previous default (override); UPGRADE.md: scraping still works with it,
  only refreshes of the token Secret wait for the next unrelated reconciliation, and the token does not
  change. Not affected. B3: no removed value is set. Not affected.

## Observed outcome

`observed/run.sh` (kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1; chart from
prometheus-community.github.io; shared by the pair), log `observed/run.log`. Flux's order is reproduced:
`infrastructure/policies` applied, then the HelmRelease as `helm install/upgrade` with the values of the
generated ConfigMap (`infrastructure/monitoring/values.yaml`). Prometheus' targets are read through the API
server's service proxy (`/api/v1/targets`, per scrape pool up/total and last error), 75 s after the install
and 120 s after the upgrade.

| Step (run.log) | Result |
|---|---|
| install 89.2.4 (L42) | deployed; no service-account-token Secret |
| baseline targets (L75) | apiserver 1/1, coredns 2/2, kubelet 3 × 1/1, … all up |
| `helm upgrade` to 90.2.0 (L101) | deployed (revision 2) |
| after the upgrade (L134-L148) | Secret `kube-prometheus-stack-prometheus-token` (type service-account-token, 3 keys) exists; kubelet ServiceMonitor uses `authorization` with it |
| targets 120 s later (L149) | apiserver 1/1, coredns 2/2, kubelet 3 × 1/1, … **all up** |

`observed/render.diff`: identical to `--obs-emea`; every hunk harmless here.
