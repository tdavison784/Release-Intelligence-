# OpenTelemetry Collector release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this
sandbox. UNVERIFIED = could not be checked here. Product: the OpenTelemetry
Collector **distributions** published by
`github.com/open-telemetry/opentelemetry-collector-releases` — binaries
(`otelcol`, `otelcol-contrib`, `otelcol-k8s`, `otelcol-otlp`,
`otelcol-ebpf-profiler`, `otelcol-prometheus`), Linux packages (.deb/.rpm),
Windows .msi and container images (`otel/…` on Docker Hub, mirrored on ghcr).

## 0. Executive summary (what a declarative definition MUST know)

1. **Scope decision: the product is the releases repository.** What users
   deploy is not `github.com/open-telemetry/opentelemetry-collector` (the
   *core* framework, tags `v0.X.Y`, no release assets) but the distributions
   built and versioned by `github.com/open-telemetry/opentelemetry-collector-releases`
   (same `v0.X.Y` numbers). The core repo and
   `github.com/open-telemetry/opentelemetry-collector-contrib` (the ~200
   community components) are the *content* of the distributions and are
   modeled as sources/pins of this product, not as separate products. The
   **k8s Operator** (`github.com/open-telemetry/opentelemetry-operator`) is a
   further product with its own release train (v0.x.y) — out of scope, noted
   as a related product/gap.
2. **Three version surfaces, three prefixes** [git ls-remote + registry
   probes]: (a) repository tags are **v-prefixed** (`v0.162.0`; 157 stable
   tags, junk = `cmd/builder/v*`, `cmd/opampsupervisor/v*` subdirectory tags
   and `v0.N.N-nightly.YYYYMMDD…` nightlies); (b) **image tags are
   unprefixed** (`otel/opentelemetry-collector-contrib:0.162.0`, same on
   ghcr); (c) **binary/package asset versions are unprefixed**
   (`otelcol-contrib_0.162.0_linux_amd64.tar.gz`,
   `otelcol-contrib_0.162.0_linux_amd64.deb`). goreleaser's `{{ .Version }}`
   strips the `v` — hence `template: "{{.Version}}"` artifacts under a
   `tagPrefix: v` versioning.
3. **Lineage: every minor ships breaking changes.** Since v0.83 the
   changelogs are "user-facing changes only" with `### 🛑 Breaking changes 🛑`
   subsections on essentially every `0.X.0`; patches (`0.X.1+`, 32 of 157
   tags) are fix/rebuild only. → `lineage: minor` (an edge 0.160.0 → 0.162.0
   traverses 0.161.0 and 0.162.0).
4. **Three changelogs, three levels of the same release number** [200]:
   - `opentelemetry-collector-releases@tag CHANGELOG.md` — **distribution
     level** (`otelcol-contrib: Remove wavefrontreceiver from the contrib
     distribution`): what components enter/leave which distribution. Exists
     only since **v0.116.0** (404 at v0.115.0 and older); one anomaly: at tag
     v0.120.0 the section is mis-headed `## v0.120.1` (no such tag exists).
   - `opentelemetry-collector@tag CHANGELOG.md` — **core level** (config,
     confmap, service, `pkg/*`): headings are dual-versioned
     `## v1.68.0/v0.162.0` since v0.90.0 (before that the core repo has no
     root `v0.X.Y` tags — its old-era tags are subdirectory tags like
     `cmd/otelcorecol/v0.58.0`).
   - `opentelemetry-collector-contrib@tag CHANGELOG.md` — **component level**
     (per-receiver/exporter/processor changes; 66–171 items per release;
     headings plain `## v0.155.0` until 0.161.0, dual `## v1.1.0/v0.162.0`
     from 0.162.0 on, when contrib started tagging stable `v1.x` modules).
   The GitHub release body of the releases repo is **not** a fourth source:
   it is two links ("Check the v0.162.0 contrib changelog and the v0.162.0
   core changelog") plus 3 release-prep commit subjects.
5. **The core/contrib version pins live in `distributions/*/manifest.yaml`
   at the release tag** [200]: every component is
   `gomod: go.opentelemetry.io/collector/… v0.162.0` /
   `gomod: github.com/open-telemetry/opentelemetry-collector-contrib/… v0.162.0`
   and `dist.version: 0.162.0`. So core and contrib are pinned **at the same
   number** as the distribution release (`version.strategy: template` is the
   honest relation), with one structural exception: **from v0.102.1 on, patch
   releases are releases-repo-only** (21 tags: 0.102.1 … 0.152.1 have no
   core/contrib tag; the 0.132.2 manifest pins core **v0.132.0** — the parent
   minor). Before 0.90.0 core/contrib root tags don't exist at all.
   `version.strategy: field` cannot read these pins: the version shares one
   string with the module path (`gomod: … v0.162.0`) and the selector
   (`[field=value]`) is exact-match only — recorded as a gap; the lockstep
   template + existence channel represents the same fact.
6. **Security**: GitHub repository security advisories, and they live where
   the code is: `open-telemetry/opentelemetry-collector-contrib` (7 published
   GHSA) and `…/opentelemetry-collector` (1); the **releases repo itself has
   0 advisories** [api.github.com, 200]. The org's security process is GitHub
   private vulnerability reporting (no SECURITY.md at the repo roots; 404).
7. **Artifacts** [probes below]: images `otel/opentelemetry-collector{,-contrib,-k8s,-otlp}`
   on Docker Hub (canonical for users; anonymous token flow answered 200
   during this onboarding) + the same six distributions on
   `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector*`
   (tags unprefixed). Availability floors of the ghcr repos: otelcol and
   contrib = full history (≥0.44/≥0.63 verified), k8s **≥ 0.98.0**, otlp
   **≥ 0.112.0**, ebpf-profiler **≥ 0.135.0** (verified window), prometheus
   **= 0.162.0** (new at 0.162.0, experimental). Binaries/packages on the
   GitHub releases of the releases repo, `otelcol[-contrib]_<version>_<os>_<arch>`
   with `.tar.gz`, `.deb`, `.rpm` (also `.msi`, `.apk` in old eras) — same
   naming since at least v0.90.0. **Upstream anomaly at 0.162.0**: the
   multi-arch index tag `0.162.0` of `otel/opentelemetry-collector-contrib`
   is absent from Docker Hub, mirror.gcr.io **and** ghcr (MANIFEST_UNKNOWN)
   while `0.162.0-amd64`/`-arm64` and `latest` exist on ghcr — the per-arch
   images were pushed, the index tag was not.
8. **No upgrade guide / no machine-readable compatibility matrix.** There is
   no per-release upgrade document (opentelemetry.io/docs/collector/upgrade →
   404); the migration discipline is the `🛑 Breaking changes` changelog
   sections plus `otelcol validate` (the `check`-style config validation
   command, documented at /docs/collector/configuration/). The
   collector-to-core matrix does not exist as a file: it is the lockstep pin
   of fact 5. The **kubernetes** matrix (which collector versions the OTel
   operator supports) lives in the operator repo — a different product.

## 1. Canonical versions

- `git ls-remote https://github.com/open-telemetry/opentelemetry-collector-releases`
  [200]: 390 tags; **157 stable `v0.X.Y`** (v0.0.1 → v0.162.0), 233 junk
  (`cmd/builder/v*` and `cmd/opampsupervisor/v*` subdirectory tags of the
  same numbers + `v0.N.N-nightly.<ts>`). Strict pattern
  `^v(?P<version>\d+\.\d+\.\d+)$`.
- Stable-tag cross-check of the three repos [200]: core has 194 stable root
  tags, contrib 188, releases 157. Releases ⊄ core: the whole 0.34–0.89 era
  (core/contrib used subdirectory tags then) **plus every patch ≥ 0.102.1**.
  Core-only extras: 112 (the 0.2–0.33 era before the releases repo existed,
  e.g. v0.2.0…v0.33.x, plus a few of its own patches).
- GitHub releases exist per stable tag (release-asset downloads + atom feed
  [200]) → fallback versions source.

## 2. Release notes / changelogs [200]

- releases repo `CHANGELOG.md` at the tag, `## v0.X.Y` sections, subsections
  `### 🛑 Breaking changes 🛑` / `### 🚀 New components 🚀` /
  `### 💡 Enhancements 💡` (+ occasional `### 🧰 Bug fixes 🧰`). Distribution
  membership changes are THE breaking unit of the product ("Remove
  wavefrontreceiver from the contrib distribution"). Since v0.116.0;
  v0.120.0's section is mis-headed `v0.120.1` (upstream anomaly).
- core repo `CHANGELOG.md` at the same-numbered tag, `## v1.N.M/v0.X.Y`
  headings (dual since v0.90.0), the same subsection vocabulary, entries
  prefixed `service/telemetry`, `pkg/confighttp`, `processor/queue_batch`, …
- contrib repo `CHANGELOG.md` at the same-numbered tag, `## v0.X.Y` (dual
  `## v1.1.0/v0.162.0` from 0.162.0 on), per-component entries
  (`exporter/elasticsearch`, `receiver/filelog`, …), 66–171 bullets per
  release, migration prose as continuation lines under the bullet.
- GitHub release body (releases repo): links to the core+contrib changelog
  releases + 3 prep commits — a pointer document, not a notes source.

## 3. Upgrade guidance

None as a document (no /docs/collector/upgrade page [404]). The 🛑 sections
carry migration instructions inline ("Migrate `num_workers` to
`sending_queue::num_consumers`…", "Disable with `skip_context_propagation:
true`"), and `otelcol validate --config` [docs, 200] is the mechanical
pre-flight. Modeled via classify rules (breaking → action-required candidates)
rather than an upgrade-guide source; recorded as a gap.

## 4. Security

- `github-advisories` on contrib (7) and core (1) [200]; releases repo: 0.
  No SECURITY.md at repo roots [404 both] — org-wide GitHub private
  vulnerability reporting.

## 5. Artifacts

- **Broken releases (upstream anomalies found by `ri check -n 50`, verified
  2026-10-01)**: **0.129.0, 0.134.0, 0.150.0** published *nothing* — the
  tags and their changelog sections exist, but the GitHub releases have no
  assets and no image tags exist on any registry (0.150.0 is partial: the
  otlp image exists); each was re-cut as the follow-up patch (0.129.1,
  0.134.1, 0.150.1), whose changelog section carries the changes (at tag
  v0.140.0 the section is even headed `## v0.140.1`; 0.146.0/0.150.0
  sections never existed at any tag). Partial contrib failures: **0.115.0**
  (no otelcol-contrib assets, no contrib image anywhere; otelcol shipped),
  **0.123.0** (no contrib release assets, ghcr image exists), **0.115.1**
  (contrib assets only; no k8s assets). Distribution-only patches without
  distribution-level changes have no releases-CHANGELOG section (0.116.1,
  0.122.1, 0.129.1, 0.132.4, 0.134.1), while those with changes do
  (0.140.1, 0.146.1, 0.150.1). Core changelog headings are dual
  `## v1.N.M/v0.X.Y` but the dual prefix is sometimes added only after the
  tag freezes: at tag v0.146.0 the section is plain `## v0.146.0`.

- **Images** (tags unprefixed, `template: "{{.Version}}"`):
  - `docker.io/otel/opentelemetry-collector` and `…-contrib`, `…-k8s`,
    `…-otlp` [200 via registry-1.docker.io anonymous token + mirror.gcr.io];
    canonical channel for users (docs/README use `otel/…`). No
    `otel/opentelemetry-collector-ebpf-profiler` / `-prometheus` /
    `-opampsupervisor` / `-builder` on Docker Hub were probed (UNVERIFIED;
    ghcr is their only verified host).
  - `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector{,-contrib,-k8s,-otlp,-ebpf-profiler,-prometheus,-opampsupervisor,-builder}`
    [200 anonymous token]. Discovery historically-validated
    core/k8s/otlp/ebpf/opampsupervisor/builder at 0.159–0.162; prometheus
    starts at 0.162.0; **contrib 0.162.0 index missing** (see summary 7).
  - `mirror.gcr.io/otel/…` answers for the four docker.io repos (nonstandard
    tags/list shape but manifest probes work) — the fallback pattern.
- **Binaries/packages** (GitHub release assets of the releases repo):
  `otelcol_<v>_<os>_<arch>.tar.gz` + `.deb` + `.rpm` (+ windows `.msi`) and
  the same for `otelcol-contrib_`; `otelcol-k8s`/`otelcol-ebpf-profiler`/
  `otelcol-otlp`/`otelcol-prometheus` ship `.tar.gz` only (k8s also .deb?
  no — tar.gz only at 0.162.0). Checksums/sigstore/SBOM sidecars exist per
  asset. Naming stable since ≥ v0.90.0 (older eras also had `.apk`; the
  signing sidecar changed `.pem/.sig` → `.sigstore.json`).
- **Core collector** (the pin): `https://github.com/open-telemetry/opentelemetry-collector/archive/refs/tags/v0.X.Y.tar.gz`
  [200 for lockstep tags] — the existence check for "core released at this
  number". Distribution-only patches (21) and everything < 0.90.0 have no
  such tag [404].
- **Builder (OCB) / opampsupervisor**: same tag train, own images/binaries
  (`cmd/builder/*`, `cmd/opampsupervisor/*` subdirectory tags) — separate
  tools (custom-distribution builder; OpAMP supervisor library), rejected as
  artifacts of this product (see record).

## 6. Compatibility

- No machine-readable matrix file in any of the three repos
  (distributions/README.md is prose policy: supported-distribution criteria,
  component-removal rule "unmaintained components are kept six months").
- The real relation is structural: contrib manifest pins core at the
  distribution version (fact 5); the OTel operator's collector support
  matrix (which collector versions each operator supports) is in the
  **operator** repo — related product, out of scope.

## 7. Hosts (from this sandbox, 2026-10-01)

- [200] github.com (git, releases HTML, expanded_assets), raw.githubusercontent.com,
  codeload-adjacent archive/refs endpoints, ghcr.io (anonymous token),
  mirror.gcr.io (anonymous token), opentelemetry.io, api.github.com
  (anonymous, 60/h; 50 remaining after this research; discovery + a handful
  of advisory calls consumed the rest — treated as transient).
- [401 anonymous / quota-class] registry-1.docker.io direct manifest probes
  return 401 without a token; with an anonymous auth.docker.io token they
  answered 200 during this onboarding (rate-limit flakiness remains the
  documented norm — PostgreSQL's 429s — so mirror.gcr.io + ghcr stay
  declared as fallback channels).
- [307-class] not applicable here; no registry.k8s.io use.

## 8. Discovery baseline (what `ri discover` got right/wrong)

Right: versioning (strict pattern, v-prefix, 157 stable), the ghcr image
family (6 of 8 proposed historically-validated), the releases CHANGELOG.md
heading shape, github-advisories default, github-releases fallback. Wrong or
incomplete: it **excluded the contrib ghcr image as "test-only"** (it is
referenced from a goreleaser config the rule misclassified), its binary
asset templates used the repo name (`opentelemetry-collector-releases_…`)
instead of `otelcol-contrib_…` and a `.Binary` template variable that does
not exist (static-validation failures), it proposed the OCB builder and
opampsupervisor images as product artifacts (separate tools), and it found
no core/contrib relationship at all (the topology fact 5 is invisible to a
single-repo scanner). No helm chart exists (correctly not-found); no
upgrade-docs/compatibility (correct).
