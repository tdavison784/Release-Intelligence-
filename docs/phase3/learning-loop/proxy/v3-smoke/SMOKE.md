# Prompt proxy-review/v3: 10-item smoke on items v2 closed as need-more-evidence (2026-10-03)

The commander's lever L2 is to show the reviewer the whole upstream section around the cited note, so it
can decide instead of closing aspects for lack of evidence.

**Smoke set.** 10 items the v2 proxy closed as `need-more-evidence` in run-2: 2 semantic-mapping,
2 applicability, 2 consequence and 4 evidence-sufficiency gates. They were picked deterministically (every 7th
item by id per question type) and are listed in `items.txt`.

**Method.** The smoke ran on a scratch copy of the store, with only these 10 items set back to `pending`. The
real store was not touched, and none of these decisions is committed to any store. It made one stateless
`claude -p --model claude-opus-5-5` call per item with the v3 prompt; the section context came from the
ingested release store (`-state`).

## Result: decision mix vs v2 on the same items

| | v2 (run-2) | v3 |
|---|---|---|
| need-more-evidence | 10 | **9** |
| correct | 0 | **1** |

- **The one change, `ri-03a8964c5a18`** (traefik applicability, HTTP/3). The cited changelog line only says
  "Moves HTTP/3 outside the experimental section". v3 matched an upgrade-guide section on the term `http3`
  ("the `experimental.http3` option has been removed … setting it would prevent Traefik from starting"), and the
  proxy corrected the exposure condition to environments that still set `experimental.http3`. This is
  exactly the L2 effect: the upgrade guide holds the decision that the changelog line lacks.
- **`ri-6e9401d25073`** (crossplane semantic-mapping) stayed need-more-evidence. But the section context let
  the proxy identify the real subject: the removed feature is `spec.mode: Resources` in Compositions, replaced
  by functions. It then said the proposed `protocol-behavior` subject is wrong. A correction would have been
  the better action, and a prompt nudge toward correct over need-more-evidence when the section names the
  subject is worth considering.
- **The other 8** are one-line changelog or PR titles ("remove Upbound code from code base", "fix diff from
  earthly +reviewable", "Support gRPC healthcheck"…). Their section is a changelog list of unrelated entries,
  and the facts the reviewer needs (which key, which default, what breaks) are in the linked PR, which is not
  ingested. **For these, more reviewer context cannot help. The lever is capture, ingesting the PR
  description or diff for one-line entries, or accepting that these stay closed.**

## Cost

$0.83 for 10 calls, $0.083 per call (v2 run-2 averaged $0.066), because the prompts carry the section
context. The median call took 7.7 s.

## Reading

On this sample v3 recovers about 10% of the v2 need-more-evidence closes, and only where an upgrade guide
covers the item. The bigger re-review gain is expected from semantic-4's new proposals, not from reviewer
context. The full re-review waits for the commander.

Files: `items.txt`, `ledger.jsonl`, `responses/`, `requests.tar.gz` (the exact v3 prompts).
