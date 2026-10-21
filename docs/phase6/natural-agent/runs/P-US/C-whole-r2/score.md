# Score — P-US C-whole r2

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | hedged |
| B2 revisionHistoryLimit | evidence-backed correct |

## B1 rotationPolicy: hedged
- R1 met: "values.yaml sets no config.featureGates or extraArgs."
- R2 met: "clusters/prod/pki.yaml points Argo at platform/pki/overlays/prod, so the patch is applied in prod."
- R3 met: "They will start generating a new private key on every renewal after the upgrade."
- R4 met: "ledger accepts it because it trusts the CA only (ssl_client_certificate /tls/ca.crt), not a specific key or cert. Nothing in the repo pins leaf keys or public-key hashes."
- U met: "Any out-of-repo system that pinned the key would break."
- Note: BORDERLINE (audit): the headline is "Yes, there is one breaking change that applies to this environment, and it is latent rather than immediate", and it leaves open that "the repo can't show that the workloads handle it". That is wider than U, and inconsistent with its own R4, which shows the workloads do handle it. Scored hedged, not appropriately uncertain. The "distinguishes" rule holds either way (repeats 1 and 3).

## B2 revisionHistoryLimit: evidence-backed correct
- R1 met: "cert-manager garbage-collects all stale CertificateRequests"
- R2 met: "Nothing in the repo consumes CertificateRequests."
- U met: "Existing CertificateRequests are dispensable (they will be pruned)."

**Overall recommendation:** "Yes, there is one breaking change that applies to this environment, and it is latent rather than immediate."

**Consumers named:** ledger, checkout
**Unsupported claims stated as fact:** none found
**Versus arm B:** Traced consumers (as r1), but framed the leaf rotation as a breaking change that applies.
