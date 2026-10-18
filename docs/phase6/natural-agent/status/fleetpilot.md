# Status — lane fleetpilot (ledger X-0007)

Branch `p6na/fleetpilot`, worktree `.claude/worktrees/p6na-fleetpilot`. Brief: `briefs/fleetpilot.md`.
Not part of the Phase 0 harness; writes only `scripts/fleet/`, `docs/phase6/fleet-pilot/`, this file, and
`~/fleetpilot/` (outside the repo).

## Done
- Step 1 (partial): pinned beads v1.3.1 (c1c4b642) and gascity v1.5.0 (f6647461), MIT both; release
  binaries for darwin_arm64 downloaded, sha256 checked against the release checksums.
- Step 2: `bd` and `gc` in `~/fleetpilot/bin`; sources at the pinned tags in `~/fleetpilot/src`.
  `source ~/fleetpilot/env.sh` puts them on PATH (never from a shell rc).
- Step 3: `scripts/fleet/ledger-to-beads.py` + `test_ledger_to_beads.py` (12 pass). `bd ready` equals the
  guard's is_ready / ledger_claude_ready / ledger_offered on the Phase 6 ledger (all empty today), the
  Phase 5 ledger (5 / 2 / 3 items) and a synthetic ledger covering every readiness edge.
  Store: `~/fleetpilot/data/ledger-mirror`.

## In progress
- Step 4: gascity toy fleet (separate Herdr session, toy repo under `~/fleetpilot/toy/`).

## Decisions
- Mirror rebuilds the store every run (build in a sibling dir, swap): `bd import` is an upsert and would
  keep dependencies/labels the ledger dropped.
- `queued`, `dropped` and a `missing` stub status are custom statuses of category `frozen`; frozen blockers
  never unblock dependents, which is what the guard does (a dependency must be `done`).

## Findings so far
- bd sends anonymous usage metrics by default and wrote `~/.beads/` (machine-id + event queue) on the first
  `bd --help`. Moved to `~/fleetpilot/logs/dot-beads-evidence`; all later runs use `BD_DISABLE_METRICS=1`
  (the script sets it too). Events from the first ~3 commands may have been flushed upstream.

## Files outside ownership
- none

## Open questions
- none yet
