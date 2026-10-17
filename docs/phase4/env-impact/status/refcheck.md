# Status — lane `refcheck` (Claude Opus 5.5, agent ri-refcheck)

Branch `p4ei/refcheck`, worktree `.claude/worktrees/p4ei-refcheck`. Brief: `briefs/refcheck.md`.
State: **IN PROGRESS** (2026-10-16). Resume from: this file + `git log` + DESIGN §2.6 / §3.1.

## Plan

1. `relations`: exported per-side reference resolution (`ResolveReferences(ctx, st, bundle, side, opts…)`):
   the same extraction and namespace semantics the graph builder uses, run on the FROM or the TO render of
   every complete environment pair plus the environment files; each reference resolved / dangling / unknown,
   with optional references (`optional: true`) flagged. Catalog field paths gain `[]` (every list element)
   so monitor endpoint secret refs can be declared.
2. `validate`: shared reference + CRD-schema checks (`refs.go`, `crdschema.go`); `RenderDiff` exposure ⇒
   `error` (never pass/fail by itself), `fail` only with a concrete failure (dangling reference, CR invalid
   against its rendered CRD schema, apiVersion not served); `StaticReference` evaluates references in the
   TO render vs FROM (new or regressed dangling ⇒ fail; pre-existing dangling ⇒ not counted; unknown target
   ⇒ not-run).
3. `products/prometheus-operator.yaml`: Secret/ConfigMap reference rules for ServiceMonitor / PodMonitor
   endpoints (same namespace). Check cert-manager / ingress-nginx rules for namespace semantics.
4. `classify`: `impact:not-joined` gap — decide legitimate vs defect.

## Done

- Step 1 (811bc872): `relations.NewViewer(st, opts…).View(ctx, bundle, SideFrom|SideTo)`; catalog
  `SplitListFieldPath` (`[]`), optional refs; graph output unchanged (all relations/app tests green).
- Step 2 (83458f93, 0c6b0e5d): `validate/refcheck.go` (cached views, FROM/TO comparison),
  `validate/crdschema.go`; RenderDiff exposure ⇒ error, concrete failure ⇒ fail; StaticReference TO-vs-FROM;
  `validate.WithRelations`, wired in `cmd/ri/investigate.go`.

## In progress / next step

- Step 3: prometheus-operator monitor Secret/ConfigMap rules (fetch v0.80.0 API types, quote verbatim).

## Decisions (with why)

## Files touched outside ownership

## Contract changes

- None.

## Open questions

## Tests
