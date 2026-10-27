# Lane verify4: status (Milestone 4 fix; ledger D-0027)

Branch `p6di/verify4` (from `p6-decision-intel` @ 278788c1). Brief: `briefs/verify4.md`. PD-6 ("the role cannot form"
is an observed outcome), building on PD-5 / lane verify3. Design, tests and known limits: `M4-IMPLEMENTATION.md`,
lane `verify4`.

## Done

1. **Outcome `role-cannot-form`** (52c56bc8, a58450d0; `verify/formation.go`): in-role default probes only, per
   version, from the run's object observations (settled + after the exercises) and the consumer check; requires a
   consuming object to report a failure in every observation; evidence = object states, conditions, warning events
   (`ProofConsumerResult.formation`), never logs; everything else stays `undetermined`. Never decisive; producer
   `verify.exercise-consumer@v4`.
2. **Observed role-membership default** (`domain.RoleMembershipDefault`, `verify.RoleMembershipDefaults`):
   "F unset ⇒ outside role R (observed)" on a confirmed proof only, when the role-value arm formed the relationship
   at FROM and TO, the unset probe is role-cannot-form at both, and the other value formed it at neither. Never a
   PD-5 field default.
3. **Used only for membership** (`applicability.MembershipFor` / `WithMembership`; impact join and pin scope): only
   the role / outside `equals` leaves; contradictions with schema, cited or observed defaults block; earlier
   contradictions stay.
4. **Guards**: confirmed proof of the same digest only (`UsableRoleMembershipDefaults`, `RoleMembershipEstablished`,
   `impact.VerifyFacts`), proof validation, human promotion carries it (gate unchanged), review UI + `ri knowledge
   decide` show it with its limitation.
5. **Tests**: see M4-IMPLEMENTATION.md (verify, impact, validate, reviewui); opt-in kind case `role-cannot-form`.
6. **Real controller data**: the baseline `observed-default` kind run (2026-10-27, cert-manager 1.17.2 → 1.18.0,
   scratch clone of the warm state, cluster `ri-verify-7b9ead0f` deleted) stored its observations; read through the
   new code offline (a throwaway test, deleted), both in-role probes are role-cannot-form at FROM and TO (the CA
   Issuer reports `Ready=False (ErrInvalidKeyPair): … certificate is not a CA`; the leaf stays
   `Ready=False (DoesNotExist)`, `Issuing=True`; its Secret never exists), giving
   `spec.isCA unset ⇒ outside role ca-trust (observed)` and no field default.
6b. **Live kind case `role-cannot-form`** with the new code (2026-10-27, same fixture `fx-063edd092de5`, 555 s,
   cluster `ri-verify-7b9ead0f` deleted): method `pass` (the refinement confirmed; probes never decide); in-role probes
   unset and `false` observed `role-cannot-form` at FROM and TO, evidence = the CA Issuer's
   `Ready=False (ErrInvalidKeyPair): certificate is not a CA` in all four observations, plus the leaf state and the
   missing leaf Secret; `RoleMembershipDefaults` → `spec.isCA unset ⇒ outside role ca-trust (observed)`; no field
   default.
7. **History**: 241/241 checked-in facts verify, the stored proof validates, `m4rescore` 1004/1004 identical.
8. **Docs**: `M4-IMPLEMENTATION.md` (lane section), `docs/INVESTIGATE.md` (outcome, membership default, real run),
   `docs/IMPACT.md` (membership leaves in the evaluator). Schemas regenerated (`knowledge-record.schema.json`).

## In progress (exact next step)

- Nothing: lane done; awaiting the commander's merge.

## Decisions

- **A reported failure is required** for role-cannot-form. The real leaf of a relationship that cannot form reads
  `Ready=False (DoesNotExist)` with `Issuing=True`, exactly like a late leaf; the decisive signal is the consuming
  issuer's failure condition. Not-ready-while-progressing is context only (`undetermined`).
- **Only the unset probe establishes membership**; the other-value probe must not form the relationship (else
  nothing: F does not decide membership as declared). An explicit value needs no default.
- **Membership leaves by shape** (F `equals` R's value or outside value, one value), since a refinement never names
  its role field elsewhere (`formalize.RefineRule`); any other predicate on F stays unknown.
- **Separate type, not an `ObservedDefault` variant**: the PD-5 field-default lookup cannot see it, so it cannot
  become a field default by construction.
- **Only in-role probes** get the new outcome; decisive arms and role-absent probes are unchanged.
- An established default **outside** the role agrees and decides the leaf by its own value.
- **Fixed a fail-open gap in verify3's `WithObserved`** (found while adding `Default.Contradiction`): a schema default
  contradicting the rule's cited default left the field unknown, but an observed default then settled it. Now any
  contradiction found earlier stays blocked (`impact.TestObservedDefaultKeepsAnEarlierContradiction`, which fails
  on the old code). Stricter only; `m4rescore` unchanged.

## Files touched outside ownership

The brief owns the verify fix; these are the PD-5 plumbing points verify3 also touched (additive):
- `internal/applicability/applicability.go` (`MembershipFor`, `WithMembership`, `Default.Contradiction`,
  `Default.Membership`).
- `internal/impact/condition.go`, `internal/impact/knowledge.go`, `internal/investigate/validate/pins.go` (evaluators).
- `internal/knowledge/decide.go` (carry on promotion; gate untouched).
- `internal/reviewui/hypothesis.go`, `internal/reviewui/templates/detail.html`, `cmd/ri/knowledge.go` (display).
- `cmd/ri/verify2_kind_test.go` (opt-in case `role-cannot-form`).
- `schemas/knowledge-record.schema.json` (regenerated).

## Contract changes (additive; CONTRACT-CHANGE(verify4))

- `domain.RoleMembershipDefault` (+ `Key`, `Statement`, `Same`, `Validate`, `RoleMembershipEstablished`),
  `ProofRecord.roleMembershipDefaults` (in the id only when present), `VerifiedFact.roleMembershipDefaults`
  (`UsableRoleMembershipDefaults`), `ProofConsumerResult.formation`, `domain.ConsumerRoleCannotForm`,
  `domain.BehaviourRoleCannotForm`; `impact.ConditionOptions.Membership`; `verify.RoleCannotForm`,
  `verify.RoleMembershipDefaults`.

## Open questions

- None blocking. For the demo: stored proofs with a decisive outcome are not re-judged, so
  `ri verify hypotheses -runtime -again` is needed to obtain a proof with the membership default.

## Test status

- `go build ./... && go vet ./... && go test ./...` green.
