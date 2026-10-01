# Loki release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox.
`RAW` = `https://raw.githubusercontent.com/grafana/loki`. Product: **Grafana Loki
OSS** (`github.com/grafana/loki`), the log aggregation system. Out of scope, on
purpose: Grafana Enterprise Logs (GEL — same tags, `grafana/enterprise-logs`
image and the in-repo GEL chart, see §7), the **Loki Operator** sub-project
(`operator/` + `operator/v0.x` tags, `docker.io/grafana/loki-operator` and
`quay.io/openshift-logging/loki-operator` images, the `loki.grafana.com`
Lokistack CRDs — a separate product, like grafana-operator was for grafana),
Grafana Alloy (promtail's successor, its own repository), and the Data-Cloud
siblings (Mimir/Tempo/Phlare, other repositories).

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, `v`-prefixed, minor lines.** Tags `vX.Y.Z` (125 app tags,
   v0.1.0 → **v3.7.8**, 2026-09-17); no `-rc/-beta` app pre-releases. The same
   repository carries four other tag families that are NOT app releases: the
   chart tags `helm-loki-X.Y.Z` (253, chart 3.0.0 → 7.3.0, see §7), the
   operator tags `operator/v0.x` (15), `helm-loki-6.x-weekly.kNNN`
   development builds, and one stray unprefixed `2.8.3`. A strict
   `^v(?P<version>\d+\.\d+\.\d+)$` selects exactly the app channel.
   **This is the trap discovery fell into**: version-sorting tags puts
   `helm-loki-7.3.0` (chart 7.x) above `v3.7.8` (app 3.x), so the chart family
   looked canonical — the actions-runner-controller lineage trap, sharpened by
   the chart major being numerically larger than the app major.
2. **Minor lines are maintained in parallel for a while** (release branches
   `release-X.Y.x` + backport workflow; 3.5.x patched into 2026-03, 3.6.x
   into 2026-09, 2.9.x into 2024). `lineage: minor`. Cadence: a minor every
   ~6 months (3.5 Jul 2025, 3.6 Nov 2025, 3.7 Mar 2026, 3.8 in flight on
   main), patches roughly weekly. No published per-minor support-window
   policy page (unlike grafana) — the release-notes `cadence.md` describes
   process, not EOL dates.
3. **Release notes are a release-please CHANGELOG read on the release
   BRANCH, not at the tag and not on main.** `CHANGELOG.md` sections
   `## [X.Y.Z](…compare…) (date)` with `### ⚠ BREAKING CHANGES` /
   `### Features` / `### Bug Fixes` / `### Documentation` groups; security
   dependency bumps arrive as `**security/HIGH/…:** Update module …
   [SECURITY]` bullets. Format exists since **2.8.9** (Feb 2024). Per-patch
   sections accumulate on `release-{{major}}.{{minor}}.x` (branches exist for
   2.5.0+); **main only carries the in-flight minor** (`## [3.8.0]`,
   sections for minors' .0/.1s and two out-of-order anomalies). The section
   for a release is merged AFTER the tag is cut (at tag v3.7.8 the branch
   changelog's top section is 3.7.7), so the file is read at the branch head,
   never at the tag — the grafana "docs merged after the cut" pattern, but
   per-branch. Upstream defect: **3.7.3 has two sections** on
   release-3.7.x (a 2026-06-24 re-cut comparing v3.7.3…v3.7.3 — selected by a
   heading prefix match — and the real 2026-06-23 one); a heading selector
   reads the first, so 3.7.3's bug-fix bullets come from the second-choice
   evidence (GitHub release body) if the fallback fires.
4. **GitHub release bodies duplicate the changelog section** (release-please
   `--release-type simple`): v3.7.8 body == the `## [3.7.8]` section
   [200]. api.github.com anonymous is quota-limited (60/h), so the body is
   the fallback, not the primary. **Release assets are 47 predictable
   per-OS/arch files** [200]: `loki`, `logcli`, `lokitool`, `loki-canary`
   and (≤ 3.6.0) `promtail` as `-linux-amd64.zip`, `-darwin-arm64.zip`,
   `-windows-amd64.exe.zip`, …, plus `loki_3.7.8_amd64.deb` /
   `loki-3.7.8-1.x86_64.rpm` style packages and `SHA256SUMS`, under
   `releases/download/v{{tag}}/`.
5. **Curated per-minor release notes + upgrade notes are docs on main:**
   `docs/sources/release-notes/vX-Y.md` (v2-3 … v3-7 + `next.md`), sections
   `## Features and enhancements`, `## Deprecations`, `## Upgrade
   Considerations` (the `**BREAKING CHANGE - area:**` list), `## Bug fixes`
   (with one `### X.Y.Z (date)` subsection per patch). Every v3-x file has
   all four sections; the v2-x files only Features/Bug fixes. The upgrade
   guide `docs/sources/setup/upgrade/_index.md` is cumulative per minor:
   `## 3.6.0 / 3.5.0 / 3.4.0 / 3.3.0 / 3.2.0 / 3.0.0` sections (each with
   `### Loki X.Y.Z` behaviour-change subsections) plus `## Main / Unreleased`
   for the in-flight minor — **there is no 3.1.0 and no 3.7.0 section**
   (3.7's breaking changes live in v3-7.md instead). 2.x guidance is
   `upgrade-2.x.md` (`## 2.9.0` … `## 2.0.0` per-minor sections) and
   `upgrade-1.x.md`; `upgrade-from-2x/`, `upgrade-to-6x/`,
   `upgrade-to-community/` are **chart** migration guides (see §7).
6. **The schema / index-format upgrade pain** (the famous story) is prose in
   those docs, not machine-readable: the 3.0 section's "Structured Metadata,
   Open Telemetry, Schemas and Indexes" (schema v13 + tsdb required for
   structured metadata), and `Main / Unreleased` already carries the 3.8-era
   `TSDB schema v14` + "TSDB head WAL chunk records use a new binary format"
   ("previously released Loki versions (3.7.x and earlier) cannot read the
   new format"). Storage-backend removals (boltdb, Cassandra, DynamoDB,
   BigTable/GCP, gRPC in 3.8's list) are the other half of the same story.
   The definition captures them as `upgrade-guide`-role note items; there is
   no per-release schema-version table to join against (recorded gap).
7. **The Helm chart was forked in March 2026 — chart topology is the wildest
   part of this product.**
   - The **OSS chart moved out**: source `github.com/grafana-community/helm-charts`
     `charts/loki` (tags `loki-X.Y.Z`, 541 [ls-remote 200]); published BOTH as
     an index (`grafana-community.github.io/helm-charts` [200], 546 versions
     0.5.0 → **18.13.7**) **and as OCI** (`ghcr.io/grafana-community/helm-charts/loki`
     [200 anonymous token; 547 chart tags]). Chart 18.13.7 ships appVersion
     3.7.8; **chart versions are fully independent of app versions** →
     `lookup` by `appVersion == {{.Version}}` (unprefixed; argo-cd/grafana
     precedent). Not every app release becomes an appVersion (**3.7.0 has
     none**), and many chart versions share one appVersion (44 charts ship
     appVersion 3.7.2) → artifact is optional, `select: latest`.
     `kubeVersion: >=1.25.0-0` (enforced since community chart 7.0.0); GEL
     support removed from the community chart in 8.0.0.
   - The **in-repo chart `production/helm/loki` is now the GEL chart** ("Helm
     chart for Grafana Enterprise Logs"): its own version line 7.x
     (`helm-loki-7.x` tags), published through the `grafana/helm-charts`
     reusable workflow to the OLD index `grafana.github.io/helm-charts` [200]
     (344 loki entries: 0.5.0→6.55.0 monorepo era **plus** the GEL 7.0.0→7.3.0)
     and to `ghcr.io/grafana/helm-charts/loki` [200; 36 tags 6.20.0→7.3.0].
     **Two different "loki" charts both have a 7.0.0** (community 2026-03-21,
     the Kubernetes-1.25 cleanup major; old-index/GEL 2026-04-23) — the old
     index must NOT be a lookup channel for the OSS product, or app 3.6.8
     resolves to the GEL chart 7.1.0. The in-repo chart is declared related
     (Enterprise), exactly like grafana-enterprise artifacts were excluded.
   - **Sibling charts are frozen in the old index** (2025-10-31, none were
     carried to the community index): `promtail` 6.17.1 (app 3.5.1),
     `loki-canary` 0.14.1, `loki-distributed` 0.80.6 (app 2.9.13),
     `loki-simple-scalable` 1.8.11 (2022), `loki-stack` 2.10.3 (app
     **v**-prefixed `v2.9.3`; its source also lives in-tree at
     `production/helm/loki-stack` — the loki-stack pins kube-prometheus-stack's
     record asked about). Chart-era appVersion shapes: `v`-prefixed before
     chart 3.x (app ≤ 2.6.x), unprefixed since, plus junk `k227…k232` weekly
     appVersions.
   - `docs/sources/setup/upgrade/upgrade-to-community/_index.md` is the
     authoritative statement: "Chart version 6.55.0 (appVersion 3.6.7) was
     the last release from the Loki repository… The Grafana Community now
     owns maintenance of the Loki Helm Chart, while Grafana Labs continues to
     maintain the Helm Chart for Grafana Enterprise Logs (GEL)."
8. **Promtail is end of life** — sharper than the mission's "deprecated since
   3.4" lead: deprecated in **Loki 3.0** (v3-0.md Deprecations: "Promtail is
   deprecated, as the code has been merged into Grafana Alloy"), code removed
   in 3.7.3 (#21248), and the docs page states **"Promtail is end of life
   (EOL) as of March 2, 2026. Commercial support has ended. No future support
   or updates will be provided."** (`docs/sources/send-data/promtail/_index.md`
   at main; lambda-promtail explicitly not included). The last promtail
   release artifacts are **3.6.0**: `promtail-linux-amd64.zip` and
   `docker.io/grafana/promtail` exist through v3.6.0 [200] and 404 for every
   3.7.x. → a lifecycle statement (ingress-nginx/traefik construct) + promtail
   artifacts with `availability: < 3.7.0`.
9. **Images on Docker Hub, tag = version without `v`:** `docker.io/grafana/loki`
   (all releases), `grafana/loki-canary`, `grafana/logcli`, and `grafana/promtail`
   (≤ 3.6.0), published via Grafana's GAR prod mirror
   (`us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror` —
   anonymous DENIED, private) → Docker Hub. **Docker Hub answered anonymously
   during this onboarding** (auth.docker.io token + registry-1 manifest 200
   for grafana/loki:3.7.8 — friendlier than earlier waves recorded), but it
   stays quota-flaky, so `mirror.gcr.io/grafana/*` is the verified fallback
   [200 for all four images]. **Neither loki chart has CRDs** (no `crds/` in
   the community chart or in `production/helm/loki` [404/raw checks]; the
   CRDs users meet are the operator's `loki.grafana.com` Lokistack etc.).
10. **Security channel**: GitHub repository advisories —
    `api.github.com/repos/grafana/loki/security-advisories?state=published`
    answers `[]` today [200, empty], and the global
    `/advisories?affects=grafana/loki` feed also returns none; dependency
    CVE fixes ship as `[SECURITY]` Renovate bullets inside the changelog
    sections, which the notes channel therefore carries. No CVE page on
    grafana.com dedicated to loki.

## 1. Versions

- `git ls-remote --tags` [200]: 396 tags; `^v(?P<version>\d+\.\d+\.\d+)$`
  keeps 125 (v0.1.0 → v3.7.8), rejects `helm-loki-*` (253), `operator/v0.*`
  (15), `pkg/logql/syntax/v…`, `helm-loki-6.x-weekly.kNNN`, unprefixed
  `2.8.3`. Latest stable **v3.7.8** (2026-09-17); main is 3.8.0-dev
  (`## [3.8.0]` changelog section, `Main / Unreleased` upgrade section).
- GitHub releases exist for every app tag (release-please, not drafts).

## 2. Release notes (per patch)

- Primary: `CHANGELOG.md` at branch head `release-{{major}}.{{minor}}.x`
  [RAW 200], `markdown-section` heading `^\[{{version}}\]`, availability
  ≥ 2.8.9 (release-please era); branches exist for 2.5.0+ but the bracketed
  section format starts 2.8.9. Group headings inside: `⚠ BREAKING CHANGES`,
  `Features`, `Bug Fixes`, `Continued`, `Documentation`.
- Fallback: GitHub release body (identical content; api.github.com quota).
- Older eras (< 2.8.9): old-format changelog headings (`## 2.2.1 (2021/04/05)`,
  component sections `## Loki Canary`, `## Logcli`) — not worth a selector;
  GitHub release bodies are the fallback there.

## 3. Curated notes and upgrade guidance (per minor, docs at main)

- `docs/sources/release-notes/v{{major}}-{{minor}}.md` [RAW 200]: `##
  Deprecations` and `## Upgrade Considerations` are the high-value sections
  (v3 files only); attach at X.Y.0 path entries, availability ≥ 3.0.0.
- `docs/sources/setup/upgrade/_index.md` [RAW 200]: section `## X.Y.0` per
  minor, `### Loki X.Y.Z` + `#### topic` subsections; exceptions needed for
  3.1.0 and 3.7.0 (no section). `upgrade-2.x.md` for 2.x minors,
  `upgrade-1.x.md` for 1.x.
- Chart-migration guides: `upgrade-to-community/_index.md` (6.55→18.x, twelve
  chart majors of breaking changes, GEL warning), `upgrade-from-2x/`,
  `upgrade-to-6x/` — chart-scoped, cited from the chart artifact/research.

## 4. Images

| image | tags | verified |
|---|---|---|
| `docker.io/grafana/loki` | `X.Y.Z` (= version) | 3.7.8, 3.0.0, 2.9.10 [200 both registries] |
| `docker.io/grafana/loki-canary` | `X.Y.Z` | 3.7.8, 3.0.0 [200] |
| `docker.io/grafana/logcli` | `X.Y.Z` | 3.7.8 [200] |
| `docker.io/grafana/promtail` | `X.Y.Z` ≤ 3.6.0 | 3.6.0/3.5.1/3.0.0 [200]; 3.7.0+ [404] |

Canonical channel first (docker.io), `mirror.gcr.io/grafana/*` fallback.

## 5. Release assets (predictable names, `http` channel, release-asset representation)

`https://github.com/grafana/loki/releases/download/v{{tag}}/<name>` [200]:
`loki-linux-amd64.zip`, `logcli-linux-amd64.zip`, `lokitool-linux-amd64.zip`,
`loki_{{version}}_amd64.deb`, `loki-{{version}}-1.x86_64.rpm` (+ arm/arm64,
darwin/freebsd/windows variants; `SHA256SUMS`). `promtail-linux-amd64.zip`
[200 ≤ 3.6.0, 404 ≥ 3.7.0].

## 6. Helm chart (OSS = community; lookup by appVersion)

- lookup `appVersion == {{.Version}}`, `select: latest`, optional (3.7.0 has
  no chart; earlier misses exist).
- channels: `helm-repo https://grafana-community.github.io/helm-charts`
  (index carries appVersion — the only channel kind that can serve the
  lookup), `helm-git github.com/grafana-community/helm-charts charts/loki
  ^loki-(\d+\.\d+\.\d+)$`, `oci ghcr.io/grafana-community/helm-charts/loki`
  (serves probes + the chart package layer).
- contents: `helm-values` + `chart-metadata` (kubeVersion `>=1.25.0-0`,
  appVersion pin) read from the **published OCI chart layer**
  (`published-oci-chart`; karpenter's recorded hope), `compareWith` the
  source tree at tag `loki-{{chart version}}` (`charts/loki/values.yaml`).

## 7. What is deliberately NOT modeled

- The in-repo GEL chart (`production/helm/loki`, `helm-loki-7.x` tags, old
  index + `ghcr.io/grafana/helm-charts/loki`): Enterprise artifact set of the
  same tags, the grafana-enterprise precedent. Its 7.0.0 version collision
  with the community chart is recorded as an ambiguity.
- Frozen sibling charts (promtail, loki-canary, loki-distributed,
  loki-simple-scalable, loki-stack) — retired channels, no releases since
  2025-10-31; loki-stack's loki/promtail/grafana dependency pins (the
  kube-prometheus-stack cross-reference) live in a frozen chart.
- The Loki Operator (sub-project, own versioning 0.x, own CRDs/images).
- `us-docker.pkg.dev/grafanalabs-global/...` mirrors (private; anonymous
  DENIED) and the weekly `kNNN` chart builds.

## 8. Hosts (truth of 2026-10-01, from this sandbox)

| host | status |
|---|---|
| github.com / raw.githubusercontent.com / codeload / releases/download | [200] |
| api.github.com | [200] anonymous 60/h (quota, not a block) |
| ghcr.io (grafana-community + grafana helm-charts) | [200] anonymous token, tags + manifests |
| grafana-community.github.io/helm-charts, grafana.github.io/helm-charts | [200] |
| auth.docker.io + registry-1.docker.io | [200] this time (anonymous token + manifest) |
| mirror.gcr.io | [200] |
| us-docker.pkg.dev (GAR prod mirrors) | DENIED (private) |
