# gitops-treasury-us

GitOps repository for the `treasury-prod-de` cluster. Argo CD runs in the cluster and syncs
`clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here) |
| `platform/cert-manager/values.yaml` | Helm values for the cert-manager chart |
| `platform/pki/` | the `vault-pki` ClusterIssuer: certificates are signed by the security team's Vault (intermediate CA `pki_int`) |
| `apps/ledger/` | the ledger API; it serves TLS with a certificate from `vault-pki` |

Vault is operated by the security team outside this cluster. cert-manager authenticates to it with
the Kubernetes auth method mounted at `auth/kubernetes-treasury-prod`; the Vault role
`cert-manager-treasury` is bound to the ServiceAccount that the ClusterIssuer names, in the
`cert-manager` namespace, with the audience `vault://vault-pki`.
