# gitops-payments-eu

GitOps repository for the `payments-prod-eu` cluster. Argo CD runs in the cluster and syncs
`clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `platform/cert-manager/values.yaml` | Helm values for the cert-manager chart |
| `platform/pki/` | the internal PKI: a self-signed bootstrap issuer, the payments mesh root CA and the `mesh-ca` ClusterIssuer |
| `apps/payments/` | the payments services (`ledger`, `checkout`); service-to-service traffic is mutual TLS with certificates from `mesh-ca` |
