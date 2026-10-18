# cert-manager-1.17-1.18--registry-apac — notes (ground truth; not for systems under test)

Not paired. The setting that decides B1 is applied through a layer the environment snapshot can only
partly see: an admission-time Kyverno mutation. The repository contains the policy, but no rendered
manifest contains its effect.

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

- Kyverno mutate rules apply at admission; the `+(...)` anchor adds a field only if it is absent
  ([kyverno/website `f48a3636` `content/en/docs/writing-policies/mutate.md` L16, L238, L281](https://github.com/kyverno/website/blob/f48a36366d98cf093fec7849aad37565a03bac90/content/en/docs/writing-policies/mutate.md#L281):
  > One of the most common scenarios when mutating resources is to add/change a field to a specified value
  > only if it is not already present. This is done by using the **add** anchor ... with the notation
  > `+(...)` for the tag.

  The mutated object is what the API server stores, so the field persists independently of later
  admission.

## Reasoning in terms of the fixture

- `platform/pki/build-root-ca.yaml`: root CA, `isCA: true`, 2y, **no** `rotationPolicy` in the manifest.
- `platform/policies/pki-ca-keys.yaml`: ClusterPolicy, rule on `cert-manager.io/v1/Certificate`,
  precondition `request.object.spec.isCA == true`, `patchStrategicMerge` `spec.privateKey.+(rotationPolicy):
  Never`. Sync waves: Kyverno −2, cert-manager −1, policies 0, PKI 1, apps 2 — the policy is active
  before the CA is created (and Kyverno refuses a policy for a kind whose CRD does not exist yet, so it must
  come after cert-manager; the first fixture draft had the order the other way round and its policy was
  rejected — corrected before any outcome was recorded).
- The stored CA therefore carries `rotationPolicy: Never`, and the changed default never reaches it.
  `artifacts-tls` and `builder-tls` are not CAs: they rotate their keys from v1.18.0, which is harmless
  (verified through the CA). **B1 is not affected.** A system reading only rendered manifests sees an
  unpinned CA and cannot clear this without understanding the policy; a system that cannot decide must not
  clear it — but the ground truth is not affected.
- B2: no `revisionHistoryLimit` anywhere; garbage collection to one; nothing reads CertificateRequests.
  Not affected.

## Observed outcome

`observed/run.sh` (kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1; Kyverno chart 3.3.7 = Kyverno
v1.13.4 with the environment's values: admission controller only), log `observed/run.log`. Argo CD
sync waves reproduced in order. Renewals as in the other cert-manager cases.

| Step (run.log) | Root CA key | Calls builder→artifacts |
|---|---|---|
| repository manifest vs stored object (L120-L131) | manifest: no `rotationPolicy`; stored: `{"algorithm":"RSA","rotationPolicy":"Never","size":3072}` | |
| baseline v1.17.2 (L134) | `13e250e0…` | OK |
| control v1.17.2: CA and artifacts renewed (L143) | kept | OK |
| upgrade to v1.18.0, no renewal (L162, L203) | unchanged | OK |
| v1.18.0: CA renewed (L212) | **kept** (new serial) | OK |
| v1.18.0: artifacts renewed + restarted (L223) | kept; artifacts key **new** (`a0ae9afb…`) | OK |
| v1.18.0: builder renewed (L240) | kept; builder key **new** (`e3a26612…`) | OK |
| v1.18.0: CA renewed again (L252) | **kept** | |

B2: CertificateRequests 2 → 1 (CA, artifacts) after the upgrade (L163-L201).

`observed/render.diff`: cert-manager release FROM/TO (same values as the other Argo cases); no breaking
rendered change. Kyverno is not upgraded.
