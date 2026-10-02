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

## Proposals to other lanes (not done here: outside ownership)

- **knowledge:** set the candidate's renderability for routing with
  `domain.EffectiveRenderability(c, vs)`. Today `AutoApproveRenderVerifiable` keys on
  `c.Renderability`, which no production code sets, so the R10 policy is inert.
- **validate / knowledge CLI:** register `app.RenderValidator(kube)` beside `semvalidate.Validators()`
  in `ri knowledge validate`.
- **applicability:** wire `impact.Input.Render` with the adapter in docs/RENDER.md over
  `RenderDiffResult.EnvironmentPairs()` behind `ri impact --render`. The adapter cannot set
  `ConditionResult.deps` (unexported), so `not(rendered-change)` needs the engine to record the
  `render` dimension itself.
- **semantic:** cite `render.EdgeRenderedChanges(...).ForChanges(memberIDs)` in prompts (release scope
  only).
- **dashboard:** render evidence already reaches `knowledge.ReviewContext` through the `rendered-diff`
  `ValidationResult` (RenderRelation + `rendered-diff` evidence with object/path/before→after), so no new
  `ReviewContext` field is needed for a "Rendered delta" panel. Undocumented rendered changes
  (`EdgeRendered.Undocumented()`) need a candidate producer to become review items.

## Open / honest gaps

- Pipeline-level R17 metrics (UNKNOWN → decided due to render, ACTION strengthened, false ACTION delta,
  applicability before/after on the Phase 3 corpus) need the applicability wiring and render-backed
  facts; not measured yet.
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
- `internal/app/render.go`, `internal/app/render_eval_test.go`, `cmd/ri/{main,impact,render}.go`,
  `README.md` usage lines.

## Tests

`go build ./... && go vet ./... && go test ./...` green (2026-10-02). Live tests skip without
helm/kubectl; the eval runner needs network and helm.
