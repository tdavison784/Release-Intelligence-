# Discovery report: helm-charts

- Repository: `github.com/prometheus-community/helm-charts` at `kube-prometheus-stack-91.8.2` (commit `7d82f66c37f9…`)
- Generated: 2026-10-02T12:36:04Z
- LLM: not used (deterministic resolver only)
- Tags: 3502 tags: prefix "kube-prometheus-stack-", 1151 stable, 0 prereleases (), 2351 junk; latest stable kube-prometheus-stack-91.8.2; lineage minor
- Strict tag pattern: `^kube-prometheus-stack-(?P<version>\d+\.\d+\.\d+)$` (junk sample: alertmanager-0.1.0, alertmanager-0.1.1, alertmanager-0.1.2, alertmanager-0.1.3, alertmanager-0.1.4, alertmanager-0.10.0)
- Scanned `github.com/prometheus-community/helm-charts@kube-prometheus-stack-91.8.2` (source profile): 1147 files listed, 468 read
- Validation: done against kube-prometheus-stack-91.6.0, kube-prometheus-stack-91.7.0, kube-prometheus-stack-91.7.1, kube-prometheus-stack-91.8.0, kube-prometheus-stack-91.8.2

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 1 | git-tags github.com/prometheus-community/helm-charts |
| release-notes | not-found | 0 |  |
| changelog | candidates-only | 1 |  |
| helm-charts | found | 91 | alertmanager-snmp-notifier via oci:ghcr.io/prometheus-community/charts/alertmanager-snmp-notifier, oci:ghcr.io/prometheus-community/charts/prom-label-proxy/alertmanager-snmp-notifier, helm-repo:https://prometheus-communi… |
| registries | candidates-only | 1 |  |
| images | candidates-only | 49 |  |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories prometheus-community/helm-charts |
| version-relations | found | 1 | alertmanager-snmp-notifier-chart: independent; alertmanager-chart: independent; jiralert-chart: independent; kube-prometheus-stack-chart: version = {{.Version}}; crds-chart: version = {{.Version}}; kube-state-metrics-cha… |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:crds | historically-validated | crd.in-repo | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-alertmanagerco… |
| artifact:kube-prometheus-stack-chart | historically-validated | helm.version-matches-release | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/Chart.yaml#L27 (+2) |
| artifact:admission-webhook | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/values.yaml#L3180 |
| artifact:alertmanager | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager/values.yaml#L13 |
| artifact:alertmanager-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager/Chart.yaml#L2 (+2) |
| artifact:alertmanager-snmp-notifier-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager-snmp-notifier/Chart.yaml#L2 (+2) |
| artifact:blackbox-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-blackbox-exporter/README.md#L66 |
| artifact:couchdb-prometheus-exporter | unverified | image.tag-assumed-release | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-couchdb-exporter/values.yaml#L20 |
| artifact:crds-chart | unverified | helm.placeholder-version-follows-release | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/Chart.yaml#L2 (+2) |
| artifact:crds-chart-2 | unverified | helm.placeholder-version-follows-release | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-operator-crds/charts/crds/Chart.yaml#L2 (+2) |
| artifact:elasticsearch-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-elasticsearch-exporter/values.yaml#L42 |
| artifact:ipmi-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-ipmi-exporter/values.yaml#L8 |
| artifact:jiralert-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/jiralert/Chart.yaml#L2 (+2) |
| artifact:json-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-json-exporter/values.yaml#L9 |
| artifact:kube-rbac-proxy | unverified | image.tag-assumed-release | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-state-metrics/values.yaml#L143 |
| artifact:kube-state-metrics | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-state-metrics/values.yaml#L5 |
| artifact:kube-state-metrics-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-state-metrics/Chart.yaml#L2 (+2) |
| artifact:kube-webhook-certgen | unverified | image.tag-assumed-release | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/values.yaml#L3299 |
| artifact:mysqld-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mysql-exporter/values.yaml#L15 |
| artifact:nginx-prometheus-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nginx-exporter/values.yaml#L22 |
| artifact:node-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-node-exporter/values.yaml#L10 |
| artifact:pgbouncer-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pgbouncer-exporter/values.yaml#L6 |
| artifact:postgres-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-postgres-exporter/values.yaml#L5 |
| artifact:prom-label-proxy | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prom-label-proxy/values.yaml#L7 |
| artifact:prom-label-proxy-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prom-label-proxy/Chart.yaml#L2 (+2) |
| artifact:prometheus | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/values.yaml#L4547 |
| artifact:prometheus-adapter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-adapter/values.yaml#L12 |
| artifact:prometheus-adapter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-adapter/Chart.yaml#L2 (+2) |
| artifact:prometheus-blackbox-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-blackbox-exporter/Chart.yaml#L3 (+2) |
| artifact:prometheus-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus/Chart.yaml#L2 (+2) |
| artifact:prometheus-cloudwatch-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-cloudwatch-exporter/Chart.yaml#L4 (+2) |
| artifact:prometheus-config-reloader | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager/values.yaml#L415 |
| artifact:prometheus-conntrack-stats-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-conntrack-stats-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-consul-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-consul-exporter/Chart.yaml#L5 (+2) |
| artifact:prometheus-couchdb-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-couchdb-exporter/Chart.yaml#L4 (+2) |
| artifact:prometheus-druid-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-druid-exporter/Chart.yaml#L4 (+2) |
| artifact:prometheus-elasticsearch-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-elasticsearch-exporter/Chart.yaml#L3 (+2) |
| artifact:prometheus-fastly-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-fastly-exporter/Chart.yaml#L5 (+2) |
| artifact:prometheus-ipmi-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-ipmi-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-json-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-json-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-kafka-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-kafka-exporter/Chart.yaml#L4 (+2) |
| artifact:prometheus-memcached-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-memcached-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-modbus-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-modbus-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-mongodb-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mongodb-exporter/Chart.yaml#L18 (+2) |
| artifact:prometheus-mysql-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mysql-exporter/Chart.yaml#L3 (+2) |
| artifact:prometheus-nats-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nats-exporter/values.yaml#L8 |
| artifact:prometheus-nats-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nats-exporter/Chart.yaml#L5 (+2) |
| artifact:prometheus-nginx-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nginx-exporter/Chart.yaml#L4 (+2) |
| artifact:prometheus-node-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-node-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-operator | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/values.yaml#L3700 |
| artifact:prometheus-operator-admission-webhook-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-operator-admission-webhook/Chart.yaml#L4 (+2) |
| artifact:prometheus-operator-crds-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-operator-crds/Chart.yaml#L4 (+2) |
| artifact:prometheus-pgbouncer-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pgbouncer-exporter/Chart.yaml#L5 (+2) |
| artifact:prometheus-pingdom-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pingdom-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-pingmesh-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pingmesh-exporter/Chart.yaml#L14 (+2) |
| artifact:prometheus-postgres-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-postgres-exporter/Chart.yaml#L5 (+2) |
| artifact:prometheus-pushgateway-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pushgateway/Chart.yaml#L5 (+2) |
| artifact:prometheus-rabbitmq-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-rabbitmq-exporter/Chart.yaml#L3 (+2) |
| artifact:prometheus-redis-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-redis-exporter/Chart.yaml#L5 (+2) |
| artifact:prometheus-smartctl-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-smartctl-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-snmp-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-snmp-exporter/Chart.yaml#L3 (+2) |
| artifact:prometheus-sql-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-sql-exporter/Chart.yaml#L3 (+2) |
| artifact:prometheus-stackdriver-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-stackdriver-exporter/Chart.yaml#L3 (+2) |
| artifact:prometheus-statsd-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-statsd-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-systemd-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-systemd-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-to-sd | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-to-sd/values.yaml#L3 |
| artifact:prometheus-to-sd-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-to-sd/Chart.yaml#L13 (+2) |
| artifact:prometheus-windows-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-windows-exporter/Chart.yaml#L2 (+2) |
| artifact:prometheus-yet-another-cloudwatch-exporter-chart | unverified | helm.independent-version | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-yet-another-cloudwatch-exporter/Chart.yaml#L2 … |
| artifact:pushgateway | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pushgateway/values.yaml#L20 |
| artifact:smartctl-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-smartctl-exporter/values.yaml#L111 |
| artifact:snmp-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-snmp-exporter/values.yaml#L6 |
| artifact:stackdriver-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-stackdriver-exporter/values.yaml#L17 |
| artifact:systemd-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-systemd-exporter/values.yaml#L6 |
| artifact:windows-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-windows-exporter/values.yaml#L7 |
| artifact:yet-another-cloudwatch-exporter | unverified | image.tag-from-chart-appversion | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-yet-another-cloudwatch-exporter/values.yaml#L9 |
| source:advisories | discovered | security.github-hosted-default | https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/SECURITY.md#L1 |
| source:tags | discovered | versions.git-tags | https://github.com/prometheus-community/helm-charts/releases/tag/kube-prometheus-stack-91.8.2#refs/tags/kube-prometheus-stack-91.8.2 |
| versioning | discovered | tags.scheme | https://github.com/prometheus-community/helm-charts/releases/tag/kube-prometheus-stack-91.8.2#refs/tags/kube-prometheus-stack-91.8.2 |

Statuses: 2 historically-validated, 3 discovered, 74 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: kube-prometheus-stack
name: helm-charts
homepage: https://github.com/prometheus-community/helm-charts
versioning:
  scheme: semver
  tagPrefix: kube-prometheus-stack-
  tagPattern: ^kube-prometheus-stack-(?P<version>\d+\.\d+\.\d+)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/prometheus-community/helm-charts
      tagPattern: ^kube-prometheus-stack-(?P<version>\d+\.\d+\.\d+)$
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: prometheus-community/helm-charts
    notes: 'Security policy: SECURITY.md → https://prometheus.io/docs/operating/security/'
artifacts:
  - id: alertmanager-snmp-notifier-chart
    type: helm-chart
    name: alertmanager-snmp-notifier
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/alertmanager-snmp-notifier
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/alertmanager-snmp-notifier
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: alertmanager-snmp-notifier
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: alertmanager-snmp-notifier
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/alertmanager-snmp-notifier
        tagPattern: ^alertmanager-snmp-notifier-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/alertmanager-snmp-notifier/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/alertmanager-snmp-notifier/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: alertmanager-chart
    type: helm-chart
    name: alertmanager
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/alertmanager
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/alertmanager
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: alertmanager
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: alertmanager
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/alertmanager
        tagPattern: ^alertmanager-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/alertmanager/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/alertmanager/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: jiralert-chart
    type: helm-chart
    name: jiralert
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/jiralert
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/jiralert
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: jiralert
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: jiralert
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/jiralert
        tagPattern: ^jiralert-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/jiralert/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/jiralert/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: kube-prometheus-stack-chart
    type: helm-chart
    name: kube-prometheus-stack
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/kube-prometheus-stack
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/kube-prometheus-stack
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: kube-prometheus-stack
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: kube-prometheus-stack
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/kube-prometheus-stack/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/kube-prometheus-stack/Chart.yaml
    validatedAgainst:
      - 91.6.0
      - 91.7.0
      - 91.7.1
      - 91.8.0
      - 91.8.2
  - id: crds-chart
    type: helm-chart
    name: crds
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/crds
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: crds
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: crds
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/kube-prometheus-stack/charts/crds/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/kube-prometheus-stack/charts/crds/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.6.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.6.0; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.6.0 (oci: unavailable)); 91.7.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.7.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.7.0; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.7.0 (oci: unavailable)); 91.7.1 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.7.1, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.7.1; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.7.1 (oci: unavailable)); 91.8.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.8.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.8.0; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.8.0 (oci: unavailable)); …).'
  - id: kube-state-metrics-chart
    type: helm-chart
    name: kube-state-metrics
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/kube-state-metrics
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/kube-state-metrics
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: kube-state-metrics
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: kube-state-metrics
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/kube-state-metrics
        tagPattern: ^kube-state-metrics-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/kube-state-metrics/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/kube-state-metrics/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prom-label-proxy-chart
    type: helm-chart
    name: prom-label-proxy
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prom-label-proxy
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prom-label-proxy
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prom-label-proxy
        tagPattern: ^prom-label-proxy-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prom-label-proxy/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prom-label-proxy/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-adapter-chart
    type: helm-chart
    name: prometheus-adapter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-adapter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-adapter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-adapter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-adapter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-adapter
        tagPattern: ^prometheus-adapter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-adapter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-adapter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-blackbox-exporter-chart
    type: helm-chart
    name: prometheus-blackbox-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-blackbox-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-blackbox-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-blackbox-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-blackbox-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-blackbox-exporter
        tagPattern: ^prometheus-blackbox-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-blackbox-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-blackbox-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-cloudwatch-exporter-chart
    type: helm-chart
    name: prometheus-cloudwatch-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-cloudwatch-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-cloudwatch-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-cloudwatch-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-cloudwatch-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-cloudwatch-exporter
        tagPattern: ^prometheus-cloudwatch-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-cloudwatch-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-cloudwatch-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-conntrack-stats-exporter-chart
    type: helm-chart
    name: prometheus-conntrack-stats-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-conntrack-stats-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-conntrack-stats-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-conntrack-stats-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-conntrack-stats-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-conntrack-stats-exporter
        tagPattern: ^prometheus-conntrack-stats-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-conntrack-stats-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-conntrack-stats-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-consul-exporter-chart
    type: helm-chart
    name: prometheus-consul-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-consul-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-consul-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-consul-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-consul-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-consul-exporter
        tagPattern: ^prometheus-consul-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-consul-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-consul-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-couchdb-exporter-chart
    type: helm-chart
    name: prometheus-couchdb-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-couchdb-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-couchdb-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-couchdb-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-couchdb-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-couchdb-exporter
        tagPattern: ^prometheus-couchdb-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-couchdb-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-couchdb-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-druid-exporter-chart
    type: helm-chart
    name: prometheus-druid-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-druid-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-druid-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-druid-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-druid-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-druid-exporter
        tagPattern: ^prometheus-druid-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-druid-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-druid-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-elasticsearch-exporter-chart
    type: helm-chart
    name: prometheus-elasticsearch-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-elasticsearch-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-elasticsearch-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-elasticsearch-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-elasticsearch-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-elasticsearch-exporter
        tagPattern: ^prometheus-elasticsearch-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-elasticsearch-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-elasticsearch-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-fastly-exporter-chart
    type: helm-chart
    name: prometheus-fastly-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-fastly-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-fastly-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-fastly-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-fastly-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-fastly-exporter
        tagPattern: ^prometheus-fastly-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-fastly-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-fastly-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-ipmi-exporter-chart
    type: helm-chart
    name: prometheus-ipmi-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-ipmi-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-ipmi-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-ipmi-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-ipmi-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-ipmi-exporter
        tagPattern: ^prometheus-ipmi-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-ipmi-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-ipmi-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-json-exporter-chart
    type: helm-chart
    name: prometheus-json-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-json-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-json-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-json-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-json-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-json-exporter
        tagPattern: ^prometheus-json-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-json-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-json-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-kafka-exporter-chart
    type: helm-chart
    name: prometheus-kafka-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-kafka-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-kafka-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-kafka-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-kafka-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-kafka-exporter
        tagPattern: ^prometheus-kafka-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-kafka-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-kafka-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-memcached-exporter-chart
    type: helm-chart
    name: prometheus-memcached-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-memcached-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-memcached-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-memcached-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-memcached-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-memcached-exporter
        tagPattern: ^prometheus-memcached-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-memcached-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-memcached-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-modbus-exporter-chart
    type: helm-chart
    name: prometheus-modbus-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-modbus-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-modbus-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-modbus-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-modbus-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-modbus-exporter
        tagPattern: ^prometheus-modbus-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-modbus-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-modbus-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-mongodb-exporter-chart
    type: helm-chart
    name: prometheus-mongodb-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-mongodb-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-mongodb-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-mongodb-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-mongodb-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-mongodb-exporter
        tagPattern: ^prometheus-mongodb-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-mongodb-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-mongodb-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-mysql-exporter-chart
    type: helm-chart
    name: prometheus-mysql-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-mysql-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-mysql-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-mysql-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-mysql-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-mysql-exporter
        tagPattern: ^prometheus-mysql-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-mysql-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-mysql-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-nats-exporter-chart
    type: helm-chart
    name: prometheus-nats-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-nats-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-nats-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-nats-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-nats-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-nats-exporter
        tagPattern: ^prometheus-nats-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-nats-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-nats-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-nginx-exporter-chart
    type: helm-chart
    name: prometheus-nginx-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-nginx-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-nginx-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-nginx-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-nginx-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-nginx-exporter
        tagPattern: ^prometheus-nginx-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-nginx-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-nginx-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-node-exporter-chart
    type: helm-chart
    name: prometheus-node-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-node-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-node-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-node-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-node-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-node-exporter
        tagPattern: ^prometheus-node-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-node-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-node-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-operator-admission-webhook-chart
    type: helm-chart
    name: prometheus-operator-admission-webhook
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-operator-admission-webhook
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-operator-admission-webhook
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-operator-admission-webhook
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-operator-admission-webhook
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-operator-admission-webhook
        tagPattern: ^prometheus-operator-admission-webhook-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-operator-admission-webhook/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-operator-admission-webhook/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-operator-crds-chart
    type: helm-chart
    name: prometheus-operator-crds
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-operator-crds
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-operator-crds
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-operator-crds
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-operator-crds
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-operator-crds
        tagPattern: ^prometheus-operator-crds-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-operator-crds/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-operator-crds/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: crds-chart-2
    type: helm-chart
    name: crds
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/crds
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: crds
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: crds
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-operator-crds/charts/crds/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-operator-crds/charts/crds/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.6.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.6.0; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.6.0 (oci: unavailable)); 91.7.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.7.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.7.0; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.7.0 (oci: unavailable)); 91.7.1 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.7.1, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.7.1; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.7.1 (oci: unavailable)); 91.8.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.8.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.8.0; not verifiable: ghcr.io/prometheus-community/charts/prom-label-proxy/crds:91.8.0 (oci: unavailable)); …).'
  - id: prometheus-pgbouncer-exporter-chart
    type: helm-chart
    name: prometheus-pgbouncer-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-pgbouncer-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-pgbouncer-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-pgbouncer-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-pgbouncer-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-pgbouncer-exporter
        tagPattern: ^prometheus-pgbouncer-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pgbouncer-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pgbouncer-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-pingdom-exporter-chart
    type: helm-chart
    name: prometheus-pingdom-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-pingdom-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-pingdom-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-pingdom-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-pingdom-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-pingdom-exporter
        tagPattern: ^prometheus-pingdom-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pingdom-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pingdom-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-pingmesh-exporter-chart
    type: helm-chart
    name: prometheus-pingmesh-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-pingmesh-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-pingmesh-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-pingmesh-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-pingmesh-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-pingmesh-exporter
        tagPattern: ^prometheus-pingmesh-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pingmesh-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pingmesh-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-postgres-exporter-chart
    type: helm-chart
    name: prometheus-postgres-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-postgres-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-postgres-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-postgres-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-postgres-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-postgres-exporter
        tagPattern: ^prometheus-postgres-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-postgres-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-postgres-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-pushgateway-chart
    type: helm-chart
    name: prometheus-pushgateway
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-pushgateway
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-pushgateway
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-pushgateway
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-pushgateway
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-pushgateway
        tagPattern: ^prometheus-pushgateway-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pushgateway/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-pushgateway/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-rabbitmq-exporter-chart
    type: helm-chart
    name: prometheus-rabbitmq-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-rabbitmq-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-rabbitmq-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-rabbitmq-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-rabbitmq-exporter
        tagPattern: ^prometheus-rabbitmq-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-rabbitmq-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-rabbitmq-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-redis-exporter-chart
    type: helm-chart
    name: prometheus-redis-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-redis-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-redis-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-redis-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-redis-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-redis-exporter
        tagPattern: ^prometheus-redis-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-redis-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-redis-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-smartctl-exporter-chart
    type: helm-chart
    name: prometheus-smartctl-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-smartctl-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-smartctl-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-smartctl-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-smartctl-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-smartctl-exporter
        tagPattern: ^prometheus-smartctl-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-smartctl-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-smartctl-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-snmp-exporter-chart
    type: helm-chart
    name: prometheus-snmp-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-snmp-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-snmp-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-snmp-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-snmp-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-snmp-exporter
        tagPattern: ^prometheus-snmp-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-snmp-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-snmp-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-sql-exporter-chart
    type: helm-chart
    name: prometheus-sql-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-sql-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-sql-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-sql-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-sql-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-sql-exporter
        tagPattern: ^prometheus-sql-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-sql-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-sql-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-stackdriver-exporter-chart
    type: helm-chart
    name: prometheus-stackdriver-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-stackdriver-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-stackdriver-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-stackdriver-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-stackdriver-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-stackdriver-exporter
        tagPattern: ^prometheus-stackdriver-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-stackdriver-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-stackdriver-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-statsd-exporter-chart
    type: helm-chart
    name: prometheus-statsd-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-statsd-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-statsd-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-statsd-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-statsd-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-statsd-exporter
        tagPattern: ^prometheus-statsd-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-statsd-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-statsd-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-systemd-exporter-chart
    type: helm-chart
    name: prometheus-systemd-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-systemd-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-systemd-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-systemd-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-systemd-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-systemd-exporter
        tagPattern: ^prometheus-systemd-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-systemd-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-systemd-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-to-sd-chart
    type: helm-chart
    name: prometheus-to-sd
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-to-sd
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-to-sd
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-to-sd
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-to-sd
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-to-sd
        tagPattern: ^prometheus-to-sd-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-to-sd/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-to-sd/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-windows-exporter-chart
    type: helm-chart
    name: prometheus-windows-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-windows-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-windows-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-windows-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-windows-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-windows-exporter
        tagPattern: ^prometheus-windows-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-windows-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-windows-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-yet-another-cloudwatch-exporter-chart
    type: helm-chart
    name: prometheus-yet-another-cloudwatch-exporter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus-yet-another-cloudwatch-exporter
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus-yet-another-cloudwatch-exporter
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus-yet-another-cloudwatch-exporter
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus-yet-another-cloudwatch-exporter
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus-yet-another-cloudwatch-exporter
        tagPattern: ^prometheus-yet-another-cloudwatch-exporter-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-yet-another-cloudwatch-exporter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus-yet-another-cloudwatch-exporter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: prometheus-chart
    type: helm-chart
    name: prometheus
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prometheus
      - kind: oci
        repository: ghcr.io/prometheus-community/charts/prom-label-proxy/prometheus
      - kind: helm-repo
        url: https://prometheus-community.github.io/helm-charts
        chart: prometheus
      - kind: helm-repo
        url: https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages
        chart: prometheus
      - kind: helm-git
        repository: github.com/prometheus-community/helm-charts
        path: charts/prometheus
        tagPattern: ^prometheus-(?P<version>\d+\.\d+\.\d+)$
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/prometheus-community/helm-charts
          path: charts/prometheus/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.8.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/prometheus-community/helm-charts
        path: charts/kube-prometheus-stack/charts/crds/crds
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 91.6.0
      - 91.7.0
      - 91.7.1
      - 91.8.0
      - 91.8.2
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - kube-prometheus-stack-91.6.0
    - kube-prometheus-stack-91.7.0
    - kube-prometheus-stack-91.7.1
    - kube-prometheus-stack-91.8.0
    - kube-prometheus-stack-91.8.2
  notes: Proposed by automated discovery; relationship checks run against kube-prometheus-stack-91.6.0, kube-prometheus-stack-91.7.0, kube-prometheus-stack-91.7.1, kube-prometheus-stack-91.8.0, kube-prometheus-stack-91.8.2.
```

## Open questions for the reviewer

- charts/prometheus-cloudwatch-exporter/CHANGELOG.md stops at 0.14.0; confirm it is abandoned (release notes come from other sources).
- Ambiguity (registry-roles): 6 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (main-chart): 46 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) alertmanager-snmp-notifier, alertmanager, jiralert, kube-state-metrics, prom-label-proxy, prometheus-adapter, prometheus-blackbox-exporter, prometheus-cloudwatch-exporter, prometheus-conntrack-stats-exporter, prometheus-consul-exporter, prometheus-couchdb-exporter, prometheus-druid-exporter, prometheus-elasticsearch-exporter, prometheus-fastly-exporter, prometheus-ipmi-exporter, prometheus-json-exporter, prometheus-kafka-exporter, prometheus-memcached-exporter, prometheus-modbus-exporter, prometheus-mongodb-exporter, prometheus-mysql-exporter, prometheus-nats-exporter, prometheus-nginx-exporter, prometheus-node-exporter, prometheus-operator-admission-webhook, prometheus-operator-crds, prometheus-pgbouncer-exporter, prometheus-pingdom-exporter, prometheus-pingmesh-exporter, prometheus-postgres-exporter, prometheus-pushgateway, prometheus-rabbitmq-exporter, prometheus-redis-exporter, prometheus-smartctl-exporter, prometheus-snmp-exporter, prometheus-sql-exporter, prometheus-stackdriver-exporter, prometheus-statsd-exporter, prometheus-systemd-exporter, prometheus-to-sd, prometheus-windows-exporter, prometheus-yet-another-cloudwatch-exporter, prometheus relate to the release version?
- Ambiguity (chart-version): How do the versions of chart(s) crds, crds relate to the release version?

## Validation matrix

| Element | Verdict | kube-prometheus-stack-91.6.0 | kube-prometheus-stack-91.7.0 | kube-prometheus-stack-91.7.1 | kube-prometheus-stack-91.8.0 | kube-prometheus-stack-91.8.2 |
|---|---|---|---|---|---|---|
| artifact:admission-webhook | failing |  |  |  |  |  |
| artifact:alertmanager | failing |  |  |  |  |  |
| artifact:alertmanager-chart | unverifiable |  |  |  |  |  |
| artifact:alertmanager-snmp-notifier-chart | unverifiable |  |  |  |  |  |
| artifact:blackbox-exporter | failing |  |  |  |  |  |
| artifact:couchdb-prometheus-exporter | failing |  |  |  |  |  |
| artifact:crds | validated |  |  |  |  |  |
| artifact:crds-chart | unverifiable |  |  |  |  |  |
| artifact:crds-chart-2 | unverifiable |  |  |  |  |  |
| artifact:elasticsearch-exporter | failing |  |  |  |  |  |
| artifact:ipmi-exporter | failing |  |  |  |  |  |
| artifact:jiralert-chart | unverifiable |  |  |  |  |  |
| artifact:json-exporter | failing |  |  |  |  |  |
| artifact:kube-prometheus-stack-chart | validated |  |  |  |  |  |
| artifact:kube-rbac-proxy | failing |  |  |  |  |  |
| artifact:kube-state-metrics | failing |  |  |  |  |  |
| artifact:kube-state-metrics-chart | unverifiable |  |  |  |  |  |
| artifact:kube-webhook-certgen | failing |  |  |  |  |  |
| artifact:mysqld-exporter | failing |  |  |  |  |  |
| artifact:nginx-prometheus-exporter | failing |  |  |  |  |  |
| artifact:node-exporter | failing |  |  |  |  |  |
| artifact:pgbouncer-exporter | failing |  |  |  |  |  |
| artifact:postgres-exporter | failing |  |  |  |  |  |
| artifact:prom-label-proxy | failing |  |  |  |  |  |
| artifact:prom-label-proxy-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus | failing |  |  |  |  |  |
| artifact:prometheus-adapter | failing |  |  |  |  |  |
| artifact:prometheus-adapter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-blackbox-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-cloudwatch-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-config-reloader | failing |  |  |  |  |  |
| artifact:prometheus-conntrack-stats-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-consul-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-couchdb-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-druid-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-elasticsearch-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-fastly-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-ipmi-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-json-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-kafka-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-memcached-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-modbus-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-mongodb-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-mysql-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-nats-exporter | failing |  |  |  |  |  |
| artifact:prometheus-nats-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-nginx-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-node-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-operator | failing |  |  |  |  |  |
| artifact:prometheus-operator-admission-webhook-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-operator-crds-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-pgbouncer-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-pingdom-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-pingmesh-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-postgres-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-pushgateway-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-rabbitmq-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-redis-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-smartctl-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-snmp-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-sql-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-stackdriver-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-statsd-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-systemd-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-to-sd | failing |  |  |  |  |  |
| artifact:prometheus-to-sd-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-windows-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:prometheus-yet-another-cloudwatch-exporter-chart | unverifiable |  |  |  |  |  |
| artifact:pushgateway | failing |  |  |  |  |  |
| artifact:smartctl-exporter | failing |  |  |  |  |  |
| artifact:snmp-exporter | failing |  |  |  |  |  |
| artifact:stackdriver-exporter | failing |  |  |  |  |  |
| artifact:systemd-exporter | failing |  |  |  |  |  |
| artifact:windows-exporter | failing |  |  |  |  |  |
| artifact:yet-another-cloudwatch-exporter | failing |  |  |  |  |  |
| content:alertmanager-chart/chart-metadata | validated |  |  |  |  |  |
| content:alertmanager-chart/helm-values | validated |  |  |  |  |  |
| content:alertmanager-snmp-notifier-chart/chart-metadata | validated |  |  |  |  |  |
| content:alertmanager-snmp-notifier-chart/helm-values | validated |  |  |  |  |  |
| content:crds-chart-2/chart-metadata | validated |  |  |  |  |  |
| content:crds-chart-2/helm-values | failing |  |  |  |  |  |
| content:crds-chart/chart-metadata | validated |  |  |  |  |  |
| content:crds-chart/helm-values | validated |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |
| content:jiralert-chart/chart-metadata | validated |  |  |  |  |  |
| content:jiralert-chart/helm-values | validated |  |  |  |  |  |
| content:kube-prometheus-stack-chart/chart-metadata | validated |  |  |  |  |  |
| content:kube-prometheus-stack-chart/helm-values | validated |  |  |  |  |  |
| content:kube-state-metrics-chart/chart-metadata | validated |  |  |  |  |  |
| content:kube-state-metrics-chart/helm-values | validated |  |  |  |  |  |
| content:prom-label-proxy-chart/chart-metadata | validated |  |  |  |  |  |
| content:prom-label-proxy-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-adapter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-adapter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-blackbox-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-blackbox-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-cloudwatch-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-cloudwatch-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-conntrack-stats-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-conntrack-stats-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-consul-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-consul-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-couchdb-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-couchdb-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-druid-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-druid-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-elasticsearch-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-elasticsearch-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-fastly-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-fastly-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-ipmi-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-ipmi-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-json-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-json-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-kafka-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-kafka-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-memcached-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-memcached-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-modbus-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-modbus-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-mongodb-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-mongodb-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-mysql-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-mysql-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-nats-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-nats-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-nginx-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-nginx-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-node-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-node-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-operator-admission-webhook-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-operator-admission-webhook-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-operator-crds-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-operator-crds-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-pgbouncer-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-pgbouncer-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-pingdom-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-pingdom-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-pingmesh-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-pingmesh-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-postgres-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-postgres-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-pushgateway-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-pushgateway-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-rabbitmq-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-rabbitmq-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-redis-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-redis-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-smartctl-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-smartctl-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-snmp-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-snmp-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-sql-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-sql-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-stackdriver-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-stackdriver-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-statsd-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-statsd-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-systemd-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-systemd-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-to-sd-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-to-sd-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-windows-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-windows-exporter-chart/helm-values | validated |  |  |  |  |  |
| content:prometheus-yet-another-cloudwatch-exporter-chart/chart-metadata | validated |  |  |  |  |  |
| content:prometheus-yet-another-cloudwatch-exporter-chart/helm-values | validated |  |  |  |  |  |

## Dropped elements

- `artifact:admission-webhook` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus-operator/admission-webhook:91.6.0); 91.7.0 fail (absent from quay.io/prometheus-operator/admission-webhook:91.7.0); 91.7.1 fail (absent from quay.io/prometheus-operator/admission-webhook:91.7.1); 91.8.0 fail (absent from quay.io/prometheus-operator/admission-webhook:91.8.0); …
- `artifact:alertmanager` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus/alertmanager:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/alertmanager:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/alertmanager:91.7.1); 91.8.0 fail (absent from quay.io/prometheus/alertmanager:91.8.0); …
- `artifact:blackbox-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus/blackbox-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/blackbox-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/blackbox-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheus/blackbox-exporter:91.8.0); …
- `artifact:couchdb-prometheus-exporter` (deterministic): failed validation: 91.6.0 fail (absent from docker.io/gesellix/couchdb-prometheus-exporter:kube-prometheus-stack-91.6.0); 91.7.0 fail (absent from docker.io/gesellix/couchdb-prometheus-exporter:kube-prometheus-stack-91.7.0); 91.7.1 fail (absent from docker.io/gesellix/couchdb-prometheus-exporter:kube-prometheus-stack-91.7.1); 91.8.0 fail (absent from docker.io/gesellix/couchdb-prometheus-exporter:kube-prometheus-stack-91.8.0); …
- `artifact:elasticsearch-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/elasticsearch-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/elasticsearch-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/elasticsearch-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/elasticsearch-exporter:91.8.0); …
- `artifact:ipmi-exporter` (deterministic): failed validation: 91.6.0 fail (absent from docker.io/prometheuscommunity/ipmi-exporter:91.6.0); 91.7.0 fail (absent from docker.io/prometheuscommunity/ipmi-exporter:91.7.0); 91.7.1 fail (absent from docker.io/prometheuscommunity/ipmi-exporter:91.7.1); 91.8.0 fail (absent from docker.io/prometheuscommunity/ipmi-exporter:91.8.0); …
- `artifact:json-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/json-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/json-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/json-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/json-exporter:91.8.0); …
- `artifact:kube-rbac-proxy` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/brancz/kube-rbac-proxy:kube-prometheus-stack-91.6.0); 91.7.0 fail (absent from quay.io/brancz/kube-rbac-proxy:kube-prometheus-stack-91.7.0); 91.7.1 fail (absent from quay.io/brancz/kube-rbac-proxy:kube-prometheus-stack-91.7.1); 91.8.0 fail (absent from quay.io/brancz/kube-rbac-proxy:kube-prometheus-stack-91.8.0); …
- `artifact:kube-state-metrics` (deterministic): failed validation: 91.6.0 fail (absent from registry.k8s.io/kube-state-metrics/kube-state-metrics:91.6.0); 91.7.0 fail (absent from registry.k8s.io/kube-state-metrics/kube-state-metrics:91.7.0); 91.7.1 fail (absent from registry.k8s.io/kube-state-metrics/kube-state-metrics:91.7.1); 91.8.0 fail (absent from registry.k8s.io/kube-state-metrics/kube-state-metrics:91.8.0); …
- `artifact:kube-webhook-certgen` (deterministic): failed validation: 91.6.0 fail (absent from ghcr.io/jkroepke/kube-webhook-certgen:kube-prometheus-stack-91.6.0); 91.7.0 fail (absent from ghcr.io/jkroepke/kube-webhook-certgen:kube-prometheus-stack-91.7.0); 91.7.1 fail (absent from ghcr.io/jkroepke/kube-webhook-certgen:kube-prometheus-stack-91.7.1); 91.8.0 fail (absent from ghcr.io/jkroepke/kube-webhook-certgen:kube-prometheus-stack-91.8.0); …
- `artifact:mysqld-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus/mysqld-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/mysqld-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/mysqld-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheus/mysqld-exporter:91.8.0); …
- `artifact:nginx-prometheus-exporter` (deterministic): failed validation: 91.6.0 fail (absent from docker.io/nginx/nginx-prometheus-exporter:91.6.0); 91.7.0 fail (absent from docker.io/nginx/nginx-prometheus-exporter:91.7.0); 91.7.1 fail (absent from docker.io/nginx/nginx-prometheus-exporter:91.7.1); 91.8.0 fail (absent from docker.io/nginx/nginx-prometheus-exporter:91.8.0); …
- `artifact:node-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus/node-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/node-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/node-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheus/node-exporter:91.8.0); …
- `artifact:pgbouncer-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/pgbouncer-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/pgbouncer-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/pgbouncer-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/pgbouncer-exporter:91.8.0); …
- `artifact:postgres-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/postgres-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/postgres-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/postgres-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/postgres-exporter:91.8.0); …
- `artifact:prom-label-proxy` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/prom-label-proxy:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/prom-label-proxy:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/prom-label-proxy:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/prom-label-proxy:91.8.0); …
- `artifact:prometheus` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus/prometheus:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/prometheus:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/prometheus:91.7.1); 91.8.0 fail (absent from quay.io/prometheus/prometheus:91.8.0); …
- `artifact:prometheus-adapter` (deterministic): failed validation: 91.6.0 fail (absent from registry.k8s.io/prometheus-adapter/prometheus-adapter:91.6.0); 91.7.0 fail (absent from registry.k8s.io/prometheus-adapter/prometheus-adapter:91.7.0); 91.7.1 fail (absent from registry.k8s.io/prometheus-adapter/prometheus-adapter:91.7.1); 91.8.0 fail (absent from registry.k8s.io/prometheus-adapter/prometheus-adapter:91.8.0); …
- `artifact:prometheus-config-reloader` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus-operator/prometheus-config-reloader:91.6.0); 91.7.0 fail (absent from quay.io/prometheus-operator/prometheus-config-reloader:91.7.0); 91.7.1 fail (absent from quay.io/prometheus-operator/prometheus-config-reloader:91.7.1); 91.8.0 fail (absent from quay.io/prometheus-operator/prometheus-config-reloader:91.8.0); …
- `artifact:prometheus-nats-exporter` (deterministic): failed validation: 91.6.0 fail (absent from docker.io/natsio/prometheus-nats-exporter:91.6.0); 91.7.0 fail (absent from docker.io/natsio/prometheus-nats-exporter:91.7.0); 91.7.1 fail (absent from docker.io/natsio/prometheus-nats-exporter:91.7.1); 91.8.0 fail (absent from docker.io/natsio/prometheus-nats-exporter:91.8.0); …
- `artifact:prometheus-operator` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus-operator/prometheus-operator:91.6.0); 91.7.0 fail (absent from quay.io/prometheus-operator/prometheus-operator:91.7.0); 91.7.1 fail (absent from quay.io/prometheus-operator/prometheus-operator:91.7.1); 91.8.0 fail (absent from quay.io/prometheus-operator/prometheus-operator:91.8.0); …
- `artifact:prometheus-to-sd` (deterministic): failed validation: 91.6.0 fail (absent from gcr.io/google-containers/prometheus-to-sd:91.6.0); 91.7.0 fail (absent from gcr.io/google-containers/prometheus-to-sd:91.7.0); 91.7.1 fail (absent from gcr.io/google-containers/prometheus-to-sd:91.7.1); 91.8.0 fail (absent from gcr.io/google-containers/prometheus-to-sd:91.8.0); …
- `artifact:pushgateway` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus/pushgateway:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/pushgateway:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/pushgateway:91.7.1); 91.8.0 fail (absent from quay.io/prometheus/pushgateway:91.8.0); …
- `artifact:smartctl-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/smartctl-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/smartctl-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/smartctl-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/smartctl-exporter:91.8.0); …
- `artifact:snmp-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheus/snmp-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/snmp-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/snmp-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheus/snmp-exporter:91.8.0); …
- `artifact:stackdriver-exporter` (deterministic): failed validation: 91.6.0 fail (absent from docker.io/prometheuscommunity/stackdriver-exporter:91.6.0); 91.7.0 fail (absent from docker.io/prometheuscommunity/stackdriver-exporter:91.7.0); 91.7.1 fail (absent from docker.io/prometheuscommunity/stackdriver-exporter:91.7.1); 91.8.0 fail (absent from docker.io/prometheuscommunity/stackdriver-exporter:91.8.0); …
- `artifact:systemd-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/systemd-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/systemd-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/systemd-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/systemd-exporter:91.8.0); …
- `artifact:windows-exporter` (deterministic): failed validation: 91.6.0 fail (absent from ghcr.io/prometheus-community/windows-exporter:91.6.0); 91.7.0 fail (absent from ghcr.io/prometheus-community/windows-exporter:91.7.0); 91.7.1 fail (absent from ghcr.io/prometheus-community/windows-exporter:91.7.1); 91.8.0 fail (absent from ghcr.io/prometheus-community/windows-exporter:91.8.0); …
- `artifact:yet-another-cloudwatch-exporter` (deterministic): failed validation: 91.6.0 fail (absent from quay.io/prometheuscommunity/yet-another-cloudwatch-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/yet-another-cloudwatch-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheuscommunity/yet-another-cloudwatch-exporter:91.7.1); 91.8.0 fail (absent from quay.io/prometheuscommunity/yet-another-cloudwatch-exporter:91.8.0); …

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 3502 tags: prefix "kube-prometheus-stack-", 1151 stable, 0 prereleases (), 2351 junk; latest stable kube-prometheus-stack-91.8.2; lineage minor; strict tagPattern excludes junk tags such as alertmanager-0.1.0, alertmanag… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (3502 tags: prefix "kube-prometheus-stack-", 1151 stable, 0 prereleases (), 2351 junk; latest stable kube-prometheus-stack-91.8.2; lineage minor). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:alertmanager-snmp-notifier-chart | include | helm.independent-version | heuristic | Chart has its own version "2.2.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:alertmanager-chart | include | helm.independent-version | heuristic | Chart has its own version "2.0.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:jiralert-chart | include | helm.independent-version | heuristic | Chart has its own version "1.9.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:kube-prometheus-stack-chart | include | helm.version-matches-release | heuristic | Chart.yaml at the scanned release carries version "91.8.2", the release version; chart versions follow the release ({{.Version}}). Published via 4 channel(s); OCI locations first. |
| artifact:crds-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "0.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 3 channel(s); OCI locations first. |
| artifact:kube-state-metrics-chart | include | helm.independent-version | heuristic | Chart has its own version "8.6.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prom-label-proxy-chart | include | helm.independent-version | heuristic | Chart has its own version "0.24.0" unrelated to the release. Published via 4 channel(s); OCI locations first. |
| artifact:prometheus-adapter-chart | include | helm.independent-version | heuristic | Chart has its own version "5.3.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-blackbox-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "11.19.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-cloudwatch-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.28.2" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-conntrack-stats-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.5.40" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-consul-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "1.1.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-couchdb-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "1.1.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-druid-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "1.2.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-elasticsearch-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "7.4.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-fastly-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.14.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-ipmi-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.8.2" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-json-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.20.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-kafka-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "4.0.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-memcached-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.6.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-modbus-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.1.4" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-mongodb-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "3.22.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-mysql-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "2.15.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-nats-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "2.23.2" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-nginx-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "1.23.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-node-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "4.59.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-operator-admission-webhook-chart | include | helm.independent-version | heuristic | Chart has its own version "0.44.2" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-operator-crds-chart | include | helm.independent-version | heuristic | Chart has its own version "32.0.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:crds-chart-2 | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "0.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 3 channel(s); OCI locations first. |
| artifact:prometheus-pgbouncer-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.10.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-pingdom-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "3.4.4" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-pingmesh-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.5.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-postgres-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "8.2.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-pushgateway-chart | include | helm.independent-version | heuristic | Chart has its own version "3.9.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-rabbitmq-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "2.1.2" unrelated to the release. Published via 4 channel(s); OCI locations first. |
| artifact:prometheus-redis-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "6.32.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-smartctl-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.17.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-snmp-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "9.18.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-sql-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.5.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-stackdriver-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "5.2.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-statsd-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "1.0.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-systemd-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.5.2" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-to-sd-chart | include | helm.independent-version | heuristic | Chart has its own version "0.5.1" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-windows-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.12.8" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-yet-another-cloudwatch-exporter-chart | include | helm.independent-version | heuristic | Chart has its own version "0.47.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:prometheus-chart | include | helm.independent-version | heuristic | Chart has its own version "29.35.0" unrelated to the release. Published via 5 channel(s); OCI locations first. |
| artifact:crds | include | crd.in-repo | heuristic | 10 CRDs of the product's API groups in charts/kube-prometheus-stack/charts/crds/crds at the release tag. |
| artifact:admission-webhook | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:alertmanager | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:blackbox-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:couchdb-prometheus-exporter | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:elasticsearch-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:ipmi-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:json-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:kube-rbac-proxy | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:kube-state-metrics | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:kube-webhook-certgen | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:mysqld-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:nginx-prometheus-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:node-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:pgbouncer-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:postgres-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:prom-label-proxy | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:prometheus | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:prometheus-adapter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:prometheus-config-reloader | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:prometheus-nats-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:prometheus-operator | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:prometheus-to-sd | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:pushgateway | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:smartctl-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:snmp-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:stackdriver-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:systemd-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:windows-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:yet-another-cloudwatch-exporter | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:alertmanager-snmp-notifier-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:alertmanager-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:jiralert-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:crds-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.6.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.6.0; not verifiable: g… |
| artifact:kube-state-metrics-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prom-label-proxy-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-adapter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-blackbox-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-cloudwatch-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-conntrack-stats-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-consul-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-couchdb-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-druid-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-elasticsearch-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-fastly-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-ipmi-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-json-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-kafka-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-memcached-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-modbus-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-mongodb-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-mysql-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-nats-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-nginx-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-node-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-operator-admission-webhook-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-operator-crds-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:crds-chart-2 | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (absent from https://prometheus-community.github.io/helm-charts crds@91.6.0, https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages crds@91.6.0; not verifiable: g… |
| artifact:prometheus-pgbouncer-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-pingdom-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-pingmesh-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-postgres-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-pushgateway-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-rabbitmq-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-redis-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-smartctl-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-snmp-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-sql-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-stackdriver-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-statsd-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-systemd-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-to-sd-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-windows-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-yet-another-cloudwatch-exporter-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:prometheus-chart | annotate | validate.unverifiable | computed | Kept unverified: 91.6.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 91.7.0 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:admission-webhook | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus-operator/admission-webhook:91.6.0); 91.7.0 fail (absent from quay.io/prometheus-operator/admission-webhook:91.7.0); 91.7.1 fail (absent from quay.io/… |
| artifact:alertmanager | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus/alertmanager:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/alertmanager:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/alertmanager:91.7… |
| artifact:blackbox-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus/blackbox-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/blackbox-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/blackbo… |
| artifact:couchdb-prometheus-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from docker.io/gesellix/couchdb-prometheus-exporter:kube-prometheus-stack-91.6.0); 91.7.0 fail (absent from docker.io/gesellix/couchdb-prometheus-exporter:kube-prometheus-st… |
| artifact:elasticsearch-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/elasticsearch-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/elasticsearch-exporter:91.7.0); 91.7.1 fail (absent fro… |
| artifact:ipmi-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from docker.io/prometheuscommunity/ipmi-exporter:91.6.0); 91.7.0 fail (absent from docker.io/prometheuscommunity/ipmi-exporter:91.7.0); 91.7.1 fail (absent from docker.io/pr… |
| artifact:json-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/json-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/json-exporter:91.7.0); 91.7.1 fail (absent from quay.io/promethe… |
| artifact:kube-rbac-proxy | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/brancz/kube-rbac-proxy:kube-prometheus-stack-91.6.0); 91.7.0 fail (absent from quay.io/brancz/kube-rbac-proxy:kube-prometheus-stack-91.7.0); 91.7.1 fail (absent… |
| artifact:kube-state-metrics | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from registry.k8s.io/kube-state-metrics/kube-state-metrics:91.6.0); 91.7.0 fail (absent from registry.k8s.io/kube-state-metrics/kube-state-metrics:91.7.0); 91.7.1 fail (abse… |
| artifact:kube-webhook-certgen | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from ghcr.io/jkroepke/kube-webhook-certgen:kube-prometheus-stack-91.6.0); 91.7.0 fail (absent from ghcr.io/jkroepke/kube-webhook-certgen:kube-prometheus-stack-91.7.0); 91.7.… |
| artifact:mysqld-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus/mysqld-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/mysqld-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/mysqld-expo… |
| artifact:nginx-prometheus-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from docker.io/nginx/nginx-prometheus-exporter:91.6.0); 91.7.0 fail (absent from docker.io/nginx/nginx-prometheus-exporter:91.7.0); 91.7.1 fail (absent from docker.io/nginx/… |
| artifact:node-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus/node-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/node-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/node-exporter:9… |
| artifact:pgbouncer-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/pgbouncer-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/pgbouncer-exporter:91.7.0); 91.7.1 fail (absent from quay.i… |
| artifact:postgres-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/postgres-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/postgres-exporter:91.7.0); 91.7.1 fail (absent from quay.io/… |
| artifact:prom-label-proxy | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/prom-label-proxy:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/prom-label-proxy:91.7.0); 91.7.1 fail (absent from quay.io/pr… |
| artifact:prometheus | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus/prometheus:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/prometheus:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/prometheus:91.7.1); 9… |
| artifact:prometheus-adapter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from registry.k8s.io/prometheus-adapter/prometheus-adapter:91.6.0); 91.7.0 fail (absent from registry.k8s.io/prometheus-adapter/prometheus-adapter:91.7.0); 91.7.1 fail (abse… |
| artifact:prometheus-config-reloader | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus-operator/prometheus-config-reloader:91.6.0); 91.7.0 fail (absent from quay.io/prometheus-operator/prometheus-config-reloader:91.7.0); 91.7.1 fail (ab… |
| artifact:prometheus-nats-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from docker.io/natsio/prometheus-nats-exporter:91.6.0); 91.7.0 fail (absent from docker.io/natsio/prometheus-nats-exporter:91.7.0); 91.7.1 fail (absent from docker.io/natsio… |
| artifact:prometheus-operator | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus-operator/prometheus-operator:91.6.0); 91.7.0 fail (absent from quay.io/prometheus-operator/prometheus-operator:91.7.0); 91.7.1 fail (absent from quay… |
| artifact:prometheus-to-sd | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from gcr.io/google-containers/prometheus-to-sd:91.6.0); 91.7.0 fail (absent from gcr.io/google-containers/prometheus-to-sd:91.7.0); 91.7.1 fail (absent from gcr.io/google-co… |
| artifact:pushgateway | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus/pushgateway:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/pushgateway:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/pushgateway:91.7.1)… |
| artifact:smartctl-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/smartctl-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/smartctl-exporter:91.7.0); 91.7.1 fail (absent from quay.io/… |
| artifact:snmp-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheus/snmp-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheus/snmp-exporter:91.7.0); 91.7.1 fail (absent from quay.io/prometheus/snmp-exporter:9… |
| artifact:stackdriver-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from docker.io/prometheuscommunity/stackdriver-exporter:91.6.0); 91.7.0 fail (absent from docker.io/prometheuscommunity/stackdriver-exporter:91.7.0); 91.7.1 fail (absent fro… |
| artifact:systemd-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/systemd-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/systemd-exporter:91.7.0); 91.7.1 fail (absent from quay.io/pr… |
| artifact:windows-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from ghcr.io/prometheus-community/windows-exporter:91.6.0); 91.7.0 fail (absent from ghcr.io/prometheus-community/windows-exporter:91.7.0); 91.7.1 fail (absent from ghcr.io/… |
| artifact:yet-another-cloudwatch-exporter | drop | validate.failed | computed | Relationship check failed: 91.6.0 fail (absent from quay.io/prometheuscommunity/yet-another-cloudwatch-exporter:91.6.0); 91.7.0 fail (absent from quay.io/prometheuscommunity/yet-another-cloudwatch-exporter:91.7.0); 91.7.… |

<details><summary>21 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| changelog `charts/prometheus-cloudwatch-exporter/CHANGELOG.md` | changelog.stale | Changelog is not maintained: newest entry 0.14.0 while the latest release is kube-prometheus-stack-91.8.2. |
| image `docker.io/maxwo/snmp-notifier` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/jiralert/jiralert-linux-amd64` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/thanos/thanos` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/cloudwatch-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/jwkohnen/conntrack-stats-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/consul-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/fastly/fastly-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/apache/kafka-native` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/danielqsj/kafka-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/memcached-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/openenergyprojects/modbus_exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/openenergyprojects/config-reloader-sidecar` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/percona/mongodb_exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/prometheus/busybox` | image.non-release-tags | Only snapshot/floating tags (floating) are produced, e.g. by branch builds. |
| image `ghcr.io/kokuwaio/pingdom-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/dongjiang1989/pingmesh-agent` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/kbudde/rabbitmq-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/oliver006/redis_exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/justwatchcom/sql_exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/statsd-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |

</details>

## Candidates


### build-tool (1)

- `chart-releaser` (medium; workflow.chart-releaser) — [prometheus-community/helm-charts:.github/workflows/release.yaml L64](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/.github/workflows/release.yaml#L64): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`

### changelog (1)

- `charts/prometheus-cloudwatch-exporter/CHANGELOG.md` (high; docs.changelog) — [prometheus-community/helm-charts:charts/prometheus-cloudwatch-exporter/CHANGELOG.md L9](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-cloudwatch-exporter/CHANGELOG.md#L9): `## 0.14.0`

### crd (10)

- `charts/kube-prometheus-stack/charts/crds/crds/crd-alertmanagerconfigs.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-alertmanagerconfigs.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-alertmanagerconfigs.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-alertmanagers.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-alertmanagers.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-alertmanagers.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-podmonitors.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-podmonitors.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-podmonitors.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-probes.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-probes.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-probes.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-prometheusagents.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-prometheusagents.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-prometheusagents.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-prometheuses.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-prometheuses.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-prometheuses.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-prometheusrules.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-prometheusrules.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-prometheusrules.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-scrapeconfigs.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-scrapeconfigs.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-scrapeconfigs.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-servicemonitors.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-servicemonitors.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-servicemonitors.yaml#L4): `kind: CustomResourceDefinition`
- `charts/kube-prometheus-stack/charts/crds/crds/crd-thanosrulers.yaml` (high; crd.file) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/crds/crd-thanosrulers.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/crds/crd-thanosrulers.yaml#L4): `kind: CustomResourceDefinition`

### helm-chart (46)

- `alertmanager-snmp-notifier@charts/alertmanager-snmp-notifier` (high; helm.chart) — [prometheus-community/helm-charts:charts/alertmanager-snmp-notifier/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager-snmp-notifier/Chart.yaml#L2): `name: alertmanager-snmp-notifier`
- `alertmanager@charts/alertmanager` (high; helm.chart) — [prometheus-community/helm-charts:charts/alertmanager/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager/Chart.yaml#L2): `name: alertmanager`
- `crds@charts/kube-prometheus-stack/charts/crds` (high; helm.chart) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/charts/crds/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/charts/crds/Chart.yaml#L2): `name: crds`
- `crds@charts/prometheus-operator-crds/charts/crds` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-operator-crds/charts/crds/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-operator-crds/charts/crds/Chart.yaml#L2): `name: crds`
- `jiralert@charts/jiralert` (high; helm.chart) — [prometheus-community/helm-charts:charts/jiralert/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/jiralert/Chart.yaml#L2): `name: jiralert`
- `kube-prometheus-stack@charts/kube-prometheus-stack` (high; helm.chart) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/Chart.yaml L27](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/Chart.yaml#L27): `name: kube-prometheus-stack`
- `kube-state-metrics@charts/kube-state-metrics` (high; helm.chart) — [prometheus-community/helm-charts:charts/kube-state-metrics/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-state-metrics/Chart.yaml#L2): `name: kube-state-metrics`
- `prom-label-proxy@charts/prom-label-proxy` (high; helm.chart) — [prometheus-community/helm-charts:charts/prom-label-proxy/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prom-label-proxy/Chart.yaml#L2): `name: prom-label-proxy`
- `prometheus-adapter@charts/prometheus-adapter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-adapter/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-adapter/Chart.yaml#L2): `name: prometheus-adapter`
- `prometheus-blackbox-exporter@charts/prometheus-blackbox-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-blackbox-exporter/Chart.yaml L3](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-blackbox-exporter/Chart.yaml#L3): `name: prometheus-blackbox-exporter`
- `prometheus-cloudwatch-exporter@charts/prometheus-cloudwatch-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-cloudwatch-exporter/Chart.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-cloudwatch-exporter/Chart.yaml#L4): `name: prometheus-cloudwatch-exporter`
- `prometheus-conntrack-stats-exporter@charts/prometheus-conntrack-stats-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-conntrack-stats-exporter/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-conntrack-stats-exporter/Chart.yaml#L2): `name: prometheus-conntrack-stats-exporter`
- `prometheus-consul-exporter@charts/prometheus-consul-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-consul-exporter/Chart.yaml L5](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-consul-exporter/Chart.yaml#L5): `name: prometheus-consul-exporter`
- `prometheus-couchdb-exporter@charts/prometheus-couchdb-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-couchdb-exporter/Chart.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-couchdb-exporter/Chart.yaml#L4): `name: prometheus-couchdb-exporter`
- `prometheus-druid-exporter@charts/prometheus-druid-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-druid-exporter/Chart.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-druid-exporter/Chart.yaml#L4): `name: prometheus-druid-exporter`
- `prometheus-elasticsearch-exporter@charts/prometheus-elasticsearch-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-elasticsearch-exporter/Chart.yaml L3](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-elasticsearch-exporter/Chart.yaml#L3): `name: prometheus-elasticsearch-exporter`
- `prometheus-fastly-exporter@charts/prometheus-fastly-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-fastly-exporter/Chart.yaml L5](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-fastly-exporter/Chart.yaml#L5): `name: prometheus-fastly-exporter`
- `prometheus-ipmi-exporter@charts/prometheus-ipmi-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-ipmi-exporter/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-ipmi-exporter/Chart.yaml#L2): `name: prometheus-ipmi-exporter`
- `prometheus-json-exporter@charts/prometheus-json-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-json-exporter/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-json-exporter/Chart.yaml#L2): `name: prometheus-json-exporter`
- `prometheus-kafka-exporter@charts/prometheus-kafka-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-kafka-exporter/Chart.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-kafka-exporter/Chart.yaml#L4): `name: prometheus-kafka-exporter`
- `prometheus-memcached-exporter@charts/prometheus-memcached-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-memcached-exporter/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-memcached-exporter/Chart.yaml#L2): `name: prometheus-memcached-exporter`
- `prometheus-modbus-exporter@charts/prometheus-modbus-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-modbus-exporter/Chart.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-modbus-exporter/Chart.yaml#L2): `name: prometheus-modbus-exporter`
- `prometheus-mongodb-exporter@charts/prometheus-mongodb-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-mongodb-exporter/Chart.yaml L18](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mongodb-exporter/Chart.yaml#L18): `name: prometheus-mongodb-exporter`
- `prometheus-mysql-exporter@charts/prometheus-mysql-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-mysql-exporter/Chart.yaml L3](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mysql-exporter/Chart.yaml#L3): `name: prometheus-mysql-exporter`
- `prometheus-nats-exporter@charts/prometheus-nats-exporter` (high; helm.chart) — [prometheus-community/helm-charts:charts/prometheus-nats-exporter/Chart.yaml L5](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nats-exporter/Chart.yaml#L5): `name: prometheus-nats-exporter`
- … 21 more

### helm-oci (43)

- `ghcr.io/prometheus-community/charts/alertmanager` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/alertmanager/README.md L17](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager/README.md#L17): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/alertmanager'`
- `ghcr.io/prometheus-community/charts/alertmanager-snmp-notifier` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/alertmanager-snmp-notifier/README.md L15](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager-snmp-notifier/README.md#L15): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/alertmanager-snmp-notifier'`
- `ghcr.io/prometheus-community/charts/jiralert` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/jiralert/Readme.md L11](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/jiralert/Readme.md#L11): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/jiralert'`
- `ghcr.io/prometheus-community/charts/kube-prometheus-stack` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/README.md L18](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/README.md#L18): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/kube-prometheus-stack'`
- `ghcr.io/prometheus-community/charts/kube-state-metrics` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/kube-state-metrics/README.md L9](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-state-metrics/README.md#L9): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/kube-state-metrics'`
- `ghcr.io/prometheus-community/charts/prom-label-proxy` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prom-label-proxy/README.md L5](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prom-label-proxy/README.md#L5): `**Homepage:** <https://github.com/oci://ghcr.io/prometheus-community/charts/prom-label-proxy>`
- `ghcr.io/prometheus-community/charts/prometheus` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus/README.md L16](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus/README.md#L16): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus'`
- `ghcr.io/prometheus-community/charts/prometheus-adapter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-adapter/README.md L13](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-adapter/README.md#L13): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-adapter'`
- `ghcr.io/prometheus-community/charts/prometheus-blackbox-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-blackbox-exporter/README.md L18](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-blackbox-exporter/README.md#L18): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-blackbox-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-cloudwatch-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-cloudwatch-exporter/README.md L16](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-cloudwatch-exporter/README.md#L16): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-cloudwatch-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-conntrack-stats-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-conntrack-stats-exporter/README.md L17](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-conntrack-stats-exporter/README.md#L17): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-conntrack-stats-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-consul-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-consul-exporter/README.md L16](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-consul-exporter/README.md#L16): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-consul-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-couchdb-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-couchdb-exporter/README.md L17](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-couchdb-exporter/README.md#L17): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-couchdb-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-druid-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-druid-exporter/README.md L27](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-druid-exporter/README.md#L27): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-druid-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-elasticsearch-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-elasticsearch-exporter/README.md L17](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-elasticsearch-exporter/README.md#L17): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-elasticsearch-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-fastly-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-fastly-exporter/README.md L15](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-fastly-exporter/README.md#L15): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-fastly-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-ipmi-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-ipmi-exporter/README.md L15](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-ipmi-exporter/README.md#L15): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-ipmi-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-json-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-json-exporter/README.md L16](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-json-exporter/README.md#L16): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-json-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-kafka-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-kafka-exporter/README.md L18](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-kafka-exporter/README.md#L18): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-kafka-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-memcached-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-memcached-exporter/README.md L16](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-memcached-exporter/README.md#L16): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-memcached-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-modbus-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-modbus-exporter/README.md L41](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-modbus-exporter/README.md#L41): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-modbus-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-mongodb-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-mongodb-exporter/README.md L12](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mongodb-exporter/README.md#L12): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-mongodb-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-mysql-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-mysql-exporter/README.md L11](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mysql-exporter/README.md#L11): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-mysql-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-nats-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-nats-exporter/README.md L11](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nats-exporter/README.md#L11): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-nats-exporter'`
- `ghcr.io/prometheus-community/charts/prometheus-nginx-exporter` (medium; docs.oci-ref) — [prometheus-community/helm-charts:charts/prometheus-nginx-exporter/README.md L14](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nginx-exporter/README.md#L14): `- OCI Artifact: 'oci://ghcr.io/prometheus-community/charts/prometheus-nginx-exporter'`
- … 18 more

### helm-repo (2)

- `https://prometheus-community.github.io/helm-charts` (medium; docs.helm-repo-add, workflow.chart-releaser-pages) — [prometheus-community/helm-charts:.github/workflows/release.yaml L64](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/.github/workflows/release.yaml#L64): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`
- `https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages` (medium; workflow.chart-releaser-pages-raw) — [prometheus-community/helm-charts:.github/workflows/release.yaml L64](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/.github/workflows/release.yaml#L64): `uses: helm/chart-releaser-action@cae68fefc6b5f367a0275617c9f83181ba54714f # v1.7.0`

### image (49)

- `docker.io/danielqsj/kafka-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-kafka-exporter/values.yaml L2](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-kafka-exporter/values.yaml#L2): `repository: docker.io/danielqsj/kafka-exporter`
- `docker.io/dongjiang1989/pingmesh-agent` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-pingmesh-exporter/values.yaml L71](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pingmesh-exporter/values.yaml#L71): `repository: dongjiang1989/pingmesh-agent`
- `docker.io/gesellix/couchdb-prometheus-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-couchdb-exporter/values.yaml L20](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-couchdb-exporter/values.yaml#L20): `repository: gesellix/couchdb-prometheus-exporter`
- `docker.io/jwkohnen/conntrack-stats-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-conntrack-stats-exporter/values.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-conntrack-stats-exporter/values.yaml#L4): `repository: jwkohnen/conntrack-stats-exporter`
- `docker.io/kbudde/rabbitmq-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-rabbitmq-exporter/values.yaml L6](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-rabbitmq-exporter/values.yaml#L6): `repository: kbudde/rabbitmq-exporter`
- `docker.io/maxwo/snmp-notifier` (high; helm.values-image) — [prometheus-community/helm-charts:charts/alertmanager-snmp-notifier/values.yaml L4](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/alertmanager-snmp-notifier/values.yaml#L4): `repository: maxwo/snmp-notifier`
- `docker.io/natsio/prometheus-nats-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-nats-exporter/values.yaml L8](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nats-exporter/values.yaml#L8): `repository: natsio/prometheus-nats-exporter`
- `docker.io/nginx/nginx-prometheus-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-nginx-exporter/values.yaml L22](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-nginx-exporter/values.yaml#L22): `repository: nginx/nginx-prometheus-exporter`
- `docker.io/oliver006/redis_exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-redis-exporter/values.yaml L13](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-redis-exporter/values.yaml#L13): `repository: oliver006/redis_exporter`
- `docker.io/openenergyprojects/config-reloader-sidecar` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-modbus-exporter/values.yaml L109](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-modbus-exporter/values.yaml#L109): `repository: openenergyprojects/config-reloader-sidecar`
- `docker.io/openenergyprojects/modbus_exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-modbus-exporter/values.yaml L100](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-modbus-exporter/values.yaml#L100): `repository: openenergyprojects/modbus_exporter`
- `docker.io/percona/mongodb_exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-mongodb-exporter/values.yaml L21](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-mongodb-exporter/values.yaml#L21): `repository: percona/mongodb_exporter`
- `docker.io/prom/cloudwatch-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-cloudwatch-exporter/values.yaml L8](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-cloudwatch-exporter/values.yaml#L8): `repository: prom/cloudwatch-exporter`
- `docker.io/prom/consul-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-consul-exporter/values.yaml L20](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-consul-exporter/values.yaml#L20): `repository: prom/consul-exporter`
- `docker.io/prom/memcached-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-memcached-exporter/values.yaml L8](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-memcached-exporter/values.yaml#L8): `repository: prom/memcached-exporter`
- `docker.io/prom/statsd-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-statsd-exporter/values.yaml L9](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-statsd-exporter/values.yaml#L9): `repository: prom/statsd-exporter`
- `docker.io/prometheuscommunity/ipmi-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-ipmi-exporter/values.yaml L8](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-ipmi-exporter/values.yaml#L8): `repository: registry.hub.docker.com/prometheuscommunity/ipmi-exporter`
- `docker.io/prometheuscommunity/stackdriver-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-stackdriver-exporter/values.yaml L17](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-stackdriver-exporter/values.yaml#L17): `repository: prometheuscommunity/stackdriver-exporter`
- `gcr.io/google-containers/prometheus-to-sd` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-to-sd/values.yaml L3](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-to-sd/values.yaml#L3): `repository: gcr.io/google-containers/prometheus-to-sd`
- `ghcr.io/fastly/fastly-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-fastly-exporter/values.yaml L17](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-fastly-exporter/values.yaml#L17): `repository: ghcr.io/fastly/fastly-exporter`
- `ghcr.io/jkroepke/kube-webhook-certgen` (high; helm.values-image) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/values.yaml L3299](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/values.yaml#L3299): `repository: jkroepke/kube-webhook-certgen`
- `ghcr.io/justwatchcom/sql_exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-sql-exporter/values.yaml L6](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-sql-exporter/values.yaml#L6): `repository: ghcr.io/justwatchcom/sql_exporter`
- `ghcr.io/kokuwaio/pingdom-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-pingdom-exporter/values.yaml L8](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-pingdom-exporter/values.yaml#L8): `repository: ghcr.io/kokuwaio/pingdom-exporter`
- `ghcr.io/prometheus-community/windows-exporter` (high; helm.values-image) — [prometheus-community/helm-charts:charts/prometheus-windows-exporter/values.yaml L7](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/prometheus-windows-exporter/values.yaml#L7): `repository: prometheus-community/windows-exporter`
- `quay.io/brancz/kube-rbac-proxy` (high; helm.values-image) — [prometheus-community/helm-charts:charts/kube-state-metrics/values.yaml L143](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-state-metrics/values.yaml#L143): `repository: brancz/kube-rbac-proxy`
- … 24 more

### registry (1)

- `ghcr.io` (medium; workflow.registry-login) — [prometheus-community/helm-charts:.github/workflows/release.yaml L79](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/.github/workflows/release.yaml#L79): `registry: ghcr.io`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [prometheus-community/helm-charts:SECURITY.md L1](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/SECURITY.md#L1): `# Reporting a security issue`

### tag-scheme (1)

- `^kube-prometheus-stack-(?P<version>\d+\.\d+\.\d+)$` (high; tags.ls-remote) — [prometheus-community/helm-charts/releases/tag/kube-prometheus-stack-91.8.2 refs/tags/kube-prometheus-stack-91.8.2](https://github.com/prometheus-community/helm-charts/releases/tag/kube-prometheus-stack-91.8.2#refs/tags/kube-prometheus-stack-91.8.2): `7d82f66c37f960ef987ca05b5c189a85dc504ba3 refs/tags/kube-prometheus-stack-91.8.2`

### version-relation (1)

- `chart.version = {{.Version}}` (high; helm.chart-version-matches-ref) — [prometheus-community/helm-charts:charts/kube-prometheus-stack/Chart.yaml L31](https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.8.2/charts/kube-prometheus-stack/Chart.yaml#L31): `version: 91.8.2`
