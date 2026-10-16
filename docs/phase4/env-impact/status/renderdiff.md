# Status — lane `renderdiff` (Claude Opus 5.5, agent ri-renderdiff)

Branch `p4ei/renderdiff`, worktree `.claude/worktrees/p4ei-renderdiff`. Brief: `briefs/renderdiff.md`
(+ `_wave1-common.md`, DESIGN §2.4 / §7 "renderdiff"). Merged `p4-env-impact` @ 918e855a (bundle lane).
State: **DONE** (2026-10-16). Resume from: this file + brief + `git log`.

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

## Render coverage on the groundtruth environments (8 cases, inputs + env/ only)

Real helm/kubectl renders, kube-version 1.31. Measured with lane bundle's real collector / snapshot builder
/ assembler + this renderer (all 8 bundles assemble and `Validate`); kube-prometheus-stack charts were fetched
online once (warmed the shared `.ri` cache), everything else offline.

| case | targets | ok | partial | unavailable | not-applicable | non-ok reasons | env rendered changes | correlated | uncorrelated |
|---|---|---|---|---|---|---|---|---|---|
| cert-manager-1.17-1.18--payments-eu | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 49 | 36 | 13 |
| cert-manager-1.17-1.18--payments-us | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 49 | 36 | 13 |
| ingress-nginx-1.11-1.12--shop-de | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 7 | 7 | 0 |
| ingress-nginx-1.11-1.12--shop-fr | 3 | 1 | 0 | 0 | 2 | version-independent×2 | 7 | 7 | 0 |
| ingress-nginx-1.11-1.12--shop-nl | 4 | 1 | 0 | 0 | 3 | version-independent×3 | 11 | 7 | 4 |
| ingress-nginx-1.11-1.12--shop-pl | 3 | 0 | 1 | 0 | 2 | values-incomplete×1 (value files in another repo), version-independent×2 | 0 | 0 | 0 |
| kube-prometheus-stack-89-90--obs-north | 2 | 1 | 0 | 0 | 1 | version-independent×1 | 5 | 0 | 5 |
| kube-prometheus-stack-89-90--obs-south | 2 | 1 | 0 | 0 | 1 | version-independent×1 | 5 | 0 | 5 |

Every case with a complete repository has exactly one complete (`ok`) render of the product's Argo CD
Application (all value files layered); shop-pl is honestly `partial`. Offline without the kps chart in
cache, the kps pairs are `unavailable: chart-unavailable` (an honest gap).

What stays uncorrelated (with bundle's real items): cert-manager — 6 product-image version bumps
(controller/webhook/cainjector/startupapicheck images, `--acme-http01-solver-image` arg), ClusterRole/
Binding split into dns01/http01 (`resource-added` ×4), CRD `shortNames` ×2, webhook probe port ×2;
shop-nl — 4 environment-only removals (`allow-snippet-annotations` ConfigMap key, `modules` init
container/volume/mount) with no release-scope counterpart; kps — ServiceMonitor endpoints ×3, operator
`--secret-field-selector` arg, a new `prometheus-token` Secret. With the collector rule proposed below
(anchor every unexplained release-scope change), the simulated counts are cert-manager 2, kps 0, shop-nl 4
(environment-specific, as it should be).

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
- **Layered values files**: a repo-discovered `values:` target whose file is a layer of an Argo/Flux/
  Helmfile target is dropped (with a note): rendering it alone indexes changes of a configuration nobody
  deploys (shop-de had 7 vs 8 spurious changes). render.DetectTargets' "skipped" note for such a file is
  rewritten to "rendered as a values layer of …".
- Incomplete pairs still store their renders and, when both sides rendered, the diff (`indexed: false`,
  no change items) for inspection; nothing is indexed.
- `Correlation` = the distinct rules joined with `,` (a change can be linked by several).
- Evidence `RetrievedAt` zero and provenance timestamps stripped: artifact digests and bundle ids are
  reproducible (determinism tested, incl. different checkout paths).

## Files touched outside ownership (additive)

- `internal/render/chartcoverage.go` — exported `(*Engine).ChartDefaults` (wrapper of `chartDefaults`).
- `internal/investigate/api.go` — the two contract changes below.
- `docs/INVESTIGATE.md` — "Render diffs" section only.

## Contract changes

- `// CONTRACT-CHANGE(renderdiff)` `RenderResult.Notes []string` — target-detection notes (skipped files,
  renders disabled, no deployment found) so "no pairs" is never silent.
- `// CONTRACT-CHANGE(renderdiff)` `DiffQuery.After string` — continuation cursor for bounded pages
  (`renderdiff.DiffPage{Changes, Total, Next}`; lane agent can surface `Next` to the model).

## Open questions / cross-lane notes (commander)

1. **Product-image version bumps have no change item** (bundle creates computed image items only for
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

- `go build ./... && go vet ./... && go test ./...` green (38 packages ok, 0 fail) after merging p4-env-impact.
- `RI_RENDERDIFF_STATE=/Users/tommydavison/repos/Release-Intelligence-/.ri go test ./internal/investigate/renderdiff -run Groundtruth -v`
  — 8/8 pass (bundle validates, every change resolves, two runs identical).
- Cleanup: no kind clusters or servers; `bin/` removed; scratch only in the session scratchpad.
