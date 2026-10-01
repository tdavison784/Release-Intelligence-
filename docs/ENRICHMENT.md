# Semantic enrichment (AI, provenance-preserving)

`ri upgrade <product> <from> <to> -enrich` adds AI-derived **enrichments** to
an upgrade edge. An enrichment groups, summarises, connects or explains the
edge's deterministic Changes. It never replaces or edits a source fact or a
Change. Each enrichment names the Changes it is about and the evidence it
relies on, and records which model produced it, from which exact prompt.

(The impact report has its own enrichment step with the same machinery and
stricter verdict rules: see docs/IMPACT-ENRICHMENT.md.)

The motivating case is cert-manager v1.17 → v1.18. The deterministic merge
leaves semantic duplicates and fragments:
- The `Certificate.Spec.PrivateKey.RotationPolicy` default change appears
  three times: in the upgrade guide, in a "Major Themes" section and in the
  feature list.
- The computed Helm default diff of `prometheus.servicemonitor.targetPort`
  (9402 → "http-metrics") is explained by the release note "Switched
  service/servicemon definitions to use port names".

```
$ ri -offline upgrade cert-manager v1.17.0 v1.18.0 -enrich -llm-exchange answers/
…                                        (deterministic sections, unchanged)
Enriched conclusions (AI · verify against evidence) (2):
  ◆ [cluster] Private key rotation policy now defaults to Always  (ai · batch-model batch-model-2026-09-30 · medium)
      The upgrade guide and two release-note entries state the same change: Certificate.Spec.PrivateKey.RotationPolicy now defaults to Always instead of Never.
      changes:
        chg-a726b336f865  [v1.18.0] We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`. …
        chg-0e1cdbe373a9  [v1.18.0] The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`: …
        chg-fa4ce35f9a12  [v1.18.0] The default value of `Certificate.Spec.PrivateKey.RotationPolicy` changed from `Never` to `Always`
      evidence:
        ev-9a2f8d3ec5e9  https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md  L8-L10
        ev-c92478af13c4  https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md  L109-L148 (Major Themes › …)
        ev-cf14b14073be  https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md  L338-L338 (v1.18.0 › Feature)
  ◆ [diff-explanation] ServiceMonitor now targets the named metrics port  (ai · batch-model batch-model-2026-09-30 · medium)
      The chart default of prometheus.servicemonitor.targetPort changed from 9402 to the port name http-metrics because the release switched service/servicemonitor definitions to port names.
      changes:
        chg-917796919822  [diff] Default of Helm value `prometheus.servicemonitor.targetPort` changed: 9402 → "http-metrics"
        chg-5aff8644f14a  [v1.18.0] Switched `service/servicemon` definitions to use port names instead of numbers
      evidence:
        ev-0c1eda13db98  https://github.com/cert-manager/cert-manager/blob/v1.18.0/deploy/charts/cert-manager/values.yaml  deploy/charts/cert-manager/values.yaml
        ev-0bb0d885678d  https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md  L337-L337 (v1.18.0 › Feature)
  Duplicates: 3 changes consolidated into 1 cluster (2 duplicates) · 8 candidate groups · 8 prompts · 5 pending · 1 rejected
```

(The model names above come from a hand-written exchange response used as a
stand-in. The recorded model is always whatever the answer says produced it.)

## What AI may and may not do

| AI may | AI may not |
|---|---|
| **group** Changes from different sources that state the same thing (`cluster`) | add, remove, edit or reclassify a Change, Fact or Evidence record |
| **summarise** one migration requirement from one or more Changes (`migration-summary`) | state versions, defaults, flags, steps or consequences that the cited evidence does not state |
| **explain** why a computed diff matters, citing the release-note statement (`diff-explanation`) | cite evidence it was not shown, or refer to Changes outside its input |
| **connect** Changes that might be related, as an explicit hypothesis (`related`, always `unverified`) | appear in `changes`, or produce output without model, prompt and evidence provenance |

The deterministic sections of the text report and the edge's `changes`,
`facts` and `evidence` are byte-identical with and without `-enrich`. Tests
assert this both for the text output (`internal/upgrade`) and for the edge
(`internal/enrich`, `internal/app`). Enrichments are rendered in their own
section, after the deterministic ones.

## Kinds

| kind | relates to | must cite | notes |
|---|---|---|---|
| `cluster` | ≥ 2 Changes | ≥ 1 evidence record **of every member** | One conclusion for semantically equivalent Changes. A Change belongs to at most one cluster. |
| `migration-summary` | ≥ 1 Change | ≥ 1 record | Only steps the evidence states. |
| `diff-explanation` | ≥ 1 **computed** Change | ≥ 1 record of a release-note / upgrade-guide Change | Why the diff matters. |
| `related` | ≥ 2 Changes | ≥ 1 record | `unverified: true`, confidence `low`. The relation is a hypothesis. |

AI output is never `high` confidence: the model's own rating is capped at
`medium`, and `related` is always `low`.

## Pipeline (`internal/enrich`)

1. **Deterministic candidate groups** (`enrich.Candidates`). Pairs of
   Changes are linked by:
   - a shared subject or key-like code span (`Certificate.Spec.X`,
     `global.rbac.disableHTTPChallengesRole`, `PathType`);
   - a shared CVE, GHSA, pull-request, issue or commit reference;
   - IDF-weighted token overlap between Changes from **different** sources;
   - a computed diff whose key is mentioned in a note: the full key, its
     specific leaf, or at least two specific parts of it including the leaf;
   - an upgrade-guide ↔ release-note pair of the same release.

   Strong links merge first. Weak links only attach single Changes, with a
   small budget per Change. Groups hold at most 6 Changes. On the recorded
   cert-manager edge this yields 8 groups for 51 Changes, including the
   RotationPolicy trio, the RevisionHistoryLimit trio, the PathType pair and
   the targetPort diff ↔ release-note pair.

   **Candidates are not conclusions.** Without a model nothing is concluded
   from them. `-enrich-candidates` prints them to stderr for debugging.
2. **One bounded prompt per group** (at most 40 per edge, `-enrich-max`):
   - the signals, then the group's Changes (category, method, release,
     sources, title, detail ≤ 500 characters, subjects);
   - then their evidence (id, source, URI, locator, excerpt ≤ 500
     characters), about 14 000 characters at most;
   - structured JSON output with a JSON Schema.

   The system prompt states the rules above and the prompt version
   (`enrich/v1`).
3. **Validator.** The answer must match the schema, otherwise the whole
   answer is rejected. Each proposed enrichment is then checked and
   rejected, with the reason recorded in `enrichmentRun.rejected`, when:
   - a change id is unknown (hallucinated) or not part of the group;
   - a citation was not shown in the prompt;
   - the content is empty or the citations are missing;
   - a cluster has a member without a cited record, or a member that is
     already clustered;
   - a diff explanation has no computed Change or does not cite a statement;
   - it duplicates an accepted enrichment.
4. **Enrichments** with AI provenance (below). `enrich.Apply` attaches them
   and the run metadata, then re-validates the whole edge.

## The provenance record

```json
{
  "id": "enr-3f0c…",
  "kind": "cluster",
  "title": "Private key rotation policy now defaults to Always",
  "content": "The upgrade guide and two release-note entries state the same change: …",
  "relatesTo": ["chg-a726b336f865", "chg-0e1cdbe373a9", "chg-fa4ce35f9a12"],
  "citations": ["ev-9a2f8d3ec5e9", "ev-c92478af13c4", "ev-cf14b14073be"],
  "provenance": {
    "method": "ai",
    "producer": "enrich@v1",
    "rule": "group:cand-acba2bf9a457",
    "confidence": "medium",
    "model": "batch-model",
    "modelVersion": "batch-model-2026-09-30",
    "promptVersion": "enrich/v1",
    "promptDigest": "sha256:68adf3bcb116…",
    "inputEvidence": ["ev-9a2f8d3ec5e9", "ev-c92478af13c4", "ev-cf14b14073be"],
    "generatedAt": "2026-10-01T08:00:00Z"
  }
}
```

- **model / modelVersion**: as reported with the answer, never assumed
  from the request. The Anthropic Messages API reports one pinned model id,
  which is recorded as both. An exchange response file must name both.
- **promptVersion**: the template version. **promptDigest**: the sha256 of
  the exact request (system prompt, user prompt, answer schema, requested
  model).
- **inputEvidence**: every evidence id shown in the prompt.
  **citations**: the subset the enrichment relies on.
- **generatedAt**: when the answer was produced. It is stored in the cache,
  so replays keep it.

`UpgradeEdge.Validate()` and `schemas/upgrade-edge.schema.json` enforce
these invariants:
- all AI provenance fields are present;
- citations ⊆ inputEvidence ⊆ the edge's evidence;
- every `relatesTo` id is a Change of the edge;
- the kind rules above hold;
- enrichment ids start with `enr-` and change ids never do, and a Change
  can never carry AI provenance, so enrichments never appear as Changes;
- `enrichmentRun` agrees with the enrichments.

Referential checks are Go-only, because JSON Schema cannot cross-reference
ids.

## The duplicate metric

`edge.enrichmentRun` records:
- `candidateGroups`, `requests`, `pending`, `failed`, `accepted`;
- every rejection;
- `clusters`, `clusteredChanges` and
  `duplicatesConsolidated = clusteredChanges − clusters`, which is the
  number of Changes that duplicate another Change of the same cluster.

The text report prints the metric on its `Duplicates:` line. A ground-truth
evaluation (G9) can compare `duplicatesConsolidated` and the cluster
membership with labelled duplicates.

## Model backends, cache and offline replay

| Setting | Backend |
|---|---|
| `-llm-exchange DIR` | file exchange (works offline) |
| `ANTHROPIC_API_KEY` set, not `-offline` | Anthropic Messages API |
| neither, or `-offline` | cached answers only; misses stay `pending` |

Every backend sits behind a **response cache** at `<state>/llm-cache/<hex
digest>.json`. Each entry stores the full request, the answer text, model,
model version, generation time and origin.
- The cache is content-addressed. A changed prompt (other evidence, another
  `-model`, a new prompt version) is a new digest.
- The cache never stores errors.
- `-refresh` re-asks the model and overwrites the entry.
- A run that succeeded once replays offline byte for byte:
  `ri -offline upgrade … -enrich` gives identical enrichments, without a key
  and without the exchange directory.

## The exchange workflow (batch / offline, any model)

1. `ri upgrade cert-manager v1.17.0 v1.18.0 -enrich -llm-exchange answers/`
   writes one `answers/<hex digest>.request.json` per prompt that has no
   cached answer, and reports them as pending:
   ```json
   {
     "format": "ri.dev/llm-exchange/request/v1",
     "promptDigest": "sha256:<hex>",
     "request": {"system": "…", "messages": [{"role": "user", "content": "…"}], "jsonSchema": {…}},
     "responseFile": "<hex>.response.json",
     "instructions": "…"
   }
   ```
2. Answer each request with any model, tool or person. Send
   `request.system` as the system prompt and `request.messages` as the
   conversation, and ask for JSON conforming to `request.jsonSchema`. Then
   write `answers/<hex>.response.json`:
   ```json
   {
     "format": "ri.dev/llm-exchange/response/v1",
     "promptDigest": "sha256:<hex>",
     "model": "<model id that answered>",
     "modelVersion": "<exact version / snapshot>",
     "generatedAt": "2026-10-01T08:00:00Z",
     "output": {"enrichments": [ … ]}
   }
   ```
   - `promptDigest` must be copied from the request.
   - `model` and `modelVersion` are required.
   - `output` (a JSON value) or `text` (a string) holds the answer.
   - `generatedAt` defaults to the file's modification time.

   A shell sketch, for any command-line client `my-llm` that takes a system
   prompt, a user prompt and a schema:
   ```sh
   for req in answers/*.request.json; do
     resp="${req%.request.json}.response.json"; [ -e "$resp" ] && continue
     out=$(my-llm --system "$(jq -r .request.system "$req")" \
                  --schema "$(jq -c .request.jsonSchema "$req")" \
                  "$(jq -r '.request.messages[0].content' "$req")")
     jq -n --arg d "$(jq -r .promptDigest "$req")" --argjson o "$out" \
        '{format:"ri.dev/llm-exchange/response/v1", promptDigest:$d,
          model:"<id>", modelVersion:"<version>", output:$o}' > "$resp"
   done
   ```
3. Run the same command again. Answered prompts are validated, cached and
   become enrichments. Unanswered ones stay pending. A response whose
   `promptDigest` differs from its request's digest is refused and recorded
   as a rejection. So is a response without a model or version, or one that
   is not valid JSON.
4. From then on, `ri -offline upgrade … -enrich` replays from the cache. The
   exchange directory is no longer needed.

The request file must not be edited: its digest identifies the prompt, and
the response is checked against the digest of the prompt the next run builds.

## How to verify an enrichment

1. **Read the cited evidence.** For each id in `citations`, open `uri` at
   `locator` (line range or heading) and compare with `excerpt` in the
   edge's `evidence`. `contentDigest` pins the exact bytes that were read.
   With `-verbose` the text report prints each excerpt under its citation.
2. **Check the claim against the Changes.** Every id in `relatesTo` is a
   deterministic Change in the same edge. For a `cluster`, each member must
   say the same thing as the content. For a `diff-explanation`, the computed
   Change shows the actual diff, and the cited statement must explain it.
3. **Treat `related` as a lead, not a finding.** It is `unverified` by
   construction.
4. **Audit the generation.** `promptDigest` names
   `<state>/llm-cache/<hex>.json`, which holds the exact request and answer.
   The digest is re-checked whenever the entry is read. `inputEvidence`
   lists everything the model was shown, so you can see what it did not
   see.
5. **Re-run with another model.** Use `-model` with the API, or another
   model through the exchange, and compare: same prompt version, different
   digest.

## Limits

- **Recall is bounded by candidate generation.** It is lexical: keys,
  references, wording. Duplicates that share no key, reference or
  vocabulary are never shown to the model. Weak links can also group
  unrelated items; the model may then answer `related` or nothing.
- **Bounded per edge**: at most 6 Changes per group and 40 prompts per edge
  (`-enrich-max`). Groups that were not asked are counted (`requests <
  candidateGroups`), but no conclusion is drawn from them.
- **The validator checks grounding structurally, not semantically.**
  Ids, citations, kinds and cluster coverage are checked. An answer can
  cite the right evidence and still overstate it. Hence "verify against
  evidence".
- **Evidence excerpts are truncated** (600 characters when ingested, 500 in
  the prompt). Helm values and CRD diffs cite whole files whose excerpt is
  the first line. Diff explanations therefore rely on the computed Change's
  title and detail, plus the cited statement.
- **Requests are sequential**, and the run stops after 3 consecutive
  failures (for example a bad key). There is no streaming or provider batch
  API; batch operation goes through the exchange.
- **Enrichments belong to one edge.** They are stored with the edge
  (`<state>/store`), not with releases. A different path or definition
  yields a different edge, and its prompts are new digests.
