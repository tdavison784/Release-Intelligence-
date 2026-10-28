# Milestone 6 result: blind comparative evaluation (2026-10-29)

Scored per GATE-M6 and amendments A1–A4, with `scripts/phase6/score-m6.py`. The full tables are in
`evidence-m6-scores.md`, reproducible from the stored runs. The sampled judgements are in `M6-GRADING.md`,
for the product owner to grade.

**Runs** on `m6-freeze-a2`:
- A: `M6-A2`;
- B: `M6-B2-r1..r3`;
- C/D: `M6-CD2-r1`, `M6-CD2-r2`, `M6-CD2b-r1` (`M6-CD2-r3` is void, A4);
- E: formalize and verify on `M6-CD2-r1`, then `M6-E-A`.

## Verdict: **M6 does not pass** (P1 fails). The safety requirements all hold.

| Requirement | Result |
|---|---|
| H1. Unsafe authoritative clearances = 0, every arm | ✅ 0 in all 8 runs |
| H2. `unsafeEligibilityRate` = 0 | ✅ nothing was eligible in any run (0/0) |
| H3. Arm E promotes only through the product owner; no clearance of an affected side | ✅ nothing was promoted (nothing was confirmed) |
| **P1.** `correct(C) ≥ correct(B) + 2` and `≥ 5` | ❌ C 3.00, B 3.67 (/8), counting P4's uncertain-correct; strict: C 2.00, B 2.67 |
| P2. `FS(C) ≤ FS(B)` and `≤ 1` | ✅ 0 and 0: **no false-safe in any of the 48 agent assessments** |
| P5. Security-regression pair: D flags it and targets ≥ the safer target, in ≥ 2 of 3 runs; no D target is regressed | ✅ 3 of 3 flag GHSA-6765 and recommend v3.7.16; all 24 recommended targets are marked safe |
| P3. Consumers named (reported) | ❌ 2 of 4 consumer mechanisms (cert-manager ClusterIssuer; external-secrets Application). The other two items were never examined (below) |
| P4. Incomplete configuration is uncertain and names the missing input (reported) | ✅ 3 of 3 C runs and all D judgements (potentially-affected; the deploying Application's ServerSideApply sync option). Product-owner audit pending |

## Why P1 fails: coverage, not reasoning

Decisive items 4 of 8 were **never reached by any agent, in any arm**, so neither B nor C could get them right:

1. **Argo CD (both environments).** The 2.14 → 3.0 upgrade has **97 change items**, and the agent examines 10. Every
   item had the same priority (`agenda:behaviour`, priority 0), so in effect the 10 were taken in id order. The
   breaking change (`feat!: Logs rbac enforce…`, ci-d033a5ea330d / ci-d3a4fd296534) was "beyond cap 10: no session".
   **Agenda ordering has no breaking-first or severity signal once priorities tie.**
2. **Traefik (both environments).** RI's change extraction produced **no change item** for the Chain-middleware
   `allowCrossNamespace` change, so nothing could match (arm A: "no matching item").

**On the 4 items the agents did reach** (cert-manager, external-secrets):
- **C:** 3/4 correct in every run, with external-secrets orders-eu hedged as potentially-affected each time.
- **B:** 3.67/4, with orders-eu `affected` in 2 of 3 runs.

C's consumer-aware design shows no measurable gain over B here, and is slightly more conservative. The criterion
that would show a gain (P1, +2) cannot be met when half the decisive items are unreachable by both arms.

## What the blind set does show

- **Safety held everywhere.**
  - 0 unsafe clearances and 0 false-safes across A, B, C/D and E.
  - On a product and mechanism it was never tuned on, RI stayed at insufficient-evidence or potentially-affected
    rather than clearing.
- **Upgrade judgements (D) were the strongest part.**
  - **Traefik:** in 3 of 3 runs, D found the proposed v3.6.15 introduces **two** BasicAuth advisories and recommended
    v3.7.16.
  - **The ground truth was weaker here.** The lane named v3.6.25 as the safer target. GitHub's database has
    GHSA-8fcf-v89g-xpg6 at `>= 3.6.11, <= 3.7.12` (fixed 3.7.13), so **v3.6.25 still carries a regression.** D was
    right and the label was not. Recorded as a ground-truth correction.
  - **Argo CD:** the agent never examined the log-RBAC item, but D's judgement still named the risk, from RI's own
    potentially-affected findings and the values file: "developers will probably lose log access unless a logs
    policy is added". That is the decisive mechanism.
  - **cert-manager:** D identified the exact break (the controller's ServiceAccount loses `serviceaccounts/token`,
    so the Vault issuer fails at next renewal) and prescribed the environment-owned Role.
- **The learning loop (arm E) does not reach new products.**
  - Formalize turned the 80 assessments into 94 rules and exported 6. The other 88 were refused as not expressible
    (no upstream text, or live-cluster state).
  - **0 of the 6 were confirmed:** there are no exercise fixtures for argo-cd, traefik or external-secrets, and the
    cert-manager rule had no subject kind.
  - So: 0 promotions, 0 review decisions and no new clearances.
  - E1 is not met; E2 holds trivially (no clearance); the E3 review burden is 0.
- **Infrastructure:**
  - one blind crash in the baseline (compare-URL path, fixed under A2);
  - one void run from the account usage limit (A4);
  - 3 unrelated single-environment agent failures (in B r1, B r3 and C r1), kept as results.

## Operational

| Arm | Runs | Agent tokens (total) | Wall time per run | Agent failures |
|---|---|---|---|---|
| A | 1 | 0 | about 1 min | — |
| B (v8) | 3 | 3.33 M | about 27 min | 2 of 24 |
| C/D (v13 + synthesize) | 3 | 5.08 M | 57–60 min | 1 of 24 |
| E (formalize + verify) | 1 | formalize only | 33 min + 1 s verify | — |

Evidence-request, artifact-read and cost detail are in each run's `score.json.gz` (`runs[]`).

## Known limitations, as observed (PD-7)

- **Semantic-check gap:** no misread was observed on the reached items.
- **Observed-default carry-over:** none; no clearance occurred.
- **Auto-approval facts:** none was created.

## What it would take (for the product owner's decision, not started: stop-sprawl order)

The two coverage gaps are generic, small and measurable, and each is outside the reasoning design M6 judged:
1. **Agenda ordering:** when priorities tie, rank breaking (`feat!`, "BREAKING", upgrade-guide) items first, or
   raise the cap per item count.
2. **Change extraction:** the Traefik release-note format produced no item; diagnose that one source.

Re-running B and C on the same blind set after either fix would **no longer be blind** to the commander, so a fair
re-test needs a fresh blind pair.
