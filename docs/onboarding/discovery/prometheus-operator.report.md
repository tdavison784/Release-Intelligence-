# Discovery report: prometheus-operator

- Repository: `github.com/prometheus-operator/prometheus-operator` at `v0.94.1` (commit `24929ab3b4c9…`)
- Generated: 2026-10-01T15:56:52Z
- LLM: not used (deterministic resolver only)
- Tags: 381 tags: prefix "v", 175 stable, 3 prereleases (rc.N×3), 203 junk; latest stable v0.94.1; lineage minor
- Scanned `github.com/prometheus-operator/prometheus-operator@v0.94.1` (source profile): 1648 files listed, 215 read
- Scanned `github.com/prometheus-operator/website@main` (docs profile): 48 files listed, 2 read
- Validation: done against v0.92.0, v0.92.1, v0.93.0, v0.93.1, v0.94.0, v0.94.1

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 4 | git-tags github.com/prometheus-operator/prometheus-operator; github-releases prometheus-operator/prometheus-operator |
| release-notes | found | 0 | github-releases prometheus-operator/prometheus-operator  @{{.Tag}} |
| changelog | found | 1 | repo-file github.com/prometheus-operator/prometheus-operator CHANGELOG.md |
| helm-charts | not-found | 0 |  |
| registries | found | 5 | quay.io/prometheus-operator |
| images | found | 14 | quay.io/prometheus-operator/admission-webhook:{{.Tag}}; quay.io/prometheus-operator/prometheus-config-reloader:{{.Tag}}; quay.io/prometheus-operator/prometheus-operator:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories prometheus-operator/prometheus-operator |
| version-relations | found | 2 | bundle: version = {{.Tag}}; bundle-manifest: version = {{.Tag}}; admission-webhook: version = {{.Tag}}; prometheus-config-reloader: version = {{.Tag}}; prometheus-operator: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:admission-webhook | historically-validated | image.tag-from-release | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/platform/webhook.md#L134 |
| artifact:bundle | historically-validated | manifest.in-repo-install | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/bundle.yaml#L77023 |
| artifact:bundle-manifest | historically-validated | asset.hosted-release-download | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/getting-started/installation.md#L38 |
| artifact:prometheus-config-reloader | historically-validated | image.tag-from-release | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/platform/operator.md#L104 |
| artifact:prometheus-operator | historically-validated | image.tag-from-release | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Makefile#L23 |
| source:changelog | historically-validated | changelog.file | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/CHANGELOG.md#L1 |
| source:advisories | discovered | security.github-hosted-default | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/SECURITY.md#L1 |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/.github/workflows/release.yaml#L22 (+1) |
| source:release-notes-github | unverified | notes.hosted-release-body | https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/.github/workflows/release.yaml#L22 (+1) |
| source:tags | discovered | versions.git-tags | https://github.com/prometheus-operator/prometheus-operator/releases/tag/v0.94.1#refs/tags/v0.94.1 |
| versioning | discovered | tags.scheme | https://github.com/prometheus-operator/prometheus-operator/releases/tag/v0.94.1#refs/tags/v0.94.1 (+1) |

Statuses: 6 historically-validated, 4 discovered, 1 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: prometheus-operator
name: prometheus-operator
homepage: https://github.com/prometheus-operator/prometheus-operator
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
      repository: github.com/prometheus-operator/prometheus-operator
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: prometheus-operator/prometheus-operator
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: prometheus-operator/prometheus-operator
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    notes: 'Unverified by discovery (unverifiable: 0.92.0 unverifiable (fetch https://api.github.com/repos/prometheus-operator/prometheus-operator/releases/tags/v0.92.0: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); 0.92.1 unverifiable (fetch https://api.github.com/repos/prometheus-operator/prometheus-operator/releases/tags/v0.92.1: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); 0.93.0 unverifiable (fetch https://api.github.com/repos/prometheus-operator/prometheus-operator/releases/tags/v0.93.0: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); 0.93.1 unverifiable (fetch https://api.github.com/repos/prometheus-operator/prometheus-operator/releases/tags/v0.93.1: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But here''s the good news: Authenticated requests get a higher rate limit. Check out the documentation for more details.)","documentation_url":"ht…); …).'
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/prometheus-operator/prometheus-operator
      path: CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    validatedAgainst:
      - 0.92.0
      - 0.92.1
      - 0.93.0
      - 0.93.1
      - 0.94.0
      - 0.94.1
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: prometheus-operator/prometheus-operator
    notes: 'Security policy: SECURITY.md → https://prometheus.io/docs/operating/security/#automated-security-scanners'
artifacts:
  - id: bundle
    type: manifest
    name: bundle.yaml
    description: Install manifest (also bundles the CRDs).
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-file
        repository: github.com/prometheus-operator/prometheus-operator
        path: bundle.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 0.92.0
      - 0.92.1
      - 0.93.0
      - 0.93.1
      - 0.94.0
      - 0.94.1
  - id: bundle-manifest
    type: manifest
    name: bundle.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: http
        url: https://github.com/prometheus-operator/prometheus-operator/releases/download/{{.Tag}}/bundle.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 0.92.0
      - 0.92.1
      - 0.93.0
      - 0.93.1
      - 0.94.0
      - 0.94.1
  - id: admission-webhook
    type: container-image
    name: admission-webhook
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/prometheus-operator/admission-webhook
    validatedAgainst:
      - 0.92.0
      - 0.92.1
      - 0.93.0
      - 0.93.1
      - 0.94.0
      - 0.94.1
  - id: prometheus-config-reloader
    type: container-image
    name: prometheus-config-reloader
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/prometheus-operator/prometheus-config-reloader
    validatedAgainst:
      - 0.92.0
      - 0.92.1
      - 0.93.0
      - 0.93.1
      - 0.94.0
      - 0.94.1
  - id: prometheus-operator
    type: container-image
    name: prometheus-operator
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/prometheus-operator/prometheus-operator
    references:
      - artifact: bundle
        pattern: quay.io/prometheus-operator/prometheus-operator:{{.Tag}}
    validatedAgainst:
      - 0.92.0
      - 0.92.1
      - 0.93.0
      - 0.93.1
      - 0.94.0
      - 0.94.1
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - v0.92.0
    - v0.92.1
    - v0.93.0
    - v0.93.1
    - v0.94.0
    - v0.94.1
  notes: Proposed by automated discovery; relationship checks run against v0.92.0, v0.92.1, v0.93.0, v0.93.1, v0.94.0, v0.94.1.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 5 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.

## Validation matrix

| Element | Verdict | v0.92.0 | v0.92.1 | v0.93.0 | v0.93.1 | v0.94.0 | v0.94.1 |
|---|---|---|---|---|---|---|---|
| artifact:admission-webhook | validated |  |  |  |  |  |  |
| artifact:bundle | validated |  |  |  |  |  |  |
| artifact:bundle-manifest | validated |  |  |  |  |  |  |
| artifact:prometheus-config-reloader | validated |  |  |  |  |  |  |
| artifact:prometheus-operator | validated |  |  |  |  |  |  |
| content:bundle-manifest/image-refs | validated |  |  |  |  |  |  |
| content:bundle/image-refs | validated |  |  |  |  |  |  |
| source:changelog | validated |  |  |  |  |  |  |
| source:release-notes-github | unverifiable |  |  |  |  |  |  |

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 381 tags: prefix "v", 175 stable, 3 prereleases (rc.N×3), 203 junk; latest stable v0.94.1; lineage minor |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (381 tags: prefix "v", 175 stable, 3 prereleases (rc.N×3), 203 junk; latest stable v0.94.1; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 0.94.1 (heading "## 0.94.1 / 2026-09-23"). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:bundle | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:bundle-manifest | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:admission-webhook | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:prometheus-config-reloader | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:prometheus-operator | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| source:release-notes-github | annotate | validate.unverifiable | computed | Kept unverified: 0.92.0 unverifiable (fetch https://api.github.com/repos/prometheus-operator/prometheus-operator/releases/tags/v0.92.0: unavailable (HTTP 403): {"message":"API rate limit exceeded for 69.9.221.149. (But h… |

<details><summary>32 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| crd `bundle.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagerconfigs.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagers.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_podmonitors.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_probes.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusagents.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_prometheuses.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusrules.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_scrapeconfigs.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_servicemonitors.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd-full/monitoring.coreos.com_thanosrulers.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_alertmanagerconfigs.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_alertmanagers.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_podmonitors.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_probes.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_prometheusagents.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_prometheuses.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_scrapeconfigs.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `example/prometheus-operator-crd/monitoring.coreos.com_thanosrulers.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| image `quay.io/brancz/prometheus-example-app` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/prometheus/alertmanager` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/prometheus/prometheus` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/prometheus-operator/prometheus-alertmanager-test-webhook` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/thanos/thanos` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/prometheus-operator/prometheus-operator-dev` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `quay.io/prometheus-operator/prometheus-config-reloader-dev` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `quay.io/prometheus-operator/admission-webhook-dev` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `quay.io/prometheus-operator/instrumented-sample-app` | image.test-only | Only referenced from test, sample or documentation files. |
| image `gcr.io/google_containers/defaultbackend` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/google_containers/nginx-ingress-controller` | image.third-party | Image is not named after the product (dependency or tooling image). |

</details>

## Candidates


### build-tool (1)

- `cosign` (high; workflow.cosign) — [prometheus-operator/prometheus-operator:.github/workflows/publish.yaml L39](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/.github/workflows/publish.yaml#L39): `uses: sigstore/cosign-installer@6f9f17788090df1f26f669e9d70d6ae9567deba6 # v4.1.2`

### changelog (1)

- `CHANGELOG.md` (high; docs.changelog) — [prometheus-operator/prometheus-operator:CHANGELOG.md L1](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/CHANGELOG.md#L1): `## 0.94.1 / 2026-09-23`

### crd (21)

- `bundle.yaml` (high; crd.file) — [prometheus-operator/prometheus-operator:bundle.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/bundle.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagerconfigs.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagerconfigs.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagerconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagers.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagers.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_alertmanagers.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_podmonitors.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_podmonitors.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_podmonitors.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_probes.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_probes.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_probes.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusagents.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusagents.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusagents.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_prometheuses.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_prometheuses.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_prometheuses.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusrules.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusrules.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_prometheusrules.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_scrapeconfigs.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_scrapeconfigs.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_scrapeconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_servicemonitors.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_servicemonitors.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_servicemonitors.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd-full/monitoring.coreos.com_thanosrulers.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd-full/monitoring.coreos.com_thanosrulers.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd-full/monitoring.coreos.com_thanosrulers.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_alertmanagerconfigs.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_alertmanagerconfigs.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_alertmanagerconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_alertmanagers.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_alertmanagers.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_alertmanagers.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_podmonitors.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_podmonitors.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_podmonitors.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_probes.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_probes.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_probes.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_prometheusagents.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_prometheusagents.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_prometheusagents.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_prometheuses.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_prometheuses.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_prometheuses.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_prometheusrules.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_scrapeconfigs.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_scrapeconfigs.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_scrapeconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_servicemonitors.yaml#L3): `kind: CustomResourceDefinition`
- `example/prometheus-operator-crd/monitoring.coreos.com_thanosrulers.yaml` (low; crd.file) — [prometheus-operator/prometheus-operator:example/prometheus-operator-crd/monitoring.coreos.com_thanosrulers.yaml L3](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/prometheus-operator-crd/monitoring.coreos.com_thanosrulers.yaml#L3): `kind: CustomResourceDefinition`

### docs-repo (1)

- `github.com/prometheus-operator/website` (medium; docs.docs-repo-ref) — [prometheus-operator/prometheus-operator:RELEASE.md L169](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/RELEASE.md#L169): `Bump the operator's version in the [website](https://github.com/prometheus-operator/website/blob/main/data/pro…`

### image (14)

- `quay.io/brancz/prometheus-example-app` (medium; docs.image-ref, manifest.image) — [prometheus-operator/prometheus-operator:Documentation/developer/getting-started.md L54](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/developer/getting-started.md#L54): `image: quay.io/brancz/prometheus-example-app:v0.5.0`
- `quay.io/prometheus-operator/admission-webhook` (medium; docs.image-ref, make.image-ref, manifest.image, script.image-ref) — [prometheus-operator/prometheus-operator:Documentation/platform/webhook.md L134](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/platform/webhook.md#L134): `image: quay.io/prometheus-operator/admission-webhook:v0.94.1`
- `quay.io/prometheus-operator/admission-webhook-dev` (medium; script.image-ref) — [prometheus-operator/prometheus-operator:scripts/push-docker-image.sh L57](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/scripts/push-docker-image.sh#L57): `WEBHOOKS="$i/${IMAGE_WEBHOOK}${IMAGE_SUFFIX} ${WEBHOOKS}"`
- `quay.io/prometheus-operator/prometheus-config-reloader` (medium; docs.image-ref, make.image-ref, script.image-ref) — [prometheus-operator/prometheus-operator:Documentation/platform/operator.md L104](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/platform/operator.md#L104): `Prometheus config reloader image (default "quay.io/prometheus-operator/prometheus-config-reloader:v0.94.1")`
- `quay.io/prometheus-operator/prometheus-config-reloader-dev` (medium; script.image-ref) — [prometheus-operator/prometheus-operator:scripts/push-docker-image.sh L56](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/scripts/push-docker-image.sh#L56): `RELOADERS="$i/${IMAGE_RELOADER}${IMAGE_SUFFIX} ${RELOADERS}"`
- `quay.io/prometheus-operator/prometheus-operator` (medium; docs.image-ref, make.image-ref, manifest.image, script.image-ref) — [prometheus-operator/prometheus-operator:Makefile L23](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Makefile#L23): `IMAGE_OPERATOR?=quay.io/prometheus-operator/prometheus-operator`
- `quay.io/prometheus-operator/prometheus-operator-dev` (medium; script.image-ref) — [prometheus-operator/prometheus-operator:scripts/push-docker-image.sh L55](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/scripts/push-docker-image.sh#L55): `OPERATORS="$i/${IMAGE_OPERATOR}${IMAGE_SUFFIX} ${OPERATORS}"`
- `quay.io/prometheus/alertmanager` (medium; docs.image-ref) — [prometheus-operator/prometheus-operator:Documentation/platform/operator.md L30](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/platform/operator.md#L30): `Alertmanager default base image (path without tag/version) (default "quay.io/prometheus/alertmanager")`
- `quay.io/prometheus/prometheus` (medium; docs.image-ref) — [prometheus-operator/prometheus-operator:Documentation/platform/operator.md L106](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/platform/operator.md#L106): `Prometheus default base image (path without tag/version) (default "quay.io/prometheus/prometheus")`
- `gcr.io/google_containers/defaultbackend` (low; manifest.image) — [prometheus-operator/prometheus-operator:test/framework/resources/default-http-backend.yml L20](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/test/framework/resources/default-http-backend.yml#L20): `image: gcr.io/google_containers/defaultbackend:1.2`
- `gcr.io/google_containers/nginx-ingress-controller` (low; manifest.image) — [prometheus-operator/prometheus-operator:test/framework/resources/nxginx-ingress-controller.yml L19](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/test/framework/resources/nxginx-ingress-controller.yml#L19): `- image: gcr.io/google_containers/nginx-ingress-controller:0.8.3`
- `quay.io/prometheus-operator/instrumented-sample-app` (low; docs.image-ref, make.image-ref, manifest.image) — [prometheus-operator/prometheus-operator:test/framework/resources/basic-app-for-daemonset-test.yaml L19](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/test/framework/resources/basic-app-for-daemonset-test.yaml#L19): `image: quay.io/prometheus-operator/instrumented-sample-app:latest`
- `quay.io/prometheus-operator/prometheus-alertmanager-test-webhook` (low; docs.image-ref, make.image-ref) — [prometheus-operator/prometheus-operator:example/alertmanager-webhook/Makefile L1](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/alertmanager-webhook/Makefile#L1): `IMAGE ?= quay.io/prometheus-operator/prometheus-alertmanager-test-webhook`
- `quay.io/thanos/thanos` (low; manifest.image) — [prometheus-operator/prometheus-operator:example/thanos/query-deployment.yaml L26](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/thanos/query-deployment.yaml#L26): `image: quay.io/thanos/thanos:v0.42.4`

### manifest (4)

- `bundle.yaml` (medium; manifest.workloads) — [prometheus-operator/prometheus-operator:bundle.yaml L77023](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/bundle.yaml#L77023): `kind: Deployment`
- `example/user-guides/alerting/alertmanager-config-example.yaml` (medium; docs.raw-url) — [prometheus-operator/prometheus-operator:Documentation/developer/alerting.md L124](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/developer/alerting.md#L124): `curl -sL https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/main/example/user-guides/al…`
- `tags/$LATEST/bundle.yaml` (medium; docs.raw-url) — [prometheus-operator/prometheus-operator:Documentation/getting-started/installation.md L48](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/getting-started/installation.md#L48): `curl -s "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/refs/tags/$LATEST/bundle.ya…`
- `tags/$LATEST/kustomization.yaml` (medium; docs.raw-url) — [prometheus-operator/prometheus-operator:Documentation/getting-started/installation.md L47](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/getting-started/installation.md#L47): `curl -s "https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/refs/tags/$LATEST/kustomiza…`

### registry (5)

- `ghcr.io` (high; workflow.registry-login) — [prometheus-operator/prometheus-operator:.github/workflows/publish.yaml L55](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/.github/workflows/publish.yaml#L55): `registry: ghcr.io`
- `quay.io` (high; workflow.registry-login) — [prometheus-operator/prometheus-operator:.github/workflows/publish.yaml L49](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/.github/workflows/publish.yaml#L49): `registry: quay.io`
- `quay.io/organization/prometheus-operator` (low; docs.registry-ref) — [prometheus-operator/prometheus-operator:RELEASE.md L157](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/RELEASE.md#L157): `Once a tag is created, the 'publish' Github action will push the container images to [quay.io](https://quay.io…`
- `quay.io/prometheus-operator/prometheus-alertmanager-test-webhook` (low; make.registry-ref) — [prometheus-operator/prometheus-operator:example/alertmanager-webhook/Makefile L14](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/example/alertmanager-webhook/Makefile#L14): `buildah manifest push --all $(IMAGE):$(TAG) docker://$(IMAGE):$(TAG)`
- `quay.io/repository/prometheus-operator/prometheus-operator` (low; docs.registry-ref) — [prometheus-operator/prometheus-operator:README.md L5](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/README.md#L5): `[![Latest Release](https://img.shields.io/github/v/release/prometheus-operator/prometheus-operator)](https://q…`

### release-asset (1)

- `https://github.com/prometheus-operator/prometheus-operator/releases/download/{{.Tag}}/bundle.yaml` (medium; docs.release-asset-url) — [prometheus-operator/prometheus-operator:Documentation/getting-started/installation.md L38](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Documentation/getting-started/installation.md#L38): `curl -sL https://github.com/prometheus-operator/prometheus-operator/releases/download/${LATEST}/bundle.yaml | …`

### release-publisher (1)

- `svenstaro/upload-release-action` (high; workflow.release-upload) — [prometheus-operator/prometheus-operator:.github/workflows/release.yaml L22](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/.github/workflows/release.yaml#L22): `uses: svenstaro/upload-release-action@29e53e917877a24fad85510ded594ab3c9ca12de # 2.11.5`

### release-trigger (1)

- `v*` (high; workflow.tag-trigger) — [prometheus-operator/prometheus-operator:.github/workflows/checks.yaml L10](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/.github/workflows/checks.yaml#L10): `- 'v*'`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [prometheus-operator/prometheus-operator:SECURITY.md L1](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/SECURITY.md#L1): `# Security`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?)$` (high; tags.ls-remote) — [prometheus-operator/prometheus-operator/releases/tag/v0.94.1 refs/tags/v0.94.1](https://github.com/prometheus-operator/prometheus-operator/releases/tag/v0.94.1#refs/tags/v0.94.1): `24929ab3b4c91b39d43d5f020833296654c627fd refs/tags/v0.94.1`

### version-relation (2)

- `binary.version = {{.Version}}` (high; make.ldflags-version) — [prometheus-operator/prometheus-operator:Makefile L100](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/Makefile#L100): `-X $(PROMETHEUS_COMMON_PKG)/version.Version=$(VERSION)`
- `VERSION file = {{.Version}}` (medium; version-file.matches-ref) — [prometheus-operator/prometheus-operator:VERSION L1](https://github.com/prometheus-operator/prometheus-operator/blob/v0.94.1/VERSION#L1): `0.94.1`
