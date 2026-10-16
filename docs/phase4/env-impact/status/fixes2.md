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

- F1–F4 tests (commit after 553e1af8): render (field named description; DocumentedField; documents-field
  only), renderdiff env scope (out-of-scope documentation item, correlated, nothing in scope), domain
  (scope:documentation shape + only on the documentation class), validate (silence test description case
  restored, harmless test correlated + requested). Mutation-checked: per-link test and env-scope test fail
  on the old code.
- F5 status/fixes.md correction committed.

## In progress (exact next step)
- F6: ValidationRecord echoes requested coverage on not-run/error. Then F7 (report timestamps as
  provenance), final diagnostics, docs (INVESTIGATE.md), cleanup.

## Diagnostics (pipeline re-runs on each case's input.yaml + env/; class counts only)
Method: `bin/ri -state <scratch> -offline investigate -repo <case>/env -kubernetes <k> -env-name <id>
<product> <from> <to>` per case (fields read from input.yaml only), scratch state with symlinks to the
shared read-mostly caches; counts from the stored bundle + run record. No eval, no scorer, no ground truth.
`base` = current p4-env-impact (1ccb476f, incl. the values revert); `after` = this branch.

| case | phase | desc changes | desc correlation | desc items (scope) | in-scope items | findings A/PA/IE/NA | eligible |
|---|---|---|---|---|---|---|---|
| payments-eu/-us | base | 28 | 28 rendered-anchor | 28 rendered-unexplained (in scope) | 46 | 2/20/7/17 | 17 |
| payments-eu/-us | after F1–F4 | 28 | 6 documents-field, 22 rendered-anchor | 22 documentation (out of scope) | 18 | 2/10/5/1 | 1 |
| ingress-nginx ×4, kube-prometheus-stack ×2 | base = after | 0 | — | — | unchanged | unchanged | unchanged |

- The 28 description items left scope entirely (base: 12 potentially-affected + 16 not-affected/eligible
  noise). 0 uncorrelated changes in both phases.
- Exposure restored (F2): the rotationPolicy item (ci-86548e9a2300) has 1 correlated rendered change
  (its own field's documentation), 4 paths and 12 components (A′/base: 0/0/0), and the
  revisionHistoryLimit item (ci-f86a125e0d17) gets its field's documentation the same way. Both stay
  controller-behaviour and move insufficient-evidence → potentially-affected (R3 with paths). Neither can
  clear through the description (F4 guard + validator silence rule).
- Other documents-field links go to out-of-scope items (fields `profile`, `revisionHistoryLimit` for
  CertificateRequest, `config`). `config` is a generic word that the Vault note happens to contain — the
  same 6+ character identifier precision as mentions-field. It lands on an out-of-scope item: no finding,
  no review noise.

## Decisions (with why)
- F1: a description change nothing documents becomes an OUT-OF-SCOPE `scope:documentation` item (release
  and environment scope), mirroring `scope:routine-self-artifact`. Why: it stays accounted for (not
  "uncorrelated", so it does not block approval for every investigation whose chart rewords CRD docs) and
  it is never review noise. The domain rejects the rule on any other class, so a real change cannot be
  filed away as documentation.
- F2: the rule `documents-field` links a description only to a change that names the documented property
  (looking through `items`/`additionalProperties`; 6+ character identifier, same bar as mentions-field).
  No other rule may link documentation (the literal word "description" never counts).
- F3: the class needs the `description` keyword with text/absent values and a parent that is not
  `properties`.
- F4: manifest:rendered-correlated is keyed on the correlated change's class (changeFacts carries
  path + class), restoring DESIGN §2.2.1.
- Found on the way, fixed (bundle/change.go correlatedOf): the "path rule" for
  manifest:rendered-correlated was aggregated per rendered change, so a change linked only by a text
  mention inherited another change's subject match and became render-decided (silence could then clear a
  behaviour change). Now per link; TestRenderedCorrelatedIsPerLink (fails on the old code).

## Files touched outside ownership
- `internal/domain/investigation.go`: `ScopeDocumentation`, `RenderedClassDocumentation`, shape + class
  checks (commander-owned contract; additive, marked CONTRACT-CHANGE(fixes2)); tests in
  investigation_scope_test.go.
- `docs/phase4/env-impact/status/fixes.md`: the F5 correction.

## Contract changes
- `domain.ScopeDocumentation = "scope:documentation"` (out-of-scope anchored item; only on
  `crd-description-changed`). Additive; DESIGN §1.3 needs a row (proposed text in "Open questions").
## Open questions
- DESIGN §1.3 row proposal (commander-owned file, not edited here): `scope:documentation` | no | a
  `crd-description-changed` rendered change (CRD schema documentation text) no change names the field of:
  anchored as an out-of-scope item so it is correlated (never "uncorrelated"); documentation configures
  nothing. The documentation of a field a change names links to that change (`documents-field`) and never
  makes it render-decided (§2.2.1).
## Test status
