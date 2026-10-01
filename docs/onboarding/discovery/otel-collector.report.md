# Discovery report: opentelemetry-collector-releases

- Repository: `github.com/open-telemetry/opentelemetry-collector-releases` at `v0.162.0` (commit `f6159a775dad…`)
- Generated: 2026-10-01T14:43:43Z
- LLM: not used (deterministic resolver only)
- Tags: 390 tags: prefix "v", 157 stable, 0 prereleases (), 233 junk; latest stable v0.162.0; lineage linear
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+)$` (junk sample: cmd/builder/v0.107.0, cmd/builder/v0.108.0, cmd/builder/v0.109.0, cmd/builder/v0.110.0, cmd/builder/v0.111.0, cmd/builder/v0.112.0)
- Scanned `github.com/open-telemetry/opentelemetry-collector-releases@v0.162.0` (source profile): 176 files listed, 130 read
- Validation: done against v0.159.0, v0.160.0, v0.161.0, v0.162.0

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 29 | git-tags github.com/open-telemetry/opentelemetry-collector-releases; github-releases open-telemetry/opentelemetry-collector-releases |
| release-notes | found | 0 | github-releases open-telemetry/opentelemetry-collector-releases  @{{.Tag}} |
| changelog | found | 1 | repo-file github.com/open-telemetry/opentelemetry-collector-releases CHANGELOG.md |
| helm-charts | not-found | 0 |  |
| registries | found | 2 | ghcr.io/open-telemetry/opentelemetry-collector-releases |
| images | found | 12 | ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector:{{.Version}}; ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-builder:{{.Version}}; ghcr.io/open-telemetry/o… |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories open-telemetry/opentelemetry-collector-releases |
| version-relations | found | 0 | opentelemetry-collector: version = {{.Version}}; opentelemetry-collector-builder: version = {{.Version}}; opentelemetry-collector-ebpf-profiler: version = {{.Version}}; opentelemetry-collector-k8s: version = {{.Version}}… |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:opentelemetry-collector | historically-validated | image.tag-from-release | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol/.goreleaser.yaml#L159 |
| artifact:opentelemetry-collector-builder | historically-validated | image.tag-from-release | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L94 |
| artifact:opentelemetry-collector-ebpf-profiler | historically-validated | image.tag-from-release | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-ebpf-profiler/.goreleaser.yaml#L70 |
| artifact:opentelemetry-collector-k8s | historically-validated | image.tag-from-release | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-k8s/.goreleaser.yaml#L85 |
| artifact:opentelemetry-collector-opampsupervisor | historically-validated | image.tag-from-release | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/opampsupervisor/.goreleaser.yaml#L130 |
| artifact:opentelemetry-collector-otlp | historically-validated | image.tag-from-release | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-otlp/.goreleaser.yaml#L155 |
| source:changelog | historically-validated | changelog.file | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/CHANGELOG.md#L7 |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.goreleaser.yaml#L4 (+2) |
| artifact:binary-aix-ppc64-if-arm-v-end-if-mips-binary | unverified | asset.hosted-release-download | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93 |
| artifact:binary-if-arm-v-end-if-mips-binary | unverified | asset.hosted-release-download | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93 |
| artifact:binary-riscv64-if-arm-v-end-if-mips-binary | unverified | asset.hosted-release-download | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93 |
| artifact:opentelemetry-collector-prometheus | unverified | image.tag-from-release | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-prometheus/.goreleaser.yaml#L131 |
| artifact:opentelemetry-collector-releases-aix-ppc64-binary | unverified | asset.hosted-release-download | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser-build.yaml#L1 |
| artifact:opentelemetry-collector-releases-binary | unverified | asset.hosted-release-download | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161 (+1) |
| artifact:opentelemetry-collector-releases-riscv64-binary | unverified | asset.hosted-release-download | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161 |
| source:advisories | discovered | security.github-hosted-default |  |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.goreleaser.yaml#L4 (+2) |
| source:tags | discovered | versions.git-tags | https://github.com/open-telemetry/opentelemetry-collector-releases/releases/tag/v0.162.0#refs/tags/v0.162.0 |
| versioning | discovered | tags.scheme | https://github.com/open-telemetry/opentelemetry-collector-releases/releases/tag/v0.162.0#refs/tags/v0.162.0 (+1) |

Statuses: 8 historically-validated, 4 discovered, 7 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: otel-collector
name: opentelemetry-collector-releases
homepage: https://github.com/open-telemetry/opentelemetry-collector-releases
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+)$
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/open-telemetry/opentelemetry-collector-releases
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: open-telemetry/opentelemetry-collector-releases
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: open-telemetry/opentelemetry-collector-releases
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/open-telemetry/opentelemetry-collector-releases
      path: CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: open-telemetry/opentelemetry-collector-releases
artifacts:
  - id: opentelemetry-collector
    type: container-image
    name: opentelemetry-collector
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
  - id: opentelemetry-collector-builder
    type: container-image
    name: opentelemetry-collector-builder
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-builder
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
  - id: opentelemetry-collector-ebpf-profiler
    type: container-image
    name: opentelemetry-collector-ebpf-profiler
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-ebpf-profiler
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
  - id: opentelemetry-collector-k8s
    type: container-image
    name: opentelemetry-collector-k8s
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-k8s
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
  - id: opentelemetry-collector-opampsupervisor
    type: container-image
    name: opentelemetry-collector-opampsupervisor
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-opampsupervisor
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
  - id: opentelemetry-collector-otlp
    type: container-image
    name: opentelemetry-collector-otlp
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-otlp
    validatedAgainst:
      - 0.159.0
      - 0.160.0
      - 0.161.0
      - 0.162.0
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - v0.159.0
    - v0.160.0
    - v0.161.0
    - v0.162.0
  notes: Proposed by automated discovery; relationship checks run against v0.159.0, v0.160.0, v0.161.0, v0.162.0.
```

## Validation matrix

| Element | Verdict | v0.159.0 | v0.160.0 | v0.161.0 | v0.162.0 |
|---|---|---|---|---|---|
| artifact:binary-aix-ppc64-if-arm-v-end-if-mips-binary | unverifiable |  |  |  |  |
| artifact:binary-if-arm-v-end-if-mips-binary | unverifiable |  |  |  |  |
| artifact:binary-riscv64-if-arm-v-end-if-mips-binary | unverifiable |  |  |  |  |
| artifact:opentelemetry-collector | validated |  |  |  |  |
| artifact:opentelemetry-collector-builder | validated |  |  |  |  |
| artifact:opentelemetry-collector-ebpf-profiler | validated |  |  |  |  |
| artifact:opentelemetry-collector-k8s | validated |  |  |  |  |
| artifact:opentelemetry-collector-opampsupervisor | validated |  |  |  |  |
| artifact:opentelemetry-collector-otlp | validated |  |  |  |  |
| artifact:opentelemetry-collector-prometheus | failing |  |  |  |  |
| artifact:opentelemetry-collector-releases-aix-ppc64-binary | failing |  |  |  |  |
| artifact:opentelemetry-collector-releases-binary | failing |  |  |  |  |
| artifact:opentelemetry-collector-releases-riscv64-binary | failing |  |  |  |  |
| source:changelog | validated |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |

## Dropped elements

- `artifact:opentelemetry-collector-releases-binary` (deterministic): failed validation: 0.159.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.159.0/opentelemetry-collector-releases_0.159.0_linux_amd64); 0.160.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.160.0/opentelemetry-collector-releases_0.160.0_linux_amd64); 0.161.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.161.0/opentelemetry-collector-releases_0.161.0_linux_amd64); 0.162.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.162.0/opentelemetry-collector-releases_0.162.0_linux_amd64)
- `artifact:opentelemetry-collector-releases-aix-ppc64-binary` (deterministic): failed validation: 0.159.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.159.0/opentelemetry-collector-releases_0.159.0_aix_ppc64); 0.160.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.160.0/opentelemetry-collector-releases_0.160.0_aix_ppc64); 0.161.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.161.0/opentelemetry-collector-releases_0.161.0_aix_ppc64); 0.162.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.162.0/opentelemetry-collector-releases_0.162.0_aix_ppc64)
- `artifact:opentelemetry-collector-releases-riscv64-binary` (deterministic): failed validation: 0.159.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.159.0/opentelemetry-collector-releases_0.159.0_linux_riscv64); 0.160.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.160.0/opentelemetry-collector-releases_0.160.0_linux_riscv64); 0.161.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.161.0/opentelemetry-collector-releases_0.161.0_linux_riscv64); 0.162.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.162.0/opentelemetry-collector-releases_0.162.0_linux_riscv64)
- `artifact:opentelemetry-collector-prometheus` (deterministic): failed validation: 0.159.0 fail (absent from ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-prometheus:0.159.0); 0.160.0 fail (absent from ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-prometheus:0.160.0); 0.161.0 fail (absent from ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-prometheus:0.161.0)
- `artifact:binary-aix-ppc64-if-arm-v-end-if-mips-binary` (deterministic): static validation: error: artifacts[0].channels[0]: template "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_aix_ppc64{{ if .Arm }}v{{ end }}{{ if .Mips }}_{{ .Mips }}{{ end }}.tar.gz": template: :1:97: executing "" at <.Binary>: can't evaluate field Binary in type catalog.RenderContext
- `artifact:binary-riscv64-if-arm-v-end-if-mips-binary` (deterministic): static validation: error: artifacts[0].channels[0]: template "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_riscv64{{ if .Arm }}v{{ end }}{{ if .Mips }}_{{ .Mips }}{{ end }}.tar.gz": template: :1:97: executing "" at <.Binary>: can't evaluate field Binary in type catalog.RenderContext
- `artifact:binary-if-arm-v-end-if-mips-binary` (deterministic): static validation: error: artifacts[0].channels[0]: template "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_amd64{{ if .Arm }}v{{ end }}{{ if .Mips }}_{{ .Mips }}{{ end }}.tar.gz": template: :1:97: executing "" at <.Binary>: can't evaluate field Binary in type catalog.RenderContext

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 390 tags: prefix "v", 157 stable, 0 prereleases (), 233 junk; latest stable v0.162.0; lineage linear; strict tagPattern excludes junk tags such as cmd/builder/v0.107.0, cmd/builder/v0.108.0, cmd/builder/v0.109.0, cmd/bui… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (390 tags: prefix "v", 157 stable, 0 prereleases (), 233 junk; latest stable v0.162.0; lineage linear). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 0.162.0 (heading "## v0.162.0"). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:opentelemetry-collector-releases-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (11 reference(s)). |
| artifact:opentelemetry-collector-releases-aix-ppc64-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:opentelemetry-collector-releases-riscv64-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:binary-aix-ppc64-if-arm-v-end-if-mips-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:binary-riscv64-if-arm-v-end-if-mips-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:binary-if-arm-v-end-if-mips-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (11 reference(s)). |
| artifact:opentelemetry-collector | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:opentelemetry-collector-builder | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:opentelemetry-collector-ebpf-profiler | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:opentelemetry-collector-k8s | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:opentelemetry-collector-opampsupervisor | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:opentelemetry-collector-otlp | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:opentelemetry-collector-prometheus | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:opentelemetry-collector-releases-binary | drop | validate.failed | computed | Relationship check failed: 0.159.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.159.0/opentelemetry-collector-releases_0.159.0_linux_amd64); 0.160.0 fail (abse… |
| artifact:opentelemetry-collector-releases-aix-ppc64-binary | drop | validate.failed | computed | Relationship check failed: 0.159.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.159.0/opentelemetry-collector-releases_0.159.0_aix_ppc64); 0.160.0 fail (absent… |
| artifact:opentelemetry-collector-releases-riscv64-binary | drop | validate.failed | computed | Relationship check failed: 0.159.0 fail (absent from https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/v0.159.0/opentelemetry-collector-releases_0.159.0_linux_riscv64); 0.160.0 fail (ab… |
| artifact:binary-aix-ppc64-if-arm-v-end-if-mips-binary | annotate | validate.unverifiable | computed | Kept unverified: 0.159.0 unverifiable (not verifiable: http channel (http: error)); 0.160.0 unverifiable (not verifiable: http channel (http: error)); 0.161.0 unverifiable (not verifiable: http channel (http: error)); 0.… |
| artifact:binary-riscv64-if-arm-v-end-if-mips-binary | annotate | validate.unverifiable | computed | Kept unverified: 0.159.0 unverifiable (not verifiable: http channel (http: error)); 0.160.0 unverifiable (not verifiable: http channel (http: error)); 0.161.0 unverifiable (not verifiable: http channel (http: error)); 0.… |
| artifact:binary-if-arm-v-end-if-mips-binary | annotate | validate.unverifiable | computed | Kept unverified: 0.159.0 unverifiable (not verifiable: http channel (http: error)); 0.160.0 unverifiable (not verifiable: http channel (http: error)); 0.161.0 unverifiable (not verifiable: http channel (http: error)); 0.… |
| artifact:opentelemetry-collector-prometheus | drop | validate.failed | computed | Relationship check failed: 0.159.0 fail (absent from ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-prometheus:0.159.0); 0.160.0 fail (absent from ghcr.io/open-telemetry/opentelemetry-col… |
| artifact:binary-aix-ppc64-if-arm-v-end-if-mips-binary | drop | proposer.static-validation | computed | error: artifacts[0].channels[0]: template "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_aix_ppc64{{ if .Arm }}v{{ end }}{{ if .Mips }}_{{ .Mips … |
| artifact:binary-riscv64-if-arm-v-end-if-mips-binary | drop | proposer.static-validation | computed | error: artifacts[0].channels[0]: template "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_riscv64{{ if .Arm }}v{{ end }}{{ if .Mips }}_{{ .M… |
| artifact:binary-if-arm-v-end-if-mips-binary | drop | proposer.static-validation | computed | error: artifacts[0].channels[0]: template "https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_amd64{{ if .Arm }}v{{ end }}{{ if .Mips }}_{{ .Mip… |

<details><summary>5 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| image `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/open-telemetry/opentelemetry-collector-contrib/golden` | image.test-only | Only referenced from test, sample or documentation files. |
| image-name `windows` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `test.deb` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `test.rpm` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### build-tool (2)

- `goreleaser` (high; goreleaser.config, workflow.goreleaser) — [open-telemetry/opentelemetry-collector-releases:.github/workflows/base-binary-release.yaml L154](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.github/workflows/base-binary-release.yaml#L154): `uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3`
- `cosign` (medium; workflow.cosign) — [open-telemetry/opentelemetry-collector-releases:.github/workflows/base-binary-release.yaml L111](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.github/workflows/base-binary-release.yaml#L111): `- uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2`

### changelog (1)

- `CHANGELOG.md` (high; docs.changelog) — [open-telemetry/opentelemetry-collector-releases:CHANGELOG.md L7](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/CHANGELOG.md#L7): `## v0.162.0`

### image (9)

- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector` (high; goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol/.goreleaser.yaml L159](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol/.goreleaser.yaml#L159): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector:{{ .Version }}-386`
- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-builder` (high; goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L94](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L94): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-builder:{{ .Version }}-amd64`
- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib` (high; docs.image-ref, goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L167](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L167): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-contrib:{{ .Version }}-386`
- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-ebpf-profiler` (high; goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-ebpf-profiler/.goreleaser.yaml L70](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-ebpf-profiler/.goreleaser.yaml#L70): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-ebpf-profiler:{{ .Version }}…`
- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-k8s` (high; goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-k8s/.goreleaser.yaml L85](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-k8s/.goreleaser.yaml#L85): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-k8s:{{ .Version }}-amd64`
- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-opampsupervisor` (high; goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:cmd/opampsupervisor/.goreleaser.yaml L130](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/opampsupervisor/.goreleaser.yaml#L130): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-opampsupervisor:{{ .Version …`
- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-otlp` (high; goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-otlp/.goreleaser.yaml L155](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-otlp/.goreleaser.yaml#L155): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-otlp:{{ .Version }}-386`
- `ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-prometheus` (high; goreleaser.image) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-prometheus/.goreleaser.yaml L131](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-prometheus/.goreleaser.yaml#L131): `- ghcr.io/open-telemetry/opentelemetry-collector-releases/opentelemetry-collector-prometheus:{{ .Version }}-am…`
- `ghcr.io/open-telemetry/opentelemetry-collector-contrib/golden` (low; docs.image-ref, manifest.image) — [open-telemetry/opentelemetry-collector-releases:tests/golden/README.md L26](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/tests/golden/README.md#L26): `4716ece9c21e   ghcr.io/open-telemetry/opentelemetry-collector-contrib/golden:latest                           …`

### image-name (3)

- `test.deb` (low; dockerfile.name) — [open-telemetry/opentelemetry-collector-releases:scripts/package-tests/Dockerfile.test.deb L3](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/scripts/package-tests/Dockerfile.test.deb#L3): `FROM debian:13@sha256:9cc080028c43b27d2074d63a5f9caf7166d731494965616c1a6d2827a004585c`
- `test.rpm` (low; dockerfile.name) — [open-telemetry/opentelemetry-collector-releases:scripts/package-tests/Dockerfile.test.rpm L3](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/scripts/package-tests/Dockerfile.test.rpm#L3): `FROM rockylinux/rockylinux:10.2@sha256:827d37bc128288ccf160ee318bb3cb92d591164cb217e92f8bc61e3982ae1834`
- `windows` (low; dockerfile.name) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/Windows.dockerfile L4](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/Windows.dockerfile#L4): `FROM mcr.microsoft.com/windows/nanoserver:ltsc${WIN_VERSION}@${WIN_VERSION_SHA}`

### registry (2)

- `docker.io` (medium; workflow.registry-login) — [open-telemetry/opentelemetry-collector-releases:.github/workflows/base-binary-release.yaml L137](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.github/workflows/base-binary-release.yaml#L137): `uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0`
- `ghcr.io` (medium; workflow.registry-login) — [open-telemetry/opentelemetry-collector-releases:.github/workflows/base-binary-release.yaml L151](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.github/workflows/base-binary-release.yaml#L151): `command: echo "$GHCR_TOKEN" | docker login ghcr.io -u "${{ github.repository_owner }}" --password-stdin`

### release-asset (26)

- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser-build.yaml L1](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser-build.yaml#L1): `# yaml-language-server: $schema=https://goreleaser.com/static/schema-pro.json`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L161](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161): `- name_template: otel/opentelemetry-collector-builder:{{ .Version }}`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L161](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161): `- name_template: otel/opentelemetry-collector-builder:{{ .Version }}`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser-build.yaml L1](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser-build.yaml#L1): `# yaml-language-server: $schema=https://goreleaser.com/static/schema-pro.json`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L161](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161): `- name_template: otel/opentelemetry-collector-builder:{{ .Version }}`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser-build.yaml L1](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser-build.yaml#L1): `# yaml-language-server: $schema=https://goreleaser.com/static/schema-pro.json`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L161](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161): `- name_template: otel/opentelemetry-collector-builder:{{ .Version }}`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L161](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161): `- name_template: otel/opentelemetry-collector-builder:{{ .Version }}`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L161](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161): `- name_template: otel/opentelemetry-collector-builder:{{ .Version }}`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser-build.yaml L1](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser-build.yaml#L1): `# yaml-language-server: $schema=https://goreleaser.com/static/schema-pro.json`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser-build.yaml L1](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser-build.yaml#L1): `# yaml-language-server: $schema=https://goreleaser.com/static/schema-pro.json`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:cmd/builder/.goreleaser.yaml L161](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/cmd/builder/.goreleaser.yaml#L161): `- name_template: otel/opentelemetry-collector-builder:{{ .Version }}`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/opentelemetry-collector-re…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser-build.yaml L1](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser-build.yaml#L1): `# yaml-language-server: $schema=https://goreleaser.com/static/schema-pro.json`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- `https://github.com/open-telemetry/opentelemetry-collector-releases/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}…` (high; goreleaser.archive) — [open-telemetry/opentelemetry-collector-releases:distributions/otelcol-contrib/.goreleaser.yaml L93](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/distributions/otelcol-contrib/.goreleaser.yaml#L93): `name_template: '{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}{{ if .Arm }}v{{ .Arm }}{{ end }}{{ if .Mips…`
- … 1 more

### release-publisher (1)

- `goreleaser` (high; goreleaser.release) — [open-telemetry/opentelemetry-collector-releases:.goreleaser.yaml L4](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.goreleaser.yaml#L4): `release:`

### release-trigger (1)

- `v*` (high; workflow.tag-trigger) — [open-telemetry/opentelemetry-collector-releases:.github/workflows/release-builder.yaml L5](https://github.com/open-telemetry/opentelemetry-collector-releases/blob/v0.162.0/.github/workflows/release-builder.yaml#L5): `tags: ["v*"]`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+)$` (high; tags.ls-remote) — [open-telemetry/opentelemetry-collector-releases/releases/tag/v0.162.0 refs/tags/v0.162.0](https://github.com/open-telemetry/opentelemetry-collector-releases/releases/tag/v0.162.0#refs/tags/v0.162.0): `f6159a775dad1e21433c49bd8450673dd13ed4d3 refs/tags/v0.162.0`
