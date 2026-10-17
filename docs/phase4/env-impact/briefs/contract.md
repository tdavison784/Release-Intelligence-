# Lane `contract` — evidence, graph and investigation contracts (architect) — MISSION P1

Model: Opus. Branch `p4ei/contract`, worktree `.claude/worktrees/p4ei-contract`.

You are the architect for Phase 4. Every other lane builds against what you define, so your output must
be precise, minimal and stable. Read first: `docs/phase4/env-impact/MISSION.md` (normative),
`docs/phase4/env-impact/FLEET.md` (binding), `docs/ARCHITECTURE.md`, `docs/IMPACT.md`,
`docs/RENDER.md`, `docs/RENDER-FIRST.md`, `docs/ACTION_CLASSIFICATION.md`, `docs/POLICY.md`,
`docs/KNOWLEDGE.md`, `docs/LINKED_EVIDENCE.md`, `docs/phase3/learning-loop/DESIGN.md` (the existing
trust ladder and semantic model), `docs/phase3/OUTCOMES.md`, then the code: `internal/domain`
(impact.go, provenance.go, evidence, semantic.go), `internal/env` (env.go, repo.go, resources.go,
inventory.go), `internal/impact` (build.go, renderfirst.go, knowledge.go), `internal/render`
(model.go, pair.go, diff.go, evaluate.go, validator.go), `internal/linkedev`, `internal/policy`,
`internal/llm` (exchange), `internal/app` (composition root).

## The key fact you must design around

Much of the mission already exists in some form: upstream change evidence (`UpgradeEdge`), repo-mode
environment parsing, helm/kustomize render-and-diff at environment scope, a deterministic impact join
with UNKNOWN reasons, verified knowledge facts and a trust ladder, and a policy verdict. Phase 4 must
**build on these, not beside them**. Your first section is an honest gap map.

## Owns

- `docs/phase4/env-impact/DESIGN.md` (new; normative after merge)
- `internal/domain/investigation.go` (+ `_test.go`): the new contract types
- `internal/investigate/api.go` + `doc.go` (new package; ports/interfaces only — no implementation)
- regenerated `schemas/*` (add `investigation-*.schema.json` via schemagen)
- `eval/investigations/FORMAT.md` (the case format; coordinate with the `groundtruth` lane — see below)
- `docs/phase4/env-impact/status/contract.md`

## Deliverables

1. **DESIGN.md §0 Gap map.** For each MISSION section (§A1–§A7, P1–P6, AC1–AC8): what exists today
   (package, type, function, doc), what is missing, what will be reused vs. extended vs. new. Be
   concrete and cite file:line. Flag existing behaviour that conflicts with the mission (e.g. the
   five-class impact classification vs. the four-class finding; "LLMs never required for a runtime
   impact decision" vs. an agent-driven investigation) and resolve each conflict explicitly.
2. **§1 First-milestone scope** (MISSION "Scope and Delivery"). Default to recommend unless the code
   shows a better choice: dependency type = Helm-chart-delivered Kubernetes add-ons already in the
   catalog; environment type = a GitOps repository (Helm values, Kustomize overlays, Argo CD
   `Application` / Flux `HelmRelease` definitions, plain manifests of consuming workloads) plus an
   optional product inventory; live-cluster observation = optional, read-only, off by default. Pick
   2–3 products with the richest existing evidence (cert-manager, ingress-nginx, and one of
   kube-prometheus-stack / argo-cd). State explicitly what is out of scope.
3. **§2 Contracts** (`internal/domain/investigation.go`), each with `Validate()`, content-derived IDs,
   provenance, JSON tags, schema generation:
   - `ChangeEvidenceRecord` (§A1): from/to, the structured breaking-change items (referencing existing
     `domain.Change` / evidence IDs rather than copying them), migration notes, schema/API deltas,
     chart/kustomize deltas, before/after manifest artifact refs, compatibility requirements, and a
     per-source **collection status** (ok / unavailable / partial / not-applicable, with reason).
   - `EnvironmentSnapshot` (§A2): environment id, the source refs (repo + commit digest, file digests,
     inventory, optional live observation with timestamp + scope), parsed facts by reference, and
     collection status per input. Immutable: identified by digest.
   - `ArtifactRef` + `ArtifactManifest` + `EvidenceBundle` (P1 deliverable): a change- and
     environment-scoped, content-addressed bundle on disk (layout, manifest fields: id, kind, media
     type, digest, size, origin, tags/keywords for search, which change items it relates to). The
     agent searches the manifest, never gets the whole bundle in context (§A4).
   - Graph (§A3): `GraphNode` (kinds: product/chart release, helm release, kustomization, workload,
     service, ingress, config object, CRD/GVK, custom resource, external integration, …: generic,
     declarative vocabulary), `GraphEdge` (relationship kinds: deploys, renders-to, configures,
     overrides, references, consumes-api, depends-on, …) each with evidence refs and a
     **verification status** (`established` from evidence / `inferred` / `missing` = known-unknown),
     `ImpactPath` (ordered node/edge ids from an exposed component to a consumer, with evidence and
     the mitigating override if any), and `TraversalResult` (roots, bound/depth used, visited, paths,
     unresolved edges, truncation flag + reason). DFS must be deterministic (stable ordering) and
     bounded; specify the bound and what "complete traversal" means for fail-closed.
   - `ValidationResult` for §A5/P4 (reuse or extend the existing render/knowledge validator result
     where sensible): validator id@version, kind (override-inspection, render-diff, schema-compat,
     static-reference, runtime), inputs by artifact digest, outcome (pass/fail/error/not-run), level
     (`deterministic` / `observed`), and evidence.
   - `AgentAssertion` / `Hypothesis` / `EvidenceRequest` (§A4): what the agent claims, cites, asks
     for; always with AI provenance; never a validation result.
   - `ImpactFinding` with exactly the four MISSION classes (§A6) and every required field (reasoning,
     evidence, environment + components, paths, validation results, uncertainty + evidence gaps,
     artifact refs, remediation / next investigation), plus `ApprovalEligibility`.
   - `Investigation` (the run record): ids of the bundle, snapshot, traversal, assertions,
     validations, findings, policy decision, human review state, and an append-only audit trail.
4. **§3 Classification and approval policy** (§A6, Principle 7, AC6, AC8): the deterministic rules that
   turn evidence + traversal + validations + agent assertions into the four classes, and when a finding
   is eligible for automated approval vs. requires human review. Required principles: `Not Affected`
   needs complete traversal within scope, no unresolved edge on any path from an exposed component,
   and deterministic/observed validation for each breaking item in scope (or an established override);
   an agent assertion can only ever *raise* concern (or explain), never lower a class without a
   validation result; LLM confidence and model agreement are never inputs to approval. Specify how the
   four classes relate to the existing five-class `ImpactReport` classification and the
   ACTION REQUIRED contract (never weakened), and how `Validate()` enforces the rules.
5. **§4 Investigation agent design** (§A4, P3): the tool surface (search manifest, read artifact,
   traverse/expand graph, request evidence, request validation, record hypothesis/assertion,
   submit findings) as a Go port in `internal/investigate/api.go`; how it is exposed to a model given
   **no API keys are available** (recommend: an MCP stdio server `ri investigate mcp` driven by
   `claude -p --mcp-config … --output-format json`, plus the existing `-llm-exchange` for replay and
   tests); budgets (turns, tool calls, tokens, wall-clock); prompt versioning; how agent output is
   validated (every citation must resolve to a bundle artifact; unknown ids reject); replay for
   reproducibility.
6. **§5 Orchestration boundary** (§A7, P5): the activity interfaces (pure Go ports) the Temporal layer
   will call — collect-change-evidence, build-snapshot, build-graph, traverse, investigate, validate,
   classify, await-human-review — with idempotency keys (content digests), retry classes
   (transient vs. permanent), timeouts and the audit events. Temporal types must not leak into these
   ports. The `orchestrate` lane is spiking the SDK in parallel; read its status file before finalising.
7. **§6 Evaluation design** (P6): metrics (false-negative rate for breaking, false-positive rate,
   evidence/dependency coverage, unresolved/incomplete rate, agreement with observed outcomes, human
   review rate, auto-approval eligibility rate, per verification level) and a **pre-registered gates
   file** `eval/investigations/gates.yaml` (conservative; FN rate must be the strictest). Commit the
   gates file in this lane; it is frozen once merged.
8. **§7 Lane plan for waves 1–3**: packages, owned files, signatures each lane implements, and the
   dependency order, so the commander can write briefs directly from it. Suggested lanes (adjust):
   `bundle` (P1 collection + bundle writer), `graph` (P2 discovery + DFS), `validate` (P4 validators),
   `agent` (P3 tools + runner), `classify` (§A6 policy + findings), `orchestrate` (P5 workflows),
   `eval` (P6 harness + report). Keep each lane to one package boundary where possible.
9. **`eval/investigations/FORMAT.md`**: the ground-truth case format. The `groundtruth` lane is
   authoring cases in parallel with a draft format in its status file; read
   `.claude/worktrees/p4ei-groundtruth/docs/phase4/env-impact/status/groundtruth.md` (read-only — never
   edit another worktree) and converge on one format; the expectations themselves are theirs.

Keep the contract minimal: types and interfaces the next waves need, nothing speculative. Tests for every
`Validate()` rule. When done: `go build ./... && go vet ./... && go test ./...` green, schemas
regenerated, status file current, final message `LANE DONE:`.
