# Lane `consumer`: Milestone 1, consumer-aware investigation (Opus)

Branch `p6di/consumer`, worktree `.claude/worktrees/p6di-consumer`. Ledger D-0001. Read `_common.md` first.
You implement `PLAN-M1.md` §3 (changes 1–8) with the tests in §4. Build the smallest version a reviewer can audit.

## Ownership

You own:
- `internal/investigate/relations/` (edges, references);
- `internal/investigate/agent/`: the new tool, dossier relations, `submit_findings` checks, the per-item budget for
  v9, and the new `prompts/investigate-agent/v9/`;
- the additive `domain` fields for the assessment and the new edge kind, with schemas regenerated;
- the assessment's storage at `internal/investigate/run/pipeline.go`;
- docs for all of the above.

Prompts v1–v8 stay byte-identical. `classify/` and `policy/` are not yours: assessments must never reach the
classifier. Extend `verdict_policy_test.go` to prove it.

## Required

1. **Plan changes 1–8**, each with the §4 tests. Edge 2 (file-path references) is generic path matching over a
   mounted ConfigMap's data against the same pod's other `mountPath`s. Unmatched absolute paths become unresolved
   edges, never silent drops.
2. **`trace_consumers`** returns every hop with an artifact id and line, and an explicit `gaps` list. Add it to
   v9's `tools.json`, and add a feature flag in `prompts.go` with a tool-count check.
3. **Prompt v9** follows the mission:
   - B2: the eight questions;
   - B4: provisional versus supported, with proof obligations named;
   - B5: the natural assessment sections;
   - A1/A3: verified, inferred and unknown kept apart, and model memory is never evidence.

   Use generic wording, with no case or product examples taken from the development cases.
4. **The completion check** (change 7). A not-affected verdict is rejected once while known consumers are
   untraced. Other verdicts get the untraced consumers recorded as gaps in the assessment's `unknown`.
5. **Replay.** A run under v9 replays deterministically with `-agent-driver replay`. Prove it with a test on a
   scripted fixture.
6. **Smoke**, unscored: one live v9 run on `eval/investigations/ingress-nginx-1.11-1.12--shop-de`, using
   `ri eval -investigations` with a cloned state directory, `-offline`, `-agent -agent-model claude-sonnet-5-5
   -agent-prompt investigate-agent/v9`, and a fresh `-work` and `-run-id`. Store it under
   `eval/investigations/results/` like the existing runs. Then show from the transcript that `trace_consumers` was
   used and the assessment was stored. Read nothing label-bearing (FLEET.md). Do not run the cert-manager or
   docs-eu cases: the commander runs and scores those.
7. **Docs:** a short `docs/phase6/decision-intel/M1-IMPLEMENTATION.md` covering what changed, where, how to run v9,
   and the known limits.

If a change needs a contract decision the plan doesn't settle, record it in your status file and ask the commander.
Don't guess.
