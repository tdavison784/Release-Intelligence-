# Lane join5: status (Milestone 4 fix; ledger D-0028)

Branch `p6di/join5` (from `p6-decision-intel` @ fac5f22c). Brief: `briefs/join5.md`. Two defects: D1 (the knowledge
join evaluates source files, not rendered objects) and D2 (observed role-membership defaults not applied in the pin
scope).

## Done

1. **D2 root cause found and fixed.** `app.LoadKnowledge` (the reader `ri investigate`, `ri impact` and `ri eval` use)
   never loaded `proof` records. `impact.VerifyFacts` then found no proof for any observed (PD-5) or role-membership
   (PD-6) default and ignored every one ("proof … does not resolve"), so the knowledge join and the pin scope (whose
   facts come from the same loader, through the bundle and `validate.WithFacts`) never saw them. The evaluators, the
   digest matching and the fact plumbing were correct; the existing tests built snapshots in memory or through the
   file store's own reader, never through `LoadKnowledge`. Fix: load `RecordProof`. Also: the investigation CLI
   dropped the loader's warnings and errors silently (now printed / returned), and an ignored default is reported as
   "fact default ignored", not "fact refused" (the fact is kept).
   Tests: `verify.TestMembershipDefaultSurvivesLoadKnowledge`, `TestObservedDefaultSurvivesLoadKnowledge` (the whole
   path: confirmed proof → human promotion → files → `LoadKnowledge` → join decides the unset object; without the
   proof on disk the default is ignored with a warning and the object is undecidable). Both fail on the old loader
   with exactly the D2 symptom.

## In progress (exact next step)

- D1: effective object view for the impact join (`internal/env/effective.go`), built in `app.ImpactRun`.

## Decisions

## Files touched outside ownership

- `internal/investigate/verify/probe_test.go`: test helper records the store directory (additive).

## Contract changes (additive)

## Open questions

## Test status

- D2 part: `go build ./...`; `go test ./internal/app ./internal/investigate/verify ./cmd/ri` green.
