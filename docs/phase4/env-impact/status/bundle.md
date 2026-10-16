# Status — lane `bundle` (Claude Sonnet 5.5, agent ri-bundle)

Branch `p4ei/bundle`, worktree `.claude/worktrees/p4ei-bundle`. Brief: `briefs/bundle.md`.
State: **IN PROGRESS**. Resume from: this file + brief + DESIGN §2/§7 + `git log`.

## Done
- (nothing committed yet)

## In progress / next step
1. `internal/investigate/store` (CAS + record store + tests).
2. `internal/investigate/bundle`: scope/manifestation rules, ChangeCollector, SnapshotBuilder, Assembler, tests.
3. docs/INVESTIGATE.md "Evidence bundle" section; groundtruth-input bundle test via warm cache.

## Decisions
- `store.New(root)`: `root` is the *state dir*; the store lives in `<root>/investigations/` (DESIGN §2.3).

## Files touched outside ownership
- none yet (docs/INVESTIGATE.md "Evidence bundle" section only, as allowed by the brief)

## Contract changes
- none yet

## Open questions
- none

## Test status
- not run yet
