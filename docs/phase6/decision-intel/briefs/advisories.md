# Lane `advisories`: Milestone 2 A2, security advisory and version intelligence (Opus)

Branch `p6di/advisories`, worktree `.claude/worktrees/p6di-advisories`. Ledger D-0008. Read `_common.md` first, then
`PLAN-M2.md`. You implement the table "Lane `advisories`" (changes 1–8) and its tests in §4. Build the smallest thing
that works and that a reviewer can audit.

## Ownership

You own:
- `internal/github/` (advisories, ranges, the new global listing);
- `internal/ingest/advisories.go`;
- `internal/upgrade/` (advisory rules, version intelligence);
- the additive `domain` fields (`Locator` ecosystem/package, `UpgradeEdge.Security`), with schemas regenerated;
- the catalog locator kind and its validation;
- the package identity in `products/*.yaml`;
- the security section in `internal/impact/`;
- the edge security summary in the agent dossier;
- docs for all of the above.

For `internal/policy/evaluate.go`, keep to an additive hunk. `advisory:regression` must be at least as strict as
`affects-target`. Nothing may become less strict.

## Required

1. **PLAN-M2 changes 1–8, each with its §4 tests.**
   - **Generic logic only.** Package identities are catalog data. The rule for choosing them (ecosystem plus
     canonical module or package name of the shipped binary) goes in the docs.
   - **Candidate versions are computed from every known stable release:** the lowest on the target's line removing
     every regression (or "none"); the lowest on any line removing every regression; the lowest free of every known
     advisory.
   - **Each candidate lists** the advisories it removes and those it leaves, with evidence ids. Where the product
     ships as a chart, it also gives the chart version.
2. **Ranges.** Handle explicit version lists (`v1.2.0,v1.2.1`) as OR-ed exact versions. Handle prerelease lower bounds
   (`>= X-beta.0, < Y`) so that a stable X is matched. A Go pseudo-version range (`< 0.0.0-<date>-<sha>`) must not
   match ordinary releases. When two copies of one GHSA id disagree, prefer the machine-readable one and keep both
   sources in the evidence.
3. **Unavailability.** An advisory source that could not be read (an error, throttling, or an offline cache miss)
   becomes an edge warning and a report gap. It is never an empty advisory list. Test an offline miss.
4. **Fixtures.**
   - Tests use httptest and checked-in JSON fixtures. You may make read-only GitHub API calls to capture the real
     response shape for fixtures, using `GITHUB_TOKEN=$(gh auth token)` in the command's environment only. Never print
     or commit the token.
   - Trim fixtures to what the tests need.
   - Live calls belong to fixture capture only. The commander records the evaluation state.
5. **No change to unrelated classification.** The existing B0/C1 runs must re-score unchanged, except for findings the
   new security rules add. List those explicitly in your status file, from a `-rescore` of a stored run, without
   opening any report or score file: compare classifications programmatically and report only counts and rule ids.
6. **No live investigation runs.** No `ri eval -investigations` with a model.
7. **Docs:** the advisory sources and rules in `docs/` (find the existing advisory section), and the README command
   list if a flag changes.

FLEET.md applies, including the files you never open (`GATE-M*.md`, `M*-RESULT.md`, `expected.yaml`, `NOTES.md`,
`observed/`, run reports and scores).
