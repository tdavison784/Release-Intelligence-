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

## 5b: waiting for the commander's note that prtext is merged

Then: v4 re-review of every item closed as need-more-evidence whose candidate gained linked-PR evidence, in the real
store (non-high) and in the shadow copy (high). After that, rebuild the proxy-incl-shadow view with `--base-git`.
Proxy-5 spend so far: $11.96 of ~$100.
