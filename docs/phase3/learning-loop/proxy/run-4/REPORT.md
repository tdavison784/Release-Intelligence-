# proxy-5: prompt v4, the leftover gates (5a), then linked-PR re-review (5b, after prtext)

Prompt **`proxy-review/v4`** changes two things from v3:
- `duplicateOf` offers only facts of the candidate's own release (the proxy-4 finding: 3 shadow verdicts named an
  endpoint fact);
- **linked-PR evidence** gets its own block, LINKED PULL REQUESTS, when the candidate has any. Those records are
  citable and valid for corrections, because they are candidate evidence. Nothing is rendered when absent.
  `proxyreview.LinkedPR` is the one predicate: kind `linked-pr`, or a document whose URI is a GitHub pull request.
  If the prtext lane's final shape differs, the change goes there.

Everything else is as v3: the blindness rules, the upstream section context, and the decoded literals.
`ri knowledge proxy-prompt -linked-pr-only` selects only items whose candidate has linked-PR evidence. Proxy-5 has
one budget guard, ~$100, over `gates-ledger.jsonl`, `ledger.jsonl` and `shadow-ledger.jsonl`.

## 5a: the 178 evidence-sufficiency gates proxy-4's budget left un-called (real store, non-high)

All 178 were still in the status proxy-4 left them (low priority) and were re-prompted with v4. **178 decided, 0
refused, 0 failed, $11.96.**

| Before | n | v4 "evidence sufficient" (recorded as defer) | v4 need-more-evidence | defer |
|---|---|---|---|---|
| needs-evidence (v2 closed it) | 120 | 12 | 108 | – |
| deferred (v2: sufficient) | 38 | 35 | 2 | 1 |
| pending | 20 | 17 | 3 | – |

Files: `gates-ledger.jsonl`, `gates-responses/`, `gates-requests.tar.gz`.

## 5b: v4 re-review with linked PR/issue/commit evidence (after prtext)

prtext merged (e59e7ab7); `proxyreview.LinkedPR` now matches its evidence kinds (`linked-pr`: PRs and issues;
`linked-commit`), and the shadow copy was rebased onto the real store at 7b493755 before any call. Selection:
every item in status **needs-evidence** whose candidate carries linked evidence (`proxy-prompt -linked-pr-only
-status needs-evidence`), non-high in the real store, high in the shadow copy. Reviewer claude-opus-5-5, one
stateless call per item, all requests prompt `proxy-review/v4`. **534 decided, 0 refused, 0 call failures,
$50.63.**

### Real store (non-high): 522 items

| Before (all needs-evidence) | n | v4 decision |
|---|---|---|
| evidence-sufficiency (gates) | 363 | defer 209 ("evidence sufficient") · need-more-evidence 154 |
| consequence | 64 | accept 31 · correct 21 · need-more-evidence 12 |
| applicability | 56 | need-more-evidence 28 · correct 13 · accept 7 · reject 8 |
| semantic-mapping | 39 | need-more-evidence 13 · accept 12 · correct 9 · reject 5 |
| **total** | **522** | **accept 50 · correct 43 · reject 13 · need-more-evidence 207 · defer 209** |

- **106 of the 159 non-gate items previously stuck on need-more-evidence got decided** (67%): the linked PR/issue
  bodies carried the missing upstream statement. 22 new proxy-level facts (14 review-required, 8 informational;
  2 with a deterministic-verified aspect). $49.19, median 8.4 s, 25 min wall clock at 4 parallel.
- Gates: linked evidence made the proxy call 209 of 363 sufficient — the remaining 154 gaps are real capture
  gaps (un-ingested PR bodies), not prompt failures.
- Self-review bias (this run only): every item was proposed by claude-family models (341 claude-only, 180
  claude+glm, 1 no proposal). Self-model items (opus among the authors, n=186): accept 41 · correct 24 ·
  reject 11 · nme 71 · defer 39. Self-family only (n=335, mostly gates): accept 9 · correct 19 · reject 2 ·
  nme 135 · defer 170. No other-family items exist in this selection, so the split carries no signal this run.

### Shadow copy (high items): 12 items

All 12 high items in needs-evidence with linked evidence: **correct 4 · reject 3 · need-more-evidence 5**,
$1.44. 2 new proxy facts, both action-eligible — **capped at REVIEW by the trust ladder**, as every proxy fact
is. The real store's high items remain untouched, still awaiting the product owner.

### Eval view

`scripts/proxy-shadow-merge.py --base-git=7b493755` over the post-5b stores: 315 real, 636 shadow, 14,291
identical, 544 real-only, 775 shadow-only, **0 conflicts**, 0 merged field-wise. **363 active facts (consensus
33, proxy 330)** against 339 at proxy-4.

Files: `ledger.jsonl`, `responses/`, `requests.tar.gz` (real); `shadow-ledger.jsonl`;
`../proxy-shadow/responses-v4/`, `../proxy-shadow/requests-v4.tar.gz`.

**Proxy-5 spend: $62.59 of ~$100** ($11.96 gates + $49.19 real + $1.44 shadow).
