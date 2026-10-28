# argo-cd-2.14-3.0--studio-east: notes (ground truth; never read by a system under test)

Pair `argo-cd-2.14-3.0--developer-access` with `argo-cd-2.14-3.0--studio-west`. Upgrade: Argo CD v2.14.11 →
v3.0.12 (argo-cd chart 7.9.1 → 8.2.7, `https://argoproj.github.io/argo-helm`; the last charts of each line
in the index), installed with helmfile. Coverage (PLAN-M6 §2): 1 (a product RI has not been tuned on;
`products/argo-cd.yaml`) and 4 (an apparent pin that is ineffective).

## Why the two environments differ

Identical except for names and **`policy.default`**: studio-west sets `policy.default: role:readonly`; this one
has none. Both keep `server.rbac.log.enforce.enable: false` in `argocd-cm`, which looks like it preserves
the v2 behaviour.

## B1: logs RBAC enforced by default (decisive) — affected, observed

Upstream: upgrade guide 2.14 → 3.0 at v3.0.12, https://github.com/argoproj/argo-cd/blob/v3.0.12/docs/operator-manual/upgrading/2.14-3.0.md#L33-L49: "2.4 introduced `logs` as a new RBAC resource.
In 2.3 and lower, users with `applications, get` access automatically got logs access. In 2.4, it became
possible to enable logs RBAC enforcement with a flag in `argocd-cm` … Starting from 3.0, this flag is
removed and the logs RBAC is enforced by default … Users who have `policy.default: role:readonly` or
`policy.default: role:admin` in their `argocd-rbac-cm` ConfigMap, are unaffected. Users who don't have a
`policy.default` in their `argocd-rbac-cm` ConfigMap, and either have `server.rbac.log.enforce.enable` set
to `false` or don't have this setting at all in their `argocd-cm` ConfigMap are affected". Recommended
remediation, L60–L62: "Explicitly add a `logs, get` policy to every role that has a policy for
`applications, get` or for `applications, *`."

In this environment (`argocd/values.yaml`): `role:developer` (Okta group `studio-devs`) has applications
get/sync on `games/*` and projects get, no `logs`; no `policy.default`; `server.rbac.log.enforce.enable:
false`. The README says the developers open pod logs when they debug.

Observed (kind v0.24.0, kindest/node v1.31.0; `observed/run.sh`, raw log `observed/run.log`). The argo-cd
chart is installed with the environment's `argocd/values.yaml` (what the helmfile release does) plus an
observation overlay (L169–L174): a local account `dev1` added to the group `studio-devs`
(`g, dev1, studio-devs`) as a stand-in for an Okta member of that group; the environment's policy lines
are unchanged. The `games` AppProject comes from `argocd/apps.yaml`; the `lobby` Application keeps its
project and destination but deploys a public chart (podinfo 6.7.1) because kind has no Git server.
- L230–L237: rendered `argocd-cm` has `server.rbac.log.enforce.enable: 'false'`; `argocd-rbac-cm` has the
  policy above and `policy.default: ''`.
- L251–L260 (FROM v2.14.11): dev1 signs in; `GET /api/v1/applications/lobby` → 200; can-i logs → yes;
  `GET …/lobby/logs` → 200 with log lines.
- L285–L296 (TO v3.0.12, same values): the flag is still rendered `'false'`; Application → 200; can-i logs
  → no; logs → `{"error":{"grpc_code":7,"http_code":403,"message":"permission denied: logs, get,
  games/lobby, sub: dev1, …"}}`, HTTP 403.

Render: `observed/render.diff` has no RBAC hunk (the enforcement is in the binary); `argocd-cm` gains the
v3.0 default `resource.exclusions` and `ignoreResourceUpdates` (another 3.0 item, not in scope), the rest
is CRD schema additions, new commit-server params and release noise. `renderedChanges: []` for B1.

Remediation: add `p, role:developer, logs, get, games/*, allow` before the upgrade.

## Other 2.14 → 3.0 items checked against this environment (not scored)

Fine-grained update/delete sub-resource RBAC (role:developer has neither), Dex `sub` claim (Dex disabled;
Okta OIDC), legacy repo config in argocd-cm (none), ApplicationSet `applyNestedSelectors` (none),
annotation-based tracking by default (applications re-annotate on the next sync; nothing here selects
on the tracking label), default resource exclusions (no Endpoints/Leases managed), removed metrics.

## Version and advisory decision (not a coverage item here)

GitHub global advisory database (read 2026-10-27), `github.com/argoproj/argo-cd/v2` at 2.14.11: 9
advisories; `github.com/argoproj/argo-cd/v3` at 3.0.12: 8, none of which is new relative to 2.14.11. No
regression. (Later v3.0 tags exist up to v3.0.23 with fewer advisories, but no chart release ships them.)
