# Assessment: cert-manager v1.17.2 -> v1.18.0, `privateKey.rotationPolicy` default `Never` -> `Always`

## Verdict

**The default change does not break this environment as declared, but it does change behavior.** `ledger-tls` and `checkout-tls` will start generating a new private key at every renewal. Nothing in the repo pins or depends on those leaf keys, so I found no breakage. The mesh root CA is the one place where rotation would be dangerous, and it is explicitly pinned to `Never` in the prod overlay. That pin should be confirmed on the live cluster before the upgrade.

This verdict comes from static review of the repo, the rendered diff and the upstream notes. Nothing was run against a cluster, so the runtime checks listed at the end are still open.

## What changes in the upgrade

- `rendered/manifest.diff` shows the CRD description for `spec.privateKey.rotationPolicy` going from "Default is `Never`" to "Default is `Always`", with the gate `DefaultPrivateKeyRotationPolicyAlways`.
- `platform/cert-manager/values.yaml` sets no `config.featureGates`, and the controller args in the render (`--v=2`, `--cluster-resource-namespace`, ...) carry no feature-gate flag. The gate is therefore **on by default** here.
- `clusters/prod/cert-manager.yaml` pins `targetRevision: v1.17.2`. The upgrade is a one-line bump there.
- The rendered diff contains only the chart (CRDs, RBAC, Deployments). It does not render the repo's `Certificate` objects, so it cannot show their effective rotation policy. I checked that from the manifests directly.

## Inventory of Certificates and their effective policy

| Certificate | Where | `rotationPolicy` set? | Effective after upgrade |
|---|---|---|---|
| `cert-manager/mesh-root-ca` (isCA, ECDSA 256) | `platform/pki/base/mesh-root-ca.yaml`, patched by `overlays/prod/mesh-root-ca-prod.yaml`. Argo app `pki` deploys `platform/pki/overlays/prod`. | Yes, in the prod patch. The file stores the value redacted. The comment says the digest is of `"Never"`. | **Never** (unchanged) |
| `payments/ledger-tls` | `apps/payments/base/ledger-certificate.yaml` | No | **Always** (changed) |
| `payments/checkout-tls` | `apps/payments/base/checkout-certificate.yaml` | No | **Always** (changed) |

No other Certificate objects exist in `env/`. The `ClusterIssuer`s are `selfsigned-bootstrap` (signs only the root) and `mesh-ca` (`ca.secretName: mesh-root-ca`). Issuers have no rotation policy.

## What is NOT affected, and why

1. **Root CA key stability.**
   - The `mesh-root-ca` prod patch keeps `Never`. When the root renews (1y duration, 720h renewBefore), it re-signs with the same key and the same subject.
   - Every issued leaf, and every `ca.crt` copy in the `ledger-tls` and `checkout-tls` Secrets, therefore stays valid across a root renewal.
   - This matters because `checkout` verifies `ledger` against its own Secret's `ca.crt`, and `ledger` verifies clients against its `ca.crt` (`ledger.conf`: `ssl_client_certificate /tls/ca.crt`, `ssl_verify_client on`). If the root key rotated, a leaf renewed against the new root would no longer verify against the stale `ca.crt` held by the peer, and mTLS would fail. The overlay pin is what prevents that.
   - The pin is also the only reason the root is safe. The base file has no rotationPolicy, so removing or breaking the prod patch would silently turn rotation on for the CA.
2. **Leaf key rotation versus the mTLS trust model.**
   - Trust is by CA signature plus SANs. Nothing pins a leaf public key, hash or SPKI: no pinning in nginx or curl, no key-hash config, and no other consumer of these Secrets in `env/`.
   - A new leaf key signed by the same CA is accepted by the peer.
3. **No immediate reissue on upgrade.**
   - `rotationPolicy` applies only when a re-issuance happens. Existing Secrets are not touched when the controller restarts or the CRD is updated.
   - The first new-key issuance happens at the next renewal: 720h duration, renewBefore 240h, so about 20 days after each cert's last issuance. A manual `cmctl renew` or any spec change that triggers reissue would also do it.
4. **Key parameters.** Both leaves specify ECDSA/256, and the policy only controls whether a new key is generated, so the algorithm and size are unchanged. Other CRD changes in the diff (`signatureAlgorithm`, `revisionHistoryLimit` default 1, short names, ACME `profile`, Vault `serverName`, RBAC split of the challenges ClusterRole) do not touch these resources. They are outside this question.

## What IS affected / residual risk

1. **Leaf key now changes on each renewal (intended upstream behavior).**
   - The Secret's `tls.key` and `tls.crt` are replaced together. Kubelet updates secret volumes atomically via symlink swap, so file-level key/cert mismatch is unlikely.
2. **`ledger` (nginx, 2 replicas in prod) does not hot-reload.**
   - It reads `/tls/tls.crt` and `/tls/tls.key` at startup and has no reload sidecar or reloader annotation.
   - After a renewal, running workers keep serving the old cert and key pair. That is self-consistent and still valid until the old cert expires (30 days). This is **pre-existing** and not caused by the policy change: with `Never`, the cert would still have been stale in memory.
   - What differs: a restart, scale-up or worker reload after a renewal picks up the new key and cert together, which is fine. But the stale pair can no longer be assumed to share a key with the on-disk files, so any ad hoc process that mixes old in-memory state with new files has no shared key to fall back on.
   - The real hazard, unchanged by this upgrade, is that the cert expires in nginx memory if no pod restarts in 30 days. Worth fixing separately, for example with a reloader or a restart on Secret change.
3. **`checkout` (curl loop)** re-reads the files on each iteration, so it picks up new material within about 30s of the Secret update. There is a small theoretical window where `tls.crt` and `tls.key` are read across a swap and mismatch, giving one failed call that heals on the next iteration. Today this is rare because the key never changed, so cert and key can't be mismatched. After the upgrade the mismatch becomes possible (negligible, and self-healing).
4. **Secrets or tooling outside this repo** (backup, Vault sync, external allowlists or pinned keys, monitoring of key fingerprints) are not visible here. ORIENTATION says the repo is the complete declaration, so I found none, but I can't prove their absence.
5. **Timing of the next renewal against the 1.19 deadline.** In 1.19 the gate goes GA and cannot be disabled. If anyone actually needs `Never` on the leaves, they must set it explicitly before then. Nothing in this repo indicates that need.

## Recommended actions

- **Make the policy explicit now rather than relying on defaults:**
  - Add `rotationPolicy: Never` to the root CA in the *base* `mesh-root-ca.yaml` as well as the overlay, so the safety property doesn't depend on one patch.
  - Add an explicit `rotationPolicy: Always` to `ledger-tls` and `checkout-tls` if key rotation is wanted. That is the recommended direction and is independent of the chart version.
- Do **not** set `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false`. It would only delay the change to 1.19.
- Treat the root CA as the only item that must stay `Never` unless a deliberate CA rotation with trust distribution is planned.

## Missing information / tests before proceeding

1. **Live check of the root CA.**
   - `kubectl get certificate -n cert-manager mesh-root-ca -o jsonpath='{.spec.privateKey.rotationPolicy}'` must print `Never`.
   - The repo stores that value redacted. I took it as `Never` from the in-file comment, which is not a direct read of the value.
   - Also render `kustomize build platform/pki/overlays/prod` and confirm the merged object keeps `isCA`, `commonName`, `secretName` and `rotationPolicy: Never` together.
2. **Live check that the gate is not overridden** in the running controller args (confirm no `--feature-gates` flags that differ from the values file).
3. **Staging rehearsal on v1.18.0:**
   - Run `cmctl renew ledger-tls checkout-tls -n payments`.
   - Confirm the key fingerprint changes (`openssl pkey -pubout`) while the `ca.crt` is the same.
   - Confirm that `checkout` to `ledger` still succeeds.
   - Confirm that `ledger` pods behave as expected after the cert changes, both with no restart (still serving the old pair) and after a rollout.
4. **Root renewal rehearsal:** force-renew `mesh-root-ca` in staging with `Never`, and confirm the public key is identical and leaves/peers still verify.
5. **Inventory outside the repo:** any other consumers of the `ledger-tls` and `checkout-tls` Secrets, any key-pinning, and any monitors keyed on key fingerprints or alerting on Secret changes.
6. **Renewal schedule:** note when each leaf was last issued so the first key rotation can be observed (about day 20 of each cycle), not discovered during an incident.
