# cert-manager v1.17.2 -> v1.18.0: impact assessment for the prod payments cluster

## Verdict

**No change to the repository is required for the upgrade to function. One documented breaking change (the `rotationPolicy` default) does change behaviour for two certificates here. From the evidence in the repo, that change is benign. It is neutralised where it matters most (the mesh root CA). The other documented breaking changes do not apply.**

I would proceed, subject to the checks in "Missing information / tests" and a few hardening edits. I could not check anything about the live cluster.

Method: I read `ORIENTATION.md`, every file under `env/`, `rendered/manifest.diff`, the 1.17-to-1.18 upgrade note and the 1.18 release notes. I also read the GHSA advisory and spot-checked `rendered/to.yaml`. I could not run shell commands (Bash was denied), so I did not hash the redacted values or diff the chart `values.yaml` files line by line. Chart-level effects are judged from the rendered diff.

## What the environment looks like

- cert-manager is installed through the Argo CD app `clusters/prod/cert-manager.yaml`. It uses the Helm chart pinned at `targetRevision: v1.17.2`. That pin is the single place to bump. The app uses `ServerSideApply`, `prune: true` and `selfHeal: true`, and runs in sync-wave 0.
- The values in `platform/cert-manager/values.yaml` are minimal: `crds.enabled` and `crds.keep` true, one replica each, Prometheus off, resource requests. No `config.featureGates` and no ingress, Gateway or ACME settings are set.
- The `pki` app (wave 1) deploys `platform/pki/overlays/prod`. It contains:
  - `selfsigned-bootstrap` (ClusterIssuer, selfSigned).
  - The `mesh-root-ca` Certificate (isCA, ECDSA P-256). The prod overlay sets a 1y duration and `privateKey.rotationPolicy: Never`.
  - The `mesh-ca` ClusterIssuer (type `ca`, using the `mesh-root-ca` Secret).
- The `payments` app (wave 2) has two leaf Certificates, `ledger-tls` and `checkout-tls`:
  - Both are issued by `mesh-ca`, are ECDSA P-256, last 720h and renew 240h before expiry.
  - **Neither sets `rotationPolicy`**, and neither sets `revisionHistoryLimit`.
  - ledger is nginx with `ssl_verify_client on`, using `/tls/ca.crt`. checkout is a curl loop doing mTLS against ledger with its own `tls.crt`, `tls.key` and `ca.crt`.
- There is no ACME Issuer, no Ingress, no Gateway and no Vault or Venafi issuer anywhere in the repo. `ORIENTATION.md` says the repo is the complete declaration of the cluster.

## Documented breaking or notable changes, and whether they apply here

### 1. `Certificate.spec.privateKey.rotationPolicy` default changes from `Never` to `Always` (BREAKING): applies, benign on the evidence

Evidence:
- The release notes and upgrade note describe the change.
- The rendered CRD diff changes the field text to "Default is `Always`".
- The `DefaultPrivateKeyRotationPolicyAlways` feature gate defaults to on (beta). The chart values here do not touch it.

Per resource:

- **`mesh-root-ca`: protected.** The prod overlay sets `rotationPolicy: Never` explicitly. The `pki` Argo app points at that overlay (`clusters/prod/pki.yaml`, path `platform/pki/overlays/prod`), so it is what gets applied. This matters because the root CA's public key is the trust anchor for every `ca.crt` in the mesh. An accidental key rotation at root renewal would be the dangerous outcome of this upgrade. Caveats:
  - The base `mesh-root-ca.yaml` has no `rotationPolicy`. Only the overlay protects it.
  - The value is shown redacted in the files I read. The comment says it is the digest of "Never". I relied on that comment and did not verify it independently.
- **`ledger-tls` and `checkout-tls`: behaviour changes.** After the upgrade, each renewal generates a new private key. Before, the key was reused.
  - **No immediate re-issuance is expected at upgrade time.** The renewal triggers are expiry, `renewBefore`, a spec mismatch or a manual renew. I read nothing that makes a rotationPolicy change a trigger. This is an inference from the docs and not something I observed, so test it (see below). The first rotation should happen at the next scheduled renewal, which is 480h (20 days) after each cert's last issuance.
  - **Nothing in the repo pins the leaf public key.** `ledger.conf` verifies clients only against the CA (`ssl_client_certificate /tls/ca.crt`). checkout verifies ledger only with `--cacert /tls/ca.crt`. Neither pins a key or fingerprint. A new leaf key under the same CA therefore validates the same way.
  - **Secret updates are consistent.** `tls.crt`, `tls.key` and `ca.crt` are written together to one Secret. Both Deployments mount the Secret as a directory volume, with no `subPath` and no `envFrom`, so kubelet propagates updates.
  - **New, tiny race.** Key and cert now change together. Under `Never`, a reader that caught the cert and key on either side of the kubelet symlink swap still got a matching pair, because the key never changed. Under `Always`, a read straddling the swap could pair the old cert with the new key. For checkout's curl this means one failed call, retried 30s later. For nginx it only matters if a reload lands in that instant. I consider this negligible but real.
  - **Pre-existing risk that key rotation does not create.** ledger's nginx loads the cert once and nothing in the repo reloads it. There is no reloader, sidecar or `nginx -s reload`. After the cert's 720h lifetime, a ledger pod that has not been restarted will serve an expired cert, with or without this upgrade. The repo gives no evidence of how this is handled today. Check it separately.
  - **Unknown: consumers outside the repo.** Anything outside it (a WAF, firewall or allow-list that pins the leaf key or fingerprint) would break on rotation. I cannot see any.

### 2. `Certificate.spec.revisionHistoryLimit` default changes from nil to 1 (potentially breaking): applies, low impact

- The rendered CRD diff shows "Default value is `1`".
- On upgrade, the controller garbage-collects stale `CertificateRequest`s. It keeps the latest revision for each Certificate, which here means the root and the two leaves.
- Nothing in the repo reads CertificateRequests: no RBAC, scripts, alerts or other consumers. CertificateRequests are controller-created and not tracked in Git.
- The only loss is history, for anyone who uses it for audit or forensics. Check whether anything outside the repo (audit tooling, log shipping, runbooks) depends on old CertificateRequests. If you want history, set `revisionHistoryLimit` explicitly on the Certificates.

### 3. ACME HTTP-01 Ingress `pathType: Exact` (BREAKING for ingress-nginx and OpenShift): does not apply

There are no ACME issuers, Ingresses or Gateways. The environment uses only selfSigned and CA issuers. I verified this by reading every manifest in `env/`. The `ACMEHTTP01IngressPathTypeExact` gate, which exists only in v1.18.1 and later, is irrelevant.

### 4. Chart and rendered changes

Per `rendered/manifest.diff`, none affect this environment:
- **Version labels and images** change as expected (`v1.18.0` for controller, webhook, cainjector, startupapicheck and the acmesolver flag).
- **Controller args** are otherwise unchanged. There are no new or removed flags.
- **Webhook probes** change from numeric port `6080` to the named port `healthcheck`. `rendered/to.yaml` defines `containerPort: 6080, name: healthcheck`, so the named port resolves.
- **RBAC.** The single `cert-manager-controller-challenges` ClusterRole and binding are split into `cert-manager-dns01-controller-challenges` and `cert-manager-http01-controller-challenges`. The old pair is removed. Argo's prune will delete the old ClusterRole and binding and create four new ones.
  - These are ACME-only permissions, and this cluster has no ACME. The `orders` role also moves a few rules. None of this touches the CA issuer or the Certificate paths.
  - If the Argo AppProject `platform` restricts cluster-scoped kinds (it is not in this repo, so I could not check), confirm that ClusterRole and ClusterRoleBinding are allowed. They are already in use, so this should be fine.
- **CRDs** gain only additive fields (`signatureAlgorithm`, ACME `profile`, Vault `serverName`) and the `ciss`/`iss` short names. None of the manifests set these fields. A theoretical short-name collision with another CRD is worth a `kubectl api-resources` check. `crds.keep: true` is retained.
- **Dropped doc text.** The `AdditionalCertificateOutputFormats` text is dropped because the feature is now GA. It is unused here.
- **Metrics and ServiceMonitor port-name changes** are irrelevant because `prometheus.enabled` is false.

### 5. Security advisory GHSA-8rvj-mm4h-c258

Challenge resources can bypass the DNS01 solver policy. The advisory page lists "Affected versions >= 1.18.0, <= 1.20.2" and patched versions 1.19.6 and 1.20.3.
- Exploiting it needs a `ClusterIssuer` with a DNS01 solver and ACME Challenge create permission. This environment has no ACME issuer, so it is not exploitable here.
- There is **no patched 1.18.x**. Staying on the 1.18 line leaves the advisory unfixed.
- The advisory lists v1.18.0 as affected and does not list v1.17.2. I do not know whether 1.17 was tested, so do not read 1.17.2 as unaffected.

## Version-choice finding: target v1.18.6, not v1.18.0

The 1.18 release notes include later patch releases:
- **v1.18.5** fixes a moderate DoS (GHSA-gx3x-vq4p-mhhv, a DNS-response panic). It also fixes an infinite re-issuance loop when an issuer returns a certificate whose public key does not match the CSR.
- **v1.18.6** fixes CVE-2025-68121 (Go bump).
- **v1.18.3** fixes an unwanted renewal when `issuerRef` kind or group is omitted. The kind and group are explicit in every Certificate here, so this is not a risk.

The `rendered/` before-and-after files cover 1.18.0 only. If you pin 1.18.6, re-render to confirm the diff is still just the above. Also decide whether to go straight to 1.19.6 or 1.20.3 for the advisory fix. That is outside this question.

## Things that look fine but do not prove runtime compatibility

- **Unchanged Certificate manifests.** The ledger and checkout Certificates do not change in the diff, but their effective behaviour changes through the controller default. The manifest is unchanged and the semantics are not.
- **Webhook availability during rollout.** There is a single webhook replica, and the validating and mutating webhooks fail closed. During the webhook rollout, creating or updating cert-manager resources is blocked briefly. Existing mTLS traffic is unaffected. The `startupapicheck` Job also runs as part of the chart. If Argo does not handle it as a hook cleanly, the sync could look stuck.
- **Wave ordering.** The CRDs update in wave 0 before the pki and payments apps in waves 1 and 2, which is good. Argo does not wait on webhook readiness unless health checks cover it.
- **Kubernetes 1.31 support.** I did not confirm that 1.18.0 supports Kubernetes 1.31. The compatibility page is not in the supplied upstream material.

## Recommended edits (optional)

1. Set `privateKey.rotationPolicy` explicitly on `ledger-tls` and `checkout-tls`. Use `Always` to adopt the new behaviour on purpose, or `Never` to preserve the old one. The `DefaultPrivateKeyRotationPolicyAlways` gate goes GA in 1.19 and cannot be turned off, so relying on the default is not stable.
2. Move `rotationPolicy: Never` from the prod overlay into `platform/pki/base/mesh-root-ca.yaml`, so a different overlay cannot silently lose it.
3. Set `revisionHistoryLimit` explicitly if you need to keep CertificateRequest history.
4. Decide whether ledger needs a reloader or pod restart on cert change. This is a pre-existing gap and not part of this upgrade.

## Missing information / tests before proceeding

Live cluster, read-only:
1. `kubectl get certificate -A -o wide` plus `status.notAfter` and `renewalTime` for `mesh-root-ca`, `ledger-tls` and `checkout-tls`. This shows when the first rotation will fire, ideally not during a change freeze.
2. `kubectl get certificate mesh-root-ca -n cert-manager -o jsonpath='{.spec.privateKey.rotationPolicy}'`. Confirm it is literally `Never` in the live object and in the raw Git file. I saw it only as a redacted value.
3. Confirm the live cluster has no Issuers, ClusterIssuers, Certificates or Ingresses that are not in this repo. `ORIENTATION.md` says there are none, but the claim is not verified.
4. Confirm the Argo AppProject `platform` and its resource whitelists (not in this repo) allow the new ClusterRole and ClusterRoleBinding names.
5. Confirm the cluster runs Kubernetes 1.31 as stated and that 1.18 supports it.
6. Check for external consumers that pin the ledger or checkout leaf public key or fingerprint. Check for anything that reads CertificateRequests.

Staging, with cert-manager 1.17.2 installed:
1. Replicate the pki and payments setup. Upgrade to the chosen 1.18.x. Confirm no Certificate is re-issued at upgrade time, by comparing `status.revision` before and after.
2. Force a leaf renewal (`cmctl renew ledger-tls checkout-tls`). Confirm that:
   - the leaf key changes (compare the `tls.key` fingerprint);
   - the root CA key and `ca.crt` are unchanged;
   - checkout still reaches ledger over mTLS after the Secret sync (watch for `ledger call failed` lines).
3. Force a `mesh-root-ca` renewal. Confirm its key is unchanged, which proves the `Never` protection works. Confirm the leaf chain still validates against old and new `ca.crt`.
4. Count CertificateRequests before and after the upgrade, to confirm the GC behaviour and that nothing breaks.
5. Observe the webhook rollout with 1 replica. Confirm the Argo sync completes cleanly, including the old challenges ClusterRole being pruned.

Process: take a backup of the `mesh-root-ca`, `ledger-tls` and `checkout-tls` Secrets, and have a rollback path (re-pin `targetRevision: v1.17.2`). `crds.keep: true` means CRDs survive a rollback, but they will have been upgraded in place.
