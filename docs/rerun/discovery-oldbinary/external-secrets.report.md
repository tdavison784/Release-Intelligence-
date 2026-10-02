# Discovery report: external-secrets

- Repository: `github.com/external-secrets/external-secrets` at `v2.11.0` (commit `e8f12e1f1646…`)
- Generated: 2026-10-02T12:41:22Z
- LLM: not used (deterministic resolver only)
- Tags: 272 tags: prefix "v", 136 stable, 4 prereleases (rcN×4), 132 junk; latest stable v2.11.0; lineage linear
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\d+))?)$` (junk sample: helm-chart-0.1.1, helm-chart-0.1.2, helm-chart-0.1.3, helm-chart-0.1.4, helm-chart-0.10.0, helm-chart-0.10.1)
- Scanned `github.com/external-secrets/external-secrets@v2.11.0` (source profile): 1804 files listed, 652 read
- Validation: done against v2.8.0, v2.9.0, v2.10.0, v2.11.0

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 4 | git-tags github.com/external-secrets/external-secrets; github-releases external-secrets/external-secrets |
| release-notes | found | 0 | github-releases external-secrets/external-secrets  @{{.Tag}} |
| changelog | not-found | 0 |  |
| helm-charts | found | 3 | external-secrets via helm-repo:https://charts.external-secrets.io |
| registries | found | 2 | docker.io/external-secrets |
| images | found | 11 | docker.io/external-secrets/external-secrets:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | candidates-only | 1 |  |
| security | found | 1 | github-advisories external-secrets/external-secrets |
| version-relations | found | 1 | external-secrets-chart: independent; crds: version = {{.Tag}}; external-secrets: version = {{.Tag}} |

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: external-secrets
name: external-secrets
homepage: https://github.com/external-secrets/external-secrets
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\d+))?)$
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/external-secrets/external-secrets
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: external-secrets/external-secrets
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: external-secrets/external-secrets
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    validatedAgainst:
      - 2.8.0
      - 2.9.0
      - 2.10.0
      - 2.11.0
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: external-secrets/external-secrets
    notes: 'Security policy: SECURITY.md'
artifacts:
  - id: external-secrets-chart
    type: helm-chart
    name: external-secrets
    version:
      strategy: independent
    channels:
      - kind: helm-repo
        url: https://charts.external-secrets.io
        chart: external-secrets
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/external-secrets/external-secrets
          path: deploy/charts/external-secrets/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/external-secrets/external-secrets
          path: deploy/charts/external-secrets/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 2.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 2.9.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 2.10.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 2.11.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable)).'
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/external-secrets/external-secrets
        path: config/crds/bases
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 2.8.0
      - 2.9.0
      - 2.10.0
      - 2.11.0
  - id: external-secrets
    type: container-image
    name: external-secrets
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: docker.io/external-secrets/external-secrets
    notes: 'Unverified by discovery (unverifiable: 2.8.0 unverifiable (not verifiable: docker.io/external-secrets/external-secrets:v2.8.0 (oci: unavailable)); 2.9.0 unverifiable (not verifiable: docker.io/external-secrets/external-secrets:v2.9.0 (oci: unavailable)); 2.10.0 unverifiable (not verifiable: docker.io/external-secrets/external-secrets:v2.10.0 (oci: unavailable)); 2.11.0 unverifiable (not verifiable: docker.io/external-secrets/external-secrets:v2.11.0 (oci: unavailable))).'
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - v2.8.0
    - v2.9.0
    - v2.10.0
    - v2.11.0
  notes: Proposed by automated discovery; relationship checks run against v2.8.0, v2.9.0, v2.10.0, v2.11.0.
```

## Open questions for the reviewer

- Ambiguity (chart-version): How do the versions of chart(s) external-secrets relate to the release version?

## Validation matrix

| Element | Verdict | v2.8.0 | v2.9.0 | v2.10.0 | v2.11.0 |
|---|---|---|---|---|---|
| artifact:crds | validated |  |  |  |  |
| artifact:external-secrets | unverifiable |  |  |  |  |
| artifact:external-secrets-chart | unverifiable |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |
| content:external-secrets-chart/chart-metadata | validated |  |  |  |  |
| content:external-secrets-chart/helm-values | validated |  |  |  |  |
| source:compatibility | failing |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |

## Dropped elements

- `source:compatibility` (deterministic): failed validation: 2.8.0 fail (no row whose ESO Version matches "^v?2\\.8\\b" in https://github.com/external-secrets/external-secrets/blob/v2.8.0/docs/introduction/stability-support.md); 2.9.0 fail (no row whose ESO Version matches "^v?2\\.9\\b" in https://github.com/external-secrets/external-secrets/blob/v2.9.0/docs/introduction/stability-support.md); 2.10.0 fail (no row whose ESO Version matches "^v?2\\.10\\b" in https://github.com/external-secrets/external-secrets/blob/v2.10.0/docs/introduction/stability-support.md)

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 272 tags: prefix "v", 136 stable, 4 prereleases (rcN×4), 132 junk; latest stable v2.11.0; lineage linear; strict tagPattern excludes junk tags such as helm-chart-0.1.1, helm-chart-0.1.2, helm-chart-0.1.3, helm-chart-0.1… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (272 tags: prefix "v", 136 stable, 4 prereleases (rcN×4), 132 junk; latest stable v2.11.0; lineage linear). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:compatibility | include | compat.markdown-table | heuristic | Support matrix keyed by release line (35 rows; key column ESO Version; Kubernetes columns Kubernetes Version). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:external-secrets-chart | include | helm.independent-version | heuristic | Chart has its own version "2.10.0" unrelated to the release. Published via 1 channel(s); OCI locations first. |
| candidate:cand-54ba14f57838 | annotate | asset.integrity-file | heuristic | Checksum/signature/provenance asset; recorded but not modelled as an artifact. |
| artifact:crds | include | crd.in-repo | heuristic | 25 CRDs of the product's API groups in config/crds/bases at the release tag. |
| artifact:external-secrets | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| source:compatibility | drop | validate.failed | computed | Relationship check failed: 2.8.0 fail (no row whose ESO Version matches "^v?2\\.8\\b" in https://github.com/external-secrets/external-secrets/blob/v2.8.0/docs/introduction/stability-support.md); 2.9.0 fail (no row whose … |
| artifact:external-secrets-chart | annotate | validate.unverifiable | computed | Kept unverified: 2.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 2.9.0 unverifiable (artifact version is independent of the release version (strategy in… |
| artifact:external-secrets | annotate | validate.unverifiable | computed | Kept unverified: 2.8.0 unverifiable (not verifiable: docker.io/external-secrets/external-secrets:v2.8.0 (oci: unavailable)); 2.9.0 unverifiable (not verifiable: docker.io/external-secrets/external-secrets:v2.9.0 (oci: un… |

<details><summary>12 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| manifest `docs/snippets/1password-connect-server-deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `docs/snippets/cloak-proxy-deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| image `ghcr.io/external-secrets/external-secrets` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/apache/skywalking-eyes` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/jnorwood/helm-docs` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/your-user/external-secrets` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/1password/connect-api` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/1password/connect-sync` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/purtontech/cloak-external-secrets` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `ghcr.io/external-secrets/external-secrets-e2e` | image.test-only | Only referenced from test, sample or documentation files. |
| image-name `standalone` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `ubi` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### build-tool (3)

- `goreleaser` (high; goreleaser.config, workflow.goreleaser) — [external-secrets/external-secrets:.github/workflows/release_esoctl.yml L112](https://github.com/external-secrets/external-secrets/blob/v2.11.0/.github/workflows/release_esoctl.yml#L112): `uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3`
- `chart-releaser` (medium; workflow.chart-releaser) — [external-secrets/external-secrets:.github/workflows/helm.yml L109](https://github.com/external-secrets/external-secrets/blob/v2.11.0/.github/workflows/helm.yml#L109): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`
- `cosign` (medium; workflow.cosign) — [external-secrets/external-secrets:.github/workflows/helm.yml L135](https://github.com/external-secrets/external-secrets/blob/v2.11.0/.github/workflows/helm.yml#L135): `uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2`

### compatibility (1)

- `docs/introduction/stability-support.md` (high; compat.markdown-table) — [external-secrets/external-secrets:docs/introduction/stability-support.md L19](https://github.com/external-secrets/external-secrets/blob/v2.11.0/docs/introduction/stability-support.md#L19): `| ESO Version | Kubernetes Version | Release Date | End of Life    |`

### crd (26)

- `config/crds/bases/external-secrets.io_clusterexternalsecrets.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/external-secrets.io_clusterexternalsecrets.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/external-secrets.io_clusterexternalsecrets.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/external-secrets.io_clusterpushsecrets.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/external-secrets.io_clusterpushsecrets.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/external-secrets.io_clusterpushsecrets.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/external-secrets.io_clustersecretstores.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/external-secrets.io_clustersecretstores.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/external-secrets.io_clustersecretstores.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/external-secrets.io_externalsecrets.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/external-secrets.io_externalsecrets.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/external-secrets.io_externalsecrets.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/external-secrets.io_pushsecrets.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/external-secrets.io_pushsecrets.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/external-secrets.io_pushsecrets.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/external-secrets.io_secretstores.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/external-secrets.io_secretstores.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/external-secrets.io_secretstores.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_acraccesstokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_acraccesstokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_acraccesstokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_beyondtrustworkloadcredentialsdynamicsecrets.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_beyondtrustworkloadcredentialsdynamicsecrets.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_beyondtrustworkloadcredentialsdynamicsecrets.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_cloudsmithaccesstokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_cloudsmithaccesstokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_cloudsmithaccesstokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_clustergenerators.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_clustergenerators.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_clustergenerators.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_ecrauthorizationtokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_ecrauthorizationtokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_ecrauthorizationtokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_fakes.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_fakes.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_fakes.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_gcraccesstokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_gcraccesstokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_gcraccesstokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_generatorstates.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_generatorstates.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_generatorstates.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_githubaccesstokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_githubaccesstokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_githubaccesstokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_gitlabdeploytokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_gitlabdeploytokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_gitlabdeploytokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_grafanas.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_grafanas.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_grafanas.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_mfas.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_mfas.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_mfas.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_passwords.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_passwords.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_passwords.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_quayaccesstokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_quayaccesstokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_quayaccesstokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_sshkeys.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_sshkeys.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_sshkeys.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_stssessiontokens.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_stssessiontokens.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_stssessiontokens.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_uuids.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_uuids.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_uuids.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_vaultdynamicsecrets.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_vaultdynamicsecrets.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_vaultdynamicsecrets.yaml#L2): `kind: CustomResourceDefinition`
- `config/crds/bases/generators.external-secrets.io_webhooks.yaml` (high; crd.file) — [external-secrets/external-secrets:config/crds/bases/generators.external-secrets.io_webhooks.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/config/crds/bases/generators.external-secrets.io_webhooks.yaml#L2): `kind: CustomResourceDefinition`
- … 1 more

### helm-chart (1)

- `external-secrets@deploy/charts/external-secrets` (high; helm.chart) — [external-secrets/external-secrets:deploy/charts/external-secrets/Chart.yaml L2](https://github.com/external-secrets/external-secrets/blob/v2.11.0/deploy/charts/external-secrets/Chart.yaml#L2): `name: external-secrets`

### helm-repo (2)

- `https://charts.external-secrets.io` (medium; docs.helm-repo-add) — [external-secrets/external-secrets:deploy/charts/external-secrets/README.md L13](https://github.com/external-secrets/external-secrets/blob/v2.11.0/deploy/charts/external-secrets/README.md#L13): `helm repo add external-secrets https://charts.external-secrets.io`
- `https://external-secrets.github.io/external-secrets` (medium; workflow.chart-releaser-pages) — [external-secrets/external-secrets:.github/workflows/helm.yml L109](https://github.com/external-secrets/external-secrets/blob/v2.11.0/.github/workflows/helm.yml#L109): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`

### image (9)

- `docker.io/external-secrets/external-secrets` (high; helm.values-image) — [external-secrets/external-secrets:deploy/charts/external-secrets/values.yaml L61](https://github.com/external-secrets/external-secrets/blob/v2.11.0/deploy/charts/external-secrets/values.yaml#L61): `repository: external-secrets/external-secrets`
- `docker.io/1password/connect-api` (medium; manifest.image) — [external-secrets/external-secrets:docs/snippets/1password-connect-server-deployment.yaml L11](https://github.com/external-secrets/external-secrets/blob/v2.11.0/docs/snippets/1password-connect-server-deployment.yaml#L11): `image: 1password/connect-api:1.5.0`
- `docker.io/1password/connect-sync` (medium; manifest.image) — [external-secrets/external-secrets:docs/snippets/1password-connect-server-deployment.yaml L20](https://github.com/external-secrets/external-secrets/blob/v2.11.0/docs/snippets/1password-connect-server-deployment.yaml#L20): `image: 1password/connect-sync:1.5.0`
- `docker.io/apache/skywalking-eyes` (medium; make.image-ref) — [external-secrets/external-secrets:Makefile L95](https://github.com/external-secrets/external-secrets/blob/v2.11.0/Makefile#L95): `$(DOCKER) run --rm -u $(shell id -u) -v $(shell pwd):/github/workspace docker.io/apache/skywalking-eyes:0.9.0@…`
- `docker.io/jnorwood/helm-docs` (medium; make.image-ref) — [external-secrets/external-secrets:Makefile L232](https://github.com/external-secrets/external-secrets/blob/v2.11.0/Makefile#L232): `$(DOCKER) run --rm -v $(shell pwd)/$(HELM_DIR):/helm-docs -u $(shell id -u) docker.io/jnorwood/helm-docs:v1.14…`
- `docker.io/purtontech/cloak-external-secrets` (medium; manifest.image) — [external-secrets/external-secrets:docs/snippets/cloak-proxy-deployment.yaml L19](https://github.com/external-secrets/external-secrets/blob/v2.11.0/docs/snippets/cloak-proxy-deployment.yaml#L19): `image: purtontech/cloak-external-secrets:latest`
- `docker.io/your-user/external-secrets` (medium; docs.image-ref) — [external-secrets/external-secrets:docs/contributing/process.md L153](https://github.com/external-secrets/external-secrets/blob/v2.11.0/docs/contributing/process.md#L153): `# you may have to set IMAGE_NAME=docker.io/your-user/external-secrets`
- `ghcr.io/external-secrets/external-secrets` (medium; docs.image-ref, make.image-ref) — [external-secrets/external-secrets:Makefile L21](https://github.com/external-secrets/external-secrets/blob/v2.11.0/Makefile#L21): `export IMAGE_NAME ?= $(IMAGE_REGISTRY)/$(IMAGE_REPO)`
- `ghcr.io/external-secrets/external-secrets-e2e` (low; make.image-ref) — [external-secrets/external-secrets:e2e/Makefile L8](https://github.com/external-secrets/external-secrets/blob/v2.11.0/e2e/Makefile#L8): `export E2E_IMAGE_NAME ?= ghcr.io/external-secrets/external-secrets-e2e`

### image-name (2)

- `standalone` (low; dockerfile.name) — [external-secrets/external-secrets:Dockerfile.standalone L3](https://github.com/external-secrets/external-secrets/blob/v2.11.0/Dockerfile.standalone#L3): `FROM golang:1.27.0-alpine@sha256:4c9fe60190a2a3350ddc51de80d0224b8a6698d12bdfc999fee45ea9d6c46dbc AS builder`
- `ubi` (low; dockerfile.name) — [external-secrets/external-secrets:Dockerfile.ubi L1](https://github.com/external-secrets/external-secrets/blob/v2.11.0/Dockerfile.ubi#L1): `FROM registry.access.redhat.com/ubi9/ubi@sha256:c5cc9c221baa8eb13093f90c31bb58c04d83f6afd14510d3496691f92566f9…`

### manifest (2)

- `docs/snippets/1password-connect-server-deployment.yaml` (medium; manifest.workloads) — [external-secrets/external-secrets:docs/snippets/1password-connect-server-deployment.yaml L3](https://github.com/external-secrets/external-secrets/blob/v2.11.0/docs/snippets/1password-connect-server-deployment.yaml#L3): `kind: Deployment`
- `docs/snippets/cloak-proxy-deployment.yaml` (medium; manifest.workloads) — [external-secrets/external-secrets:docs/snippets/cloak-proxy-deployment.yaml L3](https://github.com/external-secrets/external-secrets/blob/v2.11.0/docs/snippets/cloak-proxy-deployment.yaml#L3): `kind: Deployment`

### registry (2)

- `ghcr.io` (medium; make.registry-variable, workflow.registry-login) — [external-secrets/external-secrets:.github/workflows/e2e-managed.yml L138](https://github.com/external-secrets/external-secrets/blob/v2.11.0/.github/workflows/e2e-managed.yml#L138): `registry: ghcr.io`
- `ghcr.io/external-secrets/external-secrets` (low; make.registry-ref) — [external-secrets/external-secrets:Makefile L327](https://github.com/external-secrets/external-secrets/blob/v2.11.0/Makefile#L327): `@echo $(IMAGE_NAME):$(IMAGE_TAG)`

### release-asset (1)

- `https://github.com/external-secrets/external-secrets/releases/download/{{.Tag}}/esoctl_checksums.txt` (high; goreleaser.checksum) — [external-secrets/external-secrets:cmd/esoctl/.goreleaser.yaml L36](https://github.com/external-secrets/external-secrets/blob/v2.11.0/cmd/esoctl/.goreleaser.yaml#L36): `checksum:`

### release-publisher (2)

- `goreleaser` (high; goreleaser.release) — [external-secrets/external-secrets:cmd/esoctl/.goreleaser.yaml L1](https://github.com/external-secrets/external-secrets/blob/v2.11.0/cmd/esoctl/.goreleaser.yaml#L1): `version: 2`
- `softprops/action-gh-release` (medium; workflow.release-upload) — [external-secrets/external-secrets:.github/workflows/release.yml L65](https://github.com/external-secrets/external-secrets/blob/v2.11.0/.github/workflows/release.yml#L65): `uses: softprops/action-gh-release@efb35369e0ad2afab669f228072c1b0d510eae64 # v3.0.3`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [external-secrets/external-secrets:SECURITY.md L1](https://github.com/external-secrets/external-secrets/blob/v2.11.0/SECURITY.md#L1): `# Security Policy`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\d+))?)$` (high; tags.ls-remote) — [external-secrets/external-secrets/releases/tag/v2.11.0 refs/tags/v2.11.0](https://github.com/external-secrets/external-secrets/releases/tag/v2.11.0#refs/tags/v2.11.0): `e8f12e1f1646e0ad47966458023ff10c9577f2b0 refs/tags/v2.11.0`

### version-relation (1)

- `build.VERSION = {{.Tag}}` (high; make.version-from-git) — [external-secrets/external-secrets:Makefile L41](https://github.com/external-secrets/external-secrets/blob/v2.11.0/Makefile#L41): `export VERSION := $(shell echo "v0.0.0-$$(git rev-list HEAD --count)-g$$(git describe --dirty --always)" | sed…`
