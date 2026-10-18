# Score — P-US C-item r3

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | n/a (out of C-item scope) |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "sets no config.featureGates, so the gate DefaultPrivateKeyRotationPolicyAlways is on by default in 1.18.0."
- R2 met: "Argo app clusters/prod/pki.yaml points at platform/pki/overlays/prod, so the overlay is what is deployed."
- R3 met: "Neither apps/payments/base/ledger-certificate.yaml nor checkout-certificate.yaml sets rotationPolicy."
- R4 met: "Nothing in env/ pins the leaf public key, SPKI hash or fingerprint"
- U met: "confirm that no other Argo apps or operators create Certificates with default policy and depend on key continuity"
- Note: Assessment recovered verbatim from the session's denied Write (harness defect H1).

## B2 revisionHistoryLimit: n/a (out of C-item scope)

**Overall recommendation:** "I rate it "safe to proceed after the checks below", not "no impact"."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As C-item r1.
