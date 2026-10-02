# Semantic real run v2 (Sonnet + Haiku over 8 edges, Opus over 4)

The lane brief's real run: propose for every candidate of the 8 environment-case edges
with claude-sonnet-5-5 and claude-haiku-4-5 (claude-opus-5-5 additionally on 4 edges),
through the file exchange, one stateless `claude -p` call per request, full provenance,
proposals written as knowledge records through the store port.

Run of 2026-10-01/02 by the lane's Claude agent (bulk) and the GLM handoff agent
(retry of 2 failed calls, re-ingest, Opus report regeneration, same-model measurement).

## Scope

| edge | candidates |
|---|---|
| argo-cd v2.14.5→v3.0.0 | 183 |
| cert-manager v1.16.0→v1.17.0 | 35 |
| cert-manager v1.17.0→v1.18.0 | 39 |
| cilium v1.15.6→v1.17.0 | 98 |
| cilium v1.16.1→v1.17.0 | 52 |
| istio 1.23.4→1.24.0 | 91 |
| karpenter v0.37.8→v1.0.0 | 42 |
| strimzi 0.45.0→0.46.0 | 31 |

571 edge instances → **519 unique candidates** (computed-rule candidates share ids across
cilium edges; `propose -out` asks against the stored variant, see commit d5387fd).

Models: sonnet + haiku on all 8 edges; **opus on 4** (cert-manager ×2, karpenter, strimzi).
1 197 `claude -p` calls total, **$50.08** (haiku 525/$10.68, sonnet 525/$24.75, opus 147/$14.65,
from the kept CLI envelopes).

## Outcome

- 1 050 sonnet/haiku requests, all answered (2 only after a retry: haiku requests that first
  failed with `error_max_structured_output_retries` — istio Envoy ambient-logs, cilium
  `doublewrite_identity_crd_total_count` rename; one recovered to a proposal, one is a
  recorded refusal). 147 opus requests, all answered.
- **1 122 proposals** stored, **63 failures recorded** (59 haiku, 4 sonnet; all
  `rejected` — schema/typed-constraint violations, never silently dropped).
- Integrity (`SEMANTIC_STORE=… go test ./internal/semantic/ -run TestStoreIntegrity`):
  519 candidates, 1 122 proposals, **0 invalid** (every proposal validates against its
  stored candidate; citations ⊆ shown evidence).
- Confidence only `low`/`medium` (contract cap holds); mean citations per asserting
  proposal 1.06–1.15 (no citation spam, no uncited assertions — those are refused).

Per-model distributions: `analysis-permodel.txt`. Pairwise agreement (both models asserted
the aspect on the same candidate; digests compare every field, product excluded from subject):

| pair | shared cands | subject | change | applicability | consequence |
|---|---|---|---|---|---|
| sonnet↔haiku | 457 | 44 % | 68 % | 29 % | 34 % |
| opus↔sonnet | 145 | 59 % | 73 % | 46 % | 62 % |
| opus↔haiku | 129 | 39 % | 60 % | 18 % | 26 % |

Full abstention (all four aspects `undetermined`): sonnet 159, haiku 113, opus 33 of their
proposals; sonnet and haiku both fully abstain on 85 of 457 shared candidates.
`action-required` **requests** (PO-2: suggestable, never model-decided): haiku 47, sonnet 5,
opus 6; sonnet+haiku agree on the request for 5 candidates.

## Files

- `reports/` per-edge final run reports (`.txt`; the JSON variants stayed in the run dir),
  `opus-*.txt` for the Opus pass. `exchange-sonnet-haiku.log`, `exchange-opus.log`: one
  OK/FAIL line per call. `fullrun.sh`, `opusrun.sh`, `samerun.sh`: the runners used.
- `analysis-permodel.txt`, `analysis-agreement.txt`: outputs of `analyze.py` / `analyze2.py`.

## Full artifacts (not committed; gitignored state)

`/Users/tommydavison/repos/Release-Intelligence-/.ri/semantic-run-v2/` — the knowledge
store (`knowledge/`, 1 704 records: 519 candidates + 1 122 proposals + 63 failures), both
exchange dirs with requests/responses/CLI envelopes (`exchange/`, `exchange-opus/`), the
answer cache, and all reports. Importing the store into the committed `knowledge/` tree is
a commander decision (knowledge-lane ownership); everything needed to audit or import is kept.

## Reproduce

    bin/ri -offline -state <primary .ri> semantic propose <product> <from> <to> \
      -model claude-sonnet-5-5,claude-haiku-4-5 -llm-exchange <dir>/exchange \
      -llm-cache <dir>/cache -out <dir>/knowledge
    scripts/semantic-exchange.sh <dir>/exchange "" 8
    # re-run the propose command to ingest; analysis: analyze.py / analyze2.py <dir>

## Same-model measurement (PO-1)

`samerun.sh` (in this dir; run record added below when complete): two independent sonnet
call-sets over strimzi 0.45.0→0.46.0, separate exchange dirs and caches so every request is
its own `claude -p` session — checkably separate calls (distinct session ids / `callId`s),
both ingested into one store. Measures same-model self-agreement, the floor that
cross-model consensus must beat for `VerifiedConsensus` to mean anything.
