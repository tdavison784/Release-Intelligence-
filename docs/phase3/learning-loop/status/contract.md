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
