# gitops-partner-api

GitOps repository for the `partner-api-prod` cluster, which serves the partner integration API.
Argo CD runs in the cluster and syncs `clusters/prod/` (app-of-apps); everything the cluster runs is
declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `platform/ingress-nginx/values.yaml` | Helm values for ingress-nginx |
| `platform/memcached/` | the memcached instance that holds the ingress-nginx request counters |
| `apps/partner-api/` | the partner API and its Ingress (per-partner request quotas, counted across all controller replicas) |

Partner quotas are contractual: each partner (header `X-Partner-Id`) may send at most 120 requests per
minute; requests over the quota are answered with 429.
