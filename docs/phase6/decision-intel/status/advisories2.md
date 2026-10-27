# Lane `advisories2` status (D-0011, Milestone 2 fix)

Branch `p6di/advisories2` from `p6-decision-intel` @ f48b8c18. Brief: `briefs/advisories2.md`. **State: done**, ready
for the commander to merge.

## The defect and the fix

`UpgradeEdge.Security.candidates` chose `any-line` and `clean` from the releases **above the current version**. A later
patch of the current line, below the target, could therefore be picked. A test fixture now reproduces this: on the
base code `any-line` and `clean` come out as the current line's `v1.0.2` for an upgrade `v1.0.1 → v1.1.0`.

Now (`internal/upgrade/security.go`, `candidates`):

| kind | chosen from | predicate |
|---|---|---|
| `target-line` | stable releases at or above To, on To's line | introduces no assessed advisory From is free of (removes every regression) |
| `any-line` | stable releases at or above To | the same |
| `clean` | stable releases at or above To | affected by no assessed advisory |
| `current-line` (new, information) | stable later patches of From's line, strictly between From and To | affected by none of the advisories persisting into To, and introduces none |

- Statements say "at or above <To>". `current-line` says it is "below <To>: information for a defer or stay decision,
  not an upgrade target".
- `versionsConsidered` counts the releases at or above To.
- The pool-empty gap reads "no known release list at or above <To>".
- `target-line` and `any-line` are computed only when To has a regression (as before). `current-line` is computed
  only when advisories persist.
- Unassessed advisories are listed on every candidate, `current-line` included (unchanged behaviour). Every candidate
  keeps its `removes`, `leaves` and `charts`. `CandidateVersions` also looks up the `current-line` version's charts.
- Text reports and the agent dossier label the entry
  `current-line <line> (information for a defer or stay decision, not a target)` (`domain.VersionCandidate.Label`).

## Decisions (with why)

- **`current-line` is kept.** It is about 25 lines and the brief allows it. It is restricted to releases strictly below
  To, so it never overlaps the upgrade-target pool, including for an upgrade within one line.
- **"Persisting" for `current-line`** uses every assessed advisory that affects both endpoints, including one
  classified without an evidence record. It fails closed: such an advisory still blocks the entry.
- **`removes` and `leaves` stay relative to To for every kind**, `current-line` included. This is one meaning,
  documented in `docs/SECURITY_ADVISORIES.md`.
- **The label lives on the domain type** (`VersionCandidate.Label`) so the agent package needs no new import of
  `internal/upgrade`.

## Files touched

- `internal/upgrade/security.go`: candidate pool and statements, `current-line`.
- `internal/upgrade/security_render.go`: header "at or above <To>"; uses `Label()`.
- `internal/domain/edgesecurity.go`: `CandidateCurrentLine`, doc comments, `Label()`.
- `internal/domain/schemagen/meta.go`: enum and description. `schemas/upgrade-edge.schema.json` and
  `schemas/impact-report.schema.json` were regenerated.
- `internal/investigate/agent/dossier.go`: outside ownership, one line (uses `Label()`).
- Tests:
  - `internal/upgrade/security_test.go`;
  - `internal/investigate/agent/dossier_security_test.go`;
  - `internal/upgrade/testdata/cert-manager.golden`, a synthetic unit fixture. Only the gap wording changed.
- `docs/SECURITY_ADVISORIES.md`.

## Contract changes (additive)

- Enum value `current-line` for `VersionCandidate.kind`.
- `versionsConsidered` now counts the releases at or above the target, not the releases above the current version.
  This is a meaning change to an M2 field. No stored edge carries the section yet (see the advisories lane's
  regression comparison).

## Tests

- **Shared fixture:** it now includes `v1.0.2`, a later patch of the current line below the target. Every existing
  candidate test runs with it.
- **`TestSecurityCurrentLinePatchIsNotATarget`:** `v1.0.2` removes both the regression and the persisting advisory. It
  is not chosen as `any-line` or `clean`; it is the `current-line` entry. The test also checks the unassessed
  advisory on each candidate and the text render.
- **`TestSecurityNoCandidateAtOrAboveTarget`:** no release at or above the target qualifies, so the test checks each
  explicit "none" statement verbatim. A current-line patch still qualifies as information.
- **`TestSecurityCurrentLineWithinTargetLine`:** a patch upgrade. `current-line` is searched below the target only.
- **Updated:** the classification, across-lines and chart tests, and `CandidateVersions` (no longer `v1.0.2`).
- **Old code:** with the base `security.go`, the six candidate tests fail, and `any-line`/`clean` pick `v1.0.2`.
- **Full suite:** `go build ./... && go vet ./... && go test ./...` is green (46 packages ok). The schemas are
  regenerated. The e2e app goldens are unchanged.

## Regression comparison

No live runs (brief). The stored runs carry no edge security section, so rescoring cannot exercise this code (see
`status/advisories.md`). The comparison is the unit fixtures above, run against the base code and against this branch.

## Open questions

None.

## Cleanup

No clusters or servers were started. The temporary base worktree in the scratchpad was removed. There is no `bin/`.
