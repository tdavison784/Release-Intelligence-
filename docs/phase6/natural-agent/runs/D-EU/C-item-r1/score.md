# Score — D-EU C-item r1

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B3 strict path validation | false-safe |
| B1 annotations-risk-level | evidence-backed correct |
| B2 cross-namespace | evidence-backed correct |
| B4 annotation validation | evidence-backed correct |

## B3 strict path validation: false-safe
- R1 wrong: "the allowed set is alphanumerics plus - . _ ~ /, so . and - pass."
- R2 met: "The paths here are / and /.well-known, both pathType: Prefix."
- R3 missed: not stated
- R4 missed: not stated
- Note: Item heading "not affected (high confidence, not verified against source)". Root cause as C-whole r1.

## B1 annotations-risk-level: evidence-backed correct
- R2 met: "ssl-redirect is low risk."

## B2 cross-namespace: evidence-backed correct
- R2 met: "Both backends (docs-site and wellknown) are Services in the same docs namespace as the Ingress."

## B4 annotation validation: evidence-backed correct
- R2 met: "It is a low-risk annotation with a plain boolean value, which is a valid value."

**Overall recommendation:** "Not expected to break this environment."

**Consumers named:** docs Ingress → docs-site, wellknown
**Unsupported claims stated as fact:** none found
**Versus arm B:** Same error as arm B.
