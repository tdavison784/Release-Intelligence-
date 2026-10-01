# Argo CD release-channel map (research date 2026-10-01)

Scope: upstream `github.com/argoproj/argo-cd` (UP) and community chart repo `github.com/argoproj/argo-helm` (HELM).
All evidence was gathered through `git ls-remote`, shallow clones at tags, `raw.githubusercontent.com`, GitHub release asset
downloads, `ghcr.io` token API and Docker Hub. "RAW" below means `https://raw.githubusercontent.com/argoproj/argo-cd/<ref>/<path>`
(HTTP 200 verified unless stated). `{H-RAW}` means `https://raw.githubusercontent.com/argoproj/argo-helm/<ref>/<path>`.

Not reachable from this sandbox (403 from proxy; not retried): api.github.com, github.com HTML/atom (`releases.atom`, `tags.atom`),
argoproj.github.io, argo-cd.readthedocs.io, quay.io (API and /v2), artifacthub.io, osv.dev, `pkg-containers.githubusercontent.com`
(ghcr blob download). The GitHub MCP tools in this session are scoped to a different repo and refuse `argoproj/argo-cd`.
Everything that depends on those is marked UNVERIFIED.

---

## 1. Canonical versions

Tag pattern (UP): `v{MAJOR}.{MINOR}.{PATCH}` stable, `v{MAJOR}.{MINOR}.{PATCH}-rc{N}` pre-release. Enforced by
`hack/trigger-release.sh`: `grep -E -q '^v[0-9]+\.[0-9]+\.[0-9]+(-rc[0-9]+)*$'`, and the release workflow triggers on `tags: 'v*'`
(`.github/workflows/release.yaml` lines 4-5; tags `v2.4*`, `v2.5*`, `v2.6*` are excluded).
`hack/get-previous-release/get-previous-version-for-release-notes.go` uses `^v\d+\.\d+\.(\d+)(?:-rc(\d+))?$`.

`git ls-remote --tags https://github.com/argoproj/argo-cd`: 596 ref names (peeled `^{}` entries excluded) = 458 stable tags + 133 `-rc` tags + 5 non-conforming.
Non-conforming tags that a matcher must ignore: `stable` (moving lightweight tag), `gitops-engine/v3.6.0-rc1` (sub-module tag in the
monorepo), `v0.4.0-alpha1`, `v2.1.2-hf1`, `v2.2.8-1`. Some tags are annotated (`refs/tags/v3.5.0` and `...^{}` differ:
tag object f8a2c47..., commit e95e1be...), others lightweight (v3.5.3 = c9c369e...). Always peel with `^{}` when comparing.

Latest stable per line (ls-remote, as of 2026-10-01) and tagged commit date (from shallow fetch):

| Line | latest stable | first RC | GA tag date | notes |
|---|---|---|---|---|
| 3.6 | none (only `v3.6.0-rc1`, 2026-09-16) | v3.6.0-rc1 | planned 2026-11-03 | `release-3.6` branch exists |
| 3.5 | **v3.5.3** (2026-09-14, c9c369e) | v3.5.0-rc1 2026-06-16 (rc1..rc3) | v3.5.0 2026-08-04 | patches v3.5.1 08-12, v3.5.2 08-26 |
| 3.4 | v3.4.9 (2026-09-14) | v3.4.0-rc1 (rc1..rc7) | v3.4.0 2026-05-05 | |
| 3.3 | v3.3.14 (2026-08-12) | v3.3.0-rc1 (rc1..rc4) | v3.3.0 2026-02-02 | |
| 3.2 | v3.2.12 | v3.2.0-rc1 | v3.2.0 2025-11-04 | EOL under 3-minor policy |
| 3.1 | v3.1.16 | | v3.1.0 2025-08-13 | EOL |
| 3.0 | v3.0.23 | v3.0.0-rc1 2025-03-17 | v3.0.0 2025-05-06 | EOL |
| 2.14 | v2.14.21 (2025-11-04) | v2.14.0-rc1 | v2.14.0 2025-02-03 | last 2.x line |

- **Latest stable as of 2026-10-01 = `v3.5.3`.** Evidence: `git ls-remote` shows `c9c369efcc5b2a0bd720803f8d14a1c3eaddf579 refs/tags/stable`
  and the same SHA on `refs/tags/v3.5.3`. The `stable` tag is moved by the release workflow only when the pushed tag is the highest
  non-hyphenated tag (`.github/workflows/release.yaml` L68 `LATEST_RELEASE_TAG=$(git -c 'versionsort.suffix=-rc' tag --list --sort=version:refname | grep -v '-' | tail -n1)`,
  L337 `git tag -f stable ${{ github.ref_name }}`). Note v3.4.9 and v3.3.14 are *newer in time* (or same day) but not "latest": "latest" is semver-highest.
- Supported lines: last 3 minors (`SECURITY.md` "We currently support the last 3 minor versions of Argo CD with security and bug fixes";
  `docs/developer-guide/release-process-and-cadence.md` "Only the three most recent minor versions are eligible for patch releases").
  So at 2026-10-01: 3.5, 3.4, 3.3 supported; 3.2 and older EOL.
- Cadence: minor releases quarterly (first Tuesday of Feb/May/Aug/Nov); RC1 seven weeks before GA. The "Schedule" table in
  `docs/developer-guide/release-process-and-cadence.md` lists future dates (`v3.6 | Tuesday, Sep. 15, 2026 | Tuesday, Nov. 3, 2026`).
- **2.x -> 3.x boundary**: last 2.x minor is 2.14 (v2.14.0 2025-02-03 ... v2.14.21 2025-11-04); v3.0.0 on 2025-05-06. Go module path became `/v3`
  (`.goreleaser.yaml`: `-X github.com/argoproj/argo-cd/v3/common.version=...`). `docs/operator-manual/upgrading/2.14-3.0.md` says
  "Once 3.0 is released, no more 2.x minor versions will be released. We will continue to cut patch releases for the two most recent minor versions".
  Chart major boundary: chart 8.0.0 <-> app v3.0.0 (section 5).
- Branches: `release-X.Y` (e.g. `release-3.5` fc2d94d...). Tags are cut from release branches; `VERSION` file holds `3.5.3` (no `v`) on the tag
  (but see section 10: VERSION/manifests were not bumped on a few tags).

## 2. Release notes

GitHub Release bodies are not readable here (UNVERIFIED content). What is verifiable in-repo:

1. **`CHANGELOG.md` is STALE.** Top entry is `## v2.4.8 (2022-07-29)` at `master`, `v3.6.0-rc1`, `v3.5.3`, `v3.3.14`, `v3.0.0`, `v2.14.0`, `v2.5.0`
   (RAW `{ref}/CHANGELOG.md`, 2863 lines). Do not use it as a source for any release after v2.4.8. Structure when it was maintained:
   `## v2.4.8 (2022-07-29)` / `### Bug fixes` / `### Other changes` / `### Features`, bullets of conventional-commit titles `- fix: ... (#10058)`.
   `docs/developer-guide/releasing.md` still says "the CHANGELOG.md should be updated" (outdated).
2. **GitHub Release body is generated by GoReleaser at tag time** (`.goreleaser.yaml`, `release:` and `changelog:` blocks; workflow `.github/workflows/release.yaml` job `goreleaser`):
   - `release.header` (static): `## Quick Start` (Non-HA / HA `kubectl apply -n argocd --server-side --force-conflicts -f https://raw.githubusercontent.com/argoproj/argo-cd/{{.Tag}}/manifests/install.yaml` and `.../manifests/ha/install.yaml`), `## Release Signatures and Provenance`, `## Release Notes Blog Post` (a hard-coded link to the v3.0 RC blog post, never updated), `## Upgrading` ("If upgrading from a different minor version, be sure to read the upgrading documentation" -> `.../operator-manual/upgrading/overview/`).
   - `changelog: use: github, sort: asc, abbrev: 0` with groups (regex on PR/commit title): `Breaking Changes` (`^.*?(\([[:word:]]+\))??!:.+$`, order 0), `Features` (`feat`), `Bug fixes` (`fix`), `Documentation` (`docs`), `Dependency updates` (`(feat|fix|chore)\(deps?...`), `Other work` (default, order 999). Excludes `^test:`, `Bump...`, `[Bot]...`.
   - `release.footer`: `**Full Changelog**: https://github.com/argoproj/argo-cd/compare/{{ .PreviousTag }}...{{ .Tag }}`.
   - `release.prerelease: auto` (rc tags -> pre-release); `make_latest: '{{ .Env.GORELEASER_MAKE_LATEST }}'` (true only when tag is the semver-highest stable).
   - Previous tag for the compare link: `hack/get-previous-release/get-previous-version-for-release-notes.go <tag>` = highest semver tag strictly lower than the new tag; for `vX.Y.0` it skips tags of the same minor series (so v3.5.0's previous is the highest v3.4.x).
3. Legacy generator `hack/generate-release-notes.sh NEW_REF OLD_REF NEW_VERSION` (git-log based; sections `## Quick Start`, `## Release signatures`, `## Upgrading`, `## Changes` with `### Features (N)`, `### Bug fixes (N)`, `### Documentation (N)`, `### Other (N)`; it drops commits matching `Merge pull request from GHSA`). Whether it is still used for releases: UNVERIFIED (goreleaser is the active path).
4. Known gap documented in-repo: `docs/operator-manual/upgrading/2.14-3.0.md` section "Images missing release notes on GitHub": "Images 3.0.7 - 3.0.10 are missing release notes on GitHub" (GoReleaser darwin build issue, PR #23507). So release-note absence is a real failure mode.
5. Deterministic substitute for notes: `git log` between tags on release branch (conventional-commit titles `feat|fix|docs|chore(scope): ... (#PR)`), plus the upgrade guide (section 3). Squash-merge titles end in `(#NNNN)`.
6. Other generated in-repo artifacts: `docs/snyk/index.md` + `docs/snyk/<version>/*.html` (weekly Snyk scans of master and latest patch of 3 supported minors; table per version with Critical/High/Medium/Low counts, e.g. `### v3.4.3 ... | [argocd:v3.4.3](v3.4.3/quay.io_argoproj_argocd_v3.4.3.html) | 0 | 1 | 62 | 16 |`); `SECURITY-INSIGHTS.yml` (`project-release: v3.5.0`, `distribution-points: https://github.com/argoproj/argo-cd/releases, https://quay.io/repository/argoproj/argocd`); docs blog post links; `docs/user-guide/...`/`docs/operator-manual/*.md` (rendered doc site, readthedocs UNVERIFIED).

## 3. Upgrade / migration docs

Path: `docs/operator-manual/upgrading/{PREV}-{CUR}.md` where PREV/CUR are `MAJOR.MINOR` (no `v`), plus `overview.md`, plus
`ui-extensions-react-19-upgrading.md`. Directory at v3.5.3 (31 files): `1.0-1.1.md ... 1.8-2.0.md, 2.0-2.1.md ... 2.13-2.14.md, 2.14-3.0.md, 3.0-3.1.md, 3.1-3.2.md, 3.2-3.3.md, 3.3-3.4.md, 3.4-3.5.md, overview.md`.
Note the major jumps: `1.8-2.0.md` (no 1.9) and `2.14-3.0.md` (previous minor of 3.0 is the last 2.x minor, 2.14).

Verified (HTTP 200 at the target `.0` tag): `v3.0.0/docs/operator-manual/upgrading/2.14-3.0.md` (506 lines), `v3.1.0/.../3.0-3.1.md` (86 lines),
`v3.5.0/.../3.4-3.5.md` (320 lines); also `v3.2.0 3.1-3.2.md`, `v3.3.0 3.2-3.3.md`, `v3.4.0 3.3-3.4.md`, `v3.6.0-rc1 3.5-3.6.md` (200), and `v3.5.3 3.5-3.6.md` is 404 (not yet written on 3.5 line; exists on `master` and `release-3.6`).
Docs for future minors appear first at the RC tag (e.g. `3.5-3.6.md` already at `v3.6.0-rc1`), and are edited after GA (so read at the highest patch tag/release branch, not only `.0`).

Overview (`overview.md`): has a list `- [v3.4 to v3.5](./3.4-3.5.md)` ... newest first, plus semver rules (patch: no breaking changes; minor: "might introduce minor changes with a workaround"; major: backward incompatible), and the upgrade command with `--server-side --force-conflicts`.

Heading structure (H1 `# vX.Y to X.Z`; H2 sections; H3 per item; H4 `#### Detection`, `#### Remediation`/`#### Migration`):
- `3.4-3.5.md`: `# v3.4 to 3.5` / `## Breaking Changes` (H3 items: `###  Helm was upgraded to \`4.2.0\`.`, `### UI extensions must externalize \`react/jsx-runtime\``, `### Event-listing gRPC methods now return an Argo CD \`EventList\` type`) / `## Behavioral Improvements / Fixes` / `## API Changes` / `## Security Changes` / `## Deprecated Items` / `## Kustomize Upgraded` / `## Helm Upgraded` / `## Custom Healthchecks Added` / `## Other Changes`.
- `3.3-3.4.md`: identical skeleton (`## Breaking Changes`, `## Behavioral Improvements / Fixes`, `## API Changes`, `## Security Changes`, `## Deprecated Items`, `## Kustomize Upgraded`, `## Helm Upgraded`, `## Dex Upgraded`, `## go-oidc Upgraded`, `## Open Telemetry Upgraded`, `## Custom Healthchecks Added`).
- `3.0-3.1.md`: older/looser style: H2 per change (`## Symlink protection in API \`--staticassets\` directory`, `## v1 Actions API Deprecated`, `## OpenID Connect ... PKCE ...`, `## Helm Upgraded to 3.18.4`, `## Kustomize Upgraded to 5.7.0`, `## Breaking Changes` (placed late, near-empty), `## Sanitized project API response`, `## Added Healthchecks`). So "Breaking Changes" is NOT always the first/only place; breaking items can be plain H2s. `3.1-3.2.md` and `3.2-3.3.md` use `## Breaking Changes` with H3 items for the first section and H2 for others.
- `2.14-3.0.md`: `# v2.14 to 3.0` / intro paragraph / `## Images missing release notes on GitHub` / `## Breaking Changes` (H3 each, H4 `#### Detection`, `#### Remediation`, `##### Quick remediation`, `#### Migration`, `#### Opting Out`) / `## Other changes` / `## Added Healthchecks`.
- Excerpt (3.4-3.5): "In Helm v4, the OCI implementation is more strict ... This is a breaking change for Argo CD users with plain HTTP OCI registries."
- Excerpt (2.13-2.14, H2 at top): "## Avoid v2.14.0 manifests and use v2.14.1 / The tagged v2.14.0 manifests contain a nonexistent Argo CD image."
- Bundled-tool bumps are under headings `## Helm Upgraded[ to X]`, `## Kustomize Upgraded[ to X]`, `## Dex Upgraded` (good for tool-version change detection).
- Healthcheck additions: `## Added Healthchecks` / `## Custom Healthchecks Added` bullet list of `[group/Kind](commit-url)`.

## 4. Compatibility

**Tested Kubernetes versions**: `docs/operator-manual/tested-kubernetes-versions.md` (included into `docs/operator-manual/installation.md` L122-126 via `{!docs/operator-manual/tested-kubernetes-versions.md!}` under `## Tested versions`). File is a bare markdown table, regenerated per release branch by `hack/update-supported-versions.sh` from `.github/workflows/ci-build.yaml` `jobs.test-e2e.strategy.matrix.k3s[].version` (v3.5.3 L478-486: `v1.36.0` (latest: true), `v1.35.0`, `v1.34.2`, `v1.33.1`).
Content at v3.5.3 (RAW `v3.5.3/docs/operator-manual/tested-kubernetes-versions.md`):
```
| Argo CD version | Kubernetes versions |
|-----------------|---------------------|
| 3.5 | v1.36, v1.35, v1.34, v1.33 |
| 3.4 | v1.35, v1.34, v1.33, v1.32 |
| 3.3 | v1.35, v1.34, v1.33, v1.32 |
```
Header exactly `| Argo CD version | Kubernetes versions |`; row key `MAJOR.MINOR` (no `v`), value comma-separated `vK.M` newest-first; 3 rows = current + 2 previous lines; at `v3.6.0-rc1` first row is `3.6 | v1.37, v1.36, v1.35, v1.34`.
Caveats: the file exists since v2.8.0 (404 at v2.7.0/v2.6.0); rows for older lines are re-read from their release branches at generation time so they drift (the `3.3` row was `v1.34..v1.31` at v3.3.0 and `v1.35..v1.32` at v3.4.0); v2.12.0 and v2.12.1 have an EMPTY row (`| 2.12 |  |`). Use the FIRST row of the file at the tag (current line), not older rows.
No server-side minimum is declared in upstream beyond this CI matrix (UNVERIFIED whether docs state a hard minimum elsewhere).

**Helm chart constraints** (HELM `charts/argo-cd/Chart.yaml` at `argo-cd-10.9.5`): `kubeVersion: ">=1.25.0-0"`. History across all 790 chart tags (verified by downloading every `Chart.yaml`): no `kubeVersion` in 3.0.0..5.9.1; `>=1.22.0-0` 5.10.0..5.34.6; `>=1.23.0-0` 5.35.0..7.3.11; `>=1.25.0-0` 7.4.0..10.9.5. Helm client: no Chart.yaml constraint; `charts/argo-cd/README.md.gotmpl` "## Prerequisites": "Kubernetes `>=1.25.0-0` ... We align with Amazon EKS calendar ... Helm v3.0.0+"; chart publish CI uses Helm v4.2.3 (`.github/workflows/publish.yml` L34). Chart `apiVersion: v2`. Chart dependency: `redis-ha` 4.38.0 from `https://dandydeveloper.github.io/charts/` (condition `redis-ha.enabled`).
Note the chart floor (1.25) is below the oldest tested upstream K8s (1.33 for 3.5): chart kubeVersion is not the tested matrix.

## 5. Helm chart (argo-helm)

- Source: `https://github.com/argoproj/argo-helm`, chart dir `charts/argo-cd/` (`Chart.yaml`, `Chart.lock`, `README.md`, `README.md.gotmpl`, `values.yaml`, `templates/`, `ci/`). No `CHANGELOG.md` in the chart dir ({H-RAW} `argo-cd-10.9.5/charts/argo-cd/CHANGELOG.md` = 404).
- **Tags**: `argo-cd-{chartVersion}` (e.g. `argo-cd-10.9.5`). `git ls-remote --tags https://github.com/argoproj/argo-helm`: 1385 tags, 790 `argo-cd-*` (other prefixes: `argo-workflows-`, `argo-rollouts-`, `argo-events-`, `argocd-apps-`, `argocd-image-updater-`, `argocd-applicationset-`, `argocd-notifications-`, `argo-`). Highest: `argo-cd-10.9.5` (2026-09-30). Created by `helm/chart-releaser-action` v1.7.0 on every push to `main` touching `charts/**` (`.github/workflows/publish.yml` L3-9, L66-71; `.github/configs/cr.yaml`: `generate-release-notes: true`, `sign: true`). Each tag has a GitHub Release with `argo-cd-X.Y.Z.tgz` and `.tgz.prov` (HTTP 200 for 10.9.5, 8.0.0, 7.7.0 `https://github.com/argoproj/argo-helm/releases/download/argo-cd-X.Y.Z/argo-cd-X.Y.Z.tgz`).
- **HTTP Helm repo index** (canonical `https://argoproj.github.io/argo-helm/index.yaml`) is blocked, but the same file is the `gh-pages` branch: **`https://raw.githubusercontent.com/argoproj/argo-helm/gh-pages/index.yaml`** (HTTP 200, 1.65 MB, `generated: 2026-10-01T02:42:32Z`, 936 `argo-cd` entries with `version`, `appVersion`, `created`, `kubeVersion`, `urls: [https://github.com/argoproj/argo-helm/releases/download/argo-cd-X.Y.Z/argo-cd-X.Y.Z.tgz]`; early entries have relative urls). Cross-check: for all 790 tags, `Chart.yaml.version == tag suffix` and `appVersion == index appVersion` (0 mismatches). 148 old index entries (0.1.0 .. 2.17.5) have no git tag; tags `argo-cd-3.26.6` and `argo-cd-5.19.13` exist without index entries; there is no 10.6.2 anywhere.
- **OCI**: `oci://ghcr.io/argoproj/argo-helm/argo-cd`. Anonymous token `https://ghcr.io/token?scope=repository:argoproj/argo-helm/argo-cd:pull` then `/v2/argoproj/argo-helm/argo-cd/tags/list?n=1000` works (HTTP 200): 390 tags, `5.43.0` ... `10.9.5`, all plain semver, no `v`, no rc. All OCI tags are a subset of index versions (546 older index versions are not in OCI). Manifest (`Accept: application/vnd.oci.image.manifest.v1+json`) config mediaType `application/vnd.cncf.helm.config.v1+json`, layers `...chart.content.v1.tar+gzip` and `...chart.provenance.v1.prov`; 10.9.5 manifest annotations include `org.opencontainers.image.version: 10.9.5`, `artifacthub.io/changes`, `org.opencontainers.image.created: 2026-09-30T15:34:39Z` (older tags 8.0.0/7.7.0 have no annotations). The manifest does NOT carry `appVersion`; the config blob (Chart.yaml as JSON) does but blob download redirects to `pkg-containers.githubusercontent.com` (blocked here -> UNVERIFIED).
- **Version relationship**: chart version is independent semver (CONTRIBUTING.md "Versioning": Major = breaking chart change, Minor = new functionality / major application updates, Patch = app patch updates / backwards-compatible features); `appVersion` = upstream tag WITH `v` (e.g. `v3.5.3`), exactly equal to an existing upstream stable tag in all cases (never an rc; verified against 458 stable tags). Chart major <-> app minors (from index): 7.x -> 2.11..2.14; 8.x -> 3.0..3.1; 9.x -> 3.1..3.4; 10.x -> 3.4..3.5. Only 8.0.0 coincides with an app major bump ("In this release we upgrade the Helm chart to deploy the next major version of Argo CD (v3.0.0)" README `### 8.0.0`); 9.0.0 and 10.0.0 are chart-level breaking changes (README: `configs.params` defaults removed; `global.networkPolicy.create` default false -> true).
- **Deterministic appVersion -> chart versions**: (a) fetch `gh-pages/index.yaml`, filter entries `name: argo-cd` where `appVersion == "v"+Version`; or (b) `{H-RAW}/argo-cd-{X.Y.Z}/charts/argo-cd/Chart.yaml` for each tag from `git ls-remote` (fields `version`, `appVersion`, `kubeVersion`). Result can be a list of several chart versions, or EMPTY: of 144 upstream stable tags >= v2.11.0 only 76 were ever an appVersion (chart tracks the newest minor line only; patch jumps are common). Never shipped: v2.14.0 (chart jumped to v2.14.1), v3.4.0 (chart went v3.3.9 -> v3.4.1), v3.4.7, v3.4.8, v3.4.9, v3.3.10-v3.3.14, v3.2.10-v3.2.12.
  Example mappings (each verified from `Chart.yaml` at the tag AND the index):
  | appVersion | chart versions | evidence |
  |---|---|---|
  | v3.5.3 | 10.9.1, 10.9.2, 10.9.3, 10.9.4, 10.9.5 | `argo-cd-10.9.1` (2026-09-14) and `argo-cd-10.9.5` (2026-09-30) `appVersion: v3.5.3` |
  | v3.5.0 | 10.2.3, 10.3.0, 10.3.1, 10.3.2 | `argo-cd-10.2.3` (2026-08-04, artifacthub change "Bump argo-cd to v3.5.0"), `argo-cd-10.3.2` |
  | v3.0.0 | 8.0.0, 8.0.1 | `argo-cd-8.0.0` (2025-05-07) `appVersion: v3.0.0`, artifacthub change "Bump argo-cd to v3.0.0" |
  | v2.13.0 | 7.7.0, 7.7.1, 7.7.2, 7.7.3 | `argo-cd-7.7.0` (2024-11-05) |
  | v3.1.8 | 8.5.8, 8.5.9, 8.5.10, 8.6.0-8.6.4, 9.0.0 (9 versions; spans a chart major) | `argo-cd-9.0.0` (2025-10-17) `appVersion: v3.1.8`; index |
  | v3.4.9, v3.4.0, v3.3.14, v2.14.0, v3.6.0-rc1 | (none) | never an appVersion |
- **Chart "changelog"**: `Chart.yaml` annotation `artifacthub.io/changes` (YAML literal, list of `- kind: added|changed|fixed|...` / `description: ...`), exactly ONE entry per chart version (`scripts/lint-changelog.sh` fails the PR if >1; "previous entries are captured in git history"). Example at 10.9.5: `- kind: added / description: Add \`redis.existingSecret\` to use a custom Secret name...`. Also annotation `artifacthub.io/signKey` (fingerprint `2B8F22F57260EFA67BE1C5824B11F800CD9D2252`). Breaking/upgrade notes live in `charts/argo-cd/README.md` (generated from `README.md.gotmpl`) section `## Changelog` with `### 10.0.0`, `### 9.1.0`, `### 9.0.0`, `### 8.0.0`, `### 7.9.0`, `### 7.0.0` ... (only "highlighted" versions). ArtifactHub changelog UNVERIFIED (blocked).
- `values.yaml`: `charts/argo-cd/values.yaml` ({H-RAW} `argo-cd-10.9.5/charts/argo-cd/values.yaml` 200). `global.image.repository: quay.io/argoproj/argocd`, `global.image.tag: ""` ("defaults to the chart appVersion"). Chart-pinned dependencies differ from upstream install.yaml: at 10.9.5 `dex.image` `ghcr.io/dexidp/dex:v2.45.1`, `redis.image` `ecr-public.aws.com/docker/library/redis:8.6.4-alpine` (upstream v3.5.3 install.yaml has redis `8.2.3-alpine`); at 9.4.0 dex `v2.44.0`, redis `8.2.3-alpine`; at 8.0.0 dex `v2.42.1`, redis `7.2.8-alpine` (upstream v3.0.0: dex v2.41.1, redis 7.2.7-alpine). So the chart is NOT a mirror of upstream manifests; chart patch bumps also change dex/redis independently of appVersion.

## 6. Images

- **Release image (authoritative)**: `quay.io/argoproj/argocd:{{.Tag}}` (tag keeps the `v`: `v3.5.3`, `v3.6.0-rc1`). Platforms `linux/amd64,linux/arm64,linux/s390x,linux/ppc64le`. Cosign keyless-signed + SLSA3 provenance.
  Evidence: `.github/workflows/release.yaml` L87-88 (`IMAGE_NAMESPACE="${{ vars.IMAGE_NAMESPACE || 'argoproj' }}"`, `IMAGE_REPOSITORY="${{ vars.IMAGE_REPOSITORY || 'argocd' }}"`), L94 `echo "quay_image_name=quay.io/$IMAGE_NAMESPACE/$IMAGE_REPOSITORY:${{ github.ref_name }}"`, L32 `platforms: linux/amd64,linux/arm64,linux/s390x,linux/ppc64le`; `.github/workflows/image-reuse.yaml` (docker/build-push-action `tags: ${{ env.TAGS }}`, cosign sign step); `Makefile` L167 `IMAGE_NAMESPACE?=`, L227-233 defaults `IMAGE_REGISTRY="quay.io"`, `IMAGE_NAMESPACE="argoproj"`, `IMAGE_REPOSITORY=argocd`; `hack/update-manifests.sh` (same defaults, `IMAGE_TAG=v$(cat VERSION)` on `release-*` branch, `kustomize edit set image quay.io/argoproj/argocd=...:${IMAGE_TAG}`); `hack/trigger-release.sh` prints "Images: quay.io/argoproj/argocd:${NEW_TAG}"; `SECURITY-INSIGHTS.yml` `distribution-points: https://quay.io/repository/argoproj/argocd`.
  quay.io itself is blocked (403): existence of any specific tag in the registry is UNVERIFIED directly; indirect proof = install manifests reference it and are what the docs tell users to apply.
- **ghcr.io/argoproj/argo-cd/argocd = master snapshot builds only, NOT release tags.** Verified via ghcr token API (`https://ghcr.io/v2/argoproj/argo-cd/argocd/tags/list`, 11848 tags): tags are `{VERSION}-{sha8}` (e.g. `3.7.0-f955a903`; `.github/workflows/image.yaml` L49 `TAG="$(cat ./VERSION)-${GITHUB_SHA::8}"`, L67 `ghcr_image_name=ghcr.io/$GHCR_NAMESPACE/$GHCR_REPOSITORY:$TAG`, triggered on push to master) plus cosign `sha256-*.sig`/`.att` tags. 0 tags start with `v`; no bare `3.5.3`. Master VERSION is now 3.7.0 (bumped when v3.6.0-rc1 was cut by `post-release` job). Legacy `ghcr.io/argoproj/argocd` (1319 tags, `2.0.0-xxxx` ... `2.5.0-xxxx`) is the older equivalent. `ghcr.io/argoproj/argo-cd` (no suffix) -> DENIED.
- **Docker Hub `argoproj/argocd`**: exists but STALE (270 tags; last updated 2023-09-07, newest `v2.6.15`; `v3.5.3`, `v3.4.9`, `v2.14.0` -> 404). Not a release channel any more.
- **Install manifests reference the release image**: `manifests/install.yaml` at v3.5.3 has 8 occurrences of `image: quay.io/argoproj/argocd:v3.5.3` (e.g. L32094, L32266 ...), `image: ghcr.io/dexidp/dex:v2.45.1` (L32236), `image: public.ecr.aws/docker/library/redis:8.2.3-alpine` (L32485). `manifests/ha/install.yaml` additionally `image: public.ecr.aws/docker/library/haproxy:3.0.8-alpine` (x2) and 4 redis refs. Source of truth: `manifests/base/kustomization.yaml` `images: - name: quay.io/argoproj/argocd / newName: quay.io/argoproj/argocd / newTag: v3.5.3` (also `manifests/ha/base/kustomization.yaml`, `manifests/core-install/kustomization.yaml`, `manifests/base/commit-server/kustomization.yaml`). Variants in `manifests/`: `install.yaml`, `namespace-install.yaml`, `core-install.yaml`, `ha/install.yaml`, `ha/namespace-install.yaml`, `*-with-hydrator.yaml` (all HTTP 200 at v2.14.0, v3.0.0, v3.1.0, v3.2.0, v3.3.0, v3.5.0).
- **Dependent images in `manifests/install.yaml` (non-HA)** (dedup of all `image:` lines; verified by downloading the file at each tag):
  | tag | dex | redis |
  |---|---|---|
  | v3.6.0-rc1 | ghcr.io/dexidp/dex:v2.45.1 | public.ecr.aws/docker/library/redis:8.10.1-alpine |
  | v3.5.3 | v2.45.1 | 8.2.3-alpine |
  | v3.5.0 | v2.45.0 | 8.2.3-alpine |
  | v3.4.9 / v3.4.0 | v2.45.0 | 8.2.3-alpine |
  | v3.3.14 / v3.3.0 | v2.43.0 | 8.2.3-alpine |
  | v3.2.0 | v2.43.0 | 8.2.2-alpine |
  | v3.1.0 | v2.43.0 | 7.2.7-alpine (repo changed to `public.ecr.aws/docker/library/redis` here) |
  | v3.0.0 | v2.41.1 | `redis:7.2.7-alpine` (Docker Hub short name) |
  | v2.14.0 | v2.41.1 | `redis:7.0.15-alpine` |
  | v2.13.0 | v2.41.1 | `redis:7.0.15-alpine` |
  Change timeline across all v3 tags (first tag where pair changes): v3.0.20 redis 7.2.7->7.2.11; v3.1.0-rc1 dex v2.41.1->v2.43.0; v3.1.9 redis ->7.2.11; v3.2.0-rc2 redis 8.2.1; v3.2.0-rc4 8.2.2; v3.3.0-rc1 redis 8.2.3; v3.4.0-rc1 dex v2.45.0; v3.5.2 dex v2.45.1; v3.6.0-rc1 redis 8.10.1. Dependency versions DO change inside patch releases (3.0.20, 3.1.9, 3.5.2), so artifact diffs should be per tag, not per minor.
  Also image count: 7 argocd refs at v2.6-v2.10, 8 from v2.12 (commit-server added).
- Chart images: `values.yaml` (section 5). Images of the CLI: `argocd` binary is also in the image; CLI binaries are GitHub release assets (section 7).

## 7. Release assets (GitHub Releases, UP)

Built by GoReleaser (`.goreleaser.yaml`: `archives... name_template: {{ .ProjectName }}-{{ .Os }}-{{ .Arch }}`, `formats: [binary]`, `checksum.name_template: 'cli_checksums.txt'`, sha256) + workflow jobs `goreleaser-provenance` (`argocd-cli.intoto.jsonl`, L206), `generate-sbom` (`sbom.tar.gz`), `sbom-provenance` (`argocd-sbom.intoto.jsonl`, L306).
Verified with HEAD/GET following redirects (`https://github.com/argoproj/argo-cd/releases/download/<tag>/<asset>` -> 302 -> `release-assets.githubusercontent.com` -> 200) for tags v3.5.3, v3.4.9, v3.3.14, v3.0.0, v2.14.0, v3.5.0-rc3, v3.6.0-rc1: ALL of these exist (200) for each tag:
`argocd-linux-amd64`, `argocd-linux-arm64`, `argocd-linux-ppc64le`, `argocd-linux-s390x`, `argocd-darwin-amd64`, `argocd-darwin-arm64`, `argocd-windows-amd64.exe`, `cli_checksums.txt`, `sbom.tar.gz` (134,588 bytes at v3.5.3), `argocd-cli.intoto.jsonl`, `argocd-sbom.intoto.jsonl`. `argocd-linux-amd64` v3.5.3 = 249,693,841 bytes (range request).
- `cli_checksums.txt` format (v3.5.3): `b860f73f57cbddd993cd446f5236d797c1b1ac8554857b2683d2669f17e765b4  argocd-linux-amd64` (7 lines: darwin-amd64/arm64, linux-amd64/arm64/ppc64le/s390x, windows-amd64.exe). Good deterministic artifact for change-diffing (hash per binary).
- NOT present (404): `install.yaml` as a release asset (manifests are served from the tag via raw URL, section 6), `sbom.tar.gz.pem`, `sbom.tar.gz.sig` (listed in `docs/operator-manual/signed-release-assets.md` but 404 at v3.5.3, v3.0.0, v2.14.0), and the doc's `argocd-linux_amd64` underscore names (real names use hyphens).
- Release is `prerelease: auto` for `-rc` tags; rc tags also have all assets.

## 8. CRDs

`manifests/crds/` at v3.5.3: `application-crd.yaml`, `applicationset-crd.yaml`, `appproject-crd.yaml`, `kustomization.yaml` (`resources:` the three). All `group: argoproj.io`, `scope: Namespaced`, single version `v1alpha1` with `served: true`, `storage: true`:
`applications.argoproj.io` (kind Application), `applicationsets.argoproj.io` (ApplicationSet), `appprojects.argoproj.io` (AppProject). Same three CRDs embedded in `manifests/install.yaml` (names at L8, L7120, L30458 at v3.5.3). No new CRD kinds and no non-v1alpha1 versions at v2.10.0, v3.0.0, v3.5.3, v3.6.0-rc1 (verified by grep of the CRD files). Size note: `docs/operator-manual/upgrading/3.2-3.3.md` "ApplicationSet CRD exceeds the size limit for client-side apply" -> install with `--server-side`. Chart installs CRDs from `charts/argo-cd/templates/crds/` (not checked) with `crds.install/keep/annotations` values.

## 9. Security

- `SECURITY.md` (UP, root, v3.5.3): "Version: v1.5 (2023-03-06)". Sections: Preface, A word about security scanners, **Supported Versions** ("last 3 minor versions"), Dependency Upgrade Policy (Helm/Kustomize/git only upgraded by patch within a minor series), Reporting a Vulnerability (private GitHub Security Advisory draft `https://github.com/argoproj/argo-cd/security/advisories/new`; "We will publish security advisories using GitHub Security Advisories"). Also `SECURITY_CONTACTS`, `SECURITY-INSIGHTS.yml` (OpenSSF; `expiration-date: 2024-10-31`, stale), HELM `SECURITY.md` (chart-level; points to upstream policy).
- GitHub Security Advisories listing (`https://github.com/argoproj/argo-cd/security/advisories`, REST `/repos/.../security-advisories`) is BLOCKED -> UNVERIFIED. GHSA IDs can still be seen in git: security fixes are developed in private forks and land on release branches as commits titled `Merge pull request from GHSA-xxxx-xxxx-xxxx` (e.g. on `release-2.13` history: `c2647055 2024-06-06 Merge pull request from GHSA-3cqf-953p-h5cp`, `d69c61ae 2024-03-18 ... GHSA-6v85-wr92-q4p7`; remote branch `release-2.10-ghsa6v85` exists). Mapping GHSA -> fixed tag: `git tag --contains <merge commit>` (verified: those merges are contained in v2.13.0 and later tags on that line). The body carries the fix commit text only; no CVE/severity in the commit.
- Release notes do NOT reliably show security fixes: `.goreleaser.yaml` changelog is built from GitHub PR titles, so private-fork GHSA merges (no PR) do not appear, and the legacy script explicitly filters them (`grep -v "Merge pull request from GHSA"`). Public dependency CVE bumps do appear as normal commits, e.g. `fix: upgrade x/crypto to v0.35.0 to solve CVE-2025-22869 (#22048)`, `chore(deps): bump redoc/dompurify to v3.4.0 in /ui for fixing CVE-2026-41240 (#27751)`, `fix: CVE-2024-45296 ... (#20087)`; and upgrade guides have a `## Security Changes` H2 (3.3-3.4, 3.4-3.5; mostly empty).
- Container/dependency scan reports: `docs/snyk/` (section 2). Image/CLI signing + provenance: `docs/operator-manual/signed-release-assets.md` (cosign keyless, identity regexp `https://github.com/argoproj/argo-cd/.github/workflows/image-reuse.yaml@refs/tags/v`; slsa-verifier for CLI and SBOM).
- Dependency-of-record CVEs: advisories for bundled Helm/Kustomize/Dex/Redis come through those upgrade headings and dex/redis tag bumps (section 6).
- OSV/NVD/ArtifactHub security views: UNVERIFIED (blocked).

## 10. Historical validation (what held, what did not)

| Relationship | Checked on | Result |
|---|---|---|
| Tag regex `^v\d+\.\d+\.\d+(-rc\d+)?$` | all 596 ref names | Holds for 591 (458 stable + 133 rc); 5 exceptions: `stable`, `gitops-engine/v3.6.0-rc1`, `v0.4.0-alpha1`, `v2.1.2-hf1`, `v2.2.8-1` |
| Upgrade doc `upgrading/{prev}-{cur}.md` exists at tag `v{cur}.0` | v1.2 ... v3.5 (28 transitions) | HELD for 1.6-1.7 ... 3.4-3.5 except: `2.8-2.9.md` MISSING at v2.9.0 and v2.9.0-rc1 (added later, present at v2.9.5/v2.9.22), `2.13-2.14.md` missing at v2.14.0-rc1..rc7 (present from v2.14.0); docs for <=1.5 -> 1.6 transitions absent (404 at v1.2.0..v1.6.0). Present at `-rc1` for 3.0 ... 3.5 |
| Same doc keyed by "previous minor" arithmetic | 3.0 and 2.0 | FAILED for simple `minor-1`: `2.14-3.0.md` (major jump) and `1.8-2.0.md` (skips 1.9) |
| `tested-kubernetes-versions.md` exists and first row = current minor | v2.9.0, v2.10.0, v2.11.0, v2.12.0, v2.13.0, v2.14.0, v3.0.0, v3.1.0, v3.2.0, v3.3.0, v3.4.0, v3.5.0, v3.5.3, v3.6.0-rc1 | HELD (200, header identical) except empty first row at v2.12.0/v2.12.1; absent before v2.8.0 (404 at v2.7.0, v2.6.0); older rows drift between tags |
| `manifests/install.yaml` image == `quay.io/argoproj/argocd:{tag}` | all 112 v3.x tags + 233 v2.5-v2.14 tags (full download of install.yaml) | HELD for 340/345. FAILED for 5: `v2.14.0` (-> `v2.14.0-rc7`, documented in 2.13-2.14.md "nonexistent Argo CD image"), `v2.14.0-rc4` (-> rc3), `v3.0.10` (-> `v3.0.9`, VERSION file also 3.0.9), `v3.1.0-rc2` (-> rc1), `v3.4.0-rc4` (-> rc3). Therefore derive image tag from the git tag and treat install.yaml as a cross-check |
| CRDs at `manifests/crds/{application,appproject,applicationset}-crd.yaml` | v1.8.0, v2.0.0, v2.2.0, v2.4.0, v2.6.0, v2.10.0, v2.14.0, v3.0.0, v3.3.0, v3.5.0, v3.6.0-rc1 | HELD for application/appproject at all; `applicationset-crd.yaml` 404 at v1.8.0, v2.0.0, v2.2.0 (introduced by v2.4.0). `manifests/core-install.yaml` 404 at v1.8.0, v2.0.0 (200 from v2.2.0). All v1alpha1, 3 CRD names in install.yaml at v2.10.0, v3.0.0, v3.5.3, v3.6.0-rc1, v2.14.21 |
| Release assets pattern (`argocd-{os}-{arch}`, `cli_checksums.txt`, `sbom.tar.gz`, 2 intoto files) | v2.14.0, v3.0.0, v3.3.14, v3.4.9, v3.5.3, v3.5.0-rc3, v3.6.0-rc1 | HELD for all (200). `install.yaml` asset never present (404 all) |
| Chart tag `argo-cd-X.Y.Z` -> `Chart.yaml version == X.Y.Z`, `appVersion` == index entry | all 790 tags | HELD 790/790 (0 mismatches); appVersion non-decreasing across 7.0.0..10.9.5 |
| Chart appVersion == an existing upstream stable tag | all 936 index entries from 7.0.0 | HELD (0 orphan appVersions; no rc appVersions) |
| Chart ships every upstream patch | v2.11.0+ (144 stable tags) | FAILED: only 76 shipped (68 never appear as appVersion) |
| OCI chart tags list == subset of index/tags | 390 OCI vs 936 index | HELD (OCI starts at 5.43.0; 0 OCI-only versions) |
| `CHANGELOG.md` current | master, v3.6.0-rc1, v3.5.3, v3.0.0, v2.14.0, v2.5.0 | FAILED (frozen at v2.4.8, 2022-07-29) |
| Docker Hub as release channel | argoproj/argocd | FAILED (frozen at v2.6.15, 2023-09-07) |
| `ghcr.io/argoproj/argo-cd/argocd` as release channel | tags list | FAILED for releases (master snapshots only) |

## Suggested declarative definition hints

```yaml
product: argo-cd
upstream_repo: github.com/argoproj/argo-cd
tag:
  # stable and pre-release; ignore: stable, gitops-engine/*, v0.*, *-hf*, v2.2.8-1
  regex: '^v(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(?:-rc(?P<rc>\d+))?$'
  prerelease_if: '-rc\d+$'
  latest_stable: 'semver-max of non-rc tags'   # equals refs/tags/stable (lightweight tag moved by release.yaml)
  support_window: last 3 minor lines (SECURITY.md); EOL otherwise
  minor_line_prev: "highest MAJOR.MINOR line < current from the tag set; NOT minor-1 (3.0 -> 2.14, 2.0 -> 1.8)"
paths_at_tag:   # raw.githubusercontent.com/argoproj/argo-cd/{{.Tag}}/<path>
  upgrade_guide: docs/operator-manual/upgrading/{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md
  upgrade_overview: docs/operator-manual/upgrading/overview.md   # list of "- [vX.Y to vA.B](./X.Y-A.B.md)" (use to resolve PrevMajor.PrevMinor)
  tested_k8s: docs/operator-manual/tested-kubernetes-versions.md  # table, first data row == this minor
  install_manifests:
    - manifests/install.yaml
    - manifests/ha/install.yaml
    - manifests/namespace-install.yaml
    - manifests/ha/namespace-install.yaml
    - manifests/core-install.yaml
    - manifests/install-with-hydrator.yaml
  crds:
    - manifests/crds/application-crd.yaml
    - manifests/crds/applicationset-crd.yaml    # since v2.4
    - manifests/crds/appproject-crd.yaml
  version_file: VERSION           # "3.5.3" (no v); unreliable on 5 tags
  changelog_file: CHANGELOG.md    # DO NOT USE (stale since v2.4.8)
  security_policy: SECURITY.md
  release_branch: release-{{.Major}}.{{.Minor}}
  ci_k8s_matrix_source: .github/workflows/ci-build.yaml  # jobs.test-e2e.strategy.matrix.k3s[].version
release_assets:   # https://github.com/argoproj/argo-cd/releases/download/{{.Tag}}/<asset>
  - argocd-{linux,darwin}-{amd64,arm64}, argocd-linux-{ppc64le,s390x}, argocd-windows-amd64.exe
  - cli_checksums.txt              # "<sha256>  <name>" lines
  - sbom.tar.gz
  - argocd-cli.intoto.jsonl
  - argocd-sbom.intoto.jsonl
release_notes: GitHub Release body (UNVERIFIED reachability); groups: Breaking Changes/Features/Bug fixes/Documentation/Dependency updates/Other work; footer "Full Changelog: compare/{prev}...{tag}"
images:
  release: quay.io/argoproj/argocd:{{.Tag}}              # includes leading v; multi-arch amd64,arm64,s390x,ppc64le; cosign-signed
  snapshot_only_do_not_use_for_releases: ghcr.io/argoproj/argo-cd/argocd:{{.Version}}-<sha8>   # master builds
  deprecated: docker.io/argoproj/argocd (frozen at v2.6.15)
  dependent_in_install_yaml:
    - ghcr.io/dexidp/dex:vX.Y.Z
    - public.ecr.aws/docker/library/redis:X.Y.Z-alpine   # (redis:X.Y.Z-alpine before v3.1.0)
    - public.ecr.aws/docker/library/haproxy:X.Y.Z-alpine # ha/install.yaml only
  cross_check: "grep image: in manifests/install.yaml and compare quay.io/argoproj/argocd tag with {{.Tag}}; flag mismatch (v2.14.0, v3.0.10 known)"
helm_chart:
  repo: github.com/argoproj/argo-helm
  chart_path: charts/argo-cd
  tag_template: argo-cd-{{.ChartVersion}}               # e.g. argo-cd-10.9.5; list via git ls-remote --tags
  chart_yaml_at_tag: charts/argo-cd/Chart.yaml           # fields: version, appVersion ("v"+upstream tag), kubeVersion, annotations.artifacthub.io/changes (single entry)
  values: charts/argo-cd/values.yaml                     # global.image.repository/tag, dex.image, redis.image
  readme_changelog: charts/argo-cd/README.md  # "## Changelog" -> "### <chartVersion>" (breaking notes only)
  index_url_reachable: https://raw.githubusercontent.com/argoproj/argo-helm/gh-pages/index.yaml  # same data as argoproj.github.io/argo-helm/index.yaml
  oci: oci://ghcr.io/argoproj/argo-helm/argo-cd           # tags = plain semver (5.43.0+)
  release_asset: https://github.com/argoproj/argo-helm/releases/download/argo-cd-{{.ChartVersion}}/argo-cd-{{.ChartVersion}}.tgz (+ .prov)
  app_to_chart: "chart versions where appVersion == 'v'+{{.Version}}; may be empty (76/144 upstream stables since 2.11 ever shipped)"
  kube_version_constraint: ">=1.25.0-0"; helm >= 3.0.0 (README only)
compat_table:
  file: docs/operator-manual/tested-kubernetes-versions.md
  columns: ["Argo CD version", "Kubernetes versions"]
  row_key: "{{.Major}}.{{.Minor}}"
  value: "comma-separated vK.M, newest first"
  use_first_row_only: true
security:
  policy: SECURITY.md
  advisories: https://github.com/argoproj/argo-cd/security/advisories   # UNVERIFIED (blocked); in-git signal: commit subject "Merge pull request from GHSA-*" -> git tag --contains
  scan_reports: docs/snyk/{{.Tag}}/ (only latest patch of 3 supported minors + master)
```

### Key warnings for the prototype
1. Do not use `CHANGELOG.md`, Docker Hub, or ghcr `argo-cd/argocd` as release channels.
2. Latest = semver-max stable tag (== `stable` tag), not chronologically newest tag (v3.4.9 and v3.5.3 share a release date).
3. Previous-minor resolution must be derived from the tag set (or `overview.md`), not by decrementing the minor.
4. Chart appVersion mapping is many-to-one and not total; absence of a chart for an upstream tag is normal.
5. Anything hosted on api.github.com / quay.io / argoproj.github.io / readthedocs / artifacthub is unreachable from this sandbox (UNVERIFIED), though `gh-pages/index.yaml` via raw.githubusercontent.com is an equivalent substitute for the Helm HTTP index.
