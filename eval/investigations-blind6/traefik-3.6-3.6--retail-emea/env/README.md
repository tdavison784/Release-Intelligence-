# gitops-retail-emea

GitOps repository for the `retail-prod-emea` cluster. Flux runs in the cluster (bootstrapped
separately) and reconciles `clusters/prod/`; everything else the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Flux `Kustomization`s: `traefik`, then `edge-middlewares`, then `apps` |
| `infrastructure/traefik/` | Traefik Proxy, from the upstream Helm chart (`HelmRelease`, values inline) |
| `infrastructure/edge-middlewares/` | the platform's shared edge middlewares in the `traefik` namespace (security headers, rate limit, staff basic auth) |
| `apps/storefront/` | the public shop (`shop.retail-emea.example.com`) |
| `apps/backoffice/` | the staff back office (`backoffice.retail-emea.example.com`) |

Application teams own their namespace and attach the shared edge middlewares to their routes
through a `Chain` middleware in their own namespace, so that every team gets the same headers and
limits without copying them.
