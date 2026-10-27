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

## Attempt 2, part A rerun after verify2 (formalize/v2 on the same `inv-cd653b2be37c`), 2026-10-28

Formalization produced 3 rules, **2 scope refinements** and 13 unrepresentable records. Verification ran with
runtime.

**Fixed by verify2:**
- **The broad behaviour rule `hyp-3b48ffc30a8e`** ("a Certificate whose rotationPolicy is unset gets Always …
  explicit Never keeps the old behaviour") is **CONFIRMED (observed)**, `pf-683f9715ed0d`. The omitted-field arm
  decided it: the key changed at TO without the condition and matched FROM with it. The explicit-`Always` arm was
  "recorded, not decisive (non-discriminating)".
- **The scope refinement `hyp-cc18d162127a`** ("harmful consequence occurs only for Certificate objects in consumer
  role ca-trust (spec.isCA = true) … outside the role (spec.isCA = false) are shielded"). **Its consumer exercise
  passed:** "outside the role (ca-trust) the consumer verifies at FROM and TO on 2 arm(s): the harm is confined to
  the role."

**Still blocking:**
- **A. semantic-check refutes the refinement.** "boolean `true` / `false` does not occur in the cited passages". The
  role tokens (`isCA = true/false`) come from the catalog role and are established by the consumer exercise, not by
  the upstream citation, so the semantic check must not test them against the passage. The same applies to
  `hyp-66ec2571d3a3`. This is a defect in how semverify and verify2 interact.
- **B. The exercise signal is flaky.** The v2 copy of the same broad rule (`hyp-c9ace2753bd7`) **failed**: "with the
  mitigating condition the behaviour still changed at TO". The "change" was controller **log lines** ("unable to fetch
  certificate that owns the secret"), not a key change. The v1 copy of the identical rule passed in another cluster.
  Log lines must not count as behaviour signatures, or must be normalised away.
- **C. An unset role field.** The P-US leaf Certificates leave `isCA` **unset**. The refinement's outside-role
  condition is `isCA = false`. With no evidenced default, unset stays undecidable (fail closed), so P-US cannot clear
  even after A and B. The investigation holds no upstream evidence of the `isCA` default (Go zero value; the CRD has
  no `default`).
  - The real kind run already **observed** unset-`isCA` leaves behaving as outside-role (verify2's confirmed case,
    "including certificates that leave isCA unset").
  - **Decision for the product owner:** may an **observed** default, established by the verifier's omitted-field arm
    on the real controller and recorded with its limitation, count as an evidenced default? Today only an upstream
    citation counts.

No kind cluster was left behind.
