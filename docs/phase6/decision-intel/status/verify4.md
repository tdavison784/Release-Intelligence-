# Lane verify4: status (Milestone 4 fix; ledger D-0027)

Branch `p6di/verify4` (from `p6-decision-intel` @ 278788c1). Brief: `briefs/verify4.md`. PD-6 ("the role cannot form"
is an observed outcome), building on PD-5 / lane verify3.

## Done

- Read the brief chain (FLEET ×2, `_common.md`, MISSION, PRODUCT-DECISIONS PD-5/PD-6, PLAN-M4, M4-IMPLEMENTATION,
  `status/verify3.md`).
- Started the existing `observed-default` kind case on a scratch clone of the warm state, to read the real object
  states of the in-role probes (conditions, events) from the stored observation artifacts before fixing the rule.

## In progress (exact next step)

- Design (below), then code: `verify/formation.go` (outcome), `probe.go` (membership default), domain type,
  evaluators, promotion, review UI, tests, kind case.

## Design

- **Outcome `role-cannot-form`** (per arm and version, consumer observation only, decided in `verify`, from the
  run's stored object observations + the consumer check; never logs): the run completed (at TO the upgrade
  completed), the arm's subject exists, was applied and is ready, its trust output was captured (and, when re-issued,
  answered), and the dependent output the consumer verifies was not produced (the consumer check could not re-issue
  a consuming object or read the verified output), while a consuming object of the arm reports, in every
  observation of that version, a reconciled negative state (not ready / a failure condition, with its reason).
  Anything else that is not verified/failed stays `undetermined` (timeouts, controller failures, missing objects).
- **Role-membership default** (`domain.RoleMembershipDefault`, on a confirmed proof only): F unset ⇒ outside role R,
  when the in-role unset probe is `role-cannot-form` at FROM and TO and its base arm (role value) formed the
  relationship at both. Used by the evaluators **only** for the refinement's role / outside leaves (equals the role
  or outside value), never as a field default; contradiction with a schema / cited / observed default that puts
  unset in the role blocks it.

## Decisions

## Files touched outside ownership

## Contract changes (additive; CONTRACT-CHANGE(verify4))

## Open questions

## Test status
