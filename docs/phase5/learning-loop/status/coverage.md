# Lane `coverage` — status

Branch `p5ll/coverage`. Brief: `briefs/coverage.md`.

## Done
- Part 2 (documentation-only rendered changes) — commit cee09993. See "Decisions".

## In progress
- Part 1: declarative namespace resolution for cluster-scoped name references (catalog `Relationship.namespaceFrom`,
  relations builder, cert-manager rule). Next step: implement in `internal/catalog/relationships.go` +
  `internal/investigate/relations/edges.go`, tests with a synthetic product.

## Decisions
- Version bumps: `classify.policy@v1` → `@v2`, `validate.render-diff@v1` → `@v2` (outputs change for behaviour items).
  Stored Phase 4 records keep their versions and digests (nothing re-classifies them).
- A documentation-only record is all-or-nothing per validation call: every correlated change must be description text
  in a CRD with no other change in that pair (correlated to anything or not), else not-run naming why.
- The classifier re-checks the record's claim against the bundle (defence in depth); ValidateAgainst rejects a false claim.
- Documentation-only records never carry `Answers` (they test no hypothesis claim).

## Files touched outside ownership
- `internal/domain/investigation.go` (additive: field, predicate, Validate/ValidateAgainst/finding R2.2 hunks), schemas.
- `internal/investigate/validate/common.go` (draft.docOnly, version constant).
- `internal/investigate/agent/{answers,claims,peritem}_test.go`: literal policy string → `classify.Policy`.

## Contract changes
- `ValidationRecord.DocumentationOnly` (omitempty), `EvidenceBundle.DocumentationOnlyChange`, `domain.RenderedClassCRDDescription`.

## Tests
- `go test ./...` green at cee09993.
