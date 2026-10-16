# Status — lane `fixes2` (Claude Opus 5.5)

Branch `p4ei/fixes2` from `p4-env-impact` @ de2b5110, worktree `.claude/worktrees/p4ei-fixes2`.
Task (commander, after the fixes-lane review): fix review findings F1–F5 on the merged fix 3 (CRD
description class, 635ca18e / merge 8c7d74f2). Ownership: `internal/render`,
`internal/investigate/{bundle,validate,renderdiff}`. Integrity: FLEET rules; never open
`eval/investigations/*/{expected.yaml,NOTES.md,observed/}`; generic fixes only; the eval is not run here.

## Findings being fixed
- F1 description-only env changes self-anchor as in-scope `scope:rendered-unexplained` items
  (renderdiff/index.go self-anchor + bundle/change.go skip) → 28 noise items on cert-manager.
- F2 correlate.go refuses every description change → the genuine link (the description of the field an
  item names) is lost; exposure silently dropped (DESIGN §2.4).
- F3 diff.go classifies a schema *property named* `description` as documentation.
- F4 scope.go lost the DESIGN §2.2.1 guard (manifest:rendered-correlated needs a non-description path).
- F5 status/fixes.md:126 credits fix 4 with "not-affected 0→17"; 16 of 17 were F1 noise items.

## Done
- (setup) worktree verified clean on p4ei/fixes2 @ de2b5110.

## In progress (exact next step)
- Baseline diagnostic: build bin/ri, run `ri investigate` on each case's input.yaml + env/ with a scratch
  state (symlinked read-mostly caches), record class counts.

## Decisions (with why)
## Files touched outside ownership
## Contract changes
## Open questions
## Test status
