# Lane `values` — status

Branch `p4ei/values` (from `p4-env-impact` @ a3f62868). Brief: `briefs/values.md`. Redo of the reverted
layered-values merge (a638c39a, reverted in 73bb58ba).

## State

**Follow-up done** (commander review of 65ee78a5: render divergences + review-UI flag text; branch merged
`p4-env-impact` @ 20e958b8 first, fast-forward). First round: **done.** `go build ./... && go vet ./... && go test ./...` green. No kind clusters, servers, `bin/` or
scratch output created.

## What was built

- `internal/env/values.go` (new): one `ValuesRelease` per deployment target — Argo CD Helm source
  (`argocd:<file>#<doc>/$.spec.source[s[i]]`, ids aligned with render targets), Flux HelmRelease
  (`flux:<file>#<doc>`), helmfile release (`helmfile:<file>/$.releases[i]`), each repo values file no
  release references (`values:<file>`), the explicit `--values` group. Each has ordered `Layers` (kind,
  source, applied/gap/reason), effective `Keys` (Helm merge, per-key provenance = evidence of the layer
  entry that set it + line), `Health`, `Reasons`.
  - Argo: `valueFiles` in declared order (`$ref/…` from repo root, mirroring render; plain path relative
    to the chart root → readable only for a git `path` source in this repo, else gap; remote URL, unknown
    ref, path escaping the repo, missing file (even with `ignoreMissingValueFiles`) → gap), then
    `valuesObject` — `values` only when no `valuesObject` (else recorded as ignored, not a gap) —, then
    `parameters` (Helm strvals typing; `forceString`; `\.` escapes; list index / `{a,b}` → gap);
    `fileParameters` → gap. A ref-only source renders nothing (its helm block reaches no release).
  - Flux: `valuesFrom` in order (ConfigMap defined in the inputs is read, `valuesKey` default
    `values.yaml`; Secret or object not in the inputs → gap; several matching ConfigMaps → gap), then
    `spec.values`, then `targetPath` entries (strvals typing, `literal` ⇒ string);
    `spec.chart.spec.valuesFiles` → gap.
  - helmfile: `values:` in order (files, inline maps), `set:` last; templated helmfile, `.gotmpl` entry,
    `secrets` → gap.
  - Merge: maps deep-merge, lists/scalars replace, `{}` deletes nothing, `null` replaces the subtree
    (kept as leaf `null`), a later map replaces a scalar/null.
- `env.Inputs.Product` + `ProductHint.Charts` (filled by `HintsFromCatalog` from helm-chart artifacts):
  `ValuesKeys` and the values dimension are scoped to the product's releases (chart base = product id or
  one of its charts; Flux `chartRef` → release name). Unreferenced repo values files are attributed by
  name (stem + parent dir vs every release name/chart and catalog product, longer name shadows a
  contained one): product ⇒ its own values-files release; exactly one other ⇒ not the product's;
  none / several ⇒ **unattributed** (keys kept per file, `Release ""`, warning, `partial`).
  Several distinct releases ⇒ `ValuesScope.Ambiguous`, each release's keys kept (`ValuesKey.Release`),
  warning, `partial`; identical releases ⇒ one (`ValuesScope.Equivalent`). `Product ""` ⇒ every release
  selected (legacy callers). Explicit `--values` files are appended last to each selected stack (win per
  key — `Inputs.Repo`/`ValuesFiles` doc kept true and extended), or form their own release.
- `internal/impact/build.go`: (1) a values change matching no key while values health ≠ ok is UNKNOWN
  (`impact:insufficient-visibility`, needed = first values warning), never `values-unset` /
  unrendered not-affected; (2) `values-pinned` only when every distinct release pins the key; otherwise
  UNKNOWN "only some releases of the product pin …".
- Wiring: `app.ImpactRun` sets `Product = productID`; `bundle.SnapshotBuilder` sets `Product = in.Product`.

## Decisions (why)

- Ambiguity = gap, not per-release re-evaluation of the five-class join: the join is per environment;
  keeping each release's keys tagged lets positive matches stand for the release they cite while
  `partial` blocks every clearing verdict. Per-release evaluation of the join is a larger change
  (open question 2).
- `ignoreMissingValueFiles` + missing file stays a gap: local absence does not prove absence at the
  revision Argo CD deploys (Principle 7).
- Secrets are never read for Flux `valuesFrom` (avoids pulling secret material into facts; fail-closed).
- `$ref` resolves against the scanned repo (same assumption as `internal/render`).
- Images are still extracted per values file (unchanged); effective-stack extraction left as a gap.

## Files touched outside ownership (additive)

- `internal/app/impact.go`: 5 lines — default `opts.Environment.Product = productID`.
- `cmd/ri/impact_repo_test.go`, `internal/app/impact_repo_e2e_test.go`: expectations for the new
  per-release behaviour (below).
- `docs/IMPACT.md` (repo-mode section rewritten for per-release values), `docs/INVESTIGATE.md` (new
  section "Environment values (per release)").

Contract changes: none (`env`, `impact` are not contract files; `investigate.EnvironmentInputs.Product`
already existed). No new dependencies. No schema regeneration needed (no `internal/domain` change).

## Golden / expectation changes, line by line

`internal/app/testdata/e2e/golden/impact/cert-manager_repo_v1.17.0_v1.18.0.{txt,json}` (fixture
`internal/env/testdata/customer-repo`, which deploys cert-manager four ways: Argo CD inline, Flux,
helmfile → `clusters/prod/values-cert-manager.yaml`, and the unreferenced
`clusters/dev/values-cert-manager.yaml`):

- `values: 9 keys set` → `7`: old = every file and inline block concatenated (prod 3 + dev 1 + Argo
  `values` 2 + Argo `valuesObject` 1 + Flux 2). New = per-release effective keys: helmfile 3 + Argo 1
  (Argo ignores `helm.values` when `valuesObject` is set) + Flux 2 + dev 1.
- `INFORMATIONAL 6 → 5`, `UNKNOWN 45 → 46`: `impact:values-pinned` on
  `prometheus.servicemonitor.targetPort` is gone — only the helmfile release pins it; the other three
  releases leave it unset and get the new default. It is now an unknown ("only some releases of the
  product pin …", needed = the per-release values gap). Its environment evidence (`ev-185f2a535264`,
  prod file L8) leaves the evidence list with it.
- Warnings: `repo mode found 2 values files; all are applied, the last one wins per key` → `4 releases
  deploy cert-manager with different effective values (…): values are per release and never merged …`;
  `HelmRelease cert-manager uses spec.valuesFrom …` → `values of Flux HelmRelease cert-manager (…):
  … spec.valuesFrom ConfigMap cert-manager-values references a cluster object no input defines …`
  (same gap, now attributed to its release and object).
- `cmd/ri/impact_repo_test.go`: expects the new ambiguity warning instead of the old one.
- `internal/env/repo_test.go`: `TestRepoInlineValuesBecomeValuesKeys` now asserts Argo's documented
  behaviour (valuesObject applied, `values` recorded as ignored, release ok); `TestRepoWarnings` expects
  the ambiguity warning. `internal/env/multisource_test.go`: a ref-only source's inline values reach no
  release (was: kept as values) — Argo CD renders no chart from a ref-only source.

Eval env probes (inputs only, `env/` dirs; no expected/NOTES opened), values health scoped to the
product: payments-eu ok (1 Argo release), shop-de ok, shop-nl ok, obs-north ok — formerly `partial`
from the multi-file warning; shop-pl `partial` (its Argo `$values/shop/*.yaml` files are not in the
repo) — formerly `absent`, and its no-match values changes are now UNKNOWN, not not-affected.

## Tests added

- `internal/env/values_test.go`: Argo declared order + precedence + provenance; valuesObject vs values;
  parameter typing/escaping/null; list syntax gap; unreadable valueFiles (7 cases); git-source plain
  path; Helm merge semantics (`{}` vs `null`, deep merge vs list replace, map over scalar/null);
  Flux order incl. targetPath; Flux unreadable valuesFrom / Secret; helmfile order + template gap;
  explicit `--values` beats inline and parameters (revert defect) and explicit-only release;
  cross-release isolation (revert counter-example: per-cluster files + another product's inline block);
  equivalent releases; unreferenced files (stray ⇒ partial, other release's ⇒ ignored, product-named ⇒
  own release, two names ⇒ partial); chart-hint scoping.
- `internal/impact/values_release_test.go`: partial values never clear; pinned needs every release.
- `internal/investigate/bundle/values_test.go`: repo input ok for a fully read product release despite
  several values files; another product's cluster-only valuesFrom is not this product's gap; two
  differing releases ⇒ partial with the reason.

## Follow-up (commander, after merge 65ee78a5)

Ownership extended to `internal/render/source.go` for this, plus the review-UI flag text.

- `render.argoTargets` now layers exactly like `env.argoReleases`:
  - `valuesObject` set ⇒ `helm.values` is not applied; a DetectTargets note records it ("helm.values is
    ignored — valuesObject is set …").
  - plain `valueFiles` paths resolve relative to the source `path` in the scanned repo; a plain path of a
    Helm-repository chart source stays incomplete ("resolves inside the chart package"). `$ref/…` as
    before, plus (as in env) an undeclared ref, a remote URL, a path escaping the repo and a missing file
    (with the `ignoreMissingValueFiles` caveat) ⇒ incomplete; `fileParameters` ⇒ incomplete.
  - a git `path` source that is not a kustomization is a Helm target when it has `helm:` settings or a
    `Chart.yaml` in the repo and its path base names the product (formerly skipped — so the values
    fix is reachable). Same rule as env.
  - parameters: `env.SetValueJSON` (exported from env; one implementation) gives Helm strvals typing
    for `--set-json`: `"1.5"` and `"0123"` stay strings (render's old `encodeSetValue` turned `1.5` into a
    number), bools/null case-insensitive; a parameter without a value is `""` (was `"<nil>"`).
    `encodeSetValue` removed.
- Tests: `internal/render/argo_values_test.go` — `TestArgoRenderAndEnvStacksAgree` (multi-source chart:
  declared-order `$values` files, values+valuesObject, typed/escaped/null parameters, `{}` and list
  replace; git path source with a plain valueFile + `values`): the render target's effective values
  (its `-f` layers merged and `--set-json` applied by an independent re-implementation of Helm's
  `mergeMaps`/set in the test) equal the env release with the same id, and both are complete. A
  mutation check (re-applying `helm.values`) makes it fail. `TestArgoUnreadableValueFileAgrees`: a
  chart-source plain path is incomplete in both.
- `TestDetectTargetsCustomerRepo` expectation: the fixture's Argo target has 1 layer (valuesObject), was 2
  (values + valuesObject), and the "ignored" note is present.
- Review UI (`internal/reviewui/english.go`): `flagText` — a CLI-flag subject whose stored name already
  carries dashes is shown as stored (`--enable-metrics`, `-v`), a bare name gets `--`; never
  `----enable-metrics`. Tests in `english_test.go`.
- Goldens: **none changed** (no golden covers an Application with both values blocks, plain
  valueFiles or a git-path Helm source). Docs: `docs/RENDER.md` target table.
- Residual (not asked): helmfile `set` values in render are still encoded from the YAML-typed value
  (`encode`), env uses the scalar text with strvals typing — they differ only for floats written with
  trailing zeros / leading-zero numbers / mixed-case bools.

## Open questions / findings for the commander

1. ~~render divergences~~ — fixed in the follow-up above.
2. Ambiguous releases are a gap in the five-class join (fail-closed). If per-release verdicts are wanted
   ("each is evaluated"), the join would run once per `ValuesScope.Releases` entry and combine
   worst-case — a larger change to `internal/impact`.
3. The unknown-summary renderer groups the new values unknowns under "environment dimension not
   supplied" (its label for `environment-visibility-gap`); wording only.
