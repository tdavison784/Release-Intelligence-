# Status — lane `groundtruth` (Phase 4 env-impact)

Agent: ri-groundtruth (Claude Opus 5.5). Branch `p4ei/groundtruth`. Brief: `briefs/groundtruth.md`.
Last updated: 2026-10-16 ~13:00 CDT.

## Draft case format (for the `contract` lane to converge on in `eval/investigations/FORMAT.md`)

Goal of the split: everything a system under test may read is in `input.yaml` + `env/`; everything it
is scored against is in `expected.yaml`, which pipeline/agent lanes must never read (FLEET.md).

```
eval/investigations/
  README.md                       # index of cases (pair, product, from→to, expected class, verification level)
  <case-id>/
    input.yaml                    # INPUTS ONLY: upgrade + environment + breaking items in scope
    env/                          # the GitOps repository fixture (a plain directory, treated as a repo root)
      ...                         #   Argo CD Application / Flux HelmRelease, values, kustomize base+overlays,
                                  #   consuming workloads
    expected.yaml                 # GROUND TRUTH (blind; only the groundtruth lane writes it)
    NOTES.md                      # upstream citations (URL + line ranges / quotes) + observed outcome
    observed/                     # raw outputs of the known-outcome run (kind / helm template), never edited
```

Case ids: `<product>-<fromMinor>-<toMinor>--<env-slug>`; the two halves of a pair share
`<product>-<from>-<to>--<topic>` as `pair` and differ in the env slug (e.g. `...--rotation-affected`,
`...--rotation-pinned`).

### input.yaml (readable by every lane)

```yaml
id: cert-manager-1.17-1.18--rotation-affected
pair: cert-manager-1.17-1.18--rotation        # optional; both halves of a pair share it
product: cert-manager                          # products/<id>.yaml definition id
chart: {repo: https://charts.jetstack.io, name: cert-manager}
from: {version: v1.17.2, chartVersion: v1.17.2}
to:   {version: v1.18.0, chartVersion: v1.18.0}
environment:
  id: payments-prod                            # environment identity inside the fixture repo
  root: env/                                   # repo root, relative to the case dir
  entrypoints:                                 # where a GitOps controller would start (Application/HelmRelease/kustomization)
    - clusters/prod/kustomization.yaml
  description: >-                              # what an operator would tell you; no hints about impact
    ...
scope:                                         # the upstream breaking-change items this case examines
  - id: B1
    title: Certificate.spec.privateKey.rotationPolicy default changed from Never to Always
    subject: spec.privateKey.rotationPolicy    # same notion of subject as eval/cases matchers
    source: <upstream URL>                     # identifies the change; NOT a statement about this env
withheld: []                                   # optional: inputs deliberately absent (for insufficient-evidence
                                               # cases), described as an operator would ("values file X lives
                                               # in another repo not provided") — no expected class here
```

### expected.yaml (ground truth — pipeline/agent lanes must not read)

```yaml
case: cert-manager-1.17-1.18--rotation-affected
authoredAt: "2026-10-16"
authoredBy: groundtruth lane (blind: upstream sources + observed outcome only)
items:
  - item: B1                                   # input.yaml scope id
    classification: affected                   # affected | not-affected | potentially-affected | insufficient-evidence
                                               # = strongest over components (affected > potentially >
                                               #   insufficient > not-affected)
    verification: observed                     # observed (kind upgrade) | render (helm/kustomize before/after)
                                               # | upstream-only (no outcome could be produced; say why in NOTES)
    observed: [observed/03-after-renewal.txt]  # raw evidence for the outcome, relative to the case dir
    citations:
      - {url: <upstream URL>, lines: "L12-L20", quote: "..."}
    components:                                # one entry per environment component the item reaches
      - ref: {file: env/apps/payments/base/ca-certificate.yaml, kind: Certificate, namespace: payments, name: payments-ca}
        classification: affected
        impactPath:                            # ordered hops: exposed component → … → consumer
          - {ref: {file: env/clusters/prod/cert-manager.yaml, kind: Application, namespace: argocd, name: cert-manager}, relation: deploys}
          - {ref: {file: env/apps/payments/base/ca-certificate.yaml, kind: Certificate, name: payments-ca}, field: spec.privateKey.rotationPolicy, relation: issues-secret}
          - {ref: {file: env/apps/payments/base/ca-issuer.yaml, kind: Issuer, name: payments-ca}, relation: references}
          - {ref: {file: ..., kind: ConfigMap, name: payments-trust}, relation: consumed-by}
        reason: one-paragraph explanation in terms of the fixture (what breaks, for whom)
        mitigation: null                       # for not-affected: the override/absence that neutralises it
                                               #   {ref: {...}, field: ..., value: ..., kind: override|feature-unused|relationship-absent}
        requiredEvidence:                      # facts a correct finding must cite (scored as evidence coverage)
          - {file: env/apps/payments/base/ca-certificate.yaml, object: Certificate/payments-ca, field: spec.privateKey}
    match:                                     # how a system finding is recognised as being about this item
      - subject: spec.privateKey.rotationPolicy
      - text: '(?i)rotation ?policy'
```

Relation vocabulary is whatever DESIGN.md's `GraphEdge` kinds end up being; until then I use plain
verbs (deploys, renders-to, configures, overrides, references, issues-secret, mounts, selects,
consumed-by) and will rename on adoption of FORMAT.md.

## Case plan

| Case | Pair | Item(s) | Env difference | State |
|---|---|---|---|---|
| cert-manager-1.17-1.18--payments-eu | mesh-ca | B1 rotationPolicy default, B2 revisionHistoryLimit default | root CA has no rotationPolicy | env + observed run done; expected/NOTES pending |
| cert-manager-1.17-1.18--payments-us | mesh-ca | same | prod overlay patches root CA `rotationPolicy: Never` | env done; observed run in progress |
| ingress-nginx-1.11-1.12--shop-de | risk-level | annotations-risk-level default Critical→High (#11819) | storefront overlay adds configuration-snippet | env done; observed run in progress |
| ingress-nginx-1.11-1.12--shop-fr | risk-level | same | values-prod.yaml sets annotations-risk-level: Critical | env done; run pending |
| (stretch) insufficient-evidence case | — | ingress-nginx risk level | controller values live in a repo that is not provided | planned |

## Done

- Read FLEET.md, MISSION.md, brief, eval/FORMAT.md, Phase 3 groundtruth brief.
- Draft format above.
- Upstream research pair 1 + pair 2 (sources in scratch; citations go to NOTES.md).
- Docker Desktop was wedged (backend up 9 days, hit "no space left on device" on 2026-10-07; daemon
  did not answer `_ping`). Restarted it (killed the wedged processes, relaunched); now healthy.
- payments-eu observed run: on 1.17.2 re-issuing the root CA kept its key; on 1.18.0 it rotated, and
  checkout→ledger mTLS failed (`curl: (60)`) once ledger's leaf was re-issued under the new CA key.

## In progress / next step

- Wait for payments-us and shop-de runs; run shop-fr (`<case>/observed/run.sh <case> ri-groundtruth-<x> <charts-dir>`;
  charts are `helm pull`ed from the upstream repos into a scratch dir).
- Then write expected.yaml + NOTES.md per case, README index, insufficient-evidence case.

## Decisions

- Inputs and expectations in separate files (`input.yaml` vs `expected.yaml`) so pipeline lanes can
  read inputs without seeing ground truth (FLEET.md evaluation integrity).
- Case ids use neutral environment slugs (`--payments-eu`, `--shop-fr`), never `--affected`: the id is an
  input and must not leak the expected class. Fixture comments read like an operator's, never mention
  the upgrade.
- Each pair's two environments differ only in the one setting that decides impact (`diff -r` of the
  two env/ trees shows just that plus names), so the pair isolates the mechanism.
- Argo CD is not run in kind: each Application is resolved by hand into the equivalent
  `helm upgrade --install` / `kubectl apply -k`, documented in each run.sh.
- Renewals are forced the way `cmctl renew` does (status condition Issuing=True, ManuallyTriggered)
  to stand in for the scheduled renewBefore window.

## Files touched outside ownership

(none)

## Open questions

(none)

## Test status

(not yet run)
