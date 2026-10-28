# Milestone 3 gate (commander only; label-bearing; lanes never read this file)

**Runs:** v13 investigations plus synthesis, × 3, on the three development cases.
- **cert-manager payments-us (P-US) and payments-eu (P-EU):** online (upstream fetch), from the M2 recorded advisory
  state.
- **ingress-nginx docs-eu (D-EU):** heldout.

Each run is saved, and judgements are scored from the stored records.

## Expected per case (facts from M0–M2 and the product owner's decisions)

**P-US (cert-manager 1.17.2 → 1.18.0; root CA pinned `Never`; leaves inherit `Always`; trust is by CA):**
- **Recommendation:** `modify-target` (the target regresses security: GHSA-8rvj high, GHSA-gx3x medium; no 1.18.x
  fix).
- **Target:** 1.20.3 (PD-0e). 1.19.6 is the flagged alternative. Any later 1.20.x qualifies under the latest-patch
  rule if it holds.
- **Required:** a statement that the 1.17.2 → <new target> transition still needs its own investigation (C2).
- **rotationPolicy:**
  - **Correct:** leaf rotation is expected and benign given CA trust, with "workloads reload rotated files" as a
    precondition and a staging renewal test as validation.
  - **Wrong:** a recommended change to pin the leaf certificates (not needed); recommending adoption of `Always` for
    the root.

**P-EU (as P-US, but the root CA inherits `Always`):**
- **Recommendation:** `modify-target`, with changes.
- **Target:** as P-US.
- **Sequencing (PD-0b, the product owner's own recommendation from arm D):**
  1. pin `privateKey.rotationPolicy: Never` on `mesh-root-ca` in Git **before**, or isolated from, the version bump;
  2. validate;
  3. bump;
  4. validate;
  5. adopt `Always` for the root separately, as its own planned CA rotation work.
- **Required:** the root rotation risk (trust break between checkout and ledger) as a residual risk or hard finding.

**D-EU (ingress-nginx 1.11.5 → 1.12.0, chart 4.11.5 → 4.12.0):**
- **Recommendation:** `modify-target`, plus `validation-required` or changes for strict path validation.
- **Target:** at least 1.12.1 / chart 4.12.1 (minimum). The latest 1.12.x patch is acceptable if the latest-patch rule
  is confirmed.
- **Required:**
  - the 5 regressions named or referenced;
  - the persisting advisories (clean only at 1.13.7) as a residual risk;
  - GHSA-f53h unassessed, as a disagreement or gap;
  - strict-path validation on `/.well-known` as a hard finding or required validation (a server-side dry-run against
    a 1.12.x admission webhook).
- **Wrong:** an unconditional proceed, or a claim that `/.well-known` passes.

## Pass (≥ 2 of 3 per case)

1. The recommendation is in the expected set, and the headline commits with scope and preconditions (PD-0d).
2. The target meets the expected minimum and rule, with candidate evidence cited and the new-transition caveat
   present.
3. Sequencing is isolated and ordered where expected (P-EU pin before bump), and no unneeded change is recommended
   (P-US leaf pin).
4. Every precomputed hard finding and disagreement is present with its id. No invented evidence: every reference
   resolves, and no fact appears that is absent from the inputs (the commander spot-checks every judgement).
5. Findings, decision and review are byte-identical with and without synthesis.

**Also reported:** cost and time of synthesis per case, the floor rejections and retries, and synthesis failures.
