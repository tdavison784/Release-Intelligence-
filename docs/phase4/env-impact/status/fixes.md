# Status — lane `fixes` (GLM-5.3, agent glm-fixes; Claude lane lead paused for a usage-limit window)

Branch `p4ei/fixes`. Brief: `briefs/fixes.md` (+ `_wave2-common.md`, `_wave1-common.md`, FLEET.md).
Defect list: EVAL-REPORT.md §5 (arm A) as routed by the commander; read from `p4ei/eval` (not merged into
`p4-env-impact` at lane start; read-only via `git show`, no merge).

## Done
- Lane setup: brief + FLEET + wave preambles + EVAL-REPORT §5 + status/eval.md read; baseline
  `go build ./...` + `go vet` green.
- Fix 1 (layered values files, EVAL-REPORT §5.1a) COMPLETE, commit a638c39a; full build/vet/test green.
  - `internal/env/valuesmerge.go` (new): `valuesMerge` — layered Helm coalesce (later source overrides
    earlier per key; mappings deep-merge; any other leaf replaces the subtree below its path), one
    effective `ValuesKey` per path with evidence from the file/line that set it; `valuesContainerPaths` /
    `valuesPairs` (merge-key + dup-last-wins aware, aligned with normalize.FlattenValues syntax);
    `loader.applyValuesFile` (per-file parse warnings unchanged); `loader.finalizeValues` (materialize
    under maxValuesKeys + image extraction from the MERGED keys — repository/tag siblings may come from
    different layers, as Helm merges them).
  - `internal/env/env.go`: layering warning at old L428 REMOVED; `loadValuesFiles` feeds the merge;
    `finalizeValues` called after manifests load; `Supplied.Values` now also true for inline-only values;
    values health: a values reference that could not be collected (`valuesNamed`) is supplied-with-gap
    (partial), never absent, and deliberately does NOT count as visibility. New loader fields
    `argoValueFiles` / `helmfileValues`.
  - `internal/env/installed.go`: `inlineValues` + `parametersToValues` route through the merge (inline
    wins over files; parameter value-mappings deep-merge).
  - `internal/env/repo.go`: helmfile values reference that is missing → warning + `valuesNamed`;
    `orderValuesFiles` (Argo valueFiles list order → name-convention alphabetical → helmfile list order);
    `argoValueFilesOf` collects Argo `helm.valueFiles` ($ref/ paths resolved against repo root, mirroring
    internal/render/source.go) in list order, collects referenced files the name convention misses, and
    warns on missing refs.
  - Tests: `internal/env/valuesmerge_test.go` new (deep-merge, leaf/map replacement, merge keys, merged
    images, missing helmfile ref, Argo list-order precedence incl. non-convention filename, missing
    $values ref), repo_test.go trio rewritten for merge semantics, bundle
    `TestSnapshotLayeredValuesFilesAreComplete` (gitops-repo input CollectionOK + merged facts with list
    order beating alphabetical), app/cmd e2e assertions updated, impact goldens regenerated
    (9 keys → 6 effective, warning line gone — reviewed).
- Fix 2 (run-record reproducibility, EVAL-REPORT §5.1b) COMPLETE, commit 175709ef; full build/vet/test
  green.
  - `domain.ImpactReport.IdentifiedContent()` (internal/domain/impact.go): the report with
    GeneratedAt and every evidence RetrievedAt zeroed (fresh slices — the caller's report is not
    mutated); evidence ids never covered retrievedAt, so finding references resolve identically.
    Mirrors normalizedEdgeJSON (bundle/change.go) and the assembler's pinned clock.
  - `internal/investigate/run/pipeline.go` assemble: the stored impact-report artifact is
    `rep.IdentifiedContent()`. Behaviour change: the artifact digest — and with it the run-record
    digest — no longer depends on when the run fetched/generated evidence, so two identical runs are
    the same record (a re-run is a no-op) instead of two. Why: stamps are provenance of the run
    (record CreatedAt + audit trail keep them), never identity of content. The CLI's in-memory report
    keeps real stamps.
  - Tests: `TestRunRecordDigestIgnoresReportTimestamps` (run) — two identical runs whose reports differ
    only in their stamps give the same artifact digest AND record digest; `TestImpactReportIdentifiedContent`
    (domain) — zeroes only the stamps, ids stable, original untouched, nil slices stay nil.

  Behaviour changes (all generic, each with its why):
  - Layering no longer warns and no longer degrades the values dimension → gitops-repo input can be
    CollectionOK for repos that layer values files (the §5.1a symptom). Why: layering is the documented,
    supported shape of every gitops tool; the effective values are fully derivable from the layers.
  - One effective key per path (was: one key per file per path, "last file wins" per key only by
    accident of load order). Why: Helm coalesces; downstream rules (values-pinned etc.) must see the
    effective value, and provenance must name the layer that actually set it.
  - Effective values apply in config order (Argo list order beats alphabetical). Why: Helm's -f order is
    the application order; an alphabetical walk can invert override direction.
  - Images come from the merged set only (was: per-file scan → stale refs from overridden layers).
    Why: Helm never applies the overridden layer's image.
  - Referenced-but-missing values file (helmfile or Argo `$values/`): warning with reason, values
    partial, no visibility claim. Why: fail-closed — a file the config names but we cannot read is a
    collection gap, not silence.
  - Inline Argo/Flux-only values now count as `Supplied.Values`. Why: values visibility is about content
    presence, not file presence.

## In progress (exact next step)
- Fix 3 (CRD description-only changes, EVAL-REPORT §5.1g) — code done, tests PARTLY done, NOT committed:
  - `internal/render/diff.go`: new declared class `CRDDescriptionChanged` ("crd-description-changed",
    listed in ChangeClasses; one class covers add/remove/reword like image-changed). The differ classifies
    a `description` key inside a CustomResourceDefinition's `openAPIV3Schema` subtree as it (keyed()
    intercept: kind == CustomResourceDefinition && pattern contains openAPIV3Schema && key ==
    "description"). NOT added to RenderedClasses (auto-approval eligibility) — documentation is not an
    effect.
  - `internal/render/correlate.go`: `match()` returns "" for CRDDescriptionChanged before every rule —
    schema documentation restates no behavioural change (kills the old mentions-field link on the word
    "description").
  - `internal/investigate/bundle/change.go` `anchorUnexplained`: description changes get NO scope entry →
    they anchor no item (THE noise fix: 300 reworded descriptions no longer spawn 300
    scope:rendered-unexplained items); they stay visible as plain rendered changes in the pair.
  - `internal/investigate/validate/renderdiff.go`: `descriptionOnly` reads the class (imports render)
    instead of parsing object/path strings; harmless→pass behaviour unchanged.
  - `internal/investigate/bundle/scope.go`: `hasNonDescriptionPath` deleted — description paths can no
    longer enter CorrelatedPaths (they don't correlate), and the suffix check would wrongly demote a legit
    non-CRD path ending in "description"; `len(CorrelatedPaths) > 0` now.
  - Tests updated: renderdiff_test (description fixture now requested-changes shape — that is the only
    way an uncorrelated change reaches the validator; silence-never-clears-behaviour test uses a real
    schema-type field mention), scope_test (dropped the now-impossible description-only CorrelatedPaths
    subtest with a comment). render/validate/bundle packages green.
  - NEXT: add the render-level tests for the class itself (diff_test: CRD description → class; non-CRD
    description field stays field-changed; correlate_test: no link, not even mentions-field with the word
    "description" in the text; a bundle/anchorUnexplained test proving no item for description-only pairs
    if cheap), run FULL suite, commit fix 3, update this file.

## Not started
- Fix 4 (cert-manager traversal truncation at depth 8: diagnose, fix artefacts or raise the default with
  measured justification; truncation stays honest).

## Decisions (with why)
- Values sources ordered: Argo valueFiles list order, then by-name alphabetical, then helmfile list order
  (config intent beats directory sort; matches render's layering).
- Plain (non-`$`) Argo valueFiles entries establish no env-side order/collection: they name paths in the
  source's own repo, which a local walk cannot answer for (render already marks those targets incomplete).
- Missing referenced values file (helmfile or Argo `$values/`): warning + values dimension partial, but
  NOT `Supplied.Values` (no visibility conclusion may rest on values never seen — fail closed).
- Inline Argo/Flux/parameter values participate in the merge and apply after all files (Helm/Argo/Flux
  semantics); images are extracted from the merged effective keys only (no stale refs from overridden
  layers).

## Files touched outside ownership
- `internal/app/impact_repo_e2e_test.go` (assertion: layering warning must be absent) and
  `internal/app/testdata/e2e/golden/impact/*` (regenerated via -update, diff reviewed),
  `cmd/ri/impact_repo_test.go` (same assertion change) — behavior-change fallout of the env fix, kept
  minimal.

## Contract changes
- none.

## Open questions
- none.

## Test status
- Full `go build ./... && go vet ./... && go test ./...` green at fix-2 commit 175709ef.

## GLM handoff log
- (session start) Resumed from brief + commander routing (commit 3729ab10); no prior lane status file
  existed. Began with fix 1.
- (checkpoint) Context 82%: status committed mid-fix-1 as above; next agent resumes at "In progress".
- Fix 1 finished and committed (a638c39a) after the checkpoint: loader fields added, discHelmfile moved to
  helmfileValues, Argo list-order tests (env + bundle) added, full suite green. Next: fix 2
  (retrievedAt reproducibility) — start by locating where the ImpactReport artifact embeds `retrievedAt`
  (grep `retrievedAt` in internal/investigate + internal/app) and how bundle CreatedAt was kept out of
  identified content (prior art).
- Fix 2 finished and committed (175709ef): IdentifiedContent zeroes GeneratedAt + evidence RetrievedAt in
  the stored report artifact (same pattern as normalizedEdgeJSON); two-identical-runs test proves equal
  artifact + record digests. Note for the record: the run-level test needed both runs to point at the SAME
  repo dir — the snapshot audit detail embeds the repo path (content, not a clock), which is correct.
  Next: fix 3 (CRD description-only rendered changes) — start from internal/render rendered-change
  classification + how renderdiff emits changes; the brief wants a declared generic render class at
  render/correlation time, deterministic validator harmless verdict, no noise flood, never clears the
  item's behaviour change.
