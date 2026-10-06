# Render-first exposure (`internal/impact`, PO-7a)

How the join decides whether an upgrade change reaches THIS customer: the rendered diff
of their own configuration is the primary evidence; knowledge conditions only decide
what rendering cannot see. Implementation notes for the renderfirst lane
(brief: `docs/phase3/learning-loop/briefs/renderfirst.md`; rendering itself:
`docs/RENDER.md`; the trust ladder: `docs/phase3/learning-loop/DESIGN.md` §4).

The principle mirrors how an engineer decides an upgrade: changelog → inferred change →
apply it to OUR config in memory → render before/after with `helm template` /
`kustomize build` → **the rendered diff IS the exposure evidence**. Runtime behaviour,
API-server/binary defaults, cross-product effects and procedures stay with the
knowledge layer.

```go
rep, err := app.Impact(ctx, "cert-manager", "v1.17.0", "v1.18.0", app.ImpactOptions{
    Environment: env.Inputs{ValuesFiles: []string{"my-values.yaml"}, Repo: "."},
    Render:      &app.RenderOptions{KubeVersion: "1.31"}, // the switch
})
```

Without `Render`, or whenever no render decides, every verdict is byte-identical to the
render-free join — the new rules fire only from a decisive customer render.

## What renders decide (and how)

`ri impact --render` renders the From and To releases with the customer's detected
configuration (environment pairs only, never the chart-default pair; see RENDER.md §R7).
Three evaluators turn those renders into exposure decisions, dispatched per change
family exactly as the deterministic join itself dispatches (`internal/impact/renderfirst.go`):

| Change family | Customer state | Render question | Decides |
|---|---|---|---|
| values key removed/renamed | they SET the key | does the target chart reject their values (schema), or does anything in their rendered delta trace to the key? | `impact:render-target-rejects` (ACTION, Helm's own refusal) · attributable → today's ACTION with rendered evidence · nothing attributable → `impact:values-set-no-effect` (not-affected) |
| values default changed / key added | they leave it UNSET | counterfactual render of the source with the default unset (PO-3) | exposed with rendered evidence · nothing attributable → not-affected with a render check |
| image removed/moved/tag changed | they reference the repository | does their own From→To rendered delta change that image? (no counterfactual needed — the pair delta IS their upgrade) | attributable → today's verdict with rendered evidence · a pin keeps the change out → `impact:image-render-unchanged` (not-affected) |
| everything else (CRD semantics, runtime behaviour, procedures, …) | — | not render-verifiable | falls back to the deterministic join / knowledge |

Counterfactuals apply the change in memory as a null values layer over the customer's
values (Helm coalescing deletes the key), diffed in the pair-delta orientation so the
effect intersects their upgrade's own delta by `changeKey`. A `--set` override at or
under the key makes the evaluation undecided — a values layer cannot delete an override.
Failed renders, incomplete values (`valuesFrom` outside the repo) and release-scope
renders never decide anything (an evidence gap is UNKNOWN, never "no change").

**Chart coverage**: a no-effect/clear verdict additionally requires the chart being
rendered to *define* the key — a default at the exact path or under it
(`internal/render/chartcoverage.go`; istio's single top-level `defaults:` wrapping is
lifted exactly as the values snapshots lift it). Products ship several charts (istio
renders istiod while `cni.*` belongs to the istio-cni chart), and a key the rendered
chart never defined cannot have reached any rendered deployment: its counterfactual
renders identically, and that vacuous "nothing attributable" would read as no-effect
while the deployment consuming the key was never rendered. For set keys the guard reads
the *target* chart, so a key the upgrade removed — by definition absent from the target
— also returns to the deterministic verdict instead of a vacuous clear. Uncovered keys
make the conclusion undecided/unknown, naming the gap (an attributable match or a
target-render refusal stands regardless).

## The decision order in the join

For a render-verifiable change with a decisive customer render, **the rendered delta
decides exposure; the fact's applicability condition only decides what rendering can't**
(`builder.renderExposure`, called from the knowledge pass):

- a decisive **no-effect render stops a trusted fact's exposure claim** — the fact
  classifies not-affected and the composition guard skips it; the render's
  not-affected verdict stands;
- an **attributable render plus a trusted action-grade consequence raises** the
  deterministic review to a knowledge ACTION citing the rendered change on chain 2 —
  the only path where render and knowledge combine to ACTION.

The trust ladder is unchanged: **a render delta alone never yields ACTION**. The one
render-born ACTION — `impact:render-target-rejects` — is Helm's own deterministic refusal
of the customer's values ("values don't meet the specifications of the schema"), not a
model output and not a delta. Consequence still needs verification (a trusted fact,
PO-2/5/6 consensus, or a policy tier).

## Rendered CRDs as an environment input

Helm renders include `--include-crds` and kustomize builds include CRD bases, but the
installed-CRDs dimension used to see only a user-supplied `--crds` file. Now the
CustomResourceDefinition documents of the **FROM render of the customer's install** —
their CRD gates (`crds.enabled` / `installCRDs`) exactly as they set them — populate the
dimension (`render.RenderedCRDsOf` → `env.Inputs.RenderedCRDs`):

- provenance is a new evidence kind, `rendered` — "rendered (chart X@from, values
  digest …)" — distinct from every observed kind; `InstalledCRD.Rendered` marks it;
- **an observed `--crds` input wins** (rendered CRDs load only without one; a CRD name
  an observed file or manifest already states keeps its observed provenance);
- the dimension **never becomes "supplied"** and stays **partial**: render output is
  what a fresh install with their gates would create, not observed cluster state, so
  absence conclusions stay off — a gated-off or separately installed CRD path means the
  render says nothing, never "no CRDs installed";
- in the join, rendered CRDs **resolve CRD-change unknowns upward only**
  (`renderedCRDResolution`): `crd:removed` / `crd:version-removed|unserved|deprecated`
  with no `--crds` input move from a visibility-gap UNKNOWN to review-required at medium
  confidence on the rendered evidence — never action, never not-affected;
- the FROM→TO rendered CRD delta rides the ordinary pair diff (CRDs are rendered objects
  like any other): the customer's gates decide whether a CRD change reaches their delta
  at all.

## Adversarial guarantees

Each is pinned by a test:

- **the render shows a change that doesn't matter** — the reference exists but the pin
  keeps the chart's change out of the customer's render → not-affected with a
  no-attributable-change render check (`internal/render/images_test.go`, pinned customer);
- **render unavailable** (binary missing, chart unfetchable, template error) → undecided,
  today's verdict stands, byte-identical report (`TestRenderImagesUndecided`,
  `TestSetValuesUndecided`);
- **the customer's values are rejected by the new schema** → deterministic ACTION naming
  the refused key (`rej` chart family, `values.schema.json` with
  `additionalProperties: false`);
- **the change is visible only at runtime** → not a render-verifiable family, the render
  abstains and the knowledge/deterministic fallback decides (`renderExposure` dispatch:
  unknown families return not-decided; CRD identity resolution never clears).

## Measurements

Eval dataset (28 entries), run with the warm shared state cache, gates as in
`eval/gates.yaml` (never edited):

| run | applicability accuracy | false-action rate | notes |
|---|---|---|---|
| before (no `--render`) | 0.525 | 0.059 (1/17, the known kyverno E9) | no regressions vs stored results |
| with `--render` | 0.564 | 0.062 (1/16 — still only kyverno E9) | impact links 28/71 (before 23/71); see below |

Both runs fail the applicabilityAccuracy and falseActionRate gates as before (known
FAILs, unchanged in kind); criticalRecall 1.000 and importantRecall 0.970 pass in both.
No new false ACTION: the one false action is the same kyverno E9 finding, over a larger
(true-positive) ACTION set.

Diffs vs the stored render-free results, all accounted for:

- cilium-1.15-1.17 findingsFound 5→4 — designed: the values-removed finding becomes
  `impact:render-target-rejects` (Helm's own refusal of the customer's values), which the
  case's expected-rule matcher does not name;
- cilium-1.16-1.17--plant-edge impactLinksHit 2→4 (better) and notAffectedViolations
  0→1 — the violation is a case-matcher artifact: a correct render-backed
  `values-default-applies` review on `bgpControlPlane.statusReport.enabled` matches
  E3's `(?i)bgpControlPlane` text under a not-affected expectation;
- kyverno-1.12-1.13 impactLinksHit 3→6 (better).

The `--render` run's render-first panel (per renderability bucket: links, affected hit
by render, not-affected cleaned by render, accuracy) is printed by `ri eval -render
[-render-json stats.json]`; the stored `eval/results` stay the render-free baseline.
Totals this dataset: 11/12 renders ok · 2022 rendered changes (1329 undocumented) ·
106 unknowns restated by a complete customer render · 6 rendered-change decisions
(5 ACTION with render evidence) · **rendered CRDs resolve 21 CRD-change unknowns**
(kyverno 15, karpenter 3+3 across its two cases; cert-manager's and cilium's CRD gates
are off in the fixture values, istio's base render ships no CRD documents, and
external-secrets has an observed `--crds` input that wins) · 4/16 ACTION corroborated ·
1 target-rejects · unset defaults: 58 exposed (18 image-only), 60 cleared,
143 render-unavailable (the chart-coverage guard returns vacuous clears to the
deterministic baseline rather than deciding them).
