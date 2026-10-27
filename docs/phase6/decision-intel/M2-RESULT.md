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

**Product-owner decision, 2026-10-27:** "yes thats fine for v9".
- v9 stays the default prompt; v10 stays available.
- No further prompt iteration on the development pair.
- The scoped answer moves to M3 synthesis. **M3's gate must check that the upgrade recommendation commits, with its
  scope and preconditions in the headline** (mission C5, F3). If synthesis cannot, agent-level scoping returns, tested
  on blind cases (M6).

## §1 Advisories, 2026-10-27: docs-eu MET; cert-manager PARTIAL (one advisory-consistency gap)

**Runs:** `A0-K0-20261027-965ba258-adv-{deu,cm}`. These are deterministic, with no agent. They replay offline from a
state recorded online the same day (`record-m2-advisories.sh`), after the merges of `advisories` (d73485b8) and
`advisories2` (965ba258).

**docs-eu (ingress-nginx 1.11.5 → 1.12.0): every GATE-M2 §1 item holds.**
- **Regressions:** the 5 expected (mgvx critical; 823x, fwwp, vg63 high; 242m medium), all from the global source.
  The repository source had 0.
- **Persisting:** the 4 expected (`< 1.13.7 …`).
- **Unassessed:** GHSA-f53h (pseudo-version range), with a gap, per amendment A1.
- **Candidates:**
  - `target-line` and `any-line`: 1.12.1, chart 4.12.1, both cited (chart index entry).
  - `clean`: 1.13.7, chart 4.13.7.
  - `current-line`: none.
- **Statement example:** "controller-v1.12.1 is the lowest known release on line 1.12 at or above controller-v1.12.0
  that removes every regression of controller-v1.12.0."

**cert-manager pair (1.17.2 → 1.18.0):**
- **Regressions:** both expected. GHSA-8rvj (high) comes from the repository source only. GHSA-gx3x (medium) comes
  from both sources and is merged; the explicit-list copy and the global ranges are unioned.
- **`target-line`:** "No known release on line 1.18 at or above v1.18.0 removes every regression of v1.18.0." As
  expected.
- **`any-line` and `clean`: 1.20.3, where 1.19.6 was expected.** GHSA-8rvj states `>= 1.18.0, <= 1.20.2` and lists
  `patched: 1.19.6, 1.20.3`. The patched 1.19.6 lies inside the stated range: **the advisory contradicts itself.** RI
  follows the range, the strict reading, and does not flag the contradiction. An engineer would want: "1.19.6 is listed
  as patched but falls inside the stated range; check the advisory; if the patched list is right, 1.19.6 qualifies".
  This is the one gap.
- **Charts:** cert-manager's chart version follows a name template, not a chart index. It is stated in `chartGaps`
  and not cited, which is correct.

**Policy:** against the pre-advisories binary on identical bytes (base 0c89d9b1 comparison), no existing item changes
class. The new advisory change items are all `potentially-affected`, so the effect is only stricter.

**Next:** a small generic fix inside M2 (D-0012). When a patched version falls inside the advisory's own vulnerable
range, record an advisory-consistency gap. Candidates stay fail-closed (on the range), and the statement names the
alternative the patched list implies. Then re-run §1 offline from the same state.

**Product-owner decision, 2026-10-27:** "Im fine with it recomending 1.20.3, that could be newer since it last
recommended 1.19.x." The cert-manager `any-line` and `clean` result of 1.20.3 is accepted. D-0012 (the consistency
gap) continues as an informational improvement and does not change that answer.

### §1 re-run after D-0012 (advisories3 merged), 2026-10-27: MET

Run `A0-K0-20261027-aed956a9-adv-cm`, offline from the same recorded state.
- The cert-manager `any-line` and `clean` candidates stay 1.20.3 (accepted by the product owner). Each statement adds:
  "If GHSA-8rvj-mm4h-c258's patched list is right, v1.19.6 also qualifies; it is not chosen because the stated range
  includes it."
- The section lists the inconsistency (range `>= 1.18.0, <= 1.20.2`, contradicting patched 1.19.6) with a gap and a
  warning.
- **GATE-M2 §1 is met for both development edges.**

## §2 Strict-path semantics, 2026-10-27: live MET; ablation FAILED (the fetch is optional, so memory fills the gap)

**Runs:**
- **Live:** `C1-K0-20261027-c8b67f09-v11-deu-r{1,2,3}`, online. Upstream fetches are allowed.
- **Ablation:** `C1-K0-20261027-c8b67f09-v11-deu-abl-r{1,2,3}`, offline, from a state without upstream source caches.
- **Both:** prompt v11, D-EU.
- **Item:** `ci-0e8570d62156` (#11819 "Enable security features by default", which carries B1–B4). Expected: affected,
  through B3.

| Run | `request_evidence` on the item | Verdict | B3 |
|---|---|---|---|
| live r1 | 2 × `upstream-search` at controller-v1.12.0 (`internal/ingress/controller`, `internal/ingress/inspector`), ok | potentially-affected | not FS; assertion cites the fetched inspector source |
| live r2 | 2 × `upstream-search`, ok | potentially-affected | not FS: "the regex does not accept '.'" |
| live r3 | 2 × `upstream-search`, ok | potentially-affected | not FS |
| abl r1 | **none** | **not-affected** | **FS**: "Both paths are Prefix paths with plain values" |
| abl r2 | **none** | **not-affected** | **FS**: "'/' and '/.well-known' … are valid under strict validation" |
| abl r3 | **none** | **not-affected** | **FS**: "… with pathType Prefix, which are valid" |

**Live: met.** B3 is not false-safe in 3 of 3, and every run fetched the validator source at the target tag. Against
Phase 5 arm B (FS) and Phase 0 (5 of 6 natural runs FS), this is the first time the RI agent has grounded this item in
upstream code.

**Ablation: failed.** B3 is false-safe in 3 of 3, and no run recorded "behaviour unavailable".
- **The ablation did not exercise the cache-miss path,** because the agent never asked. The upstream port was wired
  and the item prompt named the readable repository and tags in every run (the same prompt digest everywhere).
- **The first six tool calls are the same in live and ablation runs.** Only the live runs then chose to fetch.
- **The ablation runs' own unknowns admit the gap:** "Behaviour of the 1.12.0 validation code, which I did not fetch"
  (r1). They concluded not-affected anyway.
- **Reading:** fetching is a model choice. When it is skipped, remembered behaviour returns as a false-safe. v11's
  rule ("cite it or record it unavailable") is not enforced by anything deterministic. Mission §2 warns exactly this
  about prompt wording.

**Next (inside M2): lane `upstream2` (D-0013).** A deterministic check, in the style of M1's consumer check: a
not-affected verdict on an item whose outcome rests on upstream behaviour must cite that behaviour from upstream
evidence beyond the change text (a fetched file, or a source artifact), or carry a recorded unavailable request.
Otherwise it is rejected once, and the gap is recorded on resubmission. Then re-run live and ablation × 3.
