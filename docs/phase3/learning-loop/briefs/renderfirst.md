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
