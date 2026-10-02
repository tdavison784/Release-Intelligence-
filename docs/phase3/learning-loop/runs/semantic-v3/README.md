# Semantic real run v3 (7 new edges, Sonnet + Haiku everywhere, Opus + GLM-5.3 on 4, rendered evidence)

The rendered-diff addendum run: propose for every candidate of the 7 eval-case edges not
covered by v2, with `-render` (release-level chart-default rendered diffs as citable
evidence), sonnet + haiku on all 7, opus + GLM-5.3 (provider `zai`, Z.AI's
Anthropic-compatible gateway) on the 4 smaller edges — the first real **cross-model**
pairs (family `glm` vs `claude`) measured on this lane.

Run of 2026-10-02 by the lane's Claude agent (candidates, requests, 592 of 1 333 claude
answers, all 187 zai answers) and the GLM handoff agent (remaining claude answers after
the session-limit reset, pass-3 ingest, analysis, this record).

## Scope

| edge | candidates |
|---|---|
| crossplane v1.20.1→v2.0.0 | 135 |
| external-secrets v0.15.0→v0.16.0 | 59 |
| flux v2.6.4→v2.7.0 | 84 |
| kyverno v1.12.6→v1.13.0 | 30 |
| loki v2.9.6→v3.0.0 | 157 |
| prometheus-operator v0.85.0→v0.86.2 | 14 |
| traefik v2.11.2→v3.0.0 | 94 |

**573 candidates** (no cross-edge sharing; candidate ids are member-derived and the whole
run uses `-render`, so the store is separate from v2's — it lives in the committed
`knowledge/` tree, which had no v3 products before this run).

Models: sonnet + haiku on all 7 edges (1 146 requests); **opus + glm on 4**
(prometheus-operator, kyverno, external-secrets, flux; 187 requests each side).

## Commands

`.ri/semantic-run-v3/run3.sh` (kept in the worktree, gitignored): pass 1 writes requests
and candidates for every product × model into `-out knowledge` through the store;
`scripts/semantic-exchange.sh <dir> "" 8 anthropic` and `… "" 6 zai` answer pending
requests only (one stateless `claude -p` call each, envelope kept); pass 2 re-runs the
same propose commands to ingest answers (cache replays answered requests). The GLM
handoff's pass 3 re-ran the sonnet+haiku and opus propose lines after the claude
exchange completed.

## Outcome

(filled after pass 3 — GLM handoff)

## Same-model / cross-model

(filled after pass 3 — `analyze_cross.py`, including the first cross-family pairs)

## Rendered evidence

(filled after pass 3 — `analyze_models.py` citation counts on render candidates,
prompt-version split `v1` vs `v1+rendered`)

## Files

- `knowledge/<product>/` (committed): 573 candidates + proposals + recorded failures,
  written through `knowledge.NewFileStore`.
- `.ri/semantic-run-v3/` (gitignored, durable in this worktree): `exchange-claude/`
  (1 333 requests + responses + CLI envelopes), `exchange-zai/` (187 each), `cache/`,
  `reports/` (per-product propose output), `run3.sh`, `run3.log`.
- Integrity: `SEMANTIC_STORE=<repo>/knowledge go test ./internal/semantic/ -run TestStoreIntegrity`.
