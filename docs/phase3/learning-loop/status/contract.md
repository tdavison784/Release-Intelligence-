# Lane `contract` — status

## Done
- `docs/phase3/learning-loop/DESIGN.md` draft (all brief sections: semantic model, condition
  language, entities + identity + anchors, trust ladder, UNKNOWN reasons, cross-product, routing,
  eval per verification level + transfer, storage, lane interface map, demonstration mapping).

## In progress
- `internal/domain/semantic.go` + tests, `internal/knowledge/api.go`, additive `ImpactFinding`
  fields + `Validate()` rules, schema regeneration.

## Decisions (and why)
- One assertion type (`SemanticAssertion`) for proposals, corrections and facts; four aspects
  (subject, change, applicability, consequence) are the unit of verification, agreement and review.
  Classification is derived from the consequence kind, never asserted (keeps models away from classes).
- Condition language is three-valued (true/false/unknown) with `all`/`any` and no `not`, so absence of
  evidence can never be negated into evidence; `false` requires a supplied + healthy dimension.
- Proxy-verified facts: capped at review-required and may never produce not-affected (dangerous cell).
- Storage: committed JSON files under `knowledge/` (one per entity, envelope `KnowledgeRecord`);
  no new dependency, PR-reviewable, git history = audit log.
- `unknownReason` is optional in `Validate()` until the `applicability` lane assigns it everywhere
  (keeps e2e goldens byte-identical in this lane, as the brief requires).

## Files touched outside ownership
- (none yet)

## Open questions for the commander
- (none blocking) UNKNOWN-ANALYSIS.md not yet received; will fold in when forwarded.

## Tests
- n/a yet
