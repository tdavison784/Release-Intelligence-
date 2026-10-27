# Lane `formalize`: Milestone 4 Workstream D (Opus)

Branch `p6di/formalize`, worktree `.claude/worktrees/p6di-formalize`. Ledger D-0021. Read `_common.md` first, then
`PLAN-M4.md`, and `M3-IMPLEMENTATION.md` (the synthesize pattern to copy). You implement the "Lane `formalize`"
table (changes 1–4).

## Ownership

You own:
- `internal/formalize/` (new);
- the spec checks lifted out of `internal/investigate/agent/mitigation.go` into a pure function. The agent keeps
  calling it; v8 behaviour must not change, and old transcripts must replay;
- `Hypothesis.ProposedVerification` (additive);
- the `ri investigate formalize` and `-formalize` CLI wiring;
- docs, including `M4-IMPLEMENTATION.md` (start it; the other lanes add their sections).

## Required

1. **The stage.**
   - **Input:** one v13 `AgentAssessment` with its item and the run's evidence pool.
   - **Output:** `[]domain.Hypothesis`, each with a `MitigationSpec` / `SemanticAssertion` that passes the existing
     spec checks.
   - **The call:** versioned prompt `formalize/v1` (generic, digest-pinned), one structured call through
     `llm.Cache` (exchange or `claude -p`, default `claude-opus-5-5`), complete provenance, replay as a cache hit.
2. **Non-invention floor (D4), checked after the call; bounded retry; failure recorded, never repaired.**
   - Every citation must already be in the assessment's verified, behaviour or effective-configuration evidence.
   - Every unknown, `not-checked` or `unavailable` behaviour is kept, as an `OpUndecidable` leaf or as an
     unrepresentable record.
   - Applicability is never broader than the sources: no dropped conditions, and no version bounds beyond the run.
   - Defaults appear only as evidenced defaults: the `applicability` lane adds the `Condition` default fields. Until
     they are merged, emit undecidable leaves; integrate after the commander tells you to merge.
3. **Unrepresentable claims (D3).** Keep the original claim, the citations, the intended conditions, the grammar
   limitation and `ProposedVerification`.
4. **Hand-off.** Output feeds `ExportHypotheses` unchanged, so the existing verify and review path applies. No
   Temporal activity.
5. **Tests.**
   - Non-invention: a foreign citation is rejected; a dropped unknown is rejected; a broadened scope is rejected.
   - An unrepresentable claim is kept.
   - Scripted-exchange replay.
   - The v8 agent path is unchanged.
   - Policy isolation: formalization alone never changes classification.

FLEET.md applies, including generic logic only (no product, case, file or field names in Go logic or prompts), runtime clusters named `ri-<lane>-*` and deleted after use, and the files you never open (`GATE-M*.md`, `M*-RESULT.md`, `expected.yaml`, `NOTES.md`, `observed/`, run reports and scores). No live model runs on the development cases; the commander runs the demo and the gate. Three M4 lanes run in parallel: `applicability`, `semverify`, `formalize`. Keep your `domain` changes additive and list them in your status file. The commander merges in the order applicability → semverify → formalize; when told, `git merge p6-decision-intel` into your branch and resolve.
