# Score — P-EU C-item r1

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | n/a (out of C-item scope) |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "so DefaultPrivateKeyRotationPolicyAlways stays at its default (enabled in 1.18)."
- R2 met: "which sets only duration, renewBefore and subject"
- R3 met: "Between the two renewals, one side presents a certificate the other does not trust."
- R4 met: "add privateKey.rotationPolicy: Never explicitly to all three Certificates (at minimum the root CA)"

## B2 revisionHistoryLimit: n/a (out of C-item scope)

**Overall recommendation:** "The change affects this environment, and it can break payments mTLS."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As C-whole.
