# Rendering validation (`internal/render`)

Implementation notes for the render lane (mission: `docs/phase3/learning-loop/RENDER-MISSION.md`
R1–R19; design: `docs/phase3/learning-loop/DESIGN.md` §1.3, §2.3). The renderer answers one question
honestly: **what actually changes for this customer's configuration** — by rendering the source and
target releases with that configuration (`helm template` / `kustomize build`) and diffing the objects.
It is never universal truth (R12): a render shows manifests, not runtime behaviour.

```go
eng := a.RenderEngine()                       // helm + kustomize + chart resolution, cached under <state>/render
res, _ := a.RenderDiff(ctx, "cert-manager", "v1.17.0", "v1.18.0", app.RenderOptions{
    Repo: "customer-repo", KubeVersion: "1.31", Release: true,
})
// res.Release: chart-default pair (release level); res.Pairs: one per detected deployment
// res.Correlations: rendered changes ↔ edge changes (by target id)
```

## Scope: release vs environment

Two render scopes exist and never mix (R5):

- **release** — chart defaults only. The `render.rendered-diff@v1` knowledge validator runs on these;
  its evidence may enter `knowledge/` and prompts.
- **environment** — the customer's configuration. These renders back impact findings only
  (`EvaluateRenderedChange`, below); `ValidationResult.Validate` rejects them for knowledge, and their
  values are hidden unless `--show-values` (secrets and credential-shaped values are digests always).

## Renderers

`helm template --skip-tests` (test hooks are not deployed) and `kustomize build` (or `kubectl
kustomize`). External binaries run with a scrubbed environment — empty `KUBECONFIG`, private
`HELM_*` directories — so a render never depends on the caller's cluster or repositories. Every run
records full `RenderProvenance` (tool+version, command with stable names, chart name/version/URI/
digest/representation, release name/namespace, kube version/api-versions, per-layer values digests,
values completeness, kustomize root/dir/input digests, output digest, limitations) and is
content-addressed in `<state>/render/rk-…/`. Each Helm render runs twice; fields that differ between
identical runs (random tokens, generated certs) are suppressed as `nondeterministic` noise.

### Failures are explicit (R13)

A render that cannot happen fails with a classified reason — an evidence gap (UNKNOWN / render
failed), never "no change":

| Reason | Meaning |
|---|---|
| `chart-unavailable` | the chart package could not be resolved or fetched (offline: not in cache) |
| `renderer-unavailable` | the helm/kustomize binary is not installed |
| `missing-dependency` | a chart dependency (subchart) is not packaged |
| `invalid-values` | the values do not parse, violate `values.schema.json`, or a chart-authored `fail`/`required` check rejects them. When the source rendered the same values, the pair reports **TARGET CHART REJECTS THIS CONFIGURATION** (`Pair.TargetRejects`): the upgrade with these values fails at render time |
| `missing-capability` | kubeVersion / API capability the chart requires is absent |
| `template-error` | a template failed to execute (a parse error, a nil pointer: a broken template) |
| `kustomize-dependency` | a kustomize base/component/resource could not be loaded (includes load-restrictor violations — fidelity to how the customer's deployer builds) |
| `unsupported-feature` | an input feature the renderer path does not support |
| `output-unparsable` | the renderer's output is not a YAML stream of objects |

Values that reference cluster objects (`valuesFrom` secrets/configmaps, templated Helmfile values,
secrets) mark the target **INCOMPLETE**: an absence of change in an incomplete render decides nothing.

## Customer configuration first (R7)

`--repo` discovers deployments by convention; every target records **why** it was chosen:

| Target | Source | Notes |
|---|---|---|
| `values-files` | `--values`, or a repo-local values file naming the product | |
| `argocd` | Argo CD Application | values, `valuesObject`, parameters, `$values/` valueFiles; other value sources ⇒ incomplete |
| `flux` | Flux HelmRelease | `spec.values`; `valuesFrom` ⇒ incomplete |
| `helmfile` | Helmfile release | files/inline/set; templated/secrets ⇒ incomplete |
| `kustomize` | Kustomize overlay | tied to the environment (Argo source path, `--overlay`, else top-level kustomizations referencing the product); rendered with the product version substituted (recorded); an overlay that does not pin the version is `not-applicable` |
| `chart-defaults` | `--chart-defaults` | release level |

## Object model and semantic diff

Identity is `<group>/<version>/<kind>/<namespace>/<name>` (`core` for the core group), matched
version-independently across renders (api-version-changed). Normalization: empty ≡ omitted; Secret
data and credential-named values compare by digest only. Lists Kubernetes merges by key are diffed by
that key (containers/env/volumes/ports/…); RBAC rules diff as permission sets (`group × resource ×
verb [× resourceName]`); container args diff by flag with ordering preserved (a pure reorder is one
`container-arg-changed`). Chart-metadata stamps (`helm.sh/chart`, `app.kubernetes.io/version`) and
checksum annotations are suppressed as counted noise.

### Path syntax

Diff `Change.Path` and `rendered-change` condition paths share one syntax (the condition resolver also
accepts the `Pattern` form):

- dotted keys: `spec.template.spec.containers`
- keys holding separators are quoted: `metadata.labels["app.kubernetes.io/name"]`
- lists merged by key are addressed by that key: `containers[name=controller].image`,
  `ports[port=443/TCP]` (the protocol suffix; TCP is the default)
- `[]` addresses every element — `Pattern` reduces every keyed selector to this, matching the env/CRD
  SchemaPath syntax the condition language uses: `spec.template.spec.containers[].image`

## Correlation, evidence, validation

- **Correlation** (`Correlate`): each rendered change is correlated with the edge's changelog entries
  (rules: subject, mentions-name/-permission/-object/-api-version/-field, values-default, artifact);
  undocumented rendered changes are listed.
- **Evidence** (`Pair.Evidence`): `EvidenceRenderedDiff` records citing the object, path, change,
  provenance and digests. Release-scope evidence may back knowledge; environment-scope evidence backs
  impact findings only, values hidden unless `--show-values`.
- **`rendered-diff` validator** (`validator.go`, DESIGN §2.3): checks subject+change aspects of an
  assertion against the release-level pair and records a `RenderRelation`:

| Relation | Meaning |
|---|---|
| `confirmed-by-render` | the renders show the asserted change (needs a confirmed check, no refuted check, rendered evidence) |
| `contradicted-by-render` | the renders show something else (a refuted check) |
| `not-visible-in-render` | the subject is in neither render, the render failed, or the family has no rendered shape (inconclusive — not wrong) |
| `render-not-applicable` | the change is not render-verifiable (runtime-only) |

- **Renderability** (R12, data in the domain: `domain.RenderabilityOf`, `AssessRenderability`,
  `EffectiveRenderability`; `internal/render` delegates): each family × change kind maps to `render-verifiable`
  (resources, RBAC, images, args, env vars, ports, labels, annotations, API versions),
  `partially-render-verifiable` (defaults, feature activation, cross-resource relationships) or
  `not-render-verifiable` (runtime behaviour, protocol semantics, migrations, performance). A GVK's
  value-changed / default-changed (its **storage version**) is not render-verifiable: rendered objects
  carry served versions only, and the storage flag is applied by the API server. `rendered-diff`
  answers `render-not-applicable` without rendering, and the `crd-schema` validator decides it from the
  schema (VALIDATOR-AUDIT.md).
  `RenderedClasses` lists the structural classes eligible for auto-approval under the default policy
  (R10) — the knowledge lane's `AutoApproveRenderVerifiable` consults candidate renderability.
  `domain.EffectiveRenderability(candidate, validations)` is what routing should use: the candidate's
  own assessment, else that of an assertion whose subject and change a render **confirmed** — never a
  model claim alone. (The table lives in the domain because `internal/knowledge` cannot import this
  package: render imports knowledge for the Validator port.)
- **Wiring:** `app.Validators(kubeVersion)` is the registry: the validate lane's
  `semvalidate.Validators()` followed by `rendered-diff`. Knowledge routing hands the auto-approval
  policy the candidate's `domain.EffectiveRenderability`. `knowledge.ReviewContext.Render`
  (`RenderEvidenceOf`: the most decisive render validation, with release-scope records carrying
  before/after) feeds the dashboard's Rendered delta panel (`reviewui` `deltaFromContext`).
- **Prompt evidence** (`EdgeRenderedChanges`, semantic lane addendum): the edge's release-level rendered
  changes as citable `rendered-diff` evidence, correlated with the edge changes; `ForChanges(memberIDs)`
  selects what a candidate restates, `Undocumented()` the rendered changes no entry mentions (review
  material). It refuses any pair whose renders are not release scope, so customer values cannot reach
  a prompt (`TestEnvironmentRendersNeverReachPromptsOrKnowledge`).

## `rendered-change` leaf (`EvaluateRenderedChange`, DESIGN §1.3)

```go
res := render.EvaluateRenderedChange(cond, envPairs) // RenderedChangeResult
// res.Value: "true" | "false" | "unknown"
```

A tri-state predicate over the **environment** pairs: does the field at `cond.Path` of the objects
matching `cond.Group/Kind/Name` differ between the From and To renders the way `cond.State` says
(changed / unchanged / added / removed), with the To value in `cond.Values` when given (JSON, string,
or image-tag equality)?

- **true** — any pair shows it (even one with incomplete values: a true backed by rendered evidence
  stands); `Matches` holds one `rendered-change` match citing that pair's environment-scope records.
- **false** — every pair rendered successfully with complete values and none is in the asked state —
  including an object or path absent from both renders. It carries a `Checks` entry (dimension
  `render`, objects compared) and `Examined` state records per side (DESIGN rule 4: every false carries
  a check). A failed or values-incomplete pair keeps the leaf unknown: an absence of change there
  decides nothing.
- **unknown** — reason `environment-visibility-gap`, `Needed` says why: no renders, a failed render,
  incomplete values, or a condition naming one object (`Name`) that is absent while the release name
  was *assumed* (object names derive from it).

The result's fields mirror `impact.ConditionResult` one to one. `render.ConditionEvaluator{Pairs}`
implements `impact.RenderedChangeEvaluator`, and `app.ImpactRun` wires it: with `--render` (or
`ImpactOptions.Render`) the From/To releases are rendered with the environment's configuration
*before* the join, and `impact.Input.Render` decides the facts' rendered-change leaves against the
environment pairs (never the chart-default pair). The impact engine records the `render` dimension
on decided render leaves, so `not(rendered-change)` labels its check correctly.

The leaf concludes nothing about consequences: a render difference alone never produces ACTION
REQUIRED (R11) — that classification is the trust ladder's, above this predicate.
`TestRenderDeltaAloneNeverYieldsAction` follows "RBAC verb removed" from the render through validation
and routing: one model call ⇒ no fact (review); two agreeing calls without an action request ⇒ an
auto-approved consensus fact, untrusted, capped at REVIEW; only PO-2 (every agreeing call requests
action-required) yields `ConsensusAction` — "ACTION REQUIRED · model consensus", always audited.

## Evaluation (R16, R17)

`eval/render/` holds the render cases: expectations authored from upstream material
(the chart template/CRD diff between the tags, values.yaml, release notes) before the
renderer ran on them — see its README and each case's NOTES.md, which also records
every blind-authoring caveat and every correction the comparison forced. The runner is
`go test ./internal/app -run TestEvalRenderCases -v` (skips without helm/network) and
reports per case: pairs rendered/failed (render success rate), recall (expectations
matched) and, where the expectations aim at the whole delta, precision (changes
explained). Committed comparisons live in `eval/render/results/`; the current one
(2026-10-02): release level precision 14/14, recall 14/15 (R12, a real diff-model gap:
a role new under its name is one `resource-added` record, so its permissions are not
emitted as `rbac-permission-added`), the `servicemonitor` and `crds.enabled` variants
2/2 each, the kustomize overlay failing exactly as authored. cert-manager ships its
CRDs as a gated template (`crds.enabled`, default false), so they appear only in the
variant.

**Pipeline-level R17** (`ri eval -render [-render-json stats.json]`): every environment case is
rendered with its own configuration before the join, and an R17 section follows the report. It
covers render success, target-chart rejections, rendered and undocumented changes, unknowns
restated by a customer render, UNKNOWN → decided by render, ACTION with render evidence, and ACTION
corroborated by render. The stored results stay the render-free baseline (the before; `-update`
refuses `-render`). `eval/render/tools/r17_join.py` joins the stats with the eval's per-link results.
Results (`eval/render/results/2026-10-02-pipeline.md`):
- before and after are identical on all 47 aggregate fields: false-ACTION delta 0, UNKNOWN → decided 0;
- why: no verified facts exist yet, and rendering added no false certainty;
- render success 11/12 pairs;
- ACTION corroborated by render 4/15;
- 8 of 55 missed expected links (6 action) have their change restated by a complete customer render,
  but so do 4 of 32 not-affected links. A restatement is not exposure; render-backed facts with
  conditions must decide.

## CLI

```sh
ri render diff cert-manager v1.17.0 v1.18.0 --repo customer-repo --kubernetes 1.31 --chart-defaults
ri impact cert-manager v1.17.0 v1.18.0 --repo customer-repo --kubernetes 1.31 --render
```

`ri render diff` prints (or `-o json`) the rendered delta per target: provenance digests, per-change
lines with class and path, the correlation with the changelog, undocumented changes, and per-target
verdicts. `ri impact --render` appends the same section under the impact report (text) or adds a
`render` key beside the report fields (json) — the minimum workflow: the report, then what actually
changes for this environment. Shared flags: `--values`, `--repo`, `--overlay`, `--release-name`,
`--namespace`, `--kubernetes`, `--api-versions`, `--chart-defaults` (render diff), `--show-values`.

## Live results (cert-manager v1.17.0 → v1.18.0, `--kubernetes 1.31`)

Chart defaults: 14 rendered changes (46 → 48 objects, 96 chart-metadata stamps suppressed), correlated
with the image bumps (artifact), the acmesolver image arg and the ServiceMonitor `targetPort` default
(values-default); undocumented: the webhook probe port 6080 → `healthcheck` and the challenges
ClusterRole/Binding split into `dns01-`/`http01-` roles. Customer repo: Argo 15 changes, Flux 14
(values INCOMPLETE via `valuesFrom`), Helmfile prod 11 (`targetPort` pinned ⇒ no Service change),
dev 14; the Kustomize overlay fails explicitly (`kustomize-dependency`: the fixture references files
outside the overlay dir — the customer's deployer would fail the same way).

## Live results (ingress-nginx controller-v1.11.5 → controller-v1.12.0, chart 4.11.5 → 4.12.0)

Chart defaults: 8 rendered changes (18 objects on both sides, 42 chart-metadata stamps suppressed),
7 correlated with changelog entries: the controller's `--enable-metrics=false` argument disappears
("Metrics: Disable by default" — the default moved into the binary), `runAsGroup` is now set explicitly
on the controller and both admission Jobs ("Chart: Explicitly set `runAsGroup`"), the controller image
bump, and the admission webhook certgen image moving **v1.5.2 → v1.5.0** (a downgrade, also on the edge).
Undocumented: the controller ConfigMap loses its `data` (`allow-snippet-annotations: "false"`).
