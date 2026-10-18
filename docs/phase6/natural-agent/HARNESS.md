# Phase 6 natural-agent harness

How the Phase 0 natural Claude Code runs are prepared, run, extracted and continued (PROTOCOL §3, §5–§8).
Lane ws6 built it. Everything lives in `scripts/phase6/`; nothing under `internal/`, `cmd/` or `knowledge/` changes.
Workspaces and live run copies live outside the repository, under `$P6_HOME` (default `~/ri-p6`).

## Steps

```sh
go build -o bin/ri ./cmd/ri                       # materialise.sh only

# 1. Raw RI sources for a case -> ~/ri-p6/evidence/<case-id>/ (once per case)
scripts/phase6/materialise.sh <case-dir> [<arm-B bundle.json> [<extra CAS dir>...]]

# 2. Workspace -> <out> (read-only fixtures, writable scratch/)
scripts/phase6/build-workspace.sh <case-dir> ~/ri-p6/ws/<case-id>

# 3. Evidence-equivalence table (Phase 0 cases)
python3 scripts/phase6/evidence-md.py <case-dir> <arm-B run dir> ~/ri-p6/ws/<case-id> > docs/phase6/natural-agent/runs/<case>/EVIDENCE.md

# 4. One run; C-item takes the commander's verbatim upstream section file
scripts/phase6/run-natural.sh ~/ri-p6/ws/<case-id> C-whole docs/phase6/natural-agent/runs/<case>/C-whole-r1
scripts/phase6/run-natural.sh ~/ri-p6/ws/<case-id> C-item  docs/phase6/natural-agent/runs/<case>/C-item-r1 <section.md>

# 5. Extraction (assessment.md only)
scripts/phase6/extract.sh docs/phase6/natural-agent/runs/<case>/C-whole-r1

# 6. Arm D: the product owner continues a run interactively (in a real terminal)
scripts/phase6/guided.sh docs/phase6/natural-agent/runs/<case>/C-whole-r1

# Isolation check (before any scored run, and after any change to lib.sh or the CLI)
scripts/phase6/canary.sh docs/phase6/natural-agent/runs/canary
```

**materialise.sh** re-runs RI's deterministic investigation (`ri investigate`, no agent, `-offline`) on the case's
`input.yaml` + `env/` in a throwaway state dir cloned from the shared `.ri` cache. The shared cache is only read. It
copies the upstream documents, environment files and environment-scope renders, and nothing derived. Given the arm-B
`bundle.json` (from the stored run's `cases.tar.gz`), every file must carry the digest that bundle records. A source
whose bytes have changed since (e.g. a re-fetched advisory page) is taken from an extra CAS dir passed on the command
line, or the run fails.

**build-workspace.sh** produces:
- `env/`;
- `upstream/<host>/<path>`;
- `rendered/from.yaml`, `rendered/to.yaml`: every environment render target, concatenated under `# =====` headers;
- `rendered/manifest.diff`: `diff -u` of the two renders;
- `ORIENTATION.md`;
- `scratch/`;
- `MANIFEST.sha256`.

`ORIENTATION.md` is built from `input.yaml` only: product, chart, from/to versions, Kubernetes version and
`environment.description` verbatim, with no case id and no scope. `env/` files carry RI's stored bytes, which
include RI's redactions; `P6_ENV=raw` uses the files as authored. A file under the case's `env/` that RI never
ingested must be decided in `env-nonartifact.tsv`, or the build fails. Builds are reproducible: the same inputs give
the same `MANIFEST.sha256`.

**run-natural.sh** checks the workspace against its manifest. It copies the workspace to
`$P6_LIVE/r-<random>/ws`, re-applies the read-only bits and fills the PROTOCOL §6 prompt from
`prompts/c-whole.txt`, taking the product and versions from `ORIENTATION.md`. The path is random so the agent's
working directory names no case or arm. `session.py` then runs `claude -p --model claude-sonnet-5-5 --output-format
stream-json --verbose` in the copy. Afterwards the runner checks the fixtures again, including that no file appeared
outside `scratch/`.

The run dir gets:
- `prompt.txt`, `transcript.jsonl` (every event), `init.json`, `stderr.txt`;
- `assessment.md` and `scratch/`;
- `tool-calls.tsv`: every tool call, with any path outside the workspace;
- `workspace-MANIFEST.sha256`;
- `meta.json`: model (requested, init, used), CLI version, effort, tokens, cost, wall time, turns, tool calls,
  permission denials, workspace manifest digest, live path, session id, prompt digest, exit status and harness commit.

The live copy is kept, because arm D resumes it.

**Budget (PROTOCOL §9).** Tokens are counted as RI's driver counts them (`claudecli.go`): uncached input + cache
writes + output, without cache reads. `session.py` adds them up per API message while streaming, estimating output
from the size of the emitted blocks. Above 400k it sends SIGTERM to the session's process group and records
`status: budget-stop`. When a session ends on its own, the result event's totals are recorded instead. A run that
errors or is stopped is kept, never retried.

**Effort** is the CLI default. The runner passes no `--effort`, removes `CLAUDE_EFFORT` and every other variable of
the calling session, and loads no settings. The CLI does not report the effort level it used, so `meta.json`
records it as "CLI default".

**extract.sh** makes one stateless `claude -p --model claude-opus-5-5` call. It sends the fixed, label-blind prompt
`prompts/extract.md`, with the assessment inlined, and constrains the output with `--json-schema
prompts/extract.schema.json`. The call has no tools, no settings and no MCP, and runs from an empty directory. It
writes `extraction.json` (per mechanism: claimed class plus verbatim quote, resources/consumers, mechanism,
mitigation/no-exposure, cited evidence, assumptions, missing evidence, recommended validation; then the overall
recommendation, with "not stated" where the assessment is silent), `extraction-envelope.json` and
`extraction-meta.json`.

**guided.sh** resumes the run's session in its live workspace, as a fork (`--resume <id> --fork-session
--session-id <new>`): the recorded unaided session stays untouched. It uses the same model, isolation and tool
limits, in an interactive terminal; the CLI may first ask the product owner to trust the folder. After the session
ends, `<run-dir>/guided/` holds:
- the fork's full session transcript (from `~/.claude/projects/<cwd>/<new>.jsonl`);
- `scratch/`, plus `assessment.md` if the session rewrote it;
- `meta.json`;
- `interventions.md`: every human turn, verbatim, each with an empty `Category:` for the product owner.

## Isolation (PROTOCOL §5) and how it is verified

OAuth credentials live in the macOS keychain under the default config dir, and `--bare` or a fresh
`CLAUDE_CONFIG_DIR` loses them ("Not logged in"). Isolation is therefore achieved with flags and a scrubbed
environment (`scripts/phase6/lib.sh`):

- **Environment.** `env -i` with only `HOME USER LOGNAME PATH LANG TERM`, plus `TMPDIR` set to the run's own tmp dir
  and `KUBECONFIG=/dev/null`. No `CLAUDE_*` variable of the calling session is inherited, and no token is passed.
- **Customisations off.**
  - `--safe-mode` disables CLAUDE.md, skills, installed plugins, hooks, MCP and claude.ai connectors.
  - `--setting-sources ""` loads no user, project or local settings.
  - `--strict-mcp-config` with no `--mcp-config` means no MCP servers.
  - `--disable-slash-commands`.
  - `--settings` turns off auto-memory, attribution and the four built-in plugins (`agents-md`, `telemetry`,
    `plugin-authoring`, `diff`), which `--safe-mode` alone leaves listed in init.
- **Tools.** `--tools Read,Grep,Glob,Bash,Write,Edit`, and WebFetch, WebSearch, Task and Agent are disallowed. Under
  `--permission-mode dontAsk`, a call not explicitly allowed is denied without a prompt. The allow rules:
  - `Read(./**)`, which also scopes Grep and Glob to the workspace;
  - Bash prefixes `ls cat head tail wc find grep diff jq yq` and `kubectl kustomize`. Claude Code's path checks
    deny these commands on paths outside the working directory, and also deny `cd ..`, command substitution and
    any other command;
  - `Write`/`Edit` only under `./scratch/**`;
  - one `Read` rule for the session's own `tool-results/` directory, where the CLI saves oversized outputs.
- **Filesystem.** The fixtures are `chmod -R a-w`; only `scratch/` is writable.
- **Machine tools.** `yq` is not installed on this machine, so the allowed `yq` prefix has no effect. Nothing reaches
  the network: `curl` is not allowed, and `kubectl` can only run `kustomize`, with no kubeconfig.

**Verification: `runs/canary/`** (PASS, 12/12). The canary runs through `run-natural.sh` with the same flags. The
session is asked to read decoys placed:
- above the live copy;
- beside it, as another run's `scratch/assessment.md`;
- beside the source workspace;
- under `$P6_HOME` and `$HOME`.

It must try Read, Grep, Glob, `cat`, `find`, `grep -r`, `ls ..`, `~/.claude/CLAUDE.md`, `~/.claude/projects`, the
repository, `curl` and `kubectl get`, and write outside `scratch/`, then report every instruction or memory in its
context.

`CANARY.md` records the checks:
- no decoy token appears in any tool result;
- no text of the user's CLAUDE.md or memory index appears anywhere in the transcript;
- init lists no MCP servers, plugins, skills or slash commands, only the six tools, in `dontAsk` mode, with model
  `claude-sonnet-5-5`;
- the fixtures are unchanged;
- only `scratch/` was written;
- the session's own oversized output is readable.

The one context item the canary reports beyond the built-in prompt is Claude Code's automatic `userEmail` reminder.
It carries no instructions, and no flag removes it. Rerun the canary after a CLI upgrade: the init check pins the
behaviour of 2.1.302.

**Smoke: `runs/smoke-shop-de/`** (`eval/investigations/ingress-nginx-1.11-1.12--shop-de`; not a Phase 0 case, never
scored). The run was build → C-whole → extract, end to end:
- 25 turns, 24 tool calls, about 42k tokens processed, $0.23, 63 s;
- every tool call names a workspace path (`tool-calls.tsv`; zero outside);
- one call was denied, a compound `cd …; …; wc -c $(find …)` command, and the agent continued without it;
- the extraction gave 16 mechanisms for $0.18.
