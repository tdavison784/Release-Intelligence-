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

1. **Non-discriminating arms** (`verify/discriminate.go`, `EvaluateArms`, `EvaluateConsumerArms`,
   `domain.ProofArm.NonDiscriminating`, `ProofArm.Contrast`, `ProofRecord.Validate`). Producers `verify.exercise@v2`,
   `verify.exercise-consumer@v2`. Commit d01bb30c.
2. **Role-absent observation** in the fixture catalog (`consumers[].outside`, validated; cert-manager `ca-trust`
   declares one, quotes fetched verbatim from upstream). Outside-role and omitted-role-field arms use it.
3. **Refinement-aware verification**: `verify.Mitigation.Refinement`, `verify.MitigationOf`, `ExerciseInput.Refinement`,
   refinement criterion in `EvaluateConsumerArms`, undecidable role field ⇒ limitation (not boundary failure),
   refinement limitation on every such proof.
4. **formalize/v2** (`prompts/formalize/v2`, `refine.go`, floor: `roleDefaults`, no model role narrowing; producer
   `formalize@v2`; `Record.RefinementNotes`; v1 frozen, digest pinned). CLI: `-formalize-prompt`, catalog roles wired
   into `ri investigate formalize` and `ri eval -investigations -formalize`.
5. **Tests** (see `M4-IMPLEMENTATION.md` lane verify2). Schemas regenerated.
6. **Docs**: `M4-IMPLEMENTATION.md` (lane section), `docs/INVESTIGATE.md` (non-discriminating arms, role-absent
   observation, scope refinements, formalize/v2), README command line.

7. **Real kind runs** (`TestRefinementKind`, opt-in, 2026-10-27, scratch clone of the warm state, real cert-manager
   controller and the catalog fixture, synthetic rules; no case read): **confirmed** 1.17.2 → 1.18.0 (269 s; 6 arms as
   predicted: shielded CA verified/verified, exposed CA verified/failed, role-absent non-CA and three omitted-`isCA`
   arms on the role-absent observation verified/verified); **refuted** (a refinement of a wrong base rule declaring
   `Always` shielding: shielded consumer failed/failed, 270 s); **inconclusive** 1.17.1 → 1.17.2 (exposed consumer
   still verifies at TO, 268 s). Clusters `ri-verify-*` deleted (`kind get clusters` empty).

## In progress (exact next step)

- Nothing: lane done; awaiting the commander's merge.

## Decisions

- **Deterministic candidates.** The stage, not the model, proposes refinements: the conditions are all catalog and
  trace facts, and only observation can establish the claim; a model judgement would add invention risk without
  evidence. The model may only state evidenced role-field defaults, and is forbidden to narrow a rule to a role.
- **Any explicit value** of a default-changed field is non-discriminating (not only the new default): a default change
  reaches only objects that leave the field unset. Only `default-changed` rules on a field of the fixture's subject
  kind are classified.
- **Non-discriminating arms still run** (recorded evidence, cheap in the same cluster) but never decide.
- **Refinement pass** needs one role-absent arm verified at FROM and TO; an undetermined one is recorded.
- **Unevidenced role default**: for a refinement, objects omitting the role field stay undecidable (limitation), as
  the brief asks; ordinary rules keep the applicability lane's boundary failure.
- **Cluster prefix** stays `ri-verify-` (the runtime's fixture prefix; its isolated-cluster guard recognises it).
- **ProofRecord.Validate** relaxation is backward-compatible: the principal `condition-absent` arm of stored proofs
  counts as before; other condition-absent arms count only when predicted to change and discriminating.

## Files touched outside ownership

- `internal/domain/proof.go`, `internal/domain/investigation.go` (additive fields/types, validation), `schemas/*`.
- `internal/formalize/*` (formalize lane's package: v2 prompt, refinement stage, floor additions).
- `cmd/ri/formalize.go`, `cmd/ri/eval_investigations.go` (roles, `-formalize-prompt`), `cmd/ri/verify2_kind_test.go`.
- `products/exercises/fixtures/cert-manager.yaml` (`outside:` block).
- `README.md`, `docs/INVESTIGATE.md`, `M4-IMPLEMENTATION.md`.

## Contract changes (additive; CONTRACT-CHANGE(verify2))

- `ProofArm.nonDiscriminating`; `ProposedVerification.refinement` (`ScopeRefinement`, `RoleField`, `TraceRead`,
  `RefinementMethodPrefix`); formalization `Record.refinementNotes`; fixture `consumers[].outside`.

## Open questions

- None blocking. For the demo: a refinement clears objects that omit the role field only with an evidenced role
  default, which `formalize/v2` can state only from upstream evidence the investigation holds.

## Test status

- `go build ./... && go vet ./... && go test ./...` green; kind runs above.
