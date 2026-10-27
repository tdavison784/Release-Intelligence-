# Lane `upstream`: Milestone 2 A1/A3/B6, upstream behavioural evidence at the target tag (Opus)

Branch `p6di/upstream`, worktree `.claude/worktrees/p6di-upstream`. Ledger D-0009. Read `_common.md` first, then
`PLAN-M2.md` (the "Lane `upstream`" table and §4), `M1-IMPLEMENTATION.md`, and `status/consumer*.md` and
`status/scoped.md`.

**Change from the plan:** prompt v10 (scoped verdicts) did not become the default. Build **prompt v11 from v9**, the
current default, with `scopedVerdict` off. v1–v10 stay byte-frozen.

## Ownership

You own:
- `internal/investigate/agent/`: `request_evidence` fulfilment, the fetch port, budgets, the assessment unavailable
  gaps, and `prompts/investigate-agent/v11/`;
- the run-artifact recording in `internal/investigate/run/pipeline.go`;
- additive `domain` fields if needed, with schemas regenerated;
- docs.

Additive hunks only in `internal/gitsrc/` (reuse `Files` / `Dirs`; add nothing network-facing beyond them).

## Required

1. **PLAN-M2 lane `upstream` changes 1–6, each with its §4 tests.**
   - `request_evidence` gains `kind: upstream-file` (`{repo, ref, path}` or a blob URL at a tag) and
     `kind: upstream-search` (`{repo, ref, dir, pattern}`: an in-memory regex over a `gitsrc.Dirs` listing, with result
     caps).
   - **Allowed only:**
     - immutable refs (`IsImmutableRef`), preferring the target's tag (`def.TagFor`), with the current version's tag
       also allowed for comparison;
     - repositories in the product's own code repositories (`CodeReposOf`).
   - **Refused:** anything else, with the reason.
2. **Recording.**
   - **Fulfilled:** an `upstream-document` run artifact, with `OriginUpstream`, URI = blob URL at the tag, digest,
     producer `evidence-fetch@v1`, and `Fulfilled` set on the request. It is citable at once with verbatim quotes.
     The bundle is never changed.
   - **Unavailable** (not found, offline cache miss, refused, budget): `RequestUnavailable` with the reason. The tool
     layer then records "behaviour at <tag> unavailable: <reason>" in the item's assessment `unknown`. Never silent.
3. **Bounds.** Per-item fetch count and bytes, within `MaxReadBytes`. Search caps on files scanned, matches and context
   lines. All set in v11's `meta.json`.
4. **Prompt v11** = v9 plus the tool's new behaviour, plus one generic rule. When the outcome depends on upstream
   behaviour (validation, defaulting, parsing, a feature gate) and the change text does not state it exactly, fetch
   the implementation, docs or tests at the target tag before concluding; cite them, or record the behaviour as
   unavailable. Remembered behaviour is never evidence. Use generic wording: no product, file or field names.
5. **Reproducibility.** Fetches go through the existing `fetch` and `gitsrc` caches, so a recorded state replays
   offline with identical bytes. The replay driver's result digests must match; prove it with a test.
6. **No live runs on development cases.**
   - A live smoke on a non-development case is optional and unscored. If you do one, clone the shared state, set
     `GITHUB_TOKEN=$(gh auth token)` in that command's environment only, and never print or commit the token.
   - The commander records the evaluation state and runs the scored D-EU check.
7. **Docs:** a v11 and evidence-fetch section in `M1-IMPLEMENTATION.md` (or a new `M2-IMPLEMENTATION.md`),
   `docs/INVESTIGATE.md`, and your status file.

Leave the default prompt at v9 (`pinnedDefaultPrompt`). FLEET.md applies, including the files you never open
(`GATE-M*.md`, `M*-RESULT.md`, `expected.yaml`, `NOTES.md`, `observed/`, run reports and scores).
