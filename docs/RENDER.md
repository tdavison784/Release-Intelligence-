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
| `invalid-values` | the values do not parse or violate `values.schema.json` |
| `missing-capability` | kubeVersion / API capability the chart requires is absent |
| `template-error` | a template failed to execute (required/fail/parse) |
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

- **Renderability** (`renderability.go`, R12): each family × change class maps to `render-verifiable`
  (resources, RBAC, images, args, env vars, ports, labels, annotations, API versions),
  `partially-render-verifiable` (defaults, feature activation, cross-resource relationships) or
  `not-render-verifiable` (runtime behaviour, protocol semantics, migrations, performance).
  `RenderedClasses` lists the structural classes eligible for auto-approval under the default policy
  (R10) — the knowledge lane's `AutoApproveRenderVerifiable` consults candidate renderability.

## `rendered-change` leaf (`EvaluateRenderedChange`, DESIGN §1.3)

```go
res := render.EvaluateRenderedChange(cond, envPairs) // RenderedChangeResult
// res.Value: "true" | "false" | "unknown"
```

A tri-state predicate over the **environment** pairs: does the field at `cond.Path` of the objects
matching `cond.Group/Kind/Name` differ between the From and To renders the way `cond.State` says
(changed / unchanged / added / removed), with the To value in `cond.Values` when given (JSON, string,
or image-tag equality)?

- **true** — any pair shows it; evidence cites that pair's rendered-diff records.
- **false** — every pair rendered successfully with complete values and shows something else. This is
  evidence, and it requires completeness: a failed or values-incomplete pair keeps the leaf unknown,
  because an absence of change there decides nothing.
- **unknown** — reason `environment-visibility-gap`: no renders, a failed render, incomplete values,
  the object or path absent from both renders.

The leaf concludes nothing about consequences: a render difference alone never produces ACTION
REQUIRED (R11) — that classification is the trust ladder's, above this predicate.

## Evaluation (R16, R17)

`eval/render/` holds the render cases: expectations authored from upstream material
(the chart template/CRD diff between the tags, values.yaml, release notes) before the
renderer ran on them — see its README and each case's NOTES.md, which also records
every blind-authoring caveat and every correction the comparison forced. The runner is
`go test ./internal/app -run TestEvalRenderCases -v` (skips without helm/network) and
reports per case: pairs rendered/failed (render success rate), recall (expectations
matched) and, where the expectations aim at the whole delta, precision (changes
explained). Committed comparisons live in `eval/render/results/`; the current one:
release level 14/14 precision, 15/16 recall (one marked known gap: CRD fields), the
kustomize overlay failing exactly as authored. The pipeline-level R17 metrics (UNKNOWN
→ decided due to render, ACTION strengthened, false ACTION delta, applicability
before/after) wait on the applicability lane's wiring.

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
