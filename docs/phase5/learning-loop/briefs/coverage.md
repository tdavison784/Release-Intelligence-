# Lane `coverage` — the two non-pin blockers on a behaviour item that a valid pin should not leave behind — Opus

Branch `p5ll/coverage`, worktree `.claude/worktrees/p5ll-coverage`. Read `_common.md`, DESIGN §2.5–§2.6 and §3
(policy), `internal/investigate/classify/policy.go`. You own `internal/investigate/classify`,
`internal/investigate/relations`, `internal/investigate/validate/renderdiff.go` (+ tests), and the
`relationships:`/`reconciles:` rules in `products/*.yaml`.

## Problem (from the Phase 5 gap analysis; generic, not per product)

1. **Inferred edges whose namespace is knowable.** A cluster-scoped issuer-like object referencing a Secret by
   name gets an *inferred* edge because "the Secret's namespace is unknown" (`relations/edges.go:294-296`). Many
   controllers document where such cluster-scoped references resolve (a controller flag with a documented
   default, e.g. a "cluster resource namespace"). Today this blocks R2.3 forever.
2. **Correlated documentation-only rendered changes.** A behaviour item whose only correlated rendered change is a
   CRD *description* text change cannot reach R2.2 coverage: an override-inspection pass carries no
   `RenderedChanges` (`override.go:131`) and render-diff refuses behaviour items (`renderdiff.go:196`).

## Build

1. A declarative way for a relationship rule to state where a cluster-scoped name reference resolves (a
   documented controller argument and its documented default, with a citation), resolved from the environment's
   effective controller configuration; when the argument is set through something the snapshot cannot read, or
   the rule has no citation, the edge stays inferred (fail-closed).
2. A render-diff validation result that covers a correlated rendered change **only** when that change is a
   description/documentation-only CRD change (no schema, default, enum, required, served/storage change — reuse
   the existing classification of rendered changes), so R2.2 can count it as covered. It never counts as a
   behaviour clearance: R2.4 (impact paths) still requires `ClearsBehaviour()`. Policy version bump; tests for
   "description + schema change in the same CRD ⇒ not covered".
3. Policy invariants re-tested: no new path to `not-affected` without a clearing pass; agent output never an input.

## Done

Per `_common.md`; status shows a before/after policy trace for a synthetic behaviour item.
