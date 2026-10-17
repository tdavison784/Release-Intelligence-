# Status — lane `factlevel` (Claude Opus 5.5, agent ri-factlevel)

Branch `p4ei/factlevel` (from p4-env-impact 2b064185). Brief: `briefs/factlevel.md`. State: **DONE** (2026-10-16).

## Done
1. **Needs-review by fact level** (`internal/reviewui/triage.go`): pin-source *groups* are active pin-source facts
   (scope + controller behaviour + `CanBePinSource` on the fact), or, while no fact is composed, a candidate one of whose
   items (any status but superseded) proposes a pin-source assertion. A group needs review while any aspect is below
   human. Every open item of the group's candidates is a pin source (sibling questions included); the default view sorts
   them per group under a header with the aspect levels and "N questions left to verify this fact" (`PinGroup.LeftLine`).
   A fact below human with no open item is listed in a note above the cards (run `open-audits`).
2. **Whole-fact item**: new question type `fact-review` (verifies all four aspects) and `knowledge.OpenWholeFactReview`
   (idempotent; never touches facts or decisions). The item page has a "The whole fact, every aspect" table (aspect,
   what the fact says, level now, effect of accept), primary button "Yes — the whole fact is correct". The UI records
   the displayed aspects in the new `ReviewDecision.Aspects`; `candidateState` verifies only `d.Verifies(item)`;
   `ValidateFactRecords` refuses an `rd-` basis for an aspect the decision did not record. Correct = per-aspect
   `wrong-*` labels + supersede; reject = retract (existing fact-review paths). Proxy accepts stay proxy (never human).
   Bulk accept of a whole-fact item needs it opened first.
3. **No silent upgrade**: earlier decisions are never rewritten; the remaining work is new items. On accept the
   product owner's audit decision stays the consequence basis (test: decision file byte-identical).
4. **Real store** (`knowledge/`, commit d214e440): `ri review open-audits -knowledge knowledge -apply` opened 3 items,
   nothing else changed; a re-run opens 0. Default view (verified on a spare port, read-only) = 4 questions in 4 groups:
   - `vf-62c94002eb36` cert-manager 1.18.0 "Set the default `revisionHistoryLimit` to 1 for the CertificateRequest
     revisions" → `ri-55fca8c3dfbb` (fact-review)
   - `vf-9b9cfa5c9f5f` rotationPolicy Never → Always → `ri-125b8856cb8b` (fact-review)
   - `vf-b577957e2615` `Certificate.Spec.RevisionHistoryLimit` nil → 1 → `ri-86967779ec0c` (fact-review)
     (all three: subject deterministic · change consensus · applicability consensus · consequence human)
   - `sc-298dac696f1a` ingress-nginx 1.12.0 "⚠️ Metrics: Disable by default" (no fact yet; subject/change/applicability
     human) → `ri-246f31433fd5` (consequence)
   Startup line: "4 pin-source fact(s) below human verification, 4 question(s) to verify them (listed); 205 other
   blocking and 631 other open item(s)". A scratch-copy run accepted `ri-125b8856cb8b` through the UI: the fact became
   subject deterministic / change human / applicability human / consequence human (basis still `rd-12f068bf62c6`).
- Screenshots: `docs/phase4/env-impact/review-ui/03-factlevel-default-view.png`, `04-whole-fact-item.png`,
  `05-whole-fact-item-narrow.png` (headless Firefox, ports 18631–18633, all stopped).
- Docs: REVIEW_UI.md (default view by fact level, whole-fact review, open-audits), KNOWLEDGE.md, README + usage line.

## Decisions
- The Phase 3 contract does not forbid a question verifying all four aspects (DESIGN §2.4: `relationship` verifies
  three; §2.6 only requires the `rd-` basis item's question to verify the aspect), so the whole-fact item is a new
  question type, not the per-aspect alternative.
- The audit item (`classification`, consequence only) and `OpenFactReview` are unchanged, so pipeline audit sampling and
  the R19/PO-6 metrics keep their meaning. Triage now judges a pending audit item by the aspect its accept verifies
  (consequence) instead of all four; the whole-fact item is what makes the fact human.
- A candidate group with no fact and no open item (e.g. its mapping was rejected) is not listed.

## Contract changes (additive; CONTRACT-CHANGE(factlevel) in code)
- `domain.QuestionFactReview = "fact-review"` (in `QuestionTypes`; `QuestionAspects` = all four).
- `ReviewDecision.Aspects []Aspect` (`aspects,omitempty`), `ReviewDecision.Verifies(item)`; `Validate`: accept/correct
  only, valid, unique, stated by the final assertion. Schemas regenerated. DESIGN §2.4/§2.5 got one row / one bullet.

## Files touched outside ownership
- `internal/domain/semantic.go`, `schemas/knowledge-record.schema.json` (above), `docs/phase3/learning-loop/DESIGN.md`
  (additive row + bullet), `docs/KNOWLEDGE.md`, `README.md`, `cmd/ri/main.go` (usage line),
  `knowledge/cert-manager/1.18.0/reviews/` (3 new items, step 4).

## For the commander
- Merging into p4-env-impact (86cc09fa) conflicts in two places, both trivial: `cmd/ri/main.go` usage lines (keep both)
  and `internal/reviewui/static/app.css` (both lanes appended a block; keep both). Not merged here (FLEET).
- The product owner's server on 8485 runs p4-integration code via `go run`; it needs a restart after the merge to show
  the grouped view and the whole-fact page. It was never stopped by this lane.
- Data note: `vf-62c94002eb36` and `vf-b577957e2615` have the same subject (`Certificate.spec.revisionHistoryLimit`
  default → 1) from two candidates; the reviewer may want to reject one as a duplicate.

## Test status
`go build ./... && go vet ./... && go test ./...` green (incl. `TestKnowledgeDirectoryNeverReferencesEvalCases` over the
real store). New tests: `internal/knowledge/wholefact_test.go`, `internal/reviewui/factlevel_test.go`.
Cleanup: `bin/` removed, no kind clusters, no servers left running; scratch stores/profiles are in the session scratchpad.
