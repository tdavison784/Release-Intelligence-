# Score — P-EU arm B (stored run `B0-K0-20261017-edb21ed9-v8`)

| item | outcome |
|---|---|
| B1 rotationPolicy | correct but unsupported |
| B2 revisionHistoryLimit | hedged |

## B1 rotationPolicy: correct but unsupported (agent class affected)
- R1 met: "The default flips from Never to Always."
- R2 met: "mesh-root-ca, checkout-tls and ledger-tls all inherit the default".
- R3 missed: no CA-trust mechanism. The agent reasoned "Consumers of the Secrets (mesh CA trust, payments TLS endpoints) get new keys on renewal. I did not trace these consumers". Its assumption is "Consumers of the Secrets were not traced." This is RUBRIC's named case: affected because every key rotates, without R3, is correct but unsupported.
- R4 missed: "I recorded no mitigation."

## B2 revisionHistoryLimit: hedged (potentially-affected; same reasoning as P-US B2).
