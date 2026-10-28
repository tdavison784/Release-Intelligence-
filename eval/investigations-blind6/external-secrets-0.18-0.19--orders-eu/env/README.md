# gitops-orders-eu

GitOps repository for the `orders-prod-eu` cluster (EKS). Argo CD runs in the cluster, manages
itself, and syncs `clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | the `platform` AppProject and the Argo CD `Application`s (the root app points here) |
| `platform/argocd/values.yaml` | Helm values for Argo CD itself (argo-cd chart) |
| `platform/external-secrets/values.yaml` | Helm values for External Secrets Operator |
| `platform/secret-stores/` | the `aws-secrets-manager` ClusterSecretStore (IRSA) |
| `apps/orders/` | the orders API; its database credentials come from AWS Secrets Manager through an `ExternalSecret` |
