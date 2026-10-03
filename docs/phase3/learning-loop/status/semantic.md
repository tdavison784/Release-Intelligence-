# Lane `semantic` — status

Branch `p3ll/semantic-3` (base `p3-learning-loop` @ 9d5126a; earlier waves on `p3ll/semantic`).

## semantic-3 (third handoff, in flight)

The rendered-diff addendum run (`runs/semantic-v3/`): 7 new edges (crossplane, external-secrets,
flux, kyverno, loki, prometheus-operator, traefik), `-render` everywhere, sonnet+haiku on all 7,
opus + GLM-5.3 (zai) on the 4 smaller ones — the first real cross-model pairs. The paused Claude
agent got through candidates (573), requests (1 333 claude + 187 zai), 592/1 333 claude answers
(the account session limit stopped `claude -p`; resets 15:50 America/Chicago), 187/187 zai answers
($25.76), and a partial pass-2 ingest (722 proposals, 0 invalid). Remaining: answer the 741
pending claude requests, pass-3 ingest, analysis (`analyze_models.py`, `analyze_cross.py`), run
record, commit. Driver: `.ri/semantic-run-v3/run3.sh`. Details and outcome: `runs/semantic-v3/README.md`.

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

## GLM handoff log (third handoff: semantic-3)

GLM-5.3 again, taking over the fresh `p3ll/semantic-3` wave (rendered-diff addendum run over
7 new edges). The paused Claude agent's run was mid-flight at takeover — nothing lost:

1. `a46206e` committed the paused agent's two v3 analyzers verbatim (per-model outcomes incl.
   rendered-evidence citation counts; cross-model agreement adapted for the zai-containing store).
2. `d4cfeb9` run-record skeleton (`runs/semantic-v3/README.md`: scope, commands, files) + this
   status section; outcome sections TBD until the run completes.
3. Wrote `.ri/semantic-run-v3/pass3.sh` (post-exchange ingest: sh on all 7 edges, opus on the 4
   SUB ones; glm was fully ingested in pass 2) and verified the analyzer runs on the partial store.
4. Verified at takeover: build/vet/tests green; `TestStoreIntegrity` on the combined store
   1092 candidates / 1982 proposals / 0 invalid; `bin/ri` newer than every source file (pass-3
   request digests will match the paused run's).
5. Scheduled the exchange resume for just after the 15:50 America/Chicago session-limit reset
   (the exact failure that stopped the claude runner at 592/1333, per `exchange-claude/exchange.log`):
   answer the 741 pending claude requests, run pass 3, analyze, fill the record, commit knowledge/.
   [GLM note next line updated as this proceeds.]

State at takeover for the record: 573 candidates; answered claude 592/1333 ($26.05; haiku 253,
sonnet 255, opus 84), zai 187/187 ($25.76); store 722 proposals (sonnet 246, haiku 226, opus 84,
glm 166) + 57 recorded refusals (haiku 27, glm 21, sonnet 9, all `rejected`); 51 candidates carry
rendered evidence; prompt-version split already measurable (`semantic-full/v1` vs `v1+rendered`).

Uncertainties / for the commander:
- The claude half of the run spends real budget: 741 pending stateless calls ≈ $26–35 at v2's
  per-call rates. It is the paused agent's own run3.sh design (requests already written), so I
  resumed it as-is rather than shrinking scope; say the word if you want the remainder dropped.
- If HANDBACK arrives before the claude half finishes, everything is resumable with one command
  (the exchange script answers only pending requests; then `pass3.sh`).

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

## GLM handoff log (fourth handoff)

GLM-5.3 again; the Claude agent is paused for a fresh usage-limit window. Takeover state: the
semantic-3 run was fully complete (committed, recorded, imported), but the working tree held an
uncommitted, **build-breaking** context sync — the paused agent (or the handoff prep) had copied
integration content (proxy-agreement CLI wiring, loop-diagnosis-2 docs, proxy-shadow view) without
`internal/proxyreview/agreement.go`. What I did (all `[glm-handoff] `):

1. `14b1de6a` restored the green build by landing `agreement.go` + its two test files verbatim from
   p3-learning-loop (258d9490) — the file the copy was missing. `go build/vet/test ./...` green.
2. `5c5fee3c` committed the rest of the sync verbatim (LOOP-DIAGNOSIS-2.md, proxy-shadow view,
   lines/ref-resolution docs) — all byte-identical to integration, so the commander's later merge
   is a no-op for these paths. This is how this branch learned of the L1–L8 levers without a merge.
3. `5754899e` delivered the two offline analyses the diagnosis's semantic levers need
   (`runs/semantic-v3/PROMPT-LEVERS.md` + `analyze_undecidable.py`): 157 undecidable exposure
   leaves on 152/995 exposure-bearing proposals (106 pure; Opus writes 39 — a prompt gap, not a
   weak-model artifact); the `needed` texts split into decidable-with-the-right-operator (the L1
   channel-context fix; `text-line` is used by only 13 leaves) vs genuinely-runtime (55, must stay
   undecidable). L4 is **blocked on integration** (no `lines:*` kinds, no `cc:feat` classifier on
   this branch; ES/prom-op skip 0 routine notes) and its cost is measured for when it lands
   (admitting all routine notes = +20 % candidates, dominated by loki's 68 metrics notes).
4. `18cf2eb8` staged **run4**, the controlled with/without rendered-evidence measurement the run
   README names as the addendum's open step: `ri semantic propose -only FILE` (new, tested) +
   `run4.sh` + committed id lists (`run4-only/`, 51 candidates — ES 12, kyverno 28, prom-op 8,
   traefik 3, all SUB edges) + `analyze_render_ab.py` (synthetic-store smoke-tested). Pass 1 ran
   offline: 204 plain `semantic-full/v1` requests written, 51 candidates in the run4 store, 0
   missed ids, no rendered evidence in any prompt (verified; the schema's `rendered-change` enum
   and the verb "rendering" are the only occurrences).
5. Answered the **GLM half** of run4 via the zai exchange (51 stateless calls, 51/51, 0 failures;
   the claude half stays untouched while the account sits in its limit window) and ingested it:
   **45 proposals + 6 recorded refusals** (the familiar typed-constraint shapes). Interim GLM pair
   (33 candidates answered both ways): aspect decisions barely depend on the render — 1–2 flips per
   aspect out of 33, both directions, abstention 10 vs 10 — while the render is cited 22/33 when
   shown. The claude half (153 calls ≈ $6–8) completes the measurement; see the run README.

Remaining to finish run4 (one command each, any session): answer
`.ri/semantic-run-v3/run4/exchange-claude` with `scripts/semantic-exchange.sh <dir> "" 8 anthropic`
(153 calls ≈ $6–8), re-run `run4.sh`'s two loops to ingest, then
`python3 analyze_render_ab.py knowledge .ri/semantic-run-v3/run4/knowledge`.

Uncertainties / for the commander:
- **Merge p3-learning-loop into this lane before L4 work** — both halves of L4 (lines:* changes
  and the cc:feat classifier) exist only there. FLEET says wait for your word.
- The L1/L5 prompt redesign (channel context per product, vocabulary for decidable shapes) is
  deliberately NOT started: the brief reserves prompt design to the Claude agent, and a prompt
  change without budget to re-measure would move the ground the runs measure. PROMPT-LEVERS.md is
  the input for that session.
- run4's claude half (~$6–8) was left pending rather than spending the limited account from a
  handoff; the GLM half was answered because the zai path is the FLEET's documented optional
  provider and completes the opus↔glm pair.

## semantic-3 (Claude agent, after the third GLM handoff)

- Reviewed the handoff commits (a46206e, d4cfeb9, 5873b0f). The analyzers were committed verbatim and
  are correct. The run-record skeleton was accurate except that it called the GLM pairs "the first
  cross-model measurement" (semantic-2 was); fixed. The "scheduled resume" had not run: no process
  and no cron entry were found. I resumed it myself with the handoff's `pass3.sh` (correct as written).
- `semantic-exchange.sh` now stops on an account usage limit instead of failing every remaining
  request (17fc21b).
- Run complete: 7 new edges, 573 candidates (51 with release-level rendered evidence), 1 423 proposals
  (Sonnet 559, Haiku 512, Opus 186, GLM 166), 97 refusals. Claude $57.56; GLM is billed by Z.AI.
  Committed tree: 1 092 candidates / 2 683 proposals / 0 invalid; eval-reference scan passes.
- Results replicate semantic-2: cross-model (Opus↔GLM) agreement equals the best same-family pair
  (change 79 %, consequence 70 %), and free-form subject names remain the bottleneck. Rendered evidence
  is cited in 57–68 % of proposals where shown; a controlled with/without measurement is the open next
  step. Full record: `runs/semantic-v3/README.md`.

## semantic-4 (L1 + L4) — Claude agent, after the fourth GLM handoff

Review of the fourth handoff (on `p3ll/semantic-3`, carried into `p3ll/semantic-4`):
- 14b1de6a / 5c5fee3c: files copied in from integration. Verified byte-identical to `p3-learning-loop`.
- 5754899e (undecidable-leaf analysis, PROMPT-LEVERS.md): sound, and it is the design input for L1. Its
  "L4 blocked" note was measured before the integration merge; on the merged branch `lines:*` diffs and
  the routine classifier exist.
- 18cf2eb8 / 0caf8055 / 6e320cd9 (`-only` filter; the controlled with/without-render run, GLM half
  answered): kept. The Claude half (153 staged calls) is not part of this wave's order, so it stays
  staged until the ordered steps are done.

Step 1, code, committed (8c935d2f, d1e01ebe): configSources format + validation + docs; prompt v2 with
CONFIG SOURCES and the `undecidable` rule (schema + `ErrAvoidableUndecidable`); L4 candidates
(lines:* + subject-naming non-dependency routine notes, clustered only among themselves). Checked on all
15 environment edges against the pre-change binary: L4 removes no existing candidate id. Product config
data from upstream docs: in progress (research agents; committed separately with citations).

Step 2, sizing of the property-selected v2 re-proposal (`runs/semantic-v4/select_candidates.py`; each
candidate assigned to the first environment edge that generates it; never by eval link):

| rule | candidates |
|---|---|
| R1: `undecidable` leaf on an existing VERIFIED FACT | 59 |
| R1b: `undecidable` leaf on an existing proposal (no fact leaf) | 78 |
| R2: has proposals, all fully abstained | 210 |
| R3: new L4 candidate (a member is `lines:*` or a routine note) | 102 |
| **total** | **449** |

Undecidable leaf reasons on R1/R1b: environment-visibility-gap 90, runtime-behavior-gap 52,
evidence-gap 22, release-knowledge-gap 4, cross-product-context-gap 1. So 95 leaves now have a reason v2
forbids. New candidates *not* from L4 (integration edge drift, no proposals yet; not selected):
external-secrets 24, prometheus-operator 18, cilium 13, istio 2.

Estimate: 449 × (Sonnet + Opus + GLM) = 1 347 calls. Claude side at the semantic-3 per-call rates
(Sonnet $0.047, Opus $0.102) is about $67, ~$75 with v2's longer prompts: **within the ~$100 budget,
so no priority cut**. GLM: 449 calls, billed by Z.AI.

Step 3, the v2 re-proposal run: **complete and recorded** (`runs/semantic-v4/README.md`) — 868/898
claude + 449/449 zai answered, 1 286 v2 proposals (sonnet 428, opus 436, glm 422) + 31 refusals
imported into the committed tree (now 1 194 candidates / 3 969 proposals / 0 invalid), Claude $68.36.
The L1 lever works: environment-visibility-gap leaves 87 → 0 on R1+R1b, R1b undecidable 108 → 20
(61/78 candidates cleared, 36 with a decidable predicate instead). 30 claude requests (18 sonnet,
12 opus, ≈$2.4) sit pending on the account session limit — resume command in the run README.

## GLM handoff log (fifth handoff)

GLM-5.3 again; the Claude agent is paused for a fresh usage-limit window. Takeover state: the
semantic-4 step-3 run (`run4v2.sh`) had finished on disk (log ends `RUN4V2 DONE`) with the claude
exchange stopped at the session limit — 30 pending — but nothing was committed and no run record
existed. What I did (all `[glm-handoff] `):

1. Verified the run state end to end before touching anything: build/vet/tests green; pending
   count 30 in the exchange matches the pass-2 reports; 868 + 449 answered = 1 286 proposals +
   31 refusals recorded in the worktree knowledge tree, every per-model count reconciled.
2. `SEMANTIC_STORE=knowledge TestStoreIntegrity`: 1 194 candidates / 3 969 proposals / 0 invalid
   (relative-path footgun: the test's cwd is the package dir — pass an absolute store path).
3. Ran the paused agent's `analyze_v4.py` (committed verbatim) and wrote the run record
   `runs/semantic-v4/README.md` incl. per-rule old→new undecidable tables, per-model outcomes,
   costs from the CLI envelopes, and the one-command resume for the pending 30 (4d525e74).
4. Imported the run results into the committed `knowledge/` tree: 102 candidate records + 1 286
   proposals + 31 refusals (c3e18aa1).
5. Found and measured **prompt-render drift**: `bin/ri` (02:03:16) predates 06ad59bd's
   `configSourceLine` change (02:04:22, committed while pass 1 ran). Re-proposing every edge with
   a HEAD-built binary against an exchange copy: 38/449 candidates (traefik 25, istio 6,
   karpenter 5, argo-cd 2 — the config-file-channel products) get different prompt digests under
   HEAD, all models; 3 of the 30 pending are drifted. Provenance is honest (per-proposal
   promptDigest of what was seen; exchanges hold the request bodies) and within-run comparisons
   are unaffected, but the label v2 spans two renders. Recorded in the run README § Prompt-render
   drift with the resume warning (do not rebuild `bin/ri` before the 30 are answered); the
   accept-vs-bump-and-rerun decision (≈114 calls ≈ $6) is the Claude agent's/commander's.

Uncertainties / for the commander:
- The 30 pending claude calls (≈$2.4) were left pending on purpose — the account was still inside
  its limit window at takeover (resets 6:50 America/Chicago), and prior handoffs' pattern is not to
  spend the limited account from a handoff. Resume is one command (run README § Commands); after
  it, re-run the analyzer — the record's tables will shift by at most 30 cells.
- R1's undecidable-leaf total rose 51 → 55 (runtime-behavior-gap from opus 17 / glm 27; sonnet 0
  in v2). The reason is one v2 keeps (genuinely runtime per PROMPT-LEVERS.md), so this is model
  disagreement, not lost ground — but it is an observation for the review lane, recorded in the
  run README.
- The staged run4 claude half (153 calls, fourth handoff) remains untouched and staged.
- No eval/, gates, or expectations were touched; no merge/rebase/push; `.lane-lead` untouched.

### semantic-4 step 3, completed (Claude agent, after the fifth GLM handoff)

Handoff review (4d525e74, c3e18aa1, f8b0e63f, 6065cc59):
- The run record, analyzer and import are sound.
- Two corrections:
  - The refusal records had been committed under `knowledge/<product>/failures/` and are moved to
    `runs/semantic-v4/failures/`, as in v2/v3.
  - The prompt-render drift GLM found is real (`bin/ri` was built before my last `configSourceLine`
    edit). Rather than bump-and-rerun, the code now renders the entries the run saw byte-identically.
    A rebuilt binary wrote no new request, so `semantic-full/v2` again names one rendering.
- The 30 requests pending at the account limit were answered after the reset.

Result: 449 property-selected candidates, 1 316 v2 proposals (Sonnet 446, Opus 448, GLM 422), 31
refusals. Committed tree 1 194 / 3 999 / 0 invalid; eval-reference scan passes. Claude $70.72 (budget
~$100). Every `undecidable` leaf with a v2-forbidden reason is gone: 90 → 0. On R1b, all leaves went
108 → 20, 61/78 candidates are now undecidable-free and 36 of them carry a decidable predicate instead.
122/210 all-abstained candidates and 35/102 new L4 candidates now carry assertions. Full record:
`runs/semantic-v4/README.md`.

Open for the commander:
- The controlled with/without-render run's Claude half (153 staged calls, ~$8) is still staged.
- `ri stats` reports the new `configSources` constructs as unaccounted (onboarding records are history;
  not back-dated).
- `knowledge.ProposalContext.ConfigSources` is a contract-marked additive field the proxy reviewer can
  also use (L1 names both).
