# Discovery report: redis

- Repository: `github.com/redis/redis` at `8.10.2` (commit `498ecd0d6d00…`)
- Generated: 2026-10-01T14:43:35Z
- LLM: not used (deterministic resolver only)
- Tags: 416 tags: prefix "", 269 stable, 42 prereleases (betaN×8, rcN×34), 105 junk; latest stable 8.10.2; lineage minor
- Strict tag pattern: `^(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$` (junk sample: 2.2-alpha0, 2.2-alpha1, 2.2-alpha2, 2.2-alpha3, 2.2-alpha4, 2.2-alpha5)
- Scanned `github.com/redis/redis@8.10.2` (source profile): 1861 files listed, 181 read
- Scanned `github.com/redis/docs@main` (docs profile): 10412 files listed, 1312 read
- Validation: done against 8.6.0, 8.6.7, 8.8.0, 8.8.3, 8.10.0, 8.10.2

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 2 | git-tags github.com/redis/redis; github-releases redis/redis |
| release-notes | found | 1 | repo-file github.com/redis/docs content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-{{.Major}}.{{.Minor}}-release-notes.md @main; github-releases redis/redis  @{{.Tag}} |
| changelog | candidates-only | 1 |  |
| helm-charts | candidates-only | 2 |  |
| registries | candidates-only | 2 |  |
| images | candidates-only | 11 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories redis/redis |
| version-relations | not-found | 0 |  |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/redis/redis/blob/8.10.2/.github/workflows/post-release-automation.yml#L42 |
| source:advisories | discovered | security.github-hosted-default | https://github.com/redis/redis/blob/8.10.2/SECURITY.md#L1 |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/redis/redis/blob/8.10.2/.github/workflows/post-release-automation.yml#L42 |
| source:release-notes-docs | unverified | notes.per-line-file-with-release-sections | https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/… |
| source:tags | discovered | versions.git-tags | https://github.com/redis/redis/releases/tag/8.10.2#refs/tags/8.10.2 |
| versioning | discovered | tags.scheme | https://github.com/redis/redis/releases/tag/8.10.2#refs/tags/8.10.2 |

Statuses: 1 historically-validated, 4 discovered, 1 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: redis
name: redis
homepage: https://github.com/redis/redis
versioning:
  scheme: semver
  tagPattern: ^(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/redis/redis
      tagPattern: ^(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: redis/redis
    priority: 1
  - id: release-notes-docs
    roles:
      - release-notes
    locator:
      kind: repo-file
      repository: github.com/redis/docs
      ref: main
      path: content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-{{.Major}}.{{.Minor}}-release-notes.md
    extract:
      type: markdown-section
      heading: ^v?{{regexQuote .Version}}$
    fallbackGroup: release-notes
    notes: 'Unverified by discovery (insufficient: 8.6.0 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?8\\.6\\.0$" in https://github.com/redis/docs/blob/main/content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.6-release-notes.md); 8.6.7 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?8\\.6\\.7$" in https://github.com/redis/docs/blob/main/content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.6-release-notes.md); 8.8.0 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?8\\.8\\.0$" in https://github.com/redis/docs/blob/main/content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.8-release-notes.md); 8.8.3 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?8\\.8\\.3$" in https://github.com/redis/docs/blob/main/content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.8-release-notes.md); …).'
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: redis/redis
      ref: '{{.Tag}}'
    priority: 1
    fallbackGroup: release-notes
    validatedAgainst:
      - 8.6.0
      - 8.6.7
      - 8.8.0
      - 8.8.3
      - 8.10.0
      - 8.10.2
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: redis/redis
    notes: 'Security policy: SECURITY.md → https://redis.io/redis-responsible-vulnerability-disclosure/'
artifacts: []
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - 8.6.0
    - 8.6.7
    - 8.8.0
    - 8.8.3
    - 8.10.0
    - 8.10.2
  notes: Proposed by automated discovery; relationship checks run against 8.6.0, 8.6.7, 8.8.0, 8.8.3, 8.10.0, 8.10.2.
```

## Open questions for the reviewer

- deps/hiredis/CHANGELOG.md stops at 1.2.0; confirm it is abandoned (release notes come from other sources).
- Ambiguity (registry-roles): 2 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no upgrade-guide, compatibility source was derived from it.

## Validation matrix

| Element | Verdict | 8.6.0 | 8.6.7 | 8.8.0 | 8.8.3 | 8.10.0 | 8.10.2 |
|---|---|---|---|---|---|---|---|
| source:release-notes-docs | insufficient | covered: covered by release-notes-github (fallback group release-note… | covered: covered by release-notes-github (fallback group release-note… | covered: covered by release-notes-github (fallback group release-note… | covered: covered by release-notes-github (fallback group release-note… | covered: covered by release-notes-github (fallback group release-note… | covered: covered by release-notes-github (fallback group release-note… |
| source:release-notes-github | validated | pass | pass | pass | pass | pass | pass |

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 416 tags: prefix "", 269 stable, 42 prereleases (betaN×8, rcN×34), 105 junk; latest stable 8.10.2; lineage minor; strict tagPattern excludes junk tags such as 2.2-alpha0, 2.2-alpha1, 2.2-alpha2, 2.2-alpha3 |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (416 tags: prefix "", 269 stable, 42 prereleases (betaN×8, rcN×34), 105 junk; latest stable 8.10.2; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-docs | include | notes.per-line-file-with-release-sections | heuristic | One notes file per release line (6 instances) with one section per release (e.g. "## Redis Open Source 8.10.1 (August 2026)"). |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| source:release-notes-docs | annotate | validate.insufficient | computed | Kept unverified: 8.6.0 covered (covered by release-notes-github (fallback group release-notes): no section matching "^v?8\\.6\\.0$" in https://github.com/redis/docs/blob/main/content/operate/oss_and_stack/stack-with-ente… |

<details><summary>12 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| changelog `deps/hiredis/CHANGELOG.md` | changelog.stale | Changelog is not maintained: newest entry 1.2.0 while the latest release is 8.10.2. |
| image `quay.io/centos/centos` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/rockylinux/rockylinux` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `mcr.microsoft.com/azurelinux/base/core` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/freebsd-12-3-release-amd64` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/redis/reloader` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/redislabs/debezium-server` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `registry.connect.redhat.com/redislabs/redis-enterprise` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `registry.connect.redhat.com/redislabs/redis-enterprise-operator` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `registry.connect.redhat.com/redislabs/services-manager` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `registry.connect.redhat.com/redislabs/call-home-client` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image-name `noble` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### changelog (1)

- `deps/hiredis/CHANGELOG.md` (high; docs.changelog) — [redis/redis:deps/hiredis/CHANGELOG.md L1](https://github.com/redis/redis/blob/8.10.2/deps/hiredis/CHANGELOG.md#L1): `## [1.2.0](https://github.com/redis/hiredis/tree/v1.2.0) - (2023-06-04)`

### docs-repo (1)

- `github.com/redis/docs` (medium; docs.docs-repo-ref) — [redis/redis:CONTRIBUTING.md L87](https://github.com/redis/redis/blob/8.10.2/CONTRIBUTING.md#L87): `https://github.com/redis/docs`

### helm-repo (2)

- `https://helm.redis.io` (medium; docs.helm-repo-add) — [redis/docs:content/operate/kubernetes/7.22/deployment/helm.md L39](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/kubernetes/7.22/deployment/helm.md#L39): `helm repo add redis https://helm.redis.io`
- `https://helm.redis.io/radar` (medium; docs.helm-repo-add) — [redis/docs:content/operate/radar/install.md L222](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/radar/install.md#L222): `helm repo add radar https://helm.redis.io/radar`

### image (10)

- `docker.io/library/freebsd-12-3-release-amd64` (medium; manifest.image) — [redis/redis:deps/jemalloc/.cirrus.yml L32](https://github.com/redis/redis/blob/8.10.2/deps/jemalloc/.cirrus.yml#L32): `image: freebsd-12-3-release-amd64`
- `docker.io/redis/reloader` (medium; docs.image-ref) — [redis/docs:content/integrate/redis-data-integration/release-notes/rdi-1-18-0.md L48](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/integrate/redis-data-integration/release-notes/rdi-1-18-0.md#L48): `- **Reloader image configuration for Helm installations**: The Helm chart's bundled Reloader controller, which…`
- `docker.io/redislabs/debezium-server` (medium; docs.image-ref) — [redis/docs:content/integrate/redis-data-integration/release-notes/rdi-1-18-1.md L19](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/integrate/redis-data-integration/release-notes/rdi-1-18-1.md#L19): `- **Debezium upgrade**: RDI now uses Debezium 3.5.0 by default and updates the default collector image to 'doc…`
- `mcr.microsoft.com/azurelinux/base/core` (medium; workflow.image-ref) — [redis/redis:.github/workflows/modules-build-flow.yml L94](https://github.com/redis/redis/blob/8.10.2/.github/workflows/modules-build-flow.yml#L94): `- { osnick: azurelinux3,     image: 'mcr.microsoft.com/azurelinux/base/core:3.0',  family: tdnf }`
- `quay.io/centos/centos` (medium; workflow.image-ref) — [redis/redis:.github/workflows/ci.yml L81](https://github.com/redis/redis/blob/8.10.2/.github/workflows/ci.yml#L81): `container: quay.io/centos/centos:stream9`
- `quay.io/rockylinux/rockylinux` (medium; workflow.image-ref) — [redis/redis:.github/workflows/modules-build-flow.yml L92](https://github.com/redis/redis/blob/8.10.2/.github/workflows/modules-build-flow.yml#L92): `- { osnick: rocky10,         image: 'quay.io/rockylinux/rockylinux:10',            family: dnf  }`
- `registry.connect.redhat.com/redislabs/call-home-client` (medium; docs.image-ref) — [redis/docs:content/operate/kubernetes/release-notes/7-22-0-releases/7-22-0-11-june2025.md L31](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/kubernetes/release-notes/7-22-0-releases/7-22-0-11-june2025.md#L31): `- **Call Home Client**: 'registry.connect.redhat.com/redislabs/call-home-client:7.22.0-11'`
- `registry.connect.redhat.com/redislabs/redis-enterprise` (medium; docs.image-ref) — [redis/docs:content/operate/kubernetes/release-notes/7-2-4-releases/7-2-4-12-03-24.md L87](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/kubernetes/release-notes/7-2-4-releases/7-2-4-12-03-24.md#L87): `- **Redis Enterprise**: 'registry.connect.redhat.com/redislabs/redis-enterprise:7.2.4-105.rhel8-openshift'`
- `registry.connect.redhat.com/redislabs/redis-enterprise-operator` (medium; docs.image-ref) — [redis/docs:content/operate/kubernetes/release-notes/7-2-4-releases/7-2-4-12-03-24.md L89](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/kubernetes/release-notes/7-2-4-releases/7-2-4-12-03-24.md#L89): `- **Operator**: 'registry.connect.redhat.com/redislabs/redis-enterprise-operator:7.2.4-12'`
- `registry.connect.redhat.com/redislabs/services-manager` (medium; docs.image-ref) — [redis/docs:content/operate/kubernetes/release-notes/7-2-4-releases/7-2-4-12-03-24.md L90](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/kubernetes/release-notes/7-2-4-releases/7-2-4-12-03-24.md#L90): `- **Services Rigger**: 'registry.connect.redhat.com/redislabs/services-manager:7.2.4-12'`

### image-name (1)

- `noble` (low; dockerfile.name) — [redis/redis:docker/Dockerfile.noble L27](https://github.com/redis/redis/blob/8.10.2/docker/Dockerfile.noble#L27): `FROM ubuntu:24.04`

### registry (2)

- `docker.io/redislabs/radar` (low; docs.registry-ref) — [redis/docs:content/operate/radar/install.md L411](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/radar/install.md#L411): `docker://docker.io/redislabs/radar:app-v<version> \`
- `registry.example.com/redislabs/radar` (low; docs.registry-ref) — [redis/docs:content/operate/radar/install.md L412](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/radar/install.md#L412): `docker://registry.example.com/redislabs/radar:app-v<version>`

### release-notes (1)

- `content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-{{.Major}}.{{.Minor}}-release-notes.md` (high; paths.versioned-release-notes) — [redis/docs:content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.10-release-notes.md ](https://github.com/redis/docs/blob/04b014adee6c57d9ac6329b2690439e92a59d922/content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.10-release-notes.md): `file content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.10-release-notes.md`

### release-publisher (1)

- `gh release upload` (high; workflow.release-upload) — [redis/redis:.github/workflows/post-release-automation.yml L42](https://github.com/redis/redis/blob/8.10.2/.github/workflows/post-release-automation.yml#L42): `contents: write          # required for 'gh release upload'`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [redis/redis:SECURITY.md L1](https://github.com/redis/redis/blob/8.10.2/SECURITY.md#L1): `# Security Policy`

### tag-scheme (1)

- `^(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$` (high; tags.ls-remote) — [redis/redis/releases/tag/8.10.2 refs/tags/8.10.2](https://github.com/redis/redis/releases/tag/8.10.2#refs/tags/8.10.2): `498ecd0d6d007db11ddb3aea9428552598a78622 refs/tags/8.10.2`
