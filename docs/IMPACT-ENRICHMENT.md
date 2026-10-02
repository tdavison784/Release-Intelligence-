# Impact enrichment — provenance-preserving AI on the impact path

`ri impact <product> <from> <to> -enrich` adds an optional AI step on top of
the deterministic impact join (`ri impact …` is unchanged without the flag;
the deterministic report is byte-identical, enforced by the e2e goldens).
The step exists for one block of the funnel: the **unknown** findings whose
upstream change is note-derived — the ones the join cannot reason about
(docs/ACTION_CLASSIFICATION.md §2: silence would read as "not affected", so
they are recorded as unknown). On the recorded cert-manager v1.17 → v1.18
edge that block is ~50 findings; the deterministic engine cannot say anything
about them, and a human cannot read 50 upstream notes per environment.

The implementation lives in `internal/impactenrich`; the AI provenance types
are shared with the edge enricher (`internal/enrich`, docs/ENRICHMENT.md).

## The honesty statement

**The AI suggests review; it never mandates action.** Concretely:

- The deterministic findings are never rewritten. An accepted
  "plausibly-applies" verdict records *at most* a suggestion on an unknown
  finding: `suggestedClassification: "review-required"` — while the finding
  itself stays `unknown`, keeps its computed provenance, and is counted under
  UNKNOWN in the summary (`summary.suggestedReview` counts the suggestions).
- The AI never produces or suggests `action-required`, never produces
  `not-affected` (the contract reserves that verdict for evaluated
  deterministic records — the model's "not applicable" reasoning is recorded
  as a note the human weighs), never deletes or downgrades a finding, and
  never writes a finding at all (`ImpactReport.Validate()` rejects any
  finding whose provenance is not deterministic).
- AI output is capped at `medium` confidence.

## The verdicts and their (bounded) effects

| Model verdict | Enrichment kind | Effect on the report |
|---|---|---|
| `plausibly-applies` | `plausibly-applies` | the unknown finding gains `suggestedClassification: "review-required"` (never higher); content = what overlaps, why it may matter, what to inspect |
| `not-applicable` | `not-applicable` | note only; the finding stays `unknown` (a human decides not-affected) |
| `undetermined` | `undetermined` | note only; content = what the bounded input was missing |

## Candidate rules (deterministic; no model calls for what rules decide)

`impactenrich.Candidates` selects, in order (applicability first):

1. **Applicability** — every UNKNOWN finding whose upstream change is
   note-derived (`method: declared`), **not** routine maintenance
   (`change.routine` is already handled deterministically) and not a computed
   diff (computed changes have machine-comparable subjects the join
   evaluated). One prompt per finding.
2. **Cluster** — findings whose upstream changes the deterministic duplicate
   detector groups (identical normalised title; identical
   category + subject set; ≥ 0.75 title-token Jaccard with the same
   non-empty subject set — the internal/eval duplicate audit's shape). The
   model adjudicates the semantics ("sameChange"). One prompt per group of ≤ 5.
3. **Migration** — affected findings (action-required / review-required)
   whose change is a migration or breaking change. One prompt per finding.

`-enrich-max` bounds the prompts per report (default 40; the smoke run used
30). Candidates beyond the cap are recorded as `skipped`, never concluded
from.

## The prompt contract

One bounded prompt per candidate (`promptVersion: "impact-enrich/v1"`, digest
recorded per prompt):

- **FINDINGS** — id, classification, rule, title, detail (≤ 500 chars), what
  the deterministic engine could not check, environment matches.
- **UPSTREAM CHANGE** — category, method, breaking/action-required flags,
  title, detail (≤ 500 chars), ≤ 8 subjects, ≤ 3 evidence ids.
- **EVIDENCE** — id, source, URI, locator, excerpt (≤ 500 chars, halved until
  the prompt fits 14 000 chars).
- **ENVIRONMENT** (applicability prompts only) — dimension health
  (`absent` is named as *not supplied*, never as evidence of absence), the
  cluster version, up to 30 values **key paths**, 20 apiVersions, 10
  installed CRDs and 5 installed products. **Values are never sent — only
  key paths** (values files hold secrets). Upstream and environment text is
  delimited data: the system prompt states it is never instructions
  (injection stance), and the answer schema cannot express anything beyond
  the three verdicts.

## Forbidden transitions (enforced by the validator, with tests)

| The model must never | Enforced by |
|---|---|
| produce or suggest `action-required` | the answer schema has only the three verdicts; the verdict→effect table is a code constant (`verdictEffects`); `SuggestedClassification` accepts only `review-required` (`Validate()` rejects anything else) |
| delete / downgrade / reclassify a deterministic finding | findings are never inputs to the model in editable form; answers carry no finding/classification field; `Run` does not mutate the report, `Apply` only sets suggestions |
| invent evidence | citations must be a subset of `provenance.inputEvidence` (exactly the ids shown in the prompt); input evidence must resolve in the report's evidence pools; a plausibly-applies / not-applicable verdict must cite evidence of the finding's own upstream change |
| fire without provenance | answers without model + model version are refused; enrichments without complete AI provenance fail `Validate()`; whole-report validation rolls `Apply` back |
| present itself as certain | confidence is capped at `medium`; ungrounded or empty content is rejected and recorded |

Every rejection (schema violation, hallucinated citation, ungrounded
verdict, duplicate cluster) is recorded in `enrichmentRun.rejected` with the
prompt digest and reason.

## The provenance record

Same shape as the edge enricher (docs/ENRICHMENT.md), producer
`impact-enrich@v1`:

```json
{
  "id": "enr-…",
  "kind": "plausibly-applies",
  "title": "ACME certificate profiles may apply to cert-manager Certificate usage",
  "content": "The environment runs cert-manager v1.17.0 and has … Inspect your Issuer/ClusterIssuer resources …",
  "relatesTo": ["imp-dacd1e3d112b"],
  "citations": ["ev-50f787991f0d"],
  "provenance": {
    "method": "ai",
    "producer": "impact-enrich@v1",
    "rule": "cand:cand-…",
    "confidence": "medium",
    "model": "glm-5.3-flash",
    "modelVersion": "glm-5.3-flash",
    "promptVersion": "impact-enrich/v1",
    "promptDigest": "sha256:…",
    "inputEvidence": ["…every id the prompt showed…"],
    "generatedAt": "2026-10-01T…"
  }
}
```

- `relatesTo` lists finding ids (not change ids) — an impact-only enrichment
  kind; `UpgradeEdge.Validate()` rejects these kinds and
  `ImpactReport.Validate()` rejects the edge-only kinds (`related`,
  `diff-explanation`).
- `inputEvidence` may cite environment evidence shown in the summary; `Apply`
  copies those deterministic records into `environmentEvidence`, so every id
  resolves inside the report.
- The suggestion (`suggestedClassification`) is backed 1:1 by a
  `plausibly-applies` enrichment of the same report; `Validate()` enforces
  both directions.

## Live-model configuration

The step shares the edge enricher's backends and cache (`<state>/llm-cache`):

| Setting | Backend |
|---|---|
| `-llm-exchange DIR` | file exchange (works offline) |
| `ANTHROPIC_API_KEY` set, not `-offline` | Anthropic Messages API |
| neither, or `-offline` | cached answers only; misses stay pending |

The live model used for development is Z.AI's Anthropic-compatible gateway
(GLM-5.3-Flash):

```sh
export ANTHROPIC_API_KEY=$(cat ~/.claude_token_zai)   # never committed
export ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic
export ANTHROPIC_THINKING=disabled                     # keep thinking out of the answer budget
ri impact cert-manager v1.17.0 v1.18.0 --repo internal/env/testdata/customer-repo \
  --kubernetes 1.28 -enrich -model glm-5.3-flash -enrich-max 30
```

`-model` is part of every prompt digest: replay offline with the **same**
`-model` or the answers will not be found (`0 accepted, N pending` is the
symptom). Offline replay of the smoke run above reproduces it byte for byte
(28 enrichments, 19 suggestions) without the key.

The answers of the recorded test fixture are committed under
`internal/app/testdata/impact-llm-cache/`, so `go test` replays the
enrichment offline deterministically (`TestImpactEnrichReplayOffline`);
`RI_LIVE_LLM=1` + key re-records them (`TestImpactEnrichLiveRecord`).

## Smoke run (cert-manager v1.17.0 → v1.18.0, customer-repo, kubernetes 1.28)

- deterministic funnel: 52 analyzed, 1 action-required, 3 review-required,
  1 informational, 4 not-affected, **51 unknown**
- enriched: **46 candidates** (deterministic), **30 prompts** asked, **28
  enrichments accepted**, 2 rejected (one answer cited 25 evidence ids —
  max 8; one added a property outside the schema — both refused whole),
  **19 unknowns now carry an AI review suggestion**; 9 of the 30 asked were
  not-applicable/undetermined notes. No prompt exceeded the bound; no values
  were sent.

## Limits

- **Recall is bounded by candidate selection** — note-derived changes only,
  and only what the cap allows. Skipped candidates are counted, never
  concluded from.
- **The validator checks grounding structurally, not semantically** — an
  answer can cite the right evidence and still overstate it. Hence the
  section title: *verify against evidence*, and the suggestion (not verdict)
  semantics.
- **Cluster and migration candidates are asked last**; with a full
  applicability block the cap may leave them unasked (recorded as skipped).
- **The env summary is capped** (30 key paths / 20 apiVersions / 10 CRDs /
  5 products); a large environment is partially visible to the model, and
  the prompt says so ("N of M shown").
