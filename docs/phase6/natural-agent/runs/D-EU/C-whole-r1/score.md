# Score — D-EU C-whole r1

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B3 strict path validation | false-safe |
| B1 annotations-risk-level | evidence-backed correct |
| B2 cross-namespace | evidence-backed correct |
| B4 annotation validation | evidence-backed correct |

## B3 strict path validation: false-safe
- R1 wrong: "Both use only characters that strict validation allows (letters, ., -, /)."
- R2 met: "This is the most relevant item because the .well-known path contains a dot."
- R3 missed: not stated
- R4 missed: not stated
- Note: Item verdict "Not expected to be rejected." Character set stated from model knowledge, and wrong; the workspace has only the changelog line. Root cause: missing evidence + incorrect semantic interpretation (model knowledge).

## B1 annotations-risk-level: evidence-backed correct
- R2 met: "The only annotation is nginx.ingress.kubernetes.io/ssl-redirect: "false". It is a valid boolean and low risk."

## B2 cross-namespace: evidence-backed correct
- R2 met: "Both backends (docs-site, wellknown) are in the Ingress's own docs namespace."

## B4 annotation validation: evidence-backed correct
- R2 met: "It is a valid boolean and low risk."

**Overall recommendation:** "No documented 1.12.0 breaking change appears to break this environment's configuration or its one Ingress."

**Consumers named:** docs Ingress → docs-site, wellknown
**Unsupported claims stated as fact:** security-fix claims about CVE-2025-1974 (flagged by the assessment itself as from memory)
**Versus arm B:** Same B3 error as arm B ("valid characters"); both lacked the character set in their evidence.
