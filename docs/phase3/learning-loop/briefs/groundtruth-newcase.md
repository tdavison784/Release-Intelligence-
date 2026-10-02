# Groundtruth sub-brief — author ONE new eval case, blind, with semantic labels

You are helping the `groundtruth` lane (MISSION G22: expand applicability ground truth). You author
exactly one new dataset case under `eval/cases/<case-id>/` in the worktree you are given. Nothing else.

## Integrity rules (non-negotiable — violating them invalidates the dataset)

- **Blind.** Author only from upstream sources (release notes, upgrade guides, CHANGELOGs, docs,
  chart values / CRDs at the two tags, GitHub issues/PRs) fetched with `curl`/`gh`/WebFetch. Quote them
  verbatim with URLs.
- **Never** run `ri` / `bin/ri` (no `upgrade`, `impact`, `eval`), never read `eval/results/*`,
  `eval/REPORT.md`, `eval/adjudications/*`, `internal/*/testdata/*`, `.ri/` caches, or
  `docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md` §1–§2. Do not look at what the pipeline outputs.
- Do not edit any file outside `eval/cases/<your-case-id>/`. Do not commit (the lane commits after review).
- Expectations are never derived from tool output. Matchers use upstream wording.

## What to read first (in the worktree)

1. `eval/FORMAT.md` — the whole file, especially "Semantic labels (G22) and undecided links".
2. `docs/phase3/learning-loop/DESIGN.md` §1.1–§1.5 (subject families, change kinds, the condition
   language incl. scoped vs environment-wide leaves and states, consequence kinds → class). The types are
   in `internal/domain/semantic.go` (`subjectFields`, `conditionSpecs`, `ChangeSpec.Validate`).
3. A labelled example: `eval/cases/cert-manager-1.17-1.18/` (case.yaml, NOTES.md, environment/).
4. `products/<product>.yaml` — the definition the pipeline uses: the tag format (`from`/`to` must be
   tags as published and must match `tagPattern`), and which upstream channels it ingests (release notes,
   CHANGELOG sections, upgrade guides, chart values). Prefer expected items those channels state.
5. `docs/ACTION_CLASSIFICATION.md` (what makes something action-required vs review vs informational).

## What to produce

`eval/cases/<case-id>/` with:

- `case.yaml`: id (= directory name), product, from, to, researchedAt "2026-10-01", sources (every URL
  consulted), 6–10 `expected` items, each with kind/importance/actionRequired/classification, matchers
  (`text` regexes anchored on upstream wording, specific enough not to catch unrelated changes — avoid bare
  common words), `evidence` (url + verbatim quote), and a complete **`semantics`** block
  (subject/change/consequence; a list when one item bundles several subjects). Optional `notExpected`.
- `environment/`: a realistic customer cluster for this transition — `values.yaml` and/or
  `manifests/*.yaml` (Kubernetes YAML; ConfigMaps carrying config files are fine and are line-addressable),
  optional `images.txt`, `crds/`, and **`inventory.yaml`** (a YAML list of `{product, version, note?}` —
  ONLY products/versions the `environment.description` states, each entry with a comment quoting the
  sentence; use catalog ids from `products/` where they exist; do not add a `complete:` key, it is not
  supported yet).
- `environment:` in case.yaml: description (state every fact the links rely on), kubernetes version,
  **`expectedImpact`** links — at least 5, each with relevance, why, **`exposure`** (domain condition
  language, verbatim), optional `overlap`, and **`environmentEvidence`** (`<path under environment/>#Lx-Ly`
  or `environment.kubernetes` / `from`). **Include at least 2 `not-affected` links**: the cluster looks
  similar but is clearly not exposed (e.g. sets the replacement key, does not use the removed kind, runs a
  product version outside the affected range) — so applicability is tested in both directions. Optionally
  1–2 **`undecidedImpact`** links (reason + needed) where the fixture honestly cannot decide.
  `expectedFindings` / `notExpectedFindings` are optional; only add ones you can ground in upstream
  artifacts (e.g. a values key that the chart values diff at the two tags proves removed).
- `NOTES.md`: sources read (what each contributed), every judgement call (class choices, why a link is
  not-affected/undecided, how the fixture was grounded in chart values/CRDs at the tags), and a
  "Semantic labels" section explaining non-obvious subject/condition choices.

Prioritise G22's list, and tell us which items exercise which category: prose-only defaults, feature
gates, cross-product version dependencies (`product-relationship` + `product-version` leaves against the
inventory), RBAC, migrations, behaviour changes, deprecations, required new config, compatibility hidden
in prose.

## Label conventions (follow exactly)

- Field names: `family, product, group, version, kind, path, name, component` (subject);
  `type, before, after, replacedBy` (change); `kind, exposedClass, statement, remediation, severity`
  (consequence; `exposedClass` MUST equal the kind's class: upgrade-blocked / resource-rejected /
  setting-ignored / permission-lost / workload-failure / migration-required → action-required;
  behavior-change / deprecation → review-required; none → informational. Action kinds need a
  `statement` answering "what exactly fails if I do nothing?").
- `before`/`after`/`values` are JSON-encoded strings: `before: '"Never"'`, `values: ['"WhenUnderutilized"']`,
  `after: '">=1.29.0"'` is WRONG for requirement-changed — there `after` is a bare semver constraint
  string: `after: '>=1.29.0'`.
- Subjects belong to the case's product; another product appears as
  `{family: product-relationship, product: <this product>, name: <other product id>}` with
  `change: {type: requirement-changed, after: '<constraint>'}`.
- Conditions: environment-wide leaves (`values-key`, `gvk-in-use`, `image-in-use`, `cli-flag`, `env-var`,
  `feature-gate`, `product-version`, `cluster-version`, `edge-from-version`, `rendered-change`,
  `undecidable`) never inside a `resource`/`ref` scope; scoped leaves (`field`, `text-line`, `ref`) only
  inside `resource` (or `ref`). `text-line` = a line regex over embedded text at a values-style path such
  as `data["config.yaml"]` with state `exists`/`none`. `[]` marks list elements in paths
  (`spec.containers[].args`).
- Relevance vs class: the scorer compares an item's `classification` with the class the engine outputs
  in THIS case's environment. So for every linked item, `classification` must match the link:
  action-required→action-required, review→review-required, informational→informational,
  not-affected→not-affected, undecidedImpact→unknown. Unlinked items keep their generic class. Note the
  generic reading in a comment/NOTES when it differs.

## Validate

`go test ./internal/eval -run 'TestDatasetLoads|TestDatasetEntrySelectionLoads' -count=1` must pass
(it loads and validates every case, including your labels and evidence line ranges). Also run
`go test ./internal/eval -count=1` — everything must stay green.

## Report back

A ≤25-line summary: case id, transition, items (id, one-line, class, G22 category), links (expected →
relevance, and which are not-affected / undecided), notable judgement calls, anything you were unsure of.
