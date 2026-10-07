# Lane `capture` — artifact capture for deterministic validation (V3–V5)

Read first: `docs/phase3/learning-loop/FLEET.md`, `MISSION.md` (Goal 5), `UNKNOWN-ANALYSIS.md` §3.4
(validators V1–V6), `docs/ARTIFACTS.md`, `docs/ARCHITECTURE.md` (normalize/ingest, contents snapshots,
locator kinds, the "Definition constructs" table), `internal/normalize` (CRD parsing → `CRDVersionInfo`,
values flattening), `internal/ingest/contents.go`, `internal/upgrade/build.go` (snapshot diffs),
`products/*.yaml`.

Branch `p3ll/capture`, worktree `.claude/worktrees/capture`. Owns `internal/normalize` CRD/values capture,
`internal/ingest` contents capture, `internal/upgrade` diff additions, `products/*.yaml` declarative
additions, docs/ARTIFACTS.md, schemas regeneration.

Goal: give deterministic validators more machine-comparable upstream facts, so model-proposed semantic
facts can be **proven from artifacts** instead of needing a human.

1. **V3 — CRD schema enrichment.** Capture per schema path, per CRD version: `default`, `enum`,
   `required` (parent's required list), and `type`. Diff them From→To in `upgrade.Build` as computed
   changes: CRD field default changed, enum value removed/added, field became required. Each change
   cites the CRD evidence of both sides. These are computed diffs (method computed), so they flow into
   the existing join; make sure they don't flood the narrative (consider the routine/visibility rules in
   `internal/upgrade/routine.go` and how `crd-schema` field-added changes are rendered today) and don't
   create ACTION findings by themselves unless the existing join rules justify it — note any new join
   behaviour in status rather than inventing join rules (that's the `applicability` lane).
2. **V4 — Helm chart values for charts published only to OCI registries.** The `oci` locator kind already
   supports chart packages (docs/ARTIFACTS.md). Verify karpenter's chart values are captured from its
   published OCI chart (or explain why not) via `products/karpenter.yaml` declarations only — no Go
   special cases. Generalize anything missing.
3. **V5 — operand-version tables as declarative compatibility sources.** Some products publish a
   machine-readable table of supported operand versions (e.g. Strimzi's `kafka-versions.yaml` in the
   repo at the release tag). Express it via existing constructs (`extract: yaml-records`, compatibility
   columns) in the product definition, generically; add a construct only if the existing ones truly
   can't express it, and document it in the ARCHITECTURE constructs table with its motivating case.
4. Re-ingest live where needed (`GITHUB_TOKEN=$(gh auth token)`, state
   `-state /Users/tommydavison/repos/Release-Intelligence-/.ri` for the warm cache). Tests offline with
   checked-in testdata.
5. Full eval before/after. New computed changes may shift precision/duplicate numbers — that's allowed
   only if recall/gates don't regress; explain every eval delta in your status file. Never edit
   `eval/cases` or `eval/results` (if `eval/results` snapshots need `-update` because of legitimately new
   computed changes, do NOT run it — report the delta; the commander decides).

Model: Sonnet.
