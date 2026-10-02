# Upgrade plan — cert-manager v1.17.0 → v1.18.0, customer-repo environment (cluster k8s 1.28)

*Produced by the TRADITIONAL arm of the time-study pilot: sources read by
hand, no `ri` output consulted. Sources: upstream release notes (v1.18.0),
upgrade guide 1.17→1.18, supported-releases table, chart values diff
v1.17.0↔v1.18.0, CRD diff v1.17.0↔v1.18.0, and the environment files under
`docs/phase3/review-packet/example-env/customer-repo/`.*

## 0. Compatibility gate

- Supported-releases table: v1.18.0 supports **k8s 1.29–1.33**. Our cluster is
  **1.28 → outside the tested range**.
- Note: v1.17.0 (what we run today) lists the **same** 1.29–1.33 range, so we
  are already unsupported on 1.17.0; this upgrade does not regress us, but the
  cluster upgrade to ≥1.29 should be scheduled regardless.
- Chart `kubeVersion` is `>= 1.22.0-0`, so Helm will not block on 1.28 — this
  is a support-policy line, not a hard blocker. Decision: proceed after
  cluster upgrade, or accept documented risk.

## 1. Breaking changes that touch THIS environment

- **`Certificate.Spec.PrivateKey.RotationPolicy` default `Never`→`Always`.**
  Our only Certificate (`base/certificate.yaml`, example-com) sets **no**
  `rotationPolicy` → after upgrade, renewals regenerate the private key.
  Action: decide; if key stability matters, pin `rotationPolicy: Never` on the
  Certificate before upgrading (feature-gated; gate disappears in 1.19).
- **`Certificate.Spec.RevisionHistoryLimit` default `nil`→`1`.** Same
  Certificate sets no value → on upgrade, stale CertificateRequests are
  garbage-collected. Action: none if that is acceptable (it is), otherwise pin
  a higher value; alert anyone parsing CertificateRequest history.
- **ACME HTTP01 challenge `pathType` → `Exact`.** The Certificate's issuer is
  `letsencrypt` (ACME) → HTTP01 challenges likely route through an Ingress.
  This repo shows no ingress-nginx; namespace `istio-system` suggests Istio
  ingressgateway serves the challenge. Action: identify the ingress
  controller that terminates `/.well-known/acme-challenge/` and verify it
  handles `pathType: Exact`; if ingress-nginx ≥1.12 with
  `strict-validate-path-type` default, apply one of the three documented
  workarounds (disable feature gate, disable strict validation, or upgrade
  ingress-nginx ≥1.12.6/1.13.2).

## 2. Helm values (chart diff)

- `prometheus.servicemonitor.targetPort` default **changed 9402 → "http-metrics"**.
  Prod values pin `9402` → no behavioral change for us; migrate the pin to the
  port name at leisure.
- **New key `global.rbac.disableHTTPChallengesRole`** (default false). Prod
  values set it to **true** — previously ignored, now live: cert-manager will
  drop its permission to create ACME solver pods. **If our issuer uses HTTP01,
  this breaks challenges.** Action: reconcile this value with §1 — most likely
  remove it or set false; it may have been cargo-culted while the key was
  inert.
- Feature gates flipped to GA in the chart docs (`UseDomainQualifiedFinalizer`
  now default true; `AdditionalCertificateOutputFormats` always on;
  `ValidateCAA` removed — we set none of these; no action).

## 3. Images / mirrors

Controller + webhook images pinned `v1.17.0` in: `base/deployment.yaml`,
`kustomize/kustomization.yaml`, `.github/workflows/mirror.yaml`,
`terraform/main.tf` (webhook) → bump all pins to `v1.18.0` and refresh the
mirror. (acmesolver/cainjector not referenced — they ride in the chart.)

## 4. Install-path sanity

Three tools declare cert-manager: helmfile (`version: v1.17.0`), Argo CD
(`targetRevision: v1.17.0`), Flux (`version: "1.*"` — a **range**). Action:
confirm which path is authoritative; the Flux range can roll forward without
a human decision. Vendored chart under `charts/vendored/` is not an install
source.

## 5. CRDs

`cert-manager.crds.yaml` diff v1.17.0→v1.18.0 is additive (ACME `profile`/
`serverName` fields, `iss`/`ciss` short names). Action: apply the v1.18.0
CRDs as part of the upgrade (standard `kubectl apply` step); no data migration.

## 6. Security

Five dependency bumps ship in the images (go-jose CVE-2025-27144, x/oauth2
CVE-2025-22868, x/crypto GHSA-hcg3-q754-cr77, golang-jwt GHSA-mh63-6h87-95cp,
x/net CVE-2025-22870). Action: upgrade covers them; note for scanner exceptions.

## 7. Observability

New metrics `certmanager_certificate_not_before/not_after_timestamp_seconds`;
renewal-timestamp metric help-text fix. Action: add to dashboards if wanted;
check the servicemonitor port-name change against our `targetPort: 9402` pin.

## Rollout order

1. (Recommended first) cluster upgrade to ≥1.29, or sign off on unsupported combo.
2. Reconcile `global.rbac.disableHTTPChallengesRole` with the HTTP01 decision from §1/§2.
3. Pin (or accept) the two Certificate default flips on `example-com`.
4. Apply v1.18.0 CRDs; upgrade the authoritative install path; resolve the Flux range.
5. Bump image pins + mirror; redeploy; watch ACME challenge solves and renewals.
