# Lane `backdoor`: Milestone 4 safety fix, every route that changes active knowledge names the facts it affects (Opus)

Branch `p6di/backdoor`, worktree `.claude/worktrees/p6di-backdoor`. Ledger D-0029. Read `_common.md` first, then
`M4-IMPLEMENTATION.md`, and `evidence-m4-negative-controls-build.md` (sections 3–4 and "Findings outside the brief").

## The gap (found while building negative controls)

- **The rule today:** the promotion gate's "name every active fact on the subject" (supersedes / coexists) applies
  only to **hypothesis-sourced** review items.
- **The back door:** a human `correct` through `Queue.Decide` on an **ordinary** fact-review item of the same candidate
  minted a second **active** fact on the same subject, with a contradicting applicability and consequence, while
  naming none of the active facts.
- **The consequence:** active facts that contradict each other can coexist silently. In evaluation the stricter one won
  (fail closed), but nothing reported the contradiction.

## Required: minimal and safety-only; no new verification machinery

1. **Every route that creates or changes an active fact names the others.** That includes accept, correct and
   duplicate, on hypothesis-sourced *and* ordinary items, and the FileStore-level promotion helpers the queue uses.
   Every route must name every other **active** fact on the same subject (product, release, subject) as `supersedes`
   or `coexists`. Otherwise the decision is refused, with the list. The review UI shows the choice for ordinary items
   too, as it does for hypothesis items.
2. **Contradiction reporting.** When two active facts on one subject coexist with incompatible change or applicability
   or consequence:
   - **loading knowledge** produces a warning naming both;
   - **the knowledge join** never lets either clear an item; it records a `knowledge-contradiction` gap that names both.

   Fail closed, and reported. A `coexists` relation that a human recorded explicitly is allowed only when the two facts
   are compatible.
3. **Unchanged:** the proof requirement for hypothesis items. Do **not** add new proof requirements for ordinary
   human corrections (that is the existing human-verification design), but they must satisfy (1).
4. **Tests:**
   - the route-B reproduction from the build report is refused;
   - naming the facts is accepted;
   - a contradiction produces a warning and a gap, and no clearance;
   - compatible coexistence is allowed;
   - stored facts and decisions still load. Report any stored pair that would now count as contradicting, by id,
     without changing them.
5. **Docs** and your status file. No live model runs, no kind.

FLEET.md applies. Keep it small.
