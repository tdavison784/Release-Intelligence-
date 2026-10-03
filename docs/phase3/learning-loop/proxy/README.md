# Blind AI proxy review

Product-owner decision (2026-10-02): a blind Opus proxy reviews every pending review item whose routing
priority is **not** `high`. The product owner reviews the high-priority items on the dashboard. Afterwards
the proxy shadow-reviews those high items in a **copy** of the store (`proxy-shadow/`), so proxy-vs-human
agreement can be measured without biasing the human.

Every proxy decision is recorded as `reviewerKind: proxy` with complete AI provenance. It is never presented
as human, and the trust ladder caps proxy-verified knowledge at REVIEW (DESIGN §4).

## Pipeline

```sh
go build -o bin/ri ./cmd/ri
bin/ri knowledge proxy-prompt -o <exchange-dir>            # one <item>.request.json per pending non-high item
scripts/proxy-review.sh <exchange-dir> <ledger.jsonl> 4    # one stateless claude -p call per item, then
                                                           #   ri knowledge decide -reviewer-kind proxy -proxy-response …
bin/ri knowledge proxy-report -ledger <ledger.jsonl>       # decisions, per-model outcomes, self-family split, cost
```

- **`ri knowledge proxy-prompt`** (`internal/proxyreview`) builds a self-contained prompt from
  `knowledge.ReviewContext`. It contains:
  - the question, the upstream change, and its evidence excerpts with URIs;
  - the proposed assertion, with the aspects the question verifies marked `[VERIFY]`;
  - every model proposal side by side, anonymised as P1…Pn, plus the per-aspect agreement;
  - the deterministic validation results (identical checks grouped);
  - the rendered delta, when it is decisive;
  - earlier **human** decisions and non-proxy facts about the same subject.

  It never contains an environment (an item's recorded illustration is left out), model names, any
  proxy decision or proxy fact (one proxy error must not seed the next), or anything under `eval/`.
  The system prompt is in `prompt.go`. For the aspects being verified it reuses the proposers'
  vocabulary (`semantic.Vocabulary`).
- **Verdict schema** (structured output):
  - `action`: accept | correct | reject | need-more-evidence | defer;
  - `labels`: wrong-*;
  - `reason`: must cite evidence ids;
  - `citations`: enumerated to the evidence shown;
  - `confidence`: low | medium;
  - `duplicateOf`: enumerated to the facts shown;
  - `correction`: the proposers' typed aspect shapes, enums from the domain.

  A correction goes through `semantic.ProposalFromAnswer`, so it gets the same enum and citation checks
  as a proposal. The exposed class follows from the consequence kind. Wrong-* labels are derived from
  the aspects that actually changed, plus the model's own labels that name a verified aspect.
- **Evidence-sufficiency items** verify no aspect, so they cannot be accepted. The gate verdicts are:
  - `evidence-sufficient`: recorded as `defer` with the reason prefixed "evidence sufficient". The item
    stays open for re-proposal or a human; it is not closed.
  - `need-more-evidence`
  - `defer`
- **`scripts/proxy-review.sh`** makes one call per item, following `scripts/semantic-exchange.sh`:
  `claude -p --safe-mode --model claude-opus-5-5`, no tools, thinking off, no session persistence, and a
  neutral cwd. `StartedAt`/`DecidedAt` bracket the call. The call id is the envelope's `session_id`, the
  model version is its `modelUsage` key, and the cost and duration come from the envelope.
  - Calls run in parallel. Recording is serialised, one `ri knowledge decide` per item; decisions are
    never batched.
  - A failed call leaves `<item>.failed` and a ledger line. A refused verdict keeps its response and gets
    a ledger line (stage `verdict` or `record`). An item that stopped being pending is skipped without a
    call. None of these are retried blindly. `touch <exchange-dir>/STOP` stops the run after the calls
    already in flight.
- **`ri knowledge decide -reviewer-kind proxy -proxy-request R -proxy-response S [-proxy-ledger L]`**
  records one verdict. `proxyreview.Decision` refuses any of these:
  - a response to another prompt;
  - an edited request;
  - an item whose proposed assertion changed, or that is no longer pending;
  - a response without a model version or call id;
  - a verdict outside the schema.

  Provenance:
  - `producer knowledge.proxy@v1`, `rule provider:anthropic`;
  - `model`, `modelVersion`, `promptVersion proxy-review/v1`, `promptDigest`;
  - `callId` (the CLI session), `inputEvidence` (every id shown), `generatedAt` (the call end).

  The reviewer is `proxy:<model>`.
- **Self-review bias.** Every request and ledger line records who authored the proposals under review:
  their models, their families (`domain.ModelFamily`), and whether any of them is the proxy's own model
  or family. `proxy-report` splits the outcomes in two ways:
  - by who authored the *proposed assertion*: self-model / self-family / other-family / mixed;
  - per proposing model, labelled `self-model`, `self-family` or `other-family`.

## Runs

- `run-1/`: every pending non-high item of `knowledge/` (2026-10-02). It holds `ledger.jsonl`,
  `responses/` (the verdicts with call metadata), `requests.tar.gz` (the exact prompts), `proxy-report.{txt,json}`,
  `metrics.txt` and `REPORT.md`. Prompt v1. Later runs use `proxy-review/v2`, which states the correction length limits.
  The current prompt is `proxy-review/v3`, which additionally tells the reviewer that a correction's citations
  must include at least one upstream evidence id (the recorder keeps only upstream ids; a correction citing
  only validator evidence refused).
- `run-2/`: every pending non-high item of the real store after the loop-run-3 merge (proxy-2, prompt v2).
- `../proxy-shadow/`: the **shadow pass**. The high items were decided in a COPY of the store, never the real one.
  `eval-knowledge/` is the merged evaluation view, labelled **proxy-incl-shadow**, built by
  `scripts/proxy-shadow-merge.py`. `ri knowledge proxy-agreement` compares the shadow decisions with the
  product owner's on the same items. See `../proxy-shadow/REPORT.md`.

## Prompt v3 (proxy-3)

- **Upstream section context (lever L2).** `ri knowledge proxy-prompt` loads the candidate's ingested release
  from `-state/store` and shows two things:
  - the whole section the cited note comes from: all notes of the same source and section (sub-sections
    included), with the cited ones marked ►, bounded to a window around them;
  - up to 2 upgrade-guide sections of the same release that mention a distinctive term of the subject. This
    applies only when the cited text is not itself an upgrade guide.

  Their evidence ids are citable and recorded in `inputEvidence`, and the request lists the sections shown
  (`sections`). This is release-level upstream text only: no environment, nothing from `eval/`.
  `-no-sections` turns it off.
- **Citations.** A correction must cite at least one EVIDENCE (candidate) id; section-context and validator ids
  do not count. This is the recorder's rule, now stated in the prompt.
- **Literals are shown decoded.** Condition `values` and change `before`/`after` are stored JSON-encoded. v1/v2
  printed them raw (`"\"false\""`), and the proxy "corrected" quote characters that are not there.
- **Prose-only corrections are recorded (contract-5).** A correction that changes only the consequence
  statement/remediation is labelled exactly `[corrected, improved-statement]`. v1/v2 refused all 37 such
  corrections in the proxy's own pre-check, before the domain saw them.
- Provenance now uses `Provenance.Provider` (contract-5) instead of `Rule: provider:…`.
- Smoke: `v3-smoke/SMOKE.md`.
