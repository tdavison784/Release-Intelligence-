# HashiCorp Vault release-channel map (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** helm.releases.hashicorp.com (366 chart versions), developer.hashicorp.com, www.vaultproject.io, discuss.hashicorp.com, checkpoint-api.hashicorp.com, endoflife.date, registry.terraform.io, go.hashi.co and releases.hashicorp.com answer; api.github.com shows 0 repository advisories. The helm-repo channel (declared first) validates for 7 releases; `ghcr.io` token → 403 as before (no Vault image published there). The original text below is kept as the record of 2026-10-01.

Scope: product `vault` (github.com/hashicorp/vault, "UP"), chart repository github.com/hashicorp/vault-helm ("HELM"), docs content repository github.com/hashicorp/web-unified-docs ("DOCS").
Method: `git ls-remote`, blobless clones, raw.githubusercontent.com (HTTP 200 checked), releases.hashicorp.com, Docker Hub tag API and registry manifests, `HEAD` probes. Snapshots: vault tags up to v2.1.1 (2026-09-16), DOCS `main`, HELM tags up to v0.34.1.

Abbreviations: `RAW` = https://raw.githubusercontent.com/hashicorp/vault, `DOCS-RAW` = https://raw.githubusercontent.com/hashicorp/web-unified-docs/main, `REL` = https://releases.hashicorp.com/vault.

Reachable from the sandbox: github.com (git), raw.githubusercontent.com, releases.hashicorp.com, hub.docker.com, auth.docker.io + registry-1.docker.io (manifests), public.ecr.aws (anonymous token + manifests), apt.releases.hashicorp.com, rpm.releases.hashicorp.com.
Blocked (proxy 403/000, not retried): api.github.com (also the GitHub MCP tools, scoped to another repo), helm.releases.hashicorp.com, developer.hashicorp.com, www.vaultproject.io, discuss.hashicorp.com (HCSEC bulletins), checkpoint-api.hashicorp.com, endoflife.date, registry.terraform.io, ghcr.io token endpoint (403), www.hashicorp.com/trust (429).

## 0. What a definition has to know (the surprises)

1. **The product is now 2.x.** Latest stable `v2.1.1` (2026-09-16); 2.0.0 shipped 2026-04-14. The community line 1.x ended at `v1.21.4`; `1.21.5 ... 1.21.11` exist only as Enterprise releases (no OSS tag, no Docker Hub OSS tag, no OSS zip).
2. **CHANGELOG.md is only current on `main`.** At tag `v2.1.1` it stops at `1.21.4` (byte-identical to `v2.0.0`'s). Every release's notes must be read from `main`.
3. **Changelog groups are not markdown headings.** `SECURITY:`, `CHANGES:`, `FEATURES:`, `IMPROVEMENTS:`, `BUG FIXES:`, `DEPRECATIONS:`, `BREAKING CHANGES:` are plain paragraphs followed by `*` bullets. Without help every bullet is "other".
4. **Three changelog files, four heading dialects** (section 2).
5. **Docs are versioned by directory in another repository and the layout changed twice** (section 3), and the 2.x docs live in `v2.x`, not `v2.0.x`.
6. **Chart is in a different repository with independent 0.x versions** and only a handful of Vault releases are ever an `appVersion` (section 5).
7. **Enterprise is a parallel release train with the same numbers plus a suffix** (`+ent`, `-ent`) and extra flavours (`+ent.hsm`, `+ent.fips1402/1403`).
8. **Licence**: MPL-2.0 until `v1.14.8`; BUSL-1.1 from `v1.14.9` (section 9).

## 1. Canonical versions

Source: `git ls-remote --tags https://github.com/hashicorp/vault` = 486 tag refs.
- Stable `vX.Y.Z`: 244 (0.1.0 ... 2.1.1; 199 are >= 1.0.0). Pre-releases: 13 `-betaN`, 34 `-rcN`. No `-alpha`.
- Junk a matcher must ignore (195): 174 Go sub-module tags `api/...`, `sdk/...`, `api/auth/*/vX`; 4 enterprise tags `v1.14.11+ent`, `v1.14.12+ent`, `v1.15.7+ent`, `v1.15.8+ent`; 6 `ent-changelog-1.14.11 ...`; `v0.2.0.rc1`, `v0.3.0-rc`, `v0.5.0-rc1.1`, `v0.5.0-rc1.2`, `v0.6.0-rebuild`, `v1.5.0-rc`, `v1.6.0-rc`, `last-go-modable`, `main-creation`, `old-stable-website*`.
- Regex that equals the stable set: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:beta|rc)\d+)?)$`. `ri versions vault` -> 244 releases in 35 lines.
- Lines: 2.1 (2.1.0-2.1.1), 2.0 (2.0.0-2.0.4), 1.21 (1.21.0-1.21.4, last OSS), 1.20 (to 1.20.4), 1.19 (to 1.19.5), 1.18 (to 1.18.5), 1.17 (to 1.17.6), 1.16 (to 1.16.3), ... 1.10 (to 1.10.11).
- Mirror: `REL/index.json` (HTTP 200, 2.6 MB) lists 1347 versions: 244 plain, 1064 with `+ent...` suffixes (`+ent`, `+ent.hsm`, `+ent.fips1402`, `+ent.fips1403`, `+ent.hsm.fips140x`). It is the only enumeration of Enterprise-only releases (e.g. `1.21.11+ent`). Each version has 9 builds (darwin/freebsd/linux/netbsd/openbsd/solaris/windows) plus `vault_<v>_SHA256SUMS`, `.sig`, `.72D7468F.sig`.
- GitHub Releases: not verifiable (API blocked); not declared.
- Lineage: `minor` (X.Y.0 carries the delta from the previous line; X.Y.z patches in parallel).

## 2. Release notes

### CHANGELOG (the canonical notes)
- `RAW/main/CHANGELOG.md` (479 KB): sections `## 2.1.1` ... `## 1.16.0` (129 headings). `## Previous versions` links `CHANGELOG-v1.10-v1.15.md` (`## 1.15.16 Enterprise` ... `## 1.10.0`) and `CHANGELOG-pre-v1.10.md` (`## 1.9.10` ... `## 1.0.0`, `CHANGELOG-v0.md` for 0.x).
- File at tags is frozen: `RAW/v2.1.1/CHANGELOG.md` = 350 260 bytes, top section `## 1.21.4`. **Use `ref: main`.**
- Section anatomy (2.x): `## 2.1.1`, `### September 16, 2026`, then label paragraphs. Labels seen (counts over main): BUG FIXES 125, CHANGES 105, IMPROVEMENTS 93, SECURITY 84, FEATURES 23, BREAKING CHANGES 16, DEPRECATIONS 4. Bullets are `* area: text` (older ones end with `[[GH-1234](pull-url)]`). Some labels have trailing spaces.
- Heading dialects: (a) `## 2.1.1` + H3 date (124 sections, 1.16-2.x and 1.10-1.15); (b) `## 1.19.16  Enterprise` (Enterprise-only releases, also trailing spaces `## 1.16.3 `); (c) `## 1.4.2 (May 21st, 2020)` and `## 1.1.4/1.1.5 (July 25th/30th, 2019)` in the pre-1.10 file; (d) **two defective sections**: `## 1.18.0 ` and `## 1.17.5` are followed by a second H2 date heading (`## October 9, 2024`), so the version section is empty and the notes sit under the sibling heading.
- Coverage of OSS tags by a version-heading selector: all of 1.0.0-2.1.1 except 1.3.0 (no section) and the two defective ones. Enterprise-only headings (`1.21.5 Enterprise` ... `1.21.11 Enterprise`, `1.19.6 Enterprise` ... ) never match an OSS tag.
- `main` also holds `changelog/*.txt` entry files (unreleased go-changelog format); not used.
- Notable lifecycle lines: `2.1.0 CHANGES: License: Add Agentic IAM terms to client licensing model ...`; `2.0.4 BREAKING CHANGES: containers: gnupg, openssl, procps removed from UBI images`.

### Docs-site release notes (not used)
`DOCS-RAW/content/vault/v1.21.x/content/docs/updates/release-notes.mdx` and `.../v2.x/.../release-notes.mdx`: per-docs-line page, `## Vault 2.1.1` sections with `### New features / Bug fixes and security patches / Improvements and behavior changes`, plus `## Summary` and `## Feature deprecations and EOL` (2.x). Same content as the changelog with coarser groups ("Bug fixes and security patches" would classify every fix as security). <= 1.18 used one file per minor: `docs/release-notes/1.18.0.mdx`.

## 3. Upgrade / migration docs (DOCS repo)

`DOCS-RAW/content/vault/<dir>/content/...`; `<dir>` = `v1.4.x` ... `v1.21.x`, **`v2.x`**, plus `global/` (shared partials). 24 040 files under content/vault.
| Docs line | File | Shape |
|---|---|---|
| 1.10-1.18 | `docs/upgrading/upgrade-to-X.Y.x.mdx` (one per minor; also `upgrade-to-1.5.0` ... `1.9.x` inside `v1.10.x`) | `## Overview`, `### Breaking changes`, `### Important changes` with `####` items, `### Known issues and workarounds` |
| 1.19-1.21 | `docs/updates/important-changes.mdx` in `vX.Y.x` | `## Breaking changes`, `## New behavior`, `## Known issues`; each item `### Title ((#anchor))` + one-row table `Change | Affected version | Vault edition` (known issues: `Affected version | Fixed version`) |
| 2.x | `v2.x/content/docs/updates/important-changes.mdx` | same, cumulative for the whole 2.x docs line (items say `2.0.0+`, `2.0.0 - 2.1.1`) |
Also: `docs/updates/{release-notes,deprecation,change-tracker,lts-tracker}.mdx`, `docs/upgrade/index.mdx` (generic procedure, no version content), `docs/enterprise/lts.mdx` (LTS 1.19.x, extended maintenance 1.16.x on the 1.21 docs).
Dialect: MDX. `@include '../../../global/partials/important-changes/...mdx'` pulls items from other files (resolved relative to the line's `content/partials` directory); in `v2.x/important-changes.mdx` 4 of 7 breaking items, 1 of 5 new-behavior items and 12 of 14 known issues are includes. Headings carry `((#anchor))` and `<EnterpriseAlert inline="true" />`.
Verified present (HTTP 200): upgrade-to-1.18.x, important-changes for v1.19.x/v1.20.x/v1.21.x/v2.x, release-notes for v1.21.x/v2.x.

## 4. Compatibility

- Kubernetes: `DOCS-RAW/content/vault/v2.x/content/partials/kubernetes-supported-versions.mdx` is a **bullet list** ("* 1.36 ... * 1.32", "IBM tests and verifies Vault against ..."), not a table. Chart `Chart.yaml` has `kubeVersion: ">= 1.20.0-0"` (a floor, not the matrix). vault-helm CHANGELOG states "Tested with Kubernetes versions v1.36-1.32" and "Tested with Vault v2.0.4, v1.21.9, v1.20.14, v1.19.20".
- Browser/storage matrices exist (`ent-supported-storage.mdx`, `browser-support.mdx`) but are product-internal tables, not platform constraints.
- Lifecycle: LTS tracker (`Current LTS 1.19.x | Extended maintenance 1.16.x`), support policy link `go.hashi.co/vault-support-policy` (blocked).

## 5. Helm chart (separate repository)

- `HELM` = github.com/hashicorp/vault-helm, default branch `main`, **Chart.yaml and values.yaml at the repository root**, tags `vX.Y.Z` (52 tags, v0.1.0 ... v0.34.1). Published to `https://helm.releases.hashicorp.com` (chart `vault`): blocked. No OCI copy found (Docker Hub `hashicorp/vault-helm` 404, no gh-pages branch).
- Reachable alternative: `helm-git` over the tags + `Chart.yaml` at the tag (verified: 52 versions, `appVersion` present from v0.6.0).
- Relationship: `Chart.yaml: version: 0.34.1`, `appVersion: 2.0.4` = the Vault version the chart defaults to (`server.image.tag: "2.0.4"`). Sparse: v0.30.0 -> 1.19.0, v0.30.1 -> 1.20.1, v0.31.0 -> 1.20.4, v0.32.0 -> 1.21.2, v0.33.0 -> 2.0.2, v0.34.0 -> 2.0.3, v0.34.1 -> 2.0.4; **no chart ships appVersion 2.1.0 or 2.1.1**. Mapping v0.22.1 -> 1.12.0, v0.25.0 -> 1.14.0 etc. Strategy `lookup` on `appVersion`, `optional: true` (as for argo-helm).
- `values.yaml` also pins the companion images: `injector.image.tag` (vault-k8s 1.7.6), `csi.image.tag` (vault-csi-provider 1.7.4), `server.image.tag`, `injector.agentImage.tag`, `csi.agent.image.tag`; they surface in `ri upgrade` as helm-values default changes.
- vault-helm has its own CHANGELOG.md (`## 0.34.1 (August 13, 2026)`, title-case labels `Changes:`, `Features:`, `Bug Fixes:`); not used for Vault upgrades.

## 6. Images and packages

| Artifact | Name | Version rule | Verified |
|---|---|---|---|
| OSS image | `docker.io/hashicorp/vault` (also `public.ecr.aws/hashicorp/vault`) | tag = `X.Y.Z` (no `v`), plus `X.Y`, `latest`, `X.Y.Z-rcN` | Docker Hub has every release from 1.5.2; 29 releases 1.0.0-1.5.1 absent; 211 tags |
| Enterprise image | `docker.io/hashicorp/vault-enterprise` | `X.Y.Z-ent` (also `X.Y-ent`) | from 1.9.1; `1.16.0-ent` missing (anomaly) |
| zip | `REL/X.Y.Z/vault_X.Y.Z_linux_amd64.zip` | `X.Y.Z` | 199/199 stable >= 1.0.0, HEAD 200 |
| Enterprise zip | `REL/X.Y.Z+ent/vault_X.Y.Z+ent_linux_amd64.zip` | `X.Y.Z+ent` | from 1.2.3 (13 earlier missing) |
| checksums | `REL/X.Y.Z/vault_X.Y.Z_SHA256SUMS` (+ `.sig`) | `X.Y.Z` | all |
| rpm | `https://rpm.releases.hashicorp.com/RHEL/9/x86_64/stable/vault-X.Y.Z-1.x86_64.rpm` | `X.Y.Z-1` | contiguous from 1.10.1; 1.10.0 and 31 earlier releases 404 |
| deb | `apt.releases.hashicorp.com` pool path guesses 404 (index reachable); not declared | | |
Docker Hub tag listing shows images are multi-arch manifest lists (amd64, arm64, 386). Blob download is not needed: manifests are enough for existence.

## 7. Release assets on GitHub

Not verifiable (API blocked); binaries are distributed on releases.hashicorp.com instead.

## 8. CRDs / manifests

None in the product. (Vault Secrets Operator / vault-k8s are separate products with their own CRDs and versions.)

## 9. Security and licence

- Advisories: HCSEC bulletins at discuss.hashicorp.com (blocked, not machine readable). Changelog `SECURITY:` bullets carry the fixes, often with `CVE-...`, `GHSA-...`, `GO-2026-...` ids and `[HCSEC-2024-21](https://discuss...)` links. GitHub repository advisories (`github-advisories`) are declared but unreachable here.
- Licence: `RAW/v1.14.8/LICENSE` = Mozilla Public License 2.0 (15 958 bytes); `RAW/v1.14.9/LICENSE` = Business Source License 1.1 ("Licensed Work: Vault Version 1.14.9 or later", 4 919 bytes; `v1.15.0` file still says "Vault 1.15.0-rc1"). So the relicensing is **a release-level fact at 1.14.9/1.15.0**, announced nowhere in the changelog. `2.1.0` adds "Agentic IAM terms to the client licensing model".

## 10. Historical validation (`ri check vault`, 32 releases 1.0.0 ... 2.1.1)

Final run: 15 subjects, 14 validated, 1 insufficient (`important-changes-v2`: 2.0.0 is the only major in 2.x), 0 failing, 0 unverifiable. What the first runs showed (before the definition was corrected):
- default 6 releases (1.21.0-2.1.1): 0 failures (all subjects pass, including an *empty* changelog section for 1.18.0: a check pass does not imply content).
- 28 releases (`-n 14`): `rpm-package @ 1.10.0` absent (404) -> `availability >= 1.10.1`.
- sweeping back to 1.0.0: `image` absent for 1.0.0/1.1.4/1.3.0 (Docker Hub starts at 1.5.2), `rpm-package` absent for 1.0.0/1.1.4/1.9.0 (and 27 others) -> availability windows.
- Offline scripts over all 199 stable releases: 25 releases had no changelog section with the plain heading (dates in headings <= 1.4), 2 had empty sections (1.17.5, 1.18.0), `1.3.0` has none.

## UNVERIFIED / blocked list

api.github.com (GitHub releases, advisories), helm.releases.hashicorp.com (chart index), developer.hashicorp.com (rendered docs), discuss.hashicorp.com (HCSEC), registry.terraform.io, endoflife.date, checkpoint-api.hashicorp.com, support policy page.

## Suggested declarative definition hints

- `versions`: `git-tags` with the strict pattern; tags carry the OSS train only.
- `release-notes`: three `repo-file` sources on `ref: main` (>= 1.16, 1.10-1.15, < 1.10) with `markdown-section` + `labelParagraphs: '^[A-Z][A-Z0-9 /&-]*:$'`; product rules: IMPROVEMENTS -> feature, BREAKING CHANGES -> breaking + action, `License:` -> action, plugin/Go bumps -> dependency; exceptions for 1.17.5 and 1.18.0.
- `upgrade-guide`: three docs-era sources split by `availability` (upgrade-to-X.Y.x / important-changes per line / `v2.x` for the major), `releaseKinds` to avoid repeating the cumulative 2.x page.
- artifacts: zip, checksums, OSS image (two registries), rpm, Enterprise zip + image (suffix templates), chart via `lookup(appVersion)` with `helm-git` as the reachable channel.
