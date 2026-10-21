# cert-manager v1.17.2 -> v1.18.0: impact assessment for the payments prod cluster

## Verdict

**Do not proceed as-is. One documented breaking change creates a delayed outage risk for this environment. It is easy to neutralise before the upgrade.**

- The upgrade itself (chart, CRDs, RBAC, webhook) looks safe for this repo.
- The default for `Certificate.spec.privateKey.rotationPolicy` flips from `Never` to `Always`. None of our Certificates set it, so all three silently change behaviour.
- The one that matters is `mesh-root-ca`, the root of the mTLS trust chain. Its first renewal after the upgrade will generate a **new root key**. Under 1.17.2 the key was reused.
- Nothing breaks on upgrade day. The failure appears at the next root renewal and is invisible in every diff.

## Evidence base

- `upstream/.../upgrading-1.17-1.18.md` and `release-notes-1.18.md` for the documented breaking changes.
- `rendered/manifest.diff`, `rendered/to.yaml` and the `env/` manifests.
- Not available: live cluster state, Secret contents and Argo CD state. Those gaps are listed at the end.

## What is affected

### 1. `rotationPolicy` default: Never -> Always (HIGH, delayed)

**Evidence**
- `ledger-certificate.yaml`, `checkout-certificate.yaml` and `mesh-root-ca.yaml` (including the prod patch) set no `rotationPolicy`. `rendered/to.yaml` has no `rotationPolicy` on any rendered Certificate (it appears only in the CRD schema, line 832).
- The CRD diff shows the default text changing to `Always`. The `DefaultPrivateKeyRotationPolicyAlways` feature gate is on by default, and `platform/cert-manager/values.yaml` sets no feature gates.
- The manifest diff shows no change to any consumer manifest. This is the "unchanged manifest" trap: the default is applied server-side at reissue time.

**Why the root CA is the problem**
- The trust chain is `selfsigned-bootstrap` -> `mesh-root-ca` (ECDSA, prod `duration` 8760h, `renewBefore` 720h) -> ClusterIssuer `mesh-ca` -> `ledger-tls` and `checkout-tls`.
- Under 1.17.2 (`Never`), a root renewal reuses the key. Leaf certificates signed by the old root still verify against the new root, because the subject and key are the same.
- Under 1.18.0 (`Always`), a root renewal creates a new key and a different root. Old leaves do not verify against it, and new leaves do not verify against the old root.
- Each service holds its own copy of the CA in its Secret's `ca.crt`. That copy is written only when that service's own leaf is issued.
  - `checkout` verifies `ledger` with `--cacert /tls/ca.crt`.
  - `ledger` verifies `checkout` with `ssl_client_certificate /tls/ca.crt` and `ssl_verify_client on`.
- After a root key rotation, `mesh-ca` signs with the new key. Whichever leaf renews first gets a new-root-signed cert and a new-root `ca.crt`. Its peer still has the old chain, so mutual verification fails in both directions until both sides have renewed.
- There is a second, compounding gap. nginx in `ledger` reads `ssl_client_certificate` at start or reload and does not re-read it when the mounted Secret changes. The Deployment has no reloader or checksum annotation. After a root rotation `ledger` would therefore keep trusting the old root until its pods restart. The `checkout` loop (curl) re-reads files on every call, so it picks up changes immediately.
- This nginx behaviour existed before, but it was harmless while the root key never changed. The default flip is what turns it into an outage.

**Timing**
- Leaves renew about 20 days after issuance (`duration` 720h, `renewBefore` 240h). The root renews 30 days before its 1-year expiry.
- The date of the next root renewal is unknown from the repo. It is the date the blast radius arrives, and it could be days away.

**Leaf certificates**
- `ledger-tls` and `checkout-tls` will also rotate keys on renewal. This is intended upstream and is fine for a cert and key pair delivered together in one Secret.
- Caveat: `ledger`'s nginx will not load the renewed cert and key until it is restarted. That behaviour is unchanged by 1.18, but with 10 days between renewal and expiry it is a latent risk worth fixing alongside this.

**Mitigation, in order of preference** (both are accepted by 1.17.2, so they can ship before the upgrade as a separate commit)
1. Set `spec.privateKey.rotationPolicy: Never` explicitly on `mesh-root-ca` in `platform/pki`. Keep this until a deliberate root rotation procedure exists.
2. Optionally also set it on the leaf Certificates if you want zero behaviour change. Leaving them at `Always` is arguably better security.
3. A values-level fallback is `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false`. It is temporary only: upstream says the gate is removed in 1.19.
4. Independently, fix the `ledger` reload gap (a reloader, or a checksum or rollout trigger) so that any future CA rotation is survivable.

### 2. `revisionHistoryLimit` default: nil -> 1 (LOW, one-time)

- No Certificate sets it, so on upgrade the controller garbage-collects old CertificateRequests down to one per Certificate.
- Nothing in the repo reads CertificateRequests, so there is no functional effect. The only loss is issuance audit history, which may matter for a payments environment. Export it first if it does.

### 3. Security advisory GHSA-8rvj-mm4h-c258 (informational for this repo, but relevant to version choice)

- Direct ACME `Challenge` creation can bypass DNS01 solver policy. The advisory lists affected versions >= 1.18.0 and <= 1.20.2, with fixes only in 1.19.6 and 1.20.3. **There is no 1.18.x fix**, so 1.18.0 enters an affected range that 1.17.2 is outside.
- Not exploitable here: the repo declares only a CA issuer and a self-signed issuer, with no ACME or DNS01 solver. That is a repo-level conclusion; check the live cluster for ClusterIssuers not declared here.
- It does mean 1.18.x is a dead end for this advisory. If the goal is a long-term target, plan for 1.19.6+ and do not stop at 1.18.

## What is not affected, and why

| Item | Why it is safe |
|---|---|
| ACME HTTP01 `PathType: Exact` breaking change (ingress-nginx and OpenShift) | No ACME issuer is declared, and no Ingress is created for challenges. Only `ca` and `selfSigned` issuers exist. |
| Chart values compatibility | `values.yaml` uses only `crds.enabled`, `crds.keep`, `replicaCount`, `prometheus.enabled`, `resources`, `webhook.replicaCount` and `cainjector.replicaCount`. All are present in the 1.18.0 chart values. |
| Webhook probe port change (`6080` -> named `healthcheck`) | `to.yaml` defines `containerPort: 6080` with `name: healthcheck` (line 12355), so the named port resolves. |
| Metrics service/ServiceMonitor port-name change | `prometheus.enabled: false`, so the resources are not rendered. |
| RBAC split (`controller-challenges` -> `http01-` and `dns01-controller-challenges`) | This only matters for ACME challenges. The renamed ClusterRoles and bindings are all in the rendered diff, and the cert-manager ServiceAccount is bound to the new ones. Argo's `prune: true` removes the old ones. |
| New CRD fields (`signatureAlgorithm`, ACME `profile`, Vault `serverName`, `iss`/`ciss` short names) | Additive and optional. No existing fields were removed or changed. Short names collide with nothing in this repo, but check the cluster for other CRDs using `iss` or `ciss`. |
| Issuer behaviour for CA and SelfSigned types | No change is listed in the release notes. |
| `AdditionalCertificateOutputFormats` GA and `UseDomainQualifiedFinalizer` GA | We use neither `additionalOutputFormats` nor an override of the gate. |
| Certificate specs (ECDSA/256, usages, SANs, `commonName`) | The IP-in-`commonName` fix does not apply, because all names are DNS. The `size: 256` ECDSA key is valid. |
| Kubernetes 1.31 | The chart `kubeVersion` is `>= 1.22.0-0`. |

## Operational notes for the rollout

- **Webhook availability:** the webhook has one replica and a validating webhook with a fail policy. During its rollout, Argo syncs of `pki` and `payments` Certificates can fail briefly. `selfHeal` should recover, but it is better not to sync other apps at the same moment.
- **CRDs:** the Argo `cert-manager` app uses ServerSideApply, which handles the CRD size. `crds.keep: true` protects against deletion on prune.
- **Startup check:** the `startupapicheck` Job is re-run with the new image. It should pass if the webhook is healthy.

## Missing information and tests before proceeding

1. **Live state of `mesh-root-ca`.** I could not see the cluster, so I do not know its `notBefore`, its renewal time or its current Secret. Read `status.renewalTime` on `mesh-root-ca`, `ledger-tls` and `checkout-tls`. If the root renews inside the next few weeks, apply the `rotationPolicy: Never` change first and let it settle.
2. **Confirm the repo is the full truth.** The orientation says it is, but check the cluster for extra ClusterIssuers (especially ACME/DNS01), other Certificates, and any other consumers of `mesh-root-ca` or `ca.crt`, such as other namespaces or sidecars. Also check whether Argo has any `ignoreDifferences` or other apps outside this repo.
3. **Staging rehearsal of a root rotation under 1.18.0.** On a copy of this PKI and the two services, force a root reissue (`cmctl renew mesh-root-ca`) with and without the explicit `Never` setting. Confirm the key does or does not change, then watch `checkout` -> `ledger` calls. Do the same for the leaves, and confirm whether `ledger` needs a restart.
4. **Confirm no reissue storm at upgrade time.** In staging, upgrade and verify no Certificate becomes `Issuing` and no Secret changes. I believe the new default applies only when a reissue happens, but the notes do not state that explicitly, so verify it.
5. **Check how leaf renewal reaches the pods.** Confirm how `ledger` picks up renewed certificates (restart policy, reloader) and whether anything monitors `ledger call failed` in `checkout` logs.
6. **Export CertificateRequest history** if it has audit value, before the garbage collection.
7. **The `algorithm` field is stored redacted in the repo files** (the comment says the value is "ECDSA"). I could not verify the real value from the files. Confirm it against the live Certificates.
8. **Rollback plan:** with `crds.keep: true`, CRDs stay in place on a rollback, and the new optional fields are harmless to 1.17.2. Rolling back is straightforward, but a root key rotation that has already happened cannot be rolled back.
9. **Version target.** Decide whether 1.18.0 is really the goal. 1.18.x patch releases exist (up to 1.18.6, per the release notes), and the advisory above has no 1.18 fix. Prefer the latest 1.18.x patch at minimum, or go to 1.19.6+.

## Recommended sequence

1. Commit explicit `rotationPolicy: Never` on `mesh-root-ca` (and decide on the leaves). Sync it and confirm no reissue.
2. Add a reload mechanism for `ledger`.
3. Rehearse the upgrade and a forced root rotation in staging.
4. Upgrade in a quiet window, sequenced so `cert-manager` is healthy before `pki` and `payments` sync.
5. Plan a deliberate, documented root rotation procedure, covering overlap trust bundles and restarts, before ever setting `Always` on the root.
