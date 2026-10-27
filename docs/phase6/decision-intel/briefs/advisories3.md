# Lane `advisories3`: Milestone 2 fix, advisories whose patched versions contradict their range (Opus)

Branch `p6di/advisories3`, worktree `.claude/worktrees/p6di-advisories3`. Ledger D-0012. Read `_common.md` first, then
`PLAN-M2.md`, `docs/SECURITY_ADVISORIES.md`, and `status/advisories.md` and `status/advisories2.md`.

## The defect (generic description)

Some advisories state one vulnerable range across several release lines but list per-line patched versions. One
real shape is `>= 1.2.0, <= 1.4.2` with `patched: 1.3.6, 1.4.3`. The patched version 1.3.6 lies **inside** the
advisory's own stated range, so the advisory contradicts itself.

RI follows the range, which is the strict and correct reading, and silently skips 1.3.6 as a candidate. It does not
say that the advisory is inconsistent, or that the patched list implies an alternative.

## Required

1. **Detect it.** For each advisory with a machine-readable range, check every patched version against the range. A
   patched version inside its own vulnerable range is an **advisory-consistency** finding.
2. **Report it.** Add an edge-security gap and an edge warning naming the advisory, the range and the contradicting
   patched versions, for example: "GHSA-x lists 1.3.6 as patched but its range >= 1.2.0, <= 1.4.2 includes it; check
   the advisory". Include it in the impact report's security section and in the dossier summary.
3. **Candidates stay strict.** The range still decides. Where a candidate would differ if the patched list were right,
   its statement says so. For example: "If GHSA-x's patched list is right, 1.3.6 also qualifies; it is not chosen
   because the stated range includes it." Record the alternative as `alternatives` with a reason, never as the
   candidate.
4. **Tests.**
   - A contradicting advisory: you get the gap, the warning and the alternative, and the candidate is unchanged.
   - A consistent advisory: no finding.
   - Explicit version lists: a patched version that is also listed as vulnerable is also a contradiction.
   - Prerelease bounds are handled.
5. **Docs:** `docs/SECURITY_ADVISORIES.md` and your status file.

No live runs. FLEET.md applies, including the files you never open.
