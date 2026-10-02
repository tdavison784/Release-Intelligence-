# Discovery report: falco

- Repository: `github.com/falcosecurity/falco` at `0.45.0` (commit `05d5b3e363d5…`)
- Generated: 2026-10-02T12:34:37Z
- LLM: not used (deterministic resolver only)
- Tags: 195 tags: prefix "", 76 stable, 47 prereleases (alphaN×5, rcN×42), 72 junk; latest stable 0.45.0; lineage minor
- Strict tag pattern: `^(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|rc\d+))?)$` (junk sample: 0.6.0-test, agent/0.40.0, agent/0.41.0, agent/0.42.0, agent/0.43.0, agent/0.44.0)
- Scanned `github.com/falcosecurity/falco@0.45.0` (source profile): 384 files listed, 115 read
- Scanned `github.com/falcosecurity/falco-website@master` (docs profile): 1062 files listed, 36 read
- Validation: done against 0.43.0, 0.43.1, 0.44.0, 0.44.1, 0.45.0

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 2 | git-tags github.com/falcosecurity/falco; github-releases falcosecurity/falco |
| release-notes | found | 0 | github-releases falcosecurity/falco  @{{.Tag}} |
| changelog | found | 2 | repo-file github.com/falcosecurity/falco CHANGELOG.md |
| helm-charts | found | 5 | falco via helm-repo:https://falcosecurity.github.io/charts; falco via helm-repo:https://falcosecurity.github.io/charts, helm-git:github.com/falcosecurity/charts |
| registries | found | 3 | docker.io/falcosecurity |
| images | found | 18 | docker.io/falcosecurity/falco:{{.Tag}}; docker.io/falcosecurity/falco-driver-loader:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories falcosecurity/falco |
| version-relations | found | 0 | falco-chart: independent; falco-chart-2: lookup appVersion == {{.Tag}}; falco: version = {{.Tag}}; falco-driver-loader: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:falco | historically-validated | image.tag-assumed-release | https://github.com/falcosecurity/falco/blob/0.45.0/.github/release_template.md#L16 |
| artifact:falco-chart-2 | historically-validated | chart.external-lookup-appversion | https://github.com/falcosecurity/falco/blob/0.45.0/ADOPTERS.md#L75 (+1) |
| artifact:falco-driver-loader | historically-validated | image.tag-assumed-release | https://github.com/falcosecurity/falco/blob/0.45.0/.github/release_template.md#L18 |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/release.yaml#L211 |
| artifact:falco-chart | unverified | helm.independent-version | https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/Chart.yaml#L2 (+1) |
| artifact:falcoctl | unverified | image.tag-assumed-release | https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/values.yaml#L547 |
| source:advisories | discovered | security.github-hosted-default |  |
| source:changelog | exception | changelog.file | https://github.com/falcosecurity/falco/blob/0.45.0/CHANGELOG.md#L3 |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/release.yaml#L211 |
| source:tags | discovered | versions.git-tags | https://github.com/falcosecurity/falco/releases/tag/0.45.0#refs/tags/0.45.0 |
| versioning | discovered | tags.scheme | https://github.com/falcosecurity/falco/releases/tag/0.45.0#refs/tags/0.45.0 |

Statuses: 4 historically-validated, 4 discovered, 2 unverified, 1 exception.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: falco
name: falco
homepage: https://github.com/falcosecurity/falco
versioning:
  scheme: semver
  tagPattern: ^(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|rc\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/falcosecurity/falco
      tagPattern: ^(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|rc\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: falcosecurity/falco
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: falcosecurity/falco
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    validatedAgainst:
      - 0.43.0
      - 0.43.1
      - 0.44.0
      - 0.44.1
      - 0.45.0
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/falcosecurity/falco
      path: CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    availability: '>= 0.45.0'
    notes: Availability inferred from validation (0.43.0 fail (no section matching "^\\[?v?0\\.43\\.0\\]?(\\s|$)" in https://github.com/falcosecurity/falco/blob/0.43.0/CHANGELOG.md); 0.43.1 fail (no section matching "^\\[?v?0\\.43\\.1\\]?(\\s|$)" in https://github.com/falcosecurity/falco/blob/0.43.1/CHANGELOG.md); 0.44.0 fail (no section matching "^\\[?v?0\\.44\\.0\\]?(\\s|$)" in https://github.com/falcosecurity/falco/blob/0.44.0/CHANGELOG.md); 0.44.1 fail (no section matching "^\\[?v?0\\.44\\.1\\]?(\\s|$)" in https://github.com/falcosecurity/falco/blob/0.44.1/CHANGELOG.md)).
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: falcosecurity/falco
artifacts:
  - id: falco-chart
    type: helm-chart
    name: falco
    version:
      strategy: independent
    channels:
      - kind: helm-repo
        url: https://falcosecurity.github.io/charts
        chart: falco
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/falcosecurity/falco
          path: chart/falco/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/falcosecurity/falco
          path: chart/falco/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 0.43.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 0.43.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 0.44.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 0.44.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: falco-chart-2
    type: helm-chart
    name: falco
    version:
      strategy: lookup
      field: appVersion
      match: '{{.Tag}}'
      select: latest
    channels:
      - kind: helm-repo
        url: https://falcosecurity.github.io/charts
        chart: falco
      - kind: helm-git
        repository: github.com/falcosecurity/charts
        path: falco
        tagPattern: ^falco-(?P<version>\d+\.\d+\.\d+)$
    validatedAgainst:
      - 0.43.0
      - 0.43.1
      - 0.44.0
      - 0.44.1
      - 0.45.0
    notes: Chart maintained in github.com/falcosecurity/charts with its own version; chart releases are found by appVersion. 235 of 448 chart-repository tags match ^falco-(?P<version>\d+\.\d+\.\d+)$.
  - id: falco
    type: container-image
    name: falco
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: docker.io/falcosecurity/falco
    validatedAgainst:
      - 0.43.0
      - 0.43.1
      - 0.44.0
      - 0.44.1
      - 0.45.0
  - id: falco-driver-loader
    type: container-image
    name: falco-driver-loader
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: docker.io/falcosecurity/falco-driver-loader
    validatedAgainst:
      - 0.43.0
      - 0.43.1
      - 0.44.0
      - 0.44.1
      - 0.45.0
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - 0.43.0
    - 0.43.1
    - 0.44.0
    - 0.44.1
    - 0.45.0
  notes: Proposed by automated discovery; relationship checks run against 0.43.0, 0.43.1, 0.44.0, 0.44.1, 0.45.0.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 4 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.
- Ambiguity (main-chart): 2 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) falco relate to the release version?
- Ambiguity (chart-version): How do the versions of chart(s) falco relate to the release version?

## Validation matrix

| Element | Verdict | 0.43.0 | 0.43.1 | 0.44.0 | 0.44.1 | 0.45.0 |
|---|---|---|---|---|---|---|
| artifact:falco | validated | pass | pass | pass | pass | pass |
| artifact:falco-chart | unverifiable | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… |
| artifact:falco-chart-2 | validated | pass | pass | pass | pass | pass |
| artifact:falco-driver-loader | validated | pass | pass | pass | pass | pass |
| artifact:falcoctl | failing | fail: absent from docker.io/falcosecurity/falcoctl:0.43.0 | fail: absent from docker.io/falcosecurity/falcoctl:0.43.1 | fail: absent from docker.io/falcosecurity/falcoctl:0.44.0 | fail: absent from docker.io/falcosecurity/falcoctl:0.44.1 | fail: absent from docker.io/falcosecurity/falcoctl:0.45.0 |
| content:falco-chart/chart-metadata | failing | fail: fetch https://raw.githubusercontent.com/falcosecurity/falco/… | fail: fetch https://raw.githubusercontent.com/falcosecurity/falco/… | pass | pass | pass |
| content:falco-chart/helm-values | failing | fail: fetch https://raw.githubusercontent.com/falcosecurity/falco/… | fail: fetch https://raw.githubusercontent.com/falcosecurity/falco/… | pass | pass | pass |
| source:changelog | failing | fail: no section matching "^\\[?v?0\\.43\\.0\\]?(\\s\|$)" in https:… | fail: no section matching "^\\[?v?0\\.43\\.1\\]?(\\s\|$)" in https:… | fail: no section matching "^\\[?v?0\\.44\\.0\\]?(\\s\|$)" in https:… | fail: no section matching "^\\[?v?0\\.44\\.1\\]?(\\s\|$)" in https:… | pass |
| source:release-notes-github | validated | pass | pass | pass | pass | pass |

## Dropped elements

- `artifact:falcoctl` (deterministic): failed validation: 0.43.0 fail (absent from docker.io/falcosecurity/falcoctl:0.43.0); 0.43.1 fail (absent from docker.io/falcosecurity/falcoctl:0.43.1); 0.44.0 fail (absent from docker.io/falcosecurity/falcoctl:0.44.0); 0.44.1 fail (absent from docker.io/falcosecurity/falcoctl:0.44.1); …

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 195 tags: prefix "", 76 stable, 47 prereleases (alphaN×5, rcN×42), 72 junk; latest stable 0.45.0; lineage minor; strict tagPattern excludes junk tags such as 0.6.0-test, agent/0.40.0, agent/0.41.0, agent/0.42.0 |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (195 tags: prefix "", 76 stable, 47 prereleases (alphaN×5, rcN×42), 72 junk; latest stable 0.45.0; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 0.45.0 (heading "## v0.45.0"). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:falco-chart | include | helm.independent-version | heuristic | Chart has its own version "9.2.0-rc1" unrelated to the release. Published via 1 channel(s); OCI locations first. |
| artifact:falco-chart-2 | include | chart.external-lookup-appversion | heuristic | The chart lives in a separate repository with independent versions; the chart release for a product release is looked up by appVersion == release tag (GitHub Pages URL by the chart-releaser convention). |
| artifact:falco | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:falco-driver-loader | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:falcoctl | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| source:changelog | annotate | validate.availability-inferred | computed | Fails only for releases older than every passing one; availability >= 0.45.0. |
| artifact:falco-chart | annotate | validate.unverifiable | computed | Kept unverified: 0.43.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 0.43.1 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:falcoctl | drop | validate.failed | computed | Relationship check failed: 0.43.0 fail (absent from docker.io/falcosecurity/falcoctl:0.43.0); 0.43.1 fail (absent from docker.io/falcosecurity/falcoctl:0.43.1); 0.44.0 fail (absent from docker.io/falcosecurity/falcoctl:0… |

<details><summary>16 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| changelog `chart/falco/CHANGELOG.md` | changelog.not-primary | Only the top-level changelog of the product repository is used. |
| image `docker.io/kindest/node` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `public.ecr.aws/falcosecurity/falco` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned,unresolved). |
| image `public.ecr.aws/falcosecurity/falco-driver-loader` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned,unresolved). |
| image `docker.io/library/fedora` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/debian` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/falcosecurity/plugins/plugin/container` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/falcosecurity/plugins/plugin/k8smeta` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/falcosecurity/falcosidekick` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `docker.io/falcosecurity/falcosidekick-ui` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/redis/redis-stack` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/falcosecurity/rules/falco-rules` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/falcosecurity/plugins/ruleset/k8saudit` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/falcosecurity/plugins/plugin/json` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/falcosecurity/plugins/plugin/k8saudit` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/falcosecurity/plugins/plugin/github` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |

</details>

## Candidates


### build-tool (1)

- `cosign` (medium; workflow.cosign) — [falcosecurity/falco:.github/workflows/reusable_publish_docker.yaml L146](https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/reusable_publish_docker.yaml#L146): `uses: sigstore/cosign-installer@59acb6260d9c0ba8f4a2f9d9b48431a222b68e20 # v3.5.0`

### changelog (2)

- `CHANGELOG.md` (high; docs.changelog) — [falcosecurity/falco:CHANGELOG.md L3](https://github.com/falcosecurity/falco/blob/0.45.0/CHANGELOG.md#L3): `## v0.45.0`
- `chart/falco/CHANGELOG.md` (high; docs.changelog) — [falcosecurity/falco:chart/falco/CHANGELOG.md L12](https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/CHANGELOG.md#L12): `## v9.2.0-rc1`

### chart-repo (1)

- `github.com/falcosecurity/charts` (medium; docs.chart-repo-ref) — [falcosecurity/falco:ADOPTERS.md L75](https://github.com/falcosecurity/falco/blob/0.45.0/ADOPTERS.md#L75): `* [Shapesecurity/F5](https://www.shapesecurity.com/) Shapesecurity defends against application fraud attacks l…`

### docs-repo (1)

- `github.com/falcosecurity/falco-website` (medium; docs.docs-repo-ref) — [falcosecurity/falco:RELEASE.md L175](https://github.com/falcosecurity/falco/blob/0.45.0/RELEASE.md#L175): `- IFF the ongoing release introduces a **new minor version**, [archive a snapshot of the Falco website](https:…`

### helm-chart (1)

- `falco@chart/falco` (high; helm.chart) — [falcosecurity/falco:chart/falco/Chart.yaml L2](https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/Chart.yaml#L2): `name: falco`

### helm-repo (3)

- `https://charts.helm.sh/stable` (medium; docs.helm-repo-add) — [falcosecurity/falco-website:content/en/blog/intro-k8s-security-monitoring.md L83](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/intro-k8s-security-monitoring.md#L83): `$ helm repo add stable https://charts.helm.sh/stable`
- `https://falcosecurity.github.io/charts` (medium; docs.helm-repo-add) — [falcosecurity/falco:chart/falco/README.gotmpl L18](https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/README.gotmpl#L18): `helm repo add falcosecurity https://falcosecurity.github.io/charts`
- `https://grafana.github.io/helm-charts` (medium; docs.helm-repo-add) — [falcosecurity/falco-website:content/en/blog/falco-security-and-monitoring-on-rke-bare-metal-cluster-with-rancher.md L287](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falco-security-and-monitoring-on-rke-bare-metal-cluster-with-rancher.md#L287): `helm repo add grafana https://grafana.github.io/helm-charts`

### image (18)

- `docker.io/falcosecurity/falco` (high; docs.image-ref, helm.values-image, manifest.image, workflow.image-ref) — [falcosecurity/falco:.github/release_template.md L16](https://github.com/falcosecurity/falco/blob/0.45.0/.github/release_template.md#L16): `| 'docker pull docker.io/falcosecurity/falco:FALCOVER'                      |`
- `docker.io/falcosecurity/falco-driver-loader` (high; docs.image-ref, helm.values-image, workflow.image-ref) — [falcosecurity/falco:.github/release_template.md L18](https://github.com/falcosecurity/falco/blob/0.45.0/.github/release_template.md#L18): `| 'docker pull docker.io/falcosecurity/falco-driver-loader:FALCOVER'        |`
- `docker.io/falcosecurity/falcoctl` (high; helm.values-image) — [falcosecurity/falco:chart/falco/values.yaml L547](https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/values.yaml#L547): `repository: falcosecurity/falcoctl`
- `docker.io/debian` (medium; workflow.image-ref) — [falcosecurity/falco:.github/workflows/reusable_publish_packages.yaml L104](https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/reusable_publish_packages.yaml#L104): `container: docker.io/debian:stable`
- `docker.io/falcosecurity/falcosidekick` (medium; manifest.image) — [falcosecurity/falco:docker/docker-compose/docker-compose.yaml L20](https://github.com/falcosecurity/falco/blob/0.45.0/docker/docker-compose/docker-compose.yaml#L20): `image: falcosecurity/falcosidekick`
- `docker.io/falcosecurity/falcosidekick-ui` (medium; manifest.image) — [falcosecurity/falco:docker/docker-compose/docker-compose.yaml L26](https://github.com/falcosecurity/falco/blob/0.45.0/docker/docker-compose/docker-compose.yaml#L26): `image: falcosecurity/falcosidekick-ui:2.2.0`
- `docker.io/kindest/node` (medium; manifest.image) — [falcosecurity/falco:.github/kind-config.yaml L5](https://github.com/falcosecurity/falco/blob/0.45.0/.github/kind-config.yaml#L5): `image: kindest/node:v1.24.1@sha256:fd82cddc87336d91aa0a2fc35f3c7a9463c53fd8e9575e9052d2c75c61f5b083`
- `docker.io/library/fedora` (medium; workflow.image-ref) — [falcosecurity/falco:.github/workflows/reusable_publish_packages.yaml L26](https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/reusable_publish_packages.yaml#L26): `container: docker.io/library/fedora:38`
- `docker.io/redis/redis-stack` (medium; manifest.image) — [falcosecurity/falco:docker/docker-compose/docker-compose.yaml L34](https://github.com/falcosecurity/falco/blob/0.45.0/docker/docker-compose/docker-compose.yaml#L34): `image: redis/redis-stack:7.2.0-v11`
- `ghcr.io/falcosecurity/plugins/plugin/container` (medium; docs.image-ref) — [falcosecurity/falco:chart/falco/README.md L504](https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/README.md#L504): `| collectors.containerEngine | object | '{"enabled":true,"engines":{"bpm":{"enabled":true},"containerd":{"enab…`
- `ghcr.io/falcosecurity/plugins/plugin/github` (medium; docs.image-ref) — [falcosecurity/falco-website:content/en/blog/falcoctl-install-manage-rules-plugins/index.md L243](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falcoctl-install-manage-rules-plugins/index.md#L243): `INFO  Installing the following artifacts: [ghcr.io/falcosecurity/plugins/plugin/github:latest]`
- `ghcr.io/falcosecurity/plugins/plugin/json` (medium; docs.image-ref) — [falcosecurity/falco-website:content/en/blog/falcoctl-install-manage-rules-plugins/index.md L222](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falcoctl-install-manage-rules-plugins/index.md#L222): `INFO  Preparing to pull "ghcr.io/falcosecurity/plugins/plugin/json:0.6.0"`
- `ghcr.io/falcosecurity/plugins/plugin/k8saudit` (medium; docs.image-ref) — [falcosecurity/falco-website:content/en/blog/falcoctl-install-manage-rules-plugins/index.md L227](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falcoctl-install-manage-rules-plugins/index.md#L227): `INFO  Preparing to pull "ghcr.io/falcosecurity/plugins/plugin/k8saudit:0.5.0"`
- `ghcr.io/falcosecurity/plugins/plugin/k8smeta` (medium; docs.image-ref) — [falcosecurity/falco:chart/falco/README.md L512](https://github.com/falcosecurity/falco/blob/0.45.0/chart/falco/README.md#L512): `| collectors.kubernetes | object | '{"collectorHostname":"","collectorPort":"","enabled":false,"hostProc":"/ho…`
- `ghcr.io/falcosecurity/plugins/ruleset/k8saudit` (medium; docs.image-ref) — [falcosecurity/falco-website:content/en/blog/falcoctl-install-manage-rules-plugins/index.md L216](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falcoctl-install-manage-rules-plugins/index.md#L216): `INFO  Installing the following artifacts: [ghcr.io/falcosecurity/plugins/ruleset/k8saudit:0.5 json:0.6.0 k8sau…`
- `ghcr.io/falcosecurity/rules/falco-rules` (medium; docs.image-ref) — [falcosecurity/falco-website:content/en/blog/falcoctl-install-manage-rules-plugins/index.md L185](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falcoctl-install-manage-rules-plugins/index.md#L185): `INFO  Installing the following artifacts: [ghcr.io/falcosecurity/rules/falco-rules:latest]`
- `public.ecr.aws/falcosecurity/falco` (medium; docs.image-ref, workflow.image-ref) — [falcosecurity/falco:.github/release_template.md L17](https://github.com/falcosecurity/falco/blob/0.45.0/.github/release_template.md#L17): `| 'docker pull public.ecr.aws/falcosecurity/falco:FALCOVER'                 |`
- `public.ecr.aws/falcosecurity/falco-driver-loader` (medium; workflow.image-ref) — [falcosecurity/falco:.github/workflows/reusable_publish_docker.yaml L128](https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/reusable_publish_docker.yaml#L128): `crane copy docker.io/falcosecurity/falco-driver-loader:${{ inputs.tag }} public.ecr.aws/falcosecurity/falco-dr…`

### registry (3)

- `docker.io` (medium; workflow.registry-login) — [falcosecurity/falco:.github/workflows/reusable_publish_docker.yaml L53](https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/reusable_publish_docker.yaml#L53): `uses: docker/login-action@343f7c4344506bcbf9b4de18042ae17996df046d # v3.0.0`
- `registry.gitlab.com/v2/x/falcosecurity/plugins/k8saudit/manifests/sha256` (low; docs.registry-ref) — [falcosecurity/falco-website:content/en/blog/falcoctl-gitlab-oci-artifacts-support.md L16](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falcoctl-gitlab-oci-artifacts-support.md#L16): `Error: PUT https://registry.gitlab.com/v2/x/falcosecurity/plugins/k8saudit/manifests/sha256:b29c97a6590486f8b3…`
- `registry.gitlab.com/v2/x/falcosecurity/rules/k8saudit-rules/manifests/1` (low; docs.registry-ref) — [falcosecurity/falco-website:content/en/blog/falcoctl-gitlab-oci-artifacts-support.md L15](https://github.com/falcosecurity/falco-website/blob/b6ed9de11f4a0fa371f6121f554d0e51a131ba01/content/en/blog/falcoctl-gitlab-oci-artifacts-support.md#L15): `Error: PUT https://registry.gitlab.com/v2/x/falcosecurity/rules/k8saudit-rules/manifests/1: MANIFEST_INVALID: …`

### release-publisher (1)

- `softprops/action-gh-release` (high; workflow.release-upload) — [falcosecurity/falco:.github/workflows/release.yaml L211](https://github.com/falcosecurity/falco/blob/0.45.0/.github/workflows/release.yaml#L211): `uses: softprops/action-gh-release@de2c0eb89ae2a093876385947365aca7b0e5f844 # v0.1.15`

### tag-scheme (1)

- `^(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|rc\d+))?)$` (high; tags.ls-remote) — [falcosecurity/falco/releases/tag/0.45.0 refs/tags/0.45.0](https://github.com/falcosecurity/falco/releases/tag/0.45.0#refs/tags/0.45.0): `05d5b3e363d5196c68c8ccea842ca57a2ef05562 refs/tags/0.45.0`
