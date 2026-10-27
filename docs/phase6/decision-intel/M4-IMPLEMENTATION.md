# Milestone 4 implementation: formalization and independent verification

`PLAN-M4.md` §3, three lanes (ledger D-0021..D-0023). Each lane adds its own section below. Product decisions PD-1
(humans approve; approval never promotes knowledge) and PD-2 (v13 investigations). Everything is additive: run
records, findings, decisions, reviews, knowledge, earlier prompts and recorded Temporal histories are unchanged.

## Lane `formalize` (D-0021): Workstream D

Branch `p6di/formalize`. Plan changes 1–4 with the brief's tests.

### What changed, and where

| # | Change | Where |
|---|---|---|
| 1 | **Stage.** `formalize.Formalizer{Client: llm.Cache{Inner: Exchange \| synthesize.ClaudePrint}}`, the `internal/synthesize` pattern. `Precompute` (pure) reads one assessment, its item and the run's evidence pool and builds the model's input: the assessment (headline, scope, preconditions, what changed, mechanism, consumers, effective configuration, verified, inferred, unknown, behaviour, likely impact, tests), the item's change text, the **citable evidence**, the **uncertainties** (each with a content-derived `unc-` id) and the **environment basis** (the item's verified setting claims and validation records). One call with the JSON schema of `prompts/formalize/v1/schema.json`; model `-formalize-model` (default `claude-opus-5-5`); provenance from the answer (model, model version, generation time, call id, provider), the rendered request's digest, prompt `formalize/v1`, input evidence. A replay is a cache hit. | `internal/formalize/formalize.go`, `precompute.go`, `prompts/formalize/v1/` |
| 1b | **Spec checks lifted** into the pure `agent.CheckMitigation(ctx, MitigationEnv, item, input, cites)` and `agent.BoundsWithinRun`. The session builds its `MitigationEnv` from its own lookups (bundle, artifact reader, artifact index, `basisRef`), so the v8 path is unchanged (same checks, same order, same messages; the v6/v8 mitigation tests pass unchanged). The formalizer supplies the stored run's lookups. | `internal/investigate/agent/mitigation.go` |
| 2 | **Non-invention floor** (`formalize.Floor`, after the call; strict decoding, an unknown field anywhere is a violation): every citation is citable evidence; every uncertainty is kept by an unrepresentable record (in `keeps`, or as an undecidable leaf of its partial rule whose `needed` names the uncertainty id, replaced by the uncertainty's text); a rule passes `CheckMitigation`; a partial rule validates, is of the run's product and within the run's bounds; no rule treats an unset field as a value (`any` of `unset` / `not(set)` and a value test on the same field: a default without evidence); kinds carry only their own fields; no repeated claim. A violating answer is retried with its violations and the rejected answer (one user message, as synthesis), at most `MaxAttempts` = 3 calls; then the record is `failed` with every rejected answer's digest. Nothing is repaired. | `internal/formalize/floor.go` |
| 3 | **Unrepresentable claims** (D3): a `Hypothesis` with `MitigationRejected` = "not representable as a mitigation rule: <limitation>", the original claim, its citations, and the additive `Hypothesis.ProposedVerification` (`domain.ProposedVerification`: the assessment id, the method, the intended conditions, the uncertainties kept verbatim, and the partial rule). A rule hypothesis gets `ProposedVerification` too: every uncertainty of the assessment, also appended to the spec's requested verification so the human reviewer sees it in the review item. Schemas regenerated. | `internal/domain/investigation.go` |
| 4 | **Entry points.** `ri investigate formalize <inv-id> [-assessment aas-…] [-knowledge DIR] [-formalize-model M] [-formalize-exchange DIR] [-claude-bin B]` and `-formalize` on `ri eval -investigations` (records in the work store and in the stored case as `formalizations.json`). Records are stored beside the run at `<state>/investigations/formalizations/<inv-id>/<fmz-id>.json` (`formalize.Store`). `-knowledge` feeds the run's formalized hypotheses to `agent.ExportHypotheses` **unchanged**, through a view of the run whose hypotheses are the formalized ones; `ri verify hypotheses` finds them through a run source that reads the formalization records first, then the run record. No Temporal activity. | `cmd/ri/formalize.go`, `verify.go`, `eval_investigations.go` |

### Evidenced defaults (after the `applicability` merge)

The `applicability` lane added `Condition.Default` (`domain.FieldDefault`: a JSON value, source `crd-schema` or
`controller`, citations), which the shared evaluator (`internal/applicability`) and fact validation honour only when
every citation resolves to quoted upstream evidence. Formalization now emits it instead of keeping a default-dependent
claim unrepresentable:

- **Prompt** (`formalize/v1`, edited in place: it had never been run or recorded): where the claim depends on what an
  unset field means and the citable evidence holds upstream text establishing the default, the model states it on the
  value leaf (`default: {value, source, citations}`) and lists the matching `default` uncertainty in the hypothesis'
  new `discharges`; otherwise an undecidable leaf, as before.
- **Floor**: every default citation must be citable evidence (the assessment's own), upstream text, and pass
  `FieldDefault.Unevidenced` over the citable evidence (a quoted excerpt, never environment evidence); a `discharges`
  id must be a `default` uncertainty and the hypothesis' rule must state a default; a `default` uncertainty is kept
  (record) or discharged (rule), never dropped. The `any(unset, value)` form stays rejected: it states a default
  without evidence; its message now points to `default`.
- **Output**: a default's citations are added to the hypothesis' citations, so `ExportHypotheses` carries them into the
  proposal and the candidate's evidence (a promoted fact then validates its default). The discharged uncertainty is no
  longer listed as open on the rule; the requested verification asks for the default to be verified with the field
  unset (the `applicability` lane's boundary fixtures).

### The record

`formalize.Record` (`ri.dev/formalization/v1alpha1`): id `fmz-` (investigation, assessment, input digest, call id), the
assessment and item, status `complete` (hypotheses, or `noHypothesisReason`) or `failed` (attempts, last violations,
rejected answers' digests), the uncertainties, statement kind `unverified-ai-prose`, AI provenance (producer
`formalize@v1`). `Validate` checks every hypothesis (`Hypothesis.Validate`), that each names the assessment, carries a
mitigation or the limitation that kept it out, and is of this record's call.

### Why the record is not on the run record

Run records are immutable (mission §13) and stored runs are digest-checked; `Investigation.Hypotheses` is part of the
classifier's agent input. So formalized hypotheses live only in the formalization records: formalization alone never
changes a classification (test below).

### How to run

```sh
go build -o bin/ri ./cmd/ri
bin/ri -state "$STATE" investigate formalize <inv-id>                               # claude -p, cached under <state>/llm-cache
bin/ri -state "$STATE" investigate formalize <inv-id> -formalize-exchange <dir>     # writes requests; answer them
                                                                                    # (scripts/semantic-exchange.sh <dir> claude-opus-5-5), run again
bin/ri -state "$STATE" -offline investigate formalize <inv-id>                      # replay: the cache answers
bin/ri -state "$STATE" investigate formalize <inv-id> -knowledge <K-copy>           # + export proposals and review items
bin/ri -state "$STATE" verify hypotheses -knowledge <K-copy> -runtime               # independent verification
bin/ri -state "$STATE" eval -investigations -case <case> -knowledge <K0> -agent … -formalize
```

### Tests

- **Pre-computation** (`TestPrecompute`): the citable set is the assessment's verified and behaviour evidence plus the
  item's claim evidence, never another item's; one uncertainty of each kind; the basis.
- **Non-invention** (`TestFloor`): a foreign citation, an invented citation, a dropped unknown, a dropped precondition,
  bounds beyond the run (rule and partial rule), another product, an unevidenced default (`unset` and `not(set)` forms),
  an undecidable rule, a basis of another run, no upstream citation, a rule keeping an uncertainty, a record without
  limitation, a repeated claim, an unknown kept id, an empty answer without reason; strict decoding (an unknown field
  inside a rule and at the top level).
- **Unrepresentable claim kept** (`TestUnrepresentableClaimIsKept`): claim, citations, limitation, method, conditions,
  uncertainties, and the precondition verbatim in the partial rule's undecidable leaf; the rule carries every
  uncertainty, also in its requested verification.
- **Scripted-exchange replay** (`TestCallThroughExchangeReplaysIdentically`): pending, answered, complete provenance on
  the record and each hypothesis, and an offline replay (cache only) yields a byte-identical record.
- **Bounded retry and recorded failure** (`TestBoundedRetryAndRecordedFailure`): the retry carries the violations and
  the rejected answer; a decode failure is retried; three violating answers give a `failed` record that validates; no
  answer is an error, not a record. `TestNoHypothesisIsRecorded`.
- **Policy isolation** (`TestPolicyIsolation`): the run record (findings, decision, hypotheses, everything) is
  byte-identical after formalization; the formalized hypotheses are only in the record.
- **Hand-off** (`TestFormalizedHypothesesFeedExportUnchanged`): through a real investigation store and a knowledge
  store, the rule becomes exactly one proposal (provenance rule `hypothesis:<hyp>@<inv>`, producer `formalize@v1`) and a
  whole-fact review item proposing exactly its assertion; the unrepresentable record is skipped with its limitation; no
  fact, no decision.
- **Evidenced defaults** (`TestEvidencedDefault`): a default citing upstream evidence the assessment established settles
  its uncertainty and is accepted by `applicability.RuleDefault` over the hypothesis' citations; rejected: a default
  citing evidence the assessment never established, invented evidence, or the environment; a discharge without a
  default; a discharge of a non-default uncertainty; a default neither stated nor kept.
- **The v8 path** (`internal/investigate/agent/mitigation_test.go` `TestCheckMitigationReadsOnlyItsEnv`, and the
  existing `TestV6MitigationRules`, `TestExportHypotheses`): the lifted function reads only its env; the session's
  results are unchanged.
- **Prompt** (`TestPromptIsGenericAndVersioned`): versioned with a digest; names no product, case, file, field or
  version.

### Smoke (unscored, non-development case)

Pre-computation over the stored v9 run `M1s-v9-shop-de-20261020-f6c23796` (only `investigation.json` and `bundle.json`
read): 10 assessments, inputs of 2–7 KB, 1–4 citable records each (an upstream one in 9 of 10), 3–6 basis ids, 1–7
uncertainties (unknowns and defaulted settings; v9 has no behaviour or preconditions). No model was called: no live
model run on any case (brief: the commander runs the demo and the gate).

### Known limits

- **Defaults need upstream evidence the assessment already holds.** A default the investigation never cited (for
  example a controller default no behaviour entry quotes) cannot be stated; the claim stays an unrepresentable record
  with an undecidable leaf. The floor cannot check that a discharged uncertainty is about the same field as the leaf
  carrying the default (the model's mapping); the default itself is verified independently.
- **The floor checks provenance and structure, not meaning.** It cannot tell that a rule states the claim correctly, that
  a limitation is the real one, or that the conditions are complete; `semverify`'s `semantic-check`, the rule exercise
  and the human reviewer do that.
- **"Kept" is global, not per claim.** Each uncertainty must be kept by some unrepresentable record; a rule's own
  `ProposedVerification` and requested verification carry all of them for the reviewer, but the floor cannot know which
  uncertainty a given rule depends on.
- **Environment basis offline** is the item's setting claims and validation records only; graph-subject references
  (object refs, node keys) that the live session can resolve are not offered.
- **Citable change text** only when a behaviour of the assessment quotes it (`change-text`); an assessment that relies
  on the change text without saying so cannot cite it.
- `claude -p` runs with the CLI's default thinking; `ModelVersion` is the joined `modelUsage` keys, as for synthesis.

## Lane `verify2` (D-0025): non-discriminating arms and verified scope refinement

Branch `p6di/verify2`. Two problems the commander found on a development run (brief `briefs/verify2.md`, generic
description). Everything is additive: stored proofs, formalization records, prompts and run records are unchanged and
still validate; nothing is re-judged retroactively.

### 1. Non-discriminating exercise arms

| Change | Where |
|---|---|
| **Classification.** Under a `default-changed` rule on a field of the fixture's subject kind, an arm predicted `new-behaviour` whose object sets that field explicitly cannot contrast FROM with TO by construction (an explicit new default already has the new behaviour at FROM; any explicit value is untouched by a default change). It is run and recorded with the reason (`ProofArm.NonDiscriminating`, additive) and never decides. Other change kinds are not classified: an explicit value can discriminate there. | `internal/investigate/verify/discriminate.go`, `internal/domain/proof.go` |
| **Decision.** Produced-object observation: when the principal `condition-absent` arm is non-discriminating, the omitted-field arms predicted to change (`condition-absent-omitted-N`) are the contrast; all must change for a pass; with none, `inconclusive` ("cannot discriminate"). Consumer observation: non-discriminating exposed arms are not counted; with no other exposed arm, `inconclusive`. Producers `verify.exercise@v2`, `verify.exercise-consumer@v2`. | `exercise.go` `EvaluateArms`, `consumer.go` `EvaluateConsumerArms` |
| **Validation.** A passing exercise needs the condition arm and a discriminating condition-absent arm answered (`ProofArm.Contrast`); the principal arm of a stored proof (never classified) counts as before. | `domain.ProofRecord.Validate` |

### 2. Scope refinement: proposed by formalization, established by observation

| Change | Where |
|---|---|
| **(a) Candidate.** Prompt `formalize/v2` (v1 is frozen and digest-pinned: it was run; `-formalize-prompt formalize/v1` replays it). The model sees the catalog's consumer roles but must not narrow a rule to one (floor violation); it may state evidenced role-field defaults (`roleDefaults`, checked like any default). The stage (producer `formalize@v2`) proposes deterministically, per rule hypothesis and catalog role R of its subject kind, a candidate when the run's TO-side consumer graph has established `reads-file` edges reading R's trust key from a Secret an object of the subject kind produces. Refined rule: the role fields join the exposure's own resource scope (`resource(K, E…, role)`) and the overlap becomes `resource(K, any(all(O…), all(outside)))` — one object scope each, since a resource condition holds when *some* object meets it (two scopes would pair a pinned CA with an unpinned leaf; `impact.TestRefinedRuleSpeaksOfOneObject`); a rule whose conditions are not one scope of the subject kind gets no candidate. The role leaves carry the evidenced default if any. The candidate cites the base rule's evidence (+ the default's), its `ProposedVerification` has method `consumer exercise (role R)` and `Refinement` (`domain.ScopeRefinement`: base hypothesis, base rule digest, role/outside fields, trust key, the trace edges with graph artifact, line). Its provenance call id is `<call>+refine:<role>` (derived from the base rule's call by the stage), so `ExportHypotheses` exports it as its own proposal and review item (one proposal per call and candidate). Without trace edges: no candidate; `Record.RefinementNotes` say why. | `internal/formalize/refine.go`, `floor.go`, `prompts/formalize/v2/`, `internal/domain/investigation.go` |
| **Catalog.** A consumer observation may declare its role-absent observation `outside:` (fields outside the role, subject fields, consuming objects, trust/reissue/verify, references). cert-manager `ca-trust`: a non-CA Certificate issued by a CA the arm declares; the consumer trusts its `ca.crt` (the issuing CA) and verifies its re-issued `tls.crt`. Roles reach formalization only from this file (`cmd/ri` `consumerRolesOf`). | `internal/investigate/verify/consumer.go`, `products/exercises/fixtures/cert-manager.yaml` |
| **(b) Verification.** The run source carries the refinement (`verify.MitigationOf`); the exercise must select the consumer it names and the catalog must declare `outside:` (else `not-run`). Outside-role arms use the role-absent observation. A pass also needs a role-absent arm observed without harm (verified at FROM and TO); undetermined ⇒ `inconclusive`, broken ⇒ `fail`. A role field without an evidenced default: its omitted-field arms are not built and the objects that omit it stay undecidable (a limitation, the evaluators keep them unknown) instead of failing the boundary fixtures. Every refinement proof carries the limitation that it covers only consumers that use the key material as the role's consumer does. | `verify.go`, `exercise.go`, `boundary.go`, `consumer.go` |
| **(c) Promotion.** Unchanged gate: a human only, a confirmed proof of the same rule digest, version bounds. | `internal/knowledge` (untouched) |

### Tests

- Non-discriminating: `TestNonDiscriminatingArm` (the development-run shape now passes on the omitted arm; the explicit arm never decides; omitted arm quiet ⇒ inconclusive; condition arm changed ⇒ fail; only contrast non-discriminating ⇒ inconclusive; an explicit value under `behavior-changed` does discriminate), `TestNonDiscriminatingClassification`, `TestEvaluateConsumerArmsNonDiscriminating`, `domain.TestProofValidateRejectsAnOutcomeTheMethodsDoNotSupport`.
- Candidates: `TestRefinementProposedOnlyWithTraceEvidence`, `TestNoRefinementWithoutTraceEvidence` (no graph, another key, inferred and missing edges, and formalize/v1 frozen: same input digest, producer v1), `TestRoleDefaults`, `TestModelRoleNarrowingRejected`, `TestRefinementMustRefineARuleOfTheRecord`, `TestRefinementCandidateIsExported` (its own proposal and review item, no fact); both prompts replay from the cache byte-identically; v1's digest is pinned.
- Verification on synthetic fixtures: `TestRoleAbsentObservation`, `TestRefinementExercise` (confirmed; refuted by the role-absent or the shielded consumer; inconclusive when the role-absent arm is not observed or never established, or without harm in the role; an unevidenced role default stays undecidable; with a default the omitted role field is exercised on the role-absent observation), `TestRefinementPromotionGateRequiresAHuman` (inconclusive never opens the gate; a proxy never promotes; a human with a confirmed proof does), `TestStoredConsumerProofsStillVerify`.
- Kind (opt-in, skips without kind/docker): `RI_VERIFY2_KIND=1 RI_VERIFY2_STATE=<scratch state> go test ./cmd/ri -run RefinementKind -v -timeout 60m` (confirmed / refuted / inconclusive on the real controller and the catalog fixture; clusters `ri-verify-*`, deleted).

### Known limits

- A refinement is only as good as the catalog's consumer: it covers consumers that use the key material as the role's consumer exercise does (for example, verifying peers through the issuing CA); a consumer that pins a leaf is not covered. The trace shows what workloads read, not how they verify.
- One role-absent arm without harm suffices; further role-absent arms that were not observed are recorded, not required.
- The candidate is proposed per catalog role of the subject kind, not per assessment claim: the model does not judge whether the harm is role-specific; the observation does.
- Fixture clusters keep the runtime's `ri-verify-` prefix (the isolated-cluster guard recognises it), not `ri-verify2-`.
