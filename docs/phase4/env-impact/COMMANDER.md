# COMMANDER.md — live state of the env-impact fleet

> Single source of truth for whoever is commanding (Claude `ri-commander` or GLM `glm-commander`).
> Protocol: `briefs/commander.md`. Rules: `FLEET.md`. Goal: `MISSION.md`. Update after every change.

Last updated: 2026-10-16 (Claude commander, Opus 5.5, session 77622fd0)

## Where things are

- Integration branch `p4-env-impact` (base `main` @ 05b2a147), worktree `.claude/worktrees/p4-integration`.
- Lanes: branch `p4ei/<lane>`, worktree `.claude/worktrees/p4ei-<lane>`, herdr tab `ri-<lane>` in
  workspace `w1`, agent `ri-<lane>` (Claude) or `glm-<lane>` (GLM).
- Fleet guard: `~/.claude/fleet-guard/fleet_guard.py` (v3 is being built by lane `guard`; v2 backup
  `fleet_guard.v2.py`), runs in herdr tab `ri-guard` (w1:tN). Usage at fleet start: Claude 5h 0 %,
  7d 0 % (7d resets 2026-10-22 17:00 CDT); Z.AI 5h 0 %, week 0 %.
- Commander herdr pane: `w1:pM` (tab w1:t8), agent name `ri-commander`.

## Wave plan

| Wave | Lane | Model | Mission | Depends on | Brief |
|---|---|---|---|---|---|
| 0 | contract | Opus | P1 contracts, DESIGN.md, gap map, policy, gates, lane plan | — | briefs/contract.md |
| 0 | groundtruth | Opus | AC5 known-outcome affected/unaffected pairs (blind, kind-verified) | — | briefs/groundtruth.md |
| 0 | orchestrate | Sonnet | §A7 Temporal SDK spike + skeleton | — | briefs/orchestrate.md |
| 0 | guard | Sonnet | fleet guard v3 (commander handoff, ctx checkpoints) | — | briefs/guard.md |
| 1 | bundle | Sonnet* | P1 change evidence + env snapshot → evidence bundle | contract | (from DESIGN §7) |
| 1 | graph | Opus | P2 relationship discovery + bounded DFS impact paths | contract | (from DESIGN §7) |
| 1 | validate | Sonnet | P4 override/render/schema/reference validators | contract | (from DESIGN §7) |
| 2 | agent | Opus | P3 investigation agent (MCP tools, claude -p runner, replay) | bundle, graph | (from DESIGN §7) |
| 2 | classify | Opus | §A6 four-class policy, findings, approval eligibility | contract, validate | (from DESIGN §7) |
| 2 | orchestrate (cont.) | Sonnet | P5 real activities, `ri worker`, human review signal | all wave 1 | (from DESIGN §7) |
| 3 | eval | Opus | P6 harness + scored report vs groundtruth | all | (from DESIGN §7) |

*Model per wave-1/2 lane is re-decided when the briefs are written from DESIGN §7 (Opus where the lane
needs design judgement, Sonnet where it is mostly mechanical against a settled contract).

## Lanes

| Lane | Agent | Model | Worktree | Tab | State |
|---|---|---|---|---|---|
| contract | ri-contract | claude-opus-5-5 | .claude/worktrees/p4ei-contract | pane w1:p2F | working (kicked off 2026-10-16) |
| groundtruth | ri-groundtruth | claude-opus-5-5 | .claude/worktrees/p4ei-groundtruth | pane w1:p2E | working (kicked off 2026-10-16) |
| orchestrate | ri-orchestrate | claude-sonnet-5-5 | .claude/worktrees/p4ei-orchestrate | pane w1:p2G | working (kicked off 2026-10-16) |
| guard | ri-guard | claude-sonnet-5-5 | edits ~/.claude/fleet-guard (no git); shell cwd = p4-integration, read-only | pane w1:p2D (tab w1:tN, right split; left pane w1:p1C reserved for the guard process) | working (kicked off 2026-10-16) |

## Decisions

- 2026-10-16 Herdr lanes (not in-session subagents) so the fleet guard can pause and hand off each
  lane, and the commander, to GLM during Claude usage windows (same model as Phase 3).
- 2026-10-16 First-milestone scope default: Helm-delivered Kubernetes add-ons in a GitOps repo
  (cert-manager, ingress-nginx, +1); the contract lane confirms or amends it in DESIGN §1.
- 2026-10-16 Temporal Go SDK pre-approved as a dependency, confined to `internal/orchestrate`.
- 2026-10-16 Known outcomes for AC5 come from real upgrades in throwaway `kind` clusters where
  feasible (render-level otherwise, labelled).
- 2026-10-16 ri-guard runs with shell cwd in p4-integration (already trusted) instead of ~/.claude/fleet-guard: Claude Code showed a folder-trust dialog there that the commander must not answer for the user.
- 2026-10-16 Nothing is pushed and no PR is opened without asking the user.

## Merge log

(none yet)

## Open questions for the user

(none yet)

## Next actions

1. ~~Start wave 0 lanes~~ done 2026-10-16. Watch for LANE DONE (herdr agent list / read).
2. When guard reports LANE DONE: review the diff vs `fleet_guard.v2.py`, copy STATUS-guard.md to
   `status/guard.md`, start the guard in tab ri-guard with `--reset-state` and the README command.
3. When contract reports LANE DONE: review DESIGN.md critically (gap map honesty, fail-closed policy,
   ACTION REQUIRED contract preserved), merge, tell groundtruth + orchestrate to
   `git merge p4-env-impact`, then write wave-1 briefs from DESIGN §7 and start wave 1.

## GLM commander log

(empty — appended by glm-commander during Claude usage windows)
