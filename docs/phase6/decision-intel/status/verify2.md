# Lane verify2: status (Milestone 4 fix; ledger D-0025)

Branch `p6di/verify2` (from `p6-decision-intel` @ b5d7252d). Brief: `briefs/verify2.md`.

## Design (decided 2026-10-27)

1. **Non-discriminating arms.** Under a `default-changed` rule only objects that leave the changed field (the rule's
   subject path) unset change between FROM and TO. An arm predicted `new-behaviour` whose object sets that field
   explicitly cannot contrast FROM with TO by construction (an explicit new default already has the new behaviour at
   FROM): it is classified non-discriminating (`ProofArm.NonDiscriminating`, the reason), still run and recorded, and
   excluded from the verdict. The contrast then comes from the omitted-field arms predicted `new-behaviour`; with
   none, the exercise is inconclusive. Rules of other change kinds (an explicit value can discriminate) are unchanged.
2. **Scope refinement.** Formalization (`formalize/v2`, stage producer `formalize@v2`) proposes, deterministically
   (no model judgement), a refinement candidate per rule hypothesis and catalog consumer role R of its subject kind
   when the run's consumer graph has established `reads-file` edges reading the key R's consumer trusts from a Secret
   an object of the subject kind produces. The refined rule = the behaviour rule with `exposure ∧ role(R)` and
   `overlap ∨ outside(R)`; the candidate carries `ProposedVerification.Refinement` (base hypothesis, base rule digest,
   role, outside fields, trust key, the trace edges) and the method `consumer exercise (role R)`.
3. **Verification of a refinement:** the declared consumer exercise for R, with the catalog's role-absent observation
   (`consumers[].outside`); confirmed only when, besides the usual consumer outcome, a role-absent arm is observed
   without harm (verified at FROM and TO). Undetermined role-absent ⇒ inconclusive; harm ⇒ refuted. A role field
   without an evidenced default stays undecidable for objects that omit it (a limitation, not a boundary failure).

## Done

- (in progress)

## In progress (exact next step)

- Domain contract (additive), then verify, then formalize v2, then CLI, tests, docs.

## Decisions

## Files touched outside ownership

## Contract changes (additive)

## Open questions

## Test status
