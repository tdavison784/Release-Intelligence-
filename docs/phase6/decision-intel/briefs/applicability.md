# Lane `applicability`: Milestone 4 E1–E3 (Opus)

Branch `p6di/applicability`, worktree `.claude/worktrees/p6di-applicability`. Ledger D-0022. Read `_common.md` first,
then `PLAN-M4.md`. You implement the "Lane `applicability`" table (changes 1–4) with tests.

## Ownership

You own:
- `internal/impact/condition.go` and the knowledge join;
- `internal/validate/pins.go` and `override.go` (scope and coverage);
- a shared three-valued field helper;
- `internal/classify/policy.go` coverage, as an additive hunk;
- the verify boundary fixtures (`internal/investigate/verify/` fixtures, `products/exercises/fixtures/`);
- the `Condition` default fields in `internal/domain/semantic.go`.

## Required

1. **One three-valued evaluator, for the join and the pins.**
   - Unset plus a comparison with no established default gives **unknown**, never false or true. It carries a reason
     and what would settle it.
   - Schema `default` values from the bundle's CRDs resolve unset fields, as pins already do.
   - Multiple objects: no object counts as excluded unless it is provably out of scope.
2. **Evidenced defaults.**
   - A rule's `Condition` may carry a default for a field, with citations: a CRD schema default, or an upstream
     controller default (an upstream artifact at the tag, the M2 fetch).
   - An evidenced default makes unset decidable. An unevidenced default is ignored and stays unknown.
   - Validation rejects a default without a resolvable citation.
3. **Boundary fixtures** for omitted fields, plus the shielded alternatives that `verify/consumer.go` lists but never
   exercises. A rule that only works when every field is explicit must fail its verification fixtures (E2).
4. **Exact coverage (R2.2).** A passing pin or rule covers exactly the rendered changes on its own field (object kind
   + path), including a `crd-description-changed` on that field path. That needs a field-scoped documentation-only
   check. Mutation tests must show that all of these stay gaps:
   - another field's description change in the same CRD;
   - a schema or default change on the field;
   - an unrelated resource.
5. **No fail-open.**
   - Prove with tests that no input that was unknown or affected before becomes not-affected without an evidenced
     default.
   - Re-score the stored B0, C1 and D1 runs: classifications must be identical, or stricter only. Report counts and
     rule ids, without opening report or score files.
6. **Docs:** `docs/IMPACT.md` and `docs/INVESTIGATE.md` where the evaluators are described, plus your status file.

FLEET.md applies, including generic logic only (no product, case, file or field names in Go logic or prompts), runtime clusters named `ri-<lane>-*` and deleted after use, and the files you never open (`GATE-M*.md`, `M*-RESULT.md`, `expected.yaml`, `NOTES.md`, `observed/`, run reports and scores). No live model runs on the development cases; the commander runs the demo and the gate. Three M4 lanes run in parallel: `applicability`, `semverify`, `formalize`. Keep your `domain` changes additive and list them in your status file. The commander merges in the order applicability → semverify → formalize; when told, `git merge p6-decision-intel` into your branch and resolve.
