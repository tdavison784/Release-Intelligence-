# Discovery report: crossplane

- Repository: `github.com/crossplane/crossplane` at `v2.4.2` (commit `bf8ffd4048a0…`)
- Generated: 2026-10-02T12:33:28Z
- LLM: not used (deterministic resolver only)
- Tags: 239 tags: prefix "v", 169 stable, 45 prereleases (preview.N×2, rc.N×43), 25 junk; latest stable v2.4.2; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:preview\.\d+|rc\.\d+))?)$` (junk sample: apis/v2.3.0, apis/v2.3.0-rc.1, apis/v2.3.1, apis/v2.3.2, apis/v2.3.3, apis/v2.3.4)
- Scanned `github.com/crossplane/crossplane@v2.4.2` (source profile): 950 files listed, 478 read
- Scanned `github.com/crossplane/docs@master` (docs profile): 1247 files listed, 93 read
- Validation: done against v2.2.0, v2.2.6, v2.3.0, v2.3.6, v2.4.0, v2.4.2

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 1 | git-tags github.com/crossplane/crossplane |
| release-notes | found | 1 | repo-file github.com/crossplane/docs content/v{{.Major}}.{{.Minor}}/whats-new/_index.md @master |
| changelog | not-found | 0 |  |
| helm-charts | found | 3 | crossplane via helm-repo:https://charts.crossplane.io/stable |
| registries | found | 4 | ghcr.io/crossplane |
| images | found | 2 | ghcr.io/crossplane/crossplane:{{.Tag}} |
| upgrade-docs | found | 2 | repo-file github.com/crossplane/docs content/v{{.Major}}.{{.Minor}}/guides/upgrade-crossplane.md @master |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories crossplane/crossplane |
| version-relations | found | 0 | crossplane-chart: version = {{.Version}}; crds: version = {{.Tag}}; crossplane: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:crds | historically-validated | crd.in-repo | https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_compositeresourcedefinitions.yaml#L3 (+2) |
| artifact:crossplane | historically-validated | image.tag-from-release | https://github.com/crossplane/crossplane/blob/v2.4.2/.github/workflows/ci.yml#L316 |
| artifact:crossplane-chart | historically-validated | helm.placeholder-version-follows-release | https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/charts/crossplane/Chart.yaml#L4 (+1) |
| source:release-notes-line | historically-validated | notes.per-line-file | https://github.com/crossplane/docs/blob/d1c9eb7fd1548d0d61ab3901781919a5b4aaf69d/content/v2.4/whats-new/_index.md# |
| source:upgrade-guide | historically-validated | upgrade.versioned-doc | https://github.com/crossplane/docs/blob/d1c9eb7fd1548d0d61ab3901781919a5b4aaf69d/content/v2.4/guides/upgrade-crossplane.md# |
| source:advisories | discovered | security.github-hosted-default | https://github.com/crossplane/crossplane/blob/v2.4.2/SECURITY.md#L1 |
| source:tags | discovered | versions.git-tags | https://github.com/crossplane/crossplane/releases/tag/v2.4.2#refs/tags/v2.4.2 |
| versioning | discovered | tags.scheme | https://github.com/crossplane/crossplane/releases/tag/v2.4.2#refs/tags/v2.4.2 |

Statuses: 5 historically-validated, 3 discovered.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: crossplane
name: crossplane
homepage: https://github.com/crossplane/crossplane
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:preview\.\d+|rc\.\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/crossplane/crossplane
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:preview\.\d+|rc\.\d+))?)$
  - id: release-notes-line
    roles:
      - release-notes
    locator:
      kind: repo-file
      repository: github.com/crossplane/docs
      ref: master
      path: content/v{{.Major}}.{{.Minor}}/whats-new/_index.md
    releaseKinds:
      - minor
      - major
    fallbackGroup: release-notes
    validatedAgainst:
      - 2.2.0
      - 2.3.0
      - 2.4.0
  - id: upgrade-guide
    roles:
      - upgrade-guide
    locator:
      kind: repo-file
      repository: github.com/crossplane/docs
      ref: master
      path: content/v{{.Major}}.{{.Minor}}/guides/upgrade-crossplane.md
    releaseKinds:
      - minor
      - major
    validatedAgainst:
      - 2.2.0
      - 2.3.0
      - 2.4.0
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: crossplane/crossplane
    notes: 'Security policy: SECURITY.md → https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing/privately-reporting-a-security-vulnerability https://github.com/crossplane/crossplane/security https://docs.github.com/en/code-security/security-advisories/repository-security-advisories/about-repository-security-advisories'
artifacts:
  - id: crossplane-chart
    type: helm-chart
    name: crossplane
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: helm-repo
        url: https://charts.crossplane.io/stable
        chart: crossplane
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/crossplane/crossplane
          path: cluster/charts/crossplane/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/crossplane/crossplane
          path: cluster/charts/crossplane/Chart.yaml
    validatedAgainst:
      - 2.2.0
      - 2.2.6
      - 2.3.0
      - 2.3.6
      - 2.4.0
      - 2.4.2
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/crossplane/crossplane
        path: cluster/crds
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 2.2.0
      - 2.2.6
      - 2.3.0
      - 2.3.6
      - 2.4.0
      - 2.4.2
  - id: crossplane
    type: container-image
    name: crossplane
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: ghcr.io/crossplane/crossplane
    validatedAgainst:
      - 2.2.0
      - 2.2.6
      - 2.3.0
      - 2.3.6
      - 2.4.0
      - 2.4.2
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - v2.2.0
    - v2.2.6
    - v2.3.0
    - v2.3.6
    - v2.4.0
    - v2.4.2
  notes: Proposed by automated discovery; relationship checks run against v2.2.0, v2.2.6, v2.3.0, v2.3.6, v2.4.0, v2.4.2.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 4 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, compatibility source was derived from it.
- Ambiguity (chart-version): How do the versions of chart(s) crossplane relate to the release version?

## Validation matrix

| Element | Verdict | v2.2.0 | v2.2.6 | v2.3.0 | v2.3.6 | v2.4.0 | v2.4.2 |
|---|---|---|---|---|---|---|---|
| artifact:crds | validated |  |  |  |  |  |  |
| artifact:crossplane | validated |  |  |  |  |  |  |
| artifact:crossplane-chart | validated |  |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |  |
| content:crossplane-chart/chart-metadata | validated |  |  |  |  |  |  |
| content:crossplane-chart/helm-values | validated |  |  |  |  |  |  |
| source:release-notes-line | validated |  |  |  |  |  |  |
| source:upgrade-guide | validated |  |  |  |  |  |  |

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 239 tags: prefix "v", 169 stable, 45 prereleases (preview.N×2, rc.N×43), 25 junk; latest stable v2.4.2; lineage minor; strict tagPattern excludes junk tags such as apis/v2.3.0, apis/v2.3.0-rc.1, apis/v2.3.1, apis/v2.3.… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (239 tags: prefix "v", 169 stable, 45 prereleases (preview.N×2, rc.N×43), 25 junk; latest stable v2.4.2; lineage minor). |
| source:release-notes-line | include | notes.per-line-file | heuristic | One notes document per release line (3 instances, newest content/v2.4/whats-new/_index.md), used for X.Y.0 releases. |
| source:upgrade-guide | include | upgrade.versioned-doc | heuristic | One upgrade document per release line (3 instances, newest content/v2.4/guides/upgrade-crossplane.md). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:crossplane-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "0.0.1" replaced at release time; assumed to equal the release ({{.Version}}). Published via 1 channel(s); OCI locations first. |
| artifact:crds | include | crd.in-repo | heuristic | 21 CRDs of the product's API groups in cluster/crds at the release tag. |
| artifact:crossplane | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |

<details><summary>12 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| upgrade-guide `content/v{{.Major}}.{{.Minor}}/guides/upgrade-to-crossplane-v2.md` | upgrade.lower-ranked-template | Another upgrade-guide path template ranks higher or this one does not cover the newest release lines. |
| helm-repo `https://charts.crossplane.io/master` | helm.dev-channel | Chart repository path is a development channel (unreleased main builds); a stable channel of the same chart exists. |
| crd `test/e2e/manifests/apiextensions/activation-policy/single-activation/expected-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/apiextensions/activation-policy/wildcard-activation/expected-crds/bucke…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/apiextensions/activation-policy/wildcard-activation/expected-crds/datab…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/apiextensions/activation-policy/wildcard-activation/expected-crds/insta…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/apiextensions/composition/lack-of-rights-namespaced/setup/custom-resour…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/apiextensions/xrd/subresources/crd-scale.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/protection/usage/standalone-cluster/with-by/used.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/protection/usage/standalone-cluster/with-by/using.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/e2e/manifests/protection/usage/standalone-cluster/with-reason/used.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| image `docker.io/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |

</details>

## Candidates


### crd (33)

- `cluster/crds/apiextensions.crossplane.io_compositeresourcedefinitions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/apiextensions.crossplane.io_compositeresourcedefinitions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_compositeresourcedefinitions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/apiextensions.crossplane.io_compositionrevisions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/apiextensions.crossplane.io_compositionrevisions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_compositionrevisions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/apiextensions.crossplane.io_compositions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/apiextensions.crossplane.io_compositions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_compositions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/apiextensions.crossplane.io_environmentconfigs.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/apiextensions.crossplane.io_environmentconfigs.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_environmentconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/apiextensions.crossplane.io_managedresourceactivationpolicies.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/apiextensions.crossplane.io_managedresourceactivationpolicies.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_managedresourceactivationpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/apiextensions.crossplane.io_managedresourcedefinitions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/apiextensions.crossplane.io_managedresourcedefinitions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_managedresourcedefinitions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/apiextensions.crossplane.io_usages.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/apiextensions.crossplane.io_usages.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/apiextensions.crossplane.io_usages.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/ops.crossplane.io_cronoperations.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/ops.crossplane.io_cronoperations.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/ops.crossplane.io_cronoperations.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/ops.crossplane.io_operations.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/ops.crossplane.io_operations.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/ops.crossplane.io_operations.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/ops.crossplane.io_watchoperations.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/ops.crossplane.io_watchoperations.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/ops.crossplane.io_watchoperations.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_configurationrevisions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_configurationrevisions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_configurationrevisions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_configurations.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_configurations.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_configurations.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_deploymentruntimeconfigs.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_deploymentruntimeconfigs.yaml L2](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_deploymentruntimeconfigs.yaml#L2): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_functionrevisions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_functionrevisions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_functionrevisions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_functions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_functions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_functions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_imageconfigs.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_imageconfigs.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_imageconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_locks.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_locks.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_locks.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_providerrevisions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_providerrevisions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_providerrevisions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/pkg.crossplane.io_providers.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/pkg.crossplane.io_providers.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/pkg.crossplane.io_providers.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/protection.crossplane.io_clusterusages.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/protection.crossplane.io_clusterusages.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/protection.crossplane.io_clusterusages.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/crds/protection.crossplane.io_usages.yaml` (high; crd.file) — [crossplane/crossplane:cluster/crds/protection.crossplane.io_usages.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/crds/protection.crossplane.io_usages.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/meta/meta.pkg.crossplane.io_configurations.yaml` (high; crd.file) — [crossplane/crossplane:cluster/meta/meta.pkg.crossplane.io_configurations.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/meta/meta.pkg.crossplane.io_configurations.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/meta/meta.pkg.crossplane.io_functions.yaml` (high; crd.file) — [crossplane/crossplane:cluster/meta/meta.pkg.crossplane.io_functions.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/meta/meta.pkg.crossplane.io_functions.yaml#L3): `kind: CustomResourceDefinition`
- `cluster/meta/meta.pkg.crossplane.io_providers.yaml` (high; crd.file) — [crossplane/crossplane:cluster/meta/meta.pkg.crossplane.io_providers.yaml L3](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/meta/meta.pkg.crossplane.io_providers.yaml#L3): `kind: CustomResourceDefinition`
- `test/e2e/manifests/apiextensions/activation-policy/single-activation/expected-crd.yaml` (low; crd.file) — [crossplane/crossplane:test/e2e/manifests/apiextensions/activation-policy/single-activation/expected-crd.yaml L2](https://github.com/crossplane/crossplane/blob/v2.4.2/test/e2e/manifests/apiextensions/activation-policy/single-activation/expected-crd.yaml#L2): `kind: CustomResourceDefinition`
- … 8 more

### docs-repo (1)

- `github.com/crossplane/docs` (medium; docs.docs-repo-ref) — [crossplane/crossplane:contributing/README.md L983](https://github.com/crossplane/crossplane/blob/v2.4.2/contributing/README.md#L983): `[docs]: https://github.com/crossplane/docs`

### helm-chart (1)

- `crossplane@cluster/charts/crossplane` (high; helm.chart) — [crossplane/crossplane:cluster/charts/crossplane/Chart.yaml L4](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/charts/crossplane/Chart.yaml#L4): `name: crossplane`

### helm-repo (2)

- `https://charts.crossplane.io/master` (medium; docs.helm-repo-add) — [crossplane/crossplane:cluster/charts/crossplane/README.md L43](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/charts/crossplane/README.md#L43): `helm repo add crossplane-master https://charts.crossplane.io/master/`
- `https://charts.crossplane.io/stable` (medium; docs.helm-repo-add) — [crossplane/crossplane:cluster/charts/crossplane/README.md L24](https://github.com/crossplane/crossplane/blob/v2.4.2/cluster/charts/crossplane/README.md#L24): `helm repo add crossplane-stable https://charts.crossplane.io/stable`

### image (2)

- `ghcr.io/crossplane/crossplane` (medium; workflow.image-ref) — [crossplane/crossplane:.github/workflows/ci.yml L316](https://github.com/crossplane/crossplane/blob/v2.4.2/.github/workflows/ci.yml#L316): `run: nix run --option warn-dirty false .#push-images -- ghcr.io/crossplane/crossplane`
- `docker.io/library/busybox` (low; manifest.image) — [crossplane/crossplane:test/e2e/manifests/pkg/deployment-runtime-config/setup/deployment-runtime-config.yaml L40](https://github.com/crossplane/crossplane/blob/v2.4.2/test/e2e/manifests/pkg/deployment-runtime-config/setup/deployment-runtime-config.yaml#L40): `image: busybox`

### registry (4)

- `docker.io` (medium; workflow.registry-login) — [crossplane/crossplane:.github/workflows/ci.yml L285](https://github.com/crossplane/crossplane/blob/v2.4.2/.github/workflows/ci.yml#L285): `uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4`
- `ghcr.io` (medium; workflow.registry-login) — [crossplane/crossplane:.github/workflows/ci.yml L302](https://github.com/crossplane/crossplane/blob/v2.4.2/.github/workflows/ci.yml#L302): `registry: ghcr.io`
- `docker.io/crossplanecontrib` (low; docs.registry-ref) — [crossplane/docs:content/master/guides/extensions-release-process.md L262](https://github.com/crossplane/docs/blob/d1c9eb7fd1548d0d61ab3901781919a5b4aaf69d/content/master/guides/extensions-release-process.md#L262): `XPKG_REG_ORGS ?= xpkg.crossplane.io/crossplane-contrib index.docker.io/crossplanecontrib`
- `ghcr.io/crossplane-contrib` (low; docs.registry-ref) — [crossplane/crossplane:GOVERNANCE.md L368](https://github.com/crossplane/crossplane/blob/v2.4.2/GOVERNANCE.md#L368): `from the 'ghcr.io/crossplane-contrib' organization. 'xpkg.crossplane.io' is`

### release-notes (1)

- `content/v{{.Major}}.{{.Minor}}/whats-new/_index.md` (high; paths.versioned-release-notes) — [crossplane/docs:content/v2.4/whats-new/_index.md ](https://github.com/crossplane/docs/blob/d1c9eb7fd1548d0d61ab3901781919a5b4aaf69d/content/v2.4/whats-new/_index.md): `file content/v2.4/whats-new/_index.md`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [crossplane/crossplane:SECURITY.md L1](https://github.com/crossplane/crossplane/blob/v2.4.2/SECURITY.md#L1): `# Security Policy`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-(?:preview\.\d+|rc\.\d+))?)$` (high; tags.ls-remote) — [crossplane/crossplane/releases/tag/v2.4.2 refs/tags/v2.4.2](https://github.com/crossplane/crossplane/releases/tag/v2.4.2#refs/tags/v2.4.2): `bf8ffd4048a0bb1b408b8b745e31960c717009cb refs/tags/v2.4.2`

### upgrade-guide (2)

- `content/v{{.Major}}.{{.Minor}}/guides/upgrade-crossplane.md` (high; paths.versioned-upgrade-guide) — [crossplane/docs:content/v2.4/guides/upgrade-crossplane.md ](https://github.com/crossplane/docs/blob/d1c9eb7fd1548d0d61ab3901781919a5b4aaf69d/content/v2.4/guides/upgrade-crossplane.md): `file content/v2.4/guides/upgrade-crossplane.md`
- `content/v{{.Major}}.{{.Minor}}/guides/upgrade-to-crossplane-v2.md` (high; paths.versioned-upgrade-guide) — [crossplane/docs:content/v2.4/guides/upgrade-to-crossplane-v2.md ](https://github.com/crossplane/docs/blob/d1c9eb7fd1548d0d61ab3901781919a5b4aaf69d/content/v2.4/guides/upgrade-to-crossplane-v2.md): `file content/v2.4/guides/upgrade-to-crossplane-v2.md`
