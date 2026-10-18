# gitops-docs-eu

GitOps repository for the `docs-prod-eu` cluster, which serves the public product documentation.
Argo CD runs in the cluster and syncs `clusters/prod/` (app-of-apps); everything the cluster runs is
declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `platform/ingress-nginx/` | Helm values for ingress-nginx (`values.yaml` shared by all docs clusters, `values-prod.yaml` for this one) |
| `apps/docs/` | `docs-site` (the documentation pages) and `wellknown` (the `/.well-known/` documents: security.txt, change-password) behind one Ingress |
