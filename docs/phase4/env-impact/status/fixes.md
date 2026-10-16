# Status — lane `fixes` (GLM-5.3, agent glm-fixes; Claude lane lead paused for a usage-limit window)

Branch `p4ei/fixes`. Brief: `briefs/fixes.md` (+ `_wave2-common.md`, `_wave1-common.md`, FLEET.md).
Defect list: EVAL-REPORT.md §5 (arm A) as routed by the commander; read from `p4ei/eval` (not merged into
`p4-env-impact` at lane start; read-only via `git show`, no merge).

## Done
- Lane setup: brief + FLEET + wave preambles + EVAL-REPORT §5 + status/eval.md read; baseline
  `go build ./...` + `go vet` green.
- Fix 1 core implemented (NOT yet committed at this checkpoint; working tree holds it):
  - `internal/env/valuesmerge.go` (new): `valuesMerge` — layered Helm coalesce (later source overrides
    earlier per key; mappings deep-merge; any other leaf replaces the subtree below its path), one
    effective `ValuesKey` per path with evidence from the file/line that set it; `valuesContainerPaths` /
    `valuesPairs` (merge-key + dup-last-wins aware, aligned with normalize.FlattenValues syntax);
    `loader.applyValuesFile` (per-file parse warnings unchanged); `loader.finalizeValues` (materialize
    under maxValuesKeys + image extraction from the MERGED keys — repository/tag siblings may come from
    different layers, as Helm merges them).
  - `internal/env/env.go`: layering warning at old L428 REMOVED; `loadValuesFiles` feeds the merge;
    `finalizeValues` called after manifests load (inline Argo/Flux/parameter values apply after files, as
    Helm/Argo/Flux apply them); `Supplied.Values` now also true for inline-only values
    (`len(values.eff) > 0`) — a repo whose values are only Argo/Flux inline no longer reads "no Helm values
    supplied"; values health: a values reference that could not be collected (`valuesNamed`) is
    supplied-with-gap (partial), never absent, and deliberately does NOT count as visibility.
  - `internal/env/installed.go`: `inlineValues` + `parametersToValues` route through the merge (inline
    wins over files; parameter value-mappings deep-merge).
  - `internal/env/repo.go`: helmfile values reference that is missing → warning + `valuesNamed`;
    `orderValuesFiles` — values inputs ordered as the repo's config applies them (Argo helm.valueFiles
    list order first, then name-convention alphabetical, then helmfile list order);
    `argoValueFilesOf` — collects Argo `helm.valueFiles` ($ref/ paths resolved against repo root, mirroring
    internal/render/source.go) in list order, adds files the name convention misses, warns on missing refs.
  - Tests: `internal/env/valuesmerge_test.go` (new: deep-merge, leaf/map replacement, merge keys, merged
    images, missing helmfile ref), repo_test.go trio rewritten for merge semantics, bundle
    `TestSnapshotLayeredValuesFilesAreComplete` (gitops-repo input OK + merged facts), app/cmd e2e
    assertions updated, impact goldens regenerated (9 keys → 6 effective, warning line gone — reviewed).
  - Full `go build ./... && go vet ./... && go test ./...` was GREEN before the Argo-ordering addition.

## In progress (exact next step)
- Finish fix 1's Argo ordering: ADD loader fields `argoValueFiles []string` and `helmfileValues []string`
  to the loader struct in `internal/env/env.go` (orderValuesFiles/argoValueFilesOf in repo.go already
  reference them — **the tree does NOT compile until the fields exist**); change `discHelmfile` to append
  to `l.helmfileValues` instead of `*vals`; fix `TestSnapshotLayeredValuesFilesAreComplete` to use an
  Argo Application with `valueFiles: [$values/...base, $values/...prod]` (list order must beat
  alphabetical); add an env test for Argo list-order precedence + missing `$values/` reference warning;
  run full suite; commit fix 1.

## Not started
- Fix 2 (run-record reproducibility: ImpactReport embeds evidence `retrievedAt`; two identical runs must
  give identical record digests).
- Fix 3 (CRD description-only rendered changes: declared generic render class + harmless deterministic
  verdict, no noise flood, never clears behaviour).
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
- Full suite green pre-Argo-ordering; tree currently mid-edit (see In progress).

## GLM handoff log
- (session start) Resumed from brief + commander routing (commit 3729ab10); no prior lane status file
  existed. Began with fix 1.
- (checkpoint) Context 82%: status committed mid-fix-1 as above; next agent resumes at "In progress".
