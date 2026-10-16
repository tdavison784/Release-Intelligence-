# Lane `defaults` — status

Branch `p4ei/defaults` (from p4-env-impact @ f467a54d). Brief: `briefs/defaults.md`.

## Done
- `internal/upgrade/values_defaults.go`: `FromDefaults(change)` recovers each subject's source default
  (JSON) from the Detail `diffValues` writes (removed, section-removed, default-changed); `EmptyDefault`.
  Round-trip tests against the real diff (incl. separators inside values, truncated sections).
- Join (step 1): new rule `impact:values-removed-default` (review-required, confidence medium,
  `absence` matches citing the values files) for a removed key the env leaves unset whose source default
  is non-empty, or unrecorded in a truncated section enumeration. Empty defaults (`null`, `""`, `{}`,
  `[]`) keep `impact:values-unset`. Tests: `internal/impact/removed_default_test.go`.

## In progress (next step)
- Step 2: correlation rule in `internal/render/correlate.go` (rendered change whose before holds the
  key's FROM default and whose after no longer does), then Phase 3 eval after-run and delta table.

## Decisions
- Default-changed keys left unset stay with PO-3 (product-owner decision): the counterfactual render
  decides (`values-default-no-effect` is a counterfactual proof, not render silence); without a render
  `values-default-unrendered` stays not-affected in the five-class report, and the investigation's
  `validate.join` already treats it as a gap (never a clearing pass). So only the correlation part of
  the brief applies to default-changed.
- A change that records no source default at all (hand-built, e.g. eval/adversarial fixtures) keeps
  today's verdict: there is no default known to have been relied on. Real values diffs always record it.

## Files touched outside ownership
- `internal/upgrade/values_defaults.go` (+test): new, additive — parser lives next to the format writer.

## Contract changes
- none

## Phase 3 eval (before = f467a54d)
- before: gates ✗ applicabilityAccuracy 0.525, ✗ falseActionRate 0.059 (both pre-existing); others ✓.

## Test status
- go test ./internal/{upgrade,impact,eval,render} green.
