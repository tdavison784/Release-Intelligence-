# argo-cd-2.14-3.0--studio-west: notes (ground truth; never read by a system under test)

Pair `argo-cd-2.14-3.0--developer-access` with `argo-cd-2.14-3.0--studio-east`. Upgrade: Argo CD v2.14.11 →
v3.0.12 (argo-cd chart 7.9.1 → 8.2.7), installed with helmfile. Coverage (PLAN-M6 §2): 1.

## Why the two environments differ

Identical except for names and **`policy.default: role:readonly`**, which this environment sets and
studio-east does not.

## B1: logs RBAC enforced by default (decisive) — not affected, observed

Upstream: https://github.com/argoproj/argo-cd/blob/v3.0.12/docs/operator-manual/upgrading/2.14-3.0.md#L47: "Users who have `policy.default: role:readonly` or `policy.default: role:admin` in their
`argocd-rbac-cm` ConfigMap, are unaffected." The built-in readonly role grants logs:
https://github.com/argoproj/argo-cd/blob/v3.0.12/assets/builtin-policy.csv#L17 "p, role:readonly, logs, get, */*, allow".

In this environment `role:developer` has no logs permission, but every signed-in user also holds the
default role `role:readonly`, which includes `logs, get` on every application.

Observed (kind v0.24.0, kindest/node v1.31.0; same procedure and stand-ins as studio-east; `observed/run.log`):
- L230–L237: rendered `argocd-rbac-cm` has `policy.default: 'role:readonly'`.
- L251–L260 (FROM v2.14.11): Application 200, logs 200.
- L285–L299 (TO v3.0.12): Application 200, can-i logs → yes, logs → 200 with log lines.

Render: as studio-east (no RBAC hunk). Other 3.0 items and the advisory check: as studio-east (no
regression).
