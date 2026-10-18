# Lane `ws6` — isolated natural-agent workspaces, runner, canary, extraction (Phase 0 harness) — Opus

Branch `p6na/ws6`, worktree `.claude/worktrees/p6na-ws6`. Ledger X-0001. You implement PROTOCOL §3, §5, §6, §7
(tooling only) and §8. Read `_common.md` first. Smallest thing that works and that a reviewer can audit.

## Required

1. **Evidence equivalence (PROTOCOL §3).** For each Phase 0 case (P-US, P-EU, D-EU: paths in PROTOCOL §2), work out
   what RI's structured agent could reach:
   - its tools (`internal/investigate/agent/`, tools_*.go and the v8 tools.json);
   - the bundle of the stored arm-B investigation (PROTOCOL §4 run ids; `cases.tar.gz` under the run's
     `results/` dir);
   - the artifacts it points to.

   Write `docs/phase6/natural-agent/runs/<case>/EVIDENCE.md`: every source, whether the workspace has it, and its
   path and sha256. Raw sources only: the env repo, upstream documents as RI fetched them, FROM/TO renders, and a
   plain unified diff of the renders. No change items, findings, claims, validations, dossiers, knowledge facts or
   transcripts. Use a scratch state dir for anything you need to re-materialise, reading the shared `.ri` cache
   read-only, as in Phase 5 (FLEET.md "Running things").
2. **Workspace builder** `scripts/phase6/build-workspace.sh <case-dir> <out>`: produces
   `env/`, `upstream/`, `rendered/{from,to}.yaml + manifest.diff`, `ORIENTATION.md`, `scratch/` and `MANIFEST.sha256`.
   `ORIENTATION.md` holds `input.yaml`'s product, chart, from/to and Kubernetes version and its
   `environment.description`, verbatim, with no case id and no `scope`. It is built from `input.yaml` only, so you
   never need to open a label-bearing file. Workspaces live outside the repository (e.g. `~/ri-p6/ws/`). Fixtures
   are read-only; `scratch/` is writable.
3. **Runner** `scripts/phase6/run-natural.sh <workspace> <arm: C-whole|C-item> <run-dir>`:
   - a fresh pristine copy of the workspace per run;
   - the PROTOCOL §6 prompt (C-item takes the upstream section from a file the commander provides);
   - `claude -p --model claude-sonnet-5-5 --output-format stream-json --verbose`, with PROTOCOL §5 isolation and
     tool limits and the §9 budget stop;
   - it stores the transcript, init event, `assessment.md`, `meta.json` (model id, CLI version, effort, tokens,
     cost, wall time, turns, workspace manifest digest, exit status).

   Find a way to run with no user/project CLAUDE.md, no auto-memory, no MCP, plugins, skills or hooks, while keeping
   authentication working (`--bare` may disable OAuth; check). Record how in status.
4. **Canary** (PROTOCOL §5): decoys beside and above the workspace and under `$HOME`; a canary session tries to read
   them and reports any instructions or memory it was given. Store it at `runs/canary/`. It must show no decoy read,
   no CLAUDE.md or memory content, and no MCP servers or plugins in init. If it fails, fix the isolation and repeat;
   if it can't be fixed, stop and message the commander.
5. **Extraction** `scripts/phase6/extract.sh <run-dir>` (PROTOCOL §8): `claude -p --model claude-opus-5-5` over
   `assessment.md` only, plus a generic schema prompt you write (per mechanism: claimed class, resources/consumers,
   mechanism, mitigation/no-exposure, cited evidence, assumptions, missing evidence, recommended validation; overall
   recommendation; "not stated" when absent; no inference beyond the text) → `extraction.json`.
6. **Arm D helper** `scripts/phase6/guided.sh <run-dir>`: resumes that run's session **interactively** with the same
   isolation, for the product owner to type into. The full transcript is captured, plus a template
   `interventions.md` (one row per human turn: verbatim text, category left blank for the product owner).
7. **Smoke**: one C-whole run, end to end (build → run → extract), on `eval/investigations/ingress-nginx-1.11-1.12--shop-de`.
   It is not a Phase 0 case and is never scored. Store it at `runs/smoke-shop-de/`. Check the transcript shows only
   workspace paths being read.
8. Do not run any Phase 0 case (P-US, P-EU, D-EU); the commander runs them after reviewing your EVIDENCE.md files.
   Never open RUBRIC.md, expected.yaml, NOTES.md, observed/ or the Phase 5 EVAL-REPORT/SUMMARY.

Docs: a short `docs/phase6/natural-agent/HARNESS.md` (how to build, run, extract and guide; the isolation method and
its verification). Done per `_common.md`.
