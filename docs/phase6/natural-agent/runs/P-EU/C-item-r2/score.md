# Score — P-EU C-item r2

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | n/a (out of C-item scope) |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "The feature gate is not turned off."
- R2 met: "the prod overlay platform/pki/overlays/prod/mesh-root-ca-prod.yaml"
- R3 met: "If ledger's ca.crt is new and checkout still presents an old-CA cert, ledger rejects the client cert."
- R4 met: "set privateKey.rotationPolicy: Never explicitly on mesh-root-ca"

## B2 revisionHistoryLimit: n/a (out of C-item scope)

**Overall recommendation:** "Yes, this change affects this environment, and the upgrade is not safe as-is."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As C-whole.
