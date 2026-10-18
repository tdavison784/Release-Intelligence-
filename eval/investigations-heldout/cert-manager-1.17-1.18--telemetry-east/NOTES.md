# cert-manager-1.17-1.18--telemetry-east — notes (ground truth; not for systems under test)

Pair `cert-manager-1.17-1.18--telemetry-pki` with `--telemetry-west`. The two environments differ only
in **where** `rotationPolicy: Never` is set: here on the three workload certificates (`ingest-tls`,
`gateway-tls`, `rollup-tls`), in `--telemetry-west` on the root CA `telemetry-root-ca` only.

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

- `infrastructure/pki/telemetry-root-ca.yaml`: the root CA, `isCA: true`, 1y, no `rotationPolicy`.
  `telemetry-ca` (ClusterIssuer, CA type) signs with the key in Secret `cert-manager/telemetry-root-ca`.
- `apps/telemetry` (`ingest`, nginx: `ssl_client_certificate /tls/ca.crt`, `ssl_verify_client on`) and
  `apps/edge` (`gateway`, `rollup`: `curl --cacert /tls/ca.crt --cert ...`) all trust the CA certificate
  from their **own** Secret. Their certificates (90d, renewBefore 360h) set `rotationPolicy: Never`.
- On v1.18.0 the root CA's next renewal (11 months after issuance) generates a new CA key (default
  `Always`). Each workload certificate picks up the new CA (new signature, new `ca.crt`) at its own next
  renewal, up to 75 days later. During that window, renewed and not-yet-renewed workloads no longer trust
  each other: a renewed server presents a certificate the old `ca.crt` cannot verify, and it verifies
  clients against the new CA, rejecting not-yet-renewed client certificates.
- The `rotationPolicy: Never` on the workload certificates looks like the documented mitigation but is on
  the wrong objects: it keeps the *workload* keys, which nothing depends on; the certificates are still
  re-signed by the new CA key. **B1 is affected** (component: the root CA).
- B2: nothing reads CertificateRequest history (no audit job, no tooling in the repository). Not affected.

## Observed outcome

`observed/run.sh` (kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1; charts from charts.jetstack.io),
log in `observed/run.log`. Flux is reproduced in dependency order (helm with the HelmRelease's inline
values, then `kubectl kustomize | kubectl apply` per Kustomization). Renewals are triggered the way
`cmctl renew` does (Issuing condition), standing in for the scheduled renewal at `renewBefore`; after a
renewal the server restarts (any reschedule or rollout loads the renewed files) and clients are checked
once the kubelet has refreshed their Secret volume.

| Step (run.log) | Root CA key | Calls gateway→ingest, rollup→ingest |
|---|---|---|
| baseline v1.17.2 (L109) | `84cd9a86…` | OK, OK |
| control v1.17.2: CA renewed, ingest renewed + restarted (L122) | `84cd9a86…` (kept, new serial) | OK, OK |
| upgrade to v1.18.0, no renewal (L145, L194) | unchanged | OK, OK |
| v1.18.0: CA renewed (L207) | **`5b987f8f…` (new)** | OK, OK (nobody re-issued yet) |
| v1.18.0: ingest renewed + restarted (L222) | new | **FAILED** `curl: (60) unable to get local issuer certificate`; with `-k` ingest answers **HTTP 400** to the client certificate; both callers |
| v1.18.0: gateway renewed (L263) | new | gateway OK, **rollup FAILED** |
| v1.18.0: rollup renewed (L286) | new | OK, OK (all peers on the new CA) |

The workload keys stayed the same throughout (their `Never`), which confirms that the explicit setting
was honoured and did not help. B2: CertificateRequests per Certificate went from 2 to 1 right after the
upgrade (L146-L186); issuance and calls unaffected.

`observed/render.diff` is the environment's FROM/TO render of the cert-manager release (helm template
with the HelmRelease values, canonicalised per object; index in `render-index.txt`). Kustomize sources
are unchanged by the upgrade. No rendered change is breaking; the only item-related hunks are the CRD
description updates for `rotationPolicy` (B1) and `revisionHistoryLimit` (B2).
