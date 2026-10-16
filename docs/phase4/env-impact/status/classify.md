# Status — lane `classify` (Claude Opus 5.5, agent ri-classify)

Branch `p4ei/classify`, worktree `.claude/worktrees/p4ei-classify`. Brief: `briefs/classify.md`
(+ `_wave2-common.md`, `_wave1-common.md`, DESIGN §3 / §7 "classify"). Merged `p4-env-impact` @ 359605dc
(wave 1 complete, incl. §1.3 amendment items in `EvidenceBundle.EnvironmentItems`).
State: **IN PROGRESS**. Resume from: this file + brief + `git log`.

## Done

- `internal/investigate/classify/` first cut (builds, vets): `policy.go` (R1–R4, R2.2–R2.7, §3.3, §3.5
  action-required guard), `collection.go` (R2.1 declarative family table), `reasons.go` (reasoning,
  remediation, NextSteps incl. setting extraction for behaviour items), `decision.go` (§3.4 + tier
  conjunction, shadow mode), `review.go` (`NewReviewApplier`, `Apply`), `text.go` (`WriteText`, AC6).
- Contract change (api.go): `Config.TierPolicy`, `ClassifyInput.Tier`, `TierVerdict`.

## In progress / next step

- Tests: per rule, per §3.3 row, adversarial, decision, review, reproducibility, ValidateAgainst on every
  output, ACTION REQUIRED contract (§3.5), AC6 text. Then docs/INVESTIGATE.md "Classification" section,
  opt-in groundtruth integration test.

## Decisions (with why)

(see final version)

## Contract changes

- `// CONTRACT-CHANGE(classify)` `Config.TierPolicy string` (digest of the configured tier policy; "" = none),
  `ClassifyInput.Tier *TierVerdict`, type `TierVerdict{Policy, Digest, Tier, Why}` — the §3.4 tier-policy
  conjunction had no input. Additive, omitempty (existing config digests unchanged).
