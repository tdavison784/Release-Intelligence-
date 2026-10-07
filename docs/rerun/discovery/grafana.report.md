# Discovery report: grafana

- Repository: `github.com/grafana/grafana` at `v13.2.3` (commit `6193dc03311b…`)
- Generated: 2026-10-02T12:35:00Z
- LLM: not used (deterministic resolver only)
- Tags: 739 tags: prefix "v", 551 stable, 78 prereleases (betaN×75, rcN×3), 110 junk; latest stable v13.2.3; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$` (junk sample: 1.0.0, 6.1.6, 7.0.0, 7.2.1, dupa, list)
- Scanned `github.com/grafana/grafana@v13.2.3` (source profile): 22415 files listed, 1942 read
- Validation: done against v13.0.0, v13.0.10, v13.1.0, v13.1.7, v13.2.0, v13.2.3

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 2 | git-tags github.com/grafana/grafana; github-releases grafana/grafana |
| release-notes | found | 0 | github-releases grafana/grafana  @{{.Tag}} |
| changelog | candidates-only | 3 |  |
| helm-charts | candidates-only | 3 |  |
| registries | candidates-only | 44 |  |
| images | candidates-only | 64 |  |
| upgrade-docs | candidates-only | 1 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories grafana/grafana |
| version-relations | found | 0 | crds: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:crds | historically-validated | crd.in-repo | https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/notifications/definitions/receiver.notifications.alerting.grafana.app.yaml#L2 (+2) |
| artifact:grafana-chart | unverified | chart.external-lookup-appversion | https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/administration/enterprise-licensing/activate-aws-marketplace-license/activate-license-on-… |
| source:advisories | discovered | security.github-hosted-default |  |
| source:changelog | unverified | changelog.file | https://github.com/grafana/grafana/blob/v13.2.3/CHANGELOG.md#L3 |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/grafana/grafana/blob/v13.2.3/.github/workflows/sbom-on-release.yml#L85 |
| source:release-notes-github | exception | notes.hosted-release-body | https://github.com/grafana/grafana/blob/v13.2.3/.github/workflows/sbom-on-release.yml#L85 |
| source:tags | discovered | versions.git-tags | https://github.com/grafana/grafana/releases/tag/v13.2.3#refs/tags/v13.2.3 |
| source:upgrade-guide | unverified | upgrade.versioned-doc | https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/upgrade-guide/upgrade-v13.2/index.md# |
| versioning | discovered | tags.scheme | https://github.com/grafana/grafana/releases/tag/v13.2.3#refs/tags/v13.2.3 |

Statuses: 1 historically-validated, 4 discovered, 3 unverified, 1 exception.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: grafana
name: grafana
homepage: https://github.com/grafana/grafana
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/grafana/grafana
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: grafana/grafana
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: grafana/grafana
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    notes: 'Failed validation for one release (13.0.0 fail (fetch https://api.github.com/repos/grafana/grafana/releases/tags/v13.0.0: not found (HTTP 404))); possibly published late or elsewhere.'
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: grafana/grafana
artifacts:
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/grafana/grafana
        path: apps/alerting/notifications/definitions
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 13.0.0
      - 13.0.10
      - 13.1.0
      - 13.1.7
      - 13.2.0
      - 13.2.3
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - v13.0.0
    - v13.0.10
    - v13.1.0
    - v13.1.7
    - v13.2.0
    - v13.2.3
  notes: Proposed by automated discovery; relationship checks run against v13.0.0, v13.0.10, v13.1.0, v13.1.7, v13.2.0, v13.2.3.
```

## Open questions for the reviewer

- source:release-notes-github failed validation for one release and was kept: Holds for 5 releases but failed for one (13.0.0 fail (fetch https://api.github.com/repos/grafana/grafana/releases/tags/v13.0.0: not found (HTTP 404))); kept with a comment for review.
- Ambiguity (registry-roles): 44 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (chart-version): How do the versions of chart(s) grafana relate to the release version?

## Validation matrix

| Element | Verdict | v13.0.0 | v13.0.10 | v13.1.0 | v13.1.7 | v13.2.0 | v13.2.3 |
|---|---|---|---|---|---|---|---|
| artifact:crds | validated |  |  |  |  |  |  |
| artifact:grafana-chart | failing |  |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |  |
| source:changelog | failing |  |  |  |  |  |  |
| source:release-notes-github | failing |  |  |  |  |  |  |
| source:upgrade-guide | failing |  |  |  |  |  |  |

## Dropped elements

- `source:changelog` (deterministic): failed validation: 13.0.0 fail (no section matching "^\\[?v?13\\.0\\.0\\]?(\\s|$)" in https://github.com/grafana/grafana/blob/v13.0.0/CHANGELOG.md); 13.1.0 fail (no section matching "^\\[?v?13\\.1\\.0\\]?(\\s|$)" in https://github.com/grafana/grafana/blob/v13.1.0/CHANGELOG.md); 13.2.0 fail (no section matching "^\\[?v?13\\.2\\.0\\]?(\\s|$)" in https://github.com/grafana/grafana/blob/v13.2.0/CHANGELOG.md)
- `source:upgrade-guide` (deterministic): failed validation: 13.0.0 fail (fetch https://raw.githubusercontent.com/grafana/grafana/v13.0.0/docs/sources/upgrade-guide/upgrade-v13.0/index.md: not found (HTTP 404)); 13.0.10 not-applicable (not applicable to 13.0.10: release kinds minor,major (this is a patch release)); 13.1.0 fail (fetch https://raw.githubusercontent.com/grafana/grafana/v13.1.0/docs/sources/upgrade-guide/upgrade-v13.1/index.md: not found (HTTP 404)); 13.1.7 not-applicable (not applicable to 13.1.7: release kinds minor,major (this is a patch release)); …
- `artifact:grafana-chart` (deterministic): failed validation: 13.0.0 fail (no chart version ships appVersion v13.0.0 (https://grafana-community.github.io/helm-charts (chart grafana) lists 851 versions)); 13.0.10 fail (no chart version ships appVersion v13.0.10 (https://grafana-community.github.io/helm-charts (chart grafana) lists 851 versions)); 13.1.0 fail (no chart version ships appVersion v13.1.0 (https://grafana-community.github.io/helm-charts (chart grafana) lists 851 versions)); 13.1.7 fail (no chart version ships appVersion v13.1.7 (https://grafana-community.github.io/helm-charts (chart grafana) lists 851 versions)); …

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 739 tags: prefix "v", 551 stable, 78 prereleases (betaN×75, rcN×3), 110 junk; latest stable v13.2.3; lineage minor; strict tagPattern excludes junk tags such as 1.0.0, 6.1.6, 7.0.0, 7.2.1 |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (739 tags: prefix "v", 551 stable, 78 prereleases (betaN×75, rcN×3), 110 junk; latest stable v13.2.3; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:changelog | include | changelog.file | heuristic | Changelog with 41 version sections, newest 13.2.3 (heading "# 13.2.3 (2026-09-29)"). |
| source:upgrade-guide | include | upgrade.versioned-doc | heuristic | One upgrade document per release line (32 instances, newest docs/sources/upgrade-guide/upgrade-v13.2/index.md). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:grafana-chart | include | chart.external-lookup-appversion | heuristic | The chart lives in a separate repository with independent versions; the chart release for a product release is looked up by appVersion == release tag (GitHub Pages URL by the chart-releaser convention). |
| artifact:crds | include | crd.in-repo | heuristic | 4 CRDs of the product's API groups in apps/alerting/notifications/definitions at the release tag. |
| source:release-notes-github | annotate | validate.single-failure-kept | computed | Holds for 5 releases but failed for one (13.0.0 fail (fetch https://api.github.com/repos/grafana/grafana/releases/tags/v13.0.0: not found (HTTP 404))); kept with a comment for review. |
| source:changelog | drop | validate.failed | computed | Relationship check failed: 13.0.0 fail (no section matching "^\\[?v?13\\.0\\.0\\]?(\\s\|$)" in https://github.com/grafana/grafana/blob/v13.0.0/CHANGELOG.md); 13.1.0 fail (no section matching "^\\[?v?13\\.1\\.0\\]?(\\s\|$)"… |
| source:upgrade-guide | drop | validate.failed | computed | Relationship check failed: 13.0.0 fail (fetch https://raw.githubusercontent.com/grafana/grafana/v13.0.0/docs/sources/upgrade-guide/upgrade-v13.0/index.md: not found (HTTP 404)); 13.0.10 not-applicable (not applicable to … |
| artifact:grafana-chart | drop | validate.failed | computed | Relationship check failed: 13.0.0 fail (no chart version ships appVersion v13.0.0 (https://grafana-community.github.io/helm-charts (chart grafana) lists 851 versions)); 13.0.10 fail (no chart version ships appVersion v13… |

<details><summary>66 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| changelog `packages/grafana-ui/CHANGELOG.md` | changelog.not-primary | Only the top-level changelog of the product repository is used. |
| changelog `pkg/services/ngalert/CHANGELOG.md` | changelog.not-primary | Only the top-level changelog of the product repository is used. |
| image `docker.io/tonistiigi/binfmt` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `us-docker.pkg.dev/grafanalabs-global/docker-grafana-bench-prod/grafana-bench` | image.test-only | Only referenced from test, sample or documentation files. |
| image `us-docker.pkg.dev/grafanalabs-dev/docker-grafana-dev/grafana` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `gcr.io/relyance-ext/compliance_inspector` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/grafana/generate-policy-bot-config` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/library/postgres` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/redis` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/goauthentik/ldap` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/freeipa/freeipa-server` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/keycloak/keycloak` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/bitnami/oauth2-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/osixia/openldap` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/oauth2-proxy/oauth2-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/clickhouse/clickhouse-server` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/bitnami/etcd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/grafana-image-renderer` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/graphiteapp/graphite-statsd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/fake-data-gen` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `docker.io/library/influxdb` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/telegraf` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/jaegertracing/all-in-one` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/loki` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned). |
| image `docker.io/grafana/promtail` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/maildev/maildev` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/mariadb` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/memcached` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/mimir-alpine` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `docker.io/nginxinc/nginx-unprivileged` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `mcr.microsoft.com/mssql/server` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `mcr.microsoft.com/azure-sql-edge` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/petergrace/opentsdb-docker` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/node-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/alertmanager` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/pyroscope` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned). |
| image `docker.io/prom/prometheus` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/tempo` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned). |
| image `docker.io/grafana/alloy` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/sensu/sensu` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/namshi/smtp` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/tns-db` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `docker.io/grafana/tns-app` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `docker.io/grafana/tns-loadgen` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `docker.io/library/traefik` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/grafana` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `docker.io/sigoden/dufs` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/openzipkin/brave-example` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/openzipkin/zipkin` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/mysql` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/mysqld-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/jwilder/nginx-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/webhook-receiver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/grafana-dev` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/grafana/fluent-plugin-loki` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/library/grafana-proxy` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `docker.io/library/grafana-fs-dev` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `docker.io/gofeatureflag/go-feature-flag` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/verdaccio/verdaccio` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.access.redhat.com/ubi9/ubi` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image-name `grafana-fs-dev` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `proxy` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `deb` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |
| image-name `rpm` | image-name.test-or-weak | Image name only from test/sample builds or a lone Dockerfile suffix. |

</details>

## Candidates


### changelog (3)

- `CHANGELOG.md` (high; docs.changelog) — [grafana/grafana:CHANGELOG.md L3](https://github.com/grafana/grafana/blob/v13.2.3/CHANGELOG.md#L3): `# 13.2.3 (2026-09-29)`
- `packages/grafana-ui/CHANGELOG.md` (high; docs.changelog) — [grafana/grafana:packages/grafana-ui/CHANGELOG.md L1](https://github.com/grafana/grafana/blob/v13.2.3/packages/grafana-ui/CHANGELOG.md#L1): `# 7.3.0-beta1 (2020-10-15)`
- `pkg/services/ngalert/CHANGELOG.md` (high; docs.changelog) — [grafana/grafana:pkg/services/ngalert/CHANGELOG.md L42](https://github.com/grafana/grafana/blob/v13.2.3/pkg/services/ngalert/CHANGELOG.md#L42): `## 9.0.0`

### chart-repo (1)

- `github.com/grafana/helm-charts` (medium; docs.chart-repo-ref) — [grafana/grafana:docs/sources/administration/enterprise-licensing/activate-aws-marketplace-license/activate-license-on-eks/index.md L38](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/administration/enterprise-licensing/activate-aws-marketplace-license/activate-license-on-eks/index.md#L38): `For more information about installing Grafana on Kubernetes using the Helm Chart, refer to the [Grafana Helm C…`

### crd (7)

- `apps/alerting/notifications/definitions/receiver.notifications.alerting.grafana.app.yaml` (high; crd.file) — [grafana/grafana:apps/alerting/notifications/definitions/receiver.notifications.alerting.grafana.app.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/notifications/definitions/receiver.notifications.alerting.grafana.app.yaml#L2): `kind: CustomResourceDefinition`
- `apps/alerting/notifications/definitions/routingtree.notifications.alerting.grafana.app.yaml` (high; crd.file) — [grafana/grafana:apps/alerting/notifications/definitions/routingtree.notifications.alerting.grafana.app.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/notifications/definitions/routingtree.notifications.alerting.grafana.app.yaml#L2): `kind: CustomResourceDefinition`
- `apps/alerting/notifications/definitions/templategroup.notifications.alerting.grafana.app.yaml` (high; crd.file) — [grafana/grafana:apps/alerting/notifications/definitions/templategroup.notifications.alerting.grafana.app.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/notifications/definitions/templategroup.notifications.alerting.grafana.app.yaml#L2): `kind: CustomResourceDefinition`
- `apps/alerting/notifications/definitions/timeinterval.notifications.alerting.grafana.app.yaml` (high; crd.file) — [grafana/grafana:apps/alerting/notifications/definitions/timeinterval.notifications.alerting.grafana.app.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/notifications/definitions/timeinterval.notifications.alerting.grafana.app.yaml#L2): `kind: CustomResourceDefinition`
- `apps/alerting/rules/definitions/alertrule.rules.alerting.grafana.app.yaml` (high; crd.file) — [grafana/grafana:apps/alerting/rules/definitions/alertrule.rules.alerting.grafana.app.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/rules/definitions/alertrule.rules.alerting.grafana.app.yaml#L2): `kind: CustomResourceDefinition`
- `apps/alerting/rules/definitions/recordingrule.rules.alerting.grafana.app.yaml` (high; crd.file) — [grafana/grafana:apps/alerting/rules/definitions/recordingrule.rules.alerting.grafana.app.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/rules/definitions/recordingrule.rules.alerting.grafana.app.yaml#L2): `kind: CustomResourceDefinition`
- `apps/alerting/rules/definitions/rulesequence.rules.alerting.grafana.app.yaml` (high; crd.file) — [grafana/grafana:apps/alerting/rules/definitions/rulesequence.rules.alerting.grafana.app.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/apps/alerting/rules/definitions/rulesequence.rules.alerting.grafana.app.yaml#L2): `kind: CustomResourceDefinition`

### helm-repo (2)

- `https://grafana-community.github.io/helm-charts` (medium; docs.helm-repo-add) — [grafana/grafana:docs/sources/setup-grafana/installation/helm/index.md L53](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/setup-grafana/installation/helm/index.md#L53): `helm repo add grafana-community https://grafana-community.github.io/helm-charts`
- `https://grafana.github.io/helm-charts` (medium; docs.helm-repo-add) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/grafana-operator/_index.md L22](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/grafana-operator/_index.md#L22): `helm repo add grafana https://grafana.github.io/helm-charts`

### image (60)

- `docker.io/bitnami/etcd` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/etcd/docker-compose.yaml L3](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/etcd/docker-compose.yaml#L3): `image: bitnami/etcd:latest`
- `docker.io/bitnami/oauth2-proxy` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/auth/jwt_proxy/docker-compose.yaml L31](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/auth/jwt_proxy/docker-compose.yaml#L31): `image: docker.io/bitnami/oauth2-proxy:7.4.0`
- `docker.io/clickhouse/clickhouse-server` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/clickhouse/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/clickhouse/docker-compose.yaml#L2): `image: clickhouse/clickhouse-server:latest`
- `docker.io/freeipa/freeipa-server` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/auth/freeipa/docker-compose.yaml L8](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/auth/freeipa/docker-compose.yaml#L8): `image: freeipa/freeipa-server:fedora-29`
- `docker.io/gofeatureflag/go-feature-flag` (medium; manifest.image) — [grafana/grafana:devenv/frontend-service/docker-compose.yaml L79](https://github.com/grafana/grafana/blob/v13.2.3/devenv/frontend-service/docker-compose.yaml#L79): `image: gofeatureflag/go-feature-flag:latest`
- `docker.io/grafana/alloy` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/self-instrumentation/docker-compose.yaml L36](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/self-instrumentation/docker-compose.yaml#L36): `image: grafana/alloy:v1.10.0`
- `docker.io/grafana/fake-data-gen` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/graphite/docker-compose.yaml L9](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/graphite/docker-compose.yaml#L9): `image: grafana/fake-data-gen`
- `docker.io/grafana/grafana` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/traefik/docker-compose.yml L16](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/traefik/docker-compose.yml#L16): `image: grafana/grafana:latest`
- `docker.io/grafana/grafana-image-renderer` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/grafana/docker-compose.yaml L14](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/grafana/docker-compose.yaml#L14): `image: grafana/grafana-image-renderer:latest`
- `docker.io/grafana/loki` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/jaeger/docker-compose.yaml L10](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/jaeger/docker-compose.yaml#L10): `image: grafana/loki:master`
- `docker.io/grafana/mimir-alpine` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/mimir_backend/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/mimir_backend/docker-compose.yaml#L2): `image: grafana/mimir-alpine:r316-55f47f8`
- `docker.io/grafana/promtail` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/jaeger/docker-compose.yaml L26](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/jaeger/docker-compose.yaml#L26): `image: grafana/promtail:master`
- `docker.io/grafana/pyroscope` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/pyroscope/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/pyroscope/docker-compose.yaml#L2): `image: "grafana/pyroscope:latest"`
- `docker.io/grafana/tempo` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/self-instrumentation/docker-compose.yaml L21](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/self-instrumentation/docker-compose.yaml#L21): `image: grafana/tempo:2.8.1`
- `docker.io/grafana/tns-app` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/tempo/docker-compose.yaml L26](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/tempo/docker-compose.yaml#L26): `image: grafana/tns-app:9c1ab38`
- `docker.io/grafana/tns-db` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/tempo/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/tempo/docker-compose.yaml#L2): `image: grafana/tns-db:9c1ab38`
- `docker.io/grafana/tns-loadgen` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/tempo/docker-compose.yaml L53](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/tempo/docker-compose.yaml#L53): `image: grafana/tns-loadgen:9c1ab38`
- `docker.io/graphiteapp/graphite-statsd` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/graphite/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/graphite/docker-compose.yaml#L2): `image: graphiteapp/graphite-statsd`
- `docker.io/jaegertracing/all-in-one` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/jaeger/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/jaeger/docker-compose.yaml#L2): `image: jaegertracing/all-in-one:latest`
- `docker.io/library/busybox` (medium; manifest.image) — [grafana/grafana:devenv/frontend-service/docker-compose.yaml L137](https://github.com/grafana/grafana/blob/v13.2.3/devenv/frontend-service/docker-compose.yaml#L137): `image: busybox:1.38.0@sha256:fd8d9aa63ba2f0982b5304e1ee8d3b90a210bc1ffb5314d980eb6962f1a9715d`
- `docker.io/library/grafana-fs-dev` (medium; manifest.image) — [grafana/grafana:devenv/frontend-service/docker-compose.yaml L21](https://github.com/grafana/grafana/blob/v13.2.3/devenv/frontend-service/docker-compose.yaml#L21): `image: grafana-fs-dev`
- `docker.io/library/grafana-proxy` (medium; manifest.image) — [grafana/grafana:devenv/frontend-service/docker-compose.yaml L5](https://github.com/grafana/grafana/blob/v13.2.3/devenv/frontend-service/docker-compose.yaml#L5): `image: grafana-proxy`
- `docker.io/library/influxdb` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/influxdb/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/influxdb/docker-compose.yaml#L2): `image: influxdb:latest`
- `docker.io/library/mariadb` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/mariadb/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/mariadb/docker-compose.yaml#L2): `image: mariadb:latest`
- `docker.io/library/memcached` (medium; manifest.image) — [grafana/grafana:devenv/docker/blocks/memcached/docker-compose.yaml L2](https://github.com/grafana/grafana/blob/v13.2.3/devenv/docker/blocks/memcached/docker-compose.yaml#L2): `image: memcached:latest`
- … 35 more

### image-name (4)

- `deb` (low; dockerfile.name) — [grafana/grafana:scripts/verify-repo-update/Dockerfile.deb L1](https://github.com/grafana/grafana/blob/v13.2.3/scripts/verify-repo-update/Dockerfile.deb#L1): `FROM ubuntu:24.04`
- `grafana-fs-dev` (low; dockerfile.name) — [grafana/grafana:devenv/frontend-service/grafana-fs-dev.dockerfile L1](https://github.com/grafana/grafana/blob/v13.2.3/devenv/frontend-service/grafana-fs-dev.dockerfile#L1): `FROM ubuntu:24.04`
- `proxy` (low; dockerfile.name) — [grafana/grafana:devenv/frontend-service/proxy.dockerfile L1](https://github.com/grafana/grafana/blob/v13.2.3/devenv/frontend-service/proxy.dockerfile#L1): `FROM nginx:1.29.0-alpine`
- `rpm` (low; dockerfile.name) — [grafana/grafana:scripts/verify-repo-update/Dockerfile.rpm L1](https://github.com/grafana/grafana/blob/v13.2.3/scripts/verify-repo-update/Dockerfile.rpm#L1): `FROM centos:7`

### registry (44)

- `registry.terraform.io/providers/grafana/grafana` (low; docs.registry-ref) — [grafana/grafana:docs/sources/administration/roles-and-permissions/access-control/rbac-terraform-provisioning/index.md L78](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/administration/roles-and-permissions/access-control/rbac-terraform-provisioning/index.md#L78): `- Ensure you have the grafana/grafana [Terraform provider](https://registry.terraform.io/providers/grafana/gra…`
- `registry.terraform.io/providers/grafana/grafana/2.6.0/docs` (low; docs.registry-ref) — [grafana/grafana:.changelog-archive/CHANGELOG.10.md L841](https://github.com/grafana/grafana/blob/v13.2.3/.changelog-archive/CHANGELOG.10.md#L841): `If you are using Terraform Grafana provider to manage data source permissions, you will need to upgrade your p…`
- `registry.terraform.io/providers/grafana/grafana/latest` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/_index.md L27](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/_index.md#L27): `Grafana administrators can manage dashboards, alerts and collectors, add synthetic monitoring probes and check…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs` (low; docs.registry-ref) — [grafana/grafana:docs/sources/administration/roles-and-permissions/access-control/create-custom-roles/index.md L165](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/administration/roles-and-permissions/access-control/create-custom-roles/index.md#L165): `You can use the [Grafana Terraform provider](https://registry.terraform.io/providers/grafana/grafana/latest/do…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/data-sources/cloud_stack` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md L39](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md#L39): `The ['grafana_cloud_stack' (Data Source)](https://registry.terraform.io/providers/grafana/grafana/latest/docs/…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/data-sources/folder` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/observability-as-code/git-sync/permissions-grafana.md L212](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/observability-as-code/git-sync/permissions-grafana.md#L212): `Don't manage these folders with the 'grafana_folder' resource—they're owned by Git Sync. To avoid hardcoding…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/apps_provisioning_connection_v0alpha1` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/observability-as-code/git-sync/git-sync-setup/set-up-terraform.md L82](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/observability-as-code/git-sync/git-sync-setup/set-up-terraform.md#L82): `- The connection resource configures your Git provider credentials. For examples, refer to [Connection resourc…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/apps_provisioning_repository_v0alpha1` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/observability-as-code/git-sync/git-sync-setup/set-up-terraform.md L81](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/observability-as-code/git-sync/git-sync-setup/set-up-terraform.md#L81): `- The repository resource configures the Git repository to sync Grafana resources with. For examples, refer to…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/apps_queries_query_v1` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/manage-saved-queries.md L105](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/manage-saved-queries.md#L105): `For the complete schema, refer to the ['grafana_apps_queries_query_v1' resource documentation](https://registr…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/apps_secret_keeper_activation_v1beta1` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-secrets-management.md L508](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-secrets-management.md#L508): `To set a keeper as active, use the ['grafana_apps_secret_keeper_activation_v1beta1'](https://registry.terrafor…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/apps_secret_keeper_v1beta1` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-secrets-management.md L419](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-secrets-management.md#L419): `To store secrets in an external secret manager instead, provision a keeper with the ['grafana_apps_secret_keep…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/apps_secret_securevalue_v1beta1` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-secrets-management.md L25](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-secrets-management.md#L25): `This guide covers _secure values_, which you manage with the ['grafana_apps_secret_securevalue_v1beta1'](https…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_access_policy` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md L83](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md#L83): `- An access policy named 'fleet-management-policy' with 'fleet-management:read' and 'fleet-management:write' s…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_access_policy_token` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md L84](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md#L84): `- A token named 'fleet-management-token', using ['grafana_cloud_access_policy_token' (Resource)](https://regis…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_plugin_installation` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-plugins.md L81](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-plugins.md#L81): `To learn more about plugin installation, refer to the [Grafana provider documentation](https://registry.terraf…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_provider_aws_account` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md L119](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md#L119): `| 'grafana_cloud_provider_aws_account'                      | Represents an AWS IAM role that authorizes Grafa…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_provider_aws_cloudwatch_scrape_job` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md L120](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md#L120): `| 'grafana_cloud_provider_aws_cloudwatch_scrape_job'        | Represents a Grafana AWS scrape job. This config…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_provider_aws_resource_metadata_scrape_job` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md L121](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md#L121): `| 'grafana_cloud_provider_aws_resource_metadata_scrape_job' | Represents a Grafana AWS Resource Metadata scrap…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_provider_azure_credential` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md L122](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-provider-o11y.md#L122): `| 'grafana_cloud_provider_azure_credential'                 | A resource representing an Azure Service Princip…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/cloud_stack_service_account` (low; docs.registry-ref) — [grafana/grafana:docs/sources/administration/service-accounts/migrate-api-keys.md L253](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/administration/service-accounts/migrate-api-keys.md#L253): `For migration your cloud stack api keys, use the 'grafana_cloud_stack_service_account' and 'gafana_cloud_stack…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/contact_point` (low; docs.registry-ref) — [grafana/grafana:docs/sources/alerting/set-up/provision-alerting-resources/terraform-provisioning/index.md L135](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/alerting/set-up/provision-alerting-resources/terraform-provisioning/index.md#L135): `| [Contact points](ref:contact-points)                | [grafana_contact_point](https://registry.terraform.io/…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/dashboard` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/dashboards-github-action.md L96](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/dashboards-github-action.md#L96): `- ['grafana_dashboard' (Resource)](https://registry.terraform.io/providers/grafana/grafana/latest/docs/resourc…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/data_source` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-stack.md L119](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-cloud-stack.md#L119): `This guide uses the InfluxDB data source. The required arguments for [grafana_data_source (Resource)](https://…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/data_source_config_lbac_rules` (low; docs.registry-ref) — [grafana/grafana:docs/sources/administration/data-source-management/teamlbac/_index.md L117](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/administration/data-source-management/teamlbac/_index.md#L117): `We recommend using our Terraform provider to set up provisioning for [Resource data source config LBAC rules](…`
- `registry.terraform.io/providers/grafana/grafana/latest/docs/resources/fleet_management_collector` (low; docs.registry-ref) — [grafana/grafana:docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md L138](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/as-code/infrastructure-as-code/terraform/terraform-fleet-management.md#L138): `This Terraform configuration creates a collector with a remote attribute, using ['grafana_fleet_management_col…`
- … 19 more

### release-publisher (1)

- `gh release upload` (medium; workflow.release-upload) — [grafana/grafana:.github/workflows/sbom-on-release.yml L85](https://github.com/grafana/grafana/blob/v13.2.3/.github/workflows/sbom-on-release.yml#L85): `run: gh release upload "$TAG" "$SBOM_PATH" --clobber`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$` (high; tags.ls-remote) — [grafana/grafana/releases/tag/v13.2.3 refs/tags/v13.2.3](https://github.com/grafana/grafana/releases/tag/v13.2.3#refs/tags/v13.2.3): `6193dc03311b631b9727b560d24369e683dc396e refs/tags/v13.2.3`

### upgrade-guide (1)

- `docs/sources/upgrade-guide/upgrade-v{{.Major}}.{{.Minor}}/index.md` (high; paths.versioned-upgrade-guide) — [grafana/grafana:docs/sources/upgrade-guide/upgrade-v13.2/index.md ](https://github.com/grafana/grafana/blob/v13.2.3/docs/sources/upgrade-guide/upgrade-v13.2/index.md): `file docs/sources/upgrade-guide/upgrade-v13.2/index.md`
