# Lane `advisories2`: Milestone 2 fix, candidate versions must be upgrade targets (Opus)

Branch `p6di/advisories2`, worktree `.claude/worktrees/p6di-advisories2`. Ledger D-0011. Read `_common.md` first, then
`PLAN-M2.md`, `docs/SECURITY_ADVISORIES.md` and `status/advisories.md` (merged at d73485b8).

## The defect (found by the commander on a recorded run; generic description)

`UpgradeEdge.Security.candidates` computes `any-line` and `clean` as the lowest qualifying release **above the
current version**. When the current version's own line has a later patch with no advisories, that patch is chosen:
a release **below the proposed target**. A patch of the line the engineer is leaving is not an alternative upgrade
target, so the `any-line` and `clean` candidates come out as current-line patches, not replacement targets.

The plan's wording ("the lowest release that removes every regression on any line") was ambiguous. This is the
intended meaning.

## Required

1. **Upgrade targets only.** Candidates are releases **at or above the proposed target**:
   - `target-line`: the lowest release on the target's line, at or above the target, that removes every regression.
     This is unchanged in effect.
   - `any-line`: the lowest release at or above the target, on any line, that removes every regression.
   - `clean`: the lowest release at or above the target that no assessed advisory affects.
   - Statements and docs say "at or above <target>".
2. **Current line, as information only.** Optionally add a `current-line` entry: the lowest later patch on the current
   version's line that removes the advisories persisting into the target. It is labelled information for a defer or
   stay decision, not a target. Keep it only if it is small. Downstream (M3) can use it for a "defer" recommendation.
3. **Tests.** Fixtures where the current line has a clean later patch below the target (it must not be chosen as
   `any-line` or `clean`); where no release at or above the target qualifies (an explicit "none" with its statement);
   and the existing candidate tests, updated.
4. **Unassessed advisories** (unreadable ranges) keep blocking "clean" claims as today, and are listed per candidate.
5. **Docs:** `docs/SECURITY_ADVISORIES.md` and your status file.

No live runs. FLEET.md applies, including the files you never open.
