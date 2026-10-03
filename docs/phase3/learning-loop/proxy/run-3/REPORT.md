# proxy-4: prompt v3 re-review (real store), v3 shadow pass (high items, copy), proxy-incl-shadow view

Commander order, 2026-10-03, with a budget of about $150 Claude-side. Reviewer: `claude-opus-5-5`, one stateless
`claude -p` call per item, prompt **`proxy-review/v3`**:
- the upstream section context around the cited note, from the ingested release;
- literals shown decoded;
- corrections must cite upstream evidence;
- prose-only corrections are recorded.

The blindness rules are unchanged. Every decision is `reviewerKind: proxy`. The real store's **high items were
not touched**: all 637 are still pending, and a human decision always excludes an item from selection.

**Total spend: $149.47 CLI-reported, within the $150 cap.** The runner's budget guard sums every proxy-4 ledger
and stopped launching calls at $149.23; the calls already in flight finished.

| Step | Items | Recorded | Refused | Not called | Cost |
|---|---|---|---|---|---|
| 1. Re-record the refused run-2 verdicts (no model calls) | 16 | **11** `[corrected, improved-statement]` | 5 (phantom-quote no-ops, re-reviewed in step 2) | – | $0 |
| 2a. Real store, non-gate items, by status (pending / needs-evidence / deferred, non-high) | 456 | 454 | 2 | 0 | $48.43 |
| 3. Shadow copy, ALL high items (old + new) | 637 | 634 | 3 | 0 | $65.38 |
| 2b. Real store, evidence-sufficiency gates (lowest value, run last) | 690 | 512 | 0 | **178** (budget) | $35.66 |

The 178 gates not called are listed in `gates-not-called.txt`: 120 needs-evidence, 38 deferred, 20 pending.
They are low-priority evidence-sufficiency items, and they stay as they were.

## Decision mix vs v2

**2a, real store, by the item's status when re-reviewed** (for closed items, v2's verdict is that status):

| Before (v2) | n | v3 accept | correct | reject | need-more-evidence | defer | refused |
|---|---|---|---|---|---|---|---|
| needs-evidence (v2 closed it) | 219 | 40 | 27 | 15 | 136 | – | 1 |
| deferred | 3 | – | 2 | 1 | – | – | – |
| pending (new semantic-4 items, earlier refusals) | 234 | 126 | 10 | 13 | 79 | 5 | 1 |

**v3 decided 82 of the 219 items v2 had closed as need-more-evidence (37%).**

**2b, gates:**

| Before (v2) | n called | v3 "evidence sufficient" (recorded as defer) | v3 need-more-evidence |
|---|---|---|---|
| needs-evidence | 383 | 36 | 347 |
| deferred (v2: sufficient) | 60 | 54 | 6 |
| pending | 69 | 62 | 7 |

v3 judged 36 of v2's insufficient gates sufficient, and 6 of v2's sufficient ones insufficient.

**3, shadow: the same 522 high items decided by both v2 (proxy-2) and v3.** The 112 high items new since
proxy-2 got v3 only.

| v2 \ v3 | accept | correct | reject | need-more-evidence | defer |
|---|---|---|---|---|---|
| accept | 218 | 19 | 1 | 8 | 1 |
| correct | 18 | 111 | 10 | 9 | 1 |
| reject | 0 | 2 | 21 | 6 | 1 |
| need-more-evidence | 12 | 27 | 4 | 51 | 0 |
| defer | 1 | 1 | 0 | 0 | 0 |

- need-more-evidence fell from 94 to 74.
- 43 of v2's 94 closes were decided (12 accept, 27 correct, 4 reject), while 23 items v2 had decided moved to
  need-more-evidence.
- Same action on 401 of 522 (77%). This mixes the prompt change with run-to-run variation, and it is the best
  available measure of proxy self-consistency.

All 634 shadow decisions: accept 313 · correct 186 · reject 44 · need-more-evidence 87 · defer 4.

## Facts minted

New facts relative to the branch base (`p3-learning-loop` at `2427d15f`):

| | new facts | action-eligible | review-required | informational |
|---|---|---|---|---|
| real store (`knowledge/`), proxy level | **32** | 2 | 22 | 8 |
| eval view (real + shadow), proxy level | **158** | 93 | 53 | 12 |

All are proxy facts. **The trust ladder caps them at REVIEW**, so none can produce ACTION REQUIRED.

**Active facts:**

| Store | Active facts | consensus | proxy |
|---|---|---|---|
| real store | 216 | 36 | 180 |
| `proxy-shadow/eval-knowledge` (proxy-incl-shadow) | 339 | 33 | 306 |

Proxy-2's eval view had 265.

## Eval view (step 4)

`proxy-shadow/eval-knowledge/` was rebuilt with `scripts/proxy-shadow-merge.py`. The shadow was refreshed from
the real store after step 2a (`BASE`, `base-manifest.txt`). The merge:
- took 512 gate files from the real store;
- took 637 changed files from the shadow and added 761 new ones;
- found 13,799 identical;
- had **0 conflicts**.

`LABEL.md` says to evaluate it at `-min-verification proxy`, report it as **proxy-incl-shadow**, and never
present it as human-verified.

**Note:** the view has 3 fewer consensus facts than the real store. In the shadow the proxy *corrected* 3
audit items of auto-approved consensus facts (flux `vf-d2375ef40b74` and `vf-6703cd4a7629`, karpenter
`vf-cec2d4a87881`). By the queue's rules a correction of a fact-review item supersedes the fact, so in the
view these are superseded by proxy-level facts. The queue allows it, because consensus is not a trusted level.
It is the proxy auditing auto-approvals, which is the product owner's job in the real store: worth a look.

## Findings

1. **Duplicate verdicts naming another release's fact (3 shadow refusals).** The proxy marked a 2.0.0 candidate
   as a duplicate of an endpoint fact (release ""), and the queue refuses a release mismatch. Fixed for the next
   run in **prompt v4**: `duplicateOf` offers only facts of the candidate's release. Other related facts are
   still shown as context. Not used in this run.
2. **The citation rule was not always followed (1).** `ri-6f506c485cf5` cited only validator and section ids
   despite v3's rule; it was refused, not invented around. The other 2a refusal was an invalid correction (a
   `crd-field` subject without a group).
3. **Need-more-evidence closes that remain.** These are mostly one-line changelog or PR titles whose facts live
   in the PR body, which is not ingested (see `../v3-smoke/SMOKE.md`). That is a capture lever.
4. **Cost per call:** v3 averaged $0.10–0.11 on items ($0.106 real store, $0.103 shadow) and $0.070 on gates. The section context
   costs about 30% more per call than v2.

Files:
- here: `ledger.jsonl`, `responses/`, `requests.tar.gz` (2a); `gates-ledger.jsonl`, `gates-responses/`,
  `gates-requests.tar.gz`, `gates-not-called.txt` (2b); `../run-2/rerecord-ledger.jsonl` (step 1);
- shadow: `../../proxy-shadow/ledger-v3.jsonl`, `responses-v3/`, `requests-v3.tar.gz`.
