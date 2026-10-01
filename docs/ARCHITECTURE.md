# Architecture

Release Intelligence answers one question: **given a product and two versions,
what does upgrading from A to B mean?** It answers with an `UpgradeEdge` in which
every conclusion points back to source evidence.

## Principles

- **Deterministic first.** The recurring ingestion path (`ingest`, `normalize`,
  `upgrade`) never calls an LLM. LLMs are used only for discovery, for
  resolving ambiguous sources and for optional enrichment (`discovery`, `llm`).
- **Facts ≠ conclusions ≠ AI.** `domain.Fact` holds deterministic
  statements extracted from sources. `domain.Change` holds deterministic
  conclusions, with `Provenance.Method` set to `declared`, `computed` or
  `heuristic`. `domain.Enrichment` holds AI output and always carries
  `method: ai` together with the model, the prompt digest and the input
  evidence. `UpgradeEdge.Validate()` enforces this separation.
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
internal/upgrade       pure: path selection, diffs, edge assembly, text rendering
internal/discovery     AI-assisted source discovery: repo scanner → candidates →
                       (optional LLM resolution) → validation against ≥3 releases → proposed definition
internal/app           composition root + use cases (wires adapters by locator kind; offline e2e tests)
internal/llm           LLM port + Anthropic implementation
internal/store         local JSON store for ingested releases and edges
products/              checked-in product definitions
schemas/               JSON Schemas (draft 2020-12): product-definition (hand-written); upgrade-edge and
                       release (generated from internal/domain, run `go run ./internal/domain/schemagen`;
                       a test fails when they are stale)
```

Dependency direction: `domain` ← `catalog` ← `sources` ← adapters; `normalize`
depends on `domain` and `catalog`. `ingest` depends on `sources`, `normalize`,
`catalog` and `domain`. `upgrade` depends only on `domain` and `catalog`.
`discovery` depends on `ingest`, `catalog`, `llm` and `sources`. Adapters never
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
| `optional` | Absence is a fact, not a broken relationship | about half of Argo CD releases ship in no chart |
| `exceptions` (with reason) | Curated releases where a relationship does not hold | Argo CD v3.4.0 has no release assets |
| `references` | Cross-check an artifact inside another artifact | image tags in the install manifest when quay.io is unreachable |
| `contents.stripPrefix` / `ignoreKeys` | Normalise Helm values to user-facing keys | Istio's `_internal_defaults_do_not_set`; source-tree hub/tag placeholders |
| `classify` rules | Product-specific classification, evaluated first | cert-manager "⚠️ Breaking change" callouts; Argo CD noise filters |
