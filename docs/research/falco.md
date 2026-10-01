# Falco release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could
not be checked here. `RAW` = `https://raw.githubusercontent.com/falcosecurity/falco`. Product: Falco,
the CNCF runtime-security **engine** (`github.com/falcosecurity/falco`): the userspace daemon
(licensing it "the engine" — rules evaluation, outputs, webserver/metrics), the kernel data-source
**drivers** it drives (kmod / legacy eBPF probe / modern_ebpf CO-RE, plus a dropped gVisor engine),
and its packaging (binary tarballs, deb/rpm, images, Helm chart). Onboarded as order 22, wave 3.

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, UNPREFIXED, minor lines.** Tags `0.3.0` → `0.45.0` (74 stable after excluding the
   two ancient `v0.1.0`/`v0.2.0`); pre-releases `0.X.0-alphaN|rcN` (47, `0.35.0-alpha1` is the only
   alpha family); junk: `0.6.0-test` and the **`agent/0.40.0`–`agent/0.85.1` family (73 tags of a
   different component tagged in the same repository)** — must be excluded by the tag pattern.
   Latest stable **0.45.0** (2026-09-21). There is **no 1.x era** (the "0.3x → 1.x old families" lead
   was wrong: the only old family is the v-prefixed v0.1.0/v0.2.0).
2. **Release notes are `CHANGELOG.md`** — one `## v0.X.Y` section per release (76 sections on
   `master`, back to `## v0.1.0`), grouped under H3 labels: `### Breaking Changes :warning:` (7
   releases have one; 2 more plain `### Breaking Changes`), `### Major Changes`, `### Minor
   Changes`, `### Bug Fixes`, `### Rule Changes`, `### Statistics`, `### Non user-facing changes`
   (skip), `### Security Fixes`. Items are conventional-commit bullets (`* chore!: drop gRPC output
   and server support [[#3798](…)] - [@ekoops](…)`). **Trap:** the changelog section for X is
   committed AFTER tag X about half the time (at tag 0.43.0 the newest section is v0.42.0; same lag
   at 0.40.0, 0.42.0, 0.43.1, 0.44.0, 0.44.1; in sync at 0.39.0, 0.41.0, 0.45.0) — so the section
   must be read from `master` (append-only cumulative document), not from the tag.
3. **GitHub release body** [200 via API, after the hourly quota reset] = the release template
   (`.github/release_template.md`): **LIBS and DRIVER version badges** linking to
   `falcosecurity/libs` releases, plus a packages download matrix pointing at `download.falco.org`.
   Release **assets** are only `falco-x86_64.debug` / `falco-aarch64.debug` (debug symbols) — the
   "binaries + source tarballs on GitHub release assets" lead is wrong for this era: binary/source
   packages ship from `download.falco.org` (RELEASE.md's claim that GitHub releases carry them is
   stale).
4. **The kernel-driver dimension is pinned per release in the repo and probe-able:**
   - `cmake/modules/driver.cmake` at the tag: `set(DRIVER_VERSION "11.0.0+driver")` — a **tag of
     `falcosecurity/libs`** (the driver source lives under `/driver` of the libs repo). Moves with
     (almost) every release: 0.38.0→7.2.0+driver, 0.39.0→7.3.0+driver, 0.40.0→8.0.0+driver,
     0.41.0→8.1.0+driver, 0.42.0→9.0.0+driver, 0.43.0/0.43.1→9.1.0+driver, 0.44.0/0.44.1→10.2.0+driver,
     0.45.0→11.0.0+driver. File exists since **0.33.0** (before that the driver was bundled with libs).
   - `cmake/modules/falcosecurity-libs.cmake`: `set(FALCOSECURITY_LIBS_VERSION "0.26.0")` — the libs
     tag pin (0.17.1@0.38.0 … 0.26.0@0.45.0). Pre-0.33.0 the pin is a commit **hash**, not a tag.
   - `userspace/engine/falco_engine_version.h`: `FALCO_ENGINE_VERSION` (0.65.0 @0.45.0) — internal
     engine-schema version, rules-compat relevant, mentioned in changelog breaking notes
     ("`FALCO_ENGINE_VERSION` bumped from `0.62.0` to `0.63.0`").
   - **Prebuilt-driver/kernel matrix IS machine-readable but not per-release**:
     `https://download.falco.org/driver/site/index.json` [200] lists driver versions;
     `…/driver/site/11.0.0%2Bdriver.json` [200, 2.4 MB] lists every prebuilt probe
     (`{lib, arch, kind (kmod), kernel, target (distro), name, download, lastmodified}`), snapshot
     `"date": "2026-10-01T14:08:09Z"` — a **mutable current-state document keyed by driver version**,
     with no per-falco-release history (kyverno's mutable-page rejection applies). Recorded as a gap.
   - Driver **kinds** per release are narrative: gRPC output + gVisor engine + legacy BPF probe
     deprecated in 0.43.0, **dropped in 0.44.0** (`chore!:` bullets in the v0.44.0 changelog section;
     chart `BREAKING-CHANGES.md` 9.0.0 distills the same). modern_ebpf needs kernel ≥ 5.8 or
     backported BTF (RELEASE.md, static statement).
5. **Helm chart: monorepo-published, dev-source moved into the falco repo at 0.44.0.** Chart
   `falco` versions 1.x → 9.2.0; 237 index entries; **every falco release has a chart whose
   `appVersion` == the release tag** (unprefixed string, e.g. `"0.45.0"`; multiple chart patches can
   re-publish one appVersion — 8.0.2…8.0.5 all have appVersion 0.43.1) → **lookup by appVersion ==
   {{.Tag}}, select latest**. Published at `https://falcosecurity.github.io/charts` [200] (gh-pages
   of the `falcosecurity/charts` MONOREPO, where chart tags `falco-N.N.N` are still cut — 235 tags)
   and as OCI charts at `ghcr.io/falcosecurity/charts/falco` [200]. Since 0.44.0 the chart SOURCE
   also lives in the falco repo (`chart/falco`, synced to the monorepo by `sync(charts/falco): vX`
   commits) — but **Chart.yaml at the falco tag is mid-cycle** (9.2.0-rc1/appVersion 0.45.0-rc2 at
   tag 0.45.0; `values.yaml` is already final and identical to the published 9.2.0) — the strimzi
   placeholder-Chart.yaml trap; the packaged chart from the index is the honest content source.
   `chart/falco/BREAKING-CHANGES.md` (in-repo ≥ 0.44.0, monorepo before) is a per-chart-MAJOR
   distillation — cumulative, misattributes if attached per release (rejected as a source).
6. **Packages on `download.falco.org`** — the verdict: **REACHABLE**; the host 307-redirects to its
   CloudFront distribution (`d20hasrqv82i0q.cloudfront.net`) which answers 200 anonymously (the
   crossplane-era "CloudFront blocked" no longer applies to this bucket). URL shapes (release
   template + probes [200] at 0.36.0/0.40.0/0.45.0):
   `packages/bin/x86_64/falco-{{TAG}}-x86_64.tar.gz`, `…/falco-{{TAG}}-static-x86_64.tar.gz`,
   `packages/deb/stable/falco-{{TAG}}-x86_64.deb` (note the `stable` suite path component, and that
   the deb path has it while the rpm does not), `packages/rpm/falco-{{TAG}}-x86_64.rpm`; aarch64
   twins exist. `FALCOBUCKET` in the template = `-dev` only for prereleases (canonical versions are
   stable-only, so the suffix never applies).
7. **Images: Docker Hub canonical** — `docker.io/falcosecurity/falco` and
   `docker.io/falcosecurity/falco-driver-loader` tagged `0.X.Y` (discovery validated both 5/5
   through the anonymous token flow; arch variants `x86_64-`/`aarch64-` and `-debian`/`-buster`
   suffixed tags exist per the publish workflow). `mirror.gcr.io/falcosecurity/{falco,
   falco-driver-loader}` also carries them [200 tags/list] — the redis/postgresql fallback pattern.
   Historical `falco-no-driver` is long gone (not published by the current workflow).
8. **falcoctl is a separate product, pinned by the chart**: `chart/falco/values.yaml`
   `falcoctl.image.tag: "0.14.2"` at the tag (0.12.2-era commits confirm it moves per release); the
   chart's init/sidecar containers run this image. Modeled as a pinned artifact (the
   kube-prometheus-stack/kyverno-api pin shape); falcoctl-the-product is NOT onboarded.
9. **Rules are a separate product** (`falcosecurity/rules`, submodule `submodules/falcosecurity-rules`
   pinned by gitlink — moves per release: c1b7e97@0.43.0, 4db0735@0.44.0, 7a8bfe4@0.45.0 — but a
   gitlink is not readable as a document, so the pin is a noted gap, not an artifact). Plugin refs in
   values (`container:0.7.4`, `k8smeta:0.4.2`) and subchart constraints (`falcosidekick 0.14.*`,
   `falco-talon 0.4.*`, `k8s-metacollector 0.3.*`) are separate products/constraints — noted, not
   modeled.
10. **Security**: GitHub repository advisories [200]: 8 published (1 CVE-mapped: CVE-2022-26316;
    severities low→critical). No other machine-readable feed.
11. **No upgrade-guide or support-policy channel exists** in this era: docs.falco.org is
    DNS-unreachable from the sandbox; the `falcosecurity/falco-website` repo has NO migration
    directory (checked the full tree) — the per-release `### Breaking Changes :warning:` changelog
    sections ARE the upgrade-guidance channel, and the chart majors' BREAKING-CHANGES.md distills
    them per chart major. No formal support/lifecycle policy is published (the website's
    `supported-doc-versions.md` is a DOCS-versioning statement: "current + six previous versions").

Related products (NOT this definition): `falcoctl` (artifact CLI, own repo/releases), the rules
file (`falcosecurity/rules`), plugins (`ghcr.io/falcosecurity/plugins/*`), `falcosidekick`,
`falco-talon`, `k8s-metacollector` (chart dependencies), `falcosecurity/libs` (the driver/libs
source — modeled only as the pinned tarball artifacts), `falcosecurity/charts` (the publishing
monorepo), `falcosecurity/falco-operator`, event-generator, driverkit, kernel-crawler.

## 1. Canonical versions

`git ls-remote --tags https://github.com/falcosecurity/falco` [200]: 195 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `0.X.Y` stable | 74 | `0.45.0` | releases (the engine), 0.3.0 → 0.45.0 |
| `0.X.0-rcN` (glued) | 42 | `0.45.0-rc4` | pre-releases (rc-only since 0.36; `-rc.N` never used) |
| `0.X.0-alphaN` | 5 | `0.35.0-alpha1` | the only alpha family (2023) |
| `agent/0.X.Y` | 73 | `agent/0.85.1` | a DIFFERENT component's tags (0.40.0–0.85.1, still advancing past the engine's 0.45.0) — junk for this product |
| `v0.1.0`, `v0.2.0`, `0.6.0-test` | 3 | | ancient v-prefixed family + a test tag |

- Pattern (as proposed by discovery, kept): `^(?P<version>\d+\.\d+\.\d+)$`, lineage `minor` —
  excludes rc/alpha, the `agent/*` family (slash), the v-prefixed ancients and the test tag.
- Cadence: a minor every ~2–3 months (0.43.0 2026-01, 0.44.0 2026-05, 0.45.0 2026-09); patches are
  rare and stop when the next minor is cut (0.44 line: 0.44.1 only). No documented support policy.

## 2. Release notes

- **`CHANGELOG.md`** [200 raw at tags and master]: per-release `## v0.X.Y` + `Released on DATE` +
  H3 label sections; conventional-commit bullets with PR link + author. 76 sections on `master`
  (v0.1.0 → v0.45.0). The **tag-lag trap** (section for X missing at tag X about half the time —
  0.40.0, 0.42.0, 0.43.0, 0.43.1, 0.44.0, 0.44.1 lag; 0.39.0, 0.41.0, 0.45.0 in sync) means the
  definition reads the section from `master` (append-only; content identical once committed — the
  0.43.0 section read at 0.43.1/master matches).
- **GitHub release body** [200 via API]: template-rendered LIBS/DRIVER badges + download matrix;
  no prose notes (the changelog is the notes channel; body kept as the API-quota-taxed fallback).
- Discovery's changelog proposal read at the tag and inferred `availability >= 0.45.0` from the lag
  — wrong generalization (it is a commit-timing coin-flip, not an era boundary); corrected to
  master + section select.

## 3. Upgrade guidance and breaking policy

- The per-release `### Breaking Changes :warning:` changelog sections are the channel (7+2
  releases carry one). Content is the `!`-marked conventional commits: 0.44.0 dropped gRPC
  output/server, gVisor engine, legacy BPF probe (all deprecated 0.43.0); 0.45.0 changed rule
  condition evaluation to raw field bytes and bumped `FALCO_ENGINE_VERSION` 0.62.0→0.63.0.
- `chart/falco/BREAKING-CHANGES.md` [200 in-tree ≥ 0.44.0; monorepo `charts/falco/` before]:
  per-chart-major sections (9.0.0, 8.0.0, …, 3.0.0) restating the same drops/deprecations with
  "Starting with Falco 0.44.0 …" phrasing — a distillation keyed by CHART major; attaching it per
  engine release would re-report old majors' drops (misattribution) — rejected as a source, noted.
- `docs.falco.org` = **DNS-unresolvable** from this sandbox (`Could not resolve host`); the
  website sources (`falcosecurity/falco-website`, Hugo, default branch `master`) contain no
  migration/upgrade pages — no per-version upgrade guide exists in this era.
- RELEASE.md [200 at tags]: release-process doc; carries the driver/model architecture notes
  (modern_ebpf bundled since 0.34.x, kernel ≥ 5.8 / backported BTF for CO-RE) — static statements,
  not per-release notes.

## 4. Security

- GitHub advisories [200]: 8 published on falcosecurity/falco — GHSA-7cq5-h4p2-h37p (low, 2025,
  io_uring detection gap), CVE-2022-26316/GHSA-6v9j-2vm2-ghf7 (medium, TOCTOU rule bypass), six
  2021-era GHSAs (undetected kmod crash, rules bypass, HTTP/Lua crashes). The `github-advisories`
  adapter works from here.
- No separate security page/feed (SECURITY.md → GitHub private vulnerability reporting).

## 5. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| image `falco` | `docker.io/falcosecurity/falco` (+ `mirror.gcr.io` twin) | tag == release | [200] 0.43.0–0.45.0 (discovery, 5/5); mirror tags/list [200] |
| image `falco-driver-loader` | `docker.io/falcosecurity/falco-driver-loader` (+ mirror) | tag == release | [200] 0.43.0–0.45.0 (discovery, 5/5) |
| Helm chart `falco` | `falcosecurity.github.io/charts` index + OCI `ghcr.io/falcosecurity/charts/falco` | **lookup**: chart release whose `appVersion` == `0.X.Y` (chart 9.2.0 ↔ 0.45.0; 9.0.0 ↔ 0.44.0; 8.0.x ↔ 0.43.x; …; 3.7.x ↔ 0.36.0) | [200] both; every checked release 0.36.0–0.45.0 has one |
| chart contents (values/Chart.yaml) | packaged chart from the index (published-chart-tgz) | the artifact's own channel | [200] values at falco tag 0.45.0 byte-identical to monorepo falco-9.2.0 |
| **driver source** | `github.com/falcosecurity/libs` archive `refs/tags/<N.N.N+driver>.tar.gz` | **pattern pin**: `set(DRIVER_VERSION "…+driver")` in `cmake/modules/driver.cmake` at the tag | [200] tag exists (7.2.0+driver@0.38.0 → 11.0.0+driver@0.45.0); file ≥ 0.33.0 |
| **libs source** | `github.com/falcosecurity/libs` archive `refs/tags/<N.N.N>.tar.gz` | **pattern pin**: `set(FALCOSECURITY_LIBS_VERSION "…")` in `cmake/modules/falcosecurity-libs.cmake` | [200] (0.17.1@0.38.0 → 0.26.0@0.45.0); tag-shaped ≥ 0.33.0 (commit-hash before) |
| `falcoctl` image pin | `docker.io/falcosecurity/falcoctl` | **field pin**: `falcoctl.image.tag` in `chart/falco/values.yaml` at the tag | [200] values at tag; discovery's tag==release probe failed (correctly — different product) |
| binary tarball | `download.falco.org/packages/bin/x86_64/falco-0.X.Y-x86_64.tar.gz` | tag in the name | [200] 0.36.0, 0.40.0, 0.45.0 (via CloudFront 307) |
| static binary tarball | `…/falco-0.X.Y-static-x86_64.tar.gz` | tag in the name | [200] 0.45.0 |
| deb | `download.falco.org/packages/deb/stable/falco-0.X.Y-x86_64.deb` | tag in the name | [200] 0.36.0, 0.40.0, 0.45.0 |
| rpm | `download.falco.org/packages/rpm/falco-0.X.Y-x86_64.rpm` | tag in the name | [200] 0.36.0, 0.40.0, 0.45.0 |
| source archive | `github.com/falcosecurity/falco/archive/refs/tags/0.X.Y.tar.gz` | the tag | [200] 0.45.0 |
| debug symbols | release assets `falco-{x86_64,aarch64}.debug` | at the tag | [200 via API listing]; not modeled (debugging aid, not an install artifact) |
| prebuilt kmod/ebpf probes | `download.falco.org/driver/<driver-version>/<arch>/falco_*.ko` (matrix JSON above) | keyed by DRIVER version + kernel release + distro | [200] index + per-version JSON; NOT modeled (mutable, per-driver-version, kernel-set-valued — gap) |

Not modeled on purpose: arch twins (aarch64) and `-debian`/`-buster` image suffixes (asset families
— the flux/kyverno gap), `.debug` assets, rules/plugin/subchart pins (separate products;
constraints), `agent/*` tags.

## 6. Compatibility

- **Driver/kernel**: the representable part is the per-release DRIVER/LIBS pin (pattern strategy →
  probe-able tarball instances: "falco-driver 10.2.0+driver → 11.0.0+driver [verified]"). The
  prebuilt-probe matrix (which kernel × distro × arch has a compiled kmod for which driver
  version) is machine-readable but mutable + keyed by driver version + set-valued per kernel —
  recorded as a gap (a source locator cannot render the driver-version URL from release context;
  the document has no per-release history).
- **modern_ebpf kernel floor** (≥ 5.8 / backported BTF) is a static statement in RELEASE.md, not a
  per-release table; driver-kind support changes (gVisor/legacy-BPF/gRPC drops in 0.44.0) surface
  through the changelog's breaking sections.
- The chart has no `kubeVersion` constraint (checked Chart.yaml at 0.45.0 — absent); chart-metadata
  contents still capture appVersion/version for divergence visibility.
- No per-release Kubernetes matrix exists (Falco is node-level, not Kubernetes-coupled).

## 7. Hosts (from this sandbox, 2026-10-01)

Reachable: github.com (git, raw, release downloads), codeload (via archive redirect), api.github.com
anonymous (60/h — exhausted once during discovery, hourly reset, transient), ghcr.io (anonymous
token), docker.io (anonymous token flow through the oci adapter; direct registry API also answers),
mirror.gcr.io, falcosecurity.github.io (charts index), **download.falco.org** (307 → CloudFront
`d20hasrqv82i0q.cloudfront.net`, 200 anonymously — the verdict: REACHABLE), falco.org [200].
Blocked: **docs.falco.org** (DNS resolution failure — recorded in `unreachableSources`; the
website content is available via the falco-website git repo, but holds no versioned upgrade docs).
