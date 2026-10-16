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

## Commander queue (added during the lane)
- F6 every ValidationRecord, incl. not-run and error, echoes the requested coverage (subjects, paths,
  rendered changes) exactly as requested, with the reason; tested.
- F7 report timestamps were dropped, not moved (domain/impact.go IdentifiedContent): keep per-evidence
  retrievedAt as provenance/audit detail outside the identified content; handle
  Enrichments[].Provenance.GeneratedAt consistently.
- Review add-on: cover environment scope for the description flood (renderdiff/index.go self-anchor);
  restore the description-path case of TestRenderDiffSilenceNeverClearsBehaviour.
- `git merge p4-env-impact` done (b3201ea5): the GLM layered-values fix is reverted there (73bb58ba);
  nothing here depends on it.

## Done
- (setup) worktree verified clean on p4ei/fixes2 @ de2b5110.
- Baseline diagnostic (pre-fix code, de2b5110): reproduces arm A' exactly — see "Diagnostics".
- F1–F4 code + bundle/scope tests (commit 553e1af8); also a per-link path-rule fix found on the way
  (see Decisions).

## In progress (exact next step)
- render tests (F3 field named description, DocumentedField, documents-field correlate), domain test for
  scope:documentation, renderdiff env-scope test, restore the silence test; then F5, F6, F7, diagnostics.

## Decisions (with why)
## Files touched outside ownership
## Contract changes
## Open questions
## Test status
