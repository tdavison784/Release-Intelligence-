# Discovery report: elasticsearch

- Repository: `github.com/elastic/elasticsearch` at `v9.5.4` (commit `9170df19cae1…`)
- Generated: 2026-10-02T12:33:40Z
- LLM: not used (deterministic resolver only)
- Tags: 516 tags: prefix "v", 476 stable, 28 prereleases (alphaN×11, betaN×8, rcN×9), 12 junk; latest stable v9.5.4; lineage minor
- Scanned `github.com/elastic/elasticsearch@v9.5.4` (source profile): 44098 files listed, 6047 read
- Scanned `github.com/elastic/docs@master` (docs profile): 597 files listed, 5 read
- Validation: done against v9.3.0, v9.3.8, v9.4.0, v9.4.7, v9.5.0, v9.5.4

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 1 | git-tags github.com/elastic/elasticsearch |
| release-notes | found | 1 | repo-dir github.com/elastic/elasticsearch docs/changelog |
| changelog | not-found | 0 |  |
| helm-charts | not-found | 0 |  |
| registries | candidates-only | 3 |  |
| images | candidates-only | 26 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories elastic/elasticsearch |
| version-relations | not-found | 0 |  |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| source:release-note-files | historically-validated | notes.structured-dir | https://github.com/elastic/elasticsearch/blob/v9.5.4/docs/changelog/158545.yaml# |
| source:advisories | discovered | security.github-hosted-default |  |
| source:tags | discovered | versions.git-tags | https://github.com/elastic/elasticsearch/releases/tag/v9.5.4#refs/tags/v9.5.4 |
| versioning | discovered | tags.scheme | https://github.com/elastic/elasticsearch/releases/tag/v9.5.4#refs/tags/v9.5.4 |

Statuses: 1 historically-validated, 3 discovered.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: elasticsearch
name: elasticsearch
homepage: https://github.com/elastic/elasticsearch
versioning:
  scheme: semver
  tagPrefix: v
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/elastic/elasticsearch
  - id: release-note-files
    roles:
      - release-notes
    locator:
      kind: repo-dir
      repository: github.com/elastic/elasticsearch
      baseRef: '{{.PrevTag}}'
      path: docs/changelog
      glob: '*.yaml'
    extract:
      type: release-note-yaml
    fallbackGroup: release-notes
    validatedAgainst:
      - 9.3.0
      - 9.3.8
      - 9.4.0
      - 9.4.7
      - 9.5.0
      - 9.5.4
    notes: Structured per-change notes accumulate in this directory; a release's notes are the files added since the previous release.
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: elastic/elasticsearch
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - v9.3.0
    - v9.3.8
    - v9.4.0
    - v9.4.7
    - v9.5.0
    - v9.5.4
  notes: Proposed by automated discovery; relationship checks run against v9.3.0, v9.3.8, v9.4.0, v9.4.7, v9.5.0, v9.5.4.
```

## Open questions for the reviewer

- Ambiguity (docs-sources): Documentation lives in another repository; no upgrade-guide, compatibility source was derived from it.

## Validation matrix

| Element | Verdict | v9.3.0 | v9.3.8 | v9.4.0 | v9.4.7 | v9.5.0 | v9.5.4 |
|---|---|---|---|---|---|---|---|
| source:release-note-files | validated |  |  |  |  |  |  |

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 516 tags: prefix "v", 476 stable, 28 prereleases (alphaN×11, betaN×8, rcN×9), 12 junk; latest stable v9.5.4; lineage minor |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (516 tags: prefix "v", 476 stable, 28 prereleases (alphaN×11, betaN×8, rcN×9), 12 junk; latest stable v9.5.4; lineage minor). |
| source:release-note-files | include | notes.structured-dir | heuristic | Directory of 22 structured note files (keys: summary). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |

<details><summary>26 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| image `docker.io/family/elasticsearch-ubuntu-2404` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `docker.io/family/elasticsearch-ubuntu-2204` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `docker.elastic.co/ci-agent-images/eck-region/buildkite-agent` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `docker.io/family/elasticsearch-windows-2022` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `docker.elastic.co/release-eng/wolfi-build-essential-release-eng` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.elastic.co/elasticsearch-ci/elasticsearch-cloud-ess` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `docker.elastic.co/infra/release-manager` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/library/traefik` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/prometheus` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/curlimages/curl` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.elastic.co/kibana/kibana` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/grafana/grafana` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/alpine` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.elastic.co/elasticsearch/elasticsearch` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.elastic.co/integrations/elastic-connectors` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.elastic.co/integrations/data-extraction-service` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.elastic.co/elasticsearch-dev/es-rust-cross-toolchain` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `docker.elastic.co/elasticsearch-infra/es-rust-cross-toolchain` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.elastic.co/elasticsearch-infra/es-native-cross-toolchain` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/library/elasticsearch` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/library/haproxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image-name `zstd` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `ess` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `rust-toolchain` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `cross-toolchain` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `cargo-zigbuild` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### docs-repo (1)

- `github.com/elastic/docs` (medium; docs.docs-repo-ref) — [elastic/elasticsearch:rest-api-spec/README.markdown L51](https://github.com/elastic/elasticsearch/blob/v9.5.4/rest-api-spec/README.markdown#L51): `build](https://github.com/elastic/docs#building-documentation).`

### image (21)

- `docker.elastic.co/elasticsearch-infra/es-native-cross-toolchain` (high; script.image-ref) — [elastic/elasticsearch:libs/simdvec/native/build_cross_toolchain_image.sh L33](https://github.com/elastic/elasticsearch/blob/v9.5.4/libs/simdvec/native/build_cross_toolchain_image.sh#L33): `IMAGE=$HOST/$REPOSITORY:$VERSION`
- `docker.elastic.co/elasticsearch-infra/es-rust-cross-toolchain` (high; script.image-ref) — [elastic/elasticsearch:libs/parquet-rs/native/publish_pqrs_binaries.sh L45](https://github.com/elastic/elasticsearch/blob/v9.5.4/libs/parquet-rs/native/publish_pqrs_binaries.sh#L45): `TOOLCHAIN_IMAGE="docker.elastic.co/elasticsearch-infra/es-rust-cross-toolchain:1"`
- `docker.elastic.co/ci-agent-images/eck-region/buildkite-agent` (medium; manifest.image) — [elastic/elasticsearch:.buildkite/pipelines/java-ea-check-new-build.yml L6](https://github.com/elastic/elasticsearch/blob/v9.5.4/.buildkite/pipelines/java-ea-check-new-build.yml#L6): `image: "docker.elastic.co/ci-agent-images/eck-region/buildkite-agent:1.15"`
- `docker.elastic.co/elasticsearch-ci/elasticsearch-cloud-ess` (medium; script.image-ref) — [elastic/elasticsearch:.buildkite/scripts/cloud-deploy.sh L8](https://github.com/elastic/elasticsearch/blob/v9.5.4/.buildkite/scripts/cloud-deploy.sh#L8): `DOCKER_TAG="docker.elastic.co/elasticsearch-ci/elasticsearch-cloud-ess:${ES_VERSION}-${BUILDKITE_COMMIT:0:7}"`
- `docker.elastic.co/elasticsearch-dev/es-rust-cross-toolchain` (medium; script.image-ref) — [elastic/elasticsearch:libs/parquet-rs/native/build_rust_toolchain_image.sh L35](https://github.com/elastic/elasticsearch/blob/v9.5.4/libs/parquet-rs/native/build_rust_toolchain_image.sh#L35): `IMAGE=$HOST/$REPOSITORY:$VERSION`
- `docker.elastic.co/elasticsearch/elasticsearch` (medium; docs.image-ref) — [elastic/elasticsearch:docs/reference/elasticsearch/mapping-reference/_snippets/docker-gpu-indexing.md L2](https://github.com/elastic/elasticsearch/blob/v9.5.4/docs/reference/elasticsearch/mapping-reference/_snippets/docker-gpu-indexing.md#L2): `FROM docker.elastic.co/elasticsearch/elasticsearch:9.3.0`
- `docker.elastic.co/infra/release-manager` (medium; script.image-ref) — [elastic/elasticsearch:.buildkite/scripts/dra-workflow.sh L111](https://github.com/elastic/elasticsearch/blob/v9.5.4/.buildkite/scripts/dra-workflow.sh#L111): `docker.elastic.co/infra/release-manager:latest \`
- `docker.elastic.co/integrations/data-extraction-service` (medium; docs.image-ref) — [elastic/elasticsearch:docs/reference/search-connectors/es-connectors-content-extraction.md L112](https://github.com/elastic/elasticsearch/blob/v9.5.4/docs/reference/search-connectors/es-connectors-content-extraction.md#L112): `docker.elastic.co/integrations/data-extraction-service:$EXTRACTION_SERVICE_VERSION`
- `docker.elastic.co/integrations/elastic-connectors` (medium; docs.image-ref) — [elastic/elasticsearch:docs/reference/search-connectors/api-tutorial.md L256](https://github.com/elastic/elasticsearch/blob/v9.5.4/docs/reference/search-connectors/api-tutorial.md#L256): `docker.elastic.co/integrations/elastic-connectors:{{version.stack}} \`
- `docker.elastic.co/release-eng/wolfi-build-essential-release-eng` (medium; manifest.image) — [elastic/elasticsearch:.buildkite/pipelines/version-bump-pipeline.yml L39](https://github.com/elastic/elasticsearch/blob/v9.5.4/.buildkite/pipelines/version-bump-pipeline.yml#L39): `image: docker.elastic.co/release-eng/wolfi-build-essential-release-eng:latest`
- `docker.io/family/elasticsearch-ubuntu-2204` (medium; manifest.image) — [elastic/elasticsearch:.buildkite/pipelines/dra-workflow.yml L9](https://github.com/elastic/elasticsearch/blob/v9.5.4/.buildkite/pipelines/dra-workflow.yml#L9): `image: family/elasticsearch-ubuntu-2204`
- `docker.io/family/elasticsearch-ubuntu-2404` (medium; manifest.image) — [elastic/elasticsearch:.buildkite/pipelines/agentic-workflow.yml L62](https://github.com/elastic/elasticsearch/blob/v9.5.4/.buildkite/pipelines/agentic-workflow.yml#L62): `image:          family/elasticsearch-ubuntu-2404`
- `docker.io/family/elasticsearch-windows-2022` (medium; manifest.image) — [elastic/elasticsearch:.buildkite/pipelines/pull-request/part-1-windows.yml L9](https://github.com/elastic/elasticsearch/blob/v9.5.4/.buildkite/pipelines/pull-request/part-1-windows.yml#L9): `image: family/elasticsearch-windows-2022`
- `docker.io/library/elasticsearch` (medium; manifest.image) — [elastic/elasticsearch:qa/remote-clusters/docker-compose-oss.yml L5](https://github.com/elastic/elasticsearch/blob/v9.5.4/qa/remote-clusters/docker-compose-oss.yml#L5): `image: elasticsearch:test`
- `docker.io/library/haproxy` (medium; manifest.image) — [elastic/elasticsearch:qa/remote-clusters/docker-compose-oss.yml L72](https://github.com/elastic/elasticsearch/blob/v9.5.4/qa/remote-clusters/docker-compose-oss.yml#L72): `image: haproxy:2.1.2`
- `docker.elastic.co/kibana/kibana` (low; manifest.image) — [elastic/elasticsearch:dev-tools/prometheus-local/docker-compose.yml L64](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/prometheus-local/docker-compose.yml#L64): `image: docker.elastic.co/kibana/kibana:9.6.0-SNAPSHOT`
- `docker.io/curlimages/curl` (low; manifest.image) — [elastic/elasticsearch:dev-tools/prometheus-local/docker-compose.yml L40](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/prometheus-local/docker-compose.yml#L40): `image: curlimages/curl:latest`
- `docker.io/grafana/grafana` (low; manifest.image) — [elastic/elasticsearch:dev-tools/prometheus-local/docker-compose.yml L84](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/prometheus-local/docker-compose.yml#L84): `image: grafana/grafana:latest`
- `docker.io/library/alpine` (low; manifest.image) — [elastic/elasticsearch:dev-tools/prometheus-local/docker-compose.yml L107](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/prometheus-local/docker-compose.yml#L107): `image: alpine:latest`
- `docker.io/library/traefik` (low; manifest.image) — [elastic/elasticsearch:dev-tools/prometheus-local/docker-compose.yml L3](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/prometheus-local/docker-compose.yml#L3): `image: traefik:v2.11`
- `docker.io/prom/prometheus` (low; manifest.image) — [elastic/elasticsearch:dev-tools/prometheus-local/docker-compose.yml L23](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/prometheus-local/docker-compose.yml#L23): `image: prom/prometheus:latest`

### image-name (5)

- `cargo-zigbuild` (low; dockerfile.name) — [elastic/elasticsearch:x-pack/plugin/esql-datasource-parquet-rs/native/build-tools/Dockerfile.cargo-zigbuild L12](https://github.com/elastic/elasticsearch/blob/v9.5.4/x-pack/plugin/esql-datasource-parquet-rs/native/build-tools/Dockerfile.cargo-zigbuild#L12): `FROM docker.elastic.co/elasticsearch-dev/es-rust-cross-toolchain:1.95 AS toolchain`
- `cross-toolchain` (low; dockerfile.name) — [elastic/elasticsearch:libs/simdvec/native/Dockerfile.cross-toolchain L21](https://github.com/elastic/elasticsearch/blob/v9.5.4/libs/simdvec/native/Dockerfile.cross-toolchain#L21): `FROM debian:trixie-slim`
- `ess` (low; dockerfile.name) — [elastic/elasticsearch:distribution/docker/src/docker/Dockerfile.ess L1](https://github.com/elastic/elasticsearch/blob/v9.5.4/distribution/docker/src/docker/Dockerfile.ess#L1): `FROM ${base_image} AS builder`
- `rust-toolchain` (low; dockerfile.name) — [elastic/elasticsearch:libs/parquet-rs/native/Dockerfile.rust-toolchain L20](https://github.com/elastic/elasticsearch/blob/v9.5.4/libs/parquet-rs/native/Dockerfile.rust-toolchain#L20): `FROM debian:trixie-slim`
- `zstd` (low; dockerfile.name) — [elastic/elasticsearch:dev-tools/zstd.Dockerfile L1](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/zstd.Dockerfile#L1): `FROM redhat/ubi8`

### registry (3)

- `ghcr.io` (medium; workflow.registry-login) — [elastic/elasticsearch:.github/workflows/updatecli-compose.yml L48](https://github.com/elastic/elasticsearch/blob/v9.5.4/.github/workflows/updatecli-compose.yml#L48): `registry: ghcr.io`
- `ghcr.io/v2/homebrew/core/zstd/blobs/sha256` (low; script.registry-ref) — [elastic/elasticsearch:dev-tools/publish_zstd_binaries.sh L39](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/publish_zstd_binaries.sh#L39): `curl -sS --retry 3 -H "Authorization: Bearer QQ==" --output "$OUTPUT_FILE" --location "https://ghcr.io/v2/home…`
- `ghcr.io/v2/homebrew/core/zstd/manifests` (low; script.registry-ref) — [elastic/elasticsearch:dev-tools/publish_zstd_binaries.sh L35](https://github.com/elastic/elasticsearch/blob/v9.5.4/dev-tools/publish_zstd_binaries.sh#L35): `--location "https://ghcr.io/v2/homebrew/core/zstd/manifests/$VERSION" | jq -r \`

### release-notes-dir (1)

- `docs/changelog` (high; notes.structured-dir) — [elastic/elasticsearch:docs/changelog/158545.yaml ](https://github.com/elastic/elasticsearch/blob/v9.5.4/docs/changelog/158545.yaml): `directory docs/changelog (22 files)`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (high; tags.ls-remote) — [elastic/elasticsearch/releases/tag/v9.5.4 refs/tags/v9.5.4](https://github.com/elastic/elasticsearch/releases/tag/v9.5.4#refs/tags/v9.5.4): `9170df19cae1adb107b7b489b4d82dec66d7a337 refs/tags/v9.5.4`
