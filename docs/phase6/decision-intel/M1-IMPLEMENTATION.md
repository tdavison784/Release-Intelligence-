# Milestone 1 implementation: consumer-aware investigation (lane consumer)

`PLAN-M1.md` §3 changes 1–8 with the §4 tests, on branch `p6di/consumer`. Everything is additive. The
pipeline's graph, traversal, validators, classification and approval are unchanged; v1–v8 prompts are
byte-frozen (digest-tested) and replay unchanged.

## What changed, and where

| # | Change | Where |
|---|---|---|
| 1 | **Mount detail and lines.** In consumer mode, every Secret/ConfigMap volume `references` edge carries `mount`: the volume, the projected source, `items` key→path, and each container's `mountPath`/`subPath`. `references` and `produces` edges carry `line`: the artifact, line and text that state the field. | `internal/investigate/relations/consumers.go` (`volumeMount`, `lines`); `edges.go` `podRefs` |
| 2 | **`reads-file` edges.** For each container, each ConfigMap it reads (mounted, or through `configMapKeyRef`/`envFrom`), each absolute path in its visible data, the longest other mount of the same container decides the provider and key. That is a Secret or ConfigMap, a projected source, or nothing: unresolved, with the reason. Generated ConfigMaps are read from their kustomize `files`/`literals`/`envs`. Generic path matching only. | `relations/consumers.go` (`readsFile`, `fileEdge`, `generatorData`); `domain.EdgeReadsFile`, `GraphEdge.Mount/File/Line` |
| 3 | **`trace_consumers`** `{changeItem \| node, side}`. Breadth-first over `produces`/`references`/`reads-file` from the item's exposure roots (or one node). Every hop has its artifact, line and text, plus mount/file detail. The result lists the consumers and the `gaps`. | `internal/investigate/agent/consumers.go`; `investigate.ToolTraceConsumers`, `TraceRequest` |
| 4 | **Dossier consumers.** `relations.consumers`: the chains of each exposed object; their gaps become dossier gaps (producer `investigate.dossier@v4`, only with a consumer graph). | `agent/dossier.go` `consumers()` |
| 5 | **Prompt v9** (selected with `-agent-prompt`; v8 stays the default until the M1 gate). Per-item; `mitigation:false`; the eight questions; consumers first via `trace_consumers`; verified / inferred / unknown kept apart, model memory never evidence; provisional versus supported conclusions with proof obligations; the mission B5 assessment sections. No mitigation or consistency instructions. | `agent/prompts/investigate-agent/v9/` |
| 6 | **Natural assessment** on `submit_findings`: whatChanged, mechanism, consumers, effectiveConfiguration (explicit / inherited-default / unknown), verified (citations must resolve), inferred, unknown, likelyImpact, recommendedTests. Stored as `Investigation.AgentAssessments`, linked to the verdict, kept out of the classifier input. | `agent/assessment.go`; `domain/assessment.go`; `run/pipeline.go` |
| 7 | **Completion check.** Consumers of the item's exposed objects that no tool of the session returned reject a not-affected verdict once, with their list. A resubmission, or any other verdict, is accepted, and they are recorded as `untracedConsumers` and as investigation gaps in `unknown`. | `agent/tools_record.go` `submitFindings`; `agent/consumers.go` `untracedConsumers` |
| 8 | **Budget.** v9 per-item: 24 calls, 18 turns (`meta.json` `itemCalls`/`itemTurns`, rule `item@v2`); tokens, bytes and time unchanged; v1–v8 keep `item@v1`. | `agent/agenda.go` `ItemBudgetFor` |

### Why a separate consumer graph

Consumer mode is opt-in (`relations.WithConsumers`, `relations.BuildConsumerGraph`). Only a v9 agent session
builds it, over the same bundle, with the pipeline's relationship rules (`agent.WithRelations(relations.WithCatalog(…))`,
wired in `cmd/ri`). It is stored as a run artifact (`derived/agent/consumers/<side>/<graph-id>.json`, producer
`investigate.agent.consumers@v1`). This has two effects:

- The stored graph and its traversal stay exactly what they were. New unresolved edges there would add impact
  paths, and so change classification; M1 changes no policy input.
- `expand_graph` results and v6–v8 dossiers do not change, so v1–v8 transcripts still replay.

The mount detail is a structured field, not text in `Detail`, because `Detail` is part of the edge id; this keeps
the traversal's edge ids valid in the consumer graph. `reads-file` points provider → consumer, like every edge.

## How to run v9

v8 stays the default prompt until the M1 gate is met (mission §13); select v9 explicitly:

```sh
go build -o bin/ri ./cmd/ri
scripts/phase5/snapshot.sh 03b6ff43 "$SCRATCH/K0"          # K0 knowledge snapshot
cp -Rc .ri "$SCRATCH/state"                                  # never write the shared cache
DO_NOT_TRACK=1 bin/ri -state "$SCRATCH/state" -offline eval -investigations \
  -case <case> -knowledge "$SCRATCH/K0" -work "$SCRATCH/work" -run-id <run-id> -arm <arm> \
  -agent -agent-model claude-sonnet-5-5 -agent-prompt investigate-agent/v9 -save
# replay a stored v9 run deterministically:
bin/ri -state "$SCRATCH/state" -offline eval -investigations -case <case> -knowledge "$SCRATCH/K0" \
  -agent -agent-driver replay -agent-replay-dir eval/investigations/results/<run-id> -run-id <new-id>
```

`ri investigate --agent … --agent-prompt investigate-agent/v9` runs one investigation. `ri investigate dossier
<inv-id> <item>` rebuilds a v9 dossier with the run's stored consumer graph.

## Tests

- **Edges** (`relations/consumers_test.go`): mount detail; subPath; items key mapping; projected volumes
  (with and without items); a file-path match with its line and text; a path under no mount (unresolved); a
  negative control (the path is under another pod's mount); an emptyDir; URLs, routes and the configuration's
  own volume are not edges; env-consumed configuration; generated ConfigMaps; the default build is unchanged;
  determinism.
- **`trace_consumers`** (`agent/prompt_v9_test.go`, fixture `consumer_fixture_test.go`): Credential →
  Secret → Deployment → ConfigMap. Every hop cites an artifact and a line; the Secret is missing from the
  bundle, which is a gap; so is the path under no mount; also covered: side from, an unknown node, both graphs
  stored, the dossier chains.
- **Prompt:** v9 loads, with its features, tools and instructions; the generic-wording check (v8's word list
  plus case tokens and extra words); the tool-count check refuses a surface without (or with an unasked)
  `trace_consumers`; v1–v8 digests unchanged (v8 added to the frozen list); budget 24/18 (v8 unchanged).
- **`submit_findings`:** a not-affected verdict with untraced consumers is rejected once, then accepted with the
  gaps recorded; after tracing (or reading the consumers) it is accepted at once; other classes record the gaps;
  a verified entry without a resolving citation is rejected; a bad state is rejected; the assessment is
  required; v8 refuses an assessment.
- **Policy:** `verdict_policy_test.go` `TestAssessmentsNeverReachThePolicy` (classification byte-identical with
  and without assessments, an adversarial not-affected included); `run/assessment_test.go` (stored in the
  record, absent from the classifier input).
- **Replay:** `TestV9Replay`: a scripted v9 run (trace, read, completion check) replays with no mismatch. The
  transcript, prompt digest, verdicts and assessments are identical.

## Smoke (unscored)

Run `eval/investigations/results/M1s-v9-shop-de-20261020-f6c23796` (commit f6c23796, clean tree; case
`ingress-nginx-1.11-1.12--shop-de`; cloned state, `-offline`, knowledge K0 = `03b6ff43` (digest
`sha256:672a7ce0…`, as B0); `-agent -agent-model claude-sonnet-5-5 -agent-prompt investigate-agent/v9`). The lane
did not read its score, report or gates (they are label-bearing); the commander scores it.

- Agent run `submitted`. 10 item sessions (budget rule `item@v2`, 24 calls / 18 turns per item), 65 tool calls,
  166k tokens.
- **`trace_consumers` was used in every session**: 10 calls, one per item, each `{"changeItem": <the item>}`,
  all accepted. Transcript line, item 1: `"tool":"trace_consumers","arguments":{"changeItem":"ci-0e8570d62156"},
  "ok":true`. This environment mounts no Secret or ConfigMap into its workloads (its consumer graph has no
  mount or reads-file edge), so the traces return the Ingress references and gaps, not mount chains.
- One consumer graph (TO side) is stored as a run artifact, `derived/agent/consumers/to/grf-292ee38063bf.json`.
- **10 assessments stored** (`Investigation.AgentAssessments`), one per item, each linked to its verdict. Each has
  1–3 verified entries with resolving citations. `untracedConsumers` is empty in all 10: the consumers were
  traced. The assessments use the B5 sections and the explicit / inherited-default split (e.g.
  `annotations-risk-level`: inherited-default, citing a `record_claim`).
- Tool-layer rejections the model then fixed: one `request_validation` with an unresolvable subject, and one
  `submit_findings` whose verdict cited nothing that resolved. No completion-check rejection (no not-affected
  verdict with untraced consumers).
- **Replay.** `ri eval -investigations … -agent-driver replay -agent-replay-dir <the run>` re-issues all 65 calls
  with identical arguments, outcomes and result digests (no mismatch, status `submitted`). Assessments and
  verdicts are identical except their wall-clock `provenance.generatedAt`, which is never identity.

## Known limits

- Paths are matched only in ConfigMap data. The following are not matched: container `args`/`command`, literal
  `env` values, Secret data (redacted), relative paths, and one-segment paths not under a mount (taken for
  routes).
- A projected volume whose sources carry no items and might each hold the key gives `inferred` edges to each
  source, unless the snapshot's data decides.
- Generated Secrets (`secretGenerator`) and objects created at runtime have unknown keys, so a key's existence
  is not checked for them (the trace reports the object as created at runtime).
- Kustomize generator `behavior: merge` / patches to generated ConfigMaps are not applied.
- The completion check uses the TO side and the item's exposure roots. With no roots there is no check, and
  `trace_consumers` reports that as a gap. Retrieval means the object came back from `trace_consumers` or
  `get_rendered_object`, or its environment file from `read_artifact`, in the item's own session. A dossier
  listing does not count.
- `ri investigate mcp --run` (read-only) serves no consumer graph: `trace_consumers` is refused there.
- Assessments are AI prose except the verified entries' citations; nothing checks that a verified statement
  says what its quote says (the same limit as every agent record).
- v9 is not the default: `DefaultPromptVersion` stays `investigate-agent/v8` until the M1 gate is met (commander
  review of D-0001); `TestDefaultPromptIsTheNewest` allows that pinned default.
