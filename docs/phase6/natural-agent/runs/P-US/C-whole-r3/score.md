# Score — P-US C-whole r3

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | evidence-backed correct |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "The chart values here do not touch it."
- R2 met: "The pki Argo app points at that overlay (clusters/prod/pki.yaml, path platform/pki/overlays/prod), so it is what gets applied."
- R3 met: "After the upgrade, each renewal generates a new private key."
- R4 met: "Nothing in the repo pins the leaf public key."
- U met: "Unknown: consumers outside the repo."
- Note: Deciding difference named: "neutralised where it matters most (the mesh root CA)".

## B2 revisionHistoryLimit: evidence-backed correct
- R1 met: "It keeps the latest revision for each Certificate"
- R2 met: "Nothing in the repo reads CertificateRequests"
- U met: "Check whether anything outside the repo (audit tooling, log shipping, runbooks) depends on old CertificateRequests."

**Overall recommendation:** "No change to the repository is required for the upgrade to function."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** As r1.
