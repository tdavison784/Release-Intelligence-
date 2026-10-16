# Status — lane `renderdiff` (Claude Opus 5.5, agent ri-renderdiff)

Branch `p4ei/renderdiff`, worktree `.claude/worktrees/p4ei-renderdiff`. Brief: `briefs/renderdiff.md`
(+ `_wave1-common.md`, DESIGN §2.4 / §7 "renderdiff"). Merged `p4-env-impact` @ 918e855a (bundle lane).
State: **DONE — follow-up 1 (DESIGN §1.3 amendment) complete** (2026-10-16). Resume from: this file +
brief + `git log`. Since follow-up 1 the lane also owns `internal/investigate/bundle/` (bundle lane finished).

## Done

- `internal/investigate/renderdiff/` (owned):
  - `renderer.go` — `New(app, st) investigate.EnvironmentRenderer`. Inputs from the snapshot (repo path,
    values inputs, cluster version; `cfg.KubeVersion` wins). Targets via `app.RenderTargets`, rendered with
    `app.RenderEngine().RenderPair`, correlated with `render.Correlate`. Pair status: `ok` only when both
    renders succeeded, values complete and pair-level chart coverage holds; else `partial`/`unavailable`
    with `render.FailureReason`-prefixed reason and no changes; consumer Kustomize overlays that neither pin
    nor run the product → `not-applicable` (`version-independent:`), overlays that run the product's images
    unpinned → `unavailable`. Names, reasons and provenance are repo-relative (ids independent of checkout
    path). Errors only for infrastructure (missing edge artifact → permanent; store → transient; ctx).
  - `index.go` — artifacts `derived/render/env/<slug>/{from.yaml,to.yaml,diff.json}`; `DiffDocument`
    (`ri.dev/render-diff/v1alpha1`, timestamps stripped); one `RenderedChange` per `render.Change` of an ok
    pair; correlation: Correlate links → items that exist (routine dropped), `artifact` links → computed
    image items naming the repository (bundle's release-scope rule), `release-anchor` → items anchored on
    the matching release-scope rendered change; one `rendered-diff` evidence per change (contentDigest =
    diff artifact digest, locator `changes[id=<rc>]`, values hidden).
  - `stream.go` — stored renders are the normalized (redacted) objects with `# Object:` headers; path
    parser/resolver for the render path syntax (+ member selectors).
  - `query.go` — `Query` (DESIGN signature), `QueryPage` (filters, fixed order, limit 50/200, 256 KiB page
    bound, `After` cursor; rejects unknown target/item/class/scope/cursor), `Object` (from/to, whole object or
    field, Found=false for absence, ErrNotFound for an unrendered side, 64 KiB bound), `Summaries`.
  - `measure.go` — `Measure(pairs)` coverage counts.
  - Tests: fake helm/kustomize renderers + chart resolver + in-memory store over the contract's sample
    bundle (bundle `Validate`s; every change resolves to the bytes, `QuoteOccurs` on Object text; secrets
    never stored; incomplete pairs; kustomize; layered values; release anchor; artifact attribution;
    determinism; errors), query/object/summary/path-parser tests, and the opt-in real-render
    `TestGroundtruthEnvironments` (`RI_RENDERDIFF_STATE=<warm .ri>`; skips without helm/kubectl).
- `docs/INVESTIGATE.md` — "Render diffs" section.

## Follow-up 1 (commander review of 5521569a; amendment adf4fe08) — done

1. **`scope:routine-self-artifact`** — `render.SelfArtifactBump(change, edge)`: an `image-changed` (or an
   argument `--flag=<image>`) whose before/after are one repository of the edge's container-image
   artifacts (they come from the product definition) at exactly that artifact's from/to tags. Declarative:
   reads the edge only; a mirrored repository or another tag does not qualify (→ unexplained, fail closed).
   Out-of-scope item anchored on the change, so it is correlated.
2. **`scope:rendered-unexplained`** — every other rendered change nothing explains gets its own in-scope
   item (family `rendered`, `rendered-config`, `manifest:rendered-anchor`, evidence = the change's own
   rendered-diff record):
   - release scope: bundle collector `anchorUnexplained` (was `anchorStructural`): structural →
     `scope:rendered-structural` (unchanged), self-artifact → `scope:routine-self-artifact`, else
     `scope:rendered-unexplained`;
   - environment scope: renderdiff, after Correlate links, computed-image attribution and
     `release-anchor`, anchors the rest on **environment items** (`RenderResult.Items` → assembler →
     `EvidenceBundle.EnvironmentItems`). Environment items cannot live in the change record (it is
     environment-independent; its id is the collect-change idempotency key), hence the bundle field.
   - domain: `ChangeItem` rule shape check (record + environment items), environment-item checks in
     `EvidenceBundle.Validate` (anchor is an env rendered change of the bundle and correlates to the item,
     scope rule is one of the two, unique ids, evidence), `Items()/Item()/InScope()`; `ValidateAgainst`
     uses them (an in-scope environment item needs a finding). Schema: `scopeRule` pattern `^scope:` +
     `ChangeItem` `allOf` conditionals for both rules; `environmentItems` property.
3. **Shared path convention** — `render.RenderedPath` (member selectors for args / RBAC) used by the bundle
   collector (release scope) and renderdiff (environment scope); renderdiff's local `changePath` removed.
   Release-scope ids of arg/RBAC changes therefore change (they gain the selector) — no golden pins them.
4. **`render.DetectTargets` fix** — a repo-local values file an Argo CD / Flux target layers is no longer a
   target of its own (note: "rendered as a values layer of …"); renderdiff's filter and note rewriting removed.
   **Golden changes: none** — every existing test/golden passed unchanged (no fixture has a values file both
   layered by an Argo/Flux target and discovered on its own); new test `TestDetectTargetsLayeredValuesFile`.
   The only regenerated file is `schemas/evidence-bundle.schema.json`: (a) `ChangeItem.scopeRule` gains
   `"pattern": "^scope:"`; (b) `ChangeItem` gains the two `allOf` if/then rules above; (c)
   `EvidenceBundle.environmentItems` (array of `ChangeItem`) is added. The other schemas are byte-identical.

## Render coverage on the groundtruth environments (8 cases, inputs + env/ only)

Real pipeline (bundle collector + snapshot builder → renderdiff → assembler; every bundle `Validate`s,
every indexed change resolves to the artifact bytes, two runs identical), helm/kubectl, kube-version 1.31,
offline from the warm `.ri` cache. Reproduce: `RI_RENDERDIFF_STATE=<.ri> go test
./internal/investigate/renderdiff -run Groundtruth -v` (`RI_RENDERDIFF_DUMP=1` lists every change's rule).

| case | targets | ok | partial | unavail. | n/a | non-ok reasons | env rendered changes | explained upstream | structural item | self-artifact item | unexplained item | env items | uncorrelated |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| cert-manager-1.17-1.18--payments-eu | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 49 | 34 | 2 | 5 | 8 | 2 | **0** |
| cert-manager-1.17-1.18--payments-us | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 49 | 34 | 2 | 5 | 8 | 2 | **0** |
| ingress-nginx-1.11-1.12--shop-de | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 7 | 7 | 0 | 0 | 0 | 0 | **0** |
| ingress-nginx-1.11-1.12--shop-fr | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 7 | 7 | 0 | 0 | 0 | 0 | **0** |
| ingress-nginx-1.11-1.12--shop-nl | 4 | 1 | 0 | 0 | 3 | version-independent×3 | 11 | 7 | 0 | 0 | 4 | 4 | **0** |
| ingress-nginx-1.11-1.12--shop-pl | 3 | 0 | 1 | 0 | 2 | values-incomplete×1, version-independent×2 | 0 | — | — | — | — | 0 | 0 |
| kube-prometheus-stack-89-90--obs-north | 2 | 1 | 0 | 0 | 1 | version-independent×1 | 5 | 0 | 0 | 0 | 5 | 0 | **0** |
| kube-prometheus-stack-89-90--obs-south | 2 | 1 | 0 | 0 | 1 | version-independent×1 | 5 | 0 | 0 | 0 | 5 | 0 | **0** |

(Columns "structural/self-artifact/unexplained item" count environment changes attributed to an item of
that rule, at release scope via `release-anchor` or at environment scope; "env items" are the
environment-scope ones.) Previously uncorrelated: cert-manager 13, shop-nl 4, kps 5 → now **0** everywhere.

- cert-manager: 5 self-artifact (controller / webhook / cainjector / startupapicheck images and the
  `--acme-http01-solver-image` arg); 2 structural (old `*-controller-challenges` ClusterRole/Binding
  removed); 8 unexplained in scope (dns01/http01 ClusterRole/Binding added ×4, webhook probe ports ×2, CRD
  `spec.names.shortNames` ×2). The 2 CRD `shortNames` changes are **environment items**: the chart-defaults
  render has CRDs disabled (`crds.enabled=false` by default), so they have no release counterpart.
- shop-nl: 4 environment-only removals (ConfigMap `allow-snippet-annotations`, `modules` init container,
  volume, mount) — enabled by this environment's values, absent from the chart-defaults render → 4 in-scope
  environment items. Exactly the environment-specific signal the amendment wants investigated.
- kps: 5 unexplained (ServiceMonitor endpoints ×3, operator `--secret-field-selector`, new
  `prometheus-token` Secret) — matched to release-scope `scope:rendered-unexplained` items.
- Honest gaps unchanged: shop-pl `partial` (value files in another repository → no changes indexed).

## Decisions (with why)

- **Stored renders are the normalized objects** (not helm's raw stream): exactly what the diff compared, so
  every change path resolves in the bytes, and redaction (`<sensitive sha256:…>`) is the render package's
  own — no secret value is ever stored. `Redacted: true` when a marker is present.
- **Member selectors** in `RenderedChange.Path` for container args (`…args[=--flag]`) and RBAC
  (`rules[=<group>/<resource>:<verb>]`): `render.Diff` emits several changes on one path there, and the
  contract id is (scope, target, object, path, class) — without a selector ids collide. Residual collisions
  get `[#n]` in diff order. `Object` understands both selectors.
- **Chart coverage at pair level** = every top-level values key the customer sets (layers + `--set`,
  `global` excepted) is defined by the from or to chart's defaults or names a declared dependency;
  otherwise `partial`. Top level because nested free-form maps (`podLabels`, `extraArgs`) are legitimately
  absent from defaults.
- **Consumer Kustomize overlays** (app-of-apps apps that consume the product's APIs) are
  `not-applicable` with reason `version-independent:` — their render cannot change with the product
  version; their `from` render is kept (useful to the agent/relations). Not a gap for R2.1. An overlay
  running the product's images unpinned is `unavailable` (a real gap).
- **Layered values files**: a repo-discovered values file an Argo/Flux/Helmfile target layers is not a
  target of its own — now in `render.DetectTargets` itself (follow-up 1); rendering it alone indexed
  changes of a configuration nobody deploys (shop-de had 7 vs 8 spurious changes).
- **Self-artifact bump is strict** (same repository, exact edge tags): a customer mirror becomes
  `scope:rendered-unexplained` (investigated) rather than silently routine — fail closed.
- Incomplete pairs still store their renders and, when both sides rendered, the diff (`indexed: false`,
  no change items) for inspection; nothing is indexed.
- `Correlation` = the distinct rules joined with `,` (a change can be linked by several).
- Evidence `RetrievedAt` zero and provenance timestamps stripped: artifact digests and bundle ids are
  reproducible (determinism tested, incl. different checkout paths).

## Files touched outside ownership (additive)

- `internal/render/chartcoverage.go` — exported `(*Engine).ChartDefaults` (wrapper of `chartDefaults`).
- `internal/render/rendered.go` (new: `RenderedPath`, `SelfArtifactBump` + tests), `internal/render/source.go`
  (`DetectTargets` layered-values fix + test), `docs/RENDER.md` (values-files row) — commander-requested.
- `internal/domain/investigation.go`, `internal/domain/schemagen/meta.go`, `schemas/evidence-bundle.schema.json`,
  new `internal/domain/investigation_scope_test.go` — the amendment's contract (CONTRACT-CHANGE markers).
- `internal/investigate/relations/exposure.go` — one line: exposure roots over `b.Items()` (record +
  environment items).
- `internal/investigate/api.go` — the two contract changes below.
- `docs/INVESTIGATE.md` — "Render diffs" section only.

## Contract changes

Follow-up 1 (all `// CONTRACT-CHANGE(renderdiff)`, additive):
- `domain.ScopeRenderedStructural/ScopeRenderedUnexplained/ScopeRoutineSelfArtifact` constants and the
  rendered-scope item shape rule (record and environment items; schema conditionals).
- `EvidenceBundle.EnvironmentItems []ChangeItem` + `Items()/Item()/InScope()`; `Validate` /
  `ValidateAgainst` use them. (`BundleID` unchanged: environment items are a deterministic function of the
  renders, whose key already includes every item id a rendered change carries.)
- `investigate.RenderResult.Items []domain.ChangeItem` (renderer → assembler).
- `ValidateAgainst` auto-approval: a `not-applicable` environment pair (version-independent target) no
  longer blocks; at least one `ok` pair is required. Before, every case with a consumer overlay could never
  be approval-eligible. **Commander: please confirm.**

Wave 1:

- `// CONTRACT-CHANGE(renderdiff)` `RenderResult.Notes []string` — target-detection notes (skipped files,
  renders disabled, no deployment found) so "no pairs" is never silent.
- `// CONTRACT-CHANGE(renderdiff)` `DiffQuery.After string` — continuation cursor for bounded pages
  (`renderdiff.DiffPage{Changes, Total, Next}`; lane agent can surface `Next` to the model).

## Open questions / cross-lane notes (commander)

0. Items 1–3 below were resolved by follow-up 1 (amendment + shared helpers + DetectTargets fix).
1. ~~**Product-image version bumps have no change item**~~ (bundle creates computed image items only for
   added/removed/moved images), so every upgrade leaves ~6 uncorrelated env changes for cert-manager and
   can never be approval-eligible. Proposal for lane bundle: anchor an item (family `rendered`/`image`,
   likely `scope:not-breaking`) on every release-scope rendered change no change explains (today only
   structural ones), or an item per updated artifact that the `artifact` rule can attach to. renderdiff
   picks either up unchanged (`release-anchor` / computed-image attribution).
2. **Path convention between scopes**: bundle's release-scope `RenderedChange.Path` is the raw render path
   (duplicates merged into one record); renderdiff uses member selectors. `release-anchor` matching
   tolerates both, but a common convention (a shared helper in `internal/render`) would be cleaner.
3. Suggested fix in `internal/render.DetectTargets` (not done; outside ownership): skip repo-local values
   files referenced by an Argo CD/Flux target (as it already does for Helmfile) — renderdiff filters them.
4. Bundle's open question ("drop correlation to routine changes at env scope?") — yes, dropped (no item).

## Tests

- `go build ./... && go vet ./... && go test ./...` green (38 packages ok, 0 fail) after merging p4-env-impact
  @ adf4fe08 (follow-up 1).
- `RI_STATE=<.ri> go test ./internal/investigate/bundle -run Groundtruth` ok (bundle's own groundtruth bundles).
- `RI_RENDERDIFF_STATE=/Users/tommydavison/repos/Release-Intelligence-/.ri go test ./internal/investigate/renderdiff -run Groundtruth -v`
  — 8/8 pass (bundle validates, every change resolves, two runs identical).
- Cleanup: no kind clusters or servers; `bin/` removed; scratch only in the session scratchpad.
