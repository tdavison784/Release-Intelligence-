# MinIO release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here (egress policy or inferred).
`RAW` = `https://raw.githubusercontent.com/minio/minio`. Canonical git and canonical everything is `github.com/minio/minio` itself.

## 0. Executive summary (what a declarative definition MUST know)

1. **Calendar versioning, no semver anywhere in the tag train.** 523 tags: 521 `RELEASE.YYYY-MM-DDTHH-MM-SSZ` (lightweight, on the master line, 2016-03-11 → 2025-10-15), 1 `OFFICIAL.2016-02-08T00-12-28Z`, 1 `release-1434511043` (junk). The timestamp is the whole version: **13 days carry two releases** (2016-12-12 … 2023-11-01), so a date-only reading of the tag collides; the time-of-day is a real version component upstream. Upstream itself renders the tag into a numeric pseudo-semver for RPM/DEB packaging: `minio-20250907161309.0.0-1.x86_64.rpm` (`YYYYMMDDHHMMSS.0.0`).
2. **Strictly linear history.** No branches, no backports, no parallel lines: every release builds on the previous one (`mc admin update` era docs: "all upgrades are non-disruptive… upgrading all the servers simultaneously is the recommended way"; there is no forced step-wise or latest-only policy in the README guidance). SECURITY.md: "We always provide security updates for the latest release… you just need to upgrade to the latest version." Lineage for the model is therefore `linear`.
3. **The repository is no longer maintained.** README banner (master, commit 7aac2a2c, 2026-02-12): "**THIS REPOSITORY IS NO LONGER MAINTAINED.**" with alternatives AIStor Free/Enterprise; `## Source-Only Distribution` (introduced 2025-10-15, commit 9e49d5e7 "update README.md and other docs to point to source only releases"): "The MinIO community edition is now distributed as source code only. We will no longer provide pre-compiled binary releases for the community version." The last release is `RELEASE.2025-10-15T17-29-55Z` (a Security/CVE release with **no assets**), 12 months before this onboarding; master's last commit is the 2026-02-12 README update. This is the ingress-nginx retirement pattern: a lifecycle statement citing the README, not an artifact-relationship problem.
4. **Release notes = the GitHub release body only.** "## What's Changed" PR bullet lists (+ "## New Contributors"); security releases open with a "## Security" section naming the GHSA and "All users are advised to download and upgrade… immediately" (e.g. RELEASE.2025-10-15T17-29-55Z). There is no in-repo changelog, no `docs/release/` notes, and the min.io blog is date-addressed, not tag-addressed. The community docs site is a soft-404 (docs.min.io/community/… answers 200 with a "Not Found" page); www.min.io/docs redirects to the AIStor (enterprise) docs, which contain "Upgrade from open-source MinIO" but nothing per-release. GitHub releases: **521 = the tag count** (no deletions as of today), though many old releases were bulk-edited on 2026-04-24 (atom `updated` stamps) and MinIO has deleted releases historically — an availability caveat, not a current loss.
5. **Upgrade guidance is in the README at the tag**: an `## Upgrading MinIO` section present continuously from ≥ RELEASE.2019-10-12T01-39-57Z through RELEASE.2025-09-07T16-13-09Z (removed 2025-10-06 by bde0d5a2, so absent at the final tag): zero-downtime parallel upgrades, `mc admin update` (binary installs) vs container image upgrades, airgapped binary replacement, RPM/DEB parallel upgrade guidance.
6. **Artifacts, by era**: GitHub release assets `minio.<os>-<arch>.RELEASE.<tag>` (+ `.sha256sum`/`.shasum`/`.asc`/`.minisig`) and RPM/DEB packages versioned `YYYYMMDDHHMMSS.0.0` — linux-amd64 binary assets from ≥ 2021-04-22 (absent ≤ 2020-12-23; **RELEASE.2022-02-08T04-45-21Z has no assets at all**), deb/rpm from ≥ 2023-07-07 (absent ≤ 2023-05-27), nothing at 2025-10-15 (source-only era). The old binary host `dl.min.io/server/minio/release/**` answers **410 Gone** (README still points there — stale). Images: `docker.io/minio/minio` [401 anonymous] and `quay.io/minio/minio` [401 even with a valid anonymous token; quay works for other repos, so this is repo-specific]; `mirror.gcr.io/minio/minio` exists but is **empty** (no tags cached). Helm: the community chart is published from this very repository (`helm/minio` sources + root `index.yaml` reindexed by `helm-reindex.sh`) at **charts.min.io** [200]; chart versions are an independent 3.x→5.4.0 train whose `appVersion` pins a **sparse subset** of server releases (5.4.0 → RELEASE.2024-12-18T13-15-44Z; index frozen 2025-01-02).

## 1. Canonical versions

Partial clone (`--filter=blob:none`) [200]: 523 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `RELEASE.YYYY-MM-DDTHH-MM-SSZ` | 521 | `RELEASE.2024-11-07T00-52-20Z` | releases (the whole train) |
| `OFFICIAL.…` | 1 | `OFFICIAL.2016-02-08T00-12-28Z` | pre-RELEASE-era tag, excluded |
| junk | 1 | `release-1434511043` | excluded |

- Cadence: 18 (2016) … 86 (2020) … 63 (2024) **17 (2025, last on 10-15)** releases/year; gaps of hours exist (13 same-day pairs: 2016-12-12, 2019-04-18, 2020-01-16, 2020-02-07, 2020-04-15, 2020-07-11, 2020-12-03, 2021-07-08, 2022-03-11, 2022-03-17, 2022-06-02, 2022-07-24, 2023-11-01).
- GitHub releases: 521 (`Link: rel="last"; page=521` with `per_page=1`) — none deleted today; the atom feed shows a bulk `updated` 2026-04-24 across old releases (edit event).
- **Versioning model decision (the point of this onboarding)**: the component-group tagPattern construct (postgresql) assembles `(?P<major>)(?P<minor>)(?P<patch>)` into semver, so `RELEASE.2024-11-07T00-52-20Z` reads as `2024.11.7`: **major = year, minor = month, patch = day**. Ordering is exactly chronological at day granularity; `lineage: linear` traverses every release; availability constraints get month granularity (`>= 2024.11.0` = November 2024 onwards). What the encoding cannot carry: the time-of-day (the separators `T`/`-`/`Z` inside the tag cannot be stripped into a numeric component by the assembler), so each same-day pair collapses to one version — `selectReleases` reports them as `duplicates` (visible, e.g. "ignored: … 13 duplicates") and keeps the lexicographically smaller = earlier tag, dropping the **later** release of the day with its notes and assets. See the record's gaps; the two alternatives (time as `prerelease`; skipping) are strictly worse — analyzed there.

## 2. Release notes [200]

- GitHub release body at `ref: {{.Tag}}` (the only per-release notes channel):
  ```
  ## What's Changed
  * Update console to v2.0.3 by @bexsoft in https://github.com/minio/minio/pull/21474
  * fix: record extral skippedEntry for listObject by @jiuker in https://github.com/minio/minio/pull/21484
  ...
  ## New Contributors
  ```
  Security releases instead/also carry:
  ```
  ## Security
  A CVE was reported [Privilege Escalation via Session Policy Bypass in Service Accounts and STS](…/security/advisories/GHSA-jjjj-jwhf-8rgr) and fixed in this release,
  All users are advised to download and upgrade their MinIO setup immediately.
  ```
  Release *titles* carry era hints ("Bugfix Release", "Breaking Release", "Security and bug fix release" in 2020–2022; bare tags later) but the bodies are uniform PR lists.
- No `docs/release/` convention, no CHANGELOG, no release-notes files in-tree; blog.min.io [200] publishes security announcements (SECURITY.md names it the advisory channel) but posts are date-addressed → not a per-release source.
- `github.com/minio/docs` (community docs repo) exists; its rendered site is the soft-404 above. In-repo `docs/**` is design/feature documentation (bucket notifications, erasure coding…), release-agnostic.

## 3. Security

- **GitHub advisories of minio/minio** [adapter-readable]: SECURITY.md's stated disclosure channel; the 2025-10-15 release body links GHSA-jjjj-jwhf-8rgr (Privilege Escalation via Session Policy Bypass, fixed in the final release).
- SECURITY.md [200] at the tag: security updates land in the **latest release only** ("We always provide security updates for the latest release. Whenever there is a security update you just need to upgrade to the latest version.") — i.e. no backports, which is also why the history is one line.
- VULNERABILITY_REPORT.md: report to security@min.io; disclosures published on blog.min.io.

## 4. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| container image `minio/minio` | `docker.io/minio/minio` (canonical, README badge) → `quay.io/minio/minio` (chart values pin) | tag = `RELEASE.…` | **unverifiable here**: docker.io 401 anonymous (valid token, manifest 401); quay.io 401 with valid anonymous token (control `quay.io/prometheus/prometheus` = 200 → repo-specific); mirror.gcr.io/minio/minio empty (`tags: []`) |
| linux-amd64 binary | release asset `minio.linux-amd64.RELEASE.<tag>` | = release | [206] ≥ 2021-04-22; 404 ≤ 2020-12-23 (only ppc64le/windows then, dl.min.io era); 404 at 2022-02-08 and 2025-10-15 |
| binary checksums | `.sha256sum`/`.shasum` sidecars | = release | same eras [206] |
| DEB package | `minio_YYYYMMDDHHMMSS.0.0_amd64.deb` | `YYYYMMDDHHMMSS.0.0` = timestamp pseudo-semver | [206] ≥ 2023-07-07; 404 ≤ 2023-05-27 |
| RPM package | `minio-YYYYMMDDHHMMSS.0.0-1.x86_64.rpm` | same | [206] ≥ 2023-07-07 |
| source tarballs on dl.min.io | `dl.min.io/server/minio/release/…` | = release | **410 Gone** (directory and file paths; README's "Direct downloads" link is stale) |
| Helm chart `minio` | `charts.min.io` (index + `helm-releases/minio-X.Y.Z.tgz`) | independent chart train (3.x→5.4.0), `appVersion` = a sparse subset of server tags | [200]; lookup `appVersion == {{.Tag}}`, optional (argo-cd pattern); index frozen 2025-01-02 (5.4.0/RELEASE.2024-12-18T13-15-44Z newest); chart sources live in-repo at `helm/minio` (Chart.yaml version 5.4.0, values pin `quay.io/minio/minio:RELEASE.2024-12-18T13-15-44Z` + `quay.io/minio/mc:RELEASE.2024-11-21T17-21-54Z`) |
| go module | `go install github.com/minio/minio@RELEASE.…` (release body's install instruction in the source-only era) | = release | module proxy reachable (proxy.golang.org [200] per earlier onboardings); not modeled as an artifact channel |

Related products (out of scope, noted): `minio/mc` (the client — its own full CalVer train, e.g. `RELEASE.2024-11-21T17-21-54Z`; discovery wrongly proposed `version = {{.Tag}}` for the mc image), `minio/operator` (+ console; the Kubernetes operator with its own chart), `minio/directpv`, `minio/console` (folded into the server), `minio/docs` (community docs repo), the SDKs.

## 5. Hosts unreachable / degraded from this sandbox (2026-10-01)

| Host | Verdict |
|---|---|
| `api.github.com` | reachable, anonymous quota-limited (the standing tax; releases/advisories worked) |
| `dl.min.io` | reachable but **410 Gone** on `/server/minio/release/**` (decommissioned upstream, not egress) |
| `registry-1.docker.io` (minio/minio) | 401 anonymous pulls (auth.docker.io token issued fine) |
| `quay.io` (minio/minio) | 401 with valid anonymous token — repo-specific (control repo pulls 200) |
| `mirror.gcr.io` (minio/minio) | repo known but empty: no cached tags, manifests 404 |
| `www.min.io` / `docs.min.io` / `blog.min.io` | reachable [200]; community docs pages are soft-404s (content retired), enterprise docs live |
| `charts.min.io` | [200] |

## 6. Representability notes (fed to the record)

- What works unchanged: version listing/ordering (date granularity), linear path semantics, `{{.Tag}}` templates everywhere, release-body notes, README-at-tag upgrade guide, advisories, chart lookup, lifecycle statements, era windows via availability constraints.
- What the encoding costs: the 13 same-day later releases are dropped (visible as duplicates); release kinds are always `patch` (no X.Y.0 exists), so `releaseKinds` filters are meaningless; `Line()` is a calendar month, synthetic; availability is month-granular (a day-precise window cannot be written); `PrevTag` is empty at each month's first release (previous-release-in-same-line is the only shape the renderer computes) — avoid `PrevTag`-dependent sources (git-log).
- Rejected encodings: time as `(?P<prerelease>)` (every release would be a prerelease: excluded from listing without `includePrereleases`, and `selectPath` skips prereleases even then, so upgrades would collapse to the target; `prevInLineage` also skips them → PrevTag always empty); whole timestamp as `major` à la the RPM `YYYYMMDDHHMMSS.0.0` (the tag's `-`/`T`/`Z` separators cannot be stripped inside one capture, and the assembler has no transform capability — a transform would be a new construct).
