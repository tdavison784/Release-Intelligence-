# Temporal orchestration (MISSION §A7, P5, AC7) — wave-0 spike findings

Code: `internal/orchestrate` (the only package besides `cmd/ri` wiring allowed to import
`go.temporal.io/*`). Payload types there are **placeholders**; wave 2 swaps in the contract types.

## What the skeleton demonstrates

`InvestigationWorkflow` runs stages `collect-change-evidence → build-env-snapshot →
build-graph-traverse → investigate → validate → classify → await-human-review`. Every stage is the
same generic activity (`Activities.Run`, registered as `Run`) taking a `StageRequest`; wave 2 gives
each stage its own typed activity backed by plain-Go packages.

| Concern | How |
|---|---|
| Retries | Policy: 1s initial, ×2, 30s max, 4 attempts. `InvalidInput` errors are non-retryable; `Transient` retry. Tests: retry-then-succeed, exhaustion, non-retryable (1 attempt). |
| Timeouts | StartToClose 2m, heartbeat 30s per activity; review wait has its own timer (default 72h, per-input override). |
| Idempotency | `IdempotencyKey(stage, inputDigests)` = sha256; passed to every activity so side effects (artifact writes, external calls) can dedupe on retry/resubmission. A new evidence round changes the inputs, hence the keys. |
| Human review | Signal `review-decision` {approve, reject, request-more-evidence, reviewer, note}. More-evidence re-runs investigate→validate→classify, bounded to `MaxEvidenceRounds` (2). |
| Fail closed | Review timeout ⇒ outcome `review-timed-out`; nothing is approved. Only a definite `Not Affected` skips review (stub policy; the real one is deterministic policy over validation results). Unknown decision ⇒ non-retryable error. |
| Cancellation | Cancel while awaiting review (or in an activity) records a `cancelled` audit entry on a disconnected context and returns a CanceledError. |
| Query | `state`: current stage, classification, round, awaitingReview, audit log. Works while running and after completion. |
| Audit | Ordered `AuditEntry{seq, stage, status, at, idempotencyKey, detail}`; timestamps from `workflow.Now`, so replay-deterministic. Returned in the result and queryable. |

Tests (`go test ./internal/orchestrate`) use Temporal's in-process `testsuite`: no server, no network.

## Running locally

```
temporal server start-dev                       # UI on :8233, gRPC on :7233, in-memory by default
go run ./internal/orchestrate/internal/devcheck # throwaway: worker + start + query + signal
```
`devcheck` dials `localhost:17233` (edit `HostPort` or start the dev server with `--port 17233`).
Verified 2026-10-16 against `temporal server start-dev` (CLI from Homebrew): workflow ran to the
review wait, query reported `await-human-review`, signal approved, outcome `approved`, 14 audit
entries, workflow visible as Completed in `temporal workflow list`.

Wired in wave 2 (see docs/INVESTIGATE.md "Running an investigation"): `ri worker` starts a worker on the task queue; `ri investigate
--temporal` starts/attaches to a workflow by ID and prints the result; `ri review <id> approve|reject|more`
sends the signal. The worker registers via `orchestrate.Register(worker, &Activities{...deps})`.

## Naming

- **Task queue:** `ri-investigation` (`orchestrate.TaskQueue`). Split per concern later if needed
  (e.g. `ri-investigation-llm` with its own concurrency/rate limits, `ri-render` for Helm/Kustomize).
- **Workflow ID:** `inv-` + first 24 hex of sha256(dependency, from, to, environment, env-input digest)
  (`orchestrate.WorkflowID`). Content-derived: re-submitting the same investigation attaches to the
  existing execution. Use the `WorkflowIDReusePolicy` `AllowDuplicateFailedOnly` (or reject duplicates)
  so a completed result is not silently recomputed; a deliberate re-run adds a `-r<n>` suffix, and the
  new bundle digest differs anyway when any input differs.
- **Namespace:** `default` for dev; per-environment namespaces in production.

## Versioning workflow code

Workflow code must be deterministic across replays of in-flight histories. Strategy:
1. Additive changes guarded with `workflow.GetVersion(ctx, "<change-id>", DefaultVersion, 1)`.
2. Breaking restructures ⇒ new workflow type name (`InvestigationWorkflowV2`) and route new starts to it;
   keep V1 registered until its executions drain.
3. Worker Versioning (build IDs) is the longer-term option once deployed beyond a single worker.
4. CI: capture real histories (`temporal workflow show --output json`) as fixtures and replay them with
   `worker.WorkflowReplayer` on every change.
Payload types are JSON: only add optional fields; never repurpose or remove one.

## Observability

- **History** is the audit source of truth; the in-workflow audit log is a convenient summary and part of the result.
- **Search attributes** (to add in wave 2; custom attrs must be registered on the cluster): `RiDependency`
  (Keyword), `RiEnvironment` (Keyword), `RiClassification` (Keyword, upserted after classify),
  `RiReviewState` (Keyword: none|waiting|approved|rejected|timed-out). Enables
  `temporal workflow list -q 'RiReviewState="waiting"'` as the human-review queue.
- **Metrics:** SDK client takes a `MetricsHandler`; `go.temporal.io/sdk/contrib/tally` or the OpenTelemetry
  contrib adapters export Prometheus. Add per-stage counters/latency inside activities via the same handler.
- **Logging:** use `workflow.GetLogger` / `activity.GetLogger` (replay-safe). The devcheck run shows the default logger is chatty at DEBUG; set a leveled `slog` adapter on the client.
- **Heartbeats** let long activities (agent investigation, kind-cluster validation) detect worker loss quickly.

## Findings that affect other lanes

- **Payload limits:** a single Temporal payload is capped at ~2 MB (history event batch 4 MB, history
  50 MB / 51,200 events before continue-as-new is needed). Evidence, snapshots, graphs, agent
  transcripts and manifests **must travel as `ArtifactRef` (digest + URI)**, with bodies in the
  content-addressed store; workflow code only handles refs, digests and small verdict structs. Every
  contract type crossing the workflow boundary should therefore be small and JSON-serialisable
  (no `interface{}` graphs, no maps with non-string keys, deterministic field order, no timestamps in
  digested payloads).
- **Digests as idempotency inputs:** contract artifacts need a stable content digest the workflow can read
  without opening the artifact.
- **Activity errors:** the contract should name an error taxonomy mapping to retryable vs not
  (unsupported input, evidence unavailable-permanent ⇒ non-retryable; rate limit, network ⇒ retryable).
  "Incomplete evidence" is a *result* (fail-closed classification), not an activity error.
- **Determinism:** workflow code must not call the clock, rand, network or the filesystem; all of that
  stays in activities. Reasoning/graph/validation packages stay Temporal-free.
- **Agent investigation** is the long, expensive, non-idempotent-ish step: give it a heartbeat, a large
  StartToClose, a low retry count, and persist its transcript as an artifact keyed by the idempotency key.

## Wave 2 (orchestrate2): what changed from the spike

- Workflow `InvestigationWorkflow` runs `run.RoundPlan` (DESIGN §5.1 stages, `domain.Stage` names); one activity
  per stage, named by the stage, plus `human-review` (applies a decision through `ReviewApplier`) and `finalize`
  (timeout / cancel / failure / exhausted rounds, run on a disconnected context).
- Retry classes come from `investigate.ClassOf`: Permanent ⇒ non-retryable `Permanent` application error.
  Policies/timeouts live in `run.PolicyOf` and are used by both the workflow and the in-process runner.
- A background heartbeat (5 s) keeps long single port calls alive; ports with their own loops also call `run.beat`.
- Workflow id = `invreq-` + hash of the request (the investigation id needs the bundle, unknown at start).
- Replay: `internal/orchestrate/testdata/history-*.json` are recorded from a real dev server
  (`TestRecordHistory`) and replayed on every `go test` (`TestReplayRecordedHistories`).
