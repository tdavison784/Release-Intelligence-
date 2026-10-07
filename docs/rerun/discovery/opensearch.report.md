# Discovery report: OpenSearch

- Repository: `github.com/opensearch-project/OpenSearch` at `3.9.0` (commit `4ee42a94e87f…`)
- Generated: 2026-10-02T12:37:12Z
- LLM: not used (deterministic resolver only)
- Tags: 78 tags: prefix "", 71 stable, 7 prereleases (alphaN×3, betaN×2, rcN×2), 0 junk; latest stable 3.9.0; lineage linear
- Scanned `github.com/opensearch-project/OpenSearch@3.9.0` (source profile): 17644 files listed, 1046 read
- Scanned `github.com/opensearch-project/documentation-website@main` (docs profile): 3009 files listed, 427 read
- Validation: done against 3.6.0, 3.7.0, 3.8.0, 3.9.0

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 4 | git-tags github.com/opensearch-project/OpenSearch; github-releases opensearch-project/OpenSearch |
| release-notes | found | 2 | repo-file github.com/opensearch-project/OpenSearch release-notes/opensearch.release-notes-{{.Version}}.md; github-releases opensearch-project/OpenSearch  @{{.Tag}} |
| changelog | not-found | 0 |  |
| helm-charts | candidates-only | 3 |  |
| registries | candidates-only | 1 |  |
| images | candidates-only | 7 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 7 | github-advisories opensearch-project/OpenSearch |
| version-relations | not-found | 0 |  |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| source:release-notes-docs | historically-validated | notes.per-release-file | https://github.com/opensearch-project/OpenSearch/blob/3.9.0/release-notes/opensearch.release-notes-3.9.0.md# |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/opensearch-project/OpenSearch/blob/3.9.0/.github/workflows/auto-release.yml#L20 |
| source:advisories | discovered | security.github-hosted-default | https://github.com/opensearch-project/OpenSearch/blob/3.9.0/SECURITY.md#L1 (+2) |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/opensearch-project/OpenSearch/blob/3.9.0/.github/workflows/auto-release.yml#L20 |
| source:tags | discovered | versions.git-tags | https://github.com/opensearch-project/OpenSearch/releases/tag/3.9.0#refs/tags/3.9.0 |
| versioning | discovered | tags.scheme | https://github.com/opensearch-project/OpenSearch/releases/tag/3.9.0#refs/tags/3.9.0 (+2) |

Statuses: 2 historically-validated, 4 discovered.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: opensearch
name: OpenSearch
homepage: https://github.com/opensearch-project/OpenSearch
versioning:
  scheme: semver
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/opensearch-project/OpenSearch
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: opensearch-project/OpenSearch
    priority: 1
  - id: release-notes-docs
    roles:
      - release-notes
    locator:
      kind: repo-file
      repository: github.com/opensearch-project/OpenSearch
      path: release-notes/opensearch.release-notes-{{.Version}}.md
    fallbackGroup: release-notes
    validatedAgainst:
      - 3.6.0
      - 3.7.0
      - 3.8.0
      - 3.9.0
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: opensearch-project/OpenSearch
      ref: '{{.Tag}}'
    priority: 1
    fallbackGroup: release-notes
    validatedAgainst:
      - 3.6.0
      - 3.7.0
      - 3.8.0
      - 3.9.0
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: opensearch-project/OpenSearch
    notes: 'Security policy: SECURITY.md → https://github.com/opensearch-project/.github/blob/…/SECURITY.md'
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - 3.6.0
    - 3.7.0
    - 3.8.0
    - 3.9.0
  notes: Proposed by automated discovery; relationship checks run against 3.6.0, 3.7.0, 3.8.0, 3.9.0.
```

## Open questions for the reviewer

- Ambiguity (docs-sources): Documentation lives in another repository; no upgrade-guide, compatibility source was derived from it.

## Validation matrix

| Element | Verdict | 3.6.0 | 3.7.0 | 3.8.0 | 3.9.0 |
|---|---|---|---|---|---|
| source:release-notes-docs | validated | pass | pass | pass | pass |
| source:release-notes-github | validated | pass | pass | pass | pass |

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 78 tags: prefix "", 71 stable, 7 prereleases (alphaN×3, betaN×2, rcN×2), 0 junk; latest stable 3.9.0; lineage linear |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (78 tags: prefix "", 71 stable, 7 prereleases (alphaN×3, betaN×2, rcN×2), 0 junk; latest stable 3.9.0; lineage linear). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-docs | include | notes.per-release-file | heuristic | One notes document per release (69 instances, newest release-notes/opensearch.release-notes-3.9.0.md). |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |

<details><summary>9 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| release-notes `release-notes/opensearch-documentation-release-notes-{{.Version}}.md` | notes.lower-ranked-template | Another per-release notes path template ranks higher. |
| chart-repo `github.com/opensearch-project/helm-charts` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| image `docker.io/library/opensearch` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/library/haproxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/wildfly/wildfly` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `public.ecr.aws/opensearchproject/opensearch-benchmark` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `public.ecr.aws/opensearchproject/opensearch` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `public.ecr.aws/opensearchproject/opensearch-dashboards` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image-name `eks` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### chart-repo (1)

- `github.com/opensearch-project/helm-charts` (medium; docs.chart-repo-ref) — [opensearch-project/documentation-website:_install-and-configure/install-dashboards/helm.md L14](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_install-and-configure/install-dashboards/helm.md#L14): `The [Helm chart](https://github.com/opensearch-project/helm-charts) contains the resources described in the fo…`

### docs-repo (1)

- `github.com/opensearch-project/documentation-website` (medium; docs.docs-repo-ref) — [opensearch-project/OpenSearch:CONTRIBUTING.md L58](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/CONTRIBUTING.md#L58): `If you would like to contribute to the documentation, please do so in the [documentation-website](https://gith…`

### helm-repo (2)

- `https://opensearch-project.github.io/helm-charts` (medium; docs.helm-repo-add) — [opensearch-project/documentation-website:_install-and-configure/install-opensearch/helm.md L48](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_install-and-configure/install-opensearch/helm.md#L48): `helm repo add opensearch https://opensearch-project.github.io/helm-charts/`
- `https://opensearch-project.github.io/opensearch-k8s-operator` (medium; docs.helm-repo-add) — [opensearch-project/documentation-website:_install-and-configure/install-opensearch/operator/index.md L24](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_install-and-configure/install-opensearch/operator/index.md#L24): `helm repo add opensearch-operator https://opensearch-project.github.io/opensearch-k8s-operator/`

### image (6)

- `docker.io/library/haproxy` (medium; manifest.image) — [opensearch-project/OpenSearch:qa/remote-clusters/docker-compose.yml L70](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/qa/remote-clusters/docker-compose.yml#L70): `image: haproxy:2.1.2`
- `docker.io/library/opensearch` (medium; manifest.image) — [opensearch-project/OpenSearch:distribution/docker/docker-compose.yml L5](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/distribution/docker/docker-compose.yml#L5): `image: opensearch:test`
- `public.ecr.aws/opensearchproject/opensearch` (medium; docs.image-ref) — [opensearch-project/documentation-website:_install-and-configure/install-opensearch/docker.md L83](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_install-and-configure/install-opensearch/docker.md#L83): `docker pull public.ecr.aws/opensearchproject/opensearch:{{ site.opensearch_version | split: "." | first }}`
- `public.ecr.aws/opensearchproject/opensearch-dashboards` (medium; docs.image-ref) — [opensearch-project/documentation-website:_install-and-configure/install-opensearch/docker.md L88](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_install-and-configure/install-opensearch/docker.md#L88): `docker pull public.ecr.aws/opensearchproject/opensearch-dashboards:{{ site.opensearch_version | split: "." | f…`
- `quay.io/wildfly/wildfly` (medium; manifest.image) — [opensearch-project/OpenSearch:qa/wildfly/docker-compose.yml L5](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/qa/wildfly/docker-compose.yml#L5): `image: quay.io/wildfly/wildfly:34.0.1.Final-jdk21`
- `public.ecr.aws/opensearchproject/opensearch-benchmark` (low; docs.image-ref) — [opensearch-project/documentation-website:_benchmark/user-guide/install-and-configure/installing-benchmark.md L126](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_benchmark/user-guide/install-and-configure/installing-benchmark.md#L126): `docker pull public.ecr.aws/opensearchproject/opensearch-benchmark:latest`

### image-name (1)

- `eks` (low; dockerfile.name) — [opensearch-project/OpenSearch:test/fixtures/s3-fixture/Dockerfile.eks L1](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/test/fixtures/s3-fixture/Dockerfile.eks#L1): `FROM ubuntu:22.04`

### registry (1)

- `public.ecr.aws/opensearchproject` (low; docs.registry-ref) — [opensearch-project/documentation-website:_migration-assistant/migration-phases/deploy/deploying-to-kubernetes.md L99](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_migration-assistant/migration-phases/deploy/deploying-to-kubernetes.md#L99): `Migration Assistant is published to Amazon Public ECR ('public.ecr.aws/opensearchproject/...'). The Helm chart…`

### release-notes (2)

- `release-notes/opensearch-documentation-release-notes-{{.Version}}.md` (high; paths.versioned-release-notes) — [opensearch-project/documentation-website:release-notes/opensearch-documentation-release-notes-3.9.0.md ](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/release-notes/opensearch-documentation-release-notes-3.9.0.md): `file release-notes/opensearch-documentation-release-notes-3.9.0.md`
- `release-notes/opensearch.release-notes-{{.Version}}.md` (high; paths.versioned-release-notes) — [opensearch-project/OpenSearch:release-notes/opensearch.release-notes-3.9.0.md ](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/release-notes/opensearch.release-notes-3.9.0.md): `file release-notes/opensearch.release-notes-3.9.0.md`

### release-publisher (1)

- `ncipollo/release-action` (high; workflow.release-upload) — [opensearch-project/OpenSearch:.github/workflows/auto-release.yml L20](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/.github/workflows/auto-release.yml#L20): `- uses: ncipollo/release-action@339a81892b84b4eeb0f6e744e4574d79d0d9b8dd # v1`

### release-trigger (2)

- `*` (high; workflow.tag-trigger) — [opensearch-project/OpenSearch:.github/workflows/auto-release.yml L6](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/.github/workflows/auto-release.yml#L6): `- '*'`
- `*.*.*` (high; workflow.tag-trigger) — [opensearch-project/OpenSearch:.github/workflows/version.yml L12](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/.github/workflows/version.yml#L12): `- '*.*.*'`

### security-policy (7)

- `SECURITY.md` (high; docs.security-policy) — [opensearch-project/OpenSearch:SECURITY.md L1](https://github.com/opensearch-project/OpenSearch/blob/3.9.0/SECURITY.md#L1): `## Reporting a Vulnerability`
- `_im-plugin/security.md` (high; docs.security-policy) — [opensearch-project/documentation-website:_im-plugin/security.md L1](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_im-plugin/security.md#L1): `---`
- `_observing-your-data/ad/security.md` (high; docs.security-policy) — [opensearch-project/documentation-website:_observing-your-data/ad/security.md L1](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_observing-your-data/ad/security.md#L1): `---`
- `_observing-your-data/alerting/security.md` (high; docs.security-policy) — [opensearch-project/documentation-website:_observing-your-data/alerting/security.md L1](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_observing-your-data/alerting/security.md#L1): `---`
- `_observing-your-data/forecast/security.md` (high; docs.security-policy) — [opensearch-project/documentation-website:_observing-your-data/forecast/security.md L1](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_observing-your-data/forecast/security.md#L1): `---`
- `_search-plugins/async/security.md` (high; docs.security-policy) — [opensearch-project/documentation-website:_search-plugins/async/security.md L1](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_search-plugins/async/security.md#L1): `---`
- `_security-analytics/security.md` (high; docs.security-policy) — [opensearch-project/documentation-website:_security-analytics/security.md L1](https://github.com/opensearch-project/documentation-website/blob/02e78d5e17bbfde0fe6f2b0d8d87974185b3941d/_security-analytics/security.md#L1): `---`

### tag-scheme (1)

- `^(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (high; tags.ls-remote) — [opensearch-project/OpenSearch/releases/tag/3.9.0 refs/tags/3.9.0](https://github.com/opensearch-project/OpenSearch/releases/tag/3.9.0#refs/tags/3.9.0): `4ee42a94e87f66fbf1e62a9871b1b87f91e02472 refs/tags/3.9.0`
