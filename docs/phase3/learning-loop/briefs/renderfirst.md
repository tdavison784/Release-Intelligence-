# Lane `renderfirst` — render diff as the primary exposure decision (PO-7a)

Product owner decision (2026-10-05, PO-7): mirror how engineers actually decide an upgrade — changelog →
inferred change → apply it to OUR config in memory → render before/after (helm template / kustomize build
with the customer's values) → the rendered diff IS the exposure evidence. The semantic/knowledge layer becomes
the FALLBACK for changes rendering cannot see (runtime behaviour, API-server/binary defaults,
cross-product, procedures).

Read: FLEET.md, DESIGN.md (§1.3, §4), DECISIONS.md (PO-1…PO-6), RENDER-MISSION.md, docs/RENDER.md,
LOOP-DIAGNOSIS-2.md, internal/render (counterfactual variants, EvaluateRenderedChange, R12 renderability),
internal/impact (knowledge composition). Branch `p3ll/renderfirst`.

1. **Measure first** (before changing behaviour): classify every affected / not-affected link of the
   28-entry dataset as render-decidable / partially / not, from the item's subject (generic rule: R12
   renderability of the change's family × change type) — report counts; and the applicability that
   render-alone decisions reach today with `-render`.
2. **Apply the change in memory**: generalise the PO-3 counterfactual from changed defaults to every
   render-expressible inferred change: removed / renamed values keys (render with the key as the customer set it
   on the new chart → chart rejects/ignores it?), removed/renamed flags & env vars, default changes, image/tag
   changes, values-schema tightening (new chart's values.schema.json rejecting the customer's values is a
   decisive ACTION-grade signal: "target chart rejects this configuration"). Inputs come from the edge's
   computed changes and from knowledge facts' subject+change (never their conditions).
3. **Decision order in internal/impact**: for render-verifiable changes with a successful customer render, the
   rendered delta decides exposure (true ⇒ exposed with render evidence on chain 2; both renders OK and nothing
   attributable ⇒ not-affected with a render check); knowledge conditions only decide what rendering can't.
   Trust ladder unchanged: a render delta alone never yields ACTION (consequence still needs verification —
   trusted fact, PO-2/5/6 consensus, or a policy tier from the `policy` lane). Byte-identical without `--render`.
4. Adversarial tests (render shows a change that doesn't matter; render unavailable; customer values rejected
   by the new schema; change visible only at runtime ⇒ falls back).
5. Full eval: report per level, with and without `-render`, before/after; no new false ACTION. Doc
   `docs/RENDER-FIRST.md`. Reply `LANE DONE: renderfirst`.

Model: Opus.

## Addendum (product owner, 2026-10-05): render CRDs locally as an environment input

Helm renders already use `--include-crds` and kustomize builds include CRD bases, but the environment model's
installed-CRDs dimension (`env.DimCRDs`) is filled only from a user-supplied `--crds` file — rendered CRDs are
never fed in, so CRD-dependent questions go UNKNOWN or fall back to API-group evidence when `--crds` is absent.
6. **Derive installed CRDs from the render of the customer's install at the FROM version** (helm template with
   their values — honouring CRD gates such as `crds.enabled` / `installCRDs` exactly as they set them — and
   `kustomize build` of their overlay): populate `DimCRDs` from rendered `CustomResourceDefinition` objects
   with provenance `rendered (chart X@from, values digest …)`, distinct from observed `--crds`. Precedence: an
   observed `--crds` input wins; a declared/detected separate CRD install path (CRDs gated off in the chart)
   means the render says nothing about them ⇒ dimension stays absent/partial (absence is not knowledge), never
   "no CRDs installed". Diff rendered CRDs FROM→TO (schema fields, versions, served/storage, defaults/enums)
   with the customer's gates so CRD changes are evaluated against what they actually install.
7. Tests (gated chart with CRDs on/off; kustomize base with CRDs; observed `--crds` overriding rendered;
   separately-installed CRDs ⇒ partial) and report how many CRD-dependent UNKNOWNs this resolves on the eval
   environments, with no new false ACTION.
