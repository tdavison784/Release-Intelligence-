# Score — D-EU C-item r3

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B3 strict path validation | hedged |
| B1 annotations-risk-level | evidence-backed correct |
| B2 cross-namespace | evidence-backed correct |
| B4 annotation validation | evidence-backed correct |

## B3 strict path validation: hedged
- R1 wrong: "From memory, strict validation rejects characters outside an alphanumeric, -, _, . and / class"
- R2 met: "a path segment starting with a dot"
- R3 met: "If /.well-known were rejected, the failure would show up when Argo CD syncs the Ingress"
- R4 met: "set controller.config.strict-validate-path-type: "false""
- Note: Verdict "Probably not, but one item is unverified", with an admission test required before rollout.

## B1 annotations-risk-level: evidence-backed correct
- R2 met: "The default only blocks "Critical" annotations"

## B2 cross-namespace: evidence-backed correct
- R2 met: "Its backends docs-site and wellknown are in the same namespace"

## B4 annotation validation: evidence-backed correct
- R2 met: "a valid boolean"

**Overall recommendation:** "Probably not, but one item is unverified."

**Consumers named:** docs Ingress → docs-site, wellknown
**Unsupported claims stated as fact:** none found
**Versus arm B:** Same knowledge error; required a test.
