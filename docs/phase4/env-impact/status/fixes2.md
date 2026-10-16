# Status — lane `fixes2` (Claude Opus 5.5)

Branch `p4ei/fixes2` from `p4-env-impact` @ de2b5110 (merged again at 73bb58ba/a3f62868 and 39653a88),
worktree `.claude/worktrees/p4ei-fixes2`. Task (commander): fix review findings F1–F5 on the merged fix 3
(CRD description class, 635ca18e / merge 8c7d74f2), then the queued F6 and F7. Ownership:
`internal/render`, `internal/investigate/{bundle,validate,renderdiff}` (+ `run` for F7, assigned by the
commander). Integrity: FLEET rules. I never opened `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`,
never ran the eval or the scorer, and made only generic fixes, each with its own synthetic test.

## Done (all green: `go build ./... && go vet ./... && go test ./...`)
| # | Finding | Fix | Tests |
|---|---|---|---|
| F1 | every description change self-anchored an in-scope `scope:rendered-unexplained` item (release: `bundle/change.go` skip; environment: `renderdiff/index.go` self-anchor) | unexplained documentation anchors an **out-of-scope `scope:documentation`** item at both scopes, so it stays correlated and is never review noise; the domain rejects the rule on any other class | bundle `TestChangeCRDDescriptionIsOutOfScopeDocumentation`; renderdiff `TestEnvironmentCRDDescriptionIsNeverInScopeNoise` (fails on the old code); domain `TestEnvironmentItemsValidate` (+documentation), `TestDocumentationScopeOnlyForDescriptionText`, record-shape case |
| F2 | `correlate.go` refused every description change, so exposure was dropped | rule `documents-field`: a description links only to a change naming the property it documents (`render.DocumentedField`, through `items`/`additionalProperties`; identifier ≥ 6 chars); no other rule, never the word "description" | render `TestCorrelateCRDDescriptionByDocumentedField`, `TestDocumentedField`; bundle `TestChangeCRDDescriptionLinksTheChangeOfItsField` |
| F3 | a schema property named `description` was classed as documentation | class needs the keyword (parent ≠ `properties`) with text/absent values | render `TestCRDFieldNamedDescriptionIsStructure` (added/removed/retyped field + its own doc keyword) |
| F4 | DESIGN §2.2.1 guard on `manifest:rendered-correlated` deleted | restored, keyed on the class (`changeFacts.Correlated` carries path + class) | scope `TestManifestationRules` (+3 cases); bundle field-link test; validate `TestRenderDiffSilenceNeverClearsBehaviour` description-path case restored (documents-field shape), next to the `.type` case; `TestRenderDiffDescriptionOnlyIsHarmless` correlated + requested |
| F5 | `status/fixes.md:126` credited fix 4 with "not-affected 0→17" | correction note added there: 16 of 17 were F1 noise items | — |
| F6 | not-run/error records dropped the requested coverage → unlinkable, audit lost | `domain.ValidationRecord.EchoRequested`: undecided records carry exactly the requested subjects/paths/rendered changes (decisive records unchanged); called by the shared validate builder and the runtime validator; `Investigation.ValidateAgainst` rejects an undecided record not covering its request's subjects | validate `TestUndecidedRecordsEchoTheRequest` (all five validators, via `conform`), `TestEchoRequestedLeavesDecisiveRecords`; runtime `TestUndecidedRecordsEchoRequestedRenderedChanges` (not-run + error). All three fail when the echo call is removed. |
| F7 | report stamps dropped, not moved | `ImpactReport.Stamps` / `WithStamps` (round trip); run record `reportStamps` (generatedAt + retrievedAt per evidence id), excluded from `ContentDigest`/`Digest` like `createdAt`; enrichment AI `generatedAt` stays (see Decisions) | domain `TestImpactReportStampsRoundTrip`; run `TestRunRecordDigestIgnoresReportTimestamps` (stamps kept per run, digests equal) |
| +  | found on the way: `correlatedOf` decided the rendered-correlated path rule per rendered change, so a change linked only by a text mention borrowed another change's subject match and became render-decided | per link | bundle `TestRenderedCorrelatedIsPerLink` (fails on the old code) |
| +  | found on the way (measured): identical real runs still gave **different record digests**, because the digests hashed every validation's `checkedAt`. Fix 2's test missed it: its fake validator emits no records | `Digest`/`ContentDigest` zero `checkedAt` in a copy; the stored record keeps it | run `TestRecordDigestsIgnoreOnlyStamps`; end-to-end 8/8 identical digests (below) |

Commits: 553e1af8 (F1–F4 code), tests, F5 correction, F6, F7, checkedAt, docs, status (see `git log`).

## Diagnostics (pipeline re-runs on each case's input.yaml + env/; class counts only)
Method: `bin/ri -state <scratch> -offline investigate -repo <case>/env -kubernetes <k> -env-name <id>
<product> <from> <to>`, once per case. The fields come from `input.yaml` only. The scratch state holds
symlinks to the shared, read-mostly caches. Counts come from the stored bundle and run record. No eval, no
scorer, no ground truth. `base` = `p4-env-impact` @ 39653a88 (built in a temporary detached worktree,
since removed); `after` = this branch.

| case | phase | desc changes | desc correlation | desc items (scope) | in-scope items | findings A/PA/IE/NA | eligible |
|---|---|---|---|---|---|---|---|
| payments-eu/-us | base | 28 | 28 rendered-anchor | 28 rendered-unexplained (in scope) | 46 | 2/20/7/17 | 17 |
| payments-eu/-us | after | 28 | 6 documents-field, 22 rendered-anchor | 22 documentation (out of scope) | 18 | 2/10/5/1 | 1 |
| ingress-nginx ×4, kube-prometheus-stack ×2 | base = after | 0 | — | — | unchanged | unchanged | unchanged |

- The 28 description items left scope entirely. In base they were 12 potentially-affected and 16
  not-affected/eligible noise items. Uncorrelated changes: 0 in both phases.
- Exposure is restored (F2). The rotationPolicy item (ci-86548e9a2300) now has 1 correlated rendered
  change (its own field's documentation), 4 paths and 12 components; in A′ and base it had 0/0/0. The
  revisionHistoryLimit item (ci-f86a125e0d17) gets its field's documentation the same way. Both stay
  `controller-behaviour` and move from insufficient-evidence to potentially-affected (R3 with paths).
  Neither can clear through the description: the F4 guard and the validator's silence rule prevent it.
- The other documents-field links go to out-of-scope items (fields `profile`, `revisionHistoryLimit` of
  CertificateRequest, `config`). `config` is a generic word that the Vault note happens to contain. That
  is the same 6+ character identifier precision as mentions-field, and the link lands on an out-of-scope
  item: no finding, no review noise.
- Reproducibility: before the checkedAt fix, 0/8 cases gave equal record digests across two identical
  runs; the only difference was `validations[].checkedAt`. After the fix, 8/8 match. Stored records keep
  `reportStamps` (e.g. 69 retrievedAt entries on payments-eu) and every `checkedAt`.
- F6/F7 change no class counts (final runs are identical to the after-F1–F4 runs).
- Correction to an earlier diagnostic in this file: the first baseline (pre-merge, with the GLM values fix
  still in) differed on the ingress/kps cases only because of that fix and its revert, not because of this
  lane.

## Decisions (with why)
- F1: documentation no change names becomes an out-of-scope `scope:documentation` item, mirroring
  `scope:routine-self-artifact`. It stays accounted for: it is not "uncorrelated", so it does not block
  approval for every investigation whose chart rewords CRD docs. It is never review noise.
- F2: the bar is the same as mentions-field (an identifier of 6+ characters), applied to the documented
  property, not the keyword. No other rule may link documentation.
- F6: the record echoes the request as a union with the validator's own coverage. Each validator emits at
  most one undecided record per call, so ids cannot collide. Widening an undecided record can only add
  gaps: a not-run never covers; an error on an exposed subject can only move R4 to R3.
- F7: the stamps live on the run record (`reportStamps`), not in an audit event or a separate artifact.
  `Digest` covers audit details and refs, so either of those would have broken record reproducibility
  again. The enrichment AI `generatedAt` stays in the identified content: complete AI provenance requires
  it (FLEET; `Provenance.Validate`), and it is the model answer's own time, which the answer cache replays
  unchanged, so it is not a run stamp.
- The runtime validator (lane runtime, merged) received the one-line `EchoRequested` call. That edit is
  unavoidable for "every ValidationRecord".

## Files touched outside ownership
- `internal/domain/investigation.go`, `internal/domain/impact.go` (+ tests): `ScopeDocumentation`,
  `RenderedClassDocumentation`, scope shape/class checks, `EchoRequested`, the undecided-echo run-record
  rule, `ReportStamps` / `Stamps` / `WithStamps`, `Investigation.ReportStamps` (+ its rule). Commander-owned
  contract; all additive, marked `CONTRACT-CHANGE(fixes2)`.
- `schemas/investigation.schema.json` regenerated (`reportStamps`; no other schema changed).
- `internal/investigate/runtime/outcome.go`: one line (`EchoRequested`), plus a test in `runtime_test.go`.
- `docs/phase4/env-impact/status/fixes.md`: the F5 correction. `docs/INVESTIGATE.md`: CRD schema
  documentation, undecided coverage echo, run-record stamps.

## Contract changes
- `domain.ScopeDocumentation = "scope:documentation"`: an out-of-scope anchored item, allowed only on
  `crd-description-changed` (`domain.RenderedClassDocumentation`).
- `domain.ValidationRecord.EchoRequested(subjects, paths, renderedChanges)`. `Investigation.ValidateAgainst`:
  an undecided record must cover its request's subjects.
- `domain.ReportStamps`, `ImpactReport.Stamps` / `WithStamps`, `Investigation.ReportStamps` (excluded from
  the run-record digests).
- Render rule `documents-field` (in `render.ChangeLink.Rule`'s list).

## Open questions (for the commander)
- DESIGN.md is commander-owned and was not edited here. Proposed text:
  - §1.3 new row: `scope:documentation` | no | a `crd-description-changed` rendered change (CRD schema
    documentation text) whose documented field no change names; anchored as an out-of-scope item so it
    is correlated (never "uncorrelated"); documentation configures nothing.
  - §2.2.1 `manifest:rendered-correlated`: "…on a path that is not description text" → "…by a correlated
    change whose class is not `crd-description-changed`" (same intent, keyed on the class); add rule
    `documents-field` (description → the change naming its field; never decisive).
  - §2.6: "an undecided record echoes the requested coverage (EchoRequested)".
  - §2.9: `reportStamps`; record digests exclude createdAt, audit times, reportStamps and validations'
    checkedAt.
- The eval lane should re-score (arm A″). Expected movement is on the cert-manager cases only, plus record
  digest reproducibility.

## Test status
- `go build ./... && go vet ./... && go test ./...` green at the final commit (2026-10-16).
- Mutation checks (each fails on the old code, then restored): per-link path rule, env-scope
  documentation, F6 echo (validate and runtime).
