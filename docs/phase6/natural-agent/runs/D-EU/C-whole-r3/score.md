# Score — D-EU C-whole r3

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B3 strict path validation | hedged |
| B1 annotations-risk-level | evidence-backed correct |
| B2 cross-namespace | evidence-backed correct |
| B4 annotation validation | evidence-backed correct |

## B3 strict path validation: hedged
- R1 wrong: "From memory, dots, hyphens and underscores are allowed, so /.well-known should pass."
- R2 met: "The second is the only unusual one, because it starts with a dot segment."
- R3 met: "The docs Ingress would then be refused at the next Argo sync or update."
- R4 met: "set strict-validate-path-type: "false" in values.yaml as a stopgap"
- Note: Verdict: "One item could still block the rollout and has to be tested first". Not false-safe; not appropriately uncertain, because R1 is stated wrongly.

## B1 annotations-risk-level: evidence-backed correct
- R2 met: "It is a plain boolean and low risk."

## B2 cross-namespace: evidence-backed correct
- R2 met: "The Ingress's backends are in its own docs namespace"

## B4 annotation validation: evidence-backed correct
- R2 met: "It is a plain boolean and low risk."

**Overall recommendation:** "No documented breaking change is proven to hit this environment. One item could still block the rollout and has to be tested first"

**Consumers named:** docs Ingress → docs-site, wellknown
**Unsupported claims stated as fact:** none found
**Versus arm B:** Same knowledge error as arm B; flagged it as the main risk instead of clearing it.
