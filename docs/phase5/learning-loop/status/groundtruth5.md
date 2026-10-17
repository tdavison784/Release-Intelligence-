# Lane groundtruth5 — status

Sealed held-out ground truth (brief: `briefs/groundtruth5.md`). Owns `eval/investigations-heldout/`.
Branch `p5ll/groundtruth5` (sealed until PROTOCOL §6 step 7).

## Done

- Upstream sources read (cert-manager 1.18 release notes + v1.18.0 source; ingress-nginx controller-v1.12.0
  changelog + source; kube-prometheus-stack 90.2.0 UPGRADE.md + charts). Probe: cert-manager v1.17.2 refuses
  the 1.18 feature gate (`eval/investigations-heldout/probes/`), so no case can pre-stage it.
- 10 environments under `eval/investigations-heldout/` (5 cert-manager, 3 ingress-nginx, 2 kube-prometheus-stack),
  shared kind helpers `observed-lib.sh`, canonical render diff `render-diff.py` / `render-case.sh`.
- Observed + expected.yaml + NOTES.md: telemetry-east, telemetry-west, search-latam. Observed: billing-nordics
  (expected.yaml written), docs-us.

## In progress

- Runs: docs-eu, partner-api, obs-emea (chain A); registry-apac, billing-nordics re-run (cosmetic: `ca_fp` of a
  Secret without ca.crt), obs-amer (chain B). Then expected.yaml + NOTES.md for the rest, README, cleanup.
- Fixture fixes made after a first failed run (not outcomes): registry-apac sync waves (Kyverno refuses a policy
  for a kind whose CRD does not exist yet → cert-manager before policies); ingress runs no longer use
  `helm --wait` (a LoadBalancer Service never gets an address in kind); partner-api creates its namespace first.

## Decisions

- Observed procedure for cert-manager: renewals triggered like `cmctl renew` (Issuing condition) stand in for the
  scheduled renewal at renewBefore; both versions exercised (control renewals on v1.17.2 before the upgrade).
- billing-nordics: external-secrets/Vault not reproduced; the Secret they maintain is created directly from a CA
  generated per run (cert-manager only reads that Secret). Documented in the case NOTES.

- The brief says `case.yaml`; FORMAT.md (same layout as `eval/investigations/`) names the inputs file
  `input.yaml`, which is what the harness reads. Cases use `input.yaml`.

## Files touched outside ownership

- `docs/phase5/learning-loop/status/groundtruth5.md` (this file; every lane writes its own).

## Open questions

- none

## Test status

- not run yet
