# cert-manager-1.20-1.21--treasury-de: notes (ground truth; never read by a system under test)

Pair `cert-manager-1.20-1.21--vault-pki` with `cert-manager-1.20-1.21--treasury-us`. Upgrade: cert-manager
v1.20.4 → v1.21.3 (latest patches of both lines), Helm chart `https://charts.jetstack.io`, Argo CD
multi-source Application. Coverage (PLAN-M6 §2): 2 (cert-manager, a mechanism different from
rotationPolicy / revisionHistoryLimit: chart RBAC removal).

## Why the two environments differ

Identical repositories except for names and **which ServiceAccount the `vault-pki` ClusterIssuer presents
to Vault**: this one names the controller's own ServiceAccount (`serviceAccountRef.name: cert-manager`) and
relies on the chart's `cert-manager-tokenrequest` Role; treasury-us names a dedicated `vault-issuer`
ServiceAccount and ships its own Role/RoleBinding (the pattern of the upstream Vault documentation).

## B1: chart no longer grants serviceaccounts/token for the controller's ServiceAccount (decisive) — affected, observed

Upstream:
- Release notes 1.21, "Default `tokenrequest` RBAC removed from Helm chart",
  https://github.com/cert-manager/website/blob/9ae897eae4d47184d9cd868cb6a4c7e3b45d9604/content/docs/releases/release-notes/release-notes-1.21.md#L12-L18:
  "The Helm chart no longer creates a default `Role` and `RoleBinding` granting the cert-manager controller
  permission to create tokens for its own ServiceAccount (`serviceaccounts/token: create`). … If you use
  `serviceAccountRef.name` pointing at the controller ServiceAccount, you must now either create your own
  `Role`/`RoleBinding` granting `serviceaccounts/token: create`, or migrate to a dedicated ServiceAccount
  (recommended …)."
- Changelog v1.21.0, same file L269: "**BREAKING**: The Helm chart no longer ships a default `Role` and
  `RoleBinding` … This RBAC was added in v1.16 (#7213) …" (PR cert-manager/cert-manager#8931).
- The removed objects at the FROM chart:
  https://github.com/cert-manager/cert-manager/blob/v1.20.4/deploy/charts/cert-manager/templates/rbac.yaml#L48-L88
  (Role `<fullname>-tokenrequest`: `resources: ["serviceaccounts/token"]`, `resourceNames: [<controller
  ServiceAccount>]`, `verbs: ["create"]`, and its RoleBinding to the controller ServiceAccount).
- Kubernetes-auth Vault issuers request a token for `serviceAccountRef.name` (TokenRequest) before logging in
  to Vault; the documented setup grants that with its own Role
  (https://github.com/cert-manager/website/blob/9ae897eae4d47184d9cd868cb6a4c7e3b45d9604/content/docs/configuration/vault.md#L551-L579).

In this environment: `platform/pki/vault-clusterissuer.yaml` uses `serviceAccountRef.name: cert-manager`;
nothing in the repository grants `serviceaccounts/token`. Every certificate from `vault-pki` (here
`apps/ledger/base/certificate.yaml`, served by the `ledger-api` Deployment) stops renewing.

Observed (kind v0.33.0, kindest/node v1.34.11 — cert-manager 1.21 supports Kubernetes 1.33–1.36, 1.20
supports 1.32–1.35, per https://github.com/cert-manager/website/blob/9ae897eae4d47184d9cd868cb6a4c7e3b45d9604/content/docs/releases/README.md; `observed/run.sh`, raw log `observed/run.log`). A
Vault dev server (hashicorp/vault:1.20.4, TLS) stands in for the security team's Vault with the mount,
role, policy and audience the README describes; the ClusterIssuer's `server` and the CA Secret are
substituted for the stand-in (logged at L216–L218); everything else is the environment's own manifests and
values.
- L180–L213 (FROM v1.20.4): the chart creates `role/cert-manager-tokenrequest` and its RoleBinding.
- L236–L247: ClusterIssuer `Ready=True VaultVerified`, certificate issued (revision 1), `can-i create
  serviceaccounts/cert-manager --subresource=token` → yes, a renewal is issued (revision 2).
- L249–L293 (TO v1.21.3): the tokenrequest Role/RoleBinding are gone; ClusterIssuer `Ready=False
  reason=VaultError message=Failed to initialize Vault client: while requesting a Vault token using the
  Kubernetes auth: while requesting a token for the service account /cert-manager: serviceaccounts
  "cert-manager" is forbidden: User "system:serviceaccount:cert-manager:cert-manager" cannot create resource
  "serviceaccounts/token" …`; can-i → no.
- L295–L325: a renewal is not issued in 90 s; the CertificateRequest stays `Ready=False reason=Pending
  message=Referenced issuer does not have a Ready status condition`; the Secret keeps the old certificate
  (notAfter = issue time + 168h).

Impact in time: renewBefore 48h of a 168h certificate → the first missed renewal is at most 5 days after the
upgrade, the ledger API's certificate expires at most 7 days after it.

Render: `observed/render.diff` (FROM/TO `helm template` with `platform/cert-manager/values.yaml`):
L1143–L1165 and L1225–L1246 remove the tokenrequest Role and RoleBinding (B1, breaking here); the CRD hunks
add optional fields (harmless); everything else is release noise.

Remediation: before the upgrade, grant the controller ServiceAccount `create` on `serviceaccounts/token`
(resourceNames `[cert-manager]`) with a Role/RoleBinding in `cert-manager`, or move to a dedicated
ServiceAccount (and rebind the Vault role).

## Other 1.21 changes checked against this environment (not scored)

- `cert-manager-edit` no longer grants create on Challenges/Orders (already in v1.20.3; no ACME, no tooling).
- Removed metrics values `prometheus.servicemonitor.targetPort/path`, `prometheus.podmonitor.path` (schema
  rejects them): the values file does not set them (`prometheus.enabled: false`).
- No security regression: the GitHub global advisory database lists no advisory for
  `github.com/cert-manager/cert-manager` affecting v1.20.4 or v1.21.3 (read 2026-10-27).
