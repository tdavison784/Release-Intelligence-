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
render never clears it. A release-scope rendered change that no change mentions and the default tier
policy blocks (removed RBAC permission, removed resource, API-version or CRD storage/served move)
becomes its own in-scope item (`scope:rendered-structural`).

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

(lane renderdiff)

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
