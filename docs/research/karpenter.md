# Karpenter (AWS) release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could
not be checked here. `RAW` = `https://raw.githubusercontent.com/aws/karpenter-provider-aws`. Product:
Karpenter for AWS, the open-source node autoscaler (`NodePool`/`EC2NodeClass`/`NodeClaim`, EC2 fleet
via the AWS APIs). Canonical repository `github.com/aws/karpenter-provider-aws`; the version-agnostic
libs live in `github.com/kubernetes-sigs/karpenter` (`sigs.k8s.io/karpenter`) and are **pinned per
release in go.mod** (section 6).

**`public.ecr.aws` is reachable from this sandbox** (ECR Public Distribution API: anonymous token
endpoint and `/v2/` tag/manifest endpoints all answer 200) — the first product in the catalog
published there. `gallery.ecr.aws` (the human gallery) and `karpenter.sh` also answer 200.

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, `v`-prefixed git tags, minor lines.** 272 tags: 265 stable (`v0.1.0` … latest
   **v1.14.1**, 2026-08-20), 6 pre-releases (`v0.9.0-rc.0/1`, `v0.14.0-rc.0`, `v0.28.0-rc.1/2`,
   `v0.30.0-rc.0`) and one junk tag (`v1.7.3-hash-fix-backport`). The release workflow also declares
   `-alpha.N`/`-beta.N` triggers but none exist as tags. Lines are maintained in parallel: **v1.9.x
   and v1.14.x are LTS lines** (see 7); all releases are cut from `main` (v1.13.1 and v1.14.0 were
   tagged 47 minutes apart on 2026-07-10, from different commits).
2. **Image and chart tags are `v`-LESS from 0.35.0 on.** `hack/release/release.sh` passes
   `"${git_tag#v}"` to `ko publish` and `helm package --version`, so the ECR tag is the bare semver
   (>= 0.35.0) and the `v`-prefixed git tag (< 0.35.0). This is documented upstream
   (compatibility.md: "stable releases prior to `0.35.0` are prefixed with a `v`") — **the tag-format
   boundary is v0.35.0, NOT v1.0.0**. The v0.x → v1.x boundary (v1.0.0, 2024-11 GA) instead carries
   the v1beta1 → v1 API migration (`website/content/en/v1.0/upgrading/v1-migration.md` +
   `v1beta1-migration.md`, referenced by every later upgrade-guide section's warning alert).
3. **Release notes = GitHub release bodies**, conventional-commit sections per tag: `## Features`,
   `## Bug Fixes`, `## Documentation`, `## Tests`, `## Continuous Integration`,
   `## Code Refactoring`, `## Performance Improvements`, `## Chores`, `## Commits` (exact set varies
   per release; v1.14.x bodies are CRLF). No "Breaking Changes" section exists — breaking changes
   are documented in the website upgrade guide (policy in compatibility.md). LTS releases open with
   a banner blockquote ("This is a Long-Term Support (LTS) release. Supported until Jul 2027"). 0
   assets on every release checked; api.github.com anonymous works but rate-limits at 60/h (hit
   during this research; the ri fetch cache absorbs repeats).
4. **Upgrade guide = per-line website docs, cumulative, `### Upgrading to \`X.Y.Z\`+` sections**
   (newest first; v0-era headings carry a `v` inside: `### Upgrading to v0.32.0+`). The versioned
   directory `website/content/en/vX.Y/` is created by the release CI *after* the tag (from
   `preview/`), so at tag vX.Y.0 only `website/content/en/preview/upgrading/upgrade-guide.md`
   carries the line's section — and preview at the tag has it for every minor checked (v0.32.0,
   v0.33.0, v0.35.0, v1.0.0, v1.12.0, v1.14.0) except **v1.13.0/v1.13.1** (the section was added to
   main only after the branch point; upstream anomaly). Only sections per *minor* exist (plus special
   patches like 0.27.3); patches have no section. A `### CRD Upgrades` section explains the
   `karpenter-crd` chart split.
5. **Compatibility = `hack/docs/compatibilitymatrix_gen/compatibility.yaml`** — a machine-readable
   YAML list of `{appVersion: "X.Y.x", minK8sVersion, maxK8sVersion}` records (0.21.x … 1.14.x,
   appended at release time by `hack/docs/version_compatibility_gen` from the
   `version.Min/MaxK8sVersion` constants). The website compatibility.md table is generated from it
   by `tools/kompat`. Upstream anomaly: the record for line X.Y is missing at the X.Y.0 tag for some
   lines (no 1.0.x record at v1.0.0, no 1.13.x record at v1.13.0/v1.13.1 — the file on main catches
   up between tags), and the 1.12.x record is duplicated.
6. **Security**: GitHub repository security advisories (GHSA; api.github.com anonymous). The
   repository also runs `govulncheck` in CI, not a channel.
7. **Lifecycle / LTS** (SUPPORT.md, main only — 404 at every tag): regular minors are "supported
   only until the next minor release"; every ~6 months a minor is designated LTS and supported 12
   months, at most 2 LTS lines concurrently. Release *names* carry "(LTS)": v1.9.0/1/2 and
   v1.14.0/1 — the two current lines; v1.6.0/v1.2.0/v1.10.0 are not LTS-named.
8. **Artifacts.** One container image `public.ecr.aws/karpenter/controller` (ko build of
   `./cmd/controller`; `public.ecr.aws/karpenter/kwok` exists but has 0 tags — dead); two Helm
   charts pushed as OCI to `public.ecr.aws/karpenter/karpenter` and `public.ecr.aws/karpenter/
   karpenter-crd` with **chart version == appVersion == release version** (verified from ECR chart
   configs at v0.25.0, v0.33.0, v0.34.0, 0.37.8, 1.0.0, 1.14.1); CRDs in `pkg/apis/crds/*.yaml` at
   the tag (5 files at v1.14.1, including the third-party-group `autoscaling.x-k8s.io`
   capacitybuffers CRD the 1.14 upgrade notes say is shipped). **The source-tree chart files lag the
   tag**: at tag vN the in-repo `charts/*/Chart.yaml` and `values.yaml` still carry N-1 (v1.14.1 →
   1.13.0, v1.13.0 → 1.12.1, v1.12.0 → 1.11.1, v1.0.0 → 0.37.0, v0.35.0 → 0.34.0, v0.34.0 →
   0.33.0, v0.32.10 → 0.32.0 — every era), because `release.sh` rewrites Chart.yaml/values.yaml in
   CI and the bump reaches `main` only via the generated stable-release PR. `charts.karpenter.sh`
   answers 200 but its index ends in 2022 (appVersion 0.16.3) — a dead v0-era channel. Snapshot
   builds go to a private account ECR (`021119463062.dkr.ecr.us-east-1.amazonaws.com/karpenter/
   snapshot`), not a public channel.

## 1. Canonical versions

`git ls-remote --tags` via the discovery run [200]: 272 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `vX.Y.Z` | 265 | `v1.14.1` | stable releases (v0.1.0 → v1.14.1) |
| `vX.Y.0-rc.N` | 6 | `v0.30.0-rc.0` | release candidates (excluded) |
| junk | 1 | `v1.7.3-hash-fix-backport` | backport scratch tag |

- Pattern (as proposed by discovery, kept): `^v(?P<version>\d+\.\d+\.\d+(?:-rc\.\d+)?)$`, lineage
  `minor`, pre-releases excluded.
- Cadence: minor every ~6–8 weeks (v1.12.0 2026-04-24, v1.13.0 2026-06-10, v1.14.0 2026-07-10),
  patches roughly monthly per maintained line.

## 2. Release notes (GitHub release bodies) [200]

One release per tag, created by `marvinpinto/action-automatic-releases` in
`.github/workflows/release.yaml`; the body is conventional-commit grouped (the same generator shape
as release-please output). Sections observed: `## Features`, `## Bug Fixes`, `## Documentation`,
`## Tests`, `## Continuous Integration`, `## Code Refactoring`, `## Performance Improvements`,
`## Chores`, `## Commits`. Items are `- PR title (#N) [#N](link) (author)` lines; the `## Commits`
section is a raw commit digest. LTS releases (v1.9.x, v1.14.x) open with a `> 🛡️ … Long-Term
Support … Supported until <month year> …` blockquote. v1.14.x bodies use CRLF line endings.
0 assets on all releases checked — nothing to probe beyond the body.

## 3. Upgrade guide (website, per line) [200]

- `website/content/en/vX.Y/upgrading/upgrade-guide.md` — cumulative, `### Upgrading to \`X.Y.Z\`+`
  sections newest-first (v0-era: `### Upgrading to v0.32.0+`), each with a warning alert about the
  v1beta1→v1 migration, bullets ("This version adds/graduates/drops the support …"), a
  "No breaking changes 🎉" marker when true, and a "Full Changelog" linking both the provider and
  the libs release. `### CRD Upgrades` documents the `karpenter-crd` chart and Helm's crds/
  lifecycle limitation.
- **Directory lag**: `website/content/en/vX.Y/` is created post-tag from `preview/` by
  `hack/release/common.sh` (`prepareWebsite`, keeps last 3 lines + `preview` + `docs` + `v1.0`).
  At v1.13.0: dirs v1.0, v1.9–v1.12 (no v1.13); at v1.14.0/v1.14.1: v1.0, v1.11–v1.13 (no v1.14);
  `main` today: v1.0, v1.12, v1.13, v1.14. Therefore the line's own guide at the line's .0 tag is
  readable only from `preview/upgrading/upgrade-guide.md` at the tag.
- v1.0 additionally ships one-time migration docs (`v1.0/upgrading/v1-migration.md`,
  `v1beta1-migration.md`); they are linked from every later section's alert, not separate channels.
- Special patch-level sections exist (e.g. `Upgrading to 0.27.3+`); the minor-anchored selector
  reads minor sections only (same gap shape Cilium recorded).

## 4. Security

GitHub repository security advisories (anonymous REST [200] during discovery; rate-limited later in
this research hour). `make vulncheck` (govulncheck) is CI-only. No OSV/CSV channel.

## 5. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| image `controller` | `public.ecr.aws/karpenter/controller` | tag = bare semver >= 0.35.0, `v`+semver before | [200] manifests at v0.25.0, v0.32.10, v0.34.0, 0.35.0, 1.0.0, 1.12.1, 1.13.0*, 1.14.1* |
| chart `karpenter` | OCI `public.ecr.aws/karpenter/karpenter` | chart version == appVersion == release version | [200] chart configs at v0.25.0, v0.33.0, v0.34.0, 0.37.8, 1.0.0, 1.14.1 |
| chart `karpenter-crd` | OCI `public.ecr.aws/karpenter/karpenter-crd` | same | [200] chart configs at v0.25.0, v0.33.0, 1.14.1 |
| CRDs (5 at v1.14.1) | `pkg/apis/crds/*.yaml` at the tag | at the tag | [200] discovery validated 6/6 releases |
| chart values / Chart.yaml in-repo | `charts/karpenter{,-crd}/` at the tag | **lag one release** (see 0.8) | [200] — NOT usable as the release's chart content |
| legacy helm index | `https://charts.karpenter.sh` | dead since 2022 (0.16.3) | [200] index reachable, stale |
| kwok image | `public.ecr.aws/karpenter/kwok` | — | [200] tags list empty (0 tags) |
| snapshot charts/images | private ECR `021119463062.dkr.ecr.us-east-1…/karpenter/snapshot` | per-commit | not a public channel |
| release binaries | none — ko image only, 0 release assets | | [200] |
| EKS install manifests | `website/content/en/preview/getting-started/…/cloudformation.yaml` (IAM/IRSA) | at the tag | [200] infra setup, not a product artifact |

`public.ecr.aws` quirk: tag lists are full of `sha256-*.sig/.att/.sbom` cosign noise and commit-SHA
tags; the ri OCI adapter already filters that. The chart's cosign signature is pushed to the same
repository:tag (`cosignOciArtifact` in `common.sh`).

## 6. Upstream libs (`sigs.k8s.io/karpenter`) version relationship

go.mod at the release tag pins the libs [200]:

| release | pin |
|---|---|
| v1.14.1 | `v1.14.1-0.20260819221709-6e7eab7a0f48` (pseudo-version of a commit just after the libs' v1.14.1) |
| v1.13.0 | `v1.13.0` |
| v1.12.0 / v1.6.0 / v1.0.0 / v0.35.0 / v0.34.0 | exact same version |
| v0.37.8 | `v0.37.7` (lagged one patch) |

So the relationship is "libs version == release version, occasionally a pseudo-version or one patch
behind". The `version.strategy: field` construct (kube-prometheus-stack) cannot read it: go.mod is
not YAML (`require (` blocks do not parse), and `normalize.ReadYAMLPath` YAML-parses the document.
No YAML file of the release carries the pin (Chart.yaml has no annotation for it); the upgrade-guide
"Full Changelog" bullet links the libs release tag as prose. Recorded as a gap.

## 7. Compatibility and lifecycle

- `hack/docs/compatibilitymatrix_gen/compatibility.yaml` [200]: records `appVersion` ("X.Y.x"),
  `minK8sVersion`, `maxK8sVersion` (1.14.x → 1.26/1.36). Generated into the website
  compatibility.md table by `tools/kompat` at `make docgen`; `version_compatibility_gen` appends
  the new line's record at release time (from `version.Min/MaxK8sVersion` constants). Anomalies:
  the X.Y.x record is absent at some X.Y.0 tags (1.0.0, 1.13.0, 1.13.1 verified missing; 1.12.0 and
  1.14.0/1 have theirs); 1.12.x duplicated; `1.0.5` has its own patch-granular record (maxK8s 1.31)
  shadowed by the `1.0.x` record.
- karpenter.sh website (rendered matrix) [200] — reachable, but the in-repo YAML is the
  machine-readable original.
- Chart.yaml has **no kubeVersion** (either chart, either era): no chart-level k8s guard to capture.
- LTS/EOL: SUPPORT.md on `main` only [200 at main, 404 at tags] — regular minors superseded by the
  next minor; LTS (v1.9, v1.14 by release name) supported 12 months, ≤2 concurrent. v0.x: "we will
  not release security patches to older versions" (compatibility.md).

## 8. Hosts (from this sandbox, 2026-10-01)

Reachable: github.com (git + refs), raw.githubusercontent.com, codeload (tag archives),
api.github.com (anonymous 60/h — exhausted by this research session once; recovery within the
hour), public.ecr.aws (token + /v2 tags/manifests/blobs, anonymous), gallery.ecr.aws, karpenter.sh,
charts.karpenter.sh (stale index). Blocked: none needed. Not probed: the private snapshot ECR
(requires AWS auth by design).
