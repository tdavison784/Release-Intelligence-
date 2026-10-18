# Score — P-US C-item r2

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | n/a (out of C-item scope) |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "So DefaultPrivateKeyRotationPolicyAlways is on by default, as the upstream notes say."
- R2 met: "show the final merged object with rotationPolicy: Never in both"
- R3 met: "After: a new ECDSA P-256 key is generated on every reissue."
- R4 met: "It checks client certificates against ca.crt only. It does not pin the key or certificate of either peer."
- U met: "check whether anything outside this repo reads those Secrets or pins their keys"

## B2 revisionHistoryLimit: n/a (out of C-item scope)

**Overall recommendation:** "I found nothing that depends on those keys staying stable, so I rate this low risk"

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As C-item r1.
