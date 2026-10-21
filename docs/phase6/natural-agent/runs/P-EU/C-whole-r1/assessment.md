# cert-manager v1.17.2 -> v1.18.0: impact assessment (payments prod)

## Verdict

**Yes, there is one breaking change that matters here. It is latent rather than immediate, and the manifests will not show it.**

The new default `Certificate.spec.privateKey.rotationPolicy: Always` (it was `Never`) is a real hazard for the **`mesh-root-ca` CA certificate**. When that CA next renews, it will get a *new key pair*. Nothing in this repo makes the payments mesh survive that.

Every other documented 1.18 change is either not applicable or low impact.

Proceed only after the mitigation in "Recommendation" is in place. The simplest form is to pin `rotationPolicy: Never` on `mesh-root-ca` in Git before, or in the same change as, the version bump.

## Evidence

Sources: `ORIENTATION.md`, `upstream/.../upgrading-1.17-1.18.md`, `release-notes-1.18.md`, `rendered/manifest.diff`, and `env/**`.

### 1. Default `rotationPolicy` Never -> Always: AFFECTED (high severity, delayed)

**Upstream**
- The upgrade guide and release notes list this as a breaking change.
- It is feature-gated (`DefaultPrivateKeyRotationPolicyAlways`), on by default, and no longer switchable in 1.19.
- The rendered CRD diff confirms it: `Default is Never...` becomes `Default is Always.` (`rendered/manifest.diff` ~L71-76).

**Repo**
- No `Certificate` sets `privateKey.rotationPolicy`. I checked all four:
  - `platform/pki/base/mesh-root-ca.yaml` and its prod overlay patch.
  - `apps/payments/base/ledger-certificate.yaml`.
  - `apps/payments/base/checkout-certificate.yaml`.
- All four therefore move from `Never` to `Always`.
- `platform/cert-manager/values.yaml` does not touch `config.featureGates`, so the gate stays at its new default (on).

**Timing**
- Nothing is reissued by the upgrade itself. The policy only applies when an issuance happens.
- Issuances are triggered by:
  - renewal: leaves at 480h of their 720h life, the root at 335d of its 365d life (prod overlay: `duration: 8760h`, `renewBefore: 720h`);
  - a manual `cmctl renew`;
  - any spec change that forces reissue.

**Consumer relationships**
- `ClusterIssuer/mesh-ca` is a CA issuer reading `Secret/mesh-root-ca` (`mesh-ca-issuer.yaml`). That secret is produced by `Certificate/mesh-root-ca`, which is self-signed via `selfsigned-bootstrap`.
- Both consumer pods verify peers only against the `ca.crt` in their own Secret:
  - `ledger.conf`: `ssl_client_certificate /tls/ca.crt; ssl_verify_client on`.
  - `checkout-deployment.yaml`: `curl --cacert /tls/ca.crt`.
- cert-manager copies the issuer's CA certificate into `ca.crt` **at the time each leaf is issued**. The two leaves renew independently and are not synchronised with each other or with the root.

**Why this was safe under 1.17**
- With `Never`, a root renewal reuses the same key and subject.
- Old leaves, signed by the old root cert, still verify against the renewed root cert because the public key is the same.
- Leaves with a stale `ca.crt` therefore kept working.

**What 1.18 changes**
- With `Always`, the root renewal generates a new key. The new cert has a different public key and no overlap or cross-signing.
- From then on, `mesh-ca` signs new leaves with the new key.
- A leaf that was renewed after the root gets the **new** `ca.crt`. A leaf that was not renewed still carries a cert signed by the **old** key and an old `ca.crt`.
- mTLS between a pod holding the new `ca.crt` and a peer presenting an old-key leaf fails in both directions. Example: checkout renews first, so its `ca.crt` is new, but ledger still presents an old-chain leaf. Checkout fails to verify ledger with "unable to get local issuer certificate", and the reverse holds for ledger verifying checkout.
- The mismatch lasts until both leaves are reissued, and it self-heals only if the pods pick up the new Secrets.
- Nothing in the repo reissues the leaves when the root changes. cert-manager does not do this automatically for CA issuer changes unless the leaf itself is re-triggered.
- Pods also do not reload on Secret changes. See the next section.

**Blast radius**
- This is mesh-wide failure of ledger and checkout calls in a payments production cluster.
- It is silent until it happens: no manifest diff will show it, and the first root renewal could be months away.

### 2. Leaf key rotation and nginx / curl behaviour: LOW to MODERATE, mostly a pre-existing weakness

- Leaf renewals now also rotate the leaf private key. No pinning of leaf keys or fingerprints exists anywhere in the repo, and validation is by CA only, so rotation does not break trust by itself.
- `ledger` is nginx with `ssl_certificate` / `ssl_certificate_key` read from a mounted Secret. nginx loads these at start or reload. There is no reloader, sidecar, or annotation in the repo.
  - After a leaf renewal the running nginx keeps serving the old cert and key in memory. They are consistent with each other, so nothing breaks until the old cert expires (30 days).
  - This was true on 1.17 as well. Renewal never reached the running process. It is a pre-existing expiry outage waiting to happen, not something 1.18 introduces. However, with a rotated key, someone who relies on a ledger restart picking up the new Secret will now get a new key too.
- `checkout` re-reads files on every `curl`, so it picks up new material within about 30s. Because kubelet swaps the Secret volume atomically, `tls.crt` and `tls.key` stay consistent.
- A nginx restart during root rotation would pick up the new `ca.crt` while checkout might still hold the old one, which makes the mismatch in section 1 worse.

### 3. `revisionHistoryLimit` default nil -> 1: NOT materially affected

- It is not set on any Certificate here, so the default applies. On upgrade, stale `CertificateRequest` objects are garbage-collected.
- I found no consumer of old `CertificateRequest`s (no scripts, policies, or Argo health customisations in the repo).
- The only loss is history for audit or forensic use. If the org relies on `CertificateRequest` history as a record of mesh issuance, snapshot it first.

### 4. ACME / HTTP01 `PathType: Exact`, ACME profiles, Vault `serverName`, DNS01 RBAC split: NOT AFFECTED

- There are no ACME or Vault issuers. The only issuers are `selfSigned` and `ca`. No Ingress or Gateway resources are in the repo.
- The HTTP01 `PathType: Exact` change and its `ingress-nginx` workaround only apply to ACME HTTP01 with Ingress solvers.
- The rendered ClusterRole `cert-manager-controller-challenges` is split into `cert-manager-dns01-controller-challenges` and `cert-manager-http01-controller-challenges`, with matching ClusterRoleBinding renames (rendered diff ~L691-798, ~L850-1050, ~L1123-1250).
  - It is RBAC for the same ServiceAccount (`cert-manager`), so controller permissions are preserved.
  - Argo `prune: true` will delete the old role and binding and create the new ones. That is expected churn, not a breaking change.
  - If anything else in the cluster binds to the old role name (not visible in this repo), that is unknown. See "Missing information".
- The advisory in `upstream/.../GHSA-8rvj-mm4h-c258` is about direct ACME `Challenge` resources using ClusterIssuer DNS01 credentials. It lists v1.18.6 and v1.19.5 as affected, so it is not a reason to stay on 1.17, and it is not exploitable here because no ACME issuer exists.

### 5. Rendered chart changes: NOT breaking for this environment

- **Images and labels:** every `v1.17.2` image and label becomes `v1.18.0`.
- **Webhook probes:** webhook probes change from numeric port `6080` to the named port `healthcheck` (`containerPort: 6080` is named `healthcheck` in `to.yaml` ~L12355). These are equivalent.
- **CRD additions:**
  - `profile`, `serverName` and `signatureAlgorithm` fields.
  - `iss` / `ciss` short names.
  - Doc-text edits.
  - Removal of the AdditionalCertificateOutputFormats doc text.
  - These are additive. I found no existing field in the repo that is removed or retyped.
- **CRD delivery:** CRDs are managed through the chart (`crds.enabled: true`, `keep: true`) with `ServerSideApply=true` in Argo. The `keep` policy means CRDs are not removed on uninstall.
- **Values:** `values.yaml` uses only keys that still exist in the 1.18.0 chart. I read it against `upstream/.../v1.18.0/.../values.yaml`; the keys used are `crds`, `replicaCount`, `prometheus`, `resources`, `webhook.replicaCount`, `cainjector.replicaCount`.
- **Other changes:** the `Certificate` `commonName` IP-address fix and the name-constraints fix don't apply, because the commonNames are DNS names and `NameConstraints` is not enabled.

### 6. Things that could not be verified from the repo

- `rendered/` contains only the Helm chart, not the PKI or payments resources. I judged the Certificate impact from the source YAML plus upstream semantics, not from a render of live state.
- The `algorithm: "<redacted ...>"` values in the Certificates are redacted placeholders for `ECDSA` (per the in-file comment). I took them at that value and did not need more.

## Missing information and tests needed before proceeding

1. **Live state.** The repo is declared complete, but the Secrets are not in it. For each of `mesh-root-ca`, `ledger-tls` and `checkout-tls`, check how close it is to renewal (`cmctl status certificate`, `status.notBefore`, `status.renewalTime`). In particular, find out when the root renews. If the root renews within weeks of the upgrade, the hazard is imminent.
2. **Confirm live `rotationPolicy`.** Check `kubectl get certificate -A -o yaml` for any `rotationPolicy` already applied. The repo says unset, but the manifests do not prove the cluster matches.
3. **Staging test of root rotation.** In a non-prod cluster running 1.18 with the same Certificates:
   - Force a root renewal (`cmctl renew mesh-root-ca`) and observe `ca.crt` in both leaf Secrets.
   - Exercise ledger/checkout mTLS in the mixed states: root new / leaves old, one leaf new / one old, and both new.
   - Repeat with `rotationPolicy: Never` to confirm the mitigation keeps mTLS working through a root renewal.
4. **Pod reload behaviour.** Confirm whether anything outside this repo (a reloader operator, admission mutation) restarts ledger on Secret change. If not, test that a leaf renewal is never picked up by nginx and decide whether to fix that separately.
5. **External consumers of `CertificateRequest` history or of the old `cert-manager-controller-challenges` ClusterRole.** Search other repos and policy engines (Kyverno/OPA, monitoring, audit scripts, other RBAC bindings).
6. **Argo behaviour on sync.** Dry-run the sync (`argocd app diff`) to confirm the CRD update with ServerSideApply works and that pruning the old ClusterRole and binding does not leave the controller without permissions. The cert-manager app is wave 0, the PKI app is wave 1, and payments is wave 2.
7. **Post-upgrade checks:**
   - Webhook, cainjector and controller are Ready.
   - `ClusterIssuer/mesh-ca` and `selfsigned-bootstrap` are Ready.
   - No `Certificate` triggers a surprise reissue.
   - The `ledger` endpoint answers the checkout call.

## Recommendation

1. Before or with the upgrade, add `privateKey.rotationPolicy: Never` to `mesh-root-ca` (base or prod overlay). This preserves the 1.17 behaviour for the one certificate where key rotation is harmful. Do this in Git, since Argo self-heals.
2. For the leaves, either set `Never` too to keep the change minimal, or accept `Always` once ledger reload behaviour is understood and tested.
3. Leave the feature gate alone. Turning it off is a temporary escape that disappears in 1.19, so explicit per-Certificate settings are the durable fix.
4. Plan a deliberate root CA rotation process (overlapping trust, with both old and new root in the trust bundle, then leaf reissue and pod restart) before ever setting `Always` on the root.
5. Target the latest 1.18.x patch (1.18.6 per the release notes) rather than 1.18.0. Later patches include the 1.18.1 feature gate and 1.18.3 / 1.18.5 fixes plus Go security updates. Note that rendered diffs were produced for 1.18.0 only, so re-render for the chosen version.
