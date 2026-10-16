# Status — lane `orchestrate2` (Claude Sonnet 5.5, agent ri-orchestrate2)

Branch `p4ei/orchestrate2`. Brief: `briefs/orchestrate2.md`. Merged `p4-env-impact` (wave 1 + agent) twice; built against the real packages.

## Done
- `internal/investigate/run` (plain Go, no Temporal): `Pipeline` (one method per DESIGN §5.1 stage + `ApplyReview` + `Finalize`),
  `Runner` (in-process sequential driver, same retry policy table `PolicyOf`), `Key` (content-digest idempotency keys),
  `Workspace` (scratch handoff; mem + dir), `MergeProposed` (agent edges → inferred, placeholder ends), `runtest` (test support
  over the REAL bundle/relations/store with a one-change upstream + placeholder validator/classifier/reviewer).
- Commander item 2: the deterministic pass's `ValidationRecord`s (+ artifacts, evidence) are written into the run record by the
  `validate` stage (both phases), deduped by id; classify/agent read them from the record. Live: 49 validation records in the record.
- `internal/orchestrate`: `InvestigationWorkflow` over `run.RoundPlan`, one activity per `domain.Stage` + `human-review` + `finalize`,
  §5.1 retries/timeouts/heartbeats, review signal → `ReviewApplier` (proxy decision recorded, keeps waiting; applier-rejected decision
  ignored), review timeout fails closed, cancellation (disconnected-ctx finalize), `state` query, ≤2 evidence rounds, optional
  `Ri*` search attributes, client helpers (`Dial/Start/Attach/SendReview/GetState/NewWorker`).
- `cmd/ri/investigate.go`: `ri investigate <product> <from> <to> …` (in-process or `--temporal [--detach] [--rerun N]`),
  `ri investigate review|state`, `ri worker`. `ri investigate mcp` (agent lane) untouched: main.go dispatches `mcp` first, then falls
  through to `investigateCmd`. Other lanes can add subcommands via `investigateSubcommands`.
- Docs: README command list, `docs/INVESTIGATE.md` "Running an investigation", TEMPORAL.md "Wave 2".

## Tests (all green: `go build ./... && go vet ./... && go test ./...`; `-race` on orchestrate + run)
- testsuite: happy path + state query, reject, more-evidence → new round, bounded rounds, proxy approval does not approve,
  invalid decision ignored, review timeout fail-closed (record status failed), cancel (record cancelled), transient retry →
  success, transient exhaustion, non-retryable (1 attempt), workflow id stability.
- `TestInProcessAndTemporalRecordDigestsMatch`: same investigation id and record digest on both paths (fixed clock).
- Replay: `testdata/history-approve.json` recorded from a real dev server (`TestRecordHistory`), replayed each run.
- run: audit sequence, retry audit (`started failed retried failed retried completed`), permanent not retried, idempotent
  resubmission (classifier not re-called), agent stage runs once per key, review approve/reject/timeout.

## Live end-to-end (own dev server `temporal server start-dev --port 7299`, stopped afterwards; pid 46289 untouched)
Case `cert-manager-1.17-1.18--payments-eu` `input.yaml` + `env/` only, offline against the shared cache (symlinked into a scratch
state dir so nothing was written to the primary `.ri`), binary built `-tags ri_devstubs`:
```
$ ri worker --temporal-addr localhost:7299 …
$ ri investigate cert-manager v1.17.2 v1.18.0 --repo …/env --kubernetes 1.31 --env-name payments-prod-eu --temporal --detach --rerun 2
invreq-b1095d9dd965-r2
$ ri investigate state invreq-b1095d9dd965-r2 --temporal      → currentStage human-review, review pending, awaitingReview true
$ ri investigate review invreq-b1095d9dd965-r2 approve --reviewer tommy --note "live check" --temporal
sent approve from tommy to workflow invreq-b1095d9dd965-r2
$ ri investigate state … --temporal                           → review approved, outcome approved
  21 human-review waiting / 22 human-review signal
$ ri investigate state inv-b2679cc7ae99                       → status completed, review approved by tommy (human), 18 findings,
  49 validations, 25 audit events …, 24 classify started/completed, 25 human-review decided human:tommy
$ (re-submit same request, --rerun 2)                         → attaches to the finished workflow, prints the result:
investigation inv-b2679cc7ae99 outcome approved … record digest sha256:6e0566a8…
```
(An earlier attempt failed at `classify` because the placeholder classifier did not cite rendered changes; that exercised the permanent
failure path: workflow failed, record status `failed`, no retry. The classify stub was fixed, not the pipeline.)

## Decisions (with why)
- Workflow id = `invreq-` + hash(request); the investigation id (bundle+config) is unknown before evidence collection. It is in the
  result/state. Deliberate re-run = `-rN`. Env file content is not part of the id (only paths/inputs); use `--rerun` after editing the repo.
- Handoff between stages = small `Refs` + a scratch `Workspace` (change/snapshot results only); everything else is in the run record
  and the stores. Stage outputs are memoised under their idempotency key ⇒ retries/resubmissions repeat no side effects (esp. the agent).
- Records written pre-assembly (collect/snapshot) are journaled in the workspace and written into the run record at `assemble-bundle`.
- `human-review` timeout ⇒ record status `failed` (+ `timed-out` event); exhausted rounds ⇒ `completed` + `exhausted` event, outcome rejected.
- request-more-evidence extras `values:|manifest:|crd:|image:<x>` extend the inputs; anything else is noted as not fulfillable and the
  round still re-runs (files changed in place are picked up by the new snapshot). A round with an unchanged bundle reuses the record.
- Agent `request_validation` gets validators built with a nil impact report (the join validator needs the per-run report).

## Files touched outside ownership
- `cmd/ri/main.go` (dispatch lines + usage), `README.md`, `docs/INVESTIGATE.md` (new section), `docs/phase4/env-impact/TEMPORAL.md`.
- No domain/contract changes.

## Contract/observations for the commander
- `bundle.Assembler` output has a wall-clock `CreatedAt`, so re-assembling identical inputs yields the same bundle id with different
  content, and `RecordStore.PutBundle` then refuses ("bundles are immutable"). The pipeline works around it (attach to the stored
  bundle). Suggest the bundle lane derive `CreatedAt` deterministically or the store treat same-id/same-identity as a no-op.
- Classifier is a hook (`newClassifier` in cmd/ri/investigate.go) until classify merges: replace its body with `classify.New()` (and its
  `ReviewApplier`), then delete `cmd/ri/investigate_devstubs.go` (build tag `ri_devstubs`, temporary scaffolding).
- Agent prompt v2 / agenda: consumed only through `agent.New(...)` + `AgentConfig.PromptVersion` (currently `agent.DefaultPromptVersion`).
- A live run with `--agent` (claude-cli driver) was not done here (needs classify for the end of the pipeline and a `claude` session);
  scripted-agent paths are covered by tests.

## Cleanup
Dev server on :7299 and worker stopped; scratch state/binaries live in the session scratchpad only; no kind clusters; no `bin/`.
