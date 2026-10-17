# Phase 5 commander state (learning-loop fleet)

Live state; successor resumes from here (see `briefs/commander.md`). Goal: `MISSION.md` (scope note first).
Experiment: `PROTOCOL.md` (pre-registered 2026-10-17, never edited; amendments appended).

## Plan

- **Wave 1 (parallel):** pins, coverage, verdict, verify (Opus); eval5 (Sonnet: snapshots, scoring, A0 baseline);
  groundtruth5 (Opus, sealed branch — never merged before freeze F1).
- **Wave 2 (experiment, PROTOCOL §6 steps 1–6):** freeze F1 → A0 → B0 → export + verify hypotheses → product owner
  promotes confirmed ones in the review UI → K1 → C1 + Kbad-*.
- **Wave 3:** unseal groundtruth5 → held-out A0/B0/C1 → report (EVAL-REPORT.md, SUMMARY.md) → push to PR.

## Lanes

| Lane | Agent | Model | Branch | Worktree | Tab | State |
|---|---|---|---|---|---|---|
| pins | ri-pins | Opus | p5ll/pins | .claude/worktrees/p5ll-pins | w1:t1P | working (started 07:53) |
| coverage | ri-coverage | Opus | p5ll/coverage | .claude/worktrees/p5ll-coverage | w1:t1Q | working |
| verdict | ri-verdict | Opus | p5ll/verdict | .claude/worktrees/p5ll-verdict | w1:t1R | working |
| verify | ri-verify | Opus | p5ll/verify | .claude/worktrees/p5ll-verify | w1:t1S | working |
| groundtruth5 | ri-groundtruth5 | Opus | p5ll/groundtruth5 (SEALED) | .claude/worktrees/p5ll-groundtruth5 | w1:t1T | working |
| eval5 | ri-eval5 | Sonnet | p5ll/eval5 | .claude/worktrees/p5ll-eval5 | w1:t1V | wave 1 merged; idle, kept for waves 2–3 |

## Merge log

- 2026-10-17 e5b3afa7 eval5 wave 1: knowledgeDigest/knowledgeLive in run.json, `-knowledge` mandatory (live tree refused unless -allow-live-knowledge), snapshot.sh + kbad (S3 a–h), ungated agent-verdict table (correct/false-safe/false-alarm/hedged/overstated/no-verdict; stub until verdict merges), `-compare` tables, A0-K0-20261017-bfebde39 (K0 digest sha256:672a7ce0…; 24/24 item decisions identical to Phase 4 arm A; payments-us B1 potentially-affected). Green. Worktree kept. Note: lane deleted one uncommitted earlier A0 run from a pre-commit binary (never cited).

## Decisions

- 2026-10-17 Phase 5 branches from `p4-env-impact` @ 03b6ff43 (PR #6 open, not merged into main): Phase 5 needs
  Phase 4 code + the product owner's verified knowledge.
- 2026-10-17 The payments pair is the **development** pair: the gap analysis (and so the commander) saw its labels and
  its environment differences (leaf Certificates rotate in both; only the CA's explicit setting and its consumers
  differ). Held-out ground truth is the measurement; lane briefs and prompts carry no case-shaped detail.
- 2026-10-17 No new subsystem: the rule is a normal knowledge fact (proposal → whole-fact review → human decision with a
  confirmed proof record); applicability is decided by the existing pin validator; agent verdicts are stored beside,
  never inside, policy inputs.
- 2026-10-17 Promotion is the product owner's click (human level), gated on a confirmed proof record.

- 2026-10-17 PR #6 merged into main by the product owner (merge commit bebbeb2c, history preserved: this branch's
  base 03b6ff43 is in main). Window ledger ADOPTED (product owner) → `LEDGER.yaml`; guard v4 queued (W-0008).

## Open questions for the user

- W-0019 delete superseded branch `p4ei/eval` (needs -D). W-0020 gascity/beads for the next fleet (after the report).

## Next actions

The queue is `LEDGER.yaml` (ready items, their depends and who may execute them). Guard v4 (W-0008) in
progress with ri-guard; until it merges, handoffs use guard v3 and the GLM commander reads LEDGER.yaml by hand.
