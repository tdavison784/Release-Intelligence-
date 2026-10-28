# Lane `verify3`: Milestone 4 fix, role tokens, log noise, observed defaults (Opus)

Branch `p6di/verify3`, worktree `.claude/worktrees/p6di-verify3`. Ledger D-0026. Read `_common.md` first; that
includes `PRODUCT-DECISIONS.md`, especially **PD-5**, new. Then `M4-IMPLEMENTATION.md` and the M4 status files
(`status/applicability.md`, `status/semverify.md`, `status/formalize.md`, `status/verify2.md`).

## Three problems found by the commander on a development run (generic description)

**A. The semantic check refutes a scope refinement on its role tokens.**
- A refinement's statement names the catalog role's condition values, for example a boolean role field and its
  outside-role value.
- Those values come from the catalog role and are established by the consumer exercise, not by the base rule's
  upstream citation.
- So `semantic-check` reported "boolean `true` / `false` does not occur in the cited passages" and refuted a
  refinement whose consumer exercise had passed.
- **Fix.** Tokens that belong to conditions sourced from the catalog role, or established by an exercise, are
  excluded from `semantic-check`. Record each exclusion, with the method that establishes the condition instead.
  Everything else is checked exactly as now.

**B. Controller log lines count as behaviour signatures.**
- An exercise judged "with the mitigating condition the behaviour still changed at TO", and the only differences were
  controller log lines such as "unable to fetch certificate that owns the secret".
- An identical rule passed in another cluster. Logs are nondeterministic noise.
- **Fix.** Behaviour signatures are object state only: data, spec and status fields of the exercised objects and
  their outputs. Log lines are recorded as diagnostics and never decide an arm. Document it, and test it with a fixture
  where only logs differ.

**C. Observed defaults (PD-5).**
- A scope refinement's outside-role condition can compare a field that many real objects leave unset. With no
  evidenced default those objects stay undecidable, which is correct.
- PD-5 allows the default to be established by **observation**. The verifier's omitted-field arm on the real
  controller shows an unset field behaving as a given value, for example as the outside-role value.

**Implement:**
1. **When a confirmed proof's omitted-field arm for field F** observed the same outcome as the arm with F = V, record
   an **observed default** F = V on the proof. Record its exercise evidence, controller version, fixture values, and
   the limitation "observed on the fixture's controller configuration".
2. **The applicability evaluator accepts an observed default for F only through a confirmed proof of the same rule
   digest**, carried into the verified fact on human promotion. A default that is merely proposed, or observed in an
   inconclusive or refuted proof, is ignored, so the field stays unknown.
3. **Disagreement.** An observed default that contradicts an upstream-cited default for the same field and version
   blocks both. They are recorded as a contradiction, never silently resolved.
4. **The promotion gate is unchanged:** human only. The promoted fact shows the observed default and its limitation
   to the reviewer.

## Required

- **All three fixes, with tests:**
  - A: a role token is excluded, and a non-role token still fails.
  - B: logs only produce no change; a real data change is detected.
  - C: an observed default is recorded only from a confirmed omitted-field arm; it is used only through a confirmed
    proof of the same digest; it is ignored when inconclusive or refuted; a contradiction with a cited default
    blocks; and the promoted fact carries it.
- **History:** stored proofs and facts still validate, and existing confirmed proofs are not re-judged.
- **No live model runs on development cases.** Opt-in kind tests skip without kind or docker. Clusters are deleted.
- **Docs:** `M4-IMPLEMENTATION.md`, `docs/INVESTIGATE.md` and `docs/IMPACT.md` (the evaluator), and your status file.

FLEET.md applies, including generic logic only and the files you never open.
