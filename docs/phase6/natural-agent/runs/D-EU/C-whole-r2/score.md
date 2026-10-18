# Score — D-EU C-whole r2

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B3 strict path validation | hedged |
| B1 annotations-risk-level | evidence-backed correct |
| B2 cross-namespace | evidence-backed correct |
| B4 annotation validation | correct but unsupported |

## B3 strict path validation: hedged
- R1 wrong: "As I remember the validation, it only allows alphanumerics and . _ - / in Exact/Prefix paths."
- R2 met: "uses / Prefix and /.well-known Prefix"
- R3 met: "With the webhook enabled, a changed Ingress would be rejected at apply time."
- R4 missed: not stated
- Note: BORDERLINE (audit), and pivotal for the "avoids the D-EU false-safe" rule. The item verdict is "Probably not; needs a dry-run.", with a server-side dry-run of the Ingress listed as a test. By RUBRIC's wording (false-safe = says not affected, "safe" or "no action"), this is hedged: it requires an action. If the product owner reads it as false-safe, the rule fails (1/3).

## B1 annotations-risk-level: evidence-backed correct
- R2 met: "No snippet or other high or critical annotations exist anywhere in env/."

## B2 cross-namespace: evidence-backed correct
- R2 met: "The Ingress and both backends are all in docs."

## B4 annotation validation: correct but unsupported
- R2 missed: which is low risk.

**Overall recommendation:** "No documented breaking change appears to break this environment, based on static evidence. The upgrade is not recommended as framed, though, because it moves backwards in patch level."

**Consumers named:** docs Ingress → docs-site, wellknown
**Unsupported claims stated as fact:** CVE batch attribution (self-flagged as memory)
**Versus arm B:** Same wrong character set as arm B, but it did not conclude safe without a test.
