# cert-manager v1.17.2 -> v1.18.0: impact assessment for the prod payments cluster

## Verdict

**No documented breaking change is a hard blocker for this environment, but the upgrade is not a no-op. One breaking change (the `rotationPolicy` default flip) changes runtime behaviour for two live Certificates, and one (the `revisionHistoryLimit` default) deletes data on first reconcile.**
The upgrade is acceptable to proceed with, subject to the pre-flight checks at the end. The most important check is one I could not perform from this repository: confirming that the rendered `mesh-root-ca` carries `rotationPolicy: Never` and that the live nginx and curl workloads cope with key rotation.

## What this environment actually uses

- cert-manager is installed from the Helm chart with minimal values (`env/platform/cert-manager/values.yaml`): CRDs enabled and kept, single replicas, Prometheus off, and a resource request. There are no feature gates, no `extraArgs` and no `config:` block.
- Only the `CA` and `SelfSigned` issuers are used: `selfsigned-bootstrap` and `mesh-ca`. There are no ACME, Vault, Venafi or DNS01 resources. A grep for ingress, ACME and CertificateRequest across `env/` found nothing.
- The chain is `selfsigned-bootstrap` -> `mesh-root-ca` (Certificate, `isCA`, ECDSA-256) -> `mesh-ca` ClusterIssuer -> `ledger-tls` and `checkout-tls`. Both leaf certificates are ECDSA-256 with a 720h duration and 240h `renewBefore`.
- Consumers:
  - `ledger` is nginx. It mounts the Secret `ledger-tls` at `/tls`, uses `ssl_client_certificate /tls/ca.crt` with `ssl_verify_client on`, and the config is read at start-up.
  - `checkout` is a curl loop. It re-reads `/tls/*` on every call and verifies ledger against `ca.crt` from its own Secret.
- Argo CD syncs automatically with `prune: true`, `selfHeal: true` and `ServerSideApply=true`. cert-manager is in wave 0, pki in wave 1, payments in wave 2.

## Affected

### 1. `rotationPolicy` default changes from `Never` to `Always` (breaking, on by default)

This is the one that matters most.
Evidence: upgrade guide item 1, release notes lines 109-148, and the CRD description change in `rendered/manifest.diff` ("Default is `Always`").

- **`ledger-tls` and `checkout-tls`: affected.** Neither sets `privateKey.rotationPolicy`, so from 1.18 each renewal generates a new key. Before, the key stayed the same across renewals.
  - Renewal happens roughly every 20 days (720h - 240h). The first renewal after the upgrade is the first rotation.
  - No re-issuance is forced at upgrade time. Rotation applies only when a re-issuance happens.
  - Why it matters here: nginx loads `tls.crt` and `tls.key` once, at start or reload. Nothing in the repo reloads it when the Secret changes.
    - This problem already exists today. A renewed certificate also does not reach a running nginx, so ledger's served certificate would expire after 30 days.
    - Key rotation adds a second risk. Kubernetes swaps the Secret volume atomically, but nginx opens the two files separately. A reload or worker start that straddles a swap can read a new `tls.crt` with an old `tls.key`, or the reverse. With a constant key that race was harmless. With a rotating key it produces a cert/key mismatch error.
    - `checkout` re-reads the files on every curl invocation, so it has the same small race.
  - Conclusion: this is a latent hazard, not an immediate failure. Verify the reload story for ledger (see the tests below).
- **`mesh-root-ca`: not affected, and the reason is an explicit setting.**
  - `env/platform/pki/overlays/prod/mesh-root-ca-prod.yaml` sets `rotationPolicy: Never`, and `clusters/prod/pki.yaml` deploys the `prod` overlay.
  - This setting is critical. If the root's key were rotated at its next renewal (1 year, 720h before expiry), the new root would sign new leaves, but every leaf's `ca.crt` in the Secrets is a snapshot of the old root.
  - Ledger and checkout trust only what is in `ca.crt`, and nothing here distributes the new root to both sides atomically. Mutual TLS would break until both Secrets and both workloads were refreshed. That would be an outage of the payments mesh.
  - Caveat: I could only read the Kustomize inputs. `rendered/` contains only the Helm output, so I did not see a built manifest for the pki overlay. Strategic-merge patches normally merge `privateKey` and keep `algorithm` and `size`, but this must be confirmed with `kustomize build platform/pki/overlays/prod`.

### 2. `revisionHistoryLimit` default changes from nil to 1 (potentially breaking)

Evidence: upgrade guide item 2 and release notes lines 151-160.

- No Certificate in the repo sets it, so on upgrade every Certificate (all four) has its older CertificateRequests garbage collected. Only the latest is kept.
- Nothing in the repo reads CertificateRequests. I found no approver, audit exporter or policy engine.
- **Unverified:** the repo cannot show whether anything outside it (an audit pipeline, a compliance process, or incident-forensics practice) depends on historical CertificateRequests. For a payments platform this is worth asking about. The deletion is irreversible, so export first if the history has value.

### 3. Rendered chart changes with a rollout footprint (not in the release-notes breaking list)

Evidence: `rendered/manifest.diff`.

- The ClusterRole and ClusterRoleBinding `cert-manager-controller-challenges` are removed and replaced by `cert-manager-dns01-controller-challenges` and `cert-manager-http01-controller-challenges`.
  - With Argo `prune: true`, there is a short window during rollout in which old and new bindings differ. This only affects ACME challenges, which are unused here, so there is no functional impact.
  - It does matter for anyone who has bound to or audited the old role name. Search for external references.
- The webhook liveness and readiness probes change from `port: 6080` to the named port `healthcheck`. The container does define `name: healthcheck` on 6080 (`rendered/to.yaml:12355-12356`), so this is consistent. I verified it in the render.
- New CRD content: `signatureAlgorithm`, `profile`, Vault `serverName`, and the `iss` and `ciss` shortNames. These are additive.
  - **Check `iss` and `ciss` against other CRDs and any scripts or aliases on the cluster.**
- CRDs are installed via the chart (`crds.enabled`, `keep: true`). Because of that, the CRD update will ride on this Argo sync. `ServerSideApply=true` is set, which avoids the annotation-size limit on the large CRDs.
- The webhook is a validating and mutating webhook for every cert-manager resource. Argo ordering matters: pki (wave 1) must not apply while the new webhook is not yet ready.

## Not affected

| Item | Why |
|---|---|
| HTTP01 `PathType: Exact` and the ingress-nginx incompatibility | There are no ACME issuers and no Ingress resources in `env/`. |
| ACME profiles, Vault `serverName`, Venafi URL, Cloudflare and Route53 fixes | No ACME, Vault or Venafi issuers are configured. |
| `ValidateCAA` gate removal | The gate is not set in `values.yaml`. |
| `AdditionalCertificateOutputFormats` and `UseDomainQualifiedFinalizer` promoted to GA | No gate overrides, and no `additionalOutputFormats` on any Certificate. |
| OperatorHub discontinuation | Installed via the Helm chart, not OLM. |
| ServiceMonitor and metrics port changes | `prometheus.enabled: false`. Note this also means there are no cert-expiry alerts to benefit from the new `not_after` metrics. |
| `--extra-certificate-annotations` | New and opt-in. No Ingress or Gateway resources exist. |
| Kubernetes 1.31 | The chart's `kubeVersion` is `>= 1.22.0-0` in both versions. |
| `global.rbac.disableHTTPChallengesRole` | This value is in the chart's `values.yaml` for 1.18.0 but was reverted in 1.18.2 per the release notes. It is unset here and unimportant. |

## Version choice: v1.18.0 specifically

- Targeting the first patch of a minor is unnecessary risk. The release notes list 1.18.1 to 1.18.6. 1.18.1 adds the `ACMEHTTP01IngressPathTypeExact` gate and 1.18.2 reverts a Helm option. 1.18.3 fixes an unexpected re-issuance on `issuerRef` kind/group defaulting. 1.18.5 fixes a DoS panic and a re-issuance loop. 1.18.6 patches `CVE-2025-68121`.
  - Even though this environment's issuerRefs are explicit (group and kind set on every Certificate), prefer the latest 1.18.x over 1.18.0. I recommend a retarget to 1.18.6 as the minimum.
- Security advisory `GHSA-8rvj-mm4h-c258` (the copy in `upstream/` is an HTML capture of the advisory page): "Direct ACME Challenge resources can bypass Issuer DNS01 solver policy". It is rated high.
  - It affects `>= 1.18.0, <= 1.20.2`, with fixes only in 1.19.6 and 1.20.3. There is no 1.18.x fix, so upgrading to any 1.18 puts the cluster inside the affected range, where 1.17.2 is outside it.
  - Exploitability here is **very low**. It needs a ClusterIssuer with a DNS01 solver, and there are none. There is only a CA issuer and a SelfSigned issuer. It is, however, a reason to treat 1.18 as a stepping stone to 1.19.6+ rather than a resting place.
  - I read the advisory's metadata and summary only. I did not verify the versions beyond what it states.
- Is 1.17.2 itself still supported? That is not answered by the provided material (`upstream/.../releases/README.md` was not read). Check it, since it bears on how urgent this upgrade is.

## Evidence limits and missing information

1. No built Kustomize output. I could not confirm the rendered `mesh-root-ca` has `rotationPolicy: Never`. I also could not confirm the leaf Certificates render as expected.
2. No live cluster state. Unknown: the current Certificate and Secret status, the notAfter dates and renewal timing for each certificate (which determines when the first key rotation will happen), the actual number of CertificateRequests, and whether anything in the cluster watches them.
3. The nginx reload behaviour of `ledger` is not visible. There is no reloader sidecar, no Reloader/stakater annotation, and no `shareProcessNamespace`. Deployments mount the Secret as a volume with no checksum annotation.
4. Mesh trust distribution: `ca.crt` is copied into leaf Secrets at issuance. The repo shows no mechanism for pushing a changed root to workloads, so any future root change is a manual, coordinated event. This is out of scope for 1.18 but is the reason point 1 matters.
5. Cluster-wide cert-manager consumers outside this repo. ORIENTATION.md says the repo is complete, but other teams' Certificates, `cert-manager.io/*` annotations on other resources, and external tools (cmctl, approver-policy, trust-manager, csi-driver) are not visible. Any Certificate elsewhere lacking an explicit `rotationPolicy` or `revisionHistoryLimit` gets the new defaults.
6. Argo CD application health under the pruned and replaced RBAC objects, and the webhook-readiness ordering, were not testable from files alone.

## Tests and checks before proceeding

1. `kustomize build env/platform/pki/overlays/prod` and `.../apps/payments/overlays/prod`. Confirm that `mesh-root-ca` has `privateKey.rotationPolicy: Never` with `algorithm: ECDSA` and `size: 256` preserved. Confirm the leaf Certificates have no rotationPolicy.
2. `kubectl get certificate,certificaterequest -A -o wide` on prod. Record all Certificates and their renewal times. List the CertificateRequests that will be deleted, and export them if there is any audit need.
3. **Decide the leaf-key policy explicitly rather than inheriting a default.** Either set `rotationPolicy: Always` on `ledger-tls` and `checkout-tls` on purpose, or set `Never` to keep today's behaviour. Doing this in a commit *before* the upgrade makes the behaviour change independent of the version change. Set `revisionHistoryLimit` explicitly too if history is wanted. Do not rely on the gate `DefaultPrivateKeyRotationPolicyAlways: false`, because 1.19 removes it.
4. In staging (a cluster with the same issuer chain), upgrade and then force a renewal: `cmctl renew -n payments ledger-tls checkout-tls`. Verify that:
   - the `tls.key` changes;
   - checkout still reaches ledger (`ledger ok: client=...`);
   - ledger actually serves the new certificate (`openssl s_client`), which shows whether nginx picks up changed files or needs a reload;
   - nothing logs a key/cert mismatch.
   If ledger needs a reload, add a reload mechanism (a reloader controller, a sidecar, or a checksum rollout) before the first production renewal after the upgrade.
5. Confirm the root is untouched: compare the `mesh-root-ca` Secret `tls.key` fingerprint and `ca.crt` before and after the upgrade. Both must be unchanged, and the Certificate must not show a new revision.
6. Run the upgrade through Argo with the webhook and startupapicheck observed: confirm that the cert-manager Application becomes Healthy, the CRDs update, the old challenges ClusterRole is pruned without error, and pki and payments then sync.
7. Search for external references to `cert-manager-controller-challenges` (RBAC audits, policy-as-code) and for conflicts with the `iss` and `ciss` shortNames.
8. Plan the rollback: Helm values are in Git, but the CRDs are `keep: true` and the new CRD schema fields are additive. Check that reverting to 1.17.2 does not strand any field. Do not enable `signatureAlgorithm` or `profile` use until the upgrade is settled.
9. Retarget to the latest 1.18.x (1.18.6) and schedule the follow-up to 1.19.6+/1.20.3+ because of the GHSA.

## Bottom line

For this environment the headline 1.18 breaking changes reduce to two:
the key-rotation default (which is safe for the root only because of the explicit `Never` override in the prod overlay, and which creates a rotation behaviour on the leaves that nobody has tested), and the one-time deletion of historic CertificateRequests.
Everything ACME-, ingress- and Vault-related is irrelevant here because those features are not configured.
An unchanged manifest does not prove runtime compatibility. The leaf rotation can only be shown safe by a forced renewal against the real nginx and curl workloads.
