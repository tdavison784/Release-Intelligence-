# Proposal — provider window work ledger (user-requested 2026-10-16)

> Status: PROPOSAL, not adopted policy. Requested by the user during the 2026-10-16 15:24–17:31 GLM
> window after observing idle GLM capacity while open work existed. Adoption needs: commander brief
> edits (Claude commander), FLEET.md note, fleet guard v4 (guard lane), and user sign-off on the open
> decisions at the end. Nothing in this file changes any rule until adopted.

## Problem (observed in the 2026-10-16 window)

1. **Idle provider capacity.** All three GLM lanes finished ~16:15; the GLM commander then idled ~75 min
   to handback. Open work existed (eval arm A merge, validator-gap routing, cache-pinning decision
   prep) but none of it was executable under the GLM protocol's closed may-do list.
2. **Resume burst.** Everything GLM could not do piles onto the Claude commander at resume (merge eval,
   2 decisions, 4 go-ahead messages, 3 pane cleanups) — Claude-window tokens are the scarce resource
   and get spent on coordination a GLM window could have absorbed.
3. **Authorization is prose, not data.** "Next actions" free text vs the GLM may-list produced a safe
   but wasteful hold: merging p4ei/eval was "when convenient" for the Claude commander yet forbidden to
   GLM, with no way to pre-authorize it.
4. **Provider fit is tribal knowledge.** "Arm B must run under claude-sonnet-5-5 (model under test)",
   "live runs cost ~250k Claude tokens", "mechanical fixes are GLM-friendly" live in prose and have to
   be re-derived every window.
5. **Quota blindness at enqueue time.** Live run 5 spent the remainder of a 90 %-full Claude window
   because nothing recorded item-level cost estimates against window budgets.

## The ledger

One git-versioned file per fleet: `docs/phase4/env-impact/LEDGER.yaml` (phase-scoped now; generalize to
other fleets later). Append-mostly; every change committed; lanes and both commanders write it.
COMMANDER.md stays the live *state* doc; the ledger is the *work queue + authorization + window history*.

### Item schema

```yaml
items:
  - id: W-0042                  # monotonic, never reused
    title: Re-measure eval arm A after fixes lane
    goal: p4-env-impact          # the established goal this serves
    kind: lane                   # lane | commander-task | review | decision | evidence-prep
    status: ready                # queued | ready | in-progress | blocked | done | dropped
    depends: [W-0041]            # item ids; "ready" only when depends are done
    ready-when: fixes lane merged          # prose condition a commander/guard can check
    providers: [claude, glm]     # who may EXECUTE this item
    glm-executable: true         # enqueue-time pre-authorization for GLM windows
    auto-start: true             # guard may start it without a commander in the loop
    brief: briefs/validator-gaps.md        # required when kind: lane; else absent
    owner: ""                    # agent name while in-progress (e.g. glm-validator-gaps)
    quota-estimate: {provider: zai, tokens: ~150k}   # enqueue-time budget guess
    provenance: {queued-by: claude-commander, queued-at: 2026-10-16T16:40, because: "agent v5 LANE DONE"}
    integrity: "never read eval/investigations/*/{expected.yaml,NOTES.md,observed/}"
windows:                         # append-only handoff log (one entry per provider window)
  - window: {id: 2026-10-16T15:24-claude5h, from: claude, to: glm, handback: 17:31}
    executed: [W-0037, W-0039]   # item ids completed this window
    deferred: [{id: W-0040, why: "needs claude window (model under test)"}]
    quota: {zai-5h-start: 1%, zai-5h-end: 5%}
    merges: [34afd967, caf4be01, 8c7d74f2]
```

### Mechanics

1. **Authorization moves to enqueue time.** Whoever queues an item (Claude commander or user) sets
   `providers` + `glm-executable` + `auto-start`. A GLM window may execute exactly the items whose
   `glm-executable: true` — no protocol widening, no judgment calls mid-window: the authority decision
   was made by the entity that had the right to make it, when the work was queued. Symmetrically,
   `providers: [claude]` items (model-under-test, provider-data rules) are structurally invisible to
   GLM windows, which also prevents the "run arm B under a GLM model" class of accident by construction
   rather than by vigilance.
2. **Windows never idle while sanctioned work exists.** Guard v4, on handoff: for each `status: ready`
   + `glm-executable` + `auto-start` + unowned + `kind: lane` item, start a `glm-<item>` lane from its
   brief (same tab/worktree procedure as today). Commander-task items land in the GLM commander's
   startup message. On resume, ready `providers: [claude]` items are listed in the RESUME message so
   the Claude commander executes them instead of re-deriving Next actions.
3. **Provider fit and cost are declared.** `providers` + `quota-estimate` make "can this run under
   GLM?" and "does this fit the remaining window?" data lookups. Actuals recorded at completion
   (`quota-actual`) build the estimate history, so "arm B needs ~1M sonnet tokens — only into a fresh
   Claude window" becomes a recorded fact rather than a re-learned lesson.
4. **Decisions are items too.** `kind: decision` with `decision-rights: user | claude-commander`, and
   `kind: evidence-prep` for the gather-options work that precedes a decision. Evidence-prep is
   GLM-friendly: this window could have prepared the `.ri` cache-pinning decision brief (options,
   costs, blast radius) instead of idling, while the decision itself stayed with its rightful owner.
5. **The window log replaces freehand handoff prose.** The GLM commander log entries I write today
   become structured `windows:` entries: executed, deferred-with-reason, quota, merges. The Claude
   commander's resume review reads the same data the guard summarizes.

## Seed items (from today's actual backlog — the first ledger, if adopted)

- W-0037 `Merge p4ei/eval arm A` — kind: review, providers: [claude, glm], glm-executable: true
  (this window's held merge — exactly the case pre-authorization would have unlocked), auto-start: false.
- W-0038 `.ri` cache pinning for stored-run replay` — kind: decision, decision-rights: user;
  evidence-prep child W-0038a glm-executable: true (draft options from agent-lane findings).
- W-0039 `Validator/pin-rule gap routing` — kind: lane, needs brief (Claude commander writes it),
  then glm-executable: true.
- W-0040 `Eval arm B` — providers: [claude] ONLY, reason: model under test (claude-sonnet-5-5);
  ready-when: fresh Claude window; quota-estimate ~1M tokens. Never GLM-visible as executable.
- W-0041 `Eval arm C` — providers: [claude, glm], ready-when: runtime merged (done: 34afd967).
- W-0042 `Arm A re-measure` — providers: [claude, glm], depends: fixes merged (done: 8c7d74f2);
  note: eval lane said it would ask first — enqueue resolves that ask structurally.
- W-0043 `v5 item-budget vs claim-cost lever` — kind: decision, decision-rights: claude-commander;
  evidence-prep glm-executable: true (budget-refusal stats already in status/agent.md).

## Adoption steps (owner: Claude commander + guard lane; NOT to be done by a GLM window)

1. briefs/commander.md: add one paragraph to the GLM protocol — "may also execute ledger items with
   `glm-executable: true`" (all existing must-nots stay); replace "Next actions" maintenance with
   ledger enqueue/deref duties for both commanders.
2. FLEET.md: LEDGER.yaml joins the status-file contract (lanes set `status`/`owner` on their items).
3. Guard v4 (guard lane): parse LEDGER.yaml; auto-start ready glm-executable lanes at handoff; list
   ready claude-only items in RESUME; include ledger deltas in the RESUME summary.
4. Seed the ledger from COMMANDER.md Next actions + open questions; keep COMMANDER.md as state-only.

## Alternative substrate: `beads` (raised by the user 2026-10-16)

[beads](https://github.com/gastownhall/beads) (Go, MIT, `bd` CLI + MCP) is a distributed graph issue
tracker built for AI agents: hierarchical task ids, dependencies + priorities, **dependency-aware
`bd ready`** (deterministic "what can start now" — my `depends`/`ready-when`), **atomic
`bd update --claim`** (race-safe `owner` — exactly the claim semantics a multi-agent fleet needs),
Dolt-backed storage with git sync, `bd remember`/`bd prime` durable memories, and first-class agent
integrations (`bd setup` writes AGENTS.md guidance + hooks for Claude Code and others).

**Verdict:** as the *work-queue substrate* it is strictly better than hand-rolled LEDGER.yaml — adopt it
if adding a tool is acceptable (one local `dolt sql-server` serves all worktrees on this machine; no
push needed). It does **not** provide the provider-window layer that motivated this proposal, so either
way that layer is ours:

| Need | beads | LEDGER.yaml |
|---|---|---|
| Durable items, ids, status, deps, ready-detection | ✅ `bd ready` | ✅ (guard does topo-ready) |
| Race-safe claiming by lanes | ✅ `--claim` | ⚠️ single-commander-at-a-time makes races rare |
| Durable cross-window memory | ✅ `remember`/`prime` | ⚠️ status files + COMMANDER.md |
| Per-item **provider authorization** (`glm-executable`, claude-only/model-under-test) | ❌ | ✅ core field |
| Per-item quota estimates/actuals, window budgets | ❌ | ✅ |
| Guard v4 handoff behavior (auto-start glm lanes, RESUME lists claude-only ready items) | ❌ (ours either way) | ❌ (ours either way) |
| Append-only provider-window log | ❌ | ✅ |
| New tool/DB in a git-markdown-only fleet | cost to adopt | none |

## Paired with `gascity` (raised by the user 2026-10-16, same author)

[gascity](https://github.com/gastownhall/gascity) (Go, MIT, `gc` CLI) is an orchestration-builder SDK for
multi-agent coding workflows that uses `bd` (beads) as its work store. Our fleet is a hand-rolled
instance of it: controller/supervisor reconcile ≈ commander + .lane-lead; **health patrol ≈ fleet guard**;
runtime providers include **herdr natively** (our tab/pane structure would survive migration);
convergence loops + gates ≈ LANE DONE + green-gate merge review; mail/waits ≈ herdr `prompt --wait`
(without its lost-prompt pitfalls); orders/formulas ≈ lane briefs; mayor session ≈ the commander role.

**What the pair would give us beyond the hand-rolled fleet:** maintained infrastructure instead of
fleet_guard v3/v4 + prose discipline; race-safe claiming and dependency-ready dispatch at scale; a
controller daemon reconciling desired vs running state; first-class Claude Code/codex/gemini agents
(GLM rides the claude runtime provider via ANTHROPIC_BASE_URL, exactly how glm-* agents run today).

**What it still does not give us (ours either way):** per-provider quota/window management — neither
repo tracks usage windows, provider-authorized items (model-under-test constraints like arm B), or
quota budgets. In a gascity world that becomes a custom provider-policy/health-patrol plugin (worth
upstreaming); in the current world it is guard v4 + the provider fields in this proposal.

**Adoption risks:** Bazel build + Dolt ≥2.1 + `bd` ≥1.0 dependencies (file store `GC_BEADS=file` is the
escape hatch); young/high-churn project (948 open issues, 620 open PRs); it is an SDK, so we would still
be building the fleet on top, including re-encoding our integrity rules (frozen gates, forbidden eval
paths, no-push) into orders/formulas; and phase 4 is in its eval endgame — re-platforming mid-endgame is
the risky version of this move.

## Recommended path (updated after the beads + gascity discussion)

1. **Now (finish phase 4):** do not re-platform. Pilot the idle-window fix only — LEDGER.yaml (or `bd`
   with `GC_BEADS=file`) as the cross-window queue + guard v4 provider layer. Everything else stays.
2. **Next fleet / phase 5:** build on gascity + beads from day one (herdr provider keeps the current
   operating feel), and port the ONE thing the stack lacks — provider/quota policy (usage-window
   handoff, model-under-test item constraints, quota budgets) — as a plugin; upstream it.
3. Either way, the enqueue-time provider authorization idea from this proposal is the piece to keep:
   it is what makes windows fully utilized per provider without widening any agent's standing authority.

1. **Guard auto-start vs commander-only starts.** Auto-start maximizes utilization but starts work
   without a human or commander glance; the per-item `auto-start: false` flag is the mitigation
   (default recommendation: true for mechanical items, false for merges/reviews).
2. **May GLM execute `kind: review` merges** (like the eval arm A merge) when pre-authorized per item?
   Recommendation: yes with `auto-start: false` — the Claude commander still reviews every GLM merge
   at resume, which is the real safety net.
3. **Ledger granularity** — commander-level items (this design) vs fine-grained tasks. Recommendation:
   commander-level; lanes keep their own status files.
4. **Scope** — phase-4 only, or a shared convention for the phase-3 fleet too (it has the same
   windows). Recommendation: phase-4 pilot now, generalize after one window proves it.
