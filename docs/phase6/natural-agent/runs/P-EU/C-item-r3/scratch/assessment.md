# Assessment: cert-manager v1.17.2 -> v1.18.0, `rotationPolicy` default now `Always`

## Verdict
**Yes, it affects this environment. It does not break anything at upgrade time. It creates a delayed outage risk in the payments mTLS mesh, triggered by the root CA's first renewal after the upgrade.** Do not upgrade as-is without a mitigation (see Recommendation).

## Evidence
- No Certificate in `env/` sets `privateKey.rotationPolicy`. The Certificates are `ledger-tls`, `checkout-tls`, and `mesh-root-ca` (base and prod overlay). All of them inherit the default, which moves from `Never` to `Always`.
- `env/platform/cert-manager/values.yaml` has no `config.featureGates`, so the `DefaultPrivateKeyRotationPolicyAlways` gate stays at its default (on) in 1.18.0.
- `rendered/manifest.diff` shows only the CRD description change (`Default is Always ... can be disabled via --feature-gates=DefaultPrivateKeyRotationPolicyAlways=false`). It shows no change to any Deployment arguments. The rendered diff therefore cannot show this change, because the behavior change is in controller code. An unchanged manifest does not mean unchanged runtime behavior.
- The Argo CD app `clusters/prod/cert-manager.yaml` pins `targetRevision: v1.17.2`, with automated sync and selfHeal.

## What is affected
1. **Leaf certs `ledger-tls` and `checkout-tls`** (duration 720h, renewBefore 240h, so they renew about every 20 days). Each renewal now generates a new ECDSA P-256 key. Before, the key was reused. These secrets are consumed by `ledger` (nginx, `/tls/tls.crt`, `tls.key`, `ca.crt`) and `checkout` (curl, `/tls/*`).
   - Consumers read the key and certificate from the same Secret volume. The Secret is updated in one write, and the kubelet swaps the mount atomically, so a key/cert mismatch is unlikely.
   - nginx loads certificates only at start or reload. Today a renewal already changes the certificate, so this is not new. The new key doesn't add a failure mode.
   - Nothing pins the public key or key hash. I found no such pins in `env/`.
2. **Root CA `mesh-root-ca`** (isCA, duration 8760h in prod, renewBefore 720h, self-signed by `selfsigned-bootstrap`). This is the main risk.
   - On its first renewal under 1.18, it gets a **new key pair and a new root certificate**. Under 1.17 it kept the same key.
   - The `mesh-ca` ClusterIssuer reads `mesh-root-ca` and starts signing with the new key. The CA Secret's `ca.crt` changes.
   - `ledger.conf` sets `ssl_client_certificate /tls/ca.crt` and `ssl_verify_client on`. `checkout` runs `curl --cacert /tls/ca.crt`. Each side trusts only the `ca.crt` in its own leaf Secret, which is a copy of the CA at that service's last issuance.
   - Nothing reissues leaf certificates when the CA changes. `ledger-tls` and `checkout-tls` renew on independent schedules, driven by their own issuance times.
   - **Failure window:** after the root rotates, any service that has renewed holds a certificate signed by the new CA and trusts only the new root. The service that hasn't renewed still holds a certificate signed by the old CA and trusts only the old root. Both directions of the handshake fail, so ledger and checkout can't talk until both have renewed. A window of up to roughly 20 days is possible, and it ends only when the second leaf renews.
   - This is not new CA risk in kind, because any CA key rotation does this. What changes is that the rotation previously never happened automatically. Now it is a scheduled event with no overlap or bundle handling in this design.
   - The first root renewal falls at about 8040h (~335 days) after the current root was issued. Its issuance date is not in the repo.

## What is not affected
- The upgrade itself. No key is rotated by the upgrade or by Argo syncing the new chart. The Certificate specs are unchanged, so nothing triggers reissuance by itself, and the new default only applies when an issuance occurs.
- The `ClusterIssuer` (`mesh-ca`, `selfsigned-bootstrap`) has no key-rotation setting and is unaffected directly.
- Key algorithm and size are explicitly set (ECDSA/256), so the key type does not change. Issuance, SANs, usages, and durations are unchanged.
- The other 1.18.0 diffs I saw in `rendered/manifest.diff` are out of scope here. Note the CRD change: `revisionHistoryLimit` now defaults to 1, which is separate from this change.

## Missing information and tests before proceeding
- Issuance date of the live `mesh-root-ca` and of the two leaf certs (`kubectl get certificate -o wide`, `status.notBefore`, `renewalTime`). These determine when the first rotation window occurs.
- Whether any other consumer of `ca.crt` or the CA exists outside this repo (the repo is stated to be complete). Also whether anything pins keys, such as a monitoring probe or a trust store.
- Whether nginx and curl pick up a changed Secret without a restart. nginx needs a reload (no reloader exists in this repo), so confirm in staging. This is an existing risk that rotation makes more visible.
- Staging test: install 1.18.0, run `cmctl renew` on `mesh-root-ca`, and check that ledger and checkout fail mTLS until both leaf certs are reissued. Then confirm that `cmctl renew` on a leaf certificate changes its key.
- Confirm the Argo CD cert-manager Application controls controller args, so the feature-gate change takes effect as a Helm value.

## Recommendation
Pick one of these before upgrading:
1. **Preserve current behavior (minimum change).** Set `privateKey.rotationPolicy: Never` explicitly on all three Certificates. You may also set `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false` in `values.yaml` as a stopgap. That gate disappears in 1.19, so the explicit field is the durable fix.
2. **Adopt rotation deliberately.** Keep `Always` on the leaf certificates. Keep `Never` on `mesh-root-ca` until a CA-rotation plan exists (trust bundle with old and new roots, then staged leaf reissuance). Then flip it in a planned change.

Option 2 is my suggested path. Rotating leaf keys is the stated security goal of the change, and the root CA is the only certificate where the default can cause an outage.
