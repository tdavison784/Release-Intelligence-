# Lane formalize: status (Milestone 4 Workstream D, ledger D-0021)

Branch `p6di/formalize` (from `p6-decision-intel` @ 971636bc). Brief: `briefs/formalize.md`. Plan: `PLAN-M4.md`
"Lane `formalize`" changes 1–4. Design, tests and known limits: `M4-IMPLEMENTATION.md` (formalize section).

## Done

- `internal/formalize`: `Precompute` (citable evidence, uncertainties, environment basis), `Floor` (non-invention,
  strict decoding), `Formalizer` (one call through `llm.Cache`, bounded retry, recorded failure), `Record` +
  file `Store` beside the run records; prompt `formalize/v1` (generic, digest-pinned).
- Spec checks lifted out of `agent/mitigation.go` into the pure `agent.CheckMitigation(ctx, MitigationEnv, …)` and
  `agent.BoundsWithinRun`; the session calls them with its own lookups (v8 behaviour unchanged; existing v6/v8
  mitigation tests pass).
- `domain.Hypothesis.ProposedVerification` (+ type `domain.ProposedVerification`), schemas regenerated.
- Tests: `internal/formalize/{formalize,handoff}_test.go`, `internal/investigate/agent/mitigation_test.go`
  (non-invention, unrepresentable kept, scripted-exchange replay, bounded retry/failure, policy isolation, hand-off
  through `ExportHypotheses`, v8 path, generic prompt).
- CLI: `ri investigate formalize <inv-id> [-assessment] [-knowledge] [-formalize-model] [-formalize-exchange]
  [-claude-bin]`; `-formalize` (+ model/exchange) on `ri eval -investigations` (records in the work store and the
  stored case as `formalizations.json`); `ri verify hypotheses` reads formalization records first (run source
  `formalizedRuns` in `cmd/ri/formalize.go`).
- Docs: `M4-IMPLEMENTATION.md` (started; formalize section), `docs/INVESTIGATE.md` "Evidence formalization", README
  command list.
- Smoke (unscored, non-development): pre-computation over stored v9 run `M1s-v9-shop-de-20261020-f6c23796`; no model
  call.

- Merged `p6-decision-intel` (applicability + semverify; no conflicts) and integrated evidenced defaults: the model
  states `Condition.Default` citing upstream evidence of the assessment and `discharges` the matching default
  uncertainty; the floor rejects a default citing evidence outside the assessment, non-upstream evidence, or
  unestablished per `FieldDefault.Unevidenced`; default citations join the hypothesis' citations (export carries them).
  Test `TestEvidencedDefault`. Prompt `formalize/v1` edited in place (never run or recorded).

## In progress (exact next step)

- Nothing: lane done; awaiting the commander's merge.

## Decisions (with why)

1. **Formalization never touches the run record.** Records are stored at
   `<state>/investigations/formalizations/<inv>/<fmz-id>.json`. Run records are immutable (mission §13) and stored
   runs are digest-checked; and `inv.Hypotheses` feeds the classifier's agent outcome, so formalized hypotheses must
   not go there (policy isolation).
2. **Citable evidence** = the assessment's verified citations + upstream-behaviour citations + the item's change
   text when a behaviour quotes it (`change-text`) + the evidence of the run's verified setting claims about the
   item (its effective configuration). Everything else, including other items' evidence, is rejected.
3. **Uncertainties** = the assessment's unknowns, not-checked/unavailable behaviours no unknown states, settings
   `unknown` or `inherited-default` (no evidenced default exists yet), and preconditions. Each must be kept by an
   *unrepresentable* record (its `keeps`, or an undecidable leaf of its partial rule whose `needed` names the
   uncertainty id; the stage writes the uncertainty's text there). A rule cannot carry an undecidable leaf (spec
   check), so a claim that depends on an uncertainty is a record. Every uncertainty also travels with every rule
   (`ProposedVerification.Uncertainties`, and appended to the requested verification so the human reviewer sees it).
4. **Defaults:** a rule with `any(unset, <value> on the same field)` (or `not(set)`) is rejected: it treats an unset
   field as a value. After the `applicability` merge, a default is stated as `Condition.Default` with upstream
   citations from the assessment's evidence (and discharges its `default` uncertainty); without such evidence, an
   undecidable leaf. Default citations are unioned into the hypothesis' citations (a composition of checked ids, not a
   repair) so the exported proposal carries the evidence the fact needs.
5. **Hand-off without changing `ExportHypotheses`:** a view of the run whose `Hypotheses` are the formalized ones;
   verification finds them through a run source reading the formalization records first.
6. **`claude -p` client reused** from `internal/synthesize` (`synthesize.ClaudePrint`) rather than copied.
7. **Environment basis** in formalization = the run's verified setting claims about the item and its validation
   records (no graph-subject resolution offline).

## Files touched outside ownership

- `internal/domain/investigation.go`: the `ProposedVerification` field/type and its validation hunk (owned field).
- `schemas/investigation.schema.json` (regenerated).
- `cmd/ri/verify.go`: the run source wraps `verify.StateRuns` with `formalizedRuns` (+ usage line).
- `cmd/ri/eval_investigations.go`: `-formalize` flags, `invStored.Formalizations`, `invFormalize`, the
  `formalizations.json` part of a stored case (additive hunks).
- `README.md`, `docs/INVESTIGATE.md`: one command line and one section.

## Contract changes (additive; `CONTRACT-CHANGE(formalize)`)

- `Hypothesis.proposedVerification` (`ProposedVerification`).

## Open questions

- None blocking.

## Test status

- `go build ./... && go vet ./... && go test ./...`: green.
- Cleanup: no kind cluster or server was started; the smoke extraction and the scratch binary were in the session
  scratchpad and are removed; nothing large in the worktree.
