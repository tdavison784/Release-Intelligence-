# Status — lane `agent` (Claude Opus 5.5, agent ri-agent)

Branch `p4ei/agent`, worktree `.claude/worktrees/p4ei-agent`. Brief: `briefs/agent.md` (+ `_wave2-common.md`,
`_wave1-common.md`, DESIGN §4 / §7 "agent"). Merged `p4-env-impact` @ 31fa8c2b (wave 1 complete: bundle,
store, renderdiff incl. the §1.3 amendment, validate, relations); first delivery MERGED as ed7a0acd; follow-up
(commander review: prompt v2 + agenda, subject resolution, pass outputs, live run 2) on the same branch after
`git merge p4-env-impact`. State: **DONE (follow-up complete)** (2026-10-16).
Resume from: this file + brief + `git log`.

## Done

- `internal/investigate/agent/` (owned):
  - `session.go`: `Session` implements `investigate.Tools`. It handles strict JSON `Dispatch` (the path
    MCP, exchange, replay and scripted all use), tool-call and read-byte budgets, the JSONL transcript
    (canonical arguments, result digest = sha256 of the bytes returned, counters), the run artifact and
    evidence pools, and AI provenance.
  - `tools_read.go`: `get_change_item` (edge change / compatibility, evidence, upstream sources, migration
    notes, correlated rendered changes, join findings), `list_renders` / `query_diff` / `get_rendered_object`
    via renderdiff's query engine (DESIGN §7 names it for the agent), `search_manifest` (terms, kinds, item,
    bounded RE2 grep), `read_artifact` (line windows ≤400 lines / 64 KiB; a YAML path returns the node's
    original lines so they stay quotable), `expand_graph`, `list_paths`.
  - `tools_record.go`:
    - citations, checked with `investigate.QuoteOccurs` and turned into `domain.Evidence`;
    - `record_hypothesis`, `record_assertion` (stances, proposed edge always inferred, placeholder nodes);
    - `request_evidence` (open; live → rejected; already-present → tool error);
    - `request_validation`, which runs the injected validators synchronously. Records are checked (item,
      request, not AI, `Validate`); a duplicate of an existing record id → request `rejected` with a pointer;
      no applicable validator → `unavailable`;
    - `submit_findings` (in-scope items, ≥1 citation each).
  - `mcp.go`: MCP stdio server (initialize / tools/list / tools/call / ping; tool errors become `isError`
    results) and the unix-socket bridge (`Attach`, token handshake).
  - `drivers.go`: `scripted`, `replay` (prompt digest check, per-call digest comparison, mismatches
    reported in `Mismatches` and in the stop reason), `exchange` (llm.Exchange; one round per request file;
    pending → `failed: exchange-pending: …`). `claudecli.go`: `claude-cli` (flags in docs; session id chosen
    up front; model/version from `modelUsage`; tokens = input + cache writes + output).
  - `investigator.go`: `New(st, WithDriver, WithValidators, WithClock) investigate.Investigator`, `Prepare`,
    `Budget` (defaults per field), `Session.Outcome` (fills reported model fields, stores the transcript
    artifact, validates every record), `MergeGraph` (proposed edges + placeholder nodes for re-traversal).
  - `prompts.go` + `prompts/investigate-agent/v1/{system.md,task.md,tools.json}`: generic, embedded,
    digested.
- `cmd/ri/investigate_mcp.go`: `ri investigate mcp --attach <socket> --token <t>` (bridge) and `--run <inv-id>`
  (read-only inspection of a stored investigation).
- Tests (`go test ./internal/investigate/agent`). The fixture bundle is built with the real store, the
  bundle assembler, renderdiff streams and the relations traversal, and covers:
  - the query tools;
  - **31 rejection cases**: unknown ids of every kind, uncited claims, quotes not in the artifact, invalid
    arguments / stances / edges, out-of-scope items. Each one records nothing but is counted and written to
    the transcript;
  - budgets: tool calls, read bytes, tokens, turns, seconds, pending;
  - validation requests;
  - the full record set, with the run record from the outcome passing `Investigation.ValidateAgainst(bundle)`;
  - **replay reproduces every result digest and the transcript**; a tampered recording is reported; a
    different prompt is refused;
  - the MCP server and socket bridge (incl. wrong token), and the exchange driver across pending rounds;
  - the `claude-cli` driver end to end against a fake CLI (the test binary doubles as `claude` and as the
    bridge): ok, max-turns, no envelope, session mismatch, missing CLI.
- `live_test.go`: opt-in live harness (`RI_AGENT_LIVE`). It builds a real bundle from one groundtruth
  `input.yaml` (+ knowledge facts, impact report, graph, traversal), runs lane validate's five validators as
  the deterministic pass and offers them to `request_validation`, then drives `claude -p`.
  `RI_AGENT_REPLAY=<transcript>` replays instead.
- **Live run** `docs/phase4/env-impact/runs/agent-live-1/` (ingress-nginx shop-de, claude-sonnet-5-5):
  submitted after 13 turns / 12 tool calls / 37k tokens; 3 of 27 items examined, 2 raise-concern + 1 explains,
  2 validation requests refused. The replay against a rebuilt bundle reproduced all 12 digests and the
  transcript byte for byte; the run record validates against the bundle. README there says what the agent
  did. The case's expected.yaml / NOTES.md / observed/ were never opened.
- Docs: `docs/INVESTIGATE.md` "Investigation agent"; README command line for `ri investigate mcp`.

## Follow-up (commander review of ed7a0acd) — done

- **Prompt v2 + agenda** (`agenda.go`, `prompts/investigate-agent/v2/` incl. `meta.json`). The task lists the
  agenda in priority order. `submit_findings` needs a disposition for every agenda item (raises-concern /
  claims-mitigation / explains / examined-no-concern / not-examined) and is rejected while any is missing.
  It is accepted once after budget exhaustion. Coverage: `AgentRun.Coverage` with rule, disposition, closing
  assertion, records and citations; a run with no submission reports each item `not-examined` with how the
  run ended.
  - Agenda: every in-scope item with no failing validation.
  - Priority: controller-behaviour/unknown and rendered-unexplained, then items with a pass, then the rest.
  - Budget: `agenda@v1` formula in `AgentRun.BudgetRule`.
  - `examined-no-concern` is recorded as an `explains` assertion (never a mitigation; DESIGN §3.3
    unchanged).
- **Subject resolution (v2)** (`subjects.go`): node id → node key → rendered change → environment file →
  `Kind/[ns/]name` → values key (files that set it, else the product's values files, "not set") → rendered
  setting (token in a complete env render). Unresolvable or >20 matches → rejected with the accepted forms.
  The resolution is returned in the `request_validation` result; rendered changes named as subjects join the
  request.
- **Deterministic-pass outputs**: `InvestigateInput.ValidationArtifacts` / `ValidationEvidence` (readable,
  citable, resolved; not reported as the agent's), `agent.DeterministicPass(ctx, validators, input)` for
  the runner (orchestrate2).
- **v1 preserved**: features come from v2's `meta.json` (part of v2's digest; v1 has none); the v1 tests are
  pinned to v1. Run 1 replayed byte-identically with the new code at its commit and path.
- `DefaultPromptVersion` = `investigate-agent/v2`.
- **Live run 2** `runs/agent-live-2/` (cert-manager payments-eu, claude-sonnet-5-5, v2):
  - submitted, 6 turns / 5 calls / 33k tokens;
  - **coverage 14/14** (4 raises-concern, 2 explains, 8 examined-no-concern, 0 not-examined);
  - no validation requests;
  - depth was thin (4 inspection calls) — see open questions;
  - the replay reproduced the transcript and coverage byte for byte; the run record validates.
- Tests: `agenda_test.go`:
  - v2 prompt/digest, agenda rules, budget formula, caps;
  - submit rejections (missing item, disposition, unknown disposition, uncited examined-no-concern, empty
    reason), coverage + contract mutations, closing submit after exhaustion, coverage without submission;
  - every subject form + rejection + v1 unchanged;
  - deterministic pass + served pass artifacts, v2 replay.
- Domain: coverage checks in `Investigation.Validate` (unique items, agenda rule, summary, known disposition,
  the closing assertion of the matching stance, none for not-examined) and `ValidateAgainst` (in-scope
  items, citations resolve); schemas regenerated.

## Decisions (with why)

- **"Decided" = a failing validation only** (agenda exclusion): R1 makes such an item affected whatever the
  agent records. A pass decides nothing alone (R2 needs every concern answered), so items with passes stay on
  the agenda — a concern there is what blocks an unsafe clear.
- **The agenda budget only raises** (max with configured/defaults, with caps) and is recorded per run; v1
  sessions are never scaled (their transcripts must replay).
- **The closing submit is allowed after exhaustion** (not counted or metered), so coverage is always
  reported; the run status stays `budget-exhausted`.
- **v2 behaviour switches come from `meta.json`**, so v1's digest and behaviour are byte-identical.

- **Parent-hosted session + byte bridge** for claude-cli: the MCP server process is only a pipe to the
  investigator's in-process session. Validators, budgets, records and the transcript then live in one place,
  and no state has to be shipped back from a subprocess.
- **Session id chosen up front** (`--session-id <uuid>`) so records made mid-session carry the call id the
  envelope reports. The model version is known only at the end; records get it in `Outcome` (they are
  validated with placeholders when made). A session-id mismatch fails the run.
- **Result digest = sha256 of the exact bytes returned to the model**, with wall-clock keys stripped, so a
  replay reproduces the transcript byte for byte.
- **Every call counts**, rejected ones included (bounds loops). After exhaustion, calls are refused and no
  longer counted. After submit, calls are rejected.
- **Read-byte budget counts every result's bytes**, not only artifact text (stricter; 4 MiB is ample).
- **Tokens** = input + cache-creation + output; cache reads are not counted (they re-read the same context
  each turn; counting them would exhaust every real session). Recorded in the envelope either way.
- **Turns**: the CLI overshot `--max-turns` in the smoke run (8 reported vs 5), so turns are re-checked after
  the fact; an accepted submission is allowed one closing turn.
- **Validation `Answers` is never set by the agent package.** Linking a pass to a concern would let the
  agent steer a block-lifting record. Only `Request` is set (bookkeeping the contract requires).
- **Duplicate validation** (same record id as an existing one) → request `rejected` with the existing record,
  never a second copy (the run record forbids duplicate ids).
- **Confidence `low`** on every agent record (an unvalidated AI claim); never a policy input.
- **Exchange call id**: a response without `callId` uses `exchange:<request digest>` (the round's own id;
  not a model identity).
- `request_evidence` is never fulfilled in-session (DESIGN §3.3: a new round re-enters collection).
- `ri investigate mcp --run` is read-only: with no model call behind it, there is no call id for records.
- Prompt v1 stays frozen after the live run (a change = v2 + new digest); observations are below.

## Files touched outside ownership

- `cmd/ri/main.go`: one `case "investigate"` (routes `mcp` to `c.investigateMCP`, else a usage error) and
  one usage line. **orchestrate2**: route the non-`mcp` case to your runner when merging.
- `README.md`: one command line. `docs/INVESTIGATE.md`: my section only.
- `internal/investigate/api.go`: the contract change below.

## Contract changes

- `// CONTRACT-CHANGE(agent)` (follow-up):
  - domain: `AgentRun.Coverage []AgendaCoverage`, `AgentRun.BudgetRule`, `AgendaCoverage`,
    `CoverageDisposition` (+ `CoverageDispositions`, schema enum) and the coverage checks in
    `Validate`/`ValidateAgainst`;
  - investigate: `ItemSummary.Disposition`, `InvestigateInput.ValidationArtifacts` / `ValidationEvidence`.


- `// CONTRACT-CHANGE(agent)` `AgentOutcome.ProposedNodes []domain.GraphNode`: placeholder nodes for proposed
  edges whose ends are not in the graph (a node id cannot be turned back into kind + key). `agent.MergeGraph`
  uses it.

## Open questions / notes for the commander

Follow-up (live run 2):

- **A. Coverage ≠ depth.** v2 got 14/14 closed with 4 inspection calls; 8 `examined-no-concern` rest on one
  grep and two reads. Safe (never lowers a class), but the coverage metric overstates examination. Proposed
  generic v3 rules (tool-layer enforced):
  - `examined-no-concern`/`explains` only after `get_change_item` on that item, with a citation not shared
    with another item;
  - a `request_validation` (or stated reason) for every `raises-concern` on a controller-behaviour item.
  Not done: v2 is frozen by its live run; your call.
- **B. Bundle reproducibility** (for the bundle owner, now renderdiff): snapshot ids pin the enclosing git
  HEAD and the absolute input path, and environment evidence URIs are absolute local paths. Groundtruth
  bundle ids therefore change with every commit and checkout, and recorded runs replay only at their commit
  and path (refused elsewhere — never silent).

Original delivery:

1. **Prompt v1 coverage** (live run): the agent used 12 of 80 calls and examined 3 of 27 items. A v2 could
   require a summary or skip-reason per in-scope item and push more of the budget. That is generic, but
   decide it before the eval lane scores, so v2 is not tuned after seeing results.
2. **Values keys as validation subjects**: `ValidationRequestInput.Subjects` are graph nodes; the model's
   first override-inspection named a values key and was rejected. Either v2 tool text says so, or the
   contract gains a settings field.
3. **Deterministic-pass artifacts/evidence** are not in `InvestigateInput`, so the agent cannot read
   validator output artifacts of the pass. The runner must put them in the run record (the live harness
   does).
4. User question (COMMANDER.md) still open: provider data sharing. The stage stays opt-in; docs say so.

## Tests

- `go build ./... && go vet ./... && go test ./...` green after merging p4-env-impact (see final commit);
  after the follow-up: 40 packages ok, 0 fail.
- Live: `TestLiveAgentRun` (opt-in): run 1 (v1) + replay, run 2 (v2) + replay, run-1 replay re-verified at
  c190bdd0 with the v2 code; all passed.
- Cleanup: no kind clusters or servers; `bin/` removed; the smoke run lives in the session scratchpad only.
