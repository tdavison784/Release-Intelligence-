# Discovery report: linkerd2

- Repository: `github.com/linkerd/linkerd2` at `edge-26.9.3` (commit `86d395080040…`)
- Generated: 2026-10-02T12:42:03Z
- LLM: not used (deterministic resolver only)
- Tags: 463 tags: prefix "edge-", 378 stable, 0 prereleases (), 85 junk; latest stable edge-26.9.3; lineage minor
- Strict tag pattern: `^edge-(?P<version>\d+\.\d+\.\d+)$` (junk sample: git-1a18ca8f, stable-2.0.0, stable-2.1.0, stable-2.10.0, stable-2.10.1, stable-2.10.2)
- Scanned `github.com/linkerd/linkerd2@edge-26.9.3` (source profile): 1469 files listed, 246 read
- Scanned `github.com/linkerd/website@main` (docs profile): 2549 files listed, 210 read
- Validation: done against edge-26.7.2, edge-26.8.4, edge-26.9.2, edge-26.9.3
- Error: list tags of github.com/linkerd/helm: git ls-remote --tags https://github.com/linkerd/helm: exit status 128: fatal: could not read Username for 'https://github.com': terminal prompts disabled

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 3 | git-tags github.com/linkerd/linkerd2; github-releases linkerd/linkerd2 |
| release-notes | found | 0 | github-releases linkerd/linkerd2  @{{.Tag}} |
| changelog | candidates-only | 1 |  |
| helm-charts | candidates-only | 13 |  |
| registries | candidates-only | 4 |  |
| images | candidates-only | 36 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 2 | github-advisories linkerd/linkerd2 |
| version-relations | not-found | 0 |  |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/release.yml#L140 |
| artifact:extension-init | unverified | image.tag-assumed-release | https://github.com/linkerd/linkerd2/blob/edge-26.9.3/multicluster/charts/linkerd-multicluster/values.yaml#L86 |
| artifact:linkerd-control-plane-chart | unverified | helm.placeholder-version-follows-release | https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/linkerd-control-plane/Chart.yaml#L12 (+1) |
| artifact:linkerd-crds-chart | unverified | helm.placeholder-version-follows-release | https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/linkerd-crds/Chart.yaml#L10 (+1) |
| artifact:linkerd-multicluster-chart | unverified | helm.placeholder-version-follows-release | https://github.com/linkerd/linkerd2/blob/edge-26.9.3/multicluster/charts/linkerd-multicluster/Chart.yaml#L11 (+1) |
| source:advisories | discovered | security.advisories-referenced | https://github.com/linkerd/linkerd2/blob/edge-26.9.3/SECURITY.md#L21 (+1) |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/release.yml#L140 |
| source:tags | discovered | versions.git-tags | https://github.com/linkerd/linkerd2/releases/tag/edge-26.9.3#refs/tags/edge-26.9.3 |
| versioning | discovered | tags.scheme | https://github.com/linkerd/linkerd2/releases/tag/edge-26.9.3#refs/tags/edge-26.9.3 (+1) |

Statuses: 1 historically-validated, 4 discovered, 4 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: linkerd
name: linkerd2
homepage: https://github.com/linkerd/linkerd2
versioning:
  scheme: semver
  tagPrefix: edge-
  tagPattern: ^edge-(?P<version>\d+\.\d+\.\d+)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/linkerd/linkerd2
      tagPattern: ^edge-(?P<version>\d+\.\d+\.\d+)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: linkerd/linkerd2
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: linkerd/linkerd2
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    validatedAgainst:
      - 26.7.2
      - 26.8.4
      - 26.9.2
      - 26.9.3
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: linkerd/linkerd2
    notes: 'Security policy: SECURITY.md → https://docs.github.com/en/code-security/dependabot https://github.com/linkerd/linkerd2/security/advisories'
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - edge-26.7.2
    - edge-26.8.4
    - edge-26.9.2
    - edge-26.9.3
  notes: Proposed by automated discovery; relationship checks run against edge-26.7.2, edge-26.8.4, edge-26.9.2, edge-26.9.3.
```

## Open questions for the reviewer

- CHANGES.md stops at 18.9.1; confirm it is abandoned (release notes come from other sources).
- Ambiguity (registry-roles): 4 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.
- Ambiguity (main-chart): 3 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) linkerd-control-plane, linkerd-crds, linkerd-multicluster relate to the release version?

## Validation matrix

| Element | Verdict | edge-26.7.2 | edge-26.8.4 | edge-26.9.2 | edge-26.9.3 |
|---|---|---|---|---|---|
| artifact:extension-init | failing |  |  |  |  |
| artifact:linkerd-control-plane-chart | failing |  |  |  |  |
| artifact:linkerd-crds-chart | failing |  |  |  |  |
| artifact:linkerd-multicluster-chart | failing |  |  |  |  |
| content:linkerd-control-plane-chart/chart-metadata | validated |  |  |  |  |
| content:linkerd-control-plane-chart/helm-values | validated |  |  |  |  |
| content:linkerd-crds-chart/chart-metadata | validated |  |  |  |  |
| content:linkerd-crds-chart/helm-values | validated |  |  |  |  |
| content:linkerd-multicluster-chart/chart-metadata | validated |  |  |  |  |
| content:linkerd-multicluster-chart/helm-values | validated |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |

## Dropped elements

- `artifact:linkerd-control-plane-chart` (deterministic): failed validation: 26.7.2 fail (absent from https://helm.linkerd.io/stable linkerd-control-plane@26.7.2); 26.8.4 fail (absent from https://helm.linkerd.io/stable linkerd-control-plane@26.8.4); 26.9.2 fail (absent from https://helm.linkerd.io/stable linkerd-control-plane@26.9.2); 26.9.3 fail (absent from https://helm.linkerd.io/stable linkerd-control-plane@26.9.3)
- `artifact:linkerd-crds-chart` (deterministic): failed validation: 26.7.2 fail (absent from https://helm.linkerd.io/stable linkerd-crds@26.7.2); 26.8.4 fail (absent from https://helm.linkerd.io/stable linkerd-crds@26.8.4); 26.9.2 fail (absent from https://helm.linkerd.io/stable linkerd-crds@26.9.2); 26.9.3 fail (absent from https://helm.linkerd.io/stable linkerd-crds@26.9.3)
- `artifact:linkerd-multicluster-chart` (deterministic): failed validation: 26.7.2 fail (absent from https://helm.linkerd.io/stable linkerd-multicluster@26.7.2); 26.8.4 fail (absent from https://helm.linkerd.io/stable linkerd-multicluster@26.8.4); 26.9.2 fail (absent from https://helm.linkerd.io/stable linkerd-multicluster@26.9.2); 26.9.3 fail (absent from https://helm.linkerd.io/stable linkerd-multicluster@26.9.3)
- `artifact:extension-init` (deterministic): failed validation: 26.7.2 fail (absent from cr.l5d.io/linkerd/extension-init:edge-26.7.2); 26.8.4 fail (absent from cr.l5d.io/linkerd/extension-init:edge-26.8.4); 26.9.2 fail (absent from cr.l5d.io/linkerd/extension-init:edge-26.9.2); 26.9.3 fail (absent from cr.l5d.io/linkerd/extension-init:edge-26.9.3)

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 463 tags: prefix "edge-", 378 stable, 0 prereleases (), 85 junk; latest stable edge-26.9.3; lineage minor; strict tagPattern excludes junk tags such as git-1a18ca8f, stable-2.0.0, stable-2.1.0, stable-2.10.0 |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (463 tags: prefix "edge-", 378 stable, 0 prereleases (), 85 junk; latest stable edge-26.9.3; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:advisories | include | security.advisories-referenced | heuristic | The security policy states that advisories are published as GitHub Security Advisories. |
| artifact:linkerd-control-plane-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "0.0.0-undefined" replaced at release time; assumed to equal the release ({{.Version}}). Published via 1 channel(s); OCI locations first. |
| artifact:linkerd-crds-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "0.0.0-undefined" replaced at release time; assumed to equal the release ({{.Version}}). Published via 1 channel(s); OCI locations first. |
| artifact:linkerd-multicluster-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "0.0.0-undefined" replaced at release time; assumed to equal the release ({{.Version}}). Published via 1 channel(s); OCI locations first. |
| artifact:extension-init | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:linkerd-control-plane-chart | drop | validate.failed | computed | Relationship check failed: 26.7.2 fail (absent from https://helm.linkerd.io/stable linkerd-control-plane@26.7.2); 26.8.4 fail (absent from https://helm.linkerd.io/stable linkerd-control-plane@26.8.4); 26.9.2 fail (absent… |
| artifact:linkerd-crds-chart | drop | validate.failed | computed | Relationship check failed: 26.7.2 fail (absent from https://helm.linkerd.io/stable linkerd-crds@26.7.2); 26.8.4 fail (absent from https://helm.linkerd.io/stable linkerd-crds@26.8.4); 26.9.2 fail (absent from https://helm… |
| artifact:linkerd-multicluster-chart | drop | validate.failed | computed | Relationship check failed: 26.7.2 fail (absent from https://helm.linkerd.io/stable linkerd-multicluster@26.7.2); 26.8.4 fail (absent from https://helm.linkerd.io/stable linkerd-multicluster@26.8.4); 26.9.2 fail (absent f… |
| artifact:extension-init | drop | validate.failed | computed | Relationship check failed: 26.7.2 fail (absent from cr.l5d.io/linkerd/extension-init:edge-26.7.2); 26.8.4 fail (absent from cr.l5d.io/linkerd/extension-init:edge-26.8.4); 26.9.2 fail (absent from cr.l5d.io/linkerd/extens… |

<details><summary>57 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| changelog `CHANGES.md` | changelog.stale | Changelog is not maintained: newest entry 18.9.1 while the latest release is edge-26.9.3. |
| helm-repo `https://helm.linkerd.io/edge` | helm.dev-channel | Chart repository path is a development channel (unreleased main builds); a stable channel of the same chart exists. |
| helm-repo `https://helm.linkerd.io/edge` | helm.dev-channel | Chart repository path is a development channel (unreleased main builds); a stable channel of the same chart exists. |
| helm-chart `linkerd2-cni@charts/linkerd2-cni` | helm.unpublished | No publication channel (OCI push/reference or Helm repository) mentions this chart. |
| helm-chart `partials@charts/partials` | helm.unpublished | No publication channel (OCI push/reference or Helm repository) mentions this chart. |
| helm-chart `patch@charts/patch` | helm.unpublished | No publication channel (OCI push/reference or Helm repository) mentions this chart. |
| helm-chart `linkerd-multicluster-link@multicluster/charts/linkerd-multicluster-link` | helm.unpublished | No publication channel (OCI push/reference or Helm repository) mentions this chart. |
| helm-chart `linkerd-viz@viz/charts/linkerd-viz` | helm.unpublished | No publication channel (OCI push/reference or Helm repository) mentions this chart. |
| chart-repo `github.com/linkerd/helm` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| manifest `controller/proxy-injector/fake/data/deployment-inject-disabled.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/deployment-with-injected-proxy.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-inject-empty.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-inject-enabled-cpu-ratio.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-inject-enabled-log-level.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-inject-enabled.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-nativesidecar-inject-enabled.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-with-custom-debug-tag.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-with-debug-disabled.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-with-debug-enabled.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-with-opaque-ports.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `controller/proxy-injector/fake/data/pod-without-opaque-ports.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| image `ghcr.io/linkerd/dev` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `cr.l5d.io/linkerd/proxy` | image.test-only | Only referenced from test, sample or documentation files. |
| image `cr.l5d.io/linkerd/cni-plugin` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/rancher/k3s` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/overwrite-collector-image` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/redis` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/heptio-images/contour` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/envoyproxy/envoy-alpine` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/buoyantio/emojivoto-web` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `cr.l5d.io/linkerd/proxy-init` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `cr.l5d.io/linkerd/debug` | image.test-only | Only referenced from test, sample or documentation files. |
| image `gcr.io/istio-release/proxyv2` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/istio-release/proxy_init` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/buoyantio/emojivoto-emoji-svc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/buoyantio/bb` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `cr.l5d.io/linkerd/controller` | image.test-only | Only referenced from test, sample or documentation files. |
| image `linkedin.io/prom` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gc.io/linkerd-io/proxy` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `docker.io/library/memcached` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/mysql` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/buoyantio/slow_cooker` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/alpeb/family-server` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/curlimages/curl` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.l5d.io/buoyantio/emojivoto-emoji-svc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.l5d.io/buoyantio/emojivoto-web` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.l5d.io/buoyantio/emojivoto-voting-svc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/barkardk/rabbitmq-client` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/rabbitmq` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/prometheus` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/buoyantio/emojivoto-voting-svc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/buoyantio/helloworld` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image-name `controller` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `proxy` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### build-tool (1)

- `cosign` (high; workflow.cosign) — [linkerd/linkerd2:.github/workflows/release.yml L74](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/release.yml#L74): `- uses: sigstore/cosign-installer@v3`

### changelog (1)

- `CHANGES.md` (high; docs.changelog) — [linkerd/linkerd2:CHANGES.md L7616](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/CHANGES.md#L7616): `## v18.9.1`

### chart-repo (1)

- `github.com/linkerd/helm` (medium; docs.chart-repo-ref) — [linkerd/website:linkerd.io/content/blog/2025/0905-edge-release-roundup/index.md L113](https://github.com/linkerd/website/blob/132c23968bfc23166a9381a4c3de36de5c5636dd/linkerd.io/content/blog/2025/0905-edge-release-roundup/index.md#L113): `'linkerd/cli' instead of 'linkerd/helm' when installing a Linkerd extension`

### docs-repo (1)

- `github.com/linkerd/website` (medium; docs.docs-repo-ref) — [linkerd/linkerd2:CHANGES.md L2213](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/CHANGES.md#L2213): `[docs](https://github.com/linkerd/website/blob/0c3c5cd5ae329cd7dbcca18534f3bc8ec7d57859/linkerd.io/content/2.1…`

### helm-chart (8)

- `linkerd-control-plane@charts/linkerd-control-plane` (high; helm.chart) — [linkerd/linkerd2:charts/linkerd-control-plane/Chart.yaml L12](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/linkerd-control-plane/Chart.yaml#L12): `name: "linkerd-control-plane"`
- `linkerd-crds@charts/linkerd-crds` (high; helm.chart) — [linkerd/linkerd2:charts/linkerd-crds/Chart.yaml L10](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/linkerd-crds/Chart.yaml#L10): `name: "linkerd-crds"`
- `linkerd-multicluster-link@multicluster/charts/linkerd-multicluster-link` (high; helm.chart) — [linkerd/linkerd2:multicluster/charts/linkerd-multicluster-link/Chart.yaml L14](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/multicluster/charts/linkerd-multicluster-link/Chart.yaml#L14): `name: "linkerd-multicluster-link"`
- `linkerd-multicluster@multicluster/charts/linkerd-multicluster` (high; helm.chart) — [linkerd/linkerd2:multicluster/charts/linkerd-multicluster/Chart.yaml L11](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/multicluster/charts/linkerd-multicluster/Chart.yaml#L11): `name: "linkerd-multicluster"`
- `linkerd-viz@viz/charts/linkerd-viz` (high; helm.chart) — [linkerd/linkerd2:viz/charts/linkerd-viz/Chart.yaml L11](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/viz/charts/linkerd-viz/Chart.yaml#L11): `name: "linkerd-viz"`
- `linkerd2-cni@charts/linkerd2-cni` (high; helm.chart) — [linkerd/linkerd2:charts/linkerd2-cni/Chart.yaml L11](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/linkerd2-cni/Chart.yaml#L11): `name: "linkerd2-cni"`
- `partials@charts/partials` (high; helm.chart) — [linkerd/linkerd2:charts/partials/Chart.yaml L5](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/partials/Chart.yaml#L5): `name: partials`
- `patch@charts/patch` (high; helm.chart) — [linkerd/linkerd2:charts/patch/Chart.yaml L2](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/patch/Chart.yaml#L2): `name: "patch"`

### helm-repo (4)

- `https://grafana.github.io/helm-charts` (medium; docs.helm-repo-add) — [linkerd/linkerd2:grafana/README.md L18](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/grafana/README.md#L18): `helm repo add grafana https://grafana.github.io/helm-charts`
- `https://helm.linkerd.io/edge` (medium; docs.helm-repo-add) — [linkerd/linkerd2:charts/linkerd-control-plane/README.md.gotmpl L54](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/charts/linkerd-control-plane/README.md.gotmpl#L54): `helm repo add linkerd https://helm.linkerd.io/edge`
- `https://helm.linkerd.io/stable` (medium; docs.helm-repo-add) — [linkerd/linkerd2:CHANGES.md L1712](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/CHANGES.md#L1712): `helm repo add linkerd https://helm.linkerd.io/stable`
- `https://linkerd.github.io/linkerd-smi` (medium; docs.helm-repo-add) — [linkerd/website:linkerd.io/content/2.12/tasks/upgrade.md L262](https://github.com/linkerd/website/blob/132c23968bfc23166a9381a4c3de36de5c5636dd/linkerd.io/content/2.12/tasks/upgrade.md#L262): `helm repo add l5d-smi https://linkerd.github.io/linkerd-smi`

### image (34)

- `cr.l5d.io/linkerd/extension-init` (high; helm.values-image) — [linkerd/linkerd2:multicluster/charts/linkerd-multicluster/values.yaml L86](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/multicluster/charts/linkerd-multicluster/values.yaml#L86): `name: extension-init`
- `docker.io/prom/prometheus` (high; helm.values-image, manifest.image) — [linkerd/linkerd2:test/integration/external/testdata/external_prometheus.yaml L218](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/test/integration/external/testdata/external_prometheus.yaml#L218): `image: prom/prometheus:v2.19.3`
- `cr.l5d.io/linkerd/cni-plugin` (medium; docs.image-ref) — [linkerd/linkerd2:RELEASE.md L40](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/RELEASE.md#L40): `Upgrade the 'cr.l5d.io/linkerd/cni-plugin' image tag in the test fixtures`
- `cr.l5d.io/linkerd/proxy` (medium; docs.image-ref, manifest.image) — [linkerd/linkerd2:BUILD.md L420](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/BUILD.md#L420): `DOCKER_TAG=cr.l5d.io/linkerd/proxy:dev make docker`
- `cr.l5d.io/linkerd/proxy-init` (medium; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_emojivoto_already_injected.input.yml L112](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_emojivoto_already_injected.input.yml#L112): `- image: cr.l5d.io/linkerd/proxy-init:foo`
- `docker.io/library/memcached` (medium; manifest.image) — [linkerd/linkerd2:controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml L10](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml#L10): `image: memcached`
- `docker.io/library/mysql` (medium; manifest.image) — [linkerd/linkerd2:controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml L16](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml#L16): `image: mysql`
- `docker.io/library/nginx` (medium; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject-filepath/expected/injected_nginx.yaml L22](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject-filepath/expected/injected_nginx.yaml#L22): `- image: nginx`
- `gc.io/linkerd-io/proxy` (medium; manifest.image) — [linkerd/linkerd2:controller/proxy-injector/fake/data/deployment-with-injected-proxy.yaml L30](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/deployment-with-injected-proxy.yaml#L30): `image: gc.io/linkerd-io/proxy`
- `ghcr.io/linkerd/dev` (medium; docs.image-ref, workflow.image-ref) — [linkerd/linkerd2:.github/workflows/devcontainer.yml L18](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/devcontainer.yml#L18): `container: ghcr.io/linkerd/dev:v50-rust`
- `cr.l5d.io/linkerd/controller` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_tap_deployment.input.yml L45](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_tap_deployment.input.yml#L45): `image: cr.l5d.io/linkerd/controller:git-a94122bf`
- `cr.l5d.io/linkerd/debug` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_emojivoto_deployment_debug.golden.yml L39](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_emojivoto_deployment_debug.golden.yml#L39): `- image: cr.l5d.io/linkerd/debug:test-inject-debug-version`
- `docker.io/buoyantio/bb` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_gettest_deployment.bad.input.yml L12](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_gettest_deployment.bad.input.yml#L12): `image: buoyantio/bb:v0.0.6`
- `docker.io/buoyantio/emojivoto-emoji-svc` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_emojivoto_list.golden.yml L295](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_emojivoto_list.golden.yml#L295): `image: buoyantio/emojivoto-emoji-svc:v10`
- `docker.io/buoyantio/emojivoto-voting-svc` (low; manifest.image) — [linkerd/linkerd2:test/integration/install/testdata/upgrade_test.yaml L84](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/test/integration/install/testdata/upgrade_test.yaml#L84): `image: buoyantio/emojivoto-voting-svc:v10`
- `docker.io/buoyantio/emojivoto-web` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_emojivoto_already_injected.golden.yml L33](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_emojivoto_already_injected.golden.yml#L33): `image: buoyantio/emojivoto-web:v10`
- `docker.io/buoyantio/helloworld` (low; manifest.image) — [linkerd/linkerd2:test/integration/viz/serviceprofiles/testdata/hello_world.yaml L17](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/test/integration/viz/serviceprofiles/testdata/hello_world.yaml#L17): `image: buoyantio/helloworld:0.1.7`
- `docker.io/buoyantio/slow_cooker` (low; manifest.image) — [linkerd/linkerd2:test/integration/deep/appprotocol/testdata/appprotocol_client.yaml L19](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/test/integration/deep/appprotocol/testdata/appprotocol_client.yaml#L19): `image: buoyantio/slow_cooker:1.3.0`
- `docker.io/curlimages/curl` (low; manifest.image) — [linkerd/linkerd2:test/integration/deep/dualstack/testdata/ipfamilies-server-client.yml L74](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/test/integration/deep/dualstack/testdata/ipfamilies-server-client.yml#L74): `image: curlimages/curl`
- `docker.io/envoyproxy/envoy-alpine` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_contour.golden.yml L46](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_contour.golden.yml#L46): `image: docker.io/envoyproxy/envoy-alpine:v1.6.0`
- `docker.io/library/busybox` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject_emojivoto_cronjob.golden.yml L21](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject_emojivoto_cronjob.golden.yml#L21): `image: busybox`
- `docker.io/library/overwrite-collector-image` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/addon_config_overwrite.yaml L4](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/addon_config_overwrite.yaml#L4): `image: overwrite-collector-image`
- `docker.io/library/rabbitmq` (low; manifest.image) — [linkerd/linkerd2:test/integration/external/externalresources/testdata/rabbitmq-server.yaml L18](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/test/integration/external/externalresources/testdata/rabbitmq-server.yaml#L18): `- image: rabbitmq:3.8.12-rc.3-management`
- `docker.io/library/redis` (low; manifest.image) — [linkerd/linkerd2:cli/cmd/testdata/inject-filepath/expected/injected_nginx_redis.yaml L22](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/cli/cmd/testdata/inject-filepath/expected/injected_nginx_redis.yaml#L22): `- image: redis`
- `docker.io/rancher/k3s` (low; script.image-ref) — [linkerd/linkerd2:bin/_test-helpers.sh L8](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/bin/_test-helpers.sh#L8): `k8s_version_max='docker.io/rancher/k3s:v1.36.3-k3s1'`
- … 9 more

### image-name (2)

- `controller` (low; dockerfile.name) — [linkerd/linkerd2:Dockerfile.controller L2](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/Dockerfile.controller#L2): `FROM --platform=$BUILDPLATFORM golang:1.26.7-alpine AS go-deps`
- `proxy` (low; dockerfile.name) — [linkerd/linkerd2:Dockerfile.proxy L6](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/Dockerfile.proxy#L6): `FROM --platform=$BUILDPLATFORM golang:1.26.7-alpine AS go-deps`

### manifest (14)

- `controller/proxy-injector/fake/data/deployment-inject-disabled.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/deployment-inject-disabled.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/deployment-inject-disabled.yaml#L1): `kind: Deployment`
- `controller/proxy-injector/fake/data/deployment-with-injected-proxy.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/deployment-with-injected-proxy.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/deployment-with-injected-proxy.yaml#L1): `kind: Deployment`
- `controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml L2](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/filter-pod-opaque-ports.yaml#L2): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-inject-empty.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-inject-empty.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-inject-empty.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-inject-enabled-cpu-ratio.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-inject-enabled-cpu-ratio.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-inject-enabled-cpu-ratio.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-inject-enabled-log-level.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-inject-enabled-log-level.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-inject-enabled-log-level.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-inject-enabled.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-inject-enabled.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-inject-enabled.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-nativesidecar-inject-enabled.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-nativesidecar-inject-enabled.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-nativesidecar-inject-enabled.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-with-custom-debug-tag.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-with-custom-debug-tag.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-with-custom-debug-tag.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-with-debug-disabled.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-with-debug-disabled.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-with-debug-disabled.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-with-debug-enabled.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-with-debug-enabled.yaml L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-with-debug-enabled.yaml#L1): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-with-opaque-ports.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-with-opaque-ports.yaml L3](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-with-opaque-ports.yaml#L3): `kind: Pod`
- `controller/proxy-injector/fake/data/pod-without-opaque-ports.yaml` (medium; manifest.workloads) — [linkerd/linkerd2:controller/proxy-injector/fake/data/pod-without-opaque-ports.yaml L3](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/controller/proxy-injector/fake/data/pod-without-opaque-ports.yaml#L3): `kind: Pod`
- `grafana/values.yaml` (medium; docs.raw-url) — [linkerd/linkerd2:grafana/README.md L20](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/grafana/README.md#L20): `-f https://raw.githubusercontent.com/linkerd/linkerd2/main/grafana/values.yaml`

### registry (4)

- `ghcr.io` (high; workflow.registry-login) — [linkerd/linkerd2:.github/workflows/release.yml L61](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/release.yml#L61): `registry: ghcr.io`
- `cr.l5d.io/linkerd` (low; docs.registry-ref, script.registry-ref) — [linkerd/linkerd2:BUILD.md L223](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/BUILD.md#L223): `'DOCKER_REGISTRY' (which defaults to the official registry 'cr.l5d.io/linkerd').`
- `gcr.io/linkerd-io` (low; docs.registry-ref) — [linkerd/linkerd2:CHANGES.md L7752](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/CHANGES.md#L7752): `* Move all Docker images to 'gcr.io/linkerd-io' repo`
- `ghcr.io/linkerd` (low; workflow.registry-ref) — [linkerd/linkerd2:.github/workflows/integration.yml L12](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/integration.yml#L12): `DOCKER_REGISTRY: ghcr.io/linkerd`

### release-publisher (1)

- `softprops/action-gh-release` (high; workflow.release-upload) — [linkerd/linkerd2:.github/workflows/release.yml L140](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/release.yml#L140): `uses: softprops/action-gh-release@efb35369e0ad2afab669f228072c1b0d510eae64`

### release-trigger (1)

- `edge-*` (high; workflow.tag-trigger) — [linkerd/linkerd2:.github/workflows/release.yml L6](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/.github/workflows/release.yml#L6): `- "edge-*"`

### security-advisories (1)

- `linkerd/linkerd2` (high; docs.security-advisories-link) — [linkerd/linkerd2:SECURITY.md L21](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/SECURITY.md#L21): `repo](https://github.com/linkerd/linkerd2/security/advisories). The maintainers`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [linkerd/linkerd2:SECURITY.md L1](https://github.com/linkerd/linkerd2/blob/edge-26.9.3/SECURITY.md#L1): `# Linkerd Security Policy`

### tag-scheme (1)

- `^edge-(?P<version>\d+\.\d+\.\d+)$` (high; tags.ls-remote) — [linkerd/linkerd2/releases/tag/edge-26.9.3 refs/tags/edge-26.9.3](https://github.com/linkerd/linkerd2/releases/tag/edge-26.9.3#refs/tags/edge-26.9.3): `86d395080040d07b8e0109547135520218b696f7 refs/tags/edge-26.9.3`
