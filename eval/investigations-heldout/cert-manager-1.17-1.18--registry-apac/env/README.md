# gitops-registry-apac

GitOps repository for the `registry-prod-apac` cluster. Argo CD runs in the cluster and syncs
`clusters/prod/` (app-of-apps); everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Argo CD `Application`s (the root app points here), ordered by sync wave |
| `platform/kyverno/values.yaml` | Helm values for Kyverno (admission policies only) |
| `platform/policies/` | the platform's Kyverno policies |
| `platform/cert-manager/values.yaml` | Helm values for cert-manager |
| `platform/pki/` | internal PKI: self-signed bootstrap issuer, the build root CA and the `build-ca` ClusterIssuer |
| `apps/registry/` | `artifacts`, the internal artifact registry front end (nginx, client certificates required) |
| `apps/builders/` | `builder`, the CI build agents that pull from `artifacts` with their client certificate |
