# Research notes — cert-manager 1.16 → 1.17 (transfer: eu-platform)

Transfer environment for `cert-manager-1.16-1.17` (MISSION G12). Authored
blind on 2026-10-01 from the upstream sources below; the Release Intelligence
pipeline was not run, and the base case's fixtures/labels were read only to
inherit its items — this cluster was designed from the upgrade notes, not
from the base environment.

## Cluster design

eu-platform is a multi-tenant shared platform's staging tier on Kubernetes
1.31, deliberately different from the base case's production cluster
(Kubernetes 1.29, ValidateCAA enabled, CA-issued 4096-bit leaf, pinned
replicas):

- **Single-tier private PKI with a visible signer.** The root CA Certificate
  `platform-root-ca` (isCA, RSA-4096) is signed by its own key through a
  `SelfSigned` ClusterIssuer — the bootstrapping pattern the upstream docs
  recommend ("One of the ideal use cases for `SelfSigned` issuers is to
  bootstrap a custom root certificate for a private PKI",
  docs/configuration/selfsigned.md). Unlike the base case (where the deciding
  CA key lived in an unseen Secret), here the signer's key size is in the
  fixture itself, so the RSA-hash item is decidable.
- **Change-freeze feature-gate pinning.** The values key `featureGates`
  (chart: "A comma-separated list of feature gates that should be enabled on
  the controller pod", rendered verbatim to `--feature-gates`) pins
  `NameConstraints=false,UseDomainQualifiedFinalizer=false` — the escape
  hatch against 1.17's beta promotions. ValidateCAA is simply never enabled.
- **Keystore consumer.** The `events-broker` tenant Certificate uses JKS +
  PKCS12 keystores with the pre-existing `passwordSecretRef` mechanism (leaf
  key ECDSA, so the leaf's own key cannot decide the RSA-hash item).
- **Centralized logging.** Logs are shipped by a platform Fluent Bit
  DaemonSet whose parsers live in a ConfigMap outside this repository — the
  honest UNKNOWN for the structured-logging item.

## Sources read (all fetched 2026-10-01)

- Upgrading 1.16 → 1.17 (website upgrading-1.16-1.17.md): three notes —
  RSA hash selection, structured logging, ValidateCAA deprecation.
- Release notes 1.17 (website release-notes-1.17.md): RSA compliance
  ("cert-manager always used SHA-256 when signing with RSA"; "In v1.17.0,
  cert-manager will choose a hash algorithm based on the RSA key length:
  3072-bit keys will use SHA-384, and 4096-bit keys will use SHA-512");
  "Potentially BREAKING: The CA and SelfSigned issuers now use SHA-512 when
  signing with RSA keys 4096 bits and above"; keystore passwords ("The new
  `password` field is mutually exclusive with the `passwordSecretRef`
  field"); "two feature gates have been promoted to "beta", and as such are
  now enabled by default"; "Potentially BREAKING: Log messages that were not
  structured have now been replaced with structured logs."
- PR #7368: "The algorithm is decided by the **signer**. For SelfSigned,
  that's the same as the cert being issued obviously - but for the CA issuer
  it can be easy to forget."
- features.go at v1.16.0 and v1.17.0 (internal/controller/feature): at 1.16
  NameConstraints, UseDomainQualifiedFinalizer and ValidateCAA are all
  Alpha/Default false; at 1.17 the first two are `{Default: true,
  PreRelease: featuregate.Beta}` while `ValidateCAA: {Default: false,
  PreRelease: featuregate.Alpha}` remains registered (deprecated, removed in
  1.18). Also confirms both promoted gates exist at 1.16, so pinning them
  false on the 1.16 chart is a valid no-op pin.
- Chart values.yaml at v1.16.0/v1.17.0 and deployment.yaml template at
  v1.16.0 (`--feature-gates={{ .Values.featureGates }}`).
- CRD crd-certificates.yaml at v1.16.0 (passwordSecretRef required, no
  `password`) vs v1.17.0 ("Password provides a literal password used to
  encrypt the JKS keystore. Mutually exclusive with passwordSecretRef.").

## Link judgements

- **E1 → review (affected).** The exposure is the canonical SelfSigned
  branch of the base item's semantics: a Certificate with RSA key of
  3072/4096/8192 bits whose issuerRef resolves to an issuer with
  `spec.selfSigned` set. `platform-root-ca` satisfies it and shows the
  deciding input (its own 4096-bit key), so exposure is TRUE and the
  relevance follows the consequence's `review-required` class: after
  re-issuance every certificate in this PKI (the self-signed root and the
  leaves `tenant-ca` signs with the root key, ECDSA leaf included) carries
  SHA-512 signatures. Upstream expects minimal impact ("we're not aware of
  Kubernetes-based environments which support RSA 2048 and SHA-256 but fail
  with RSA 4096 and SHA-512") — review, not action. The base case could not
  decide this because its CA key was invisible; this cluster is the
  mirror image.
- **E2 → not-affected.** Exposure `feature-gate ValidateCAA unset` at the
  values path `featureGates` is FALSE against a supplied, managed gate list
  that omits it (the cluster touches the feature-gate area — it pins other
  gates — but never enabled the deprecated one). Deprecation warnings and
  the 1.18 removal only matter to clusters that enable it.
- **E3 → not-affected.** Exposure is the conjunction of both promoted gates
  explicitly disabled: FALSE (the escape hatch). An explicit
  `--feature-gates=...=false` overrides the new beta defaults
  (component-base semantics), so the default flip changes nothing on this
  cluster. The base case left both gates unset (informational); the freeze
  policy here is the deliberate contrast.
- **E4 → informational.** Exposure: a Certificate setting
  `spec.keystores.{jks,pkcs12}.passwordSecretRef` — TRUE (`events-broker`).
  The field is purely additive and mutually exclusive with the mechanism
  this cluster already uses, so with exposure true the relevance follows the
  consequence's informational class: a know-only "new option exists for
  you". (Deliberate choice: the exposure is spelled as "actively uses
  keystores with the existing mechanism" rather than "does not set the new
  field", which would be vacuously false for every 1.16 cluster.)
- **E5 → undecided (environment-visibility-gap).** Whether any parser greps
  literal cert-manager log strings is decided by the central Fluent Bit
  ConfigMap, which this repository does not contain; absence of the config
  cannot be negated into a clean bill. Needed: that ConfigMap
  (platform-fluent-bit-config).
- **notExpectedFindings.** Both grounded in my own artifact diffs: the
  v1.16.0→v1.17.0 values.yaml diff removes only comments (the values-removed
  rule must stay silent), and deploy/crds ships the same six CRD files at
  both tags (the crd-removed rule must stay silent).

## Base-label observations (not changed)

- The base case's F1 `why` cites a "README support table" for cert-manager
  v1.17's Kubernetes range. I could not find a version table in the v1.17.0
  README; the supported-versions table lives on the website's "Supported
  Releases" page, which today lists only the currently supported minors
  (1.20/1.21). The claim (1.29 in range) is plausible but the citation is
  not checkable at that tag — flagged for the maintainer, left untouched.
- The base values fixture's `crds: {enabled: true}` and top-level
  `featureGates` string are both chart-valid (verified against the v1.16.0
  chart), so no correction is needed there.

## Exposure direction corrections (2026-10-02, groundtruth-6; LOOP-DIAGNOSIS-2 §8.1)

E2/E3 exposures were authored as the *clearing* state (the state that makes the
link not-affected) instead of the would-be-affected state every other
not-affected link in the dataset expresses (they must evaluate FALSE on the
fixture). Inverted with the engine's own evaluation as the check; labels,
whys and evidence unchanged. See eval/CHANGELOG.md "2026-10-02 (f)".
