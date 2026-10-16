# Status — lane `contract` (Claude Opus 5.5, agent ri-contract)

Branch `p4ei/contract`, worktree `.claude/worktrees/p4ei-contract`. Brief: `briefs/contract.md`.
State: **DONE** (2026-10-16). Resume from: this file + `docs/phase4/env-impact/DESIGN.md` + `git log`.

## Done

- `docs/phase4/env-impact/DESIGN.md` — §0 gap map (file:line, reuse/extend/new, 8 conflicts resolved),
  §1 milestone scope, §2 contract, §3 policy + approval + five-class mapping + enforcement table, §4 agent,
  §5 orchestration boundary, §6 evaluation, §7 lane plan (waves 1–3 with signatures and order).
- `internal/domain/investigation.go` — every §2 type with content-derived ids, `Validate()`;
  `Investigation.ValidateAgainst(bundle)`; `TraversalResult.ValidateAgainst(graph)`.
- `internal/domain/investigation_test.go` — valid cert-manager-shaped fixture + a mutation test per rule
  (bundle, change record, snapshot, artifact/manifest, rendered changes, graph, traversal, validation,
  agent records, findings, run-level policy, review, audit chain, against-bundle resolution); sample
  documents `internal/domain/testdata/sample-{evidence-bundle,investigation,investigation-graph}.json`
  (golden; `RI_UPDATE_SAMPLES=1 go test ./internal/domain -run TestInvestigationSamples`).
- schemas: `schemas/{evidence-bundle,investigation,investigation-graph}.schema.json` (schemagen targets,
  enums, provenance-variant patches); schemagen tests validate the samples and reject AI findings, AI
  validations, deterministic/uncited assertions and a fifth class.
- `internal/investigate/{doc,api,api_test}.go` — ports: ChangeCollector, SnapshotBuilder,
  EnvironmentRenderer, BundleAssembler, GraphBuilder, ExposureMapper, Traverser, Validator,
  Investigator, Classifier, ReviewApplier, ArtifactStore, RecordStore, Tools (12 MCP tools, diff tools
  first), AgentDriver; Config digest / idempotency keys; retry classes (`ClassOf`); `QuoteOccurs`.
  No Temporal import (`go list -deps` checked).
- `eval/investigations/FORMAT.md` — converged with groundtruth's draft (input.yaml vs blind
  expected.yaml; contract vocabulary mapping; fictional examples only).
- `eval/investigations/gates.yaml` — pre-registered before any scored run; FN rate `max 0` (strictest);
  `minSample` → INSUFFICIENT-DATA, never a silent pass; autonomy criteria (20+20 observed decisions).
- Product-owner clarification ("DFS" = diffs) absorbed: `RenderedChange` is the centerpiece evidence
  (citable, correlated to change items; findings must list their item's correlated env changes;
  not-affected must show each harmless; uncorrelated changes / no complete render block auto-approval);
  graph/DFS kept light (lane `relations`).

## In progress / next step

- Nothing. Commander: review DESIGN.md (gap-map honesty, §3 fail-closed policy, §3.5 ACTION REQUIRED
  never weakened), merge (clean against `p4-env-impact` @ 80cfd819 — checked with `git merge-tree`; the
  merged tree builds and `internal/{domain,investigate,orchestrate}` tests pass), then write wave-1
  briefs from DESIGN §7.

## Decisions (with why)

- Renamed brief types that collide with existing domain names: `BundleManifest` (ArtifactManifest is an
  ArtifactType constant), `ValidationRecord` (ValidationResult is release-level and forbids environment
  evidence), `InvestigationFinding` (ImpactFinding is the five-class join finding).
- Edges point in the direction impact propagates (From = provider, To = consumer); unresolved = inferred
  or missing; traversal complete ⇔ not truncated and every path established; bound 8/500/5000.
- Validation outcome is relative to the safety property (pass = not reached/broken; fail = demonstrated
  and breaking). A validator that cannot decide returns not-run/error, never pass.
- Affected always cites a failing deterministic/observed validation; decisive join verdicts are wrapped as
  ValidationRecords (lane validate); `values-default-unrendered` wraps as not-run (approval needs
  evidence); only trusted (deterministic/human) knowledge facts are wrapped.
- Agent records can only raise concern; `claims-mitigation` is an Assumption that blocks not-affected
  until validated; proposed edges are inferred and traversal re-runs after the agent stage.
- Automated approval: shadow-only in milestone 1; eligibility additionally needs the agent stage
  completed, a complete traversal, ≥1 complete env render, zero uncorrelated env rendered changes, and
  (if configured) a tier-policy auto-pass.
- Scope: cert-manager + ingress-nginx (the groundtruth pairs), argo-cd third; kube-prometheus-stack
  deferred to milestone 2. Live observation: contract only, off, unimplemented.
- Investigation id = bundle id + config digest (identifies the request → Temporal workflow id).
- Stage names are `domain.Stage`; the orchestrate spike's three differing names are renamed in wave 2.

## Files touched outside ownership (all additive)

- `internal/domain/schemagen/gen.go` (3 targets), `meta.go` (enums, field patches, 3 descriptions),
  `schemagen_test.go` (2 tests) — needed to generate and test the new schemas.
- `internal/domain/testdata/sample-*.json` (new; pinned by a domain test, read by schemagen tests).
- `docs/ARCHITECTURE.md` — package list (`internal/investigate`) and schemas line.

## Contract changes

n/a (this lane defines the contract). After merge, changes go through the commander (FLEET.md).

## Open questions

1. **User**: the agent stage sends environment configuration (values, manifests; Secret values redacted)
   to the model provider. DESIGN makes it opt-in per run (`--agent`). Acceptable?
2. **Commander**: auto-approval eligibility requires the agent stage to have completed (DESIGN §0
   conflict 2: the agent can only block, so this only makes approval rarer). Confirm.
3. **Commander**: I never received the product-owner clarification as a message in my pane; I read it
   from `MISSION.md`/`COMMANDER.md` on `p4-env-impact` and redesigned around it before finishing.
   This branch is not merged with `p4-env-impact` (FLEET: only on the commander's instruction).

## Tests

`go build ./... && go vet ./... && go test ./...` green on this branch (33 packages ok, 0 fail).
No kind clusters, servers, bin/ or scratch output were created in the worktree.
