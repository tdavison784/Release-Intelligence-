# Kyverno release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here.
`RAW` = `https://raw.githubusercontent.com/kyverno/kyverno`. Product: Kyverno, Kubernetes-native policy
management (admission controller + background/reports/cleanup controllers + `kyverno` CLI). Canonical
repository `github.com/kyverno/kyverno`. CNCF graduated (2026-03). The Helm chart `kyverno` lives in the
SAME repository under `charts/` and is the primary install path.

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, `v`-prefixed, minor lines.** Tags `v0.1.0` → `v1.19.1` (125 stable); pre-releases
   `v1.X.0-alpha.N|beta.N|rc.N` and glued `-rcN` (before ~1.13); 427 junk tags (`1.6-dev`, unprefixed
   ancient). Latest stable **v1.19.1** (2026-09-10). A minor every ~3–4 months (1.15 2025-07,
   1.16 2025-11, 1.17 2026-02, 1.18 2026-04, 1.19 2026-08). **Only the newest minor line is
   supported** (~3 months of community patch support, critical fixes only — releases.md).
2. **Release notes are the GitHub release body** for every release (patches included): a
   github-generated "## What's Changed" PR list (`* fix: … by @author in …/pull/N`) — complete but
   noisy, exactly the Argo CD shape. For minors, the **website announcement blog**
   (`kyverno/website`, `src/content/blog/announcing-kyverno-release-1.X/index.md`, 1.12+) is the
   curated channel: TL;DR bullets, feature sections, deprecation schedules. The in-repo
   `CHANGELOG.md` is **stale** (newest section is v1.13.0 even at the v1.19.1 tag).
3. **Deprecation/removal policy with exact removal minors is Kyverno's high-value breaking
   source.** It lives on the website: `docs/installation/upgrading.md` carries per-minor
   `## Upgrading to Kyverno v1.X` sections (only for minors that need upgrade steps — today 1.13
   and 1.19); `docs/policy-types/overview.md` carries the schedule table (legacy
   `ClusterPolicy`/`Policy`/`CleanupPolicy` + `kyverno.io` PolicyException: marked 1.17 →
   critical-fixes-only 1.18 → **deprecated 1.19 → removed 1.20, Nov 2026 estimated**); the 1.19
   blog repeats the schedule. Both are markdown at `main` of `kyverno/website` (not at tags).
4. **Seven container images per release on `ghcr.io/kyverno`** (ko, tagged `v1.X.Y`, all [200]):
   `kyverno`, `kyvernopre` (init container), `kyverno-cli`, `cleanup-controller` (≥1.9.0),
   `background-controller`, `reports-controller` (≥1.10.0), `readiness-checker` (≥1.17.2, chart
   test hook image). No other registry is published to by the release workflow.
5. **Helm charts in the same repo, versioned independently, cut on chart tags** (`kyverno-chart-v*`,
   `kyverno-policies-chart-v*`). `charts/kyverno` chart 3.9.1 has `appVersion: v1.19.1` (**v-prefixed**).
   Chart majors track app majors (chart 2.x → app 1.5–1.9, chart 3.x → app 1.10+); every stable app
   release 1.10.0–1.19.1 has a chart whose appVersion == the app tag, so the relation is a **lookup
   by appVersion == {{.Tag}}** (268 index entries for `kyverno`, 204 for `kyverno-policies`).
   Published to `https://kyverno.github.io/kyverno` (gh-pages index, [200]) and as OCI charts to
   `ghcr.io/kyverno/charts/{kyverno,kyverno-policies}` [200]. Since 1.17 the chart pins the
   **`kyverno-api` chart** (the CRD chart, `dependencies[name=kyverno-api].version` in Chart.yaml at
   the tag: 0.0.1-alpha.2 in 1.17/1.18, 0.0.1-alpha.4 in 1.19) from `https://kyverno.github.io/api` [200].
6. **CRDs** in `config/crds` (recursive; group subdirs `kyverno/`, `policies.kyverno.io/` (≥1.14),
   `policyreport/`, `reports/` since 1.12, flat before): 11 → 22 CRDs. Also shipped as 22 individual
   release assets per release [200] and inside the chart (`charts/crds` subchart; via the kyverno-api
   dependency from 1.19 on).
7. **CLI** = `kyverno-cli_{{.Tag}}_{os}_{arch}.tar.gz` release assets (v-prefixed in the name) [200
   v1.8.0–v1.19.1] + the `kyverno-cli` image; plus `install.yaml` (generated all-in-one manifest,
   release asset since ≥v1.9.0 [200]) and `checksums.txt` / sigstore signatures.
8. **Compatibility**: chart `kubeVersion` is the per-release Kubernetes minimum
   (`>=1.16.0-0` through 1.10/1.13 era, `>=1.25.0-0` from 1.13/1.16 on — read from Chart.yaml at the
   tag). The website `installation/releases.md` page states the CURRENT line's support (v1.19:
   Kubernetes v1.33–v1.35) — mutable page, per-release history not kept.
9. **Security**: GitHub repository security advisories (34 published, GHSA + CVE, api.github.com
   anonymous [200]); kyverno.io/docs/security redirects (301) — process page, not a feed.

Related products (NOT this definition): `kyverno-chainsaw` (CLI test tool, own repo), `kyverno-json`
(JSON policy engine, own repo, its CLI was removed from this repo in 1.19), the
`kyverno/kyverno-policies` policy library (the `kyverno-policies` CHART is here and is declared; the
policy library repo is the same content upstream), `reports-server` (own repo; a chart dependency),
`kyverno/api` (the kyverno-api chart's source repo).

## 1. Canonical versions

`git ls-remote --tags https://github.com/kyverno/kyverno` [200]: 733 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `v1.X.Y` stable | 125 | `v1.19.1` | releases (the product), v0.1.0 → v1.19.1 |
| `v1.X.0-alpha.N/beta.N/rc.N` | ~99 | `v1.19.0-rc.4` | modern pre-releases |
| `v1.X.Y-rcN` (glued) | ~82 | `v1.13.0-rc1` | old-style pre-releases (before ~1.13) |
| junk | 427 | `1.6-dev`, `1.3.0-rc10`, `snapshot` | unprefixed/legacy — excluded |

- Pattern (as proposed by discovery, kept): `^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\.\d+|beta\.\d+|rc\.\d+|rc\d+))?)$`, lineage `minor`.
- Anomaly: tag `v1.15.20` (between 1.15.3 and 1.16.0) — a mistaken-looking extra patch of the 1.15
  line; only ever traversed when To is in line 1.15.
- Support policy (releases.md): "approximately three (3) months of community patch support" for the
  **Supported Release** (the newest minor) only; patches = critical bugs + critical/high CVEs;
  release branches `release-1.X`, everything cherry-picked from main. Older lines receive nothing
  (checked: 1.17 last patch 1.17.2 Mar 2026, then 1.18.0; 1.18 last patch 1.18.2, then 1.19.0).

## 2. Release notes

- **GitHub release body** [200 via API]: `## What's Changed` + github-native PR bullets
  (`* <subject> by @author in https://github.com/kyverno/kyverno/pull/N`), sometimes a
  `## New Contributors` tail. Complete for every tag (patches included), but unclassified and
  dependency/CI-heavy (Argo CD's git-log shape). api.github.com anonymous is 60 req/h — the standing
  quota tax on per-release body fetches.
- **Website blog** [200 raw]: `src/content/blog/announcing-kyverno-release-1.X/index.md` on
  `kyverno/website@main`, minors 1.12–1.19 (older minors used `kyverno-1.X-released` naming —
  different product era). Astro-markdown frontmatter + `# Announcing Kyverno Release 1.X!` +
  `## **TL;DR**` (bold-wrapped headings) + feature `## **Sections**` + deprecation tables.
  1.19's post carries the deprecation/removal schedule verbatim.
- **CHANGELOG.md** in-repo [200]: per-release `## v1.X.Y` sections with `### Note/Bug Fixes` label
  headings — but the newest section is **v1.13.0** even at the v1.19.1 tag: stale since 2023-10,
  useless for recent releases (rejected as a source).

## 3. Upgrade guide and deprecation policy (website, `main` only)

`RAW` does not apply: these live in `github.com/kyverno/website@main` [200].

- `src/content/docs/docs/installation/upgrading.md`: `## Upgrading Kyverno` (generic procedure,
  YAML manifest = uninstall/reinstall, Helm = no direct upgrade across 1.10) then per-minor
  sections **only when the minor needs upgrade steps**: today `## Upgrading to Kyverno v1.19`
  (Deprecations, Storage Versions, Helm Chart Changes, CLI Changes) and
  `## Upgrading to Kyverno v1.13` (Breaking Changes, Dropped API versions). Minors without a
  section (1.14–1.18) have no per-minor steps — upstream's statement that none are needed.
- `src/content/docs/docs/policy-types/overview.md`: the API-status tables — policy types →
  API version → status ("**Deprecated (v1.19), removed in v1.20**"), deprecated API versions,
  and the **Deprecation Schedule for Legacy Types** (v1.17 marked → v1.18 critical fixes only →
  v1.19 officially deprecated → v1.20 removed, Nov 2026 estimated).
- Blog + release bodies repeat the policy; the 1.19 body's PR list contains the
  deprecation-warning plumbing commits.

## 4. Security

- GitHub advisories [200]: 34 published on kyverno/kyverno (e.g. GHSA-5qq8-67g6-4h2w critical
  2026-09-10; GHSA-79gf-7frw-68m9 CVE-2026-54523). The `github-advisories` adapter works from here.
- `https://kyverno.io/docs/security/` → 301 (redirects); SECURITY.md → GitHub private vulnerability
  reporting + the advisories feed. No separate machine-readable CVE feed.

## 5. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| image `kyverno` | `ghcr.io/kyverno/kyverno:v1.X.Y` | `v` + release | [200] v1.8.5–v1.19.1 |
| image `kyvernopre` (init) | `ghcr.io/kyverno/kyvernopre` | same | [200] v1.8.5–v1.19.1 |
| image `kyverno-cli` | `ghcr.io/kyverno/kyverno-cli` | same | [200] v1.8.5–v1.19.1 |
| image `cleanup-controller` | `ghcr.io/kyverno/cleanup-controller` | same | [200] ≥ v1.9.0 (404 at 1.8.x) |
| image `background-controller` | `ghcr.io/kyverno/background-controller` | same | [200] ≥ v1.10.0 (404 at 1.9.x) |
| image `reports-controller` | `ghcr.io/kyverno/reports-controller` | same | [200] ≥ v1.10.0 (404 at 1.9.x) |
| image `readiness-checker` | `ghcr.io/kyverno/readiness-checker` | same | [200] ≥ v1.17.2 (404 at 1.17.1; chart test hook) |
| Helm chart `kyverno` | `kyverno.github.io/kyverno` index + OCI `ghcr.io/kyverno/charts/kyverno` | **lookup**: chart release whose `appVersion` == `v1.X.Y` (chart majors: 2.x→app 1.5–1.9, 3.x→app 1.10+; chart 3.9.1 ↔ app v1.19.1) | [200] both; every stable 1.10.0–1.19.1 has one |
| Helm chart `kyverno-policies` | same index + OCI `ghcr.io/kyverno/charts/kyverno-policies` | same lookup shape | [200] both |
| `kyverno-api` chart pin | `dependencies[name=kyverno-api].version` in `charts/kyverno/Chart.yaml` at the tag; chart published at `kyverno.github.io/api` | per-release pin (0.0.1-alpha.2 @1.17–1.18, 0.0.1-alpha.4 @1.19); ≥1.17.0 | [200] both |
| CRDs | `config/crds/**/*.yaml` at the tag (recursive; group subdirs since 1.12) + 22 release assets + chart `crds` subchart / kyverno-api dep (1.19+) | at the tag | [200] |
| CLI archives | `releases/download/{{.Tag}}/kyverno-cli_{{.Tag}}_linux_x86_64.tar.gz` (+darwin/windows, arm64/s390x) | tag in the asset name | [200] v1.8.0–v1.19.1 |
| `install.yaml` | release asset | at the tag | [200] v1.9.0–v1.19.1 |
| checksums / sigstore | `checksums.txt`, `*.sigstore.json` assets | at the tag | [200] |

Chart releases are tagged separately (`kyverno-chart-v3.9.1`) and the workflow publishes index +
OCI + cosign signatures. Chart.yaml at every app-release tag already carries the matching
`appVersion` and chart `version` (bumped by the release commit), so source-tree Chart.yaml is the
join evidence. Chart `kubeVersion`: `>=1.16.0-0` (1.9/1.10) → `>=1.25.0-0` (1.13+).

## 6. Compatibility

- Chart `kubeVersion` (per-release Kubernetes minimum) from Chart.yaml at the tag — captured as
  chart-metadata content (cilium precedent).
- releases.md (mutable, main): current line only — v1.19, EOL at v1.20 (Nov 2026 est.),
  Kubernetes v1.33–v1.35. Used as lifecycle evidence, not as a per-release compatibility table
  (history is not kept).
- No machine-readable per-release Kubernetes matrix exists upstream (e2e workflow matrices are test
  config, not support statements).

## 7. Hosts (from this sandbox, 2026-10-01)

Reachable: github.com (git + release downloads), raw.githubusercontent.com, ghcr.io (anonymous
tokens; all 7 image repos + both OCI chart repos), kyverno.github.io (kyverno index + api chart
index), kyverno.io (301 for /docs/security/, 200 elsewhere), api.github.com anonymous (60 req/h —
quota ran low during research; reset is hourly, transient).
Blocked: none of the channels this definition needs. Docker Hub mirrors are not published for
Kyverno images (release workflow pushes to ghcr.io only) — not declared at all.
