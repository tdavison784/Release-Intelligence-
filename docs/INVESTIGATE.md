# Environment-aware investigation (`ri investigate`, Phase 4)

Design: [docs/phase4/env-impact/DESIGN.md](phase4/env-impact/DESIGN.md). Each section below is owned by
one lane; edit only your own section.

## Evidence bundle

An investigation starts from an **evidence bundle**: an immutable, content-addressed record of one
upgrade (`product from → to`) against one environment. Code: `internal/investigate/bundle` (collectors,
assembler) and `internal/investigate/store` (stores). Contract: DESIGN.md §2.

| Piece | Built by | What it holds |
|---|---|---|
| `ChangeEvidenceRecord` | `bundle.NewChangeCollector(app, store)` | the `UpgradeEdge` (one `upgrade-edge` artifact), a `ChangeItem` per non-routine change and per moved compatibility constraint, the upstream documents, the release-scope (chart-default) render, and a status for **every** change source |
| `EnvironmentSnapshot` | `bundle.NewSnapshotBuilder(store)` | every environment file (secrets redacted), the parsed `env.Environment` (`environment-facts`), a status per input, the repo pinned by commit, dirty flag and a tree digest |
| `EvidenceBundle` | `bundle.NewAssembler(store)` | the two above plus the environment renders (from the `renderdiff` lane), the manifest of all artifacts and the evidence pool; `Validate()` resolves every id |

**Change sources.** Each of `release-notes`, `migration-guide`, `source-diff`, `api-schema`, `chart`,
`kustomize`, `rendered-manifests`, `compatibility`, `advisories` is stated exactly once:
`ok` (with the collected artifacts), `partial`, `unavailable` or `not-applicable`, always with a reason
unless `ok`. Upstream documents are fetched through the fetch cache (a GitHub blob URL is read as its raw
content) and compared with the digest recorded when they were ingested; a document that changed since is
`partial`. A product with no upgrade guide (ingress-nginx) is `unavailable`, one that publishes no CRD
bundle is `not-applicable`; source diffs and release-side Kustomize are not collected in milestone 1
and say so.

**Scope and manifestation.** Every item carries `InScope` + `ScopeRule` (DESIGN §1.3) and
`Manifestation` + `ManifestationRule` (§2.2.1), decided by declared rules over the upgrade edge
(`internal/investigate/bundle/scope.go`), never by product code. A behaviour default compiled into a
controller (`manifest:declared-behaviour`, `manifest:crd-default`) is not render-decisive: a silent
render never clears it. Every release-scope rendered change that no change explains becomes its own
item, anchored on it (DESIGN §1.3, first match wins): a class the default tier policy blocks (removed
RBAC permission, removed resource, API-version or CRD storage/served move) is in scope
(`scope:rendered-structural`); the version bump of an image the product definition itself publishes
(`render.SelfArtifactBump`: same repository, exactly the edge artifact's from/to tags) is out of scope
(`scope:routine-self-artifact`) — so it is correlated, never "uncorrelated"; anything else is in scope
(`scope:rendered-unexplained`, manifestation `rendered-config`).

**Layout** (`<state>/investigations/`, `store.New(<state>)`):

```
cas/sha256/<hex[0:2]>/<hex>   artifact bytes — immutable, written once, digest verified on every read
bundles/<bnd-id>.json         EvidenceBundle — immutable (re-writing the same bundle is a no-op)
runs/<inv-id>.json            Investigation — rewritten as it progresses; its audit trail may only grow
```

Writes are atomic (temp file + rename). Artifact paths are logical: `upstream/<product>/<host>/<path>`,
`env/<path inside the repo>` (explicit inputs: `env/inputs/<file>`), `derived/<stage>/…`.

**Redaction** happens before hashing: Kubernetes `Secret` `data`/`stringData` values and values whose key
names a credential (`password`, `token`, `apiKey`, `privateKey`, `credentials`, …) become
`"<redacted sha256:…>"` (keys kept); the artifact is marked `Redacted`. Credential values are also
withheld from the facts artifact and from evidence excerpts. Files that are not YAML/JSON are stored
verbatim.

**Reproducibility.** The bundle id is derived from content only. Wall-clock fields (`generatedAt`,
`retrievedAt`) are zeroed in the stored edge copy and `CreatedAt` is not part of the id, so two runs over
the same upstream evidence, repository state and renders yield the same bundle id; a changed file, a
different commit or a dirty tree yields a different one.

**Known gaps (milestone 1).** Live-cluster observation is not implemented (a `live` request records an
`unavailable` input). Routine changes are summarised in the edge, not itemised. Render rules that name
an artifact version bump (rule `artifact`) carry no change id; the collector attributes them to the
computed image items of that repository.

## Render diffs

The centerpiece evidence: the product's from and to releases rendered with the environment's own
configuration, diffed, and indexed so every rendered change is a citable record
(`internal/investigate/renderdiff`, DESIGN.md §2.4). `renderdiff.New(app, store)` implements the
`EnvironmentRenderer` port over `internal/render` (target detection, helm/kustomize renders, the semantic
diff, changelog correlation, chart coverage).

**One pair per render target.** Every deployment of the product the snapshot's repository (or explicit
values files) declares becomes a `ManifestPair` named by the target, with paths repo-relative so ids do
not depend on where the repository is checked out: `argocd:clusters/prod/app.yaml#0/$.spec.sources[0]`,
`flux:…`, `helmfile:…`, `values:<file>`, `kustomize:<dir>`. A repo-discovered values file that an Argo
CD / Flux / Helmfile target already layers is not rendered on its own (`render.DetectTargets`: that
configuration is not deployed anywhere); a note says so.

**Status** (what decides whether an absence of change is evidence):

| Status | When | Changes indexed |
|---|---|---|
| `ok` | both renders succeeded, values complete, and the chart (from or to, or a declared dependency) defines every top-level values key the customer sets | yes — `[]` means "rendered, nothing changed" |
| `partial` | `values-incomplete: …` (valuesFrom / value files outside the repo), `chart-coverage: …` (customer keys the chart never reads), or one side rendered and the other failed (e.g. `invalid-values: the target chart rejects…`) | **no** |
| `unavailable` | neither side rendered (`chart-unavailable`, `renderer-unavailable`, `template-error`, …), or a Kustomize overlay runs the product's images without pinning their version | **no** |
| `not-applicable` | `version-independent:` a Kustomize overlay (a consumer app) that neither pins nor runs the product: the upgrade does not change what it renders; its `from` render is kept for inspection | no |

An incomplete pair is an evidence gap, never "no change". `RenderResult.Notes` records what target
detection skipped, so "no pairs" is never silent.

**Artifacts** (`derived/render/env/<target-slug>/`): `from.yaml` / `to.yaml` are the *normalized* objects
the diff compared, one document per object headed by `# Object: <group>/<version>/<kind>/<ns>/<name>`
(Secret data and credential-named values are `<sensitive sha256:…>` digest markers; the artifact is then
`redacted`); `diff.json` is the full diff (`ri.dev/render-diff/v1alpha1`: provenance of both renders
without timestamps, every change with its id, correlation links and change items; `indexed: false` for an
incomplete pair).

**Rendered changes.** One `RenderedChange` per `render.Change` of an ok pair, id
`RenderedChangeID(environment, target, object, path, class)`. The path is `render.RenderedPath`, shared
by both render scopes: container args and RBAC rules are sets addressed by one path, so their changes
carry a member selector — `…args[=--flag]`, `rules[=apps/deployments:get]`. Each cites one `rendered-diff` evidence record whose `contentDigest` is
the diff artifact's digest and whose locator is `changes[id=<rc>]`, so it resolves to the bytes.
Sensitive changes record no values.

**Correlation** to change items (`Correlation` = the rules, comma-joined):

- a `render.Correlate` link to an edge change → that change's item (links to routine changes, which have
  no item, are dropped);
- an `artifact` link (no change id) → the computed image items (`manifest:computed-image`) whose title or
  subjects name the repository — the collector's release-scope rule;
- `release-anchor` → the item anchored on the matching release-scope rendered change (same class, API
  group, kind, object name and path with the release name normalized);
- otherwise `rendered-anchor` → the change's **own environment item** (`RenderResult.Items` →
  `EvidenceBundle.EnvironmentItems`, anchored on the environment-scope rendered change):
  `scope:routine-self-artifact` (out of scope) for a self-artifact version bump, else
  `scope:rendered-unexplained` (in scope, so it gets a finding: the deployment changes and nothing
  upstream explains it — exactly what the investigation must look at).

`EvidenceBundle.Items()` / `Item()` / `InScope()` cover record and environment items alike. With every
change anchored, `Uncorrelated()` is empty for a renderdiff-built bundle; it still blocks automated
approval when non-empty (DESIGN §3.4).

**Queries** (what the agent's diff tools call): `renderdiff.QueryPage(bundle, DiffQuery)` filters by
scope (default environment), target, change item, uncorrelated, classes, kinds, namespaces (`-` =
cluster-scoped) and path prefix; results are ordered by (target, object, path, class), at most 200 per
page (default 50, ≤256 KiB), with `Next` → `DiffQuery.After` to continue. Unknown targets, items,
classes, scopes or cursors are rejected (`ErrRejected`), never answered with an empty page.
`renderdiff.Object` reads one object or field (`spec.template.spec.containers[name=x].args[=--flag]`,
`metadata.annotations["a/b"]`, `ports[port=443/TCP]`) from either side, as YAML whose text occurs in the
stored render (cite it with `read_artifact`-style quotes); an absent object or path is `found: false`; a
side that was not rendered is `ErrNotFound` with the pair's reason; objects over 64 KiB must be read by
field. `renderdiff.Summaries` lists every pair with counts by class and uncorrelated.

Coverage on the groundtruth environments (8 cases) is recorded in
`docs/phase4/env-impact/status/renderdiff.md`; reproduce with
`RI_RENDERDIFF_STATE=<warm .ri> go test ./internal/investigate/renderdiff -run Groundtruth -v`.

## Validators

(lane validate)

## Relationships

Secondary supporting context (the rendered diff is the centerpiece). Package `internal/investigate/relations`
answers one question for the agent and the policy: **which consumers reference this changed object, and how well
do we know it?** Design: DESIGN.md §2.5.

**Graph** (`relations.NewBuilder(store, opts...)`, producer `relations.builder@v1`). Nodes come from the TO
render of every complete environment pair, the environment's YAML files, and every object an environment rendered
change names (so a removed object can still be a root). Edges point in the direction impact propagates (provider →
consumer):

| Edge | From → to | Source |
|---|---|---|
| `references` | Secret / ConfigMap / ServiceAccount → workload | pod template volumes (incl. projected), `env`/`envFrom`, `serviceAccountName`, `imagePullSecrets` |
| `references` | Service / Secret / IngressClass → Ingress | backends, `defaultBackend`, `tls[].secretName`, `ingressClassName` / class annotation |
| `references` | referenced object → custom resource | generic `*Ref` fields `{name, kind[, group, namespace]}`; Flux `valuesFrom` |
| `selects` | workload → Service | `spec.selector` ⊆ pod template labels |
| `serves-api` / `consumes-api` | CRD → api-type → custom resource | CRD `spec.versions`, object `apiVersion`/`kind` |
| `produces` | custom resource → object it creates | declarative `ProducesRule`, else the `spec.secretName` convention |
| `renders-to` / `deploys` / `configures` | Application, kustomization, render source → objects; values file → Application | Argo `source.path`, `$values/…` value files, kustomize `resources`, generators, render targets |

Kustomize `namespace:` (outermost wins) and an Application's `destination.namespace` are applied so references
resolve; kustomize patch files are not objects; `configMapGenerator`/`secretGenerator` outputs are nodes.

**Status = how well a relationship is known.** `established` is an explicit name reference or resolved selector in
the snapshot. A reference to something the snapshot does not contain becomes a **placeholder node** and a `missing`
edge (reason stated) — never dropped. A name-only match (a namespace unknown on either side) and the generic
`spec.secretName` produces-convention are `inferred`. A declared `produces` relationship (below) makes the produces edge and the reference through it `established`, so a
path Certificate → Secret → Deployment can be complete.
Unresolved (inferred/missing) edges can only add paths that need validation: a path through one leaves the
traversal incomplete, which blocks `not-affected` (fail closed).

**Exposure** (`relations.NewExposure()`): the roots of an in-scope change item are the graph nodes of the
environment rendered changes correlated to it (rule `exposure:rendered-change`), then the nodes the deterministic
join's matches name (`exposure:join-match`; a subject is a node key, `Kind/name` or `namespace/Kind/name`). One
root per (item, node); an item with no signal has no root.

**Traversal** (`relations.NewTraverser()`, `graph.dfs@v1`): roots sorted by (item, node id); outgoing edges by
(kind, to-node id, edge id); simple paths only; a maximal path is emitted when no outgoing edge leads off the
path, so every edge that some simple path from a root can take lies on an emitted path (an edge that would
re-enter a node already on the path is on none). Bound `{MaxDepth 8, MaxPaths 500, MaxNodes 5000}` by default;
hitting any bound sets `Truncated` and a reason (`max depth N reached`, `max paths N reached`, `max nodes N
reached`) and the traversal is not complete. Path status is the weakest edge status; `Unresolved` lists exactly the
non-established edges on paths. Same graph, roots and bound ⇒ byte-identical result (input order does not matter).

**Declared relationships** (`relationships:` in a product definition, `internal/catalog/relationships.go`,
validated by `ri catalog validate`/`catalog.Validate`, schema `schemas/product-definition.schema.json`). Object
kinds relate in ways field semantics cannot show, so the catalog states them, each citing upstream API/CRD
documentation with a verbatim quote (never an environment or an evaluation case):

```yaml
relationships:
- id: certificate-secret
  kind: produces            # produces | references | selects
  from: {group: cert-manager.io, kind: Certificate}
  to: {kind: Secret}
  path: spec.secretName     # dotted keys; a key with dots/slashes in brackets: metadata.annotations[a.b/c]
  format: name              # or namespace/name (default name = namespace of `from`)
  scope: namespaced         # or cluster (scope of `to`)
  whenPath: spec.issuerRef.kind   # optional condition on `from`; with whenEquals
  summary: ...
  references: [{url: https://…, quote: "verbatim, ≤400 chars"}]
- id: servicemonitor-services
  kind: selects             # to: Service (metadata.labels) or Pod (pod-template labels of workloads)
  from: {group: monitoring.coreos.com, kind: ServiceMonitor}
  to: {kind: Service}
  selectorPath: spec.selector                 # a LabelSelector: matchLabels + matchExpressions
  namespaceSelectorPath: spec.namespaceSelector   # {any, matchNames}; default: the selector's own namespace
```

`produces` makes the edge from → to (status `established`; a missing `to` object becomes a placeholder explained by
its producer); `references` links `to` → `from`; `selects` links the selected object → the selector. A field that
does not hold the documented form (e.g. no `namespace/name`) yields no edge. `relations.NewBuilder(store,
relations.WithCatalog(cat))` applies the relationships of every product in the catalog (rules of products the
environment does not use match no object); `WithRelationships(product, rs...)` and `WithProducesRules` add rules
directly. Edges carry the rule id `relationship:<product>/<id>`. Shipped: cert-manager (Certificate secret,
Issuer/ClusterIssuer CA secrets, `issuerRef` to a ClusterIssuer), ingress-nginx (`auth-secret`,
`auth-tls-secret`, `proxy-ssl-secret` annotations), prometheus-operator (ServiceMonitor, PodMonitor). A ClusterIssuer
reading its CA Secret stays `inferred`: the issuer does not state the cluster resource namespace.

Limits (milestone 1): no NetworkPolicy/Gateway-API selectors, no live observation, no values-file `configures` for
Helm values outside Argo `$values/` references; cluster scope of a kind is known only for built-in kinds, declared
`scope: cluster` and CRDs in the snapshot.
