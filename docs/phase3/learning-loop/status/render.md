# Lane `render` — status

Branch `p3ll/render` (worktree `.claude/worktrees/render`), merged `p3-learning-loop` @ 4571d5d (contract-2).
Mission: `RENDER-MISSION.md`; brief: `briefs/render.md`.

## Done

- **(a) renderer + model + diff + CLI** — `internal/render`:
  - `Renderer` port; `helm template` and `kustomize build` / `kubectl kustomize` adapters (external
    binaries, scrubbed env: no kubeconfig, private HELM_* dirs). Full `Provenance` (tool+version, command
    with stable names, chart name/version/URI/digest/representation, release name/namespace, kube
    version/api-versions, per-layer values digests + combined values digest, values completeness,
    kustomize root/dir/input digests/substitutions, output digest, limitations) and a content-addressed
    cache (`<state>/render/<rk-…>/`). Every R13 failure class is an explicit `Failure` → pair `failed`
    (never "no change").
  - Nondeterminism probe: each Helm render runs twice; fields that differ between identical renders
    (random tokens, generated certs) are suppressed as `nondeterministic` noise.
  - Object model: identity `<group>/<version>/<kind>/<namespace>/<name>` (`core` for the core group),
    version-independent matching (api-version-changed), normalization (empty ≡ omitted, Secret data and
    credential-named values compared by digest only).
  - Semantic diff with the R4 classes; keyed lists (containers/env/volumes/ports/… by name/port);
    RBAC rules as permission sets (group × resource × verb [× resourceName]); args keyed by flag with
    ordering preserved (pure reorder = one `container-arg-changed`); noise rules as data
    (`chart-metadata` helm.sh/chart + app.kubernetes.io/version stamps, `checksum-annotation`), counted.
  - Customer configuration first (R7): explicit `--values`; Argo CD Applications (values, valuesObject,
    parameters, `$values/` valueFiles; others ⇒ incomplete), Flux HelmReleases (valuesFrom ⇒
    incomplete), Helmfile releases (files/inline/set; templated/secrets ⇒ incomplete), repo-local
    values files naming the product and not referenced by Helmfile; Kustomize overlays tied to the
    environment (Argo source path, `--overlay`, else top-level kustomizations referencing the product),
    each with a recorded `why`. Kustomize target render = the overlay with the product version
    substituted (images newTag / remote refs), recorded; overlays that do not pin the version ⇒
    `not-applicable` (never "no change").
  - Correlation of rendered changes with edge changes (rules: subject, mentions-name,
    mentions-permission, mentions-object, mentions-api-version, mentions-field, values-default,
    artifact); undocumented rendered changes listed.
  - `ri render diff <product> <from> <to> [--values|--repo|--overlay] [--kubernetes] [--chart-defaults]
    [--show-values] [-o json]` (`cmd/ri/render.go`); chart resolution via the existing artifact
    resolution (`internal/app/render.go`: ingested release's chart artifact version → registered
    `ChartPackageReader`s; packages come through the fetch cache).
  - Tests (offline): golden render streams (`testdata/golden`, produced by helm from
    `testdata/charts/demo-{1.0.0,1.1.0}`), normalization/noise, sensitive values, api-version identity,
    failure classes (fake runner), kustomize inputs/substitution, repo target detection on
    `internal/env/testdata/customer-repo`; live helm/kustomize tests skip when the tool is absent.

## Live results so far

cert-manager v1.17.0 → v1.18.0, `--kubernetes 1.31`:
- chart defaults: 14 rendered changes (46 → 48 objects), 96 chart-metadata stamps suppressed. Correlated:
  5 image bumps + the acmesolver image arg (artifact), `prometheus.servicemonitor.targetPort` default
  9402 → "http-metrics" (values-default). **Undocumented:** webhook liveness/readiness probe port 6080 →
  "healthcheck"; the challenges ClusterRole/Binding split into `dns01-` / `http01-` roles (the note
  about `disableHTTPChallengesRole` explains the motive, but no entry names the split).
- customer-repo: Argo (15 changes incl. ServiceMonitor endpoint), Flux (14, values INCOMPLETE via
  valuesFrom), Helmfile prod values (11: `targetPort` pinned ⇒ no Service change;
  `disableHTTPChallengesRole: true` ⇒ no http01 role), dev values (14). Kustomize overlay: explicit
  `kustomize-dependency` failure (the fixture references files outside the overlay dir, rejected by
  kustomize's default load restrictor — the customer's deployer would fail the same way).

## Next

(b) evidence + `rendered-diff` validator (sets contract `RenderRelation`), renderability table; (c)
`EvaluateRenderedChange` + `ri impact --render`; (d) auto-approval policy + metrics; (e) `eval/render/`
cases; docs/RENDER.md.

## Decisions (why)

- Path syntax: keyed selectors for display (`containers[name=controller].args`), plus a `Pattern` with
  `[]` (the env/CRD SchemaPath syntax) that `rendered-change` conditions match against.
- `--skip-tests` on helm template (test hooks are not deployed); other hooks rendered.
- Values of environment renders are hidden in text/JSON unless `--show-values`; Secret/credential values
  are digests always.
- Kustomize default load restrictions are kept (fidelity to how the customer's deployer builds).

## Files touched outside ownership (additive)

- `internal/domain/evidence.go`: `EvidenceRenderedDiff` kind. `// CONTRACT-CHANGE(render)`
- `internal/domain/semantic.go`: `RenderProvenance.{FromArtifact,ToArtifact,Object,Path,Change}` +
  `RenderArtifact`. `// CONTRACT-CHANGE(render)`
- `internal/domain/impact.go`: `DimensionRender`, `MatchRenderedChange`. `// CONTRACT-CHANGE(render)`
- `internal/domain/schemagen/meta.go` enums + regenerated `schemas/*.json`.
- `internal/sources/artifacts.go`: `ChartPackage.Archive []byte`; set in `internal/helm/package.go` and
  `internal/oci/chartlayer.go` (one line each).
- `internal/app/render.go` (new), `cmd/ri/main.go` (dispatch + usage line).

## Contract changes

See "Files touched outside ownership" — all additive, marked `CONTRACT-CHANGE(render)`.

## Tests

`go build ./... && go vet ./... && go test ./...` green.
