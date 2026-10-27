# Lane `semverify`: Milestone 4 E4–E5 and the M2 misread finding (Opus)

Branch `p6di/semverify`, worktree `.claude/worktrees/p6di-semverify`. Ledger D-0023. Read `_common.md` first, then
`PLAN-M4.md`. You implement the "Lane `semverify`" table (changes 1–3).

## Ownership

You own:
- `internal/domain/proof.go` (the new method kind);
- `internal/investigate/verify/` (the semantic-check);
- `internal/investigate/runtime/` and `products/exercises/` (`admission-dry-run`);
- a report-only check over agent `behaviour` entries sourced `upstream`;
- docs.

## Required

1. **Proof method `semantic-check`.**
   - It extracts the value tokens a claim asserts: defaults, enumerated or allowed values, character classes,
     before/after values, booleans. Each must appear in the cited passage with the same polarity.
   - A token the citation lacks, or a flipped polarity, makes the claim **unsupported**. Polarity flips include
     "accepts" vs "rejects", "true" vs "false", and "enabled" vs "disabled".
   - Where the claim cannot be decided deterministically, it is routed to a **human** (not to a model).
   - It counts toward a proof only when it passes, and does not replace the required citation-check and
     rule-exercise methods.
2. **Report-only check of agent assessments.** For each `behaviour` entry sourced `upstream`, run the same check
   against its cited evidence, and record `supported` / `unsupported` / `undetermined`.
   - It is shown in reports and in the synthesis input, as a new disagreement kind `behaviour-unsupported-by-citation`.
   - It never changes classification.
   - Add the disagreement kind to `internal/synthesize` precompute as an additive hunk.
3. **Runtime exercise `admission-dry-run`.**
   - Server-side dry-run (`kubectl apply --dry-run=server` or the driver's equivalent) of one environment object
     against the target version's admission webhook in an isolated kind cluster.
   - The outcome is `observed`: accepted, or rejected with the message.
   - It is declared in the product exercises catalog.
   - Opt-in, bounded like `runtime.isolated-upgrade@v1`, cluster deleted afterwards.
4. **Tests.**
   - Claims that add a token, flip polarity, or agree with the citation.
   - A character-class claim checked against a passage listing the allowed characters.
   - The report-only check on a stored-style assessment fixture.
   - The dry-run accepted and rejected, on kind fixtures; skip with a clear message when kind or docker is absent.
5. **Docs:** `docs/INVESTIGATE.md` (verification), the exercises docs, and your status file.

FLEET.md applies, including generic logic only (no product, case, file or field names in Go logic or prompts), runtime clusters named `ri-<lane>-*` and deleted after use, and the files you never open (`GATE-M*.md`, `M*-RESULT.md`, `expected.yaml`, `NOTES.md`, `observed/`, run reports and scores). No live model runs on the development cases; the commander runs the demo and the gate. Three M4 lanes run in parallel: `applicability`, `semverify`, `formalize`. Keep your `domain` changes additive and list them in your status file. The commander merges in the order applicability → semverify → formalize; when told, `git merge p6-decision-intel` into your branch and resolve.
