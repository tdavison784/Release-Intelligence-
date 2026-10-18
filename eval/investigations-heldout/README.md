# Held-out investigation ground truth (Phase 5, PROTOCOL §2 / §6 step 7)

Known-outcome cases for environment-aware impact investigation, authored blind by lane
`groundtruth5` on the sealed branch `p5ll/groundtruth5`: the upstream sources plus an observed
upgrade in a throwaway kind cluster, never Release Intelligence output, never a pipeline lane's
design. They were not available to any lane while the pipeline, the agent prompt, the validators or
the knowledge were built, and are the held-out measurement of PROTOCOL §5 S4.

Same layout and format as `eval/investigations/` ([FORMAT.md](../investigations/FORMAT.md),
including its additive fields). **A system under test may read `input.yaml` and `env/` only.**
`expected.yaml`, `NOTES.md` and `observed/` are ground truth. This index deliberately states no
expected class.

## Cases

| Case | Pair | Product | From → to | Verification |
|---|---|---|---|---|
| [cert-manager-1.17-1.18--telemetry-east](cert-manager-1.17-1.18--telemetry-east/) | cert-manager-1.17-1.18--telemetry-pki | cert-manager | v1.17.2 → v1.18.0 | observed |
| [cert-manager-1.17-1.18--telemetry-west](cert-manager-1.17-1.18--telemetry-west/) | cert-manager-1.17-1.18--telemetry-pki | cert-manager | v1.17.2 → v1.18.0 | observed |
| [cert-manager-1.17-1.18--registry-apac](cert-manager-1.17-1.18--registry-apac/) | – | cert-manager | v1.17.2 → v1.18.0 | observed |
| [cert-manager-1.17-1.18--billing-nordics](cert-manager-1.17-1.18--billing-nordics/) | – | cert-manager | v1.17.2 → v1.18.0 | observed |
| [cert-manager-1.17-1.18--search-latam](cert-manager-1.17-1.18--search-latam/) | – | cert-manager | v1.17.2 → v1.18.0 | observed |
| [ingress-nginx-1.11-1.12--docs-eu](ingress-nginx-1.11-1.12--docs-eu/) | ingress-nginx-1.11-1.12--docs-paths | ingress-nginx | controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) | observed |
| [ingress-nginx-1.11-1.12--docs-us](ingress-nginx-1.11-1.12--docs-us/) | ingress-nginx-1.11-1.12--docs-paths | ingress-nginx | controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) | observed |
| [ingress-nginx-1.11-1.12--partner-api](ingress-nginx-1.11-1.12--partner-api/) | – | ingress-nginx | controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) | observed |
| [kube-prometheus-stack-89-90--obs-emea](kube-prometheus-stack-89-90--obs-emea/) | kube-prometheus-stack-89-90--token-secret | kube-prometheus-stack | chart 89.2.4 → 90.2.0 | observed |
| [kube-prometheus-stack-89-90--obs-amer](kube-prometheus-stack-89-90--obs-amer/) | kube-prometheus-stack-89-90--token-secret | kube-prometheus-stack | chart 89.2.4 → 90.2.0 | observed |

Environments that share a `pair` differ only in the setting(s) that decide impact (plus names).
Every observed run used kind v0.24.0 with Kubernetes v1.31.0 (`kindest/node:v1.31.0`) and deleted
its cluster afterwards.

## Re-running an observed outcome

```sh
# charts from the upstream repos, e.g. into /tmp/charts:
helm pull cert-manager --repo https://charts.jetstack.io --version v1.17.2 -d /tmp/charts
helm pull cert-manager --repo https://charts.jetstack.io --version v1.18.0 -d /tmp/charts
helm pull ingress-nginx --repo https://kubernetes.github.io/ingress-nginx --version 4.11.5 -d /tmp/charts
helm pull ingress-nginx --repo https://kubernetes.github.io/ingress-nginx --version 4.12.0 -d /tmp/charts
helm pull kube-prometheus-stack --repo https://prometheus-community.github.io/helm-charts --version 89.2.4 -d /tmp/charts
helm pull kube-prometheus-stack --repo https://prometheus-community.github.io/helm-charts --version 90.2.0 -d /tmp/charts
helm pull kyverno --repo https://kyverno.github.io/kyverno/ --version 3.3.7 -d /tmp/charts   # registry-apac
cd eval/investigations-heldout
<case>/observed/run.sh <case> ri-groundtruth5-<name> /tmp/charts   # creates and deletes a kind cluster
./render-case.sh <case> /tmp/charts                                  # rewrites <case>/observed/render.diff
```

A re-run rewrites `observed/run.log`. Key fingerprints, serials, pod names and timestamps differ per
run; the verdicts must not. Shared tooling: `observed-lib.sh` (kind and certificate helpers),
`render-diff.py` / `render-case.sh` (canonical per-object FROM/TO render diff; `observed/render-index.txt`
lists its hunks).
