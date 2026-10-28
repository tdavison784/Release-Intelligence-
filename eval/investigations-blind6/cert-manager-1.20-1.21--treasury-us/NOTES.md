# cert-manager-1.20-1.21--treasury-us: notes (ground truth; never read by a system under test)

Pair `cert-manager-1.20-1.21--vault-pki` with `cert-manager-1.20-1.21--treasury-de`. Upgrade: cert-manager
v1.20.4 → v1.21.3, Helm chart `https://charts.jetstack.io`, Argo CD multi-source Application. Coverage
(PLAN-M6 §2): 2.

## Why the two environments differ

Identical repositories except for names and **which ServiceAccount the `vault-pki` ClusterIssuer presents
to Vault**: this one names a dedicated `vault-issuer` ServiceAccount and grants the controller
`serviceaccounts/token` create for it with its own Role/RoleBinding
(`platform/pki/vault-issuer-serviceaccount.yaml`); treasury-de names the controller's own ServiceAccount and
relies on the chart's Role, which v1.21 removes.

## B1: chart no longer grants serviceaccounts/token for the controller's ServiceAccount (decisive) — not affected, observed

Upstream: release notes 1.21 (https://github.com/cert-manager/website/blob/9ae897eae4d47184d9cd868cb6a4c7e3b45d9604/content/docs/releases/release-notes/release-notes-1.21.md#L12-L18, L269), the
removed template (https://github.com/cert-manager/cert-manager/blob/v1.20.4/deploy/charts/cert-manager/templates/rbac.yaml#L48-L88)
and the documented dedicated-ServiceAccount pattern (https://github.com/cert-manager/website/blob/9ae897eae4d47184d9cd868cb6a4c7e3b45d9604/content/docs/configuration/vault.md#L551-L579), which this
environment follows. The removed Role only covered tokens for the controller's own ServiceAccount; this
issuer never requests those.

Observed (kind v0.33.0, kindest/node v1.34.11; Vault dev stand-in as in treasury-de, role bound to
`vault-issuer`; `observed/run.log`):
- L216–L222: the environment's `vault-issuer` ServiceAccount, Role and RoleBinding are applied.
- L239–L250 (FROM v1.20.4): ClusterIssuer Ready, certificate issued and renewed; can-i (controller → token for
  `vault-issuer`) yes.
- L252–L313 (TO v1.21.3): the chart's tokenrequest Role/RoleBinding are gone, the environment's remain;
  ClusterIssuer `Ready=True VaultVerified`; can-i yes; a renewal is issued (revision 3).

Render: as treasury-de (same values); the tokenrequest Role/RoleBinding removals are harmless here.

No security regression (no advisory for v1.20.4 or v1.21.3 in the GitHub global advisory database, read
2026-10-27).
