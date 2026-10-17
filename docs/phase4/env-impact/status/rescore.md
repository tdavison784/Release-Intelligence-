# Status — lane `rescore` (Claude Sonnet 5.5, agent ri-rescore)

Branch `p4ei/rescore` (from p4-env-impact d1ed12aa). Brief: `briefs/rescore.md`.

## In progress / next
1. Score arm A (+ control A with the pre-review knowledge store from e62355fc^, to separate store effect from the
   27 code commits since 0790b001), then arm C (runtime), new run ids, `-save`.
2. Explain the two ingress-nginx accepts that composed no fact.
3. EVAL-REPORT section "Human-verified facts (2026-10-16)".

## Decisions
- Harness already defaults to `-knowledge knowledge` and minimum verification `human` (`cmd/ri/investigate.go:221`),
  so the runs use default flags.
