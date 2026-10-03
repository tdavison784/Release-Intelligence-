# Semantic re-proposal v4 (prompt v2 on the property-selected 449: Sonnet + Opus + GLM-5.3)

Step 3 of the semantic-4 wave (L1 + L4, `status/semantic.md` § semantic-4): re-propose the
449 property-selected candidates under **prompt v2** — the CONFIG SOURCES section (L1,
product config data in every prompt) and the rule forbidding avoidable `undecidable` with
reasons the evidence settles (`ErrAvoidableUndecidable`) — on all 14 environment edges,
with `-render`. Models: claude-sonnet-5-5 + claude-opus-5-5 via the claude exchange,
glm-5.3 via Z.AI (`-provider zai`). Selection (`select_candidates.py`, committed 4a3c5796):

| rule | candidates | meaning |
|---|---|---|
| R1 | 59 | `undecidable` leaf on an existing VERIFIED FACT |
| R1b | 78 | `undecidable` leaf on an existing proposal (no fact leaf) |
| R2 | 210 | has proposals, all fully abstained |
| R3 | 102 | new L4 candidate (`lines:*` member or subject-naming routine note) |

Run of 2026-10-03 by the lane's Claude agent (driver `run4v2.sh`, requests, 868/898 claude
and 449/449 zai answers, pass-2 ingest) and the GLM handoff agent (takeover verification,
analysis, this record). The claude exchange stopped on the account session limit
(resets 6:50 America/Chicago); **30 requests (18 sonnet, 12 opus) are pending**, ≈$2.4 at
the run's average $0.079/call. Everything answered was ingested; the store is consistent
with or without them (each pending request is one candidate×model cell that simply has no
v2 proposal yet).

## Commands

`.ri/semantic-run-v4/run4v2.sh OUT KN` (gitignored, in the worktree): pass 1 writes
requests + candidate records into `-out knowledge` for every edge × model with
`-only .ri/semantic-run-v4/edge-only/<edge>.txt` (the union id list per edge) and
`-render`; `scripts/semantic-exchange.sh <dir> "" 8 anthropic` / `… "" 6 zai` answer
pending requests only (one stateless call each, envelope kept); pass 2 re-runs the same
propose commands to ingest (cache replays answered requests, reports per-model
`proposals / pending / failed`). Selection id lists per rule: `.ri/semantic-run-v4/only/`.

Resume for the 30 pending (any session, after the limit resets):

```sh
scripts/semantic-exchange.sh .ri/semantic-run-v4/exchange-claude "" 8 anthropic
cd .ri/semantic-run-v4 && bash run4v2.sh . "$(cd ../.. && pwd)/knowledge"   # pass 2 re-ingests
python3 docs/phase3/learning-loop/runs/semantic-v4/analyze_v4.py knowledge .ri/semantic-run-v4/only knowledge
```

## Outcome

| model | proposals | refusals | full abstention | all 4 aspects | subject | change | applicability | consequence | action-required | cost |
|---|---|---|---|---|---|---|---|---|---|---|
| claude-sonnet-5-5 | 428 | 3 | 275 | 30 | 118 | 130 | 32 | 104 | 25 | $21.59 |
| claude-opus-5-5 | 436 | 1 | 224 | 50 | 138 | 164 | 66 | 153 | 31 | $46.77 |
| glm-5.3 (zai) | 422 | 27 | 155 | 121 | 178 | 241 | 148 | 218 | 42 | billed by Z.AI |

**1286 v2 proposals + 31 recorded refusals** (`knowledge/<product>/failures/`; all typed-
contract violations: glm statement>400 chars ×10, undetermined-reason length ×9, missing
`before`/`after` on value-changed ×8, sonnet ×3, opus ×1). Claude cost **$68.36**; with the
30 pending ≈ $70.7 — inside the ~$75 sizing estimate. Prompt version `semantic-full/v2`
(1252) or `semantic-full/v2+rendered` (34). Committed tree after this run: **1194
candidates, 3969 proposals, 0 invalid** (`TestStoreIntegrity`); `go build/vet/test ./...`
green.

### The L1 effect (old `semantic-*/v1*` proposals vs v2 on the same candidates)

From `analyze_v4.py` (old = the candidates' v1 proposals, new = this run):

| rule | candidates | undecidable leaves old → new | env-visibility-gap | cleared of every undecidable leaf | a v2 model asserts something |
|---|---|---|---|---|---|
| R1 | 59 | 51 → 55 | 24 → 0 | 8/32 | 59 |
| R1b | 78 | 108 → 20 | 63 → 0 | 61/78 (36 with a decidable predicate instead) | 77 |
| R2 | 210 | 0 → 21 | — | — | 122 |
| R3 | 102 | 0 → 15 | — | — | 35 |

- **Every `environment-visibility-gap` leaf is gone (87 → 0 across R1+R1b)** — the reason
  v2 forbids, closed by CONFIG SOURCES. This is the wave's intended effect.
- R1b (the bulk of avoidable undecidability): 108 → 20 leaves; 61 of 78 candidates now
  have v2 proposals free of undecidable, 36 carrying a decidable predicate instead.
- R1's total *rose* 51 → 55: the growth is `runtime-behavior-gap` (opus 17, glm 27; sonnet
  0) — a reason v2 keeps because the L1 analysis classed it genuinely runtime
  (`PROMPT-LEVERS.md`). These candidates already hold VERIFIED FACTs; the new leaves are
  re-proposals disagreeing about *how much* is decidable, not lost ground.
- Sonnet wrote **zero** undecidable leaves in v2 across all rules (asserts or fully
  abstains); glm asserts the most (applicability 148/422 vs opus 66, sonnet 32) and also
  writes the most leaves. Behavioural spread to watch, not a defect.
- R2 (previously all-abstained): 122/210 candidates now carry at least one assertion from
  some model — sonnet stays abstention-heavy (183/203 full abstention), glm 91, opus 139.
  Whether these new assertions are *right* is a review question, not measured here.
- R3 (new L4 candidates from `lines:*` / routine notes): 35/102 asserted by some model.
- Rendered evidence reached only 34 proposals (13 opus, 13 sonnet, 8 glm) — the render
  correlation still rarely links rendered changes to selected members — and was cited
  once. The with/without render measurement remains run4's job (semantic-3 § run4).

## Files

- Selection + analysis: `select_candidates.py`, `merge_configsources.py`,
  `analyze_v4.py` (this dir; the analyzer is the reproduction entry point).
- Run artifacts (gitignored, worktree): `.ri/semantic-run-v4/` — `run4v2.sh`,
  `run4v2.log`, `edges.txt`, `only/` (per-rule id lists), `edge-only/` (per-edge union),
  `exchange-claude/` (898 requests, 868 responses + CLI envelopes), `exchange-zai/`
  (449/449), `cache/`, `reports/` (per-edge pass-1/pass-2 propose output).
- Proposals/candidates/refusals: committed `knowledge/` tree (this run added 102
  candidate records — the L4 and edge-drift candidates — and 1286 proposals).
