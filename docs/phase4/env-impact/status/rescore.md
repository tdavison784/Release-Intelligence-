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

## Done so far
- Arm A `A-20261017T020337Z-d1ed12aa8d` (clean, default flags = `-knowledge knowledge`, min `human`): identical gates to A₅
  (7 PASS / 5 FAIL / 1 INSUFFICIENT-DATA). 0 of 24 decision classes moved; 3 reasoning strings differ only in the
  order of cited validation ids (shop-nl B1, obs-north/south B3).
- Control `A-control-prereview-20261017T020459Z-d1ed12aa8d`: same code, knowledge store from e62355fc^ → identical
  to the run above. So the human review changed nothing end to end (and the 27 code commits since A₅ changed no class).
- Cause (verified in code + data): `VerifiedFact.Level()` (`internal/domain/semantic.go:2352`) is the WEAKEST aspect.
  The 3 cert-manager facts have subject deterministic, change/applicability `consensus`, only consequence `human`
  ⇒ fact level `consensus` ⇒ dropped by `-min-verification human` (`internal/knowledge/filestore.go:414`) and not trusted
  by `override.go:344`. The review UI marks the item "verified" because `trustedAspects` checks only the aspects the
  question asked (consequence).
- Ingress-nginx: ri-29af5ead5e5f accepted subject+change, ri-57cfa14b0bff accepted applicability; the consequence item
  `ri-246f31433fd5` (candidate sc-298dac696f1a) is still `pending` ⇒ no fact composed. It is NOT in the default
  view (0 pin-source items), it is in `?scope=blocking` ("Other blocking items") because its proposed assertion has
  only a consequence so `CanBePinSource` is false.
## Next
- Arm C running (`$scratch/c.log`), then save/commit, write EVAL-REPORT section, build/vet/test, cleanup, LANE DONE.
