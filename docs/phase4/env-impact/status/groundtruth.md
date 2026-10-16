# Status — lane `groundtruth` (Phase 4 env-impact)

Agent: ri-groundtruth (Claude Opus 5.5). Branch `p4ei/groundtruth`. Brief: `briefs/groundtruth.md`.
Last updated: 2026-10-16 (final wave: FORMAT.md adoption complete).

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

## Case format

Adopted `eval/investigations/FORMAT.md` (contract lane → commander) in all 8 cases (final wave,
after `git merge p4-env-impact` @ 6e03b550). The earlier draft that lived here is superseded.

- Relations use the contract's `EdgeKind`s (internal/domain/investigation.go:1021-1030):
  `issues` → `produces` (with the produced Secret as its own hop), `signs` → `references`
  (Secret → ClusterIssuer → leaf Certificate), mounts → `references`, `routes-to`/`calls` → the path
  ends at the consuming object (OTel export → `depends-on`), ServiceMonitor → Prometheus `configures`,
  Prometheus → PrometheusRule `depends-on`. Every path's last hop carries no relation.
- Citations carry `lines:`; values-file refs are file-only; `requiredEvidence` has no `object: values`;
  `input.yaml` has `kubernetes: "1.31"` (the version every observed run used).
- `README.md` index no longer states any expected class or mechanism (FORMAT.md: index lists case,
  pair, product, from→to, verification).

## Cases

| Case | Group | Items | Env difference | Expected | Verification | State |
|---|---|---|---|---|---|---|
| cert-manager-1.17-1.18--payments-eu | mesh-ca | B1 rotationPolicy default, B2 revisionHistoryLimit default | root CA has no rotationPolicy | B1 affected, B2 not-affected | observed | done |
| cert-manager-1.17-1.18--payments-us | mesh-ca | same | prod overlay patches root CA `rotationPolicy: Never` | B1 not-affected, B2 not-affected | observed | done |
| ingress-nginx-1.11-1.12--shop-de | security-defaults | B1 risk level, B2 cross-ns, B3 strict path, B4 annotation validation | storefront overlay adds configuration-snippet; no risk level set | B1 affected, B2–B4 not-affected | observed | done |
| ingress-nginx-1.11-1.12--shop-fr | security-defaults | same | values-prod.yaml sets annotations-risk-level: Critical | all not-affected | observed | done |
| ingress-nginx-1.11-1.12--shop-pl | security-defaults | same | ingress-nginx value files withheld | B1 insufficient-evidence (acceptable potentially-affected), B2–B4 not-affected | observed-alternatives | done |
| kube-prometheus-stack-89-90--obs-north | control-plane-auth | B1 control-plane SM auth via token Secret, B2 secretFieldSelector, B3 removed values | values-prod sets ignoreNamespaceSelectors: true | B1 affected, B2/B3 not-affected | observed | done (follow-up) |
| kube-prometheus-stack-89-90--obs-south | control-plane-auth | same | does not | all not-affected | observed | done (follow-up) |
| ingress-nginx-1.11-1.12--shop-nl | chart-cleanup | B1 OTel init container removed (#12024), B2 metrics default (#12153) | OTel + metrics enabled | both not-affected (diff-visible, harmless) | observed | done (follow-up) |

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

## Follow-up wave results (2026-10-16)

- **kube-prometheus-stack 89.2.4 → 90.2.0** (chart-only change, operator v0.93.1 and CRDs identical):
  - obs-north (`ignoreNamespaceSelectors: true`): after the upgrade the operator rejects
    `kube-system/…-kubelet`, `kube-system/…-coredns`, `default/…-apiserver` ("unable to get secret
    kube-prometheus-stack-prometheus-token … not found"); scrape jobs 12 → 7; cAdvisor series for the
    checkout memory alert: 2 → none (alert silently dead); KSM-based rule unaffected.
  - obs-south: same rendered diff; all 12 jobs, kubelet/coredns/apiserver up, rule inputs intact.
  - The decisive fact (ServiceMonitor namespace vs Secret namespace) is in the rendered objects'
    unchanged context, not inside any diff hunk.
- **ingress-nginx shop-nl** (OTel + metrics enabled): rendered diff removes the OTel init container and
  `modules` volume and adds `--enable-metrics=true`; observed spans at the collector (4/4 before and
  after), metrics endpoint 200 before and after, both hosts 200. Harmless.

## In progress / next step

First 5 cases merged by the commander into p4-env-impact (23f79063). Do NOT merge p4-env-impact
until told (FORMAT.md + EdgeKind adoption comes later).

Follow-up requested by the commander (2026-10-16) — DONE (see "Follow-up wave results"):
1. Diff-visible pair — chosen: **kube-prometheus-stack 89.2.4 → 90.2.0** (chart-only change, same
   operator v0.93.1): control-plane ServiceMonitors switch from `bearerTokenFile` to
   `authorization` from a chart-created token Secret in the release namespace (PR #7238). With
   `prometheus.prometheusSpec.ignoreNamespaceSelectors: true` the kubelet/control-plane
   ServiceMonitors render into kube-system (apiserver: default), where that Secret does not exist ⇒
   operator rejects them (resource_selector.go L237, L150-L155) ⇒ kubelet/cAdvisor scraping stops.
   Pair: env sets ignoreNamespaceSelectors (affected) vs not (not-affected, same diff shape).
   Next: write env fixtures, run in kind, expected + NOTES.
2. Diff-visible-but-harmless trap: ingress-nginx 4.11.5 → 4.12.0 with OpenTelemetry enabled
   (init container + module volume vanish from the rendered Deployment; #12024 says OTel keeps
   working) + the `--enable-metrics=false` arg removal (#12153; chart compensates).
- When the commander says so: `git merge p4-env-impact`, adopt FORMAT.md (rename fields/relations if
  it differs from the draft above), keep the expectations unchanged.

## Final wave (2026-10-16)

- `git merge p4-env-impact` (fast-forward to 6e03b550).
- FORMAT.md + EdgeKind adoption across all 8 cases (see "Case format").
- `renderedChanges` added to the first five cases from their own `observed/render.diff`
  (cert-manager: 6 groups, all harmless by themselves — the B1/B2 defaults appear only as CRD
  documentation text; shop-de/fr: 2 groups, none touching B1–B4; shop-pl: `[]`, no render possible
  without the withheld values). No new kind runs.
- Re-verified with a FORMAT validator (scratch; checks required keys, class/verification/EdgeKind/
  mitigation vocabularies, item = strongest component class, refs/evidence/observed files exist,
  render.diff line ranges in bounds, regexes compile): 8 cases, no errors. 37 kustomizations render.
- Expectations unchanged in substance: no classification, component, mitigation or match was altered.

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

- `eval/investigations/FORMAT.md` (commander-owned): one additive CONTRACT-CHANGE section, as instructed.

## Contract changes

- `eval/investigations/FORMAT.md`: appended section "Additive fields used by the dataset", marked
  `<!-- CONTRACT-CHANGE(groundtruth) -->` (format only; nothing relaxed, no scoring changed):
  `renderedChanges` (per-hunk labels of `observed/render.diff`: diff range(s), object, change, item or
  null, impact breaking|harmless of the rendered change by itself; `[]` when no render is possible),
  `ref.renderedFrom`, file-only values refs, `requiredEvidence` without `object`,
  `acceptable` (informational — the scored label stays `classification`), `evidenceGaps`,
  `remediation`, `verification: observed-alternatives` (reported separately like upstream-only), and
  the `withheld[]` entry shape.

## Open questions

(none blocking)

## Cleanup (follow-up wave)

- kind clusters created and deleted by the run scripts: ri-groundtruth-kps-n, ri-groundtruth-kps-s,
  ri-groundtruth-in-nl (earlier wave: -cm-eu, -cm-us, -in-de, -in-fr). `kind get clusters`: none.
- Scratch only (not in the repo): chart tarballs and renders under the session scratchpad.
- No Docker images or volumes removed (kind node image and pulled images stay in the local cache).

## Test status

2026-10-16 final wave (after merging p4-env-impact @ 6e03b550): `go build ./... && go vet ./... &&
go test ./...` green — 35 packages ok, 4 without tests, no FAIL (incl. internal/investigate/relations,
which reads the env/ fixtures). FORMAT validator: 8 cases, no errors. 37 kustomizations render.
No kind clusters exist.
