# Status — lane `relations` (Claude Sonnet 5.5, agent ri-relations)

Branch `p4ei/relations`, worktree `.claude/worktrees/p4ei-relations`. Brief: `briefs/relations.md`.

## Done
- `internal/investigate/relations/`: `NewBuilder(st, ...Option) GraphBuilder`, `NewExposure() ExposureMapper`,
  `NewTraverser() Traverser` (`graph.dfs@v1`), `WithProducesRules` / `ProducesRule` (declarative).
- Builder tests incl. every `eval/investigations/*/env` (inputs only) building a graph that `Validate`s.

## In progress
- Exposure + traverser tests (determinism, bound/truncation, unresolved edges), then `docs/INVESTIGATE.md` "Relationships".

## Decisions
- Objects are read straight from the bundle's artifacts (TO renders of complete env pairs + `environment-file`
  YAML), not from `env.Resources`: rendered objects have no `env` equivalent, and one parser serves both.
- Kustomize `namespace:` (outermost wins) and Argo `destination.namespace` give file objects their namespace so
  references resolve; kustomize patch files are not objects; `configMapGenerator`/`secretGenerator` create nodes.
- Reference to an object absent from the snapshot ⇒ placeholder node + `missing` edge (reason stated). If a custom
  resource names it via a declared `ProducesRule` the `produces` edge and the reference are `established`; via the
  generic `spec.secretName` convention they are `inferred` (unresolved ⇒ blocks R2, never clears).
- Name-only matches (namespace unknown on either side) are `inferred`, never `established`.
- No catalog `relationships:` construct yet: `ProducesRule` is the declarative data type; wiring it to product
  definitions is deferred (see Open questions) — the builder takes rules by option.

## Files touched outside ownership
- `docs/INVESTIGATE.md` ("Relationships" section only, as the skeleton directs).

## Contract changes
- None.

## Open questions
- Should `ProducesRule` come from a product-definition `relationships:` block (additive catalog + schema change)?
  Without it cert-manager's Certificate→Secret edge is `inferred` (fail closed), which is the safe default.

## Test status
- see last commit.
