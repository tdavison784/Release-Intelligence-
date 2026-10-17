# Lane `eval` — reliability and confidence evaluation (MISSION P6, AC5) — Opus

Branch `p4ei/eval`, worktree `.claude/worktrees/p4ei-eval`. Read `_wave2-common.md` (and `_wave1-common.md`)
first, then DESIGN §6 (normative), `eval/investigations/FORMAT.md`, `eval/investigations/gates.yaml`
(**frozen** — never edit), `eval/FORMAT.md` and `internal/eval` (how Phase 3 scores honestly).

You are the only pipeline-side lane allowed to read `eval/investigations/*/expected.yaml`: you score
against it. You never change pipeline code, prompts or policy to move a number — if the evaluation exposes
a defect, report it (status file + final message) and the commander routes the fix to its owner; every
scored run is kept, never overwritten or cherry-picked.

## Owns

`internal/eval/investigations*.go` (+ tests), `eval/investigations/results/`, `cmd/ri` wiring for
`ri eval -investigations`, `docs/phase4/env-impact/EVAL-REPORT.md`, your status file.

## Deliverables

1. **Harness** (`ri eval -investigations [-agent <driver>] [-runtime] [-case <id>]`): runs the real
   `internal/investigate/run` pipeline on every case's `input.yaml` + `env/`, matches findings to expected
   items via FORMAT.md `match`, computes every DESIGN §6.2 metric and evaluates `gates.yaml` (non-zero exit
   on a failing gate; INSUFFICIENT-DATA is printed as such and is never a pass). Reproducibility:
   re-classifying a stored run record yields identical finding digests.
2. **Scored runs**, each stored under `eval/investigations/results/<run-id>/` with the binary's git commit,
   inputs digests and full run records:
   - **A — deterministic only** (no agent, no runtime).
   - **B — + agent** (claude-cli driver, claude-sonnet-5-5, prompt v2 if merged; record prompt digest; one run
     per case; the exchange/replay recording is committed so the run replays offline).
   - **C — + runtime** (if lane `runtime` is merged; opt-in, kind clusters deleted afterwards).
   Report every metric **per finding basis** (deterministic / observed / none) and per expected verification
   level, and per arm — so the effect of the agent and of runtime validation is visible separately.
3. **EVAL-REPORT.md**: per-case table (expected vs actual per item, with the rule that decided it and the
   evidence gap if any), the gates table per arm, FN/FP analysis with root causes (a pipeline defect vs. a
   missing input vs. an honest fail-closed outcome), unresolved/incomplete investigations, human-review rate,
   auto-approval eligibility (and whether any eligible finding was wrong — `unsafeEligibilityRate`), and what
   would have to change for autonomy (DESIGN §3.4, §6.3). State n everywhere; 8 cases is small — say so.
4. Tests for the harness (matching, metrics, gate evaluation, INSUFFICIENT-DATA) on synthetic fixtures.

Done = runs A and B committed (C if runtime merged), report written, build/vet/test green, cleanup,
`LANE DONE:` with the headline gate results per arm.
