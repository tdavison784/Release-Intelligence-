# cert-manager-1.17-1.18--telemetry-west — notes (ground truth; not for systems under test)

Pair `cert-manager-1.17-1.18--telemetry-pki` with `--telemetry-east`. The two environments differ only
in **where** `rotationPolicy: Never` is set: here on the root CA `telemetry-root-ca` only, in
`--telemetry-east` on the three workload certificates only.

## Upstream sources

- Release notes 1.18, website commit `2e32a5ef`,
  [`release-notes-1.18.md#L109-L148`](https://github.com/cert-manager/website/blob/2e32a5efc7a4ef284e271f54ac1b15e800e66fcc/content/docs/releases/release-notes/release-notes-1.18.md#L109-L148):
  > We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`. (L113)
  >
  > Users who want to preserve the old default have two options: 1. Explicitly set `rotationPolicy: Never` on your Certificate resources. 2. Turn off the feature gate in this release ... (L122-L124)
- API doc, cert-manager `v1.18.0` (`83dae3ef`),
  [`pkg/apis/certmanager/v1/types_certificate.go#L330-L345`](https://github.com/cert-manager/cert-manager/blob/v1.18.0/pkg/apis/certmanager/v1/types_certificate.go#L330-L345):
  > If set to `Never`, a private key will only be generated if one does not already exist in the target `spec.secretName`. ... If set to `Always`, a private key matching the specified requirements will be generated whenever a re-issuance occurs. Default is `Always`.
- The default is applied at runtime by the controller, not stored in the object:
  [`internal/apis/certmanager/v1/defaults.go#L52-L63`](https://github.com/cert-manager/cert-manager/blob/v1.18.0/internal/apis/certmanager/v1/defaults.go#L52-L63)
  (`SetRuntimeDefaults_Certificate`: empty `RotationPolicy` → `Always` while the
  `DefaultPrivateKeyRotationPolicyAlways` gate is on, default true, Beta), called by the key manager
  ([`pkg/controller/certificates/keymanager/keymanager_controller.go#L158-L192`](https://github.com/cert-manager/cert-manager/blob/v1.18.0/pkg/controller/certificates/keymanager/keymanager_controller.go#L158-L192)):
  `Never` → reuse the key in the Secret, `Always` → generate a new one. So the stored Certificate keeps
  showing no `rotationPolicy`; nothing in a render changes for it (the CRD only changes its description,
  `render.diff#L703-L719`).
- CA issuer: the issued Secret's `ca.crt` is the CA certificate from the issuer's Secret
  ([`pkg/controller/certificaterequests/ca/ca.go#L82-L132`](https://github.com/cert-manager/cert-manager/blob/v1.18.0/pkg/controller/certificaterequests/ca/ca.go#L82-L132):
  `CA: bundle.CAPEM`). A workload's trust anchor therefore only changes when *its own* certificate is
  re-issued.
- `revisionHistoryLimit`: [`release-notes-1.18.md#L151-L160`](https://github.com/cert-manager/website/blob/2e32a5efc7a4ef284e271f54ac1b15e800e66fcc/content/docs/releases/release-notes/release-notes-1.18.md#L151-L160):
  > When you upgrade to cert-manager 1.18, all stale `CertificateRequest` resources will be garbage collected, unless you explicitly set the `revisionHistoryLimit` value on your `Certificate` resources.

Why the environment cannot use the feature gate instead: cert-manager v1.17.2 does not know
`DefaultPrivateKeyRotationPolicyAlways` and refuses to start with it (probe in kind, same versions:
`failed to set feature gates from initial flags-based config: unrecognized feature gate:
DefaultPrivateKeyRotationPolicyAlways`, `../probes/featuregate-1.17.2.log`), so a pre-upgrade environment can only pin the old behaviour
per Certificate.

## Reasoning in terms of the fixture

- `infrastructure/pki/telemetry-root-ca.yaml`: the root CA, `isCA: true`, 1y, **`rotationPolicy: Never`**.
  `telemetry-ca` (ClusterIssuer, CA type) signs with the key in Secret `cert-manager/telemetry-root-ca`.
- `apps/telemetry` (`ingest`, nginx, client certificates required) and `apps/edge` (`gateway`, `rollup`)
  trust the CA certificate from their own Secret. Their certificates set no `rotationPolicy`.
- The explicit `Never` on the root CA keeps the CA key across its renewal on v1.18.0 (the changed default
  only applies to an empty field). A renewed CA certificate has the same subject and key, so certificates
  signed before and after the renewal verify against either CA certificate: no workload's `ca.crt` goes
  stale.
- The workload certificates do rotate their keys on every renewal from v1.18.0. Nothing depends on a
  workload key: peers verify through the CA, and a workload loads its new key together with its new
  certificate. **B1 is not affected** (mitigation: the explicit setting on the CA; the workload key
  rotation is harmless).
- B2: nothing reads CertificateRequest history. Not affected.

## Observed outcome

`observed/run.sh` (same script as `--telemetry-east`; kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1),
log in `observed/run.log`. Procedure as in `--telemetry-east`: Flux reproduced in dependency order;
renewals triggered like `cmctl renew` in place of the scheduled renewals; the server restarts after its
renewal; clients are checked once their Secret volume is refreshed.

| Step (run.log) | Root CA key | ingest / gateway / rollup keys | Calls gateway→ingest, rollup→ingest |
|---|---|---|---|
| baseline v1.17.2 (L109) | `8aef46b5…` | `add439c4…` / `7ae16b79…` / `5a38fe1b…` | OK, OK |
| control v1.17.2: CA and ingest renewed (L122) | kept | ingest kept (old default `Never`) | OK, OK |
| upgrade to v1.18.0, no renewal (L145, L194) | unchanged | unchanged | OK, OK |
| v1.18.0: CA renewed (L207) | **kept** (new serial) | unchanged | OK, OK |
| v1.18.0: ingest renewed + restarted (L222) | kept | ingest **`71bb6a9e…` (new)** | OK, OK |
| v1.18.0: gateway renewed (L249) | kept | gateway **`972c07ee…` (new)** | OK, OK |
| v1.18.0: rollup renewed (L265) | kept | rollup **`8dd1be32…` (new)** | OK, OK |

So the new default is in effect for the workload certificates (their keys change on v1.18.0 and did not
on v1.17.2) and is harmless; the CA keeps its key because of its explicit setting. B2: CertificateRequests
per Certificate 2 → 1 after the upgrade (L146-L186); no effect.

`observed/render.diff`: identical to `--telemetry-east` (same chart values); no breaking rendered change.
