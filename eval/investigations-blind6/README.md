# Blind ground truth for Milestone 6 (PLAN-M6 §2, ledger D-0032)

Four affected/unaffected pairs, eight environments, authored blind by the `groundtruth6` lane from
upstream sources (release notes, upgrade guides, code and charts at tags, the GitHub advisory
database) and decided by outcomes observed in throwaway kind clusters. Never derived from Release
Intelligence output; RI was not run on these cases. Same layout and format as `eval/investigations/`
([`FORMAT.md`](../investigations/FORMAT.md)).

**A system under test may read `input.yaml` and `env/` only.** `expected.yaml`, `NOTES.md` and
`observed/` are ground truth. This index states no expected class.

## Cases

| Case | Pair | Product | From → to | Verification |
|---|---|---|---|---|
| [traefik-3.6-3.6--retail-emea](traefik-3.6-3.6--retail-emea/) | traefik-3.6-3.6--shared-middlewares | traefik | v3.6.10 → v3.6.15 (chart 39.0.5 → 39.0.9) | observed |
| [traefik-3.6-3.6--retail-apac](traefik-3.6-3.6--retail-apac/) | traefik-3.6-3.6--shared-middlewares | traefik | v3.6.10 → v3.6.15 (chart 39.0.5 → 39.0.9) | observed |
| [cert-manager-1.20-1.21--treasury-de](cert-manager-1.20-1.21--treasury-de/) | cert-manager-1.20-1.21--vault-pki | cert-manager | v1.20.4 → v1.21.3 | observed |
| [cert-manager-1.20-1.21--treasury-us](cert-manager-1.20-1.21--treasury-us/) | cert-manager-1.20-1.21--vault-pki | cert-manager | v1.20.4 → v1.21.3 | observed |
| [external-secrets-0.18-0.19--orders-eu](external-secrets-0.18-0.19--orders-eu/) | external-secrets-0.18-0.19--crd-delivery | external-secrets | v0.18.2 → v0.19.2 (chart 0.18.2 → 0.19.2) | observed |
| [external-secrets-0.18-0.19--orders-br](external-secrets-0.18-0.19--orders-br/) | external-secrets-0.18-0.19--crd-delivery | external-secrets | v0.18.2 → v0.19.2 (chart 0.18.2 → 0.19.2) | observed-alternatives (an input is withheld) |
| [argo-cd-2.14-3.0--studio-east](argo-cd-2.14-3.0--studio-east/) | argo-cd-2.14-3.0--developer-access | argo-cd | v2.14.11 → v3.0.12 (chart 7.9.1 → 8.2.7) | observed |
| [argo-cd-2.14-3.0--studio-west](argo-cd-2.14-3.0--studio-west/) | argo-cd-2.14-3.0--developer-access | argo-cd | v2.14.11 → v3.0.12 (chart 7.9.1 → 8.2.7) | observed |

Environments of a pair differ only in the setting(s) that decide the outcome. Kind: v0.24.0 with
Kubernetes v1.31.0, except the cert-manager pair (cert-manager 1.21 needs Kubernetes ≥ 1.33): kind
v0.33.0 with Kubernetes v1.34.11. Every cluster was deleted after its run.

Upstream data (release notes, advisories) is not recorded here; the commander records it through RI's
online mode (brief).

## Additive fields (format only; nothing in FORMAT.md is relaxed)

| Field | Where | Meaning |
|---|---|---|
| `decisive` | expected.yaml | the item that decides the pair |
| `coverage` | expected.yaml | the PLAN-M6 §2 coverage items the case carries |
| `items[].apparentMitigation` | expected.yaml | a setting that looks like it preserves the old behaviour, with `effective` and why |
| `versionDecision` | expected.yaml | the version and advisory decision: current, proposed, regression, advisories introduced by the proposed target, the safer target (GitHub global advisory database, read date given) |

## Re-running an observed outcome

```sh
C=/tmp/charts; mkdir -p $C
helm pull traefik --repo https://traefik.github.io/charts --version 39.0.5 -d $C
helm pull traefik --repo https://traefik.github.io/charts --version 39.0.9 -d $C
helm pull cert-manager --repo https://charts.jetstack.io --version v1.20.4 -d $C
helm pull cert-manager --repo https://charts.jetstack.io --version v1.21.3 -d $C
helm pull external-secrets --repo https://charts.external-secrets.io --version 0.18.2 -d $C
helm pull external-secrets --repo https://charts.external-secrets.io --version 0.19.2 -d $C
for v in 7.9.1 8.2.7 8.5.1; do helm pull argo-cd --repo https://argoproj.github.io/argo-helm --version $v -d $C; done
helm pull argocd-apps --repo https://argoproj.github.io/argo-helm --version 2.0.2 -d $C
cd eval/investigations-blind6
traefik-3.6-3.6--retail-emea/observed/run.sh traefik-3.6-3.6--retail-emea ri-groundtruth6-x $C
# cert-manager: kind >= v0.31 on PATH and
#   KIND_NODE_IMAGE=kindest/node:v1.34.11@sha256:44e222ee2132dab25ff87301682f89eb82c7880ea3a1bf543bfe9708fd08d67d
cert-manager-1.20-1.21--treasury-de/observed/run.sh cert-manager-1.20-1.21--treasury-de ri-groundtruth6-x $C
external-secrets-0.18-0.19--orders-eu/observed/run.sh external-secrets-0.18-0.19--orders-eu ri-groundtruth6-x $C env
external-secrets-0.18-0.19--orders-br/observed/run.sh external-secrets-0.18-0.19--orders-br ri-groundtruth6-x $C ssa run-ssa.log
argo-cd-2.14-3.0--studio-east/observed/run.sh argo-cd-2.14-3.0--studio-east ri-groundtruth6-x $C
./sanitize-log.sh */observed/*.log
```

A re-run rewrites the case's log and `render.diff`. Pod names, keys, serials and timestamps differ per
run; the verdicts must not.
