# Status — lane `orchestrate` (Claude Sonnet 5.5, agent ri-orchestrate)

Branch `p4ei/orchestrate`, worktree `.claude/worktrees/p4ei-orchestrate`. Brief: `briefs/orchestrate.md`.

## Done
- Temporal Go SDK added: `go.temporal.io/sdk v1.50.0` (latest stable at 2026-10-16).
- `internal/orchestrate`: `InvestigationWorkflow`, stub `Activities.Run`, placeholder types, `Register`,
  `WorkflowID`, `IdempotencyKey`; 12 tests with in-process `testsuite` (happy path, idempotency keys,
  retry-then-succeed, retry exhaustion, non-retryable, approve, reject, request-more-evidence, bounded
  rounds, review timeout fail-closed, cancellation + state query, query after completion). Race detector clean.
- `docs/phase4/env-impact/TEMPORAL.md` written (how to run, naming, versioning, observability, findings).
- Verified against a real `temporal server start-dev` via `internal/orchestrate/internal/devcheck`
  (throwaway, not in tests): workflow → review wait → query → signal approve → completed, 14 audit entries.

## In progress
- Nothing. Wave 2 (not this lane's current scope): replace placeholder types with contract types, typed per-stage activities, `ri worker`/`ri investigate --temporal`, search attributes.

## SDK weight / go directive (deliverable 1)
- Version v1.50.0. Module graph grew from 5 to 82 modules (`go list -m all`); `go.sum` 16 → 125 lines;
  `internal/orchestrate` pulls ~224 non-stdlib packages (grpc, protobuf, temporal api, nexus, otel...).
- **It raises the `go` directive from 1.24.7 to 1.26.0** (SDK requirement). Implication: toolchain
  go1.27.1 is fine; anyone/CI on Go < 1.26 now needs to upgrade or auto-fetch a toolchain. Commander should
  confirm CI/Docker images use ≥1.26.
- Only `internal/orchestrate` imports it; `cmd/ri` does not yet, so the `ri` binary is unaffected until wiring.
- testify is NOT a direct dependency (commander review: repo tests are stdlib-only); `workflow_test.go` uses plain `testing` with small local helpers. testify remains `// indirect` via the SDK testsuite. Also bumped `golang.org/x/text` 0.14.0 → 0.41.0 (indirect).
- `go build ./... && go vet ./... && go test ./...` all green on the full module.

## Decisions
- One generic `Run` activity (stage in the request) for the spike: keeps the skeleton small; wave 2 splits it.
- Review timeout ⇒ `review-timed-out`, never auto-approve (Principle 7). Only `Not Affected` skips review in the stub.
- More-evidence loop bounded at 2 rounds; exceeding it ends `rejected` with an audit entry.
- Cancellation returns a CanceledError (workflow shows Canceled) and records audit on a disconnected context.

## Files touched outside ownership
- None beyond `go.mod`/`go.sum` (the x/text bump and indirect testify are consequences of the SDK addition).

## Contract changes
- n/a. Guidance for the contract lane is in TEMPORAL.md "Findings that affect other lanes" (payload size
  limits ⇒ artifact refs; stable readable digests; activity error taxonomy; incomplete evidence is a result,
  not an error). The contract lane's status file had no design content yet when I read it (2026-10-16), so
  these were not cross-checked against its types.

## Open questions (for commander)
- (Resolved by commander) Go 1.26.0 directive accepted; no CI/Docker pins Go.

## Tests
`go build ./... && go vet ./... && go test ./...` pass; `go test -race ./internal/orchestrate` pass.
