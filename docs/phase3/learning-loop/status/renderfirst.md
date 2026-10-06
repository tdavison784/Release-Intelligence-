# Lane `renderfirst` — render diff as the primary exposure decision (PO-7a)

Branch `p3ll/renderfirst`, brief `briefs/renderfirst.md` (PO-7 + the rendered-CRDs
addendum, items 6–7). This file was missing until the first GLM handoff window (the
Claude lead had not created it); it now carries the lane's running state.

## Done

- **Step 1 (measure first), commit `c24733cf`**: `internal/eval/renderfirst.go` —
  per expectedImpact link, the R12 renderability bucket of the item's labelled
  subjects (`LinkRenderability`) and whether a render-backed finding decided it
  (`RenderBacked`: a rendered-change match or a render-dimension check). Panel
  `WriteRenderFirst` prints links / affected hit (by render) / not-affected clean
  (by render) / accuracy / render-alone, per bucket and total. Reported, never
  gated; it never feeds the pipeline.
- **Uncommitted at handoff (now finished, see handoff log)**: the two PO-7a join
  rules in `internal/domain` — `impact:render-target-rejects` (action-required:
  the target chart refuses the customer's values at render time, naming a removed
  key they set) and `impact:values-set-no-effect` (not-affected: the customer sets
  a key the target removes and rendering attributes nothing of their upgrade to
  it) — with their `Validate` shapes (values-key + environment-render matches; no-
  attributable-change render check), schemagen meta rules, regenerated
  `schemas/impact-report.schema.json`, and the eval render-first panel wired per
  run and per verification level in `cmd/ri/eval.go` (commit `3af064cc`).
- **Values family end-to-end, commits `f1463a4c`, `c5619ef1`, `d2e6ed6c`**:
  `internal/impact/setvalues.go` (SetValuesEvaluator contract + join emission:
  rejected → `impact:render-target-rejects` ACTION citing the refusal, no-effect →
  `impact:values-set-no-effect`, attributable/undecided/no evaluator → today's
  `impact:values-removed` wording, render matches appended on chain 2) and
  `internal/render/setvalues.go` (the evaluator: rejection pass — `TargetRejects`
  plus a refusal that names the key or an ancestor; From-minus-key
  counterfactual — a null values layer deletes the key, the diff in pair
  orientation `FromResult → cf` intersects the pair delta by `changeKey`;
  `--set` paths at/under the key make it undecided), wired as `in.Set` beside
  `in.Unset` in `internal/app/impact.go`. Tests: live-helm `rej` chart family
  (1.0.0 reads `legacy.feature`, 1.1.0 drops the section, 2.0.0 refuses unknown
  keys via `values.schema.json`), join test incl. byte identity of the report
  without a decisive render, and pure tests of the refusal matcher / null layer /
  --set guard. `go build ./... && go vet ./... && go test ./...` green.
- **Image family end-to-end, commits `441ddb86`, `cab2f988`**: the customer's
  own From→To render decides a referenced image repository (no counterfactual
  needed — the pair delta IS their upgrade). New not-affected rule
  `impact:image-render-unchanged` (domain const + validate shape + schemagen +
  regenerated schema); `impact.ImageRenderEvaluator` (`internal/impact/images.go`)
  with `imageFamily` dispatch (attributable → today's review-required
  `impact:image-changed` with rendered matches on chain 2; no-effect — a pinned
  reference keeps the chart's image change out of the customer's render →
  not-affected with a no-attributable-change render check; undecided/nil →
  today's verdict); `render.RenderImages` over the environment pairs (Helm and
  Kustomize), wired as `in.Image`. Tests: perm customers pinning repo-only
  (attributable) vs repo+tag (no-effect), join byte identity, failed/incomplete
  renders never clear. Full suite green.
- **Decision order in the join, commit `1b7d576d`** (brief item 3):
  `builder.renderExposure` (`internal/impact/renderfirst.go`) — when the
  customer's render decisively decides a change's exposure, it wins over the
  fact's applicability condition (knowledge conditions only decide what
  rendering can't). Dispatch mirrors the join's own families: values:removed
  with the key set → SetValues (rejection/attributable = True, no-effect =
  False); values:default-changed/added with the key unset → the PO-3
  counterfactual; image rules with the repo referenced → per-repo aggregation
  (any attributable wins, one undecided abstains). Both consequences tested in
  `internal/impact/renderfirst_test.go`: a decisive no-effect stops a trusted
  fact's exposure claim (render's not-affected stands); an attributable render
  + trusted action-grade consequence raises review to a knowledge ACTION
  citing the rendered change on chain 2 — the only render+knowledge→ACTION
  path, so the trust ladder holds. Without evaluators the composition is
  byte-identical (full suite unchanged).
- **Rendered CRDs as an environment input (addendum items 6–7), commits
  `2603e180`, `fd3767fc`, `52ab9dfc`, `40b16308`, `f042c869`**: new evidence
  kind `rendered` (a document a render produced, distinct from every observed
  kind); `env.Inputs.RenderedCRDs` loads the CustomResourceDefinition
  documents of the FROM render of the customer's install (their CRD gates
  exactly as set) through the same CRD machinery — marked
  `InstalledCRD.Rendered`, cited by an `EvidenceRendered` record naming the
  render and its chart/values digests. Observed `--crds` wins (also
  per-CRD against manifest-observed ones); the dimension never becomes
  "supplied" (absence conclusions stay off) and stays PARTIAL; a render
  without CRDs loads nothing (gate or separate install path — absence is not
  knowledge). `render.RenderedCRDsOf` extracts from the environment pairs'
  FROM renders (dedup by name, never the chart-default pair); `ImpactRun`
  reloads the environment with them before the join. In the join,
  `renderedCRDResolution` resolves the identity question of
  crd:removed / crd:version-removed / -unserved / -deprecated that a missing
  `--crds` leaves unknown — raise-only: review-required at medium confidence
  on the rendered evidence, never action, never not-affected, all-or-nothing
  per change. The FROM→TO rendered CRD delta rides the existing pair diff
  (CRDs are ordinary rendered objects); tested that the target's dropped
  version appears in the customer's delta only when their gate is open.
  Fixtures: `crdg` chart with `crds.enabled`-gated CRDs (customer values
  on/off; 1.1.0 drops v1alpha2).

## In progress (PO-7a steps 2–3)

The values family, the image family and the decision order are done (see
Done). Remaining render-expressible inputs ride along with the knowledge-inputs
work (see Next).

## Next (not started)

- Steps 2–3 for the remaining render-expressible inputs: knowledge-fact
  subject+change inputs (render decides what it can see; knowledge conditions
  only what rendering can't) and removed/renamed flags & env vars, which exist
  only as render diff classes and note-derived changes (no computed upgrade
  rule) — they ride along with the knowledge-inputs work.
- Adversarial tests at system level (brief item 4) and the full eval with/without
  `-render` (item 5) — numbers must be re-run before quoting; the stored
  `eval/results/*` are untouched.
- `docs/RENDER-FIRST.md` (item 5).
- Addendum items 6–7: installed-CRDs environment dimension derived from the FROM
  render of the customer's install, precedence under `--crds`, rendered CRD
  FROM→TO diff.

## Decisions taken (local defaults)

- Byte-identical without `--render` is preserved per finding family: the new rules
  fire only from a decisive customer render.
- The render-rejection ACTION is deterministic (Helm's own refusal), not a model
  output and not a render *delta* — the trust ladder's "a render delta alone never
  yields ACTION" is untouched (exposure-by-delta stays at `values-removed`'s
  existing deterministic class).
- When the render is unavailable or cannot decide, today's verdicts stand (the
  render overrides only with a decisive result) — mirrors PO-3's aggregation.
- The set-key counterfactual diff runs in the pair-delta orientation
  (`FromResult → cf`), mirroring unset.go's old→new orientation, so a key
  removal intersects the From→To delta by `changeKey` (class and target value
  must match).
- A `--set` override at or under the key (Argo/Helmfile parameters) makes the
  set-key evaluation undecided: a values layer cannot delete an override, so the
  counterfactual could not honestly unset the key.

## Files touched outside ownership (all additive)

- `internal/domain/impact.go`, `internal/domain/schemagen/meta.go`,
  `schemas/impact-report.schema.json` (the two rule constants, validation shapes,
  schema conditionals) — domain is contract-lane owned; noted here per FLEET.md.
- `cmd/ri/eval.go` (render-first panel after the render section).

## Tests

`go build ./... && go vet ./... && go test ./...` — see the handoff log for the
last verified run.

## GLM handoff log

### Window 1 (2026-10-05, glm-renderfirst)

Inherited: step 1 committed; WIP on the tree adding the two domain rules, schema
entries and the eval panel (no producers, no tests, no status file).

Done this window (all four planned items):

- (a) `ae2f107d` this status file.
- (b) `3af064cc` banked the inherited WIP after verifying it (schemagen
  regeneration produced no diff; domain/schemagen/eval tests green).
- (c) `f1463a4c` the join emission (`internal/impact/setvalues.go`, `Input.Set`,
  valuesFamily dispatch) and `c5619ef1` the render evaluator
  (`internal/render/setvalues.go`) wired as `in.Set` in `internal/app/impact.go`.
- (d) `d2e6ed6c` tests + the live-helm `rej` fixtures, including the fix for a
  real bug the live test caught: the counterfactual diff was computed
  cf→FromResult, reversing every change so the `changeKey` intersection was
  always empty; it is now FromResult→cf (pair orientation).

Beyond the plan, same window: the image family end-to-end (`441ddb86` evaluator
+ rules + wiring, `cab2f988` tests), the decision order (`1b7d576d`:
`renderExposure` override in knowledge composition + tests, brief item 3), and
the rendered-CRDs addendum items 6–7 end-to-end (`2603e180` evidence kind,
`fd3767fc` env input, `52ab9dfc` extraction, `40b16308` join resolution +
wiring, `f042c869` the gate-honoring FROM→TO delta test) — see Done.

`go build ./... && go vet ./... && go test ./...` green at `f042c869`.

Uncertainties (for the returning lead / commander):

- The refusal-names-a-key matcher matches the subject's dotted path or any of its
  ancestor prefixes on word boundaries in helm's refusal text. Root-level
  `additionalProperties: false` schemas name the top-level property, so ancestor
  matching is needed; a refusal naming an unrelated kept key does not fire.
- Whether the attributable case should keep today's ACTION class or drop to
  review-required under a stricter reading of the trust ladder: I kept ACTION
  (same deterministic join as today, render only adds chain-2 evidence). The
  rejection ACTION rests on helm's deterministic refusal, which I read as
  satisfying "consequence established deterministically"; flag if you disagree.
- The live 28-entry eval with `-render` was NOT rerun in this window (needs the
  warm primary-checkout state and network); the eval panel and the new rules are
  covered by unit/live-tool tests only.
- The refusal matcher is case-sensitive (helm echoes property names verbatim);
  if a real refusal lower-cases paths it will fall to undecided, which is safe
  (today's verdict stands) but invisible — worth one live sample during the eval
  rerun.
- Decision order (item 3): the render+knowledge→ACTION path only fires when the
  fact's consequence kind is action-eligible (the fact contract pins
  exposedClass to the kind — `semantic.go` `Kind.ExposedClass()`); a
  behavior-change consequence caps at review, which the composition guard then
  skips next to the deterministic review finding. That is the contract working
  as designed, not a gap, but it means the render's practical raise is narrower
  than "any trusted fact".

### Window 2 (2026-10-05, glm-renderfirst)

Inherited: brief items 1–4 and addendum 6–7 committed; remaining: the full eval
±render (item 5/addendum 7 count), docs/RENDER-FIRST.md uncommitted, and an istio
E1 regression discovered in the first `-render` eval run (impactLinksHit 1→0).

Done this window:

- Root-caused the istio regression: the product's primary chart is **base**, so the
  rendered pair never consumes `pilot.*` (istiod chart) or `cni.*` (cni chart) — both
  true values-removed ACTIONs became vacuous `values-set-no-effect` because the
  counterfactual renders identically when no rendered deployment reads the key.
- `f9a17f25` the chart-coverage guard: a no-effect/clear verdict additionally requires
  the rendered charts to define the key (`internal/render/chartcoverage.go`; exact path
  or under it — a default above the key covers nothing). Uncovered keys → undecided,
  naming the gap; attributable matches and target refusals stay decisive.
- `ce9d09c6` two guard fixups found by the istio repro: coverage is the UNION of
  source and target chart defaults (the counterfactual renders the source — rej's
  `legacy.dead` must stay clearable), and istio's values wrapper (`defaults` ≤1.23,
  `_internal_defaults_do_not_set` 1.24+) is lifted exactly as the values snapshots
  lift it. Also caught and fixed a sync.Once re-entrancy deadlock in unset.go's
  sourceDefaults memoization before it ever ran.
- `a63455a2` `crdResolvedByRender` in the R17 panel — the addendum-7 measurement.
- `bf380d02` brief item 4 residual closed (runtime-visible fallback pinned in
  TestRenderExposureDispatch).
- Full eval ±render, warm cache (item 5): base applicability 0.525 / false-action
  0.059 (1/17) / links 23/71, no regressions. With `--render`: applicability 0.564 /
  false-action 0.062 (**still only the known kyverno E9 — no new false ACTION**) /
  links 28/71. Gates: criticalRecall 1.000 ✓, importantRecall 0.970 ✓, the two known
  FAILs unchanged in kind. Diffs vs stored, all accounted for (see RENDER-FIRST.md):
  cilium-1.15-1.17 findingsFound 5→4 (designed target-rejects replacement),
  plant-edge links 2→4 better + notAffectedViolations 0→1 (case-matcher artifact on a
  correct values-default-applies review), kyverno links 3→6 better. **istio regression
  resolved.**
- Addendum 7 count: **rendered CRDs resolve 21 CRD-change unknowns** — kyverno 15,
  karpenter 3+3 (two cases); cert-manager/cilium CRD gates off in fixture values,
  istio's base render ships no CRD documents, external-secrets' observed --crds wins.
- docs/RENDER-FIRST.md completed (measurements + chart-coverage section) and committed
  with the guard.

Lane brief items 1–5 and addendum 6–7 are complete. `go build ./... && go vet ./... &&
go test ./...` green.

Uncertainties (for the returning lead / commander):

- The guard's uncovered-key message says "another chart of X owns them" — for a key
  the rendered chart's own versions never defined that is right; for istio the
  primary artifact being the base chart (not istiod) is a catalog fact worth knowing
  when reading the message.
- `uncoveredKeysEither` treats a key covered when either chart defines it at-or-under.
  For section-level subjects (a 40-key `pilot.*` section change) this is all-or-
  nothing per change: one uncovered sibling makes the whole section change undecided.
  That is the conservative direction (baseline ACTION stands), and it is what
  restored istio E1, but a future split-subject refinement could clear the covered
  subset.
- plant-edge E3 `notAffectedViolations 0→1` is a case-matcher artifact (the case's
  `(?i)bgpControlPlane` text catches a correct `values-default-applies` review on
  `bgpControlPlane.statusReport.enabled` under a not-affected expectation). Not
  edited — eval expectations are read-only for this lane.
- cilium-1.15-1.17 findingsFound 5→4 is the designed SetValuesRejected replacement
  (the case's expected rule names values-removed; the new ACTION is helm's own
  refusal). If the case is ever updated to expect `render-target-rejects`, the diff
  disappears.
