# Milestone 2 result (commander only; label-bearing; lanes never read this file)

## §0 Prompt v10 check (scoped verdicts), 2026-10-27: NOT PASSED

**Runs:** `C1-K0-20261027-161c077c-v10-r{1,2,3}`, the cert-manager pair, offline on K0, `claude-sonnet-5-5`.

| Item | US r1 | US r2 | US r3 | EU r1 | EU r2 | EU r3 |
|---|---|---|---|---|---|---|
| ROT `ci-86548e9a2300` (expected: US not-affected, EU affected) | **affected** (false alarm) | potentially | potentially | potentially | potentially | potentially |
| ROT `ci-93299c596271` | potentially | potentially | potentially | potentially | potentially | potentially |
| RHL `ci-25e3f7619968` (expected not-affected) | potentially | **scoped not-affected** | **scoped not-affected** | **scoped not-affected** | **scoped not-affected** | **scoped not-affected** |
| RHL `ci-f86a125e0d17` (expected not-affected) | **scoped not-affected** | **scoped not-affected** | **scoped not-affected** | **scoped not-affected** | **scoped not-affected** | **scoped not-affected** |

- **The pass required** ROT scoped not-affected for US and scoped affected for EU in ≥ 2 of 3 per case. Result:
  **0 of 3 in each.** v10 stays available but does not become the default (v9 remains).
- **Where scoping works.** RHL becomes a correct scoped not-affected in 11 of 12 item-runs, against 8 of 12 under v9
  (attempt 3), with headlines such as "Not affected within the declared Certificates and their traced consumers,
  provided nothing outside the repository relies on old CertificateRequest history".
- **Where it does not.** ROT stays `potentially-affected` in 11 of 12. The agent classifies "do the workloads reload a
  rotated key?" and "does a peer pin the key?" as **in-scope unknowns the mechanism depends on**, not as preconditions:
  "The verdict depends on whether the workloads reload rotated keys" (EU r1). US r1 committed to `affected` while
  listing reload only as a precondition. That is a false alarm on an expected not-affected item.
- **Scope is often missing.** In 5 of 12 ROT assessments it is empty. The validation allows that for
  `potentially-affected`.
- **No false-safe** anywhere. The classification is identical to B0 in 6 of 6 case-runs.

**Reading.** The prompt rule exists, but the model does not apply it to this mechanism. Whether a rotated leaf key
"breaks" a consumer depends on runtime reload, so the agent treats that question as part of the mechanism. Mission §2:
"Do not assume that … changing prompt wording … will independently improve correctness." Tuning v10's wording against
this development pair would be fitting to the case.

**Recommendation to the product owner:** keep v9 as the default and v10 available, and stop iterating the prompt here.
Put the scoped, actionable answer where the mission puts the decision: M3 synthesis ("Proceed with changes: pin `Never`
or confirm reload; validation required: a staging renewal test") and M4 verification (a runtime renewal test settles
reload). Revisit agent-level scoping with M6's blind cases.
