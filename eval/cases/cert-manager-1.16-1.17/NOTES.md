# Research notes — cert-manager 1.16 → 1.17

Authored blind on 2026-10-01 from the upstream documents below; the Release
Intelligence pipeline was not run before the expectations were written.

## Sources read

- Upgrading 1.16 → 1.17
  (website/content/docs/releases/upgrading/upgrading-1.16-1.17.md): exactly
  three numbered notes — RSA hash selection, structured logging, ValidateCAA
  deprecation. All three became expectations (E1, E5, E2).
- Release notes 1.17
  (website/content/docs/releases/release-notes/release-notes-1.17.md):
  Major Themes — OperatorHub (already discontinued at 1.16, covered by the
  cert-manager-1.15-1.16 case, not repeated), RSA compliance (E1), keystore
  passwords (E4), feature-gate promotions/deprecation (E2/E3).
- Chart values at v1.16.0 and v1.17.0 (deploy/charts/cert-manager/values.yaml):
  diffed to ground the environment fixture. The diff removes no values keys
  and changes no defaults — the feature-gate list changes live only in
  comments (`#      ValidateCAA: true` → `#      ValidateCAA: false # ALPHA -
  default=false`). Hence the environment's expected findings are limited to
  the cluster-version check, with explicit notExpectedFindings for
  values-removed and crd-removed.

## Judgement calls

- E1 classification is review-required, not action-required: the upstream
  guide itself says "we don't expect this to be an issue, but it's worth
  bearing in mind" — evidence of intersection (the fixture issues 4096-bit
  RSA) without deterministic proof the verifier accepts SHA-512.
- E2 is action-required for operators who enable the gate; the fixture does
  (`featureGates: "ValidateCAA=true"`), and the guide's advice is to stop
  before 1.18 removes it.
- E5 kept minor/informational: it breaks only log-string consumers, which
  the guide itself calls "highly discouraged".
- The join cannot see any of E1/E2/E5 (note-derived changes with no values
  or CRD subjects): the expectedImpact links for E1 (review) and E2
  (action-required) are expected to MISS until the join grows a rule for
  operator-flag-shaped configuration. Recorded here as the honest
  expectation; a miss is a measurement of the join, not of the dataset.
