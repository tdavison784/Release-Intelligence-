# Score — P-EU C-item r3

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | n/a (out of C-item scope) |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "so the DefaultPrivateKeyRotationPolicyAlways gate stays at its default (on) in 1.18.0."
- R2 met: "mesh-root-ca (base and prod overlay). All of them inherit the default"
- R3 met: "Both directions of the handshake fail, so ledger and checkout can't talk until both have renewed."
- R4 met: "Keep Never on mesh-root-ca until a CA-rotation plan exists"

## B2 revisionHistoryLimit: n/a (out of C-item scope)

**Overall recommendation:** "Yes, it affects this environment."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As C-whole.
