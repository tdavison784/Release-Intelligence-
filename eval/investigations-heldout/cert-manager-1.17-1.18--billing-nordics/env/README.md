# platform-billing-nordics

Deployment repository for the `billing-prod-nordics` cluster. The deploy pipeline runs, in order:

1. `helmfile -e prod apply` — the platform charts in `helmfile.yaml` (values in `values/`);
2. `kubectl apply -k clusters/prod` — the PKI objects and the billing services.

Nothing else is applied to the cluster.

| Path | What |
|---|---|
| `helmfile.yaml` | cert-manager and external-secrets from their upstream charts |
| `pki/` | the billing CA key pair (synced from Vault by external-secrets) and the `billing-ca` ClusterIssuer |
| `apps/invoicing/` | `invoicing` (nginx, mutual TLS) and its two callers `dunning` and `ledger-export` |

The billing CA is operated by the security team in Vault (`pki/billing-ca`); this cluster only holds a
copy of its key pair, kept in sync by external-secrets.
