# traefik-3.6-3.6--retail-emea: notes (ground truth; never read by a system under test)

Pair `traefik-3.6-3.6--shared-middlewares` with `traefik-3.6-3.6--retail-apac`. Upgrade: Traefik Proxy
v3.6.10 → v3.6.15, Helm chart 39.0.5 → 39.0.9 (`https://traefik.github.io/charts`), Flux HelmRelease.
Coverage (PLAN-M6 §2): 1 (untuned product), 4 (apparent mitigation that is ineffective), 5 (controller
configuration differing from the default, through its sibling), 6 (security regression of the target).

## Why the two environments differ

Both repositories are identical except for names and **where the operator turned on cross-namespace
references**: this one sets the environment variable `TRAEFIK_PROVIDERS_KUBERNETESCRD_ALLOWCROSSNAMESPACE=true`
through the chart's `env` value; retail-apac sets the chart value `providers.kubernetesCRD.allowCrossNamespace: true`.
Both look like "cross-namespace allowed". Only the chart value takes effect.

## B1: the Chain middleware honours allowCrossNamespace (decisive) — affected, observed

Upstream:
- Migration guide, v3.6.14 section,
  https://github.com/traefik/traefik/blob/v3.6.15/docs/content/migrate/v3.md#L21-L29:
  "In `v3.6.14`, the `Chain` middleware now honors the Kubernetes CRD provider's `allowCrossNamespace`
  option. Previously, a `Chain` could reference middlewares in other namespaces regardless of the
  `allowCrossNamespace` configuration. If `allowCrossNamespace` is set to `false` (the default) and a `Chain`
  middleware references a middleware in a different namespace from its own, the whole `Chain` is now
  rejected and an error is logged."
- CHANGELOG v3.6.14, https://github.com/traefik/traefik/blob/v3.6.15/CHANGELOG.md#L31:
  "Honor allowCrossNamespace with chain middleware CRD (#12976 @rtribotte)".
- The change is the fix of GHSA-xhjw-95fp-8vgq / CVE-2026-41174 (medium), https://github.com/advisories/GHSA-xhjw-95fp-8vgq:
  "When `providers.kubernetesCRD.allowCrossNamespace=false`, Traefik correctly rejects direct
  cross-namespace middleware references from `IngressRoute` objects, but fails to apply the same
  restriction to middleware references nested inside a `Chain` middleware's `spec.chain.middlewares[]`."
  Patched in v3.6.14 (and v2.11.43, v3.7.0-rc.2).

Why the environment variable does nothing:
- https://github.com/traefik/traefik/blob/v3.6.15/docs/content/getting-started/configuration-overview.md#L41-L47:
  "There are three different, **mutually exclusive** (i.e. you can use only one at the same time), ways to
  define install configuration options in Traefik: 1. In a configuration file 1. In the command-line
  arguments 1. As environment variables. These ways are evaluated in the order listed above."
- Code: `cmd/traefik/traefik.go#L58` at v3.6.15 registers the loaders in the order
  `DeprecationLoader, FileLoader, FlagLoader, EnvLoader`; the paerser runner (github.com/traefik/paerser
  v0.2.2, `cli/commands.go#L122-L130`) stops at the first loader that reports `done`, and
  `pkg/cli/loader_flag.go#L15-L18` reports done whenever there are arguments.
- The chart always passes arguments (`--entryPoints.*`, `--providers.kubernetescrd`, …) and renders
  `--providers.kubernetescrd.allowCrossNamespace=true` only from the chart value:
  https://github.com/traefik/traefik-helm-chart/blob/v39.0.5/traefik/templates/_podtemplate.tpl#L478-L480;
  the default is `allowCrossNamespace: false` (values.yaml L297 of chart 39.0.5).

In this environment: `apps/storefront/edge-chain.yaml` and `apps/backoffice/edge-chain.yaml` are Chain
middlewares in the application namespaces whose members (`security-headers`, `rate-limit`, `staff-auth`)
live in the `traefik` namespace; each IngressRoute uses its namespace's `edge` Chain. Effective
`allowCrossNamespace` is false, so both Chains are rejected from v3.6.14 and both routers disappear.

Observed (kind v0.24.0, kindest/node v1.31.0; `observed/run.sh`, raw log `observed/run.log`):
- L87–L108: the rendered container args have no allowCrossNamespace flag; the env var is set.
- L135–L146 (FROM v3.6.10): shop 200 with the shared headers (`x-edge`, HSTS); back office 401 without and
  200 with the staff credentials. The Chains work because of the pre-v3.6.14 bypass.
- L148–L174 (FROM): a probe IngressRoute (not part of the environment, deleted after use) with a *direct*
  reference to `traefik/security-headers` gets 404 and the log says `middleware traefik/security-headers is
  not in the IngressRoute namespace storefront`: the env var was never applied.
- L227–L265 (TO v3.6.15): shop and back office answer 404 (with and without credentials); the log says
  `Error while reading chain middleware error="middleware traefik/security-headers is not in the chain
  namespace storefront"` and `middleware "storefront-edge@kubernetescrd" does not exist` (same for the
  back office).
- L267–L326: v3.6.25 (the safer target, below) behaves like v3.6.15.

Render: `observed/render.diff` (the environment's own FROM/TO `helm template` with the HelmRelease values)
holds only release noise (chart labels, image tag). The behaviour change is compiled into the binary;
`renderedChanges: []`.

Remediation: set `providers.kubernetesCRD.allowCrossNamespace: true` in the HelmRelease values (and drop
the variable), or copy the shared middlewares into each application namespace, before upgrading.

## Other changes between v3.6.10 and v3.6.15, checked against this environment (not scored)

CHANGELOG v3.6.11–v3.6.15 and the migration guide sections up to v3.6.15: removal of untrusted X- headers
with underscores (no such headers are used), StripPrefix URL sanitization (no StripPrefix), ForwardAuth
`trustForwardHeader` (no ForwardAuth), BasicAuth timing/duplicate-header/notFoundSecret fixes (the
staff-auth behaviour stays 401/200 where routable; observed in the sibling), SameSite cookie case (no sticky
sessions), Errors middleware `errorRequestHeaders` (default unchanged; not used). None changes this
environment's behaviour.

## Version and advisory decision (coverage item 6)

Source: GitHub global advisory database, ecosystem go, package `github.com/traefik/traefik/v3`, read
2026-10-27 (`gh api /advisories?ecosystem=go&affects=github.com/traefik/traefik/v3`), ranges evaluated per
version:
- v3.6.10 (current): 28 advisories.
- v3.6.15 (proposed): 18 advisories; fixes 11 of the current ones but **introduces GHSA-6765-c87h-8mrf**
  (CVE-2026-71326, low, "BasicAuth singleflight key collision allows authenticated identity spoofing",
  range `>= 3.6.11, <= 3.6.24`, patched 3.6.25). The advisory text: "Only the v3.6 line from v3.6.11
  onwards and the v3.7 line are affected; earlier v3 releases and the v2 line do not carry the vulnerable
  deduplication path." This environment uses the BasicAuth middleware (`staff-auth`). **The proposed target
  is a security regression versus the current version.**
- v3.6.25 (latest v3.6 patch): 6 advisories, all of which also affect v3.6.10 (GHSA-7ghq-v6jf-g56c,
  GHSA-rf44-j88r-hh8c, GHSA-f52w-8j3h-j724, GHSA-qqjf-53cj-pwvv, GHSA-v67p-phpq-fc8x,
  GHSA-w4v4-9rw7-5326; fixed only in v3.7.12/v3.7.13). No regression; 22 fixed. **Expected safer target:
  v3.6.25.** No chart ships it as appVersion (39.0.9 ships v3.6.15, 40.0.0 ships v3.7.0), so in this
  repository it is chart 39.0.9 with `image.tag: v3.6.25` (observed in the run). v3.7.13+ (chart 41.5.0+)
  has no known advisory but is a minor upgrade, a separate change.
- B1 still applies at the safer target (observed), so the remediation is needed either way.
