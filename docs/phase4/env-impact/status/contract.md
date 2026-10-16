# Status — lane `contract` (Claude Opus 5.5, agent ri-contract)

Branch `p4ei/contract`, worktree `.claude/worktrees/p4ei-contract`. Brief: `briefs/contract.md`.

## Done
- Read MISSION/FLEET/brief, core docs and code (gap-map survey done; findings feed DESIGN §0).
- `internal/domain/investigation.go` drafted: all §2 contract types with content-derived ids and
  Validate()/ValidateAgainst(); builds and vets.

- `internal/domain/investigation_test.go`: valid fixture + one mutation test per Validate rule;
  sample documents in `internal/domain/testdata/sample-*.json` (golden, `RI_UPDATE_SAMPLES=1`).
- schemagen: new targets `schemas/{evidence-bundle,investigation,investigation-graph}.schema.json`,
  enums, provenance-variant patches; schemagen tests validate the samples and reject AI findings /
  AI validations / deterministic assertions / a fifth class.

- `internal/investigate/{doc,api,api_test}.go`: stage ports, stores, agent Tools + AgentDriver, error classes, idempotency keys; no Temporal import.

## In progress
- DESIGN.md

## Next step
- ports → DESIGN.md (§0–§7) → eval/investigations/{FORMAT.md,gates.yaml} → full build/vet/test.

## Decisions
- Name collisions with existing domain types forced renames (brief names in parentheses):
  `BundleManifest` (ArtifactManifest — an ArtifactType constant already), `ValidationRecord`
  (ValidationResult — the learning loop's candidate-scoped verdict, which forbids environment
  evidence), `InvestigationFinding` (ImpactFinding — the five-class join finding).
- Graph edges point in the direction impact propagates (From = provider, To = consumer).
- "Unresolved" edge = inferred or missing; traversal complete ⇔ not truncated and every path established.
- Validation outcome is always relative to the safety property (pass = not reached/broken, fail = demonstrated).
- Affected always cites a failing validation; the join's action-required / not-affected verdicts are
  wrapped as deterministic ValidationRecords (ImpactFinding set) by the classify lane.

## Files touched outside ownership
(none)

## Contract changes
n/a (this lane defines the contract)

## Open questions
(none)

## Tests
not run yet
