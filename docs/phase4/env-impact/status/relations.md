# Status — lane `relations` (Claude Sonnet 5.5, agent ri-relations)

Branch `p4ei/relations`, worktree `.claude/worktrees/p4ei-relations`. Brief: `briefs/relations.md`.

## Done
- `internal/investigate/relations/`: `NewBuilder(st, ...Option) GraphBuilder`, `NewExposure() ExposureMapper`,
  `NewTraverser() Traverser` (`graph.dfs@v1`), `WithProducesRules` / `ProducesRule` (declarative).
- Builder tests incl. every `eval/investigations/*/env` (inputs only) building a graph that `Validate`s.

- Exposure tests (rendered changes first, join-match subjects, out-of-scope items ignored) and traverser tests
  (maximal simple paths, cycles, unresolved edges, depth/paths/nodes bounds + reasons, mitigations, per-item
  completeness, input-order determinism, brute-force "every simple-path-enterable edge is on a path").
- End-to-end on the cert-manager groundtruth env: Certificate -> Secret -> Deployment is an *inferred* path, so the
  traversal is incomplete (fail closed). `docs/INVESTIGATE.md` "Relationships" written.

## In progress
- Nothing. Lane complete.

## Decisions
- Objects are read straight from the bundle's artifacts (TO renders of complete env pairs + `environment-file`
  YAML), not from `env.Resources`: rendered objects have no `env` equivalent, and one parser serves both.
- Kustomize `namespace:` (outermost wins) and Argo `destination.namespace` give file objects their namespace so
  references resolve; kustomize patch files are not objects; `configMapGenerator`/`secretGenerator` create nodes.
- Reference to an object absent from the snapshot ⇒ placeholder node + `missing` edge (reason stated). If a custom
  resource names it via a declared `ProducesRule` the `produces` edge and the reference are `established`; via the
  generic `spec.secretName` convention they are `inferred` (unresolved ⇒ blocks R2, never clears).
- Name-only matches (namespace unknown on either side) are `inferred`, never `established`.
- The catalog `relationships:` construct was added in the follow-up (see below).

## Files touched outside ownership
- `docs/INVESTIGATE.md` ("Relationships" section only, as the skeleton directs).

## Contract changes
- None.

## Follow-up (commander review: catalog construct) — done
- `relationships:` in product definitions: `internal/catalog/relationships.go` (types, `SplitFieldPath`, validator),
  `ProductDefinition.Relationships`, `schemas/product-definition.schema.json`, tests (valid, 19 bad-rule cases,
  schema). Kinds: produces / references / selects (+ `scope`, `whenPath`/`whenEquals`, `format`, namespaceSelector).
- `relations.WithCatalog` / `WithRelationships` / (compat) `WithProducesRules`; label selectors evaluate
  matchLabels + matchExpressions; cluster scope known from built-ins, declared rules and snapshot CRDs.
- Rules authored from upstream API source docs (quotes verified against the fetched files): cert-manager
  (certificate-secret, issuer-ca-secret, clusterissuer-ca-secret, certificate-cluster-issuer), ingress-nginx
  (auth-secret, auth-tls-secret, proxy-ssl-secret), prometheus-operator (servicemonitor-services, podmonitor-pods).
  ServiceMonitor/PodMonitor also apply to kube-prometheus-stack envs (catalog-wide). No eval files were opened.
- Test: with the catalog, Certificate → Secret → Deployment is established and the traversal is complete.
- Fixes found on the way: ClusterIssuer-style kinds were mis-scoped by Application destination namespace (now via
  declared cluster scope); `*Ref` without `group` no longer fails to resolve; placeholder status no longer overwritten.
- Golden files in internal/app/testdata/e2e changed ONLY in `definitionDigest` (cert-manager.yaml edited): regenerated
  with `-update`; diff verified to be digest lines only.
- A ClusterIssuer reading its CA Secret stays `inferred` on purpose (cluster resource namespace not stated).

## Open questions (resolved)
- (answered: yes, done) Should `ProducesRule` come from a product-definition `relationships:` block (additive catalog + schema change)?
  Without it cert-manager's Certificate→Secret edge is `inferred` (fail closed), which is the safe default.

## Notes for integration
- DESIGN §2.5 says every reachable edge lies on some emitted path; precisely: every edge a *simple* path from a
  root can take (an edge that would re-enter a node already on the path is on none). Documented in INVESTIGATE.md.
- Rendered-change objects get nodes even when absent from the TO render (removed objects), so they can be roots.
- Wiring (run package): `relations.NewBuilder(store)`, then `NewExposure().Exposure(...)`, then
  `NewTraverser().Traverse(..., cfg.Bound)`. The builder needs the ArtifactStore to read renders/env files.

## Test status
- `go build ./... && go vet ./... && go test ./...` green; `go test -race ./internal/investigate/relations` green.
- No clusters, servers or large outputs were created; nothing to clean.
