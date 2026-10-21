# Milestone 1: consumer-aware investigation (Workstream B), plan

Mission §10 M1. Development cases: two cert-manager 1.17 → 1.18 environments that differ in their configuration.
The commander runs and scores them; their expected outcomes are kept in `GATE-M1.md`, which lanes never read. Code
references are to this branch (`p6-decision-intel`, based on Phase 0 at 1e72d145).

## 1. Problem (from the Phase 0 transcripts and a code read)

The Phase 5 agent (prompt `investigate-agent/v8`) never opened the workloads that consume the affected objects, or
the application configuration that reads the mounted files, although all of it was in its bundle. Its reads were keyword
greps and objects of the product itself. Four causes, all in the code:

1. **The trace is not representable.** The relationship graph records Certificate → Secret (`produces`) and
   Secret → workload (`references`, via volumes and env: `internal/investigate/relations/edges.go:373-412`). It
   does not record **where** a Secret is mounted (`volumeMounts[].mountPath`, projected keys). Nothing links an
   application config file to the mounted key it reads. Such a config file is often not its own artifact: it can
   exist only inside a rendered ConfigMap (kustomize `configMapGenerator`). The decisive hop, a config file that
   names a path under another volume's `mountPath`, has no edge.
2. **The dossier hides consumers.** Its Relations section is counts and unresolved edges only
   (`dossier.go:225-231,1307-1358`).
3. **The prompt asks for consumers, but rewards formal rules.** v8 mentions consumers as guidance
   (`system.md:41-43,156-164`), while its strongest instructions require a structured `mitigation` whenever upstream
   supports a condition (`system.md:117-154`, `item.md:60-69`). About a quarter of the calls went to
   `submit_findings`, and as many again to `record_claim`.
4. **Nothing checks the trace.** `submit_findings` accepts a not-affected verdict without any consumer being read,
   and the per-item budget (16 calls) leaves no room for a multi-hop trace after the formal work.

## 2. Expected behaviour (generic)

For a change to an object that other objects consume, the investigator:
- **reads the consumers:** the workloads that mount or reference the affected object, and the configuration that
  reads the mounted files;
- **states the relationship chain** with citations, from the changed object to the consumer behaviour that depends
  on it;
- **separates explicit configuration from inherited defaults**, for every setting the change touches;
- **explains** how the effective configuration and the consumers determine the impact in this environment;
- **delivers a natural technical assessment** (mission B5 sections) without a `SemanticAssertion` or `MitigationSpec`
  being required.

The commander's scored gate is in `GATE-M1.md`.

## 3. Minimal implementation

All changes are additive; v1–v8 prompts stay byte-frozen (`prompt_v8_test.go:45`).

| # | Change | Where | Grounding |
|---|---|---|---|
| 1 | **Mount detail on edges.** Record container, `mountPath`, `subPath`, `items[].key→path` and projected sources in each Secret/ConfigMap → workload `references` edge's `Detail` | `internal/investigate/relations/edges.go` `podRefs` | rendered workload spec |
| 2 | **File-path references.** For a ConfigMap mounted into a workload, find absolute paths in its data values that fall under another volume's `mountPath` in the same pod. Record an edge ConfigMap → Secret/ConfigMap key (`reads-file`, with key, line and excerpt). Generic path matching, no product knowledge. Paths that match no mount become unresolved edges | `internal/investigate/relations/edges.go`, new edge kind in `domain/investigation.go:1143` | rendered ConfigMap + pod spec |
| 3 | **`trace_consumers` tool.** Given an object (side from/to), return the chain: produced Secrets → workloads mounting them (container, path, keys) → config that reads those paths, with an artifact id and line for every hop, plus an explicit `gaps` list (unresolved refs, objects outside the bundle) | new tool next to `expandGraph` (`tools_read.go:479`), `api.go:476`, `session.go:621` | edges 1 and 2 |
| 4 | **Dossier consumers.** List each exposed object's consumers and their chains deterministically, not just counts | `dossier.go:1307` `relations()` | edges 1 and 2 |
| 5 | **Prompt v9.** Per-item mode kept (mission B1). Feature `mitigation:false`. The B2 eight-question checklist. Consumers via `trace_consumers` first. The verified / inferred / unknown discipline: model memory is never evidence (mission A1). Mitigation and consistency instructions removed | `prompts/investigate-agent/v9/` | — |
| 6 | **Natural assessment.** `submit_findings` gains an `assessment` object: what changed, mechanism, consumers, effective configuration, verified, inferred, unknown, likely impact, recommended tests. Each "verified" entry needs a resolving citation. It is stored beside the verdict and kept out of `agentOutcome` | `api.go:675-764`, `verdict.go:77`, `run/pipeline.go:621` | tool argument, so replay and exchange reproduce it |
| 7 | **Completion check.** When the item's exposed objects have graph consumers the session never retrieved, `submit_findings` rejects a not-affected verdict once, listing them. For any other verdict, the untraced consumers are added to `unknown` as investigation gaps (mission B2, B4) | `tools_record.go:925`, `session.go:945-987` | session retrieval log |
| 8 | **Budget.** v9 per-item: 24 calls, 18 turns (from 16 and 14); tokens and time unchanged | `agenda.go:197` (v9 only) | — |

**Out of scope for M1:** global advisories and strict-path semantics (M2), synthesis (M3), formalization and
verification (M4), the change report (M5).

## 4. Tests

- **Edges:** mount detail; `subPath`; `items` key mapping; projected volumes; a file-path match; a path matching no
  mount (unresolved); a negative control where the path is under a different pod's mount.
- **`trace_consumers`:** a fixture with Certificate → Secret → Deployment → ConfigMap path; missing Secret in
  bundle → gap; every hop cites an artifact.
- **Prompt:** v9 loads; v1–v8 digests unchanged.
- **`submit_findings`:**
  - not-affected with an untraced consumer is rejected once;
  - after tracing, it is accepted;
  - other classes record the gap;
  - an assessment "verified" entry without a resolving citation is rejected.
- **Policy:** `verdict_policy_test.go` extended to assessments, so they never change classification.
- **Replay:** the M1 eval run replays deterministically (`-agent-driver replay`).

## 5. Guardrails

- **Blinding:** the implementation lane never reads `GATE-M1.md`, `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`, the
  B0 run's `report.txt` / `score.json.gz` (they print expected classes), `docs/phase6/natural-agent/RUBRIC.md`, or
  the Phase 0 assessments.
  - Edge 2 must be generic path matching. Any rule naming a product, an application or a specific file or key fails review.
- **Development case only:** cert-manager is development data (mission §11, ground-truth integrity). It is not proof
  of generalisation; M6 blind cases are.
- **No policy change:** classification and auto-approval are untouched.

## 6. Decision after M1

The commander writes `M1-RESULT.md`:
- gate met or not;
- per-run scores;
- B0 vs C1 tables;
- what remains (e.g. the D-EU strict-path item stays an M2 evidence problem).

M2 starts only on a met gate or the product owner's call (mission §13).
