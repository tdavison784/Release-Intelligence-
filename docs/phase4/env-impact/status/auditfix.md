# Status — lane `auditfix` (Claude Sonnet 5.5)

Branch `p4ei/auditfix`. Brief: `briefs/auditfix.md`. **Done.**

## Done
- Root cause (`internal/investigate/run/pipeline.go`, `Pipeline.Run`): the `completed` event was stamped with
  `p.now()` before `res.apply` ran; the investigate stage's apply appends one `requested` event per agent
  evidence/validation request (later stamps), so `completed` went back in time and `Investigation.Validate`
  rejected the record.
- Fix: `done.At = p.now()` is set at the point `done` is appended (inside the `update` closure after `apply`, and
  just before append in the assemble/create path). Other stages checked: only investigate's apply appends events;
  the no-record (pre-assemble) path has no apply; `failed`, `Finalize`, `ApplyReview` append a single event
  stamped at use. Times are not identity: `ContentDigest` already strips audit/timestamps, so digests unchanged.
- Tests: `runtest.TickClock` (strictly advancing clock; a fixed clock hid the bug), `runtest.RequestingAgent`
  (one evidence + one validation request), `runtest.CheckAuditMonotonic`.
  `run.TestAgentRequestsKeepAuditMonotonic` (in-process runner) and `orchestrate.TestAgentRequestsKeepAuditMonotonic`
  (Temporal testsuite path, parity). Both fail without the fix ("event 16 goes back in time"), pass with it.
- The review decision in these tests needs `At` after the ticking clock (the fake Reviewer stamps `d.At` verbatim).

## Files touched outside ownership
- `internal/orchestrate/workflow_test.go` (additive test only).

## Test status
`go build ./... && go vet ./... && go test ./...` green. `gofmt -l` lists `internal/upgrade/values_defaults_test.go`
(pre-existing, not touched here).

## Contract changes / open questions
None.
