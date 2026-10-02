# Discovery report: strimzi-kafka-operator

- Repository: `github.com/strimzi/strimzi-kafka-operator` at `1.2.0` (commit `6c7b43c4af0d…`)
- Generated: 2026-10-02T12:38:03Z
- LLM: not used (deterministic resolver only)
- Tags: 182 tags: prefix "", 81 stable, 100 prereleases (rcN×100), 1 junk; latest stable 1.2.0; lineage linear
- Scanned `github.com/strimzi/strimzi-kafka-operator@1.2.0` (source profile): 2700 files listed, 1108 read
- Scanned `github.com/strimzi/strimzi.github.io@main` (docs profile): 5354 files listed, 23 read
- Validation: done against 1.0.0, 1.0.1, 1.1.0, 1.2.0

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 6 | git-tags github.com/strimzi/strimzi-kafka-operator; github-releases strimzi/strimzi-kafka-operator |
| release-notes | found | 0 | github-releases strimzi/strimzi-kafka-operator  @{{.Tag}} |
| changelog | found | 1 | repo-file github.com/strimzi/strimzi-kafka-operator CHANGELOG.md |
| helm-charts | found | 6 | strimzi-kafka-operator via oci:quay.io/strimzi-helm/strimzi-kafka-operator, helm-repo:https://strimzi.io/charts; strimzi-kafka-operator via oci:quay.io/strimzi-helm/strimzi-kafka-operator, helm-repo:https://strimzi.io/ch… |
| registries | found | 6 | quay.io/strimzi |
| images | found | 24 | quay.io/strimzi/operator:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 2 | github-advisories strimzi/strimzi-kafka-operator |
| version-relations | found | 0 | strimzi-kafka-operator-chart: independent; strimzi-kafka-operator-chart-2: independent; install-cluster-operator-060-deployment-strimzi-cluster-oper: version = {{.Tag}}; install-topic-operator-05-deployment-strimzi-topic… |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:install-cluster-operator-060-deployment-strimzi-cluster-oper | historically-validated | manifest.in-repo-install | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml#L2 |
| artifact:install-topic-operator-05-deployment-strimzi-topic-operator | historically-validated | manifest.in-repo-install | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/topic-operator/05-Deployment-strimzi-topic-operator.yaml#L2 |
| artifact:install-user-operator-05-deployment-strimzi-user-operator | historically-validated | manifest.in-repo-install | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/user-operator/05-Deployment-strimzi-user-operator.yaml#L2 |
| artifact:operator | historically-validated | image.tag-from-release | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.azure/scripts/setup_upgrade.sh#L18 |
| artifact:packaging-install-cluster-operator-060-deployment-strimzi-cl | historically-validated | manifest.in-repo-install | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml#L2 |
| artifact:strimzi-binary | historically-validated | asset.hosted-release-download | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/upgrade/README.md#L26 |
| artifact:strimzi-crds | historically-validated | asset.hosted-release-download | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/README.md#L54 |
| source:changelog | historically-validated | changelog.file | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/CHANGELOG.md#L3 |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/README.md#L54 (+2) |
| artifact:api-conversion-binary | unverified | asset.hosted-release-download | https://github.com/strimzi/strimzi.github.io/blob/b1b73f86942532c4a081a63d6d3237d441e79c80/_posts/2021-04-29-api-conversion.md#L43 |
| artifact:strimzi-kafka-operator-chart | unverified | helm.independent-version | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/Chart.yaml#L4 (+2) |
| artifact:strimzi-kafka-operator-chart-2 | unverified | helm.independent-version | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/helm-charts/helm3/strimzi-kafka-operator/Chart.yaml#L4 (+2) |
| source:advisories | discovered | security.github-hosted-default | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/development-docs/systemtests/labels/security.md#L1 (+1) |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/README.md#L54 (+2) |
| source:tags | discovered | versions.git-tags | https://github.com/strimzi/strimzi-kafka-operator/releases/tag/1.2.0#refs/tags/1.2.0 |
| versioning | discovered | tags.scheme | https://github.com/strimzi/strimzi-kafka-operator/releases/tag/1.2.0#refs/tags/1.2.0 |

Statuses: 9 historically-validated, 4 discovered, 3 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: strimzi
name: strimzi-kafka-operator
homepage: https://github.com/strimzi/strimzi-kafka-operator
versioning:
  scheme: semver
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/strimzi/strimzi-kafka-operator
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: strimzi/strimzi-kafka-operator
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: strimzi/strimzi-kafka-operator
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/strimzi/strimzi-kafka-operator
      path: CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: strimzi/strimzi-kafka-operator
artifacts:
  - id: strimzi-kafka-operator-chart
    type: helm-chart
    name: strimzi-kafka-operator
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: quay.io/strimzi-helm/strimzi-kafka-operator
      - kind: helm-repo
        url: https://strimzi.io/charts
        chart: strimzi-kafka-operator
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/strimzi/strimzi-kafka-operator
          path: helm-charts/helm3/strimzi-kafka-operator/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/strimzi/strimzi-kafka-operator
          path: helm-charts/helm3/strimzi-kafka-operator/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 1.0.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.0.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.1.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.2.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable)).'
  - id: strimzi-kafka-operator-chart-2
    type: helm-chart
    name: strimzi-kafka-operator
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: quay.io/strimzi-helm/strimzi-kafka-operator
      - kind: helm-repo
        url: https://strimzi.io/charts
        chart: strimzi-kafka-operator
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/strimzi/strimzi-kafka-operator
          path: packaging/helm-charts/helm3/strimzi-kafka-operator/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/strimzi/strimzi-kafka-operator
          path: packaging/helm-charts/helm3/strimzi-kafka-operator/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 1.0.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.0.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.1.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.2.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable)).'
  - id: install-cluster-operator-060-deployment-strimzi-cluster-oper
    type: manifest
    name: 060-Deployment-strimzi-cluster-operator.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/strimzi/strimzi-kafka-operator
        path: install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: install-topic-operator-05-deployment-strimzi-topic-operator
    type: manifest
    name: 05-Deployment-strimzi-topic-operator.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/strimzi/strimzi-kafka-operator
        path: install/topic-operator/05-Deployment-strimzi-topic-operator.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: install-user-operator-05-deployment-strimzi-user-operator
    type: manifest
    name: 05-Deployment-strimzi-user-operator.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/strimzi/strimzi-kafka-operator
        path: install/user-operator/05-Deployment-strimzi-user-operator.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: packaging-install-cluster-operator-060-deployment-strimzi-cl
    type: manifest
    name: 060-Deployment-strimzi-cluster-operator.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/strimzi/strimzi-kafka-operator
        path: packaging/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: strimzi-binary
    type: binary
    name: strimzi
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: http
        url: https://github.com/strimzi/strimzi-kafka-operator/releases/download/{{.Tag}}/strimzi-{{.Version}}.zip
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: strimzi-crds
    type: crd
    name: strimzi-crds-{{.Version}}.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: http
        url: https://github.com/strimzi/strimzi-kafka-operator/releases/download/{{.Tag}}/strimzi-crds-{{.Version}}.yaml
    contents:
      - kind: crds
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
  - id: operator
    type: container-image
    name: operator
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/strimzi/operator
    references:
      - artifact: install-cluster-operator-060-deployment-strimzi-cluster-oper
        pattern: quay.io/strimzi/operator:{{.Tag}}
      - artifact: install-topic-operator-05-deployment-strimzi-topic-operator
        pattern: quay.io/strimzi/operator:{{.Tag}}
      - artifact: install-user-operator-05-deployment-strimzi-user-operator
        pattern: quay.io/strimzi/operator:{{.Tag}}
      - artifact: packaging-install-cluster-operator-060-deployment-strimzi-cl
        pattern: quay.io/strimzi/operator:{{.Tag}}
    validatedAgainst:
      - 1.0.0
      - 1.0.1
      - 1.1.0
      - 1.2.0
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - 1.0.0
    - 1.0.1
    - 1.1.0
    - 1.2.0
  notes: Proposed by automated discovery; relationship checks run against 1.0.0, 1.0.1, 1.1.0, 1.2.0.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 4 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.
- Ambiguity (main-chart): 2 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) strimzi-kafka-operator, strimzi-kafka-operator relate to the release version?

## Validation matrix

| Element | Verdict | 1.0.0 | 1.0.1 | 1.1.0 | 1.2.0 |
|---|---|---|---|---|---|
| artifact:api-conversion-binary | failing | fail: absent from https://github.com/strimzi/strimzi-kafka-operato… | fail: absent from https://github.com/strimzi/strimzi-kafka-operato… | fail: absent from https://github.com/strimzi/strimzi-kafka-operato… | fail: absent from https://github.com/strimzi/strimzi-kafka-operato… |
| artifact:install-cluster-operator-060-deployment-strimzi-cluster-oper | validated | pass | pass | pass | pass |
| artifact:install-topic-operator-05-deployment-strimzi-topic-operator | validated | pass | pass | pass | pass |
| artifact:install-user-operator-05-deployment-strimzi-user-operator | validated | pass | pass | pass | pass |
| artifact:operator | validated | pass | pass | pass | pass |
| artifact:packaging-install-cluster-operator-060-deployment-strimzi-cl | validated | pass | pass | pass | pass |
| artifact:strimzi-binary | validated | pass | pass | pass | pass |
| artifact:strimzi-crds | validated | pass | pass | pass | pass |
| artifact:strimzi-kafka-operator-chart | unverifiable | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… |
| artifact:strimzi-kafka-operator-chart-2 | unverifiable | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… | unverifiable: artifact version is independent of the release version (stra… |
| content:install-cluster-operator-060-deployment-strimzi-cluster-oper/image-refs | validated | pass | pass | pass | pass |
| content:install-topic-operator-05-deployment-strimzi-topic-operator/image-refs | validated | pass | pass | pass | pass |
| content:install-user-operator-05-deployment-strimzi-user-operator/image-refs | validated | pass | pass | pass | pass |
| content:packaging-install-cluster-operator-060-deployment-strimzi-cl/image-refs | validated | pass | pass | pass | pass |
| content:strimzi-crds/crds | validated | pass | pass | pass | pass |
| content:strimzi-kafka-operator-chart-2/chart-metadata | validated | pass | pass | pass | pass |
| content:strimzi-kafka-operator-chart-2/helm-values | validated | pass | pass | pass | pass |
| content:strimzi-kafka-operator-chart/chart-metadata | validated | pass | pass | pass | pass |
| content:strimzi-kafka-operator-chart/helm-values | validated | pass | pass | pass | pass |
| source:changelog | validated | pass | pass | pass | pass |
| source:release-notes-github | validated | pass | pass | pass | pass |

## Dropped elements

- `artifact:api-conversion-binary` (deterministic): failed validation: 1.0.0 fail (absent from https://github.com/strimzi/strimzi-kafka-operator/releases/download/1.0.0/api-conversion-1.0.0.zip); 1.0.1 fail (absent from https://github.com/strimzi/strimzi-kafka-operator/releases/download/1.0.1/api-conversion-1.0.1.zip); 1.1.0 fail (absent from https://github.com/strimzi/strimzi-kafka-operator/releases/download/1.1.0/api-conversion-1.1.0.zip); 1.2.0 fail (absent from https://github.com/strimzi/strimzi-kafka-operator/releases/download/1.2.0/api-conversion-1.2.0.zip)

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 182 tags: prefix "", 81 stable, 100 prereleases (rcN×100), 1 junk; latest stable 1.2.0; lineage linear |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (182 tags: prefix "", 81 stable, 100 prereleases (rcN×100), 1 junk; latest stable 1.2.0; lineage linear). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 1.2.0 (heading "## 1.2.0"). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:strimzi-kafka-operator-chart | include | helm.independent-version | heuristic | Chart has its own version "0.1.1" unrelated to the release. Published via 2 channel(s); OCI locations first. |
| artifact:strimzi-kafka-operator-chart-2 | include | helm.independent-version | heuristic | Chart has its own version "0.1.1" unrelated to the release. Published via 2 channel(s); OCI locations first. |
| artifact:install-cluster-operator-060-deployment-strimzi-cluster-oper | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:install-topic-operator-05-deployment-strimzi-topic-operator | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:install-user-operator-05-deployment-strimzi-user-operator | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:packaging-install-cluster-operator-060-deployment-strimzi-cl | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:api-conversion-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (2 reference(s)). |
| artifact:strimzi-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:strimzi-crds | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:operator | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:strimzi-kafka-operator-chart | annotate | validate.unverifiable | computed | Kept unverified: 1.0.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.0.1 unverifiable (artifact version is independent of the release version (strategy in… |
| artifact:strimzi-kafka-operator-chart-2 | annotate | validate.unverifiable | computed | Kept unverified: 1.0.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.0.1 unverifiable (artifact version is independent of the release version (strategy in… |
| artifact:api-conversion-binary | drop | validate.failed | computed | Relationship check failed: 1.0.0 fail (absent from https://github.com/strimzi/strimzi-kafka-operator/releases/download/1.0.0/api-conversion-1.0.0.zip); 1.0.1 fail (absent from https://github.com/strimzi/strimzi-kafka-ope… |

<details><summary>86 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| manifest `install/access-operator/050-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `install/drain-cleaner/certmanager/060-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `install/drain-cleaner/kubernetes/060-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `install/drain-cleaner/openshift/060-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `packaging/install/access-operator/050-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `packaging/install/drain-cleaner/certmanager/060-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `packaging/install/drain-cleaner/kubernetes/060-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `packaging/install/drain-cleaner/openshift/060-Deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `packaging/install/topic-operator/05-Deployment-strimzi-topic-operator.yaml` | manifest.limit | More install manifest variants exist; only the four best ranked are modelled. |
| manifest `packaging/install/user-operator/05-Deployment-strimzi-user-operator.yaml` | manifest.limit | More install manifest variants exist; only the four best ranked are modelled. |
| crd `crd-generator/src/test/resources/io/strimzi/crdgenerator/simpleTest.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `crd-generator/src/test/resources/io/strimzi/crdgenerator/simpleTestHelmMetadata.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `crd-generator/src/test/resources/io/strimzi/crdgenerator/simpleTestWithSubresources.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `crd-generator/src/test/resources/io/strimzi/crdgenerator/simpleTestWithoutDescriptions.yam…` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `crd-generator/src/test/resources/io/strimzi/crdgenerator/versionedTest.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/042-Crd-strimzipodset.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/043-Crd-kafkatopic.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/044-Crd-kafkauser.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/045-Crd-kafkanodepool.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/046-Crd-kafkabridge.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/047-Crd-kafkaconnector.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/048-Crd-kafkamirrormaker2.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `helm-charts/helm3/strimzi-kafka-operator/crds/049-Crd-kafkarebalance.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/access-operator/040-Crd-kafkaaccess.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/040-Crd-kafka.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/041-Crd-kafkaconnect.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/042-Crd-strimzipodset.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/043-Crd-kafkatopic.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/044-Crd-kafkauser.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/045-Crd-kafkanodepool.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/046-Crd-kafkabridge.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/047-Crd-kafkaconnector.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/048-Crd-kafkamirrormaker2.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/cluster-operator/049-Crd-kafkarebalance.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/topic-operator/04-Crd-kafkatopic.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `install/user-operator/04-Crd-kafkauser.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/042-Crd-strimzipodset.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/043-Crd-kafkatopic.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/044-Crd-kafkauser.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/045-Crd-kafkanodepool.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/046-Crd-kafkabridge.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/047-Crd-kafkaconnector.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/048-Crd-kafkamirrormaker2.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/049-Crd-kafkarebalance.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/access-operator/040-Crd-kafkaaccess.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/040-Crd-kafka.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/041-Crd-kafkaconnect.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/042-Crd-strimzipodset.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/043-Crd-kafkatopic.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/044-Crd-kafkauser.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/045-Crd-kafkanodepool.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/046-Crd-kafkabridge.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/047-Crd-kafkaconnector.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/048-Crd-kafkamirrormaker2.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/cluster-operator/049-Crd-kafkarebalance.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/topic-operator/04-Crd-kafkatopic.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `packaging/install/user-operator/04-Crd-kafkauser.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `systemtest/src/test/resources/tracing/cert-manager.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| crd `systemtest/src/test/resources/tracing/open-telemetry-operator.yaml` | crd.prefer-release-asset | A CRD bundle is attached to hosted releases (strimzi-crds-{{.Version}}.yaml); in-repo CRD sources are not modelled. |
| image `registry.k8s.io/cloud-provider-kind/cloud-controller-manager` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/strimzi/kafka` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `quay.io/strimzi/jmxtrans` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `ghcr.io/strimzi/binaries-operators` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: unresolved). |
| image `ghcr.io/catthehacker/ubuntu` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/kafka` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/strimzi/kafka-bridge` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/strimzi/custom-connect-build` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/containers/buildah` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/kaniko-project/executor` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ttl.sh/strimzi-connect-example-4.3.0` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/grafana/grafana` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/kube-state-metrics/kube-state-metrics` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/strimzi/access-operator` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `quay.io/strimzi/drain-cleaner` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `registry.k8s.io/sig-storage/nfs-provisioner` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-cainjector` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-controller` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-acmesolver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-webhook` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jaegertracing/jaeger` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/open-telemetry/opentelemetry-operator/opentelemetry-operator` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/brancz/kube-rbac-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |

</details>

## Candidates


### changelog (1)

- `CHANGELOG.md` (high; docs.changelog) — [strimzi/strimzi-kafka-operator:CHANGELOG.md L3](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/CHANGELOG.md#L3): `## 1.2.0`

### crd (53)

- `helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/042-Crd-strimzipodset.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/042-Crd-strimzipodset.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/042-Crd-strimzipodset.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/043-Crd-kafkatopic.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/043-Crd-kafkatopic.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/043-Crd-kafkatopic.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/044-Crd-kafkauser.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/044-Crd-kafkauser.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/044-Crd-kafkauser.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/045-Crd-kafkanodepool.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/045-Crd-kafkanodepool.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/045-Crd-kafkanodepool.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/046-Crd-kafkabridge.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/046-Crd-kafkabridge.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/046-Crd-kafkabridge.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/047-Crd-kafkaconnector.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/047-Crd-kafkaconnector.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/047-Crd-kafkaconnector.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/048-Crd-kafkamirrormaker2.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/048-Crd-kafkamirrormaker2.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/048-Crd-kafkamirrormaker2.yaml#L2): `kind: CustomResourceDefinition`
- `helm-charts/helm3/strimzi-kafka-operator/crds/049-Crd-kafkarebalance.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/crds/049-Crd-kafkarebalance.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/crds/049-Crd-kafkarebalance.yaml#L2): `kind: CustomResourceDefinition`
- `install/access-operator/040-Crd-kafkaaccess.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/access-operator/040-Crd-kafkaaccess.yaml L3](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/access-operator/040-Crd-kafkaaccess.yaml#L3): `kind: CustomResourceDefinition`
- `install/cluster-operator/040-Crd-kafka.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/040-Crd-kafka.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/040-Crd-kafka.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/041-Crd-kafkaconnect.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/041-Crd-kafkaconnect.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/041-Crd-kafkaconnect.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/042-Crd-strimzipodset.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/042-Crd-strimzipodset.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/042-Crd-strimzipodset.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/043-Crd-kafkatopic.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/043-Crd-kafkatopic.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/043-Crd-kafkatopic.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/044-Crd-kafkauser.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/044-Crd-kafkauser.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/044-Crd-kafkauser.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/045-Crd-kafkanodepool.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/045-Crd-kafkanodepool.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/045-Crd-kafkanodepool.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/046-Crd-kafkabridge.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/046-Crd-kafkabridge.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/046-Crd-kafkabridge.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/047-Crd-kafkaconnector.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/047-Crd-kafkaconnector.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/047-Crd-kafkaconnector.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/048-Crd-kafkamirrormaker2.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/048-Crd-kafkamirrormaker2.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/048-Crd-kafkamirrormaker2.yaml#L2): `kind: CustomResourceDefinition`
- `install/cluster-operator/049-Crd-kafkarebalance.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/cluster-operator/049-Crd-kafkarebalance.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/049-Crd-kafkarebalance.yaml#L2): `kind: CustomResourceDefinition`
- `install/topic-operator/04-Crd-kafkatopic.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/topic-operator/04-Crd-kafkatopic.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/topic-operator/04-Crd-kafkatopic.yaml#L2): `kind: CustomResourceDefinition`
- `install/user-operator/04-Crd-kafkauser.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:install/user-operator/04-Crd-kafkauser.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/user-operator/04-Crd-kafkauser.yaml#L2): `kind: CustomResourceDefinition`
- `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:packaging/helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/helm-charts/helm3/strimzi-kafka-operator/crds/040-Crd-kafka.yaml#L2): `kind: CustomResourceDefinition`
- `packaging/helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml` (high; crd.file) — [strimzi/strimzi-kafka-operator:packaging/helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/helm-charts/helm3/strimzi-kafka-operator/crds/041-Crd-kafkaconnect.yaml#L2): `kind: CustomResourceDefinition`
- … 28 more

### docs-repo (1)

- `github.com/strimzi/strimzi.github.io` (medium; docs.docs-repo-ref) — [strimzi/strimzi-kafka-operator:documentation/contributing/introduction.adoc L137](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/documentation/contributing/introduction.adoc#L137): `For information on contributing to the quick starts, see the https://github.com/strimzi/strimzi.github.io/blob…`

### helm-chart (2)

- `strimzi-kafka-operator@helm-charts/helm3/strimzi-kafka-operator` (high; helm.chart) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/Chart.yaml L4](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/Chart.yaml#L4): `name: strimzi-kafka-operator`
- `strimzi-kafka-operator@packaging/helm-charts/helm3/strimzi-kafka-operator` (high; helm.chart) — [strimzi/strimzi-kafka-operator:packaging/helm-charts/helm3/strimzi-kafka-operator/Chart.yaml L4](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/helm-charts/helm3/strimzi-kafka-operator/Chart.yaml#L4): `name: strimzi-kafka-operator`

### helm-oci (3)

- `docker.io/envoyproxy/gateway-helm` (medium; docs.oci-ref) — [strimzi/strimzi.github.io:_posts/2026-07-09-gateway-api-tlsroute-support-in-strimzi.md L90](https://github.com/strimzi/strimzi.github.io/blob/b1b73f86942532c4a081a63d6d3237d441e79c80/_posts/2026-07-09-gateway-api-tlsroute-support-in-strimzi.md#L90): `helm install envoygateway oci://docker.io/envoyproxy/gateway-helm \`
- `quay.io/strimzi-helm/strimzi-drain-cleaner` (medium; docs.oci-ref) — [strimzi/strimzi-kafka-operator:documentation/modules/drain-cleaner/proc-drain-cleaner-deploying-helm-chart.adoc L93](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/documentation/modules/drain-cleaner/proc-drain-cleaner-deploying-helm-chart.adoc#L93): `helm install strimzi-drain-cleaner oci://quay.io/strimzi-helm/strimzi-drain-cleaner`
- `quay.io/strimzi-helm/strimzi-kafka-operator` (medium; docs.oci-ref) — [strimzi/strimzi-kafka-operator:documentation/modules/deploying/proc-deploy-cluster-operator-helm-chart.adoc L31](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/documentation/modules/deploying/proc-deploy-cluster-operator-helm-chart.adoc#L31): `helm install strimzi-cluster-operator oci://quay.io/strimzi-helm/strimzi-kafka-operator`

### helm-repo (1)

- `https://strimzi.io/charts` (medium; docs.helm-repo-add) — [strimzi/strimzi.github.io:_posts/2018-11-01-using-helm.md L57](https://github.com/strimzi/strimzi.github.io/blob/b1b73f86942532c4a081a63d6d3237d441e79c80/_posts/2018-11-01-using-helm.md#L57): `helm repo add strimzi https://strimzi.io/charts/`

### image (24)

- `quay.io/strimzi/operator` (high; docs.image-ref, manifest.image, script.image-ref) — [strimzi/strimzi-kafka-operator:.azure/scripts/setup_upgrade.sh L18](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.azure/scripts/setup_upgrade.sh#L18): `sed -i "s#quay.io/strimzi/operator#${DOCKER_REGISTRY}/${DOCKER_ORG}/operator#g" packaging/install/cluster-oper…`
- `gcr.io/kaniko-project/executor` (medium; make.image-ref) — [strimzi/strimzi-kafka-operator:docker-images/kaniko-executor/Makefile L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/docker-images/kaniko-executor/Makefile#L2): `KANIKO_EXECUTOR = gcr.io/kaniko-project/executor:v1.24.0`
- `ghcr.io/strimzi/binaries-operators` (medium; workflow.image-ref) — [strimzi/strimzi-kafka-operator:.github/workflows/cve-rebuild.yml L42](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.github/workflows/cve-rebuild.yml#L42): `oras pull ghcr.io/${{ env.REPOSITORY }}/binaries-operators:${{ env.TAG }}`
- `quay.io/containers/buildah` (medium; make.image-ref) — [strimzi/strimzi-kafka-operator:docker-images/buildah/Makefile L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/docker-images/buildah/Makefile#L2): `BUILDAH = quay.io/containers/buildah:v1.41.4`
- `quay.io/strimzi/access-operator` (medium; manifest.image) — [strimzi/strimzi-kafka-operator:install/access-operator/050-Deployment.yaml L30](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/access-operator/050-Deployment.yaml#L30): `image: quay.io/strimzi/access-operator:0.3.0`
- `quay.io/strimzi/drain-cleaner` (medium; manifest.image) — [strimzi/strimzi-kafka-operator:install/drain-cleaner/certmanager/060-Deployment.yaml L21](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/drain-cleaner/certmanager/060-Deployment.yaml#L21): `image: quay.io/strimzi/drain-cleaner:1.6.1`
- `quay.io/strimzi/jmxtrans` (medium; script.image-ref) — [strimzi/strimzi-kafka-operator:.azure/scripts/setup_upgrade.sh L19](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.azure/scripts/setup_upgrade.sh#L19): `sed -i "s#quay.io/strimzi/jmxtrans#${DOCKER_REGISTRY}/${DOCKER_ORG}/jmxtrans#g" packaging/install/cluster-oper…`
- `quay.io/strimzi/kafka` (medium; docs.image-ref, script.image-ref) — [strimzi/strimzi-kafka-operator:.azure/scripts/setup_upgrade.sh L17](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.azure/scripts/setup_upgrade.sh#L17): `sed -i "s#quay.io/strimzi/kafka:#${DOCKER_REGISTRY}/${DOCKER_ORG}/kafka:#g" packaging/install/cluster-operator…`
- `quay.io/strimzi/kafka-bridge` (medium; docs.image-ref) — [strimzi/strimzi-kafka-operator:development-docs/TESTING.md L326](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/development-docs/TESTING.md#L326): `| BRIDGE_IMAGE                          | Specify the Kafka bridge image used in system tests                 …`
- `registry.k8s.io/cloud-provider-kind/cloud-controller-manager` (medium; script.image-ref) — [strimzi/strimzi-kafka-operator:.azure/scripts/setup-kind.sh L320](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.azure/scripts/setup-kind.sh#L320): `registry.k8s.io/cloud-provider-kind/cloud-controller-manager:"${KIND_CLOUD_PROVIDER_VERSION}"`
- `docker.io/grafana/grafana` (low; manifest.image) — [strimzi/strimzi-kafka-operator:examples/metrics/grafana-install/grafana.yaml L22](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/examples/metrics/grafana-install/grafana.yaml#L22): `image: grafana/grafana:7.4.5`
- `docker.io/library/kafka` (low; manifest.image) — [strimzi/strimzi-kafka-operator:api/src/test/resources/io/strimzi/api/kafka/model/podset/StrimziPodSet-regular.out.yaml L21](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/api/src/test/resources/io/strimzi/api/kafka/model/podset/StrimziPodSet-regular.out.yaml#L21): `image: "kafka:latest"`
- `ghcr.io/catthehacker/ubuntu` (low; workflow.image-ref) — [strimzi/strimzi-kafka-operator:.github/workflows/github-actions-integration.yml L105](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.github/workflows/github-actions-integration.yml#L105): `if eval "act '$event' -W '$WORKFLOW' -e '$fixture' -P ubuntu-latest=ghcr.io/catthehacker/ubuntu:act-22.04 --pu…`
- `ghcr.io/open-telemetry/opentelemetry-operator/opentelemetry-operator` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/tracing/open-telemetry-operator.yaml L16762](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/tracing/open-telemetry-operator.yaml#L16762): `image: ghcr.io/open-telemetry/opentelemetry-operator/opentelemetry-operator:0.126.0`
- `quay.io/brancz/kube-rbac-proxy` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/tracing/open-telemetry-operator.yaml L16793](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/tracing/open-telemetry-operator.yaml#L16793): `image: quay.io/brancz/kube-rbac-proxy:v0.13.1`
- `quay.io/jaegertracing/jaeger` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/tracing/jaeger-instance.yaml L6](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/tracing/jaeger-instance.yaml#L6): `image: quay.io/jaegertracing/jaeger:2.6.0`
- `quay.io/jetstack/cert-manager-acmesolver` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/tracing/cert-manager.yaml L13053](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/tracing/cert-manager.yaml#L13053): `- --acme-http01-solver-image=quay.io/jetstack/cert-manager-acmesolver:v1.17.2`
- `quay.io/jetstack/cert-manager-cainjector` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/tracing/cert-manager.yaml L12984](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/tracing/cert-manager.yaml#L12984): `image: "quay.io/jetstack/cert-manager-cainjector:v1.17.2"`
- `quay.io/jetstack/cert-manager-controller` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/tracing/cert-manager.yaml L13047](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/tracing/cert-manager.yaml#L13047): `image: "quay.io/jetstack/cert-manager-controller:v1.17.2"`
- `quay.io/jetstack/cert-manager-webhook` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/tracing/cert-manager.yaml L13129](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/tracing/cert-manager.yaml#L13129): `image: "quay.io/jetstack/cert-manager-webhook:v1.17.2"`
- `quay.io/strimzi/custom-connect-build` (low; docs.image-ref) — [strimzi/strimzi-kafka-operator:development-docs/TESTING.md L350](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/development-docs/TESTING.md#L350): `| CONNECT_BUILD_IMAGE_PATH              | Specify the registry together with org used by KafkaConnect build. F…`
- `registry.k8s.io/kube-state-metrics/kube-state-metrics` (low; manifest.image) — [strimzi/strimzi-kafka-operator:examples/metrics/kube-state-metrics/ksm.yaml L110](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/examples/metrics/kube-state-metrics/ksm.yaml#L110): `image: registry.k8s.io/kube-state-metrics/kube-state-metrics:v2.16.0`
- `registry.k8s.io/sig-storage/nfs-provisioner` (low; manifest.image) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/nfs/nfs.yaml L159](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/nfs/nfs.yaml#L159): `image: "registry.k8s.io/sig-storage/nfs-provisioner:v4.0.8"`
- `ttl.sh/strimzi-connect-example-4.3.0` (low; manifest.image) — [strimzi/strimzi-kafka-operator:examples/connect/kafka-connect-build.yaml L36](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/examples/connect/kafka-connect-build.yaml#L36): `image: ttl.sh/strimzi-connect-example-4.3.0:24h`

### manifest (15)

- `install/access-operator/050-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:install/access-operator/050-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/access-operator/050-Deployment.yaml#L2): `kind: Deployment`
- `install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml#L2): `kind: Deployment`
- `install/drain-cleaner/certmanager/060-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:install/drain-cleaner/certmanager/060-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/drain-cleaner/certmanager/060-Deployment.yaml#L2): `kind: Deployment`
- `install/drain-cleaner/kubernetes/060-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:install/drain-cleaner/kubernetes/060-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/drain-cleaner/kubernetes/060-Deployment.yaml#L2): `kind: Deployment`
- `install/drain-cleaner/openshift/060-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:install/drain-cleaner/openshift/060-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/drain-cleaner/openshift/060-Deployment.yaml#L2): `kind: Deployment`
- `install/topic-operator/05-Deployment-strimzi-topic-operator.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:install/topic-operator/05-Deployment-strimzi-topic-operator.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/topic-operator/05-Deployment-strimzi-topic-operator.yaml#L2): `kind: Deployment`
- `install/user-operator/05-Deployment-strimzi-user-operator.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:install/user-operator/05-Deployment-strimzi-user-operator.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/install/user-operator/05-Deployment-strimzi-user-operator.yaml#L2): `kind: Deployment`
- `packaging/install/access-operator/050-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:packaging/install/access-operator/050-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/access-operator/050-Deployment.yaml#L2): `kind: Deployment`
- `packaging/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:packaging/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/cluster-operator/060-Deployment-strimzi-cluster-operator.yaml#L2): `kind: Deployment`
- `packaging/install/drain-cleaner/certmanager/060-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:packaging/install/drain-cleaner/certmanager/060-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/drain-cleaner/certmanager/060-Deployment.yaml#L2): `kind: Deployment`
- `packaging/install/drain-cleaner/kubernetes/060-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:packaging/install/drain-cleaner/kubernetes/060-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/drain-cleaner/kubernetes/060-Deployment.yaml#L2): `kind: Deployment`
- `packaging/install/drain-cleaner/openshift/060-Deployment.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:packaging/install/drain-cleaner/openshift/060-Deployment.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/drain-cleaner/openshift/060-Deployment.yaml#L2): `kind: Deployment`
- `packaging/install/topic-operator/05-Deployment-strimzi-topic-operator.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:packaging/install/topic-operator/05-Deployment-strimzi-topic-operator.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/topic-operator/05-Deployment-strimzi-topic-operator.yaml#L2): `kind: Deployment`
- `packaging/install/user-operator/05-Deployment-strimzi-user-operator.yaml` (high; manifest.workloads) — [strimzi/strimzi-kafka-operator:packaging/install/user-operator/05-Deployment-strimzi-user-operator.yaml L2](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/packaging/install/user-operator/05-Deployment-strimzi-user-operator.yaml#L2): `kind: Deployment`
- `kafka-versions.yaml` (low; docs.raw-url) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/upgrade/README.md L37](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/upgrade/README.md#L37): `- 'fromKafkaVersionsUrl: https://raw.githubusercontent.com/strimzi/strimzi-kafka-operator/0.47.0/kafka-version…`

### registry (6)

- `docker.io` (medium; make.registry-variable) — [strimzi/strimzi-kafka-operator:Makefile.docker L14](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/Makefile.docker#L14): `DOCKER_REGISTRY    ?= docker.io`
- `ghcr.io` (medium; workflow.registry-login) — [strimzi/strimzi-kafka-operator:.github/workflows/cve-rebuild.yml L38](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.github/workflows/cve-rebuild.yml#L38): `run: echo "${{ github.token }}" | oras login ghcr.io -u ${{ github.actor }} --password-stdin`
- `gcr.io/google_containers/kube-registry-proxy` (low; script.registry-ref) — [strimzi/strimzi-kafka-operator:.azure/scripts/setup-kubernetes.sh L95](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/.azure/scripts/setup-kubernetes.sh#L95): `docker build --pull -t gcr.io/google_containers/kube-registry-proxy:0.4-${ARCH} kubernetes/cluster/addons/regi…`
- `quay.io/organization/strimzi` (low; docs.registry-ref) — [strimzi/strimzi-kafka-operator:documentation/shared/attributes.adoc L172](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/documentation/shared/attributes.adoc#L172): `:DockerRepository: https://quay.io/organization/strimzi[Container Registry^]`
- `quay.io/organization/strimzi-helm` (low; docs.registry-ref) — [strimzi/strimzi-kafka-operator:CHANGELOG.md L638](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/CHANGELOG.md#L638): `Please use the Helm Chart OCI artifacts from our [Helm Chart OCI repository instead](https://quay.io/organizat…`
- `quay.io/strimzi` (low; docs.registry-ref) — [strimzi/strimzi-kafka-operator:development-docs/DEV_GUIDE.md L261](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/development-docs/DEV_GUIDE.md#L261): `sed -Ei -e "s#(image|value): quay.io/strimzi/([a-z0-9-]+):latest#\1: $DOCKER_REGISTRY/$DOCKER_ORG/\2:latest#" …`

### release-asset (5)

- `https://github.com/strimzi/strimzi-kafka-operator/releases/download/{{.Tag}}/api-conversion-{{.Version}}.tar.gz` (medium; docs.release-asset-url) — [strimzi/strimzi.github.io:_posts/2021-04-29-api-conversion.md L43](https://github.com/strimzi/strimzi.github.io/blob/b1b73f86942532c4a081a63d6d3237d441e79c80/_posts/2021-04-29-api-conversion.md#L43): `The 'api conversion' tool is shipped with Strimzi 0.22 as a [zip file](https://github.com/strimzi/strimzi-kafk…`
- `https://github.com/strimzi/strimzi-kafka-operator/releases/download/{{.Tag}}/api-conversion-{{.Version}}.zip` (medium; docs.release-asset-url) — [strimzi/strimzi.github.io:_posts/2021-04-29-api-conversion.md L43](https://github.com/strimzi/strimzi.github.io/blob/b1b73f86942532c4a081a63d6d3237d441e79c80/_posts/2021-04-29-api-conversion.md#L43): `The 'api conversion' tool is shipped with Strimzi 0.22 as a [zip file](https://github.com/strimzi/strimzi-kafk…`
- `https://github.com/strimzi/strimzi-kafka-operator/releases/download/{{.Tag}}/strimzi-crds-{{.Version}}.yaml` (medium; docs.release-asset-url) — [strimzi/strimzi-kafka-operator:helm-charts/helm3/strimzi-kafka-operator/README.md L54](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/helm-charts/helm3/strimzi-kafka-operator/README.md#L54): `kubectl apply -f https://github.com/strimzi/strimzi-kafka-operator/releases/download/0.50.0/strimzi-crds-0.50.…`
- `https://github.com/strimzi/strimzi-kafka-operator/releases/download/0.45.0/` (low; docs.release-asset-url) — [strimzi/strimzi-kafka-operator:development-docs/RELEASE.md L63](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/development-docs/RELEASE.md#L63): `helm repo index ./strimzi-0.45.0/ --merge ./strimzi.github.io/charts/index.yaml --url https://github.com/strim…`
- `https://github.com/strimzi/strimzi-kafka-operator/releases/download/{{.Tag}}/strimzi-{{.Version}}.zip` (low; docs.release-asset-url) — [strimzi/strimzi-kafka-operator:systemtest/src/test/resources/upgrade/README.md L26](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/systemtest/src/test/resources/upgrade/README.md#L26): `- 'toUrl: https://github.com/strimzi/strimzi-kafka-operator/releases/download/0.47.0/strimzi-0.47.0.zip'`

### security-policy (2)

- `development-docs/systemtests/labels/security.md` (high; docs.security-policy) — [strimzi/strimzi-kafka-operator:development-docs/systemtests/labels/security.md L1](https://github.com/strimzi/strimzi-kafka-operator/blob/1.2.0/development-docs/systemtests/labels/security.md#L1): `# Security`
- `security.md` (high; docs.security-policy) — [strimzi/strimzi.github.io:security.md L1](https://github.com/strimzi/strimzi.github.io/blob/b1b73f86942532c4a081a63d6d3237d441e79c80/security.md#L1): `---`

### tag-scheme (1)

- `^(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (high; tags.ls-remote) — [strimzi/strimzi-kafka-operator/releases/tag/1.2.0 refs/tags/1.2.0](https://github.com/strimzi/strimzi-kafka-operator/releases/tag/1.2.0#refs/tags/1.2.0): `6c7b43c4af0db547c10463ba09d1dfa6f5e156a0 refs/tags/1.2.0`
