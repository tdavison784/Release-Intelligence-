# Lane synthesis: status (Milestone 3, ledger D-0017)

Branch `p6di/synthesis` (from `p6-decision-intel` @ 91957f82). Brief: `briefs/synthesis.md`. Plan: `PLAN-M3.md`
§3 changes 1–7 with the §4 tests. Design, tests, smoke and known limits: `M3-IMPLEMENTATION.md`.

## Done

- Changes 1–7, each with its §4 tests (see `M3-IMPLEMENTATION.md` "What changed, and where" and "Tests"):
  1. `domain.UpgradeJudgement`, `Investigation.Judgement`, stage `synthesize`; schemas regenerated.
  2. `synthesize.Precompute`: hard findings and disagreements with content-derived ids, PD-3 target options,
     deterministic locations, per-item digests, candidate tests, residual risks, interactions, backout.
  3. One structured call through `llm.Cache` over the file exchange or a stateless `claude -p` (`ClaudePrint`);
     default model `claude-opus-5-5`; prompt `synthesize/v1` (generic, digest-pinned).
  4. `synthesize.Floor`, strict decoding, bounded retry (3 calls), recorded failure.
  5. `Pipeline.Synthesize`, `Runner.Synthesize`, Temporal activity behind `GetVersion("synthesize")`; a new recorded
     history `orchestrate/testdata/history-synthesize.json`; `history-approve.json` still replays.
  6. `RenderJudgement` in `ri investigate` output and the reportui page / export.
  7. `eval.ScoreJudgements` ungated side table.
- CLI: `-synthesize`, `-synthesize-model`, `-synthesize-exchange` on `ri investigate`, `ri worker`, `ri eval -investigations`.
- Docs: `M3-IMPLEMENTATION.md`, `docs/INVESTIGATE.md` (new section + evaluation bullet), README command list.
- Smoke (unscored, non-development case shop-nl, scripted exchange answer): offline and online; found and fixed one
  defect (options marked safe while a security source was unread).

## In progress (exact next step)

- Nothing: lane done. Next is the commander's scored M3 check (`-agent -agent-prompt investigate-agent/v13 -synthesize`
  on the development cases; the synthesis needs the recorded state's advisory and chart caches for an offline replay).

## Decisions (with why)

1. **Synthesis is not part of `investigate.Config`.** The config digest is part of the investigation id; the switch on
   the runner / workflow input keeps the id, findings and audit of every run identical with or without a judgement.
2. **Hard findings** (block `proceed`; an M5 approval against one is an override): security regression, confirmed
   breakage, failed validation, candidate still affected (PD-1's list). The plan's other entries are disagreements.
3. **Disagreement** = the agent's class and the classification differ and either is affected or not-affected (PD-1:
   every disagreement; a difference between potentially-affected and insufficient-evidence is not about the outcome).
4. **Safe needs a complete advisory assessment** (found by the smoke; mission A2). Stricter than "no regression in the
   sources read"; fail closed.
5. **Latest-patch facts** come from the app's cached releases and advisories, read with the edge's own range reading
   (`upgrade.AdvisoryReading`) and accepted only when they reproduce the edge's regressions.
6. **When the target has no regression**, the minimum is the target and the rule still names the latest patch on its
   line (PD-0b); both are options, the model chooses.
7. **The transition test of a changed target is deterministic**: added whatever the answer says (mission C2), like the
   findings. Nothing else in an answer is added or changed.
8. **Retry as one user message** (task + violations + rejected answer), so `scripts/semantic-exchange.sh`, which
   answers single-message requests, serves retries too.
9. **A synthesis with no answer** (pending exchange, model unreachable) is an audited `failed` stage and the run goes
   on to the review; a judgement whose every answer violated the floor is recorded `failed`.

## Files touched outside ownership

- `internal/domain/investigation.go`: `StageSynthesize` in `Stages`, the `Judgement` field, its validation hunks
  (the field itself is owned).
- `internal/domain/schemagen/meta.go`: the four new enums.
- `internal/upgrade/advisories.go`: additive `AdvisoryReading` (pure; the edge's own range reading).
- `internal/investigate/run/pipeline.go` (`Deps.Synthesizer`), `runner.go` (`Runner.Synthesize`), `types.go`
  (stage policy), `runtest/fixture.go` (test support: `Synth`, `SynthClient`, `ValidationRequiredAnswer`).

## Contract changes (additive; `CONTRACT-CHANGE(synthesis)`)

- `Investigation.judgement` (`UpgradeJudgement`), stage `synthesize` in the audit enum, `upgrade.AdvisoryReading`.

## Open questions

- None blocking. For the commander: the strict "safe needs complete advisories" rule (decision 4) makes every
  judgement `validation-required` / `do-not-proceed` when any security source is unread; say if a weaker rule is wanted.

## Test status

- `go build ./... && go vet ./... && go test ./...`: green (see the final commit).
- Cleanup: smoke state clones, the exchange directories and `bin/` removed; the local Temporal dev server I started
  for the history recording (port 7299) is stopped. No kind cluster was created.
