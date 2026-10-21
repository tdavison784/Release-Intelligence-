# Milestone 1: consumer-aware investigation (Workstream B), plan

Mission §10 M1. Development cases: cert-manager 1.17 → 1.18, payments-us (not affected) and payments-eu (affected).
Code references are to this branch (`p6-decision-intel`, based on Phase 0 at 1e72d145).

## 1. Problem (from the Phase 0 transcripts and a code read)

The Phase 5 agent (prompt `investigate-agent/v8`) never opened a payments Deployment or the `ledger-nginx`
ConfigMap, although both were in its bundle. Its reads were greps for acme, http01, featureGates and rotationPolicy.
Four causes, all in the code:

1. **The trace is not representable.** The relationship graph records Certificate → Secret (`produces`) and
   Secret → workload (`references`, via volumes and env: `relations/edges.go:373-412`). It does not record **where**
   a Secret is mounted (`volumeMounts[].mountPath`, projected keys), and nothing links a config file to the mounted
   key it reads. `ledger.conf` is not even its own artifact: it exists only inside the rendered ConfigMap
   `ledger-nginx` (`art-1df346a0c673`). The decisive hop, `ssl_client_certificate /tls/ca.crt` → Secret
   `ledger-tls` key `ca.crt`, has no edge.
2. **The dossier hides consumers.** Its Relations section is counts and unresolved edges only
   (`dossier.go:225-231,1307-1358`).
3. **The prompt asks for consumers, but rewards formal rules.** v8 mentions consumers as guidance
   (`system.md:41-43,156-164`), while its strongest instructions require a structured `mitigation` whenever upstream
   supports a condition (`system.md:117-154`, `item.md:60-69`). 12 of 45 calls per case went to `submit_findings`
   and 9–10 to `record_claim`.
4. **Nothing checks the trace.** `submit_findings` accepts a not-affected verdict without any consumer being read,
   and the per-item budget (16 calls) leaves no room for a 4-hop trace after the formal work.

## 2. Expected behaviour (the M1 gate)

On both cases, for the rotationPolicy item (and the revisionHistoryLimit item as a control), the RI investigator:

- **(a)** reads the consuming workloads (`ledger`, `checkout`) and the `ledger-nginx` config;
- **(b)** states the CA trust relationship: Certificate → Secret → mounted `ca.crt` → nginx/curl trust;
- **(c)** distinguishes explicit configuration from inherited defaults: the P-US prod overlay pins
  `rotationPolicy: Never` on `mesh-root-ca`, while P-EU inherits the new default;
- **(d)** explains why the two environments have different outcomes;
- **(e)** delivers this as a natural technical assessment (mission B5 sections) without any `SemanticAssertion` or
  `MitigationSpec` being required.

**Gate:** (a)–(e) hold in ≥ 2 of 3 repeats per case, with:
- no new false-safe on any item;
- `classify` output byte-identical in policy terms (verdicts and assessments stay out of the classifier, as
  `verdict_policy_test.go` already proves for verdicts).

The comparison is against the stored Phase 5 run `B0-K0-20261017-edb21ed9-v8`, on the same knowledge snapshot K0, in
offline mode, with the same model (`claude-sonnet-5-5`).

## 3. Minimal implementation

All changes are additive; v1–v8 prompts stay byte-frozen (`prompt_v8_test.go:45`).

| # | Change | Where | Grounding |
|---|---|---|---|
| 1 | **Mount detail on edges.** Record container, `mountPath`, `subPath`, `items[].key→path` and projected sources in each Secret/ConfigMap → workload `references` edge's `Detail` | `relations/edges.go` `podRefs` | rendered workload spec |
| 2 | **File-path references.** For a ConfigMap mounted into a workload, find absolute paths in its data values that fall under another volume's `mountPath` in the same pod. Record an edge ConfigMap → Secret/ConfigMap key (`reads-file`, with key, line and excerpt). Generic path matching, no product knowledge. Paths that match no mount become unresolved edges | `relations/edges.go`, new edge kind in `domain/investigation.go:1143` | rendered ConfigMap + pod spec |
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

## 5. Regression comparison

```sh
go build -o bin/ri ./cmd/ri
scripts/phase5/snapshot.sh 03b6ff43 <scratch>/K0          # expect sha256:672a7ce0…
bin/ri -state <clone of shared .ri> -offline eval -investigations -arm C1 -run-id <id> \
  -case cert-manager-1.17-1.18--payments-eu,cert-manager-1.17-1.18--payments-us \
  -knowledge <scratch>/K0 -work <empty> -agent -agent-model claude-sonnet-5-5 \
  -agent-prompt investigate-agent/v9 -save           # × 3 repeats
bin/ri eval -investigations -compare base=eval/investigations/results/B0-K0-20261017-edb21ed9-v8,new=<dir>
```

- **Scoring:** the commander scores (a)–(e) per run, quoting the stored assessment, like Phase 0's `score.md`. The
  product owner audits the scores.
- **Also reported:** tool calls, tokens, cost and time against B0 ($0.80 per case, about 160 s, 45 calls).
- **Expected spend:** about $5–8 for 3 × 2 cases, more if the budget increase is used.
- **The shared `.ri`:** cloned, never written.

## 6. Guardrails

- **Blinding:** the implementation lane never reads `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`, the
  B0 run's `report.txt` / `score.json.gz` (they print expected classes), `docs/phase6/natural-agent/RUBRIC.md`, or
  the Phase 0 assessments.
  - Edge 2 must be generic path matching. Any rule naming nginx, cert-manager or `ca.crt` fails review.
- **Development case only:** cert-manager is development data (mission §11, ground-truth integrity). It is not proof
  of generalisation; M6 blind cases are.
- **No policy change:** classification and auto-approval are untouched.

## 7. Decision after M1

The commander writes `M1-RESULT.md`:
- gate met or not;
- per-run scores;
- B0 vs C1 tables;
- what remains (e.g. the D-EU strict-path item stays an M2 evidence problem).

M2 starts only on a met gate or the product owner's call (mission §13).
