# COMMANDER.md — live state of the env-impact fleet

> Single source of truth for whoever is commanding (Claude `ri-commander` or GLM `glm-commander`).
> Protocol: `briefs/commander.md`. Rules: `FLEET.md`. Goal: `MISSION.md`. Update after every change.

Last updated: 2026-10-16 (Claude commander, Opus 5.5, session 77622fd0)

## Where things are

- Integration branch `p4-env-impact` (base `main` @ 05b2a147), worktree `.claude/worktrees/p4-integration`.
- Lanes: branch `p4ei/<lane>`, worktree `.claude/worktrees/p4ei-<lane>`, herdr tab `ri-<lane>` in
  workspace `w1`, agent `ri-<lane>` (Claude) or `glm-<lane>` (GLM).
- Fleet guard: `~/.claude/fleet-guard/fleet_guard.py` v3 (README-guard.md; v2 backup `fleet_guard.v2.py`),
  RUNNING in herdr pane w1:p1C (tab `ri-guard`, w1:tN) since 2026-10-16 12:50, `--fleet-dir docs/phase4/env-impact`. Usage at fleet start: Claude 5h 0 %,
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
| 1 | renderdiff | Opus | **critical path**: env-scoped render before/after manifests + structured diffs into the bundle, diff query API | contract | (from DESIGN §7) |
| 1 | relations | Sonnet | (only if needed) lightweight consumer/reference discovery from rendered objects | contract | (from DESIGN §7) |
| 1 | validate | Opus | P4 override/render/schema/reference validators | contract | (from DESIGN §7) |
| 2 | agent | Opus | P3 investigation agent centred on diff-query tools (MCP, claude -p runner, replay) | bundle, renderdiff | (from DESIGN §7) |
| 2 | classify | Opus | §A6 four-class policy, findings, approval eligibility | contract, validate | (from DESIGN §7) |
| 2 | orchestrate (cont.) | Sonnet | P5 real activities, `ri worker`, human review signal | all wave 1 | (from DESIGN §7) |
| 3 | eval | Opus | P6 harness + scored report vs groundtruth | all | (from DESIGN §7) |

*Model per wave-1/2 lane is re-decided when the briefs are written from DESIGN §7 (Opus where the lane
needs design judgement, Sonnet where it is mostly mechanical against a settled contract).

## Lanes

| Lane | Agent | Model | Worktree | Tab | State |
|---|---|---|---|---|---|
| contract | — | claude-opus-5-5 | (removed) | (closed) | MERGED 861e847d + cleaned up |
| groundtruth | ri-groundtruth | claude-opus-5-5 | .claude/worktrees/p4ei-groundtruth | pane w1:p2E (tab w1:tZ) | 5 cases merged (23f79063); follow-up: diff-visible pair + diff-visible-harmless trap; adopt FORMAT.md after contract merges; worktree kept until then |
| orchestrate | — | claude-sonnet-5-5 | (removed) | (closed) | wave 0 merged + cleaned up; wave 2 gets a fresh worktree + agent |
| renderdiff | ri-renderdiff | claude-opus-5-5 | .claude/worktrees/p4ei-renderdiff | pane w1:p2H (tab w1:t12) | wave 1, working since 13:20 (CRITICAL PATH) |
| validate | ri-validate | claude-opus-5-5 | .claude/worktrees/p4ei-validate | pane w1:p2J (tab w1:t13) | wave 1, working since 13:20 |
| bundle | ri-bundle | claude-sonnet-5-5 | .claude/worktrees/p4ei-bundle | pane w1:p2K (tab w1:t14) | wave 1, working since 13:20 |
| relations | ri-relations | claude-sonnet-5-5 | .claude/worktrees/p4ei-relations | pane w1:p2M (tab w1:t15) | MERGED 9c280dc4; follow-up: declarative `relationships:` catalog construct (rules from upstream API docs) — worktree kept |
| guard | ri-guard | claude-sonnet-5-5 | edits ~/.claude/fleet-guard (no git); shell cwd = p4-integration, read-only | pane w1:p2D (tab w1:tN, right split; left pane w1:p1C reserved for the guard process) | DONE — v3 (16 tests) running since 2026-10-16 12:50 in pane w1:p1C; agent idle, kept for guard fixes |

## Decisions

- 2026-10-16 Herdr lanes (not in-session subagents) so the fleet guard can pause and hand off each
  lane, and the commander, to GLM during Claude usage windows (same model as Phase 3).
- 2026-10-16 First-milestone scope default: Helm-delivered Kubernetes add-ons in a GitOps repo
  (cert-manager, ingress-nginx, +1); the contract lane confirms or amends it in DESIGN §1.
- 2026-10-16 Temporal Go SDK pre-approved as a dependency, confined to `internal/orchestrate`.
- 2026-10-16 Known outcomes for AC5 come from real upgrades in throwaway `kind` clusters where
  feasible (render-level otherwise, labelled).
- 2026-10-16 ri-guard runs with shell cwd in p4-integration (already trusted) instead of ~/.claude/fleet-guard: Claude Code showed a folder-trust dialog there that the commander must not answer for the user.
- 2026-10-16 PRODUCT OWNER: "DFS" in the goal was a mistype for **diffs**. Render before/after diffs (internal/render) are the centerpiece evidence for the agent; graph/DFS demoted to lightweight supporting context. `graph` lane replaced by `renderdiff` (critical path) + optional `relations`. Sent to ri-contract mid-design; noted in MISSION.md.
- 2026-10-16 orchestrate: go directive 1.24.7 → 1.26.0 accepted (Temporal SDK v1.50.0 requires it; no CI/Docker pins Go in this repo; toolchain go1.27.1). testify removed as a direct dep before merge (stdlib test idiom).
- 2026-10-16 USER: every lane cleans up when complete to keep disk free — lanes delete kind clusters/bin/scratch output before LANE DONE; the commander removes the worktree + merged branch (`-d`) and closes the agent/tab right after each merge (FLEET.md, briefs/commander.md).
- 2026-10-16 Pre-existing processes NOT from this fleet, left alone: `temporal server start-dev` (pid 46289, up 22 d) and `./bin/worker` (pid 80813, up 16 d) on :7233. Lanes needing a Temporal dev server use another port (`--port`).
- 2026-10-16 GROUNDTRUTH FINDING: the decisive breaks in both pairs are controller-binary defaults, invisible (or only CRD description text) in the rendered diff. Passed to contract as a generic design input: classify each breaking item by where it manifests (rendered-config vs controller-behaviour vs unknown); render-diff silence is never sufficient for Not Affected; for controller-behaviour items the deterministic check is an explicit pinning override in env evidence.
- 2026-10-16 ri-groundtruth killed and relaunched a hung Docker Desktop at lane start (hung since a disk-full event on 2026-10-07); healthy since.
- 2026-10-16 DELIVERY BUG: every commander prompt sent without `--wait` (output discarded) was lost — incl. the diffs clarification to contract (it found it in MISSION.md anyway), the controller-default input, the cleanup note, and groundtruth's follow-up. Resent with `--wait` and verified. Rule added to briefs/commander.md.
- 2026-10-16 Auto-approve eligibility requires the agent stage completed (contract question; commander: keep — it is completeness, not model confidence).
- 2026-10-16 relations: the declarative `relationships:` catalog construct is NOT deferred — without it cert-manager Certificate→Secret stays inferred, traversal is never complete, and R2.3 could never clear any cert-manager environment. Rules authored from upstream API docs only (cited per rule).
- 2026-10-16 Nothing is pushed and no PR is opened without asking the user.

## Merge log

- 2026-10-16 79aa8627 orchestrate (e07fec3b + d241ed5f): Temporal SDK v1.50.0 spike, InvestigationWorkflow skeleton, 12 testsuite tests, TEMPORAL.md. Integration build/vet/test green after merge. Cleanup: agent exited, tab w1:t11 closed, worktree removed, branch p4ei/orchestrate deleted (-d).
- 2026-10-16 23f79063 groundtruth (ff91c6de..c5eedd71): 5 cases in eval/investigations — cert-manager 1.17.2→1.18.0 rotationPolicy pair (payments-eu affected / payments-us not), ingress-nginx 4.11.5→4.12.0 risk-level pair (shop-de affected / shop-fr not), shop-pl insufficient-evidence. All observed in kind v1.31. Green after merge. Worktree NOT removed: lane continues (follow-up).
- 2026-10-16 861e847d contract (3757157d..ae2e4ab2): DESIGN.md (gap map, scope cert-manager + ingress-nginx (+argo-cd), contracts, manifestation rule §2.2.1, fail-closed four-class policy §3, agent §4, Temporal boundary §5, eval §6, lane plan §7), internal/domain/investigation.go, internal/investigate/api.go, schemas, eval/investigations/{FORMAT.md,gates.yaml} (gates now FROZEN). Green after merge. Cleanup: agent exited, tab w1:t0 closed, worktree removed, branch deleted.
- 2026-10-16 9c280dc4 relations: graph builder (env resources + rendered objects; Argo/Flux/kustomize structure; placeholder nodes for missing objects), exposure mapper, deterministic bounded DFS (25-seed shuffle determinism, bounds, truncation, unresolved edges). Green. Worktree kept for the follow-up.

## Open questions for the user

- (asked 2026-10-16) Is it OK that the opt-in agent stage sends environment configuration (Secret values redacted) to the model provider? Default until answered: opt-in, redacted; milestone-1 runs use only synthetic fixture environments.

## Next actions

1. Wave 1 running (renderdiff, validate, bundle, relations; briefs/_wave1-common.md + briefs/<lane>.md). On each LANE DONE: review against DESIGN §7 + the brief (renderdiff: coverage numbers in status; validate: OverrideInspection pin source is declarative and release-level, absent setting never passes), merge, clean up, and tell still-running wave-1 lanes to `git merge p4-env-impact` when they need a merged sibling.
2. Groundtruth follow-up running (diff-visible pair + diff-visible-harmless trap). When it reports LANE DONE: review + merge; then tell it to `git merge p4-env-impact`, adopt FORMAT.md and the contract's EdgeKind relation names, re-verify, LANE DONE; then merge + clean up.
3. Wave 2 briefs WRITTEN (briefs/_wave2-common.md, agent.md (Opus), classify.md (Opus), orchestrate2.md (Sonnet; lane name `orchestrate2`, branch p4ei/orchestrate2)). Start agent once bundle + renderdiff are merged; classify once validate is merged; orchestrate2 after all of wave 1. GLM commander may start them (as glm-<lane>) per protocol.
4. Wave 3: eval (Opus) after everything.

## GLM commander log

(empty — appended by glm-commander during Claude usage windows)
