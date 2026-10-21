# Score — P-US C-item r1

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | n/a (out of C-item scope) |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "The gate is therefore on by default here."
- R2 met: "Argo app pki deploys platform/pki/overlays/prod."
- R3 met: "ledger-tls and checkout-tls will start generating a new private key at every renewal."
- R4 met: "Nothing pins a leaf public key, hash or SPKI"
- U met: "Secrets or tooling outside this repo"

## B2 revisionHistoryLimit: n/a (out of C-item scope)

**Overall recommendation:** "The default change does not break this environment as declared, but it does change behavior."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** Single-change scope, same consumer tracing as C-whole: scope did not reduce depth.
