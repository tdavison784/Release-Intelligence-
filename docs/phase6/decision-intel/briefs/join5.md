# Lane `join5`: Milestone 4 fix, evaluate rendered objects; membership defaults in the pin scope (Opus)

Branch `p6di/join5`, worktree `.claude/worktrees/p6di-join5`. Ledger D-0028. Read `_common.md` first, then
`M4-IMPLEMENTATION.md` and the M4 status files (`status/applicability.md`, `status/verify3.md`, `status/verify4.md`).

## Two defects found by the commander on a development evaluation (generic description)

**D1. The knowledge join evaluates source files, not rendered objects.**
- The environment declares a Certificate in a kustomize **base** and patches a field in an **overlay** that the
  environment's entrypoint renders.
- The knowledge join (`impact:knowledge-exposed`) matched "<object> leaves <field> unset", with evidence pointing at
  the base file. But the rendered object, what is deployed, sets the field through the overlay patch.
- So the join judged an object that does not exist in that form. It fails closed here (exposed), but the same mistake
  fails **open** wherever an overlay removes or overrides a setting the base has.

**The fix:**
- Every applicability evaluation goes over the **rendered** objects of the environment's entrypoints, the same objects
  the impact paths and the validators use: the knowledge join, the pin scope, and the override inspection.
- Source-file evidence is mapped through the render, citing the file and line that produced the rendered field, which
  is the patch when one applies.
- When an object's rendered form is unavailable, evaluation is **unknown**, never the base file's view.
- Check every caller that evaluates conditions against environment objects for the same mistake.

**D2. Observed role-membership defaults are not applied in the pin scope.**
- `pin:knowledge-exposure@v2`, from a promoted fact that carries a role-membership default (PD-6: "role field unset ⇒
  outside role R", recorded through a confirmed proof), reported an object that leaves the role field unset as
  **undecidable**.
- The fact's default should have decided that object's membership (outside the role).
- **Fix:** the pin scope and the knowledge join apply a fact's membership default through its confirmed proof,
  exactly as the impact evaluator does. Trace why it was not applied (lookup of the proof, digest matching, fact field
  plumbing), and fix that.

## Required

- **Tests:**
  - an overlay patch that sets a field the base leaves unset (rendered: set);
  - an overlay that removes or overrides a base field, which must not be read as the base's value (the fail-open
    direction);
  - no render available: unknown;
  - membership default applied in the pin scope;
  - no membership default without a confirmed proof;
  - contradiction blocks.
- **Regression:**
  - re-score the stored B0, C1 and D1 runs; report differences as counts and rule ids, without opening reports or
    scores;
  - any change in classification must be explained by D1/D2 and must never weaken safety;
  - historical records still validate.
- **No live model runs on development cases.** The commander re-runs part B.
- **Docs:** `docs/IMPACT.md`, `M4-IMPLEMENTATION.md`, and your status file.

FLEET.md applies, including generic logic only and the files you never open.
