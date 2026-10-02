# Lane `semantic` — candidate generator + multi-model proposal layer (G1–G4)

Read `_wave1-common.md` first. Package `internal/semantic` per DESIGN.md §9 `semantic`, plus
`docs/SEMANTIC.md` and the CLI surface the `knowledge` lane's `ri knowledge propose` calls (coordinate
via the `knowledge.Proposer` port only).

- Candidates (restatement clusters per DESIGN.md) from an UpgradeEdge, deterministic, with Hints.
- Prompt design is the heart of this lane (you are Opus for this reason): one prompt version per task
  (`semantic-extract/v1` …), enumerated typed answer schema from domain enums, `Statement`/`Detail` the
  only free text, citations restricted to shown evidence, explicit abstention (`undetermined`) that is
  cheap to choose, injection stance like `internal/impactenrich`, values never sent, eval expectations
  never in prompts. Study `internal/impactenrich/prompt.go` and the model-comparison findings
  (`docs/phase3/model-comparison/RESULTS.md`) — e.g. structured-output enforcement differs by transport.
- `LLMProposer` over `llm.Client` (Anthropic API, Anthropic-compatible gateways e.g. Z.AI for GLM, the
  file exchange, the cache). Provider-agnostic seam so a typed "System One" model (TypeSafe Jev) or a
  Codex-backed proposer can be added as another implementation.
- A multi-model runner for the file exchange: answer exchange requests with stateless `claude -p`
  calls per model (generalize `docs/phase3/model-comparison/raw-arm.sh` into a reusable script under
  `scripts/` or `docs/phase3/learning-loop/`), so Claude Opus/Sonnet/Haiku can each propose for the same
  candidates with full provenance (`provider: anthropic`, served model from the CLI envelope).
- Tests offline with fake clients (incl. hallucinated citations, schema violations, abstentions, a model
  trying to emit action-required — all refused and recorded).
- Then run it for real on the 8 environment cases' edges (env-independent), for claude-sonnet-5-5 and
  claude-haiku-4-5 (and claude-opus-5-5 if budget allows), writing proposals into
  `knowledge/` through the store port when the knowledge lane's FileStore has landed on
  `p3-learning-loop` (ask the commander), otherwise into a scratch dir you can import later. Report
  counts, failures, agreement.

Model: Opus.
