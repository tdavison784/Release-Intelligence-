# Score — P-US arm B (stored run `B0-K0-20261017-edb21ed9-v8`, prompt v8, claude-sonnet-5-5)

Scored from the agent's final `submit_findings` record in the stored transcript (RUBRIC: class = agent verdict; R-elements from mechanism, environmentReasoning and claims).

| item | outcome |
|---|---|
| B1 rotationPolicy | hedged |
| B2 revisionHistoryLimit | hedged |

## B1 rotationPolicy: hedged (agent class potentially-affected)
- R1 met: "The default for Certificate.spec.privateKey.rotationPolicy changes from Never to Always." "The gate is not disabled anywhere."
- R2 met (through RI's enum read-back): "mesh-root-ca is explicitly set (its value is redacted but is the digest of \"Never\") and is unaffected."
- R3 met: "Two of the three Certificates (payments/checkout-tls and payments/ledger-tls) inherit the default and will rotate keys."
- R4 missed: "I did not trace what consumes the two certificates' Secrets (those Secrets are absent from the snapshot), so I cannot say whether anything depends on a stable key."
- Transcript: 23 tool calls across the whole upgrade's change items. No read of `apps/payments/base/*-deployment.yaml` or `ledger.conf`, although both were bundle artifacts (EVIDENCE.md §1). Root cause: **incomplete consumer analysis**. The agent looked for the Secrets, which are not declared, instead of their consumers, which are. Retrieval, not evidence absence.

## B2 revisionHistoryLimit: hedged (potentially-affected)
- R1 met: "the controller prunes old CertificateRequests for each Certificate, keeping only the latest one."
- R2 missed: "I did not check for such consumers."

**Overall:** case human-review-required (the policy is not changed by the agent).
