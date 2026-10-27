# Milestone 3 result (commander only; label-bearing; lanes never read this file)

## Attempt 1, 2026-10-28: MET on every GATE-M3 criterion but one; that one is for the product owner (P-US leaf pins)

**Runs:**
- `D1-K0-20261027-686fab92-v13-synth-cm-r{1,2,3}` (P-US and P-EU) and `…-v13-synth-deu-r{1,2,3}` (D-EU).
- v13 investigation plus `synthesize/v1`, synthesis on `claude-opus-5-5` through `claude -p`.
- Online, from the M2 recorded advisory state, on K0.
- About 14 minutes per cert-manager pair run and 8 minutes per D-EU run.

**Status:** 9 of 9 judgements complete, with no synthesis failure. Every recommendation is `modify-target`.

### Per case

**P-US** (expected: modify-target to 1.20.3 or a later 1.20.x; transition caveat; leaf rotation as a precondition or validation):
- **Target:** minimum v1.20.3, recommended v1.20.4 (PD-3: latest safe patch) in 3 of 3.
- **Transition:** every run says "a separate v1.17.2 → v1.20.4 investigation" is required before production.
- **The root CA:** every run notes the root is already explicit `Never`. "Confirm the existing explicit rotationPolicy on
  cert-manager/mesh-root-ca is Never. The value is redacted…" (r1).
- **Differs from GATE-M3 in 3 of 3:** every run recommends **pinning `rotationPolicy: Never` on the two leaf Certificates
  before the bump**, then adopting `Always` later as a separate change. GATE-M3 had marked a leaf pin as an unneeded
  change, since leaf rotation is benign with CA trust. But the product owner's PD-0b ("pin today's behaviour, test the
  new default separately") supports exactly this. It is conservative, isolated and reversible, not a safety error.
  **Product-owner call.**
- **Other required parts:** the hard findings are present (9–17 per judgement); there is a staging renewal test that
  checkout and ledger pick up reissued keys; a live ACME-issuer check (from the ACME items) appears as a precondition.

**P-EU** (expected: modify-target, plus a root pin before the bump, then adopt `Always` separately):
- **Target:** minimum v1.20.3, recommended v1.20.4, in 3 of 3.
- **Sequencing in 3 of 3:**
  - "First pin rotationPolicy: Never on all three Certificates, then bump the version, and adopt the Always default
    later as a separate change" (r1).
  - "mesh-root-ca first" (r3).
- **The adoption step:** r3 adds a forced renewal in staging, to show that checkout and ledger reload the new key,
  before adopting.
- **The new transition:** caveat present in 3 of 3.

**D-EU** (expected: modify-target to ≥ 1.12.1 / chart 4.12.1; strict path as a hard finding or validation; persisting
advisories; no unconditional proceed):
- **Target:** minimum controller-v1.12.1, recommended **controller-v1.12.8 (chart 4.12.8)** in 3 of 3. Every run says
  "do not deploy 1.12.0".
- **Strict path:**
  - r2 and r3: "First pin strict-validate-path-type to today's value (false) so the docs /.well-known route keeps
    working, then bump", with a later, separate change to make `/.well-known` pass strict validation. This is PD-0b
    applied without prompting.
  - r1: dry-run every Ingress against the 1.12.8 admission webhook (validation required).
- **Persisting advisories:** "accepts four advisories that 1.11.5 already has… a move to 1.13.7 should be planned as a
  follow-up" (r1; r3 says the same).
- **Further required validation:** the transition investigation of 1.11.5 → 1.12.8, and a re-check of the render diff
  that removes `allow-snippet-annotations`.

### Criteria (≥ 2 of 3 per case)

| Criterion | P-US | P-EU | D-EU |
|---|---|---|---|
| 1. Recommendation in the expected set; the headline commits with scope and preconditions | 3/3 | 3/3 | 3/3 |
| 2. Target ≥ the expected minimum under PD-3, cited, with the transition caveat | 3/3 | 3/3 | 3/3 |
| 3. Isolated and ordered where expected; no unneeded change | **0/3 by GATE wording** (leaf pin; consistent with PD-0b) | 3/3 | 3/3 |
| 4. Hard findings present with ids; no invented evidence (floor plus commander spot-check) | 3/3 | 3/3 | 3/3 |
| 5. Classification identical to the deterministic runs | 3/3 | 3/3 | 3/3 |

- **Spot-check of claims against the inputs.** Every claim traces to an input: ACME issuers (the ACME change items),
  challenge RBAC (the RBAC split items), webhook probe ports (the probe change), CertificateRequest snapshot
  (revisionHistoryLimit), the four persisting advisories and 1.13.7 (edge security), allow-snippet-annotations (the
  render diff). **No invented fact found.**
- **Disagreements: 0 in every judgement.** Agent verdicts and the classifier did not contradict each other on these
  runs. No regression was recommended, and no "not-affected" was set against an affected classification. The hard
  findings carry the regressions and gaps.

**Reading.**
- This is the first time RI gives the engineer-level answer the product owner asked for in Phase 0: a concrete safer
  version, the isolated sequence, the tests, the caveat about the new upgrade path, and every hard finding attached.
- The one deviation from the gate is a judgement on behaviour preservation that follows the product owner's own
  principle.
