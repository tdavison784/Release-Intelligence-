# proxy-2: run-2 (real store, non-high) and the shadow pass (high items, in a copy)

Commander order, 2026-10-02. Reviewer: `claude-opus-5-5`, one stateless `claude -p` call per item, prompt
`proxy-review/v2`, with the same blindness as run-1:
- no environment, no model names, no eval data;
- no proxy decisions and no proxy-level facts in the prompt;
- shadow prompts also leave out every human decision (`-no-human-context`).

Every decision is `reviewerKind: proxy`.

## 1. Run-2: pending non-high items in the REAL store (`../proxy/run-2/`)

| | n |
|---|---|
| items (mostly the 7 new products) | 886 |
| decisions recorded | **866**: accept 300 · correct 75 · reject 45 · need-more-evidence 399 · defer 47 |
| verdicts refused at recording (item stays pending) | 20: 16 prose-only corrections, 4 corrections citing only validator evidence |
| session-limit call failures | 5 (never reached the model; decided after the reset) |
| new proxy-level facts in the real store | 51 (proxy facts 96 → 147) |
| cost | **$59.07** CLI-reported; median call 6.4 s |

Per question type (decided):

| Question type | Decisions |
|---|---|
| semantic-mapping | accept 95, correct 25, reject 25, need-more-evidence 26, defer 1 |
| applicability | accept 47, correct 35, reject 20, need-more-evidence 38 |
| consequence | accept 151, correct 15, need-more-evidence 44 |
| classification | accept 7, need-more-evidence 4 |
| evidence-sufficiency | need-more-evidence 287, defer 46 ("evidence sufficient") |

## 2. Shadow pass: every pending HIGH item, in the copy `knowledge/` (this directory)

The copy was taken after run-2 at base commit `9e3fe9ca` (`BASE`, with a sha256 per file in
`base-manifest.txt`). **The real store's high items were not touched**: all 550 are still pending, and the run
made no change under `knowledge/`.

| | n |
|---|---|
| items (old + new products) | 550 |
| shadow decisions recorded | **538**: accept 254 · correct 156 · reject 32 · need-more-evidence 94 · defer 2 |
| verdicts refused | 10, all prose-only corrections |
| decisions the queue refused at recording | 2 (see finding 3) |
| new proxy-level facts in the shadow | **87** (65 action-eligible · 18 review-required · 4 informational) |
| cost | **$46.76** CLI-reported; median call 8.6 s; 26 min wall clock |

Per question type:

| Question type | Decisions |
|---|---|
| semantic-mapping | accept 99, correct 43, reject 9, need-more-evidence 12, defer 2 |
| applicability | accept 64, correct 78, reject 10, need-more-evidence 26 |
| consequence | accept 89, correct 35, reject 13, need-more-evidence 56 |
| classification | accept 2 |

High items are the high-impact ones, so most new shadow facts carry an action-eligible consequence. **As proxy
facts, the trust ladder caps them at REVIEW.** None of them can produce ACTION REQUIRED.

Files here: `ledger.jsonl`, `responses/`, `requests.tar.gz`, `proxy-report.txt` (counts every proxy decision in
the shadow store; the tables above are shadow-only, taken from the ledger).

## 3. Evaluation view: `eval-knowledge/`, labelled **proxy-incl-shadow**

`scripts/proxy-shadow-merge.py base-manifest.txt knowledge proxy-shadow/knowledge proxy-shadow/eval-knowledge`
does a per-file three-way merge: real vs shadow vs the base, and **the real store wins every conflict**.
This build:
- 538 files taken from the shadow and 626 added by the shadow;
- 11,011 files identical;
- 0 conflicts (the real store has not changed since the copy).

The view holds 265 active facts (consensus 31, proxy 234), against 178 in the real store. `LABEL.md` at its top
says what it is. Evaluate it at `-min-verification proxy`, report it as proxy-incl-shadow, and never present it
as human-verified. Re-run the merge whenever the real store changes; human decisions always win.

## 4. Proxy vs human (later)

`ri knowledge proxy-agreement -shadow docs/phase3/learning-loop/proxy-shadow/knowledge` compares the product
owner's decisions in the real store with these shadow decisions on the same items. It reports:
- same action and same coarse outcome (verify | reject | open);
- results per question type;
- per aspect where both verified;
- a confusion matrix.

Today: **0 items decided by both** (538 shadow-only). Run it when the product owner has finished.

## Findings

1. **Prose-only corrections** account for 26 more refusals in proxy-2 (16 + 10), 37 in all since run-1. This is
   still the open contract question: should a correction that changes only the consequence statement or
   remediation be recordable?
2. **Corrections citing only validator evidence (4).** `semantic.ProposalFromAnswer` needs at least one upstream
   (candidate) citation. Fix for a v3 prompt: say so. Citations are never invented to get past the check.
3. **Composition produced invalid facts (2, knowledge lane).** In `rd-ad6c79da2e14` and `rd-cbaca99991df`, the
   proxy's accept would have minted a fact combining change `migration-required` with a `gvk` subject. The domain
   forbids that combination (`migration-required` goes with the `migration` family). The aspects came from
   different items of the same candidate, so routing proposes items whose combination cannot validate. The queue
   refused the decisions, the items stay pending in the shadow, and nothing was written.
4. **Cost (proxy-2 total): $105.83** ($59.07 + $46.76). Run-1 cost $60.10, so the lane total is about $166.
