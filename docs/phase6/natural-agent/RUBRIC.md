# Phase 0 scoring rubric (frozen with PROTOCOL.md, before any run)

**Label-bearing. Lanes and investigators never read this file.** Ground truth: each case's `expected.yaml` and
`NOTES.md` (authored blind by the Phase 4/5 groundtruth lanes, confirmed by observed kind upgrades). The rubric turns
it into checkable criteria, so a score cannot rest on how convincing an answer sounds.

## Outcome per mechanism (one of)

| outcome | meaning |
|---|---|
| evidence-backed correct | the class matches, and the required elements (R…) are each stated with evidence from the workspace: a file path, a render line or an upstream quote |
| correct but unsupported | the class matches, but one or more required elements are missing, asserted without evidence, or taken from model knowledge where the workspace had the evidence |
| appropriately uncertain | expected not-affected or affected, and the answer is "potentially affected / cannot confirm", **and** it states every R-element that is decidable from the workspace correctly, **and** its residual uncertainty is exactly the one the case leaves open (listed per case as U) |
| hedged | potentially affected / insufficient, without meeting "appropriately uncertain" |
| false-safe | says not affected (or "safe", "no action") where expected affected |
| false-alarm | says affected where expected not-affected |
| missed | the mechanism is not discussed at all |

For arm B, the class is the agent verdict for the matched item, and R-elements are read from its mechanism /
environmentReasoning / claims. For arm A, the class is the policy class; A gets "hedged" or a class match only (its
reasoning is the policy record, not prose).

Also recorded per run (not part of the outcome):
- unsupported claims stated as fact;
- consumers identified, by name;
- upstream changes outside scope that are claimed affected (listed, not graded);
- the overall recommendation;
- validation recommended for each residual uncertainty.

## P-US — cert-manager 1.17.2 → 1.18.0, payments-us

**B1 rotationPolicy — expected not-affected**
- R1 mechanism: the 1.18 default changes from `Never` to `Always`. A Certificate that leaves
  `spec.privateKey.rotationPolicy` unset gets a new private key on every re-issuance (and the
  `DefaultPrivateKeyRotationPolicyAlways` feature gate is not turned off in `platform/cert-manager/values.yaml`).
- R2 effective CA setting: the root CA `cert-manager/mesh-root-ca` resolves to `rotationPolicy: Never` through the prod
  overlay patch `platform/pki/overlays/prod/mesh-root-ca-prod.yaml`, the overlay Argo CD deploys
  (`clusters/prod/pki.yaml`). Reading the base alone (unset) is an R2 failure.
- R3 leaves: `payments/checkout-tls` and `payments/ledger-tls` leave it unset, so their keys **do** rotate under 1.18.
- R4 consumers: checkout and ledger trust the CA (`ca.crt` from their own Secret) and verify peers through the chain;
  nothing pins a leaf key. So leaf-key rotation is not observable, and the CA key, which they depend on, is stable.
- U (allowed residual uncertainty): whether anything outside the repository pins a leaf key or copies a leaf
  certificate.
- "The CA is pinned, so everything is safe" without R3+R4 is **correct but unsupported**. So is "leaves rotate, but
  that's fine" without the reason (R4).

**B2 revisionHistoryLimit — expected not-affected**
- R1: the default becomes `1`, so only the latest CertificateRequest per Certificate is kept.
- R2: nothing in the environment reads CertificateRequest history (no tooling, policy or audit job references it).
- U: consumers outside the repository.

## P-EU — same upgrade, payments-eu

**B1 rotationPolicy — expected affected**
- R1 as P-US.
- R2 effective CA setting: `mesh-root-ca` leaves `rotationPolicy` unset in **both** base and prod overlay. So under
  1.18 the CA's next re-issuance (renewBefore 720h) generates a new CA key.
- R3 consequence: leaves re-issued afterwards are signed by the new CA key. A peer still holding the previous
  `ca.crt` rejects them, so checkout→ledger mTLS breaks (ledger verifies client certs against `/tls/ca.crt` from its
  own Secret, and checkout verifies ledger's server certificate the same way).
- R4 remediation: set `rotationPolicy: Never` on `mesh-root-ca` before upgrading (or plan a CA rotation with an
  overlapping trust bundle, or turn the feature gate off).
- "Potentially affected" that states R1–R3 and leaves only the timing of re-issuance open counts as **evidence-backed
  correct** (the environment is affected; when it bites is operational). "Affected" because "every Certificate's key
  will rotate", without R3's CA-trust mechanism, is **correct but unsupported**.

**B2 revisionHistoryLimit — expected not-affected**: as P-US B2.

**Distinguishing P-US from P-EU** (C-whole only, across the paired runs with the same repeat index): the assessments
must name the CA's effective `rotationPolicy` as the deciding difference: P-US's prod overlay patch
`mesh-root-ca-prod.yaml` sets `Never`, while P-EU's patch file of the same name exists but does not set it.

## D-EU — ingress-nginx chart 4.11.5 → 4.12.0 (controller 1.11 → 1.12), docs-eu

All four expected items fall in upstream change "Enable security features by default (#11819)".

**B3 strict path validation — expected affected** (the Phase 5 false-safe)
- R1 mechanism: 1.12 turns on strict path validation by default (`strict-validate-path-type`). For `Exact`/`Prefix`
  paths, only alphanumerics, `-`, `_` and `/` are accepted.
- R2 environment: the docs Ingress declares `/.well-known` with `pathType: Prefix`. It contains a dot, so it fails the
  check.
- R3 consequence: the check runs in the admission webhook. The stored Ingress keeps being served, but every
  create/update of it is rejected after the upgrade, so the next GitOps sync or change of that Ingress fails.
- R4 remediation: set `controller.config.strict-validate-path-type: "false"`, or use `pathType: ImplementationSpecific`
  for that path, or route the exact well-known documents with `Exact` paths that pass the check.
- If the workspace's upstream text does not state the accepted character set, an answer that correctly says "a dot
  may be rejected; I cannot confirm the accepted character set from the available documents" is **appropriately
  uncertain**. An answer that states the set from memory is **correct but unsupported** (flag: model knowledge).
  "Paths look valid, so not affected" is **false-safe**.

**B1 annotations risk level — expected not-affected**
- R1: 1.12 defaults `annotations-risk-level` to `High`, which rejects Critical annotations.
- R2: the only ingress-nginx annotation in use is `ssl-redirect` (below High); no snippet or other Critical
  annotation.

**B2 cross-namespace resources — expected not-affected**
- R1: cross-namespace references in annotations are disallowed by default.
- R2: the Ingress references only Services in its own namespace, and no annotation refers to a Secret or ConfigMap in
  another namespace.

**B4 annotation validation — expected not-affected**
- R1: annotation value validation is on by default.
- R2: the annotation values in use (`ssl-redirect: "false"`) are valid.

## Scoring record format

Per run, in `runs/<case>/<arm>-r<n>/score.md`:
- the outcome of each item;
- the R-elements met, missed or wrong, each with a quote from `assessment.md`;
- U handled (yes/no);
- unsupported claims;
- consumers named;
- the overall recommendation;
- one line on what it did differently from arm B, with transcript references.
