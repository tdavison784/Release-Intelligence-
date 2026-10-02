# Lane `knowledge` — status

**State: done.** `go build ./... && go vet ./... && go test ./...` green on `p3ll/knowledge` (with contract-2 merged).

## Done
- `internal/knowledge`: `filestore.go` (FileStore), `route.go` (Route/RouteWith, agreements, signals, items, `AutoApprovePolicy` seam),
  `autoapprove.go` (`ConsensusAspects`, `AutoApproveRenderVerifiable`), `decide.go` (`FactFromDecision`, per-aspect state, `buildFact`),
  `queue.go` (Queue: Inbox with G7 counts+filters, Item → `ReviewContext` incl. related decisions/facts by subject key and shared members,
  Decide incl. bulk; `OpenFactReview`), `pipeline.go` (`RouteStore`, audit sampling), `dataset.go` (`ExportDataset`), `metrics.go`
  (every DESIGN §7 metric, per model and per task, batch vs individual separate, auto-approval audit), `report.go`, tests incl. the
  integrity test (no file under `knowledge/` references eval cases/expectations/case ids).
- CLI `ri knowledge route|decide|review-fact|export|metrics` (`cmd/ri/knowledge.go`); `decide --reviewer-kind proxy` requires AI provenance flags.
  `candidates|propose|validate` print "provided by the semantic/validate lane" (not wired: those packages are not on this branch).
- Docs: `docs/KNOWLEDGE.md`, README command lines.

## Decisions (and why)
- Fact evidence = validator evidence + candidate evidence cited by proposals agreeing with the fact (all candidate evidence if none): makes the grounding metric non-trivial.
- Per aspect, later decisions override earlier; validator-confirmed identical value stays deterministic; a proxy never overrides a trusted aspect.
- Aspects nobody proposed → one `evidence-sufficiency` item on route `missing-evidence`.
- Fact re-review = item whose complete Proposed is exactly an existing fact (`OpenFactReview`): reject retracts, correct supersedes; routing never revives a retracted/superseded fact.
- Auto-approval (contract-2): `Route()` stays the pure DESIGN §6 table; the policy is opt-in (`RouteWith`, `RouteOptions.Policy`). `ri knowledge route` enables `AutoApproveRenderVerifiable` by default (`-auto-approve=false` to disable) and samples 1-in-5 (`-audit-every`) into human audit.
- An audit **accept** of an auto-approved fact measures it and does NOT upgrade it (the marker means no reviewer decided it). See open question.
- Metrics `GeneratedAt` is the latest snapshot timestamp (deterministic).

## Contract changes
- `FileStore` lets a fact's per-aspect verification be upgraded (never weakened) under the same id (proxy/consensus → human). `// CONTRACT-CHANGE(knowledge)` in filestore.go.
- `knowledge.ModelMetric.ByTask` / `ModelTaskMetric` added (api.go, `CONTRACT-CHANGE(knowledge)`): DESIGN §7 asks for per-task metrics.

## Files outside ownership
- `cmd/ri/main.go` (additive: `knowledge` command + usage), `README.md` (command lines), `docs/KNOWLEDGE.md` (new).

## Open questions for the commander
1. Should a human audit-accept upgrade an auto-approved fact to `human`? Currently no (keeps R19 measurement clean); the ladder keeps such facts capped.
2. The committed `knowledge/` directory does not exist yet (no data authored by this lane); the integrity test skips until it does.
3. Consensus facts rest on `sp-` basis; `ValidateFactBasis` cannot verify them, only `ValidateFactRecords` (used everywhere here). Other lanes reading facts should do the same.
