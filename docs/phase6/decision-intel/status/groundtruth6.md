# Lane `groundtruth6` status (sealed: no per-case labels in this file)

Brief: `briefs/groundtruth6.md` (D-0032). Branch `p6di/groundtruth6`, never pushed or merged by the lane.
Output: `eval/investigations-blind6/` (README.md is the label-free index).

## State: done

## Pairs (products and versions only; labels live in the cases' expected.yaml / NOTES.md)

| Pair | Cases | Product | From → to | Coverage items |
|---|---|---|---|---|
| traefik-3.6-3.6--shared-middlewares | --retail-emea, --retail-apac | traefik | v3.6.10 → v3.6.15 (chart 39.0.5 → 39.0.9) | 1, 4, 5, 6 |
| cert-manager-1.20-1.21--vault-pki | --treasury-de, --treasury-us | cert-manager | v1.20.4 → v1.21.3 | 2 |
| external-secrets-0.18-0.19--crd-delivery | --orders-eu, --orders-br | external-secrets | v0.18.2 → v0.19.2 | 1, 3 |
| argo-cd-2.14-3.0--developer-access | --studio-east, --studio-west | argo-cd | v2.14.11 → v3.0.12 (chart 7.9.1 → 8.2.7) | 1, 4 |

Decisive items, class counts only: 4 affected, 3 not-affected, 1 insufficient-evidence. Verification: 7
observed, 1 observed-alternatives (both alternatives observed); 0 upstream-only.

## Done

- Read FLEET.md (phase 6 + phase 5), `_common.md`, the brief, PLAN-M6, `eval/investigations/FORMAT.md` and README,
  existing inputs for format, the held-out helper scripts, `products/*.yaml`. Not read (brief): MISSION.md,
  PRODUCT-DECISIONS.md, any label file, other lanes' status or branches, any stored results.
- RI was never run on these cases.
- Four pairs authored, observed in kind, committed with input.yaml, env/, expected.yaml, NOTES.md, observed/.
- Shared ground-truth tooling: `observed-lib.sh` (from the held-out one, plus helpers), `render-diff.py` (copy),
  `sanitize-log.sh` (replaces machine paths in logs).

## Decisions

- One pair carries the incomplete-input environment (coverage 3), so the decisive classes are 4/3/1, not 4/4.
- The version/advisory decision (coverage 6) is recorded in NOTES.md and as an additive `versionDecision` block in
  expected.yaml; `apparentMitigation`, `decisive`, `coverage` are also additive (README "Additive fields").
- cert-manager 1.21 supports Kubernetes 1.33–1.36 only: that pair ran with kind v0.33.0 (downloaded into the
  session scratchpad, checksum verified, deleted afterwards) and Kubernetes v1.34.11. All others: kind v0.24.0,
  Kubernetes v1.31.0.
- Stand-ins used in observations, each disclosed in the case's NOTES.md: a Vault dev server for an external
  Vault; inlined Helm values instead of Argo CD `$values` multi-source (no Git server in kind); a local Argo CD
  account in an SSO group; a public chart as the source of an Argo CD Application; fake-provider probe objects.

## Kind time

About 51 minutes of cluster runtime in 16 runs (including 3 diagnostic reruns of the cert-manager pair, one of
the External Secrets pair and 2 Traefik reruns for a cleaner log and the safer-target phase): Traefik ~9.5 min (5 runs), cert-manager ~16 min (5), External Secrets ~21 min (4),
Argo CD ~5 min (2).

## Files touched outside ownership

None (only `eval/investigations-blind6/` and this status file).

## Open questions

None. Upstream data is not recorded (brief); the commander records it through RI's online mode.
