# Lane consumer — status (Milestone 1, ledger D-0001)

Branch `p6di/consumer`. Brief: `briefs/consumer.md`; plan: `PLAN-M1.md` §3 (changes 1–8), §4 tests.

## Done

- Changes 1–2 (relations consumer mode): `internal/investigate/relations/consumers.go` — `WithConsumers()`,
  `BuildConsumerGraph(ctx, st, bundle, side, opts…)`. Mount detail (`GraphEdge.Mount`), the line stating each
  references/produces edge (`GraphEdge.Line`), `reads-file` edges with `GraphEdge.File`; data of kustomize
  `configMapGenerator` ConfigMaps read from their `files`/`literals`/`envs`. Tests: `consumers_test.go`.
- Change 3: `trace_consumers` (`internal/investigate/agent/consumers.go`), side to/from, hops with artifact +
  line + text, explicit `gaps`; feature `traceConsumers` with tool-count check (`prompts.go checkSurface`).
- Change 4: dossier `relations.consumers` (producer `investigate.dossier@v4`, only with a consumer graph).
- Change 5: `prompts/investigate-agent/v9/` (selected with `-agent-prompt`; v8 stays the default until the M1 gate;
  v1–v8 frozen by digest, v8 added to the list).
- Change 6: `assessment` on `submit_findings` (`agent/assessment.go`), `domain.AgentAssessment`
  (`internal/domain/assessment.go`), `AgentOutcome.Assessments`, `Investigation.AgentAssessments`, stored in
  `run/pipeline.go`, omitted from `agentOutcome` (classifier input).
- Change 7: completion check in `submit_findings` (rejects not-affected once; gaps recorded otherwise).
- Change 8: per-item budget from prompt meta (`itemCalls` 24, `itemTurns` 18; rule `item@v2`); v8 unchanged.
- Tests (§4): relations edges; trace fixture; prompt; submit checks; policy (`verdict_policy_test.go`
  `TestAssessmentsNeverReachThePolicy`, `run/assessment_test.go`); replay (`TestV9Replay`).
- Schemas regenerated. `go build ./... && go vet ./... && go test ./...` green at ad27e133.

- Smoke (unscored): `eval/investigations/results/M1s-v9-shop-de-20261020-f6c23796` (commit f6c23796, K0,
  cloned state, `-offline`, sonnet). 10 sessions, `trace_consumers` called in all 10, 10 assessments stored,
  consumer graph stored. Replay of the stored run: 65/65 calls identical (args, outcome, result digest); records
  differ only in `generatedAt`. The lane never opened its `report.txt`, `score.json.gz` or `gates.json` (stdout
  went to /dev/null); `eval` exited 1 from the gate check, not a pipeline failure.
- Docs: `M1-IMPLEMENTATION.md`, `docs/INVESTIGATE.md` (consumer mode, v9 as opt-in), README default-prompt note.
- Cleanup: `bin/` and the scratch state clone, work dirs and extracts removed; no kind cluster or server started.

## In progress (exact next step)

- Nothing: lane done (D-0001), approved for merge after the commander's one change (default pinned to v8),
  applied and tested (`go build/vet/test ./...` green, 46 packages). Next is the commander's D-0002 (v9 x3 on the
  cert-manager pair, `-agent-prompt investigate-agent/v9`).

## Decisions (with why)

1. **Consumer edges are opt-in and live in a separate session graph (v9 only).** The pipeline's stored graph,
   traversal and therefore every policy input are unchanged. Why: (a) the plan forbids policy changes, and new
   (unresolved) edges in the stored graph would change impact paths and so classification; (b) any change to
   the stored graph (new edges, or extra fields on existing edges) changes `expand_graph` results and dossier
   bytes of v1–v8 sessions, so historical transcripts (e.g. `B0-K0-…-v8`) would no longer replay. The v9
   session builds the consumer graph over the bundle with the pipeline's relationship rules
   (`agent.WithRelations(relations.WithCatalog(…))`, wired in `cmd/ri`) at TO (and FROM on demand), keeps the
   stored graph for every other tool and for validators, and stores each consumer graph as a run artifact.
2. **Mount detail is a structured additive field (`GraphEdge.Mount`), not text in `Detail`.** `Detail` is part
   of the edge id; changing it would rename every references edge and break the traversal's edge ids. Content
   is what the plan lists (container, mountPath, subPath, items key→path, projected source).
3. **`reads-file` direction follows the graph convention** (From = provider, To = consumer): the mounted
   Secret/ConfigMap → the ConfigMap whose file names a path under its mountPath. Unresolved paths get an
   `external` placeholder node `file:<workload>#<container>:<path>` as From (status missing; inferred under the
   Kubernetes service-account token directory).
4. **What counts as a file path:** absolute, not after a word char, `/`, `$` or `}` (no URLs, no `${VAR}/x`);
   a one-segment path counts only when it is under a mount (so HTTP routes like `/healthz` are not unresolved
   noise). A path under the configuration's own volume resolves to itself (no edge). ConfigMaps read through env
   (`configMapKeyRef`, `envFrom`) are readers too (mission B3 "ConfigMap → env → application").
5. **"Retrieved" for the completion check** = a consumer node returned by `trace_consumers`, by
   `get_rendered_object` (whole object), or defined by an environment file `read_artifact` returned, in the
   item's own session. Being listed in the dossier does not count (else the check is vacuous). Consumers =
   non-placeholder To-nodes of references / reads-file hops from the item's exposure roots (TO side).
6. **Default prompt stays v8 until the M1 gate** (commander review of D-0001, mission §13: a milestone does not
   change default behaviour before its gate). `DefaultPromptVersion = investigate-agent/v8`;
   `TestDefaultPromptIsTheNewest` allows a pinned default (`pinnedDefaultPrompt`, comment names the M1 gate; set it
   to "" when the default moves). v9 via `-agent-prompt investigate-agent/v9`. (First submission had made v9 the
   default by the newest-is-default convention; reverted.)
7. **v9 keeps `verdict`** (class for scoring) and adds `assessment`; mitigation is off (`record_hypothesis`
   schema has no `mitigation`, the tool layer rejects one).

## Files touched outside ownership

- `internal/investigate/api.go` (additive: `ToolTraceConsumers`, `TraceRequest`, `AssessmentInput`,
  `VerifiedInput`, `ItemSummary.Assessment`, `AgentOutcome.Assessments`).
- `cmd/ri/investigate.go` (one option: `agent.WithRelations(relations.WithCatalog(a.Catalog))`).
- `internal/investigate/run/assessment_test.go` (new test).
- `cmd/ri/investigate_verdict.go`: `ri investigate dossier` passes the run's stored consumer graph (v9 dossiers match).
- `README.md`: default-prompt note (v8).
- `cmd/ri/eval_investigations.go`: `-agent-prompt` help text names the pinned default (was "the newest").
- `internal/investigate/agent/prompt_v8_test.go`: v8 digest frozen; the default assertion moved to the v9 test;
  the generic-word check made a helper (same words for v8).

## Contract changes (all additive, `CONTRACT-CHANGE(consumer)`)

`EdgeReadsFile`; `GraphEdge.Mount/File/Line` (+ `EdgeMount`, `KeyToPath`, `VolumeMount`, `EdgeFile`, `EdgeLine`);
`AgentAssessment` (+ `AssessmentSetting`, `AssessmentFinding`); `Investigation.AgentAssessments`; dossier
`relations.consumers`; prompt meta `traceConsumers`, `assessment`, `itemCalls`, `itemTurns`.

## Open questions

- None blocking. Known limits are listed in `M1-IMPLEMENTATION.md`.
