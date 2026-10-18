# cert-manager-1.17-1.18--search-latam — notes (ground truth; not for systems under test)

Not paired. The environment *intends* to pin the CA key — the production PKI overlay carries a patch
"Production CAs keep their private key across renewals" — but the patch never reaches the CA.

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

- Kustomize `patches` with a `target`: the patch is applied to every resource the target selects; a
  target that selects nothing is not an error (observed: `kubectl kustomize platform/pki/overlays/prod`
  renders the root CA without `rotationPolicy`, run.log L56-L104; kustomize v5.7.1 as bundled with
  kubectl v1.34.1; reference: https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/patches/).

## Reasoning in terms of the fixture

- `platform/pki/base/search-root-ca.yaml`: root CA, `isCA: true`, 3y, no `rotationPolicy`, labelled
  `pki.example.com/tier: root`. `search-ca` (ClusterIssuer, CA type) signs with its Secret.
- `platform/pki/overlays/prod/kustomization.yaml`: `ca-key-continuity.yaml` (JSON patch
  `add /spec/privateKey/rotationPolicy Never`) targets `kind: Certificate, labelSelector:
  pki.example.com/tier=ca` — **no object carries that label**, so the patch is silently a no-op. The
  second patch (subject) applies by name and does render.
- `apps/search`: `search-api` (nginx, client certificates verified against `/tls/ca.crt`) and its callers
  `indexer`, `query-frontend` (curl with their own certificate and `ca.crt`). Workload certificates: 90d,
  no `rotationPolicy`, `revisionHistoryLimit: 2`.
- On v1.18.0 the root CA's next renewal (2.5 years after issuance) generates a new CA key; every peer
  picks up the new CA only at its own next renewal (up to 60 days later), and in between renewed and
  not-yet-renewed peers reject each other. **B1 is affected**: an explicit setting exists in the
  repository and *looks* like the documented mitigation, but it is not in the rendered object.
- B2: the workload certificates set `revisionHistoryLimit: 2` (kept); the root CA has none (garbage
  collected to one). Nothing reads CertificateRequests. Not affected.

## Observed outcome

`observed/run.sh` (kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1), log `observed/run.log`. Argo CD's
sync waves are reproduced in order (helm with the Application's chart, version and value file; kustomize
paths applied as rendered). Renewals as in the other cert-manager cases (`cmctl renew` equivalent,
standing in for the scheduled renewals; server restarted after its renewal; clients checked once their
Secret volume is refreshed).

| Step (run.log) | Root CA key | Calls indexer→search-api, query-frontend→search-api |
|---|---|---|
| rendered/stored root CA (L56-L104, L128-L133) | no `rotationPolicy` (label `tier: root`) | |
| baseline v1.17.2 (L135) | `37205639…` | OK, OK |
| control v1.17.2: CA and search-api renewed (L148) | kept; search-api key kept | OK, OK |
| upgrade to v1.18.0, no renewal (L174, L217) | unchanged | OK, OK |
| v1.18.0: CA renewed (L230) | **`031bf323…` (new)** | OK, OK |
| v1.18.0: search-api renewed + restarted (L245) | new | **FAILED** (`unable to get local issuer certificate`; `-k`: **HTTP 400**), both callers |
| v1.18.0: indexer renewed (L283) | new | indexer OK, **query-frontend FAILED** |
| v1.18.0: query-frontend renewed (L306) | new | OK, OK |

B2: root CA CertificateRequests 2 → 1 after the upgrade; workload certificates keep 2 (L175-L215,
L321-L325).

`observed/render.diff`: the cert-manager release's FROM/TO render (identical to the other Argo cases
with these values); no breaking rendered change; the kustomize overlay renders identically at FROM and
TO (the patch miss is a property of the environment, not of the upgrade).
