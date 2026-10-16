# Live agent run 5 — `kube-prometheus-stack-89-90--obs-north` (inputs only, prompt v5: verified setting claims)

The same input as [run 4](../agent-live-4/README.md), under prompt `investigate-agent/v5`, so v4 and v5 can be
compared on claim verification. v5 turns environment facts into structured, deterministically verified claims
(`record_claim`) and labels everything else the agent writes as unverified AI prose (commander decision
2026-10-16). This page records **what the agent did**. Correctness is the eval lane's call: lane agent never
opened the case's `expected.yaml`, `NOTES.md` or `observed/`.

| | |
|---|---|
| Date | 2026-10-16 (GLM handoff session; driver and model unchanged from runs 1–4) |
| Case | `eval/investigations/kube-prometheus-stack-89-90--obs-north/input.yaml` + `env/` |
| Driver | `claude-cli`: one `claude -p` process (own session id) per item session |
| Model | requested and reported `claude-sonnet-5-5` |
| Prompt | `investigate-agent/v5`; run digest `sha256:a626f285934a…` (digest of the 10 item digests) |
| Agenda | 26 items (as in runs 3–4); **cap 10** → 10 sessions, 16 items closed `not-examined: beyond cap 10` by the runner |
| Item budget | `item@v1`: 16 calls, 14 turns, 150 000 tokens, 1 MiB, 360 s per session |
| Outcome | **budget-exhausted** — every session spent its item budget, then made its allowed closing submit; all 10 items closed by the model. 186 turns, 167 tool calls, 253 869 tokens total |
| Claims | 20 `record_claim` calls: **13 verified and recorded**; **1 rejected by the verification** (a false absence, with the contradicting line); 6 refused by the item budget before verification (see below) |
| Validation requests | 4 (override-inspection ×3, static-reference ×1) → all `not-run` (no pin rule names these items' settings; the static-reference item has no impact paths) |
| Answer links | 0 (two requests named their concern; the `not-run` records lifted nothing) |
| Notes | 0 (every concern cited an artifact retrieved in its own session) |

The bundle is `bnd-071392e03fc2` (run 4: `bnd-e4e9c4a8f814`; bundle ids are content-derived and the change
side moved with the upstream cache since then). The inputs, the 26-item agenda and the deterministic pass are
the same shape as run 4.

**Replay check.** The transcript was re-issued against a bundle rebuilt from the same inputs at this commit:
all 167 result digests reproduced, and the transcript and the coverage table were byte-identical. The run
record (outcome, deterministic pass) passes `Investigation.ValidateAgainst(bundle)`.

## What the claims did (the feature under test)

- **The run-4 failure mode, now structured.** Run 4's session 1 closed `raises-concern` with summary prose
  claiming the environment does not set `ignoreNamespaceSelectors` while `values-prod.yaml:8` sets it `true`.
  Run 5's first claim is `value prometheus.prometheusSpec.ignoreNamespaceSelectors = true in
  values-prod.yaml` — **verified by the tool layer** (evidence: the file, L8) and named by two assertions.
  The fact in the record is now the checked one, not the model's prose.
- **A false absence caught.** Session 8 claimed `arbitraryFSAccessThroughSMs` is absent from the environment
  render; the verifier found it (the render sets `arbitraryFSAccessThroughSMs: false`) and **rejected the
  claim with the contradicting line**. The session then established presence through other records instead.
- **Verified facts back the records**: 10 of the 13 recorded assertions name claim ids (e.g. the
  kube-scheduler and controller-manager `examined-no-concern` closes rest on verified `enabled: false`
  value claims plus absence claims in the render).
- **Prose is labelled.** Every closing summary and assertion statement carries
  `statementKind: unverified-ai-prose`; the beyond-cap entries (runner-written) carry none.
- **Budget, not verification, limited some claims.** 6 of the 20 `record_claim` calls were refused with
  `budget:tool-calls` before any check ran (sessions 6 and 9 spent their 16-call item budget first —
  `kubeProxy.enabled`, `kubeProxy.serviceMonitor`, `bearerTokenFile` ×2, `grafana.enabled`, `grafana.ini`).
  Those facts stayed prose in the closing summaries; the transcript's `errorClass: budget` distinguishes them
  from verification rejections.

## v4 (run 4) vs v5 (run 5) on the same input

| | run 4 (v4) | run 5 (v5) |
|---|---|---|
| tool calls | 72 (4 rejected) | 181 transcript calls, 167 metered; 28 rejected (record_claim 7: 1 failed verification + 6 budget refusals; record_assertion 8; submit_findings 4; request_validation 4; read_artifact 3; search_manifest 1; record_hypothesis 1) |
| tokens | 124 425 | 253 869 |
| session ends | all 10 `submitted` (budget left over: 5–10 of 16 calls used) | all 10 `budget-exhausted`, then the allowed closing submit |
| items examined (`get_change_item`) | 10 of 10 sessions | 10 of 10 sessions |
| environment facts | prose only (one false "not set" observed) | 13 verified claims + 1 false absence rejected |
| validation requests | 4 (all `not-run`) | 4 (all `not-run`) |
| answer links | 3 (lifted nothing) | 0 (the two named concerns' records were `not-run`) |
| dispositions (10 sessions) | 4 raises-concern, 5 examined-no-concern, 1 explains | 4 raises-concern, 4 examined-no-concern, 1 claims-mitigation, 1 explains |

Reading (what the agent did, not a judgement of which run is right): v5 sessions work the whole item budget —
the claim loop (state a fact, get it checked, adjust) is what spends it — at roughly 2× run 4's tokens. The
dispositions barely moved; the `claims-mitigation` (kube-proxy ServiceMonitor credential change) and the
`explains` (operator `--secret-field-selector`) differ from run 4's reads of the same items. Coverage of the
environment's actual settings is now checked rather than asserted.

## Files

`tasks/NN-<item>.md` (each session's task, recovered afterwards by re-rendering at the run's commit; every one reproduces the session's recorded prompt digest in the transcript's `item` lines), `transcript.jsonl` (segmented per item),
`outcome.json`, `coverage.md` (per item: disposition, examined, inspections, validation requests, session
status, tool calls, turns, tokens; then the validation requests, answer links and notes), `bundle-summary.json`.
