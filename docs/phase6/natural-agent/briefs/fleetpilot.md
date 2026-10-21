# Lane fleetpilot — beads mirror + gascity pilot (ledger X-0007)

**Product owner's question (2026-10-18):** "How big/hard of a change would it be to shift over into gascity and
beads at this point?" The commander's answer: don't switch the running Phase 0. Mirror the Phase 6 ledger into
beads now, and pilot gascity on a toy fleet, so the switch decision rests on hands-on evidence. Background: the
GLM commander's comparison in `docs/phase4/env-impact/proposals/2026-10-16-window-ledger.md` (§beads, §gascity).
Treat every claim in it as unverified until you check it against the current upstream.

## Hard limits (in addition to FLEET.md)

- **Install nothing globally.** No `brew install`, no writes to `/usr/local`, `/opt/homebrew`, `~/go/bin`, shell rc
  files or `~/.claude`. Everything goes under `~/fleetpilot/` (binaries in `~/fleetpilot/bin`, data in
  `~/fleetpilot/data`). Put that bin dir on PATH for your own commands only. If a dependency cannot be installed
  this way, stop and report it; do not work around the limit.
- **Never touch the live fleet.** Don't touch the live Herdr session's existing tabs, panes and agents, the fleet
  guard (`~/.claude/fleet-guard/`), `docs/phase6/natural-agent/LEDGER.yaml` (read only) or any other lane. Also off
  limits: temporal on :7233, `./bin/worker`, the :8484 dashboard and the shared `.ri` cache. Run gascity's Herdr
  provider against a **separate named Herdr session** (see `herdr session`) that you create and later remove. If it
  can only target the current session, stop and report that.
- **Toy fleet only.** Toy lanes work in a throwaway git repo under `~/fleetpilot/toy/`, never in this repository.
  Agents use `claude-haiku-4-5-20251001`. The toy fleet's total spend must stay under $5; record it.
- **Network** is fine for fetching upstream code, docs and release binaries. Don't open issues, PRs or comments
  upstream. Don't push.
- **Phase 6 integrity rules still apply.** Never open RUBRIC.md, expected.yaml, NOTES.md, observed/ or the Phase 5
  EVAL-REPORT/SUMMARY. You don't need any of them.

## Work

1. **Upstream check.** Pin the current releases of beads (`bd`) and gascity (`gc`): version, commit, licence, build
   and runtime dependencies (Dolt? Bazel?), and storage backends. Verify whether a file-based store exists, and
   under what name (the proposal says `GC_BEADS=file`). Record recent churn (releases and breaking changes in the
   last 60 days).
2. **Install** both into `~/fleetpilot/` (prefer prebuilt release binaries; else `go build` with `GOBIN` set there).
3. **beads mirror.** Write `scripts/fleet/ledger-to-beads.py`, a one-way import of `docs/phase6/natural-agent/LEDGER.yaml`
   (ids, titles, kind, status, depends, providers, glm-executable, decision-rights) into a beads store under
   `~/fleetpilot/data`. Show that `bd ready` returns the same ready set as the fleet guard's topo-ready logic
   (`~/.claude/fleet-guard/fleet_guard.py`; read only) for the current ledger. Show which ledger fields beads has
   no home for, and how you carried them (labels, custom fields, notes).
4. **gascity toy fleet.** Configure a 2-lane fleet: a coordinator plus two Haiku lanes, through the Herdr runtime
   provider in the separate session, each lane in its own worktree of the toy repo. Give it a 3-item queue with
   one dependency. Exercise:
   - race-safe claiming;
   - the lane-done signal and merge;
   - restarting a lane killed mid-task (health patrol);
   - a "forbidden path" rule: a lane must never read `toy/SEALED.md`. Show how gascity enforces or encodes it, if
     at all.
5. **Gap report** against our fleet. For each item, say how gascity covers it (built in / order or formula /
   plugin needed / not possible), with the evidence from step 4:
   - forbidden eval paths per lane;
   - frozen or pre-registered files;
   - no push without the user;
   - commander-only merges;
   - model-under-test constraints (e.g. "this item must run on Sonnet 5.5 only");
   - the GLM usage-window handoff (fleet_guard v4: quota exhaustion → glm-* lanes continue → RESUME list);
   - `herdr agent prompt --wait` pitfalls;
   - integrity of status files and the ledger.
6. **Port plan and estimate:** what a Phase 7 fleet on gascity + beads needs (setup, plugin for the provider/window
   policy, migration of `fleet_guard.py`), in days, with the riskiest step named. Also say whether a beads-only
   switch for the ledger is worth doing on its own.

## Deliverables

- `docs/phase6/fleet-pilot/REPORT.md` (steps 1, 4, 5, 6; plain English, tables where they help, every claim
  backed by a command output or an upstream link at a pinned commit);
- `scripts/fleet/ledger-to-beads.py` and its test;
- `docs/phase6/natural-agent/status/fleetpilot.md`.
- **Cleanup:**
  - remove the separate Herdr session and its agents;
  - keep `~/fleetpilot/` (bin + data) for the commander's review, and list its size;
  - remove the toy worktrees, unless the report references them.

Final message: `LANE DONE:` in ≤15 lines, including the recommendation in one sentence.
