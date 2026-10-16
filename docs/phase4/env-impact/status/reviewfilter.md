# Status — lane `reviewfilter` (Claude Sonnet 5.5, agent ri-reviewfilter)

Branch `p4ei/reviewfilter`, worktree `.claude/worktrees/p4ei-reviewfilter`. Brief: `briefs/reviewfilter.md`.
State: **IN PROGRESS** (2026-10-16).

## Done
- `internal/reviewui/triage.go`: per-item verdict (needs review / hidden + reason / blocking), `BlockingScope`
  (`ScopeFromInvestigations` reads `eval/investigations/*/input.yaml` products — inputs only, never expected.yaml —
  and the upstream repos their `products/<p>.yaml` read).
- Server: default `/` = Needs review, `?scope=all` = "Show all staged data"; counts/tiles reflect the default view;
  blocking-first ordering; hidden tally with reasons; server fails open (an item the snapshot doesn't know is shown).
- `validate.CanBePinSource` (additive export in `internal/investigate/validate/pins.go`).

## In progress / next
- triage + server tests; `ri review serve` wiring (`-investigations`, `-products` flags, prints waiting count);
  docs/REVIEW_UI.md; browser check on a non-8484 port with screenshot under `docs/phase4/env-impact/review-ui/`.

## Decisions
- Needs review = status pending ∧ no decision of any kind ∧ no active fact holding the item's aspects at
  human/deterministic level ∧ (audit items) audited fact still active. needs-evidence/deferred are decisions → hidden
  (reachable via show-all), per the brief's "no human decision yet".
- Blocking = item of an investigated product (or of an upstream repo an investigated product reads) whose candidate has
  only prose members (⇒ manifest:declared-behaviour, controller-behaviour) and no trusted fact. Items whose proposed
  assertion `CanBePinSource` (pin rule derivation) rank first, other blockers next, the rest after (stable inside).
  Approximation: computed `crd:default-changed` candidates (also controller-behaviour) are not flagged (rule not on the candidate).
- Existing inbox tests now use `getAll` (full view); behaviour of the full view is unchanged.

## Files outside ownership
- `internal/investigate/validate/pins.go`: additive `CanBePinSource`.
- `cmd/ri/review.go` (+ main.go usage line): flags/wiring.

## Open questions
- none yet
