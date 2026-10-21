# Score — D-EU C-item r2

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B3 strict path validation | appropriately uncertain |
| B1 annotations-risk-level | evidence-backed correct |
| B2 cross-namespace | evidence-backed correct |
| B4 annotation validation | evidence-backed correct |

## B3 strict path validation: appropriately uncertain
- R1 met: "The upstream docs describe the allowed set as alphanumerics, -, _ and /."
- R2 met: "/.well-known contains a ."
- R3 met: "the admission webhook (ingress-nginx-admission, enabled in values.yaml) would refuse the Ingress on its next create or update."
- R4 met: "Change /.well-known to pathType: ImplementationSpecific, or set strict-validate-path-type: "false" in controller.config."
- Note: Flag: model knowledge. The character set is from memory ("The upstream docs describe…"; the workspace has no such text), but stated correctly and held as unconfirmed. The only run of six that is not false-safe or hedged on B3.

## B1 annotations-risk-level: evidence-backed correct
- R2 met: "The Ingress has no snippet or other Critical-risk annotation."

## B2 cross-namespace: evidence-backed correct
- R2 met: "The Ingress references only same-namespace Services"

## B4 annotation validation: evidence-backed correct
- R2 met: "so it should validate."

**Overall recommendation:** "Do not call the upgrade safe until the server-side dry-run below passes."

**Consumers named:** docs Ingress → docs-site, wellknown
**Unsupported claims stated as fact:** none found
**Versus arm B:** Avoided arm B's false-safe: kept the dot as an open question and gave the remediation.
