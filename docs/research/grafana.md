# Grafana (OSS) release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here.
`RAW` = `https://raw.githubusercontent.com/grafana/grafana`. Product: **Grafana OSS**
(the open-source visualization/observability platform, `github.com/grafana/grafana`);
Grafana Enterprise (`grafana-enterprise` image, `grafana-enterprise_*` packages,
same-versioned from the same repository) is a different artifact set and is NOT onboarded.

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, `v`-prefixed, minor lines.** Tags `vX.Y.Z`; 739 tags: 551 stable,
   78 pre-releases (`-betaN` ×75, `-rcN` ×3), 110 junk (unprefixed `1.0.0`-style
   ancient tags, `dupa`, `list`, …). Lines `vX.Y` maintained in parallel for
   9 months (last minor of a major: 15 months) per the published support policy.
   Latest stable today: **v13.2.3** (2026-09-29). Cadence: minor every other
   month, patching at least monthly, major yearly in April/May.
2. **Security-only releases are extra tags `vX.Y.Z+security-N`** (Docker
   `X.Y.Z-security-N`). A `+security-01` release = "X.Y.Z plus the security fix";
   the fix is included in all future releases of that line but is NOT repeated in
   the next regular patch's changelog section. They are semver build metadata, not
   separate versions: excluded from the canonical list (recorded gap, section 8).
3. **v13.0.0 was removed from distribution** (Git Sync migration bug that could
   lose dashboards; upgrade guide tells affected users to restore from backup and
   go to v13.0.1): no GitHub release, no plain Docker tag, no dl.grafana.com
   package [404s]. The git tag and the changelog section still exist.
4. **Release notes = `CHANGELOG.md` in the repository, read on `main`.** One
   `# X.Y.Z (date)` H1 per release, newest first, sub-grouped under `###` H3s:
   `Security`, `Features and enhancements`, `Bug fixes`, `Breaking changes`,
   `Plugin development fixes & changes`. Items are `- **Area:** Description
   [#PR](link), [@author]`; Enterprise-only entries end with `(Enterprise)`.
   The section for a release is merged to main *after* the tag is cut
   (at tag v13.0.0/v13.1.0/v13.2.0 the release's own section is absent; patch
   tags like v13.2.3 usually have it) — so the file must be read at `main`, not
   at the tag. Main covers 11.0.0 → today; older majors live in
   `.changelog-archive/CHANGELOG.NN.md` (per-major files). The GitHub Release
   body (exists since ~v6.5.0) is the same section plus download-page links.
5. **Upgrade guides are per-minor docs written on `main`:**
   `docs/sources/upgrade-guide/upgrade-vX.Y/index.md` (exists for every minor
   8.0 → 13.2), `## Technical notes` with one `###` heading per behaviour change
   and `#### You are affected if` / `#### Migration` / `#### Action required`
   sub-sections. Also written after the branch is cut (404 at the X.Y.0 tag),
   so read at `main` too. **What's new** (`docs/sources/whatsnew/whats-new-in-vX-Y.md`)
   is a Hugo page index since v12: the item content lives outside the repository
   (rendered by the `{{< docs/whats-new >}}` shortcode) — not ingestible from git.
   **Breaking changes** pages exist only for v10.0, v10.3, v11.0
   (`docs/sources/breaking-changes/breaking-changes-vX-Y.md`); from v12 on,
   breaking changes live in What's new (outside the repo), so the in-repo
   upgrade guide + changelog `### Breaking changes` carry them.
6. **Security**: GitHub repository security advisories (GHSA + CVE;
   api.github.com anonymous worked during discovery, then hit the 60/h limit
   during research). Grafana also publishes grafana.com/security/security-advisories
   [200] (HTML, not machine-readable).
7. **Helm chart lives in a MONOREPO that itself moved.** `charts/grafana` was in
   `github.com/grafana/helm-charts` (index `grafana.github.io/helm-charts`,
   chart versions `grafana-X.Y.Z` tags 5.6.0 → 10.5.15, last entry marked
   `deprecated: true`, appVersion 12.3.1); after **January 30, 2026** it moved to
   `github.com/grafana-community/helm-charts` (index
   `grafana-community.github.io/helm-charts` [200], full history re-published:
   815 chart versions, 5.0.4 → 13.2.7). **Chart version is independent of the
   app version** (chart 10.5.15 ships app 12.3.1; chart 13.2.7 ships app
   13.2.3; app 9.0.0/10.0.0 never shipped as an appVersion) → `lookup` by
   `appVersion == {{.Version}}` (no `v`; argo-cd precedent). Not every app
   release has a chart (no chart appVersion for 13.0.10, 13.1.7, 12.4.12, …),
   so the artifact is optional.
8. **Container image is `docker.io/grafana/grafana`, tag = version (no `v`),
   OSS across all of history.** Grafana's own docker docs (in-repo): "Grafana
   Open Source: `grafana/grafana`; Grafana Enterprise:
   `grafana/grafana-enterprise`", and "Starting with 12.4.0 the
   `grafana/grafana-oss` repository will no longer be updated … these two
   repositories have the same Grafana OSS docker images". The parallel
   `grafana/grafana-oss` repository published the same OSS images from 10.0.0
   to 13.0.2 and then stopped. Variants per release: `-ubuntu`, `-slim`,
   `-distroless` (13.x) suffix tags; `-security-N` for security releases;
   build-id tags `X.Y.Z-<CI id>`; floating `X.Y`/`latest`.
9. **Packages and tarballs on `dl.grafana.com/oss/release/`** [200]:
   `grafana_X.Y.Z_amd64.deb`, `grafana-X.Y.Z-1.x86_64.rpm`,
   `grafana-X.Y.Z.linux-amd64.tar.gz` (+ arm64/arm), verified 5.4.3 → 13.2.3;
   5.0.0 and 13.0.0 are 404. GitHub releases also attach `grafana_X.Y.Z_<CI
   build id>_linux_amd64.deb`/`.rpm` assets, but the CI build id in the asset
   name is not predictable from the version — the predictable dl.grafana.com
   paths are the channel.
10. **CRDs (Grafana's Kubernetes-style app APIs, `*.grafana.app` groups)**:
    `apps/alerting/notifications/definitions/*.yaml` (4 CRDs + a kustomize
    manifest, from v12.3.0) and `apps/alerting/rules/definitions/*.yaml`
    (3 CRDs from v12.4.0, 4 from v13.1.0). Experimental feature, not shipped by
    the Helm chart. No other in-repo CRDs.

## 1. Canonical versions

`git ls-remote` [200]: 739 tags; strict pattern (as discovery proposed):
`^v(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$` → 551 stable + 78
pre-releases; excludes the 110 junk tags and the `+security-NN` security tags.
Lineage `minor`. Support policy (docs `upgrade-guide/when-to-upgrade`): each
minor supported 9 months, last minor of a major 15 months — 11.6, 12.4, 13.x
current; a markdown table of per-minor support end dates exists there but has
no slot in the model (recorded gap).

## 2. Release notes (CHANGELOG.md on main) [200]

`RAW/main/CHANGELOG.md` — newest first, one H1 `# X.Y.Z (date)` per release,
wrapped in `<!-- X.Y.Z START/END -->` comments; H3 groups: `### Security`,
`### Features and enhancements`, `### Bug fixes`, `### Breaking changes`,
`### Plugin development fixes & changes`; one bullet per change:
`- **Area:** Description [#123](PR link), [@user]` (Enterprise-only entries
end with `(Enterprise)`). Coverage on main today: 10.3.9+/10.4.6+ … 13.2.3
(237 sections); everything older is archived per major in
`.changelog-archive/CHANGELOG.NN.md` (1 … 10).

- At the release's own tag the section is usually missing for X.Y.0 (the
  changelog PR lands on main after the branch cut): v13.0.0, v13.1.0, v13.2.0
  tags lack their own section; v13.2.3's tag has it. Therefore read at `main`
  with `availability: >= 11.0.0` (stable window; main will archive 11.x
  eventually — drift-report material, not a broken relationship).
- GitHub Release body = the same section + `[Download page]`/`[What's new
  highlights]` link lines; releases exist since ~v6.5.0-beta1 (500 listed on
  5 API pages); v13.0.0 has none (404, removed from distribution).
- api.github.com is rate-limited (60/h anonymous): exhausted by discovery +
  research in this session, so the GitHub body is the fallback channel, the
  raw file the primary (the opposite of cilium, where the tag-time changelog
  is primary and the API body the fallback).

## 3. Upgrade guides (per-minor, on main) [200]

- `RAW/main/docs/sources/upgrade-guide/upgrade-vX.Y/index.md`, one per minor
  v8.0 … v13.2. Frontmatter (title/weight) + shared-include shortcodes
  (`{{< docs/shared lookup="upgrade/intro_2.md" …>}}`) + `## Technical notes`
  + one `### <change>` heading per technical/behaviour change, each typically
  with `#### You are affected if` and `#### Migration`/`#### Action required`
  sub-sections, plus `{{< admonition …>}}` warning blocks (e.g. the v13.0.0
  removal notice). 404 at the X.Y.0 tag (docs land after the branch cut),
  present at later patch tags of the line — read at `main`, `releaseKinds:
  [minor]`.
- `docs/sources/breaking-changes/breaking-changes-vX-Y.md`: only v10.0, v10.3,
  v11.0 ("as of v12.0 we no longer publish a dedicated breaking changes page").
  Per-minor availability windows pin each file to exactly its X.Y.0.
- What's new (`whats-new-in-vX-Y.md`, v7.0 … v13.2): full inline content up to
  v11.x; from v12 on the file is a Hugo `posts:` index + prose and the item
  content is rendered by shortcode from outside the repository — not a git
  channel; not ingested (gap).

## 4. Security

- GitHub advisories (`github-advisories` adapter, repository grafana/grafana):
  the machine-readable feed; worked anonymously during discovery, rate-limited
  afterwards.
- grafana.com/security/security-advisories [200] (HTML index of write-ups);
  declared as the documented channel, not parsed.
- Security-only releases `vX.Y.Z+security-N` (tags + `X.Y.Z-security-N` Docker
  tags + changelog sections, e.g. `# 12.4.3+security-02 (2026-05-12)` with
  `### Bug fixes` → `- **Security:** Fix CVE-…`): out-of-band releases of an
  already-shipped patch. Not separate canonical versions (the fix ships in
  every later release of the line); the *next regular patch's changelog does
  not repeat them* — upgrading 12.4.3 → 12.4.4 crosses +security-02 silently
  in the notes (gap).

## 5. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| Helm chart `grafana` | index `https://grafana-community.github.io/helm-charts` (815 versions, 5.0.4 → 13.2.7); old index `https://grafana.github.io/helm-charts` (≤ 10.5.15); git `github.com/grafana-community/helm-charts` tags `grafana-X.Y.Z` (647); OCI `ghcr.io/grafana-community/helm-charts/grafana` (chart 10.0.0–10.5.15 only) | independent; chart for an app release found by `appVersion == {{.Version}}` (no `v`) | [200] index + old index + ghcr manifest 10.5.15; 13.0.0→charts 12.0.0–12.1.0, 13.2.3→13.2.7; NO chart for 13.0.10, 13.1.7, 12.4.12 |
| chart values / metadata | `charts/grafana/{values,Chart}.yaml` at tag `grafana-{{.ArtifactVersion}}` in the community repo (full history) | per chart version | [200] |
| image `grafana` | `docker.io/grafana/grafana:X.Y.Z` (+ `-ubuntu`, `-slim`, `-distroless` variants); `mirror.gcr.io/grafana/grafana` pull-through | `{{.Version}}` (no `v`), OSS across history | [200] plain tags 6.7.0 → 13.2.3 via registry-1 + mirror; **13.0.0 absent** (only build-id tags); `-security-01` tags present |
| image `grafana-oss` (parallel repo) | `docker.io/grafana/grafana-oss:X.Y.Z` | same images | [200] 10.0.0 … 13.0.2, then stopped (12.4.0 announcement) — noted, not an artifact |
| deb | `https://dl.grafana.com/oss/release/grafana_X.Y.Z_amd64.deb` | `{{.Version}}` | [200] 5.4.3 … 13.2.3; 5.0.0, 13.0.0 404 |
| rpm | `https://dl.grafana.com/oss/release/grafana-X.Y.Z-1.x86_64.rpm` | `{{.Version}}` | [200] 13.2.3 |
| linux tarball | `https://dl.grafana.com/oss/release/grafana-X.Y.Z.linux-amd64.tar.gz` | `{{.Version}}` | [200] 6.7.0, 13.2.3 |
| CRDs (alerting notifications, `notifications.alerting.grafana.app`) | `apps/alerting/notifications/definitions/*.yaml` at the tag | at the tag | [200] ≥ v12.3.0 |
| CRDs (alerting rules, `rules.alerting.grafana.app`) | `apps/alerting/rules/definitions/*.yaml` at the tag | at the tag | [200] ≥ v12.4.0 |
| GitHub release assets (deb/rpm/spdx) | `github.com/grafana/grafana/releases/download/…` | asset name embeds a CI build id → not templatable | [200] listing only |

## 6. Compatibility

- Grafana publishes **no platform compatibility matrix** (it is not a
  Kubernetes component): no k8s/OS/database matrix exists to ingest. The chart's
  `kubeVersion` (`^1.25.0-0` at chart 13.2.7) is captured via chart-metadata.
  Plugin/React compatibility statements (React 19 in v13) are prose inside the
  upgrade guide's technical notes → note items, not constraints.
- Support windows (9/15 months) are a markdown table in
  `upgrade-guide/when-to-upgrade/index.md` [200] — per-minor end dates; the
  model has no support-window construct (gap).

## 7. Hosts (from this sandbox, 2026-10-01)

Reachable [200]: github.com (git), raw.githubusercontent.com (main + tags),
codeload (via cilium onboarding earlier today; grafana source tarball not
needed — dl.grafana.com serves the release tarball), grafana-community.github.io
(index.yaml), grafana.github.io (old index.yaml), ghcr.io (anonymous token;
grafana chart OCI 10.0.0–10.5.15), auth.docker.io + registry-1.docker.io
(anonymous token pull; tag lists + manifests), mirror.gcr.io (grafana/grafana
manifests), dl.grafana.com (oss/release), grafana.com (docs, download,
security-advisories pages).
Rate-limited: **api.github.com** — anonymous 60/h consumed by discovery's
validation pass and this research session (release bodies for the 6 discovery
releases were fetched successfully and are in the fetch cache; later API calls
403). Unreachable: none of the declared canonical channels.
