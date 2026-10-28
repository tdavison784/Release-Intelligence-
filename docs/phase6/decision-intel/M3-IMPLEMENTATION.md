# Milestone 3 implementation: upgrade-level synthesis (lane synthesis)

`PLAN-M3.md` §3 changes 1–7 with the §4 tests, on branch `p6di/synthesis` (ledger D-0017). Product decisions PD-1
(the judgement an engineer approves or denies), PD-3 (latest safe patch, lowest safe shown as the minimum), PD-0a/0b
(a concrete safer target; small isolated changes) and PD-2 (v13 investigations). Everything is additive: findings,
the decision, the review, knowledge, earlier prompts and recorded Temporal histories are unchanged.

## What changed, and where

| # | Change | Where |
|---|---|---|
| 1 | **Contract.** `domain.UpgradeJudgement` (schema `ri.dev/upgrade-judgement/v1alpha1`) and `Investigation.Judgement`; stage `synthesize` (between classify and human-review). `Validate` checks the derived ids (`jdg-`, `jf-`, `loc-`, `rr-`, `jt-`, `xc-`), the hard flags, the status (a failed judgement has no recommendation, headline, steps or chosen target, and records its attempts, violations and rejected answers), that the chosen target is a **safe** computed option, that a step is `proposed` exactly when it names no location and that a named location is one of the judgement's, and the AI provenance (producer `synthesize@v1`, `statementKind: unverified-ai-prose`). `Investigation.ValidateAgainst` resolves every cited evidence id in the bundle, the run pool or the judgement's own `evidence` (the edge's advisory and chart records the run lacks), and every change item. Schemas regenerated. | `internal/domain/judgement.go`, `investigation.go`, `schemagen/meta.go`, `schemas/investigation.schema.json` |
| 2 | **Pre-computation** (`synthesize.Precompute`, pure) over the run record, bundle, impact report, edge and the product's release facts. **Findings** (copied verbatim into the judgement; hard ones marked): security regressions; confirmed breakage (a finding classified affected); failed validations; target options still affected (or no release removes the regressions); agent-versus-classification disagreements (classes differ and either says affected or not-affected); untraced consumers; unestablished upstream behaviour (`not-checked` / `unavailable` behaviour entries, unavailable requests no entry names); advisory inconsistencies; an incomplete or missing advisory assessment. **Target** (PD-3, below). **Locations**: a passing pin (`ValidationRecord.Pin`), verified setting claims (set with file and line; unset with the input file), and the GitOps line that states the deployed version (the impact report's product inventory entry of the product from an Argo CD Application or a Flux HelmRelease whose evidence excerpt states the version: file, line, key, value). **Per-item digest** (classification, rule, agent verdict, assessment headline, what changed, mechanism, likely impact, effective configuration, preconditions, unknowns, tests). **Candidate tests** (assessment recommended tests, findings' next steps, the transition a changed target needs). **Residual risks** (every potentially-affected or insufficient-evidence item). **Interactions** (items correlated to the same rendered object, or locations on the same key). **Backout** (restore the deployed-version line to its current value). Text is clipped and lists capped for the model; the judgement's own records are not. | `internal/synthesize/precompute.go`, `target.go` |
| 3 | **One structured call**: `synthesize.Synthesizer{Client: llm.Cache{Inner: Exchange \| ClaudePrint}}` with the JSON schema of `prompts/synthesize/v1/schema.json`; model `-synthesize-model` (default `claude-opus-5-5`); provenance from the answer (model, model version, generation time, call id, provider), the rendered request's digest, prompt `synthesize/v1`, input evidence = every evidence id of the input. A replay is a cache hit. `ClaudePrint` is one stateless `claude -p --tools "" --json-schema … --output-format json --no-session-persistence --setting-sources ""` call in an empty directory; model and version from `modelUsage`, call id from `session_id`. | `internal/synthesize/synthesize.go`, `claudep.go`, `prompts/synthesize/v1/`, `cmd/ri/synthesize.go` |
| 4 | **Floor and retry** (`synthesize.Floor`): `proceed` impossible with a hard finding; `targetVersion` empty or a computed option, never an unsafe one ("a target with a regression cannot be recommended"); `proceed` / `proceed-with-changes` keep the proposed target; `modify-target` takes another option and names it in the headline; every finding, interaction, test, location, change-item and evidence id resolves; a verified statement cites evidence; a step's location is a computed location (or none: `proposed`); at least one required test unless `do-not-proceed`. The answer is decoded strictly (an unknown field — a rewritten findings list, a file written as a field — is a violation). A violating answer is retried with its violations and the rejected answer, as one user message (so any exchange answerer serves it), at most `MaxAttempts` = 3 calls; then the judgement is recorded `failed` with every rejected answer's digest. Nothing is repaired. A changed target always gets the transition investigation as a required test (deterministic, like the findings). | `internal/synthesize/floor.go`, `synthesize.go` |
| 5 | **Stage.** `Pipeline.Synthesize` (memoised on the record's content before it; sets only `Judgement` and the `synthesize` audit events; validates the record against its bundle). In-process: `Runner.Synthesize`. Temporal: `Input.Synthesize` → activity `synthesize` behind `workflow.GetVersion("synthesize", DefaultVersion, 1)`, called only when the input asks for it. A synthesis without a judgement (pending exchange, model unreachable, no synthesizer) is audited `failed` and the run continues to the review. | `internal/investigate/run/synthesize.go`, `runner.go`, `types.go`, `internal/orchestrate/` |
| 6 | **Rendering.** `synthesize.RenderJudgement`: a change-ticket section (summary and recommendation; target options with charts and what each still leaves; disagreements and hard findings with the model's comments; interactions; residual risks; impact with the agent's assessments; ordered steps with their file, line, key and current value or "proposed"; validation; backout; audit). Printed by `ri investigate` after the run summary; shown on the reportui page and its self-contained export in an AI box. | `internal/synthesize/render.go`, `cmd/ri/investigate.go`, `internal/investigate/reportui/` |
| 7 | **Scoring hook.** `eval.ScoreJudgements`, an ungated side table (`judgements` in the score JSON; text "upgrade judgements"): status, recommendation, chosen / minimum / rule target, counts of steps (located), tests, risks, findings (hard), interactions, and the missing required parts. It reads no expectation; gates unchanged. | `internal/eval/investigations_judgements.go` |

## The target rule (PD-3), as computed

- **Minimum**: the proposed target when it has no regression; otherwise the edge's `target-line` candidate, else its
  `any-line` candidate.
- **Recommended**: the latest known stable release on the minimum's line, above it, when it is affected by no assessed
  advisory the current version is free of **and** by none the minimum is free of; otherwise the minimum. It is read
  against every advisory of the product with the edge's own range reading (`upgrade.AdvisoryReading`, an additive
  helper), and only when those release facts reproduce the edge's regressions exactly.
- **Options** the model may choose: the proposed target (marked unsafe when it regresses), the minimum, the edge's
  `clean` candidate when it differs, and the latest patch when it qualifies. The `current-line` candidate is never a
  target.
- **Nothing is safe while the advisory assessment is incomplete or unavailable** (an unread source may list a
  regression: mission A2). Then there is no minimum and no recommendation; the rule says why. Found by the smoke below.

## How to run

```sh
go build -o bin/ri ./cmd/ri
# synthesis answered by a stateless claude -p call (cached under <state>/llm-cache)
bin/ri -state "$STATE" investigate <product> <from> <to> -repo <env> -agent -synthesize
# file exchange: the first run writes <dir>/<digest>.request.json and continues without a judgement;
# answer it (scripts/semantic-exchange.sh <dir> claude-opus-5-5), then run again
bin/ri -state "$STATE" investigate … -synthesize -synthesize-exchange <dir>
# offline replay: the cache answers
bin/ri -state "$STATE" -offline investigate … -synthesize
# evaluation, with the judgement side table
bin/ri -state "$STATE" eval -investigations -case <case> -knowledge <K0> -agent … -synthesize [-synthesize-exchange <dir>]
# Temporal: ri worker (same flags for model/exchange) serves the activity; the client passes -synthesize
bin/ri investigate … -synthesize -temporal
```

The latest-patch rule reads the product's releases, advisories and chart index through the app's cached ingestion; an
offline replay therefore needs the recorded state's caches (as the edge itself does).

## Tests

- **Pre-computation** (`internal/synthesize/precompute_test.go`): every finding kind with a derived, stable id and
  the right hard flag; both directions of agent-versus-classifier disagreement (and no disagreement between the two
  undecided classes); PD-3 — the latest patch versus the minimum (qualifies; adds an advisory; no release list; release
  facts that disagree with the edge; no regression; no safe release; incomplete and unavailable advisory pictures);
  pin, set and unset setting, and targetRevision locations with file, line, key, value and evidence, and the backout
  line; a failed pin and a non-GitOps version source give no location; interactions, deterministic risks, the per-item
  digest and the transition test.
- **Floor** (`synthesize_test.go` `TestFloor`): proceed with a hard finding; a target with a regression; a version
  outside the options; modify-target keeping the target; a headline without the version; unresolvable finding, test,
  evidence, ref and change-item ids; an invented location; a location written as a field and a rewritten findings
  list (strict decoding); a verified statement without citations; no validation.
- **Findings verbatim** (`TestFindingsAreCopiedVerbatim`): an answer that drops every comment still yields exactly the
  pre-computed findings, interactions, locations and backout.
- **The call** (`TestCallThroughExchangeReplaysIdentically`): through `llm.Cache` over a scripted file exchange —
  pending, answered, complete provenance, the changed target and its required transition, located and proposed steps —
  and an offline replay (cache only) yields a byte-identical judgement. `TestBoundedRetryAndRecordedFailure`: the retry
  carries the violations and the rejected answer; a decode failure is retried; three violating answers give a recorded
  `failed` judgement that validates.
- **Prompt** (`TestPromptIsGenericAndVersioned`): versioned with a digest; names no product, case, file, field, version
  or advisory.
- **Rendering** (`TestRenderGolden`, `testdata/judgement.golden`; regenerate with `RI_UPDATE_GOLDEN=1`).
- **Policy isolation** (`internal/investigate/run/synthesize_test.go`): findings, decision, review, validations and
  agent verdicts are byte-identical with and without synthesis, before and after a human approval; the investigation
  id is unchanged; only the judgement and two audit events are added; the record validates against its bundle.
  Idempotence (a second call is a memo hit, no model call), a missing synthesizer is a permanent error, and a failed
  synthesis continues to the review.
- **Temporal** (`internal/orchestrate/synthesize_test.go`): the testsuite workflow with `Synthesize` records the same
  findings, decision and judgement as the in-process runner; without it no synthesis runs; a failing activity is
  audited and the run is approved. `TestReplayRecordedHistories` replays the existing `history-approve.json` and the
  new `history-synthesize.json` (recorded from a local dev server with `TestRecordHistorySynthesize`; it has the
  version marker and the `synthesize` activity).
- **Eval** (`internal/eval/investigations_judgements_test.go`) and **reportui** (`judgement_test.go`: the section on
  the page and in the export; none without a judgement).

## Smoke (unscored, non-development case, scripted answer)

Case `ingress-nginx-1.11-1.12--shop-nl` (only `input.yaml` and `env/` read), deterministic arm (no agent), cloned state,
empty knowledge directory, `-synthesize -synthesize-exchange <dir>`.

- **Offline** (no global advisory cache in the state): 30 findings; pre-computed findings: confirmed breakage, failed
  validation, security-incomplete. The proposed target and the latest patch came out *safe* from the sources that were
  read: that was the defect fixed above (now no option is safe and nothing is recommended while a source is unread).
- **Online** (`GITHUB_TOKEN` in that command only): 39 findings; five security regressions of the target (hard), minimum
  `controller-v1.12.1` (chart 4.12.1), PD-3 recommendation `controller-v1.12.8` (chart 4.12.8; checked against 16
  assessed advisories), clean `controller-v1.13.7` (chart 4.13.7), the four advisories persisting from the current
  version flagged as "still affected" for 1.12.1 and 1.12.8; one deployed-version location (Argo
  `$.spec.sources[0].targetRevision`, line 16, value `4.11.5`, the chart version) and the backout line on it; one
  interaction (four items on the controller Deployment); 26 deterministic residual risks.
- A hand-written answer (modify-target to 1.12.8, version bump on the location, one test) was written to the exchange;
  the re-run ingested it: judgement `complete modify-target`, the record validated, and the change-ticket section
  printed, with the transition investigation required.

No live model call was made and no development case was run (brief: the commander runs the scored M3 check).

## Known limits

- **The switch is outside the request configuration**, so the investigation id is the same with or without a judgement.
  The Temporal workflow id is too: re-submitting a finished request with `-synthesize` attaches to the earlier
  execution; use `-rerun`.
- **Release facts are read at synthesis time** from the app's cached ingestion, not stored in the bundle. They are
  checked against the edge (same regressions); a mismatch, or no release list, recommends the minimum and says why.
- **Safe needs a complete advisory assessment.** A product whose security sources are partly unread always gets no
  safe option, so its judgement can only be `validation-required` or `do-not-proceed`. Deliberate (fail closed); it may
  be stricter than a reviewer wants when the unread source is minor.
- **When the proposed target has no regression,** the rule still recommends the latest patch of its line; the model may
  choose the proposed target instead (both are options).
- **Locations are narrow on purpose**: passing pins, verified setting claims, and the Argo CD / Flux line of the
  upgraded product. A Helm annotation or label is not a location. The file is the evidence URI (an absolute checkout path
  in a local run). Everything else is a `proposed` step.
- **The floor checks structure, not prose.** It checks ids, versions, the headline's version for `modify-target`,
  and kinds; it cannot check that a precondition is the right one, that a step does what its summary says, or that
  `inferred` / `unknown` are complete. The judgement stays unverified AI prose.
- **Interactions** are exact matches on rendered object ids and location keys; a semantic overlap (two settings with
  the same effect) is not found.
- **Input size**: the per-item digests and candidate tests grow with the items (about 117 KB, 39 items and 152 candidate tests on the online smoke).
- `claude -p` runs with the CLI's default thinking; `ModelVersion` is the joined `modelUsage` keys, as for the agent
  driver.
- **Out of scope** (M5): recording approve / deny / override. The judgement only shapes them: stable ids for every
  finding, risk and test.
