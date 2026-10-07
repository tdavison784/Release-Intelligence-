# Discovery report: minio

- Repository: `github.com/minio/minio` at `master` (commit `7aac2a2c5b7c…`)
- Generated: 2026-10-02T12:37:06Z
- LLM: not used (deterministic resolver only)
- Tags: 523 tags: prefix "", 0 stable, 0 prereleases (), 523 junk; latest stable ; lineage linear
- Scanned `github.com/minio/minio@master` (source profile): 1364 files listed, 189 read
- Scanned `github.com/minio/docs@main` (docs profile): 765 files listed, 45 read
- Validation: skipped (only 0 stable releases available, 3 required)

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 1 | git-tags github.com/minio/minio |
| release-notes | not-found | 0 |  |
| changelog | not-found | 0 |  |
| helm-charts | found | 3 | minio via helm-repo:https://charts.min.io |
| registries | found | 1 | quay.io/minio |
| images | found | 12 | quay.io/minio/mc:{{.Tag}}; quay.io/minio/minio:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 3 | github-advisories minio/minio |
| version-relations | found | 1 | minio-chart: independent; mc: version = {{.Tag}}; minio: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:mc | inferred | image.tag-assumed-release | https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/helm/minio/values.yaml#L27 |
| artifact:minio | discovered | image.tag-from-release | https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/.github/workflows/mint.yml#L40 |
| artifact:minio-chart | inferred | helm.independent-version | https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/helm/minio/Chart.yaml#L3 (+1) |
| source:advisories | discovered | security.advisories-referenced | https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/SECURITY.md#L36 (+2) |
| source:tags | discovered | versions.git-tags | https://github.com/minio/minio/releases/tag/OFFICIAL.2016-02-08T00-12-28Z#refs/tags/OFFICIAL.2016-02-08T00-12-28Z |
| versioning | discovered | tags.scheme | https://github.com/minio/minio/releases/tag/OFFICIAL.2016-02-08T00-12-28Z#refs/tags/OFFICIAL.2016-02-08T00-12-28Z |

Statuses: 4 discovered, 2 inferred.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: minio
name: minio
homepage: https://github.com/minio/minio
versioning:
  scheme: semver
  lineage: linear
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/minio/minio
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: minio/minio
    notes: 'Security policy: SECURITY.md'
artifacts:
  - id: minio-chart
    type: helm-chart
    name: minio
    version:
      strategy: independent
    channels:
      - kind: helm-repo
        url: https://charts.min.io
        chart: minio
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/minio/minio
          path: helm/minio/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/minio/minio
          path: helm/minio/Chart.yaml
  - id: mc
    type: container-image
    name: mc
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/minio/mc
  - id: minio
    type: container-image
    name: minio
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/minio/minio
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  notes: Proposed by automated discovery; NOT validated against releases (only 0 stable releases available, 3 required). Review before use.
```

## Open questions for the reviewer

- No stable release tags were found; the tag scheme could not be inferred.
- Run the relationship checks (validation) before adopting this definition.
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.
- Ambiguity (chart-version): How do the versions of chart(s) minio relate to the release version?

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 523 tags: prefix "", 0 stable, 0 prereleases (), 523 junk; latest stable ; lineage linear |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (523 tags: prefix "", 0 stable, 0 prereleases (), 523 junk; latest stable ; lineage linear). |
| source:advisories | include | security.advisories-referenced | heuristic | The security policy states that advisories are published as GitHub Security Advisories. |
| artifact:minio-chart | include | helm.independent-version | heuristic | Chart has its own version "5.4.0" unrelated to the release. Published via 1 channel(s); OCI locations first. |
| artifact:mc | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:minio | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |

<details><summary>10 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| image `quay.io/minio/openldap` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `quay.io/coreos/etcd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/minio/dex` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `docker.io/minio/mint` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.min.dev/community/minio` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,unresolved). |
| image-name `cicd` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `hotfix` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `release.old_cpu` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `scratch` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### docs-repo (1)

- `github.com/minio/docs` (medium; docs.docs-repo-ref) — [minio/minio:README.md L24](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/README.md#L24): `Use the [MinIO Documentation](https://github.com/minio/docs) project to build and host a local copy of the doc…`

### helm-chart (1)

- `minio@helm/minio` (high; helm.chart) — [minio/minio:helm/minio/Chart.yaml L3](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/helm/minio/Chart.yaml#L3): `name: minio`

### helm-repo (2)

- `https://charts.min.io` (medium; docs.helm-repo-add) — [minio/minio:helm/minio/README.md L24](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/helm/minio/README.md#L24): `helm repo add minio https://charts.min.io/`
- `https://operator.min.io` (medium; docs.helm-repo-add) — [minio/docs:source/operations/deployments/k8s-deploy-minio-tenant-helm-on-kubernetes.rst L86](https://github.com/minio/docs/blob/35f2bb81280a3573c64947e8bd979e2c7026d2dd/source/operations/deployments/k8s-deploy-minio-tenant-helm-on-kubernetes.rst#L86): `helm repo add minio-operator https://operator.min.io`

### image (8)

- `quay.io/minio/mc` (high; helm.values-image) — [minio/minio:helm/minio/values.yaml L27](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/helm/minio/values.yaml#L27): `repository: quay.io/minio/mc`
- `quay.io/minio/minio` (high; docs.image-ref, helm.values-image, make.image-ref, manifest.image, workflow.image-ref) — [minio/minio:.github/workflows/mint.yml L40](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/.github/workflows/mint.yml#L40): `TAG="quay.io/minio/minio:${{ steps.vars.outputs.sha_short }}" make docker`
- `docker.io/library/nginx` (medium; manifest.image) — [minio/minio:buildscripts/upgrade-tests/compose.yml L48](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/buildscripts/upgrade-tests/compose.yml#L48): `image: nginx:1.19.2-alpine`
- `docker.io/minio/mint` (medium; script.image-ref) — [minio/minio:.github/workflows/run-mint.sh L19](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/.github/workflows/run-mint.sh#L19): `docker pull docker.io/minio/mint:edge`
- `quay.io/coreos/etcd` (medium; workflow.image-ref) — [minio/minio:.github/workflows/iam-integrations.yaml L33](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/.github/workflows/iam-integrations.yaml#L33): `image: "quay.io/coreos/etcd:v3.5.1"`
- `quay.io/minio/dex` (medium; workflow.image-ref) — [minio/minio:.github/workflows/iam-integrations.yaml L45](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/.github/workflows/iam-integrations.yaml#L45): `image: quay.io/minio/dex`
- `quay.io/minio/openldap` (medium; manifest.image, workflow.image-ref) — [minio/minio:.github/workflows/iam-integrations.yaml L24](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/.github/workflows/iam-integrations.yaml#L24): `image: quay.io/minio/openldap`
- `registry.min.dev/community/minio` (medium; script.image-ref) — [minio/minio:docker-buildx.sh L59](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/docker-buildx.sh#L59): `-t "registry.min.dev/community/minio:latest" \`

### image-name (4)

- `cicd` (low; dockerfile.name) — [minio/minio:Dockerfile.cicd L1](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/Dockerfile.cicd#L1): `FROM minio/minio:edge`
- `hotfix` (low; dockerfile.name) — [minio/minio:Dockerfile.hotfix L1](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/Dockerfile.hotfix#L1): `FROM golang:1.24-alpine as build`
- `release.old_cpu` (low; dockerfile.name) — [minio/minio:Dockerfile.release.old_cpu L1](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/Dockerfile.release.old_cpu#L1): `FROM golang:1.24-alpine AS build`
- `scratch` (low; dockerfile.name) — [minio/minio:Dockerfile.scratch L1](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/Dockerfile.scratch#L1): `FROM scratch`

### registry (1)

- `quay.io/minio` (low; make.registry-ref) — [minio/minio:Makefile L9](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/Makefile#L9): `REPO ?= quay.io/minio`

### security-advisories (1)

- `minio/minio` (high; docs.security-advisories-link) — [minio/minio:SECURITY.md L36](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/SECURITY.md#L36): `5. On the date that the fixes are applied a security advisory will be published on <https://blog.min.io>.`

### security-policy (2)

- `SECURITY.md` (high; docs.security-policy) — [minio/minio:SECURITY.md L1](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/SECURITY.md#L1): `# Security Policy`
- `source/operations/checklists/security.rst` (high; docs.security-policy) — [minio/docs:source/operations/checklists/security.rst L1](https://github.com/minio/docs/blob/35f2bb81280a3573c64947e8bd979e2c7026d2dd/source/operations/checklists/security.rst#L1): `.. _minio-security-checklist:`

### tag-scheme (1)

- `^(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (low; tags.ls-remote) — [minio/minio/releases/tag/OFFICIAL.2016-02-08T00-12-28Z refs/tags/OFFICIAL.2016-02-08T00-12-28Z](https://github.com/minio/minio/releases/tag/OFFICIAL.2016-02-08T00-12-28Z#refs/tags/OFFICIAL.2016-02-08T00-12-28Z): `e79a73a3f5fa9b0111ecf2620be84ff5f3e9698a refs/tags/OFFICIAL.2016-02-08T00-12-28Z`

### version-relation (1)

- `build.VERSION = {{.Tag}}` (high; make.version-from-git) — [minio/minio:Makefile L8](https://github.com/minio/minio/blob/7aac2a2c5b7c882e68c1ce017d8256be2feea27f/Makefile#L8): `VERSION ?= $(shell git describe --tags)`
