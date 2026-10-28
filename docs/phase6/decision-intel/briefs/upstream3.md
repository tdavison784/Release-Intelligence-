# Lane `upstream3`: Milestone 2 fix, attempt the fetch before declaring behaviour not-checked (Opus)

Branch `p6di/upstream3`, worktree `.claude/worktrees/p6di-upstream3`. Ledger D-0014. Read `_common.md` first, then
`M2-IMPLEMENTATION.md`, and `status/upstream.md` and `status/upstream2.md`.

## The gap (generic description)

Under v12, `not-affected` can no longer rest on unestablished upstream behaviour, and that works. But with upstream
fetching configured and working, the agent often declares the deciding behaviour `not-checked` and takes a
`potentially-affected` or `insufficient-evidence` verdict, without ever calling `request_evidence`. The verdict is
safe but the evidence is never gathered, although it was one call away.

## Required: prompt v13 = v12 plus a feature flag (for example `fetchFirst`) in `meta.json`

v1–v12 stay frozen. Stored v12 transcripts must replay unchanged, so the check runs only under the new flag. The
default prompt stays v9.

1. **Check on `submit_findings`.** Applies when the session has an upstream port and the item is behavioural (the
   classification upstream2 uses). Any `behaviour` entry with source `not-checked`, when the session made no
   `request_evidence` (`upstream-file` / `upstream-search`) attempt for the item, is rejected **once**, whatever the
   verdict.
   - **The message:** "Upstream fetching is available for <repos> at <tags>: request the behaviour, or state in the
     entry why upstream source cannot answer it."
   - **To answer "cannot answer"** without a fetch, add an optional `reason` field on a `not-checked` entry. It is
     accepted on the resubmission and recorded as a gap.
   - **After an attempt,** the entry must be `upstream` (cited) or `unavailable` (with the request id), per v12's rules.
2. **Prompt text.** One generic sentence on the order: fetch first, then conclude.
3. **Tests.**
   - With a port and no attempt, `not-checked` is rejected once.
   - After an attempt, the entry must be `upstream` or `unavailable`.
   - Without a port (not configured), `not-checked` is accepted.
   - A non-behavioural item is unaffected.
   - Resubmission with a `reason` is accepted, with the gap.
   - v12 replays unchanged; the v13 replay works.
   - Policy isolation is unchanged.
4. **Docs:** `M2-IMPLEMENTATION.md`, `docs/INVESTIGATE.md` and your status file.

No live runs on development cases. FLEET.md applies.
