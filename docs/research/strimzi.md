# Strimzi release-channel research (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the checks and probes were re-run live; the live `ri check` outcomes are unchanged. Any unreachable host recorded below was a transient anonymous rate limit or an anonymous-auth refusal, not a block of this product's channels (details: docs/rerun/REPORT.md, per-product table). The original text below is kept as the record of 2026-10-01.

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox.
UNVERIFIED = could not be checked here. `RAW` =
`https://raw.githubusercontent.com/strimzi/strimzi-kafka-operator`. Product:
Strimzi, the Apache Kafka operator for Kubernetes (cluster operator + topic/user
operators + operand images + HTTP bridge pinning). Canonical repository
`github.com/strimzi/strimzi-kafka-operator`.

## 0. Executive summary (what a declarative definition MUST know)

1. **Plain semver, no prefix, minor lines.** Tags `0.X.Y` and, since 2026-06,
   `1.X.Y` (1.0.0 was the `v1`-CRD-API breaking release). 182 tags: 81 stable,
   100 pre-releases (`N.N.N-rcM`), 1 junk (`v0.2.0-rc1`, ancient, v-prefixed).
   Latest stable: **1.2.0**. Minors are the feature/breaking unit in both the
   0-major and the 1-major era: every `N.M.0` changelog section carries a
   `### Major changes, deprecations, and removals` subsection (`### Changes,
   deprecations and removals` before ~0.46) while patches list fixes only, and
   `documentation/modules/upgrading/con-upgrade-paths.adoc` defines the upgrade
   path as "consecutive minor versions". → `lineage: minor` (a 0.45.2 → 0.47.0
   edge traverses 0.46.0); the 0-major scheme needs no special casing.
2. **Release notes = CHANGELOG.md in the repository** [200], one `## <version>`
   section per release (bare version, no `v`), newest first, at the release's
   own tag; exists for every checked era (0.12.0 → 1.2.0) including patches
   (`## 0.50.1`). The GitHub Release body is **not** the changelog section: it
   is a curated announcement ("Important ⚠️" v1-only API warning for 1.x,
   "Main Changes Since X.Y", Maven artifact coordinates, and a
   **Container Images table with sha256 digests**) — richer callouts, largely
   restating the changelog; declared as the fallback of the changelog.
3. **Documentation is AsciiDoc, in-tree `documentation/`** [200] (since ≤0.36,
   previously `strimzi/strimzi.github.io`). It is generic procedures
   (assemblies/modules: "Upgrading the Cluster Operator", "Required upgrade
   sequence", upgrade paths), parameterised by AsciiDoc attributes
   (`{ProductVersion}`, `{KubernetesVersion}`) generated from
   `kafka-versions.yaml`; there is **no per-release section** in the docs.
   Per-release upgrade intelligence lives in the CHANGELOG's "Major changes…"
   subsection → **no `format: adoc` extraction construct is needed** (see
   record: considered and rejected; the second AsciiDoc product, Elasticsearch
   in wave 3, should decide on actual evidence).
4. **Compatibility = `kafka-versions.yaml`** [200] at the repo root, a YAML
   **list of records** (`version`, `supported: true/false`, `default:
   true/false`, `metadata`, `url`, `checksum`) — the single place that drives
   builds, the operator's image map and the docs (its own header says so).
   1.2.0: supported 4.2.0/4.2.1/4.3.0/4.3.1, default **4.3.1**; 1.1.0: default
   4.3.0; 1.0.x: default 4.2.0; 0.45.0: default 3.9.0. The same file exists
   (same record shape) at 0.22.0; at ≤0.12 it was `kafka-versions` (no
   extension). `KAFKA_VERSION_SUPPORT.md` states the policy (last two Kafka
   minors, one common Kafka version across consecutive Strimzi minors). The
   **Kubernetes minimum** is only an AsciiDoc attribute
   (`documentation/shared/attributes.adoc`: `:KubernetesVersion: 1.30 and
   later` at 1.2.0, `1.25 and later` at 0.45.0) plus changelog prose ("From
   Strimzi 0.51 on, we support only Kubernetes 1.30 and newer") → captured as
   classified notes, not a constraint (recorded gap).
5. **Security**: GitHub repository security advisories; patch changelog
   sections lead with `**Fixes CVE-… (GHSA-…)…**` bullets (1.0.1 fixes
   CVE-2026-55225/55226). api.github.com worked anonymously for discovery but
   the 60/h quota was exhausted during this onboarding (HTML release pages and
   raw files still answer).
6. **Artifacts.** Helm chart `strimzi-kafka-operator` (chart version ==
   appVersion == release version; 65 OCI tags 0.x → 1.2.0) canonical on
   **OCI `quay.io/strimzi-helm/strimzi-kafka-operator`** (upstream deprecated
   the classic repo — CHANGELOG notice) with `https://strimzi.io/charts`
   (alive, newest 1.2.0; tgz hosted as GitHub release assets) as the
   alternative. The **in-tree Chart.yaml is a placeholder** (`version: 0.1.1`,
   `appVersion: 0.1.0` at every tag — rewritten by packaging), so chart
   metadata must come from the published chart, not the source tree. Images on
   `quay.io/strimzi/*` with docker.io/strimzi mirrors: `operator`,
   `kaniko-executor`, `maven-builder` (release tags, all eras), `buildah`
   (≥ 0.49.0), `jmxtrans` (< 0.35.0, dropped in 0.35.0); `kafka` is a
   **matrix artifact** tagged `<release>-kafka-<kafka-version>` (1.2.0:
   `1.2.0-kafka-4.3.1` etc. — also pushed with a build counter
   `<release>-0-kafka-<kv>`); `kafka-bridge` is an independently versioned
   companion (own train, pinned per release by chart values
   `kafkaBridge.image.tag` and the root `bridge.version` scalar). CRDs ship as
   the release asset `strimzi-crds-<v>.yaml` (multi-doc, 10–13 CRDs) and
   inside the chart/`packaging/install`. Install bundle `strimzi-<v>.zip` per
   release. **Drain Cleaner, Access Operator and Kafka Bridge are separate
   projects** (own repositories and version trains: 1.6.1, 0.3.0, 1.1.0)
   pinned into Strimzi's install files — not artifacts of this product.

## 1. Canonical versions

`git ls-remote --tags` (partial clone) [200]: 182 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `0.X.Y` / `1.X.Y` | 81 | `1.2.0`, `0.51.0` | stable releases (bare semver, no `v`) |
| `N.N.N-rcM` | 100 | `1.2.0-rc2` | pre-releases (excluded from the canonical list) |
| junk | 1 | `v0.2.0-rc1` | ancient tag with `v` prefix — excluded |

- Lineage evidence for `minor`: docs upgrade paths are "between consecutive
  minor versions"; changelog minors carry the breaking subsection; patches are
  fixes-only. Strimzi does re-patch older lines out of order (0.45.2 after
  0.46.0) but the upgrade path stays minor-sequential.
- 0-major semantics: nothing structural changes at 1.0.0 for the model — the
  same minor-driven cadence continues (1.1.0, 1.2.0 each with "Major changes"
  sections); `lineage: minor` treats (0,51) → (1,0) as the next line.
- `release.version` at the tag carries the release string (placeholder on
  main); tags are the canonical channel, GitHub releases mirror them.

## 2. Release notes (CHANGELOG.md) [200]

`RAW/<tag>/CHANGELOG.md` — one `## <version>` section per release:

```
# CHANGELOG
## 1.2.0
* Add support for Apache Kafka 4.3.1
* ...
### Major changes, deprecations, and removals     <- subsection (1.x name)
* The Cluster, Topic, and User Operator YAML installation files ... Restricted
  Kubernetes Pod Security Standard ...
```

- Subsection name eras: `### Changes, deprecations and removals` (≤ ~0.45),
  `### Major changes, deprecations and removals` (1.x). Both match
  `(?i)changes, deprecations`.
- Bullets are plain `*` with continuation lines; security fixes are bold
  (`**Fixes CVE-… GHSA-…**`); Kafka support changes are explicit bullets
  ("Add support for Apache Kafka 4.3.1", "Remove support for Kafka 4.1.x").
- GitHub Release body (HTML page [200]; API quota exhausted — see §7): curated
  announcement, different from the changelog section: `⚠️ Important` v1-only
  CRD API warning (1.x releases), "Main Changes Since X.Y.Z" highlights,
  "Upgrading from Strimzi X.Y" pointer to docs, Maven coordinates and a
  Container Images table with `name → quay.io/strimzi/*@sha256:…` digests
  (operator, kafka × supported versions, bridge, kaniko, buildah,
  maven-builder). Declared as release-notes fallback only, to avoid
  double-counting the restated highlights.

## 3. Upgrade guidance (AsciiDoc, generic) [200]

- `documentation/assemblies/upgrading/assembly-upgrade*.adoc` + modules:
  procedure docs (OLM/YAML/Helm upgrade steps, Kafka upgrade order,
  downgrades). Same content every release, parameterised by attributes; no
  per-release sections exist.
- `documentation/modules/upgrading/con-upgrade-sequence.adoc`: the required
  order (supported Kubernetes → KRaft migration (pre-0.39) → v1 API conversion
  (pre-0.49 `KafkaUser` step) → Cluster Operator → Kafka `version`/
  `metadataVersion`) — version-gated prose, not per-release records.
- `documentation/shared/version-dependent-attrs.adoc` is **generated** from
  `kafka-versions.yaml` (`:DefaultKafkaVersion: 4.3.1` at 1.2.0); the k8s
  minimum sits in `documentation/shared/attributes.adoc`
  (`:KubernetesVersion: 1.30 and later`).
- Conclusion: every per-release upgrade fact is already in CHANGELOG.md
  (markdown) or kafka-versions.yaml (YAML). An `extract.format: adoc` renderer
  would have no load-bearing content to read for this product → not built
  (record: constructs.considered).

## 4. Security

- `github-advisories` for `strimzi/strimzi-kafka-operator` (the changelog's
  CVE bullets link `github.com/strimzi/strimzi-kafka-operator/security/advisories/GHSA-…`).
  api.github.com anonymous worked for discovery, then hit the 60/h cap during
  research [HTML pages still 200].

## 5. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| Helm chart `strimzi-kafka-operator` | OCI `quay.io/strimzi-helm/strimzi-kafka-operator` (canonical); `https://strimzi.io/charts` index (tgz = GitHub release asset `strimzi-kafka-operator-helm-3-chart-<v>.tgz`) | chart version == appVersion == release version (65 OCI tags, 0.x → 1.2.0) | [200] both |
| chart values | `packaging/helm-charts/helm3/strimzi-kafka-operator/values.yaml` at the tag | real per-release defaults (`defaultImageTag: 1.2.0`, `kafkaBridge.image.tag: 1.1.0`, securityContext…) | [200] |
| chart metadata | in-tree `Chart.yaml` is a **placeholder** (`version: 0.1.1`, `appVersion: 0.1.0` at 1.2.0 and older tags); the published chart carries the real version | — | [200]; not captured (no chart-kubeVersion is set either) |
| image `operator` | `quay.io/strimzi/operator` + `docker.io/strimzi/operator` mirror | release tag | [200] manifest 1.2.0; docker.io token pull 200 |
| image `kaniko-executor` | `quay.io/strimzi/kaniko-executor` (+ mirror) | release tag (0.22.0 → 1.2.0) | [200] |
| image `maven-builder` | `quay.io/strimzi/maven-builder` (+ mirror) | release tag | [200] |
| image `buildah` | `quay.io/strimzi/buildah` (+ mirror) | release tag, **≥ 0.49.0** (env introduced then) | [200] |
| image `jmxtrans` | `quay.io/strimzi/jmxtrans` | release tag, **< 0.35.0** (dropped in 0.35.0) | [200] 0.34.0 |
| image `kafka` | `quay.io/strimzi/kafka` | **matrix artifact**: tag `<release>-kafka-<kafkaVersion>` (also `<release>-0-kafka-<kv>` build-counter variants); install manifest references `<rel>-kafka-<default>` | [200] both tag forms; not modelable as one version relation (gap) |
| image `kafka-bridge` | `quay.io/strimzi/kafka-bridge` | independent train (1.0.0/1.1.0/1.2.0…), pinned per release by chart `kafkaBridge.image.tag` (and `bridge.version`) | [200] 1.1.0 |
| CRDs | release asset `strimzi-crds-<v>.yaml` (multi-doc; `packaging/install/**/04x-Crd-*.yaml` and chart `crds/` are the same definitions) | release version | [200] 0.38.0 → 1.2.0 |
| install bundle | release asset `strimzi-<v>.zip` (install YAMLs + examples) | release version | [200] 0.38.0 → 1.2.0 |
| install manifests | `packaging/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml` at the tag (`install/` tree mirrors `packaging/install/`) | at the tag; embeds every default image env (`STRIMZI_KAFKA_IMAGES` map, bridge/kaniko/buildah/maven pins) | [200] |
| (pinned companions) | Drain Cleaner (`strimzi/drain-cleaner`, 1.6.1), Access Operator (`strimzi/kafka-access-operator`, 0.3.0), test-client | separate products with own trains | [200] via install files/changelog |

quay.io tag lists are paginated (100/page, lexicographic) and include
`-amd64/-arm64/-ppc64le/-s390x` per-arch tags and `-rcN` pre-releases; the
plain release tags are what the definition probes.

## 6. Compatibility

- `kafka-versions.yaml` [200]: root YAML list, one record per Kafka version
  ever shipped (`version`, `supported`, `default`, plus build metadata). The
  supported set and the default are per-release facts; dropping a Kafka
  version is a breaking change announced in the changelog ("Remove support
  for Kafka 4.1.x" in 1.1.0).
- `KAFKA_VERSION_SUPPORT.md` [200]: support policy (≥ last two Kafka minors;
  ≥ one common Kafka version across consecutive Strimzi minors — the reason
  Kafka-version drops lag).
- Kubernetes minimum: `:KubernetesVersion:` AsciiDoc attribute + changelog
  bullets (0.51.0: "From Strimzi 0.51 on, we support only Kubernetes 1.30 and
  newer"); no machine-readable file → notes only (gap).
- Java/OLM constraints: OLM channel `stable`; `api` modules need Java 21
  (1.0.0) — changelog notes.

## 7. Hosts (from this sandbox, 2026-10-01)

Reachable: github.com (git + release-asset downloads, 206 range requests),
raw.githubusercontent.com, quay.io (/v2 tags + manifests, anonymous, with
Accept header), quay.io/strimzi-helm (chart manifests), strimzi.io/charts
(index.yaml + tgz via github release assets), auth.docker.io +
registry-1.docker.io (anonymous token pull), github.com HTML release pages.
**Rate-limited during this onboarding**: api.github.com anonymous (60/h
consumed by `ri discover` before research began; earlier onboardings recorded
it working — the github-releases fallback and the advisories source may
report `unavailable` while the cap holds; git tags and raw files carry the
canonical channels). Not needed: codeload tarballs (release assets suffice),
OLM/OperatorHub, Maven Central.
