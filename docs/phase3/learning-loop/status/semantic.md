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
- Reviewed the second handoff (22aa879, 4e15ca1, 86ec7de, 0239d20). The run record, integrity test,
  analyzers and same-model experiment are sound: separate exchange dirs/caches, 0 shared call ids,
  identical prompt digests across the two call-sets. One correction: the *reading*. It called the
  Claude pairs "cross-model" (they are one family: scope `same-model`) and compared a strimzi-only
  same-model floor with all-edge agreement. Like for like, strimzi S↔H is 2/13 vs S↔S 2/11. The driver
  is free-form-name subject families (see Contract changes). README and this file are corrected;
  nothing reverted. GLM wrote the artifacts to the primary checkout's gitignored
  `.ri/semantic-run-v2/` (fine; that path is the durable copy).

## Real run

Brief scope: the 8 environment-case edges, sonnet + haiku everywhere, opus on 4
(cert-manager ×2, karpenter, strimzi) — 519 unique candidates, 1 197 stateless
`claude -p` calls ($50.08), 1 122 proposals + 63 recorded failures in a
knowledge-layout store, 0 invalid records (env-gated integrity test). Full numbers,
agreement tables and reproduction steps: `docs/phase3/learning-loop/runs/semantic-v2/README.md`.
Headlines: cross-tier (one family: consensus scope same-model) aspect agreement (both asserted) sonnet↔haiku 44/68/29/34 %,
opus↔sonnet 59/73/46/62 % (subject/change/applicability/consequence); full abstention
sonnet 159 / haiku 113 / opus 33; `action-required` requests haiku 47 / sonnet 5 / opus 6
(both S+H on 5 candidates); confidence only low/medium; mean citations per asserting
proposal 1.06–1.15. The store, exchanges, envelopes and reports live in
`/Users/tommydavison/repos/Release-Intelligence-/.ri/semantic-run-v2/` (gitignored);
importing it into the committed `knowledge/` tree is left to the commander (knowledge-lane
ownership). The run also fixed d5387fd (proposals built on another candidate variant are
now refused) and completed 2 initially-failed haiku calls (1 recovered, 1 recorded refusal).

PO-1 same-model measurement (second GLM handoff; reading corrected by the Claude agent): two
separate sonnet call-sets on strimzi (62 calls, $2.46; 0 shared call ids). Subject agreement
2/11, the same floor as sonnet↔haiku *on the same edge* (2/13). The driver is free-form-name
families, not scope. Across all edges, structured-family subjects agree exactly 68 % (S↔H) / 94 % (O↔S),
free-form-family subjects 8 % / 31 %. All pairs in this run are one family (scope `same-model`);
no cross-model measurement exists yet (needs GLM/Codex). See `runs/semantic-v2/README.md` § Same-model.

## Commander decisions executed (semantic-2)

1. Run imported into the committed `knowledge/` tree (66f23cf): 519 candidates, 1122 Claude proposals,
   through the store layout. The knowledge lane's eval-reference scan passes, and the store integrity
   check finds 0 invalid. Refusals go to `runs/semantic-v2/failures/`. The SAME-MODEL experiment stays
   a separately labelled store in `runs/semantic-v2/same-model-store/` and is never routed.
2. GLM-5.3 via Z.AI cross-model run (provider switch in `scripts/semantic-exchange.sh`, 77a0cec).
   It covers the 147-candidate Opus subset: 138 proposals (provider `zai`), 9 refusals, token never
   printed, logged or passed as an argument. Results are in `runs/semantic-v2/README.md` § Cross-model.
   Cross-model agreement (GLM↔Opus/Sonnet) matches the best same-family pair: change 73 %, subject
   52–55 % vs 60 %, consequence up to 65 %. Free-form subject names stay low in every scope.
   Committed tree after the GLM import: 519 candidates, 1260 proposals, 0 invalid.
3. Free-form subject names: decision (c) stays (exact digests, human review); nothing built.

## Next

- Rendered-diff addendum: **wired** now that render-2 is merged. `BuildCandidatesWith(…,
  CandidateOptions{RenderedEvidence: render.EdgeRendered.ForChanges})` attaches release-scope rendered
  evidence to the candidates it correlates with; environment renders are refused and recorded
  (`environment-render-refused`); prompts showing it are `semantic-<task>/v1+rendered`;
  `ri semantic propose -render [-kubernetes V]`. A render failure is reported and the run proceeds
  without render evidence (never "no change"). Smoke on cert-manager v1.17.0→v1.18.0: 14 rendered
  changes reach the seam, but the render correlation links them only to image artifacts and one
  joined values diff, none of them a candidate member, so 0 candidates gain evidence. The with/without
  measurement needs edges where correlation reaches prose or no-join members (render-lane input).
  Candidate ids are member-derived, so a `-render` run must use its own `-out` store.
  Render-lane note: offline, a missing chart makes `EdgeRenderedChanges` say "a  render carries
  customer configuration" (empty scope checked before status); the real cause is chart-unavailable.
- Commander decision requested: import `.ri/semantic-run-v2/knowledge` into the committed
  `knowledge/` tree (or keep per-run scratch stores), and route `ri knowledge candidates|propose`
  through the semantic subcommands (stub drop listed above).
- Measure a real cross-model pair (GLM-5.3-Flash via Z.AI, or Codex after quota) on the 147-candidate
  Opus subset: the commands are ready; every pair so far is one family.
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

- `internal/reviewui/demo.go`: none left. The GLM stopgap call-id hunk was superseded by the dashboard
  lane's own call ids when p3-learning-loop was merged (file identical to integration).

## Contract changes

None made. **Proposed for the contract owner (from the real run, not implemented):** subjects of
free-form-name families (protocol-behavior, migration, api-endpoint, and the name of
product-relationship/compatibility-boundary) practically never agree by `AspectDigest` (exact
subject key): 8 % S↔H, 31 % O↔S, against 68 % / 94 % for structured families. The names are invented
per call (`pod-restart-event-regardingobject` vs `…-events-…`). Since a candidate is one change, two
proposals *for the same candidate* that assert the same free-form family and the same change describe
the same subject by construction. Options:
(a) for those families, compare the subject aspect by family (+ component when stated), with the name
    recorded but not compared;
(b) a canonical-name rule chosen at review (the first accepted name wins and later proposals are mapped
    onto it);
(c) keep exact digests and accept that these subjects always route to human review.
(c) is today's behaviour and is safe; (a) would let consensus reach these subjects. Product
decision.

## Test status

`go build ./... && go vet ./... && go test ./...` green on the branch at 233b890 (after merging
p3-learning-loop @ 223b506, render-2 included). `TestStoreIntegrity` (env-gated) passes against both
real-run stores: 519 candidates / 1122 proposals / 0 invalid, and 31 / 50 / 0 same-model.

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
