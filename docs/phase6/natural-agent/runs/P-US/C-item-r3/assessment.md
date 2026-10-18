# Assessment: cert-manager v1.17.2 -> v1.18.0, default `privateKey.rotationPolicy` becomes `Always`

## Verdict

**The change does not break the mesh root CA, and it does not break the upgrade itself. It does change runtime behaviour for the two leaf certificates (`ledger-tls`, `checkout-tls`), and that exposes some latent risks in how the workloads consume them.** I rate it "safe to proceed after the checks below", not "no impact".

## What is and is not affected

### Not affected: `cert-manager/mesh-root-ca` (the trust anchor)
- The base `platform/pki/base/mesh-root-ca.yaml` sets no `rotationPolicy`.
- The prod overlay `platform/pki/overlays/prod/mesh-root-ca-prod.yaml:14-15` patches in `rotationPolicy: Never`.
- Argo app `clusters/prod/pki.yaml` points at `platform/pki/overlays/prod`, so the overlay is what is deployed.
- The rendered output confirms this: `rendered/from.yaml` and `rendered/to.yaml` contain an identical `mesh-root-ca` with `rotationPolicy: Never`. The pki render is marked "identical in from and to".
- This matters because the leaf `ca.crt` values and ledger's `ssl_client_certificate` trust this CA. Silent rotation of the CA key would have been the dangerous case, and it is explicitly pinned.
- Caveat: `Never` only protects the key. The root renews on its own schedule (`duration` 8760h, `renewBefore` 720h) with the same key, and that is unchanged by this upgrade.

### Affected: `payments/ledger-tls` and `payments/checkout-tls`
- Neither `apps/payments/base/ledger-certificate.yaml` nor `checkout-certificate.yaml` sets `rotationPolicy`. The prod overlay only patches ledger replicas (now 2).
- On 1.17.2 these keep their private key on renewal. On 1.18.0 each renewal generates a new ECDSA P-256 key.
- Timing:
  - Nothing happens at upgrade time. The change is a controller-side default. The CRD schema diff only changes description text and sets no CRD default, so Argo sees no drift on the Certificate objects.
  - A new key appears only at the next issuance, which is the first renewal after the upgrade. With `duration` 720h and `renewBefore` 240h, that is about day 20 of the current cert's life.
  - A manual `cmctl renew`, or deleting the Secret, would also trigger it.
- The issuer is `ClusterIssuer mesh-ca`, a CA issuer signing from the root Secret. It does not depend on leaf key continuity, and neither do `algorithm` or `size`. Issuance will succeed.

### Not affected: the cert-manager install values
- `platform/cert-manager/values.yaml` sets no `config.featureGates`, so the gate `DefaultPrivateKeyRotationPolicyAlways` is on by default in 1.18.0.
- The rendered controller args show no feature-gate flags in either version.
- The other rendered differences (RBAC split of challenge roles, new CRD fields, webhook probe port name) are out of scope for this change. They have no obvious link to the mesh, which uses no ACME.

## Consumers: where key rotation can actually bite

1. **checkout** (`checkout-deployment.yaml`)
   - It mounts the Secret as a volume and runs `curl --cert /tls/tls.crt --key /tls/tls.key` every 30s, re-reading the files each time. It will pick up new credentials with no restart.
   - New risk: kubelet updates volume files atomically through a `..data` symlink swap. A single curl invocation opens the cert and key as two separate reads. If the swap lands between them, curl gets a mismatched pair and one 30s iteration fails ("ledger call failed", no crash). Under `Never` the key was unchanged, so a cert/key race was harmless. This is a small window and self-healing.

2. **ledger** (`ledger-deployment.yaml`, `ledger.conf`, 2 replicas in prod)
   - nginx reads `/tls/tls.crt` and `/tls/tls.key` only at startup/reload. There is no reloader, sidecar or `nginx -s reload` anywhere in the repo (grep for reload/reloader/stakater found nothing).
   - So nginx never picks up a renewed certificate, with or without this change. After the old cert expires (720h), ledger would serve an expired cert whether the key rotated or not. This is a pre-existing latent bug, but the rotation default makes it more visible.
   - Under `Always` there is a further hazard if a pod restarts mid-rotation. Because cert and key are swapped atomically, that is safe in the normal case. The possible failure mode is an nginx reload or restart racing the symlink swap, which could read a new cert with an old key and refuse to start. Probability is low.
   - Impact is bounded: peers verify chains against the CA (`ca.crt`), not pinned leaf keys, so a key change alone does not break trust.

3. **No pinning found.** Nothing in `env/` pins the leaf public key, SPKI hash or fingerprint (no HPKP-like config, no allow-lists, no other consumers of these Secrets). Within the repo, key rotation is therefore semantically harmless.

## Evidence summary
- Files: `platform/cert-manager/values.yaml`, `platform/pki/{base,overlays/prod}/*`, `apps/payments/base/*`, `apps/payments/overlays/prod/*`, `clusters/prod/*`.
- `rendered/manifest.diff`: CRD description for `rotationPolicy` changes to "Default is `Always`"; the diff contains no changed Certificate resources.
- Not independently verified: I did not open the `upstream/` documents in detail beyond the change text supplied in the request. I rely on that text for the feature-gate name and the 1.19 GA statement.

## Missing information / tests before proceeding
1. **Live-cluster state** (the repo is declared complete but not runtime-verified):
   - `kubectl get certificate -A -o yaml` to confirm there are no other Certificates (for example created by ingress-shim or other teams) that rely on the old default.
   - Confirm the live `mesh-root-ca` has `rotationPolicy: Never`, not just the repo.
2. **Staging rehearsal:** upgrade, then force a renewal (`cmctl renew ledger-tls checkout-tls -n payments`). Verify:
   - the Secret gets a new key (compare `tls.key` public key before and after);
   - checkout can reach ledger on the next loop iterations;
   - ledger behaviour: does nginx keep serving (old cert) or fail? Run `nginx -s reload` and confirm it succeeds.
3. **Ledger reload story:** decide how nginx picks up renewed certs (reloader annotation, sidecar, or rolling restart on renewal). Fix this regardless of the upgrade, ideally before the first post-upgrade renewal at about day 20.
4. **Renewal-time observation:** watch the first real renewals in prod, and alert on handshake failures from ledger to checkout.
5. **Other cert-manager users in the cluster:** confirm that no other Argo apps or operators create Certificates with default policy and depend on key continuity (for example for key pinning or HSM/escrow workflows).

## Recommendation
- Treat the default flip as acceptable. Rotating leaf keys on renewal is the intended and more secure behaviour for the mesh, and the root CA is already protected by `Never`.
- Optionally make intent explicit by setting `privateKey.rotationPolicy: Always` on the two leaf Certificates in the base. This pins the behaviour independently of the gate, which is removed in 1.19.
- If you want zero behaviour change on this upgrade, set `rotationPolicy: Never` explicitly on the leaf certs instead of using the feature gate. The gate disappears in 1.19, and the root's explicit `Never` is already correct for both versions.
- Do not rely on `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false`, as it only defers the decision.
- Fix the ledger reload gap as a separate prerequisite.
