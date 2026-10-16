# gitops-shop-de

GitOps repository for the `shop-prod-de` cluster. Argo CD runs in the cluster and syncs
`clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `platform/ingress-nginx/` | Helm values for the ingress-nginx chart (`values.yaml` shared, `values-prod.yaml` for this cluster) |
| `apps/storefront/` | the public web shop (`shop.example.com`) |
| `apps/catalog-api/` | the product catalog API (`api.shop.example.com`) |
