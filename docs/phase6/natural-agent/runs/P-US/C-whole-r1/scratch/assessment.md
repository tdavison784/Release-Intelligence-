# cert-manager v1.17.2 → v1.18.0: impact assessment for the prod payments cluster

## Verdict

**No change that must be fixed before upgrading, but not "safe to just merge".** The two documented breaking changes (the rotationPolicy default and the revisionHistoryLimit default) both reach this environment. Neither breaks it at upgrade time. The rotationPolicy flip changes behaviour at the next leaf renewal, and that renewal is where I would test.

The mesh root CA is protected by an explicit `rotationPolicy: Never` in the prod overlay. That one line is what keeps the upgrade safe. The two leaf certificates have no such setting and will start rotating their private keys.

Nothing in `env/` configures cert-manager feature gates, ACME issuers, Ingress or Gateway shims, or Vault/Venafi issuers. That removes most of the other 1.18 changes from scope.

## What this environment is

- **Install:** the Helm chart through Argo CD (`env/clusters/prod/cert-manager.yaml:16`, `targetRevision: v1.17.2`). Values are in `env/platform/cert-manager/values.yaml`. They set `crds.enabled/keep`, replica counts, resources and `prometheus.enabled: false`. There are no feature gates and no `extraArgs`. The sync options are `ServerSideApply=true`, `prune: true` and `selfHeal: true`.
- **Issuers:** only `ClusterIssuer/selfsigned-bootstrap` (selfSigned) and `ClusterIssuer/mesh-ca` (type `ca`, from Secret `cert-manager/mesh-root-ca`). There is no ACME, Vault, Venafi or other external issuer.
- **Certificates:**
  - `mesh-root-ca`: isCA, ECDSA 256, 1y in prod, renewBefore 720h.
  - `ledger-tls` and `checkout-tls`: ECDSA 256, duration 720h, renewBefore 240h, issued by `mesh-ca`.
- **Consumers:** `ledger` is nginx doing mTLS (`ssl_verify_client on`, `ssl_client_certificate /tls/ca.crt`). It runs 2 replicas in prod, which the rendered output shows. `checkout` is a curl loop that uses `/tls/ca.crt`, `tls.crt` and `tls.key` from its own Secret.

## Affected

### 1. Default `privateKey.rotationPolicy` changes from `Never` to `Always` (documented breaking change)

- Evidence: the upgrade guide, the 1.18 release notes, and the CRD text in `rendered/manifest.diff` ("Default is `Always`… gate `DefaultPrivateKeyRotationPolicyAlways`").
- **`ledger-tls` and `checkout-tls` (affected).** Neither sets `rotationPolicy` (`env/apps/payments/base/*-certificate.yaml`), so both move to `Always`.
  - Every future reissuance, including the routine renewal at 480h of age, will generate a new key. Today the key is reused.
  - The flip is a change in controller behaviour. It does not rewrite the Certificate objects or force reissuance when 1.18 starts. The first key change should therefore appear at each cert's next renewal, not at upgrade. I could not verify this from the repo. It should be confirmed in staging (see tests).
  - Consumers are probably fine:
    - `checkout` re-reads the files on every curl call.
    - `ledger`'s nginx loads `tls.crt` and `tls.key` only at start or reload. The kubelet swaps the Secret volume atomically, so a restart always sees a matching cert/key pair.
    - Pinning of the leaf public key is not visible anywhere in the repo, but it cannot be ruled out (see missing information).
  - **Pre-existing problem, not caused by the upgrade:** nothing reloads `ledger`'s nginx when the Secret changes. The cert held in memory expires after 30d unless the pod restarts. A renewal with a rotated key gives the same result as one without. This is the biggest practical risk when exercising renewal, so test it explicitly.
- **`mesh-root-ca` (NOT affected, by an explicit setting).** `env/platform/pki/overlays/prod/mesh-root-ca-prod.yaml` sets `privateKey.rotationPolicy` to `Never`.
  - The rendered prod output (`rendered/to.yaml:14032-14035`) confirms the field is present in the final object. Both `from.yaml` and `to.yaml` use the same render, so it is the same before and after.
  - This matters because a root key rotation would change the CA's key while keeping its subject. `ledger` and `checkout` hold `ca.crt` from the time each leaf was issued, so nginx and curl would then reject peers signed under the other root key until both sides reissued.
  - **The protection exists only in the prod overlay.** `platform/pki/base/mesh-root-ca.yaml` has no rotationPolicy. Anyone who removes or restructures the patch, or points Argo at base, silently gets `Always` on the root.
  - Caveat on the evidence: the repo stores the enum values as redacted digests, with a comment saying the digest is of `"Never"`. I could not recompute the hash because shell access was denied. I took the annotation at face value, and the render agrees.
  - Please also confirm that the live object matches. The repo is declared as the full source of truth, but that is worth a `kubectl get certificate mesh-root-ca -o yaml`.

### 2. Default `revisionHistoryLimit` changes from unset to `1` (documented, "potentially breaking")

- On upgrade, old `CertificateRequest` objects for all three Certificates will be garbage-collected, keeping only the latest per Certificate. That is a one-time deletion.
- Nothing in the repo reads CertificateRequests, so there is no functional dependency here.
- Check outside the repo for audit or compliance tooling, such as export jobs or SIEM, that treats CertificateRequests as the record of issuance. On a payments platform that is plausible. If anyone needs the history, export it before the upgrade, or set `revisionHistoryLimit` explicitly on the Certificates.

### 3. Operational items from the rendered diff (low risk)

- **Webhook probes** now use the named port `healthcheck` instead of `6080` (`rendered/to.yaml:12344-12362`). The container declares `containerPort: 6080` with `name: healthcheck`, so the manifest is internally consistent.
- **Single replicas:** the webhook, controller and cainjector each run 1 replica (values.yaml). While the webhook rolls, Certificate and Issuer admission is unavailable. An Argo sync that overlaps the rollout could see transient admission errors. Sync wave 0 for cert-manager and waves 1 and 2 for pki and payments help, but only in an app-of-apps ordering. I did not see a rollout strategy in the render, so I did not verify the surge or unavailable settings.
- **RBAC restructuring:** `cert-manager-controller-challenges` is replaced by `cert-manager-http01-controller-challenges` and `cert-manager-dns01-controller-challenges`. The ClusterRoleBindings change to match. Both roles serve ACME only, which this environment does not use. With `prune: true` Argo will delete the old pair. If any other repo or policy references the old ClusterRole name, it will break, but I found none here.
- **CRD changes** (new `signatureAlgorithm`, `profile`, `serverName`, short names `iss` and `ciss`, doc text) are additive. Existing objects are unaffected. The CRD ships through the chart (`crds.enabled: true`), and the sync uses ServerSideApply, so there is no manual CRD step.
- **Values file:** every key in use (`crds.enabled`, `crds.keep`, `replicaCount`, `prometheus.enabled`, `resources`, `webhook.replicaCount`, `cainjector.replicaCount`) still exists in the v1.18.0 chart values. I only confirmed the top-level keys.

## Not affected, and why

| Change | Why it does not apply |
|---|---|
| ACME HTTP01 Ingress `pathType: Exact` (ingress-nginx, OpenShift) | No ACME issuer, no Ingress or Gateway in `env/`. The `nginx` here is the ledger app, not ingress-nginx. |
| ACME profiles, Cloudflare/Route53 fixes, ACME client fork | No ACME. |
| Vault `serverName`, Venafi URL doc | No Vault or Venafi issuer. |
| `AdditionalCertificateOutputFormats` GA, `UseDomainQualifiedFinalizer` GA, `ValidateCAA` removed | No feature gates set in values. |
| OperatorHub discontinued | Installed via Helm, not OLM. |
| `--extra-certificate-annotations`, new metrics, `managed-by` label on ACME account keys | Opt-in or ACME only. |
| IP address in `commonName` handling | None of the three certs has an IP in commonName. |
| `--namespace` scoping fix | Not used (no extra args). |
| `signatureAlgorithm` new field | Not set, so the default is unchanged and existing issued certs are not re-evaluated. This is the repo-level view; it is not proven at runtime. |

## Security advisory

`GHSA-8rvj-mm4h-c258` (High): a namespace user can create `Challenge` resources directly and use a ClusterIssuer's DNS01 credentials. It is listed as affecting 1.18.0 through 1.20.2. The patched versions are 1.19.6 and 1.20.3. **There is no 1.18.x fix**, so v1.18.0 and the later 1.18 patches all remain exposed.

- It is **not exploitable in this environment as declared**, because it needs an ACME ClusterIssuer with a DNS01 solver and there is none.
- It does matter for planning. v1.18.0 is a stepping stone, and the version to stay on is 1.19.6 or later. Be aware that 1.19 makes the rotationPolicy gate GA (see "Missing information").
- The advisory's tested-versions list starts at 1.18.6, so I cannot say from this document whether 1.17.2 is also vulnerable.
- Also, `v1.18.0` itself predates the 1.18.1 to 1.18.6 fixes. If you do go to the 1.18 line, `v1.18.6` or later is the better target. The release notes list 1.18.6 as fixing a Go CVE, 1.18.5 a moderate DoS (`GHSA-gx3x-vq4p-mhhv`), and 1.18.3 a case where a Certificate whose issuerRef kind or group was omitted could be re-issued after an upgrade (the fix mentions upgrading to 1.19.x). Our Certificates set both fields explicitly, so this does not apply.

## Missing information and tests needed before proceeding

1. **Live state of `mesh-root-ca`.** Confirm `rotationPolicy: Never` is on the live object, plus its current `notAfter`, `renewBefore` and renewal time. If a root renewal is due soon after the upgrade, do not combine the two events.
2. **Staging rehearsal of renewal on 1.18.** Install 1.18.0 over a 1.17.2 copy with the same PKI and payments overlays. Then run `cmctl renew` on `ledger-tls` and `checkout-tls`. Verify:
   - A new key is generated (expected under 1.18).
   - The `ca.crt` in both Secrets is unchanged.
   - `checkout` still reaches `ledger` after renewal (curl re-reads files).
   - The `ledger` nginx behaviour after the Secret changes, with and without a pod restart. This also exercises the existing reload gap.
   - That `ledger-tls` and `checkout-tls` do **not** reissue merely from the upgrade (check Certificate revision and `notBefore` before and after).
3. **Root rotation negative test (staging only).** Remove the prod patch and confirm what happens on root renewal, to measure the blast radius the `Never` line is preventing. Optionally add a CI check that fails if the prod render of `mesh-root-ca` lacks `rotationPolicy: Never`.
4. **Key pinning.** Check any consumers outside this repo (service mesh, WAF, partner allow-lists, HSM or audit systems) that pin leaf public keys or fingerprints. The repo shows none, but it only declares two in-cluster apps.
5. **CertificateRequest history.** Ask whether anything depends on old CertificateRequests, and export them first if so (item 2 above).
6. **Argo ordering.** Verify how Argo handles the webhook rollout (the single webhook replica) when the pki and payments apps sync. Run the upgrade in a window with no pending Certificate changes.
7. **Metrics/alerts.** Prometheus is disabled in values, so there are no cert-manager metrics. Confirm what alerts you rely on to detect failed renewals; there may be none in this repo.
8. **Decide the gate stance.** Either set `rotationPolicy: Never` explicitly on the two leaf Certificates to keep today's behaviour, or accept `Always` (which is the upstream direction). Setting `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false` is only a short-term workaround. The upgrade guide says the gate becomes GA in 1.19 and can no longer be turned off. Whichever you choose, make it explicit in the manifests before 1.19.
9. **Rollback.** No downgrade path is documented in the supplied material. Since CRDs are kept (`crds.keep: true`) and 1.18 adds only optional fields, rollback to 1.17.2 looks feasible, but I could not verify it. Take a backup of cert-manager resources and Secrets (`kubectl get -A cert-manager.io,acme.cert-manager.io -o yaml`) beforehand.

## Limits of this review

- I worked from repo files and the supplied upstream documents only, with no cluster access. Everything about runtime behaviour (reissue timing, nginx reload, pinning) is inferred.
- The enum values in the manifests are stored as redacted digests with explanatory comments. I could not recompute them.
- `rendered/to.yaml` shows `selfsigned-bootstrap` without a `spec`, while the source has `selfSigned: {}`. This looks like the renderer dropping an empty map rather than a real difference. It is identical in both renders, so it is not an upgrade effect.
- The advisory page is saved HTML. I read the affected-versions table and the description but not its full proof-of-concept.
