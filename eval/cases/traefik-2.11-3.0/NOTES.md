# traefik v2.11.2 → v3.0.0 — research notes

Authored blind on 2026-10-01 from upstream material only (no `ri` run, no
`eval/results`, no adjudications). All upstream files were fetched with
`curl` / `gh` at the two tags (source tarballs of `v2.11.2` and `v3.0.0`).

## Edge choice

- Tags verified with `git ls-remote --tags https://github.com/traefik/traefik`:
  `v2.11.2` and `v3.0.0` exist and match the product `tagPattern`.
- `from: v2.11.2` is the newest v2.11 patch that appears in the v3.0.0
  CHANGELOG (`## [v2.11.2] (2024-04-11)`), i.e. the v2 line a user ran when
  v3.0.0 shipped (2024-04-29). No later v2.11 patch is needed: everything in
  this case is a v2→v3 major change.

## Sources and what each contributed

| Source | Contribution |
|---|---|
| `docs/content/migration/v2-to-v3.md` @ v3.0.0 | The breaking-change list (product channel `upgrade-guide-v2-to-v3`): E1, E2, E3, E4, E5, E7, E8, E9 (v1beta1 removals), E10. At v3.0.0 there are no separate "migration details" pages in the migration directory (only `v1-to-v2.md`, `v2-to-v3.md`, `v2.md`), so this single guide is the whole migration document at the tag. |
| `CHANGELOG.md` @ v3.0.0 | Corroborating bullets: "Moves HTTP/3 outside the experimental section" (#9570), "Update routing syntax" (#9531), "Disable Content-Type auto-detection by default" (#9546), "Toggle support for experimental channel" (#10435), "Update Kubernetes version for v3 Helm chart" (#10637). Also the source of the `notExpected` merge/typo bullets. |
| GitHub release v3.0.0 | Starts with "Please read the migration guide"; contains "Upgrade gateway api to v1.0.0" (#10205) and "Remove support of the networking.k8s.io/v1beta1 APIVersion" (#9949) — used for E6 and E9. |
| `providers/kubernetes-gateway.md` @ both tags | v2.11.2: "only supports the `v0.4.0` (v1alpha2)"; v3.0.0: "partially supports Gateway API v1.0.0". E6. |
| `pkg/provider/kubernetes/gateway/client.go` @ both tags, `go.mod` | v2.11.2 imports only `gateway-api/apis/v1alpha2` (go.mod `sigs.k8s.io/gateway-api v0.4.0`); v3.0.0 imports `apis/v1`, `v1alpha2`, `v1beta1` (go.mod v1.0.0). Grounds E6's statement that v3 reads v1 resources. |
| `getting-started/install-traefik.md` @ both tags | "Kubernetes 1.16+" → "Kubernetes 1.22+" (PR #10637). E9: a requirement stated only in prose. |
| `includes/kubernetes-requirements.md` @ v3.0.0 | "supports at least the latest three minor versions of Kubernetes" — too vague to be a constraint; not used as the bound, recorded here only. |
| `reference/static-configuration/cli-ref.md` @ both tags | v3.0.0 `--core.defaultrulesyntax` "(Default: ```v3```)"; v2.11.2 has `--experimental.http3` and no `core.*`; v3.0.0 has `--providers.kubernetesgateway.experimentalchannel` and no `--experimental.http3`. Grounds the flag names in E2/E4/E7. |
| `pkg/config/static/static_config.go` @ v2.11.2 | `Pilot *Pilot` "Traefik Pilot configuration (Deprecated)" still parsed in v2.11.2 (it is not in the v2.11.2 cli-ref), so pilot flags could be present on a running v2.11.2. E5. |
| `reference/dynamic-configuration/kubernetes-crd-definition-v1.yml` @ both tags | 18 `containo` occurrences at v2.11.2, 0 at v3.0.0; the v2.11.2 file defines nine kinds in `traefik.containo.us` (the E1 subject list). |
| `reference/dynamic-configuration/kubernetes-crd-rbac.yml` @ both tags | diff: v3.0.0 drops `traefik.containo.us` from the rule and adds `serverstransporttcps`. E8; the fixture ClusterRole is the v2.11.2 file verbatim (plus the v2.11.2 `kubernetes-gateway-rbac.yml` rule). |
| `pkg/provider/kubernetes/crd/traefikio/v1alpha1/middleware.go` @ both tags, `migration/v2.md` @ v2.11.2, `middlewares/http/ipallowlist.md` @ v2.11.2 | `ipAllowList` already exists in v2.11.2 (both groups); v3.0.0 still has `IPWhiteList` with "// Deprecated: please use IPAllowList instead." → E3 is a deprecation, not a removal, at v3.0.0. |
| `traefik.io_ingressroutes.yaml` @ v3.0.0, `routing/routers/index.md` @ v3.0.0 | Per-route `syntax` field on IngressRoute routes; "The default value of the `ruleSyntax` option is inherited from the `defaultRuleSyntax` option". E2 paths. |
| `middlewares/http/contenttype.md` @ v3.0.0 | ContentType middleware enables auto-detection. E10. |

## Items and G22 categories

| Item | One line | Class | G22 category |
|---|---|---|---|
| E1 | `traefik.containo.us` CRD group removed → `traefik.io` | action-required | removal (gvk removed, migration of resources) |
| E2 | default rule syntax is now v3; v2-only rules break unless rewritten or opted out (`core.defaultRuleSyntax` / per-route `syntax`) | action-required | behaviour change with escape hatch; prose-only default |
| E3 | `ipWhiteList` → `ipAllowList` (deprecated, still accepted) | review-required | deprecation with replacement |
| E4 | `experimental.http3` removed, Traefik will not start | action-required | removed static option / CLI flag |
| E5 | Pilot options removed, Traefik will not start | action-required | removed static option / CLI flag |
| E6 | Gateway provider needs Gateway API v1.0.0 (was v0.4.0) | action-required | cross-product version dependency (`product-relationship` + `product-version` leaf) |
| E7 | experimental-channel resources off by default; `experimentalChannel` needed for TCPRoute/TLSRoute | action-required | required new config / changed default |
| E8 | RBAC must add `serverstransporttcps` | action-required | RBAC |
| E9 | Kubernetes requirement 1.16+ → 1.22+ | action-required | compatibility hidden in prose (install page, not the migration guide) |
| E10 | Content-Type no longer auto-detected | review-required | behaviour change, prose-only default |

`classification` on each item is the item's generic exposed class (= the
consequence kind's class). The per-environment answer is the link's
`relevance`; for E3, E5, E7 and E9 that is `not-affected` (see below).

## Environment and links

The fixture is a plain-manifest install (Deployment + RBAC + CRs), because
the brief asks for `cli-flag` leaves read from container `args`. The static
configuration lives only in the args (stated in the description), so "flag
absent" is a decidable fact for the `cli-flag` leaves.

| Link | Relevance | Decided by |
|---|---|---|
| E1 | action-required | `legacy-dashboard.yaml`: IngressRoute + Middleware in `traefik.containo.us/v1alpha1` |
| E2 | action-required | `apps.yaml` shop route `Host(`shop.example.com`, `www.shop.example.com`)`, no `syntax`, no `--core.defaultRuleSyntax` |
| E3 | **not-affected** | the only IP-filter Middleware uses `ipAllowList`; no `ipWhiteList` anywhere |
| E4 | action-required | `--experimental.http3=true` in args |
| E5 | **not-affected** | no `pilot.*` arg |
| E7 | **not-affected** | Gateway provider on, but only GatewayClass/Gateway/HTTPRoute, no TCPRoute/TLSRoute |
| E8 | action-required | ClusterRole `traefik` = v2.11.2 reference RBAC, no `serverstransporttcps`; `--providers.kubernetescrd` on |
| E9 | **not-affected** | `environment.kubernetes: "1.29"` is inside `>=1.22.0` |
| E6 | undecided (`cross-product-context-gap`) | Gateway API CRD version not recorded and not in the inventory |
| E10 | undecided (`runtime-behavior-gap`) | whether backends set Content-Type is runtime behaviour |

Judgement calls:

- **E3 not-affected vs informational.** The environment does not touch the
  deprecated field at all (it uses the replacement), so it is not an overlap
  ("touches the subject but shielded"); it is a clean not-affected. The
  legacy-group Middleware (`dashboard-auth`, basicAuth) was kept free of
  `ipWhiteList` on purpose, so the link stays not-affected even under a
  group-agnostic reading.
- **E5 / E7 not-affected** rest on the args being the complete static
  configuration (stated in the description). E7 is the "looks similar"
  case: the Gateway provider is enabled, yet no experimental-channel kind is
  in use.
- **E9 not-affected** is a cluster-version check; it does not depend on the
  install method.
- **E6 undecided, not affected.** The fixture's Gateway API objects are
  `v1alpha2`, which is what v2.11 reads. The Gateway API v1.0.0 bundle also
  serves `v1`, so the manifests alone do not show which bundle is
  installed; the honest answer is UNKNOWN until the CRD bundle version is
  supplied. The inventory deliberately lists only Traefik itself (the one
  product version the description states) — no `complete:` key, so absence
  of `gateway-api` is not proof of absence. `gateway-api` is not a catalog id
  in `products/`; it is used as the related-product name as written.
- **E10 undecided.** "No ContentType middleware" is decidable, but the
  consequence depends on whether backends omit Content-Type; no static input
  carries that.
- **E2 relevance action-required.** The shop route's rule uses two values in
  one `Host()` matcher, which the guide says v3 no longer accepts ("All
  matchers now take a single value ... and should be explicitly combined
  using logical operators"). The `orders-api` and `dashboard` rules are valid
  in both syntaxes and would not be exposed on their own.
- **E8 consequence.** The guide says it is "necessary to update RBAC and CRD
  manifests" but does not spell out the failure mode. The statement (provider
  cannot list/watch `serverstransporttcps` and does not sync) is my reading
  of what a missing list/watch permission does to an informer-based
  provider; flagged as the least directly quoted statement in the case.
- **E6 consequence.** Grounded in the v3.0.0 Gateway client importing
  `gateway-api/apis/v1` and the docs' "Add/update the Kubernetes Gateway API
  definitions"; the exact runtime error is not quoted upstream.
- **E9 class.** The guide never says "Kubernetes 1.22 is required"; the bound
  comes from the install page (Helm chart section, changed by PR #10637) and
  is consistent with the removal of `networking.k8s.io/v1beta1` and
  `apiextensions.k8s.io/v1beta1` (both "removed since Kubernetes v1.22").
  `upgrade-blocked` was chosen as the kind (an install outside the platform
  requirement), not `workload-failure`.

Not modelled (kept the item count at 10): Docker/Swarm provider split,
`tls.caOptional` removals, Consul/Nomad `namespace` removals,
Rancher v1 / Marathon / InfluxDB v1 removals, vendor tracing backends →
OpenTelemetry only, internal-resource observability (`addInternals`), gRPC
metrics status code, TCP `terminationDelay`, Headers middleware `ssl*`
options. None of them touches this fixture (no Docker, Consul, Nomad,
tracing or Headers-middleware configuration), so they would only add
not-affected links of the same shape as E5.

## Semantic labels

- **E1** — `gvk` family, one subject per kind of the removed group (the
  v2.11.2 CRD file defines nine), each `removed` with `replacedBy` the same
  kind in `traefik.io/v1alpha1`. Consequence `setting-ignored` rather than
  `resource-rejected`: the objects are not rejected by the API server (the
  old CRDs stay installed unless deleted); v3 simply stops reading them, so
  the routing they define stops being honoured. The exposure is the
  canonical `gvk-in-use` per subject, OR-ed.
- **E2** — subject `config-key` `core.defaultRuleSyntax` with
  `default-changed` `"v2"` → `"v3"`. `before` is the effective v2 behaviour
  (v2.11.2 has no such option and only parses v2 syntax). Consequence
  `resource-rejected` (Traefik rejects the router whose rule does not parse),
  not `behavior-change`: rules valid in both syntaxes are simply not exposed,
  which the exposure encodes with a `matches` pattern for the v2-only
  constructs the guide lists (several values in one matcher, `Headers` /
  `HeadersRegexp`, `HostHeader`). The overlap (shielded → informational) is
  `--core.defaultRuleSyntax=v2` or a route with `syntax: v2`. Caveat: inside
  the `resource` scope, `spec.routes[].match` and `spec.routes[].syntax` may
  bind different list elements of one IngressRoute; the fixture has one route
  per IngressRoute, so it does not matter here. The exposure reads the
  `cli-flag` spelling of the guide (`--core.defaultRuleSyntax`); Traefik flags
  are case-insensitive (`--core.defaultrulesyntax` in the cli-ref).
- **E3** — `crd-field` `traefik.io` Middleware `spec.ipWhiteList`,
  `deprecated` with `replacedBy` `spec.ipAllowList` (still honoured in v3.0.0
  code). Exposure: one Middleware with old set ∧ new unset.
- **E4 / E5** — `cli-flag` family, because this environment carries the
  static configuration as args; `removed`; `workload-failure` (Traefik does
  not start — "would prevent Traefik to start"). E5 bundles the two pilot
  options v2.11.2 parses (`pilot.token`, `pilot.dashboard`).
- **E6** — `product-relationship` (`name: gateway-api`), `requirement-changed`
  `~0.4.0` → `>=1.0.0`; canonical exposure `product-version out-of-range
  >=1.0.0`, AND-ed with the provider being enabled.
- **E7** — `cli-flag` `--providers.kubernetesgateway.experimentalchannel`,
  `now-required` with `after: 'true'` (JSON bool): previously TCPRoute/
  TLSRoute worked without it. `setting-ignored` because those route objects
  stop being honoured.
- **E8** — `rbac-permission` `serverstransporttcps` (group `traefik.io`),
  `now-required`, consequence `permission-lost`. The exposure is "CRD
  provider on ∧ no ClusterRole `traefik` lists `serverstransporttcps`"
  (`not` over a `resource` scope, so the false branch carries the examined
  ClusterRole as evidence).
- **E9** — `compatibility-boundary` `kubernetes`, `requirement-changed`
  `>=1.16.0` → `>=1.22.0`; canonical exposure `cluster-version out-of-range`.
- **E10** — `protocol-behavior` `content-type-auto-detection`,
  `default-changed` `"enabled"` → `"disabled"`; `behavior-change`. The
  undecided link's exposure is "no ContentType middleware ∧ undecidable
  (runtime)".
