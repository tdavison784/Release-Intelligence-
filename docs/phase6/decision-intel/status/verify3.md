# Lane verify3: status (Milestone 4 fix; ledger D-0026)

Branch `p6di/verify3` (from `p6-decision-intel` @ 79922ecc). Brief: `briefs/verify3.md`. PD-5 (observed defaults).
Design, tests and known limits: `M4-IMPLEMENTATION.md`, lane `verify3`.

## Done

1. **A. Role tokens** (commit 3d724924): `semantic-check` excludes the tokens of the refinement clause that are catalog
   role / outside-role values, recording each with its condition and the method that establishes it
   (`consumer exercise (role R)`); everything else is checked as before. The clause text lives in
   `domain.ScopeRefinement.Clause` (formalize/v2 replays byte-identically).
2. **B. Log noise** (3d724924): log lines and warning events are `ProofArm.diagnostics`, never signatures; producers
   `verify.exercise@v3`, `verify.exercise-consumer@v3`.
3. **C. Observed defaults** (aa0807bb): default probes for refinement role fields without an evidenced default;
   `ProofRecord.observedDefaults` on confirmed proofs only, from a distinguishing observation; carried into the fact on
   human promotion; used by the impact join and the pin scope only through a confirmed proof of the same digest
   (`UsableObservedDefaults` + `impact.VerifyFacts` record check); a contradiction with a schema / cited / other
   observed default blocks both; shown in the review UI and by `ri knowledge decide`.
4. **Tests** for all three (see M4-IMPLEMENTATION.md); schemas regenerated (`knowledge-record.schema.json`).
5. **History**: 241/241 checked-in facts verify, the stored proof validates, the Phase 5 stored proofs still validate
   (existing tests), `m4rescore` 1004/1004 identical.
6. **Docs**: `M4-IMPLEMENTATION.md` (lane section), `docs/INVESTIGATE.md` (semantic check role tokens, signatures are
   object state, observed defaults), `docs/IMPACT.md` (observed defaults in the evaluator).

7. **Real kind run** (2026-10-27, `TestRefinementKind` case `observed-default`, scratch clone of the warm state, real
   cert-manager controller 1.17.2 → 1.18.0, catalog fixture, synthetic refinement with no stated `spec.isCA` default;
   no case read; 551 s, cluster `ri-verify-7b9ead0f` deleted). The refinement is **confirmed** as before (probes never
   decided). **No observed default was recorded**, correctly:
   - role-absent observation: `isCA` unset, `false` and `true` all verified at FROM and TO, so the observation does
     not distinguish the values and shows nothing about the default;
   - in-role observation: with `isCA` unset or `false`, the probe's leaf was never re-issued (the would-be CA cannot
     sign), so both probes are `undetermined` and their observation is left out.

## In progress (exact next step)

- Nothing: lane done; awaiting the commander's merge.

## Decisions

- **The observation must distinguish the values** (local default, fail closed): an observed default F = V is recorded
  only when the unset probe matched the arm with F = V *and* the other-value probe behaved differently in the same
  observation. Matching a value that behaves like the other one shows nothing about the default; recording it would
  let the evaluator decide objects that are undecided. Why the probe pair (unset + other value) per base arm: it is the
  smallest arm set that both shows the unset behaviour and shows that the observation can tell the values apart.
- **Probes only for scope-refinement role fields**: that is where an undecidable omitted field was already a limitation
  (verify2); an ordinary rule that cannot predict an omitted field still fails its boundary fixtures (E2,
  applicability lane), unchanged.
- **Ignored, not rejected**: a fact whose observed default is not established by a confirmed proof of its digest keeps
  being used without it (brief: "ignored, so the field stays unknown"); `VerifyFacts` reports the ignored default
  among its rejections. Fact validation does not reject a fact for it.
- **Warning events count as diagnostics** too, not only log lines: the brief defines signatures as object state (data,
  spec, status); events are neither, and are as nondeterministic. Environment runs are unchanged.
- **Proof id**: observed defaults enter the id digest only when present, so every earlier proof keeps its id.
- **Semantic-check exclusion is by the clause verbatim**, not by value anywhere in the statement: a role value the base
  statement asserts itself is still checked.

## Files touched outside ownership

- `internal/formalize/refine.go` (uses `domain.ScopeRefinement.Clause`; output unchanged).
- `internal/knowledge/decide.go` (carry observed defaults on promotion; the gate is untouched).
- `internal/impact/condition.go`, `internal/impact/knowledge.go`, `internal/investigate/validate/pins.go`,
  `internal/applicability/applicability.go` (evaluator use; additive).
- `internal/reviewui/hypothesis.go`, `internal/reviewui/templates/detail.html`, `cmd/ri/knowledge.go` (display).
- `internal/investigate/runtime/observe.go` (`Degradation.Diagnostic`, additive).
- `cmd/ri/verify2_kind_test.go` (opt-in case `observed-default`).
- `schemas/knowledge-record.schema.json` (regenerated).

## Contract changes (additive; CONTRACT-CHANGE(verify3))

- `ProofArm.diagnostics`, `ProofArm.probe` (`ArmProbe`), `ProofRecord.observedDefaults`, `VerifiedFact.observedDefaults`
  (`ObservedDefault`, `ObservedDefaultLimitation`, `ObservedDefaultEstablished`, `UsableObservedDefaults`),
  `ScopeRefinement.Clause`, `RoleFieldsText`; `impact.ConditionOptions.Observed`; `applicability.ObservedFor`,
  `WithObserved`; `semantic.CheckAssertionExcluding`; `verify.CheckSemantics` gains the refinement parameter.

## Open questions

- **For the commander / product owner (blocks C on the development run, not the lane):** with the current catalog
  fixture, PD-5 cannot establish the `isCA`-style role default. The role-absent observation does not depend on the role
  field, and in the role's observation an object without the role cannot form the consumer relationship at all
  (`undetermined`). That "no relationship forms" result is the same for unset and for the outside value, and different
  from the role value, so it may be evidence. It is not counted, because `undetermined` also covers timeouts and missing
  objects. Two options: (1) count a declared "relationship cannot form" outcome (for example the leaf is never issued
  while the role arm's leaf is) as an observable outcome for probes only; (2) a catalog observation in which the role
  field changes the consumer outcome. The lane did neither: either one changes what counts as evidence. For the demo: the stored refuted refinement proof is decisive, so `ri verify hypotheses -runtime
  -again` is needed to re-verify it with A, B and C.

## Test status

- `go build ./... && go vet ./... && go test ./...` green.
