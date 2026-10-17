# fleet-telemetry-west

GitOps repository for the `telemetry-prod-west` cluster, reconciled by Flux. The `flux-system`
GitRepository points at this repository and the root Kustomization applies `clusters/prod/`;
everything the cluster runs is declared here.

| Path | What |
|---|---|
| `clusters/prod/` | Flux `Kustomization`s, applied in dependency order: cert-manager → pki → telemetry, edge |
| `infrastructure/cert-manager/` | cert-manager from the upstream Helm chart (`HelmRelease`, values inline) |
| `infrastructure/pki/` | internal PKI: a self-signed bootstrap issuer, the telemetry root CA and the `telemetry-ca` ClusterIssuer |
| `apps/telemetry/` | `ingest`, the telemetry intake (nginx, mutual TLS) |
| `apps/edge/` | `gateway` and `rollup`, which push to `ingest` over mutual TLS |

All service-to-service traffic is mutual TLS with certificates from `telemetry-ca`; every workload
trusts the CA certificate that comes with its own certificate Secret (`ca.crt`).
