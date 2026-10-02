# Knowledge store, review queue, dataset and metrics

Implementation notes for `internal/knowledge` (design: `docs/phase3/learning-loop/DESIGN.md` §2–§8).

## Store (`knowledge/`)

`NewFileStore(dir)` keeps one JSON file per record, wrapped in `domain.KnowledgeRecord`:

```text
knowledge/<product>/<release|_endpoint>/{candidates,proposals,validations,reviews,decisions,facts}/<id>.json
```

- Writes are idempotent (content-derived ids), atomic, and validated (`Validate()`; facts additionally
  `domain.ValidateFactRecords`, re-proving every aspect from the stored validations, decisions and proposals).
- Decisions, candidates, proposals and validations are immutable (`ErrConflict`). Review items change only
  `status`; facts only `status`, `anchors`, `candidates` and verification **upgrades** (never weakened).
- Records other than candidates and facts are placed by resolving their candidate (`ErrOrphan` if missing).

## Routing (`Route`, `RouteStore`)

DESIGN §6: validations confirming every aspect → auto-verified fact; otherwise review items for the open
aspects (semantic-mapping / relationship / applicability / consequence), with signals and priority recorded.
Aspects nobody proposed produce one `evidence-sufficiency` item on route `missing-evidence`.

Auto-approval is a policy hook (`AutoApprovePolicy`, installed by `ri knowledge route` as `DefaultAutoApprove`; `Route()` alone
stays the pure table). A policy counts only if, with the validator-confirmed aspects, it covers all four.

- **Consensus (PO-1)** = ≥2 *separate* stateless calls agree on an aspect digest (`domain.SeparateCalls`; any models,
  including two calls of the same model), no validator refuted it. Basis = one proposal per call; the aspect is labelled
  `cross-model` or `same-model` (`domain.ConsensusScopeOf`), and `FactMetrics.ConsensusAgreementByScope` measures whether
  same-model agreement is as reliable as cross-model against the human audit.
- `AutoApproveRenderVerifiable` (RENDER-MISSION Goal 10): class render-verifiable, subject + change confirmed by the render
  (`renderRelation: confirmed-by-render`), consensus on the rest.
- `AutoApproveConsensusAction` (PO-2): ≥2 separate calls agree on an action-eligible consequence and all requested
  `action-required`, other aspects validator-confirmed or consensus, nothing refuted → fact with `ConsensusAction`.
  Models may request action-required; the ladder (applicability lane) still needs exposure TRUE with both evidence chains.
  Consensus never produces NOT AFFECTED.

- `AutoApproveGeneral` (MISSION Goal 16; **opt-in**, `ri knowledge route -auto-approve-general`): every aspect consensus-or-better,
  at least one of subject/change confirmed by a validator, no validator refutation on any aspect. Minted at its weakest level
  (consensus, so capped at review by the ladder unless PO-2 consensus-action holds). A proxy decision is never an input.
- `DeriveRenderability` gives the policies the renderability of what the change IS (`domain.RenderabilityOf(family, change kind)`),
  from a validator-confirmed or ≥2-separate-call consensus subject+change; never from one model alone.
- The audit item of an auto-approved fact carries routing signals (`policy-render|policy-general`, `auto-approved`,
  `consensus-same-model|cross-model`, `consensus-action`) so the policies can be tested against human outcomes later.
  Open items made moot by an auto-verified fact are marked `superseded`.

Such facts are `autoApproved`, level `consensus`. `RouteOptions.AuditEvery` samples one in N auto-approved facts into
human review (a fact-review item); `ConsensusAction` facts and auto-approved facts with an action-eligible consequence are
**always** audited (100%). The audit decision is written before the fact is touched, and the R19 agreement metrics
(overall, per family, per consensus scope, consensus-action) are computed from those decision records. An audit **accept**
upgrades the aspects the human verified to `human` (the `autoApproved` marker stays as history; `ConsensusAction` stays
while the fact's weakest aspect is still consensus); a reject retracts the fact, a correction supersedes it.

## Decisions → facts (`Queue.Decide`, `FactFromDecision`)

Accept/correct verify the aspects the item's question covers; a fact is minted when all four aspects are verified.
Corrections keep `Original` and `Corrected`. Later decisions override earlier ones per aspect; a validator-confirmed
identical value stays deterministic; a proxy never overrides a trusted aspect. Reject + `duplicate` adds the
candidate's anchors to the named fact. Retraction/supersession: `OpenFactReview` / `ri knowledge review-fact`.
All decisions of a call (bulk review shares a `BatchID`) are validated before anything is written.

## Dataset and metrics

`ExportDataset` writes JSONL `domain.FeedbackExample` (all G13 labels, original/corrected, proposals, validations,
fact). `ComputeMetrics` implements the DESIGN §7 table: agreement (per aspect × task, pairwise), per-model
accepted-as-is / corrected / rejected / insufficient / abstentions (also per task), FP/FN per aspect vs final facts,
grounded citations, review volume, median/p90 time, **batch vs individual reported separately**, auto-validation
and repeat-pattern rates, fact counts. Fact evidence is the validators' artifact evidence plus the candidate
evidence cited by proposals agreeing with the fact (all candidate evidence if none), which makes the grounding
metric non-trivial.

## CLI

`ri knowledge route|decide|review-fact|export|metrics`. `decide --reviewer-kind proxy` is the only way proxy
decisions enter and requires the AI provenance flags. The blind proxy reviewer (`ri knowledge proxy-prompt` → `scripts/proxy-review.sh` →
`decide -reviewer-kind proxy -proxy-response …`, `ri knowledge proxy-report`) is described in
[docs/phase3/learning-loop/proxy/README.md](phase3/learning-loop/proxy/README.md). `ri knowledge candidates|propose` are aliases of
`ri semantic candidates|propose` (same flags, same code); `validate` belongs to the validate lane (not wired yet).

## Environment context and the transfer subset

`ReviewItem.Context` is **absent** unless an environment was shown to the reviewer. When present, `Context.Label` is
exactly the **eval case id** (the directory name under `eval/cases/`, e.g. `cert-manager-1.17-1.18`; no spaces or path
separators) and `Context.Digest` optionally digests the environment shown. The eval's transfer subset compares the label
with the case id for equality, and treats a missing context as "never reviewed with an environment".

Whatever creates review items with environment context sets it, at creation: `RouteOptions.Environment` /
`ri knowledge route -env-label <case-id> [-env-digest D]` stamp it on the items that call creates
(`ValidateEnvironment` enforces the label shape); follow-up items created by `Queue.Decide` inherit the decided item's
context (same reviewer session). Items default to environment-free, and `Context` cannot be added later: it is not
one of the mutable fields, and the item id does not include it, so the first creation of an item fixes its context.
The dashboard must not invent contexts.

Integrity: this is the one place a case id may appear under `knowledge/` — the integrity test exempts
`reviewItem.context` (it names the environment shown, not what the case expects) and still fails on a case id anywhere
else.

## Integrity

`integrity_test.go` fails if any file under `knowledge/` mentions `eval/cases`, expectation blocks, or an eval case
id (blind authoring).
