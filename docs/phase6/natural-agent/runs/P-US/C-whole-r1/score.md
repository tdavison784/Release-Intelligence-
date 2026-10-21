# Score — P-US C-whole r1

Commander score against RUBRIC.md (X-0004). Quotes are verbatim from `assessment.md`, checked by `scripts/phase6/scores.py`.

| item | outcome |
|---|---|
| B1 rotationPolicy | evidence-backed correct |
| B2 revisionHistoryLimit | evidence-backed correct |

## B1 rotationPolicy: evidence-backed correct
- R1 met: "There are no feature gates and no extraArgs."
- R2 met: "env/platform/pki/overlays/prod/mesh-root-ca-prod.yaml sets privateKey.rotationPolicy to Never"
- R3 met: "The two leaf certificates have no such setting and will start rotating their private keys."
- R4 met: "Pinning of the leaf public key is not visible anywhere in the repo"
- U met: "Check any consumers outside this repo (service mesh, WAF, partner allow-lists, HSM or audit systems) that pin leaf public keys or fingerprints."
- Note: Deciding difference named: "That one line is what keeps the upgrade safe." R2 also cites the render (rendered/to.yaml:14032-14035).

## B2 revisionHistoryLimit: evidence-backed correct
- R1 met: "keeping only the latest per Certificate"
- R2 met: "Nothing in the repo reads CertificateRequests, so there is no functional dependency here."
- U met: "Check outside the repo for audit or compliance tooling"

**Overall recommendation:** "No change that must be fixed before upgrading, but not "safe to just merge"."

**Consumers named:** ledger (nginx mTLS, /tls/ca.crt), checkout (curl loop)
**Unsupported claims stated as fact:** none found
**Versus arm B:** Read the payments Deployments and ledger.conf and concluded on consumer trust; arm B never opened them (its transcript has no read of env/apps/payments).
