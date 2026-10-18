# cert-manager v1.17.2 -> v1.18.0: `rotationPolicy` default flips to `Always`

## Verdict

**The upgrade does not break the environment at deploy time, and the one Certificate where a key change would be dangerous (the root CA) is protected. The default change does alter runtime behaviour for two certificates (`ledger-tls`, `checkout-tls`). From the next renewal on, each renewal generates a new private key. I found nothing that depends on those keys staying stable, so I rate this low risk, but a few things need confirming in the live cluster first (see the last section).**

## Evidence

### Controller configuration
- `env/platform/cert-manager/values.yaml` has no `config.featureGates` and no controller `extraArgs`. The chart is not told to keep the old behaviour.
- In `rendered/to.yaml` the controller args are only `--v=2`, `--cluster-resource-namespace`, `--leader-election-namespace`, `--acme-http01-solver-image` and `--max-concurrent-challenges`. So `DefaultPrivateKeyRotationPolicyAlways` is on by default, as the upstream notes say.
- `rendered/manifest.diff` shows the only CRD change to this field is a description edit: the default text goes from `Never` to `Always`. The enum and the schema are unchanged. The default is applied in the controller, not by the API server, so existing Certificate objects are not mutated.
- `clusters/prod/cert-manager.yaml` pins `targetRevision: v1.17.2`. This is the only place the version is set, and it is where the bump happens.

### Certificates that set `rotationPolicy` explicitly
| Certificate | rotationPolicy | Effect |
|---|---|---|
| `cert-manager/mesh-root-ca` (prod) | `Never`, set by the patch `platform/pki/overlays/prod/mesh-root-ca-prod.yaml` | **Unaffected.** The behaviour is the same before and after. |

- The base `platform/pki/base/mesh-root-ca.yaml` sets no policy. Only the prod overlay pins `Never`.
- The `rendered/*.yaml` files for both versions (`from.yaml:13906`, `to.yaml:14034`) show the final merged object with `rotationPolicy: Never` in both. So the kustomize patch does merge, and the value is identical before and after.
- This matters because the root CA signs everything through `ClusterIssuer/mesh-ca`, which reads Secret `mesh-root-ca`. Each leaf Secret's `ca.crt` is a copy of the root cert, taken when the leaf was issued, and ledger's nginx trusts that file (`ledger.conf`).
- If the root had defaulted to `Always`, its next renewal (at about 8040h of its 8760h life) would have issued a new root key. Any leaf not yet reissued would then chain to the old root key, and the mTLS between `checkout` and `ledger` would break. The explicit `Never` prevents that.
- Any other environment that builds from `pki/base` without that overlay would not have this protection.

### Certificates that rely on the default (affected)
- `ledger-tls` (`apps/payments/base/ledger-certificate.yaml`) and `checkout-tls` (`checkout-certificate.yaml`) have a `privateKey` block with `algorithm` and `size` only, and no `rotationPolicy`.
- Before: the key was reused on every renewal. After: a new ECDSA P-256 key is generated on every reissue.
- Nothing is reissued by the upgrade itself. The policy is only read when an issuance happens. Both certificates have `duration: 720h` and `renewBefore: 240h`, so they renew roughly every 20 days. The first key rotation will happen at each one's next renewal after the controller upgrade, or at any manual renewal.
- Anything that triggers a reissue earlier will also rotate the key. Examples: editing the spec, deleting the Secret, `cmctl renew`.

### Consumers of the leaf keys
- **ledger** (`ledger-deployment.yaml`, `ledger.conf`): nginx mounts Secret `ledger-tls` and reads `/tls/tls.crt` and `/tls/tls.key`. It checks client certificates against `ca.crt` only. It does not pin the key or certificate of either peer.
- **checkout** (`checkout-deployment.yaml`): a curl loop every 30s, presenting `tls.crt` and `tls.key` and verifying ledger against its own `ca.crt`. Nothing pins ledger's key.
- I found no other consumers in the repo: no key hashes, no allow-lists, no SPKI pins, no external copies of these Secrets. Checked by grep and by reading every file under `env/`.

Expected behaviour after a rotation:
1. The kubelet updates the mounted Secret volume atomically, so `tls.crt` and `tls.key` change together.
2. **ledger:** nginx loads the cert and key into memory at start or reload. It keeps serving the old pair until something restarts it. This is not caused by the new default (see risk 1 below). When it does restart, it reads a matching new pair.
3. **checkout:** curl re-reads the files on every call. It could in theory open `tls.crt` and `tls.key` on opposite sides of the swap and get a mismatched pair. That would cost at most one failed call ("ledger call failed") per rotation. Before 1.18 this could not happen, because the key was constant.
4. Because the signing CA is unchanged, the new leaves chain to the same root and are accepted by the peer.

## Risks, ranked
1. **Pre-existing and independent of this change:** nginx does not reload on Secret changes, and no reloader (Reloader, a sidecar, or a checksum annotation) is present in the repo. Renewed `ledger-tls` certificates are not picked up until a pod restart. With a 720h certificate and no reload, ledger will serve an expired certificate if the pod is never restarted. Key rotation does not make this worse, but it is the thing most likely to cause a real incident around certificate renewal, so check it while you are looking at renewals.
2. Brief single-call mismatch window for checkout during each rotation (see above). Self-heals within 30s.
3. If anything outside this repo is not shown (see unknowns), it could be pinned to the old keys.
4. Longer term: the feature gate goes away in 1.19, so the choice has to be made explicitly before then.

## What is not affected
- The root CA `mesh-root-ca`, which keeps `Never` through the prod overlay.
- The `mesh-ca` ClusterIssuer, which is a CA issuer reading the root Secret. It does not use `rotationPolicy`.
- `selfsigned-bootstrap` (not reviewed in depth) is an issuer, and issuers have no `rotationPolicy`.
- The Argo CD applications, the Helm values, and the CRD schema compatibility. The CRD diff is additive and cosmetic for this field.
- Nothing else in the rendered diff touches rotation. I did not assess the other 1.18 changes beyond what I saw in the diff. Those are the RBAC split for challenges, the webhook probe port name, and the new `signatureAlgorithm`, `profile` and `serverName` fields. None of them looks relevant to this environment, but they were outside the question asked.

## Recommendation
Explicitly decide the leaf behaviour in git before or with the upgrade, rather than relying on the default:
- To keep today's behaviour, add `rotationPolicy: Never` to `ledger-tls` and `checkout-tls`. This also survives the removal of the feature gate in 1.19.
- To take the new, more secure behaviour, add `rotationPolicy: Always`. Do this only once the unknowns below are checked.
- Do not use the chart feature-gate opt-out as the long-term answer. It disappears in 1.19.
- Keep the root CA's `Never`. Consider moving it into the base so other overlays inherit it.

## Missing information and tests needed before proceeding
1. **Live state:** confirm that the live `Certificate/mesh-root-ca` in `cert-manager` really has `rotationPolicy: Never`, and that Argo CD is synced and not out of sync on that field. Confirm that the current `ledger-tls` and `checkout-tls` Secrets exist and are `Ready`. I only saw the repo, not the cluster.
2. **Renewal timing:** get `status.notAfter` and `status.renewalTime` for `ledger-tls` and `checkout-tls`. This shows when the first rotation will happen after the upgrade.
3. **Out-of-repo consumers:** check whether anything outside this repo reads those Secrets or pins their keys (other namespaces, a service mesh, monitoring that scrapes certificates, HSM or allow-lists). `ORIENTATION.md` says the repo is complete, but I could not verify this. Also confirm that there are no other Certificate resources created outside the repo, such as ingress-shim annotations or other tooling.
4. **Test in staging with the same manifests:**
   - Upgrade to 1.18.0 and run `cmctl renew ledger-tls checkout-tls`.
   - Confirm that the key hash changes and the issuer chain still validates.
   - Watch checkout logs for "ledger call failed" during the swap.
   - Confirm what ledger serves after the renewal without a pod restart, and after a restart.
5. **Reload behaviour:** decide how ledger picks up renewed certificates (restart, reloader, or nginx reload). This is risk 1 and applies whatever policy you choose.
6. **Staging coverage:** check whether staging or dev builds the PKI from `pki/base` without the prod overlay. If so, its root CA would rotate on the new default, which would be a good place to see the failure mode, but also something to fix.
