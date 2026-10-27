# Milestone 2 gate (commander only; label-bearing; lanes never read this file)

## 0. Prompt v10 check (D-0007, scoped verdicts; product owner decision 2026-10-26)

Run v10 × 3 on the cert-manager pair: `run-m1-gate.sh` with `-agent-prompt investigate-agent/v10`, offline, on K0.

**Pass:**
- **ROT items:** US is scoped `not-affected` and EU is scoped `affected` in ≥ 2 of 3 per case. Each headline carries
  scope and preconditions, and the reasoning still meets GATE-M1 (a)–(e).
- **RHL items:** no false-safe; their expected class is not affected in both cases.
- **No item anywhere** turns from a correct class into a false-safe.
- **Classification** is identical to B0.

Then v10 becomes the default.

(Sections 1– follow with PLAN-M2.)
