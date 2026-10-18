# Status — lane fleetpilot (ledger X-0007)

Branch `p6na/fleetpilot`, worktree `.claude/worktrees/p6na-fleetpilot`. Brief: `briefs/fleetpilot.md`.
Not part of the Phase 0 harness. Writes only `scripts/fleet/`, `docs/phase6/fleet-pilot/`, this file, and
`~/fleetpilot/` (outside the repo).

## Done (all brief steps)
- Step 1 (upstream check): beads v1.3.1 (`c1c4b642`) and gascity v1.5.0 (`f6647461`), both MIT. Records
  dependencies, storage backends and churn. `GC_BEADS=file` exists but is test-grade. Bazel is optional.
- Step 2 (install): bd, gc, dolt 2.1.7 (release binaries, checksums verified) and flock 0.4.0 (built from a
  source release whose hash matches Homebrew's) are in `~/fleetpilot/bin`. `source ~/fleetpilot/env.sh` sets up
  PATH and the containment env vars.
- Step 3 (beads mirror): `scripts/fleet/ledger-to-beads.py` + `test_ledger_to_beads.py`, 12 tests pass.
  `bd ready` equals the guard's is_ready, ledger_claude_ready and ledger_offered views on the Phase 6, Phase 5,
  Phase 6 what-if and synthetic ledgers. Field mapping is in REPORT §3.
- Step 4 (toy fleet): a coordinator plus a 2-session Haiku lane pool on the herdr runtime in the separate
  session `fpilot`, with worktrees per slot and 3 beads, one of them dependent.
  - Exercised: claim race (1 winner of 10), dependency gating, lane-done → merge (including a fail-closed
    merge failure and a rework), and a `kill -9` restart (~24 s, same session and bead).
  - Forbidden-path probe: the prompt held; the hook and sparse-checkout layers are partial.
  - Spend: ≤ $1.41 at list price ($0.21 measured).
- Steps 5–6: gap report and port plan (~9–13 days; riskiest step is the window-policy plugin) are in
  `docs/phase6/fleet-pilot/REPORT.md`.
- Cleanup:
  - herdr session `fpilot` deleted, Dolt stopped, toy worktrees removed, symlink removed, transcripts moved
    to `~/fleetpilot/logs/transcripts`.
  - `~/fleetpilot` (784 MB) is kept for review.

## Decisions
- The mirror rebuilds the store on every run, because `bd import` is an upsert and keeps stale dependencies
  and labels.
- `queued`, `dropped` and `missing` are custom statuses in bd's `frozen` category, which reproduces the
  guard's rule that every dependency must be done.
- The toy uses the bd/Dolt store, because the file store cannot run work (`gc bd` refuses it).
- I avoided `gc init`/`start`/`register`/`restart` (each writes a launchd agent). Instead: `gc init --no-start`,
  registration by hand in `$GC_HOME/cities.toml`, and `gc supervisor run` in the foreground.
- Operator interventions are logged in `~/fleetpilot/logs/interventions.txt`:
  - answered 2 trust dialogs;
  - reset 2 sessions after the SHELL fix;
  - the kill test;
  - commander fix and rework of T1;
  - force-closed T3 and the probe.

## Footprint outside ~/fleetpilot (all reported)
- bd's first commands wrote `~/.beads` (and may have sent ~3 metric events) and `~/.dolt`. Both were moved
  under `~/fleetpilot/logs`, and all later runs were contained.
- Three Claude Code trust entries in `~/.claude.json` (backup at `~/fleetpilot/logs/claude.json.before-trust`).
- A temporary symlink in `~/Library/Application Support/herdr` (removed).
- Toy agent transcripts were written to `~/.claude/projects` (moved out).

## Files outside ownership
- none

## Open questions
- X-0008 is the product owner's decision. Recommendation in REPORT.md: do not switch yet, and do not do a
  beads-only ledger switch.
