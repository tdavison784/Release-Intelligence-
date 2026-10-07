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

**Use the worktree's existing `bin/ri` (built 02:03:16) — do not rebuild it before the 30 are
answered.** 06ad59bd changed prompt-rendering code after that binary was built (see
*Prompt-render drift* below); a rebuilt binary changes some request digests, and the pending
requests would stop matching their answers.

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

### Prompt-render drift (found at the GLM handoff)

`bin/ri` was built at 02:03:16; the final prompt-rendering edit of the wave (`configSourceLine`,
06ad59bd at 02:04:22 — config-file channel wording, "a file on disk (…) … text-line on its data
key", file/configMap-optional loading) landed **after** the build and after pass 1 had started.
The run therefore proposed under the pre-06ad59bd rendering. Re-running every edge with a binary
built at HEAD against a copy of the exchange: **38 of the 449 candidates** (traefik 25, istio 6,
karpenter 5, argo-cd 2 — the edge products whose CONFIG SOURCES include a `config-file` channel)
produce different prompt digests under HEAD, for every model (claude ×2 each = 76 new claude
request digests; the same 38 glm requests in `exchange-zai` are equally affected). 3 of the 30
pending claude requests are for drifted candidates.

What this does and does not mean:

- The stored proposals are honest: each carries the `promptDigest` of the prompt its model
  actually saw, and the exchanges hold the full request bodies. Within-run comparisons
  (everything measured above) are unaffected — all models saw the same prompt per candidate.
- The version label `semantic-full/v2` now spans two renders of the config-file channel line:
  the run's (411 candidates identical to HEAD, 38 not) and HEAD's. Anyone re-proposing the 38
  from source at HEAD gets new digests (the cache will not replay the run's answers for them).
- Decision for the Claude agent / commander (prompt versions are the Claude agent's): accept and
  document (this section), or bump the prompt version and re-propose the 38 candidates (38 × 3
  models ≈ 114 calls ≈ $6 Claude + 38 GLM) so one label means one prompt. Not done from the
  handoff — it moves measured ground and spends the limited account.

### Drift resolved (Claude agent, after the handoff)

The current code now renders a `config-file` entry with a file, a format and no chart key exactly as
the run did (the wording the run shipped as `semantic-full/v2`). The newer wordings apply only to the
forms the 7 later products introduced (no file, chart-set channels), and none of those products is in
this run. Check: re-running every edge with a binary built at the fix wrote **no new request file**
(898/449 unchanged), so one prompt version names one rendering again and the run's answers replay.
No re-proposal was needed.

## Final outcome (Claude agent, all 1 347 requests answered)

The 30 Claude requests left pending at the account limit were answered after the reset (0 pending).
Final: **1 316 v2 proposals** (Sonnet 446, Opus 448, GLM 422) and **31 refusals** (GLM 27, Sonnet 3,
Opus 1; all typed-contract `rejected`, kept in `failures/`, not in `knowledge/`). Committed tree:
1 194 candidates, 3 999 proposals, 0 invalid; the eval-reference scan passes.

Cost: **Claude $70.72** (Sonnet $22.56 / 449 calls, Opus $48.16 / 449 calls), within the ~$100 budget.
GLM: 449 calls billed by Z.AI (the CLI's $66.21 is an Anthropic-price estimate, not a bill).

**Undecidable leaves replaced** (`analysis-v4.txt`, old = v1 proposals, new = v2, same candidates):

| rule | leaves with a now-forbidden reason, old → new | all undecidable leaves, old → new | candidates whose every v2 proposal is undecidable-free | … of those with a decidable predicate instead |
|---|---|---|---|---|
| R1: leaf on a verified fact (59) | 25 → 0 | 51 → 56 (runtime 44, evidence 12) | 8 of 32 | 2 |
| R1b: leaf on a proposal (78) | 65 → 0 | 108 → 20 | 61 of 78 | 36 |

All **90** leaves with a reason v2 forbids (environment-visibility 87, cross-product 1,
release-knowledge 2) are gone, and the validator refused no proposal for it: the schema enforcement
of the `claude -p` transport and the prompt kept every model inside the rule. What remains is
`runtime-behavior-gap` / `evidence-gap`, which v2 deliberately allows. On fact candidates (R1) GLM
writes such runtime leaves often (37 of the 56).

**All-abstained candidates (R2, 210):** 122 now carry an assertion (GLM asserts on 110, Opus on 68,
Sonnet on 22). **L4 candidates (R3, 102):** 35 carry an assertion.

## Files

- Selection + analysis: `select_candidates.py`, `merge_configsources.py`,
  `analyze_v4.py` (this dir; the analyzer is the reproduction entry point).
- Run artifacts (gitignored, worktree): `.ri/semantic-run-v4/` — `run4v2.sh`,
  `run4v2.log`, `edges.txt`, `only/` (per-rule id lists), `edge-only/` (per-edge union),
  `exchange-claude/` (898 requests, 868 responses + CLI envelopes), `exchange-zai/`
  (449/449), `cache/`, `reports/` (per-edge pass-1/pass-2 propose output).
- Proposals/candidates/refusals: committed `knowledge/` tree (this run added 102
  candidate records — the L4 and edge-drift candidates — and 1286 proposals).
