# Discovery report: ingress-nginx

- Repository: `github.com/kubernetes/ingress-nginx` at `helm-chart-4.15.1` (commit `0a5901f3c64f…`)
- Generated: 2026-10-02T12:35:14Z
- LLM: not used (deterministic resolver only)
- Tags: 275 tags: prefix "helm-chart-", 119 stable, 6 prereleases (beta.N×6), 150 junk; latest stable helm-chart-4.15.1; lineage minor
- Strict tag pattern: `^helm-chart-(?P<version>\d+\.\d+\.\d+(?:-(?:beta\.\d+))?)$` (junk sample: controller-v0.34.0, controller-v0.34.1, controller-v0.35.0, controller-v0.40.0, controller-v0.40.1, controller-v0.40.2)
- Scanned `github.com/kubernetes/ingress-nginx@helm-chart-4.15.1` (source profile): 1146 files listed, 503 read
- Scanned `github.com/kubernetes/k8s.io@main` (docs profile): 16823 files listed, 118 read
- Validation: done against helm-chart-4.13.0, helm-chart-4.13.9, helm-chart-4.14.0, helm-chart-4.14.5, helm-chart-4.15.0, helm-chart-4.15.1

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 8 | git-tags github.com/kubernetes/ingress-nginx; github-releases kubernetes/ingress-nginx |
| release-notes | found | 2 | repo-file github.com/kubernetes/ingress-nginx charts/ingress-nginx/changelog/helm-chart-{{.Version}}.md; github-releases kubernetes/ingress-nginx  @{{.Tag}} |
| changelog | candidates-only | 1 |  |
| helm-charts | found | 7 | ingress-nginx via helm-repo:https://kubernetes.github.io/ingress-nginx, helm-repo:https://raw.githubusercontent.com/kubernetes/ingress-nginx/gh-pages, helm-git:github.com/kubernetes/ingress-nginx |
| registries | found | 8 | eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256; k8s.gcr.io/kubernetes; quay.io/repository/kubernetes-ingress-controller; quay.io/repository/kubernetes-ingress-controller/nginx-ingress… |
| images | found | 32 | eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256/defaultbackend-amd64:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories kubernetes/ingress-nginx |
| version-relations | found | 1 | ingress-nginx-chart: version = {{.Version}}; deploy-static-provider-aws-deploy: version = {{.Tag}}; deploy-static-provider-do-deploy: version = {{.Tag}}; deploy-static-provider-scw-deploy: version = {{.Tag}}; deploy-stat… |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:deploy-static-provider-aws-deploy | historically-validated | manifest.in-repo-install | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/aws/deploy.yaml#L396 |
| artifact:deploy-static-provider-baremetal-deploy | historically-validated | manifest.in-repo-install | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/ISSUE_TEMPLATE/bug_report.md#L98 |
| artifact:deploy-static-provider-do-deploy | historically-validated | manifest.in-repo-install | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/do/deploy.yaml#L395 |
| artifact:deploy-static-provider-scw-deploy | historically-validated | manifest.in-repo-install | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/scw/deploy.yaml#L395 |
| artifact:ingress-nginx-chart | historically-validated | helm.version-matches-release | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/Chart.yaml#L20 (+1) |
| source:release-notes-docs | historically-validated | notes.per-release-file | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/changelog/helm-chart-4.15.1.md# |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/plugin.yaml#L30 (+1) |
| artifact:controller | unverified | image.tag-assumed-release | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/vulnerability-scans.yaml#L59 |
| artifact:defaultbackend-amd64 | unverified | image.tag-assumed-release | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/values.yaml#L1031 |
| artifact:kubectl-ingress-nginx-binary | unverified | asset.hosted-release-download | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.goreleaser.yaml#L28 |
| source:advisories | discovered | security.github-hosted-default | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/SECURITY.md#L1 |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/plugin.yaml#L30 (+1) |
| source:tags | discovered | versions.git-tags | https://github.com/kubernetes/ingress-nginx/releases/tag/helm-chart-4.15.1#refs/tags/helm-chart-4.15.1 |
| versioning | discovered | tags.scheme | https://github.com/kubernetes/ingress-nginx/releases/tag/helm-chart-4.15.1#refs/tags/helm-chart-4.15.1 |

Statuses: 7 historically-validated, 4 discovered, 3 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: ingress-nginx
name: ingress-nginx
homepage: https://github.com/kubernetes/ingress-nginx
versioning:
  scheme: semver
  tagPrefix: helm-chart-
  tagPattern: ^helm-chart-(?P<version>\d+\.\d+\.\d+(?:-(?:beta\.\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/kubernetes/ingress-nginx
      tagPattern: ^helm-chart-(?P<version>\d+\.\d+\.\d+(?:-(?:beta\.\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: kubernetes/ingress-nginx
    priority: 1
  - id: release-notes-docs
    roles:
      - release-notes
    locator:
      kind: repo-file
      repository: github.com/kubernetes/ingress-nginx
      path: charts/ingress-nginx/changelog/helm-chart-{{.Version}}.md
    fallbackGroup: release-notes
    validatedAgainst:
      - 4.13.0
      - 4.13.9
      - 4.14.0
      - 4.14.5
      - 4.15.0
      - 4.15.1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: kubernetes/ingress-nginx
      ref: '{{.Tag}}'
    priority: 1
    fallbackGroup: release-notes
    validatedAgainst:
      - 4.13.0
      - 4.13.9
      - 4.14.0
      - 4.14.5
      - 4.15.0
      - 4.15.1
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: kubernetes/ingress-nginx
artifacts:
  - id: ingress-nginx-chart
    type: helm-chart
    name: ingress-nginx
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: helm-repo
        url: https://kubernetes.github.io/ingress-nginx
        chart: ingress-nginx
      - kind: helm-repo
        url: https://raw.githubusercontent.com/kubernetes/ingress-nginx/gh-pages
        chart: ingress-nginx
      - kind: helm-git
        repository: github.com/kubernetes/ingress-nginx
        path: charts/ingress-nginx
        tagPattern: ^ingress-nginx-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/kubernetes/ingress-nginx
          path: charts/ingress-nginx/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/kubernetes/ingress-nginx
          path: charts/ingress-nginx/Chart.yaml
    validatedAgainst:
      - 4.13.0
      - 4.13.9
      - 4.14.0
      - 4.14.5
      - 4.15.0
      - 4.15.1
  - id: deploy-static-provider-aws-deploy
    type: manifest
    name: deploy.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/kubernetes/ingress-nginx
        path: deploy/static/provider/aws/deploy.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 4.13.0
      - 4.13.9
      - 4.14.0
      - 4.14.5
      - 4.15.0
      - 4.15.1
  - id: deploy-static-provider-do-deploy
    type: manifest
    name: deploy.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/kubernetes/ingress-nginx
        path: deploy/static/provider/do/deploy.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 4.13.0
      - 4.13.9
      - 4.14.0
      - 4.14.5
      - 4.15.0
      - 4.15.1
  - id: deploy-static-provider-scw-deploy
    type: manifest
    name: deploy.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/kubernetes/ingress-nginx
        path: deploy/static/provider/scw/deploy.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 4.13.0
      - 4.13.9
      - 4.14.0
      - 4.14.5
      - 4.15.0
      - 4.15.1
  - id: deploy-static-provider-baremetal-deploy
    type: manifest
    name: deploy.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/kubernetes/ingress-nginx
        path: deploy/static/provider/baremetal/deploy.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 4.13.0
      - 4.13.9
      - 4.14.0
      - 4.14.5
      - 4.15.0
      - 4.15.1
  - id: defaultbackend-amd64
    type: container-image
    name: defaultbackend-amd64
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256/defaultbackend-amd64
      - kind: oci
        repository: quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller/defaultbackend-amd64
      - kind: oci
        repository: quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller-arm/defaultbackend-amd64
      - kind: oci
        repository: quay.io/repository/kubernetes-ingress-controller/defaultbackend-amd64
      - kind: oci
        repository: k8s.gcr.io/kubernetes/defaultbackend-amd64
    notes: 'Unverified by discovery (unverifiable: 4.13.0 unverifiable (absent from k8s.gcr.io/kubernetes/defaultbackend-amd64:helm-chart-4.13.0; not verifiable: eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256/defaultbackend-amd64:helm-chart-4.13.0 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller/defaultbackend-amd64:helm-chart-4.13.0 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller-arm/defaultbackend-amd64:helm-chart-4.13.0 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/defaultbackend-amd64:helm-chart-4.13.0 (oci: unavailable)); 4.13.9 unverifiable (absent from k8s.gcr.io/kubernetes/defaultbackend-amd64:helm-chart-4.13.9; not verifiable: eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256/defaultbackend-amd64:helm-chart-4.13.9 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller/defaultbackend-amd64:helm-chart-4.13.9 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller-arm/defaultbackend-amd64:helm-chart-4.13.9 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/defaultbackend-amd64:helm-chart-4.13.9 (oci: unavailable)); 4.14.0 unverifiable (absent from k8s.gcr.io/kubernetes/defaultbackend-amd64:helm-chart-4.14.0; not verifiable: eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256/defaultbackend-amd64:helm-chart-4.14.0 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller/defaultbackend-amd64:helm-chart-4.14.0 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller-arm/defaultbackend-amd64:helm-chart-4.14.0 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/defaultbackend-amd64:helm-chart-4.14.0 (oci: unavailable)); 4.14.5 unverifiable (absent from k8s.gcr.io/kubernetes/defaultbackend-amd64:helm-chart-4.14.5; not verifiable: eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256/defaultbackend-amd64:helm-chart-4.14.5 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller/defaultbackend-amd64:helm-chart-4.14.5 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller-arm/defaultbackend-amd64:helm-chart-4.14.5 (oci: unavailable), quay.io/repository/kubernetes-ingress-controller/defaultbackend-amd64:helm-chart-4.14.5 (oci: unavailable)); …).'
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - helm-chart-4.13.0
    - helm-chart-4.13.9
    - helm-chart-4.14.0
    - helm-chart-4.14.5
    - helm-chart-4.15.0
    - helm-chart-4.15.1
  notes: Proposed by automated discovery; relationship checks run against helm-chart-4.13.0, helm-chart-4.13.9, helm-chart-4.14.0, helm-chart-4.14.5, helm-chart-4.15.0, helm-chart-4.15.1.
```

## Open questions for the reviewer

- Changelog.md stops at 1.5.1; confirm it is abandoned (release notes come from other sources).
- Ambiguity (registry-roles): 11 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no upgrade-guide, compatibility source was derived from it.

## Validation matrix

| Element | Verdict | helm-chart-4.13.0 | helm-chart-4.13.9 | helm-chart-4.14.0 | helm-chart-4.14.5 | helm-chart-4.15.0 | helm-chart-4.15.1 |
|---|---|---|---|---|---|---|---|
| artifact:controller | failing |  |  |  |  |  |  |
| artifact:defaultbackend-amd64 | unverifiable |  |  |  |  |  |  |
| artifact:deploy-static-provider-aws-deploy | validated |  |  |  |  |  |  |
| artifact:deploy-static-provider-baremetal-deploy | validated |  |  |  |  |  |  |
| artifact:deploy-static-provider-do-deploy | validated |  |  |  |  |  |  |
| artifact:deploy-static-provider-scw-deploy | validated |  |  |  |  |  |  |
| artifact:ingress-nginx-chart | validated |  |  |  |  |  |  |
| artifact:kubectl-ingress-nginx-binary | failing |  |  |  |  |  |  |
| content:deploy-static-provider-aws-deploy/image-refs | validated |  |  |  |  |  |  |
| content:deploy-static-provider-baremetal-deploy/image-refs | validated |  |  |  |  |  |  |
| content:deploy-static-provider-do-deploy/image-refs | validated |  |  |  |  |  |  |
| content:deploy-static-provider-scw-deploy/image-refs | validated |  |  |  |  |  |  |
| content:ingress-nginx-chart/chart-metadata | validated |  |  |  |  |  |  |
| content:ingress-nginx-chart/helm-values | validated |  |  |  |  |  |  |
| source:release-notes-docs | validated |  |  |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |  |  |

## Dropped elements

- `artifact:kubectl-ingress-nginx-binary` (deterministic): failed validation: 4.13.0 fail (absent from https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-4.13.0/kubectl-ingress-nginx_linux_amd64.tar.gz); 4.13.9 fail (absent from https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-4.13.9/kubectl-ingress-nginx_linux_amd64.tar.gz); 4.14.0 fail (absent from https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-4.14.0/kubectl-ingress-nginx_linux_amd64.tar.gz); 4.14.5 fail (absent from https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-4.14.5/kubectl-ingress-nginx_linux_amd64.tar.gz); …
- `artifact:controller` (deterministic): failed validation: 4.13.0 fail (absent from registry.k8s.io/ingress-nginx/controller:helm-chart-4.13.0); 4.13.9 fail (absent from registry.k8s.io/ingress-nginx/controller:helm-chart-4.13.9); 4.14.0 fail (absent from registry.k8s.io/ingress-nginx/controller:helm-chart-4.14.0); 4.14.5 fail (absent from registry.k8s.io/ingress-nginx/controller:helm-chart-4.14.5); …

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 275 tags: prefix "helm-chart-", 119 stable, 6 prereleases (beta.N×6), 150 junk; latest stable helm-chart-4.15.1; lineage minor; strict tagPattern excludes junk tags such as controller-v0.34.0, controller-v0.34.1, contro… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (275 tags: prefix "helm-chart-", 119 stable, 6 prereleases (beta.N×6), 150 junk; latest stable helm-chart-4.15.1; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-docs | include | notes.per-release-file | heuristic | One notes document per release (127 instances, newest charts/ingress-nginx/changelog/helm-chart-4.15.1.md). |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:ingress-nginx-chart | include | helm.version-matches-release | heuristic | Chart.yaml at the scanned release carries version "4.15.1", the release version; chart versions follow the release ({{.Version}}). Published via 3 channel(s); OCI locations first. |
| artifact:deploy-static-provider-aws-deploy | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:deploy-static-provider-do-deploy | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:deploy-static-provider-scw-deploy | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:deploy-static-provider-baremetal-deploy | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:kubectl-ingress-nginx-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (6 reference(s)). |
| artifact:controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:defaultbackend-amd64 | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 5 registries; ordered by evidence strength. |
| artifact:kubectl-ingress-nginx-binary | drop | validate.failed | computed | Relationship check failed: 4.13.0 fail (absent from https://github.com/kubernetes/ingress-nginx/releases/download/helm-chart-4.13.0/kubectl-ingress-nginx_linux_amd64.tar.gz); 4.13.9 fail (absent from https://github.com/k… |
| artifact:controller | drop | validate.failed | computed | Relationship check failed: 4.13.0 fail (absent from registry.k8s.io/ingress-nginx/controller:helm-chart-4.13.0); 4.13.9 fail (absent from registry.k8s.io/ingress-nginx/controller:helm-chart-4.13.9); 4.14.0 fail (absent f… |
| artifact:defaultbackend-amd64 | annotate | validate.unverifiable | computed | Kept unverified: 4.13.0 unverifiable (absent from k8s.gcr.io/kubernetes/defaultbackend-amd64:helm-chart-4.13.0; not verifiable: eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256/defaultb… |

<details><summary>39 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| release-notes `charts/ingress-nginx/changelog/helm-chart-{{.Version}}-beta.0.md` | notes.lower-ranked-template | Another per-release notes path template ranks higher. |
| changelog `Changelog.md` | changelog.stale | Changelog is not maintained: newest entry 1.5.1 while the latest release is helm-chart-4.15.1. |
| manifest `deploy/grafana/deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `deploy/prometheus/deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `deploy/static/provider/kind/deploy.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `deploy/static/provider/oracle/deploy.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `deploy/static/provider/cloud/deploy.yaml` | manifest.limit | More install manifest variants exist; only the four best ranked are modelled. |
| manifest `deploy/static/provider/exoscale/deploy.yaml` | manifest.limit | More install manifest variants exist; only the four best ranked are modelled. |
| manifest `deploy/static/provider/aws/nlb-with-tls-termination/deploy.yaml` | manifest.limit | More install manifest variants exist; only the four best ranked are modelled. |
| image `registry.k8s.io/ingress-nginx/nginx` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned,snapshot). |
| image `registry.k8s.io/ingress-nginx/controller-chroot` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `k8s.gcr.io/ingress-nginx/controller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `k8s.gcr.io/ingress-nginx/controller-chroot` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `us.gcr.io/k8s-artifacts-prod/ingress-nginx/controller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `eu.gcr.io/k8s-artifacts-prod/ingress-nginx/controller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `asia.gcr.io/k8s-artifacts-prod/ingress-nginx/controller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/kubernetes-ingress-controller/nginx-ingress-controller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `gcr.io/google_containers/nginx-ingress-controller` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/controller` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `registry.k8s.io/images/k8s-staging-ingress-nginx/images.yaml` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/controller-chroot` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `registry.k8s.io/ingress-nginx/kube-webhook-certgen` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `registry.k8s.io/ingress-nginx/e2e-test-runner` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `gcr.io/k8s-staging-test-infra/gcb-docker-gcloud` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/grafana` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/prometheus` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/oauth2-proxy/oauth2-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/vouch/vouch-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/ingress-nginx/e2e-test-echo` | image.test-only | Only referenced from test, sample or documentation files. |
| image `registry.k8s.io/e2e-test-images/echoserver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/ingress-nginx/custom-error-pages` | image.test-only | Only referenced from test, sample or documentation files. |
| image `registry.k8s.io/ingress-nginx/custom-error-pages` | image.test-only | Only referenced from test, sample or documentation files. |
| image `gcr.io/k8s-staging-ingress-nginx/ext-auth-example-authsvc` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/library/registry` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/google_containers/nginx` | image.test-only | Only referenced from test, sample or documentation files. |
| image `gcr.io/cloud-builders/docker` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/nginx` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/e2e-test-runner` | image.test-only | Only referenced from test, sample or documentation files. |

</details>

## Candidates


### build-tool (2)

- `goreleaser` (high; goreleaser.config, workflow.goreleaser) — [kubernetes/ingress-nginx:.github/workflows/plugin.yaml L30](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/plugin.yaml#L30): `uses: goreleaser/goreleaser-action@ec59f474b9834571250b370d4735c50f8e2d1e29 # v7.0.0`
- `chart-releaser` (medium; workflow.chart-releaser) — [kubernetes/ingress-nginx:.github/workflows/chart.yaml L60](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/chart.yaml#L60): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`

### changelog (1)

- `Changelog.md` (high; docs.changelog) — [kubernetes/ingress-nginx:Changelog.md L5](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L5): `### 1.5.1`

### docs-repo (2)

- `github.com/kubernetes/k8s.io` (medium; docs.docs-repo-ref) — [kubernetes/ingress-nginx:MANUAL_RELEASE.md L100](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/MANUAL_RELEASE.md#L100): `### b. Add the new image to [k8s.io](http://github.com/kubernetes/k8s.io)`
- `github.com/kubernetes/registry.k8s.io` (medium; docs.docs-repo-ref) — [kubernetes/k8s.io:infra/aws/terraform/registry-k8s-io-prod/README.md L3](https://github.com/kubernetes/k8s.io/blob/55167bb0a2867208070e7b5c5b90f2381b1bf685/infra/aws/terraform/registry-k8s-io-prod/README.md#L3): `The resources created here are used by [archeio](https://github.com/kubernetes/registry.k8s.io/tree/main/cmd/a…`

### helm-chart (1)

- `ingress-nginx@charts/ingress-nginx` (high; helm.chart) — [kubernetes/ingress-nginx:charts/ingress-nginx/Chart.yaml L20](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/Chart.yaml#L20): `name: ingress-nginx`

### helm-oci (2)

- `registry.k8s.io/subproject/charts/subproject` (medium; docs.oci-ref) — [kubernetes/k8s.io:registry.k8s.io/README.md L152](https://github.com/kubernetes/k8s.io/blob/55167bb0a2867208070e7b5c5b90f2381b1bf685/registry.k8s.io/README.md#L152): `6. Once the PR is merged, ensure the image promoter job for your merge commit is successful, then confirm that…`
- `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/charts` (medium; make.helm-push, make.oci-ref) — [kubernetes/ingress-nginx:charts/Makefile L18](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/Makefile#L18): `REPOSITORY ?= $(REGISTRY)/charts`

### helm-repo (4)

- `https://grafana.github.io/helm-charts` (medium; docs.helm-repo-add) — [kubernetes/ingress-nginx:docs/user-guide/third-party-addons/opentelemetry.md L178](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/user-guide/third-party-addons/opentelemetry.md#L178): `helm repo add grafana https://grafana.github.io/helm-charts`
- `https://kubernetes.github.io/ingress-nginx` (medium; docs.helm-repo-add, workflow.chart-releaser-pages) — [kubernetes/ingress-nginx:.github/workflows/chart.yaml L60](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/chart.yaml#L60): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`
- `https://open-telemetry.github.io/opentelemetry-helm-charts` (medium; docs.helm-repo-add) — [kubernetes/ingress-nginx:docs/user-guide/third-party-addons/opentelemetry.md L177](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/user-guide/third-party-addons/opentelemetry.md#L177): `helm repo add open-telemetry https://open-telemetry.github.io/opentelemetry-helm-charts`
- `https://raw.githubusercontent.com/kubernetes/ingress-nginx/gh-pages` (medium; workflow.chart-releaser-pages-raw) — [kubernetes/ingress-nginx:.github/workflows/chart.yaml L60](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/chart.yaml#L60): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`

### image (31)

- `docker.io/grafana/grafana` (high; kustomize.image, manifest.image) — [kubernetes/ingress-nginx:deploy/grafana/deployment.yaml L15](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/grafana/deployment.yaml#L15): `- image: grafana/grafana`
- `docker.io/prom/prometheus` (high; kustomize.image, manifest.image) — [kubernetes/ingress-nginx:deploy/prometheus/deployment.yaml L12](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/prometheus/deployment.yaml#L12): `image: prom/prometheus`
- `registry.k8s.io/ingress-nginx/controller` (high; docs.image-ref, manifest.image, workflow.image-ref) — [kubernetes/ingress-nginx:.github/workflows/vulnerability-scans.yaml L59](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/vulnerability-scans.yaml#L59): `run: echo "Scanning registry.k8s.io/ingress-nginx/controller@${{ matrix.versions }}"`
- `asia.gcr.io/k8s-artifacts-prod/ingress-nginx/controller` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L1489](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L1489): `- 'asia.gcr.io/k8s-artifacts-prod/ingress-nginx/controller:v0.34.1@sha256:0e072dddd1f7f8fc8909a2ca6f65e76c5f0d…`
- `eu.gcr.io/k8s-artifacts-prod/ingress-nginx/controller` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L1488](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L1488): `-   'eu.gcr.io/k8s-artifacts-prod/ingress-nginx/controller:v0.34.1@sha256:0e072dddd1f7f8fc8909a2ca6f65e76c5f0d…`
- `gcr.io/cloud-builders/docker` (medium; workflow.image-ref) — [kubernetes/ingress-nginx:images/cfssl/cloudbuild.yaml L5](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/images/cfssl/cloudbuild.yaml#L5): `- name: gcr.io/cloud-builders/docker`
- `gcr.io/google_containers/nginx-ingress-controller` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L3961](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L3961): `**Image:** 'gcr.io/google_containers/nginx-ingress-controller:0.9.0-beta.15'`
- `gcr.io/k8s-staging-test-infra/gcb-docker-gcloud` (medium; workflow.image-ref) — [kubernetes/ingress-nginx:charts/ingress-nginx/cloudbuild.yaml L5](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/cloudbuild.yaml#L5): `- name: gcr.io/k8s-staging-test-infra/gcb-docker-gcloud:v20260127-c1affcc8de`
- `k8s.gcr.io/ingress-nginx/controller` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L335](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L335): `- k8s.gcr.io/ingress-nginx/controller:v1.2.1@sha256:5516d103a9c2ecc4f026efbd4b40662ce22dc1f824fb129ed121460aaa…`
- `k8s.gcr.io/ingress-nginx/controller-chroot` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L336](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L336): `- k8s.gcr.io/ingress-nginx/controller-chroot:v1.2.1@sha256:d301551cf62bc3fb75c69fa56f7aa1d9e87b5079333adaf38af…`
- `quay.io/kubernetes-ingress-controller/nginx-ingress-controller` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L1609](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L1609): `**Image:** 'quay.io/kubernetes-ingress-controller/nginx-ingress-controller:0.33.0'`
- `registry.k8s.io/images/k8s-staging-ingress-nginx/images.yaml` (medium; docs.image-ref) — [kubernetes/ingress-nginx:MANUAL_RELEASE.md L112](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/MANUAL_RELEASE.md#L112): `- In the related branch, of your fork, edit the file /registry.k8s.io/images/k8s-staging-ingress-nginx/images.…`
- `registry.k8s.io/ingress-nginx/controller-chroot` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L15](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L15): `* registry.k8s.io/ingress-nginx/controller-chroot:v1.5.1@sha256:c1c091b88a6c936a83bd7b098662760a87868d12452529…`
- `registry.k8s.io/ingress-nginx/e2e-test-runner` (medium; make.image-ref, script.image-ref) — [kubernetes/ingress-nginx:build/run-in-docker.sh L44](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/build/run-in-docker.sh#L44): `E2E_IMAGE=${E2E_IMAGE:-registry.k8s.io/ingress-nginx/e2e-test-runner:v2.2.9@sha256:6eda6a8d17ff65c5af647abb071…`
- `registry.k8s.io/ingress-nginx/kube-webhook-certgen` (medium; docs.image-ref, manifest.image) — [kubernetes/ingress-nginx:NEW_CONTRIBUTOR.md L328](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/NEW_CONTRIBUTOR.md#L328): `▪ Using image registry.k8s.io/ingress-nginx/kube-webhook-certgen:v1.1.1`
- `registry.k8s.io/ingress-nginx/nginx` (medium; workflow.image-ref) — [kubernetes/ingress-nginx:.github/workflows/ci.yaml L184](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/ci.yaml#L184): `cd images/nginx/rootfs && docker buildx build --platform=${{ env.PLATFORMS }} --load -t registry.k8s.io/ingres…`
- `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/controller` (medium; docs.image-ref, make.image-ref, script.image-ref) — [kubernetes/ingress-nginx:MANUAL_RELEASE.md L96](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/MANUAL_RELEASE.md#L96): `pushing manifest for us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/controller:v1.0.2@sha256:e15f…`
- `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/controller-chroot` (medium; make.image-ref) — [kubernetes/ingress-nginx:Makefile L97](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Makefile#L97): `-t $(REGISTRY)/controller-chroot:$(TAG) rootfs -f rootfs/Dockerfile-chroot`
- `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx/nginx` (medium; make.image-ref) — [kubernetes/ingress-nginx:images/nginx/Makefile L18](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/images/nginx/Makefile#L18): `IMAGE ?= $(REGISTRY)/nginx`
- `us.gcr.io/k8s-artifacts-prod/ingress-nginx/controller` (medium; docs.image-ref) — [kubernetes/ingress-nginx:Changelog.md L1487](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L1487): `-   'us.gcr.io/k8s-artifacts-prod/ingress-nginx/controller:v0.34.1@sha256:0e072dddd1f7f8fc8909a2ca6f65e76c5f0d…`
- `docker.io/ingress-nginx/custom-error-pages` (low; manifest.image) — [kubernetes/ingress-nginx:docs/examples/customization/custom-errors/custom-default-backend.helm.values.yaml L8](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/customization/custom-errors/custom-default-backend.helm.values.yaml#L8): `image: ingress-nginx/custom-error-pages`
- `docker.io/library/busybox` (low; manifest.image) — [kubernetes/ingress-nginx:charts/ingress-nginx/tests/controller-daemonset_test.yaml L104](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/tests/controller-daemonset_test.yaml#L104): `image: busybox`
- `docker.io/library/registry` (low; manifest.image) — [kubernetes/ingress-nginx:docs/examples/docker-registry/deployment.yaml L25](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/docker-registry/deployment.yaml#L25): `image: registry:2.6.2`
- `gcr.io/google_containers/nginx` (low; manifest.image) — [kubernetes/ingress-nginx:docs/examples/multi-tls/multi-tls.yaml L35](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/multi-tls/multi-tls.yaml#L35): `image: gcr.io/google_containers/nginx`
- `gcr.io/k8s-staging-ingress-nginx/ext-auth-example-authsvc` (low; manifest.image) — [kubernetes/ingress-nginx:docs/examples/customization/external-auth-headers/auth-service.yaml L21](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/customization/external-auth-headers/auth-service.yaml#L21): `image: gcr.io/k8s-staging-ingress-nginx/ext-auth-example-authsvc:v1.0.0`
- … 6 more

### image-name (1)

- `defaultbackend-amd64` (medium; helm.values-image-name) — [kubernetes/ingress-nginx:charts/ingress-nginx/values.yaml L1031](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/values.yaml#L1031): `image: defaultbackend-amd64`

### manifest (21)

- `deploy/grafana/deployment.yaml` (high; manifest.workloads) — [kubernetes/ingress-nginx:deploy/grafana/deployment.yaml L2](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/grafana/deployment.yaml#L2): `kind: Deployment`
- `deploy/prometheus/deployment.yaml` (high; manifest.workloads) — [kubernetes/ingress-nginx:deploy/prometheus/deployment.yaml L2](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/prometheus/deployment.yaml#L2): `kind: Deployment`
- `deploy/static/provider/aws/deploy.yaml` (high; docs.raw-url, manifest.workloads) — [kubernetes/ingress-nginx:deploy/static/provider/aws/deploy.yaml L396](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/aws/deploy.yaml#L396): `kind: Deployment`
- `deploy/static/provider/aws/nlb-with-tls-termination/deploy.yaml` (high; docs.raw-url, manifest.workloads) — [kubernetes/ingress-nginx:deploy/static/provider/aws/nlb-with-tls-termination/deploy.yaml L405](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/aws/nlb-with-tls-termination/deploy.yaml#L405): `kind: Deployment`
- `deploy/static/provider/baremetal/deploy.yaml` (high; docs.raw-url, manifest.workloads) — [kubernetes/ingress-nginx:.github/ISSUE_TEMPLATE/bug_report.md L98](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/ISSUE_TEMPLATE/bug_report.md#L98): `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/deploy/static/provider/bareme…`
- `deploy/static/provider/cloud/deploy.yaml` (high; docs.raw-url, manifest.workloads) — [kubernetes/ingress-nginx:NEW_CONTRIBUTOR.md L653](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/NEW_CONTRIBUTOR.md#L653): `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.3.0/deploy/static/pr…`
- `deploy/static/provider/do/deploy.yaml` (high; docs.raw-url, manifest.workloads) — [kubernetes/ingress-nginx:deploy/static/provider/do/deploy.yaml L395](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/do/deploy.yaml#L395): `kind: Deployment`
- `deploy/static/provider/exoscale/deploy.yaml` (high; docs.raw-url, manifest.workloads) — [kubernetes/ingress-nginx:deploy/static/provider/exoscale/deploy.yaml L401](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/exoscale/deploy.yaml#L401): `kind: DaemonSet`
- `deploy/static/provider/kind/deploy.yaml` (high; manifest.workloads) — [kubernetes/ingress-nginx:deploy/static/provider/kind/deploy.yaml L391](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/kind/deploy.yaml#L391): `kind: Deployment`
- `deploy/static/provider/oracle/deploy.yaml` (high; manifest.workloads) — [kubernetes/ingress-nginx:deploy/static/provider/oracle/deploy.yaml L396](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/oracle/deploy.yaml#L396): `kind: Deployment`
- `deploy/static/provider/scw/deploy.yaml` (high; docs.raw-url, manifest.workloads) — [kubernetes/ingress-nginx:deploy/static/provider/scw/deploy.yaml L395](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/deploy/static/provider/scw/deploy.yaml#L395): `kind: Deployment`
- `docs/examples/http-svc.yaml` (medium; docs.raw-url) — [kubernetes/ingress-nginx:.github/ISSUE_TEMPLATE/bug_report.md L102](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/ISSUE_TEMPLATE/bug_report.md#L102): `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/http-svc.yaml`
- `docs/examples/auth/oauth-external-auth/oauth2-proxy.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/auth/oauth-external-auth/README.md L54](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/auth/oauth-external-auth/README.md#L54): `3. Configure values in the file ['oauth2-proxy.yaml'](https://raw.githubusercontent.com/kubernetes/ingress-ngi…`
- `docs/examples/auth/oauth-external-auth/vouch-proxy.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/auth/oauth-external-auth/README.md L102](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/auth/oauth-external-auth/README.md#L102): `3. Configure Vouch Proxy values in the file ['vouch-proxy.yaml'](https://raw.githubusercontent.com/kubernetes/…`
- `docs/examples/customization/custom-configuration/configmap.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/customization/custom-configuration/README.md L20](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/customization/custom-configuration/README.md#L20): `curl https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/customization/custom-config…`
- `docs/examples/customization/custom-headers/configmap-client-response.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/customization/custom-headers/README.md L36](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/customization/custom-headers/README.md#L36): `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/customization/c…`
- `docs/examples/customization/custom-headers/configmap.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/customization/custom-headers/README.md L26](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/customization/custom-headers/README.md#L26): `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/customization/c…`
- `docs/examples/customization/custom-headers/custom-headers.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/customization/custom-headers/README.md L20](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/customization/custom-headers/README.md#L20): `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/customization/c…`
- `docs/examples/docker-registry/deployment.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/docker-registry/README.md L10](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/docker-registry/README.md#L10): `kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/docker-registry…`
- `docs/examples/docker-registry/ingress-with-tls.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/docker-registry/README.md L38](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/docker-registry/README.md#L38): `wget https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/docker-registry/ingress-wit…`
- `docs/examples/docker-registry/ingress-without-tls.yaml` (low; docs.raw-url) — [kubernetes/ingress-nginx:docs/examples/docker-registry/README.md L25](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/examples/docker-registry/README.md#L25): `wget https://raw.githubusercontent.com/kubernetes/ingress-nginx/main/docs/examples/docker-registry/ingress-wit…`

### registry (8)

- `docker.io` (medium; workflow.registry-login) — [kubernetes/ingress-nginx:.github/workflows/images.yaml L186](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/images.yaml#L186): `uses: docker/login-action@b45d80f862d83dbcd57f89517bcf500b2ab88fb2 # v4.0.0`
- `us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx` (medium; make.registry-ref, make.registry-variable, script.registry-ref) — [kubernetes/ingress-nginx:Makefile L61](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Makefile#L61): `REGISTRY ?= us-central1-docker.pkg.dev/k8s-staging-images/ingress-nginx`
- `docker.io/ingress-controller` (low; script.registry-variable) — [kubernetes/ingress-nginx:test/e2e/run-chart-test.sh L51](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/test/e2e/run-chart-test.sh#L51): `export REGISTRY=ingress-controller`
- `eu.gcr.io/v2/k8s-artifacts-prod/ingress-nginx/kube-webhook-certgen/manifests/sha256` (low; docs.registry-ref) — [kubernetes/ingress-nginx:docs/troubleshooting.md L324](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/docs/troubleshooting.md#L324): `Warning  Failed     5m5s (x4 over 6m34s)   kubelet            Failed to pull image "registry.k8s.io/ingress-ng…`
- `k8s.gcr.io/kubernetes` (low; docs.registry-ref) — [kubernetes/k8s.io:registry.k8s.io/images/k8s-staging-kubernetes/README.md L20](https://github.com/kubernetes/k8s.io/blob/55167bb0a2867208070e7b5c5b90f2381b1bf685/registry.k8s.io/images/k8s-staging-kubernetes/README.md#L20): `- '{us,eu,asia}.gcr.io/k8s-artifacts-prod/kubernetes' --> 'k8s.gcr.io/kubernetes'`
- `quay.io/repository/kubernetes-ingress-controller` (low; docs.registry-ref) — [kubernetes/ingress-nginx:Changelog.md L3870](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L3870): `- Images are published to [quay.io](https://quay.io/repository/kubernetes-ingress-controller)`
- `quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller` (low; docs.registry-ref) — [kubernetes/ingress-nginx:Changelog.md L1509](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L1509): `The repository https://quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller is deprecated…`
- `quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller-arm` (low; docs.registry-ref) — [kubernetes/ingress-nginx:Changelog.md L2393](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/Changelog.md#L2393): `- [ARM image](https://quay.io/repository/kubernetes-ingress-controller/nginx-ingress-controller-arm?tab=logs)`

### release-asset (6)

- `https://github.com/kubernetes/ingress-nginx/releases/download/{{.Tag}}/kubectl-ingress-nginx_darwin_amd64.tar.gz` (high; goreleaser.archive) — [kubernetes/ingress-nginx:.goreleaser.yaml L28](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.goreleaser.yaml#L28): `name_template: "kubectl-{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/kubernetes/ingress-nginx/releases/download/{{.Tag}}/kubectl-ingress-nginx_darwin_arm64.tar.gz` (high; goreleaser.archive) — [kubernetes/ingress-nginx:.goreleaser.yaml L28](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.goreleaser.yaml#L28): `name_template: "kubectl-{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/kubernetes/ingress-nginx/releases/download/{{.Tag}}/kubectl-ingress-nginx_linux_amd64.tar.gz` (high; goreleaser.archive) — [kubernetes/ingress-nginx:.goreleaser.yaml L28](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.goreleaser.yaml#L28): `name_template: "kubectl-{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/kubernetes/ingress-nginx/releases/download/{{.Tag}}/kubectl-ingress-nginx_linux_arm64.tar.gz` (high; goreleaser.archive) — [kubernetes/ingress-nginx:.goreleaser.yaml L28](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.goreleaser.yaml#L28): `name_template: "kubectl-{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/kubernetes/ingress-nginx/releases/download/{{.Tag}}/kubectl-ingress-nginx_windows_amd64.tar.gz` (high; goreleaser.archive) — [kubernetes/ingress-nginx:.goreleaser.yaml L28](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.goreleaser.yaml#L28): `name_template: "kubectl-{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"`
- `https://github.com/kubernetes/ingress-nginx/releases/download/{{.Tag}}/kubectl-ingress-nginx_windows_arm64.tar.gz` (high; goreleaser.archive) — [kubernetes/ingress-nginx:.goreleaser.yaml L28](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.goreleaser.yaml#L28): `name_template: "kubectl-{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"`

### release-notes (2)

- `charts/ingress-nginx/changelog/helm-chart-{{.Version}}.md` (high; paths.versioned-release-notes) — [kubernetes/ingress-nginx:charts/ingress-nginx/changelog/helm-chart-4.15.1.md ](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/changelog/helm-chart-4.15.1.md): `file charts/ingress-nginx/changelog/helm-chart-4.15.1.md`
- `charts/ingress-nginx/changelog/helm-chart-{{.Version}}-beta.0.md` (medium; paths.versioned-release-notes) — [kubernetes/ingress-nginx:charts/ingress-nginx/changelog/helm-chart-4.12.0-beta.0.md ](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/changelog/helm-chart-4.12.0-beta.0.md): `file charts/ingress-nginx/changelog/helm-chart-4.12.0-beta.0.md`

### release-publisher (1)

- `goreleaser` (high; goreleaser.release, workflow.goreleaser) — [kubernetes/ingress-nginx:.github/workflows/plugin.yaml L30](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/.github/workflows/plugin.yaml#L30): `uses: goreleaser/goreleaser-action@ec59f474b9834571250b370d4735c50f8e2d1e29 # v7.0.0`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [kubernetes/ingress-nginx:SECURITY.md L1](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/SECURITY.md#L1): `# Security Policy`

### tag-scheme (1)

- `^helm-chart-(?P<version>\d+\.\d+\.\d+(?:-(?:beta\.\d+))?)$` (high; tags.ls-remote) — [kubernetes/ingress-nginx/releases/tag/helm-chart-4.15.1 refs/tags/helm-chart-4.15.1](https://github.com/kubernetes/ingress-nginx/releases/tag/helm-chart-4.15.1#refs/tags/helm-chart-4.15.1): `0a5901f3c64f11e92e487799b8da3f00cca37515 refs/tags/helm-chart-4.15.1`

### version-relation (1)

- `chart.version = {{.Version}}` (high; helm.chart-version-matches-ref) — [kubernetes/ingress-nginx:charts/ingress-nginx/Chart.yaml L23](https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.15.1/charts/ingress-nginx/Chart.yaml#L23): `version: 4.15.1`
