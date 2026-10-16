# Status — lane `agent` (Claude Opus 5.5, agent ri-agent)

Branch `p4ei/agent`, worktree `.claude/worktrees/p4ei-agent`. Brief: `briefs/agent.md` (+ `_wave2-common.md`,
`_wave1-common.md`, DESIGN §4 / §7 "agent"). Merged `p4-env-impact` @ 31fa8c2b (wave 1 complete: bundle,
store, renderdiff incl. the §1.3 amendment, validate, relations); first delivery MERGED as ed7a0acd; follow-ups
1–4 merged through ea983257. State: **follow-up 5 (verified setting claims, prompt v5) complete** (2026-10-16,
GLM handoff).
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

## Follow-up 2 (commander notes after classify merged) — done

- **Answer links (prompt v3, now the default; v2 frozen by live run 2).** `request_validation` takes
  `answers` (assertion ids). It also links `raises-concern` / `claims-mitigation` assertions recorded with the
  request's hypothesis. A link is written into the record's `Answers` under a deterministic guard:
  - same item;
  - the record covers every subject, path and rendered change the assertion names;
  - an assertion naming none needs an item-wide record (every root and path).

  A named assertion the request cannot cover is rejected up front. `ValidationRequest.Answers` persists
  the ask. A link to an existing (deterministic-pass) record goes into `AgentOutcome.AnswerLinks`, and
  the runner applies it with `agent.ApplyAnswerLinks` before classify (orchestrate2).
  - Tested against the real classifier: a linked pass lifts the concern; the same pass without the link
    leaves it open; a pass-record link lifts it only after `ApplyAnswerLinks`; plus every guard rejection.
- **Environment-scope items**: every item iteration in the agent package (task, agenda, session item index,
  deterministic pass, live harness) and my domain coverage check now use `EvidenceBundle.Items()` /
  `Item()`. Tested with a fixture `scope:rendered-unexplained` item in `EnvironmentItems`: it is in the
  task, the agenda, `get_change_item`, assertions, `request_validation`, the submission requirement,
  coverage and the deterministic pass.
  - `query_diff changeItem=<env item>`: works after merging `p4-env-impact` with renderdiff's `QueryPage` fix;
    covered by the same test.
- Live run 2 had already been done (under v2) when the "merge before live run 2" note arrived. Its
  bundle's in-scope items were all record items (16 in scope, none environment-scope), so it is unaffected
  by the env-item fix. A v3 live run was not requested; offered.

## Follow-up 3 (commander decision on question A) — done

- **Depth rules in prompt v3** (meta feature `depthRules`; `depth.go`), enforced in the tool layer when a record
  is made:
  - D1: `explains` / `examined-no-concern` only after `get_change_item` on the item and with ≥1 citation
    (artifact + normalised quote) no other item's records share — incl. the other entries of the same
    submission;
  - D2: `submit_findings` is rejected while a raises-concern on a controller-behaviour/unknown item has neither
    a `request_validation` for the item nor a `noValidationReason`.

  Coverage entries carry `examined` / `inspections` / `validationRequests`. Documented in
  `docs/INVESTIGATE.md` (Investigation agent → Depth rules). Tests: `depth_test.go` (both rules, the
  shared-quote-with-another-locator case, same-submission sharing, reasons, v2 unchanged).
- **Live run 3** `runs/agent-live-3/` (kube-prometheus-stack obs-north, claude-sonnet-5-5, v3):
  - submitted, 12 calls (2 rejected: a misattributed quote; a D1 rejection of an explanation of an unretrieved
    item), 58k tokens;
  - coverage 26/26 (10 raises-concern, 16 not-examined, 0 examined-no-concern);
  - depth: 2 items examined;
  - 1 validation request (override-inspection → not-run, no pin rule), 0 answer links;
  - replay byte-identical; the run record validates.
- Question B (bundle reproducibility) → orchestrate2, per the commander.

## Follow-up 4 (commander decision: structural per-item sessions) — done

- **Prompt v4** (meta `perItem`; `item.md`; default now v4, v1–v3 frozen and byte-identical):
  - `agent.ItemAgenda` order: behaviour/unknown → rendered-unexplained → rest, ties by id;
  - one driver session per item with its own task, `item@v1` budget (`AgentConfig.ItemBudget`, else 16 calls /
    14 turns / 150k tokens / 1 MiB / 360 s), prompt digest and call id;
  - cap `AgentConfig.MaxItems` (default 10) → `AgentRun.ItemCap`; beyond-cap items are closed by the runner;
  - one-entry `submit_findings` for the session's item; D1/D2 apply.
- **Concerns need item evidence**: a raises-concern is an item concern only in its item's session and citing
  an artifact retrieved there (read_artifact, get_rendered_object, grep hits, the item's sources via
  get_change_item, the renders behind query_diff results). Otherwise it becomes an `AgentNote` (AI, labelled,
  `Investigation.AgentNotes`; never reaches classify). A closing concern without item evidence → the item is
  closed not-examined, naming the note.
- **Aggregate + per-item reporting**:
  - status = worst session; reason lists the non-submitted items;
  - totals for turns, tool calls and tokens; run digest = digest of the item digests;
  - coverage per item: session status, reason, call id, turns, tool calls, tokens;
  - the transcript is segmented per item, and replay walks the segments.
- Tests: `peritem_test.go` (order, v4 task, sessions + notes + classifier blindness to notes, cap + submission
  scope, independent budgets, byte-identical per-item replay); fake-CLI per-item test (one CLI session per
  item). v1–v3 tasks/digests verified byte-identical against the previous commit. Run 3 replays byte-identically
  with the v4 agent code at its commit.
- **Live run 4** `runs/agent-live-4/` (obs-north again, v4):
  - 10 sessions, all submitted; 72 calls, 124k tokens;
  - 10/10 sessions examined their item (5–10 calls each);
  - 4 concerns, 5 examined-no-concern, 1 explains, 16 beyond cap;
  - 4 validation requests (all not-run), 3 answer links, 0 notes;
  - replay byte-identical. The v3 vs v4 table is in its README.
- **For orchestrate2 (not my file)**: `run/pipeline.go` must copy `AgentOutcome.Notes` into
  `Investigation.AgentNotes`. Its default prompt (agent.DefaultPromptVersion) is now v4.

## Follow-up 5 (commander decision: verified setting claims) — done (GLM handoff)

- **Prompt v5** (meta feature `claims`; default now v5, v1–v4 frozen and byte-identical):
  environment facts are structured, not prose. `record_claim` states "setting S is absent /
  present / has value V in input X" (an environment file, an environment render, or
  `environment-files` = all environment files) and the tool layer verifies it
  deterministically before recording (`claims.go`, verifier `investigate.claim-check@v1`):
  - the setting is searched as a dotted key path from a YAML root, as its last key anywhere,
    and as a whole token in the text — so `absent` holds only when it occurs nowhere in any
    form (a wrong or partial path is still caught by the last-key and token searches);
  - `value` compares the decoded YAML scalar at every key match, canonically;
  - a failed verification **rejects the claim with the contradicting line** (the live-run-4
    failure mode — "the environment does not set X" while `values-prod.yaml:8` sets it — is
    now a rejection, not prose);
  - present/value claims cite the matching lines (≤5 evidence); `absent` cites none but lists
    the artifacts `Checked`.
- **Domain** (`SettingClaim` `clm-…`, AI provenance — the claim is the agent's, the check is
  deterministic): `Investigation.AgentClaims` with `Validate` (derived id, kind/value shape,
  verifier/checked/detail, present-value needs evidence, no duplicates) and `ValidateAgainst`
  (item, evidence, artifacts resolve). `AgentAssertion` / `Hypothesis` / `AgentNote` /
  `AgendaCoverage` gain `StatementKind` / `SummaryKind` (`unverified-ai-prose` for agent free
  text under a claims prompt; runner-written summaries carry none) and `Claims` (checked ids).
- **Tool layer**: `Tools.RecordClaim` (+ Dispatch, not in base `ToolNames`; prompts without the
  feature reject it as an unknown tool). `record_assertion` / `submit_findings` accept `claims`
  ids (`claimRefs`: recorded in this run, and about the record's item or no item). MCP serves
  `record_claim` from the prompt's tools.json (v5 only). Schemas regenerated.
- **Runner** (`run/pipeline.go`, orchestrate2's file, additive): copies `AgentOutcome.Notes`
  → `Investigation.AgentNotes` (the follow-up-4 gap I had flagged) and `AgentOutcome.Claims`
  → `Investigation.AgentClaims`; `agentOutcome()` reads both back.
- **Live run 5** `runs/agent-live-5/` (obs-north again, claude-sonnet-5-5, v5; run during the GLM handoff, no
  quota failure): 10 sessions, all closed by the model after spending their item budgets (status
  `budget-exhausted`; the allowed closing submit each); 186 turns, 167 metered calls, 253 869 tokens (~2×
  run 4). **20 `record_claim` calls: 13 verified + recorded, 1 verification rejection** (a false absence of
  `arbitraryFSAccessThroughSMs` in the render, rejected with the contradicting line) **and 6 budget
  refusals** (never verified; `errorClass: budget` in the transcript distinguishes them). The run-4 failure
  mode is structurally fixed: run 5's first claim verifies
  `prometheus.prometheusSpec.ignoreNamespaceSelectors = true` in `values-prod.yaml` — the fact run 4 stated
  backwards in prose. 10 of 13 assertions name claim ids; every agent summary/statement is labelled
  `unverified-ai-prose`. 4 validation requests (all `not-run`: no pin rules for these items), 0 answer links,
  0 notes; dispositions 4 raises-concern / 4 examined-no-concern / 1 claims-mitigation / 1 explains, 16
  beyond cap. Replay byte-identical (167 digests); the run record validates against the rebuilt bundle.
- Tests (`claims_test.go`): every verification outcome + rejection (false absence with the
  contradicting line, wrong kind, non-environment input, value on a token, unknown input,
  value/kind shape), claims backing assertions/submissions (wrong item, unknown id), prose
  labels, v5 per-item run with a rejected false-absence claim replayed byte for byte, v4
  unchanged, `DefaultPromptVersion` = newest embedded (guard test).
- Docs: `docs/INVESTIGATE.md` "Setting claims (prompt v5…)".

## Merge review (commander request, 2026-10-16 GLM handoff)

- **Queued work beyond follow-up 5: none** (brief complete; the follow-up-4 orchestrate2 note was done here
  in 0576e618; remaining open questions are commander/user decisions).
- **v1–v4 prompt files byte-identical to ea983257**: PASS — `git diff ea983257 -- …/prompts/investigate-agent/{v1,v2,v3,v4}/`
  = 0 lines each; the only change under `prompts/` is the new `v5/` directory.
- **Runs 1–4 replay under current code: REFUSED (prompt-digest mismatch), root cause external to this
  branch.** Each replay driver refused: "the transcript was recorded under prompt sha256:X …, this session's
  prompt is sha256:Y" — the *rendered task* differs because the rebuilt bundles differ from the recorded
  ones (run 1 `bnd-d5289346bfdb`→`bnd-31638a3c8883`, run 2 `bnd-b44c21b921e1`→`bnd-a47aa06e6959` with the
  agenda itself changed 14→16 items and the deterministic pass 49→55, run 3 `bnd-7cbbf08745dc`→
  `bnd-071392e03fc2`, run 4 `bnd-e4e9c4a8f814`→`bnd-071392e03fc2`). Control experiment: `ea983257`'s own
  code (git-archive export) under today's cache rebuilds run 4's case to the **same** `bnd-071392e03fc2`
  with the identical digest mismatches — this branch changes nothing about bundle assembly or v1–v4 prompt
  digests. The drift is the **shared `.ri` cache** (121 entries written since 14:00 today by other lanes —
  runtime since 14:30, eval since 15:02 incl. live fetches on cache misses). Within today's cache state the
  code is deterministic: runs 3/4/5 (same case) all rebuild to one id, and run 5 (recorded under this
  branch + current cache) replays byte-identically.
- **Fleet finding for the commander (not a lane gap):** stored runs replay only at their recording-time cache
  state; the shared cache is live. This also affects the eval lane's stored runs (run 2's case now rebuilds
  with a different agenda). Options — freeze/pin the cache for recorded runs, re-record at a frozen state, or
  accept replay-at-commit-and-cache semantics — are the commander's. Sharp edge noted: the opt-in live test
  PASSes on a refused replay (Mismatches stays empty; the refusal shows only in the run status), so replay
  checks must grep for the refusal, not rely on the test exit.

## GLM handoff log

- 2026-10-16 (glm-agent, GLM-5.3): took over the lane mid-follow-up-5. Found the feature
  (domain contract, claims.go, prompt v5, wiring, tests) implemented and green in the working
  tree but uncommitted; nothing in it looked unfinished — build/vet/fresh tests passed and
  schema regen produced no drift. Continued by: committing it in three `[glm-handoff]`
  commits (domain contract; claims + v5 + tests; runner), writing the missing
  `docs/INVESTIGATE.md` section (v5 + prose labels; dropped stale "the default" annotations
  from v3/v4), and running live run 5 (obs-north, v5) + replay. The run completed on
  claude-sonnet-5-5 with no quota failure (the commander's quota warning arrived mid-run; the
  single in-flight attempt was left to finish, no retries were made and no model was
  substituted). Run 5 artifacts + README committed under `runs/agent-live-5/`; docs live-runs
  list updated. Full `go build ./... && go vet ./... && go test ./...` green on the branch;
  gofmt clean. Cleanup: `bin/` removed, scratch outputs under `/tmp` deleted; no kind
  clusters or servers were created by this handoff. Status entries above the handoff log were
  written by ri-agent; this section is the GLM handoff record the handoff prompt requires.
- 2026-10-16 (glm-agent): ran the commander's merge-review checks (section above). One process note: my
  first replay sweep "failed" because I passed the transcript path relative to the repo root — the test's
  cwd is the package dir, so the file wasn't found; re-run with absolute paths. The four subsequent PASSes
  are *vacuous* (refused replays leave Mismatches empty); the real result is the refusal analysis above.
- Uncertainties: (1) I inferred follow-up 5's scope from the in-flight diff and the
  "commander decision 2026-10-16" comment in claims.go — COMMANDER.md's decision list (at
  p4-env-impact @ ea983257 merge state) does not yet name it; if the commander's ask differed,
  the delta is visible in these commits. (2) I chose obs-north for live run 5 to compare
  against run 4's false-absence failure mode (the feature's motivating case); a different
  input was not specified anywhere I could see. (3) Open observation for the commander (not
  acted on): v5 sessions spend their whole item budget on the claim/verify loop (all 10
  budget-exhausted, ~2× run 4's tokens; 6 claims refused unverified by the budget). If every
  agenda item should get a chance to state its facts, either the item budget or the claim
  cost needs a look — your call, generic lever, not tuned to a case.

## Decisions (with why)

- **v4 uses `AgentConfig.ItemBudget` (a pointer)**, not `Budget`: the runner passes the investigation-wide
  defaults (80 calls, 1200 s) as `Budget`, which would make every item session large; a pointer keeps existing
  config digests and investigation ids unchanged.
- **Retrieved = content returned in the session** (not ids listed): a reference list (search terms without
  grep) does not count; the renders behind query_diff results and the item's upstream sources do.
- **Scripted steps are consumed across sessions** (up to each accepted submit), so a flat script also drives
  per-item runs; `ByItem` gives each session its own steps.

- **D1 compares quotes, not evidence ids**: an evidence id includes the locator, so a reused quote under a
  new locator would otherwise count as unique.
- **D1/D2 are checked when a record is made** (no retroactive invalidation), so an accepted call stays
  accepted and replays deterministically.
- **D2 counts any recorded request for the item**, whatever its outcome: the rule demands that the agent ask,
  not that the validator can decide.

- **Answer links need the agent's request plus a coverage guard.** The agent can only weaken its own concern
  this way, and the classifier still decides whether the pass counts (behaviour items: pin or observed only).
  Linking by subject overlap alone would let a validation of a different property answer a concern nobody
  asked it about.

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
- `internal/investigate/run/pipeline.go` (follow-up 5, additive): stores `AgentOutcome.Notes` →
  `Investigation.AgentNotes` and `AgentOutcome.Claims` → `Investigation.AgentClaims` (the note copy was the
  follow-up-4 gap flagged for orchestrate2; done from this lane as a two-line additive change, plus
  `agentOutcome()` reading them back). `run/agent_test.go`: one test.

## Contract changes

- `// CONTRACT-CHANGE(agent)` (follow-up 5): domain `SettingClaim` (+ prefix `clm-`, `ClaimKind` /
  `ClaimKinds`, `SettingClaimID`, checks in `Validate`/`ValidateAgainst`, `Investigation.AgentClaims`),
  `StatementUnverifiedProse`, `StatementKind` on `AgentAssertion` / `Hypothesis` / `AgentNote`,
  `SummaryKind` on `AgendaCoverage`, `AgentAssertion.Claims` (+ check); investigate `ClaimInput`,
  `Tools.RecordClaim` (not in `ToolNames`), `AgentOutcome.Claims`, `AssertionInput.Claims`,
  `ItemSummary.Claims`. Schema enum `ClaimKinds`.
- `// CONTRACT-CHANGE(agent)` (follow-up):
  - domain: `AgentRun.Coverage []AgendaCoverage`, `AgentRun.BudgetRule`, `AgendaCoverage`,
    `CoverageDisposition` (+ `CoverageDispositions`, schema enum) and the coverage checks in
    `Validate`/`ValidateAgainst`;
  - investigate: `ItemSummary.Disposition`, `InvestigateInput.ValidationArtifacts` / `ValidationEvidence`.
- `// CONTRACT-CHANGE(agent)` (follow-up 4): domain `AgentNote` (+ prefix `ntn-`, `Investigation.AgentNotes`,
  checks in `Validate`/`ValidateAgainst`), `AgendaCoverage` session fields, `AgentRun.ItemCap`; investigate
  `AgentConfig.MaxItems` / `ItemBudget`, `AgentSession.ChangeItem`, `AgentOutcome.Notes`. Follow-up 3:
  `AgentAssertion.NoValidationReason`, coverage depth fields, `NoValidationReason` inputs.
- `// CONTRACT-CHANGE(agent)` (follow-up 2): domain `ValidationRequest.Answers` (+ `Validate` check);
  investigate `ValidationRequestInput.Answers`, `AgentOutcome.AnswerLinks`, `AnswerLink`.


- `// CONTRACT-CHANGE(agent)` `AgentOutcome.ProposedNodes []domain.GraphNode`: placeholder nodes for proposed
  edges whose ends are not in the graph (a node id cannot be turned back into kind + key). `agent.MergeGraph`
  uses it.

## Open questions / notes for the commander

Follow-up 4 (live run 4; see its README):

- Per-item sessions bought depth (10/10 sessions examined vs 2/26 items in run 3), at 2.2× the tokens; the cap
  (10) now decides coverage — a cost trade-off for you.
- Prose is not checked, only citations: session 1's summary says the environment does not set
  `ignoreNamespaceSelectors`, but `values-prod.yaml:8` sets it to true (still closed raises-concern, fail-safe).
  A generic remedy: a structured "absent" claim checked by a recorded grep. Your call.
- Every requested validation was not-run, so the answer links lifted nothing: a validator-coverage question.

Follow-up 3 (live run 3; see its README):

- The depth rules made coverage honest: `examined-no-concern` disappeared and unexamined items are
  `not-examined`. They do not make the agent spend its budget (12 of 168 calls). A budget-floor rule
  (k inspections per priority-0 item before the first submission, unless not-examined with a reason) is
  the next generic lever — your call.
- Concerns are exempt from D1 (fail-safe): 9 of 10 concerns in run 3 rest on one examined item, and one
  blocked explanation was relabelled raises-concern. This inflates concerns, never clearances.

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
