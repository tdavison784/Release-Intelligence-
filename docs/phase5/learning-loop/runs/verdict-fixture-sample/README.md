# Lane verdict: sample dossier, verdict, hypotheses and export (synthetic fixture)

These files come from the **synthetic** test fixture `internal/investigate/agent/widget_fixture_test.go`. It models
an invented product "widgets" 1.4.0 → 1.5.0 whose controller changes the default of `Widget.spec.cleanup.policy`
from Retain to Purge. No evaluation case is modelled. The files were produced by the same code paths the tests
exercise (prompt v6, scripted driver).

| File | What it is |
|---|---|
| `dossier.json` | the item dossier the behaviour item's session starts from (`ri.dev/item-dossier/v1`) |
| `hypotheses.json` | two hypotheses: one with a well-formed `mitigation`, and one whose release bounds lie outside the run's transition (kept as free text, `mitigationRejected`) |
| `verdict.json` | the agent's provisional verdict on the item (AI, never a policy input) |
| `export.json` + `knowledge/` | `ExportHypotheses` into an empty knowledge dir: one candidate, one proposal (rule `hypothesis:<hyp>@<inv>`), one pending whole-fact `fact-review` item; the malformed one is skipped. No fact. |
