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

- **(b) evidence + rendered-diff validator + renderability** — `internal/render`:
  - `evidence.go`: `Pair.Evidence(change, valuesShown)` — `EvidenceRenderedDiff` records (upstream-ok,
    never environment values unless `--show-values`; secrets/credentials always digests), plus state
    records (`Pair.stateEvidence`) so unchanged/absent paths have evidence too.
  - `validator.go`: `render.rendered-diff@v1`, a `knowledge.Validator` over release-level (chart-default)
    pairs via `Engine.ReleasePairs(kubeVersion)`: sets `RenderRelation` (confirmed-by-render requires a
    confirmed check, no refuted check and cited rendered evidence; refuted-by-render on a decisive
    mismatch; render-not-applicable for unpinned/unsupported), `Renderability` per subject.
    `Validate` rejects environment-scope renders (they may back impact findings, never knowledge).
  - `renderability.go` + `inventory.go`: renderability table (`RenderedClasses` maps each
    semantic family/subject class to render-verifiable | partially | not, with why), `AssessRenderability`,
    subject inventory (`Occurrence`) resolving where in the render a subject lives.
  - tests: `inventory_test.go`, `validator_test.go` (golden streams; confirm/refute/inconclusive/error;
    the environment-render rejection; the renderability table).
- **(c) `EvaluateRenderedChange` + `ri impact --render`** — the minimum workflow:
  - `evaluate.go`: the `rendered-change` leaf (DESIGN.md §1.3) as a tri-state —
    `RenderedChangeResult{Value true|false|unknown, Detail, Reason environment-visibility-gap,
    Evidence, Targets}` over the environment's render pairs. Any pair showing the change ⇒ true; all
    pairs complete-and-otherwise ⇒ false; failed/incomplete/absent ⇒ unknown (an absence decides
    nothing). Path syntax: the diff's `[]` patterns and keyed selectors, quoted map keys, port
    protocol suffixes (`ports[port=443/TCP]`). Evidence is environment-scope only (R5); the leaf never
    classifies consequences (R11).
  - `internal/app/render.go`: `RenderDiffEdge` (impact reuses its edge + loaded environment);
    `cmd/ri/impact.go`: `--render` — text appends the rendered section, `-o json` wraps the report
    (`{...impact report..., "render": …}`). Offline every target fails explicitly (R13).
  - tests: `evaluate_test.go`, `cmd/ri/impact_render_test.go`.

- **(e) `eval/render/` cases + comparison runner (R16, R17 render-scoped)** —
  `eval/render/README.md` (format, metrics, the knownGap convention);
  `cert-manager-1.17-1.18` (16 expectations from the git diff of chart templates/values/
  CRDs between the tags + the 1.18 release notes: RBAC split, probe ports, targetPort
  default, images, acmesolver arg, CRD field as knownGap, rotationPolicy as
  not-render-verifiable; a values-driven ServiceMonitor variant) and
  `kustomize-customer-overlay` (explicit `kustomize-dependency` failure, R13).
  Runner `go test ./internal/app -run TestEvalRenderCases` (skips without helm/network).
  First live comparison (`eval/render/results/2026-10-11-glm-handoff.md`): release
  precision 14/14, recall 15/16 (R16 gap open: no CRD in the rendered delta despite
  `--include-crds`), variant 2/2, kustomize 2/2. Honest gaps: no env-var/api-version
  case (no such change between these tags), pipeline-level R17 metrics wait on the
  applicability lane, one more live product than cert-manager not run. NOTES.md files
  record the blind-authoring caveats (lane status prose + one live run seen before
  authoring) and every correction the comparison forced.

## Next

Nothing buildable is left on this lane's brief. Open items handed to the commander /
next agent: the `SemanticCandidate.Renderability` join (see "(d)"), the R16 CRD gap
(are CRD objects meant to reach the rendered stream?), the applicability seam signature,
and the honest (e) gaps above.

## (d) auto-approval — verified, one join missing (commander decision)

`AutoApproveRenderVerifiable` (knowledge lane, `internal/knowledge/autoapprove.go`) matches R10/G10:
it gates on render-verifiable, verifies unconfirmed aspects by consensus of separate model calls
(PO-1), and a refuted digest — e.g. contradicted-by-render from the `rendered-diff` validator — can
never be auto-approved. Metrics incl. the audit sample are the knowledge lane's (`metrics.go`).
**Gap:** `SemanticCandidate.Renderability` is set by no production code (fixtures only), so the policy
is inert until the candidate-producing lane (or `RouteStore`) folds it — from this lane's
`render.RenderabilityOf(family, changeType)` / `RenderedClasses` or the validator's `RenderRelation`.
That wiring lives in `internal/knowledge` (knowledge lane's ownership), so it is flagged here rather
than done during the handoff.

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

`go build ./... && go vet ./... && go test ./...` green except one pre-existing failure:
`internal/reviewui TestFixturesAreValidDomainRecords` (dashboard demo proposals lack
`provenance.callId`, contract-3) — present on the merge base 6647e61, not touched by this lane
(dashboard lane owns `internal/reviewui`).

## GLM handoff log

 glm-render (GLM-5.3) continued the lane while its Claude agent was paused. Commits ab5b31c, 118c068,
638055a, 5140da0, 7060480, 5cab7a8, 4473d4e, ad7e984, c2cd538, e0a2df7, f2d770b (`[glm-handoff]`
prefix):

- Banked the paused agent's WIP as ab5b31c before touching anything.
- (b) tests: `inventory_test.go`, `validator_test.go` (golden streams; environment-render rejection;
  renderability table).
- (c) `evaluate.go` `EvaluateRenderedChange` + `evaluate_test.go`; `ri impact --render`
  (`app.RenderDiffEdge`, `cmd/ri/impact.go` `--render`, JSON wrap) + `impact_render_test.go`.
  **RENDER EVALUATOR READY** — the applicability lane can adapt
  `render.EvaluateRenderedChange(cond, pairs) RenderedChangeResult` (tri-state + evidence + targets).
- (d) verified `AutoApproveRenderVerifiable` against R10 (see its section); flagged the missing
  `SemanticCandidate.Renderability` join for the commander.
- docs/RENDER.md written (path syntax, scopes, failure classes, validator, evaluator, CLI, evaluation).
- (e) `eval/render/` cases authored from upstream (template/CRD git diff + release notes), the
  comparison runner, and the first results (see the (e) bullet under Done).
- Cleaned `internal/app/testdata/e2e/state/store` (gitignored) that an earlier manual run wrote into
  the recording; `TestE2EFixtureHygiene` enforces its absence.
- Fixed the last failing `ri impact --render` test assertion: `recordedState(t)` scrubs PATH (hermetic
  offline replays), so the kustomize pair fails `renderer-unavailable` in tests and
  `kustomize-dependency` with kubectl on PATH — the test accepts either (R13 wants an explicit
  failure, not a specific reason).

Uncertainties for the returning agent:

- The applicability lane's condition evaluator has not landed; the seam is unilaterally chosen (a
  standalone tri-state in `internal/render`, not a `ConditionResult`). Confirm the signature with that
  lane before building on it.
- **R16 CRD gap (open):** the packaged cert-manager chart's CRD changes (e.g. the new
  `signatureAlgorithm` field) did not appear in the rendered delta although the renderer passes
  `--include-crds` (46 → 48 objects, no CustomResourceDefinition among them). Decide whether CRD
  objects are meant to reach the rendered stream, and if so why helm's output drops them.
- The eval/render authoring was not fully blind (the lane status prose and one live run were seen
  before the case files were written) — the caveats and mitigations are in each case's NOTES.md; a
  reviewer should weigh them before treating the results as a clean baseline.
- The eval runner lives in `internal/app/render_eval_test.go` (it needs the app's chart resolution);
  if the eval tooling should expose the metrics beyond `go test -v` (like `ri eval`), that is new
  CLI surface for the commander to route.
- `recordedState(t)` in `cmd/ri` scrubs PATH (hermetic offline replays), so the impact-render test
  sees `renderer-unavailable` for kustomize; a machine with kubectl on PATH sees `kustomize-dependency`
  instead. The test accepts either (R13 wants an explicit failure, not a specific reason).
