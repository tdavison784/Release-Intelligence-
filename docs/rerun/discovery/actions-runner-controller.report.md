# Discovery report: actions-runner-controller

- Repository: `github.com/actions/actions-runner-controller` at `v0.27.6` (commit `b511953df7a8…`)
- Generated: 2026-10-02T12:32:37Z
- LLM: not used (deterministic resolver only)
- Tags: 149 tags: prefix "v", 61 stable, 0 prereleases (), 88 junk; latest stable v0.27.6; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+)$` (junk sample: 0.2.0, 0.3.0, actions-runner-controller-0.1.2, actions-runner-controller-0.10.0, actions-runner-controller-0.10.1, actions-runner-controller-0.10.2)
- Scanned `github.com/actions/actions-runner-controller@v0.27.6` (source profile): 480 files listed, 199 read
- Validation: done against v0.25.0, v0.25.2, v0.26.0, v0.27.0, v0.27.6

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 4 | git-tags github.com/actions/actions-runner-controller; github-releases actions/actions-runner-controller |
| release-notes | found | 1 | repo-file github.com/actions/actions-runner-controller docs/releasenotes/{{.Major}}.{{.Minor}}.md; github-releases actions/actions-runner-controller  @{{.Tag}} |
| changelog | not-found | 0 |  |
| helm-charts | candidates-only | 11 |  |
| registries | candidates-only | 1 |  |
| images | candidates-only | 19 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories actions/actions-runner-controller |
| version-relations | found | 1 | actions-runner-controller-manifest: version = {{.Tag}}; crds: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:actions-runner-controller-chart | historically-validated | helm.appversion-lookup | https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/Chart.yaml#L2 (+2) |
| artifact:actions-runner-controller-manifest | historically-validated | asset.hosted-release-download | https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/installing-arc.md#L15 |
| artifact:crds | historically-validated | crd.in-repo | https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.github.com_autoscalinglisteners.yaml#L3 (+2) |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/installing-arc.md#L15 (+1) |
| artifact:actions-runner-controller | unverified | image.tag-from-chart-appversion | https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/values.yaml#L53 |
| artifact:gha-runner-scale-set-chart | unverified | helm.independent-version | https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set/Chart.yaml#L2 (+2) |
| artifact:gha-runner-scale-set-controller | unverified | image.tag-from-chart-appversion | https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/values.yaml#L12 |
| artifact:gha-runner-scale-set-controller-chart | unverified | helm.independent-version | https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/Chart.yaml#L2 (+2) |
| source:advisories | discovered | security.github-hosted-default | https://github.com/actions/actions-runner-controller/blob/v0.27.6/SECURITY.md#L1 |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/installing-arc.md#L15 (+1) |
| source:release-notes-docs | unverified | notes.per-line-file-with-release-sections | https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/releasenotes/0.27.md# |
| source:tags | discovered | versions.git-tags | https://github.com/actions/actions-runner-controller/releases/tag/v0.27.6#refs/tags/v0.27.6 |
| versioning | discovered | tags.scheme | https://github.com/actions/actions-runner-controller/releases/tag/v0.27.6#refs/tags/v0.27.6 |

Statuses: 4 historically-validated, 4 discovered, 5 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: actions-runner-controller
name: actions-runner-controller
homepage: https://github.com/actions/actions-runner-controller
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/actions/actions-runner-controller
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: actions/actions-runner-controller
    priority: 1
  - id: release-notes-docs
    roles:
      - release-notes
    locator:
      kind: repo-file
      repository: github.com/actions/actions-runner-controller
      path: docs/releasenotes/{{.Major}}.{{.Minor}}.md
    extract:
      type: markdown-section
      heading: ^v?{{regexQuote .Version}}$
    fallbackGroup: release-notes
    notes: 'Unverified by discovery (insufficient: 0.25.0 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?0\\.25\\.0$" in https://github.com/actions/actions-runner-controller/blob/v0.25.0/docs/releasenotes/0.25.md); 0.25.2 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?0\\.25\\.2$" in https://github.com/actions/actions-runner-controller/blob/v0.25.2/docs/releasenotes/0.25.md); 0.26.0 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?0\\.26\\.0$" in https://github.com/actions/actions-runner-controller/blob/v0.26.0/docs/releasenotes/0.26.md); 0.27.0 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?0\\.27\\.0$" in https://github.com/actions/actions-runner-controller/blob/v0.27.0/docs/releasenotes/0.27.md); …).'
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: actions/actions-runner-controller
      ref: '{{.Tag}}'
    priority: 1
    fallbackGroup: release-notes
    validatedAgainst:
      - 0.25.0
      - 0.25.2
      - 0.26.0
      - 0.27.0
      - 0.27.6
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: actions/actions-runner-controller
    notes: 'Security policy: SECURITY.md'
artifacts:
  - id: actions-runner-controller-manifest
    type: manifest
    name: actions-runner-controller.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: http
        url: https://github.com/actions/actions-runner-controller/releases/download/{{.Tag}}/actions-runner-controller.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 0.25.0
      - 0.25.2
      - 0.26.0
      - 0.27.0
      - 0.27.6
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/actions/actions-runner-controller
        path: config/crd/bases
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 0.25.0
      - 0.25.2
      - 0.26.0
      - 0.27.0
      - 0.27.6
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - v0.25.0
    - v0.25.2
    - v0.26.0
    - v0.27.0
    - v0.27.6
  notes: Proposed by automated discovery; relationship checks run against v0.25.0, v0.25.2, v0.26.0, v0.27.0, v0.27.6.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 3 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (main-chart): 3 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) gha-runner-scale-set-controller, gha-runner-scale-set relate to the release version?

## Validation matrix

| Element | Verdict | v0.25.0 | v0.25.2 | v0.26.0 | v0.27.0 | v0.27.6 |
|---|---|---|---|---|---|---|
| artifact:actions-runner-controller | failing |  |  |  |  |  |
| artifact:actions-runner-controller-chart | validated |  |  |  |  |  |
| artifact:actions-runner-controller-manifest | validated |  |  |  |  |  |
| artifact:crds | validated |  |  |  |  |  |
| artifact:gha-runner-scale-set-chart | unverifiable |  |  |  |  |  |
| artifact:gha-runner-scale-set-controller | failing |  |  |  |  |  |
| artifact:gha-runner-scale-set-controller-chart | unverifiable |  |  |  |  |  |
| content:actions-runner-controller-chart/chart-metadata | validated |  |  |  |  |  |
| content:actions-runner-controller-chart/helm-values | validated |  |  |  |  |  |
| content:actions-runner-controller-manifest/image-refs | validated |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |
| content:gha-runner-scale-set-chart/chart-metadata | failing |  |  |  |  |  |
| content:gha-runner-scale-set-chart/helm-values | failing |  |  |  |  |  |
| content:gha-runner-scale-set-controller-chart/chart-metadata | failing |  |  |  |  |  |
| content:gha-runner-scale-set-controller-chart/helm-values | failing |  |  |  |  |  |
| source:release-notes-docs | insufficient |  |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |  |

## Dropped elements

- `artifact:actions-runner-controller` (deterministic): failed validation: 0.25.0 fail (absent from docker.io/summerwind/actions-runner-controller:0.25.0); 0.25.2 fail (absent from docker.io/summerwind/actions-runner-controller:0.25.2); 0.26.0 fail (absent from docker.io/summerwind/actions-runner-controller:0.26.0); 0.27.0 fail (absent from docker.io/summerwind/actions-runner-controller:0.27.0); …
- `artifact:gha-runner-scale-set-controller` (deterministic): failed validation: 0.25.0 fail (absent from ghcr.io/actions/gha-runner-scale-set-controller:0.25.0); 0.25.2 fail (absent from ghcr.io/actions/gha-runner-scale-set-controller:0.25.2); 0.26.0 fail (absent from ghcr.io/actions/gha-runner-scale-set-controller:0.26.0); 0.27.0 fail (absent from ghcr.io/actions/gha-runner-scale-set-controller:0.27.0); …
- `artifact:actions-runner-controller-chart` (deterministic): static validation: error: artifacts[0].channels[0]: template "ghcr.io/${{/actions-runner-controller": template: :1: unexpected "/" in command
- `artifact:gha-runner-scale-set-controller-chart` (deterministic): static validation: error: artifacts[0].channels[1]: template "ghcr.io/${{/gha-runner-scale-set-controller": template: :1: unexpected "/" in command
- `artifact:gha-runner-scale-set-chart` (deterministic): static validation: error: artifacts[0].channels[1]: template "ghcr.io/${{/gha-runner-scale-set": template: :1: unexpected "/" in command

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 149 tags: prefix "v", 61 stable, 0 prereleases (), 88 junk; latest stable v0.27.6; lineage minor; strict tagPattern excludes junk tags such as 0.2.0, 0.3.0, actions-runner-controller-0.1.2, actions-runner-controller-0.10… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (149 tags: prefix "v", 61 stable, 0 prereleases (), 88 junk; latest stable v0.27.6; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-docs | include | notes.per-line-file-with-release-sections | heuristic | One notes file per release line (6 instances) with one section per release (e.g. "# actions-runner-controller v0.27.0"). |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:actions-runner-controller-chart | include | helm.appversion-lookup | heuristic | Chart.yaml at the scanned release still packages the previous chart (appVersion "0.27.5", 1 release(s) behind): chart releases are cut after the product tag; the chart for a release is looked up by appVersion == {{.Versi… |
| artifact:gha-runner-scale-set-controller-chart | include | helm.independent-version | heuristic | Chart has its own version "0.6.1" unrelated to the release. Published via 4 channel(s); OCI locations first. |
| artifact:gha-runner-scale-set-chart | include | helm.independent-version | heuristic | Chart has its own version "0.6.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| candidate:cand-6ff6ab5a5daa | annotate | asset.integrity-file | heuristic | Checksum/signature/provenance asset; recorded but not modelled as an artifact. |
| artifact:actions-runner-controller-manifest | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:crds | include | crd.in-repo | heuristic | 9 CRDs of the product's API groups in config/crd/bases at the release tag. |
| artifact:actions-runner-controller | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which equals the release tag. |
| artifact:gha-runner-scale-set-controller | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which equals the release tag. |
| source:release-notes-docs | annotate | validate.insufficient | computed | Kept unverified: 0.25.0 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?0\\.25\\.0$" in https://github.com/actions/actions-runner-controller/blob/v0.25.0/docs/releasenotes… |
| artifact:gha-runner-scale-set-controller-chart | annotate | validate.unverifiable | computed | Kept unverified: 0.25.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 0.25.2 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:gha-runner-scale-set-chart | annotate | validate.unverifiable | computed | Kept unverified: 0.25.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 0.25.2 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:actions-runner-controller | drop | validate.failed | computed | Relationship check failed: 0.25.0 fail (absent from docker.io/summerwind/actions-runner-controller:0.25.0); 0.25.2 fail (absent from docker.io/summerwind/actions-runner-controller:0.25.2); 0.26.0 fail (absent from docker… |
| artifact:gha-runner-scale-set-controller | drop | validate.failed | computed | Relationship check failed: 0.25.0 fail (absent from ghcr.io/actions/gha-runner-scale-set-controller:0.25.0); 0.25.2 fail (absent from ghcr.io/actions/gha-runner-scale-set-controller:0.25.2); 0.26.0 fail (absent from ghcr… |
| artifact:actions-runner-controller-chart | drop | proposer.static-validation | computed | error: artifacts[0].channels[0]: template "ghcr.io/${{/actions-runner-controller": template: :1: unexpected "/" in command |
| artifact:gha-runner-scale-set-controller-chart | drop | proposer.static-validation | computed | error: artifacts[0].channels[1]: template "ghcr.io/${{/gha-runner-scale-set-controller": template: :1: unexpected "/" in command |
| artifact:gha-runner-scale-set-chart | drop | proposer.static-validation | computed | error: artifacts[0].channels[1]: template "ghcr.io/${{/gha-runner-scale-set": template: :1: unexpected "/" in command |

<details><summary>22 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| helm-chart `actions-runner@contrib/examples/actions-runner` | helm.test-or-library | Chart lives in a test/sample directory or is a library chart. |
| manifest `config/default/manager_auth_proxy_patch.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `config/github-webhook-server/deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `config/github-webhook-server/gh-webhook-server-auth-proxy-patch.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `config/manager/manager.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| image `ghcr.io/actions/actions-runner` | image.non-release-tags | Only snapshot/floating tags (floating) are produced, e.g. by branch builds. |
| image `quay.io/brancz/kube-rbac-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-controller` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-cainjector` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-webhook` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/amazon/aws-cli` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/alpine` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/helmpack/chart-testing` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/controller` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/summerwind/actions-runner` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/actions-runner-controller/actions-runner-controller/actions-runner-dind-rootless` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image-name `actions-runner-dind-rootless.ubuntu-20.04` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `actions-runner-dind-rootless.ubuntu-22.04` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `actions-runner-dind.ubuntu-20.04` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `actions-runner-dind.ubuntu-22.04` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `actions-runner.ubuntu-20.04` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `actions-runner.ubuntu-22.04` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### build-tool (1)

- `chart-releaser` (medium; workflow.chart-releaser) — [actions/actions-runner-controller:.github/workflows/arc-publish-chart.yaml L155](https://github.com/actions/actions-runner-controller/blob/v0.27.6/.github/workflows/arc-publish-chart.yaml#L155): `uses: helm/chart-releaser-action@v1.4.1`

### crd (20)

- `charts/actions-runner-controller/crds/actions.summerwind.dev_horizontalrunnerautoscalers.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/actions-runner-controller/crds/actions.summerwind.dev_horizontalrunnerautoscalers.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/crds/actions.summerwind.dev_horizontalrunnerautoscalers.yaml#L3): `kind: CustomResourceDefinition`
- `charts/actions-runner-controller/crds/actions.summerwind.dev_runnerdeployments.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/actions-runner-controller/crds/actions.summerwind.dev_runnerdeployments.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/crds/actions.summerwind.dev_runnerdeployments.yaml#L3): `kind: CustomResourceDefinition`
- `charts/actions-runner-controller/crds/actions.summerwind.dev_runnerreplicasets.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/actions-runner-controller/crds/actions.summerwind.dev_runnerreplicasets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/crds/actions.summerwind.dev_runnerreplicasets.yaml#L3): `kind: CustomResourceDefinition`
- `charts/actions-runner-controller/crds/actions.summerwind.dev_runners.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/actions-runner-controller/crds/actions.summerwind.dev_runners.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/crds/actions.summerwind.dev_runners.yaml#L3): `kind: CustomResourceDefinition`
- `charts/actions-runner-controller/crds/actions.summerwind.dev_runnersets.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/actions-runner-controller/crds/actions.summerwind.dev_runnersets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/crds/actions.summerwind.dev_runnersets.yaml#L3): `kind: CustomResourceDefinition`
- `charts/gha-runner-scale-set-controller/crds/actions.github.com_autoscalinglisteners.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/gha-runner-scale-set-controller/crds/actions.github.com_autoscalinglisteners.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/crds/actions.github.com_autoscalinglisteners.yaml#L3): `kind: CustomResourceDefinition`
- `charts/gha-runner-scale-set-controller/crds/actions.github.com_autoscalingrunnersets.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/gha-runner-scale-set-controller/crds/actions.github.com_autoscalingrunnersets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/crds/actions.github.com_autoscalingrunnersets.yaml#L3): `kind: CustomResourceDefinition`
- `charts/gha-runner-scale-set-controller/crds/actions.github.com_ephemeralrunners.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/gha-runner-scale-set-controller/crds/actions.github.com_ephemeralrunners.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/crds/actions.github.com_ephemeralrunners.yaml#L3): `kind: CustomResourceDefinition`
- `charts/gha-runner-scale-set-controller/crds/actions.github.com_ephemeralrunnersets.yaml` (high; crd.file) — [actions/actions-runner-controller:charts/gha-runner-scale-set-controller/crds/actions.github.com_ephemeralrunnersets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/crds/actions.github.com_ephemeralrunnersets.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.github.com_autoscalinglisteners.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.github.com_autoscalinglisteners.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.github.com_autoscalinglisteners.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.github.com_autoscalingrunnersets.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.github.com_autoscalingrunnersets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.github.com_autoscalingrunnersets.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.github.com_ephemeralrunners.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.github.com_ephemeralrunners.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.github.com_ephemeralrunners.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.github.com_ephemeralrunnersets.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.github.com_ephemeralrunnersets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.github.com_ephemeralrunnersets.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.summerwind.dev_horizontalrunnerautoscalers.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.summerwind.dev_horizontalrunnerautoscalers.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.summerwind.dev_horizontalrunnerautoscalers.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.summerwind.dev_runnerdeployments.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.summerwind.dev_runnerdeployments.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.summerwind.dev_runnerdeployments.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.summerwind.dev_runnerreplicasets.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.summerwind.dev_runnerreplicasets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.summerwind.dev_runnerreplicasets.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.summerwind.dev_runners.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.summerwind.dev_runners.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.summerwind.dev_runners.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/bases/actions.summerwind.dev_runnersets.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/bases/actions.summerwind.dev_runnersets.yaml L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/bases/actions.summerwind.dev_runnersets.yaml#L3): `kind: CustomResourceDefinition`
- `config/crd/patches/cainjection_in_runners.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/patches/cainjection_in_runners.yaml L4](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/patches/cainjection_in_runners.yaml#L4): `kind: CustomResourceDefinition`
- `config/crd/patches/webhook_in_runners.yaml` (high; crd.file) — [actions/actions-runner-controller:config/crd/patches/webhook_in_runners.yaml L4](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/crd/patches/webhook_in_runners.yaml#L4): `kind: CustomResourceDefinition`

### helm-chart (4)

- `actions-runner-controller@charts/actions-runner-controller` (high; helm.chart) — [actions/actions-runner-controller:charts/actions-runner-controller/Chart.yaml L2](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/Chart.yaml#L2): `name: actions-runner-controller`
- `gha-runner-scale-set-controller@charts/gha-runner-scale-set-controller` (high; helm.chart) — [actions/actions-runner-controller:charts/gha-runner-scale-set-controller/Chart.yaml L2](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/Chart.yaml#L2): `name: gha-runner-scale-set-controller`
- `gha-runner-scale-set@charts/gha-runner-scale-set` (high; helm.chart) — [actions/actions-runner-controller:charts/gha-runner-scale-set/Chart.yaml L2](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set/Chart.yaml#L2): `name: gha-runner-scale-set`
- `actions-runner@contrib/examples/actions-runner` (low; helm.chart) — [actions/actions-runner-controller:contrib/examples/actions-runner/Chart.yaml L2](https://github.com/actions/actions-runner-controller/blob/v0.27.6/contrib/examples/actions-runner/Chart.yaml#L2): `name: actions-runner`

### helm-oci (4)

- `ghcr.io/${{` (medium; workflow.helm-push) — [actions/actions-runner-controller:.github/workflows/gha-publish-chart.yaml L154](https://github.com/actions/actions-runner-controller/blob/v0.27.6/.github/workflows/gha-publish-chart.yaml#L154): `helm push gha-runner-scale-set-controller-"${GHA_RUNNER_SCALE_SET_CONTROLLER_CHART_VERSION_TAG}".tgz oci://ghc…`
- `ghcr.io/actions` (medium; docs.oci-ref) — [actions/actions-runner-controller:docs/adrs/2022-11-04-crd-api-group-name.md L54](https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/adrs/2022-11-04-crd-api-group-name.md#L54): `'helm repo remove actions-runner-controller' and 'helm repo add actions-runner-controller oci://ghcr.io/action…`
- `ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set` (medium; docs.oci-ref) — [actions/actions-runner-controller:docs/adrs/2023-02-10-limit-manager-role-permission.md L99](https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/adrs/2023-02-10-limit-manager-role-permission.md#L99): `The 'Role' and 'RoleBinding' creation will happen during 'helm install demo oci://ghcr.io/actions/actions-runn…`
- `ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set-controller` (medium; docs.oci-ref) — [actions/actions-runner-controller:docs/adrs/2023-02-10-limit-manager-role-permission.md L80](https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/adrs/2023-02-10-limit-manager-role-permission.md#L80): `The 'Role' and 'RoleBinding' creation will happen during the 'helm install demo oci://ghcr.io/actions/actions-…`

### helm-repo (3)

- `https://actions-runner-controller.github.io/actions-runner-controller` (medium; docs.helm-repo-add) — [actions/actions-runner-controller:docs/installing-arc.md L23](https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/installing-arc.md#L23): `helm repo add actions-runner-controller https://actions-runner-controller.github.io/actions-runner-controller`
- `https://actions.github.io/actions-runner-controller` (medium; workflow.chart-releaser-pages) — [actions/actions-runner-controller:.github/workflows/arc-publish-chart.yaml L155](https://github.com/actions/actions-runner-controller/blob/v0.27.6/.github/workflows/arc-publish-chart.yaml#L155): `uses: helm/chart-releaser-action@v1.4.1`
- `https://raw.githubusercontent.com/actions/actions-runner-controller/gh-pages` (medium; workflow.chart-releaser-pages-raw) — [actions/actions-runner-controller:.github/workflows/arc-publish-chart.yaml L155](https://github.com/actions/actions-runner-controller/blob/v0.27.6/.github/workflows/arc-publish-chart.yaml#L155): `uses: helm/chart-releaser-action@v1.4.1`

### image (13)

- `docker.io/summerwind/actions-runner-controller` (high; helm.values-image, kustomize.image) — [actions/actions-runner-controller:charts/actions-runner-controller/values.yaml L53](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/values.yaml#L53): `repository: "summerwind/actions-runner-controller"`
- `ghcr.io/actions/gha-runner-scale-set-controller` (high; helm.values-image) — [actions/actions-runner-controller:charts/gha-runner-scale-set-controller/values.yaml L12](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/gha-runner-scale-set-controller/values.yaml#L12): `repository: "ghcr.io/actions/gha-runner-scale-set-controller"`
- `quay.io/brancz/kube-rbac-proxy` (high; helm.values-image, make.image-ref, manifest.image) — [actions/actions-runner-controller:Makefile L256](https://github.com/actions/actions-runner-controller/blob/v0.27.6/Makefile#L256): `kind load docker-image quay.io/brancz/kube-rbac-proxy:$(KUBE_RBAC_PROXY_VERSION) --name ${CLUSTER}`
- `docker.io/library/controller` (medium; manifest.image) — [actions/actions-runner-controller:config/github-webhook-server/deployment.yaml L22](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/github-webhook-server/deployment.yaml#L22): `image: controller:latest`
- `ghcr.io/actions-runner-controller/actions-runner-controller/actions-runner-dind-rootless` (medium; docs.image-ref) — [actions/actions-runner-controller:docs/releasenotes/0.27.md L112](https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/releasenotes/0.27.md#L112): `ghcr.io/actions-runner-controller/actions-runner-controller/actions-runner-dind-rootless:v2.299.1-ubuntu-22.04`
- `ghcr.io/actions/actions-runner` (medium; helm.values-image, workflow.image-ref) — [actions/actions-runner-controller:.github/workflows/gha-e2e-tests.yaml L815](https://github.com/actions/actions-runner-controller/blob/v0.27.6/.github/workflows/gha-e2e-tests.yaml#L815): `--set template.spec.containers[0].image="ghcr.io/actions/actions-runner:latest" \`
- `quay.io/helmpack/chart-testing` (medium; script.image-ref) — [actions/actions-runner-controller:charts/.ci/scripts/local-ct-lint.sh L3](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/.ci/scripts/local-ct-lint.sh#L3): `docker run --rm -it -w /repo -v $(pwd):/repo quay.io/helmpack/chart-testing ct lint --all --config charts/.ci/…`
- `quay.io/jetstack/cert-manager-cainjector` (medium; make.image-ref) — [actions/actions-runner-controller:Makefile L260](https://github.com/actions/actions-runner-controller/blob/v0.27.6/Makefile#L260): `kind load docker-image quay.io/jetstack/cert-manager-cainjector:$(CERT_MANAGER_VERSION) --name ${CLUSTER}`
- `quay.io/jetstack/cert-manager-controller` (medium; make.image-ref) — [actions/actions-runner-controller:Makefile L259](https://github.com/actions/actions-runner-controller/blob/v0.27.6/Makefile#L259): `kind load docker-image quay.io/jetstack/cert-manager-controller:$(CERT_MANAGER_VERSION) --name ${CLUSTER}`
- `quay.io/jetstack/cert-manager-webhook` (medium; make.image-ref) — [actions/actions-runner-controller:Makefile L261](https://github.com/actions/actions-runner-controller/blob/v0.27.6/Makefile#L261): `kind load docker-image quay.io/jetstack/cert-manager-webhook:$(CERT_MANAGER_VERSION) --name ${CLUSTER}`
- `docker.io/amazon/aws-cli` (low; manifest.image) — [actions/actions-runner-controller:acceptance/pipelines/eks-integration-tests.yaml L24](https://github.com/actions/actions-runner-controller/blob/v0.27.6/acceptance/pipelines/eks-integration-tests.yaml#L24): `image: amazon/aws-cli`
- `docker.io/library/alpine` (low; manifest.image) — [actions/actions-runner-controller:acceptance/pipelines/runner-integration-tests.yaml L13](https://github.com/actions/actions-runner-controller/blob/v0.27.6/acceptance/pipelines/runner-integration-tests.yaml#L13): `image: alpine`
- `docker.io/summerwind/actions-runner` (low; helm.values-image) — [actions/actions-runner-controller:contrib/examples/actions-runner/values.yaml L2](https://github.com/actions/actions-runner-controller/blob/v0.27.6/contrib/examples/actions-runner/values.yaml#L2): `repository: summerwind/actions-runner`

### image-name (6)

- `actions-runner-dind-rootless.ubuntu-20.04` (low; dockerfile.name) — [actions/actions-runner-controller:runner/actions-runner-dind-rootless.ubuntu-20.04.dockerfile L1](https://github.com/actions/actions-runner-controller/blob/v0.27.6/runner/actions-runner-dind-rootless.ubuntu-20.04.dockerfile#L1): `FROM ubuntu:20.04`
- `actions-runner-dind-rootless.ubuntu-22.04` (low; dockerfile.name) — [actions/actions-runner-controller:runner/actions-runner-dind-rootless.ubuntu-22.04.dockerfile L1](https://github.com/actions/actions-runner-controller/blob/v0.27.6/runner/actions-runner-dind-rootless.ubuntu-22.04.dockerfile#L1): `FROM ubuntu:22.04`
- `actions-runner-dind.ubuntu-20.04` (low; dockerfile.name) — [actions/actions-runner-controller:runner/actions-runner-dind.ubuntu-20.04.dockerfile L1](https://github.com/actions/actions-runner-controller/blob/v0.27.6/runner/actions-runner-dind.ubuntu-20.04.dockerfile#L1): `FROM ubuntu:20.04`
- `actions-runner-dind.ubuntu-22.04` (low; dockerfile.name) — [actions/actions-runner-controller:runner/actions-runner-dind.ubuntu-22.04.dockerfile L1](https://github.com/actions/actions-runner-controller/blob/v0.27.6/runner/actions-runner-dind.ubuntu-22.04.dockerfile#L1): `FROM ubuntu:22.04`
- `actions-runner.ubuntu-20.04` (low; dockerfile.name) — [actions/actions-runner-controller:runner/actions-runner.ubuntu-20.04.dockerfile L1](https://github.com/actions/actions-runner-controller/blob/v0.27.6/runner/actions-runner.ubuntu-20.04.dockerfile#L1): `FROM ubuntu:20.04`
- `actions-runner.ubuntu-22.04` (low; dockerfile.name) — [actions/actions-runner-controller:runner/actions-runner.ubuntu-22.04.dockerfile L1](https://github.com/actions/actions-runner-controller/blob/v0.27.6/runner/actions-runner.ubuntu-22.04.dockerfile#L1): `FROM ubuntu:22.04`

### manifest (4)

- `config/default/manager_auth_proxy_patch.yaml` (high; manifest.workloads) — [actions/actions-runner-controller:config/default/manager_auth_proxy_patch.yaml L4](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/default/manager_auth_proxy_patch.yaml#L4): `kind: Deployment`
- `config/github-webhook-server/deployment.yaml` (high; manifest.workloads) — [actions/actions-runner-controller:config/github-webhook-server/deployment.yaml L2](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/github-webhook-server/deployment.yaml#L2): `kind: Deployment`
- `config/github-webhook-server/gh-webhook-server-auth-proxy-patch.yaml` (high; manifest.workloads) — [actions/actions-runner-controller:config/github-webhook-server/gh-webhook-server-auth-proxy-patch.yaml L4](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/github-webhook-server/gh-webhook-server-auth-proxy-patch.yaml#L4): `kind: Deployment`
- `config/manager/manager.yaml` (high; manifest.workloads) — [actions/actions-runner-controller:config/manager/manager.yaml L9](https://github.com/actions/actions-runner-controller/blob/v0.27.6/config/manager/manager.yaml#L9): `kind: Deployment`

### registry (1)

- `ghcr.io` (medium; workflow.registry-login) — [actions/actions-runner-controller:.github/workflows/gha-publish-chart.yaml L89](https://github.com/actions/actions-runner-controller/blob/v0.27.6/.github/workflows/gha-publish-chart.yaml#L89): `registry: ghcr.io`

### release-asset (3)

- `https://github.com/actions/actions-runner-controller/releases/download/{{.Tag}}/actions-runner-controller.yaml` (medium; docs.release-asset-url) — [actions/actions-runner-controller:docs/installing-arc.md L15](https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/installing-arc.md#L15): `kubectl create -f https://github.com/actions/actions-runner-controller/releases/download/v0.25.2/actions-runne…`
- `https://github.com/actions/actions-runner-controller/releases/download/{{.Tag}}/actions-runner-controller.yaml.asc` (medium; docs.release-asset-url) — [actions/actions-runner-controller:hack/signrel/README.md L18](https://github.com/actions/actions-runner-controller/blob/v0.27.6/hack/signrel/README.md#L18): `curl -LO https://github.com/actions/actions-runner-controller/releases/download/v0.23.0/actions-runner-control…`
- `https://github.com/actions/actions-runner-controller/releases/download/actions-runner-controller-${CHART_VERSION}/action…` (low; docs.release-asset-url) — [actions/actions-runner-controller:charts/actions-runner-controller/docs/UPGRADING.md L27](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/docs/UPGRADING.md#L27): `curl -L https://github.com/actions/actions-runner-controller/releases/download/actions-runner-controller-${CHA…`

### release-notes (1)

- `docs/releasenotes/{{.Major}}.{{.Minor}}.md` (high; paths.versioned-release-notes) — [actions/actions-runner-controller:docs/releasenotes/0.27.md ](https://github.com/actions/actions-runner-controller/blob/v0.27.6/docs/releasenotes/0.27.md): `file docs/releasenotes/0.27.md`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [actions/actions-runner-controller:SECURITY.md L1](https://github.com/actions/actions-runner-controller/blob/v0.27.6/SECURITY.md#L1): `Thanks for helping make GitHub safe for everyone.`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+)$` (high; tags.ls-remote) — [actions/actions-runner-controller/releases/tag/v0.27.6 refs/tags/v0.27.6](https://github.com/actions/actions-runner-controller/releases/tag/v0.27.6#refs/tags/v0.27.6): `b511953df7a8a752faf0a0f840bd832e02796c10 refs/tags/v0.27.6`

### version-relation (1)

- `chart.appVersion = {{.Version}}` (high; helm.chart-appversion-lags-release) — [actions/actions-runner-controller:charts/actions-runner-controller/Chart.yaml L21](https://github.com/actions/actions-runner-controller/blob/v0.27.6/charts/actions-runner-controller/Chart.yaml#L21): `appVersion: 0.27.5`
