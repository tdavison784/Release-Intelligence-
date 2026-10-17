# Status — lane `factlevel` (Claude Opus 5.5, agent ri-factlevel)

Branch `p4ei/factlevel` (from p4-env-impact 2b064185). Brief: `briefs/factlevel.md`.

## Plan
1. Domain (additive, CONTRACT-CHANGE(factlevel)): question type `fact-review` (verifies all four aspects) and
   `ReviewDecision.Aspects` (the aspects the reviewer was shown and verified; empty = the question's aspects).
2. knowledge: `OpenWholeFactReview` (one `fact-review` item per fact); `candidateState` / `ValidateFactRecords`
   honour `Aspects`.
3. reviewui: fact-level triage (pin-source groups per fact / candidate, sibling questions, "N questions left");
   whole-fact page section; decision records the displayed aspects.
4. `ri review open-audits` opens whole-fact items for pin-source facts below human; apply to `knowledge/`.

## In progress / next
- Step 1.

## Decisions
- The Phase 3 contract does not forbid a question that verifies all four aspects (DESIGN §2.4: `relationship`
  already verifies three; §2.6 only requires the `rd-` basis item's question to verify the aspect). So the whole-fact
  item is a new question type, not the per-aspect alternative.
- The existing audit item (`classification`, consequence only) and `OpenFactReview` are unchanged (pipeline audit
  sampling and R19 metrics keep their meaning); the whole-fact item is a separate item id.

## Files touched outside ownership
- (none yet)

## Test status
- not run yet
