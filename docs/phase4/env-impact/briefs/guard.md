# Lane `guard` — fleet guard v3: commander handoff, context checkpoints, fleet-dir (infrastructure)

Model: Sonnet. Works in `~/.claude/fleet-guard/` (NOT a git repo — first copy `fleet_guard.py` to
`fleet_guard.v2.py` as the rollback point). Herdr agent `ri-guard`. Status file: write it at
`~/.claude/fleet-guard/STATUS-guard.md` (the commander copies it into the repo).

Read first: `~/.claude/fleet-guard/fleet_guard.py` (all of it), `lead_hook.sh`, `models.txt`,
`state.json`, the last 50 lines of `events.jsonl`, and in the repo
`.claude/worktrees/p4-integration/docs/phase4/env-impact/{FLEET.md,briefs/commander.md,COMMANDER.md}`
(the commander protocol you are automating). Load the `herdr` skill for CLI syntax; use `herdr agent`,
`herdr pane` help output as the authority.

## Context

The guard watches Claude account usage (statusline snapshots in `~/.claude/statusline-snapshots/`) and
Z.AI usage (quota API; key file `~/.claude_token_zai`, never print it). At ≥90 % of a Claude window it
pauses working `ri-*` lane agents and hands each to a GLM-5.3 agent `glm-<lane>` in the same worktree;
at the reset it hands back. **Gap:** the commander itself has no handoff, prompts hard-code Phase 3
paths, and per-agent context-window usage is collected but never acted on.

## Required changes (keep the v2 behaviour otherwise identical)

1. **`--fleet-dir`** (default `docs/phase4/env-impact`): every prompt uses `<fleet-dir>/FLEET.md`,
   `<fleet-dir>/briefs/<lane>.md`, `<fleet-dir>/status/<lane>.md` instead of the Phase 3 paths.
2. **Commander handoff.** New flags `--commander-agent` (default `ri-commander`) and `--commander-cwd`
   (the integration worktree; required when handoff is on). The commander has the `ri-` prefix, so it is
   paused with the lanes; for it:
   - handoff **first** (before lanes), into a split of the commander's pane but with
     `--cwd <commander-cwd>`, agent name `glm-commander`, same env/credential handling as lanes,
     `.lane-lead` in the commander-cwd set to `glm`;
   - commander-specific prompt: "You are glm-commander, temporarily commanding the env-impact fleet
     while the Claude commander is paused for a usage window. Read <fleet-dir>/FLEET.md,
     <fleet-dir>/briefs/commander.md (follow its **GLM commander protocol** exactly) and
     <fleet-dir>/COMMANDER.md, then continue. On a message starting 'HANDBACK', follow the protocol's
     handback steps and reply 'HANDBACK DONE'.";
   - handback **last** (after lanes), and the Claude commander's RESUME prompt is a single message
     that includes the lane handback summary (replacing v2's separate commander-pane wake prompt),
     the number of commits on the integration branch since the handoff base, and an instruction to
     read the "GLM commander log" in COMMANDER.md and review GLM merges before continuing;
   - `glm-*` agents that exist at handback but were not started by the guard (lanes the GLM commander
     started) are **not** closed; list them in the commander's RESUME prompt.
   - If the commander agent is not found, log `handoff-skip` and continue with lanes (v2 behaviour).
3. **Context checkpoints.** Map each live `ri-*` / `glm-*` agent to its session
   (`agent_session.value` from `herdr agent list`) and that session's statusline snapshot
   `context_window.used_percentage`. When an agent crosses `--ctx-threshold` (default 80), send it once
   `CONTEXT CHECKPOINT (fleet guard): your context window is at N%. Update and commit your status file
   now (commander: COMMANDER.md) so work survives compaction, then continue.` Re-arm only after it drops
   below `threshold-30` (i.e. after a compaction). Persist the armed/sent state in state.json. Emit a
   `CTX` event. Snapshots older than 1 h are ignored.
4. **Stale state.** `--reset-state` archives `state.json`, `PAUSED`, `events.jsonl` into
   `archive/<YYYY-MM-DD-HHMMSS>/` and starts clean. The current files are stale from 2026-10-06 (they
   name dead panes and ri-proxy / ri-renderfirst handoffs) — the first real start will use it.
5. **models.txt** stays the relaunch model map; document that the commander appends lanes to it.
6. **Tests:** `test_fleet_guard.py` (stdlib `unittest`, no network, no real herdr): monkeypatch
   `herdr()`, `git_head`, time and the snapshot dir / Z.AI fetch. Cover: Claude trip → commander handoff
   before lanes with the right cwd/name/prompt and lead files; lane handoff prompt uses `--fleet-dir`;
   reset → lanes handed back, commander handed back last, single combined commander RESUME prompt;
   unregistered glm-* agents survive handback and are listed; context checkpoint fires once and re-arms
   after a drop; `--reset-state` archives; Z.AI trip pauses glm-* (v2 behaviour kept). Also a `--dry-run`
   smoke run of ~2 loop iterations against the real (read-only) herdr with `--interval 2` that you stop
   with a timeout. Do **not** start the real guard, and do not touch other agents.
7. Write `README-guard.md` in the guard dir: what it does, every flag, the exact start command for this
   fleet (the commander will run it in the `ri-guard` tab), how to stop it, how to roll back to v2.

Done = tests pass (`python3 -m unittest -v test_fleet_guard`), STATUS-guard.md + README-guard.md
written, final message `LANE DONE:` with the start command.
