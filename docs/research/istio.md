# Istio release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git op in this sandbox. UNVERIFIED = could not be checked here (proxy-blocked or inferred).
`RAW` = `https://raw.githubusercontent.com`. `IO` = repo `istio/istio.io`. `ISTIO` = repo `istio/istio`. `RB` = repo `istio/release-builder`.

## 0. Executive summary (things a declarative definition MUST know)

1. Tags are bare semver (no `v`), annotated, created by "Istio Release Robot". Latest stable today = **1.31.1** (tag 2026-09-21T09:40:09-05:00). Supported minors: 1.31, 1.30, 1.29 (1.29 EOL announced for 2026-10-12).
2. **Structured release notes DO exist** in `ISTIO/releasenotes/notes/*.yaml` (schema `release-notes/v2`: `kind`, `area`, `issue`, `releaseNotes[]`, `upgradeNotes[{title,content}]`, `securityNotes[]`, `docs`). The directory ACCUMULATES (1836 files at 1.31.1); notes for a release = files added between two tags. There is NO "action required" field; breaking items = `upgradeNotes`.
3. Human-readable notes live in `IO` (Hugo). Patch notes for the LATEST minor land on the `release-<M>.<m>` branch of istio.io; patch notes for older minors land on `master`, cherry-picked late (1.29.8 and 1.30.5 notes reached master on 2026-09-30, 9 days after tags). 1.31.1 notes exist only on `release-1.31` as of today.
4. **Infrastructure migration (GCP -> AWS/Cloudflare R2) is in progress and changes artifact locations at 1.31**: stable 1.31.0+ is NOT on `gcr.io/istio-release` nor in the GCS Helm index; it is on Docker Hub, `ghcr.io/istio/release/charts` (OCI), `blob.istio.io` (blocked from this sandbox) and GitHub release assets. GCS/gcr stay populated for <=1.30.x (1.30.5 present) until deletion in Dec 2026.
5. Compatibility data is structured YAML: `IO/data/compatibility/supportStatus.yml` (master). It is mutable after release (1.31 gained K8s 1.37 on 2026-09-30) and has a few inconsistencies vs announcements (see 4).

---

## 1. Canonical versions

Source: `git ls-remote --tags https://github.com/istio/istio` [200], 465 refs.

- Pattern: `^\d+\.\d+\.\d+$` stable (e.g. `1.31.1`), pre-release `^\d+\.\d+\.\d+-(alpha|beta|rc)\.\d+$` (e.g. `1.31.0-rc.4`). Zero tags in `ISTIO` start with `v` (verified: `grep -c '^v'` = 0).
- Junk/legacy tags to exclude: `1.0.0-snapshot.N`, `1.1.0-snapshot.N`, `1.1.0.snapshot.N`, `1.15-beta.0`.
- Pre-release numbering is NOT contiguous in git: 1.31.0 has `alpha.0, alpha.2, beta.0, beta.1, beta.2, rc.0, rc.2, rc.3, rc.4` (no `alpha.1`, no `rc.1`, though `1.31.0-alpha.1` exists in `RB`/Docker Hub/ghcr).
- Go-module `v`-prefixed tags exist only in `istio/api` and `istio/client-go` (`v1.31.1`, 153/268 `v*` tags); `istio/ztunnel`, `istio/release-builder`, `istio/tools` use bare tags. Evidence: `RB pkg/publish/github.go:123-135` (`GithubTag` adds `v` only if `goVersionEnabled`).
- `ISTIO/VERSION` file at tag contains only the minor (`1.31`) — do not use it for the patch.
- Stable tags, last three minors (from ls-remote):
  - 1.31: `1.31.0 1.31.1` (pre: alpha.0, alpha.2, beta.0-2, rc.0, rc.2-4)
  - 1.30: `1.30.0 1.30.1 1.30.2 1.30.3 1.30.4 1.30.5`
  - 1.29: `1.29.0 ... 1.29.8` (no 1.29.9; an istio.io commit message "cherry-pick release notes 1.29.9" is a typo for 1.29.8)
  - 1.28: `1.28.0 ... 1.28.10` (EOL 2026-07-01)
- Tag dates (annotated tagger date, `git for-each-ref`): 1.31.1 2026-09-21T09:40-05:00; 1.31.0 2026-08-31T08:15-05:00; 1.31.0-rc.4 2026-08-27; 1.30.5 2026-09-21T14:17-05:00; 1.30.0 2026-05-18T11:06-07:00; 1.29.8 2026-09-21T13:41-05:00; 1.29.0 2026-02-16; 1.28.10 2026-07-01; 1.28.0 2025-11-05; 1.27.0 2025-08-11; 1.26.0 2025-05-08. Tag commit date can precede tag date by days (1.31.1 commit 2026-09-16).
- **Latest stable as of 2026-10-01: 1.31.1.** Corroboration: Docker Hub `istio/pilot:1.31.1` exists; ghcr chart `1.31.1` exists; GitHub release asset `istio-1.31.1-linux-amd64.tar.gz` [200]; `RAW/istio/istio.io/release-1.31/data/args.yml` [200] has `full_version: "1.31.1"`, `preliminary: false`. (`RAW/istio/istio.io/master/data/args.yml` [200] is the PRELIMINARY site: `full_version: "1.32.0"`, `preliminary: true` — do not use master args.yml for "latest stable".)
- istio.io `release-1.NN` branches' `args.yml` full_version is stale for non-latest minors (release-1.30 says 1.30.4, release-1.29 says 1.29.2) — only trust the newest release branch.
- Git release branches: `ISTIO` `release-1.31` ... (`git ls-remote --heads`); also odd branches `release-1.27.4-patch`, `release-1.30.4`.
- GitHub Release objects: UNVERIFIED (api.github.com blocked; GitHub MCP not authorized for this repo). `RB pkg/publish/github.go:77-83` creates each release as `Draft: true, Prerelease: true` and humans later publish, so the GitHub "prerelease" flag is NOT a reliable stable/pre-release discriminator. Release body (github.go:71-73): `[Artifacts](https://blob.istio.io/istio-release/releases/<ver>/)\n[Release Notes](https://istio.io/news/releases/<M.m>.x/announcing-<ver>/)` (note: for `X.Y.0` that link points to `announcing-X.Y.0`, which only resolves through a Hugo alias; the real directory is `announcing-X.Y`). Older branches used `http://gcsweb.istio.io/gcs/istio-release/releases/%s/` (release-1.26..1.30 `pkg/publish/github.go:71`).

## 2. Release notes

### 2a. istio.io layout (verified on local sparse clone of master + raw URLs)

Root: `content/en/news/releases/<M>.<m>.x/` (has `_index.md` grid page). Verified for 1.20 ... 1.31 (every minor has the same 3 items):

| Item | Path template (relative to repo root) | Notes |
|---|---|---|
| Minor announcement | `content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/_index.md` | NOTE `_index.md` (not index.md); front matter below |
| Change notes | `.../announcing-{{.Major}}.{{.Minor}}/change-notes/index.md` | heading structure below |
| Upgrade notes | `.../announcing-{{.Major}}.{{.Minor}}/upgrade-notes/index.md` | |
| Patch notes | `content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Version}}/index.md` | e.g. `1.30.x/announcing-1.30.5/index.md` |
| EOL notices | `content/en/news/support/announcing-{{.Major}}.{{.Minor}}-eol/index.md` and `...-eol-final/index.md` | |

Verified [200] raw: `RAW/istio/istio.io/master/content/en/news/releases/1.31.x/announcing-1.31/_index.md`, `.../1.31.x/announcing-1.31/change-notes/index.md`, `.../1.31.x/announcing-1.31/upgrade-notes/index.md`, `.../1.30.x/announcing-1.30.5/index.md`; `RAW/istio/istio.io/release-1.31/content/en/news/releases/1.31.x/announcing-1.31.1/index.md` [200] (the same path on `master` = 404 today).
Historic layout: minors 1.4-1.31 = `_index.md + change-notes + upgrade-notes`; 1.1-1.3 also have `helm-changes/`; 1.0 = single `index.md`.
Coverage: of 92 stable tags >= 1.20, 90 have a notes dir on master; missing = `1.24.1` (never published) and `1.31.1` (only on release-1.31 branch).
Branch behaviour: `release-1.30` has `announcing-1.30.4` but not `1.30.5` (404); `master` has 1.30.5 and 1.29.8 (added 2026-09-30, commit b30d6bd72 "Manually cherry-pick release notes 1.29.9 and 1.30.5 to master (#17639)"). => poll BOTH `master` and the newest `release-X.Y` branch; expect lag of days.

Front matter (minor announcement, 1.31):
```yaml
title: Announcing Istio 1.31.0
linktitle: 1.31.0
subtitle: Major Release
description: Istio 1.31 Release Announcement.
publishdate: 2026-08-31
release: 1.31.0
aliases: [/news/announcing-1.31, /news/announcing-1.31.0]
```
Body: `{{< relnote >}}`, a `{{< tip >}}Istio 1.31.0 is officially supported on Kubernetes versions 1.32 to 1.36.{{< /tip >}}`, then `## What's new?` etc.
Front matter (change-notes): `title: Istio 1.31.0 Change Notes`, `subtitle: Minor Release`, `release: 1.31.0`, `publishdate: 2026-08-31`, `weight: 10`.
Front matter (upgrade-notes): `title: Upgrade Notes`, `description: Important changes to consider when upgrading to Istio 1.31.0.`, `weight: 20` (no `release`/`publishdate`).
Front matter (patch): `subtitle: Patch Release`, `release: 1.31.1`, `publishdate: 2026-09-21`; body "This release note describes what's different between Istio 1.31.0 and 1.31.1." then `{{< relnote >}}`, optional `## Security Update`, then `## Changes`.
publishdate matches tag date for: 1.31.0 (2026-08-31), 1.30.0 (2026-05-18), 1.29.0 (2026-02-16), 1.28.0 (2025-11-05), 1.27.0 (2025-08-11), 1.26.0 (2025-05-08), 1.31.1/1.30.5/1.29.8 (2026-09-21), 1.28.10 (2026-07-01) -> all held.

### 2b. change-notes heading structure and verb prefixes

H2 headings (1.31, order): `## Traffic Management`, `## Security`, `## Telemetry`, `## Extensibility`, `## Installation`, `## istioctl`. 1.30 adds `## Documentation changes`; 1.29 has no `## Extensibility`. Same set in 1.27, 1.28. No H3 inside change-notes.
Each entry is a top-level bullet starting with a bold verb (counts per release):

| Release | Fixed | Added | Promoted | Removed | Updated | Improved | Deprecated | Other |
|---|---|---|---|---|---|---|---|---|
| 1.31 | 78 | 32 | 0 | 1 | 1 | 3 | 0 | Upgraded 1, Optimized 1, Enabled 1 |
| 1.30 | 52 | 40 | 0 | 0 | 2 | 3 | 0 | Enabled 1 |
| 1.29 | 18 | 31 | 4 | 1 | 2 | 1 | 1 | |
| 1.28 | 10 | 26 | 1 | 3 | 4 | 1 | 0 | Upgraded 1, Enabled 1 |
| 1.27 | 24 | 41 | 1 | 2 | 1 | 0 | 0 | |

Excerpts (`.../1.29.x/announcing-1.29/change-notes/index.md`):
```
- **Promoted** the `cni.ambient.dnsCapture` value to default to `true`.
  This enables DNS proxying ... This can be disabled explicitly or with `compatibilityVersion=1.24`.
- **Removed** obsolete manifests from the `base` Helm chart. See Upgrade Notes for more information.
- **Deprecated** the `sidecar.istio.io/statsCompression` annotation, ... ([Issue #48051](https://github.com/istio/istio/issues/48051))
```
`.../1.31.x/.../change-notes/index.md:559`: `- **Removed** the PILOT_SPAWN_UPSTREAM_SPAN_FOR_GATEWAY feature flag. ...`
Breaking/action-required flagging: NO dedicated marker/shortcode in change-notes (0 `{{< warning >}}` usages). Signals are (a) verbs `**Removed**`, `**Deprecated**`, `**Promoted**` (default flips), (b) occasional prose "See Upgrade Notes", (c) the separate upgrade-notes page (H2 per item). Issue links appear as `([Issue #NNNN](https://github.com/istio/istio/issues/NNNN))`.
Patch notes: `## Changes` (47 of the 50 patch notes for 1.24-1.30), `## Security Update`/`## Security update`/`## Security Updates` (case varies; 24 occurrences), optional H3 `### Envoy CVEs`, `### Istio CVEs`, `### Istio Security Fixes`, `### Other Istio Security Fixes`. Same bold verbs (`**Fixed**`, `**Updated**`...). A security-reporter credit appears as `**Credit**: ...`.

### 2c. STRUCTURED notes in istio/istio — `releasenotes/notes/*.yaml` (EXISTS, important)

- Present at tags: `RAW/istio/istio/<tag>/releasenotes/template.yaml` [200] for 1.26.0 1.26.8 1.27.0 1.27.9 1.28.0 1.28.10 1.29.0 1.29.8 1.30.0 1.30.5 1.31.0 1.31.0-rc.4 1.31.1. File counts in `releasenotes/notes/`: 1.28.10=1596, 1.29.0=1600, 1.30.0=1693, 1.30.5=1772, 1.31.0=1811, 1.31.1=1836.
- Docs: `ISTIO/releasenotes/README.md` and `ISTIO/releasenotes/template.yaml`. Consumed by `istio/tools` `cmd/gen-release-notes` (`--oldBranch <tag> --newBranch <branch/tag> --notes <dir>`; README at `RAW/istio/tools/master/cmd/gen-release-notes/README.md` [200]).
- The directory accumulates; per-release set = files ADDED between two refs. Verified: `git diff-tree --diff-filter=A 1.30.0 1.31.0 -- releasenotes/notes` = **118 files = 118 bullets** in 1.31 change-notes (exact match). 1.29.0->1.30.0: 95 files vs 98 bullets; 1.28.0->1.29.0: 67 vs 58 (approximate; some files hold several bullets, some notes predate the baseline tag). 1.31.0->1.31.1: 25 files added (patch note has ~25 bullets: fixes + the GHSA item).
- Schema (`apiVersion: release-notes/v2`):
  - `kind`: `feature | bug-fix | security-fix | test` (stats over 1830 files: feature 929, bug-fix 876, security-fix 19, rare typos `promotion`, `enhancement`, `bug`)
  - `area`: `traffic-management | security | telemetry | extensibility | installation | istioctl | documentation` (legacy/typo values exist: `networking`, `environments`, `pilot`, `cni`...)
  - `issue`: list of ints (istio/istio issue numbers) or URLs (may be `[]`; legacy keys `issues`, `Issue`)
  - `releaseNotes`: list of markdown strings; the bold verb (`**Fixed** ...`) is INSIDE the text, not a field
  - `upgradeNotes`: list of `{title, content}` (85/1830 files; legacy typos `upgradeNodes`, `upgradeNote`)
  - `securityNotes`: list of markdown strings (7 files)
  - `docs`: list of `'[usage] https://istio.io/...'`
  - There is NO "action required"/"breaking" field; `upgradeNotes` is the closest analog.
- Example security fix (`releasenotes/notes/backendtlspolicy-sidecar-failclosed.yaml` @1.31.1):
```yaml
apiVersion: release-notes/v2
kind: security-fix
area: security
issue: []
releaseNotes:
- |
  **Fixed** [GHSA-qm8v-g4f9-qhjx](https://github.com/istio/istio/security/advisories/GHSA-qm8v-g4f9-qhjx): a fail-open in `BackendTLSPolicy` ...
```
- Example upgradeNotes (`releasenotes/notes/sni-dnat-default.yaml`): `kind: feature`, `area: networking`, `issue: [27749]`, `releaseNotes: ["**Updated** the default installation of gateways ..."]`, `upgradeNotes: [{title: "`AUTO_PASSTHROUGH` Gateway mode", content: |...}]`.
- Example bug-fix (`61020.yaml`): `kind: bug-fix`, `area: traffic-management`, `issue: [61020]`, `releaseNotes: ["**Fixed** an issue in ambient mode where ..."]`.
- Mapping to istio.io upgrade-notes: YAML `upgradeNotes[].title` ~= H2 headings in `upgrade-notes/index.md` (1.31: 5 YAML titles vs 6 H2s; 1.30: 5 vs 6; 1.29: 5 vs 7). Release managers ADD hand-written H2s (e.g. "Deprecation of GCP infrastructure and hosting", "Gateway API CRDs must be upgraded to `v1.5.x`", "Base Helm chart removals"). So YAML is a strict subset; istio.io page is authoritative for breaking changes.
- Fallibility: the YAML is only as good as PR authors; a handful have typos in key names (e.g. `upgradeNodes` in `spawn-upstream-span-for-gateway.yaml`), so parse leniently.

## 3. Upgrade / migration docs

- Per-release upgrade notes: `content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/upgrade-notes/index.md` (verified 1.4-1.31). Intro boilerplate: "When you upgrade from Istio 1.30.0 to Istio 1.31.0, you need to consider the changes on this page. These notes detail the changes which purposefully break backwards compatibility with Istio 1.30.x." One `##` per item (1.28: Enabling `seccompProfile`..., `BackendTLSPolicy` alpha removal, ...).
- General upgrade guides in `IO`: `content/en/docs/setup/upgrade/{_index.md, canary/, in-place/, helm/}` [present in sparse checkout].
- Compatibility-version mechanism referenced in notes (`compatibilityVersion=1.24`); chart files `manifests/charts/base/files/profile-compatibility-version-1.2x.yaml` exist at 1.31.1.
- Deprecation announcements also appear as H2 in the minor announcement (e.g. 1.23 "Deprecating the in-cluster Operator"; 1.31 "Deprecation of GCP infrastructure and hosting") and as blog posts (`content/en/blog/2026/retirement-of-gcp/index.md`).

## 4. Compatibility

- Structured source (BEST): `RAW/istio/istio.io/master/data/compatibility/supportStatus.yml` [200]. Rendered by shortcode `layouts/shortcodes/support_status_table.html` into `content/en/docs/releases/supported-releases/index.md` [200] (`{{< support_status_table >}}`). Table columns: **Version | Currently Supported | Release Date | End of Life | Supported Kubernetes Versions | Tested, but not supported**. YAML keys: `version, supported, releaseDate, eolDate, k8sVersions[], testedK8sVersions[]`.
```yaml
- version: "1.31"
  supported: "Yes"
  releaseDate: "Aug 27, 2026"
  eolDate: "~Feb 2027 (Expected)"
  k8sVersions: ["1.32", "1.33", "1.34", "1.35", "1.36", "1.37"]
  testedK8sVersions: ["1.27", "1.28", "1.29", "1.30", "1.31"]
- version: "1.29"
  supported: "Yes"
  releaseDate: "Feb 16, 2026"
  eolDate: "12 Oct 2026 (Expected)"
  k8sVersions: ["1.31", "1.32", "1.33", "1.34", "1.35"]
  testedK8sVersions: ["1.26", "1.27", "1.28", "1.29", "1.30"]
```
  Also a `version: "master"` row (supported "No, development only"). Date formats are free text ("Sept 30, 2025", "~Feb 2027 (Expected)").
- Same page has other tables: "Supported releases without known CVEs" (`| Minor Releases | Patched versions with no known CVEs |`, rows `1.31.x | 1.31.0+`, `1.29.x | 1.29.2+`) and "Supported Envoy Versions" (`| Istio version | Envoy release branch |`, `1.31.x | release/v1.39`, `1.30.x | release/v1.38`). Support policy text: minor supported until 6 weeks after the N+2 minor release.
- Announcement tip as 2nd source (`{{< tip >}}Istio 1.31.0 is officially supported on Kubernetes versions 1.32 to 1.36.{{< /tip >}}`): matches table for 1.23, 1.24, 1.25, 1.27, 1.29, 1.30 (verified) but MISMATCHES for: 1.26 (table 1.29-1.33, tip 1.29-1.32 "expect 1.33 to work"), 1.28 (table 1.30-1.34, tip 1.29-1.34), 1.31 (table 1.32-1.37, tip 1.32-1.36). Cause: table is edited post-release (commit 7838df586 2026-09-30 "update support k8s versions (#17638)" added 1.37 to 1.31 and master). => compatibility is time-varying; treat supportStatus.yml (latest commit) as authoritative, tip as release-time snapshot.
- Release-date discrepancies in table vs tags: 1.31 table "Aug 27, 2026" vs tag/announcement 2026-08-31 (and EOL notice says "released on the 31st of August"); 1.30 table "May 14, 2026" vs tag/announcement 2026-05-18. Prefer `publishdate`/tag date.
- Helm `kubeVersion`: only the sample umbrella chart has one — `manifests/sample-charts/ambient/Chart.yaml`: `kubeVersion: ">= 1.23.0-0"` (present at 1.28.0, 1.29.0, 1.30.0, 1.31.1; also in GCS index entries for `ambient`, 101 entries). Core charts (`base`, `istiod`, `gateway`, `cni`, `ztunnel`) have NO `kubeVersion` (0 matches in 1231 index entries except ambient). The ambient value is stale vs real support (1.23 vs 1.32+), so do not use it.
- Gateway API CRD minimum is called out in upgrade notes (1.30: "Gateway API CRDs must be upgraded to `v1.5.x`").
- Data-plane/control-plane skew policy text in supported-releases page: control plane may be one version ahead of data plane.
- Envoy/ztunnel/proxy pins: `ISTIO/istio.deps` (JSON: `PROXY_REPO_SHA`, `ZTUNNEL_REPO_SHA`, `AGENTGATEWAY_IMAGE` = `v1.4.1` at 1.31.1).

## 5. Helm charts

### 5a. Published locations
| Location | Status | Evidence |
|---|---|---|
| `https://istio-release.storage.googleapis.com/charts/index.yaml` [200] | Frozen for 1.31: last entries `1.31.0-rc.0`, stable up to `1.30.5` (also 1.29.8, 1.28.10...). `generated: 2026-09-21T19:17:15Z`. To be deleted Dec 2026. | parsed index, 1231 entries |
| `https://blob.istio.io/istio-release/charts/index.yaml` | New canonical HTTP repo. UNVERIFIED (proxy 403 for blob.istio.io) | `RB release/publish.sh:32` `R2_HELM_URL`; blog `retirement-of-gcp` |
| `oci://ghcr.io/istio/release/charts/<chart>` [200 tags/list via ghcr token] | OCI; has ALL of 1.26...1.31.1 incl. `1.31.0`, `1.31.1`; also 38 junk tags like `1.31.0-alpha.<sha>`, `1.30-alpha.<sha>` (daily builds; exclude) | `RB release/publish.sh:35`, release-1.30 `publish.sh:51,71` |
| `gcr.io/istio-release/charts` | OCI legacy, <=1.30 (release-1.30 `publish.sh:50,85`) UNVERIFIED listing | |
OCI manifest example (`ghcr.io/v2/istio/release/charts/istiod/manifests/1.31.1`): `config.mediaType application/vnd.cncf.helm.config.v1+json`, layer `application/vnd.cncf.helm.chart.content.v1.tar+gzip`, annotations `org.opencontainers.image.version: 1.31.1`, `...created: 2026-09-16T21:03:51Z` (build time, 5 days before publish). Blob download is redirected to a blocked host (UNVERIFIED chart contents for 1.31.x).

### 5b. Charts in GCS index (entry counts): `base` 239, `istiod` 239, `cni` 239, `gateway` 239, `ztunnel` 164 (since 1.18), `ambient` 101 (umbrella; tgz under `charts/samples/ambient-<v>.tgz`), `istiod-remote` 10 (1.23.x and earlier only). Same 5 core charts on ghcr. `RB pkg/build/helm.go` `repoHelmCharts` (core) = base, gateway, istio-cni, ztunnel, istio-control/istio-discovery; `repoSampleHelmCharts` = sample-charts/ambient; archive-only (not in repo index) = gateways/istio-egress, gateways/istio-ingress. `manifests/charts/default` exists in-repo but is not published.
Entry shape: `apiVersion: v2`, `appVersion`, `version`, `created`, `digest`, `urls[]`, `keywords`, `sources: [https://github.com/istio/istio]`, `icon`; ambient adds `type`, `kubeVersion`, `dependencies`.

### 5c. Chart version <-> app version
- Index: `version == appVersion == Istio version` for ALL 1225 entries that have appVersion (0 mismatches; the 6 entries without appVersion are 1.12.0-alpha.0/1 of base/cni/istiod). Examples: `istiod-1.30.5.tgz` version 1.30.5 / appVersion 1.30.5 / created 2026-09-21T19:17:15Z; `istiod-1.31.0-rc.0` 1.31.0-rc.0.
- In-repo `Chart.yaml` is a placeholder: `version: 1.0.0`, `appVersion: 1.0.0` ("This version is never actually shipped. istio/release-builder will replace it at build-time"). `RB pkg/build/helm.go` `stampChartForRelease` sets Version=AppVersion=manifest.Version and rewrites `hub:`/`tag:` in values.yaml. Therefore git tags do NOT contain real chart versions; use the Helm index/OCI or the tag name.
- Digest check: sha256 of downloaded `istiod-1.30.5.tgz` == index digest `c91d0f8c...` (verified).
- Default image hub baked into published charts DIFFERS by release line: 1.30.5 chart `values.yaml:258 hub: registry.istio.io/release` (RB release-1.30 `release/build.sh:50 DOCKER_HUB=${DOCKER_HUB:-registry.istio.io/release}`), 1.28 line `docker.io/istio` (release-1.28 build.sh:50), 1.31 line `docker.io/istio` (reverted in RB commit 77ff441 "Revert default registry in helm charts to docker (#2367)", 2026-07-25). `registry.istio.io` unreachable from sandbox (UNVERIFIED), scheduled for decommission Dec 2026 => 1.30.x charts' default hub will break.
### 5d. In-repo source paths (all [200] at 1.28.0, 1.29.0, 1.30.0, 1.31.1)
`manifests/charts/base/{Chart.yaml,values.yaml,files/crd-all.gen.yaml}`, `manifests/charts/istio-control/istio-discovery/{Chart.yaml,values.yaml}` (published as chart `istiod`), `manifests/charts/gateway/{Chart.yaml,values.yaml,values.schema.json}`, `manifests/charts/istio-cni/` (published as chart `cni`), `manifests/charts/ztunnel/`, `manifests/sample-charts/ambient/Chart.yaml`, profiles `manifests/charts/base/files/profile-*.yaml` and `manifests/zzz_profile.yaml`.

## 6. Images

### 6a. Registries / tags
- Authoritative for all current releases: **`docker.io/istio/<image>:<Version>`** (Docker Hub registry API [200] via `auth.docker.io` token + `registry-1.docker.io/v2/istio/<img>/tags/list`).
- Mirror `gcr.io/istio-release/<image>` (`https://gcr.io/v2/istio-release/<img>/tags/list` [200], includes `timeUploadedMs`): has every release through **1.30.5** (uploaded 2026-09-21T19:20Z) plus 1.31.0-rc.0; has NO `1.31.0`, `1.31.0-rc.2+`, `1.31.1`. Blog: "Istio 1.30 will be the last minor version with ... images published to gcr.io/istio-release, registry.istio.io/release". `registry.istio.io/release`: UNVERIFIED (no network route).
- Pre-release tags exist for each: `1.31.0-alpha.0 ... -rc.4` + `-debug`/`-distroless`.
- Tag format: `{{.Version}}` (no `v`), variants as suffix: `1.31.1` (default == debug base), `1.31.1-debug`, `1.31.1-distroless`. No `latest`, no floating minor tags (`1.31`) on Docker Hub pilot. Cosign signature tags `sha256-<digest>.sig` (934 in pilot) must be filtered out. Junk legacy tags (`1.15-beta.0`, `1.1.0.snapshot.0`, commit SHAs) exist.
- Signing keys (blog `retirement-of-gcp`): 1.18-1.31.0 `https://istio.io/misc/istio-key.pub`; 1.31.1+ `https://istio.io/misc/istio-key-v2.pub`.

### 6b. Image set (verified per-tag presence matrix on Docker Hub; P=plain, D=-debug, L=-distroless)
| version | pilot | proxyv2 | install-cni | ztunnel | istioctl | agentgateway | operator | app / ext-authz (test images) |
|---|---|---|---|---|---|---|---|---|
| 1.31.1, 1.31.0, 1.31.0-rc.4 | PDL | PDL | PDL | PDL | PDL | PDL | - | PDL |
| 1.30.5, 1.30.0 | PDL | PDL | PDL | PDL | PDL | PDL | - | PDL |
| 1.29.8, 1.29.0 | PDL | PDL | PDL | PDL | PDL | PDL | - | PDL |
| 1.28.10, 1.28.0 | PDL | PDL | PDL | PDL | PDL | - | - | PDL |
| 1.27.0, 1.26.0, 1.25.0, 1.24.0 | PDL | PDL | PDL | PDL | PDL | - | - | PDL |
| 1.23.6 | PDL | PDL | PDL | PDL | PDL | - | PDL | PDL |
- Image-set changes: `operator` removed after 1.23.6 (Docker Hub last plain tag 1.23.6; `tools/docker.yaml` lists `operator` at 1.23.6, absent at 1.24.0; announced in `announcing-1.23/_index.md` "Deprecating the in-cluster Operator"). `agentgateway` image (mirror of `cr.agentgateway.dev/agentgateway`) first appears in 1.29.0 (`tools/docker.yaml`; Docker Hub `istio/agentgateway:1.29.0` True, `1.28.0` False). `ztunnel` image first at 1.18.0. `app`, `ext-authz`, `app_sidecar_*` are test/sample images also pushed.
### 6c. Build-config evidence (exact file:line)
- `ISTIO tools/docker.yaml` @1.31.1: image names at lines 20 `base`, 24 `distroless`, 29 `proxyv2`, 36 `pilot`, 40 `agentgateway`, 43 `istioctl`, 48 `install-cni`, 54 `ztunnel`, 60 `app`, 69 `ext-authz`. `RAW/istio/istio/<tag>/tools/docker.yaml` [200] for 1.23.6, 1.24.0, 1.26.0, 1.28.0, 1.29.0, 1.30.0, 1.31.0, 1.31.1.
- `ISTIO tools/docker-builder/types.go:162-166` @1.31.1: `PrimaryVariant = DebugVariant`, `DefaultVariant = "default"`, `DebugVariant = "debug"`, `DistrolessVariant = "distroless"`; comment: "Tags will have the variant append (like 1.0-distroless). The DefaultVariant ... has no explicit tag ... Currently, it represents DebugVariant."
- `RB pkg/build/docker.go:29` `env := []string{"DOCKER_BUILD_VARIANTS=debug distroless"}`.
- `RB pkg/publish/docker.go:118-124`: `NewTag: fmt.Sprintf("%s/%s:%s", hub, imageName, tag)` (tag defaults to manifest.Version; line 82-84).
- `RB release/publish.sh` (master): line 34 `DOCKER_HUB=${DOCKER_HUB:-docker.io/istio}`, line 62 `--dockerhub "${DOCKER_HUB}" --dockertags "${VERSION}"`. release-1.30 publish.sh:49-50,85-86 also publishes `DOCKER_HUB_MIRROR=${DOCKER_HUB_MIRROR:-gcr.io/istio-release}`; release-1.28 same (publish.sh second `go run`); release-1.31 has no mirror.
- `ISTIO Makefile.core.mk:124` `HUB ?=istio`, `:133` `TAG ?= $(shell git rev-parse --verify HEAD)` (dev defaults only).

## 7. Release assets (GitHub release download URLs; HEAD with -L = 200 for every item below on 1.31.1, 1.31.0, 1.30.5, 1.30.0, 1.29.0, 1.28.0; also on pre-releases 1.31.0-rc.4, 1.31.0-alpha.0, 1.30.0-rc.0)
Pattern: `https://github.com/istio/istio/releases/download/{{.Tag}}/<asset>`
- `istio-{{.Version}}-linux-amd64.tar.gz`, `-linux-arm64.tar.gz`, `-linux-armv7.tar.gz`, `-osx-amd64.tar.gz`, `-osx-arm64.tar.gz`, `-win-amd64.zip`; legacy aliases `istio-{{.Version}}-osx.tar.gz`, `istio-{{.Version}}-win.zip` (RB `pkg/build/archive.go:37,125-139`).
- `istioctl-{{.Version}}-{linux-amd64,linux-arm64,linux-armv7,osx-amd64,osx-arm64}.tar.gz`, `istioctl-{{.Version}}-win-amd64.zip`, plus `istioctl-{{.Version}}-osx.tar.gz`, `-win.zip` aliases (archive.go:143-149).
- Every archive has a `<asset>.sha256` sibling (archive.go `util.CreateSha`; verified `.tar.gz.sha256`, `-win.zip.sha256`).
- SBOMs: `istio-source.spdx`, `istio-release.spdx` [200 on all tested tags] (RB `pkg/build/sbom.go:33-36`).
- 404 (do not exist as GH assets): `istio_<v>.sbom.json`, `istio-<v>.tar.gz`, `istio-sidecar.deb`. DEB/RPM/sources/licenses are NOT GitHub assets (upload filter `githubArtifiactsPattern = istio.*` on top-level dir only, github.go:36,102); they live on `blob.istio.io/istio-release/releases/<ver>/` (UNVERIFIED, blocked) and legacy `istio-release.storage.googleapis.com/releases/...`.
- Install script: `ISTIO release/downloadIstioCandidate.sh:72-73` builds `.../releases/download/${ISTIO_VERSION}/istio-${ISTIO_VERSION}-${OSEXT}[-${ISTIO_ARCH}].tar.gz`.

## 8. CRDs
- Path at tag (>=1.24): `manifests/charts/base/files/crd-all.gen.yaml` — [200] at 1.24.0, 1.25.0, 1.26.0, 1.27.0, 1.28.0, 1.28.10, 1.29.0, 1.30.0, 1.30.5, 1.31.0, 1.31.1. For <=1.23 (verified 1.19, 1.20, 1.21, 1.22, 1.23.0, 1.23.6): `manifests/charts/base/crds/crd-all.gen.yaml` (the `files/` path 404s there; the reverse 404s for >=1.24).
- Content (1.31.1, 15 CRDs, `apiextensions.k8s.io/v1`, header "DO NOT EDIT - Generated by Cue OpenAPI generator"): groups `extensions.istio.io` (WasmPlugin v1alpha1, TrafficExtension v1alpha1), `networking.istio.io` (DestinationRule/Gateway/ServiceEntry/Sidecar/VirtualService/WorkloadEntry/WorkloadGroup: v1 storage + v1alpha3 + v1beta1; EnvoyFilter v1alpha3; ProxyConfig v1beta1), `security.istio.io` (AuthorizationPolicy/PeerAuthentication/RequestAuthentication: v1 storage + v1beta1), `telemetry.istio.io` (Telemetry: v1 + v1alpha1 storage). Annotation `"helm.sh/resource-policy": keep`.
- History: 14 CRDs 1.20-1.29 (identical kind/version sets at 1.24, 1.25, 1.26, 1.27, 1.28.0, 1.28.10, 1.29.0); **`TrafficExtension` (extensions.istio.io) added in 1.30.0** (15 CRDs at 1.30.0, 1.30.5, 1.31.0, 1.31.1). No version removals in 1.24-1.31.
- Gateway API CRDs are NOT bundled (users install them; min version is in upgrade notes).

## 9. Security
- Bulletins: `content/en/news/security/istio-security-<YYYY>-<NNN>/index.md` (57 entries incl. `incorrect-sidecar-image-1.2.4`; 2026-001..006 so far; listing page `content/en/news/security/_index.md`). Front matter keys (verified on 2024-001, 2024-007, 2025-001, 2025-003, 2026-001, 2026-003, 2026-006):
```yaml
title: ISTIO-SECURITY-2026-006
subtitle: Security Bulletin
description: CVEs reported by Envoy, plus Istio security fixes ...
cves: [CVE-2026-73513, CVE-2026-73552, ...]      # may contain placeholder CVE-2026-XXXXX (2026-003)
cvss: "7.7"
vector: "CVSS:3.1/AV:N/AC:L/PR:L/UI:N/S:C/C:N/I:N/A:H"
releases: ["1.29.0 to 1.29.6", "1.30.0 to 1.30.3"]  # AFFECTED ranges, free text ("All releases prior to 1.19.0" possible)
publishdate: 2026-08-27
keywords: [CVE]
skip_seealso: true
```
  Body: `{{< security_bulletin >}}`, `## CVE` with `### Envoy CVEs` (bullets `__[CVE-...](nvd link)__: (CVSS score N): ...`) and `### Istio CVEs` (GHSA links, e.g. `GHSA-qm8v-g4f9-qhjx (CVSS score 6.8, Moderate)`), `## Am I Impacted?`, `## Mitigation` ("For Istio 1.30 users: upgrade to **1.30.4** or later.").
- Patch release notes: `## Security Update` (case varies) with bullets `- [CVE-2026-47774](https://github.com/envoyproxy/envoy/security/advisories/GHSA-...) (CVSS score 7.5, High): ...` (1.30.1, 1.28.8, 1.30.4...) or `### Istio CVEs` GHSA bullet (1.31.1). Not every security fix is called out there: some fixes appear only as `**Fixed** ...` bullets with `**Credit**:` lines (e.g. 1.31.1 sidecar annotation injection fix).
- Structured: `releasenotes/notes/*.yaml` with `kind: security-fix` (19 files) and optional `securityNotes` (7 files), see 2c.
- Policy: `ISTIO .github/SECURITY.md` ([200] at tag 1.31.1; NOT at repo root or master root) points to `https://istio.io/news/support/`, `https://istio.io/about/security-vulnerabilities/` (source `content/en/docs/releases/security-vulnerabilities/index.md`), `https://istio.io/news/security/`; reports to `istio-security-vulnerability-reports@googlegroups.com`.
- GitHub Security Advisories (GHSA) list: UNVERIFIED (github.com/api.github.com blocked); they are linked from bulletins/patch notes.
- Support/EOL: `content/en/news/support/announcing-1.29-eol/index.md` ("support for 1.29 will end on the 12th of October, 2026"), `announcing-1.28-eol-final` (2026-07-01).

## 10. Historical validation matrix (script run 2026-10-01)

| version | git tag | notes dir (IO) | DockerHub pilot P/D/L + ztunnel | gcr pilot | GCS Helm index istiod | ghcr OCI istiod/base/ztunnel | GH asset linux-amd64 | CRD path 200 | releasenotes/ at tag |
|---|---|---|---|---|---|---|---|---|---|
| 1.26.0 | yes | master | T/T/T/T | yes | yes | yes/yes/yes | 200 | 200 | 200 |
| 1.26.8 | yes | master | T/T/T/T | yes | yes | yes | 200 | 200 | 200 |
| 1.27.0 | yes | master | T | yes | yes | yes | 200 | 200 | 200 |
| 1.27.9 | yes | master | T | yes | yes | yes | 200 | 200 | 200 |
| 1.28.0 | yes | master | T | yes | yes | yes | 200 | 200 | 200 |
| 1.28.10 | yes | master | T | yes | yes | yes | 200 | 200 | 200 |
| 1.29.0 | yes | master | T | yes | yes | yes | 200 | 200 | 200 |
| 1.29.8 | yes | master (late cherry-pick) | T | yes | yes | yes | 200 | 200 | 200 |
| 1.30.0 | yes | master | T | yes | yes | yes | 200 | 200 | 200 |
| 1.30.5 | yes | master (late cherry-pick) | T | yes | yes | yes | 200 | 200 | 200 |
| 1.31.0 | yes | master | T | **NO** | **NO** | yes | 200 | 200 | 200 |
| 1.31.0-rc.4 | yes | n/a | T | NO | NO | yes | 200 | 200 | 200 |
| 1.31.1 | yes | **release-1.31 branch only** | T | **NO** | **NO** | yes | 200 | 200 | 200 |

Relationships that HELD for all >=3 historical releases: bare-semver tags; `announcing-*` path templates; `publishdate` == tag date; Docker Hub P/-debug/-distroless triple for pilot, proxyv2, install-cni, ztunnel, istioctl; chart version == appVersion == tag; GitHub asset names and `.sha256` siblings; `releasenotes/notes` present with schema v2; CRD file path (with the `crds/`->`files/` switch at 1.24); `TrafficExtension` CRD first in 1.30; supportStatus rows for each minor.
Relationships that did NOT hold / have exceptions:
- gcr.io mirror and GCS Helm index: stop at 1.31.0-rc.0 (stable 1.31.0+ absent).
- Docker Hub `operator` image: only <=1.23.6; `agentgateway`: only >=1.29.0.
- Patch notes on `master`: 1.31.1 missing (branch-only), 1.24.1 missing entirely, 1.29.8/1.30.5 arrived 9 days late.
- istio/istio git tags missing for some published pre-releases (`1.31.0-alpha.1`, `-rc.1`).
- supportStatus.yml vs announcement tip for K8s versions (1.26, 1.28, 1.31) and release dates (1.30, 1.31).
- Chart default hub differs across minors (docker.io/istio vs registry.istio.io/release).
- Not reachable from this sandbox (UNVERIFIED): blob.istio.io (Helm index, releases dir, DEB/RPM/SPDX), registry.istio.io, quay.io, artifacthub, osv, GitHub Releases API (draft/prerelease flags, release bodies), GitHub security advisories list, OCI chart blob contents.

## 11. Suggested declarative definition hints

```yaml
product: istio
repos:
  source: istio/istio
  docs: istio/istio.io
  tooling: [istio/release-builder, istio/tools]
tags:
  stable_regex: '^(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)$'          # no v prefix
  prerelease_regex: '^\d+\.\d+\.\d+-(?P<stage>alpha|beta|rc)\.(?P<n>\d+)$'
  ignore_regex: 'snapshot|^\d+\.\d+-beta'                                   # 1.0.0-snapshot.N, 1.1.0.snapshot.N, 1.15-beta.0
  note: pre-release numbering has gaps; ISTIO git VERSION file holds only "M.m"; GitHub prerelease flag unreliable (RB creates all as draft+prerelease)
latest_stable_cross_check: RAW/istio/istio.io/<newest release-M.m branch>/data/args.yml -> full_version   # NOT master (master = preliminary)
release_notes:
  repo: istio/istio.io
  branches: [master, "release-{{.Major}}.{{.Minor}}"]      # newest minor's patch notes only on its release branch; older minors on master with lag
  minor_announcement: content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/_index.md      # patch==0
  change_notes:      content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/change-notes/index.md
  upgrade_notes:     content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/upgrade-notes/index.md
  patch_notes:       content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Version}}/index.md                    # patch>0
  eol_notice:        content/en/news/support/announcing-{{.Major}}.{{.Minor}}-eol/index.md
  public_url:        https://istio.io/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Version}}/   # minor: announcing-{{.Major}}.{{.Minor}}/
  front_matter: [title, linktitle, subtitle(Patch Release|Major Release|Minor Release), publishdate, release]
  change_notes_h2: [Traffic Management, Security, Telemetry, Extensibility, Installation, istioctl, Documentation changes]
  entry_regex: '^- \*\*(?P<verb>Fixed|Added|Improved|Updated|Upgraded|Promoted|Removed|Deprecated|Optimized|Enabled)\*\*'
  breaking_signal_verbs: [Removed, Deprecated, Promoted]   # plus every H2 in upgrade-notes
  patch_security_h2: '(?i)^## Security Updates?$'          # sub-H3: Envoy CVEs | Istio CVEs | (Other )?Istio Security Fixes
structured_notes:
  path_at_tag: releasenotes/notes/*.yaml                   # @{{.Tag}}; set added between previous tag and this tag
  schema: release-notes/v2   # kind(feature|bug-fix|security-fix|test), area, issue[], releaseNotes[], upgradeNotes[{title,content}], securityNotes[], docs[]
  caveats: no action-required field; lenient parse (typos upgradeNodes/upgradeNote/issues); accumulated dir
security:
  bulletins: content/en/news/security/istio-security-{{year}}-{{nnn}}/index.md   # fm: cves[], cvss, vector, releases[] (affected ranges), publishdate
  policy: .github/SECURITY.md @tag
compatibility:
  file: data/compatibility/supportStatus.yml (istio.io master)      # also rendered in content/en/docs/releases/supported-releases/index.md
  columns: [Version, Currently Supported, Release Date, End of Life, Supported Kubernetes Versions, Tested but not supported]
  keys: [version, supported, releaseDate, eolDate, k8sVersions, testedK8sVersions]
  extra_tables: ["Supported releases without known CVEs", "Supported Envoy Versions (Istio version -> Envoy release branch)"]
  announcement_tip_regex: 'officially supported on Kubernetes versions (\d+\.\d+) to (\d+\.\d+)'
  caveat: mutable post-release; release dates in table can differ from tags (prefer publishdate/tag date)
images:
  registries_by_version: {">=1.31.0": [docker.io/istio], "<=1.30.x": [docker.io/istio, gcr.io/istio-release]}   # gcr.io removal Dec 2026
  repositories: [pilot, proxyv2, install-cni, ztunnel, istioctl, agentgateway(>=1.29), operator(<=1.23)]
  test_images_ignore: [app, ext-authz, app_sidecar_*]
  tag: '{{.Version}}'          # variants: '{{.Version}}-debug', '{{.Version}}-distroless'; default (unsuffixed) == debug base
  exclude_tags: '\.sig$|^[0-9a-f]{40}$|snapshot'
  signing_key: {"<=1.31.0": https://istio.io/misc/istio-key.pub, ">=1.31.1": https://istio.io/misc/istio-key-v2.pub}
helm:
  charts: [base, istiod, gateway, cni, ztunnel, ambient(sample umbrella)]   # istiod-remote only <=1.23
  version_rule: chart version == appVersion == '{{.Version}}'               # in-repo Chart.yaml is placeholder 1.0.0
  http_repo: ["https://blob.istio.io/istio-release/charts/index.yaml (current; unverified here)", "https://istio-release.storage.googleapis.com/charts/index.yaml (legacy, <=1.30.x + 1.31.0-rc.0)"]
  oci: 'oci://ghcr.io/istio/release/charts/{{chart}}:{{.Version}}'          # verified for 1.26..1.31.1; legacy oci://gcr.io/istio-release/charts (<=1.30)
  oci_exclude_tags: '-alpha\.[0-9a-f]{40}$|^\d+\.\d+-alpha\.'
  source_paths: {base: manifests/charts/base, istiod: manifests/charts/istio-control/istio-discovery, gateway: manifests/charts/gateway, cni: manifests/charts/istio-cni, ztunnel: manifests/charts/ztunnel, ambient: manifests/sample-charts/ambient}
  kubeVersion: only ambient chart (">= 1.23.0-0", stale) -> do not use
assets:
  base_url: https://github.com/istio/istio/releases/download/{{.Tag}}/
  files: [istio-{{.Version}}-{linux-amd64,linux-arm64,linux-armv7,osx-amd64,osx-arm64}.tar.gz, istio-{{.Version}}-win-amd64.zip, istioctl-{{.Version}}-<same suffixes>, each + .sha256, istio-source.spdx, istio-release.spdx]
  legacy_aliases: [istio-{{.Version}}-osx.tar.gz, istio-{{.Version}}-win.zip]
  mirror_unverified: https://blob.istio.io/istio-release/releases/{{.Version}}/
crds:
  path: {">=1.24": manifests/charts/base/files/crd-all.gen.yaml, "<=1.23": manifests/charts/base/crds/crd-all.gen.yaml}
  groups: [extensions.istio.io, networking.istio.io, security.istio.io, telemetry.istio.io]
  kinds_added: {"1.30.0": [TrafficExtension]}
```
