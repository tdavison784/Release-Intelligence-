# cert-manager v1.17.2 → v1.18.0: impact assessment for the payments prod cluster

## Verdict

**Not safe to upgrade as-is.** One documented breaking change does affect this environment, and the rendered diff hides it.

The new default `Certificate.spec.privateKey.rotationPolicy: Always` applies to all three Certificates here (`mesh-root-ca`, `ledger-tls`, `checkout-tls`). None of them sets `rotationPolicy`. The effect on the root CA is the dangerous part: the next renewal of `mesh-root-ca` will produce a new root key, where today it reuses the old one. That can break mutual TLS between `checkout` and `ledger` until both have been reissued and `ledger` has been restarted.

The fix is small. Pin `rotationPolicy: Never` explicitly on every Certificate before or with the upgrade. Everything else in the release is either not exercised here or low risk.

Nothing breaks at upgrade time. The change bites at the next renewal of each Certificate, so it is a delayed failure that a post-upgrade smoke test won't catch.

## Affected

### 1. `rotationPolicy` default flips from `Never` to `Always` (breaking, affects us)

**Evidence**
- Release notes and the upgrade guide (`upstream/.../release-notes-1.18.md`, `upgrading-1.17-1.18.md`) document the flip. The feature gate `DefaultPrivateKeyRotationPolicyAlways` is on by default.
- `rendered/manifest.diff` shows the new CRD description for `rotationPolicy`: "Default is `Always`... disabled by `--feature-gates=DefaultPrivateKeyRotationPolicyAlways=false`".
- `env/platform/cert-manager/values.yaml` sets no `config.featureGates`, so the gate stays at its new default of on.
- The controller args in the rendered diff are unchanged apart from the image tags, so nothing else turns the gate off.
- A grep of `env/` finds no `rotationPolicy` on any Certificate:
  - `platform/pki/base/mesh-root-ca.yaml` (overlay `mesh-root-ca-prod.yaml` patches only duration, renewBefore and subject)
  - `apps/payments/base/ledger-certificate.yaml`
  - `apps/payments/base/checkout-certificate.yaml`

**Why the root CA is the dangerous case**
- `mesh-root-ca` is a self-signed CA (`isCA: true`) with a 1y duration in prod and `renewBefore: 720h`.
- The `mesh-ca` ClusterIssuer (`ca.secretName: mesh-root-ca`) signs both leaf certs.
- Today, a renewal reuses the same key and subject. The new root cert still validates leaves signed under the old one, so renewal is invisible.
- After the upgrade, a renewal generates a new root key. Leaves signed by the old key do not validate against the new root.
- The `ca.crt` that cert-manager writes into each leaf Secret is the issuing CA at issuance time. It does not carry the old root as an overlap.
- `ledger-tls` and `checkout-tls` renew independently, on a 720h duration with `renewBefore` 240h. Their renewal times are staggered and unrelated to the root's.
- After a root rotation, one side will hold a `ca.crt` for the new root while the other still has a leaf from the old root:
  - `checkout` verifies ledger with `--cacert /tls/ca.crt` (`checkout-deployment.yaml`).
  - `ledger` verifies clients with `ssl_client_certificate /tls/ca.crt` and `ssl_verify_client on` (`ledger.conf`).
  - The mismatch fails the handshake in both directions.
- Existing leaf certs are not re-signed when the root changes. The mismatch lasts until each leaf renews, which can be up to roughly 30 days.

**Runtime gap that makes this worse**
- `ledger` runs nginx with `ssl_certificate`, `ssl_certificate_key` and `ssl_client_certificate` read from a mounted Secret.
- The Secret volume updates on disk, but nginx reads these files at startup and reloads only on SIGHUP.
- Nothing in the repo triggers a reload or restart: no reloader annotation, no sidecar, no checksum annotation.
- So `ledger` keeps serving its old leaf and trusting its old CA after any renewal, until its pods restart.
- `checkout` is a `curl` loop that re-reads the files on every call, so it picks up new files immediately.
- This is a pre-existing weakness: the 30-day leaf would already expire in a running pod. Key and root rotation make it fail in a new way (CA mismatch).
- I could not verify from the repo how pods are actually restarted in practice.

**Leaf key rotation alone** is low risk. Each leaf's key lives in its own Secret and the cert is reissued together with it. The remaining exposure is the nginx reload gap above.

**Mitigation, in order of preference**
1. Add `privateKey.rotationPolicy: Never` to all three Certificates, in the base manifests. This restores today's behavior with no feature-gate dependency.
   - It must be done before 1.19, where the gate goes GA and cannot be turned off.
   - Merge it before or together with the version bump.
2. Optionally, as a stopgap, set `config.featureGates.DefaultPrivateKeyRotationPolicyAlways: false` in `values.yaml`. This has an expiry date of 1.19.
3. Longer term, design root rotation deliberately instead of relying on defaults:
   - Use an overlapping trust bundle (for example trust-manager, not present in this repo).
   - Make `ledger` reload on Secret change.
   - Only then adopt `Always`.

### 2. `revisionHistoryLimit` default `nil` → `1` (potentially breaking, low impact here)

- The release notes say all stale `CertificateRequest` objects are garbage collected on upgrade unless `revisionHistoryLimit` is set. None of our Certificates sets it.
- Effect: on upgrade, old CertificateRequests for the three Certificates are deleted, keeping the latest one.
- This only matters if some audit or tooling reads historical CertificateRequests. Nothing in `env/` does.
- Cert issuance is not affected.
- Worth checking whether PKI audit or compliance practice for the payments platform relies on CertificateRequest history. This is the only "does anything outside the repo care" question.

### 3. RBAC rename (observable in the diff, low risk)

- ClusterRole `cert-manager-controller-challenges` and its binding are removed. The permissions are split into `cert-manager-dns01-controller-challenges` and `cert-manager-http01-controller-challenges`.
- Both new roles are bound to the same ServiceAccount.
- The `cert-manager-edit` role gains `update` on its last rule.
- Both Argo CD apps have `prune: true`, so the old role will be pruned. The repo declares nothing else that references the old name.
- The roles only support ACME Challenge handling, which this environment doesn't use (see "Not affected"). A grep of `env/` finds no reference to the old role name, but cluster-external bindings can't be ruled out from here.

## Not affected, and why

| Change | Why it does not apply here |
|---|---|
| HTTP-01 Ingress `PathType: Exact` (ingress-nginx and OpenShift caveats) | No ACME issuer, no HTTP-01 solver, no Ingress or Route in `env/`. The only issuers are `selfSigned` and `ca`. |
| ACME profiles, Vault `serverName`, Venafi URL default, Cloudflare and Route53 fixes | Fields and code paths for issuers we don't define. They appear as CRD additions only. |
| New `signatureAlgorithm` field | New optional field. We don't set it and the default is unchanged. |
| `iss`/`ciss` shortNames | Additive. Only matters if another CRD in the cluster claims the same short names, which I can't see from the repo. |
| IP-address `commonName` fix | Our common names are DNS (`ledger.payments.svc`, `checkout.payments.svc`, `payments-mesh-root-ca`). |
| `AdditionalCertificateOutputFormats` GA, `UseDomainQualifiedFinalizer` GA, `ValidateCAA` removed | We don't use these. The CRD diff only drops "Beta feature gate" wording. |
| Webhook probes `port: 6080` → `port: healthcheck` | `rendered/to.yaml` still names containerPort 6080 `healthcheck`, so the probe resolves. Rendered, not tested. |
| `global.rbac.disableHTTPChallengesRole`, `--extra-certificate-annotations`, new metrics | Opt-in or off. Prometheus is disabled in `values.yaml`. |
| OperatorHub discontinuation | We install from the Helm chart. |
| `values.yaml` keys | Only `crds.enabled`, `crds.keep`, `replicaCount`, `prometheus.enabled`, `resources`, `webhook.replicaCount` and `cainjector.replicaCount` are set. I did not diff chart `values.yaml` between versions line by line. |
| GHSA-8rvj-mm4h-c258 (direct Challenge resources abuse ClusterIssuer DNS01 credentials) | The advisory requires an ACME ClusterIssuer with DNS01 credentials. We have none. |

The advisory GHSA-8rvj-mm4h-c258 still matters for version choice:
- It lists affected versions as >= 1.18.0 and <= 1.20.2, with patches only in 1.19.6 and 1.20.3. There is no 1.18.x fix.
- It was reproduced on v1.18.6.
- So the 1.18 line has no fix for it. Irrelevant for us today, but if ACME DNS01 is added later it is a reason to target a patched 1.19.x or 1.20.x rather than 1.18.

## Not proven by the unchanged manifests

- **The rendered diff cannot show the main risk.** Defaults live in controller code. The only trace in the diff is a CRD description change. The three Certificates are byte-identical before and after, and that identity is exactly why the change is invisible in review.
- **Certificates are not re-issued at upgrade time.** The break appears at the next renewal of each cert, so a smoke test right after sync will pass.
- **The rendered chart tells us what is declared, not how the controller will behave.** It does not show how the root CA key and `ca.crt` propagate at renewal.

## Missing information and tests before proceeding

1. **Live state:**
   - `notAfter` and renewal time of `mesh-root-ca`, `ledger-tls`, `checkout-tls`. This tells you when the first rotation under the new default would hit if the pin were missed.
   - Confirm no `rotationPolicy` is set on live objects, and that the live controller args match `from.yaml`.
2. **Consumers of the CA outside this repo:**
   - Anything else that reads `cert-manager/mesh-root-ca` or a `ca.crt` from these Secrets, or pins the root.
   - Other namespaces issuing from `mesh-ca`.
   - The `payments` namespace is the only consumer visible here.
3. **How `ledger` picks up new certs:** whether anything restarts or reloads it on Secret change. Nothing in the repo does.
4. **Staging rehearsal on v1.18.0:**
   - Without the pin, run `cmctl renew mesh-root-ca` and confirm the new key and the `checkout`/`ledger` handshake failure. This verifies the analysis above.
   - With `rotationPolicy: Never` pinned, repeat the renewal and confirm the root key is unchanged and mTLS keeps working.
   - Renew a leaf in each state and confirm the `ledger` reload behavior.
5. **After upgrade (prod or staging):**
   - Webhook becomes Ready with the named probe port.
   - `startupapicheck` job completes.
   - CRDs updated cleanly under ServerSideApply with `crds.keep: true`.
   - The ClusterIssuers `mesh-ca` and `selfsigned-bootstrap` show Ready.
6. **Kubernetes 1.31 support for cert-manager 1.18:** not covered by the supplied documents. Confirm against upstream's supported-releases page.
7. **Audit dependency on CertificateRequest history** (item 2 under "Affected").
8. **Version choice:** v1.18.0 has since been followed by 1.18.1 through 1.18.6 (documented in the release notes). 1.18.1 adds the `ACMEHTTP01IngressPathTypeExact` gate and an authorization-timeout fix. 1.18.5 and 1.18.6 contain security fixes. If staying on 1.18, target 1.18.6, not 1.18.0, unless the intent is to test the 1.18.0 diff specifically.

## Recommended path

1. Land a change adding `privateKey.rotationPolicy: Never` to `mesh-root-ca`, `ledger-tls` and `checkout-tls`. This is a no-op on 1.17.2.
2. Run the staging rehearsal in item 4, then upgrade (preferably to 1.18.6) with a normal Argo sync.
3. Separately, plan the root rotation and trust-distribution design, and add a reload mechanism for `ledger`, before ever moving to `Always`. Do this before 1.19 makes `Always` unavoidable for any Certificate that doesn't pin `Never`.

*No files in `env/` were modified.*
