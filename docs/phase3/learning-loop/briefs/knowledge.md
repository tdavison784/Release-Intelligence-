# Lane `knowledge` — store, queue, routing, decisions → facts, dataset, metrics (G6, G11–G16, G24)

Read `_wave1-common.md` first. Package `internal/knowledge` per DESIGN.md §9 `knowledge`, CLI
`cmd/ri/knowledge.go`. FileStore over committed JSON under `knowledge/` (idempotent content-ID writes,
append-only decisions, status-only mutations, `ValidateFactBasis` on every minted fact), Queue,
Route (DESIGN §6 hypotheses, recorded signals), FactFromDecision (corrections keep both values;
supersession/retraction), ExportDataset (JSONL FeedbackExample with all G13 labels), ComputeMetrics
(every metric in DESIGN §7's table, per model and per task), and the integrity test (no file under
`knowledge/` references eval cases or expected-item ids). Also a proxy-review CLI path
(`ri knowledge decide --reviewer-kind proxy` with required AI provenance) — the only way proxy decisions
enter. Expose `knowledge.ReviewContext` assembly for the dashboard (everything G8 lists, including
previous related decisions by subject key). `ri knowledge metrics` prints a readable report and `-o json`.
Merge early: the semantic and dashboard lanes need your FileStore on `p3-learning-loop` — commit the
store + queue first, tell the commander (final message `STORE READY` if not fully done), then continue.
Model: Sonnet.
