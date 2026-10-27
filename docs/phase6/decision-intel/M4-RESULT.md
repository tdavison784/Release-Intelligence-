# Milestone 4 result (commander only; label-bearing; lanes never read this file)

## Attempt 1, part A, 2026-10-28: the loop runs end to end and refuses correctly; no promotable rule yet (two blockers)

**Inputs:**
- K0: the Phase 5 snapshot 03b6ff43, `sha256:672a7ce0…`.
- The P-US v13 investigation `inv-cd653b2be37c` (online, from the M2 recorded advisory state).
- `ri investigate formalize` → K1 (a working copy).
- `ri verify hypotheses -runtime` (kind).

**Formalization (`formalize/v1`):**
- 15 claims became **3 rules** and **12 unrepresentable records**. Each record carries its limitation.
- **The non-invention floor held.** For example, the rotationPolicy behaviour claim: "No upstream evidence is available
  for the new default or the gate's default state: the release-notes excerpt is not marked upstream…".
- **The rotationPolicy rule** (`hyp-3b48ffc30a8e`, item `ci-93299c596271`): "a Certificate whose
  spec.privateKey.rotationPolicy is unset gets the default Always … set explicitly to Never keeps the old key-reuse
  behaviour". It is correct as a behaviour rule, but **not CA-scoped**, so it cannot clear P-US: the leaves leave the
  field unset.

**Verification:**
- **The rotationPolicy rule** (`pf-3a5f9569fd82`): citation-check pass, semantic-check pass (3 tokens),
  **rule-exercise inconclusive**.
- **The revisionHistoryLimit rule:** citation and semantic pass; the exercise could not be synthesised (a null
  previous value).
- **The ValidateCAA rule:** citation inconclusive (no passage names the subject); semantic inconclusive, routed to a
  human.

### Blocker 1, a defect: rule-exercise arm synthesis for a default-change rule

| Arm | Predicted | Observed | Result |
|---|---|---|---|
| `Never` | previous behaviour | previous behaviour | pass |
| field **omitted** | new behaviour | new behaviour, with `tls.key` changed FROM → TO | pass: the real evidence of the default change |
| explicit **`Always`** (the negation the exercise synthesised) | "new behaviour" | no change, because `Always` rotates at both versions | inconclusive |

For a default-change rule, the contrast is the omitted field. An explicit opposite value that behaves identically at
both versions is not evidence either way. The verifier has to classify such an arm as non-discriminating, not as
inconclusive.

### Blocker 2, a design gap: the clearing scope needs consumer-trust evidence that no stage produces

- **What is needed.** P-US is cleared only by the CA-scoped consequence: a trust break occurs when a **CA**
  certificate's key rotates. Leaf rotation is benign because consumers verify peers by the CA.
- **What the investigation had.** It traced the consumers (`ca.crt` in checkout's curl command, and `ledger-nginx`
  `ssl_certificate_key` …). But its assessments state "The root CA is unaffected because it is pinned to Never" only
  as **inferred**. Consumer trust semantics (what `ssl_client_certificate` and `--cacert` mean) are behaviours of
  consumer software outside the product's repositories, which the M2 fetch cannot reach.
- **Where the right evidence exists.** In verification: the `ca-trust` consumer exercise (role `isCA: true`, as in the
  Phase 5 confirmed proof `pf-23e47aa9c6a9`) *observes* that a leaf issued after a CA re-key no longer verifies
  against the captured `ca.crt`.
- **So the fix.** The CA-scoped consequence must be **established by verification** (observed), not asserted by the
  formalizer:
  - formalization proposes the scope refinement as a candidate, with `ProposedVerification: consumer exercise`, when
    the investigation traced consumers that read the issuer's CA key;
  - the verifier confirms or refutes it.

No kind cluster was left behind. Part B (product-owner promotion, K1 evaluation, controls) is **not started**:
nothing is confirmed to promote.
