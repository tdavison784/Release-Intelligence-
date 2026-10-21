# cert-manager v1.17.2 -> v1.18.0: impact assessment (prod payments cluster)

## Verdict

**The upgrade has no hard breaking change for this environment, but two behaviour changes do apply, and one of them needs a mitigation decision before merge.** Nothing needs to be fixed for the manifests to apply. Two behaviours need a decision or a test. I'd also change the target version (see "Target version").

| Change in 1.18 | Applies here? | Severity |
|---|---|---|
| `privateKey.rotationPolicy` default `Never` -> `Always` | **Yes**: `ledger-tls` and `checkout-tls` | Medium. Behaviour change at the next renewal. Not an outage by itself, but it adds a race for the mTLS consumers. |
| `revisionHistoryLimit` default `nil` -> `1` | **Yes**: all 4 Certificates | Low. Old CertificateRequests are deleted. |
| Helm RBAC split (`controller-challenges` -> `http01-`/`dns01-controller-challenges`) | Yes, mechanically | Low. Argo prunes and creates. Verify no external bindings. |
| Webhook probes use the named port `healthcheck` | Yes, mechanically | None. The port is defined in `rendered/to.yaml`. |
| HTTP01 Ingress `pathType: Exact` | No | n/a |
| ACME profiles, Vault `serverName`, `signatureAlgorithm`, short names | No (new optional features) | n/a |
| OperatorHub discontinued | No (Helm install) | n/a |
| GHSA-8rvj-mm4h-c258 (affects >=1.18.0, no 1.18.x fix) | Preconditions not met here | Informational (see below) |

## What is affected

### 1. `rotationPolicy` default flips to `Always` (the real change)

Evidence:
- `upstream/.../upgrading-1.17-1.18.md` and `release-notes-1.18.md` list it as the main breaking change. The gate is `DefaultPrivateKeyRotationPolicyAlways`, enabled by default, and it becomes GA and non-disableable in 1.19.
- `rendered/manifest.diff` shows only a description change on the Certificate CRD, with no `default:` added. The default is applied in the controller, not the schema. So Argo CD will not see drift on the Certificate objects, and nothing in git changes.
- `env/apps/payments/base/ledger-certificate.yaml` and `checkout-certificate.yaml` set `privateKey.algorithm` and `size` but **no `rotationPolicy`**. They will switch from "reuse the key" to "new key on every re-issuance". With `duration: 720h` and `renewBefore: 240h`, this happens about every 20 days.
- Nothing is triggered at upgrade time. The policy only matters at the next re-issuance. The first rotation will land up to 20 days after the upgrade, so a clean post-upgrade check proves nothing about it.

Why it matters for these consumers:
- **checkout** (`checkout-deployment.yaml`) runs curl every 30s against `/tls/tls.crt` and `/tls/tls.key`, so it re-reads the mounted Secret. Kubelet updates Secret volumes by swapping a symlink, but curl opens the cert and the key as two separate file opens. With the old default the key never changed, so a cert/key mismatch was impossible. With `Always`, a read that straddles the swap could pair the new cert with the old key. I expect that to cost at most one failed 30s iteration ("ledger call failed"). Real consumers with a similar read pattern would be worse. I haven't tested it.
- **ledger** (nginx, `ledger.conf`) loads `ssl_certificate`/`ssl_certificate_key` only at start or reload. No reloader or sidecar exists anywhere in `env/`. So ledger keeps serving its old cert and key after renewal until a pod restart. That is **already true today**, and it is a latent problem. Renewal happens at day 20, the old cert expires at day 30, and nothing reloads nginx. The key rotation does not make it worse, because a running nginx keeps a consistent in-memory pair. It does mean a restart after renewal will load an entirely new keypair. The upgrade only matters here if you assume ledger is renewing cleanly today, so check that it is.
- Any key pinning, or any system that records the leaf public key (SPKI pins, allowlists, HSM or audit records), would break on rotation. Nothing in the repo does this. The repo is declared to be complete, but only an engineer can confirm there are no out-of-band consumers.

Mitigations, in order of preference:
1. Set `privateKey.rotationPolicy: Never` or `Always` **explicitly** on `ledger-certificate.yaml` and `checkout-certificate.yaml` in a PR *before* the upgrade. That makes the behaviour independent of the default, and it is required anyway before 1.19 removes the gate. For a payments mTLS mesh, key rotation on renewal is probably desirable. If so, pick `Always` explicitly and fix the reload gap first.
2. Or set `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false` in `platform/cert-manager/values.yaml` as a temporary hold. The 1.18 chart exposes `config:`/`featureGates` (`v1.18.0/.../values.yaml`). This only defers the decision to 1.19.

### 2. `revisionHistoryLimit` default is now `1`

- None of the four Certificates (`mesh-root-ca`, `ledger-tls`, `checkout-tls`, and the prod patch) set it.
- On upgrade, historical CertificateRequests are garbage-collected, keeping only the latest revision per Certificate. The release notes say this explicitly.
- Nothing in `env/` reads CertificateRequests. Argo CD does not manage them, because they are controller-created. But **if the payments platform uses the CertificateRequest history for audit or compliance of certificate issuance, it will be lost.** Export it first, or set `revisionHistoryLimit` explicitly.
- Like the key-rotation default, this is applied in the controller. The diff shows only a description change.

### 3. RBAC changes in the chart (mechanical, low risk)

- `cert-manager-controller-challenges` ClusterRole and ClusterRoleBinding are removed. They are replaced by `cert-manager-http01-controller-challenges` and `cert-manager-dns01-controller-challenges` (`manifest.diff`). The controller ServiceAccount is bound to both new roles, so it loses nothing.
- Argo has `prune: true`, so the old objects will be deleted. The only risk is something else in the cluster that references the old role name. That could be an aggregated role, another ClusterRoleBinding, or a policy-as-code rule. `env/` contains no such reference, but the cluster could. Check with `kubectl get clusterrolebinding,clusterrole -A | grep controller-challenges`.
- Because this environment uses no ACME, the challenge roles are effectively unused. Their rename has no functional effect.

### 4. Webhook probes use a named port

- The probes change from `port: 6080` to `port: healthcheck`. `rendered/to.yaml` defines `containerPort: 6080, name: healthcheck`, so they resolve. Not a risk.

## What is not affected, and why

- **HTTP01 `pathType: Exact` / ingress-nginx**: there are no ACME issuers, no Ingress, and no `ingress-shim` usage in `env/`. The only issuers are `selfSigned` (`selfsigned-bootstrap.yaml`) and `ca` (`mesh-ca-issuer.yaml`).
- **Mesh root CA key**: `platform/pki/overlays/prod/mesh-root-ca-prod.yaml` sets `privateKey.rotationPolicy: Never` **explicitly**, so the flipped default does not apply. The CA key stays stable, and the `ca.crt` already in the leaf Secrets (used by `ledger.conf` and checkout's `--cacert`) stays valid across CA renewals. Caveat: this guard lives **only in the prod overlay**. The base `mesh-root-ca.yaml` has no rotationPolicy. The cluster uses only `overlays/prod` (`clusters/prod/pki.yaml`). If someone repoints or refactors the overlay, the CA key would rotate at the next renewal, and every Secret's `ca.crt` would then be untrusted. I'd move the `Never` into the base. The patch targets exactly one object, so it matches the root-CA name and namespace.
- **Prometheus/ServiceMonitor port-name change**: `prometheus.enabled: false` in `values.yaml`, so no ServiceMonitor is rendered.
- **Certificate field changes**: `signatureAlgorithm` is a new optional field and nothing sets it, so the default stays unchanged. The IP-in-`commonName` fix does not apply, because all CNs are DNS names (`ledger.payments.svc`, `checkout.payments.svc`, `payments-mesh-root-ca`). `AdditionalCertificateOutputFormats` going GA is irrelevant, since no `additionalOutputFormats` are used.
- **`issuerRef` defaulting bug** (fixed in 1.18.3, relevant to the 1.19 upgrade): all Certificates here set `group` and `kind` explicitly.
- **Chart values**: `crds.enabled: true`, `crds.keep: true`, `replicaCount`, `resources`, `prometheus.enabled`, and the `webhook`/`cainjector` replicas all remain valid in the 1.18.0 chart `values.yaml`. `kubeVersion: ">= 1.22.0-0"` is unchanged and compatible with cluster 1.31 on the chart side.
- **CRDs**: only additive changes (`ciss`/`iss` short names, `profile`, `serverName`, `signatureAlgorithm`) plus comment edits. No schema removals or tightened validation appear in the diff. The `AdditionalCertificateOutputFormats` description was edited only.
- **GHSA-8rvj-mm4h-c258** (direct Challenge creation bypassing solver policy): the advisory lists affected versions `>= 1.18.0, <= 1.20.2`, patched in `1.19.6`/`1.20.3`. There is no 1.18.x fix. It requires a ClusterIssuer with a DNS01 solver. This environment has none, so it is not exploitable here. It is, however, a reason not to stay on the 1.18 line long term. Note also that it was *tested* on 1.18.6, and that 1.17.2 is outside the listed range, so the upgrade moves you from "not listed" to "listed, unfixed".

## Target version

`ORIENTATION.md` and `clusters/prod/cert-manager.yaml` (`targetRevision: v1.17.2`) point at **v1.18.0**. The release notes show later fixes in the same line: 1.18.1 (the `ACMEHTTP01IngressPathTypeExact` gate and a longer ACME authorization timeout), 1.18.2 (a name-constraints fix), 1.18.3 (PEM size limits and an issuerRef bug), and **1.18.5 (a moderate DoS fix: GHSA-gx3x-vq4p-mhhv)**, and 1.18.6 (Go CVE fix). If you upgrade, go to **v1.18.6**, not v1.18.0. `rendered/` only covers 1.18.0, so re-render for the final version. I assume the chart diff is similar but have not checked it.

## Missing information and tests needed before proceeding

1. **Re-render and diff for the real target (v1.18.6)**. I only reviewed 1.17.2 -> 1.18.0.
2. **Staging run that actually exercises key rotation**. Upgrade, then `cmctl renew ledger-tls checkout-tls` (or lower `renewBefore`). Confirm:
   - the Secrets get a new `tls.key`;
   - checkout's loop shows no more than a one-iteration failure, and none at steady state;
   - ledger's behaviour after renewal. Does it pick up the new pair, or keep the old one until restart? Either way, document it.
3. **Confirm how ledger reloads**. Check whether anything outside this repo (a reloader, cronjob, or Argo hook) restarts ledger on renewal. If not, the day-20-renewal / day-30-expiry gap is a pre-existing outage waiting to happen.
4. **Live-cluster state I can't see from git**:
   - the actual `rotationPolicy` and `revisionHistoryLimit` on live Certificates, since Argo might not show a controller-defaulted field;
   - whether any ClusterRoleBinding/ClusterRole outside this repo references `cert-manager-controller-challenges`;
   - whether any consumers outside `payments` read the mesh Secrets, or pin keys.
5. **Audit/compliance owners**: is CertificateRequest history retained or required anywhere? If so, export it before the upgrade, or set `revisionHistoryLimit` explicitly.
6. **Argo behaviour with the chart upgrade**: confirm a dry-run sync shows no unexpected `OutOfSync` on the CRDs, with `ServerSideApply=true` and `crds.keep: true`, and that the pruning of the old challenge RBAC objects happens cleanly. Also verify the cert-manager webhook comes up before the `pki` app (sync-wave 1) syncs, since that wave creates Certificates and ClusterIssuers.
7. **Kubernetes 1.31 runtime compatibility** is not covered by the files provided. Only the chart `kubeVersion` constraint is. Check the upstream compatibility page.
8. **Rollback plan**. Downgrading the chart does not revert any Secrets that were already re-keyed. Because `crds.keep: true`, the CRDs persist. State the rollback procedure before the change.

## Suggested sequence

1. PR A (before upgrading): set `rotationPolicy` and `revisionHistoryLimit` explicitly on the service Certificates, and move `rotationPolicy: Never` into the base `mesh-root-ca.yaml`. Fix ledger's reload gap if confirmed.
2. Staging upgrade to v1.18.6, with a forced renewal test.
3. Prod upgrade (`targetRevision` and the chart values) once the staging results are clean.
