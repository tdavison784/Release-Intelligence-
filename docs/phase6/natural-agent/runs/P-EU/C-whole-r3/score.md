# Score — P-EU C-whole r3

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | evidence-backed correct |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "sets no config.featureGates, so the gate stays at its new default of on."
- R2 met: "overlay mesh-root-ca-prod.yaml patches only duration, renewBefore and subject"
- R3 met: "The mismatch fails the handshake in both directions."
- R4 met: "Add privateKey.rotationPolicy: Never to all three Certificates"

## B2 revisionHistoryLimit: evidence-backed correct
- R1 met: "old CertificateRequests for the three Certificates are deleted, keeping the latest one"
- R2 met: "Nothing in env/ does."

**Overall recommendation:** "Not safe to upgrade as-is."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As r1.
