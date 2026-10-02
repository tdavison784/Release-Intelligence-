# Lane `semantic` — status

Branch `p3ll/semantic` (base `p3-learning-loop` @ 1174492, knowledge-3 merged).

## Done

- `candidates.go`, `hints.go`: deterministic restatement clusters (`BuildCandidates`,
  groupings single / same-statement / same-title / title-jaccard / subject-named, skips recorded,
  no-join computed rules become their own candidates) + typed hints.
- `schema.go`, `prompt.go`, `vocabulary.go`: typed per-task answer schemas (structured-output
  dialect, per-request citation enums, confidence capped at medium/low), per-task prompt versions
  `semantic-<task>/v1`, vocabulary with one Validate-proven example per entry. Injection stance as
  `impactenrich`; values never sent; eval expectations never in prompts.
- `answer.go`, `proposer.go`: `LLMProposer` over `llm.Client`, `ProposeAll(With)`, refusals recorded
  never retried (hallucinated citation, uncited assertion, exposed class, high confidence,
  not-affected, action-required without an action-eligible consequence, missing call id, schema
  violations).
- PO-1 (call ids): call id threaded through every llm transport (Messages API message id, exchange
  `callId`, per-call fake ids; cache replays the original id); `AnswerMeta.CallID` required;
  run-report agreement keys answers by provider+model+call id so two calls of one model count
  (pair labelled `model|model`); `scripts/semantic-exchange.sh` records the `claude -p` session id.
- PO-2 (action-required is a request): `suggestableClasses(task)` offers action-required only to
  consequence-answering tasks; the domain validator still demands an action-eligible consequence in
  the same proposal; prompt states the trust ladder.
- CLI `ri semantic candidates|propose` (`cmd/ri/semantic.go`): knowledge-layout record writer
  (`report.go WriteRecords`), run report (per-model outcomes, per-aspect agreement), exchange/cache
  plumbing.
- `scripts/semantic-exchange.sh`: stateless one-`claude -p`-per-request exchange runner, envelope
  kept for audit, failures logged and never fabricated.
- docs/SEMANTIC.md + the two commands in the README list.
- Offline smoke (warm cache, no LLM calls): `candidates` on cert-manager v1.17.0→v1.18.0 →
  39 candidates / 52 changes / 43 members, skips {multi-subject-computed:2, security-fix:5};
  `propose -llm-exchange` writes requests + candidate records and reports them pending.

## Claude agent resumed (after the GLM handoff)

- Reviewed every `[glm-handoff]` commit: 0e21a21/4d6ce5f are the paused agent's own work committed
  verbatim; 0be37e9 (reviewui demo call id) and 1a28d64 (agreement keyed per call) are correct; the
  docs were accurate apart from small errors, corrected (example versions, missing skip and
  never-cross-subjects rules, store wording, Go API section). Nothing reverted.
- GLM's open questions: scope stays the brief's **8 environment cases** (the brief is explicit); the two
  "possible missed joins" are deliberately conservative (different subject signatures; the duplicate
  task / review catches them; a wrong join would attach a fact to a change it does not describe).
- Merged knowledge-3 (clean). `-out` now writes through `knowledge.NewFileStore` (validated by the
  store; an existing candidate under the same id is kept). Run-report label uses the domain's
  consensus scope (Sonnet+Haiku = one family = `same-model`).
- `ri knowledge candidates|propose` stay stubs in the knowledge lane's file (their test pins the stub
  message). Recommendation for the commander/knowledge lane: route them with
  `case "candidates": return c.semanticCandidates(rest)` / `case "propose": return c.semanticPropose(rest)`
  and drop the stub assertion in `knowledge_test.go`.
- Real run in progress (see "Real run" below).

## Next

- Rendered-diff addendum: plumbing ready (render evidence enters through the candidate's evidence;
  prompt version `+rendered`); waits for the render lane.
- Opus and repeated same-model calls (PO-1 same-model consensus measurement) after the Sonnet+Haiku run.

## Decisions

- PO-1: an answer without a call id is refused at the `AnswerMeta` level (recorded as a failure),
  so every stored proposal is checkably one stateless call.
- PO-1: run-report agreement keyed by provider+model+call id; same-model pairs reported, not hidden
  (`SameFamily` keeps labelling one-family groups).
- GLM handoff: live `claude -p` runs deferred (see above); scratch test zz_scratch_test.go deleted
  (its cluster-inspection need is covered by `ri semantic candidates`).

## Files touched outside ownership

- `cmd/ri/main.go`: one additive `case "semantic"` + usage line (merge with knowledge/review cases resolved).
- `internal/llm/{llm,anthropic,exchange,cache,fake}.go`: additive `CallID` plumbing (PO-1), with tests
  (`callid_test.go`, one assertion in `anthropic_test.go`).

- `internal/reviewui/demo.go` (dashboard lane): one additive hunk — demo proposals get a
  deterministic `CallID`, required by contract-3's domain validation
  (commits 0be37e9). The real dashboard follow-up (consensus-scope labels on the disagreement
  items) is left to that lane.

## Contract changes

none proposed (PO-1/PO-2 implementations follow contract-3 as merged).

## Test status

`go build ./... && go vet ./... && go test ./...` green on the branch at the last GLM-handoff
commit (incl. the reviewui repair; see handoff log).

## GLM handoff log

GLM-5.3 continued this lane while its Claude agent sits out a usage-limit window. What I did
(all commits subject-prefixed `[glm-handoff] `):

1. Committed the paused agent's finished but uncommitted work: llm call-id plumbing (0e21a21);
   semantic PO-2 + call-id wiring incl. the exchange runner's session-id recording (4d6ce5f).
   Build/vet/tests were already green for these.
2. Repaired the cross-lane test breakage contract-3 introduced: reviewui demo proposals now carry
   call ids (0be37e9) — minimal, additive, dashboard lane notified via this file.
3. Fixed the run report to count same-model separate calls in agreement (1a28d64), with a test
   that fails on the old model-keyed behaviour.
4. Wrote docs/SEMANTIC.md and added the semantic commands to the README list (8123144).
5. Smoke-tested the deterministic half of the real run on the warm cache (details above) and the
   propose→exchange pending flow offline; deleted zz_scratch_test.go; confirmed the shared
   `.ri/llm-cache` was not written by the smoke.

Uncertainties / for the commander:
- Real-run scope: brief says "the 8 environment cases' edges"; eval/cases has 17 now. Which set?
- The reviewui demo repair may conflict trivially with the dashboard lane's own PO-1/PO-2
  follow-up; my hunk is one line inside their builder.
- I did not retune the clustering on the two observed possible missed joins — that should follow
  the real run's measurements, and changing it before would move the ground the run measures.
