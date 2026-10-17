# Investigation ground truth (Phase 4, MISSION AC5 / P6)

Known-outcome cases for environment-aware impact investigation: one dependency upgrade applied to one
specific GitOps environment, with expected classifications per in-scope change item and per
environment component, confirmed by an observed upgrade in an isolated kind cluster wherever
possible. Authored blind by the `groundtruth` lane from upstream sources; never derived from Release
Intelligence output (docs/phase4/env-impact/FLEET.md, "Evaluation integrity").

The format is specified in [`FORMAT.md`](FORMAT.md); the pre-registered gates are in
[`gates.yaml`](gates.yaml).

**A system under test may read `input.yaml` and `env/` only.** `expected.yaml`, `NOTES.md` and
`observed/` are ground truth (NOTES.md and observed/ explain and evidence the expectations, so they
leak them) and must not be read while designing heuristics, prompts or policy. This index
deliberately states no expected class.

## Cases

| Case | Pair | Product | From → to | Verification |
|---|---|---|---|---|
| [cert-manager-1.17-1.18--payments-eu](cert-manager-1.17-1.18--payments-eu/) | cert-manager-1.17-1.18--mesh-ca | cert-manager | v1.17.2 → v1.18.0 | observed |
| [cert-manager-1.17-1.18--payments-us](cert-manager-1.17-1.18--payments-us/) | cert-manager-1.17-1.18--mesh-ca | cert-manager | v1.17.2 → v1.18.0 | observed |
| [ingress-nginx-1.11-1.12--shop-de](ingress-nginx-1.11-1.12--shop-de/) | ingress-nginx-1.11-1.12--security-defaults | ingress-nginx | controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) | observed |
| [ingress-nginx-1.11-1.12--shop-fr](ingress-nginx-1.11-1.12--shop-fr/) | ingress-nginx-1.11-1.12--security-defaults | ingress-nginx | controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) | observed |
| [ingress-nginx-1.11-1.12--shop-pl](ingress-nginx-1.11-1.12--shop-pl/) | ingress-nginx-1.11-1.12--security-defaults | ingress-nginx | controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) | observed-alternatives (an input is withheld) |
| [ingress-nginx-1.11-1.12--shop-nl](ingress-nginx-1.11-1.12--shop-nl/) | ingress-nginx-1.11-1.12--chart-cleanup | ingress-nginx | controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) | observed |
| [kube-prometheus-stack-89-90--obs-north](kube-prometheus-stack-89-90--obs-north/) | kube-prometheus-stack-89-90--control-plane-auth | kube-prometheus-stack | chart 89.2.4 → 90.2.0 | observed |
| [kube-prometheus-stack-89-90--obs-south](kube-prometheus-stack-89-90--obs-south/) | kube-prometheus-stack-89-90--control-plane-auth | kube-prometheus-stack | chart 89.2.4 → 90.2.0 | observed |

Environments that share a `pair` differ only in the setting(s) that decide impact (FORMAT.md). Every
observed run used kind v0.24.0 with Kubernetes v1.31.0 and deleted its cluster afterwards.

## Re-running an observed outcome

```sh
# charts from the upstream repos, e.g. into /tmp/charts:
helm pull cert-manager --repo https://charts.jetstack.io --version v1.17.2 -d /tmp/charts
helm pull cert-manager --repo https://charts.jetstack.io --version v1.18.0 -d /tmp/charts
helm pull ingress-nginx --repo https://kubernetes.github.io/ingress-nginx --version 4.11.5 -d /tmp/charts
helm pull ingress-nginx --repo https://kubernetes.github.io/ingress-nginx --version 4.12.0 -d /tmp/charts
helm pull kube-prometheus-stack --repo https://prometheus-community.github.io/helm-charts --version 89.2.4 -d /tmp/charts
helm pull kube-prometheus-stack --repo https://prometheus-community.github.io/helm-charts --version 90.2.0 -d /tmp/charts
cd eval/investigations
<case>/observed/run.sh <case> ri-groundtruth-<name> /tmp/charts   # creates and deletes a kind cluster
```

A re-run rewrites `observed/run.log` and `observed/render.diff`. Key fingerprints, pod names and
timestamps differ per run; the verdicts must not. (shop-pl has no run script: its deciding input is
withheld.)
