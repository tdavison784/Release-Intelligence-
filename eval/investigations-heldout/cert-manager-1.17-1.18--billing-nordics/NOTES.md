# cert-manager-1.17-1.18--billing-nordics — notes (ground truth; not for systems under test)

Not paired. Every workload certificate is exposed to the new default (no `rotationPolicy`, one explicit
`Always`), and there is no CA Certificate for cert-manager to re-issue: the trust anchor is managed
outside cert-manager. The rotation happens and is harmless.

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

- `pki/billing-ca-issuer.yaml`: ClusterIssuer `billing-ca`, CA type, `secretName: billing-ca-keypair`.
  That Secret is written by external-secrets from Vault (`pki/billing-ca-keypair.yaml`, ExternalSecret,
  `template.type: kubernetes.io/tls`); there is no cert-manager `Certificate` for the CA, so cert-manager
  never re-issues it and its key does not change with the upgrade (it changes only when the security team
  rotates it in Vault — independent of cert-manager).
- `apps/invoicing`: `invoicing` (nginx, client certificates verified against `/tls/ca.crt`;
  `rotationPolicy: Always` explicitly, `revisionHistoryLimit: 3`), callers `dunning` and `ledger-export`
  (no `rotationPolicy`, no `revisionHistoryLimit`).
- From v1.18.0 the callers get a new key on each renewal; nothing depends on a workload key (all peers
  verify through the stable billing CA). **B1 is not affected.**
- B2: `invoicing-tls` keeps 3 CertificateRequests (explicit); the callers' history is garbage collected to
  one; nothing reads it. Not affected.

## Observed outcome

`observed/run.sh` (kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1), log `observed/run.log`. The deploy
pipeline is reproduced: the helmfile release `cert-manager` (helm with `values/cert-manager.yaml` then
`values/cert-manager-prod.yaml`), then `kubectl apply -k clusters/prod`. **Substitution**: external-secrets
and its Vault are not reproduced; the Secret they maintain (`cert-manager/billing-ca-keypair`, type
`kubernetes.io/tls`, `tls.crt` + `tls.key`, no `ca.crt`) is created directly from a P-384 CA generated for
the run, and the two external-secrets objects are left out of the apply. cert-manager only reads that
Secret, so the substitution does not touch the behaviour under test. Renewals as in the other
cert-manager cases.

| Step (run.log) | CA key | invoicing / dunning / ledger-export keys | Calls dunning→invoicing, ledger-export→invoicing |
|---|---|---|---|
| baseline v1.17.2 (L93) | `ceb6f732…` | `fb3bfe27…` / `06e0d878…` / `6604b9ed…` | OK, OK |
| control v1.17.2: invoicing (restart), dunning renewed (L106) | same | invoicing **new** (explicit `Always`) / dunning kept (old default) | OK, OK |
| upgrade to v1.18.0, no renewal (L133, L174) | same | unchanged | OK, OK |
| v1.18.0: invoicing renewed + restarted (L187) | same | invoicing new | OK, OK |
| v1.18.0: dunning renewed (L211) | same | dunning **new** (`d004ff51…`) | OK, OK |
| v1.18.0: ledger-export renewed (L227) | same | ledger-export **new** (`6046bc69…`) | OK, OK |

B2: before the upgrade 2/2/1 CertificateRequests (dunning/invoicing/ledger-export), after 1/2/1, at the
end 1/3/1 (L134-L172, L242-L245).

`observed/render.diff`: cert-manager release FROM/TO with both value files (prometheus enabled, two
replicas, PDB): the same harmless changes as the other cert-manager cases plus the metrics Service's
`targetPort` becoming the named port `http-metrics`. No breaking rendered change.
