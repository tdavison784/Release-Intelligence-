# Environment-aware investigation (`ri investigate`, Phase 4)

Design: [docs/phase4/env-impact/DESIGN.md](phase4/env-impact/DESIGN.md). Each section below is owned by
one lane; edit only your own section.

## Evidence bundle

(lane bundle)

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
`spec.secretName` produces-convention are `inferred`. A `ProducesRule` (declarative data: producing group/kind, the
field naming the produced object, its kind) makes the produces edge and the reference through it `established`;
rules are passed with `relations.WithProducesRules` and are meant to come from the product catalog (not wired yet).
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

Limits (milestone 1): no ServiceMonitor/Gateway-API/NetworkPolicy selectors, no live observation, no values-file
`configures` for Helm values outside Argo `$values/` references, scope of unknown custom kinds assumed namespaced.
