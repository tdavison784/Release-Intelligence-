# Status — lane `groundtruth` (Phase 4 env-impact)

Agent: ri-groundtruth (Claude Opus 5.5). Branch `p4ei/groundtruth`. Brief: `briefs/groundtruth.md`.
Last updated: 2026-10-16 ~13:30 CDT.

## Finding for the commander / contract lane (read this first)

The MISSION clarification (rendered FROM/TO diffs are the centerpiece evidence) meets a hard fact in
these cases: **every decisive change in the five cases is a default compiled into the new controller
binary.** In the chart's rendered diff with the environment's values it shows up at most as CRD
*documentation text* (cert-manager: `spec.privateKey.rotationPolicy` description "Default is
`Never`…" → "Default is `Always`…", render.diff L87-L93) or not at all (ingress-nginx: the risk
level, cross-namespace, path-type and annotation-validation defaults leave the rendered manifests
and the controller ConfigMap byte-identical; the chart only renders those flags when a value
deviates from the chart's own default). Consequences for DESIGN.md §3 policy:

1. "No uncorrelated rendered change ⇒ safe" is unsafe on its own: upstream change items with *no*
   rendered counterpart must still be resolved against the environment (or yield
   insufficient-evidence). A not-affected rule of the form "every correlated rendered change is
   covered by a passing validation" is vacuously true for these items.
2. Defaults in binaries are only checkable by correlating the upstream item (release notes / source)
   with the *absence* of an override in the environment (values, overlay patches) and following the
   consumers. The cases are built to test exactly that.
3. Conversely, rendered changes can be harmless (ingress-nginx 4.12 drops `--enable-metrics=false`
   from the args because the controller default became false: no behaviour change).

No action requested from me; recording it so the contract lane can make sure its not-affected rule
does not pass vacuously for prose/default-only items. I did not add a diff-visible pair (see
"Possible follow-up").

## Case format (draft, for `eval/investigations/FORMAT.md`; the contract lane owns the file)

Goal of the split: everything a system under test may read is in `input.yaml` + `env/`; everything it
is scored against is in `expected.yaml` (and NOTES.md / observed/, which explain it), which
pipeline/agent lanes must never read (FLEET.md).

```
eval/investigations/
  README.md                       # index of cases
  <case-id>/
    input.yaml                    # INPUTS ONLY: upgrade + environment + breaking items in scope
    env/                          # the GitOps repository fixture (a plain directory = repo root)
    expected.yaml                 # GROUND TRUTH (blind; only the groundtruth lane writes it)
    NOTES.md                      # upstream citations (URL + line ranges / quotes) + observed outcome
    observed/                     # run.sh (reproducible), run.log, render.diff — raw, never edited by hand
```

Case ids: `<product>-<fromMinor>-<toMinor>--<neutral-env-slug>` (e.g. `--payments-eu`). Never encode
the expected class in an id, file name or fixture comment: the id is an input.

### input.yaml (readable by every lane)

```yaml
id: cert-manager-1.17-1.18--payments-eu
pair: cert-manager-1.17-1.18--mesh-ca   # group id shared by the environments of one upgrade+topic
product: cert-manager                   # products/<id>.yaml
chart: {repo: https://charts.jetstack.io, name: cert-manager}
from: {version: v1.17.2, chartVersion: v1.17.2}
to:   {version: v1.18.0, chartVersion: v1.18.0}
environment:
  id: payments-prod-eu
  root: env/
  entrypoints: [clusters/prod/kustomization.yaml]  # where a GitOps controller starts
  description: >-                                  # what an operator would say; no impact hints
    ...
  complete: true                                   # the repo declares everything that runs (or not)
scope:                                             # upstream items this case examines
  - id: B1
    title: Certificate.spec.privateKey.rotationPolicy default changed from Never to Always
    subject: spec.privateKey.rotationPolicy        # same notion as eval/cases matchers
    source: <upstream URL, pinned to a commit where possible>
withheld:                                          # inputs deliberately absent (insufficient-evidence cases)
  - ref: https://git.example.com/platform/ingress-values.git (shop/common.yaml, shop/prod-pl.yaml)
    description: as an operator would describe the gap; no expected class
```

### expected.yaml (ground truth — pipeline/agent lanes must not read)

```yaml
case: <id>
authoredAt: "2026-10-16"
authoredBy: groundtruth lane (model), blind
items:
  - item: B1                                 # input.yaml scope id
    classification: affected                 # affected | not-affected | potentially-affected | insufficient-evidence
                                             # = strongest over components (affected > potentially >
                                             #   insufficient > not-affected)
    acceptable: [potentially-affected]       # optional; only where MISSION's definitions genuinely
                                             #   overlap (NOTES.md must justify); scorers count a match
                                             #   on classification OR acceptable as correct
    verification: observed                   # observed (kind upgrade) | observed-alternatives (the
                                             #   deciding input is withheld; each possible value was
                                             #   observed in a sibling case) | render | upstream-only
    observed: [observed/run.log]             # raw evidence, relative to the case dir
    citations: [{url, quote}]                # url pinned (commit/tag) with #Lx-Ly where possible
    components:                              # one per environment object the item reaches
      - ref: {file: env/..., kind, namespace, name}     # object declared in the repo, or
             # {renderedFrom: env/<file declaring the release>, kind, namespace, name} for objects
             # that only exist after rendering a chart (e.g. the controller ConfigMap)
        classification: affected
        reason: >- …                         # in terms of the fixture
        impactPath:                          # ordered hops, exposed component → … → consumer
          - {ref: {...}, field: <path>, relation: deploys|configures|issues|signs|routes-to|calls|...}
        mitigation: null | {kind: override|feature-unused|relationship-absent, ref?, field?, value?, detail?}
        evidenceGaps: [ ... ]                # insufficient-evidence only: what is missing
        requiredEvidence:                    # facts a correct finding must cite (evidence coverage)
          - {file: env/..., object: Kind/ns/name, field: <path>}
    remediation: >- …                        # or the next investigation step
    match:                                   # how a system finding is recognised as being about this item
      - subject: spec.privateKey.rotationPolicy
      - text: '(?i)rotation ?policy'
```

Relation vocabulary is plain verbs until DESIGN.md's `GraphEdge` kinds are final; I will rename on
adoption of FORMAT.md. Edges point from provider to consumer (same direction the contract lane chose).

## Cases

| Case | Group | Items | Env difference | Expected | Verification | State |
|---|---|---|---|---|---|---|
| cert-manager-1.17-1.18--payments-eu | mesh-ca | B1 rotationPolicy default, B2 revisionHistoryLimit default | root CA has no rotationPolicy | B1 affected, B2 not-affected | observed | done |
| cert-manager-1.17-1.18--payments-us | mesh-ca | same | prod overlay patches root CA `rotationPolicy: Never` | B1 not-affected, B2 not-affected | observed | done |
| ingress-nginx-1.11-1.12--shop-de | security-defaults | B1 risk level, B2 cross-ns, B3 strict path, B4 annotation validation | storefront overlay adds configuration-snippet; no risk level set | B1 affected, B2–B4 not-affected | observed | done |
| ingress-nginx-1.11-1.12--shop-fr | security-defaults | same | values-prod.yaml sets annotations-risk-level: Critical | all not-affected | observed | done |
| ingress-nginx-1.11-1.12--shop-pl | security-defaults | same | ingress-nginx value files withheld | B1 insufficient-evidence (acceptable potentially-affected), B2–B4 not-affected | observed-alternatives | done |

Required minimum (2 pairs, 2 products) met; stretch met: both pairs are only visible through
transitive relationships (overlay patches on base objects; consumers 2–5 hops away), plus the
insufficient-evidence case.

## Done

- Read FLEET.md, MISSION.md (+ the binding "DFS = diffs" clarification on p4-env-impact), brief,
  eval/FORMAT.md, Phase 3 groundtruth brief.
- Upstream research (release notes, upgrade guide, docs, PR diffs, source at the release tags, field
  issues), cited with pinned URLs and line ranges in each NOTES.md.
- Five cases with env fixtures, input.yaml, expected.yaml, NOTES.md, observed/{run.sh,run.log,render.diff}.
- Known outcomes observed in kind (kind v0.24.0, Kubernetes v1.31.0), clusters deleted:
  - payments-eu: on 1.17.2 re-issuing the root CA kept its key; on 1.18.0 it rotated, and
    checkout→ledger mTLS failed (`curl: (60) unable to get local issuer certificate`) once ledger's
    leaf was re-issued; recovered only after both leaves were re-issued.
  - payments-us: root CA key unchanged across re-issuance on 1.18.0; leaves rotated; every call OK.
  - shop-de: after the upgrade the storefront Ingress is dropped (`store.go:938 … risky annotation`),
    `shop.example.com` → 404, re-sync denied by the admission webhook; catalog-api unaffected.
  - shop-fr: everything served and admitted on 1.12.0, snippet headers intact.
- `eval/investigations/README.md` index.
- Docker Desktop was wedged at lane start (backend up 9 days, hit "no space left on device" on
  2026-10-07, daemon not answering `_ping`). Restarted it (killed the wedged processes, relaunched).

## In progress / next step

- Lane complete (LANE DONE sent).
- When the commander says so: `git merge p4-env-impact`, adopt FORMAT.md (rename fields/relations if
  it differs from the draft above), keep the expectations unchanged.

## Possible follow-up (not started; commander's call)

- A pair whose break **is** visible in the rendered diff (e.g. a chart values key renamed/removed so
  an environment override silently stops applying). The two releases used here have none: the
  4.11→4.12 chart removes only `controller.opentelemetry.*` (harmless per #12024: the module is in
  the controller image since v1.10) and PSP templates; cert-manager's chart changes nothing relevant.
  Candidates would need a different release (e.g. argo-cd 2.14→3.0 tracking-method default in the
  rendered argocd-cm, or a kube-prometheus-stack selector change); each needs fresh research.
- A diff-visible-but-harmless case (OpenTelemetry init container removal in an OTel-enabled
  environment) as a false-positive trap for diff-driven investigation.

## Decisions

- Inputs and expectations in separate files (`input.yaml` vs `expected.yaml`) so pipeline lanes can
  read inputs without seeing ground truth (FLEET.md evaluation integrity). NOTES.md and observed/
  also leak expectations and are listed as ground truth in the README.
- Neutral environment slugs in case ids; fixture comments read like an operator's, never mention the
  upgrade.
- Each pair's environments differ only in the one setting that decides impact (`diff -r` shows just
  that plus names), so the pair isolates the mechanism.
- Argo CD is not run in kind: each Application is resolved by hand into the equivalent
  `helm upgrade --install` / `kubectl apply -k` (documented in each run.sh).
- Scheduled renewals are forced the way `cmctl renew` does (status condition Issuing=True,
  ManuallyTriggered) to stand in for the renewBefore window.
- Affected components are the consumers that break (Deployments / the Ingress), not the exposed
  object; objects whose behaviour changes harmlessly (leaf key rotation) are listed as not-affected
  components so over-flagging is measurable.
- Sibling default flips from the same release entry (ingress-nginx B2–B4, cert-manager B2) are in
  scope as not-affected items to measure false positives.
- shop-pl: `insufficient-evidence` primary with `acceptable: [potentially-affected]` (MISSION's
  definitions overlap when the deciding input is withheld; both fail closed). B2–B4 stay not-affected
  because they do not depend on the withheld input.
- Demo backends are `hashicorp/http-echo` so a 404 can only come from the ingress controller.
- Scripts retry only connection-level curl errors (exit 7/28) right after a rollout; TLS verdicts are
  recorded on the first attempt. Recorded runs needed no retries.

## Files touched outside ownership

(none)

## Contract changes

(none; format draft above is a proposal for the contract lane's FORMAT.md)

## Open questions

(none blocking)

## Test status

2026-10-16 on 1b14540f: `go build ./... && go vet ./... && go test ./...` green (35 packages ok, no
FAIL; this lane changes no Go code). All case YAML parses; every env overlay renders with
`kubectl kustomize`. No kind clusters left behind.
