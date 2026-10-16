# Status — lane `orchestrate2` (Claude Sonnet 5.5, agent ri-orchestrate2)

Branch `p4ei/orchestrate2`. Brief: `briefs/orchestrate2.md`.

## Done
- `internal/investigate/run`: workspace (mem + dir), shared types (Refs, StageInput/Output, RoundPlan, Policy table).

## In progress (exact next step)
- Write `run/pipeline.go` (stage exec over ports, audit, memo idempotency), `run/merge.go` (proposed edges),
  `run/review.go` (ApplyReview/Finalize), `run/runner.go` (in-process driver); then rewrite `internal/orchestrate`
  (typed activities per `domain.Stage`, workflow using `run.RoundPlan`), `cmd/ri/investigate.go` + `ri worker`, docs.

## Decisions
- Stage handoff via small Refs; intermediate change/snapshot results live in a scratch Workspace, everything else in the run record + stores.
- Workflow ID identifies the request (bundle id is unknown at start); the Investigation id is in the result/state.
- Request-more-evidence round creates a new bundle, hence a new Investigation record.
- Subcommands of `ri investigate` register through a map so the agent lane's `mcp` can plug in without file conflicts.

## Files touched outside ownership
- none yet (will add `cmd/ri/main.go` dispatch lines, README, docs/INVESTIGATE.md section)

## Contract changes
- none yet

## Tests
- not yet run
