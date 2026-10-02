# Lane `rerun` status: DONE

Branch `p3ll/rerun`. Writes only under `docs/rerun/` + this file. Full results: `docs/rerun/REPORT.md`.

## Done
- Live `ri check` x28 (same release sets, fresh per-product state, `-refresh`), `compare.py`, `report_tables.py`.
- Discovery x25 pinned to baseline refs + old-binary controls (isolate network vs tool evolution).
- `ri drift` x28 (vs baseline, vs live, newest-3), `ri stats` over live checks, `ri eval` live (read-only).
- 129 research-claim probes + content/registry-auth probes.
- REPORT.md: classification (a sandbox / b genuine / c upstream changed / d defect / e definition changed / t throttling), recommendations.

## Headline
- 212 differences are sandbox artifacts (unverifiable/covered -> pass). Real defects: ingress-nginx controller-chroot v1.10.0
  absent upstream (D1); argo-cd v3.4.0 is a tag, not a release (D2, FINDINGS claim wrong); 429 reported as unavailable (D3);
  baselines stale vs definitions (D4); eval gates applicabilityAccuracy 0.495 and falseActionRate 0.059 fail (D5, not network).

## Incident
ENOSPC mid-run (parallel discovery/drift clones). That batch was discarded and re-run serially; nothing disk-failed is counted as unavailable.
Stale `raw/*.disc.started|finished` markers remain (rm blocked by a safety check).

## Decisions
- argo-cd 3.4.0 -> 3.4.1 for the live check. No product definitions or originals touched.

## Files outside ownership
None. No Go changes; `go build ./... && go vet ./... && go test ./...` pass.

## Open
kube-prometheus-stack old-binary control; drift vs an older baseline; body-level claim verification.

## Phase 12 follow-up (branch `p3ll/phase12-fix`)

After LANE DONE, the lane continued on `p3ll/phase12-fix`, executing REPORT.md recommendations 1–4.
Full record: `docs/rerun/REPORT.md` §11.

- Definition fixes: ingress-nginx chroot exception (D1) + argo-cd exception wording (D2) (`bf9889a`);
  vault `helm-repo` primary and crossplane/external-secrets canonical chart channels first, dated
  provenance notes (`65cdb8f`).
- Pipeline fixes (`cb81c58`): distinct `throttled` state with Retry-After backoff + per-host
  concurrency, 401/403/000-DNS reasons, stale-baseline digest check, `definitionDigest` in check output.
- 14 baselines replaced with the post-fix re-runs (`docs/rerun/phase12/`, fresh state, same `-versions`;
  argo-cd 3.4.0→3.4.1 per D2). Totals: 2766 pass / 922 not-applicable / 17 covered /
  **0 failed / 0 unavailable / 0 unverifiable / 0 throttled**. Karpenter's 429s were throttling
  (sequential re-run clean); falco/minio and the 12 untouched products unchanged.
- Dated 2026-10-02 corrections in README, FINDINGS.md, phase2 PLAN/OUTCOMES, all 30 `docs/research/*.md`
  (originals kept as the 2026-10-01 record); Corrections section added to the stats renderer;
  `docs/ONBOARDING.md` regenerated from the live baselines.
- `go build ./... && go vet ./... && go test ./...` pass.

## GLM handoff log

Held by GLM-5.3 (glm-rerun) from 2026-10-02 08:02 while the lane's Claude agent paused for a
usage-limit window. Found phase12 mid-flight — 14 live re-checks on disk untracked, doc corrections
uncommitted, baselines replaced for only the 12 pre-pipeline-fix products — and finished it:

1. Committed the phase12 raw outputs as-is (`docs/rerun/phase12/`, empty stderr captures).
2. Replaced the 14 `docs/onboarding/checks/` baselines with the phase12 outputs; release sets equal
   the old baselines except argo-cd 3.4.0→3.4.1 (the recorded D2 decision).
3. Committed the doc corrections after updating their karpenter/argo-cd wording to the final phase12
   outcome — they were drafted mid-run and not yet committed, so updated in place rather than stacking
   a second same-day correction.
4. Rebuilt `bin/ri` (the checked-in binary predated the renderer edit) and regenerated
   `docs/ONBOARDING.md`; verified the new G2 numbers (0 unverifiable for all 14).
5. Added REPORT.md §11 (recommendations status) and this log; ran build/vet/test.

Uncertainties / judgement calls, for the commander:

- Replacing the 12 already-replaced baselines again (pre-fix → post-fix output) was read as intended:
  `cb81c58` adds `definitionDigest`, which the stale-baseline check compares, and outcomes are
  identical either way. If unwanted, `git revert` of that single commit restores the 07:52 versions.
- `docs/rerun/stats.json|stats.md` left as the earlier measurement record; regenerating them would now
  read the replaced baselines (ONBOARDING.md is the authoritative regenerated doc) — commander's call.
- The FLEET.md gate-baseline note (D5) was not added: FLEET.md is the commander's file.
- Recommendation 5 items remain open, as before.
- Commit subjects carry the required `[glm-handoff] ` prefix; co-author line follows the FLEET.md
  convention with this agent's model name (`GLM-5.3`).

## phase12-fix: Claude review after the GLM handoff (2026-10-02)
Reviewed the six `[glm-handoff]` commits: the 14 baselines equal the phase12 outputs (0 fail, 0 unverifiable, `definitionDigest`
recorded), ONBOARDING.md equals `ri stats -o markdown`, doc corrections are dated notes; no code or definitions were changed by the
handoff. Verified: `go build/vet/test ./...` pass; `ri drift` over all 28 products live: 0 events (14 baselines `current`, 14
legacy `unrecorded`); `ri eval`: no regressions, identical to `p3-learning-loop` (applicabilityAccuracy 0.476 and
falseActionRate 0.059 gates fail on both: pre-existing, not caused by this lane). Outputs: `docs/rerun/phase12-drift/`,
`phase12-eval*.txt`. D5 note for FLEET.md is left to the commander.
