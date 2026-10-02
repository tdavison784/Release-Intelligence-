# Lane `knowledge` — status

**State: in progress.** Store + queue + routing + decisions→facts committed (STORE READY for semantic/dashboard lanes).

## Done
- `internal/knowledge/filestore.go`: FileStore (`NewFileStore`), layout `knowledge/<product>/<release|_endpoint>/<kind>/<id>.json`, idempotent writes, atomic, validate on write and read, immutable kinds, status-only mutation, `ValidateFactBasis` on every fact.
- `route.go`: `Route`/`RouteWith` (DESIGN §6), per-aspect `Agreements`, signals, priority, items for open aspects; `AutoApprovePolicy` seam (see below).
- `decide.go`: `FactFromDecision` (accept/correct/reject/duplicate/retract/supersede), per-aspect state, `buildFact`.
- `queue.go`: `NewQueue` (Inbox with G7 counts+filters, Item → `ReviewContext` via `AssembleContext`, Decide incl. bulk), `OpenFactReview`.

## Decisions
- Fact evidence = validator evidence + candidate evidence cited by proposals that agree with the fact on ≥1 aspect (all candidate evidence if none), so the grounding metric is meaningful.
- A decision overrides earlier ones per aspect in time order; a validator-confirmed identical value stays `deterministic`; a proxy never overrides a trusted aspect.
- Aspects nobody proposed → one `evidence-sufficiency` item on route `missing-evidence`.
- Fact re-review ("fact-review item") = item whose complete Proposed is exactly an existing fact: reject retracts, correct supersedes. Created by `OpenFactReview`.
- Auto-approval: `AutoApprovePolicy` hook in `RouteWith` (commander note: consensus level + auto-approved marker land in contract-2; sampling to human review to be added with the marker).

## Contract changes
- `FileStore` lets a fact's per-aspect verification be upgraded (never weakened) under the same id (proxy → human re-review). `// CONTRACT-CHANGE(knowledge)`.
