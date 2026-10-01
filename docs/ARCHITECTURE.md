# Architecture

Release Intelligence answers one question: **given a product and two versions,
what does upgrading from A to B mean?** It answers with an `UpgradeEdge` in which
every conclusion points back to source evidence.

## Principles

- **Deterministic first.** The recurring ingestion path (`ingest`, `normalize`,
  `upgrade`, `drift`) never calls an LLM. LLMs are used only for discovery, for
  resolving ambiguous sources and for optional enrichment (`discovery`,
  `enrich`, `llm`).
- **Facts ≠ conclusions ≠ AI.** `domain.Fact` holds deterministic
  statements extracted from sources. `domain.Change` holds deterministic
  conclusions, with `Provenance.Method` set to `declared`, `computed` or
  `heuristic`. `domain.Enrichment` holds AI output and always carries
  `method: ai` together with the model and model version, the prompt
  version and digest, the input evidence and the evidence it cites.
  `UpgradeEdge.Validate()` enforces this separation.
- **Evidence everywhere.** Every Change, Fact, NoteItem, compatibility
  constraint and artifact instance references `domain.Evidence`. Each Evidence
  record carries a URI a human can open, a locator such as a line range, a
  short excerpt and the digest of the exact bytes retrieved.
- **Declarative products.** Product-specific knowledge lives in
  `products/<id>.yaml` (`catalog.ProductDefinition`), not in Go code. Generic
  code is driven by locator kinds, extract types, classification rules,
  version relations and availability constraints.
- **Many artifacts per release, versions don't have to match.** An artifact
  declares how its version relates to the release version: `template`, or
  `lookup` (for example "chart versions whose appVersion == {{.Tag}}"), or
  `independent`.
- **Ports and adapters.** External systems are reached only through the
  interfaces in `internal/sources`, which are registered by locator kind. The
  domain model has no dependency on GitHub, Helm, OCI or Kubernetes.
- **Reproducible.** All HTTP goes through `internal/fetch` and its filesystem
  cache. `--offline` replays a run from the cache; tests use the same
  mechanism or `httptest`.
- **Honest about gaps.** An unreachable source is reported with
  `SourceStatus` set to `unavailable`. An artifact that only the definition
  predicts is `expected`, never `verified`.

## Packages

```
cmd/ri                 CLI
internal/domain        core model: Version, Release, ArtifactInstance, Evidence, Fact,
                       NoteItem, Snapshot, CompatibilityConstraint, Change, UpgradeEdge,
                       Enrichment, Provenance, Advisory
internal/catalog       ProductDefinition format, loader, static validator, template rendering
internal/fetch         HTTP client + filesystem cache (online / offline / refresh)
internal/sources       ports (VersionLister, DocumentFetcher, DirectoryFetcher, ArtifactProbe,
                       VersionIndex, AdvisorySource) + Registry keyed by locator kind
internal/github        adapters: github-releases, github-advisories (GitHub REST API)
internal/gitsrc        adapters: git-tags, repo-file, repo-dir, git-log (git + raw content hosts)
internal/httpsrc       adapters: http (documents + release-asset probes)
internal/oci           adapters: oci (Distribution v2 API: tags, manifests, chart config)
internal/helm          adapters: helm-repo (index.yaml), helm-git (chart in git, tags)
internal/normalize     pure parsing for ONE release: markdown sections, note items +
                       classification, tables → compatibility, Chart.yaml, values, CRDs, images
internal/ingest        deterministic pipeline: definition + registry → domain.Release;
                       version listing; historical relationship checks
internal/drift         deterministic source-drift detection: re-validates the definition
                       against the newest releases (vs a saved `ri check` baseline),
                       reports evidence-backed events + proposed changes, never mutates
                       (docs/DRIFT.md)
internal/upgrade       pure: path selection, diffs, edge assembly, text rendering
internal/env           environment model parsed from LOCAL files (values, manifests,
                       installed CRDs, images) with per-fact evidence
internal/impact        pure: joins an UpgradeEdge with an environment into an
                       ImpactReport; every finding cites upstream + environment
                       evidence (docs/IMPACT.md)
internal/discovery     source discovery: repo scanner (workflows, Makefiles, charts, values,
                       manifests, docs) → candidates → tag-train and version-relation inference →
                       (optional LLM resolution) → validation against ≥3 historical releases →
                       proposed definition in which every element carries a status
                       (historically-validated / discovered / inferred / unverified / exception)
                       and the evidence file that grounds it
internal/app           composition root + use cases (wires adapters by locator kind; offline e2e tests)
internal/llm           LLM port + Anthropic implementation, response cache (by prompt digest), file exchange
internal/enrich        AI enrichment of edges: deterministic candidate groups → bounded prompts →
                       validator → Enrichments with full provenance (docs/ENRICHMENT.md)
internal/store         local JSON store for ingested releases and edges
internal/eval          validation dataset: loads eval/cases ground truth, runs the
                       real pipeline per entry (via an injected Pipeline port) and
                       scores recall, false positives, duplicates, unsupported
                       conclusions and environment-impact accuracy; compares against
                       committed snapshots in eval/results and reports regressions
                       (eval/FORMAT.md, eval/REPORT.md)
products/              checked-in product definitions
eval/                  validation dataset (cases with hand-curated ground truth +
                       stored result snapshots) consumed by `ri eval`
schemas/               JSON Schemas (draft 2020-12): product-definition (hand-written); upgrade-edge,
                       release and impact-report (generated from internal/domain, run
                       `go run ./internal/domain/schemagen`; a test fails when they are stale)
```

Dependency direction: `domain` ← `catalog` ← `sources` ← adapters; `normalize`
depends on `domain` and `catalog`. `ingest` depends on `sources`, `normalize`,
`catalog` and `domain`. `drift` depends on `ingest`, `catalog`, `sources` and
`domain`. `upgrade` depends only on `domain` and `catalog`.
`env` depends on `domain` and `normalize` (it reuses the values-flattening so
both sides of the impact join speak the same key-path syntax). `impact`
depends on `domain`, `env` and `upgrade` (constraint semantics).
`discovery` depends on `ingest`, `catalog`, `llm` and `sources`. `enrich`
depends only on `domain` and `llm`. `eval` depends on `domain` and `env`
only; the pipeline under test is injected as a `eval.Pipeline` port, which
`cmd/ri` implements over `app` (so eval scores live runs and offline test
replays with the same code, and `app` never imports `eval`). Adapters never
import `ingest` or `upgrade`.

## Data flow of `ri upgrade <product> <from> <to>`

1. `catalog` loads and validates `products/<product>.yaml`.
2. `ingest.ListVersions` queries the `versions` sources in priority order,
   falling back when one is unavailable, parses tags and sorts them.
3. `upgrade.SelectPath` picks the traversed releases according to the
   lineage. With `minor`, these are the X.Y.0 release of each line after
   From's line, then the patches of To's line.
4. `ingest.IngestRelease` runs for From, every path release and To:
   - It renders source locators with a `catalog.RenderContext`, fetches
     documents, extracts the relevant part (`markdown-section`,
     `markdown-table`, `yaml-records`, `release-note-yaml` or the whole document) and turns
     it into `NoteItem`s and `CompatibilityConstraint`s.
   - It resolves each artifact's version (template or lookup) and probes its
     channels in order. If every channel is unreachable, it cross-checks
     `references` (for example the image tag inside the install manifest).
   - It captures `contents` snapshots (Helm values, chart metadata, CRDs,
     image references).
5. `ingest.Advisories` lists advisories from the `security` sources.
6. `upgrade.Build` aggregates the path's NoteItems into Changes and diffs the
   From/To snapshots: values keys added or removed and defaults changed, CRD
   versions or schema fields removed, images added or removed. It also
   compares compatibility constraints, matches advisories (fixed by the
   upgrade, or still affecting To) and validates the edge.
7. `upgrade.RenderText` prints the report. `-o json` prints the edge.

## Data flow of `ri drift <product>` (docs/DRIFT.md)

1. `ingest.ListVersions` lists the canonical versions; if no versions source
   answers at all, the failure itself becomes the report (unreachable is
   `unverifiable`, a reachable source whose tags no longer match the declared
   convention is `relationship-broken`).
2. The baseline is the saved `ri check -o json` report in
   `docs/onboarding/checks/<id>.json` (when present) or the definition's
   `validatedAgainst` lists; the cutoff is the newest baseline release.
3. `ingest.IngestChecked` re-runs the exhaustive relationship validation on
   the newest `-n` releases newer than the cutoff.
4. `drift.Analyze` compares fresh outcomes with baseline outcomes:
   reachable-and-different is **drift**, unreachable is **unverifiable**
   (never drift), already-failing-at-baseline is a note. It additionally
   re-probes declared channels for move detection (an artifact absent from
   its leading channel but present at a declared fallback) and availability
   violations, and scans `image-refs` snapshots for release-tagged image
   repositories no declared artifact covers.
5. Events cite `domain.Evidence` from the same run; each may carry a proposed,
   annotated definition fragment. The command prints the report (or `-o json`)
   plus the proposal (stdout or `-out`), never touching `products/`.

## Data flow of `ri impact <product> <from> <to> --kubernetes … --values …`

1–7 are the `ri upgrade` pipeline above. Then:

8. `env.Load` parses the local environment inputs (values files, manifest
   directories, installed CRDs, an image list, a cluster version flag) into
   an `env.Environment`: every extracted fact carries local evidence
   (file, line/YAML path, excerpt, file digest).
9. `impact.Build` joins the edge's computed changes and compatibility
   constraints with the environment facts (key-path, apiVersion, constraint
   and image matching; docs/IMPACT.md) into a `domain.ImpactReport`. Each
   finding cites the upstream evidence of the change AND the environment
   evidence of the matched fact; `ImpactReport.Validate()` enforces both
   chains. `impact.RenderText` prints the funnel summary and per-finding
   why-blocks; `-o json` prints the report
   (`schemas/impact-report.schema.json`).

## Data flow of `ri eval [entries...]`

1. `eval` loads the dataset entries (`eval/cases/<id>/case.yaml`, format in
   `eval/FORMAT.md`): hand-curated expectations with matchers (text regex,
   exact subject, category, release, evidence-URI regex, flags), `notExpected`
   entries, and — when the case ships an `environment/` fixture —
   environment-side expectations (`expectedImpact` links, `expectedFindings`,
   `notExpectedFindings`).
2. Per entry the REAL pipeline runs through the injected `Pipeline` (the
   `app.App` of this process): `Upgrade` for every case, plus `Impact` for
   environment cases (inputs assembled from the fixture directory).
3. `eval.ScoreEntry` scores the edge/report against the ground truth:
   recall of the must-find list (with the importance of every miss), false
   positives (changes matching `notExpected`), duplicate conclusions (same
   normalized title, same category+subject set, or ≥0.75 title-token overlap
   on the same subjects), unsupported conclusions (evidence chains that do
   not resolve inside their document, findings joining nonexistent changes)
   and environment accuracy (expectedImpact links surfaced as findings,
   expected findings found, forbidden findings absent). Every hit records
   the matching change, the matcher that fired and a human-openable evidence
   URI, so hits and misses are auditable without re-running.
4. The command prints per-entry and aggregate results (text) or the whole
   report (JSON), then diffs against the committed snapshots in
   `eval/results/`: fewer found items, new misses, more false
   positives/duplicates/unsupported or worse environment numbers are
   regressions and exit non-zero (CI gate). `-update` rewrites the
   snapshots after review — expectations are never derived from output.

## Locator kinds

| kind | fields | capabilities |
|---|---|---|
| `github-releases` | repository `owner/name` (+ ref = tag for documents) | versions, documents (release body) |
| `github-advisories` | repository | advisories |
| `git-tags` | repository `host/owner/name`, tagPattern? | versions |
| `repo-file` | repository, ref (default `{{.Tag}}`), path | documents |
| `repo-dir` | repository, ref, path, glob, baseRef? (only files added since baseRef) | directories |
| `git-log` | repository, ref `A..B` | documents (markdown list of commit subjects) |
| `http` | url | documents, probes |
| `helm-repo` | url, chart | versions, version-index, probes |
| `helm-git` | repository, path (chart dir), tagPattern | version-index, probes |
| `oci` | repository `registry/path` | version-index (tags), probes |

## Conventions for contributors

- Packages expose small, documented APIs. `*/api.go` files define contracts
  that other packages call, so do not change those signatures casually.
- Tests use no network. Use `httptest`, fakes of the `sources` ports, or a
  checked-in fetch cache under `testdata/` replayed in offline mode.
- Producer strings take the form `component@vN`, for example
  `normalize.notes@v1`.
- Evidence IDs are content-derived (`domain.NewEvidence`), so re-running
  ingestion is idempotent.

## Definition constructs and the cases that motivated them

Each construct is generic and declarative. Each one exists because a real
upstream channel needed it.

| Construct | Meaning | Motivating case |
|---|---|---|
| `extract: markdown-section` + `heading` | Pick one release's section from a cumulative document | cert-manager per-minor notes with one section per patch |
| `extract: markdown-table` / `yaml-records` | Pick a row from a support matrix | cert-manager README tables; Istio `supportStatus.yml` |
| `columns[].separator/part` | Split a combined cell | "1.33 → 1.36 / 4.20 → 4.22" (Kubernetes / OpenShift) |
| `locator.baseRef` (repo-dir) | Only the files added since another ref | Istio's accumulating `releasenotes/notes/*.yaml` |
| `git-log` locator | Commit subjects between tags, as notes | Argo CD release notes exist only behind the GitHub API |
| `fallbackGroup` | Alternatives tried in priority order | website notes vs GitHub release body; master vs release branch |
| `releaseKinds`, `availability` | Restrict to minor releases or version ranges | upgrade guides exist only for X.Y.0; cert-manager-ctl < 1.15 |
| `version.strategy: lookup` | Find artifact versions by a field in an index | Argo CD chart whose `appVersion` is the app tag |
| `version.strategy: field` + `from` | Read the artifact version out of a YAML field of a document at the release ref | kube-prometheus-stack's pinned sub-components: `appVersion` (prometheus-operator), `dependencies[name=x].version` (charts), values.yaml image tags |
| `optional` | Absence is a fact, not a broken relationship | about half of Argo CD releases ship in no chart |
| `exceptions` (with reason) | Curated releases where a relationship does not hold | Argo CD v3.4.0 has no release assets |
| `references` | Cross-check an artifact inside another artifact | image tags in the install manifest when quay.io is unreachable |
| `contents.stripPrefix` / `ignoreKeys` | Normalise Helm values to user-facing keys | Istio's `_internal_defaults_do_not_set`; source-tree hub/tag placeholders |
| `classify` rules | Product-specific classification, evaluated first | cert-manager "⚠️ Breaking change" callouts; Argo CD noise filters |
| `extract.labelParagraphs` | Treat standalone label paragraphs (`SECURITY:`, `BUG FIXES:`) as headings one level below the last real heading | Vault CHANGELOG.md (HashiCorp-style changelogs: Terraform and its providers, Consul, Nomad) |
| `extract.format: docbook/rst` | Render non-markdown sources as line-preserving markdown (headings, bullets, tables) before extraction; converted structural markup is read in list-item mode | PostgreSQL SGML release notes (docbook); Cilium rst upgrade notes and compatibility grid tables (rst) |
