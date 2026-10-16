# Live agent run 1 — `ingress-nginx-1.11-1.12--shop-de` (inputs only)

The first live run of the investigation agent (lane `agent`, DESIGN §4). This page records **what the agent
did**. Whether it was right is the eval lane's call: lane agent never opened the case's `expected.yaml`,
`NOTES.md` or `observed/`.

| | |
|---|---|
| Date | 2026-10-16 |
| Case | `eval/investigations/ingress-nginx-1.11-1.12--shop-de/input.yaml` + `env/` |
| Upgrade | ingress-nginx `controller-v1.11.5 → controller-v1.12.0` (chart 4.11.5 → 4.12.0), Kubernetes 1.31 |
| Driver | `claude-cli` (`claude -p` 2.1.301, logged in, no API key), MCP bridge `ri investigate mcp --attach` |
| Model | requested `claude-sonnet-5-5`; envelope reported `claude-sonnet-5-5` |
| Prompt | `investigate-agent/v1`, digest `sha256:b1e74000…3617` |
| Budget | 40 turns, 80 tool calls, 600 000 tokens, 4 MiB, 1200 s (defaults) |
| Outcome | **submitted** after 13 turns, 12 tool calls and 37 086 tokens (input + cache writes + output) |
| Validators | lane validate's five validators. The deterministic pass before the agent produced 91 records; all five were also available to `request_validation` |

## Reproduce

```
go build -o bin/ri ./cmd/ri
RI_AGENT_LIVE=ingress-nginx-1.11-1.12--shop-de RI_STATE=<warm .ri> RI_BIN=$PWD/bin/ri RI_AGENT_OUT=<dir> \
  go test ./internal/investigate/agent -run TestLiveAgentRun -v -timeout 40m
# replay this transcript against a freshly rebuilt bundle (no model call):
RI_AGENT_REPLAY=docs/phase4/env-impact/runs/agent-live-1/transcript.jsonl RI_AGENT_LIVE=… (same as above)
```

**Replay check:** the transcript was re-issued against a bundle rebuilt from the same inputs, through the
`replay` driver. All 12 result digests reproduced, and the replayed transcript was byte-identical to this
one. The run record assembled from the outcome (plus the deterministic pass) passes
`Investigation.ValidateAgainst(bundle)`.

## Files

- `task.md` — the rendered task message (27 in-scope items with their upstream citations, the render summary,
  the deterministic validations already run).
- `transcript.jsonl` — every tool call: arguments, ok/error, result digest, budget counters, and the CLI's
  result envelope (`envelope.result` is the agent's closing message).
- `outcome.json` — the `investigate.AgentOutcome`: run, assertions, validation requests, citation evidence.
- `bundle-summary.json` — bundle id, in-scope items (id, family, manifestation, scope rule), renders, traversal.

## The investigation it was given

27 in-scope change items: 15 `controller-behaviour`, 12 `rendered-config`. Renders:

- one complete environment render (the Argo CD Application), 7 changes, 0 uncorrelated;
- two consumer Kustomize overlays (`not-applicable`);
- the chart-default release render, 8 changes.

Traversal: complete, with 1 root and 2 paths.

## What the agent did (12 calls)

1. `query_diff` (first 20 environment rendered changes).
2. `search_manifest`: one grep over the environment files for metrics / plugins / rate-limit / annotation /
   OpenTelemetry / snippet / ServiceMonitor terms.
3. `read_artifact` ×4: the ingress-nginx values files and the storefront and catalog Ingress manifests.
4. `record_assertion` ×3:
   - **raises-concern** on `ci-0e8570d62156` ("Enable security features by default"). Annotation
     validation turns on by default, nothing in the environment pins it, and the storefront Ingress carries a
     `configuration-snippet` annotation (cited verbatim from the Ingress manifest).
   - **raises-concern** on `ci-384b14645d77` (chart default `controller.enableAnnotationValidations`
     false → true). Not overridden by the environment values; it cited `allowSnippetAnnotations: true`.
   - **explains** on `ci-3ef136dfe385` ("Metrics: Disable by default"). The environment already sets
     `controller.metrics.enabled: false` (cited), so the rendered removal of `--enable-metrics=false` is
     behaviour-neutral.
5. `request_validation` ×2, both refused:
   - `override-inspection` for `ci-0e8570d62156` named the values key
     `controller.enableAnnotationValidations` as a *subject*. The call was rejected: subjects are graph nodes.
   - `override-inspection` for `ci-384b14645d77` was recorded as `rejected`: it duplicates the
     deterministic pass's existing `not-run` record of the same validator.
6. `submit_findings` with three `explains` summaries (the same three items).

It never called `get_change_item`, `get_rendered_object`, `expand_graph`, `list_paths` or
`request_evidence`. It left 24 of the 27 items untouched; its closing message says so. Every citation
it made passed the verbatim-quote check. No call was rejected for a citation.

## Observations for the commander (generic; not tuned to this case)

1. **Coverage, not correctness, is the weak point of prompt v1.** The agent stopped at 12 of 80 calls after
   3 of 27 items. In policy terms that is fail-safe: unexamined items keep whatever the deterministic
   validations give them. But it adds little for most items. A `v2` prompt could ask for a closing summary
   per in-scope item, or a short reason for skipping one, and say plainly to use more of the budget.
   That is a prompt change, so it gets a new version and digest. v1 stays frozen so this transcript keeps
   replaying.
2. **Values keys as validation subjects.** For `override-inspection` the natural subject is a values key,
   but `ValidationRequestInput.Subjects` are graph nodes (contract), so the model's first attempt was
   rejected. v2's tool description should say "graph nodes only; the item's own subjects are implied", or
   the contract could gain a field for settings.
3. **A duplicate of a `not-run` record** is refused with a pointer to the existing record. That is correct
   (a validation is never recorded twice), but the agent cannot ask for the same check "with this input".
   Such a re-run only differs when the agent supplies new inputs. Today they would hash into a new
   record id, and the validators ignore suggested inputs.
