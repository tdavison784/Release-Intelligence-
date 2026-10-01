# cert-manager release channel map (verified 2026-10-01)

Scope: product `cert-manager` (github.com/cert-manager/cert-manager), docs (github.com/cert-manager/website), release tooling (github.com/cert-manager/release = "cmrel").
Method: `git ls-remote`, shallow clones at tags, raw.githubusercontent.com (HTTP 200 checked), GitHub release-asset downloads (HEAD/GET).
Snapshots read: website `master` @ fba4f5c (2026-09-30); cert-manager tags v1.21.2 / v1.21.0 / v1.20.4 / v1.20.0 / v1.19.6 / v1.18.6 / v1.17.4 / v1.16.5 / v1.15.5 / v1.14.7 / v1.13.6; cmrel `master` @ 09eb29d (2026-09-27).

Abbreviations used for URLs:
- `WEB` = https://raw.githubusercontent.com/cert-manager/website/master/content/docs
- `CM@<tag>` = https://raw.githubusercontent.com/cert-manager/cert-manager/<tag>
- `REL` = https://raw.githubusercontent.com/cert-manager/release/master  (NB: branch is `master`, `main` returns 404)
- website default branch = `master`; cert-manager default branch = `master`.

Blocked, not retried: api.github.com, github.com HTML (Releases/Security pages), quay.io, charts.jetstack.io, artifacthub.io, cert-manager.io, osv.dev, vuln.go.dev, storage.googleapis.com/cert-manager-release (403), jetstack/jetstack-charts (private/404 on raw).

---

## 1. Canonical versions

Source: `git ls-remote --tags https://github.com/cert-manager/cert-manager` (299 refs).

- Product tag regex: `^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(-(alpha|beta)\.\d+)?$`. `v` prefix always (one legacy outlier `v0.15-alpha.3`). Tags are annotated (e.g. `v1.21.2` tag object `f00885ac...` peels to commit `922a06aa...`).
- Pre-releases: only `-alpha.N` (65 tags) and `-beta.N` (41 tags). No `-rc`. Examples: `v1.21.0-alpha.0`, `v1.21.0-alpha.1`, `v1.21.0-beta.0`. Oddity: patch-level pre-releases exist (`v1.15.2-alpha.0`, `v1.15.2-alpha.1`).
- Non-product tags to ignore: 37 tags `cmd/ctl/v1.12.0 ... cmd/ctl/v1.14.7` (Go submodule tags for the old in-repo cmctl). Filter with the regex above (anchored, no path prefix).
- Documented scheme (website `releases/README.md`, "Terminology" table): final `v1.3.0`, patch `v1.3.1`, pre-release `v1.4.0-alpha.0`; release branch `release-1.3`. URL: WEB/releases/README.md (200).
- Release branches present: `release-1.16 ... release-1.21` (`git ls-remote --heads`).

Stable tags, three most recent minor lines (+ pre-release tags of newest line):
| Minor | Stable tags | Tag date of first / last (annotated tag date) |
|---|---|---|
| 1.21 | v1.21.0, v1.21.1, v1.21.2 | v1.21.0 2026-07-08; v1.21.2 2026-09-10 |
| 1.20 | v1.20.0, v1.20.1, v1.20.2, v1.20.3, v1.20.4 | v1.20.0 2026-03-10; v1.20.4 2026-09-16 |
| 1.19 | v1.19.0 ... v1.19.6 | v1.19.0 2025-10-07; v1.19.6 2026-06-25 |
| (1.18) | v1.18.0 ... v1.18.6 | v1.18.0 2025-06-10; v1.18.6 2026-02-24 |
| (1.17) | v1.17.0 ... v1.17.4 | v1.17.4 2025-07-02 |
| (1.16) | v1.16.0 ... v1.16.5 | v1.16.5 2025-04-24 |
Pre-releases of newest line: v1.21.0-alpha.0, v1.21.0-alpha.1, v1.21.0-beta.0. No v1.22 tag exists yet (README: 1.22 "~Nov 2026").

**Latest stable as of 2026-10-01 = `v1.21.2`** (highest semver stable). Cross-checks (all 200):
- WEB/variables.json: `{"cert_manager_latest_version": "v1.21.2"}` (used by install docs via `[[VAR::cert_manager_latest_version]]`).
- WEB/releases/release-notes/release-notes-1.21.md top patch section is `## \`v1.21.2\``.
- GOTCHA for ranking: chronological newest tag != semver highest. `v1.20.4` was tagged 2026-09-16, six days AFTER `v1.21.2` (2026-09-10). Sort by semver, not by date.

Support status (WEB/releases/README.md "Currently supported releases"): supported = 1.21, 1.20 (EOL "Release of 1.23" / "Release of 1.22"). 1.19 EOL Jul 08, 2026 (last patch v1.19.6).

## 2. Release notes (beyond GitHub Releases)

- GitHub Release body = `github-release-description.md` (generated with the k8s `release-notes` tool, `--markdown-links=false`); website version = `website-release-notes.md` (`--markdown-links=true`). Source: WEB/contributing/release-process.md (200), "Prepare the Release Notes PR". GitHub Releases itself UNVERIFIED (api blocked).
- **Path template (per minor, one file for all patches):** `content/docs/releases/release-notes/release-notes-{{.Major}}.{{.Minor}}.md`
  - Raw URL: `WEB/releases/release-notes/release-notes-1.21.md` (200; also 1.20, 1.19, 1.18, 1.17, 1.16, 1.15, 1.14, 1.13 all 200).
  - Files exist for 0.1 ... 1.21. Patch notes are appended to the same file (release-process.md step 4.5).
  - Site route registered in `content/docs/manifest.json` (e.g. `"path": "/docs/releases/release-notes/release-notes-1.21.md"`). Public URL pattern UNVERIFIED (cert-manager.io blocked); README.md mentions https://cert-manager.io/docs/release-notes/.
- **Front matter:** `title: Release 1.18`, `description: 'cert-manager release notes: cert-manager 1.18'` (1.15 file has copy-paste description "cert-manager 1.14" - do not trust description).
- **Per-patch heading:** `## \`v1.18.6\`` (H2, version in backticks) for 1.12-1.19 and 1.21; `## v1.20.4` (no backticks) for 1.20 and 1.5-1.11. Regex that matched every patch of 1.5-1.21 and equals the git stable-tag set exactly (verified by diffing headings vs `git ls-remote`, all 17 minors identical): `^##\s+` + "`"? + `v(\d+\.\d+\.\d+)` + "`"? + `\s*$`. For 1.0-1.4 headings differ (`# Final Release \`v1.4.0\``) and per-patch headings are absent.
- Top of file (before patches): intro paragraph, `## Major Themes` (or `## Breaking changes`/`## Themes`/`## Important Upgrade Notes`), `## Community` (contributors/maintainers/steerers), then newest-first patch sections. Breaking changes are called out inside Major Themes with blockquote markers `> ⚠️ Breaking change` / `> ⚠️ Potentially breaking change` (1.18, 1.19, 1.21). 1.16 uses `## Breaking changes`, 1.19 uses `## Important Upgrade Notes`.
- **Category headings inside a patch (from k8s release-notes "kind"):** `Feature`, `Documentation`, `Bug or Regression`, `Other (Cleanup or Flake)`; legacy variants: `Bug Fixes`, `Bugfixes`, `Other`, `Other Changes`, `Uncategorized`, `Design`, `Security`, `Breaking Changes`. Level is `###` (older/some) or `####` when wrapped by `### Changes by Kind` (1.18.5+, 1.19.x, partly) or `### Changelog since vX` (1.20). There is NO dedicated "Breaking Changes"/"Deprecations" kind section in 1.17-1.21 (1.14 has `Breaking Changes`, 1.16 a top-level `## Breaking changes`); breaking/deprecation text lives in Major Themes prose and in the upgrade doc. Individual bullets: `- <text> ([\`#8941\`](https://github.com/cert-manager/cert-manager/pull/8941), [\`@user\`](https://github.com/user))`.
- Patch sections have a prose summary then `Changes since \`vX.Y.Z-1\`:` (or `### Changelog since vX`), no dates.
- Optional machine markers (inconsistent, do not rely): `{/* BEGIN changelog v1.21.2 */}` ... `{/* END changelog v1.21.2 */}` present for all 3 patches in 1.21, v1.19.0-2 only in 1.19, v1.20.0 only in 1.20, absent in <= 1.18. Also `{/* BEGIN contributors|maintainers|steerers */}` blocks in 1.19+.
- Pre-releases (alpha/beta) are NOT covered by website notes.

Heading excerpt, release-notes-1.18.md (`grep -nE '^#{1,4} '`):
```
14:## Major Themes
16:### OperatorHub Packages Discontinued
28:### ACME HTTP01 challenge paths now use `PathType` `Exact` in Ingress routes   (> ⚠️ Breaking change)
109:### The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`
168:## Community
210:## `v1.18.6`
216:### Changes by Kind
218:#### Bug or Regression
222:## `v1.18.5`  (### Changes by Kind / #### Bug or Regression / #### Other (Cleanup or Flake))
240:## `v1.18.4`  (### Bug or Regression / ### Other (Cleanup or Flake))
292:## `v1.18.1`  (### Feature / ### Bug or Regression / ### Other (Cleanup or Flake))
319:## `v1.18.0`
323:### Feature
341:### Documentation
345:### Bug or Regression
362:### Other (Cleanup or Flake)
```
Heading excerpt, release-notes-1.21.md:
```
10:## Major Themes   (### Default `tokenrequest` RBAC removed from Helm chart  > ⚠️ Breaking change; ### Restrict Challenge and Order RBAC...; ### Metrics port name and path Helm values removed ...)
77:## Community
135:## `v1.21.2`      (preceded by {/* BEGIN changelog v1.21.2 */})
148:### Bug or Regression
168:### Other (Cleanup or Flake)
176:## `v1.21.1`
206:## `v1.21.0`   -> ### Feature / ### Bug or Regression / ### Other (Cleanup or Flake)
```
1.20 file: `## v1.20.4` ... `### Changelog since v1.20.3` / `#### Bug or Regression` / `#### Other (Cleanup or Flake)`; `## v1.20.0` has `### Major Themes`, `### Community`, `### Changelog since v1.19.0`.

- **CHANGELOG in main repo: NONE.** `CHANGELOG.md`, `CHANGELOG`, `docs/CHANGELOG.md` are 404 at master, v1.21.2, v1.18.6, v1.16.5; `find -iname '*changelog*'` empty. Repo root has `RELEASE.md` (200; points to website for process and artifacts).
- Related per-minor support/timeline page: WEB/releases/README.md (see section 4).

## 3. Upgrade / migration docs

- **Path template:** `content/docs/releases/upgrading/upgrading-{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md` (PrevMinor = Minor-1, major 1). One file per minor transition, created at the FINAL release (release-process.md: "(final release) Create a new file ... upgrading-1.19-1.20.md"), so no doc exists for a minor before `X.Y.0`.
- Verified 200 (raw): `WEB/releases/upgrading/upgrading-1.20-1.21.md`, `upgrading-1.19-1.20.md`, `upgrading-1.18-1.19.md`, `upgrading-1.17-1.18.md`, `upgrading-1.16-1.17.md`, `upgrading-1.15-1.16.md`, `upgrading-1.14-1.15.md`, `upgrading-1.13-1.14.md`. Continuous run 1.0-1.1 ... 1.20-1.21 except 1.12->1.13 (404).
- EXCEPTION: `upgrading-1.12.md` (title "Upgrading from v1.12"; 1.12 was an LTS; covers 1.12->1.13 and 1.12->1.17). `upgrading-1.12-1.13.md` = 404. Also non-versioned docs in the same dir: `ingress-class-compatibility.md`, `remove-deprecated-apis.md`.
- Structure: front matter `title: Upgrading from v1.20 to v1.21`, `description: 'cert-manager installation: Upgrading v1.20 to v1.21'`; body is either a numbered breaking-change list ("Before upgrading cert-manager from 1.17 to 1.18, please read the following important notes about breaking changes in 1.18:" with items linking to `../release-notes/release-notes-1.18.md`) or `## Potentially Breaking: <topic>` / `## Use the latest patch version` headings (1.18->1.19 warns "Do not install v1.19.0 ... fixed in v1.19.1"); ends with `## Next Steps` linking `../../installation/upgrade.md`. Size 14-65 lines. 1.19->1.20 doc (14 lines) only describes a change shipped in v1.20.3 (GHSA-8rvj-mm4h-c258), i.e. upgrade docs are also edited after patch releases.
- Registered in WEB/manifest.json routes ("Upgrade 1.20 to 1.21").
- Generic upgrade procedure: WEB/installation/upgrade.md (200).

## 4. Compatibility

Page: `content/docs/releases/README.md` (title "Supported Releases"). URL: `WEB/releases/README.md` (200). Same page contains TWO release tables with DIFFERENT headers (parse by section heading):

(a) `## Currently supported releases` (and `## Upcoming releases`, same columns):
```
| Release  | Release Date | End of Life     | [Supported Kubernetes / OpenShift Versions][s] | [Tested Kubernetes Versions][test] |
| [1.21][] | Jul 08, 2026 | Release of 1.23 | 1.33 → 1.36 / 4.20 → 4.22                      | 1.33 → 1.36                        |
| [1.20][] | Mar 10, 2026 | Release of 1.22 | 1.32 → 1.35 / 4.19 → 4.21                      | 1.32 → 1.35                        |
Upcoming: | 1.22     | ~Nov 2026    | Release of 1.24 | TBD                                             | TBD                                 |
```
(b) `## Old cert-manager releases` (EOL):
```
| Release      | Release Date | EOL          | Compatible Kubernetes versions | Compatible OpenShift versions |
| [1.19][]     | Oct 07, 2025 | Jul 08, 2026 | 1.31 → 1.35                    | 4.18 → 4.20                   |
| [1.18][]     | Jun 10, 2025 | Mar 10, 2026 | 1.29 → 1.33                    | 4.16 → 4.20                   |
| [1.12 LTS][] | May 19, 2023 | May 19, 2025 | 1.22 → 1.32                    | 4.9 → 4.16                    |
```
Parsing notes: first col is a markdown reference link `[1.21][]` (target `./release-notes/release-notes-1.21.md`) or plain; arrow is U+2192 `→`; supported-table K8s cell is "K8sMin → K8sMax / OpenShiftMin → OpenShiftMax"; EOL is relative ("Release of 1.23") for supported rows and an absolute date for old rows; "1.12 LTS" label; 1.7 row has a typo date `Jan 26, 2021` (should be 2022). Other tables on page: vendor table (EKS/GKE/AKS/OpenShift "Oldest K8s Release"), OpenShift->K8s mapping (`OpenShift versions | Kubernetes version`), LTS vendors table (`Release | Vendor | End of Life`, e.g. "1.17 LTS | Palo Alto Networks | Feb 03 2027"). Policy text: "at least two supported versions", support window ends at release of second subsequent minor; security fixes back-ported to last two releases.
- Historical edits of the page (git log, 23 commits since 2024-09): release dates are edited before the final release (e.g. "Delay cert-manager 1.21 release by 1 week", "update 1.20 release date from Feb 10 to Feb 24"; final shipped date Mar 10). Treat future dates as soft.
- Helm chart `kubeVersion` is NOT a reliable support signal: Chart template has `kubeVersion: ">= 1.22.0-0"` at every tag checked (v1.13.6, v1.14.7, v1.15.5, v1.16.5, v1.17.4, v1.18.6, v1.19.6, v1.20.4, v1.21.2) while README supports 1.33+ for 1.21. Evidence: CM@v1.21.2/deploy/charts/cert-manager/Chart.yaml line 22; CM@v1.20.4/deploy/charts/cert-manager/Chart.template.yaml line 23.
- Corroborating (not authoritative): `go.mod` `k8s.io/api` v0.36.2 at v1.21.2, v0.35.2 at v1.20.4, v0.34.1 at v1.19.6, v0.32.0 at v1.18.6/v1.17.4; `make/e2e-setup.mk:27 K8S_VERSION := 1.34`.

## 5. Helm chart

- In-repo path: `deploy/charts/cert-manager/` containing `Chart.yaml` (>= v1.21.0-alpha.1) or `Chart.template.yaml` (<= v1.21.0-alpha.0), `values.yaml`, `values.schema.json`, `README.template.md`, `signkey_annotation.txt`, `templates/`, `tests/`. 
  - `Chart.template.yaml` 200 at v1.20.4, v1.19.6, v1.18.6, v1.17.4, v1.16.5, v1.14.7, v1.13.6 (and v1.21.0-alpha.0); `Chart.yaml` 200 at v1.21.0-alpha.1, v1.21.0-beta.0, v1.21.0, v1.21.2 and 404 at v1.20.0-beta.0 and earlier (checked via raw). **Rename happened at v1.21.0-alpha.1.** Keep both candidates in a definition.
- Chart.yaml is a TEMPLATE with placeholders: CM@v1.21.2/deploy/charts/cert-manager/Chart.yaml lines 22-26:
```
kubeVersion: ">= 1.22.0-0"
# The version and appVersion fields are set automatically by the release tool
version: v0.0.0
appVersion: v0.0.0
```
  (name: cert-manager; description "A Helm chart for cert-manager"; annotations artifacthub.io/license, artifacthub.io/category). Identical placeholders in Chart.template.yaml at v1.16.5-v1.20.4. At build, make/manifests.mk:119 rewrites with yq: `.annotations."artifacthub.io/signKey" = ... | .annotations."artifacthub.io/prerelease" = "$(IS_PRERELEASE)" | .version = "$(VERSION)" | .appVersion = "$(VERSION)"` and make/manifests.mk:87 `$(HELM) package --app-version=$(VERSION) --version=$(VERSION)`. `VERSION` = `git describe --tags --match='v*'` (Makefile:87-88), so **chart version == appVersion == git tag including the `v` prefix** (e.g. `v1.21.2`); package file `cert-manager-$(VERSION).tgz` (manifests.mk:25/86). `IS_PRERELEASE` true iff tag contains `-` (Makefile:89).
- values.yaml: `deploy/charts/cert-manager/values.yaml` at the tag (200 for v1.16.5-v1.21.2); JSON schema `values.schema.json` (200 same tags; chart rejects unknown keys since 1.16).
- Image values change over time (important for chart parsing):
  - <= v1.19.x: per-component `repository: quay.io/jetstack/cert-manager-<component>`, tag defaults to `.Chart.AppVersion`.
  - >= v1.20.0-beta.0: global `imageRegistry: quay.io` + `imageNamespace: jetstack` + `<component>.image.name: cert-manager-<component>`, with `repository: ""` and deprecated `registry` (values.yaml v1.21.2 lines 182, 189, 206 `name: cert-manager-controller`, 1042 webhook, 1428 cainjector, 1499 acmesolver, 1669 startupapicheck). Helper `cert-manager.image` in templates/_helpers.tpl; templates deployment.yaml:89, webhook-deployment.yaml:84, cainjector-deployment.yaml:79, startupapicheck-job.yaml:60, controller arg `--acme-http01-solver-image=` deployment.yaml:117.
- **Published locations (evidence in repo/tooling; endpoints unreachable here, so UNVERIFIED live):**
  - OCI: `oci://quay.io/jetstack/charts/cert-manager`. Evidence: CM@v1.21.2/RELEASE.md:16 ("published to `quay.io/jetstack/charts/cert-manager` on each cert-manager release"); CM@v1.21.2/make/e2e-setup.mk:281 `E2E_CERT_MANAGER_CHART ?= oci://quay.io/jetstack/charts/cert-manager`; CM@v1.21.2/hack/verify-upgrade.sh:67 `HELM_URL="oci://quay.io/jetstack/charts/cert-manager"`; REL/hack/push_and_sign_chart.sh:90 `helm push $TEMP_DIR/cert-manager-$RELEASE_VERSION.tgz oci://quay.io/jetstack/charts`; same script copies to a NON-v-prefixed tag (`crane copy ...:$RELEASE_VERSION ...:$RELEASE_VERSION_NO_V`) -> OCI tags `v1.21.2` AND `1.21.2`; both cosign-signed. WEB/installation/helm.md: "OCI Helm charts are the source of truth and are published immediately upon release" and `oci://quay.io/jetstack/charts/cert-manager:[[VAR::cert_manager_latest_version]]`.
  - HTTP repo: `https://charts.jetstack.io` (`helm repo add jetstack https://charts.jetstack.io`). Evidence: WEB/installation/helm.md ("legacy HTTP Helm repository ... is updated a few hours after the OCI charts are published"; "Very old versions (earlier than v1.12) are only officially available from the legacy Helm repository"); CM@v1.21.2/design/20240625.push-charts-to-oci.md; WEB/contributing/release-process.md step 10-12 (cmrel opens a PR to github.com/jetstack/jetstack-charts `charts/cert-manager-<ver>.tgz` + `.prov`; REL/pkg/release/consts.go:64-72 `DefaultHelmChartGitHubOwner = "jetstack"`, `DefaultHelmChartGitHubRepo = "jetstack-charts"`; REL/pkg/release/helm/helm.go `"charts/" + chartFileName`). jetstack-charts repo is not publicly readable (raw 404).
  - ArtifactHub page (per release-process.md): https://artifacthub.io/packages/helm/cert-manager/cert-manager (blocked).
  - Chart is NOT attached to GitHub Releases (probe 404 for `cert-manager-v1.21.2.tgz`).
  - Staging bucket `gs://cert-manager-release/stage/gcb/release/<version>/cert-manager-manifests.tar.gz` (contains `deploy/chart/cert-manager-<ver>.tgz(.prov)` + `deploy/manifests/*.yaml`) is private (403).
- Chart <-> app relationship: identical string `vX.Y.Z[-pre]`; default image tag = `.Chart.AppVersion`.

## 6. Container images

Registry/namespace: **quay.io/jetstack** (REL/pkg/release/consts.go:52 `DefaultImageRepository = "quay.io/jetstack"`). Naming (REL/cmd/cmrel/cmd/gcb_publish.go:691-697):
```
func buildManifestListName(repo, componentName, tag string) string { return fmt.Sprintf("%s/cert-manager-%s:%s", repo, componentName, tag) }   // multi-arch manifest list = the public tag
func buildImageTag(repo, componentName, arch, tag string) string  { ...  "%s/cert-manager-%s-%s:%s" }                                          // per-arch images (…-amd64:vX.Y.Z etc.)
```
Tag format: git tag verbatim incl. `v` and pre-release suffix (`v1.21.2`, `v1.21.0-beta.0`); no `latest`, no digest-only scheme evidence. Images are cosign-signed after push (gcb_publish.go `signRegistryContent`); published only in the manual `cmrel publish --nomock` step (WEB/contributing/release-process.md step 10), NOT by GitHub Actions (CM@v1.21.2/.github/workflows has only govulncheck.yaml + scorecards.yml). Build is triggered by tag push via Google Cloud Build: CM@v1.21.2/gcb/build_cert_manager.yaml (`make ... upload-release`, line 25 at v1.16.5).

Image set (v1.21.2 = latest; identical v1.15.x-v1.21.x):
| Image | Full name | Evidence (CM@v1.21.2 unless noted) |
|---|---|---|
| controller | quay.io/jetstack/cert-manager-controller:<tag> | make/containers.mk:58 `TAG := cert-manager-controller-$*:$(VERSION)`; values.yaml:206 |
| webhook | quay.io/jetstack/cert-manager-webhook:<tag> | make/containers.mk:71; values.yaml:1042 |
| cainjector | quay.io/jetstack/cert-manager-cainjector:<tag> | make/containers.mk:84; values.yaml:1428 |
| acmesolver | quay.io/jetstack/cert-manager-acmesolver:<tag> | make/containers.mk:97; values.yaml:1499; Go default `internal/apis/config/controller/v1alpha1/defaults.go:94` = `fmt.Sprintf("quay.io/jetstack/cert-manager-acmesolver:%s", util.AppVersion)` |
| startupapicheck | quay.io/jetstack/cert-manager-startupapicheck:<tag> | make/containers.mk:110; values.yaml:1669; templates/startupapicheck-job.yaml:60 |
Aggregate lists: make/containers.mk:52 `all-containers: cert-manager-controller-linux cert-manager-webhook-linux cert-manager-acmesolver-linux cert-manager-cainjector-linux cert-manager-startupapicheck-linux`; make/00_mod.mk:62 `build_names := controller acmesolver webhook cainjector startupapicheck`; make/release.mk:110-114 (`gunzip ... acmesolver/cainjector/controller/webhook/startupapicheck .tar`) and `.docker_tag` files = `$(VERSION)` (lines ~105-109). Platforms: linux amd64, arm64, s390x, ppc64le, arm (containers.mk:55). `ko` (make/ko.mk) is dev-only (`KO_REGISTRY` required, no quay).

Change across minors (verified via values.yaml/make at tags):
- <= v1.13.x: 5 images but the 5th is `quay.io/jetstack/cert-manager-ctl` (CM@v1.13.6 values.yaml:687; containers.mk:52 lists `cert-manager-ctl-linux`).
- v1.14.x: BOTH `cert-manager-ctl` (containers.mk:58 lists ctl + startupapicheck) and new `cert-manager-startupapicheck` (values.yaml:1225 at v1.14.7; first appears v1.14.0-alpha.0 values.yaml:733). Release notes 1.14: "startupapicheck job uses a new OCI image called 'startupapicheck', instead of the ctl image" (WEB/releases/release-notes/release-notes-1.14.md:178,273).
- >= v1.15.0: `cert-manager-ctl` no longer published ("there will no further `quay.io/jetstack/cert-manager-ctl` OCI images", release-notes-1.15.md:9). Set stable through v1.21.2.
- No other image added/removed v1.15 -> v1.21. Chart repo/name keys changed in v1.20 (section 5) but resulting image names are identical.
- Docker Hub / ghcr / gcr: UNVERIFIED and no repo evidence of publication there (anonymous token probes: Docker Hub jetstack/cert-manager-controller 401, ghcr 403, gcr 401 - inconclusive). Treat quay.io as sole registry.
- quay.io itself unreachable; cross-validation done via static manifests (section 7).

## 7. Release assets (GitHub Releases)

Publisher: `cmrel publish --nomock` -> `pushGitHubRelease` creates a DRAFT release (name = tag) and uploads manifests; maintainer then pastes notes and publishes (REL/cmd/cmrel/cmd/gcb_publish.go:492-575; release-process.md step 11, checks "pre-release" for alpha/beta and "latest" for final/patch).
Assets verified by `curl -I -L` / GET on `https://github.com/cert-manager/cert-manager/releases/download/<tag>/<asset>` (HTTP 200) :
- `cert-manager.yaml` and `cert-manager.crds.yaml`: 200 for v1.21.2, v1.21.1, v1.21.0, v1.21.0-beta.0, v1.20.4, v1.20.0, v1.19.6, v1.19.0, v1.18.6, v1.18.0, v1.17.4, v1.17.0, v1.16.5, v1.16.0, v1.15.5, v1.14.7, v1.13.6, v1.12.17 (all 18 downloaded, sizes ~0.4-1.0 MB; pre-release `v1.21.0-beta.0` carries them too).
- Binaries: `cmctl-<os>-<arch>.tar.gz|.zip` and `kubectl-cert_manager-<os>-<arch>.tar.gz` exist ONLY for <= v1.14.x (v1.14.7: linux-amd64/arm64/arm/s390x/ppc64le, darwin-amd64/arm64 200; windows-amd64.zip 200; windows-arm64 404). For v1.15.0, v1.15.5, v1.21.2 all cmctl/kubectl-cert_manager probes 404. Rule in REL/pkg/release/platforms.go:178 `CmctlIsShipped(v) = v < 1.15.0-alpha.0`. CLI moved to https://github.com/cert-manager/cmctl (tags v2.x, latest seen v2.6.1) - separate product, out of scope; RELEASE.md:17 confirms.
- Probed 404 (not attached): `cert-manager-manifests.tar.gz`, `cert-manager-<tag>.tgz`, `.tgz.prov`, `SHA256SUMS`, `metadata.json`, `cert-manager.yaml.sha256` (v1.14.7, v1.15.0, v1.15.5, v1.21.2). No per-asset checksum/signature on the GitHub release (UNVERIFIED for assets I did not probe, e.g. `.sig`).
- So current asset list (v1.15+): exactly `cert-manager.yaml`, `cert-manager.crds.yaml` (full list UNVERIFIED because Releases API is blocked; derived from cmrel code + probes). Built by make/manifests.mk:34 `static-manifests: $(bin_dir)/yaml/cert-manager.crds.yaml $(bin_dir)/yaml/cert-manager.yaml` using `helm template --kube-version="1.29.0" ... --set="creator=static" --set="startupapicheck.enabled=false"` (manifests.mk:136,141).

Static-manifest image lines (`grep -nE '^\s*(- )?image:|--acme-http01-solver-image'` on downloaded `cert-manager.yaml`):
```
v1.21.2  (https://github.com/cert-manager/cert-manager/releases/download/v1.21.2/cert-manager.yaml)
13675:  image: "quay.io/jetstack/cert-manager-cainjector:v1.21.2"
13739:  image: "quay.io/jetstack/cert-manager-controller:v1.21.2"
13745:  - --acme-http01-solver-image=quay.io/jetstack/cert-manager-acmesolver:v1.21.2
13822:  image: "quay.io/jetstack/cert-manager-webhook:v1.21.2"
```
Same 4 lines (tag = release tag, `v`-prefixed) in ALL 18 manifests, including v1.21.0-beta.0 (`...:v1.21.0-beta.0`), v1.14.7, v1.13.6, v1.12.17. `startupapicheck` / `cert-manager-ctl` strings: 0 occurrences (job disabled in static manifest), so the startupapicheck image is only derivable from the chart values or release tooling, not from `cert-manager.yaml`. Static manifests also carry `app.kubernetes.io/version: "v1.21.2"` labels and 6 CRDs.

## 8. CRDs

- In-repo (source/reference only per `deploy/crds/README.md`: "for reference, development and testing purposes only"): 
  - >= v1.19.0-alpha.0: `deploy/crds/{cert-manager.io_certificates,cert-manager.io_certificaterequests,cert-manager.io_issuers,cert-manager.io_clusterissuers,acme.cert-manager.io_orders,acme.cert-manager.io_challenges}.yaml` (200 at v1.19.6, v1.20.4, v1.21.2).
  - <= v1.18.x: `deploy/crds/crd-{certificates,certificaterequests,issuers,clusterissuers,orders,challenges}.yaml` (200 at v1.16.5, v1.17.4, v1.18.6; 404 at v1.19.0-alpha.0).
  - Helm templates: `deploy/charts/cert-manager/templates/crd-<group>_<plural>.yaml` gated by `{{- if or .Values.crds.enabled .Values.installCRDs }}` (`crds.keep` annotation).
- Release asset (end-user form): `https://github.com/cert-manager/cert-manager/releases/download/<tag>/cert-manager.crds.yaml` (documented in WEB/installation/upgrade.md:58 `kubectl apply -f .../<version>/cert-manager.crds.yaml`). Verified in all 10 asset sets parsed (v1.21.2, 1.20.4, 1.19.6, 1.18.6, 1.17.4, 1.16.5, 1.15.5, 1.14.7, 1.13.6, 1.12.17): exactly 6 CRDs, every one `versions: [v1]` with `served: true, storage: true`, none deprecated/unserved.
- The 6 CRDs: `certificates.cert-manager.io`, `certificaterequests.cert-manager.io`, `issuers.cert-manager.io`, `clusterissuers.cert-manager.io` (Cluster-scoped), `orders.acme.cert-manager.io`, `challenges.acme.cert-manager.io`. API groups: `cert-manager.io/v1`, `acme.cert-manager.io/v1`. (Earlier alpha2/alpha3/beta1 versions are gone from 1.12+ assets.)
- CRD labels in asset: `app.kubernetes.io/version: <tag>` (>= 1.19 also `app.kubernetes.io/component: crds`).

## 9. Security

- SECURITY.md: `CM@<tag>/SECURITY.md` (200 at master, v1.21.2, v1.18.6, v1.16.5) is a 1-line stub: "Please refer to the cert-manager organisation security document" -> https://github.com/cert-manager/community/blob/main/SECURITY.md (raw 200). Also `SECURITY_CONTACTS.md` (same stub). Org policy: report by email to `cert-manager-security@googlegroups.com`; acknowledgement target within 3 working days; vulnerability-scanner dumps explicitly NOT accepted; fixes back-ported to last two releases with an immediate patch release (WEB/releases/README.md "Security issues"). Website `SECURITY.md` and WEB/contributing/security.md point to the same file.
- Advisory source: GitHub Security Advisories of the repo: `https://github.com/cert-manager/cert-manager/security/advisories/GHSA-xxxx-xxxx-xxxx` (pattern cited in release notes; HTML + REST API blocked here -> UNVERIFIED live). Also global DB links `https://github.com/advisories/GHSA-...`. Attempt to fetch GHSA JSON from github/advisory-database via raw (GHSA-8rvj-mm4h-c258 under 2026/05-09) returned 404; osv.dev / vuln.go.dev blocked.
- How CVE/GHSA fixes appear in release notes (WEB/releases/release-notes):
  - Patch summary prose names the advisory: v1.19.6 / v1.20.3: "fixes a security issue ([`GHSA-8rvj-mm4h-c258`](https://github.com/cert-manager/cert-manager/security/advisories/GHSA-8rvj-mm4h-c258), HIGH) where the default `cert-manager-edit` aggregate ClusterRole granted namespace users permission to create ACME Challenge and Order resources..." (release-notes-1.19.md:77, 1.20.md:52); also noted in 1.21 Major Themes and upgrading-1.19-1.20.md, upgrading-1.18-1.19.md.
  - Bullet prefix `Security (HIGH):` / `Security (MODERATE):` under `#### Bug or Regression`: release-notes-1.19.md:95 `- Security (HIGH): Remove Challenge create and Order create, patch, update verbs ... (GHSA-8rvj-mm4h-c258). (#8941 ...)`; release-notes-1.18.md:234 `- Security (MODERATE): Fix a potential panic in the cert-manager controller when a DNS response in an unexpected order was cached ...` for v1.18.5 which also cites `GHSA-gx3x-vq4p-mhhv`.
  - Go toolchain/dependency CVEs under `Other (Cleanup or Flake)` / `Bug or Regression`: v1.18.6 "A simple bug-fix release to address `CVE-2025-68121`" ([GHSA-h355-32pf-p2xm]) with note that `CVE-2026-24051` (GHSA-9h8m-3fm2-qjrq) was intentionally not patched (macOS only); v1.18.4 "Update Go to v1.24.11 to fix CVE-2025-61727 and CVE-2025-61729"; v1.18.3 lists 9 Go CVEs; v1.20.4 lists `CVE-2026-46600, CVE-2026-56852, CVE-2026-56854, CVE-2026-84303/84304/84445`, plus a section "Security scanners still report three golang.org/x/crypto findings" (CVE-2026-56855, CVE-2026-78662, GO-2026-5932) explaining non-exploitability; v1.21.2 "Upgrade Go to 1.26.8 ... Bump `golang.org/x/crypto` to v0.56.0 to fix reported security vulnerabilities" (no ids). Many fixes carry no CVE id ("to fix a reported security vulnerability").
  - Regex hints: `GHSA-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}`, `CVE-\d{4}-\d{4,7}`, `Security \((HIGH|MODERATE|LOW|CRITICAL)\)`, `GO-\d{4}-\d+`.
- Distinct CVE/GHSA ids per release-notes file (grep count): 1.16: 13, 1.17: 10, 1.18: 23, 1.19: 22, 1.20: 19, 1.21: 1.

## 10. Historical validation matrix

Legend: HELD / PARTIAL / FAILED.

| Relationship | Tested on | Result |
|---|---|---|
| Tag regex `^v\d+\.\d+\.\d+(-(alpha\|beta)\.\d+)?$` | all 262 non-`cmd/ctl` tags | HELD for 261/262; sole outlier is legacy `v0.15-alpha.3` (no patch number). `cmd/ctl/v*` prefix tags must be excluded |
| Release-notes path `release-notes-{{.Major}}.{{.Minor}}.md` | 1.0-1.21 (all files exist); 1.13-1.21 fetched 200 | HELD |
| Per-patch heading `## \`?vX.Y.Z\`?` set == git stable-tag set | diffed headings vs tags for 1.5 ... 1.21 (17 minors, incl. 1.16.x, 1.17.x, 1.18.x, 1.19.x, 1.20.x, 1.21.x) | HELD exactly; FAILED for 1.0-1.4 (different heading style). Backtick style varies (1.20 & <=1.11 no backticks) |
| Category headings Feature / Bug or Regression / Other (Cleanup or Flake) | 1.12-1.21 | PARTIAL: present everywhere but extra/legacy names (Bug Fixes, Bugfixes, Other, Uncategorized, Design, Security, Breaking Changes) and variable level (###/####) |
| `{/* BEGIN changelog vX */}` markers | 1.14-1.21 | FAILED as a general rule (absent <=1.18; partial in 1.19/1.20; full only 1.21) |
| Upgrade doc `upgrading-{{prev}}-{{cur}}.md` | 1.13-1.14 ... 1.20-1.21 (8 transitions fetched 200; 1.0-1.1 ... all present on disk) | HELD except 1.12->1.13 (`upgrading-1.12.md` instead) |
| Releases README K8s table headers | page snapshots @ commits 41c2b78 (2025-06-10, supported 1.18/1.17), 93a7ed5 (2025-10-07, 1.19/1.18), 64342e0 (2026-03-11, 1.20/1.19) and HEAD | HELD: same 5 column headers in "Currently supported" table every time; row cell format varies slightly ("1.29 → 1.33   /   4.16 → 4.18" extra spaces in 1.18/1.17 rows) |
| Release date in README == annotated tag date of `vX.Y.0` | 1.15 (Jun 05 2024), 1.16 (Oct 03 2024), 1.17 (Feb 03 2025), 1.18 (Jun 10 2025), 1.19 (Oct 07 2025), 1.20 (Mar 10 2026), 1.21 (Jul 08 2026) | HELD for 1.15-1.21; 1.14.0: README Feb 03 2024 vs tag 2024-01-31 (3-day mismatch) |
| Chart path `deploy/charts/cert-manager/` + `values.yaml` + `values.schema.json` | v1.13.6, v1.14.7, v1.15.5, v1.16.5, v1.17.4, v1.18.6, v1.19.6, v1.20.4, v1.21.2 | HELD (schema file confirmed 200 for 1.16-1.21) |
| Chart file name `Chart.template.yaml` with `version: v0.0.0`/`appVersion: v0.0.0` | v1.13.6 ... v1.20.4 | HELD; FAILED from v1.21.0-alpha.1 (renamed `Chart.yaml`, same placeholders) |
| Chart `kubeVersion` constant `>= 1.22.0-0` | 9 tags above | HELD (but does not track support matrix) |
| values image keys `image.repository: quay.io/jetstack/cert-manager-controller` | v1.13.6-v1.19.6 | HELD; FAILED from v1.20.0-beta.0 (repository empty; `imageRegistry`+`imageNamespace`+`image.name`) |
| CRDs in release asset `cert-manager.crds.yaml` (6 CRDs, v1 served+storage) | 10 releases v1.12.17-v1.21.2 | HELD |
| CRD in-repo path | v1.16.5/1.17.4/1.18.6 (`crd-*.yaml`), v1.19.6/1.20.4/1.21.2 (`<group>_<plural>.yaml`) | PARTIAL: file naming changed at v1.19.0-alpha.0 |
| Install manifest `cert-manager.yaml` images `quay.io/jetstack/cert-manager-{cainjector,controller,webhook}:<tag>` + acmesolver arg | 18 tags v1.12.17-v1.21.2 incl. one beta | HELD (tag text == git tag); startupapicheck never present |
| Image set | v1.13.6 (ctl), v1.14.7 (ctl+startupapicheck), v1.15.5-v1.21.2 (startupapicheck only) | CHANGED at 1.14/1.15 as described in section 6 |
| `cmctl` assets on GitHub release | v1.14.7 present; v1.15.0/1.15.5/1.21.2 absent | HELD as `<1.15.0-alpha.0` rule |
| `cert-manager.yaml`/`cert-manager.crds.yaml` assets | 18 tags | HELD |
| No CHANGELOG.md in repo | master, v1.21.2, v1.18.6, v1.16.5 | HELD |
| `variables.json cert_manager_latest_version` == highest stable tag | HEAD only | HELD (v1.21.2); historical not tested |

## UNVERIFIED / blocked list
- Live existence of any quay.io image/OCI chart tag, charts.jetstack.io index, ArtifactHub, GitHub Releases body/asset list (only probed named assets), GitHub Security Advisories list (api/HTML blocked), cert-manager.io public URLs, GHSA JSON via advisory-database (404 on guessed paths), Docker Hub/ghcr/gcr mirrors.
- Whether older (<=1.11) releases follow the same asset/image rules (not tested beyond v1.12.17).

---

## Suggested declarative definition hints

```yaml
product: cert-manager
upstream_repo: github.com/cert-manager/cert-manager
default_branch: master
docs_repo: github.com/cert-manager/website      # default branch: master

versions:
  tag_regex: '^v(?P<major>\d+)\.(?P<minor>\d+)\.(?P<patch>\d+)(?:-(?P<pre>(?:alpha|beta)\.\d+))?$'   # exclude 'cmd/ctl/v*'
  tag_template: 'v{{.Major}}.{{.Minor}}.{{.Patch}}'                  # {{.Tag}}; {{.Version}} = no v prefix
  prerelease_markers: [alpha, beta]            # no rc
  stable_rule: 'no hyphen in tag'
  ordering: semver (NOT chronological: v1.20.4 tagged after v1.21.2)
  release_branch: 'release-{{.Major}}.{{.Minor}}'
  latest_cross_check: 'https://raw.githubusercontent.com/cert-manager/website/master/content/docs/variables.json  -> .cert_manager_latest_version'
  support_policy: 'last two minors; EOL at release of 2nd subsequent minor'

release_notes:
  url: 'https://raw.githubusercontent.com/cert-manager/website/master/content/docs/releases/release-notes/release-notes-{{.Major}}.{{.Minor}}.md'
  scope: per-minor file, patches as sections
  patch_section_heading_regex: '^##\s+`?v(\d+\.\d+\.\d+)`?\s*$'     # valid for minors >= 1.5
  kind_headings: ['Feature','Documentation','Bug or Regression','Other (Cleanup or Flake)']   # legacy: Bug Fixes, Other, Uncategorized, Design, Security
  breaking_marker: '> ⚠️ Breaking change' / '> ⚠️ Potentially breaking change'   # in Major Themes
  security_regex: 'Security \((HIGH|MODERATE|LOW|CRITICAL)\)|GHSA-[a-z0-9]{4}-[a-z0-9]{4}-[a-z0-9]{4}|CVE-\d{4}-\d{4,7}'
  changelog_in_main_repo: none
upgrade_doc:
  url: 'https://raw.githubusercontent.com/cert-manager/website/master/content/docs/releases/upgrading/upgrading-{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md'
  exceptions: ['upgrading-1.12.md  (instead of 1.12-1.13)']
  exists_from: final X.Y.0

compatibility:
  url: 'https://raw.githubusercontent.com/cert-manager/website/master/content/docs/releases/README.md'
  tables:
    - section: '## Currently supported releases' (and '## Upcoming releases')
      columns: ['Release','Release Date','End of Life','Supported Kubernetes / OpenShift Versions','Tested Kubernetes Versions']
    - section: '## Old cert-manager releases'
      columns: ['Release','Release Date','EOL','Compatible Kubernetes versions','Compatible OpenShift versions']
  cell_format: 'K8sMin → K8sMax [/ OCPMin → OCPMax]'   # U+2192; release col is '[1.21][]'
  chart_kubeVersion: constant '>= 1.22.0-0' (do not use as support signal)

helm_chart:
  source_dir: deploy/charts/cert-manager
  chart_file_candidates: ['Chart.yaml' (>= v1.21.0-alpha.1), 'Chart.template.yaml' (<= v1.21.0-alpha.0)]    # placeholders v0.0.0 filled at build
  values: 'deploy/charts/cert-manager/values.yaml'; schema: 'deploy/charts/cert-manager/values.schema.json'
  chart_name: cert-manager
  chart_version == app_version == git tag with v prefix
  oci: 'oci://quay.io/jetstack/charts/cert-manager'      # tags: 'v{{.Version}}' and '{{.Version}}'
  http_repo: 'https://charts.jetstack.io'                # chart 'cert-manager', version 'v{{.Version}}'; lags OCI by hours
  not_on_github_release: true

images:
  registry: quay.io
  namespace: jetstack
  tag: '{{.Tag}}'        # v-prefixed, equals git tag, incl. alpha/beta
  repositories:          # valid v1.15.0 .. v1.21.2
    - quay.io/jetstack/cert-manager-controller
    - quay.io/jetstack/cert-manager-webhook
    - quay.io/jetstack/cert-manager-cainjector
    - quay.io/jetstack/cert-manager-acmesolver
    - quay.io/jetstack/cert-manager-startupapicheck
  legacy: ['quay.io/jetstack/cert-manager-ctl  (<=1.14; replaced by startupapicheck in 1.14, last published 1.14)']
  per_arch_tags: '<repo>-{amd64|arm64|arm|s390x|ppc64le}:{{.Tag}}' (internal; public tag is multi-arch list)
  cross_validate_with: 'cert-manager.yaml image: lines (controller, webhook, cainjector + acmesolver arg; no startupapicheck)'

release_assets:
  base: 'https://github.com/cert-manager/cert-manager/releases/download/{{.Tag}}/'
  files: ['cert-manager.yaml','cert-manager.crds.yaml']          # v1.15+
  legacy_files_le_1_14: ['cmctl-{os}-{arch}.tar.gz|zip','kubectl-cert_manager-{os}-{arch}.tar.gz']
  image_extraction: grep -E '^\s*image:|--acme-http01-solver-image=' in cert-manager.yaml
crds:
  asset: cert-manager.crds.yaml  (6 CRDs, only v1 served+storage)
  in_repo: 'deploy/crds/<group>_<plural>.yaml (>=v1.19.0-alpha.0); deploy/crds/crd-<plural>.yaml (<=v1.18.x)'
security:
  policy: 'https://raw.githubusercontent.com/cert-manager/community/main/SECURITY.md'
  advisories: 'https://github.com/cert-manager/cert-manager/security/advisories'   # blocked in sandbox; GHSA ids also appear in release notes
```
