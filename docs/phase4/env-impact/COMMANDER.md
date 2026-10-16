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
| 2b | runtime | Opus | §A5 runtime: throwaway kind cluster upgrade → observed-level pass/fail (user-approved 2026-10-16) | bundle, renderdiff, validate | briefs/runtime.md |
| 3 | eval | Opus | P6 harness + scored report vs groundtruth | all | (from DESIGN §7) |

*Model per wave-1/2 lane is re-decided when the briefs are written from DESIGN §7 (Opus where the lane
needs design judgement, Sonnet where it is mostly mechanical against a settled contract).

## Lanes

| Lane | Agent | Model | Worktree | Tab | State |
|---|---|---|---|---|---|
| contract | — | claude-opus-5-5 | (removed) | (closed) | MERGED 861e847d + cleaned up |
| groundtruth | — | claude-opus-5-5 | (removed) | (closed) | MERGED (23f79063, 6e03b550, d2bdd171) + cleaned up — 8 cases, FORMAT.md adopted |
| orchestrate | — | claude-sonnet-5-5 | (removed) | (closed) | wave 0 merged + cleaned up; wave 2 gets a fresh worktree + agent |
| renderdiff | — | claude-opus-5-5 | (removed) | (closed) | MERGED 5521569a + 31fa8c2b (follow-up) + cleaned up |
| agent | ri-agent | claude-opus-5-5 | .claude/worktrees/p4ei-agent | pane w1:p2N (tab w1:t16) | MERGED ed7a0acd; follow-up: deterministic per-item agenda + prompt v2, subject resolution, live run 2 on another product |
| validate | — | claude-opus-5-5 | (removed) | (closed) | MERGED e9566913 + cleaned up |
| classify | ri-classify | claude-opus-5-5 | .claude/worktrees/p4ei-classify | pane w1:p2P (tab w1:t17) | MERGED 7bb6e559 + 1a355107; follow-up 2: renderdiff query env-scope lookup (owns renderdiff now) |
| orchestrate2 | ri-orchestrate2 | claude-sonnet-5-5 | .claude/worktrees/p4ei-orchestrate2 | pane w1:p2Q (tab w1:t18) | MERGED c7f6d5ab; follow-up: bundle CreatedAt determinism (owns bundle now), then wire classify.New() + drop devstubs when classify merges |
| runtime | ri-runtime | claude-opus-5-5 | .claude/worktrees/p4ei-runtime | pane w1:p2R (tab w1:t19) | working since 14:30 |
| bundle | — | claude-sonnet-5-5 | (removed) | (closed) | MERGED 251d7cde + cleaned up |
| relations | — | claude-sonnet-5-5 | (removed) | (closed) | MERGED 9c280dc4 + e993219a (relationships construct) + cleaned up |
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
- 2026-10-16 Milestone-1 third product = kube-prometheus-stack (chart-level, diff-visible break; groundtruth's choice), replacing argo-cd from DESIGN §1 (argo-cd would need Argo CD + git source in kind).
- 2026-10-16 Commander owns eval/investigations/FORMAT.md now (contract lane gone); groundtruth adds renderedChanges labelling as an additive CONTRACT-CHANGE section.
- 2026-10-16 DESIGN §1.3 amendment (adf4fe08, commander): `scope:rendered-unexplained` (in scope; every rendered change nothing upstream explains, release or env scope, becomes its own item, treated like scope:category) and `scope:routine-self-artifact` (out of scope, correlated). Why: diffs are the centerpiece + Principle 3; an unexplained change to the customer's rendered deployment must be investigated, not only block approval. Generic rule, not tuned to a case.
- 2026-10-16 Agent coverage must not depend on model diligence: deterministic agenda of undecided in-scope items, a mandatory per-item entry (incl. examined-no-concern / not-examined), submission rejected while any item lacks one; examined-no-concern never lowers a class. Prompt v2; v1 kept for replay. Live run 2 on a different product to keep the prompt generic.
- 2026-10-16 USER APPROVED a runtime validation lane: throwaway kind cluster, apply env FROM render → TO render, generic observations on impact-path objects, declared exercises; quiet window without the relevant exercise is never a pass. Opus, opt-in (--runtime).
- 2026-10-16 Nothing is pushed and no PR is opened without asking the user.

## Merge log

- 2026-10-16 79aa8627 orchestrate (e07fec3b + d241ed5f): Temporal SDK v1.50.0 spike, InvestigationWorkflow skeleton, 12 testsuite tests, TEMPORAL.md. Integration build/vet/test green after merge. Cleanup: agent exited, tab w1:t11 closed, worktree removed, branch p4ei/orchestrate deleted (-d).
- 2026-10-16 23f79063 groundtruth (ff91c6de..c5eedd71): 5 cases in eval/investigations — cert-manager 1.17.2→1.18.0 rotationPolicy pair (payments-eu affected / payments-us not), ingress-nginx 4.11.5→4.12.0 risk-level pair (shop-de affected / shop-fr not), shop-pl insufficient-evidence. All observed in kind v1.31. Green after merge. Worktree NOT removed: lane continues (follow-up).
- 2026-10-16 861e847d contract (3757157d..ae2e4ab2): DESIGN.md (gap map, scope cert-manager + ingress-nginx (+argo-cd), contracts, manifestation rule §2.2.1, fail-closed four-class policy §3, agent §4, Temporal boundary §5, eval §6, lane plan §7), internal/domain/investigation.go, internal/investigate/api.go, schemas, eval/investigations/{FORMAT.md,gates.yaml} (gates now FROZEN). Green after merge. Cleanup: agent exited, tab w1:t0 closed, worktree removed, branch deleted.
- 2026-10-16 9c280dc4 relations: graph builder (env resources + rendered objects; Argo/Flux/kustomize structure; placeholder nodes for missing objects), exposure mapper, deterministic bounded DFS (25-seed shuffle determinism, bounds, truncation, unresolved edges). Green. Worktree kept for the follow-up.
- 2026-10-16 6e03b550 groundtruth follow-up: kube-prometheus-stack 89.2.4→90.2.0 pair (obs-north affected: identical diff, decided by the render namespace of the ServiceMonitors vs the token Secret; obs-south not-affected) + ingress-nginx shop-nl diff-visible-harmless trap. 8 cases total, all observed in kind. Green.
- 2026-10-16 251d7cde bundle: store (atomic, digest-verified, immutable, audit-only-grows), change collector (§1.3 scope + §2.2.1 manifestation from declared tables, 9 ChangeSourceKinds with status), snapshot builder (RepoRef, Secret redaction before hashing), assembler; bundles for the 5 original cases Validate and are deterministic. Green. 84dcd9ad gofmt of investigation.go. Cleanup: agent exited, tab w1:t14 closed, worktree removed, branch deleted. Its render-link notes forwarded to renderdiff.
- 2026-10-16 e993219a relations follow-up: declarative `relationships:` catalog construct (internal/catalog, schema, 19 bad-rule tests) + rules for cert-manager, ingress-nginx, prometheus-operator (ServiceMonitor/PodMonitor selectors incl. namespaceSelector) from upstream API docs; 3 bug fixes (cluster scope, group-less *Ref, placeholder status). cert-manager e2e goldens regenerated — verified by commander: definitionDigest lines only, nothing under eval/. Green. Cleanup: agent exited, tab w1:t15 closed, worktree removed, branch deleted.
- 2026-10-16 d2bdd171 groundtruth final: FORMAT.md + EdgeKind names in all 8 cases, renderedChanges labels, README no longer leaks expected classes. Commander verified (without reading contents) that classification/match/acceptable fields are byte-identical to 6e03b550 after the lane's truncation slip; FORMAT.md change +42/-0. Green. Cleanup done (tab w1:tZ).
- 2026-10-16 5521569a renderdiff: ManifestPairs per target, renders + diffs as artifacts, RenderedChange correlation, Query/Object/Summaries; all 8 envs assemble. Coverage table showed unexplained rendered changes (cert-manager 13/49, kps 5/5, shop-nl 4/11) had NO change item → DESIGN amendment adf4fe08. Green. Worktree kept for the follow-up.
- 2026-10-16 e9566913 validate: JoinVerdicts, RenderDiff, OverrideInspection (PinnedSetting; pin rules only from join pinned verdicts and deterministic/human facts), SchemaCompat, StaticReference; 112 tests; real-bundle run over all 8 inputs conforms. Gaps (a) bundle to store fact artifacts and (b) not-applicable pairs exempt from the approval block → queued to renderdiff. Cleanup done (tab w1:t13).
- 2026-10-16 6f3ef2e4 (commander) TestEvalRenderCases offline by default: it went online on cache misses and hung in GitHub rate-limit backoff past the 10-min test timeout, blocking the merge check. Live with RI_RENDER_EVAL_LIVE=1.
- 2026-10-16 31fa8c2b renderdiff follow-up: §1.3 amendment implemented (render.SelfArtifactBump; unexplained items at release + env scope), shared render path convention, DetectTargets skips layered values files (no golden changes), not-applicable pairs never block approval, bundle.WithKnowledge stores attached facts as artifacts (cert-manager 18/bundle; ingress-nginx and kps 0 — no facts in knowledge/). 0 uncorrelated rendered changes on all 8 envs. a0da019f gofmt validate. Green (-count=1). Cleanup done (tab w1:t12). WAVE 1 COMPLETE.
- 2026-10-16 ed7a0acd agent: Tools with id/citation/verbatim-quote validation before recording, budgets, JSONL transcript; drivers claude-cli/exchange/replay/scripted; `ri investigate mcp`; 31 rejection tests; replay byte-identical. Live run 1 (shop-de, sonnet, prompt v1): 12/80 tool calls, but only 3/27 in-scope items examined; 2 validation requests refused (values key as subject). Green.
- 2026-10-16 c7f6d5ab orchestrate2: internal/investigate/run (plain Go Pipeline shared by in-process Runner and Temporal activities; parity test same id + digest), internal/orchestrate activities per domain.Stage, review signal via ReviewApplier (timeout fails closed), bounded rounds, hash-chained audit; `ri investigate`, `ri investigate review|state`, `ri worker`; deterministic-pass ValidationRecords in the run record (49 live). Live run on own dev server :7299 (payments-eu input, placeholder classifier, no agent). Green.
- 2026-10-16 7bb6e559 classify: R1–R4 in order, manifestation rule, §3.3 agent-record table, tier-policy input (Config.TierPolicy, additive), ReviewApplier (proxy never approves), ~45 tests, real-pipeline run over 8 inputs valid + reproducible. Domain relaxed: eligible ⇒ every finding eligible (was ⇔) — accepted, fail-closed preserved. INVESTIGATE.md conflict (both lanes appended sections) resolved keeping both. Gaps routed: (a) validate env-scope lookup → classify; (b) snapshot 'partial' for unbuilt kustomizations blocks R2.1 everywhere → orchestrate2 (owns bundle); (c) Answers must carry assertion ids → agent.
- 2026-10-16 1a355107 classify follow-up: validate env-scope item lookup fixed (+ regression test); records for env-scope items 0 → 6 (cert-manager), 0 → 12 (shop-nl), all not-run (honest: unexplained change present in the env render can't be cleared by an empty diff nor fail without a concrete failure). Same bug found in renderdiff query.go:67 → classify; agent prompts.go/session.go → agent.

## Open questions for the user

- (raised 2026-10-16) FINDING: on the real bundles, the knowledge facts behind controller-behaviour items (the cert-manager / ingress-nginx defaults) are only consensus-verified, so OverrideInspection cannot clear them and those environments can at best be potentially-affected — fail-closed and honest, but it caps notAffectedConfirmation. Two routes: (1) a human verifies those facts in the existing review UI (`ri review serve`), which makes them trusted pin sources; (2) a later runtime/observed validator (isolated kind upgrade) — MISSION §A5 last bullet. Commander recommendation: (1) for milestone 1, (2) as a follow-up wave.

- (asked 2026-10-16) Is it OK that the opt-in agent stage sends environment configuration (Secret values redacted) to the model provider? Default until answered: opt-in, redacted; milestone-1 runs use only synthetic fixture environments.

## Next actions

- ~~classify merged; orchestrate2 told~~ (14:40). When orchestrate2, classify and agent follow-ups merge: (wire classify.New(), drop devstubs, live e2e with --agent scripted/replay); then start lane eval (Wave 3, Opus — write briefs/eval.md from DESIGN §6/§7 first; include runtime results if merged, reported per basis).

1. ~~Wave 1~~ COMPLETE 2026-10-16 14:20 (all merged + cleaned up). Old text for reference: (renderdiff, validate, bundle, relations; briefs/_wave1-common.md + briefs/<lane>.md). On each LANE DONE: review against DESIGN §7 + the brief (renderdiff: coverage numbers in status; validate: OverrideInspection pin source is declarative and release-level, absent setting never passes), merge, clean up, and tell still-running wave-1 lanes to `git merge p4-env-impact` when they need a merged sibling.
2. Groundtruth follow-up running (diff-visible pair + diff-visible-harmless trap). When it reports LANE DONE: review + merge; then tell it to `git merge p4-env-impact`, adopt FORMAT.md and the contract's EdgeKind relation names, re-verify, LANE DONE; then merge + clean up.
3. Wave 2 briefs WRITTEN (briefs/_wave2-common.md, agent.md (Opus), classify.md (Opus), orchestrate2.md (Sonnet; lane name `orchestrate2`, branch p4ei/orchestrate2)). agent STARTED 13:55; classify + orchestrate2 STARTED 14:15; classify once validate is merged; orchestrate2 after all of wave 1. GLM commander may start them (as glm-<lane>) per protocol.
4. Wave 3: eval (Opus) after everything.

## GLM commander log

(empty — appended by glm-commander during Claude usage windows)
