# Lane guard status — DONE

Done: `fleet_guard.py` v3 (v2 kept as `fleet_guard.v2.py`), `test_fleet_guard.py` (14 tests, pass),
`README-guard.md`. Loop refactored into `step()` for testability; v2 behaviour otherwise unchanged.
- `--fleet-dir` (default docs/phase4/env-impact) in all prompts.
- Commander handoff first / handback last; single combined commander RESUME; unregistered glm-* listed,
  not closed; missing commander ⇒ `handoff-skip`.
- Context checkpoints (`--ctx-threshold` 80, re-arm <50, persisted in `ctx_sent`; not sent to paused agents).
- `--reset-state` archives state/PAUSED/events. `--state-dir` added (test/dry-run isolation).
Decisions: idle commander is also handed off (lanes keep v2 "only if working at trip" rule); `--commander-pane`
kept as ignored arg; `--commander-cwd` required unless `--no-handoff`; fleet-guard dir is not a git repo.
Not done: no real start (commander does it). Stale real state.json/PAUSED/events.jsonl (2026-10-06) untouched
— first start must use `--reset-state`. Dry-run smoke OK against real herdr (used a temp state dir).
Test: `python3 -m unittest -v test_fleet_guard` → 14 OK.

Review fix (commander): RESUME to the commander is now sent when it is in `paused` OR was handed off (`cres`),
as one message; a paused-but-not-handed-off commander gets a plain resume note ("not handed off", no GLM-log
instruction). Note `handoff_commander` already appended an idle handed-off commander to `st['paused']`, so the
reported path was latent; the explicit `or cres` makes it robust. Tests added: idle-commander-at-trip and
paused-not-handed-off (16 tests, all OK).
