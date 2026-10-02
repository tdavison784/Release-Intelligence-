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
- (knowledge-2, commander Q1) An audit **accept** of an auto-approved fact upgrades the aspects the human verified to `human`; the audit decision is persisted first and the R19 metric reads decisions, not the post-upgrade fact. `autoApproved` stays as history.
- (knowledge-2, PO-2) `AuditRequired`: auto-approved facts with an action-eligible consequence are audited at 100%, others 1-in-N.
- Metrics `GeneratedAt` is the latest snapshot timestamp (deterministic).

## Contract changes
- `domain.VerifiedFact.Validate`: `autoApproved` is now one-directional (required when every aspect is deterministic/consensus; may stay set as history after a human audit). `CONTRACT-CHANGE(knowledge)` in semantic.go + test; no schema change.
- `FileStore` lets a fact's per-aspect verification be upgraded (never weakened) under the same id (proxy/consensus → human). `// CONTRACT-CHANGE(knowledge)` in filestore.go.
- `knowledge.ModelMetric.ByTask` / `ModelTaskMetric` added (api.go, `CONTRACT-CHANGE(knowledge)`): DESIGN §7 asks for per-task metrics.

## Files outside ownership
- `cmd/ri/main.go` (additive: `knowledge` command + usage), `README.md` (command lines), `docs/KNOWLEDGE.md` (new).

## Open questions for the commander
1. Should a human audit-accept upgrade an auto-approved fact to `human`? Currently no (keeps R19 measurement clean); the ladder keeps such facts capped.
2. The committed `knowledge/` directory does not exist yet (no data authored by this lane); the integrity test skips until it does.
3. Consensus facts rest on `sp-` basis; `ValidateFactBasis` cannot verify them, only `ValidateFactRecords` (used everywhere here). Other lanes reading facts should do the same.

## contract-3 (PO-1/PO-2): implemented (knowledge-3)
- Routing and consensus count separate calls (`Provenance.CallID`), not models; `ConsensusAspects` labels cross-model/same-model on `AspectVerification.Consensus`; basis = one proposal per call.
- `buildFact` sets `ConsensusAction` (every aspect ≥ consensus, action-eligible consequence, every agreeing consequence proposal requested action-required, nothing refuted); merges recompute it.
- Policies: `AutoApproveRenderVerifiable` (now requires subject+change `confirmed-by-render`), `AutoApproveConsensusAction`, `CombinePolicies`, `DefaultAutoApprove` (used by `ri knowledge route`). `RouteWith` honours a policy only if it completes all four aspects.
- `AuditRequired` = ConsensusAction || (auto-approved ∧ action-eligible): always audited. Metrics: `ConsensusAgreementByScope`, `ConsensusAction{,Audited,Agreement}` from audit decisions; report prints them.
- Not done here (other lanes): ladder/eval per-level `falseActionRate`/`actionFindingEvidence` (applicability), NOT AFFECTED stays trusted-only (impact/domain).
