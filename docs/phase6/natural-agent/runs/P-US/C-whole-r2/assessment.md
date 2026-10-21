# cert-manager v1.17.2 -> v1.18.0: impact assessment (payments prod)

## Verdict

**Yes, there is one breaking change that applies to this environment, and it is latent rather than immediate.** The default for `Certificate.spec.privateKey.rotationPolicy` flips from `Never` to `Always`. Two of the three Certificates here leave the field unset. They will start generating a new private key on every renewal after the upgrade.

I found nothing in the repo that this breaks outright. But the mTLS mesh's behavior at renewal time changes, and the repo can't show that the workloads handle it. The chart-level changes in the rendered diff are low risk. Details and open questions are below.

The safest path is to pin the behavior explicitly (see Recommendations). Pin it before or together with the upgrade, not after.

## What is affected

### 1. `rotationPolicy` default flips to `Always` (documented breaking change)

Sources:
- `upstream/.../upgrading-1.17-1.18.md` item 1.
- `release-notes-1.18.md`, "The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`".
- The `certificates.cert-manager.io` CRD diff in `rendered/manifest.diff` (`Default is Always ... DefaultPrivateKeyRotationPolicyAlways`).

Environment evidence:

| Certificate | File | rotationPolicy set? | Effect on 1.18 |
|---|---|---|---|
| `ledger-tls` | `apps/payments/base/ledger-certificate.yaml` | no | now rotates key on each issuance |
| `checkout-tls` | `apps/payments/base/checkout-certificate.yaml` | no | now rotates key on each issuance |
| `mesh-root-ca` | `platform/pki/base/mesh-root-ca.yaml` + `overlays/prod/mesh-root-ca-prod.yaml` | **yes, `Never`** in the prod overlay | unchanged |

Both leaf Certificates have `duration: 720h` and `renewBefore: 240h`, so they reissue roughly every 20 days. Every reissue from the first renewal after the upgrade will carry a new key.

Why this matters here:
- **The key change is not visible in the manifests.** `rendered/` only covers the Helm chart. It has no Certificate manifests, and the `apps/payments` and `platform/pki` manifests are byte-identical before and after. The behavior change comes from controller defaulting, so a clean git diff for the apps proves nothing.
- **Leaf Secrets are consumed in two ways:**
  - `ledger` (nginx) loads `/tls/tls.crt` and `tls.key` at startup. `ledger.conf` has no reload mechanism, and I found no reloader annotation or sidecar in the Deployments.
  - `checkout` (curl loop) re-reads `/tls/*` on every call.
  - With a new key per renewal, checkout presents a fresh cert and key. ledger accepts it because it trusts the CA only (`ssl_client_certificate /tls/ca.crt`), not a specific key or cert. Nothing in the repo pins leaf keys or public-key hashes.
- **The renewal gap is a pre-existing problem, not a new one.** nginx never reloads, so after the first renewal ledger keeps serving the old in-memory cert until a restart. That cert expires 30 days after issuance, so checkout's `--cacert` verification would then fail. This problem exists on 1.17.2 too, because the cert changes on renewal either way. Rotation doesn't make it worse, but it does make renewal visibly different (new key, new public key). Any out-of-repo system that pinned the key would break.
- **Checkout's trust anchor is `ca.crt` in its own Secret.** It is a copy of the CA made at issuance time. This is safe only because the CA key is not rotated (next section).

### 2. The root CA is protected only by one overlay line

`mesh-root-ca` is the trust anchor for the whole mesh. The base Certificate does not set `rotationPolicy`. Only the prod overlay patch sets it to `Never` (the YAML value is stored redacted as the digest of "Never"; the comment in the file says so). `clusters/prod/pki.yaml` points Argo at `platform/pki/overlays/prod`, so the patch is applied in prod.

- If the patch were not applied, the CA would get a new key at its first renewal. The renewal is `renewBefore: 720h` on a 1y cert, about day 335. Every leaf's `ca.crt` would then stop matching the issuer, and the mesh would fail until all leaves were reissued.
- This is **not affected as long as the overlay patch is in effect.** I verified it by reading the files, not by running `kustomize build`.
- The pin is invisible in the base and easy to remove. It deserves a comment or a test.

### 3. `revisionHistoryLimit` default `nil` -> `1` (potentially breaking)

- On upgrade, cert-manager garbage-collects all stale CertificateRequests (release notes: "all stale `CertificateRequest` resources will be garbage collected").
- No Certificate in the repo sets `revisionHistoryLimit`.
- Impact for this environment is operational only: loss of issuance history for forensics and debugging. Nothing in the repo consumes CertificateRequests.
- Argo CD will not prune them, because they are not tracked in git.

### 4. Chart changes that need a sync plan (not functional breaks)

From `rendered/manifest.diff`:
- **RBAC restructure.** `cert-manager-controller-challenges` (ClusterRole and binding) is deleted. It is replaced by `cert-manager-dns01-controller-challenges` and `cert-manager-http01-controller-challenges`. Argo has `prune: true`, so old objects go and new ones are created in the same sync.
  - This environment has no ACME issuers and no Challenge resources (only `selfSigned` and `ca` issuers), so the challenge controller is idle. The `orders` ClusterRole also gains secrets and events rules, which is harmless.
- **Webhook probes** change from `port: 6080` to the named port `healthcheck`. The rendered `to.yaml` defines `containerPort: 6080, name: healthcheck`, so the probe resolves. No problem.
- **Webhook availability during rollout.** `webhook.replicaCount: 1`, and both ValidatingWebhookConfiguration and MutatingWebhookConfiguration have `failurePolicy: Fail`. While the single webhook pod restarts, any Certificate create or update is rejected. Argo `selfHeal` retries, but a payments sync in that window can fail transiently. `payments` is wave 2, `pki` wave 1, `cert-manager` wave 0, so ordering helps, but Argo waves do not wait on webhook readiness after a later resync.
- **CRD changes are additive:**
  - New `signatureAlgorithm`, `profile`, and Vault `serverName` fields.
  - New short names `iss` and `ciss`.
  - Doc text changes.
  - `crds.keep: true` means CRDs persist, and `ServerSideApply=true` is set, so CRD updates are safe.
  - No existing field in this repo is removed or renamed. I found no field the repo uses that was altered, other than the default above.

## What is not affected (and why)

- **ACME HTTP01 `PathType: Exact` change** and OperatorHub removal: no ACME issuers, no Ingress-shim usage, no OpenShift. Not applicable.
- **Feature gates:** `values.yaml` sets no `config.featureGates` or `extraArgs`. The removed `ValidateCAA` gate and the `AdditionalCertificateOutputFormats` GA promotion do not matter here.
- **Helm values schema:** `values.yaml` only uses `crds`, `replicaCount`, `prometheus`, `resources`, and `webhook`/`cainjector.replicaCount`. All still render (the rendered diff succeeds). I did not diff the upstream `values.yaml` files line by line.
- **Issuers:** `selfsigned-bootstrap` and `mesh-ca` (CA issuer) use stable fields. Nothing in them changed.
- **Kubernetes 1.31:** the 1.18 line is not documented as requiring a newer Kubernetes in the files I reviewed. I did not find a minimum-version statement for 1.18 in them.

## Security advisory in `upstream/` (read this before choosing the target)

`GHSA-8rvj-mm4h-c258` (direct ACME Challenge creation bypasses solver policy and can use ClusterIssuer DNS01 credentials) lists affected versions **>= 1.18.0, <= 1.20.2**. The patched versions are **1.19.6 and 1.20.3**. There is no patched 1.18.x.

- **v1.18.0 is in the affected range, v1.17.2 is not.** The upgrade moves the cluster into a vulnerable version.
- **Exploitability here looks very low.** The advisory requires a ClusterIssuer with a DNS01 solver (this environment has none) and untrusted namespace users who can create Challenges. But it's a posture regression, and the new `dns01-controller-challenges` role is part of this area.
- **I'd consider targeting 1.19.6+ or 1.20.3+ instead.** Those releases add their own breaking changes, which I haven't reviewed (no 1.19/1.20 notes are in `upstream/`). That is a decision for the team.

## Missing information / tests before proceeding

1. **Pin the rotation behavior explicitly in git before upgrading.** Either:
   - set `privateKey.rotationPolicy: Never` on `ledger-tls` and `checkout-tls` to keep current behavior, or
   - set `Always` deliberately and accept it.

   Do not rely on the default. The `DefaultPrivateKeyRotationPolicyAlways=false` gate is only a stopgap, because it becomes GA in 1.19.
2. **Decide the intended policy for the leaves.** Rotating leaf keys is the better security posture. If that is the goal, the nginx reload gap (item 4 below) must be fixed first.
3. **Run `kustomize build platform/pki/overlays/prod`** and confirm `mesh-root-ca` renders with `rotationPolicy: Never`. Do the same for `apps/payments/overlays/prod`.
4. **Staging test of a forced renewal on 1.18.0** (`cmctl renew ledger-tls checkout-tls -n payments`):
   - Check that the Secret key changes.
   - Check that checkout can still call ledger.
   - Check whether ledger serves the new cert without a restart. I expect not, which would be a pre-existing outage waiting at day 30. Use a reloader, a restart on Secret change, or an nginx reload.
5. **Out-of-repo consumers.** ORIENTATION says the repo is complete, but confirm that nothing else pins leaf public keys, SPKI hashes, or key IDs (service mesh, WAF, partners, monitoring). The Secrets are mounted by the two payments Deployments in this repo only.
6. **Confirm the live cluster matches git.** Check:
   - The `mesh-root-ca` Certificate in-cluster has `rotationPolicy: Never`.
   - No Certificates exist outside this repo (`kubectl get certificates -A`) that rely on the old default.
   - Existing CertificateRequests are dispensable (they will be pruned).
7. **Check `iss` / `ciss` short names** for collisions with other CRDs (`kubectl api-resources`).
8. **Sync plan.** Bump `targetRevision` in `clusters/prod/cert-manager.yaml`, then watch the rollout of the single-replica webhook. Pause `payments` auto-sync during the cert-manager rollout, or retry on webhook errors. Confirm Argo prunes the old challenges ClusterRole and binding cleanly.
9. **Decide the target version** given the advisory above.
10. **Rollback plan:** CRDs are kept (`crds.keep: true`) and 1.18 adds only fields, so a chart rollback to 1.17.2 should be safe. But a key already rotated under 1.18 stays rotated. Confirm that rolling back doesn't reintroduce any old-key assumption.

## Method and limits

I read `ORIENTATION.md`, all files under `env/`, `rendered/manifest.diff` in full, the upgrade guide, the 1.18 release notes (breaking and major sections), and the advisory's version metadata and summary. I did not run `helm`, `kustomize`, or `kubectl`; all conclusions come from static review. The rendered diff covers only the cert-manager chart, not the PKI or app manifests. I did not read the full release-notes changelog list beyond the feature-gate lines, or the 1.17 notes. I did not diff the upstream `values.yaml` files directly.
