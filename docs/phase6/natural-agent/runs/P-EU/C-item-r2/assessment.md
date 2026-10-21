# cert-manager v1.17.2 -> v1.18.0: `privateKey.rotationPolicy` default flips to `Always`

## Verdict
**Yes, this change affects this environment, and the upgrade is not safe as-is.**
The main risk is the **`mesh-root-ca` Certificate**. At its next renewal it will generate a new CA key instead of reusing the old one. That would break mutual TLS between checkout and ledger for a period.

Nothing breaks at upgrade time. The change only takes effect when a Certificate is next re-issued.

## Evidence
- **No Certificate sets `rotationPolicy`.** I checked all of them:
  - `apps/payments/base/ledger-certificate.yaml`
  - `apps/payments/base/checkout-certificate.yaml`
  - `platform/pki/base/mesh-root-ca.yaml`
  - the prod overlay `platform/pki/overlays/prod/mesh-root-ca-prod.yaml`

  All of them inherit the new default, `Always`.
- **The feature gate is not turned off.** `platform/cert-manager/values.yaml` has no `config.featureGates`. The rendered 1.18.0 controller args in `rendered/to.yaml` have no `--feature-gates`. So the gate is on by default.
- **The rendered diff confirms the change.** `rendered/manifest.diff` shows the CRD text changing from "Default is `Never` for backward compatibility" to "Default is `Always`". The upstream upgrade doc (`upgrading-1.17-1.18.md`) says the same.
- **The upgrade rolls out automatically.** `clusters/prod/cert-manager.yaml` pins the chart with `targetRevision: v1.17.2`. Argo CD has automated sync with `selfHeal` and `prune`. Bumping the pin applies the change with no manual gate.

## What is affected

### 1. `mesh-root-ca` (cert-manager namespace): high risk
- It is the self-signed root, signed by `selfsigned-bootstrap`. The `mesh-ca` ClusterIssuer signs every leaf with it.
- In prod it has a duration of 8760h and `renewBefore` of 720h, so it renews about 11 months after issue.
- On 1.17 that renewal reused the key. The new root cert had the same public key, so existing leaf chains and every `ca.crt` copy stayed valid.
- On 1.18 the renewal creates a **new key pair**, which is a different trust anchor from the old one.

How it breaks, using the consumers in this repo:
- Each service gets its trust anchor from its own Secret's `ca.crt`, written when that leaf was last issued:
  - ledger: `ledger.conf` uses `ssl_client_certificate /tls/ca.crt` with `ssl_verify_client on`.
  - checkout: `curl --cacert /tls/ca.crt` against ledger.
- After the root rotates, `mesh-ca` signs with the new key. cert-manager does **not** re-issue leaves when the issuer's CA changes.
- `ledger-tls` and `checkout-tls` each renew on their own schedule (`renewBefore` 240h on a 720h duration, so about every 20 days, at different offsets).
- Until both have renewed, the two sides can disagree on the root:
  - If ledger's `ca.crt` is new and checkout still presents an old-CA cert, ledger rejects the client cert.
  - If checkout's `ca.crt` is new and ledger still serves an old-CA cert, checkout's curl fails verification.
- The window can last up to about 20 days. The visible symptom is checkout logging "ledger call failed" and payments mTLS failing.

### 2. `ledger-tls` and `checkout-tls`: low to medium risk
- Each renewal now produces a new ECDSA P-256 key. That is the intended security behaviour and works with mTLS as long as the CA stays the same.
- **checkout** re-reads its cert and key files on every curl, so it follows a renewal immediately.
- **ledger (nginx)** loads `tls.crt` and `tls.key` at startup and does not reload them when the files change. It keeps serving the old, still-valid pair until it restarts or reloads. This is already true today, but with key rotation the files now contain a new key, so a restart or reload is what picks up the new pair. The repo has no reloader or checksum annotation that would trigger one.
- I found no key pinning or other consumer of the leaf public keys in the repo.

## What is not affected
- The upgrade itself does not re-issue anything. Existing Secrets stay as they are until a renewal, a manual `cmctl renew`, or another re-issue trigger.
- The `mesh-ca` and `selfsigned-bootstrap` ClusterIssuers are not changed by this default. It only applies to Certificates.
- Adding an explicit `rotationPolicy: Never` to a Certificate does not change its key and should not cause a re-issue.
- The other diff items do not touch this environment: the ACME, Vault and Venafi CRD text, the `ciss`/`iss` short names, the new `signatureAlgorithm` field (unused), the challenge RBAC split (no ACME issuers exist), and the webhook probe port name.
- The `revisionHistoryLimit` default change (unset to 1) is a separate upstream item. I did not assess it here.

## Recommendation
Before bumping the pin, do one of these:
1. **Preferred:** set `privateKey.rotationPolicy: Never` explicitly on `mesh-root-ca`. This keeps same-key renewal. For the leaves, either set `Never` too or accept `Always`, which is the secure default. Add the field to the base manifests so it is explicit.
2. **Stopgap:** set `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false` in `platform/cert-manager/values.yaml`. This only defers the problem, because 1.19 removes the gate.

If you want the root key rotated at some point, plan it as a deliberate migration with a trust overlap (distribute both old and new CA to the consumers before switching). A bare `Always` on the root has no overlap.

## Missing information and tests needed
- **Root age and renewal time.** I could not see live cluster state. Run `kubectl get certificate -n cert-manager mesh-root-ca -o yaml` and check `notAfter` and `renewalTime`. That tells you how soon the exposure arrives.
- **Other consumers of the CA.** The repo is stated to be complete, but confirm that nothing outside it (other namespaces, teams or tools) trusts `mesh-root-ca` or its `ca.crt`.
- **Staging test on 1.18.0.** Run `cmctl renew mesh-root-ca` with the default and check three things:
  - The root key changes.
  - checkout-to-ledger mTLS fails until both leaves re-issue.
  - The same test with `rotationPolicy: Never` passes.
- **Leaf test.** Renew `ledger-tls` alone and confirm what nginx serves before and after a restart.
- **After the fix is merged.** Verify that the live Certificates show `spec.privateKey.rotationPolicy: Never` where intended, and that Argo CD shows no drift.
