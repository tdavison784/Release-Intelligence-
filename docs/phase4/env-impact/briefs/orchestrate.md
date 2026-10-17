# Lane `orchestrate` — Temporal SDK spike (wave 0) — MISSION §A7, P5

Model: Sonnet. Branch `p4ei/orchestrate`, worktree `.claude/worktrees/p4ei-orchestrate`.

Wave 0 is a de-risking spike; wave 2 wires the real activities once the contract exists. Read first:
`docs/phase4/env-impact/MISSION.md` (§A7, P5, AC7), `docs/phase4/env-impact/FLEET.md` (binding;
"Temporal stays at the edge"), `docs/ARCHITECTURE.md`, `go.mod`, `cmd/ri` (how subcommands are wired).

## Owns

- `internal/orchestrate/` (new), `go.mod` / `go.sum` (the Temporal SDK addition only)
- `docs/phase4/env-impact/TEMPORAL.md` (new; spike findings + how to run locally)
- `docs/phase4/env-impact/status/orchestrate.md`

## Deliverables

1. Add the Temporal Go SDK (`go.temporal.io/sdk`, latest stable). Record in your status file: version,
   transitive dependency count/weight, whether it raises the `go` directive in go.mod (it is `1.24.7`
   now; toolchain is go1.27.1) and what that implies, and confirm `go build ./...` for the whole module
   stays green.
2. A minimal, **generic** skeleton in `internal/orchestrate`: an `InvestigationWorkflow` with stub
   activities shaped like the mission's stages (collect change evidence → build env snapshot → build
   graph + traverse → investigate → validate → classify → await human review if required), using
   placeholder input/output structs defined locally (the `contract` lane is defining the real types in
   parallel; do not invent domain types in `internal/domain`). Demonstrate: activity retry policy with
   transient vs. non-retryable errors, timeouts, idempotency keys derived from input digests, a
   cancellation path, a human-review **signal** (approve / reject / request-more-evidence) with a
   timeout, a **query** for current state, and a deterministic audit log of stage transitions.
3. Tests with Temporal's in-process `testsuite` (no server, no network): happy path, retry-then-succeed,
   non-retryable failure, cancellation, human approve/reject via signal, review timeout.
4. A `ri worker` / `ri investigate --temporal` wiring sketch is **not** required yet; instead document
   in TEMPORAL.md how a local dev server runs (`temporal server start-dev`), how the worker would be
   started, task queue naming, workflow id scheme (content-derived, for idempotent re-submission),
   versioning strategy for workflow code changes, and observability (search attributes, history,
   metrics hooks). Optionally verify against a real `temporal server start-dev` with a tiny throwaway
   main under `internal/orchestrate/internal/devcheck` (not part of tests) and record the result.
5. Read the contract lane's status file (`.claude/worktrees/p4ei-contract/docs/phase4/env-impact/status/contract.md`,
   read-only) near the end and note in your status file anything in your skeleton that the contract
   should know about (e.g. payload size limits — large evidence must travel as artifact refs, not
   inline payloads).

Done = `go build ./... && go vet ./... && go test ./...` green, TEMPORAL.md + status file committed,
final message `LANE DONE:`.
