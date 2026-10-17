# Live agent run 4 — `kube-prometheus-stack-89-90--obs-north` (inputs only, prompt v4: per-item sessions)

The same input as [run 3](../agent-live-3/README.md), under prompt `investigate-agent/v4`, so v3 and v4 can be
compared on depth. v4 runs one short, focused session per agenda item (commander decision 2026-10-16). This page
records **what the agent did**. Correctness is the eval lane's call: lane agent never opened the case's
`expected.yaml`, `NOTES.md` or `observed/`.

| | |
|---|---|
| Date | 2026-10-16 |
| Case | `eval/investigations/kube-prometheus-stack-89-90--obs-north/input.yaml` + `env/` |
| Driver | `claude-cli`: one `claude -p` process (own session id) per item session |
| Model | requested and reported `claude-sonnet-5-5` |
| Prompt | `investigate-agent/v4`; run digest `sha256:71985c9d…adfb` (digest of the 10 item digests) |
| Agenda | 26 items (as in run 3); **cap 10** → 10 sessions, 16 items closed `not-examined: beyond cap 10` by the runner |
| Item budget | `item@v1`: 16 calls, 14 turns, 150 000 tokens, 1 MiB, 360 s per session |
| Outcome | **submitted** (all 10 sessions submitted); 83 turns, 72 tool calls (4 rejected), 124 425 tokens total |
| Validation requests | 4 (override-inspection ×1, render-diff ×1, static-reference ×2) → all `not-run` |
| Answer links | 3 (the three requests that named their concern); none lifts a concern, the records being `not-run` |
| Notes | 0 (every concern cited an artifact retrieved in its own session) |

The bundle id differs from run 3 (`bnd-e4e9c4a8f814` vs `bnd-7cbbf08745dc`). That is orchestrate2's
snapshot/bundle-id reproducibility fix (open question B), landed between the runs. The inputs, the 26-item agenda
and the deterministic pass (92 records) are the same.

**Replay check.** The transcript has one segment per item session. It was re-issued against a bundle rebuilt from
the same inputs: all 72 result digests reproduced, and the transcript and the coverage table were byte-identical.
The run record (outcome, deterministic pass, answer links applied) passes `Investigation.ValidateAgainst(bundle)`.

## v3 (run 3) vs v4 (run 4) on the same input

| | run 3 (v3, one session) | run 4 (v4, per-item sessions) |
|---|---|---|
| tool calls | 12 (of 168) | 72 (10 sessions × 5–10 of 16) |
| items examined (`get_change_item`) | 2 of 26 | **10 of 10 sessions** (16 beyond the cap) |
| inspections per examined item | 1, 1 | 1–3 (item-naming calls); 5–10 tool calls per session |
| validation requests | 1 | 4 |
| answer links | 0 | 3 |
| concerns resting on another item's evidence | 9 of 10 ("same as …") | 0: each concern cites an artifact retrieved in its own session |
| dispositions (first 10 agenda items in v4 order) | 9 raises-concern, 1 not-examined | 4 raises-concern, 5 examined-no-concern, 1 explains |
| tokens | 57 785 | 124 425 (6–26k per item) |

What changed on those 10 items, as the agent recorded it (not a judgement of which run is right):

- **Concerns: four** — the 90.x upgrade-guide item and three environment-visible ServiceMonitor changes (coredns,
  apiserver, kubelet). Each concern is in the item's own session, cites the item's own render or source, and
  has a validation request: the three ServiceMonitor requests name their concern in `answers` (the 3 links); the
  upgrade-guide item's override inspection names none.
- **Release-only ServiceMonitor changes: examined-no-concern** — controller-manager, scheduler, etcd and
  kube-proxy. In run 3 these were concerns "by analogy". In v4 each session retrieved its item's diff, found the
  change only in the chart-default render, and closed it `examined-no-concern`. That disposition is an `explains`
  assertion: it never lowers a class, and these items keep their deterministic classification.
- **Grafana `grafana.ini` change: examined-no-concern.** It was `not-examined` in run 3.
- **Operator `--secret-field-selector` change: explains.** It was a concern in run 3.

## Files

The per-item tasks are not included: the harness wrote the (empty) session-level task for per-item runs (a bug, fixed afterwards), and they cannot be re-rendered now because the shared upstream cache has drifted since the run (the bundle rebuilds differently; see the status file's merge review). Each session's prompt digest is in the transcript's `item` lines. `transcript.jsonl` (segmented per item),
`outcome.json`, `coverage.md` (per item: disposition, examined, inspections, validation requests, session status,
tool calls, turns, tokens; then the validation requests, answer links and notes), `bundle-summary.json`.

## Rejected calls (4, all in closing submissions, each corrected)

- `noValidationReason` placed outside the item entry;
- a quote not occurring verbatim in the cited render;
- an artifact named by its URI;
- an evidence id used as an artifact id.

## Observations for the commander (generic; not tuned to this case)

1. **Per-item sessions bought depth.** Every session examined its item with 5–10 calls, against 12 calls for the
   whole agenda in run 3. Concerns now rest on their own items' evidence. The cost: 2.2× the tokens, for 10 of 26
   items.
2. **The cap decides coverage now.**
   - The 16 items beyond cap 10 are all lower priority: 1 rendered-unexplained (the new token Secret) and 15
     removed values keys whose deterministic join passed.
   - They are closed by the runner and keep their deterministic class.
   - Whether 10 is right is a cost/coverage trade-off for you.
3. **Prose is not checked; only citations are.** Session 1's closing summary says the environment sets none of the
   deciding settings, "or ignoreNamespaceSelectors". But `env/platform/monitoring/values-prod.yaml:8` sets
   `ignoreNamespaceSelectors: true` (run 3 cited exactly that line).
   - The item was still closed `raises-concern` (fail-safe), but a false negative-existence claim in agent prose
     is possible.
   - A generic remedy: require a citation, or a recorded grep with no hits, for any "not set" claim. That would be
     a structured `absent` assertion that the tool layer checks with a `search_manifest` grep. Your call.
4. **The validators could not decide any requested check** (`not-run`: no pin rule, no impact paths, or a
   rendered-unexplained item a render cannot judge). So the answer links, though made correctly, lifted nothing.
   That is a validator-coverage question, not an agent one.
5. Bundle ids changed between runs 3 and 4 (question B fix, orchestrate2). Run 3's transcript replays only at its
   own commit, where it still replays byte-identically with today's agent code.
