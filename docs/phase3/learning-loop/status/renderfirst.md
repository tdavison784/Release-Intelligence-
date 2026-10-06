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
  run and per verification level in `cmd/ri/eval.go`.

## In progress (PO-7a steps 2–3, values family)

Generalising the PO-3 counterfactual to values keys the customer SETS whose key
the target removes (`values:removed` / `values:section-removed` with a values
match): the render decides between rejection (schema/chart-authored `fail` naming
the key ⇒ `impact:render-target-rejects` ACTION), attributable rendered changes
(⇒ today's `impact:values-removed` ACTION, now with rendered-change matches on
chain 2), and nothing attributable (⇒ `impact:values-set-no-effect` not-affected).
Without `--render`, or when the render cannot decide, the verdict is exactly
today's (byte-identical). See the handoff log for state.

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
entries and the eval panel (no producers, no tests, no status file). Plan for this
window: (a) this status file, (b) verify + commit the inherited WIP, (c) implement
the producers for the values-removed-with-match case (`internal/impact/setvalues.go`
+ `internal/render/setvalues.go`, wired through api/build/app), (d) render-side and
join-side tests. Progress is recorded below as each lands.

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
