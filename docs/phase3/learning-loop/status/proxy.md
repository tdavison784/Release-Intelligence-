# Lane `proxy` — status

Brief: [briefs/proxy.md](../briefs/proxy.md). Branch `p3ll/proxy`. Design and usage: [proxy/README.md](../proxy/README.md).

## Done

- Step 1 (runner): `internal/proxyreview` builds the prompt and verdict schema and turns a verdict into a decision;
  it also holds the ledger and the report. `ri knowledge proxy-prompt` writes the requests, `scripts/proxy-review.sh`
  makes one stateless `claude -p --model claude-opus-5-5` call per item, and `ri knowledge decide -reviewer-kind
  proxy -proxy-request R -proxy-response S -proxy-ledger L` records one decision per item.
- Step 2 (provenance): complete `ProxyProvenance` (model, modelVersion from the CLI envelope, promptVersion
  `proxy-review/v1`, promptDigest, callId = CLI session id, inputEvidence, generatedAt); StartedAt/DecidedAt = call
  start/end. Failures and refusals go to the ledger; none are retried blindly.
- Step 3 (self-review bias): the request and each ledger line record the proposal authors' models and families and
  the self-model / self-family flags. `ri knowledge proxy-report` splits outcomes by who authored the proposed
  assertion and per proposing model (self-model / self-family / other-family).
- Pilot: 8 items on a scratch copy of the store, all recorded, about $0.06 and 6 s per call.

## Next

- Nothing runnable on this branch: proxy-2 (run-2 + shadow pass + eval view) is done — see the
  proxy-2 section and [../proxy-shadow/REPORT.md](../proxy-shadow/REPORT.md).
- Waiting on the commander: the prose-only-corrections contract question; `Provenance.Provider`;
  and whether to re-run the 4 run-2 items refused for validator-only citations now that prompt v3
  states the rule (the prompt changed, so that is not a blind retry — the commander's call, since
  it spends calls and the prose-only decision may change the recorder too).
- Waiting on the product owner: `ri knowledge proxy-agreement` once the high items are decided.
- A new backlog (anything merged into `p3-learning-loop` since) needs a commander-ordered merge
  into this branch first.

## Decisions taken (local)

- **Proposals are anonymised (P1…Pn) in the prompt.** Model names are recorded in the request and ledger only.
  This removes overt self-preference; stylistic self-recognition cannot be removed.
- **The proxy sees no proxy decisions and no proxy-level facts.** One proxy error must not seed the next, and
  each prompt depends only on human/validator knowledge. It sees earlier *human* decisions on other items (as the
  brief asks) and never a decision on the item under review. `-no-human-context` exists for the shadow pass.
- **Evidence-sufficiency items:** these verify no aspect, so they cannot be accepted. The verdict
  `evidence-sufficient` is recorded as `defer` with the reason prefixed "evidence sufficient …"; the item stays
  open for re-proposal or a human. `need-more-evidence` is recorded as `need-more-evidence`.
- **Thinking off** (`MAX_THINKING_TOKENS=0`), as in `scripts/semantic-exchange.sh`; this is cost parity with the
  proposers.
- **Provider** is recorded as `Provenance.Rule = "provider:anthropic"`, because `domain.Provenance` has no provider
  field. Proposal to the contract owner: add `Provenance.Provider` (additive).
- **Wrong-* labels on corrections** are derived from the aspects that actually changed (`wrong-classification`
  when the exposed class changed), plus the model's own labels, kept only if they name a verified aspect.

## Files touched outside ownership (additive)

- `cmd/ri/knowledge.go`: usage lines; the `proxy-prompt` and `proxy-report` subcommands; on `decide`, the flags
  `-proxy-call-id` (the manual proxy path previously had no way to record a call id), `-proxy-request`,
  `-proxy-response` and `-proxy-ledger`.
- `internal/semantic/proxy_export.go` (new file): `semantic.Vocabulary(aspects)`, which exports the proposers'
  vocabulary text.
- `docs/KNOWLEDGE.md`: one CLI sentence linking the proxy README.

## Test status

`go build ./... && go vet ./... && go test ./...`: see the latest commit message.

## GLM handoff log (2026-10-02, glm-5.3 as glm-proxy)

- 08:10 CDT: took over the lane. Audited run-1 state: 952 requests in `.ri/proxy-run-1`, 567 answered
  (561 decided: 246 accept / 67 correct / 21 reject / 200 need-more-evidence / 27 defer; 6 refused at
  verdict stage — the 5 prose-only corrections and 1 >400-char statement already listed below), 7
  `.failed` markers all reading "session limit · resets 10:50am (America/Chicago)". 385 items remain
  (378 never called + 7 session-limit). `go build/vet/test` re-run to confirm the branch is green.
- Plan: the account resets at 10:50 CDT; a scheduled step then probes the limit with one cheap call and,
  if open, resumes `scripts/proxy-review.sh` (same arguments) after deleting the 7 session-limit
  `.failed` markers and the `STOP` file. If the probe still fails, it reschedules ~30 min later. After
  the run: step-6 REPORT.md via `ri knowledge proxy-report`, package requests/responses, commit.
- 08:20 CDT: prompt v2 (`internal/proxyreview/prompt.go`): the decision rules now state the correction's hard
  limits (statement ≤400 chars, consequence statement/remediation ≤600, aspect reason ≤400, ≤12 citations),
  the exact checks `semantic.ProposalFromAnswer` refuses on; `PromptVersion` is `proxy-review/v2`. Run-1 is
  unaffected: its requests are v1 files on disk, and `Decision` re-hashes the request file's own prompt, not
  current code, so the remaining v1 responses still record. The shadow pass will build v2 prompts.
  Deliberately NOT added: any text about no-op corrections — that is the open contract question below, and
  embedding current recorder behaviour in the prompt would bias it before the commander decides.
- Uncertain: nothing new. The two open contract questions (prose-only corrections; Provenance.Provider)
  remain with the commander. Step 5 (shadow) still waits for the commander.

## Run-1: DONE (2026-10-02 11:05 CDT): see [proxy/run-1/REPORT.md](../proxy/run-1/REPORT.md)

- 952 items. 940 proxy decisions recorded (accept 400, correct 121, reject 42, need-more-evidence 323, defer 54).
  12 verdicts were refused at recording; those items stay pending and were not retried. 96 proxy-level facts.
- The 7 session-limit call failures were retried after the reset and decided. Those calls had never reached the
  model, so the retry was not blind. The ledger keeps both lines.
- Cost $60.10 CLI-reported; about 34 min of active calling at 4 in parallel.
- I reviewed the GLM handoff work (prompt v2, the stated correction limits, the test) and kept it: the limits
  match `semantic.checkLengths`, and run-1 kept its v1 requests on disk.
- Open for the commander:
  - **Prose-only corrections** account for 11 of the 12 refusals. Should a consequence statement/remediation
    correction with an unchanged digest be allowed?
  - `Provenance.Provider`.
  - 3 proxy facts with an action-eligible consequence. They are capped at REVIEW by the ladder; a human look is
    suggested.
- Step 5 (shadow pass on the high items, in a copy at `proxy-shadow/`) **waits for the commander**: "the product
  owner has finished the high items".

## GLM handoff log — takeover 2 (2026-10-02 18:23 CDT, glm-5.3 as glm-proxy)

- Took over after the Claude agent paused for another usage-limit window (lane lead held since 18:21).
- Audited run-2 state: the paused agent had launched it at 18:04 CDT over the 886 pending non-high items
  (the backlog after the semantic-3 / loop merges). 478 answered: **471 decided** (163 accept / 42 correct /
  32 reject / 214 need-more-evidence / 20 defer), 7 verdict refusals (6 "correction changes no aspect",
  1 "an answer that asserts something must cite evidence" — the known open contract question, items stay
  pending). 5 calls failed with "session limit · resets 8:50pm (America/Chicago)" (never reached the
  model); the watchdog touched STOP and the runner exited cleanly. 403 items were never called.
  The 499 modified store files and the ledger were uncommitted.
- Committed the paused agent's uncommitted shadow-pass prep (`scripts/proxy-shadow-merge.py`,
  `internal/knowledge/integrity_shadow_test.go`) unchanged after `go build ./... && go vet ./... &&
  go test ./...` passed; then committed the run-2 WIP store + ledger.
- Plan: a scheduled step at ~20:52 CDT probes the limit with one cheap call; if open it rotates the run
  log (so the paused session's stale watchdog, which counts cumulative FAILs, cannot insta-STOP the
  resume), deletes the 5 session-limit `.failed` markers and the STOP file, and resumes
  `scripts/proxy-review.sh .ri/proxy-run-2 docs/phase3/learning-loop/proxy/run-2/ledger.jsonl 4` with a
  fresh 3-new-failures watchdog. If the probe is still closed it reschedules ~30 min later. After the
  run: proxy-report + metrics + REPORT.md + packaging like run-1, then commit with provenance.
- Uncertain: nothing new. Open items unchanged: prose-only corrections (now 6 more in run-2, total 17
  refused-at-recording items pending), `Provenance.Provider`, step 5 waits for the commander.

## proxy-2 (branch `p3ll/proxy-2`, commander order 2026-10-02): Claude resumed 20:52 CDT

- I reviewed GLM takeover 2: the merge script and integrity test are committed unchanged, and the run-2 WIP (471
  decisions plus the ledger) is committed. Kept. Two corrections to its log:
  - **Step 5 is no longer waiting.** The commander ordered the shadow pass (proxy-2 brief: run the non-high items in
    the real store, shadow the high items in a COPY at `proxy-shadow/knowledge/`, build a merged
    `proxy-shadow/eval-knowledge/` view labelled proxy-incl-shadow, and report).
  - **The "must cite evidence" refusal is a runner gap, not the prose-only contract question.** The proxy cited only
    validator artifact evidence. `semantic.ProposalFromAnswer` needs a candidate (upstream) citation, and the
    runner keeps only those. Fix for a v3 prompt: tell the proxy that a correction must cite at least one UPSTREAM
    EVIDENCE id. Citations are not invented.
- Run-2 resumed at 20:52 after the 8:50pm reset: the 5 session-limit markers were cleared (those calls never reached
  the model) and the log rotated to `proxy-review.log.1`.
- **proxy-2 DONE (2026-10-02 ~22:30 CDT)**; see [proxy-shadow/REPORT.md](../proxy-shadow/REPORT.md).
  - Run-2: 866 decisions in the real store, 20 refused, $59.07.
  - Shadow pass: 538 decisions in the copy, 10 + 2 refused, 87 new proxy facts, $46.76. The real store's 550 high
    items are untouched.
  - Eval view: `proxy-shadow/eval-knowledge/` (proxy-incl-shadow, 265 facts).
  - New: `ri knowledge proxy-agreement` (human vs shadow; 0 overlap today);
    `internal/knowledge/integrity_shadow_test.go` (blind-authoring scan of both shadow stores).
  - New knowledge-lane finding: composition proposes facts the domain rejects (migration-required on a gvk subject).
  - Still open for the commander: prose-only corrections; `Provenance.Provider`.
  - Later: proxy-vs-human agreement once the product owner finishes.

## GLM handoff log — takeover 3 (2026-10-02 ~22:40 CDT, glm-5.3 as glm-proxy)

- Took over with the lane lead (held since 22:06). Audit: working tree clean at 258d9490; proxy-2
  already complete and committed (run-2 866 decisions in the real store, shadow pass 538 in the copy,
  eval view, `proxy-agreement`, integrity test). `.ri/proxy-run-2` and `.ri/proxy-shadow` hold no STOP
  file and no `.failed` markers — nothing to resume, nothing scheduled against a reset window.
- The one identified-but-unbuilt item was the v3 prompt fix the proxy-2 notes prescribe for the
  "corrections citing only validator evidence" refusals. Built it (commit 30504441): the `correct`
  action now says a correction's citations must include at least one id from the EVIDENCE (upstream)
  section (validation-only ids do not count; ids are copied exactly, never invented). This matches the
  recorder exactly: `correctedAssertion` keeps only candidate ids, so a validator-only citation set
  reached `ProposalFromAnswer` empty and was refused. `PromptVersion` is now `proxy-review/v3`.
  No model calls were made and no recorded decision changes: decisions re-hash the request file's own
  on-disk prompt, and run-1/v2, run-2 and the shadow requests are files on disk.
- Verified `go build ./... && go vet ./... && go test ./...` pass, and read-only
  `ri knowledge metrics` (178 active facts: consensus 31, proxy 147 — matches the reports).
- Numbers for the v3 decision: of run-2's 20 recording refusals, 4 were validator-only citations
  (v3-recoverable); the other 16 and all 10 shadow refusals are prose-only corrections, the open
  contract question.
- Uncertain: nothing new. Open items unchanged and with the commander: prose-only corrections,
  `Provenance.Provider`, the v3 re-run order above; proxy-agreement waits for the product owner.

## proxy-3a (branch `p3ll/proxy-3`, commander order 2026-10-03): DONE

- I reviewed GLM takeover 3: its v3 citation rule is correct (it matches `correctedAssertion`, which keeps only
  candidate ids). Cherry-picked onto `p3ll/proxy-3`.
- **Blindness:** `LOOP-DIAGNOSIS-2.md` analyses eval links ("unhit links"), so I did not read it. L2 was built from
  the commander's description.
- **Prompt v3:** the upstream section context from the ingested release store (cited section + up to 2
  upgrade-guide sections that mention the subject, bounded), plus the citation rule. Blindness is unchanged.
- **Why wording-only corrections were still refused after contract-5:** the cause is in the proxy's own decide
  path. `proxyreview.correctedAssertion` refused every correction with unchanged aspect digests ("the correction
  changes no aspect") *before* the domain's contract-5 check ran. Even past that, it would have labelled them with
  wrong-* labels instead of exactly `[corrected, improved-statement]`.
  - Fixed: the pre-check now mirrors the domain (it refuses only when neither a digest nor the consequence prose
    changed), and prose-only corrections get the contract-5 labels.
  - Verified by re-recording the 16 refused run-2 prose-only verdicts against a scratch copy (no model calls):
    **11 now record**. The other 5 are a second bug, below.
- **Second bug (mine, fixed):** the prompt printed JSON-encoded literals raw (`"\"false\""`). The proxy then
  "corrected" phantom quote characters, which is a no-op (those 5) or a spurious wrong-* label. About 41 v1/v2
  verdicts mention it (≈39 corrections, 2 rejects). The recorded corrected values are canonical, so facts are
  not corrupted, but the wrong-* label counts are inflated by roughly that much. v3 shows the literals decoded.
- **Smoke** (10 items v2 closed as need-more-evidence): v3 gives 9 need-more-evidence and 1 correct. See
  `proxy/v3-smoke/SMOKE.md`. Most closes are one-line changelog/PR titles whose facts live in the un-ingested
  PR: that's a capture lever.
- **Not done (waits for the commander):**
  - the full v3 re-review after semantic-4;
  - optionally re-recording the 11 now-recordable run-2 verdicts in the real store (no model calls; those items
    are still pending and would otherwise be re-called in the re-review).
