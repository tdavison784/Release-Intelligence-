# Phase 3 model comparison — GLM-5.3-Flash vs Claude (Codex pending)

**Question.** Phase 3 measured the AI suggestion layer at 62–69% "worth a look" proxy precision
with GLM-5.3-Flash as the answering model. Is that number a property of GLM, or of the enrichment
layer (candidate selection, prompt, contract, rendering)?

**Method in one line.** Re-run the Phase 3 enrichment benchmark *unchanged* (same binary, same
prompts and promptVersion, same validator, same fixture, same eval), swap only the model that
answers, and score the results with the same metrics. Results and the honest reading are in
[RESULTS.md](RESULTS.md).

## What was held fixed

- Code: branch `p3-model-comparison` at base `7bde5a4` (phase3-main), `go build -o bin/ri ./cmd/ri`.
  No product, eval case, stored eval result or GLM cache was modified.
- Prompts: `impact-enrich/v1`; every request's `system`, user message and JSON schema are byte-identical
  across models. The prompt digest differs only because `-model` is part of it.
- Baseline gate: GLM replayed offline from the committed cache printed exactly
  41 candidates, 20 prompts, 14 accepted, 2 rejected, 4 pending, 9 suggestions (`runs/glm-5.3-flash/r4.stderr`).

## Tasks (identical for every model)

| task | what | command |
|---|---|---|
| T1 | report-4 fixture (digest-comparable with GLM's 14/2/4/9) | `ri impact cert-manager v1.17.0 v1.18.0 --kubernetes 1.28 --values/--manifests/--crds/--images testdata/e2e/env/cert-manager/… -enrich -model M -enrich-max 20 -llm-exchange …` (from `internal/app`) |
| T2 | customer-repo smoke | `ri impact … --repo internal/env/testdata/customer-repo --kubernetes 1.28 -enrich -model M -enrich-max 30 -llm-exchange …` |
| T3 | eval suggestion scoring, brief's literal command | `ri eval cert-manager-1.17-1.18 -enriched -model M -llm-cache <bench>/r4-state/llm-cache` (repo root) |
| T3b | eval suggestion scoring, digest-matched (added; see deviations) | same, run from `internal/app` with `-dir ../../eval -enriched-env testdata/e2e/env/cert-manager -enriched-state testdata/e2e/state` |
| T4 | blinded proxy review of every accepted T1 suggestion | [judging/](judging/) |

## Two answering arms

The brief specified one arm (A). Partway through, the user asked whether it truly mimicked how GLM was
called; it did not (see asymmetries), so arm B was added as the faithful control. **RESULTS.md leads with
arm B.**

| arm | how each prompt is answered | directory |
|---|---|---|
| **A — agent harness** (brief's protocol) | one interactive Claude Code session per model, launched by the fleet manager in a herdr pane; the agent reads every `<hex>.request.json` and writes `<hex>.response.json` itself | `runs/<model>/` |
| **B — stateless `claude -p`** | one isolated `claude -p` call per prompt ([raw-arm.sh](raw-arm.sh)): `--safe-mode` (no CLAUDE.md/skills/plugins), `--system-prompt` = `request.system`, user message on stdin, `--tools ""`, `--json-schema` = `request.jsonSchema`, `MAX_THINKING_TOKENS=0`, `--no-session-persistence`, neutral temp cwd. The CLI envelope of every call is kept (`*.claude-p.json`: model, token usage) | `runs/<model>-raw/` |

GLM's own path (for comparison): `ri` called the Anthropic-compatible Messages API at the z.ai gateway once
per prompt — `system` + one user message, `output_config.format = json_schema`, `ANTHROPIC_THINKING=disabled`,
`max_tokens` 16000, no tools (`internal/llm/anthropic.go`).

## Fleet (herdr; one sibling pane per agent, `--no-focus`)

| name | kind | launch args | role |
|---|---|---|---|
| bench-opus | claude | `--model claude-opus-5-5 --permission-mode auto` | arm A answerer |
| bench-sonnet | claude | `--model claude-sonnet-5-5 --permission-mode auto` | arm A answerer |
| bench-haiku | claude | `--model claude-haiku-4-5 --permission-mode auto` → **ran in accept-edits** (see deviations) | arm A answerer |
| (panes) | shell | `raw-arm.sh claude-{opus-5-5,sonnet-5-5,haiku-4-5}` | arm B |
| judge-claude | claude | `--model claude-opus-5-5 --permission-mode auto` | T4 judge (rounds 1 and 2) |
| bench-astra / bench-sol / bench-luna | codex | `-m gpt-6-astra` / `-m gpt-6.1-sol` / `-m gpt-6-luna` | **pending** — Codex quota exhausted until 2026-10-03 12:29 PM |
| judge-codex | codex | `-m gpt-6-astra` | **pending** (same quota) |

Model ids: `claude-opus-5-5`, `claude-sonnet-5-5`, `claude-haiku-4-5` (the `*.claude-p.json` envelopes
confirm the served model per call), GLM `glm-5.3-flash` (committed cache, `origin: api`).

Every agent transcript was audited (tool calls only) before its results were accepted: no agent read
repo source, docs, the GLM cache, another model's answers or the web, or called another LLM; the answer
*content* is the model's own (Opus and Sonnet wrote their answers by hand and used a small script only to
wrap them in the response envelope — scripts kept in the transcripts).

## Asymmetries (read before comparing numbers)

1. **Arm A is not a raw API answer.** The agents had Claude Code's system prompt, tools, thinking and
   — most important — **one shared context for all 20/30 prompts**, so later answers could be calibrated
   against earlier ones. GLM saw each prompt in isolation. They also knew they were under test and were
   given the rules in a brief. Arm B removes these differences except the ones below.
2. **Structured output enforcement differs.** GLM's two T1 refusals were schema violations
   (`additional properties 'missing'` / `'evidence_ids'`); the z.ai gateway evidently did not enforce
   `json_schema`. Claude via `--json-schema` (arm B) and agents that can read the schema (arm A) cannot
   easily fail that way, so **acceptance rate partly measures the transport, not the model**.
3. **Arm B still runs through the Claude Code CLI** (structured output is delivered via a tool turn,
   `num_turns: 2`), not a bare Messages call; ~1k tokens of harness overhead per call.
4. **GLM's T1 has 4 prompts that were never answered** (the recording run left them pending), so GLM
   agreement statistics use n=14 or 16, not 20.
5. **T2 is not digest-comparable with GLM**: GLM's documented smoke run (46/30/28/2/19) was live, on the
   state before the round-4 changes (today's code produces 41 candidates).
6. **The judges are AI proxies, and judge-claude is the same family as the Claude answerers**
   (self-family judging; flagged per row). The cross-family judge (judge-codex) is pending.
7. **The published 62%/69% were scored by different proxy reviewers on GLM's 13 packet-time
   suggestions**; today's replay yields 9. GLM's 9 are therefore re-scored by our judges alongside every
   other model, and only same-judge comparisons are drawn.

## Deviations from the brief (all recorded, none silent)

- **Prior aborted run.** A partial run from before this session (no traceable method) was found in
  `.ri/bench/` and archived unused to `.ri/bench-aborted-20261001T2009/`; everything here is a fresh run.
- **Haiku could not use auto mode.** `--permission-mode auto` fell back to manual for `claude-haiku-4-5`.
  The fleet manager inspected each prompt and chose "accept edits for this session" at the first write,
  then approved the exact brief commands (`ri` T2 ingest, T3 eval) and one read-only `jq` individually.
- **Haiku stopped after T1 ingest** and was nudged to continue (no answer was re-asked).
- **Haiku's first T2 attempt was discarded.** It generated the 30 T2 answers with a keyword-rule shell
  script (forbidden by the brief), noticed, deleted them, and tried a second heuristic Python script, which
  the fleet manager denied. The scripted answers were never ingested (`smoke-state/llm-cache` was empty).
  The session was cleared and T2 re-run from [runs/claude-haiku-4-5/BRIEF-T2.md](runs/claude-haiku-4-5/BRIEF-T2.md):
  30 Reads → 30 hand-written Writes, verified in the transcript. Haiku's arm-A T2 therefore ran in a fresh
  context, unlike its T1. Its arm-A T1 responses carry a placeholder `generatedAt` (`2026-10-01T00:00:00Z`).
- **The brief's T3 command cannot score suggestions.** Prompt digests include the environment paths;
  the eval spells them `internal/app/testdata/…` while T1 spelled them `testdata/…`, so every prompt
  misses the cache — **for GLM's committed cache too** — and `suggestionPrecision/Recall` print 0 with no n
  (nothing was scored). T3b re-runs the eval from `internal/app` with the T1 spelling — the invocation `eval/REPORT.md` § Reproducing already documents; the digests then match
  (the only pending prompts are GLM's 4 unanswered ones plus candidates beyond the 20 cap). Both are reported.
  The `findingsFound 3 → 2` "regression" in every enriched eval is also pre-existing and model-independent:
  `-enriched` swaps in the recording fixture's Kubernetes 1.28 for the case's 1.31, so F3
  (`impact:kubernetes-in-range`) cannot match. Plain `ri eval cert-manager-1.17-1.18` shows no regression.
- **Judging started before the Codex answerers** (they cannot run until the quota resets) and was split
  into two rounds because arm B was added after round 1 had finished. Round 2 mixes the 18 arm-B
  suggestions with 8 round-1 items re-blinded under new ids, run in a cleared session, which doubles as a
  judge test-retest check. In round 1 the judge ran `ls -R` on this directory (it saw the `runs/<model>/`
  folder names, never `key.json`); round 2 was told not to browse.

## Layout

- [RESULTS.md](RESULTS.md) — metric tables, reading, default and fallback model.
- [analyze.py](analyze.py) — every table in RESULTS.md (`python3 analyze.py`, `--judging`; `--packet`/`--packet2` rebuild the packets, seeded).
- [raw-arm.sh](raw-arm.sh) — arm B.
- `runs/<model>/` — `r4.json`, `smoke.json`, `eval.json`, `eval-t3b.json`, `*.stderr`, `*.summary`, the
  answered exchange directories (`r4-exchange/`, `smoke-exchange/`: request + response, plus the
  `claude -p` envelope in arm B), and the arm-A briefs.
- `judging/` — `JUDGE-BRIEF*.md`, `packet*.json` (blinded), `key*.json` (unblinding), `answers-*.json`.

Reproduce a table: `python3 docs/phase3/model-comparison/analyze.py` (no network, standard library only).

**Fixture hygiene after T3/T3b.** Every `ri eval -enriched` (the brief's form and the documented
`internal/app` form alike) writes an ingestion store into `internal/app/testdata/e2e/state/store`, which
`TestE2EFixtureHygiene` rejects. This is existing ri behaviour (the eval should arguably not write into the
recorded fixture). Delete that directory after replaying; it is untracked. One such store, left by the earlier
aborted run, was moved to `.ri/bench-aborted-20261001T2009/` so `go test ./...` passes offline.
