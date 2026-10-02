# Lane `envinv` status

**State: done** (branch `p3ll/envinv`). `go build ./... && go vet ./... && go test ./...` pass; schemas regenerated.
Full eval before/after: output identical (`no regressions against eval/results`; the only failing gate is the
pre-existing applicabilityAccuracy 0.095, unchanged).

## Done
- `internal/env/inventory.go`: `Environment.Products []ProductInstance` (product id = catalog id when it maps,
  else lower-cased name; normalized `Version` + `RawVersion`; `Source` declared|detected-helm|-argo|-flux|-helmfile|-label|-image;
  `VersionOf` app|chart; `Note`; `Conflict`; per-entry `domain.Evidence`). `Installed` is untouched and feeds it.
- Inputs: `Inputs.Inventory` / `ri impact --inventory <file>` (YAML list of `{product, version, note?}`),
  `<repo>/inventory.yaml` in `--repo` mode, `environment/inventory.yaml` in eval fixtures (`eval.DirInputs`),
  `--kubernetes` as a declared `kubernetes` entry, image detection driven by `products/*.yaml`
  (`env.HintsFromCatalog`: container-image artifacts with OCI channels; `app.ImpactParts` supplies them).
- Health: `DimProducts` (`absent|ok|partial`) via `Health(env.DimProducts)` / `ProductsStatus()`. Supplied only when an
  inventory file was given or a product was detected (`--kubernetes` alone does not count).
- Accessors: `Product(id)`, `ProductInstances(id)`, `ProductInRange(id, *domain.CompatibilityConstraint)` ->
  `{Present, Computable, InRange, Conflict, Instance}`; uses `upgrade.EvaluatePlatformConstraint` (line precision) and
  full-semver comparison when the entry is a full version and the constraint has patch-level bounds.
- Report: `ImpactEnvironment.Products` (`ImpactProduct`) + `ProductsHealth` (additive, omitted when absent); inventory
  evidence is added to `environmentEvidence`; text output lists it with `CONFLICT with ...`. Schemas regenerated.
- Fixtures: `inventory.yaml` in 8 eval environments, facts only from `case.yaml` `environment.description`, each with the
  quoted sentence in a comment. Did not read `expectedImpact`/`expectedFindings`; no expectations/results touched.
- Tests: `internal/env/inventory_test.go` (declared, absent vs not-listed, unparsable/malformed/missing file,
  declared-vs-detected conflict, image detection incl. mirror/digest/unparsable tag, Helm chart-vs-app version kinds,
  range table, repo mode, catalog hints), `internal/impact/inventory_test.go` (report + render + evidence resolution).
- Docs: `docs/IMPACT.md` (new "Product inventory" section), `eval/FORMAT.md`, README mention.

## Decisions (and why)
- **`Statuses()` does not list `products`.** It feeds the enrichment prompt ("dimensions: ..."); listing it changed every
  prompt digest and invalidated `internal/app/testdata/impact-llm-cache` (TestImpactEnrichReplayOffline failed).
  Read it through `Health(DimProducts)` instead.
- **Line-precision versions stay lines** (`"2.14"` is not padded to `2.14.0`); such entries are range-checked at line
  granularity only.
- **Conflicts are never resolved**: both entries kept, both flagged, dimension `partial`; `ProductInRange` is
  `Computable=false, Conflict=true` when the entries disagree on the verdict, decidable when they agree.
- Helm/Argo/Flux/Helmfile detections carry a *chart* version (`VersionOf: chart`), not the app version. Conflicts are only
  computed between entries of the same `VersionOf`. The contract lane's predicate should prefer `app` entries.
- Mirror images match a catalog image repository by path (host differs), noted on the entry; a path with a single
  segment never matches by path alone.
- Report `Products` is omitted when the dimension is absent, so kubernetes-only runs are unchanged.

## Changes outside ownership / merge-conflict heads-up
- `internal/impact/build.go` (`environmentSummary`, +cite inventory evidence) and `internal/impact/render.go`
  (environment block, `productVersionText`) — additive hunks.
- `internal/app/impact.go` (+3 lines: default `ProductHints` from the catalog), `internal/eval/case.go` / `run.go`
  (`inventory.yaml` in `envFileNames` / `DirInputs`), `schemas/impact-report.schema.json` (generated).
- E2E goldens updated, with reason: `internal/app/testdata/e2e/golden/impact/cert-manager_*` (.txt/.json). Those
  environments carry Helm/Argo/Flux/label/image facts, so detected inventory entries now appear in the report's
  environment block (additive `products`, `productsHealth`, extra environment evidence; repo mode also gains one warning
  for Flux's `1.*` chart version). Findings are unchanged.
- No new dependencies (`Masterminds/semver` was already in go.mod).

## Open questions / for the commander
- The contract lane's applicability predicate can call `env.ProductInRange`; the engine decides what "product not listed
  while `Health(products)==ok`" licenses (I report presence only).
- `istio-csr` and `kafka` are not catalog products, so they appear as non-catalog normalized names.
