# gitops-search-latam

GitOps repository for the `search-prod-latam` cluster. Argo CD runs in the cluster and syncs
`clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here), ordered by sync wave |
| `platform/cert-manager/values.yaml` | Helm values for the cert-manager chart |
| `platform/pki/` | internal PKI (`base/` shared by all regions, `overlays/prod/` for production) |
| `apps/search/` | `search-api` (nginx, mutual TLS) and its callers `indexer` and `query-frontend` |
