# Lane `render` — status

Branch `p3ll/render` (worktree `.claude/worktrees/render`), merged with `p3-learning-loop` @ 1174492
(contract-3 PO-1/PO-2, knowledge-3). Mission: `RENDER-MISSION.md`; brief: `briefs/render.md`;
user-facing doc: `docs/RENDER.md`.

## Done (brief order)

- **(a) renderers + model + diff + CLI** — `internal/render`: Helm (`helm template`) and Kustomize
  (`kustomize build` / `kubectl kustomize`) adapters behind a `Renderer` port (scrubbed env, no cluster
  access); full `Provenance` + content-addressed cache; every R13 failure class explicit (never "no
  change"); nondeterminism probe (render twice, suppress differing fields); normalized objects with
  identity `<group>/<version>/<kind>/<namespace>/<name>`; semantic diff (R4 classes, keyed lists, RBAC
  permission sets, flag-keyed args, counted noise rules); customer configuration first (R7: `--values`,
  Argo CD, Flux, Helmfile, repo values files, Kustomize overlays with version substitution, each with a
  recorded `why` and values completeness); changelog correlation + undocumented list;
  `ri render diff`; chart resolution through the existing artifact path (`ChartPackageReader`s, fetch
  cache).
- **(b) evidence + `rendered-diff` validator** — `rendered-diff` evidence (R5) with both endpoints,
  object, path; `render.rendered-diff@v1` validator over release-level renders sets the contract's
  `RenderRelation` (confirmed / contradicted / not-visible / not-applicable), checks subject + change
  only; subject `Inventory` for rbac-permission, image, cli-flag, env-var, feature-gate, gvk.
  Renderability (R12) is data in the domain (`domain.RenderabilityOf`, `EffectiveRenderability`).
  `app.RenderValidator(kube)` for registration beside the validate lane's validators.
  `EdgeRenderedChanges` exposes release-level rendered changes as citable prompt evidence
  (semantic-lane addendum); environment renders are refused.
- **(c) `rendered-change` evaluator** — `render.EvaluateRenderedChange(cond, pairs)`; its result
  mirrors `impact.ConditionResult` field for field (Matches / Checks / Examined / Reason / Needed /
  Records). true needs rendered evidence; false needs complete successful renders and carries a
  `render` check; unknown otherwise. `ri impact --render` (minimum workflow) appends the rendered delta
  to the report (JSON: a `render` key beside the report). **RENDER EVALUATOR READY** (stated to the
  commander; adapter snippet in docs/RENDER.md).
- **(d) auto-approval** — the policy itself is the knowledge lane's (`AutoApproveRenderVerifiable`,
  `AutoApproveConsensusAction`, audit sampling). The render lane supplies its inputs: the
  `RenderRelation`-bearing validation and `domain.EffectiveRenderability`. Adversarial R11 test
  `TestRenderDeltaAloneNeverYieldsAction` runs validator → `knowledge.RouteWith`:
  - a render delta plus one model call ⇒ no fact (review);
  - two agreeing calls without an action request ⇒ an auto-approved consensus fact, untrusted ⇒ at
    most REVIEW;
  - only PO-2 (every agreeing call requests action) ⇒ `ConsensusAction`, labelled "model consensus".
- **(e) evaluation** — `eval/render/` (2 cases + 2 variants), runner
  `go test ./internal/app -run TestEvalRenderCases -v`. Results 2026-10-02
  (`eval/render/results/2026-10-02.md`):
  - render success 7/8 pairs (the 1 failure is the expected explicit kustomize-dependency);
  - cert-manager release level: precision 14/14, recall 14/15 (R12: a real diff-model gap);
  - variants: servicemonitor 2/2, crds.enabled 2/2;
  - kustomize: 2/2.
  - Second live product: ingress-nginx 1.11.5 → 1.12.0 (docs/RENDER.md). 8 changes, 7 correlated.
    Undocumented: the controller ConfigMap loses `allow-snippet-annotations: "false"`; the certgen
    image moves v1.5.2 → v1.5.0.
- **(f) pipeline-level R17 metrics** — `ri eval -render`: every environment case renders with its own
  configuration before the join (`renderEval` over `app.ImpactRun`), an R17 rendering section follows
  the report, `-render-json` writes per-case stats, `-update` refuses `-render` (stored results stay
  the render-free baseline). Chart-authored `fail`/`required` rejections are `invalid-values`
  (`Pair.TargetRejects`, shown as "TARGET CHART REJECTS THIS CONFIGURATION"); `mentions` no longer
  counts wildcards/punctuation as documented. Results `eval/render/results/2026-10-02-pipeline.md`
  (join tool `eval/render/tools/r17_join.py`): before/after identical on all 47 aggregate fields —
  false-ACTION delta 0, UNKNOWN→decided 0 (honest: no verified facts exist yet, the semantic lane is
  unmerged; rendering added no false certainty); render success 11/12 across the environment cases
  (the one failure is a real finding: cilium 1.15→1.17 refuses `containerRuntime.integration`,
  corroborating the deterministic values-removed ACTION); ACTION corroborated by render 4/15; 8
  missed expected links restated by a complete render vs 4 not-affected links also restated — the
  seam render-backed facts must decide.

## Review of the GLM handoff (12 `[glm-handoff]` commits)

Kept: validator/inventory tests, the evaluator's path resolver and tests, `ri impact --render`,
docs/RENDER.md, the eval runner and cases. Fixed:

1. **Evaluator contract gaps:** false carried no `ImpactCheck` (DESIGN rule 4) and true no
   `ImpactMatch`, and absence from both *complete* renders stayed unknown. Now false carries a render
   check plus examined state records, true carries a `rendered-change` match, and absence decides false.
   One exception stays unknown: a name-matched object under an assumed release name. Two GLM tests that
   pinned the old behaviour were updated.
2. **eval integrity:** R12 had been dropped *after* a comparison run, hiding a real tool limitation.
   It is restored (`knownGap`, counted as a miss).
3. **eval error:** the "R16 CRD renderer gap" was a misread of the chart. cert-manager ships its CRDs
   as `templates/crds.yaml`, gated by `crds.enabled=false`. R16/R17 moved into a `crds.enabled`
   variant. R17's matcher was narrowed from `default` to `rotationPolicy.default`, because the loose
   form matched description prose. All recorded in the case NOTES.md.
4. A mis-dated results file (2026-10-11) was renamed; three unformatted test files were gofmt'ed.
5. Missing brief items added: the R11 adversarial test, the prompt-evidence function, and the
   environment-renders-never-reach-prompts/knowledge test.

GLM's own disclosure stands: the cert-manager case was not authored fully blind (the lane status prose
and one live run were seen first). Weigh its numbers accordingly. The ingress-nginx run is a live
demonstration, not a scored case.

## Integration wiring (render-2, commander request; minimal additive hunks in other lanes' packages)

- **applicability:** `render.ConditionEvaluator` implements `impact.RenderedChangeEvaluator`.
  `app.ImpactRun` renders before the join, and `ri impact --render` decides facts' rendered-change
  leaves against the environment pairs. `internal/impact/condition.go` (one hunk,
  `CONTRACT-CHANGE(render)`) records the `render` dimension on decided render leaves (`deps` is
  unexported).
- **validate:** `app.Validators(kube)` = `semvalidate.Validators()` + `rendered-diff` (test-pinned).
  `ri knowledge validate` itself is still the validate/semantic lanes' to wire.
- **knowledge:** `route.go` (one hunk) hands the policy a copy of the candidate with
  `domain.EffectiveRenderability`. The R11 test now relies on routing deriving it.
  `ReviewContext.Render` + `RenderEvidenceOf` (new `knowledge/render.go`), filled by the queue.
- **dashboard:** `reviewui` `renderedDelta()` maps `ReviewContext.Render` (`deltaFromContext`) when no
  `RenderedDeltaSource` answers. `RenderProvenance.Before/After` (release scope) gives the panel source
  vs target values.
- **semantic (still open):** cite `render.EdgeRenderedChanges(...).ForChanges(memberIDs)` in prompts.
  Undocumented rendered changes (`EdgeRendered.Undocumented()`) need a candidate producer to become
  review items.

## render-3 (validator audit fix, branch `p3ll/render-3`)

VALIDATOR-AUDIT.md found that `rendered-diff` refuted the true NodePool/NodeClaim storage-version
moves (`v1beta1 → v1`). The render showed both versions served on both sides and read that as "the
rendered value did not change", but storage is a CRD flag, not observable in rendered objects. Fix:
`domain.RenderabilityOf` classifies `gvk` × `value-changed`/`default-changed` as not-render-verifiable
(data, `valuelessFamilies`). The validator returns `render-not-applicable` with inconclusive checks
and never renders. Served-version changes (added/removed/renamed) stay render-verifiable. Regression
test `TestRenderedDiffStorageVersionIsNotApplicable` reproduces the audit's CRD shape: it failed with
the old table (refuted) and passes now. Side effect, intended: storage moves are no longer eligible
for render auto-approval (R10); the `crd-schema` validator still confirms them deterministically.
Waiting on the product-owner decision about render-backed classification of unset changed defaults.

## Open / honest gaps

- Pipeline-level R17 (before/after on the corpus) is **measured** (see (f)); the follow-on — re-run
  `ri eval -knowledge <dir> -render` once candidates/proposals exist for the 8 restated links, per
  verification level — needs the semantic lane's facts; not possible yet.
- R12 diff-model gap: permissions of a Role/ClusterRole that is new under its name are not emitted
  per permission. Possible follow-up: emit per-permission records for added/removed roles. The
  trade-off is noisier output.
- No env-var / API-version change between the cert-manager tags; the testdata chart covers both
  classes, but no scored upstream case does.
- R8 counterfactual variants are supported by the engine (`Variant`), but nothing asks for them yet:
  a candidate-driven trigger needs the knowledge pipeline.
- Kustomize: `helmCharts` overlays are an explicit `unsupported-feature`; remote bases are recorded as
  limitations, not digested.

## Decisions (why)

- Paths: keyed selectors for display, `[]` `Pattern` (env/CRD SchemaPath syntax) for conditions.
- `--skip-tests`; Kustomize default load restrictions kept (fidelity to the customer's deployer).
- Environment values hidden unless `--show-values`; Secret/credential values always digests.
- The renderability table lives in the domain (knowledge cannot import render: import cycle).

## Files touched outside ownership (all additive, `CONTRACT-CHANGE(render)` where contract)

- `internal/domain/evidence.go` (`EvidenceRenderedDiff`), `semantic.go`
  (`RenderProvenance.{FromArtifact,ToArtifact,Object,Path,Change}`, `RenderArtifact`), `impact.go`
  (`DimensionRender`, `MatchRenderedChange`), new `renderability.go`; `schemagen/meta.go` + regenerated
  `schemas/*.json`.
- `internal/sources/artifacts.go` `ChartPackage.Archive`, set in `internal/helm/package.go` and
  `internal/oci/chartlayer.go` (one line each).
- render-2 wiring: `internal/impact/condition.go` (render deps), `internal/knowledge/{route.go,api.go,queue.go}`
  + new `render.go` (`ReviewContext.Render`, `RenderEvidence`, `RenderEvidenceOf`),
  `internal/reviewui/{rendered.go,server.go}`, `internal/domain/semantic.go`
  (`RenderProvenance.Before/After`), `internal/app/{impact.go,validators.go}`.
- `internal/app/render.go`, `internal/app/render_eval_test.go` (also: the case loader skips
  `eval/render/tools/` beside `results/`), `cmd/ri/{main,impact,render,eval}.go` (eval: `-render`,
  `-render-json`), `README.md` usage lines.

## Tests

`go build ./... && go vet ./... && go test ./...` green (2026-10-02, re-verified after the handoff
commits below). Live tests skip without helm/kubectl; the eval runner needs network and helm.

## GLM handoff log (2026-10-02, second GLM window)

The Claude lead paused mid-item with the pipeline-level R17 measurement uncommitted on the tree. I
reviewed, verified, finished and committed it (4 commits, `d8728b8`…`7ceb245`):

1. Chart-authored `fail`/`required` checks (`execution error at (` on stderr) classify
   `invalid-values` — distinct from genuine template bugs (`template: …` prefix), pinned by two new
   failure-test cases. `Pair.TargetRejects()` + a `ri render diff` line surface "the upgrade with
   these values fails at render time". Display path is nil-safe (`Pair.Failure` is assigned from
   the failing result, pair.go:107).
2. `mentions` requires an alphanumeric rune in the needle, so `"*"` permissions and `"-"` args are
   never "documented" by markdown bullets (new `correlate_test.go`).
3. `ri eval -render` / `-render-json` (`cmd/ri/eval_render.go`): per-case render stats, R17
   rendering section, `-update` refuses `-render`.
4. `eval/render/tools/r17_join.py` + `eval/render/results/2026-10-02-pipeline.md`; the eval case
   loader skips the new `tools/` directory (a broken `case.yaml` elsewhere still fails loudly).

Fixes I made to the inherited tree: gofmt on `cmd/ri/eval_render.go` and `internal/render/helm.go`
(the first GLM window had left unformatted files too); the loader skip above (the new tools dir
broke `TestEvalRenderCases`). Everything else was committed as reviewed, unchanged.

Uncertainties / not done (deliberate):

- The R12 per-permission follow-up and an env-var/API-version upstream case stay open (trade-offs
  already recorded above); I did not start them — they change output noise and scored-case
  surface, which felt like the paused lead's / commander's call, not a local default.
- The results file's numbers (47 identical fields, 8 restated links) are the Claude lead's run; I
  verified the code paths and the join tool's logic, but did not re-run the 28-entry live eval
  (needs the warm state and network; the stored JSONs were not checked in).
- Reran the full `go build/vet/test` — green — and smoke-tested the new flags in the built binary.

### Review of the second GLM window (by the returning Claude lead)

The 5 commits bank the lead's own uncommitted work. I checked the diff: nothing was altered apart
from gofmt, plus one correct fix (the eval-case loader skips `eval/render/tools/`, which the new join
tool would otherwise have broken). One error was mine and is corrected: the results file said 8
manifest-only cases, but it is 7 (19 env cases − 12 with values files).
