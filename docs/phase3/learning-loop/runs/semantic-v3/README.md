# Semantic real run v3 (7 new edges, Sonnet + Haiku everywhere, Opus + GLM-5.3 on 4, rendered evidence)

The rendered-diff addendum run: propose for every candidate of the 7 eval-case edges not
covered by v2, with `-render` (release-level chart-default rendered diffs as citable
evidence), sonnet + haiku on all 7, opus + GLM-5.3 (provider `zai`, Z.AI's
Anthropic-compatible gateway) on the 4 smaller edges. These are **cross-model** pairs (family `glm`
vs `claude`), measured on new products; the first cross-model measurement was semantic-2's GLM run
over the v2 Opus subset (`runs/semantic-v2/README.md` § Cross-model).

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

The run hit the account session limit at 592/1 333 Claude answers. The GLM handoff recorded the
state, and the Claude agent resumed after the reset. `semantic-exchange.sh` now stops on a usage
limit instead of failing every remaining request. Final state: **1 333/1 333 Claude and 187/187
GLM requests answered, 0 pending on every edge.**

| model | proposals | refusals | full abstention | all four aspects | `action-required` requests | cost |
|---|---|---|---|---|---|---|
| claude-sonnet-5-5 (7 edges) | 559 | 14 | 198 | 76 | 12 | $26.92 |
| claude-haiku-4-5 (7 edges) | 512 | 61 | 170 | 214 | 94 | $11.58 |
| claude-opus-5-5 (4 edges) | 186 | 1 | 48 | 54 | 9 | $19.06 |
| glm-5.3 via Z.AI (4 edges) | 166 | 21 | 22 | 86 | 32 | n/a (CLI estimate $25.76 at Anthropic prices; Z.AI bills separately) |

**1 423 proposals, 97 recorded refusals** (`failures/`; all typed-contract violations). Claude
cost $57.56. Prompt version `semantic-full/v1`, or `semantic-full/v1+rendered` where rendered
evidence was shown. Generated 2026-10-02. Committed tree after the import: 1 092 candidates,
2 683 proposals, 0 invalid (`TestStoreIntegrity`); the knowledge lane's eval-reference scan passes.
Evidence is upstream only; rendered records are release scope (chart defaults); the Z.AI token
appears in no file.

## Same-model / cross-model

Both asserted, on the 166 candidates GLM answered (`analysis-cross-model.txt`):

| pair | scope | subject | change | applicability | consequence |
|---|---|---|---|---|---|
| opus ↔ glm | cross-model | 60/87 (69 %) | 84/107 (79 %) | 19/56 (34 %) | 66/94 (70 %) |
| sonnet ↔ glm | cross-model | 57/78 (73 %) | 67/91 (74 %) | 10/32 (31 %) | 43/85 (51 %) |
| haiku ↔ glm | cross-model | 59/78 (76 %) | 74/103 (72 %) | 25/73 (34 %) | 11/82 (13 %) |
| opus ↔ sonnet | same-model | 52/74 (70 %) | 69/87 (79 %) | 14/29 (48 %) | 36/77 (47 %) |
| haiku ↔ sonnet | same-model | 52/69 (75 %) | 52/80 (65 %) | 10/29 (34 %) | 25/67 (37 %) |
| haiku ↔ opus | same-model | 42/69 (61 %) | 59/88 (67 %) | 14/48 (29 %) | 7/69 (10 %) |

Subject agreement by class (exact): structured families 39–52 of 55–67 for every pair, crd-field
up to 39/39 (Sonnet↔GLM) and gvk 9/9. Free-form names: 3/7–14/28, and migration 0–5 of 3–11.

Reading. The semantic-2 findings replicate on 7 new products:
- Cross-model agreement equals the best same-family pair: change 79 % for both Opus↔GLM and
  Opus↔Sonnet. Opus↔GLM again has the highest consequence agreement (70 %).
- Subject agreement is higher than in v2 because these products' changes are mostly structured
  (CRD fields, GVKs). Free-form names remain the bottleneck in every scope.
- Haiku is the outlier: most refusals (61), most `action-required` requests (94), and the lowest
  consequence agreement (10–37 %).

## Rendered evidence

`-render` succeeded on the edges whose charts render. **51 of 573 candidates** carry release-level
rendered evidence (the render lane's correlation reached their members), so their prompts are
`semantic-full/v1+rendered`. When it is shown, models cite it: Haiku 32/45, GLM 23/35, Opus 27/47,
Sonnet 29/51 of their proposals on those candidates.

Descriptive split (Opus/Sonnet/GLM pairs pooled; **not** a controlled with/without comparison,
because rendered candidates differ in kind, mostly computed/values changes):

| | subject | change | applicability | consequence | models assert ≥1 aspect |
|---|---|---|---|---|---|
| candidates with rendered evidence | 49/81 | 66/91 | 15/51 | 54/84 | opus 40/47, sonnet 41/51, glm 31/35 |
| candidates without | 125/168 | 164/205 | 28/70 | 97/180 | opus 98/139, sonnet 320/508, glm 113/131 |

Models commit more often when a render is shown, and consequence agreement is higher (64 % vs
54 %). A controlled measurement would ask the same 51 candidates again *without* rendered evidence
into a separate store (ids are member-derived). This is the open next step for the addendum.

**run4 (prepared 2026-10-03, GLM handoff): that measurement, staged.** The 51 with-render
candidate ids are committed in `run4-only/` (external-secrets 12, kyverno 28, prometheus-operator 8,
traefik 3 — all SUB edges, so all four models). `run4.sh` re-proposes them with `-only` and **no**
`-render` into `.ri/semantic-run-v3/run4/` (fresh store, fresh cache). Its deterministic pass 1 is
done: 204 requests written (Sonnet/Haiku/Opus 51 each in `exchange-claude/`, GLM 51 in
`exchange-zai/`), 51 plain `semantic-full/v1` candidates stored, 0 ids missed. Finishing it is:
answer both exchanges (`scripts/semantic-exchange.sh`), re-run `run4.sh`'s two loops to ingest, then
`python3 analyze_render_ab.py knowledge .ri/semantic-run-v3/run4/knowledge` (paired per-aspect flip
table per model, abstentions, rendered-evidence citations on the with-side; repeated calls of one
model collapse to the latest answer).

## Files

- `knowledge/<product>/` (committed): 573 candidates + 1 423 proposals, written through
  `knowledge.NewFileStore`. Recorded refusals (97) are in `failures/` here, not in `knowledge/`.
- `run3.sh`, `pass3.sh`, `exchange-*.log`, `reports/`, `analyze_*.py`, `analysis-*.txt`: here.
- `.ri/semantic-run-v3/` (gitignored, durable in this worktree): `exchange-claude/`
  (1 333 requests + responses + CLI envelopes; logs copied here as `exchange-*.log`), `exchange-zai/` (187 each), `cache/`,
  `reports/` (per-product propose output), `run3.sh`, `run3.log`.
- Integrity: `SEMANTIC_STORE=<repo>/knowledge go test ./internal/semantic/ -run TestStoreIntegrity`.
