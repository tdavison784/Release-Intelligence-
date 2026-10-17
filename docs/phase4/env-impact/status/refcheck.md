# Status — lane `refcheck` (Claude Opus 5.5, agent ri-refcheck)

Branch `p4ei/refcheck`, worktree `.claude/worktrees/p4ei-refcheck`. Brief: `briefs/refcheck.md`.
State: **DONE** (2026-10-16). Resume from: this file + `git log` + DESIGN §2.6 / §3.1 + `docs/INVESTIGATE.md`
"Validators" → "Reference and schema checks (exposure vs failure)".

## Done

1. **Per-side reference views** (`internal/investigate/relations/references.go`, 811bc872, dd79e476):
   `relations.NewViewer(st, opts…).View(ctx, bundle, SideFrom|SideTo)` reads the side's render of every complete
   environment pair plus the environment files and resolves every reference with the graph builder's extraction
   and namespace semantics (referrer namespace; cluster scope; explicit namespace fields / formats):
   `resolved` (object of the side, declared producer, `ServiceAccount default` / `ConfigMap kube-root-ca.crt`),
   `dangling`, `unknown` (name-only match, unknown namespace, convention-only producer, `optional: true` selector,
   image pull secret, a name the render redacted). A view has no nodes for rendered-change objects, so a reference
   to a removed object dangles. Graph output unchanged (the builder refactor is `build(ctx, b, side, view)`).
2. **RenderDiff: exposure ⇒ inconclusive** (83458f93, 27122101): a strongly correlated effect reaching the
   rendered deployment ⇒ `error` ("exposure: <change> reaches <object>" + what the concrete checks found), whatever
   the item's breaking flag; `fail` only on a concrete failure in TO that FROM did not have: a dangling reference of
   or to the changed object, a custom resource invalid against its rendered CRD schema (unknown field, type, enum,
   required), an apiVersion the rendered CRD no longer defines/serves. Text-mention ⇒ not-run, pass paths unchanged.
3. **StaticReference: TO vs FROM** (0c6b0e5d, 27122101): reference of/to an object the item's rendered changes
   touch that is regressed or new-and-dangling ⇒ `fail` naming referrer, field path, target kind/namespace/name;
   pre-existing dangles never fail; a dangle caused by no change of this item, or an undecidable reference, on a
   path ⇒ that path is not passed (`not-run`, reason stated). Graph-path checks kept (incl. their own
   provider-removed fail). References are compared **by target**, not by field/list position (see decisions).
4. **Catalog** (dd79e476): prometheus-operator — 18 declared references for ServiceMonitor `spec.endpoints[]` and
   PodMonitor `spec.podMetricsEndpoints[]` (authorization.credentials, bearerTokenSecret, basicAuth
   username/password, tlsConfig ca/cert Secret|ConfigMap, keySecret), same namespace as the monitor; each cites the
   v0.80.0 API type doc and the `resource_selector.go` line that reads it with the monitor's namespace and rejects
   the monitor (quotes verified verbatim against the fetched files). `[]` list paths
   (`catalog.SplitListFieldPath`, relationship `path` only). ingress-nginx `auth-secret` ⇒ format
   `[namespace/]name` (upstream `cache.SplitMetaNamespaceKey`, default the Ingress namespace); `auth-tls-secret`,
   `proxy-ssl-secret` keep `namespace/name` (upstream `k8s.ParseNameNS` requires it); cert-manager rules checked —
   correct (Certificate/Issuer namespaced, ClusterIssuer CA secret name-only ⇒ unknown, ClusterIssuer cluster scope).
5. **Classify `impact:not-joined`** (8ca46ced): **a defect, fixed.** The join emits not-joined
   (`release-knowledge-gap`) for every change it does not evaluate (every declared change, diff rules without a
   join rule, subject-less changes); as an R2 blocker it made every declared item uncleareable even with R2.1–R2.8
   met (eval: two observed passes on "Metrics: Disable by default" in shop-fr/shop-nl). It now never blocks R2 by
   itself and stays listed (gap + next step) on every finding that is not cleared; every other unknown join rule
   still blocks.
6. Wiring: `cmd/ri/investigate.go` passes `validate.WithRelations(relations.WithCatalog(a.Catalog))` to RenderDiff and
   StaticReference. Docs: `docs/INVESTIGATE.md` (Validators table/options, RenderDiff, StaticReference, new
   "Reference and schema checks (exposure vs failure)" section, Relationships formats/list paths/shipped rules).

## Real-bundle diagnostic (opt-in groundtruth test, inputs only, not scored)

`RI_VALIDATE_STATE=<warm .ri> go test ./internal/investigate/validate -run Groundtruth -v` (now passes the catalog
relationships). Render-diff / static-reference outcomes:

| case | render-diff | static-reference |
|---|---|---|
| payments-eu / -us | 8 error, 7 not-run, 1 pass | 16 not-run |
| shop-de / shop-fr | 13 pass, 14 not-run | 1 pass, 26 not-run |
| shop-nl | 12 pass, 1 error, 14 not-run | 2 pass, 25 not-run |
| shop-pl | 27 not-run (incomplete render) | 27 not-run |
| obs-north | **3 fail**, 2 error, 17 pass, 1 not-run | **3 fail**, 20 not-run |
| obs-south | **0 fail**, 5 error, 17 pass, 1 not-run | 23 not-run |

obs-north's 3 fails: ServiceMonitors `default/…-apiserver`, `kube-system/…-coredns`, `kube-system/…-kubelet` whose
new `spec.endpoints[0].authorization.credentials` names Secret `…-prometheus-token` that nothing provides in their
namespace (the same three the runtime lane observed the operator reject). obs-south's monitors live in
`monitoring`, where the environment provides it ⇒ exposure only (error). The previous obs-south false positive came
from RenderDiff failing on exposure; it is gone here. Eval should re-score A and C (commander's call).

## Decisions (with why)

- **Exposure ⇒ `error`, not `not-run`** (brief; DESIGN R3 lists "an inconclusive (error) validation on an exposed
  subject"). The breaking flag no longer decides RenderDiff's outcome: same change, same outcome unless the
  consequences differ.
- **Concrete failures need a strong correlation** (subject/values-default/artifact/release-anchor or the anchoring
  change): a text mention does not make the failure the item's.
- **Compare by target, not by field path.** First real run: a removed volume shifted `webhook-cert` from
  `volumes[1]` to `volumes[0]`, and a path-keyed identity made the hook-created admission Secret look "new"
  (false shop-nl fail). Judging by the target (kind/ns/name) across all FROM references fixes reordering and new
  referrers of a target the environment already provides outside the snapshot.
- **New dangling reference ⇒ fail** (brief), but only when *no* FROM reference names the target; targets created
  at runtime that FROM already relied on are pre-existing. Risk kept: a *new* reference to something only a
  runtime actor creates (no declared `produces`) would fail — declare a `produces` rule when one is found.
- **Optional refs, image pull secrets, redacted names, unknown namespaces ⇒ unknown** (never dangling, never
  resolved): fail-closed both ways.
- **CRD schema**: only `apiextensions.k8s.io/v1` CRDs in the same side's view; violations new at TO only; unknown
  fields count (strict field validation rejects them; otherwise the setting is silently pruned). Not modelled:
  CEL rules, patterns/min/max, properties only under anyOf/oneOf/allOf (never invented as violations).
- **Graph untouched** by the cluster-provided / optional / redaction refinements (they apply to views only) to keep
  graph ids and other lanes' outputs stable. The graph still has `<sensitive …>` placeholders for rendered ingress
  annotations (unresolved ⇒ fail closed) — noted, not changed.
- `views` cache per validator instance keyed by bundle id (one bundle per run; Validate called per item).

## Files touched outside ownership

- `internal/catalog/relationships.go` (+test): additive `SplitListFieldPath`, `ListElements`,
  `RelFormatOptionalNamespace`; `schemas/product-definition.schema.json`: format enum + `[namespace/]name`.
- `products/ingress-nginx.yaml` (auth-secret format + 2 citations), `products/prometheus-operator.yaml` (18 rules) —
  the brief grants `relationships:` entries.
- `internal/investigate/classify/policy.go` (+`decision_test.go`): the not-joined point only (brief).
- `cmd/ri/investigate.go`: two `WithRelations(...)` options (wiring).
- `docs/INVESTIGATE.md`.

## Contract changes

- None to `internal/domain` / `api.go`. DESIGN text the commander may want to amend: §3.5 table row `unknown` —
  "gap signal; `impact:not-joined` (the join does not evaluate the change) does not block R2 by itself" — and §2.6
  RenderDiff wording (exposure ⇒ error; fail only with a named concrete failure).

## Open questions / notes for the commander

1. Eval: re-score arms A and C after merge (obs-south FP expected gone; obs-north B1 affected now deterministic via
   dangling references; not-joined behaviour items with observed passes can clear).
2. cert-manager Certificate `issuerRef` without `kind` (default Issuer) is not followed by any rule (no false dangle,
   simply unchecked). A declared rule would need a "when absent or equals" condition — small catalog follow-up.
3. ingress-nginx cross-namespace Secret annotations are denied unless `allow-cross-namespace-resources`; that is a
   controller setting, not reference resolution — not modelled.

## Tests

`go build ./... && go vet ./... && go test ./...` green (all packages). New tests: relations
`references_test.go` (same-namespace, env-provided, unknown targets, declared list/namespace/cluster/producer
rules, removed-object dangling, determinism), catalog `TestSplitListFieldPath`, relations
`TestCatalogMonitorSecretReferences` / `TestCatalogIngressNginxSecretFormats` / `TestCatalogRedactedNameIsUnknown`;
validate `concrete_test.go` (removed referenced object, new dangling ref tells environments apart incl.
other-namespace, pre-existing + optional, CR invalid against rendered CRD ×6, apiVersion no longer served ×2,
declared monitor reference, reordered pre-existing) and `reference_test.go` (namespaces, env-provided, graph
coverage, unknown target ⇒ not-run, pre-existing / unattributable, explicit namespace + cluster scope);
renderdiff exposure tests; classify `TestNotJoinedJoinIsNoR2Blocker`. Real-bundle diagnostic green (8 cases).
Cleanup: no kind clusters, servers, `bin/` or scratch output in the worktree; upstream downloads were in the
session scratchpad only.
