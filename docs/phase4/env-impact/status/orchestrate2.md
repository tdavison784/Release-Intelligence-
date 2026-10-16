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
- (Fixed in the follow-up below.) The bundle had wall-clock content.
- Classifier is a hook (`newClassifier` in cmd/ri/investigate.go) until classify merges: replace its body with `classify.New()` (and its
  `ReviewApplier`), then delete `cmd/ri/investigate_devstubs.go` (build tag `ri_devstubs`, temporary scaffolding).
- Agent prompt v2 / agenda: consumed only through `agent.New(...)` + `AgentConfig.PromptVersion` (currently `agent.DefaultPromptVersion`).
- A live run with `--agent` (claude-cli driver) was not done here (needs classify for the end of the pipeline and a `claude` session);
  scripted-agent paths are covered by tests.

## Cleanup
Dev server on :7299 and worker stopped; scratch state/binaries live in the session scratchpad only; no kind clusters; no `bin/`.

## Follow-up: bundle determinism (Principle 6) — FIX DONE
- `bundle.Assembler.CreatedAt` and `SnapshotBuilder.CollectedAt` now default to the zero time (`pinnedClock`); when the run happened is
  in the run record's `CreatedAt` and audit trail. `NewAssemblerWithClock` / `NewSnapshotBuilderWithClock` let a caller pin a clock.
  Pooled evidence `RetrievedAt` (wall clock, not in the evidence id) is zeroed at assembly. Re-assembling identical inputs is
  byte-identical, so `PutBundle` is a no-op; the attach-to-stored-bundle workaround in `run.Pipeline` is removed.
- Test: `TestAssembleByteIdenticalAndPutIsNoop` (a wall-clock second passes between two assemblies; JSON equal; stored file unchanged).
- Pending (waiting for the commander's classify message): replace `newClassifier` with `classify.New()`, delete
  `cmd/ri/investigate_devstubs.go` and the `ri_devstubs` tag, re-run the live end-to-end with `--agent` (scripted/replay driver).

## Follow-up 2: kustomization status, classify wired, live e2e with --agent (LANE DONE)
- **Bundle (owned):** the snapshot builder now builds every kustomization the env loader reports as "not built with kustomize"
  (`bundle/kustomize.go`, real `kustomize build` / `kubectl kustomize` through `internal/render`). A built one no longer makes the repo
  input `partial`; an unbuildable one stays `partial` with `kustomization X cannot be built: <reason>` (also when no binary exists).
  Tests: built ⇒ ok, unbuildable ⇒ partial+reason (injected builder), real kustomize good ⇒ ok / broken ⇒ partial (skips without a binary).
- **Remaining partial on the groundtruth env (flag for commander):** `payments-prod-eu`'s repo input is still `partial`, now only because
  of two other loader warnings: `detected-argo "cert-manager"/"prod": version "main" is not a parsable version; kept as stated, it cannot
  be range-checked` (Argo `targetRevision: main`). That is an informational warning about version range checks, not lost data, but the
  snapshot counts any loader warning as partial. I did NOT loosen it (fail-open risk, env-loader semantics): decide whether such warnings
  should be non-gaps (env loader / bundle rule) — until then R2.1 still fails for that env.
- **Outside ownership (one line):** `validate/common.go` `prepare` looked items up with `Bundle.Change.Item`, so every validator failed
  permanently on environment-anchored items (`bundle.EnvironmentItems`, DESIGN §1.3 amendment); now `Bundle.Item` (the domain lookup that
  covers both lists). Pipeline iterates `Bundle.Items()`.
- `classify.New()` / `classify.NewReviewApplier()` wired in `cmd/ri/investigate.go`; `investigate_devstubs.go` and the `ri_devstubs` tag deleted;
  `Config.Policy = classify.Policy`. New flag `--agent-driver scripted --agent-script FILE` (JSON list of tool calls) for deterministic runs.
- **Live e2e with --agent (scripted driver), own dev server :7299 (stopped after), same case/state setup as before, real classify:**
```
worker …  --agent-driver scripted --agent-script script.json     # [{"tool":"list_renders"},{"tool":"submit_findings","args":{"items":[]}}]
$ ri investigate cert-manager v1.17.2 v1.18.0 --repo …/env --kubernetes 1.31 --env-name payments-prod-eu --agent \
      --agent-driver scripted --agent-script script.json --temporal --detach --rerun 1        → invreq-3a6252f01a0e-r1
state: validate (initial) 72 validator runs, 55 records → investigate "agent submitted" → traverse final → validate final
       → classify "18 findings, decision human-review-required" → human-review waiting (168h)
$ ri investigate review invreq-3a6252f01a0e-r1 approve --reviewer tommy --note "live check 2" --temporal
record inv-63b726e47824: status completed, review approved by tommy (human), 55 validations, agent submitted (scripted, 2 turns,
2 tool calls, transcript artifact), findings affected=2 potentially-affected=9 insufficient-evidence=7, audit: …
  24 human-review decided human:tommy
```
  (The first attempt of this run failed `validate` permanently on the environment items — the bug fixed above — and its `failed` events
  are kept in the append-only trail of the same record, the retry shows the resubmission attaching to it.)
- Tests: `go build ./... && go vet ./... && go test ./...` green. Cleanup: dev server/worker stopped, scratch under the scratchpad only.

## Follow-up 3: answer links, product-scoped loader warnings, reproducible snapshot ids (LANE DONE)
1. **Answer links:** the investigate stage applies `agent.ApplyAnswerLinks` to the run's validation records (deterministic-pass records
   included) before classify. `TestAnswerLinkReachesClassifyAndRecord`: with the link the classifier input and the stored record show the
   passing record answering the concern; without it they do not.
2. **Loader warnings (decision applied, not a blanket waiver):** `EnvironmentInputs.Product` (CONTRACT-CHANGE, additive; the snapshot stage
   fills it from the request, and it is in the snapshot idempotency key). A `"<product>": version "x" is not a parsable version` warning is
   a gap when `<product>` is the product under investigation (or no product is given: fail closed); otherwise it is recorded as
   `note: …` in the repo input's reason and the input stays `ok`. Tests both directions + `TestSplitWarnings`.
   **Observation on the groundtruth env:** for `payments-prod-eu` the `detected-argo "cert-manager"` warning comes from the Application's
   *values* source (`ref: values`, `targetRevision: main`); the chart itself is pinned (`targetRevision: v1.17.2`). The loader attributes
   that `main` to the product, so the gap stays by the rule although the deployed chart version is known. That is an env-loader
   attribution question (`internal/env/inventory.go`), not changed here.
3. **Reproducible ids:** the repo input is `Path: "."`; inputs use logical `env/<rel>` paths; `RepoRef` keeps only the tree digest
   (content + repo-relative paths); commit, dirty flag, remote and checkout path are provenance (`SnapshotResult.Provenance`, recorded in the
   `build-snapshot` audit detail, never in the bundle). Evidence URIs, artifact URIs, the facts JSON and warnings are made repo-relative
   (evidence ids are re-derived from the new URI and remapped in the facts). `SnapshotResult.Local` carries the absolute checkout for the
   environment renderer (not stored; `renderdiff.inputsOf` prefers it: one small edit outside bundle).
   `TestSnapshotAndBundleIdenticalAcrossCheckouts`: two checkouts at different paths and commits ⇒ same snapshot id, bundle id and bytes, and
   no checkout path anywhere in the bundle JSON. Explicit inputs OUTSIDE a repo keep their absolute paths in the facts (not relocatable).
- Contract changes (all additive): `EnvironmentInputs.Product`, `SnapshotResult.Provenance`, `SnapshotResult.Local`.
- Tests: `go build ./... && go vet ./... && go test ./...` green; groundtruth suites (bundle, renderdiff, validate, classify) pass offline against the warm cache.

## Follow-up 4: loader attribution, ProposedNodes (LANE DONE)
- **`internal/env/installed.go` (owned for this fix):** an Argo CD source with a `ref:` and no `chart` (a values-only source, referenced as
  `$<ref>/path` from another source's `helm.valueFiles`) is no longer an installed product: its `targetRevision` is the values repo
  revision. Its inline `helm.values/valuesObject/parameters` still become values keys (`valuesOnlySource`). The product version comes from
  the chart source. A single-source Application tracking a branch for the product chart keeps its warning (gap). Flux: already the
  HelmRelease `spec.chart.spec.version` only (a GitRepository branch is never read as a version); documented in the comment and pinned
  by a test. Tests (`internal/env/multisource_test.go`): multi-source pinned chart + values ref on main ⇒ one `cert-manager v1.17.2`
  installed entry, no unparsable-version warning; single-source branch ⇒ the warning stays; inline values of a values-only source kept;
  Flux chart version vs GitRepository branch.
- **Effect on the groundtruth envs (checked by loading `payments-eu` / `payments-us` `env/`):** the `detected-argo "cert-manager": version
  "main"` warning is gone; the remaining non-kustomize warning is `detected-argo "prod": version "main"` (another product's Application:
  a note under the follow-up-3 rule). With `Product=cert-manager` the repo input is `ok` with a `note:`.
- **Golden / eval-result changes: none.** No file under `eval/` or any testdata golden changed; `go test ./...` and the bundle, renderdiff,
  validate and classify groundtruth suites pass unchanged against the warm cache. (Their snapshot ids already changed in follow-up 3, by design.)
- **ProposedNodes:** the second traverse now uses `agent.MergeGraph(graph, {assertions, ProposedNodes})`; the nodes are kept in the workspace
  by the investigate stage (they are not in the run record); my own `MergeProposed` and its placeholder synthesis are deleted. A merged
  graph that does not validate is ignored with a note in the traverse audit detail. Test `TestProposedEdgeAndNodesExtendTheRunGraph`
  (inferred edge + two placeholder nodes in the run's graph artifact).
- Tests: `go build ./... && go vet ./... && go test ./...` green.
