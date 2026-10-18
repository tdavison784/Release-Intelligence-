# Score — P-EU C-whole r2

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | evidence-backed correct |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "platform/cert-manager/values.yaml sets no feature gates."
- R2 met: "mesh-root-ca.yaml (including the prod patch) set no rotationPolicy."
- R3 met: "so mutual verification fails in both directions until both sides have renewed."
- R4 met: "Set spec.privateKey.rotationPolicy: Never explicitly on mesh-root-ca in platform/pki."

## B2 revisionHistoryLimit: evidence-backed correct
- R1 met: "garbage-collects old CertificateRequests down to one per Certificate"
- R2 met: "Nothing in the repo reads CertificateRequests"

**Overall recommendation:** "Do not proceed as-is."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As r1.
