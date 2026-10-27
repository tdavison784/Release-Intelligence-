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
- Tests: `internal/formalize/formalize_test.go`, `internal/investigate/agent/mitigation_test.go`.

## In progress (exact next step)

- CLI: `ri investigate formalize <inv>` (records + optional `-knowledge` export through `ExportHypotheses`), a
  `verify` run source that finds formalized hypotheses, `-formalize` on `ri eval -investigations`; then docs.

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
   field as a value. Until the `applicability` lane's evidenced default is merged, the prompt asks for an undecidable
   leaf.
5. **Environment basis** in formalization = the run's verified setting claims about the item and its validation
   records (no graph-subject resolution offline).

## Files touched outside ownership

- `internal/domain/investigation.go`: the `ProposedVerification` field/type and its validation hunk (owned field).
- `schemas/investigation.schema.json` (regenerated).

## Contract changes (additive; `CONTRACT-CHANGE(formalize)`)

- `Hypothesis.proposedVerification` (`ProposedVerification`).

## Open questions

- None blocking.

## Test status

- `go test ./internal/formalize/ ./internal/investigate/agent/ ./internal/domain/...`: green.
