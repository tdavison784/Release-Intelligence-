# Investigation ground truth (Phase 4, MISSION AC5 / P6)

Known-outcome cases for environment-aware impact investigation: one dependency upgrade, one
specific GitOps environment, an expected classification per in-scope breaking-change item and per
affected component, each confirmed by an observed upgrade in an isolated kind cluster wherever
possible. Authored blind by the `groundtruth` lane from upstream sources; never derived from
Release Intelligence output (docs/phase4/env-impact/FLEET.md, "Evaluation integrity").

**Pipeline and agent lanes may read `input.yaml` and `env/` only. `expected.yaml`, `NOTES.md` and
`observed/` are ground truth and must not be read while designing heuristics, prompts or policy.**
(NOTES.md and observed/ explain and evidence the expectations, so they leak them.)

The case format is specified in `FORMAT.md` (owned by the contract lane). Until it lands, the draft
is in `docs/phase4/env-impact/status/groundtruth.md`.

## Cases

| Case | Group | Upgrade | Environment difference | Expected (per item) | Verification |
|---|---|---|---|---|---|
| [cert-manager-1.17-1.18--payments-eu](cert-manager-1.17-1.18--payments-eu/) | mesh-ca | cert-manager v1.17.2 → v1.18.0 | internal CA root Certificate leaves `rotationPolicy` unset (base + overlay); two services trust only the `ca.crt` of their own Secret | B1 rotationPolicy default: **affected** (checkout, ledger); B2 revisionHistoryLimit: not-affected | observed |
| [cert-manager-1.17-1.18--payments-us](cert-manager-1.17-1.18--payments-us/) | mesh-ca | same | prod overlay patches the root CA to `rotationPolicy: Never` | B1: **not-affected** (override); B2: not-affected | observed |
| [ingress-nginx-1.11-1.12--shop-de](ingress-nginx-1.11-1.12--shop-de/) | security-defaults | chart 4.11.5 → 4.12.0 (controller v1.11.5 → v1.12.0) | storefront prod overlay adds a `configuration-snippet`; value files set no risk level | B1 annotations-risk-level: **affected** (storefront); B2–B4: not-affected | observed |
| [ingress-nginx-1.11-1.12--shop-fr](ingress-nginx-1.11-1.12--shop-fr/) | security-defaults | same | `values-prod.yaml` sets `annotations-risk-level: Critical` | B1: **not-affected** (override); B2–B4: not-affected | observed |
| [ingress-nginx-1.11-1.12--shop-pl](ingress-nginx-1.11-1.12--shop-pl/) | security-defaults | same | ingress-nginx value files live in another repo, withheld from the input | B1: **insufficient-evidence** (acceptable: potentially-affected); B2–B4: not-affected | observed-alternatives (shop-de / shop-fr) |
| [kube-prometheus-stack-89-90--obs-north](kube-prometheus-stack-89-90--obs-north/) | control-plane-auth | chart 89.2.4 → 90.2.0 (operator v0.93.1 in both) | `values-prod.yaml` sets `prometheus.prometheusSpec.ignoreNamespaceSelectors: true` | B1 control-plane ServiceMonitor auth: **affected** (kubelet/apiserver/coredns monitors rejected; checkout memory alert loses its input); B2, B3: not-affected | observed |
| [kube-prometheus-stack-89-90--obs-south](kube-prometheus-stack-89-90--obs-south/) | control-plane-auth | same | does not set it | B1: **not-affected**; B2, B3: not-affected | observed |
| [ingress-nginx-1.11-1.12--shop-nl](ingress-nginx-1.11-1.12--shop-nl/) | chart-cleanup | chart 4.11.5 → 4.12.0 | OpenTelemetry tracing + annotation-scraped metrics enabled | B1 OTel init container removed: **not-affected**; B2 metrics default: **not-affected** (diff-visible, harmless) | observed |

Each pair's environments differ only in the one environmental setting that decides impact (`diff -r` of
the two `env/` trees shows that line group plus repo/cluster names). In the first two pairs the impact is only
visible through relationships: a Kustomize overlay patch on a base object, and a consumer two or
more hops from the exposed component (CA Certificate → CA issuer → leaf Certificates → Secrets →
Deployments; Application value files → rendered controller ConfigMap → Ingress → Service).

## What the rendered FROM/TO diff shows

- **Invisible in the diff** (mesh-ca, security-defaults): the decisive change is a default compiled
  into the new controller binary. The chart's rendered diff (`observed/render.diff`) shows it at
  most as CRD documentation text (cert-manager) or not at all (ingress-nginx). These cases test that
  an investigation correlates upstream change evidence with the environment instead of concluding
  "no relevant rendered change ⇒ not affected".
- **Visible in the diff, decided by the environment** (control-plane-auth): both environments get
  the same rendered change on their control-plane ServiceMonitors (`bearerTokenFile` →
  `authorization` from a new token Secret in the release namespace). Whether it breaks depends on a
  relationship the diff hunks do not show: the namespace those ServiceMonitors render into
  (`ignoreNamespaceSelectors`) versus the Secret's namespace.
- **Visible in the diff, harmless** (chart-cleanup / shop-nl): the OTel init container and module
  volume disappear and the metrics flag flips in the rendered controller Deployment, with no change
  in behaviour. A false-positive trap for diff-driven investigation.

## Re-running an observed outcome

```sh
# charts from the upstream repos, e.g. into /tmp/charts:
helm pull cert-manager --repo https://charts.jetstack.io --version v1.17.2 -d /tmp/charts
helm pull cert-manager --repo https://charts.jetstack.io --version v1.18.0 -d /tmp/charts
helm pull ingress-nginx --repo https://kubernetes.github.io/ingress-nginx --version 4.11.5 -d /tmp/charts
helm pull ingress-nginx --repo https://kubernetes.github.io/ingress-nginx --version 4.12.0 -d /tmp/charts
cd eval/investigations
<case>/observed/run.sh <case> ri-groundtruth-<name> /tmp/charts   # creates and deletes a kind cluster
```

Re-runs rewrite `observed/run.log` and `observed/render.diff`; key fingerprints and timestamps
differ per run; the verdicts must not.
