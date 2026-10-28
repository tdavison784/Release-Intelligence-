# M6 scores (scripts/phase6/score-m6.py)

| case | decisive | expected |
|---|---|---|
| argo-cd-2.14-3.0--studio-east | B1 | affected |
| argo-cd-2.14-3.0--studio-west | B1 | not-affected |
| cert-manager-1.20-1.21--treasury-de | B1 | affected |
| cert-manager-1.20-1.21--treasury-us | B1 | not-affected |
| external-secrets-0.18-0.19--orders-br | B1 | insufficient-evidence |
| external-secrets-0.18-0.19--orders-eu | B1 | affected |
| traefik-3.6-3.6--retail-apac | B1 | not-affected |
| traefik-3.6-3.6--retail-emea | B1 | affected |

## Arm A

| run | case | authoritative | agent | outcome | unsafe clearance |
|---|---|---|---|---|---|
| M6-A2 | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | - | no |
| M6-A2 | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | - | no |
| M6-A2 | cert-manager-1.20-1.21--treasury-de | potentially-affected | - | - | no |
| M6-A2 | cert-manager-1.20-1.21--treasury-us | potentially-affected | - | - | no |
| M6-A2 | external-secrets-0.18-0.19--orders-br | insufficient-evidence | - | - | no |
| M6-A2 | external-secrets-0.18-0.19--orders-eu | insufficient-evidence | - | - | no |
| M6-A2 | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | - | no |
| M6-A2 | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | - | no |

## Arm B

| run | case | authoritative | agent | outcome | unsafe clearance |
|---|---|---|---|---|---|
| M6-B2-r1 | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | no-verdict | no |
| M6-B2-r1 | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | no-verdict | no |
| M6-B2-r1 | cert-manager-1.20-1.21--treasury-de | potentially-affected | affected | correct | no |
| M6-B2-r1 | cert-manager-1.20-1.21--treasury-us | potentially-affected | not-affected | correct | no |
| M6-B2-r1 | external-secrets-0.18-0.19--orders-br | potentially-affected | potentially-affected | correct? | no |
| M6-B2-r1 | external-secrets-0.18-0.19--orders-eu | potentially-affected | affected | correct | no |
| M6-B2-r1 | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | no-verdict | no |
| M6-B2-r1 | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | no-verdict | no |
| M6-B2-r2 | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | no-verdict | no |
| M6-B2-r2 | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | no-verdict | no |
| M6-B2-r2 | cert-manager-1.20-1.21--treasury-de | potentially-affected | affected | correct | no |
| M6-B2-r2 | cert-manager-1.20-1.21--treasury-us | potentially-affected | not-affected | correct | no |
| M6-B2-r2 | external-secrets-0.18-0.19--orders-br | potentially-affected | potentially-affected | correct? | no |
| M6-B2-r2 | external-secrets-0.18-0.19--orders-eu | potentially-affected | potentially-affected | hedged | no |
| M6-B2-r2 | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | no-verdict | no |
| M6-B2-r2 | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | no-verdict | no |
| M6-B2-r3 | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | no-verdict | no |
| M6-B2-r3 | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | no-verdict | no |
| M6-B2-r3 | cert-manager-1.20-1.21--treasury-de | potentially-affected | affected | correct | no |
| M6-B2-r3 | cert-manager-1.20-1.21--treasury-us | potentially-affected | not-affected | correct | no |
| M6-B2-r3 | external-secrets-0.18-0.19--orders-br | potentially-affected | potentially-affected | correct? | no |
| M6-B2-r3 | external-secrets-0.18-0.19--orders-eu | potentially-affected | affected | correct | no |
| M6-B2-r3 | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | no-verdict | no |
| M6-B2-r3 | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | no-verdict | no |

## Arm C/D

| run | case | authoritative | agent | outcome | unsafe clearance |
|---|---|---|---|---|---|
| M6-CD2-r1 | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | no-verdict | no |
| M6-CD2-r1 | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | no-verdict | no |
| M6-CD2-r1 | cert-manager-1.20-1.21--treasury-de | potentially-affected | affected | correct | no |
| M6-CD2-r1 | cert-manager-1.20-1.21--treasury-us | potentially-affected | not-affected | correct | no |
| M6-CD2-r1 | external-secrets-0.18-0.19--orders-br | potentially-affected | potentially-affected | correct? | no |
| M6-CD2-r1 | external-secrets-0.18-0.19--orders-eu | potentially-affected | potentially-affected | hedged | no |
| M6-CD2-r1 | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | no-verdict | no |
| M6-CD2-r1 | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | no-verdict | no |
| M6-CD2-r2 | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | no-verdict | no |
| M6-CD2-r2 | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | no-verdict | no |
| M6-CD2-r2 | cert-manager-1.20-1.21--treasury-de | potentially-affected | affected | correct | no |
| M6-CD2-r2 | cert-manager-1.20-1.21--treasury-us | potentially-affected | not-affected | correct | no |
| M6-CD2-r2 | external-secrets-0.18-0.19--orders-br | potentially-affected | potentially-affected | correct? | no |
| M6-CD2-r2 | external-secrets-0.18-0.19--orders-eu | potentially-affected | potentially-affected | hedged | no |
| M6-CD2-r2 | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | no-verdict | no |
| M6-CD2-r2 | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | no-verdict | no |
| M6-CD2b-r1 | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | no-verdict | no |
| M6-CD2b-r1 | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | no-verdict | no |
| M6-CD2b-r1 | cert-manager-1.20-1.21--treasury-de | potentially-affected | affected | correct | no |
| M6-CD2b-r1 | cert-manager-1.20-1.21--treasury-us | potentially-affected | not-affected | correct | no |
| M6-CD2b-r1 | external-secrets-0.18-0.19--orders-br | potentially-affected | potentially-affected | correct? | no |
| M6-CD2b-r1 | external-secrets-0.18-0.19--orders-eu | potentially-affected | potentially-affected | hedged | no |
| M6-CD2b-r1 | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | no-verdict | no |
| M6-CD2b-r1 | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | no-verdict | no |

## Arm E (A + KE)

| run | case | authoritative | agent | outcome | unsafe clearance |
|---|---|---|---|---|---|
| M6-E-A | argo-cd-2.14-3.0--studio-east | insufficient-evidence | - | - | no |
| M6-E-A | argo-cd-2.14-3.0--studio-west | insufficient-evidence | - | - | no |
| M6-E-A | cert-manager-1.20-1.21--treasury-de | potentially-affected | - | - | no |
| M6-E-A | cert-manager-1.20-1.21--treasury-us | potentially-affected | - | - | no |
| M6-E-A | external-secrets-0.18-0.19--orders-br | insufficient-evidence | - | - | no |
| M6-E-A | external-secrets-0.18-0.19--orders-eu | insufficient-evidence | - | - | no |
| M6-E-A | traefik-3.6-3.6--retail-apac | insufficient-evidence | - | - | no |
| M6-E-A | traefik-3.6-3.6--retail-emea | insufficient-evidence | - | - | no |

## Summary

| arm | runs | mean correct /8 | mean false-safe | unsafe clearances | unsafeEligibility |
|---|---|---|---|---|---|
| A | 1 | 0.00 | 0.00 | 0 | 0/0 |
| B | 3 | 2.67 | 0.00 | 0 | 0/0, 0/0, 0/0 |
| C/D | 3 | 2.00 | 0.00 | 0 | 0/0, 0/0, 0/0 |
| E (A + KE) | 1 | 0.00 | 0.00 | 0 | 0/0 |

## D judgements (for P5 and the A3 grading)

| run | case | status | recommendation | proposed | chosen | minimum |
|---|---|---|---|---|---|---|
| M6-CD2-r1 | argo-cd-2.14-3.0--studio-east | complete | modify-target | v3.0.12 | v3.0.23 | v3.0.12 |
| M6-CD2-r1 | argo-cd-2.14-3.0--studio-west | complete | modify-target | v3.0.12 | v3.0.23 | v3.0.12 |
| M6-CD2-r1 | cert-manager-1.20-1.21--treasury-de | complete | proceed-with-changes | v1.21.3 | v1.21.3 | v1.21.3 |
| M6-CD2-r1 | cert-manager-1.20-1.21--treasury-us | complete | validation-required | v1.21.3 | v1.21.3 | v1.21.3 |
| M6-CD2-r1 | external-secrets-0.18-0.19--orders-br | complete | proceed-with-changes | v0.19.2 | v0.19.2 | v0.19.2 |
| M6-CD2-r1 | external-secrets-0.18-0.19--orders-eu | complete | proceed-with-changes | v0.19.2 | v0.19.2 | v0.19.2 |
| M6-CD2-r1 | traefik-3.6-3.6--retail-apac | complete | modify-target | v3.6.15 | v3.7.16 | v3.7.15 |
| M6-CD2-r1 | traefik-3.6-3.6--retail-emea | complete | modify-target | v3.6.15 | v3.7.16 | v3.7.15 |
| M6-CD2-r2 | argo-cd-2.14-3.0--studio-east | complete | modify-target | v3.0.12 | v3.0.23 | v3.0.12 |
| M6-CD2-r2 | argo-cd-2.14-3.0--studio-west | complete | modify-target | v3.0.12 | v3.0.23 | v3.0.12 |
| M6-CD2-r2 | cert-manager-1.20-1.21--treasury-de | complete | proceed-with-changes | v1.21.3 | v1.21.3 | v1.21.3 |
| M6-CD2-r2 | cert-manager-1.20-1.21--treasury-us | complete | validation-required | v1.21.3 | v1.21.3 | v1.21.3 |
| M6-CD2-r2 | external-secrets-0.18-0.19--orders-br | complete | proceed-with-changes | v0.19.2 | v0.19.2 | v0.19.2 |
| M6-CD2-r2 | external-secrets-0.18-0.19--orders-eu | complete | proceed-with-changes | v0.19.2 | v0.19.2 | v0.19.2 |
| M6-CD2-r2 | traefik-3.6-3.6--retail-apac | complete | modify-target | v3.6.15 | v3.7.16 | v3.7.15 |
| M6-CD2-r2 | traefik-3.6-3.6--retail-emea | complete | modify-target | v3.6.15 | v3.7.16 | v3.7.15 |
| M6-CD2b-r1 | argo-cd-2.14-3.0--studio-east | complete | modify-target | v3.0.12 | v3.0.23 | v3.0.12 |
| M6-CD2b-r1 | argo-cd-2.14-3.0--studio-west | complete | modify-target | v3.0.12 | v3.0.23 | v3.0.12 |
| M6-CD2b-r1 | cert-manager-1.20-1.21--treasury-de | complete | proceed-with-changes | v1.21.3 | v1.21.3 | v1.21.3 |
| M6-CD2b-r1 | cert-manager-1.20-1.21--treasury-us | complete | validation-required | v1.21.3 | v1.21.3 | v1.21.3 |
| M6-CD2b-r1 | external-secrets-0.18-0.19--orders-br | complete | proceed-with-changes | v0.19.2 | v0.19.2 | v0.19.2 |
| M6-CD2b-r1 | external-secrets-0.18-0.19--orders-eu | complete | proceed-with-changes | v0.19.2 | v0.19.2 | v0.19.2 |
| M6-CD2b-r1 | traefik-3.6-3.6--retail-apac | complete | modify-target | v3.6.15 | v3.7.16 | v3.7.15 |
| M6-CD2b-r1 | traefik-3.6-3.6--retail-emea | complete | modify-target | v3.6.15 | v3.7.16 | v3.7.15 |

## Human checks

- **M6-B2-r1 external-secrets-0.18-0.19--orders-br**: P4/insufficient-evidence: does the agent name the missing input? Agent mechanism: The v0.19 CRDs exceed the annotation size limit that client-side apply relies on. A sync without ServerSideApply=true fails to install or update the CRDs.
- **M6-B2-r2 external-secrets-0.18-0.19--orders-br**: P4/insufficient-evidence: does the agent name the missing input? Agent mechanism: The v0.19 CRDs exceed the annotation size limit of client-side apply. If the chart is synced with Argo's default client-side apply, the CRD install or upgrade fails or stays out of sync. The new CRDs are not applied, so the new controller runs against old CRDs.
- **M6-B2-r3 external-secrets-0.18-0.19--orders-br**: P4/insufficient-evidence: does the agent name the missing input? Agent mechanism: CRDs in v0.19.x exceed the annotation size limit for client-side apply. Argo CD's default sync would fail to install or update the CRDs unless ServerSideApply=true is set.
- **M6-CD2-r1 external-secrets-0.18-0.19--orders-br**: P4/insufficient-evidence: does the agent name the missing input? Agent mechanism: The v0.19.0 CRDs exceed the annotation size limit for client-side apply, so Argo's default apply fails. The Application that installs the chart must set ServerSideApply=true.
- **M6-CD2-r2 external-secrets-0.18-0.19--orders-br**: P4/insufficient-evidence: does the agent name the missing input? Agent mechanism: Argo client-side apply of the larger CRDs fails because the last-applied-configuration annotation exceeds the size limit. The CRD sync then fails or stays stale, so the controller upgrade may break.
- **M6-CD2b-r1 external-secrets-0.18-0.19--orders-br**: P4/insufficient-evidence: does the agent name the missing input? Agent mechanism: CRDs in v0.19.x are too large for client-side apply (the last-applied annotation exceeds the size limit). A sync without ServerSideApply=true can fail on the CRDs or leave them stale at the old version.
