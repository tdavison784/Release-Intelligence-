# Lane `groundtruth` — applicability ground truth expansion (G22, G12 transfer, dataset corrections)

Read `_wave1-common.md` first, then `eval/FORMAT.md`, `eval/cases/*/NOTES.md` (how cases were authored),
DESIGN.md §9 `groundtruth` (the `semantics` / `exposure` / `environmentEvidence` label format), and
UNKNOWN-ANALYSIS.md §4 (dataset observations D1–D14). You are Opus because this is careful, blind research.

BLINDNESS: you must not run the pipeline (`ri upgrade/impact/eval`) on a case before its expectations are
committed, and must not read `eval/results/*` or pipeline output while authoring. Author from upstream
sources only (WebFetch / `gh` / git), with quotes and URLs, recording judgement calls in NOTES.md.

1. Format: implement the additive label types + validation in `internal/eval/case.go` and document in
   `eval/FORMAT.md` (labels are scored later; they must never be read by knowledge authoring).
2. Add `semantics` + `exposure` + `environmentEvidence` labels to the existing 21 links (from upstream
   sources + the fixtures; do not change existing relevance/matchers in this step).
3. Corrections (D2, D3, D8, D10 and other genuine contradictions): only where upstream sources or the
   fixture contradict a label; each with an upstream-grounded rationale in NOTES.md and a line in a new
   `eval/CHANGELOG.md` (pre-correction vs post-correction, why). Do NOT add matchers motivated by
   pipeline output (D1) — leave D1 as-is and record it. Do not touch `eval/gates.yaml`.
4. Transfer environments (G12): for ≥4 existing cases, add a second, independently authored environment
   for the same release transition (a different realistic cluster: some links affected, some not, some
   undecidable), so verified facts can be shown to transfer. The eval format may need a second
   environment per case or a sibling case id — choose the least invasive option and document it.
5. New cases prioritizing G22's list (prose-only defaults, feature gates, cross-product version
   dependencies, RBAC, migrations, behaviour changes, deprecations, required new config, compatibility
   in prose), each with an environment fixture + inventory.yaml, full labels. Aim for ≥6 new cases;
   include NOT-AFFECTED links (environments that look similar but are clear) so applicability is tested
   in both directions.
6. Commit cases first, then (and only then) run `ri eval` once to make sure they parse and execute; record
   the first numbers in your status file without tuning anything.
Model: Opus.
