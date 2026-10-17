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

## Merge log

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

## Open questions for the user

- (Phase 4 carry-over) delete superseded branch `p4ei/eval` (needs -D); window-ledger proposal sign-off.

## Next actions

1. Start wave-1 lanes (tabs, agents, models.txt), restart fleet guard with `--fleet-dir docs/phase5/learning-loop
   --commander-cwd .claude/worktrees/p5-integration`.
2. Merge lanes as they finish (review every diff for fail-open behaviour); tell eval5 to merge `verdict` when it lands.
