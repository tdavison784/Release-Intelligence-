# Lane `factlevel` — make human review actually verify pin-source facts — Opus

Branch `p4ei/factlevel`, worktree `.claude/worktrees/p4ei-factlevel`. Read FLEET.md, `docs/KNOWLEDGE.md`,
`docs/REVIEW_UI.md`, `docs/phase3/learning-loop/DESIGN.md` (aspects, question types, verification levels),
`status/rescore.md` (the finding), `internal/domain/semantic.go` (fact level = weakest aspect, ~L2352),
`internal/reviewui/triage.go`, `cmd/ri/review.go` (open-audits). You own `internal/reviewui`, `internal/knowledge`
(minimal), `cmd/ri/review.go` for this. Never open eval expectations.

## Finding (lane rescore)

The product owner accepted the 5 pin-source items, but the three cert-manager facts stay `consensus`: only their
**consequence** aspect became `human`; change and applicability stayed consensus, and a fact's level is its weakest
aspect. The review UI then hid the remaining aspects because it judges an item by the aspect it asks about, not by
the fact's level — the reviewer was shown "done" when the facts were not verified. ingress-nginx `--enable-metrics`
(candidate sc-298dac696f1a): its consequence question ri-246f31433fd5 is pending but sits under "Other blocking".

## Do

1. **Needs-review by fact level**: for pin-source facts/candidates, "needs review" = any aspect below human (or no
   fact yet because a required question is open). The default view lists every open question needed to make each
   pin-source fact human — including sibling questions of a candidate (the ingress consequence question) — grouped
   per fact with a one-line "N questions left to verify this fact".
2. **Whole-fact verification item** (new, explicit): `open-audits` opens, for a pin-source fact below human, ONE
   fact-review item that shows every aspect (subject, change, applicability, consequence) and asks "Is this whole
   fact correct?". Accepting it raises every aspect it displayed to human (record which aspects in the decision);
   "correct" records corrections per aspect; reject retracts. If the Phase 3 contract forbids this, implement the
   per-aspect alternative instead and say why. Keep proxy decisions never human.
3. **No silent upgrade of past decisions**: the product owner's 2026-10-16 decisions stay exactly as recorded (they
   verified the consequence aspect). Re-open the remaining work as new items (whole-fact items per 2) so the product
   owner can finish in a few clicks.
4. Apply `open-audits` to the real store (opens items only, verifies nothing), confirm the default view lists the
   right items (expect the 3 cert-manager facts + the ingress `--enable-metrics` candidate), report titles + counts.

Tests for each rule (fact-level triage, sibling questions surfaced, whole-fact accept raises displayed aspects only,
proxy never human, past decisions untouched), build/vet/test green incl. store integrity, status, `LANE DONE:`.
