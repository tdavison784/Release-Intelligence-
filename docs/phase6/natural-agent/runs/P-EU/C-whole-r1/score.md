# Score — P-EU C-whole r1

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | evidence-backed correct |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "platform/cert-manager/values.yaml does not touch config.featureGates, so the gate stays at its new default (on)."
- R2 met: "No Certificate sets privateKey.rotationPolicy."
- R3 met: "mTLS between a pod holding the new ca.crt and a peer presenting an old-key leaf fails in both directions."
- R4 met: "add privateKey.rotationPolicy: Never to mesh-root-ca"
- Note: Deciding difference named: the CA's unset policy ("a real hazard for the mesh-root-ca CA certificate").

## B2 revisionHistoryLimit: evidence-backed correct
- R1 met: "stale CertificateRequest objects are garbage-collected"
- R2 met: "I found no consumer of old CertificateRequests"

**Overall recommendation:** "Yes, there is one breaking change that matters here."

**Consumers named:** ledger, checkout, mesh-ca ClusterIssuer
**Unsupported claims stated as fact:** none found
**Versus arm B:** Arm B reached 'affected' only because 'every Certificate's key rotates' and recorded 'Consumers of the Secrets were not traced'. C traced the CA-trust mechanism through ca.crt.
