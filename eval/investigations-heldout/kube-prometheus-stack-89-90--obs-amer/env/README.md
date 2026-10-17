# fleet-obs-amer

GitOps repository for the `obs-prod-amer` cluster, reconciled by Flux. The `flux-system`
GitRepository points at this repository and the root Kustomization applies `clusters/prod/`;
everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Flux `Kustomization`s: policies first, then monitoring |
| `infrastructure/policies/` | the platform's admission policies (Kubernetes `ValidatingAdmissionPolicy`) |
| `infrastructure/monitoring/` | kube-prometheus-stack from the upstream Helm chart; the `HelmRelease` takes its values from the `kube-prometheus-stack-values` ConfigMap generated from `values.yaml` |

The cluster runs on a managed Kubernetes service: the scheduler, controller manager, etcd and
kube-proxy are not reachable from inside the cluster. Dashboards and alert routing are central.
