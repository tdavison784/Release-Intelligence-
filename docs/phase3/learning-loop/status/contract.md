# Lane `contract` — status

**State: done** (DESIGN.md final; types, ports, Validate rules, schemas, and tests committed).

## Done
- `docs/phase3/learning-loop/DESIGN.md`: the semantic model, condition language (bound to the
  envparse/envinv APIs), entities + identity, restatement clusters + statement anchors, trust ladder,
  UNKNOWN reasons, cross-product, routing, bulk review, rendered-manifest diffs, eval per verification
  level + transfer, storage, lane interface map, and both demonstration paths (rotationPolicy → REVIEW;
  HTTP01 × ingress-nginx → ACTION).
- `internal/domain/semantic.go` (+ `semantic_test.go`, table-driven, offline):
  - The vocabulary: Subject/ChangeSpec (with replacedBy), the three-valued Condition tree (all, any,
    not, resource scope; field, text-line, ref; values-key, gvk-in-use, image-in-use, cli-flag,
    env-var, feature-gate, product-version, cluster-version, edge-from-version, rendered-change,
    undecidable), Consequence (the class follows the kind), Aspects, UnknownReason, VerificationLevel.
  - The entities, each with `Validate()`: SemanticCandidate (restatement cluster), SemanticProposal,
    ValidationResult, ReviewItem, ReviewDecision (batch fields), VerifiedFact.
  - Helpers: `ValidateFactBasis` (proves that a proxy cannot pose as a human), `ChangeAnchor`,
    `IsUmbrella`, `AttachesByAnchor`, `RenderProvenance`, the `KnowledgeRecord` envelope.
- `internal/domain/impact.go`: additive `ImpactFinding.unknownReason`, `ImpactFinding.knowledge`,
  `DimensionProducts`. `Validate()` enforces the trust ladder and supersession; the schema mirrors it.
- `internal/knowledge/{doc,api}.go`: the ports (Store, Proposer, Validator, Queue, Snapshot,
  ReviewContext, RouteResult, LoopMetrics with batch vs individual stats).
- `schemas/knowledge-record.schema.json` (new); the impact-report, upgrade-edge and release schemas
  were regenerated (new optional fields only).

## Decisions (and why)
- **One assertion type for proposals, corrections and facts, with four aspects.** Verification,
  agreement and review all work per aspect. Classification is never asserted by a model: the
  consequence kind determines it (`Kind.ExposedClass()`), and the fact stores the class explicitly.
- **Three-valued logic.** `not(false)` is true only with examined evidence; `text-line none` and
  withheld values are unknown. Absence of input is never negated into evidence.
- **ACTION from knowledge requires all four aspects at a trusted level** (deterministic or human),
  including applicability. This is stricter than the commander's minimum: an unverified condition
  must not decide mandatory work.
- **Proxy-verified facts** are capped at review-required, cannot carry high confidence, and never
  produce not-affected.
- **Fact identity is product + release + assertion.** Attachment uses statement anchors (no chg- ids)
  plus subject restatement and duplicate grouping (done by the applicability lane). Umbrellas are
  excluded.
- **`unknownReason` is optional in `Validate()` for now**, so the e2e goldens stay byte-identical.
  The applicability lane makes it mandatory once the join assigns it.
- **Storage is JSON files under `knowledge/`.** No new dependency.

## Files touched outside ownership (all additive)
- `internal/domain/evidence.go`: `Evidence.Render *RenderProvenance` (optional; the render addendum).
- `internal/domain/schemagen/{gen.go,meta.go}`: knowledge schema target, enums, trust-ladder
  conditionals.
- `docs/ACTION_CLASSIFICATION.md` §8 (knowledge findings); `docs/ARCHITECTURE.md` (package + schema list).

## Requests to other lanes
- envinv: an `inventory.yaml` `complete: true` declaration → `Environment.InventoryComplete` + evidence
  (DESIGN §1.3).
- envparse/envinv API suggestions (non-blocking): see DESIGN §1.3, "Rename / API suggestions".
- applicability: assign `unknownReason` everywhere, then make it mandatory; V2 `crd:storage-changed`;
  `AttachedChanges` per DESIGN §2.6.

## Open questions for the commander / user
- D11 stands as decided: rotationPolicy → behavior-change → REVIEW. Changing it is the user's call.
- The ingress-nginx ACTION example (DESIGN §10, path 2) is illustrative. Whether that fixture's fact
  is accepted is a reviewer decision, not a design one.

## Tests
- `go build ./... && go vet ./... && go test ./...` pass. The e2e goldens are unchanged (no knowledge
  supplied ⇒ byte-identical).

---

## contract-2 (branch `p3ll/contract-2`, from `p3-learning-loop` @ 2007556) — done

The render-mission contract change (RENDER-MISSION R6, R10–R12, R19; briefs/render.md §7):
- **New `consensus` verification level.**
  - Basis: ≥2 `sp-` proposals from independent model families asserting the fact's aspect digest,
    plus an optional `val-` that does not refute it.
  - **Untrusted.** It is treated exactly like proxy: ≤ review-required, never not-affected or
    action-required, confidence ≤ medium (enforced in `ImpactFinding` validation).
  - Ordering for `Level()`: deterministic ≡ human > consensus > proxy. Consensus is a structural
    agreement test rather than one model's judgement; the ladder treats both the same anyway.
  - `AtLeast(consensus)` admits deterministic, human and consensus.
- **Independence = distinct model families** (`ModelFamily`, `IndependentModels`). This deliberately
  narrows the request's "distinct providers or families": the same model behind two gateways, or
  Opus + Sonnet, is not two opinions.
- **`ValidateFactRecords(fact, FactRecords{Validations, Decisions, Items, Proposals})`** resolves
  consensus bases. `ValidateFactBasis` keeps its signature (a non-breaking wrapper), and it fails on
  consensus facts because it resolves no proposals.
- **`VerifiedFact.AutoApproved`**: set exactly when every aspect is deterministic or consensus (this is
  validated). **Behaviour change:** all-deterministic facts must now carry `autoApproved: true`.
- **`ValidationResult.RenderRelation`**: confirmed-by-render (needs a confirmed check plus rendered
  evidence) | contradicted-by-render (needs a refuted check) | not-visible-in-render /
  render-not-applicable (inconclusive checks only).
- **`SemanticCandidate.Renderability`**: render-verifiable | partially-render-verifiable |
  not-render-verifiable.
- **`knowledge.FactMetrics`**: auto-approval audit fields (R19).
- **Docs and schemas:** DESIGN.md §2.3, §2.6, §4, §6 (auto-approval route), §7 (consensus level row,
  auto-approval and render-relation metrics); ACTION_CLASSIFICATION §8; schemas regenerated.
- **Tests:** `go build`, `go vet` and `go test ./...` pass; goldens unchanged.

---

## contract-3 (branch `p3ll/contract-3`, from `p3-learning-loop` @ e93db45) — done

The product-owner decisions PO-1 and PO-2 are recorded verbatim in `docs/phase3/learning-loop/DECISIONS.md`.

**PO-1 — consensus is separate calls agreeing.**
- `Provenance.CallID`: required on proposals, part of `ProposalID`, and forbidden on deterministic
  provenance.
- `SeparateCalls` replaces `IndependentModels`.
- `ConsensusScope` labels each consensus `cross-model` or `same-model`. The label lives on
  `AspectVerification` and `KnowledgeRef`, and `ValidateFactRecords` checks it.
- No call may be counted twice.

**PO-2 — consensus may produce ACTION REQUIRED.**
- `SuggestedClass` may be `action-required`. It is a request and needs an action-eligible consequence.
- `VerifiedFact.ConsensusAction`: conditions (a), (b) and (d), enforced by `Validate` and
  `ValidateFactRecords`.
- `KnowledgeRef.ConsensusAction` / `ActionLabel()` = "model consensus".
- `ImpactReport.Validate` and the schema allow consensus ACTION only with `consensusAction`.
  Condition (c) is the unchanged affected-class rule. NOT AFFECTED stays trusted-only. Proxy stays
  capped. High confidence on a consensus finding is allowed only for a consensus-action ACTION.

**Docs:** DESIGN §2.2, §2.6, §4 (a three-column ladder), §6 (a consensus-action route with 100%
audit), §7 (per-level `falseActionRate`/`actionFindingEvidence`; gates on the combined output; new
metrics); ACTION_CLASSIFICATION §8; MISSION Goal 21 note. `knowledge.FactMetrics` gains the
consensus-by-scope and consensus-action audit fields.

**Edits in the knowledge lane's package, minimal and marked `CONTRACT-CHANGE(contract-3)`:**
- `autoapprove.go`: `SeparateCalls` instead of `IndependentModels`.
- `decide.go`: sets the consensus scope label when minting.
- `autoapprove_test.go`: the family test is rewritten to the PO-1 meaning.
- `fixtures_test.go`, `cmd/ri/knowledge_test.go`: call ids added.

**Left for the knowledge lane:**
- `route.go` still counts agreement signals per model name. Under PO-1 it should count separate
  calls.
- Minting `ConsensusAction` facts and the 100% audit are not implemented yet.

**Left for the applicability lane:** consensus ACTION findings (label, KnowledgeRef copying) and the
per-level eval reporting.

**Tests:** `go build`, `go vet` and `go test ./...` pass; goldens unchanged.

---

## contract-4 (branch `p3ll/contract-4`, from `p3-learning-loop` @ de4bf57) — done

PO-3 and PO-4 are recorded in DECISIONS.md (verbatim in substance, from the commander relay).

**PO-3 — unset changed defaults and new keys decided by rendering.**
- `ImpactCheck.Render{outcome attributable-change|no-attributable-change|unavailable, key,
  counterfactual, reason}`, set exactly on `render`-dimension checks.
- Three rule constants in domain:
  - `impact:values-default-applies`: review-required, needs a rendered-change match with
    environment-render evidence;
  - `impact:values-default-no-effect`: not-affected, needs a no-attributable-change check citing
    environment-render evidence;
  - `impact:values-default-unrendered`: not-affected, needs a visible unavailable check with a reason.
- Enforced by `Validate()` and the schema. The render and applicability lanes implement the emission.
- **Goldens:** these rules do not exist in the engine yet, so nothing changes until those lanes
  switch `values-unset` for default-changed/added keys over to them. That switch will change goldens
  and eval cells (TRUSTFIX §5 blast radius); it is theirs to state.

**PO-4 — superseded-upstream and narrow refinement.**
- `ConsequenceSupersededUpstream` (review-required, not action-eligible), with a semantic vocabulary
  entry.
- `RuleKnowledgeRefined`, `ImpactFinding.RefinedFrom`, `KnowledgeRef.Subject`, `Subject.CoversMatch`.
- `validateRefinements` rules: trusted facts only; affected → affected and never not-affected or
  unknown; the class must change; the original must be deterministic and must be replaced; every
  match lies within the fact's subject.
- Adversarial tests cover: proxy refining, consensus-action refining, refining to not-affected or
  unknown, a sibling-key mismatch, a wrong family, the original kept, and refinedFrom without its rule
  (and the reverse).

**Edits outside ownership (marked `CONTRACT-CHANGE(contract-4)`):**
- `internal/semantic/vocabulary.go`: one consequence entry, which the vocabulary test requires.
- `internal/domain/impact.go` knowledge rule allow-list (added by the applicability lane): the refined
  rule.

**Docs:** DESIGN §1.4 and §4; ACTION_CLASSIFICATION §5 and §8; IMPACT.md values table.

**Tests:** `go build`, `go vet` and `go test ./...` pass; goldens unchanged.

---

## contract-5 (branch `p3ll/contract-5`, from `p3-learning-loop` @ 9d5126a) — done

These answer the proxy lane's two open contract questions.

**Prose-only corrections.** A `correct` decision that changes only the consequence
`Statement`/`Remediation` is now valid.
- It is labelled exactly `[corrected, improved-statement]`. `improved-statement` is a new
  `FeedbackLabel`. It is not a `wrong-*` label, so it is not a model error for the typed-accuracy
  metrics.
- A typed correction may add `improved-statement` only when the prose also changed.
- A change only to `SemanticAssertion.Statement` is still refused.
- Original and Corrected are both kept. The fact id is unchanged (digests exclude prose), and the fact
  takes the corrected prose.
- **I chose a new label rather than `wrong-consequence`.** Reusing `wrong-consequence` would charge
  models with an error in the G15 accuracy metrics when the typed consequence was right.

**`Provenance.Provider`.** AI-only, optional, and forbidden on deterministic provenance.
`ProviderName()` reads either this field or the legacy `Rule "provider:<name>"`. On a proposal it must
agree with `SemanticProposal.Provider`.

**Follow-ups:**
- **dashboard:** `internal/reviewui/decision.go` still refuses a correction whose digest is unchanged.
  It should accept consequence-prose-only edits and label them `[corrected, improved-statement]`.
- **knowledge:** if the FileStore treats a fact's prose as immutable under the same id, it must accept
  the corrected prose from an `improved-statement` decision.
- **proxy, semantic:** set `Provenance.Provider` when writing.
- **metrics:** report prose edits separately.

**Docs:** DESIGN §2.2, §2.5 and §7; schemas regenerated (new label enum; `provider` on provenance).
**Tests:** `go build`, `go vet` and `go test ./...` pass; goldens unchanged.
