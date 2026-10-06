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

## In progress (PO-7a steps 2–3)

The values family is done end-to-end (see Done). The same render-first pattern
is next for the remaining render-expressible families (see Next).

## Next (not started)

- Steps 2–3 for the remaining render-expressible families: image/tag changes,
  removed/renamed flags & env vars, knowledge-fact subject+change inputs.
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

`go build ./... && go vet ./... && go test ./...` green at `d2e6ed6c`.

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
