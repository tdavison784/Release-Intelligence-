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

## 1. Advisories (deterministic; lane `advisories`; no agent)

Recorded online once into a cloned state (`GITHUB_TOKEN` from `gh auth token`), then replayed with `-offline`. Expected
values come from GitHub on 2026-10-26.

### ingress-nginx 1.11.5 → 1.12.0 (chart 4.11.5 → 4.12.0; D-EU `eval/investigations-heldout/ingress-nginx-1.11-1.12--docs-eu`)

- **Regressions (5).** Each is global-only: the repository has 0 advisories. Each has the range
  `< 1.11.5 || >= 1.12.0-beta.0, < 1.12.1`.
  - GHSA-mgvx-rpfc-9mpv (CVE-2025-1974, critical)
  - GHSA-242m-6h72-7hgp (CVE-2025-24513)
  - GHSA-fwwp-xcxw-39vq (CVE-2025-24514)
  - GHSA-823x-fv5p-h7hw (CVE-2025-1097)
  - GHSA-vg63-w3p9-jc9m (CVE-2025-1098)
- **Lowest release on the target's line removing them:** 1.12.1, chart 4.12.1.
- **Persisting:** GHSA-2pf9-vr92-6h3v, GHSA-9h3p-52vh-959w, GHSA-4g2f-xcph-2335 and GHSA-jx8c-56mg-h6vp, each
  `< 1.13.7 || >= 1.14.0, < 1.14.3`. The lowest release free of every known advisory is 1.13.7. GHSA-f53h-mxv9-cp98
  (pseudo-version range `< 0.0.0-2026…`) must not mark 1.x as affected.

### cert-manager 1.17.2 → 1.18.0 (both payments cases)

- **Regressions (2):**
  - GHSA-8rvj-mm4h-c258 (CVE-2026-62290, high): repository-only, `>= 1.18.0, <= 1.20.2`, patched 1.19.6 and 1.20.3.
  - GHSA-gx3x-vq4p-mhhv (CVE-2026-25518, medium): global `>= 1.18.0, < 1.18.5 || >= 1.19.0, < 1.19.3`. The repository
    copy is an explicit version list; the machine-readable copy must win.
- **Lowest release on the target's line removing every regression:** none. 1.18.5 removes gx3x only.
- **Lowest release removing every regression:** 1.19.6.
- GHSA-r4pg-vg54-wxx4 and GHSA-ghw8-3xqw-hhcj (`< 1.16.2`) affect neither version.

### Pass

- All of the above, each statement with advisory id, source and range evidence.
- Shown in the impact report's security section.
- The authoritative classification is unchanged, or stricter only through `advisory:regression`.
- No offline cache miss is reported as "no advisories".
- B0 and C1 histories still re-score and replay.

## 2. Strict-path semantics (agent; lane `upstream`, prompt v11)

D-EU, item B3 (`strict-validate-path-type` enabled by default; the Ingress has `/.well-known` Prefix). The expected
class is **affected**: the 1.12.0 validator does not accept the path as written. Phase 0 arm B and 5 of 6 natural runs
were false-safe on it, from memory ("." allowed).

- **Run:** v11 × 3 on D-EU, offline, from a state with the upstream fetches recorded.
  - **Pass:** B3 is not false-safe in ≥ 2 of 3. Each non-false-safe run cites the validator implementation or its test
    at `controller-v1.12.0`, fetched through `request_evidence`. No run states the accepted character set without a
    citation.
- **Ablation:** v11 × 3 with upstream fetching unavailable (an offline state without those cache entries).
  - **Pass:** B3 is never false-safe; each run records "behaviour at controller-v1.12.0 unavailable".
- **Both runs:** the other D-EU items get no new false-safe, and the classification is identical to the stored D-EU
  baseline.

