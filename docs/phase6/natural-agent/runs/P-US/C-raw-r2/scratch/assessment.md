# cert-manager v1.17.2 -> v1.18.0: impact assessment (prod payments platform)

## Verdict

**No change in this repo is required to keep the platform working, but one documented breaking change does apply to it.** The default `privateKey.rotationPolicy` flips from `Never` to `Always`. The two mesh leaf certificates (`ledger-tls`, `checkout-tls`) don't set it, so they will start rotating their private key on every renewal. I found nothing that pins a key, so I don't expect an outage. I could not verify the ledger and checkout runtime behaviour from the repo alone, so test it before prod (see "Needed before proceeding").

Nothing is applied at upgrade time. The new default only matters at the next reissuance. Changing `targetRevision` triggers no reissue. The `issuerRef`s are fully explicit (group, kind, name), so the issuerRef re-issuance bug fixed in v1.18.3 does not apply.

I did not read the full rendered CRD YAML or the remaining sections of the 1.18 notes line by line. The conclusions below rest on the render diff, the upgrade guide, the release notes, and a search of the upstream files.

## What the upgrade changes

Sources: `upstream/.../upgrading-1.17-1.18.md`, `release-notes-1.18.md`, `rendered/manifest.diff`.

| Change | Affects this environment? |
|---|---|
| `rotationPolicy` default `Never` -> `Always` (breaking) | **Yes, for `ledger-tls` and `checkout-tls`.** Not for the root CA (pinned, see below). |
| `revisionHistoryLimit` default `nil` -> `1` (potentially breaking) | **Yes, as a quiet behaviour change.** Old CertificateRequests are garbage collected. Nothing in `env/` reads them (grep found no CertificateRequest references). External tooling and audit pipelines are unknown. |
| ACME HTTP01 Ingress uses `pathType: Exact` | **No.** The only issuers are `selfSigned` and `ca`. There is no ACME issuer and no Ingress in the repo. |
| New RBAC: `controller-challenges` split into `http01-` and `dns01-controller-challenges` | **No functional impact seen.** The diff shows the old role and binding removed and the new ones added, all bound to the same `cert-manager` ServiceAccount. Argo has `prune: true`, so the old objects get removed. |
| Webhook probes use named port `healthcheck` instead of `6080` | **No.** `to.yaml` declares `containerPort: 6080` named `healthcheck`, so the probes resolve. |
| CRD schema additions: `signatureAlgorithm`, ACME `profile`, Vault `serverName`, `shortNames` `ciss`/`iss` | **No.** These are additive and the repo uses none of them. The shortNames are a theoretical collision risk with another CRD (see open items). |
| OperatorHub packages discontinued | **No.** The install is from the Helm chart. |
| Feature gates | `values.yaml` sets none, so all defaults apply, including `DefaultPrivateKeyRotationPolicyAlways=true`. |

## Why the rotationPolicy change matters here

- `env/apps/payments/base/ledger-certificate.yaml` and `checkout-certificate.yaml` have `privateKey: {algorithm: ECDSA, size: 256}` and no `rotationPolicy`. Duration is 720h and renewBefore is 240h, so the first renewal under 1.18 happens within about 20 days of the last issuance. Each renewal then produces a new key.
- **The root CA is protected.** `platform/pki/overlays/prod/mesh-root-ca-prod.yaml` pins `rotationPolicy: Never`, and `clusters/prod/pki.yaml` points at `platform/pki/overlays/prod`, so the pin is in effect. This is the important case. The `mesh-ca` ClusterIssuer signs with the Secret `mesh-root-ca`. If the root key rotated, every existing leaf would stop chaining to the CA that `ledger.conf` (`ssl_client_certificate /tls/ca.crt`) and checkout (`--cacert /tls/ca.crt`) trust. Rendered/overlay evidence shows it stays pinned. Do not remove that overlay patch, and do not point any app at `platform/pki/base`, which has no pin.
- **The leaf consumers look tolerant of key rotation.** Trust is anchored on the CA cert (`ca.crt`), not on a leaf key or leaf fingerprint. Neither `ledger.conf`, the Deployments nor the Services contain any key or fingerprint pinning. Key and cert live in one Secret and are updated together, so a mounted `tls.crt`/`tls.key` pair stays consistent.
- **Residual risk (not new, but worth confirming):** the ledger nginx reads `/tls/*` only at start or reload. No reloader, sidecar or `checksum` annotation is visible. So a renewed leaf is not served until the pod restarts, and the same would have been true for same-key renewals under 1.17. Key rotation does not make it worse, because nginx keeps a consistent old cert and key pair in memory. The pre-existing hazard is a pod running past the 720h expiry without a restart. Checkout's curl re-reads the files on every call, so it picks up new material.
- Ledger runs 2 replicas in prod (`ledger-replicas.yaml`). Both mount the same Secret, so there is no per-replica key divergence.

## Related findings

- **The v1.18 line is not a good long-term target as written.** The bundled advisory GHSA-8rvj-mm4h-c258 lists affected versions `>= 1.18.0, <= 1.20.2`, with fixes only in 1.19.6 and 1.20.3, so no 1.18.x release is patched. It concerns direct ACME Challenge objects bypassing Issuer DNS01 solver policy to use ClusterIssuer DNS credentials. It is **not exploitable here as configured**, since there is no ACME or DNS01 issuer, but a 1.18.0 pin carries it. If 1.18.x is only a step, plan the follow-on upgrade. If the target is flexible, use a later 1.18 patch release: 1.18.6 and the other patch releases carry fixes for Go CVEs and a DoS bug (GHSA-gx3x-vq4p-mhhv). The upstream notes cover 1.18.1 through 1.18.6 and contain unrelated fixes.
- `rotationPolicy` default is not permanent: per the notes, 1.19 makes `Always` GA and removes the ability to turn the gate off. Setting `rotationPolicy: Never` explicitly on the leaves is a short-term hold, not a permanent opt-out. The better fix is to confirm rotation is safe and then set `Always` explicitly.
- The cert-manager Argo Application has `ServerSideApply=true`, `crds.enabled: true`, `crds.keep: true`. CRDs are handled by the chart, so no separate CRD step. The CRD diff is additive. Sync-wave ordering is cert-manager (0), pki (1), payments (2).

## Not affected, with evidence

- ACME, Ingress, Gateway, DNS01, Vault and Venafi paths: no such issuers or resources exist in `env/`.
- Prometheus: `prometheus.enabled: false`, so no ServiceMonitor or metrics changes matter.
- Chart values: `values.yaml` uses `crds`, `replicaCount`, `prometheus`, `resources`, `webhook.replicaCount` and `cainjector.replicaCount`, all of which the 1.18 chart still has. I did not diff the full upstream `values.yaml`; the rendered diff shows no unexpected value-driven changes.

## Needed before proceeding (missing information and tests)

1. **Rehearse a leaf renewal under 1.18 in a non-prod cluster** with the same issuer chain. Run `cmctl renew ledger-tls checkout-tls -n payments` (or wait for the real renewal). Confirm a new key is issued, `ca.crt` is unchanged, checkout still gets `ledger ok`, and nothing in the mesh logs a handshake failure.
2. **Find out how ledger picks up renewed certs.** Is there a reloader, a restart-on-secret-change mechanism, or a scheduled rollout? This is not in the repo. If there is none, fix it independently of this upgrade.
3. **Confirm the live root CA Secret matches the pinned state.** Check `kubectl get certificate mesh-root-ca -n cert-manager -o yaml` for `rotationPolicy: Never` and that the Secret's key is the one in use. Also confirm its next renewal date (1y duration, 30d renewBefore) so you know whether a root renewal falls close to the upgrade.
4. **Check for anything outside this repo that reads CertificateRequests** (audit, compliance export, alerting) before the garbage collection runs, or set `revisionHistoryLimit` explicitly if history is needed. Note the repo is declared complete, so also check the live cluster for resources not declared here.
5. **Check live CRDs for `iss`/`ciss` collisions** with any other installed CRD (`kubectl api-resources | grep -E '\b(iss|ciss)\b'`).
6. **Confirm Kubernetes 1.31 support for 1.18.** The upstream compatibility page is not among the provided documents, so I could not verify this.
7. **Dry-run the Argo sync**: confirm the pruning of `cert-manager-controller-challenges` happens after the new roles exist and that the controller has no RBAC errors in its logs, and that the webhook becomes ready (the startupapicheck job passes).
8. **Rollback plan.** CRD upgrades with `keep: true` are not undone by downgrading the chart. Decide how you'd revert, and note that downgrade after Certificates have rotated keys is safe for trust but leaves the new Secrets in place.
9. Decide the target patch version (see the advisory note above) and re-render for that version, since `rendered/` covers only v1.18.0.

## Summary

Affected: leaf-cert key rotation behaviour (new default), and CertificateRequest garbage collection. Not affected: ACME/Ingress changes, webhook probes, RBAC split, CRD additions, OperatorHub removal. Protected by an explicit setting: the mesh root CA. Unverified: ledger cert reload, external readers of CertificateRequests, live cluster state, Kubernetes 1.31 compatibility.
