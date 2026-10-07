# Lane `contract` — the semantic-knowledge contract (architect)

You are the architect for the Phase 3 learning loop. Every other lane builds against what you define, so
your output must be precise, minimal, and stable. Read first: `docs/phase3/learning-loop/MISSION.md`,
`docs/phase3/learning-loop/FLEET.md`, `docs/ARCHITECTURE.md`, `docs/ACTION_CLASSIFICATION.md`,
`docs/IMPACT.md`, `docs/IMPACT-ENRICHMENT.md`, `docs/phase3/OUTCOMES.md`, `eval/FORMAT.md`, and the code
in `internal/domain` (esp. impact.go, enrichment.go, provenance.go), `internal/impact/build.go`,
`internal/impactenrich`, `internal/env/env.go`, `internal/eval/compare.go` (how applicability links are
scored: a link is "hit" only when an AFFECTED-class finding joins a change that matches the expected item).

Branch `p3ll/contract`, worktree `.claude/worktrees/contract`.

## Owns

- `docs/phase3/learning-loop/DESIGN.md` (new)
- `internal/domain/semantic.go`, `internal/domain/semantic_test.go` (new)
- `internal/knowledge/api.go` (new — interfaces/ports only, plus doc.go)
- additive fields on `domain.ImpactFinding` for the UNKNOWN reason (Goal 18) and the knowledge link
- regenerated `schemas/*`

## Deliverables

1. **DESIGN.md** — the learning loop end to end (MISSION Goal 25), concretely:
   - The semantic knowledge model (Goal 1): subject families as a generic, declarative vocabulary
     (crd-field, helm-value, gvk, feature-gate, image, config-key, rbac-permission, cli-flag, env-var,
     compatibility-boundary, api-endpoint, protocol-behavior, terraform-attribute, product-relationship,
     migration), change types, before/after state, the **applicability condition language** (a small
     declarative predicate set that the deterministic engine can evaluate against `env.Environment`:
     e.g. field unset/set/equals on resources of a GVK, values key present/absent/equals, feature gate
     enabled in values/args, product present with version in range, apiVersion in use, image in use;
     plus explicit "not statically decidable" → UNKNOWN with a reason), and consequence types.
   - Entity lifecycle and identity: SemanticCandidate (from an UpgradeEdge change + evidence) →
     SemanticProposal (one per model, full AI provenance incl. provider, never collapsed) →
     ValidationResult (deterministic validators; which fields each can prove) → ReviewItem
     (question type per Goal 10) → ReviewDecision (accept/reject/correct/need-more-evidence/defer,
     labels per Goal 13, original + corrected value, reviewer + reviewerKind human|proxy, reason,
     timestamps, duration) → VerifiedFact (release-scoped, reusable knowledge entity with verification
     level `deterministic|human|proxy` and the decision/validation records that justify it).
     Content-derived stable IDs. How a fact attaches to an UpgradeEdge in a future run (product +
     introducing release + change fingerprint/evidence), so facts are reused across environments
     (Goal 12) and across re-ingestion.
   - **Trust ladder → classification** (Goals 20, 21): exactly when a fact + env evaluation may yield
     ACTION REQUIRED vs REVIEW vs INFORMATIONAL vs NOT AFFECTED vs UNKNOWN. Required principle: ACTION
     requires a fact whose subject/change AND consequence are verified at `human` or `deterministic`
     level, an exposure proven deterministically from environment evidence (both evidence chains), and
     high confidence. Proxy-verified facts are capped (propose: at REVIEW) and flagged. Model proposals
     alone never change a classification. Specify how this composes with the existing join (a knowledge
     finding replaces/upgrades the UNKNOWN record for that change; never downgrades a stronger
     deterministic class) and how `ImpactReport.Validate()` enforces it.
   - UNKNOWN reasons (Goal 18) on every UNKNOWN finding, and how the existing join assigns them.
   - Cross-product (Goal 19): how facts say "requires product P version range R" and how that is
     evaluated against the environment product inventory (the `envinv` lane is adding an explicit
     product inventory to `internal/env` in parallel — define the predicate against
     `env.Environment.Installed` / a product-inventory accessor and note the assumption).
   - Routing hypotheses (Goal 16) and the metrics each entity must support (Goals 4, 15, 24).
   - Storage: a lightweight, durable, diff-able local store. Default recommendation: committed
     JSON/YAML files under a top-level `knowledge/` directory (proposals, review items, decisions,
     facts — one file per entity or per release), plus a stdlib-only HTTP dashboard. Justify or
     change this.
   - How the eval measures the loop honestly: per verification level (deterministic-only,
     +proxy, +human), with the rule that facts are authored/reviewed without consulting eval
     expectations, and a held-out check that facts transfer to environments they were not reviewed
     against (Goal 12).
   - **Lane interface map**: for each wave-1 lane below, the package, files, and exported function
     signatures it implements, so they can work in parallel without talking to each other:
     `semantic` (candidate generator + multi-model proposal layer, prompts, provider-agnostic proposer
     port — must allow plugging in non-Claude providers such as GLM or a typed "System One" model like
     TypeSafe's Jev later; prefer enumerated/typed answer schemas), `validate` (deterministic
     validators over ingested snapshots: CRD schemas/defaults, Helm values/schema, compatibility tables,
     images), `knowledge` (store impl, review queue, decisions→facts, feedback dataset export,
     model/agreement/cost metrics), `applicability` (fact × environment evaluation in
     `internal/impact`, UNKNOWN reasons, trust ladder enforcement, eval integration), `dashboard`
     (`ri review serve`), `groundtruth` (dataset expansion per Goal 22: case.yaml format additions for
     expected semantic subject/applicability/consequence/env evidence).
2. **internal/domain/semantic.go** — the types above with `Validate()` methods enforcing the invariants
   (AI proposal must carry complete provenance; a VerifiedFact must carry ≥1 justifying validation or
   decision record; proxy decisions can't masquerade as human; corrected decisions keep both values;
   only allowed enum values). Table-driven tests. Keep it boring and explicit.
3. **internal/knowledge/api.go** — the store/queue ports (interfaces only) the other lanes implement
   against.
4. Additive `ImpactFinding` fields (UNKNOWN reason; knowledge fact reference + verification level) and
   their `Validate()` rules; regenerate schemas; keep existing tests green and the deterministic
   report byte-identical where no knowledge is supplied (e2e goldens).

## Inputs arriving while you work

The `analysis` lane is classifying every current ACTION→UNKNOWN failure (why unknown, which subject
family, what env evidence decides it). The commander will forward its findings
(`docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md`) when ready — fold them in; don't block on them for the
first DESIGN.md draft. Write the DESIGN.md draft and commit it early (the commander reviews it before
the wave-1 lanes start), then implement the Go types.

Model: you are Opus — this is the lane where judgement matters most. Prefer fewer, sharper constructs.
