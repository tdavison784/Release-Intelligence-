# Assessment: cert-manager v1.17.2 -> v1.18.0, `rotationPolicy` default `Never` -> `Always`

## Verdict

**The change affects this environment, and it can break payments mTLS. It does not break anything at upgrade time. The risk is deferred to each Certificate's next re-issuance.** The worst case is the mesh root CA renewal.

The change is not neutral for this environment, and it can be neutralised cheaply before upgrading (see Recommendation).

## Evidence

- None of the three Certificates sets `spec.privateKey.rotationPolicy`:
  - `env/platform/pki/base/mesh-root-ca.yaml` (patched by `overlays/prod/mesh-root-ca-prod.yaml`, which sets only duration, renewBefore and subject)
  - `env/apps/payments/base/ledger-certificate.yaml`
  - `env/apps/payments/base/checkout-certificate.yaml`
  
  Each sets only `algorithm` (ECDSA) and `size`. So all three inherit the default, which flips to `Always`.
- `env/platform/cert-manager/values.yaml` has no `config.featureGates`, so `DefaultPrivateKeyRotationPolicyAlways` stays at its default (enabled in 1.18). A grep for `rotationPolicy` and `featureGates` outside `upstream/` and `rendered/` finds nothing.
- `rendered/manifest.diff` confirms the CRD description change to "Default is `Always`". Its only controller changes are image tags and the acmesolver image arg. No feature-gate arg is rendered, so nothing preserves the old behaviour.
- The CRD schema has no server-side default for the field. The default is applied in controller logic, so existing Certificate objects are not mutated and no manifest diff appears for them. An unchanged manifest therefore hides a runtime behaviour change.

## What happens at upgrade

- Upgrading by itself does not re-issue anything. The policy only applies when an issuance occurs.
- Current keys remain in place until each Certificate is next renewed, manually re-issued (e.g. `cmctl renew`), or has its spec changed in a way that triggers re-issuance.
- Renewal times from the specs:
  - Leaf certificates (ledger, checkout): duration 720h, renewBefore 240h, so renewal about 20 days after each issuance.
  - Root CA: prod duration 8760h, renewBefore 720h, so renewal about 11 months after issuance.
- After the upgrade, each of these renewals produces a **new private key** where it previously reused the old one.

## What is affected and why

1. **Leaf certificates (ledger-tls, checkout-tls). Low to moderate risk.**
   - Every renewal now rotates the key. That is the intended hardening.
   - Consumers:
     - ledger (nginx) loads `/tls/tls.crt` and `tls.key` from a Secret volume. It reads them only at start or reload, and no reloader is declared in the repo.
     - checkout (curl loop) rereads its files on every call.
   - Kubelet updates the Secret volume atomically, so a cert/key mismatch within one pod is unlikely.
   - nginx keeps serving its old pair until restarted. That was already true with `Never`, since the certificate changes on renewal either way. A new key only changes the effect if an old key were revoked or distrusted.
   - ECDSA P-256 key generation is cheap, so there is no performance concern.

2. **Root CA (`cert-manager/mesh-root-ca`). Highest risk.**
   - This is the CA behind the `mesh-ca` ClusterIssuer. At its next renewal (about 11 months after its last issuance) it now gets a new key, which means a new CA identity. Previously it was the same key and subject with a new validity period, so verifiers that trusted the old certificate still validated chains.
   - Both services trust only the `ca.crt` in their own Secret. This is stated in the checkout comment and in `ledger.conf` (`ssl_client_certificate /tls/ca.crt`, `ssl_verify_client on`).
   - After a CA key rotation, a leaf certificate issued after the rotation carries the new CA in `ca.crt` and is signed by the new key. A peer still holding a leaf Secret issued before the rotation has the old CA in `ca.crt` and trusts only the old key.
   - ledger and checkout renew independently, with no coordination. Between the two renewals, one side presents a certificate the other does not trust.
   - The result is mTLS failures: either checkout fails to verify ledger, or ledger rejects checkout. checkout would log "ledger call failed". The window can be as long as the gap between the two leaf renewals, up to about 20 days if they are offset, until both have been issued from the new CA.
   - Leaves issued under the old CA are not automatically re-issued when the CA changes. cert-manager does not trigger leaf re-issuance on issuer CA change.
   - A single-CA bundle with no overlap or trust-bundle mechanism means there is no graceful rotation path.
   - Whether this failure already existed is unclear. With `Never`, the root CA key stayed the same, so chains stayed valid across renewals. This change newly introduces the failure mode.
   - Date and cadence matter: the root CA renewal is far enough away that it could surprise someone months after the upgrade.

3. **Not affected**
   - The ClusterIssuers (`mesh-ca`, `selfsigned-bootstrap`) are not Certificates and have no rotationPolicy.
   - The cert-manager install itself (CRDs `crds.keep: true`, single replicas, no prometheus) is unaffected by this change.
   - The algorithm and size settings are unaffected. `Always` regenerates keys with the same specified requirements.
   - No other Certificate resources exist in the repository (I listed all manifest files under `env/`). The repo is stated to be the complete declaration of what runs, so there are no hand-applied certificates.

## Recommendation

Before upgrading, add `privateKey.rotationPolicy: Never` explicitly to all three Certificates (at minimum the root CA). This keeps today's behaviour exactly and is the documented option 1. Optionally also set `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false` as a belt-and-braces measure. Note that this gate disappears in 1.19 (it goes GA and cannot be turned off), so per-resource `Never` is the durable choice.

Then, as a separate change, plan deliberate rotation:
- Leaf certificates can move to `Always` once ledger and checkout reload their certificates reliably.
- For the root CA, design a rotation procedure first, for example trusting both old and new CA during an overlap, or trust-manager, or a coordinated re-issue of ledger and checkout right after the CA rotates.

## Missing information and tests before proceeding

- Actual live state: Certificate `status.revision`, `notBefore`, `renewalTime` and Secret contents in the prod cluster. Is the CA near renewal, and when do ledger and checkout renew? The repo does not show live status.
- Whether anything else consumes the `mesh-root-ca` Secret or the leaf Secrets (external clients, pinned keys, trust stores, SPKI pinning). Nothing in the repo does, but the repo is the stated source of truth, so confirm.
- Whether ledger gets reloaded when its Secret changes (reloader sidecar, operator). None is declared in `ledger-deployment.yaml`.
- The upstream default's exact behaviour on existing Secrets, for example when the Secret's existing key doesn't match the requested algorithm or size. I did not find this spelled out in the files I read.
- Tests, in a non-prod cluster mirroring this setup:
  1. Install 1.18.0 without the explicit `Never` and run `cmctl renew` on mesh-root-ca. Confirm the key changes and observe the mTLS failure between ledger and checkout.
  2. Repeat with `rotationPolicy: Never` set. Confirm the key is unchanged and mTLS keeps working.
  3. Run `cmctl renew` on a leaf and confirm ledger and checkout behave across the Secret update, including whether nginx needs a reload.
  4. Confirm that applying the `Never` change through Argo CD produces no re-issuance (the spec change should not trigger one).
- Other 1.18 changes seen in the rendered diff but out of scope here: `revisionHistoryLimit` default now `1`, a new `signatureAlgorithm` field, and a `ciss` shortName. These should be reviewed separately.
