# gitops-shop-pl

GitOps repository for the `shop-prod-pl` cluster. Argo CD runs in the cluster and syncs
`clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `apps/storefront/` | the public web shop (`shop.example.com`) |
| `apps/catalog-api/` | the product catalog API (`api.shop.example.com`) |

The ingress-nginx Helm values are not kept here: the platform team maintains them for all shop
clusters in `https://git.example.com/platform/ingress-values.git` (`shop/common.yaml` and
`shop/prod-pl.yaml`), which the `ingress-nginx` Application pulls in as a second source.
