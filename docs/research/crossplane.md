# Crossplane release channel map (verified 2026-10-01)

Scope: product `crossplane` (github.com/crossplane/crossplane, "core"), docs (github.com/crossplane/docs), release process (github.com/crossplane/release), CLI (github.com/crossplane/cli, since v2.3). Out of scope: providers, functions, configurations and `crossplane-runtime` (library, versioned in lockstep with core but not shipped to operators); they are released by their own repositories (`docs` guides/extensions-release-process.md).
Method: `git ls-remote` / partial clones at tags, raw.githubusercontent.com, S3 bucket listings and HEAD requests, Docker Hub and ghcr.io manifest API, Docker Hub tag API.
Snapshots read: core tags v2.4.2 (2026-09-22) / v2.3.0 / v2.0.0 / v1.20.13 / v1.14.0 / v1.10.0 / v1.2.0; docs `master` plus the `v2.0-archive` and `v2.1-archive` tags; release repo `main`; cli repo `main`.

Blocked from the sandbox (proxy 403 on CONNECT, not retried): api.github.com, github.com HTML (releases, security advisories), charts.crossplane.io, releases.crossplane.io, cli.crossplane.io, xpkg.upbound.io, xpkg.crossplane.io, docs.crossplane.io, artifacthub.io, crossplane.github.io, charts.upbound.io. The GitHub MCP tools of the session are scoped to another repository.

---

## 1. Canonical versions

- Tag scheme: `vX.Y.Z` (stable), `vX.Y.Z-rc.N` (pre-release), `v2.0.0-preview.0/1` (two preview tags before 2.0). 239 tags total; 214 match the strict pattern `^v\d+\.\d+\.\d+(-(preview|rc)\.\d+)?$` = 169 stable + 45 pre-releases (`ri versions`: "169 releases from 214 tags").
- Non-product tags to ignore (25): `apis/vX.Y.Z[-rc.N]` (Go submodule `apis/`, created by `tag.yml` at the same commit as the core tag), legacy `v0.5.0-rc` ... `v0.14.0-rc`, `v1.1.0-rc` (no dot before `rc`).
- Lines: v0.5 ... v0.14, v1.0 ... v1.20, v2.0 ... v2.4 (40 lines). The major transition: v1.20 (final v1, 1.20.0 2025-05-21, extended support, 1.20.13 on 2026-09-15) -> v2.0.0 (2025-08-08). v2.1 2025-11-05, v2.2 2026-02-17, v2.3 2026-05-21, v2.4 2026-08-20; latest stable at research time **v2.4.2** (2026-09-22). `v2.5.0-rc.0` (2026-08-13) is cut from main.
- Pre-release tags are not all published: `X.Y.0-rc.0` is cut from `main` (the build version for the next minor) and is never promoted; `X.Y.0-rc.1` is cut from `release-X.Y` and promoted to the `stable` channel as a pre-release (crossplane/release `release.md`: "Run the Promote workflow ... ticking the box for This is a pre-release"). `ri` ignores pre-releases by default.
- Branches: `release-X.Y` (e.g. `release-2.4`), plus `backport-NNNN-to-release-X.Y` PR branches.
- Cadence/support (docs `learn/release-cycle.md`): quarterly (13 weeks); the three most recent minors are maintained (nine months); `README.md` table of maintained releases: v1.20 (extended, EOL TBD), v2.1 (EOL Aug 2026), v2.2, v2.3, v2.4 (EOL May 2027), v2.5/v2.6 planned.
- The release process is documented only in github.com/crossplane/release issue templates (`release.md`, `patch_release.md`, `cli_release.md`, `cli_patch_release.md`); `RELEASE.md` in core just links there.
- Previous-tag convention for notes: the GitHub release for `vX.Y.0` is generated against `vX.<Y-1>.0` ("the first of the releases for the previous minor"), which is exactly `{{.PrevTag}}` of the catalog for X.Y.0 (and `v1.20.0` for `v2.0.0`).

## 2. Release notes

- **GitHub release bodies are the only curated notes**: hand-written for X.Y.0 ("descriptive release notes", release MVP, reviewed as draft; crossplane/release `release.md` lines 72-77), generated change lists for patches. Only reachable through the GitHub API (blocked) => content shape UNVERIFIED. Declared as the canonical notes source.
- No CHANGELOG file in core. No per-release notes in the docs repo: `content/vX.Y/whats-new/_index.md` is the **"What's New in v2?" announcement, identical in every v2.x docs line** (v2.2 == v2.3 == v2.4 == master byte for byte; v2.0 differs only by one added tip). It is a major-release document, not per-minor notes. Its `## Backward compatibility` section lists the five breaking changes of v2.
- Deterministic fallback: `git log {{.PrevTag}}..{{.Tag}}` (non-merge commits). Subjects: about half free-form ("Add Operation API type"), the rest conventional commits (`fix(pkg): ...`, `feat(pkg): ...`, `chore(deps): ...`, `ci`, `test`, `build`). No `!:` breaking markers in v1.20.0..v2.4.2. Renovate vulnerability bumps carry a literal `[security]` suffix (about 17 in v2.3.6 -> v2.4.2). Range sizes: v2.3.0..v2.4.0 = 247 commits (incl. merges), v1.20.0..v2.0.0 ~ 400 non-merge.
- Noise sources found by exercising edges: Renovate bumps of GitHub actions/lock files, CI/test/e2e subjects, review chatter ("Rabbit nitpick suggestion applied", "Re-trigger checks"), backport/cherry-pick duplicates (same subject, different SHA, e.g. two "feat(pkg): scale safe-start provider runtimes ..." in v2.3.0..v2.4.0).

## 3. Upgrade / migration docs

Docs repo layout: `content/<line>/...` where `master` holds the dev docs plus `v1.20`, `v2.2`, `v2.3`, `v2.4` (and `content/cli/...`, `contribute`); lines that left master live on tags `vX.Y-archive` (`v1.8` ... `v1.19`, `v2.0`, `v2.1`; one typo: `v1.10-architve`). Each archive tag is a snapshot of the whole docs tree at archive time, so `v2.0-archive` also contains `content/v2.0` itself.

- v2.x: `guides/upgrade-crossplane.md` (generic "helm upgrade" how-to, identical in every line; states "always upgrade one minor version at a time") and `guides/upgrade-to-crossplane-v2.md` (the v1 -> v2 guide: prerequisites "running v1.20", `## Removed features` with five `###` sections - native patch&transform, ControllerConfig, external secret stores, XR connection details, `--registry` flag - each with "Deprecated in"/"Replaced by"/migration help, then a five-step approach). It exists in v2.0 ... v2.4; only the v2.0.0 edge needs it.
- v1.x: `software/upgrade.md` (generic, no per-release content).
- Upgrade path constraints are prose only: one minor at a time, v2 only from v1.20.
- Tooling note in the guide: `crossplane beta upgrade check` (v1.20 CLI) scans for removed features.

## 4. Compatibility

- Kubernetes: docs `get-started/install.md` says only "An actively supported Kubernetes version"; the chart has no `kubeVersion`; no matrix anywhere (CI uses kind via nix). **No machine-readable Kubernetes compatibility.**
- Support window per line: README table + release-cycle docs (see section 1); no equivalent in the definition model.
- Helm >= v3.2.0 (v2 upgrade guide prerequisite).
- Package metadata of providers/functions carries `spec.crossplane.version` constraints (the inverse relation); not a property of core releases.

## 5. Helm chart

- Source: `cluster/charts/crossplane/` in core. `Chart.yaml` in the tree is a placeholder (`version: 0.0.1`, `appVersion: 0.0.1`, no kubeVersion); `values.yaml` is real since v1.12 (before: `values.yaml.tmpl` with build-time placeholders). CRDs are not in the chart (the init container of the controller installs them from the image).
- Published to the `stable` channel by `promote.yml` -> `nix/apps.nix promoteArtifacts`: `helm repo index --url https://charts.crossplane.io/stable` over the S3 bucket `crossplane-helm-charts`, fronted by CloudFront at `https://charts.crossplane.io/{stable,master}`. `master` = unreleased main builds (2732 entries like `2.5.0-rc.0.79.gab4371d05`): **not a release channel**.
- **Reachable alternative**: the bucket itself, `https://crossplane-helm-charts.s3.amazonaws.com/stable/index.yaml` (HTTP 200, 161 entries) and `.../stable/crossplane-X.Y.Z.tgz` (the index `urls` still point at charts.crossplane.io). The release bundle also has `https://crossplane-releases.s3.amazonaws.com/stable/vX.Y.Z/charts/crossplane-X.Y.Z.tgz`.
- **Version relation**: chart `version` == `appVersion` == tag without `v`, for all 161 index entries (0 mismatches, from 1.0.0 to 2.4.2, incl. `-rc.1`). 214 matching tags minus 53 without chart: all `rc.0`/preview/`0.x`/`1.0.0-rc.0` tags (never promoted), `0.1`-`0.14` (predate the repo index) and two real releases that were never promoted: **`v1.20.2` and `v2.0.3`** (tagged 2025-11-06 together with 1.20.3 / 2.0.4; absent from the chart index and from `stable/` in the release bucket).
- No OCI chart found (nothing in workflows, README or docs mentions `oci://`).

## 6. Container images

One controller image, also used by the RBAC manager deployment (`rbacManager` values use the same `image.repository`).

| Registry | Role | Reachable |
|---|---|---|
| `xpkg.crossplane.io/crossplane/crossplane` | chart default `image.repository` since **v1.20.0** | no (403) |
| `xpkg.upbound.io/crossplane/crossplane` | chart default until v1.19; `promote.yml` still pushes it | no (403) |
| `docker.io/crossplane/crossplane` | `promote.yml` "Promote Images to DockerHub"; tags `vX.Y.Z` incl. `-rc.1`, `-preview.1`; 5849 tags, mostly dev builds `v2.3.6-9.gb71028e52` | yes (manifests, tag API) |
| `ghcr.io/crossplane/crossplane` | `promote.yml` "Promote Images to GitHub Container Registry"; same release tags | yes (manifests) |

Verified (HEAD on manifests): v2.4.2, v2.4.0, v2.2.0, v2.0.0, v1.20.13, v1.20.0 and rc/preview tags exist on Docker Hub and ghcr.io; `v2.5.0-rc.0` does not (rc.0 is never promoted). Discovery rejected the ghcr.io candidate as "snapshot only" - wrong, it carries release tags. Promotion also adds the moving tags `stable`, `vX.Y.Z-stable`, `master`, `alpha`.

## 7. Release assets

- GitHub releases: assets UNVERIFIED (API blocked); nothing in the workflows uploads to GitHub releases.
- **Release bundle on S3** (`crossplane-releases` bucket, `https://releases.crossplane.io`): listing is public: `stable/vX.Y.Z/{bin/<os>_<arch>/<binary>[.sha256], charts/crossplane-X.Y.Z.tgz, images/linux_<arch>/image.tar.gz, version}`. Starts at v1.10.0; same gaps (no v1.20.2, v2.0.3). Binaries per release: `crank` (CLI) up to v2.2.x; from v2.3 the directory only holds the core binary `crossplane` (the server), no CLI.
- **CLI since v2.3.0** is released from github.com/crossplane/cli with its own tags and bucket `crossplane-cli-releases` (`https://cli.crossplane.io/<channel>/<version>/bin/<os>_<arch>/crossplane`, install script `install.sh` picks host/binary name by version). cli tags: v2.3.0 ... v2.3.4, v2.4.0, v2.4.1, v2.5.0, v2.6.0-rc.0 (v2.5.0 CLI exists although core 2.5.0 does not): **minor follows the core line, patch is independent** (core v2.3.5/v2.3.6 have no CLI v2.3.5/6). S3 URLs verified: `stable/v2.4.0/bin/linux_amd64/crossplane` and `stable/v2.4.1/...` HTTP 200. The binary was renamed `crank` -> `crossplane` at the same time.

## 8. CRDs

`cluster/crds/*.yaml` in core (generated; 21 CRDs at v2.4.2 in groups `apiextensions`, `pkg`, `ops`, `protection`; `cluster/meta/*` are package-metadata kinds, never installed). Directory appears in v1.2.0 (v1.0/1.1 differ). Mirrored in docs `content/vX.Y/api/crds/`. Not published as a separate release asset or in the chart. Exercising 1.20.13 -> 2.0.8 shows what the CRD diff buys: removed `controllerconfigs.pkg.crossplane.io` and `storeconfigs.secrets.crossplane.io`, `spec.resources[]`/`spec.patchSets[]` removed from Composition v1, new `operations.ops.crossplane.io` CRDs, new XRD API version v2.

## 9. Security

- `SECURITY.md`: private reporting through GitHub security advisories or crossplane-security@lists.cncf.io; disclosure through crossplane-security-announce, a published GitHub security advisory and "the fixed versions' release notes". Supported versions: see release-cycle docs.
- Advisories (GHSA) need the API (blocked) -> declared, unverifiable here.
- In-repo substitute: Renovate subjects ending in `[security]` and "combined security bumps" commits; marked `heuristic` by the classifier. Audit PDFs in `security/` (2023) are not release-specific.

## 10. Historical validation matrix (`ri check`, 2026-10-01)

| Subject | Result |
|---|---|
| commit-log (git log) | pass on all 27 releases checked (v1.0.0 ... v2.4.2) |
| image (docker.io fallback) | pass on all 27 |
| helm-chart (S3 index) | pass on all 27 except 1.20.2 and 2.0.3 (absent from the index: exceptions) |
| crds artifact / contents | pass from v1.2.0 |
| helm-values content | pass from v1.12.0 (older: `values.yaml.tmpl`, now `availability`) |
| crank | pass v1.10.0 ... v2.2.6 (earlier: not on the stable channel) |
| cli (X.Y.0 representative) | pass v2.3.0 ... v2.4.2 |
| whats-new / upgrade-to-v2 (archive tag) | pass on v2.0.0, the only major release in range |

## UNVERIFIED / blocked list

- GitHub release bodies (format, "Breaking Changes" conventions) and advisories.
- Existence of xpkg.crossplane.io / xpkg.upbound.io image tags (only the Docker Hub and ghcr.io mirrors were read).
- Canonical hosts charts.crossplane.io, releases.crossplane.io, cli.crossplane.io (the S3 buckets behind them were read; CloudFront cache behaviour unknown).
- docs.crossplane.io rendering (docs read from the docs repo).
- Whether xpkg.crossplane.io carries pre-1.20 tags.

## Suggested declarative definition hints

- tagPattern `^v(?P<version>\d+\.\d+\.\d+(?:-(?:preview\.\d+|rc\.\d+))?)$` (excludes `apis/` tags), lineage `minor`.
- Notes: github-releases (canonical) with `git-log {{.PrevTag}}..{{.Tag}}` as the fallback group; classify `[security]` before skipping `chore`/`ci`/`test`; do not use `whats-new` per minor (identical text).
- Docs repo: `content/v{{.Major}}.{{.Minor}}/...` on master, fallback ref `v{{.Major}}.{{.Minor}}-archive`.
- Chart: `template {{.Version}}`, canonical charts.crossplane.io plus the S3 origin; exceptions for 1.20.2 and 2.0.3.
- Image: four registries, same repository path; verify through docker.io / ghcr.io.
- CLI: `crank` in the core bucket before v2.3; `crossplane` from crossplane/cli at the line's X.Y.0 afterwards (approximation).
- Gaps: Kubernetes compatibility, support window / EOL, upgrade-path constraints, per-line CLI patch relation.
