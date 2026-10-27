# Lane `groundtruth6` status (sealed: no per-case labels in this file)

Brief: `briefs/groundtruth6.md` (D-0032). Branch `p6di/groundtruth6`, never pushed or merged by the lane.
Output: `eval/investigations-blind6/`.

## Pairs (products and versions only; labels live in the cases' expected.yaml / NOTES.md)

| Pair | Product | From → to | Coverage items |
|---|---|---|---|
| traefik-3.6-3.6--shared-middlewares | traefik | v3.6.10 → v3.6.15 (chart 39.0.5 → 39.0.9) | 1, 4, 5, 6 |
| cert-manager-1.20-1.21--vault-pki | cert-manager | v1.20.x → v1.21.x | 2 |
| external-secrets-0.18-0.19--crd-delivery | external-secrets | v0.18.2 → v0.19.x | 1, 3 |
| argo-cd-2.14-3.0--developer-access | argo-cd | v2.14.x → v3.0.x | 1 |

## Done

- Read FLEET.md (phase 6 + phase 5), `_common.md`, the brief, PLAN-M6, `eval/investigations/FORMAT.md` and README,
  existing inputs for format, the held-out helper scripts. Not read (brief): MISSION.md, PRODUCT-DECISIONS.md, any
  label file, other lanes' status.
- Upstream research for all four pairs (release notes, upgrade guides, advisories).

## In progress / next step

- Traefik pair: write envs, observe in kind.

## Decisions

- One pair carries the incomplete-input environment, so the decisive classes are not 4 + 4.
- The version/advisory decision (coverage 6) is recorded in NOTES.md and as an additive `versionDecision` block in
  expected.yaml.

## Kind time

(running total, filled as runs complete)
