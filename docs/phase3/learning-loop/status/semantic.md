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
- Real run complete (see "Real run" below; full record in `docs/phase3/learning-loop/runs/semantic-v2/`).

## Real run

Brief scope: the 8 environment-case edges, sonnet + haiku everywhere, opus on 4
(cert-manager ×2, karpenter, strimzi) — 519 unique candidates, 1 197 stateless
`claude -p` calls ($50.08), 1 122 proposals + 63 recorded failures in a
knowledge-layout store, 0 invalid records (env-gated integrity test). Full numbers,
agreement tables and reproduction steps: `docs/phase3/learning-loop/runs/semantic-v2/README.md`.
Headlines: cross-model aspect agreement (both asserted) sonnet↔haiku 44/68/29/34 %,
opus↔sonnet 59/73/46/62 % (subject/change/applicability/consequence); full abstention
sonnet 159 / haiku 113 / opus 33; `action-required` requests haiku 47 / sonnet 5 / opus 6
(both S+H on 5 candidates); confidence only low/medium; mean citations per asserting
proposal 1.06–1.15. The store, exchanges, envelopes and reports live in
`/Users/tommydavison/repos/Release-Intelligence-/.ri/semantic-run-v2/` (gitignored);
importing it into the committed `knowledge/` tree is left to the commander (knowledge-lane
ownership). The run also fixed d5387fd (proposals built on another candidate variant are
now refused) and completed 2 initially-failed haiku calls (1 recovered, 1 recorded refusal).

PO-1 same-model consensus measurement (second GLM handoff): two independent sonnet
call-sets on strimzi (62 calls, $2.46, 0 shared call ids, 0 proposal-id overlap).
Same-model self-agreement is *low* — subject 2/11, change 9/16, applicability 1/11,
consequence 7/13 (both-asserted, strict digests) — below sonnet↔haiku cross-model
agreement, while `action-required` requests are stable across calls (5 common of 6/5).
Two calls of one model are checkably separate measurements that do not converge on typed
detail; consensus signal is cross-model + deterministic validation, as the contract's
consensus-scope rules encode. Full record: `runs/semantic-v2/README.md` § Same-model.

## Next

- Rendered-diff addendum: plumbing ready (render evidence enters through the candidate's evidence;
  prompt version `+rendered`); waits for the render lane (its work sits unmerged on `p3ll/render`).
- Commander decision requested: import `.ri/semantic-run-v2/knowledge` into the committed
  `knowledge/` tree (or keep per-run scratch stores), and route `ri knowledge candidates|propose`
  through the semantic subcommands (stub drop listed above).
- Possible follow-up the real run suggests (needs product-owner judgement, not done): the 58 haiku
  refusals are dominated by typed-constraint violations (empty `all` conditions, missing
  gvk/crd-field identity fields, `before`/`after` on value-changed) — prompt-vocabulary examples for
  exactly those shapes would likely cut the refusal rate; refusing is correct behaviour meanwhile.

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

`go build ./... && go vet ./... && go test ./...` green on the branch at 86ec7de (verified again
after the second handoff's commits; the new `TestStoreIntegrity` skips offline and passes against
both real-run stores: 519 candidates / 1122 proposals / 0 invalid, and 31/50/0 same-model).

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

## GLM handoff log (second handoff)

GLM-5.3 again, while the Claude agent sits out the usage-limit window. The real run had
finished on disk (run-v2 + opus in the paused session's scratchpad) but was unreported;
nothing of it was lost — I found it, completed it, measured, and recorded it. Commits
(all `[glm-handoff] `):

1. `22aa879` env-gated `TestStoreIntegrity` (polished from the paused agent's uncommitted
   zz scratch test): validates any `propose -out` store — candidates, every proposal against
   its stored candidate, citations ⊆ shown evidence. Skips offline.
2. `4e15ca1` real-run record `docs/phase3/learning-loop/runs/semantic-v2/` + this file's
   missing "Real run" section. Completed the 2 haiku calls that had failed with
   `error_max_structured_output_retries` (1050/1050 answered; 1 recovered, 1 refused+recorded),
   re-ingested all 8 edges from the answered exchange (0 pending), regenerated the opus
   reports, and copied every artifact to `.ri/semantic-run-v2/` (gitignored, path recorded)
   because the scratchpad dies with the paused session.
3. `86ec7de` analyze.py agreement tail crashed once opus (4 of 8 edges) joined the store;
   fixed (agreement → analyze2.py, pairwise action-required cross-listing added), outputs
   regenerated on the final 1122-proposal store.
4. same-model measurement (PO-1) + final record (this commit): `samerun2.sh`, two independent
   sonnet call-sets on strimzi (62 calls, $2.46), `analyze_same.py`, results in the run
   README § Same-model and summarised above.

Live `claude -p` spend this handoff: $2.46 (same-model) + ~$0.10 (2 retried haiku calls +
1 probe). Everything else replayed from the answered exchange.

Uncertainties / for the commander:
- Importing `.ri/semantic-run-v2/knowledge` (1 704 records) into the committed `knowledge/`
  tree is your call (knowledge-lane ownership; it is release-level content, env-independent).
  The full audit trail (requests, responses, CLI envelopes with cost) is in the same dir.
- The same-model numbers are one edge (strimzi) with small both-asserted denominators; treat
  them as a floor measurement, not precise rates. Extending to a second edge is mechanical
  (`samerun2.sh` pattern) if wanted.
- One zsh footgun cost nothing but time: an unquoted `$EDGES` loop collapsed to one iteration
  and a junk report filename (deleted; the risky cleanup glob also hit 3 report files, restored
  from the scratchpad copies and then regenerated — final reports in the run dir are complete).
- The 58 haiku refusals cluster on a few typed-constraint shapes (noted under Next); fixing
  them via vocabulary examples is a prompt-version change I did not make unilaterally — it
  would move the ground the just-finished run measured.
