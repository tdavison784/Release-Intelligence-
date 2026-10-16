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

- Real runs re-started 2026-10-16 by GLM continuation (see "GLM handoff log"): all three products sequentially
  with the current code, then the "Real runs" table below, cleanup, LANE DONE.

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
- `go build ./... && go vet ./...` green; `go test ./internal/investigate/runtime` green (GLM continuation,
  2026-10-16).

## GLM handoff log

- 2026-10-16 (glm-runtime, GLM-5.3): took over from ri-runtime during its usage-limit window, mid "real runs".
  The paused agent had finished real run 1 (cert-manager payments-eu) and folded its findings into 433574f7 —
  but that commit postdates the run, so run 1's per-item outcomes reflect pre-fix code and were never recorded
  here (the promised "Real runs" section was missing). Decision: re-run **all three** cases (cert-manager
  payments-eu, ingress-nginx shop-de, kube-prometheus-stack obs-north) sequentially against current code so the
  summary table describes the validator as it will merge; this also exercises the 433574f7 fixes for real.
  Runs launched via `RI_RUNTIME_STATE=<warm .ri> RI_RUNTIME_CASES=<id> RI_RUNTIME_TIMEOUT=<budget> go test
  ./internal/investigate/runtime -run Groundtruth -v -timeout 60m`, one case at a time (the validator enforces
  one `ri-runtime-*` cluster at a time), logs kept in an untracked `.glm-scratch/` dir (deleted before done).
  Uncertain: whether the shared warm `.ri` cache holds everything the two new cases need offline; if a case
  skips with "change evidence not available offline" I will warm the cache with `GITHUB_TOKEN=$(gh auth token)`
  and re-run it.
