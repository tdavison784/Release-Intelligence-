# Lane `fixes5` — status

Branch `p5ll/fixes5`. Brief: `briefs/fixes5.md`. Ledger W-0024, W-0025.

## Done
- **W-0024 run hijack** (`internal/investigate/run/keys.go`). Root cause, broader than reported: a memo hit
  returns the first call's whole `StageOutput`, refs included, but keys did not cover the refs a stage passes
  on. build-graph's key (bundle, builder) ⇒ second config continued the first investigation; snapshot's key
  omitted the change ref ⇒ a second change on the same env would have continued with the first change;
  collect's key omitted `Config.Render`. Fix: every key adds `passed(rf)` = change, snapshot, bundle,
  investigation, previous, and the pre-record audit (`Pending`) by content with times zeroed; assemble
  drops `Pending` (it consumes it; on a hit the record exists); build-graph also covers investigation +
  record; collect adds `:render` when on.
  - Tests (each fails on the old keys, checked): `run.TestSecondRunWithOtherConfigIsItsOwnInvestigation`
    (both orders, shared store/DirWorkspace; each record completed with its own config/agent status, equal
    id+digest to a fresh-store run), `run.TestStageKeysCoverPassedRefs` (every stage × every passed ref,
    record, Render; audit times not identity), `orchestrate.TestSecondRunWithOtherConfigIsItsOwnInvestigation`
    (Temporal testsuite, both orders, ids + record digests equal the in-process runner's).
  - Historical: keys are workspace-only (never in a record), so no id/digest can change. Proved:
    `ri eval -investigations -rescore` on all 21 stored runs, JSON output byte-identical before/after; new
    `cmd/ri.TestStoredRunsReverifyTheirDigests` re-derives every stored investigation id
    (InvestigationID(bundle, config)) and content digest against run.json (160 case records). The existing
    Temporal history replay still passes.

## In progress (exact next step)
- W-0025: verdict inherits the closing entry's verified citations; prompt/tool v7.

## Decisions
- Keys include pass-through refs rather than re-deriving them on a memo hit: smallest change, one function,
  same on both runners (the workflow computes `run.Key` too).
- No producer/policy version bump for W-0024 (keys are not identity).

## Findings for the commander (not fixed, outside the brief)
- The two A″ runs `A-20261016T231142Z-5d4da37635` and `A-20261016T231217Z-5d4da37635` do **not** re-verify
  their pinned `recordDigest` at this lane's base (7cb8d6ff, before any change of mine); their ids re-derive.
  Pre-existing; the new test allowlists exactly these two with this explanation. Rescore output is unaffected.
- Existing state dirs: memos under the old keys are not hit, so resubmitting an old run in an old state dir
  re-runs its stages once (assemble reuses the existing record and appends audit events). Fresh state dirs
  (eval) are unaffected.

## Files touched outside ownership
- none beyond the brief (run/keys.go, tests in run/, orchestrate/, cmd/ri; docs/INVESTIGATE.md).

## Contract changes
- none.

## Tests
- `go build ./... && go vet ./... && go test ./...` green after W-0024.
