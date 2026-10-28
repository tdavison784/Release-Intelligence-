# Lane `synthesis`: Milestone 3, upgrade-level synthesis (Opus)

Branch `p6di/synthesis`, worktree `.claude/worktrees/p6di-synthesis`. Ledger D-0017. Read `_common.md` first; that
includes `PRODUCT-DECISIONS.md`, especially PD-1 and PD-3. Then read `PLAN-M3.md`, and `M1-IMPLEMENTATION.md` and
`M2-IMPLEMENTATION.md` for the records you consume. You implement PLAN-M3 §3, changes 1–7, with the §4 tests. Build
the smallest thing that works and that a reviewer can audit.

## Ownership

You own:
- the new `internal/synthesize/` package (pre-computation, LLM call, floor, rendering);
- `domain.UpgradeJudgement` and `Investigation.Judgement` (additive; schemas regenerated);
- `Pipeline.Synthesize`, plus a Temporal activity behind `workflow.GetVersion`;
- the `--synthesize` / `-synthesize` CLI wiring;
- the judgement section in `ri investigate` output and the reportui view/export;
- the ungated `ScoreJudgements` side table;
- docs, including a new `docs/phase6/decision-intel/M3-IMPLEMENTATION.md`.

Never change findings, the decision, the review or knowledge. Extend the policy-isolation tests to prove it.

## Required

1. **PLAN-M3 §2 contents and §3 changes 1–7, each with its §4 tests.**
   - **Pre-computed hard findings and disagreements** carry stable ids derived from their content, and are copied
     verbatim into the judgement. The model may comment on them; it can never remove one.
   - **The target rule (PD-3)** is computed deterministically from the edge's candidates and the version list. The
     model chooses only among the computed options.
   - **Change locations** come only from deterministic facts (`PinnedSetting`, Argo `targetRevision` with file and
     line, values set and unset). Anything else is `proposed`, without a location.
2. **The LLM call.** Use one structured call through `llm.Cache` over the existing exchange / `claude -p` path, the
   same pattern as semantic. The model is configurable; the default is `claude-opus-5-5`. Provenance is complete. A
   replay is a cache hit. The floor is checked after the call, with a bounded retry; a failure is recorded, never
   repaired.
3. **The synthesis prompt** is generic, with no product, case, file or field names, and versioned
   (`synthesize/v1`, with a digest).
4. **Temporal.** `orchestrate/replay_test.go` must still pass. Add a replay test for the new versioned path.
5. **No live runs on development cases.** An optional smoke with a scripted exchange, or one live call on a
   non-development case, is unscored. The commander runs the scored M3 check.
6. **Docs:** `M3-IMPLEMENTATION.md` (what, where, how to run, known limits), `docs/INVESTIGATE.md`, the README command
   list, and your status file.

FLEET.md applies, including generic logic only and the files you never open (`GATE-M*.md`, `M*-RESULT.md`,
`expected.yaml`, `NOTES.md`, `observed/`, run reports and scores).
