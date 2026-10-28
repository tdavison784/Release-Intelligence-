# Lane `scoped`: prompt v10, scoped verdicts (Opus)

Branch `p6di/scoped`, worktree `.claude/worktrees/p6di-scoped`. Ledger D-0007. Read `_common.md` first, then
`PLAN-M1.md`, `M1-IMPLEMENTATION.md`, and the M1 lanes' status files (`status/consumer*.md`).

## Why (product owner decision, 2026-10-26)

Under v9 the agent keeps every verdict provisional while anything is unproven. In practice it answers
`potentially-affected` for environments whose reasoning clearly differs, because operational unknowns stay open:
reload behaviour, pinning outside the repository, runtime Secret contents. Engineers need a verdict they can act on,
with its limits stated. The product owner approved **scoped verdicts**: the agent commits to the class its evidence
supports **within a declared scope**, and puts the scope and any precondition in the headline. Example shape:
"Not affected within the consumers this repository declares, provided the workloads reload rotated certificates."

Agent verdicts remain unverified AI prose and never reach the classifier (`verdict_policy_test.go`). Proven
clearance stays with verification (mission M4). This is calibration, not a policy change.

## Required

1. **Prompt v10**, copied from v9 with only the verdict rules changed; v1–v9 stay byte-frozen.
   - **Scope and preconditions.** The agent declares the scope it investigated, such as the consumers declared in the
     environment, the rendered objects, or the cited upstream evidence. It separates two kinds of unknown:
     - **(a) unknowns inside the scope that the breaking mechanism depends on.** These keep the verdict
       `potentially-affected` or `insufficient-evidence`, as today.
     - **(b) preconditions outside the scope.** Runtime or operational facts the repository cannot state (reload
       behaviour, pinning by external systems, runtime object contents). These do not block a scoped class; they
       become explicit preconditions.
   - **Committing.** When every traced consumer and every effective setting in scope is accounted for, the agent
     commits to `affected` or `not-affected` within scope and lists the preconditions.
   - **Never.** The agent never uses model memory for in-scope facts (A1). Untraced consumers stay in-scope unknowns.
     The M1 completion check is unchanged.
2. **Assessment fields**, additive:
   - `headline`: one sentence with the class, the scope and the preconditions;
   - `scope`: what was examined;
   - `preconditions`: a list.

   Validate that a committed class (`affected` / `not-affected`) carries a non-empty `scope`, and that a
   `not-affected` with in-scope unknowns or untraced consumers is rejected once, with the list. Regenerate the
   schemas.
3. **Tests.**
   - v10 loads; v1–v9 digests are unchanged; the generic-word check covers v10.
   - The assessment validation: committed class without scope is rejected; not-affected with an in-scope unknown is
     rejected once; preconditions only, accepted.
   - Policy isolation is extended to the new fields.
   - A v10 replay test on a scripted fixture.
4. **Default prompt.** Leave the default at v9. v10 waits for its check: `pinnedDefaultPrompt` in `claims_test.go`
   is the mechanism.
5. **No live runs** on development cases. A live smoke on a non-development case is optional and unscored.
6. **Docs:** update `M1-IMPLEMENTATION.md` (a v10 section) and `docs/INVESTIGATE.md`; keep your status file current.

FLEET.md applies, including generic wording only (no case, product or file names in the prompt) and the files you never
open.
