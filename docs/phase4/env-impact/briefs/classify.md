# Lane `classify` — four-class policy, findings, approval eligibility, review (MISSION §A6, AC6, AC8) — Opus

Branch `p4ei/classify`, worktree `.claude/worktrees/p4ei-classify`. Read `_wave2-common.md` first.

Implement DESIGN §3 and §7 "classify" exactly (owns `internal/investigate/classify/`): `classify.New()
investigate.Classifier` (rules R1–R4 in order, incl. R2.1 per-family collection, R2.4 manifestation —
render silence never clears controller-behaviour/unknown items, only a pinned setting or an observed
validation — the §3.3 agent-record table, the tier-policy conjunction, NextSteps that name the setting to
look for) and `investigate.ReviewApplier` (human review: approve / reject / request-more-evidence; human-only
approval; `AutoApprovalEnabled` stays false in milestone 1). Findings must distinguish validated conclusions
from unresolved assumptions on their face (AC6) — check the rendered text too, not only the struct.

Done: a test per rule and per §3.3 row (incl. adversarial: an agent claims-mitigation with no validation
never clears; an absent setting never clears a changed default; an uncorrelated rendered change blocks
eligibility); every output `ValidateAgainst`s its bundle; a reproducibility test (same inputs → same
finding digests); ACTION REQUIRED contract tests (§3.5).
