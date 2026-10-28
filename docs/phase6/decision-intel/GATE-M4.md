# Milestone 4 gate (commander only; label-bearing; lanes never read this file)

The development pair is cert-manager 1.17.2 → 1.18.0.
- **payments-us (P-US): expected not-affected** on rotationPolicy (`ci-86548e9a2300`, `ci-93299c596271`) and
  revisionHistoryLimit (`ci-25e3f7619968`, `ci-f86a125e0d17`).
- **payments-eu (P-EU): expected affected** on rotationPolicy; revisionHistoryLimit not-affected.
- **Facts:** the P-US prod overlay pins `rotationPolicy: Never` on the CA Certificate `mesh-root-ca` (`isCA: true`).
  The leaves (`checkout-tls`, `ledger-tls`) leave both rotationPolicy and isCA unset. P-EU pins nothing.
- **The mechanism that matters** is a CA key changing on renewal (trust break). Leaf key rotation is benign with
  CA-based trust (Phase 0, M1).

## 1. The loop

From K0 (Phase 5 snapshot 03b6ff43, digest `sha256:672a7ce0…`):
1. A v13 + formalize run on the pair.
2. `verify` with runtime.
3. Promotion by the product owner.
4. K1.

**Pass, every one required:**
- **(a) Formalization** yields a cited rule whose harmful exposure is "a CA Certificate (`isCA: true`) whose
  `privateKey.rotationPolicy` is unset". Any equivalent CA-scoped form is acceptable. Every citation is present in
  the assessment's evidence, and the unknowns are kept.
- **(b) Verification confirms it** with:
  - citations;
  - semantic-check;
  - boundary fixtures: isCA unset, isCA false, isCA true with Never, isCA true unset;
  - a runtime renewal plus the CA-trust exercise.

  The `isCA` default for unset is **established by cited evidence** (the CRD schema or the controller default at the
  tag), not assumed.
- **(c) K1 on P-US:** the rotationPolicy items become **not-affected** (authoritative, with the proof cited). The CRD
  description change on `privateKey.rotationPolicy` is covered by field-scoped coverage. Every other P-US item keeps
  its previous class and gaps.
- **(d) K1 on P-EU:** the rotationPolicy items are **not cleared** (affected or potentially-affected), because
  `mesh-root-ca` is exposed. Every other item is unchanged.
- **(e)** `falseNegativeRate = 0` and `unsafeEligibilityRate = 0` on both. The comparison with K0 shows clearances
  gained only on P-US's rotationPolicy items.

## 2. Negative controls (all must fail closed)

- **Inapplicable:** a rule scoped to `isCA: false` on P-EU's root does not clear.
- **Expired bounds:** a rule bounded `(1.16, 1.17]` does not apply to the 1.17.2 → 1.18.0 edge.
- **Contradicted:** a second active fact on the same subject with a contradicting consequence blocks the clearance
  (the supersedes/coexists gate).
- **Unevidenced default:** the same rule with the isCA default asserted without a citation gives undecidable, not
  not-affected.

## 3. M2 misread and admission

- **`semantic-check`** on the stored M2 run `C1-K0-20261027-*-v13-deu-r3`, behaviour "Strict path validation
  character set". The claim adds '.', and the cited passage reads `alphanumeric chars, "-", "_", "/"`. It must be
  **flagged unsupported**.
- **`admission-dry-run`** of the D-EU `docs` Ingress (`/.well-known` Prefix, strict validation on) against a
  controller-v1.12.x admission webhook in kind. The outcome is **observed**, and becomes the ground truth for that
  question in later reporting.

## Reporting

`M4-RESULT.md`: per-step records, the K0 → K1 compare tables, the negative controls, the costs, and the runtime
cluster time.
