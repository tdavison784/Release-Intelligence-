# Discovery report: loki

- Repository: `github.com/grafana/loki` at `helm-loki-7.3.0` (commit `50c738b4d5b2…`)
- Generated: 2026-10-01T16:31:00Z
- LLM: not used (deterministic resolver only)
- Tags: 396 tags: prefix "helm-loki-", 247 stable, 0 prereleases (), 149 junk; latest stable helm-loki-7.3.0; lineage linear
- Strict tag pattern: `^helm-loki-(?P<version>\d+\.\d+\.\d+)$` (junk sample: 2.8.3, helm-loki-5.41.9-distributed, helm-loki-5.41.9-distributed-rc2, helm-loki-6.19.0-weekly.227, helm-loki-6.20.0-weekly.229, helm-loki-6.22.0-weekly.230)
- Scanned `github.com/grafana/loki@helm-loki-7.3.0` (source profile): 4084 files listed, 801 read
- Validation: done against helm-loki-7.0.0, helm-loki-7.1.0, helm-loki-7.2.0, helm-loki-7.3.0
- Error: default branch of github.com/grafana/docs: git ls-remote --symref https://github.com/grafana/docs HEAD: exit status 128: fatal: could not read Username for 'https://github.com': terminal prompts disabled
- Error: list tags of github.com/grafana/loki-helm-test: git ls-remote --tags https://github.com/grafana/loki-helm-test: exit status 128: fatal: could not read Username for 'https://github.com': terminal prompts disabled

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 2 | git-tags github.com/grafana/loki; github-releases grafana/loki |
| release-notes | not-found | 0 |  |
| changelog | found | 3 | repo-file github.com/grafana/loki production/helm/loki/CHANGELOG.md |
| helm-charts | found | 9 | loki via oci:ghcr.io/grafana-community/helm-charts/loki, helm-repo:https://grafana.github.io/helm-charts, helm-repo:https://grafana-community.github.io/helm-charts; loki.md via helm-repo:https://grafana.github.io/k8s-mon… |
| registries | candidates-only | 7 |  |
| images | found | 84 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories grafana/loki |
| version-relations | found | 1 | loki-chart: version = {{.Version}}; loki-md-chart: lookup appVersion == {{.Tag}}; operator-hack-addon-grafana-gateway-ocp: version = {{.Tag}}; operator-hack-addon-grafana-gateway-ocp-oauth: version = {{.Tag}}; crds: vers… |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:crds | historically-validated | crd.in-repo | https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/bases/config.grafana.com_projectconfigs.yaml#L3 (+2) |
| artifact:loki-chart | historically-validated | helm.version-matches-release | https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/Chart.yaml#L2 (+2) |
| artifact:operator-hack-addon-grafana-gateway-ocp | historically-validated | manifest.in-repo-install | https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/docs/user-guides/howto_connect_grafana.md#L57 |
| artifact:operator-hack-addon-grafana-gateway-ocp-oauth | historically-validated | manifest.in-repo-install | https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/docs/user-guides/howto_connect_grafana.md#L45 |
| source:changelog | historically-validated | changelog.file | https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/CHANGELOG.md#L16 |
| artifact:loki-binary | unverified | asset.hosted-release-download | https://github.com/grafana/loki/blob/helm-loki-7.3.0/CHANGELOG.md#L3254 |
| artifact:loki-canary | unverified | image.tag-assumed-release | https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/values.yaml#L837 |
| artifact:loki-chart-2 | unverified | chart.external-lookup-appversion | https://github.com/grafana/loki/blob/helm-loki-7.3.0/CONTRIBUTING.md#L206 (+2) |
| artifact:loki-md-chart | unverified | chart.external-lookup-appversion | https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/operations/meta-monitoring/deploy.md#L87 |
| artifact:loki-operator | unverified | image.tag-assumed-release | https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community-openshift/manifests/loki-operator.clusterserviceversion.yaml#L2006 (+1) |
| source:advisories | discovered | security.github-hosted-default |  |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/grafana/loki/blob/helm-loki-7.3.0/CHANGELOG.md#L3254 |
| source:release-notes-github | unverified | notes.hosted-release-body | https://github.com/grafana/loki/blob/helm-loki-7.3.0/CHANGELOG.md#L3254 |
| source:tags | discovered | versions.git-tags | https://github.com/grafana/loki/releases/tag/helm-loki-7.3.0#refs/tags/helm-loki-7.3.0 |
| versioning | discovered | tags.scheme | https://github.com/grafana/loki/releases/tag/helm-loki-7.3.0#refs/tags/helm-loki-7.3.0 |

Statuses: 5 historically-validated, 4 discovered, 6 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: loki
name: loki
homepage: https://github.com/grafana/loki
versioning:
  scheme: semver
  tagPrefix: helm-loki-
  tagPattern: ^helm-loki-(?P<version>\d+\.\d+\.\d+)$
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/grafana/loki
      tagPattern: ^helm-loki-(?P<version>\d+\.\d+\.\d+)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: grafana/loki
    priority: 1
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/grafana/loki
      path: production/helm/loki/CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    validatedAgainst:
      - 7.0.0
      - 7.1.0
      - 7.2.0
      - 7.3.0
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: grafana/loki
artifacts:
  - id: loki-chart
    type: helm-chart
    name: loki
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/grafana-community/helm-charts/loki
      - kind: helm-repo
        url: https://grafana.github.io/helm-charts
        chart: loki
      - kind: helm-repo
        url: https://grafana-community.github.io/helm-charts
        chart: loki
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/grafana/loki
          path: production/helm/loki/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/grafana/loki
          path: production/helm/loki/Chart.yaml
    validatedAgainst:
      - 7.0.0
      - 7.1.0
      - 7.2.0
      - 7.3.0
  - id: loki-md-chart
    type: helm-chart
    name: loki.md
    version:
      strategy: lookup
      field: appVersion
      match: '{{.Tag}}'
      select: latest
    channels:
      - kind: helm-repo
        url: https://grafana.github.io/k8s-monitoring-helm
        chart: loki.md
      - kind: helm-git
        repository: github.com/grafana/k8s-monitoring-helm
        path: charts/k8s-monitoring/charts/feature-integrations/docs/integrations/loki.md
        tagPattern: ^loki\.md-(?P<version>\d+\.\d+\.\d+)$
    notes: 'Chart maintained in github.com/grafana/k8s-monitoring-helm with its own version; chart releases are found by appVersion. 0 of 324 chart-repository tags match ^loki\.md-(?P<version>\d+\.\d+\.\d+)$. Unverified by discovery (unverifiable: 7.0.0 unverifiable (no version index answered: helm-repo index https://grafana.github.io/k8s-monitoring-helm (chart loki.md): not-found; helm-git index github.com/grafana/k8s-monitoring-helm:charts/k8s-monitoring/charts/feature-integrations/docs/integrations/loki.md does not expose appVersion); 7.1.0 unverifiable (no version index answered: helm-repo index https://grafana.github.io/k8s-monitoring-helm (chart loki.md): not-found; helm-git index github.com/grafana/k8s-monitoring-helm:charts/k8s-monitoring/charts/feature-integrations/docs/integrations/loki.md does not expose appVersion); 7.2.0 unverifiable (no version index answered: helm-repo index https://grafana.github.io/k8s-monitoring-helm (chart loki.md): not-found; helm-git index github.com/grafana/k8s-monitoring-helm:charts/k8s-monitoring/charts/feature-integrations/docs/integrations/loki.md does not expose appVersion); 7.3.0 unverifiable (no version index answered: helm-repo index https://grafana.github.io/k8s-monitoring-helm (chart loki.md): not-found; helm-git index github.com/grafana/k8s-monitoring-helm:charts/k8s-monitoring/charts/feature-integrations/docs/integrations/loki.md does not expose appVersion)).'
  - id: operator-hack-addon-grafana-gateway-ocp
    type: manifest
    name: addon_grafana_gateway_ocp.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/grafana/loki
        path: operator/hack/addon_grafana_gateway_ocp.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 7.0.0
      - 7.1.0
      - 7.2.0
      - 7.3.0
  - id: operator-hack-addon-grafana-gateway-ocp-oauth
    type: manifest
    name: addon_grafana_gateway_ocp_oauth.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/grafana/loki
        path: operator/hack/addon_grafana_gateway_ocp_oauth.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 7.0.0
      - 7.1.0
      - 7.2.0
      - 7.3.0
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/grafana/loki
        path: operator/config/crd/bases
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 7.0.0
      - 7.1.0
      - 7.2.0
      - 7.3.0
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - helm-loki-7.0.0
    - helm-loki-7.1.0
    - helm-loki-7.2.0
    - helm-loki-7.3.0
  notes: Proposed by automated discovery; relationship checks run against helm-loki-7.0.0, helm-loki-7.1.0, helm-loki-7.2.0, helm-loki-7.3.0.
```

## Open questions for the reviewer

- CHANGELOG.md stops at 3.8.0; confirm it is abandoned (release notes come from other sources).
- operator/CHANGELOG.md stops at 0.10.2; confirm it is abandoned (release notes come from other sources).
- Ambiguity (registry-roles): 9 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.
- Ambiguity (main-chart): 3 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) loki, loki.md relate to the release version?

## Validation matrix

| Element | Verdict | helm-loki-7.0.0 | helm-loki-7.1.0 | helm-loki-7.2.0 | helm-loki-7.3.0 |
|---|---|---|---|---|---|
| artifact:crds | validated |  |  |  |  |
| artifact:loki-binary | failing |  |  |  |  |
| artifact:loki-canary | failing |  |  |  |  |
| artifact:loki-chart | validated |  |  |  |  |
| artifact:loki-chart-2 | failing |  |  |  |  |
| artifact:loki-md-chart | unverifiable |  |  |  |  |
| artifact:loki-operator | failing |  |  |  |  |
| artifact:operator-hack-addon-grafana-gateway-ocp | validated |  |  |  |  |
| artifact:operator-hack-addon-grafana-gateway-ocp-oauth | validated |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |
| content:loki-chart/chart-metadata | validated |  |  |  |  |
| content:loki-chart/helm-values | validated |  |  |  |  |
| content:operator-hack-addon-grafana-gateway-ocp-oauth/image-refs | validated |  |  |  |  |
| content:operator-hack-addon-grafana-gateway-ocp/image-refs | validated |  |  |  |  |
| source:changelog | validated |  |  |  |  |
| source:release-notes-github | failing |  |  |  |  |

## Dropped elements

- `source:release-notes-github` (deterministic): failed validation: 7.0.0 fail (fetch https://api.github.com/repos/grafana/loki/releases/tags/helm-loki-7.0.0: not found (HTTP 404)); 7.1.0 fail (fetch https://api.github.com/repos/grafana/loki/releases/tags/helm-loki-7.1.0: not found (HTTP 404)); 7.2.0 fail (fetch https://api.github.com/repos/grafana/loki/releases/tags/helm-loki-7.2.0: not found (HTTP 404)); 7.3.0 fail (fetch https://api.github.com/repos/grafana/loki/releases/tags/helm-loki-7.3.0: not found (HTTP 404))
- `artifact:loki-chart-2` (deterministic): failed validation: 7.0.0 fail (no chart version ships appVersion helm-loki-7.0.0 (https://grafana.github.io/helm-charts (chart loki) lists 344 versions)); 7.1.0 fail (no chart version ships appVersion helm-loki-7.1.0 (https://grafana.github.io/helm-charts (chart loki) lists 344 versions)); 7.2.0 fail (no chart version ships appVersion helm-loki-7.2.0 (https://grafana.github.io/helm-charts (chart loki) lists 344 versions)); 7.3.0 fail (no chart version ships appVersion helm-loki-7.3.0 (https://grafana.github.io/helm-charts (chart loki) lists 344 versions))
- `artifact:loki-binary` (deterministic): failed validation: 7.0.0 fail (absent from https://github.com/grafana/loki/releases/download/helm-loki-7.0.0/loki-linux-amd64.zip); 7.1.0 fail (absent from https://github.com/grafana/loki/releases/download/helm-loki-7.1.0/loki-linux-amd64.zip); 7.2.0 fail (absent from https://github.com/grafana/loki/releases/download/helm-loki-7.2.0/loki-linux-amd64.zip); 7.3.0 fail (absent from https://github.com/grafana/loki/releases/download/helm-loki-7.3.0/loki-linux-amd64.zip)
- `artifact:loki-canary` (deterministic): failed validation: 7.0.0 fail (absent from docker.io/grafana/loki-canary:helm-loki-7.0.0); 7.1.0 fail (absent from docker.io/grafana/loki-canary:helm-loki-7.1.0); 7.2.0 fail (absent from docker.io/grafana/loki-canary:helm-loki-7.2.0); 7.3.0 fail (absent from docker.io/grafana/loki-canary:helm-loki-7.3.0)
- `artifact:loki-operator` (deterministic): failed validation: 7.0.0 fail (absent from docker.io/grafana/loki-operator:helm-loki-7.0.0, quay.io/openshift-logging/loki-operator:helm-loki-7.0.0); 7.1.0 fail (absent from docker.io/grafana/loki-operator:helm-loki-7.1.0, quay.io/openshift-logging/loki-operator:helm-loki-7.1.0); 7.2.0 fail (absent from docker.io/grafana/loki-operator:helm-loki-7.2.0, quay.io/openshift-logging/loki-operator:helm-loki-7.2.0); 7.3.0 fail (absent from docker.io/grafana/loki-operator:helm-loki-7.3.0, quay.io/openshift-logging/loki-operator:helm-loki-7.3.0)

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 396 tags: prefix "helm-loki-", 247 stable, 0 prereleases (), 149 junk; latest stable helm-loki-7.3.0; lineage linear; strict tagPattern excludes junk tags such as 2.8.3, helm-loki-5.41.9-distributed, helm-loki-5.41.9-dis… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (396 tags: prefix "helm-loki-", 247 stable, 0 prereleases (), 149 junk; latest stable helm-loki-7.3.0; lineage linear). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 7.3.0 (heading "## 7.3.0"). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:loki-chart | include | helm.version-matches-release | heuristic | Chart.yaml at the scanned release carries version "7.3.0", the release version; chart versions follow the release ({{.Version}}). Published via 3 channel(s); OCI locations first. |
| artifact:loki-chart-2 | include | chart.external-lookup-appversion | heuristic | The chart lives in a separate repository with independent versions; the chart release for a product release is looked up by appVersion == release tag (GitHub Pages URL by the chart-releaser convention). |
| artifact:loki-md-chart | include | chart.external-lookup-appversion | heuristic | The chart lives in a separate repository with independent versions; the chart release for a product release is looked up by appVersion == release tag (GitHub Pages URL by the chart-releaser convention). |
| artifact:operator-hack-addon-grafana-gateway-ocp | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:operator-hack-addon-grafana-gateway-ocp-oauth | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:loki-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:crds | include | crd.in-repo | heuristic | 5 CRDs of the product's API groups in operator/config/crd/bases at the release tag. |
| artifact:loki-canary | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:loki-operator | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 2 registries; ordered by evidence strength. |
| source:release-notes-github | drop | validate.failed | computed | Relationship check failed: 7.0.0 fail (fetch https://api.github.com/repos/grafana/loki/releases/tags/helm-loki-7.0.0: not found (HTTP 404)); 7.1.0 fail (fetch https://api.github.com/repos/grafana/loki/releases/tags/helm-… |
| artifact:loki-chart-2 | drop | validate.failed | computed | Relationship check failed: 7.0.0 fail (no chart version ships appVersion helm-loki-7.0.0 (https://grafana.github.io/helm-charts (chart loki) lists 344 versions)); 7.1.0 fail (no chart version ships appVersion helm-loki-7… |
| artifact:loki-md-chart | annotate | validate.unverifiable | computed | Kept unverified: 7.0.0 unverifiable (no version index answered: helm-repo index https://grafana.github.io/k8s-monitoring-helm (chart loki.md): not-found; helm-git index github.com/grafana/k8s-monitoring-helm:charts/k8s-m… |
| artifact:loki-binary | drop | validate.failed | computed | Relationship check failed: 7.0.0 fail (absent from https://github.com/grafana/loki/releases/download/helm-loki-7.0.0/loki-linux-amd64.zip); 7.1.0 fail (absent from https://github.com/grafana/loki/releases/download/helm-l… |
| artifact:loki-canary | drop | validate.failed | computed | Relationship check failed: 7.0.0 fail (absent from docker.io/grafana/loki-canary:helm-loki-7.0.0); 7.1.0 fail (absent from docker.io/grafana/loki-canary:helm-loki-7.1.0); 7.2.0 fail (absent from docker.io/grafana/loki-ca… |
| artifact:loki-operator | drop | validate.failed | computed | Relationship check failed: 7.0.0 fail (absent from docker.io/grafana/loki-operator:helm-loki-7.0.0, quay.io/openshift-logging/loki-operator:helm-loki-7.0.0); 7.1.0 fail (absent from docker.io/grafana/loki-operator:helm-l… |

<details><summary>100 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| changelog `CHANGELOG.md` | changelog.stale | Changelog is not maintained: newest entry 3.8.0 while the latest release is helm-loki-7.3.0. |
| changelog `operator/CHANGELOG.md` | changelog.stale | Changelog is not maintained: newest entry 0.10.2 while the latest release is helm-loki-7.3.0. |
| chart-repo `github.com/grafana/meta-monitoring-chart` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| chart-repo `github.com/grafana/loki-helm-test` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| manifest `operator/config/manager/manager.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/config/overlays/development/minio/deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/config/overlays/openshift/size-calculator/logfile_metric_daemonset.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/config/overlays/openshift/size-calculator/storage_size_calculator.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/hack/addons_cert_manager.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/hack/addons_dev.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/hack/addons_hydra.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/hack/addons_logger.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/hack/addons_ocp.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/hack/addons_token_refresher.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `operator/hack/addons_traefik.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| crd `operator/config/crd/patches/webhook_in_alertingrules.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `operator/config/crd/patches/webhook_in_recordingrules.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `operator/config/crd/patches/webhook_in_rulerconfigs.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `operator/hack/addons_cert_manager.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/logql-analyzer` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/loki-canary-boringcrypto` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/loki-canary` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/loki-query-tee` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/fluent-bit-plugin-loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/fluent-plugin-loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/logcli` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/logstash-output-loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/loki-docker-driver` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/loki-helm-test` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `gcr.io/relyance-ext/compliance_inspector` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/grafana` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned). |
| image `docker.io/grafana/loki` | image.non-release-tags | Only snapshot/floating tags (floating,pinned) are produced, e.g. by branch builds. |
| image `docker.io/grafana/fluent-plugin-loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/fluent/fluent-bit` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/docs-base` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/grafana/loki-debug` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/grafana/promtail-debug` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `mirror.gcr.io/grafana/loki` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/grafana/alloy` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/minio/minio` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/mingrammer/flog` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/pgsty/minio` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/pgsty/mc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/observatorium/api` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/observatorium/opa-openshift` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/openshift-logging/passthrough-gateway` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/operator-framework/scorecard-test` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/openshift-logging/loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/library/controller` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/openshift-logging/log-file-metric-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/openshift-logging/storage-size-calculator` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror/loki-operator` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/my-company-org/loki-operator` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/openshift/origin-oauth-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/fedora` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-cainjector` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-controller` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-acmesolver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jetstack/cert-manager-webhook` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/logcli` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/grafana/promtail` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned). |
| image `docker.io/oryd/hydra` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/alpine/curl` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/openshift-logging/cluster-logging-load-client` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/observatorium/token-refresher` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/traefik` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `us.gcr.io/hosted-grafana/hosted-grafana-pro` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `us.gcr.io/hosted-grafana/pdc` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `us.gcr.io/hosted-grafana/hg-plugins` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `us.gcr.io/hosted-grafana/hgrun` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `us.gcr.io/hosted-grafana/hosted-grafana-security` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/prom/prometheus` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/alertmanager` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/enterprise-logs` | image.non-release-tags | Only snapshot/floating tags (floating,pinned) are produced, e.g. by branch builds. |
| image `us-docker.pkg.dev/grafanalabs-global/docker-enterprise-provisioner-prod/enterprise-provisi…` | image.non-release-tags | Only snapshot/floating tags (floating) are produced, e.g. by branch builds. |
| image `docker.io/grafana/loki-helm-test` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `docker.io/nginxinc/nginx-unprivileged` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/memcached-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/kiwigrid/k8s-sidecar` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/grafana-enterprise` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/provectuslabs/kafka-ui` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/apache/kafka` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/theperiklis/log-generator` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/hashicorp/consul` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/memcached` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/jaegertracing/all-in-one` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/loki` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/wurstmeister/zookeeper` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/wurstmeister/kafka` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/confluentinc/cp-zookeeper` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/confluentinc/cp-kafka` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/theperiklis/loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `docker.io/theperiklis/stream-generator` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image-name `cross` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `bundle` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `calculator` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `passthrough-gateway` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### changelog (3)

- `CHANGELOG.md` (high; docs.changelog) — [grafana/loki:CHANGELOG.md L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/CHANGELOG.md#L3): `## [3.8.0](https://github.com/grafana/loki/compare/v3.7.1...v3.8.0) (2026-06-08)`
- `operator/CHANGELOG.md` (high; docs.changelog) — [grafana/loki:operator/CHANGELOG.md L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/CHANGELOG.md#L3): `## [0.10.2](https://github.com/grafana/loki/compare/operator/v0.10.1...operator/v0.10.2) (2026-06-09)`
- `production/helm/loki/CHANGELOG.md` (high; docs.changelog) — [grafana/loki:production/helm/loki/CHANGELOG.md L16](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/CHANGELOG.md#L16): `## 7.3.0`

### chart-repo (4)

- `github.com/grafana/helm-charts` (medium; docs.chart-repo-ref) — [grafana/loki:CONTRIBUTING.md L206](https://github.com/grafana/loki/blob/helm-loki-7.3.0/CONTRIBUTING.md#L206): `[grafana/helm-charts](https://github.com/grafana/helm-charts) respository.`
- `github.com/grafana/k8s-monitoring-helm` (medium; docs.chart-repo-ref) — [grafana/loki:docs/sources/operations/meta-monitoring/deploy.md L87](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/operations/meta-monitoring/deploy.md#L87): `For further information on how to configure these methods, see the [Kubernetes Monitoring Helm examples](https…`
- `github.com/grafana/meta-monitoring-chart` (medium; docs.chart-repo-ref) — [grafana/loki:docs/sources/release-notes/v3-6.md L25](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/release-notes/v3-6.md#L25): `* **meta-monitoring:** Moves the meta-monitoring responsibilities from the [Grafana meta-monitoring Helm chart…`
- `github.com/grafana/loki-helm-test` (low; docs.chart-repo-ref) — [grafana/loki:production/helm/loki/src/helm-test/README.md L7](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/src/helm-test/README.md#L7): `Run 'go test .' from this directory, or use the Docker image published at 'grafana/loki-helm-test'.`

### crd (22)

- `operator/bundle/community-openshift/manifests/loki.grafana.com_alertingrules.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community-openshift/manifests/loki.grafana.com_alertingrules.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community-openshift/manifests/loki.grafana.com_alertingrules.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/community-openshift/manifests/loki.grafana.com_lokistacks.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community-openshift/manifests/loki.grafana.com_lokistacks.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community-openshift/manifests/loki.grafana.com_lokistacks.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/community-openshift/manifests/loki.grafana.com_recordingrules.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community-openshift/manifests/loki.grafana.com_recordingrules.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community-openshift/manifests/loki.grafana.com_recordingrules.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/community-openshift/manifests/loki.grafana.com_rulerconfigs.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community-openshift/manifests/loki.grafana.com_rulerconfigs.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community-openshift/manifests/loki.grafana.com_rulerconfigs.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/community/manifests/loki.grafana.com_alertingrules.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community/manifests/loki.grafana.com_alertingrules.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community/manifests/loki.grafana.com_alertingrules.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/community/manifests/loki.grafana.com_lokistacks.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community/manifests/loki.grafana.com_lokistacks.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community/manifests/loki.grafana.com_lokistacks.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/community/manifests/loki.grafana.com_recordingrules.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community/manifests/loki.grafana.com_recordingrules.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community/manifests/loki.grafana.com_recordingrules.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/community/manifests/loki.grafana.com_rulerconfigs.yaml` (high; crd.file) — [grafana/loki:operator/bundle/community/manifests/loki.grafana.com_rulerconfigs.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community/manifests/loki.grafana.com_rulerconfigs.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/openshift/manifests/loki.grafana.com_alertingrules.yaml` (high; crd.file) — [grafana/loki:operator/bundle/openshift/manifests/loki.grafana.com_alertingrules.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/openshift/manifests/loki.grafana.com_alertingrules.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/openshift/manifests/loki.grafana.com_lokistacks.yaml` (high; crd.file) — [grafana/loki:operator/bundle/openshift/manifests/loki.grafana.com_lokistacks.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/openshift/manifests/loki.grafana.com_lokistacks.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/openshift/manifests/loki.grafana.com_recordingrules.yaml` (high; crd.file) — [grafana/loki:operator/bundle/openshift/manifests/loki.grafana.com_recordingrules.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/openshift/manifests/loki.grafana.com_recordingrules.yaml#L2): `kind: CustomResourceDefinition`
- `operator/bundle/openshift/manifests/loki.grafana.com_rulerconfigs.yaml` (high; crd.file) — [grafana/loki:operator/bundle/openshift/manifests/loki.grafana.com_rulerconfigs.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/openshift/manifests/loki.grafana.com_rulerconfigs.yaml#L2): `kind: CustomResourceDefinition`
- `operator/config/crd/bases/config.grafana.com_projectconfigs.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/bases/config.grafana.com_projectconfigs.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/bases/config.grafana.com_projectconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/bases/loki.grafana.com_alertingrules.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/bases/loki.grafana.com_alertingrules.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/bases/loki.grafana.com_alertingrules.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/bases/loki.grafana.com_lokistacks.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/bases/loki.grafana.com_lokistacks.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/bases/loki.grafana.com_lokistacks.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/bases/loki.grafana.com_recordingrules.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/bases/loki.grafana.com_recordingrules.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/bases/loki.grafana.com_recordingrules.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/bases/loki.grafana.com_rulerconfigs.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/bases/loki.grafana.com_rulerconfigs.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/bases/loki.grafana.com_rulerconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/patches/webhook_in_alertingrules.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/patches/webhook_in_alertingrules.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/patches/webhook_in_alertingrules.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/patches/webhook_in_lokistacks.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/patches/webhook_in_lokistacks.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/patches/webhook_in_lokistacks.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/patches/webhook_in_recordingrules.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/patches/webhook_in_recordingrules.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/patches/webhook_in_recordingrules.yaml#L3): `kind: CustomResourceDefinition`
- `operator/config/crd/patches/webhook_in_rulerconfigs.yaml` (high; crd.file) — [grafana/loki:operator/config/crd/patches/webhook_in_rulerconfigs.yaml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/crd/patches/webhook_in_rulerconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `operator/hack/addons_cert_manager.yaml` (high; crd.file) — [grafana/loki:operator/hack/addons_cert_manager.yaml L23](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_cert_manager.yaml#L23): `kind: CustomResourceDefinition`

### docs-repo (1)

- `github.com/grafana/docs` (medium; docs.docs-repo-ref) — [grafana/loki:docs/README.md L64](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/README.md#L64): `2. Run the command 'make docs'. This uses the 'grafana/docs' image which internally uses Hugo to generate the …`

### helm-chart (1)

- `loki@production/helm/loki` (high; helm.chart) — [grafana/loki:production/helm/loki/Chart.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/Chart.yaml#L2): `name: loki`

### helm-oci (1)

- `ghcr.io/grafana-community/helm-charts/loki` (medium; docs.oci-ref) — [grafana/loki:docs/sources/setup/upgrade/upgrade-to-community/_index.md L62](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/setup/upgrade/upgrade-to-community/_index.md#L62): `helm upgrade <RELEASE_NAME> oci://ghcr.io/grafana-community/helm-charts/loki -f values.yaml --version 18.4.4`

### helm-repo (3)

- `https://fluent.github.io/helm-charts` (medium; docs.helm-repo-add) — [grafana/loki:docs/sources/send-data/fluentbit/community-plugin.md L114](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/send-data/fluentbit/community-plugin.md#L114): `helm repo add fluent https://fluent.github.io/helm-charts`
- `https://grafana-community.github.io/helm-charts` (medium; docs.helm-repo-add) — [grafana/loki:docs/sources/send-data/k8s-monitoring-helm/_index.md L81](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/send-data/k8s-monitoring-helm/_index.md#L81): `helm repo add grafana-community https://grafana-community.github.io/helm-charts && helm repo update`
- `https://grafana.github.io/helm-charts` (medium; docs.helm-repo-add) — [grafana/loki:docs/sources/operations/meta-monitoring/deploy.md L30](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/operations/meta-monitoring/deploy.md#L30): `helm repo add grafana https://grafana.github.io/helm-charts`

### image (80)

- `docker.io/grafana/enterprise-logs` (high; helm.values-image, manifest.image) — [grafana/loki:production/helm/loki/docs/examples/enterprise/batchjob.yaml L17](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/docs/examples/enterprise/batchjob.yaml#L17): `image: grafana/enterprise-logs:latest`
- `docker.io/grafana/loki` (high; docs.image-ref, helm.values-image, manifest.image) — [grafana/loki:clients/cmd/fluentd/docker/docker-compose.yml L6](https://github.com/grafana/loki/blob/helm-loki-7.3.0/clients/cmd/fluentd/docker/docker-compose.yml#L6): `image: grafana/loki:main`
- `docker.io/grafana/loki-canary` (high; helm.values-image) — [grafana/loki:production/helm/loki/values.yaml L837](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/values.yaml#L837): `repository: grafana/loki-canary`
- `docker.io/grafana/loki-helm-test` (high; helm.values-image) — [grafana/loki:production/helm/loki/values.yaml L777](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/values.yaml#L777): `repository: grafana/loki-helm-test`
- `docker.io/grafana/loki-operator` (high; docs.image-ref, kustomize.image, manifest.image) — [grafana/loki:operator/bundle/community-openshift/manifests/loki-operator.clusterserviceversion.yaml L2006](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community-openshift/manifests/loki-operator.clusterserviceversion.yaml#L2006): `image: docker.io/grafana/loki-operator:0.10.2`
- `docker.io/kiwigrid/k8s-sidecar` (high; helm.values-image) — [grafana/loki:production/helm/loki/values.yaml L4126](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/values.yaml#L4126): `repository: kiwigrid/k8s-sidecar`
- `docker.io/nginxinc/nginx-unprivileged` (high; helm.values-image) — [grafana/loki:production/helm/loki/values.yaml L1100](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/values.yaml#L1100): `repository: nginxinc/nginx-unprivileged`
- `docker.io/pgsty/mc` (high; helm.values-image, manifest.image) — [grafana/loki:examples/ha-monolithic/docker-compose.yaml L59](https://github.com/grafana/loki/blob/helm-loki-7.3.0/examples/ha-monolithic/docker-compose.yaml#L59): `image: pgsty/mc:latest`
- `docker.io/pgsty/minio` (high; helm.values-image, manifest.image) — [grafana/loki:examples/ha-monolithic/docker-compose.yaml L39](https://github.com/grafana/loki/blob/helm-loki-7.3.0/examples/ha-monolithic/docker-compose.yaml#L39): `image: pgsty/minio:latest`
- `docker.io/prom/memcached-exporter` (high; helm.values-image) — [grafana/loki:production/helm/loki/values.yaml L3621](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/values.yaml#L3621): `repository: prom/memcached-exporter`
- `quay.io/openshift-logging/loki-operator` (high; kustomize.image, manifest.image) — [grafana/loki:operator/bundle/openshift/manifests/loki-operator.clusterserviceversion.yaml L1991](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/openshift/manifests/loki-operator.clusterserviceversion.yaml#L1991): `image: quay.io/openshift-logging/loki-operator:0.1.0`
- `us-docker.pkg.dev/grafanalabs-global/docker-enterprise-provisioner-prod/enterprise-provisioner` (high; helm.values-image) — [grafana/loki:production/helm/loki/values.yaml L740](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/values.yaml#L740): `repository: grafanalabs-global/docker-enterprise-provisioner-prod/enterprise-provisioner`
- `docker.io/alpine/curl` (medium; manifest.image) — [grafana/loki:operator/hack/addons_hydra.yaml L125](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_hydra.yaml#L125): `image: alpine/curl`
- `docker.io/apache/kafka` (medium; manifest.image) — [grafana/loki:tools/dev/kafka/docker-compose.yaml L39](https://github.com/grafana/loki/blob/helm-loki-7.3.0/tools/dev/kafka/docker-compose.yaml#L39): `image: apache/kafka:latest`
- `docker.io/confluentinc/cp-kafka` (medium; manifest.image) — [grafana/loki:tools/kafka/sasl-plain/docker-compose.yml L16](https://github.com/grafana/loki/blob/helm-loki-7.3.0/tools/kafka/sasl-plain/docker-compose.yml#L16): `image: confluentinc/cp-kafka:latest`
- `docker.io/confluentinc/cp-zookeeper` (medium; manifest.image) — [grafana/loki:tools/kafka/sasl-plain/docker-compose.yml L3](https://github.com/grafana/loki/blob/helm-loki-7.3.0/tools/kafka/sasl-plain/docker-compose.yml#L3): `image: confluentinc/cp-zookeeper:latest`
- `docker.io/fluent/fluent-bit` (medium; manifest.image) — [grafana/loki:clients/cmd/fluentd/docker/docker-compose.yml L27](https://github.com/grafana/loki/blob/helm-loki-7.3.0/clients/cmd/fluentd/docker/docker-compose.yml#L27): `image: fluent/fluent-bit:5.1@sha256:bf09d620b6b45c080b4da86ac5d98fd3739c1213a148f93dec884fdcc64084cb`
- `docker.io/grafana/docs-base` (medium; manifest.image) — [grafana/loki:cmd/logql-analyzer/docker-compose.yaml L12](https://github.com/grafana/loki/blob/helm-loki-7.3.0/cmd/logql-analyzer/docker-compose.yaml#L12): `image: grafana/docs-base:latest`
- `docker.io/grafana/fluent-plugin-loki` (medium; manifest.image) — [grafana/loki:clients/cmd/fluentd/docker/docker-compose.yml L17](https://github.com/grafana/loki/blob/helm-loki-7.3.0/clients/cmd/fluentd/docker/docker-compose.yml#L17): `image: grafana/fluent-plugin-loki:main`
- `docker.io/grafana/grafana` (medium; manifest.image) — [grafana/loki:clients/cmd/docker-driver/docker-compose.yaml L4](https://github.com/grafana/loki/blob/helm-loki-7.3.0/clients/cmd/docker-driver/docker-compose.yaml#L4): `image: grafana/grafana`
- `docker.io/grafana/logcli` (medium; manifest.image) — [grafana/loki:operator/hack/addons_dev.yaml L32](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_dev.yaml#L32): `image: docker.io/grafana/logcli:3.7.3-amd64`
- `docker.io/grafana/loki-debug` (medium; manifest.image) — [grafana/loki:debug/docker-compose.yaml L11](https://github.com/grafana/loki/blob/helm-loki-7.3.0/debug/docker-compose.yaml#L11): `image: grafana/loki-debug:latest`
- `docker.io/grafana/promtail` (medium; manifest.image) — [grafana/loki:operator/hack/addons_dev.yaml L76](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_dev.yaml#L76): `image: docker.io/grafana/promtail:3.7.3`
- `docker.io/grafana/promtail-debug` (medium; manifest.image) — [grafana/loki:debug/docker-compose.yaml L23](https://github.com/grafana/loki/blob/helm-loki-7.3.0/debug/docker-compose.yaml#L23): `image: grafana/promtail-debug:latest`
- `docker.io/library/controller` (medium; manifest.image) — [grafana/loki:operator/config/manager/manager.yaml L22](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/manager/manager.yaml#L22): `image: controller:latest`
- … 55 more

### image-name (4)

- `bundle` (low; dockerfile.name) — [grafana/loki:operator/bundle/community-openshift/bundle.Dockerfile L1](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/bundle/community-openshift/bundle.Dockerfile#L1): `FROM scratch`
- `calculator` (low; dockerfile.name) — [grafana/loki:operator/calculator.Dockerfile L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/calculator.Dockerfile#L2): `FROM golang:1.26.5@sha256:3aff6657219a4d9c14e27fb1d8976c49c29fddb70ba835014f477e1c70636647 as builder`
- `cross` (low; dockerfile.name) — [grafana/loki:cmd/loki-canary/Dockerfile.cross L5](https://github.com/grafana/loki/blob/helm-loki-7.3.0/cmd/loki-canary/Dockerfile.cross#L5): `FROM golang:${GO_VERSION} AS build`
- `passthrough-gateway` (low; dockerfile.name) — [grafana/loki:operator/passthrough-gateway.Dockerfile L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/passthrough-gateway.Dockerfile#L2): `FROM golang:1.26.5@sha256:3aff6657219a4d9c14e27fb1d8976c49c29fddb70ba835014f477e1c70636647 as builder`

### manifest (20)

- `operator/config/manager/manager.yaml` (high; manifest.workloads) — [grafana/loki:operator/config/manager/manager.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/manager/manager.yaml#L2): `kind: Deployment`
- `operator/config/overlays/development/minio/deployment.yaml` (high; manifest.workloads) — [grafana/loki:operator/config/overlays/development/minio/deployment.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/overlays/development/minio/deployment.yaml#L2): `kind: Deployment`
- `operator/config/overlays/openshift/size-calculator/logfile_metric_daemonset.yaml` (high; manifest.workloads) — [grafana/loki:operator/config/overlays/openshift/size-calculator/logfile_metric_daemonset.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/overlays/openshift/size-calculator/logfile_metric_daemonset.yaml#L2): `kind: DaemonSet`
- `operator/config/overlays/openshift/size-calculator/storage_size_calculator.yaml` (high; manifest.workloads) — [grafana/loki:operator/config/overlays/openshift/size-calculator/storage_size_calculator.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/config/overlays/openshift/size-calculator/storage_size_calculator.yaml#L2): `kind: Deployment`
- `cmd/loki/loki-docker-config.yaml` (medium; docs.raw-url) — [grafana/loki:docs/sources/setup/upgrade/upgrade-2.x.md L901](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/setup/upgrade/upgrade-2.x.md#L901): `I would recommend taking the previous default file from the [1.6.0 tag on github](https://raw.githubuserconten…`
- `cmd/loki/loki-local-config.yaml` (medium; docs.raw-url) — [grafana/loki:docs/sources/setup/install/docker.md L33](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/setup/install/docker.md#L33): `wget https://raw.githubusercontent.com/grafana/loki/v3.6.0/cmd/loki/loki-local-config.yaml -O loki-config.yaml`
- `examples/getting-started/alloy-local-config.yaml` (medium; docs.raw-url) — [grafana/loki:docs/sources/get-started/quick-start/quick-start.md L100](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/get-started/quick-start/quick-start.md#L100): `wget https://raw.githubusercontent.com/grafana/loki/main/examples/getting-started/alloy-local-config.yaml -O a…`
- `examples/getting-started/docker-compose.yaml` (medium; docs.raw-url) — [grafana/loki:docs/sources/get-started/quick-start/quick-start.md L101](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/get-started/quick-start/quick-start.md#L101): `wget https://raw.githubusercontent.com/grafana/loki/main/examples/getting-started/docker-compose.yaml -O docke…`
- `examples/getting-started/loki-config.yaml` (medium; docs.raw-url) — [grafana/loki:docs/sources/get-started/quick-start/quick-start.md L99](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/get-started/quick-start/quick-start.md#L99): `wget https://raw.githubusercontent.com/grafana/loki/main/examples/getting-started/loki-config.yaml -O loki-con…`
- `operator/hack/addon_grafana_gateway_ocp.yaml` (medium; docs.raw-url, manifest.workloads) — [grafana/loki:operator/docs/user-guides/howto_connect_grafana.md L57](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/docs/user-guides/howto_connect_grafana.md#L57): `An example configuration using this technique is available in ['addon_grafana_gateway_ocp.yaml'](https://raw.g…`
- `operator/hack/addon_grafana_gateway_ocp_oauth.yaml` (medium; docs.raw-url, manifest.workloads) — [grafana/loki:operator/docs/user-guides/howto_connect_grafana.md L45](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/docs/user-guides/howto_connect_grafana.md#L45): `An example configuration authenticating to the gateway in this manner is available in  ['addon_grafana_gateway…`
- `operator/hack/addons_cert_manager.yaml` (medium; manifest.workloads) — [grafana/loki:operator/hack/addons_cert_manager.yaml L13233](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_cert_manager.yaml#L13233): `kind: Deployment`
- `operator/hack/addons_dev.yaml` (medium; manifest.workloads) — [grafana/loki:operator/hack/addons_dev.yaml L15](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_dev.yaml#L15): `kind: Deployment`
- `operator/hack/addons_hydra.yaml` (medium; manifest.workloads) — [grafana/loki:operator/hack/addons_hydra.yaml L21](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_hydra.yaml#L21): `kind: Deployment`
- `operator/hack/addons_logger.yaml` (medium; manifest.workloads) — [grafana/loki:operator/hack/addons_logger.yaml L2](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_logger.yaml#L2): `kind: Deployment`
- `operator/hack/addons_ocp.yaml` (medium; manifest.workloads) — [grafana/loki:operator/hack/addons_ocp.yaml L15](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_ocp.yaml#L15): `kind: Deployment`
- `operator/hack/addons_token_refresher.yaml` (medium; manifest.workloads) — [grafana/loki:operator/hack/addons_token_refresher.yaml L13](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_token_refresher.yaml#L13): `kind: Deployment`
- `operator/hack/addons_traefik.yaml` (medium; manifest.workloads) — [grafana/loki:operator/hack/addons_traefik.yaml L118](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/hack/addons_traefik.yaml#L118): `kind: Deployment`
- `production/helm/loki/values.yaml` (medium; docs.raw-url) — [grafana/loki:docs/sources/setup/upgrade/upgrade-2.x.md L851](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/setup/upgrade/upgrade-2.x.md#L851): `We suggest using the included [values.yaml file from the 1.6.0 tag](https://raw.githubusercontent.com/grafana/…`
- `production/helm/meta-monitoring/values.yaml` (medium; docs.raw-url) — [grafana/loki:docs/sources/operations/meta-monitoring/deploy.md L96](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/operations/meta-monitoring/deploy.md#L96): `curl -O https://raw.githubusercontent.com/grafana/loki/main/production/helm/meta-monitoring/values.yaml`

### registry (7)

- `quay.io` (medium; workflow.registry-login) — [grafana/loki:.github/workflows/operator-reusable-image-build.yml L57](https://github.com/grafana/loki/blob/helm-loki-7.3.0/.github/workflows/operator-reusable-image-build.yml#L57): `registry: quay.io`
- `docker.io/grafana` (low; make.registry-ref, make.registry-variable) — [grafana/loki:Makefile L63](https://github.com/grafana/loki/blob/helm-loki-7.3.0/Makefile#L63): `IMAGE_PREFIX           ?= grafana`
- `quay.io/openshift-logging` (low; make.registry-ref) — [grafana/loki:operator/Makefile L31](https://github.com/grafana/loki/blob/helm-loki-7.3.0/operator/Makefile#L31): `REGISTRY_BASE_OPENSHIFT = quay.io/openshift-logging`
- `registry.terraform.io/providers/fgouteroux/loki/latest` (low; docs.registry-ref) — [grafana/loki:docs/sources/alert/_index.md L300](https://github.com/grafana/loki/blob/helm-loki-7.3.0/docs/sources/alert/_index.md#L300): `With the [Terraform provider for Loki](https://registry.terraform.io/providers/fgouteroux/loki/latest), you ca…`
- `us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror` (low; workflow.registry-ref) — [grafana/loki:.github/workflows/minor-release-pr.yml L10](https://github.com/grafana/loki/blob/helm-loki-7.3.0/.github/workflows/minor-release-pr.yml#L10): `IMAGE_PREFIX: "us-docker.pkg.dev/grafanalabs-global/dockerhub-loki-prod-mirror"`
- `us.gcr.io/v2/hosted-grafana/hosted-grafana-pro/manifests/10.1.0-ephemeral-oss-77506-8314-2` (low; docs.registry-ref) — [grafana/loki:pkg/pattern/drain/testdata/journald.txt L474](https://github.com/grafana/loki/blob/helm-loki-7.3.0/pkg/pattern/drain/testdata/journald.txt#L474): `E0507 11:59:37.414253    4589 pod_workers.go:1300] "Error syncing pod, skipping" err="failed to \"StartContain…`
- `us.gcr.io/v2/hosted-grafana/hosted-grafana-pro/manifests/11.1.0-ephemeral-6436-938-1` (low; docs.registry-ref) — [grafana/loki:pkg/pattern/drain/testdata/journald.txt L699](https://github.com/grafana/loki/blob/helm-loki-7.3.0/pkg/pattern/drain/testdata/journald.txt#L699): `E0507 11:59:34.353776    4585 pod_workers.go:1300] "Error syncing pod, skipping" err="failed to \"StartContain…`

### release-asset (1)

- `https://github.com/grafana/loki/releases/download/{{.Tag}}/loki-linux-amd64.zip` (medium; docs.release-asset-url) — [grafana/loki:CHANGELOG.md L3254](https://github.com/grafana/loki/blob/helm-loki-7.3.0/CHANGELOG.md#L3254): `$ curl -O -L "https://github.com/grafana/loki/releases/download/v2.9.3/loki-linux-amd64.zip"`

### tag-scheme (1)

- `^helm-loki-(?P<version>\d+\.\d+\.\d+)$` (high; tags.ls-remote) — [grafana/loki/releases/tag/helm-loki-7.3.0 refs/tags/helm-loki-7.3.0](https://github.com/grafana/loki/releases/tag/helm-loki-7.3.0#refs/tags/helm-loki-7.3.0): `50c738b4d5b28db6e26723c3828da13bb46f0f06 refs/tags/helm-loki-7.3.0`

### version-relation (1)

- `chart.version = {{.Version}}` (high; helm.chart-version-matches-ref) — [grafana/loki:production/helm/loki/Chart.yaml L6](https://github.com/grafana/loki/blob/helm-loki-7.3.0/production/helm/loki/Chart.yaml#L6): `version: 7.3.0`
