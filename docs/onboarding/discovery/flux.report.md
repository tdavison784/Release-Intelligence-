# Discovery report: flux2

- Repository: `github.com/fluxcd/flux2` at `v2.9.6` (commit `b9c17924adb5…`)
- Generated: 2026-10-01T14:43:28Z
- LLM: not used (deterministic resolver only)
- Tags: 216 tags: prefix "v", 206 stable, 10 prereleases (alpha.N×1, beta.N×4, rc.N×5), 0 junk; latest stable v2.9.6; lineage minor
- Scanned `github.com/fluxcd/flux2@v2.9.6` (source profile): 748 files listed, 263 read
- Scanned `github.com/fluxcd/website@main` (docs profile): 837 files listed, 68 read
- Validation: done against v2.7.0, v2.7.5, v2.8.0, v2.8.8, v2.9.0, v2.9.6

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 15 | git-tags github.com/fluxcd/flux2; github-releases fluxcd/flux2 |
| release-notes | found | 1 | github-releases fluxcd/flux2  @{{.Tag}} |
| changelog | not-found | 0 |  |
| helm-charts | candidates-only | 24 |  |
| registries | found | 10 | docker.io/flux-system; docker.io/fluxcd; ghcr.io/flux-system; ghcr.io/fluxcd |
| images | found | 32 | ghcr.io/fluxcd/flux-manifests:{{.Tag}}; ghcr.io/flux-system/gotk-components.yaml:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories fluxcd/flux2 |
| version-relations | found | 0 | flux-sbom-spdx-manifest: version = {{.Tag}}; flux-manifests: version = {{.Tag}}; gotk-components-yaml: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:flux-manifests | historically-validated | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L160 (+1) |
| artifact:flux-sbom-spdx-manifest | historically-validated | asset.hosted-release-download | https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/blog/2022-02-09-security-the-value-of-sboms/index.md#L77 |
| artifact:binary | unverified | asset.hosted-release-download | https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33 |
| artifact:gotk-components-yaml | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L134 (+1) |
| artifact:helm-controller | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L23 |
| artifact:image-automation-controller | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L29 |
| artifact:image-reflector-controller | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L27 |
| artifact:kustomize-controller | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L21 |
| artifact:notification-controller | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L25 |
| artifact:source-controller | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L19 |
| artifact:source-watcher | unverified | image.tag-assumed-release | https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L31 |
| source:advisories | discovered | security.github-hosted-default |  |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L75 (+2) |
| source:release-notes | unverified | notes.generated-release-body | https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L2 |
| source:tags | discovered | versions.git-tags | https://github.com/fluxcd/flux2/releases/tag/v2.9.6#refs/tags/v2.9.6 |
| versioning | discovered | tags.scheme | https://github.com/fluxcd/flux2/releases/tag/v2.9.6#refs/tags/v2.9.6 (+1) |

Statuses: 2 historically-validated, 4 discovered, 10 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: flux
name: flux2
homepage: https://github.com/fluxcd/flux2
versioning:
  scheme: semver
  tagPrefix: v
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/fluxcd/flux2
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: fluxcd/flux2
    priority: 1
  - id: release-notes
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: fluxcd/flux2
      ref: '{{.Tag}}'
    extract:
      type: whole
    notes: 'GitHub release body generated by goreleaser (groups: ) Unverified by discovery (unverifiable: 2.7.0 unverifiable (fetch https://api.github.com/repos/fluxcd/flux2/releases/tags/v2.7.0: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); 2.7.5 unverifiable (fetch https://api.github.com/repos/fluxcd/flux2/releases/tags/v2.7.5: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); 2.8.0 unverifiable (fetch https://api.github.com/repos/fluxcd/flux2/releases/tags/v2.8.0: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); 2.8.8 unverifiable (fetch https://api.github.com/repos/fluxcd/flux2/releases/tags/v2.8.8: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); …).'
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: fluxcd/flux2
artifacts:
  - id: flux-sbom-spdx-manifest
    type: manifest
    name: flux_{{.Version}}_sbom.spdx.json
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: http
        url: https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/flux_{{.Version}}_sbom.spdx.json
    contents:
      - kind: image-refs
    validatedAgainst:
      - 2.7.0
      - 2.7.5
      - 2.8.0
      - 2.8.8
      - 2.9.0
      - 2.9.6
  - id: flux-manifests
    type: container-image
    name: flux-manifests
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: ghcr.io/fluxcd/flux-manifests
      - kind: oci
        repository: docker.io/fluxcd/flux-manifests
    validatedAgainst:
      - 2.7.0
      - 2.7.5
      - 2.8.0
      - 2.8.8
      - 2.9.0
      - 2.9.6
  - id: gotk-components-yaml
    type: container-image
    name: gotk-components.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: ghcr.io/flux-system/gotk-components.yaml
      - kind: oci
        repository: docker.io/flux-system/gotk-components.yaml
    notes: 'Unverified by discovery (unverifiable: 2.7.0 unverifiable (not verifiable: ghcr.io/flux-system/gotk-components.yaml:v2.7.0 (oci: unavailable), docker.io/flux-system/gotk-components.yaml:v2.7.0 (oci: unavailable)); 2.7.5 unverifiable (not verifiable: ghcr.io/flux-system/gotk-components.yaml:v2.7.5 (oci: unavailable), docker.io/flux-system/gotk-components.yaml:v2.7.5 (oci: unavailable)); 2.8.0 unverifiable (not verifiable: ghcr.io/flux-system/gotk-components.yaml:v2.8.0 (oci: unavailable), docker.io/flux-system/gotk-components.yaml:v2.8.0 (oci: unavailable)); 2.8.8 unverifiable (not verifiable: ghcr.io/flux-system/gotk-components.yaml:v2.8.8 (oci: unavailable), docker.io/flux-system/gotk-components.yaml:v2.8.8 (oci: unavailable)); …).'
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - v2.7.0
    - v2.7.5
    - v2.8.0
    - v2.8.8
    - v2.9.0
    - v2.9.6
  notes: Proposed by automated discovery; relationship checks run against v2.7.0, v2.7.5, v2.8.0, v2.8.8, v2.9.0, v2.9.6.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 11 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no upgrade-guide, compatibility source was derived from it.

## Validation matrix

| Element | Verdict | v2.7.0 | v2.7.5 | v2.8.0 | v2.8.8 | v2.9.0 | v2.9.6 |
|---|---|---|---|---|---|---|---|
| artifact:binary | unverifiable |  |  |  |  |  |  |
| artifact:flux-manifests | validated |  |  |  |  |  |  |
| artifact:flux-sbom-spdx-manifest | validated |  |  |  |  |  |  |
| artifact:gotk-components-yaml | unverifiable |  |  |  |  |  |  |
| artifact:helm-controller | failing |  |  |  |  |  |  |
| artifact:image-automation-controller | failing |  |  |  |  |  |  |
| artifact:image-reflector-controller | failing |  |  |  |  |  |  |
| artifact:kustomize-controller | failing |  |  |  |  |  |  |
| artifact:notification-controller | failing |  |  |  |  |  |  |
| artifact:source-controller | failing |  |  |  |  |  |  |
| artifact:source-watcher | failing |  |  |  |  |  |  |
| content:flux-sbom-spdx-manifest/image-refs | validated |  |  |  |  |  |  |
| source:release-notes | unverifiable |  |  |  |  |  |  |

## Dropped elements

- `artifact:helm-controller` (deterministic): failed validation: 2.7.0 fail (absent from ghcr.io/fluxcd/helm-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/helm-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/helm-controller:v2.8.0); 2.8.8 fail (absent from ghcr.io/fluxcd/helm-controller:v2.8.8); …
- `artifact:image-automation-controller` (deterministic): failed validation: 2.7.0 fail (absent from ghcr.io/fluxcd/image-automation-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/image-automation-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/image-automation-controller:v2.8.0); 2.8.8 fail (absent from ghcr.io/fluxcd/image-automation-controller:v2.8.8); …
- `artifact:image-reflector-controller` (deterministic): failed validation: 2.7.0 fail (absent from ghcr.io/fluxcd/image-reflector-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/image-reflector-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/image-reflector-controller:v2.8.0); 2.8.8 fail (absent from ghcr.io/fluxcd/image-reflector-controller:v2.8.8); …
- `artifact:kustomize-controller` (deterministic): failed validation: 2.7.0 fail (absent from ghcr.io/fluxcd/kustomize-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/kustomize-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/kustomize-controller:v2.8.0); 2.8.8 fail (absent from ghcr.io/fluxcd/kustomize-controller:v2.8.8); …
- `artifact:notification-controller` (deterministic): failed validation: 2.7.0 fail (absent from ghcr.io/fluxcd/notification-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/notification-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/notification-controller:v2.8.0); 2.8.8 fail (absent from ghcr.io/fluxcd/notification-controller:v2.8.8); …
- `artifact:source-controller` (deterministic): failed validation: 2.7.0 fail (absent from ghcr.io/fluxcd/source-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/source-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/source-controller:v2.8.0); 2.8.8 fail (absent from ghcr.io/fluxcd/source-controller:v2.8.8); …
- `artifact:source-watcher` (deterministic): failed validation: 2.7.0 fail (absent from ghcr.io/fluxcd/source-watcher:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/source-watcher:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/source-watcher:v2.8.0); 2.8.8 fail (absent from ghcr.io/fluxcd/source-watcher:v2.8.8); …
- `artifact:binary` (deterministic): static validation: error: artifacts[1].channels[0]: template "https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_amd64.tar.gz": template: :1:62: executing "" at <.Binary>: can't evaluate field Binary in type catalog.RenderContext

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 216 tags: prefix "v", 206 stable, 10 prereleases (alpha.N×1, beta.N×4, rc.N×5), 0 junk; latest stable v2.9.6; lineage minor |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (216 tags: prefix "v", 206 stable, 10 prereleases (alpha.N×1, beta.N×4, rc.N×5), 0 junk; latest stable v2.9.6; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes | include | notes.generated-release-body | heuristic | The release pipeline generates the hosted release body from commit/PR titles. |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:flux-sbom-spdx-manifest | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (7 reference(s)). |
| artifact:flux-manifests | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 2 registries; ordered by evidence strength. |
| artifact:gotk-components-yaml | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 2 registries; ordered by evidence strength. |
| artifact:helm-controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:image-automation-controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:image-reflector-controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:kustomize-controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:notification-controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:source-controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:source-watcher | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| source:release-notes | annotate | validate.unverifiable | computed | Kept unverified: 2.7.0 unverifiable (fetch https://api.github.com/repos/fluxcd/flux2/releases/tags/v2.7.0: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here's the good news: Authenti… |
| artifact:binary | annotate | validate.unverifiable | computed | Kept unverified: 2.7.0 unverifiable (not verifiable: http channel (http: error)); 2.7.5 unverifiable (not verifiable: http channel (http: error)); 2.8.0 unverifiable (not verifiable: http channel (http: error)); 2.8.8 un… |
| artifact:gotk-components-yaml | annotate | validate.unverifiable | computed | Kept unverified: 2.7.0 unverifiable (not verifiable: ghcr.io/flux-system/gotk-components.yaml:v2.7.0 (oci: unavailable), docker.io/flux-system/gotk-components.yaml:v2.7.0 (oci: unavailable)); 2.7.5 unverifiable (not veri… |
| artifact:helm-controller | drop | validate.failed | computed | Relationship check failed: 2.7.0 fail (absent from ghcr.io/fluxcd/helm-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/helm-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/helm-controller:v2.8.0); 2… |
| artifact:image-automation-controller | drop | validate.failed | computed | Relationship check failed: 2.7.0 fail (absent from ghcr.io/fluxcd/image-automation-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/image-automation-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/im… |
| artifact:image-reflector-controller | drop | validate.failed | computed | Relationship check failed: 2.7.0 fail (absent from ghcr.io/fluxcd/image-reflector-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/image-reflector-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/imag… |
| artifact:kustomize-controller | drop | validate.failed | computed | Relationship check failed: 2.7.0 fail (absent from ghcr.io/fluxcd/kustomize-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/kustomize-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/kustomize-contro… |
| artifact:notification-controller | drop | validate.failed | computed | Relationship check failed: 2.7.0 fail (absent from ghcr.io/fluxcd/notification-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/notification-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/notificati… |
| artifact:source-controller | drop | validate.failed | computed | Relationship check failed: 2.7.0 fail (absent from ghcr.io/fluxcd/source-controller:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/source-controller:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/source-controller:v2.8… |
| artifact:source-watcher | drop | validate.failed | computed | Relationship check failed: 2.7.0 fail (absent from ghcr.io/fluxcd/source-watcher:v2.7.0); 2.7.5 fail (absent from ghcr.io/fluxcd/source-watcher:v2.7.5); 2.8.0 fail (absent from ghcr.io/fluxcd/source-watcher:v2.8.0); 2.8.… |
| artifact:binary | drop | proposer.static-validation | computed | error: artifacts[1].channels[0]: template "https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_amd64.tar.gz": template: :1:62: executing "" at <.Binary>: can't evaluate field Binar… |

<details><summary>26 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| chart-repo `github.com/fluxcd/helm-controller` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| chart-repo `github.com/fluxcd/helm-operator` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| chart-repo `github.com/fluxcd/flux2-kustomize-helm-example` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| crd `cmd/flux/testdata/cluster_info/gitrepositories.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `internal/utils/testdata/components-with-crds.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| image `ghcr.io/fluxcd/kindest/node` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/fluxcd/flux-cli` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/stefanprodan/podinfo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/test/podinfo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/fluxcd/website` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/flagger` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/flagger-manifest` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/fluxcd/flux-cli` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/fluxcd/source-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/source-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/fluxcd/kustomize-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/kustomize-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/fluxcd/notification-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/notification-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/fluxcd/helm-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/helm-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/fluxcd/image-reflector-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/image-reflector-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/fluxcd/image-automation-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/fluxcd/image-automation-contoller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |

</details>

## Candidates


### build-tool (2)

- `cosign` (high; workflow.cosign) — [fluxcd/flux2:.github/workflows/release.yaml L41](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L41): `uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2`
- `goreleaser` (high; goreleaser.config, workflow.goreleaser) — [fluxcd/flux2:.github/workflows/release.yaml L75](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L75): `uses: goreleaser/goreleaser-action@5daf1e915a5f0af01ddbcd89a43b8061ff4f1a89 # v7.2.2`

### chart-repo (3)

- `github.com/fluxcd/flux2-kustomize-helm-example` (medium; docs.chart-repo-ref) — [fluxcd/website:content/en/flux/use-cases/gh-actions-helm-promotion.md L29](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/use-cases/gh-actions-helm-promotion.md#L29): `[helm example repository](https://github.com/fluxcd/flux2-kustomize-helm-example)`
- `github.com/fluxcd/helm-controller` (medium; docs.chart-repo-ref) — [fluxcd/flux2:CONTRIBUTING.md L20](https://github.com/fluxcd/flux2/blob/v2.9.6/CONTRIBUTING.md#L20): `- [fluxcd/helm-controller](https://github.com/fluxcd/helm-controller): Kubernetes operator for lifecycle manag…`
- `github.com/fluxcd/helm-operator` (medium; docs.chart-repo-ref) — [fluxcd/website:content/en/flux/migration/flux-v1-migration.md L71](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/migration/flux-v1-migration.md#L71): `Flux v1 repository to the bootstrap one.  Typically deleting Flux v1 can be done by deleting these helm instal…`

### crd (2)

- `cmd/flux/testdata/cluster_info/gitrepositories.yaml` (low; crd.file) — [fluxcd/flux2:cmd/flux/testdata/cluster_info/gitrepositories.yaml L3](https://github.com/fluxcd/flux2/blob/v2.9.6/cmd/flux/testdata/cluster_info/gitrepositories.yaml#L3): `kind: CustomResourceDefinition`
- `internal/utils/testdata/components-with-crds.yaml` (low; crd.file) — [fluxcd/flux2:internal/utils/testdata/components-with-crds.yaml L8](https://github.com/fluxcd/flux2/blob/v2.9.6/internal/utils/testdata/components-with-crds.yaml#L8): `kind: CustomResourceDefinition`

### docs-repo (1)

- `github.com/fluxcd/website` (medium; docs.docs-repo-ref) — [fluxcd/flux2:CONTRIBUTING.md L24](https://github.com/fluxcd/flux2/blob/v2.9.6/CONTRIBUTING.md#L24): `- [fluxcd/website](https://github.com/fluxcd/website): The Flux documentation website accessible at <https://f…`

### helm-oci (17)

- `docker.io/fluxcd/flux-manifests` (high; workflow.oci-ref) — [fluxcd/flux2:.github/workflows/release.yaml L149](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L149): `oci://docker.io/fluxcd/flux-manifests:${{ steps.prep.outputs.version }} \`
- `ghcr.io/fluxcd/flux-manifests` (high; workflow.oci-ref) — [fluxcd/flux2:.github/workflows/release.yaml L137](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L137): `oci://ghcr.io/fluxcd/flux-manifests:${{ steps.prep.outputs.version }} \`
- `docker.io/org/app-config` (medium; docs.oci-ref) — [fluxcd/flux2:rfcs/0003-kubernetes-oci/README.md L48](https://github.com/fluxcd/flux2/blob/v2.9.6/rfcs/0003-kubernetes-oci/README.md#L48): `flux push artifact oci://docker.io/org/app-config:v1.0.0 \`
- `ghcr.io/controlplaneio-fluxcd/charts/flux-operator` (medium; docs.oci-ref) — [fluxcd/website:content/en/flux/installation/_index.md L186](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/_index.md#L186): `helm install flux-operator oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator \`
- `ghcr.io/controlplaneio-fluxcd/flux-operator-manifests` (medium; docs.oci-ref) — [fluxcd/website:content/en/flux/installation/_index.md L213](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/_index.md#L213): `artifact: "oci://ghcr.io/controlplaneio-fluxcd/flux-operator-manifests"`
- `ghcr.io/fluxcd-community/charts/flux2` (medium; docs.oci-ref) — [fluxcd/website:content/en/flux/installation/_index.md L281](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/_index.md#L281): `helm install -n flux-system --create-namespace flux oci://ghcr.io/fluxcd-community/charts/flux2`
- `ghcr.io/fluxcd/charts` (medium; docs.oci-ref) — [fluxcd/website:content/en/flagger/install/flagger-install-with-flux.md L51](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-with-flux.md#L51): `url: oci://ghcr.io/fluxcd/charts`
- `ghcr.io/fluxcd/charts/flagger` (medium; docs.oci-ref) — [fluxcd/website:content/en/flagger/install/flagger-install-with-flux.md L19](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-with-flux.md#L19): `- 'ghcr.io/fluxcd/charts/flagger:<version>' Helm charts`
- `ghcr.io/fluxcd/flagger-manifests` (medium; docs.oci-ref) — [fluxcd/website:content/en/flagger/install/flagger-install-with-flux.md L121](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-with-flux.md#L121): `url: oci://ghcr.io/fluxcd/flagger-manifests`
- `ghcr.io/my-org/charts` (medium; docs.oci-ref) — [fluxcd/flux2:rfcs/0002-helm-oci/README.md L166](https://github.com/fluxcd/flux2/blob/v2.9.6/rfcs/0002-helm-oci/README.md#L166): `url: oci://ghcr.io/my-org/charts/`
- `ghcr.io/my-org/charts/my-app` (medium; docs.oci-ref) — [fluxcd/flux2:rfcs/0002-helm-oci/README.md L209](https://github.com/fluxcd/flux2/blob/v2.9.6/rfcs/0002-helm-oci/README.md#L209): `image: ghcr.io/my-org/charts/my-app`
- `ghcr.io/my-org/my-fleet-manifests` (medium; docs.oci-ref) — [fluxcd/website:content/en/flux/installation/_index.md L244](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/_index.md#L244): `url: "oci://ghcr.io/my-org/my-fleet-manifests"`
- `ghcr.io/org/my-app-config` (medium; docs.oci-ref) — [fluxcd/flux2:rfcs/0003-kubernetes-oci/README.md L301](https://github.com/fluxcd/flux2/blob/v2.9.6/rfcs/0003-kubernetes-oci/README.md#L301): `flux push artifact oci://ghcr.io/org/my-app-config:v1.0.0 \`
- `ghcr.io/prometheus-community/charts` (medium; docs.oci-ref) — [fluxcd/website:content/en/blog/2022-11-11-prove-the-authenticity-of-helm-charts/index.md L110](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/blog/2022-11-11-prove-the-authenticity-of-helm-charts/index.md#L110): `--url=oci://ghcr.io/prometheus-community/charts \`
- `ghcr.io/stefanprodan/charts` (medium; docs.oci-ref) — [fluxcd/website:content/en/blog/2022-11-11-prove-the-authenticity-of-helm-charts/index.md L40](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/blog/2022-11-11-prove-the-authenticity-of-helm-charts/index.md#L40): `url: oci://ghcr.io/stefanprodan/charts`
- `ghcr.io/stefanprodan/charts/podinfo` (medium; docs.oci-ref) — [fluxcd/website:content/en/flux/guides/helmreleases.md L173](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/guides/helmreleases.md#L173): `url: oci://ghcr.io/stefanprodan/charts/podinfo`
- `ghcr.io/stefanprodan/manifests/podinfo` (medium; docs.oci-ref, workflow.oci-ref) — [fluxcd/flux2:.github/workflows/e2e.yaml L187](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/e2e.yaml#L187): `--url oci://ghcr.io/stefanprodan/manifests/podinfo \`

### helm-repo (4)

- `https://aws.github.io/eks-charts` (medium; docs.helm-repo-add) — [fluxcd/website:content/en/flagger/install/flagger-install-on-eks-appmesh.md L63](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-on-eks-appmesh.md#L63): `helm repo add eks https://aws.github.io/eks-charts`
- `https://charts.jetstack.io` (medium; docs.helm-repo-add) — [fluxcd/website:content/en/flagger/install/flagger-install-on-google-cloud.md L201](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-on-google-cloud.md#L201): `helm repo add jetstack https://charts.jetstack.io && \`
- `https://flagger.app` (medium; docs.helm-repo-add) — [fluxcd/website:content/en/flagger/install/flagger-install-on-alibaba-servicemesh.md L29](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-on-alibaba-servicemesh.md#L29): `helm repo add flagger https://flagger.app`
- `https://helm.traefik.io/traefik` (medium; docs.helm-repo-add) — [fluxcd/website:content/en/flux/use-cases/helm.md L43](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/use-cases/helm.md#L43): `helm repo add traefik https://helm.traefik.io/traefik`

### image (32)

- `docker.io/flux-system/gotk-components.yaml` (high; workflow.image-ref) — [fluxcd/flux2:.github/workflows/release.yaml L146](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L146): `--export > ./docker.io/flux-system/gotk-components.yaml`
- `docker.io/fluxcd/flux-manifests` (high; workflow.image-ref) — [fluxcd/flux2:.github/workflows/release.yaml L161](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L161): `cosign sign --yes docker.io/fluxcd/flux-manifests:${{ steps.prep.outputs.version }}`
- `ghcr.io/flux-system/gotk-components.yaml` (high; workflow.image-ref) — [fluxcd/flux2:.github/workflows/release.yaml L134](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L134): `--export > ./ghcr.io/flux-system/gotk-components.yaml`
- `ghcr.io/fluxcd/flux-cli` (high; docs.image-ref, goreleaser.image) — [fluxcd/flux2:.goreleaser.yml L94](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L94): `- 'ghcr.io/fluxcd/flux-cli:{{ .Tag }}-amd64'`
- `ghcr.io/fluxcd/flux-manifests` (high; workflow.image-ref) — [fluxcd/flux2:.github/workflows/release.yaml L160](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L160): `cosign sign --yes ghcr.io/fluxcd/flux-manifests:${{ steps.prep.outputs.version }}`
- `ghcr.io/fluxcd/helm-controller` (high; docs.image-ref, kustomize.image) — [fluxcd/flux2:manifests/install/kustomization.yaml L23](https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L23): `newName: ghcr.io/fluxcd/helm-controller`
- `ghcr.io/fluxcd/image-automation-controller` (high; docs.image-ref, kustomize.image) — [fluxcd/flux2:manifests/install/kustomization.yaml L29](https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L29): `newName: ghcr.io/fluxcd/image-automation-controller`
- `ghcr.io/fluxcd/image-reflector-controller` (high; docs.image-ref, kustomize.image) — [fluxcd/flux2:manifests/install/kustomization.yaml L27](https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L27): `newName: ghcr.io/fluxcd/image-reflector-controller`
- `ghcr.io/fluxcd/kustomize-controller` (high; docs.image-ref, kustomize.image) — [fluxcd/flux2:manifests/install/kustomization.yaml L21](https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L21): `newName: ghcr.io/fluxcd/kustomize-controller`
- `ghcr.io/fluxcd/notification-controller` (high; docs.image-ref, kustomize.image) — [fluxcd/flux2:manifests/install/kustomization.yaml L25](https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L25): `newName: ghcr.io/fluxcd/notification-controller`
- `ghcr.io/fluxcd/source-controller` (high; docs.image-ref, kustomize.image) — [fluxcd/flux2:manifests/install/kustomization.yaml L19](https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L19): `newName: ghcr.io/fluxcd/source-controller`
- `ghcr.io/fluxcd/source-watcher` (high; kustomize.image) — [fluxcd/flux2:manifests/install/kustomization.yaml L31](https://github.com/fluxcd/flux2/blob/v2.9.6/manifests/install/kustomization.yaml#L31): `newName: ghcr.io/fluxcd/source-watcher`
- `docker.io/fluxcd/flux-cli` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/installation/_index.md L110](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/_index.md#L110): `* 'docker.io/fluxcd/flux-cli:<version>'`
- `docker.io/fluxcd/helm-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L148](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L148): `| [helm-controller](https://github.com/fluxcd/helm-controller)                         | 'docker.io/fluxcd/hel…`
- `docker.io/fluxcd/image-automation-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L150](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L150): `| [image-automation-controller](https://github.com/fluxcd/image-automation-controller) | 'docker.io/fluxcd/ima…`
- `docker.io/fluxcd/image-reflector-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L149](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L149): `| [image-reflector-controller](https://github.com/fluxcd/image-reflector-controller)   | 'docker.io/fluxcd/ima…`
- `docker.io/fluxcd/kustomize-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L146](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L146): `| [kustomize-controller](https://github.com/fluxcd/kustomize-controller)               | 'docker.io/fluxcd/kus…`
- `docker.io/fluxcd/notification-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L147](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L147): `| [notification-controller](https://github.com/fluxcd/notification-controller)         | 'docker.io/fluxcd/not…`
- `docker.io/fluxcd/source-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L145](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L145): `| [source-controller](https://github.com/fluxcd/source-controller)                     | 'docker.io/fluxcd/sou…`
- `docker.io/fluxcd/website` (medium; docs.image-ref) — [fluxcd/website:README.md L90](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/README.md#L90): `(For maintainers) Using a machine with 'docker' and logged in with an account that has permission to push to '…`
- `ghcr.io/fluxcd/flagger` (medium; docs.image-ref) — [fluxcd/website:content/en/flagger/install/flagger-install-with-flux.md L17](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-with-flux.md#L17): `- 'ghcr.io/fluxcd/flagger:<version>' multi-arch container images`
- `ghcr.io/fluxcd/flagger-manifest` (medium; docs.image-ref) — [fluxcd/website:content/en/flagger/install/flagger-install-with-flux.md L18](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flagger/install/flagger-install-with-flux.md#L18): `- 'ghcr.io/fluxcd/flagger-manifest:<version>' Kubernetes manifests`
- `ghcr.io/fluxcd/helm-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L148](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L148): `| [helm-controller](https://github.com/fluxcd/helm-controller)                         | 'docker.io/fluxcd/hel…`
- `ghcr.io/fluxcd/image-automation-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L150](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L150): `| [image-automation-controller](https://github.com/fluxcd/image-automation-controller) | 'docker.io/fluxcd/ima…`
- `ghcr.io/fluxcd/image-reflector-contoller` (medium; docs.image-ref) — [fluxcd/website:content/en/flux/security/slsa-assessment.md L149](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/security/slsa-assessment.md#L149): `| [image-reflector-controller](https://github.com/fluxcd/image-reflector-controller)   | 'docker.io/fluxcd/ima…`
- … 7 more

### manifest (1)

- `manifests/openshift/scc.yaml` (medium; docs.raw-url) — [fluxcd/website:content/en/flux/installation/configuration/openshift.md L13](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/configuration/openshift.md#L13): `First copy the [scc.yaml](https://raw.githubusercontent.com/fluxcd/flux2/main/manifests/openshift/scc.yaml)`

### registry (10)

- `docker.io` (high; workflow.registry-login) — [fluxcd/flux2:.github/workflows/release.yaml L53](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L53): `uses: docker/login-action@650006c6eb7dba73a995cc03b0b2d7f5ca915bee # v4.2.0`
- `ghcr.io` (high; workflow.registry-login) — [fluxcd/flux2:.github/workflows/release.yaml L49](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L49): `registry: ghcr.io`
- `docker.io/flux-system` (low; workflow.registry-ref) — [fluxcd/flux2:.github/workflows/release.yaml L143](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L143): `mkdir -p ./docker.io/flux-system`
- `docker.io/fluxcd` (low; workflow.registry-ref) — [fluxcd/flux2:.github/workflows/release.yaml L144](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L144): `flux install --registry=docker.io/fluxcd \`
- `ghcr.io/flux-system` (low; workflow.registry-ref) — [fluxcd/flux2:.github/workflows/release.yaml L131](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L131): `mkdir -p ./ghcr.io/flux-system`
- `ghcr.io/fluxcd` (low; docs.registry-ref, workflow.registry-ref) — [fluxcd/flux2:.github/workflows/release.yaml L132](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L132): `flux install --registry=ghcr.io/fluxcd \`
- `registry.internal/fluxcd` (low; docs.registry-ref) — [fluxcd/website:content/en/flux/installation/configuration/air-gapped.md L47](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/configuration/air-gapped.md#L47): `--registry=registry.internal/fluxcd \`
- `registry.terraform.io/providers/fluxcd/flux` (low; docs.registry-ref) — [fluxcd/website:content/en/flux/installation/_index.md L163](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/_index.md#L163): `[registry.terraform.io](https://registry.terraform.io/providers/fluxcd/flux).`
- `registry.terraform.io/providers/fluxcd/flux/latest/docs/resources/bootstrap_git` (low; docs.registry-ref) — [fluxcd/website:content/en/flux/installation/_index.md L166](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/flux/installation/_index.md#L166): `[flux_bootstrap_git](https://registry.terraform.io/providers/fluxcd/flux/latest/docs/resources/bootstrap_git)`
- `us-central1-docker.pkg.dev` (low; workflow.registry-login) — [fluxcd/flux2:.github/workflows/e2e-gcp.yaml L65](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/e2e-gcp.yaml#L65): `registry: us-central1-docker.pkg.dev`

### release-asset (11)

- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_darwin_amd64.tar.gz` (high; goreleaser.archive) — [fluxcd/flux2:.goreleaser.yml L33](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33): `- name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_darwin_arm64.tar.gz` (high; goreleaser.archive) — [fluxcd/flux2:.goreleaser.yml L33](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33): `- name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_amd64.tar.gz` (high; goreleaser.archive) — [fluxcd/flux2:.goreleaser.yml L33](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33): `- name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_arm.tar.gz` (high; goreleaser.archive) — [fluxcd/flux2:.goreleaser.yml L33](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33): `- name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_linux_arm64.tar.gz` (high; goreleaser.archive) — [fluxcd/flux2:.goreleaser.yml L33](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33): `- name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_windows_amd64.zip` (high; goreleaser.archive) — [fluxcd/flux2:.goreleaser.yml L33](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33): `- name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/{{ .Binary }}_{{.Version}}_windows_arm64.zip` (high; goreleaser.archive) — [fluxcd/flux2:.goreleaser.yml L33](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L33): `- name_template: "{{ .Binary }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/fluxcd/flux2/releases/download/{{.Tag}}/flux_{{.Version}}_sbom.spdx.json` (medium; docs.release-asset-url) — [fluxcd/website:content/en/blog/2022-02-09-security-the-value-of-sboms/index.md L77](https://github.com/fluxcd/website/blob/643963b610d8285885b5cd01991489b32bfff834/content/en/blog/2022-02-09-security-the-value-of-sboms/index.md#L77): `curl -sL https://github.com/fluxcd/flux2/releases/download/v0.25.3/flux_0.25.3_sbom.spdx.json | jq`
- `https://github.com/fluxcd/flux2/releases/download/v${VERSION_FLUX}/flux2_${VERSION_FLUX}_checksums.txt` (low; script.release-asset-url) — [fluxcd/flux2:install/flux.sh L170](https://github.com/fluxcd/flux2/blob/v2.9.6/install/flux.sh#L170): `HASH_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION_FLUX}/flux2_${VERSION_FLUX}_checksums…`
- `https://github.com/fluxcd/flux2/releases/download/v${VERSION_FLUX}/flux_${VERSION_FLUX}_checksums.txt` (low; script.release-asset-url) — [fluxcd/flux2:install/flux.sh L165](https://github.com/fluxcd/flux2/blob/v2.9.6/install/flux.sh#L165): `HASH_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION_FLUX}/flux_${VERSION_FLUX}_checksums.…`
- `https://github.com/fluxcd/flux2/releases/download/v${VERSION_FLUX}/flux_${VERSION_FLUX}_darwin_arm.tar.gz` (low; script.release-asset-url) — [fluxcd/flux2:install/flux.sh L182](https://github.com/fluxcd/flux2/blob/v2.9.6/install/flux.sh#L182): `BIN_URL="https://github.com/${GITHUB_REPO}/releases/download/v${VERSION_FLUX}/flux_${VERSION_FLUX}_${OS}_${ARC…`

### release-notes (1)

- `github-release-body` (high; goreleaser.changelog) — [fluxcd/flux2:.goreleaser.yml L2](https://github.com/fluxcd/flux2/blob/v2.9.6/.goreleaser.yml#L2): `changelog:`

### release-publisher (2)

- `goreleaser` (high; goreleaser.release, workflow.goreleaser) — [fluxcd/flux2:.github/workflows/release.yaml L75](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L75): `uses: goreleaser/goreleaser-action@5daf1e915a5f0af01ddbcd89a43b8061ff4f1a89 # v7.2.2`
- `upload-assets: true` (high; workflow.release-upload) — [fluxcd/flux2:.github/workflows/release.yaml L180](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L180): `upload-assets: true`

### release-trigger (1)

- `v*` (high; workflow.tag-trigger) — [fluxcd/flux2:.github/workflows/release.yaml L5](https://github.com/fluxcd/flux2/blob/v2.9.6/.github/workflows/release.yaml#L5): `tags: ["v*"]`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (high; tags.ls-remote) — [fluxcd/flux2/releases/tag/v2.9.6 refs/tags/v2.9.6](https://github.com/fluxcd/flux2/releases/tag/v2.9.6#refs/tags/v2.9.6): `b9c17924adb533d3617ec7de97a7b0f4acc07b39 refs/tags/v2.9.6`
