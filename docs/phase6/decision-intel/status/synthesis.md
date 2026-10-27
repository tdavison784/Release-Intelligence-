# Lane synthesis: status (Milestone 3, ledger D-0017)

Branch `p6di/synthesis` (from `p6-decision-intel` @ 91957f82). Brief: `briefs/synthesis.md`. Plan: `PLAN-M3.md`
§3 changes 1–7 with the §4 tests. Design, tests and known limits: `M3-IMPLEMENTATION.md` (written as the work lands).

## Done

- Read the brief chain (FLEET ×2, `_common.md`, MISSION, PRODUCT-DECISIONS, PLAN-M3, M1/M2-IMPLEMENTATION). No
  label-bearing file opened.

## In progress (exact next step)

- Change 1: `domain.UpgradeJudgement` + `Investigation.Judgement` in `internal/domain/judgement.go`.

## Decisions (with why)

1. **Synthesis is not part of `investigate.Config`.** The config digest is part of the investigation id; putting the
   flag there would change the id, findings and audit of every run. The switch lives on the runner
   (`Runner.Synthesize`) and the workflow input (`orchestrate.Input.Synthesize`); the model is the synthesizer's
   configuration and is recorded in the judgement's provenance.

## Files touched outside ownership

- (none yet)

## Contract changes

- (none yet)

## Open questions

- (none)

## Test status

- not run yet
