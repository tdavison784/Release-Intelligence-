# traefik-3.6-3.6--retail-apac: notes (ground truth; never read by a system under test)

Pair `traefik-3.6-3.6--shared-middlewares` with `traefik-3.6-3.6--retail-emea`. Upgrade: Traefik Proxy
v3.6.10 → v3.6.15, Helm chart 39.0.5 → 39.0.9 (`https://traefik.github.io/charts`), Flux HelmRelease.
Coverage (PLAN-M6 §2): 1 (untuned product), 5 (controller configuration differing from the default: the
`allowCrossNamespace` flag decides the outcome), 6 (security regression of the target).

## Why the two environments differ

Both repositories are identical except for names and **where the operator turned on cross-namespace
references**: this one sets the chart value `providers.kubernetesCRD.allowCrossNamespace: true` (rendered as
the flag `--providers.kubernetescrd.allowCrossNamespace=true`); retail-emea sets the environment variable
`TRAEFIK_PROVIDERS_KUBERNETESCRD_ALLOWCROSSNAMESPACE=true`, which Traefik ignores because the chart passes
command-line arguments (see retail-emea's NOTES.md). An observation from a default-like controller (flag
off) must not be carried over to this environment, and vice versa.

## B1: the Chain middleware honours allowCrossNamespace (decisive) — not affected, observed

Upstream: migration guide v3.6.14 section,
https://github.com/traefik/traefik/blob/v3.6.15/docs/content/migrate/v3.md#L21-L29 ("If
`allowCrossNamespace` is set to `false` (the default) and a `Chain` middleware references a middleware in a
different namespace from its own, the whole `Chain` is now rejected and an error is logged."); CHANGELOG
v3.6.14 (https://github.com/traefik/traefik/blob/v3.6.15/CHANGELOG.md#L31); GHSA-xhjw-95fp-8vgq. The chart
renders the flag from the value:
https://github.com/traefik/traefik-helm-chart/blob/v39.0.5/traefik/templates/_podtemplate.tpl#L478-L480.

In this environment the Chains (`apps/*/edge-chain.yaml`) reference middlewares of the `traefik` namespace
and the flag is on, so v3.6.14+ accepts them.

Observed (kind v0.24.0, kindest/node v1.31.0; `observed/run.sh`, raw log `observed/run.log`):
- L85–L96: the container args include `--providers.kubernetescrd.allowCrossNamespace=true`.
- L134–L168 (FROM v3.6.10): shop 200 with the shared headers; back office 401 / 200 with credentials; a
  direct cross-namespace probe reference (not part of the environment) is accepted (200).
- L227–L261 (TO v3.6.15): identical results; no chain errors in the log (L267).
- L269–L325: v3.6.25 (the safer target) identical.

Render: `observed/render.diff` holds release noise only; `renderedChanges: []`.

Other v3.6.11–v3.6.15 changes: as in retail-emea's NOTES.md, none applies.

## Version and advisory decision (coverage item 6)

Same upgrade, same decision as retail-emea (GitHub global advisory database, package
`github.com/traefik/traefik/v3`, read 2026-10-27): the proposed v3.6.15 **introduces GHSA-6765-c87h-8mrf**
(CVE-2026-71326, low, BasicAuth singleflight key collision, `>= 3.6.11, <= 3.6.24`, patched 3.6.25), which
does not affect the current v3.6.10; this environment uses BasicAuth (`staff-auth`). **Expected safer
target: v3.6.25** (6 advisories, all also affecting v3.6.10; 22 fixed), deployed as chart 39.0.9 with
`image.tag: v3.6.25` because no chart ships it; observed working here.
