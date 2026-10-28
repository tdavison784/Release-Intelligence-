# gitops-orders-br

GitOps repository for the `orders-prod-br` cluster (EKS). The cluster is registered in the platform
team's central Argo CD (`argocd.platform.example.com`), which syncs `clusters/prod/` from this
repository.

| Path | What |
|---|---|
| `clusters/prod/` | the Argo CD `Application`s for this cluster's own workloads (destination `orders-prod-br`) |
| `platform/external-secrets/values.yaml` | Helm values for External Secrets Operator in this cluster |
| `platform/secret-stores/` | the `aws-secrets-manager` ClusterSecretStore (IRSA) |
| `apps/orders/` | the orders API; its database credentials come from AWS Secrets Manager through an `ExternalSecret` |

External Secrets Operator itself is a cluster add-on: the platform team's `platform-addons`
ApplicationSet (in the platform repository, `gitops-platform`) installs it in every registered
cluster from the upstream chart, at the chart version the platform team sets there, with the values
file from this repository. The proposed upgrade is that chart version bump.
