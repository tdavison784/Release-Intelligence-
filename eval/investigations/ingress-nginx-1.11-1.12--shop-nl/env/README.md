# gitops-shop-nl

GitOps repository for the `shop-prod-nl` cluster. Argo CD runs in the cluster and syncs
`clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `platform/ingress-nginx/` | Helm values for the ingress-nginx chart (`values.yaml` shared, `values-prod.yaml` for this cluster) |
| `apps/storefront/` | the public web shop (`shop.example.com`) |
| `apps/catalog-api/` | the product catalog API (`api.shop.example.com`) |
| `platform/otel-collector/` | the OpenTelemetry collector that receives the ingress traces |

Requests are traced at the edge: ingress-nginx sends OpenTelemetry spans to the in-cluster
collector. The controller's Prometheus metrics are scraped by the central Prometheus through the pod
annotations.
