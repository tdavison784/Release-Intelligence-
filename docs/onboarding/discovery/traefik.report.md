# Discovery report: traefik

- Repository: `github.com/traefik/traefik` at `v3.7.13` (commit `fc92cc118a05…`)
- Generated: 2026-10-01T14:07:23Z
- LLM: not used (deterministic resolver only)
- Tags: 581 tags: prefix "v", 311 stable, 180 prereleases (alphaN×8, beta.N×71, betaN×6, rc.N×3, rcN×92), 90 junk; latest stable v3.7.13; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|beta\.\d+|beta\d+|rc\.\d+|rc\d+))?)$` (junk sample: v1.0, v1.0.alpha.0e683cc5355bc507dabac68bbc7559d3f179e185, v1.0.alpha.11781087cadf9068d1d0b43902b6161ee10ea458, v1.0.alpha.157, v1.0.alpha.164, v1.0.alpha.170)
- Scanned `github.com/traefik/traefik@v3.7.13` (source profile): 2277 files listed, 826 read
- Validation: done against v3.5.0, v3.5.6, v3.6.0, v3.6.25, v3.7.0, v3.7.13

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 4 | git-tags github.com/traefik/traefik; github-releases traefik/traefik |
| release-notes | found | 0 | github-releases traefik/traefik  @{{.Tag}} |
| changelog | found | 1 | repo-file github.com/traefik/traefik CHANGELOG.md |
| helm-charts | found | 2 | traefik via helm-repo:https://traefik.github.io/charts, helm-git:github.com/traefik/traefik-helm-chart |
| registries | candidates-only | 1 |  |
| images | candidates-only | 26 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 2 | github-advisories traefik/traefik |
| version-relations | found | 0 | traefik-chart: lookup appVersion == {{.Tag}}; crds: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:crds | historically-validated | crd.in-repo | https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml#L2 (+2) |
| source:changelog | historically-validated | changelog.file | https://github.com/traefik/traefik/blob/v3.7.13/CHANGELOG.md#L1 |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/release.yaml#L68 (+1) |
| artifact:traefik-chart | exception | chart.external-lookup-appversion | https://github.com/traefik/traefik/blob/v3.7.13/docs/content/migrate/nginx-to-traefik.md#L252 (+1) |
| source:advisories | discovered | security.advisories-referenced | https://github.com/traefik/traefik/blob/v3.7.13/SECURITY.md#L14 (+1) |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/release.yaml#L68 (+1) |
| source:tags | discovered | versions.git-tags | https://github.com/traefik/traefik/releases/tag/v3.7.13#refs/tags/v3.7.13 |
| versioning | discovered | tags.scheme | https://github.com/traefik/traefik/releases/tag/v3.7.13#refs/tags/v3.7.13 (+1) |

Statuses: 3 historically-validated, 4 discovered, 1 exception.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: traefik
name: traefik
homepage: https://github.com/traefik/traefik
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|beta\.\d+|beta\d+|rc\.\d+|rc\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/traefik/traefik
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|beta\.\d+|beta\d+|rc\.\d+|rc\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: traefik/traefik
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: traefik/traefik
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    validatedAgainst:
      - 3.5.0
      - 3.5.6
      - 3.6.0
      - 3.6.25
      - 3.7.0
      - 3.7.13
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/traefik/traefik
      path: CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    validatedAgainst:
      - 3.5.0
      - 3.5.6
      - 3.6.0
      - 3.6.25
      - 3.7.0
      - 3.7.13
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: traefik/traefik
    notes: 'Security policy: SECURITY.md → https://github.com/traefik/traefik/security/advisories https://doc.traefik.io/traefik/contributing/submitting-security-issues/'
artifacts:
  - id: traefik-chart
    type: helm-chart
    name: traefik
    version:
      strategy: lookup
      field: appVersion
      match: '{{.Tag}}'
      select: latest
    channels:
      - kind: helm-repo
        url: https://traefik.github.io/charts
        chart: traefik
      - kind: helm-git
        repository: github.com/traefik/traefik-helm-chart
        path: traefik
        tagPattern: ^traefik-(?P<version>\d+\.\d+\.\d+)$
    optional: true
    validatedAgainst:
      - 3.5.0
      - 3.6.0
      - 3.7.0
      - 3.7.13
    notes: Chart maintained in github.com/traefik/traefik-helm-chart with its own version; chart releases are found by appVersion. 0 of 397 chart-repository tags match ^traefik-(?P<version>\d+\.\d+\.\d+)$. Not published for every release (3.5.6 fail (no chart version ships appVersion v3.5.6 (https://traefik.github.io/charts (chart traefik) lists 350 versions)); 3.6.25 fail (no chart version ships appVersion v3.6.25 (https://traefik.github.io/charts (chart traefik) lists 350 versions))); marked optional by validation.
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/traefik/traefik
        path: docs/content/reference/dynamic-configuration
        glob: '*.yml'
    contents:
      - kind: crds
    validatedAgainst:
      - 3.5.0
      - 3.5.6
      - 3.6.0
      - 3.6.25
      - 3.7.0
      - 3.7.13
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - v3.5.0
    - v3.5.6
    - v3.6.0
    - v3.6.25
    - v3.7.0
    - v3.7.13
  notes: Proposed by automated discovery; relationship checks run against v3.5.0, v3.5.6, v3.6.0, v3.6.25, v3.7.0, v3.7.13.
```

## Open questions for the reviewer

- Ambiguity (chart-version): How do the versions of chart(s) traefik relate to the release version?

## Validation matrix

| Element | Verdict | v3.5.0 | v3.5.6 | v3.6.0 | v3.6.25 | v3.7.0 | v3.7.13 |
|---|---|---|---|---|---|---|---|
| artifact:crds | validated |  |  |  |  |  |  |
| artifact:traefik-chart | failing |  |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |  |
| source:changelog | validated |  |  |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |  |  |

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 581 tags: prefix "v", 311 stable, 180 prereleases (alphaN×8, beta.N×71, betaN×6, rc.N×3, rcN×92), 90 junk; latest stable v3.7.13; lineage minor; strict tagPattern excludes junk tags such as v1.0, v1.0.alpha.0e683cc5… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (581 tags: prefix "v", 311 stable, 180 prereleases (alphaN×8, beta.N×71, betaN×6, rc.N×3, rcN×92), 90 junk; latest stable v3.7.13; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 3.7.13 (heading "## [v3.7.13](https://github.com/traefik/traefik/tree/v3.7.13) (2026-09-04)"). |
| source:advisories | include | security.advisories-referenced | heuristic | The security policy states that advisories are published as GitHub Security Advisories. |
| artifact:traefik-chart | include | chart.external-lookup-appversion | heuristic | The chart lives in a separate repository with independent versions; the chart release for a product release is looked up by appVersion == release tag (GitHub Pages URL by the chart-releaser convention). |
| artifact:crds | include | crd.in-repo | heuristic | 20 CRDs of the product's API groups in docs/content/reference/dynamic-configuration at the release tag. |
| artifact:traefik-chart | annotate | validate.optional-lookup | computed | Lookup artifact present for 4 sampled release(s) and absent for others; kept as optional. |

<details><summary>34 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| manifest `docs/content/reference/dynamic-configuration/kubernetes-gateway-traefik-lb-svc.yml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `docs/content/reference/dynamic-configuration/kubernetes-whoami-svc.yml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| crd `integration/fixtures/gateway-api-conformance/00-experimental-v1.6.1.yml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `integration/fixtures/k8s-gateway/00-experimental-v1.5.1.yml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `integration/fixtures/k8s-gateway/00-experimental-v1.6.1.yml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `integration/fixtures/k8s/01-traefik-crd.yml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `integration/fixtures/knative/00-knative-crd-v1.20.0.yml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `integration/fixtures/knative/03-knative-serving-v1.20.0.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| image `ghcr.io/traefik/traefik` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,snapshot). |
| image `docker.io/library/traefik` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/traefik/whoami` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/traefik/traefik` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/traefik/whoamitcp` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/traefik/whoamiudp` | image.test-only | Only referenced from test, sample or documentation files. |
| image `gcr.io/knative-releases/knative.dev/serving/cmd/queue` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/knative-releases/knative.dev/serving/cmd/activator` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/knative-releases/knative.dev/serving/cmd/autoscaler` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/knative-releases/knative.dev/serving/cmd/controller` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/knative-releases/knative.dev/serving/cmd/webhook` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/consul` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/hashicorpnomad/uuid-api` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/coreos/etcd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/letsencrypt/pebble` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/redis` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/tailscale/tailscale` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/postgres` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/yaman/timeout` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/tempo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/otel/opentelemetry-collector-contrib` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/zookeeper` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image-name `check` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `docs` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `buildx` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### build-tool (2)

- `goreleaser` (high; workflow.goreleaser) — [traefik/traefik:.github/workflows/release.yaml L68](https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/release.yaml#L68): `uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3`
- `ko` (medium; workflow.ko) — [traefik/traefik:.github/workflows/test-knative-conformance.yaml L40](https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/test-knative-conformance.yaml#L40): `uses: ko-build/setup-ko@ace48d793556083a76f1e3e6068850c1f4a369aa # v0.6`

### changelog (1)

- `CHANGELOG.md` (high; docs.changelog) — [traefik/traefik:CHANGELOG.md L1](https://github.com/traefik/traefik/blob/v3.7.13/CHANGELOG.md#L1): `## [v3.7.13](https://github.com/traefik/traefik/tree/v3.7.13) (2026-09-04)`

### chart-repo (1)

- `github.com/traefik/traefik-helm-chart` (medium; docs.chart-repo-ref) — [traefik/traefik:docs/content/migrate/nginx-to-traefik.md L252](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/migrate/nginx-to-traefik.md#L252): `Or using a [values file](https://github.com/traefik/traefik-helm-chart/blob/master/traefik/VALUES.md) for more…`

### crd (17)

- `docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml L2](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml#L2): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_ingressroutes.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_ingressroutes.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_ingressroutes.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_ingressroutetcps.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_ingressroutetcps.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_ingressroutetcps.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_ingressrouteudps.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_ingressrouteudps.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_ingressrouteudps.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_middlewares.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_middlewares.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_middlewares.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_middlewaretcps.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_middlewaretcps.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_middlewaretcps.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_serverstransports.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_serverstransports.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_serverstransports.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_serverstransporttcps.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_serverstransporttcps.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_serverstransporttcps.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_tlsoptions.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_tlsoptions.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_tlsoptions.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_tlsstores.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_tlsstores.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_tlsstores.yaml#L3): `kind: CustomResourceDefinition`
- `docs/content/reference/dynamic-configuration/traefik.io_traefikservices.yaml` (high; crd.file) — [traefik/traefik:docs/content/reference/dynamic-configuration/traefik.io_traefikservices.yaml L3](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/traefik.io_traefikservices.yaml#L3): `kind: CustomResourceDefinition`
- `integration/fixtures/gateway-api-conformance/00-experimental-v1.6.1.yml` (low; crd.file) — [traefik/traefik:integration/fixtures/gateway-api-conformance/00-experimental-v1.6.1.yml L23](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/gateway-api-conformance/00-experimental-v1.6.1.yml#L23): `kind: CustomResourceDefinition`
- `integration/fixtures/k8s-gateway/00-experimental-v1.5.1.yml` (low; crd.file) — [traefik/traefik:integration/fixtures/k8s-gateway/00-experimental-v1.5.1.yml L23](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/k8s-gateway/00-experimental-v1.5.1.yml#L23): `kind: CustomResourceDefinition`
- `integration/fixtures/k8s-gateway/00-experimental-v1.6.1.yml` (low; crd.file) — [traefik/traefik:integration/fixtures/k8s-gateway/00-experimental-v1.6.1.yml L23](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/k8s-gateway/00-experimental-v1.6.1.yml#L23): `kind: CustomResourceDefinition`
- `integration/fixtures/k8s/01-traefik-crd.yml` (low; crd.file) — [traefik/traefik:integration/fixtures/k8s/01-traefik-crd.yml L3](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/k8s/01-traefik-crd.yml#L3): `kind: CustomResourceDefinition`
- `integration/fixtures/knative/00-knative-crd-v1.20.0.yml` (low; crd.file) — [traefik/traefik:integration/fixtures/knative/00-knative-crd-v1.20.0.yml L16](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/knative/00-knative-crd-v1.20.0.yml#L16): `kind: CustomResourceDefinition`
- `integration/fixtures/knative/03-knative-serving-v1.20.0.yaml` (low; crd.file) — [traefik/traefik:integration/fixtures/knative/03-knative-serving-v1.20.0.yaml L418](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/knative/03-knative-serving-v1.20.0.yaml#L418): `kind: CustomResourceDefinition`

### helm-repo (1)

- `https://traefik.github.io/charts` (medium; docs.helm-repo-add) — [traefik/traefik:docs/content/getting-started/kubernetes.md L59](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/getting-started/kubernetes.md#L59): `helm repo add traefik https://traefik.github.io/charts`

### image (23)

- `docker.io/library/traefik` (medium; manifest.image) — [traefik/traefik:docs/content/reference/dynamic-configuration/kubernetes-gateway-traefik-lb-svc.yml L27](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/kubernetes-gateway-traefik-lb-svc.yml#L27): `image: traefik:v3.7`
- `docker.io/traefik/whoami` (medium; manifest.image) — [traefik/traefik:docs/content/reference/dynamic-configuration/kubernetes-whoami-svc.yml L20](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/kubernetes-whoami-svc.yml#L20): `image: traefik/whoami`
- `ghcr.io/traefik/traefik` (medium; workflow.image-ref) — [traefik/traefik:.github/workflows/sync-docker-images.yaml L28](https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/sync-docker-images.yaml#L28): `diff <(crane ls traefik) <(crane ls ghcr.io/traefik/traefik) | grep '^<' | awk '{print $2}' | while read -r ta…`
- `docker.io/grafana/tempo` (low; manifest.image) — [traefik/traefik:integration/resources/compose/tracing.yml L4](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/tracing.yml#L4): `image: grafana/tempo:2.6.1`
- `docker.io/hashicorpnomad/uuid-api` (low; manifest.image) — [traefik/traefik:integration/resources/compose/consul_catalog.yml L40](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/consul_catalog.yml#L40): `image: hashicorpnomad/uuid-api:v5`
- `docker.io/library/consul` (low; manifest.image) — [traefik/traefik:integration/resources/compose/consul.yml L3](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/consul.yml#L3): `image: consul:1.6`
- `docker.io/library/nginx` (low; manifest.image) — [traefik/traefik:integration/resources/compose/error_pages.yml L3](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/error_pages.yml#L3): `image: nginx:1.25.3-alpine3.18`
- `docker.io/library/postgres` (low; manifest.image) — [traefik/traefik:integration/resources/compose/tcp.yml L61](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/tcp.yml#L61): `image: postgres:17-alpine`
- `docker.io/library/redis` (low; manifest.image) — [traefik/traefik:integration/resources/compose/ratelimit.yml L6](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/ratelimit.yml#L6): `image: redis:5.0`
- `docker.io/library/zookeeper` (low; manifest.image) — [traefik/traefik:integration/resources/compose/zookeeper.yml L3](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/zookeeper.yml#L3): `image: zookeeper:3.5`
- `docker.io/otel/opentelemetry-collector-contrib` (low; manifest.image) — [traefik/traefik:integration/resources/compose/tracing.yml L9](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/tracing.yml#L9): `image: otel/opentelemetry-collector-contrib:0.103.0`
- `docker.io/tailscale/tailscale` (low; manifest.image) — [traefik/traefik:integration/resources/compose/tailscale.yml L4](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/tailscale.yml#L4): `image: tailscale/tailscale:v1.24.0`
- `docker.io/traefik/traefik` (low; manifest.image) — [traefik/traefik:integration/fixtures/gateway-api-conformance/02-traefik.yml L44](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/gateway-api-conformance/02-traefik.yml#L44): `image: traefik/traefik:latest`
- `docker.io/traefik/whoamitcp` (low; manifest.image) — [traefik/traefik:integration/fixtures/k8s-gateway/01-services.yml L68](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/k8s-gateway/01-services.yml#L68): `image: traefik/whoamitcp`
- `docker.io/traefik/whoamiudp` (low; manifest.image) — [traefik/traefik:integration/fixtures/k8s/02-services.yml L110](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/k8s/02-services.yml#L110): `image: traefik/whoamiudp:latest`
- `docker.io/yaman/timeout` (low; manifest.image) — [traefik/traefik:integration/resources/compose/timeout.yml L3](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/timeout.yml#L3): `image: yaman/timeout`
- `gcr.io/knative-releases/knative.dev/serving/cmd/activator` (low; manifest.image) — [traefik/traefik:integration/fixtures/knative/03-knative-serving-v1.20.0.yaml L8763](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/knative/03-knative-serving-v1.20.0.yaml#L8763): `image: gcr.io/knative-releases/knative.dev/serving/cmd/activator@sha256:701507d9c480ff87dcfa4755ca7d3d6b727438…`
- `gcr.io/knative-releases/knative.dev/serving/cmd/autoscaler` (low; manifest.image) — [traefik/traefik:integration/fixtures/knative/03-knative-serving-v1.20.0.yaml L8919](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/knative/03-knative-serving-v1.20.0.yaml#L8919): `image: gcr.io/knative-releases/knative.dev/serving/cmd/autoscaler@sha256:485c0a009cede9138a7ec1e5ab5a5ef22ff9d…`
- `gcr.io/knative-releases/knative.dev/serving/cmd/controller` (low; manifest.image) — [traefik/traefik:integration/fixtures/knative/03-knative-serving-v1.20.0.yaml L9044](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/knative/03-knative-serving-v1.20.0.yaml#L9044): `image: gcr.io/knative-releases/knative.dev/serving/cmd/controller@sha256:3718bf2e2f135ac70699db930145b22e52fb4…`
- `gcr.io/knative-releases/knative.dev/serving/cmd/queue` (low; manifest.image) — [traefik/traefik:integration/fixtures/knative/03-knative-serving-v1.20.0.yaml L7137](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/knative/03-knative-serving-v1.20.0.yaml#L7137): `image: gcr.io/knative-releases/knative.dev/serving/cmd/queue@sha256:b78cfa015872b12cf64f01fc21f29190c6f2fa69aa…`
- `gcr.io/knative-releases/knative.dev/serving/cmd/webhook` (low; manifest.image) — [traefik/traefik:integration/fixtures/knative/03-knative-serving-v1.20.0.yaml L9227](https://github.com/traefik/traefik/blob/v3.7.13/integration/fixtures/knative/03-knative-serving-v1.20.0.yaml#L9227): `image: gcr.io/knative-releases/knative.dev/serving/cmd/webhook@sha256:0d9c4d4971d9b67eaf5ce1359f6ff334145d32b3…`
- `ghcr.io/letsencrypt/pebble` (low; manifest.image) — [traefik/traefik:integration/resources/compose/pebble.yml L3](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/pebble.yml#L3): `image: ghcr.io/letsencrypt/pebble:2.8.0`
- `quay.io/coreos/etcd` (low; manifest.image) — [traefik/traefik:integration/resources/compose/etcd.yml L3](https://github.com/traefik/traefik/blob/v3.7.13/integration/resources/compose/etcd.yml#L3): `image: quay.io/coreos/etcd:v3.5.14`

### image-name (3)

- `buildx` (low; dockerfile.name) — [traefik/traefik:webui/buildx.Dockerfile L1](https://github.com/traefik/traefik/blob/v3.7.13/webui/buildx.Dockerfile#L1): `FROM node:24-alpine3.22`
- `check` (low; dockerfile.name) — [traefik/traefik:docs/check.Dockerfile L1](https://github.com/traefik/traefik/blob/v3.7.13/docs/check.Dockerfile#L1): `FROM alpine:3.24`
- `docs` (low; dockerfile.name) — [traefik/traefik:docs/docs.Dockerfile L1](https://github.com/traefik/traefik/blob/v3.7.13/docs/docs.Dockerfile#L1): `FROM alpine:3.24`

### manifest (7)

- `docs/content/reference/dynamic-configuration/kubernetes-crd-definition-v1.yml` (medium; docs.raw-url) — [traefik/traefik:docs/content/migrate/v3.md L360](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/migrate/v3.md#L360): `To do so, please apply the [CRDs](https://raw.githubusercontent.com/traefik/traefik/v3.7/docs/content/referenc…`
- `docs/content/reference/dynamic-configuration/kubernetes-crd-rbac.yml` (medium; docs.raw-url) — [traefik/traefik:docs/content/migrate/v3.md L1080](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/migrate/v3.md#L1080): `kubectl apply -f https://raw.githubusercontent.com/traefik/traefik/v3.4/docs/content/reference/dynamic-configu…`
- `docs/content/reference/dynamic-configuration/kubernetes-gateway-rbac.yml` (medium; docs.raw-url) — [traefik/traefik:docs/content/reference/install-configuration/providers/kubernetes/kubernetes-gateway.md L45](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/install-configuration/providers/kubernetes/kubernetes-gateway.md#L45): `kubectl apply -f https://raw.githubusercontent.com/traefik/traefik/v3.7/docs/content/reference/dynamic-configu…`
- `docs/content/reference/dynamic-configuration/kubernetes-gateway-traefik-lb-svc.yml` (medium; manifest.workloads) — [traefik/traefik:docs/content/reference/dynamic-configuration/kubernetes-gateway-traefik-lb-svc.yml L8](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/kubernetes-gateway-traefik-lb-svc.yml#L8): `kind: Deployment`
- `docs/content/reference/dynamic-configuration/kubernetes-ingress-nginx-rbac.yml` (medium; docs.raw-url) — [traefik/traefik:docs/content/reference/install-configuration/providers/kubernetes/kubernetes-ingress-nginx.md L32](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/install-configuration/providers/kubernetes/kubernetes-ingress-nginx.md#L32): `kubectl apply -f https://raw.githubusercontent.com/traefik/traefik/v3.7/docs/content/reference/dynamic-configu…`
- `docs/content/reference/dynamic-configuration/kubernetes-knative-rbac.yml` (medium; docs.raw-url) — [traefik/traefik:docs/content/reference/install-configuration/providers/kubernetes/knative.md L53](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/install-configuration/providers/kubernetes/knative.md#L53): `kubectl apply -f https://raw.githubusercontent.com/traefik/traefik/v3.7/docs/content/reference/dynamic-configu…`
- `docs/content/reference/dynamic-configuration/kubernetes-whoami-svc.yml` (medium; manifest.workloads) — [traefik/traefik:docs/content/reference/dynamic-configuration/kubernetes-whoami-svc.yml L2](https://github.com/traefik/traefik/blob/v3.7.13/docs/content/reference/dynamic-configuration/kubernetes-whoami-svc.yml#L2): `kind: Deployment`

### registry (1)

- `docker.io` (medium; workflow.registry-login) — [traefik/traefik:.github/workflows/documentation.yaml L33](https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/documentation.yaml#L33): `uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0`

### release-publisher (2)

- `gh release create` (high; workflow.release-upload) — [traefik/traefik:.github/workflows/release.yaml L137](https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/release.yaml#L137): `gh release create ${VERSION} ./dist/**/traefik*.{zip,tar.gz} ./dist/traefik*.{tar.gz,txt} --repo traefik/traef…`
- `goreleaser` (high; workflow.goreleaser) — [traefik/traefik:.github/workflows/release.yaml L68](https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/release.yaml#L68): `uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3`

### release-trigger (1)

- `v*.*.*` (high; workflow.tag-trigger) — [traefik/traefik:.github/workflows/release.yaml L6](https://github.com/traefik/traefik/blob/v3.7.13/.github/workflows/release.yaml#L6): `- 'v*.*.*'`

### security-advisories (1)

- `traefik/traefik` (high; docs.security-advisories-link) — [traefik/traefik:SECURITY.md L14](https://github.com/traefik/traefik/blob/v3.7.13/SECURITY.md#L14): `by creating a [security advisory](https://github.com/traefik/traefik/security/advisories).`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [traefik/traefik:SECURITY.md L1](https://github.com/traefik/traefik/blob/v3.7.13/SECURITY.md#L1): `# Security Policy`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|beta\.\d+|beta\d+|rc\.\d+|rc\d+))?)$` (high; tags.ls-remote) — [traefik/traefik/releases/tag/v3.7.13 refs/tags/v3.7.13](https://github.com/traefik/traefik/releases/tag/v3.7.13#refs/tags/v3.7.13): `fc92cc118a0557a029c7019d5ee06665127b0f13 refs/tags/v3.7.13`
