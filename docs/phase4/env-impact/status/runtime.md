# Status — lane `runtime` (Claude Opus 5.5, agent ri-runtime)

Branch `p4ei/runtime`, worktree `.claude/worktrees/p4ei-runtime`. Brief: `briefs/runtime.md` (+ `_wave2-common.md`,
`_wave1-common.md`, DESIGN §2.6 / §3.1). State: **IN PROGRESS** (2026-10-16).
Resume from: this file + brief + `git log`.

## Done

- `exercises:` construct — `internal/catalog/exercises.go` (+ tests), `schemas/product-exercises.schema.json`,
  declarations `products/exercises/{cert-manager,ingress-nginx,prometheus-operator}.yaml` (every quote checked
  verbatim against the pinned upstream URL on 2026-10-16).

- `internal/investigate/runtime/` (validator `runtime.isolated-upgrade@v1`): `validator.go` (options, node-image
  pinning, run cache per bundle), `session.go` (renders → preflight → kind cluster → FROM / exercises / TO /
  exercises → degradations → cleanup), `observe.go` (generic observations + diff), `exercise.go`, `outcome.go`
  (per-item rules), `renders.go` (raw re-render verified against the stored artifact), `apprender.go`
  (`NewAppRenderer(app)`), `kind.go` (`KindDriver`, kubectl), `driver.go` (ports), `model.go`, `disk_*.go`.
- Unit tests with a fake driver + in-memory cluster (fictional product `gizmo`): every outcome path, quiet window
  without exercise ⇒ never pass, cleanup on every exit path (create fails, no baseline, timeout, cancellation,
  delete fails, delete unverified), observation diffing, render verification, prune/hooks, determinism.
- `docs/INVESTIGATE.md` "Runtime validation".
- Real run 1: cert-manager payments-eu (see "Real runs").

## In progress / next step

- Real runs ingress-nginx shop-de + kube-prometheus-stack obs-north (running), then status table, LANE DONE.

## Decisions (with why)

- **Exercises live in `products/exercises/<product>.yaml` (kind `ProductExercises`), not inside
  `products/<product>.yaml`.** A product definition's raw digest pins its ingested releases and upgrade edges
  (`store.LoadCurrentRelease`, `UpgradeEdge.DefinitionDigest`); editing it would invalidate the shared warm `.ri`
  store other lanes use and regenerate e2e goldens, for a validation-time trigger that says nothing about a
  release. Same catalog package, same citation rules (https upstream docs, verbatim quote ≤400 chars, never
  eval data), its own schema; `catalog.LoadDir` skips the subdirectory.
- **Exercise families**: an item counts as exercised only when its family is listed and a target object lies on
  its impact paths (cert-manager renewal: behavior/procedure/api-schema; ingress / prometheus-operator updates:
  behavior/procedure).

## Files touched outside ownership

- none yet

## Contract changes

- none yet

## Open questions

- none yet

## Test status

- `go test ./internal/catalog` green.
