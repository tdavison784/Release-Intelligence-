# Cilium release-channel research (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the checks and probes were re-run live; the live `ri check` outcomes are unchanged. Any unreachable host recorded below was a transient anonymous rate limit or an anonymous-auth refusal, not a block of this product's channels (details: docs/rerun/REPORT.md, per-product table). The original text below is kept as the record of 2026-10-01.

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here (egress policy or inferred).
`RAW` = `https://raw.githubusercontent.com/cilium/cilium`. Product: Cilium, eBPF-based
networking, security and observability for Kubernetes (agent + operator + Envoy proxy +
Hubble). Canonical repository `github.com/cilium/cilium`.

**This sandbox could reach more hosts than earlier waves recorded**: quay.io,
helm.cilium.io, api.github.com (anonymous), registry-1.docker.io and
hub.docker.com all answer from here (see section 7).

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, `v`-prefixed, minor lines.** Tags `v1.X.Y`; lines maintained in
   parallel on `v1.X` branches (lineage `minor`); pre-releases `v1.X.0-pre.N`,
   `v1.X.0-rc.N` (and bare `-rcN` before ~1.13). Latest stable today: **v1.20.2**
   (chart published 2026-09-16); maintained lines 1.18, 1.19, 1.20; 1.21 is in
   pre-release (`v1.21.0-pre.2`). 885 tags: 340 stable, 137 pre-releases, 408 junk
   (unprefixed `1.0.4`-style ancient tags, submodule bumps, `snapshot`).
2. **Release notes = CHANGELOG.md in the repository** (since v1.8.0), one
   `## v1.X.Y` section per release at the release's own tag, cumulative per line,
   newest first. Items are grouped under bold label paragraphs
   (`**Major Changes:**`, `**Minor Changes:**`, `**Bugfixes:**`, `**CI Changes:**`,
   `**Misc Changes:**`, `**Other Changes:**`). The GitHub Release body is the same
   section verbatim (both [200]).
3. **Upgrade guide is reStructuredText.** Before 1.20: the `X.Y Upgrade Notes`
   section of `Documentation/operations/upgrade.rst` at the tag (exists since
   v1.9.0). From 1.20 on: `Documentation/operations/upgrade-current.inc` (the
   `X.Y Upgrade Notes` content, split out of upgrade.rst; identical across the
   line's patches). Subsections are the upgrade intelligence:
   `Action Required`, `Informational Notes`, `Changes to Features`,
   `New Options`, `Changed Options`, `Deprecated Options`, `Removed Options`,
   `Changes to Metrics` (Added/Changed/Removed), `Removed CRD Fields`.
   Markdown selectors cannot read rst underline headings — this is what forces
   `extract.format: rst` (see section 3).
4. **Compatibility.** Chart `kubeVersion` is the minimum Kubernetes (`>= 1.21.0-0`
   since 1.16, `>= 1.16.0-0` before). `Documentation/network/kubernetes/compatibility.rst`
   at the tag carries a grid table whose single `k8s Version` row lists the line's
   e2e-tested Kubernetes minors (1.19: `1.29, 1.30, 1.31, 1.32`). Both captured.
5. **Security**: GitHub repository security advisories (GHSA + CVE, 10 published;
   api.github.com anonymous [200]).
6. **Artifacts.** Helm chart `cilium` (chart version == appVersion == release
   version, checked across 1.13.1–1.20.2) published to quay.io (OCI) and
   helm.cilium.io; container images on quay.io/cilium: `cilium`, `operator` (+
   `operator-aws|azure|alibabacloud|generic`), `clustermesh-apiserver`,
   `hubble-relay`, all tagged `v1.X.Y` [200]; docker.io/cilium/* mirrors track the
   same tags [200]; 17 CRDs in `pkg/k8s/apis/cilium.io/client/crds/v2` [200];
   **no release binaries/assets** (GitHub releases ship 0 assets); tag tarball via
   codeload is blocked. The chart also pins independently versioned images
   (`cilium-envoy` v1.37.6-…, `certgen` v0.4.11, `hubble-ui`/`hubble-ui-backend`
   v0.13.6, `ztunnel` v1.0.0, `startup-script` timestamp tags) — dependencies, not
   release artifacts.

## 1. Canonical versions

`git ls-remote --tags https://github.com/cilium/cilium` [200]: 885 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `v1.X.Y` | 340 | `v1.20.2` | stable releases (the product) |
| `v1.X.0-pre.N` / `v1.X.0-rc.N` | 46 | `v1.21.0-pre.2` | modern pre-releases |
| `v1.X.0-rcN` | 91 | `v1.13.0-rc0` | old-style glued rc pre-releases |
| unprefixed `N.N(.N)` + `N.N.N-rcN` | 408 | `1.0.4`, `1.10.0-rc0` | pre-1.9 tags **without** the `v` prefix (plus submodule/junk) — excluded |
| junk | rest | `snapshot`, `v1.0-rc0`… | ignored |

- Pattern (as proposed by discovery, kept): `^v(?P<version>\d+\.\d+\.\d+(?:-(?:pre\.\d+|rc\.\d+|rc\d+))?)$`, lineage `minor`, pre-releases excluded from the canonical list.
- Lines `v1.X` branches exist for maintained lines [200]; `main` carries 1.21 pre-release development (`v1.21.0-pre.2`).
- Release cadence: a minor every ~4–5 months (1.18 2025-…, 1.19, 1.20), patches
  roughly monthly per line; several lines maintained in parallel.

## 2. Release notes (CHANGELOG.md) [200]

`RAW/<tag>/CHANGELOG.md` — exists since v1.8.0 (v1.7 and earlier: no file; the
GitHub Release body is the only channel there).

```
# Changelog
## v1.20.2                     <- one section per release, newest first
Summary of Changes
------------------             <- setext underline (NOT an ATX heading; the
                                 section extends to the next ## heading)
**Bugfixes:**                  <- bold label paragraphs (not headings)
* aws/ipam: Fixed a bug where the operator ... (Backport PR cilium/cilium#48418, Upstream PR cilium/cilium#48125, @41ks)
```

- Sections are per release; the file at a tag is cumulative for the line
  (v1.18.14 lists 1.18.14 … 1.18.0). Read at the release's own tag.
- Label paragraphs (stable since 1.8): `**Major Changes:**`, `**Minor Changes:**`,
  `**Bugfixes:**`, `**CI Changes:**`, `**Misc Changes:**`, `**Other Changes:**`.
  Minor releases (X.Y.0) carry Major/Minor; patches carry Minor/Bugfixes.
- Every bullet ends with a PR/contribution trailer (`(Backport PR …, Upstream PR …,
  @author)`); breaking changes are **not** labelled in the changelog — they live in
  the upgrade notes (section 3). The changelog is the per-release change list.
- GitHub Release body == the release's CHANGELOG section verbatim [200]; api.github.com
  anonymous works but is rate-limited (60/h), so the raw file is the primary channel.
- Release announcement blog posts (cilium.io/blog, repo `cilium/cilium.io`) are
  narrative, not per-change notes; not ingested.

## 3. Upgrade guide (reStructuredText) [200]

- **< 1.20**: `RAW/<tag>/Documentation/operations/upgrade.rst`. Structure (rst
  underline headings): title (`*`), `=` sections (`Running pre-flight check`,
  `Upgrading Cilium`, `Version Specific Notes`, `Advanced`), `-` subsections —
  including **`X.Y Upgrade Notes`** — `~~~~` sub-subsections (`Action Required`,
  `Informational Notes`, `Changes to Features`, `Changes to Metrics`), `####`
  sub-sub-subsections (`Network Policy`, `New Options`, `Changed Options`,
  `Deprecated Options`, `Removed Options`, `Added/Changed/Removed Metrics`, …).
  The `X.Y Upgrade Notes` section exists for every line from 1.9 to 1.19 at the
  line's tags (1.11: yes, under several patch-level notes).
- **≥ 1.20**: the version-specific content moved to
  `Documentation/operations/upgrade-notes.inc` + `upgrade-current.inc`; upgrade.rst
  only `.. include::`s them. `upgrade-current.inc` is identical across a line's
  patches (v1.20.0 == v1.20.2 [200]) and its first heading is
  `|CURRENT_RELEASE| Upgrade Notes` (a Sphinx substitution, `v1.20`).
- Special **patch-level** upgrade notes exist for a few releases — sections
  `1.9.1 Upgrade Notes` (kernel requirement regression), `1.11.5`, `1.11.15`,
  `1.11.16+`, `1.11.18+` (map-in-map regression), `1.14.2` (IPsec CVE), `1.17.4` —
  additional to the line's `X.Y Upgrade Notes`; a single-section selector picks the
  line notes, so these are a recorded gap, not a source.
- **Format**: all of this is reStructuredText. Markdown section selection cannot
  see underline headings, and grid tables are not pipe tables. A line-preserving
  rst→markdown renderer (headings by rst first-use underline hierarchy,
  `*` bullets kept, ``literal`` → `` `literal` ``, directive/target lines
  (`.. _x:`, `.. include::`, `.. only::`) blanked, grid tables → pipe tables)
  makes the existing markdown-section/markdown-table/whole extraction and classify
  rules work unchanged. Motivated here; benefits other rst projects (Python
  NEWS/upgrade notes, OpenStack release notes, towncrier `CHANGELOG.rst` projects
  like Airflow; Cilium's own README.rst uses the same markup).

## 4. Security

- GitHub repository security advisories [200]
  (`/repos/cilium/cilium/security-advisories`, anonymous): 10 published GHSA/CVEs
  (e.g. GHSA-3fcv-jvfp-m4q9 CVE-2026-49445 critical). The `github-advisories`
  adapter works from this sandbox.
- Cilium also documents its security process at
  `https://cilium.io/blog/category/security/` and SECURITY.md → community Security
  Team; CVE write-ups are blog posts. Not machine-readable; the GHSA feed covers it.

## 5. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| Helm chart `cilium` | `quay.io/cilium/charts/cilium` (OCI) + `https://helm.cilium.io` (index) | chart version == appVersion == `1.X.Y` (no `v`) | [200] both; 321 chart versions in the index; sampled 1.13.1, 1.16.0, 1.18.0, 1.19.0, 1.20.0, 1.20.2 |
| chart values / metadata | `install/kubernetes/cilium/values.yaml`, `Chart.yaml` at the tag | bumped by the release commit (`Prepare for release vX.Y.Z`); image tags at the tag read `v1.X.Y` | [200] |
| image `cilium` | `quay.io/cilium/cilium:v1.X.Y`; mirror `docker.io/cilium/cilium` | `v` + release | [200] quay v1.9.10…v1.20.2, docker.io v1.20.2 |
| image `operator` (+ `-aws`, `-azure`, `-alibabacloud`, `-generic`) | `quay.io/cilium/operator*`; mirrors `docker.io/cilium/operator*` | `v` + release | [200] all 5 variants at v1.13.1/v1.20.2; values.yaml picks the suffix per cloud |
| image `clustermesh-apiserver` | `quay.io/cilium/clustermesh-apiserver`; docker.io mirror | `v` + release | [200] v1.9.10…v1.20.2 |
| image `hubble-relay` | `quay.io/cilium/hubble-relay`; docker.io mirror | `v` + release | [200] v1.9.10…v1.20.2 |
| CRDs (17, `cilium.io/v2` etc.) | `pkg/k8s/apis/cilium.io/client/crds/v2/*.yaml` at the tag; also rendered into the chart (`templates/cilium-crds.yaml`) | at the tag | [200] |
| source tarball | `github.com/cilium/cilium/archive/refs/tags/v1.20.2.tar.gz` (redirects to codeload.github.com) | tag | [200] (earlier onboardings found codeload blocked; it answers 200 from this sandbox) |
| binaries | none — GitHub releases ship **0 assets** | | [200] |
| (deps pinned by the chart) | `cilium-envoy` (`v1.37.6-<ts>-<sha>`), `certgen` (v0.4.11), `hubble-ui`/`hubble-ui-backend` (v0.13.6), `ztunnel` (v1.0.0), `startup-script` (timestamp) | independent projects, not release-versioned | [200] via values.yaml; docker.io/cilium/cilium-envoy:v1.20.2 = 404, confirming independence |

quay.io tag lists are Link-paginated (100/page) and full of cosign noise
(`sha256-*.sig/.att/.sbom`); the OCI adapter already follows pagination and filters
that noise.

## 6. Compatibility

- `Chart.yaml` `kubeVersion`: `">= 1.16.0-0"` through 1.15, `">= 1.21.0-0"` from
  1.16 on — the **minimum** Kubernetes; captured via chart-metadata content.
- `Documentation/network/kubernetes/compatibility.rst` at the tag: grid table,
  single row, `k8s Version` = comma list of the line's e2e-tested minors
  (v1.13.1: `1.16 … 1.26`; v1.16.12: `1.27 … 1.30`; v1.17.6: `1.29 … 1.32`;
  v1.20.2: `1.33, 1.34, 1.35, 1.36`). The page freezes per line at the tag
  (mutable on main). Ingested with markdown-table after rst conversion
  (key = the `k8s Version` column, kind `tested`).
- CI per-cloud k8s matrices live in `.github/actions/{aks,eks,gke}/k8s-versions.yaml`
  (per-branch YAML) — internal test config, not a support statement; not ingested.
- `compatibility-table.rst`: Cilium version → `io.cilium.k8s.crd.schema.version`
  (CNP/CCNP CRD schema); per-patch, simple rst table; not ingested (no slot for
  CRD-schema-version compatibility).
- Kernel requirements (>= 5.10 since 1.18, RHEL 8.10 4.18): prose in
  `system_requirements.rst` / upgrade notes bullets; captured as note items, not
  as platform constraints.

## 7. Hosts (from this sandbox, 2026-10-01)

Reachable: github.com (git + info/refs), raw.githubusercontent.com, quay.io
(/v2/ tags + manifests, anonymous), helm.cilium.io (index.yaml), api.github.com
(anonymous, 60 req/h), auth.docker.io + registry-1.docker.io (anonymous token
pull), hub.docker.com (tag API), mirror.gcr.io.
Blocked: none of the channels this definition needs (quay.io, helm.cilium.io,
api.github.com anonymous, registry-1.docker.io, codeload archive tarballs and
docs.cilium.io all answered 200 during this onboarding). docs.cilium.io
and cilium.io also answer 200 (rendered docs not needed — the rst sources carry
the evidence). Notably **quay.io and helm.cilium.io are reachable from this
sandbox today**, unlike the Phase-2 plan's standing assumption; the plan text
still calls them blocked, so definitions should not rely on that staying true
(canonical-first declaration is unaffected either way).
