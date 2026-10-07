# Elasticsearch release-channel research (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the checks and probes were re-run live; the live `ri check` outcomes are unchanged. Any unreachable host recorded below was a transient anonymous rate limit or an anonymous-auth refusal, not a block of this product's channels (details: docs/rerun/REPORT.md, per-product table). The original text below is kept as the record of 2026-10-01.

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here (egress policy or inferred).
`RAW` = `https://raw.githubusercontent.com/elastic/elasticsearch`; research used a blobless clone of `github.com/elastic/elasticsearch` (canonical git `github.com/elastic/elasticsearch.git` [200]).

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, dual live majors.** Tags are `vX.Y.Z` (516 tags: 476 stable, 28 `alphaN/betaN/rcN` prereleases, 12 junk like `v0.19.0.RC1`); latest stable **v9.5.4** (2026-09-15). A line is X.Y. **8.19 (latest v8.19.22, 2026-09-23) is maintained alongside the 9.x lines** — v8.19.22 shipped 8 days before v9.5.4. elastic.co/support/eol [200]: 8.x End of Maintenance **2027-01-15** (End of Support 2027-07-15); 9.x maintained ≥ 2027-10-15; 7.17 End of Support 2026-01-15 → **7.x is end-of-life today**, everything older unsupported. Policy: each major is maintained for the longer of 30 months from GA or 18 months after the next major; only the newest two minors of the current major plus the final minor of the prior major get maintenance.
2. **The per-release notes eras differ sharply.**
   - **9.x era (from v9.0.0): markdown in-repo** — `docs/release-notes/index.md` (one `## X.Y.Z [anchor]` section per release, `### Features and enhancements` / `### Fixes` subsections, area label paragraphs `ES|QL:`, `Inference:`), plus `docs/release-notes/breaking-changes.md` and `docs/release-notes/deprecations.md` with the same per-version sections (all 51 releases 9.0.0–9.5.4 present; a release with no breaking changes says so explicitly). **A release's own section is NOT in the file at the release's own tag** (at v9.5.4 the newest section is 9.4.3); sections land on main after release, so the source is read at `main` (traefik's fixed-ref precedent).
   - **7.14–8.x era: `docs/changelog/<PR>.yaml` at the tag** — one YAML file per change (`area`/`issues`/`pr`/`summary`/`type: bug|enhancement|feature|breaking|upgrade`), and the directory is REPLACED at each release (diff v9.5.3→v9.5.4: deletes 9.5.3's files, adds 9.5.4's), so the dir at a tag IS that release's notes. `extract:release-note-yaml` cannot read this schema (it reads Istio's `releaseNotes` lists; `summary`/`type` keys are ignored → **zero note items**), so 8.x per-release notes are a recorded gap. 9.x also generates per-version `docs/release-notes/changelog-bundles/X.Y.Z.yml` (only at later tags; same schema problem).
   - **GitHub release bodies are stubs** (atom feed [200]): "Downloads: … Release notes: <elastic.co link>" — no note content; useful only as a versions fallback.
   - **No `docs/changelog.asciidoc` exists at any tag** (checked 5.6–9.5.4) — that lead is stale. The 7.0–7.13 era has no in-repo per-release notes at all (elastic.co HTML only, which no format reads).
3. **The AsciiDoc question — YES for migration guides.** The per-major breaking-changes/migration content exists **only as AsciiDoc** in-repo: `docs/reference/migration/migrate_7_0.asciidoc` + `migrate_7_0/*.asciidoc` (10 files, `[float]` + `====` headings + prose), `migrate_8_0.asciidoc` + `migrate_8_0/*.asciidoc` (15 files; the aggregator only `include::`s them, so the directory is the content), `migrate_9_0.asciidoc` (single file; `== Migrating to 9.0` → `=== Breaking changes`/`=== Deprecations` → `==== topic` with `[[anchor]]`/`.Block title`/`[%collapsible]`/`====` example blocks and `*Details* +`/`*Impact* +` labels). The 9.x markdown alternative `docs/release-notes/breaking-changes.md` embeds **AsciiDoc definition lists** (`Area\n:   * bullet`) inside .md, which the markdown reader collapses to one item per area — not equal evidence quality. Decision: implement generic `format:adoc` (line-preserving converter paralleling `normalize.RSTToMarkdown`).
4. **Artifacts** (all hosts REACHABLE from this sandbox — first product with the whole Elastic publication chain verifiable): Docker image `docker.elastic.co/elasticsearch/elasticsearch:X.Y.Z` (NEW registry for this catalog; anonymous token flow like ghcr, 49156 tags; **every** stable tag 5.0.0–9.5.4 present, 332/332); tarball/deb/rpm `https://artifacts.elastic.co/downloads/elasticsearch/elasticsearch-X.Y.Z-linux-x86_64.tar.gz|-amd64.deb|-x86_64.rpm` (200 from 7.0.0 through 9.5.4; the plain `elasticsearch-7.0.0.tar.gz` name is 404 — the arch suffix is the stable naming); source = GitHub tag archive; **no upstream Helm chart for the server** (elastic/helm-charts is archived/deprecated since 2023; ECK operator = separate product); no CRDs/manifests (not a Kubernetes-native product).
5. **Compatibility pins are machine-readable**: `build-tools-internal/version.properties` at the tag (from v7.14.0; `buildSrc/version.properties` before) carries `lucene = X.Y.Z` (8.11.3 at 7.17.23 → 9.0.0 at 8.0.0 → 10.5.1 at 9.5.4) and `bundled_jdk = 26.0.2+10@hash`. Lucene is verifiable on Maven Central (`repo1.maven.org/maven2/org/apache/lucene/lucene-core/X.Y.Z/…jar` [200]); the bundled JDK (vendor `openjdk`) has no stable anonymous per-build URL (gap). Properties files are not YAML → `version:pattern`, not `version:field`.

## 1. Canonical versions

`git ls-remote --tags` via blobless clone [200]: 516 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `vX.Y.Z` | 476 | `v9.5.4`, `v8.19.22`, `v7.17.23` | stable releases (332 of them ≥ 5.0.0) |
| `vX.Y.Z-alphaN/-betaN/-rcN` | 28 | `v9.0.0-beta1`, `v8.0.0-rc2` | prereleases (parsed, outside canonical list) |
| junk | 12 | `v0.19.0.RC1`, `v0.90.0.Beta1` | pre-1.0 dot-shapes |

- Pattern (kept from discovery): `^v(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$`; lineage `minor`; latest stable **9.5.4**; live lines on 2026-10-01: 9.5, 9.4, 9.3, 9.2, 9.1, 9.0 (9.0.x still patched — 9.0.8, Oct 2025), 8.19.
- License era: `ELv2|SSPL` through 8.18; **AGPLv3 added from 8.19/9.0** (2024-10-31 announcement). No technical release-topology effect; noted in the definition description.

## 2. Release notes and the eras [200]

- `docs/changelog/` (per-PR YAML) exists from ~v7.14.0 (v7.15.0: 3 files — early adoption; v7.17.0: 24; v8.0.0: 38; v9.1.0: 342; v9.5.4: 22). The dir at the tag = that release's notes (replacement, not accumulation; the 9.1.0 dir's 342 files match the generated `changelog-bundles/9.1.0.yml`'s 343 summaries). Schema per file: `area`, `issues`, `pr`, `summary`, `type` (`bug|enhancement|feature|breaking|upgrade`; v8.19.0 sample: 42 enhancement, 11 bug, 5 feature, 1 breaking, 1 upgrade). **Unreadable by `extract:release-note-yaml`** (no `releaseNotes`/`upgradeNotes`/`securityNotes` lists) — recorded as a gap for the 8.x era.
- `docs/release-notes/` (markdown, 9.x only; introduced at v9.0.0): `index.md` + `breaking-changes.md` + `deprecations.md` + `known-issues.md`, each `## X.Y.Z [anchor]`-sectioned; sections are added on main after the release (at v9.4.0's tag the newest index section is 9.3.3; main today has all of 9.0.0–9.5.4). All three main files carry a section for **every** 9.x version — empty ones say "There are no breaking changes associated with this release." — so a section selector needs no exceptions. `index.md` subsections: `### Highlights`, `### Features and enhancements`, `### Fixes`; items are `* text [#PR](url)` bullets under area label paragraphs (`ES|QL:`, `Machine Learning:`, `Infra/Logging:`). MyST artefacts (`{{es}}`, `::::{dropdown}` fences) appear in Highlights prose.
- `docs/release-notes/changelog-bundles/X.Y.Z.yml` (9.x): generated per-version YAML (`version/released/generated/changelogs: [{pr, summary, area, type, issues, highlight}]`), but a version's own bundle first exists at a LATER tag (9.5.4's own bundle is absent at v9.5.4).
- GitHub releases [200 via atom]: every tag ≥ 5.x has one; body = "Downloads: … Release notes: <elastic.co URL>" stub by `elastic-vault-github-plugin-prod[bot]`. elastic.co renders the full per-version release notes (`/guide/.../release-notes-X.Y.Z.html` for 8.x, `/docs/release-notes/elasticsearch#...` for 9.x) but as HTML (no format reads it).

## 3. Migration guides (the AsciiDoc decision) [200]

| Major | Source at the major tag | Shape |
|---|---|---|
| 7.0 | `docs/reference/migration/migrate_7_0.asciidoc` + `migrate_7_0/*.asciidoc` (10 files) | `[float]`/`[[anchor]]` blocks, `===`/`====` prefix headings, prose paragraphs |
| 8.0 | `migrate_8_0.asciidoc` (aggregator, `include::` only) + `migrate_8_0/*.asciidoc` (15 files) | `====` headings, `.Block title` + `[%collapsible]` + `====` example blocks, `*Details* +`/`*Impact* +` labels, `[cols=…]`/`|====` pipe tables, `//tag::[]` regions |
| 9.0 | `docs/reference/migration/migrate_9_0.asciidoc` (single file, 71 lines) | `== Migrating to 9.0` → `=== Breaking changes` / `=== Deprecations` → `==== topic`, same collapsible blocks; `{es}` attribute refs, `<<xrefs>>`, `coming::[9.0.0]` macro |

- No markdown twin of this content exists in-repo for 7.0/8.0; the 9.0 twin (`docs/release-notes/breaking-changes.md`) uses the AsciiDoc definition-list dialect inside .md. `format:adoc` converts headings (`= prefix` and `= - ~ ^ +` underlines with FIXED style levels, unlike rst's first-use ordering), blankets `[[anchor]]`/`[attrs]`/`ifdef`/`//comment`/`include::`/`tag::` lines, flattens `{attr}`/`xref[text]`/`<<anchor>>` inline, strips ` +` hard breaks, promotes `.Block title` to bold text, converts `|====` tables to pipe tables and definition-list `Label\n:   entry` markers — then the existing markdown-section/whole + classify machinery works on the original line numbers.
- Upgrade-ordering evidence: `docs/release-notes/breaking-changes.md` "If you are migrating from a version prior to version 9.0, you must first upgrade to the last 8.x version available."

## 4. Security

- GitHub advisories of elastic/elasticsearch (discovery default; the ESA/CVE history — e.g. CVE-2021-22145, CVE-2024-23444 era — is published both as GHSA records and on elastic.co/discuss security announcements; the GHSA feed is the machine-readable channel). api.github.com anonymous quota is the usual constraint.
- No CVEs are restated in the 9.x markdown notes (they point to the security-announcements category); the 8.x yaml changelog carries `type: breaking` security backports but is unreadable (see gap).

## 5. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| Docker image | `docker.elastic.co/elasticsearch/elasticsearch:X.Y.Z` | `X.Y.Z` exactly | [200] token flow `docker-auth.elastic.co/auth` → tags/list (49156 tags; all 332 stable tags 5.0.0–9.5.4 present), manifest 9.5.4 [200]. No docker.io mirror of the server image (Docker Hub `library/elasticsearch` is unofficial-frozen at 8.x era tags? unverified — canonical registry is reachable, no fallback needed) |
| tarball | `https://artifacts.elastic.co/downloads/elasticsearch/elasticsearch-X.Y.Z-linux-x86_64.tar.gz` | `X.Y.Z` exactly | [200] HEAD 7.0.0, 7.17.23, 8.0.0, 8.19.22, 9.5.4 (plain `elasticsearch-7.0.0.tar.gz` = 404); `.sha512` sidecars exist |
| deb / rpm | same host, `elasticsearch-X.Y.Z-amd64.deb` / `-x86_64.rpm` | `X.Y.Z` exactly | [200] HEAD 7.0.0, 9.5.4 |
| source archive | GitHub tag archive (`…/archive/refs/tags/vX.Y.Z.tar.gz`) | tag | codeload reachable per earlier onboardings |
| bundled lucene | `repo1.maven.org/maven2/org/apache/lucene/lucene-core/X.Y.Z/lucene-core-X.Y.Z.jar`, pin read from `build-tools-internal/version.properties` (`lucene = 10.5.1`) at the tag | per-release pin (`version:pattern`) | [200] jar HEAD 9.12.2, 10.5.1; properties file at the tag from v7.14.0 (`buildSrc/version.properties` before) |
| bundled JDK | `bundled_jdk = 26.0.2+10@hash` in the same file | per-release pin | no stable anonymous URL for the exact `openjdk` vendor build (gap; not declared) |
| Helm chart / CRDs / manifests | none upstream for the server | | `elastic/helm-charts` archived (deprecated 2023, unverified network-wise; recorded from repository state); ECK (`elastic/cloud-on-kubernetes`) is a separate product — noted as related, out of scope |

## 6. Compatibility and lifecycle

- elastic.co/support/eol [200] (WebFetch of the HTML for research; declared in the definition as the lifecycle evidence source): maintenance = max(30 months from GA, 18 months after next-major GA) + 6 months support; only the newest two minors of the current major + the final minor of the prior major are maintained. 8.x EOM **2027-01-15** / EOS 2027-07-15; 9.x EOM 2027-10-15 (or 18 months after 10.0); 7.17 EOS **2026-01-15**. Footnotes: 8.17 maintained until 8.19's release; 8.18 until 9.2's release.
- elastic.co/support/matrix (stack/JVM/OS per version) is HTML — no format reads it; the machine-readable per-release compatibility signal in-repo is the lucene/JDK pin above. No Kubernetes matrix exists for the server (the karpenter-record expectation of "max Kubernetes per stack version" belongs to ECK, not this product).

## 7. Hosts

Reachable: github.com (git/raw), codeload, api.github.com (anonymous 60/h; discovery's validation calls succeeded), `docker.elastic.co` + `docker-auth.elastic.co` (anonymous token flow), `artifacts.elastic.co` (302→200 on download paths), `www.elastic.co` (200; elastic.co 301→www), `repo1.maven.org`. No blocked host so far — the first onboarding with the entire canonical publication chain (registry + artifacts + website) reachable; nothing had to fall back to a mirror.
