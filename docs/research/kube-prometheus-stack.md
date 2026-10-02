# kube-prometheus-stack release-channel map (research date 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the checks and probes were re-run live; the live `ri check` outcomes are unchanged. Any unreachable host recorded below was a transient anonymous rate limit or an anonymous-auth refusal, not a block of this product's channels (details: docs/rerun/REPORT.md, per-product table). The original text below is kept as the record of 2026-10-01.

Scope: the **Helm chart** `kube-prometheus-stack` as published by the chart *monorepository*
`github.com/prometheus-community/helm-charts` (~44 charts, one tag family per chart, one shared
`gh-pages` Helm repository). The product is deliberately chart-centric: the chart version is the
release, and the "software" is an aggregation of sub-components pinned per chart release.

"RAW" = `https://raw.githubusercontent.com/prometheus-community/helm-charts/<ref>/<path>`, "index" =
the `gh-pages` branch's `index.yaml` (6.4 MB, regenerated on every chart release).

Reachable from this sandbox (all verified 2026-10-01): `git ls-remote`/clones of github.com,
`raw.githubusercontent.com` (branches and tags), `https://prometheus-community.github.io/helm-charts/index.yaml`
(HTTP 200 — unlike most `*.github.io` hosts), `api.github.com` anonymous (200, 60 req/h budget),
`ghcr.io` OCI manifests via the anonymous token endpoint (charts under
`ghcr.io/prometheus-community/charts/*` and `ghcr.io/grafana-community/helm-charts/grafana`;
needs an Accept header that includes `application/vnd.oci.image.manifest.v1+json`, a
helm-config-only Accept gets 404), `quay.io` manifests (200). `registry.k8s.io` answers 307
(redirect chain not followed); `registry-1.docker.io` answers 401 to anonymous curl (the OCI
adapter's token dance and the `mirror.gcr.io` fallback exist for that).

## 1. Canonical versions: one tag family among 3501

`git tag -l` → 3501 tags. `kube-prometheus-stack-NdN.N.N` (NdN = the chart version, e.g.
`kube-prometheus-stack-91.8.2`): **1151 stable tags, no pre-releases, no other shapes**; the other
2350 tags belong to sibling charts (`prometheus-...-N.N.N`, `alertmanager-...`, `kube-state-metrics-...`,
...). First: `kube-prometheus-stack-9.3.4` (the chart was renamed from `prometheus-operator` at 9.x;
the old family still exists in the repo). Latest: **91.8.2, 2026-09-29**.

- **The chart version is the release version.** `charts/kube-prometheus-stack/Chart.yaml` at the tag
  says `version: 91.8.2` verbatim — the product needs no version mapping, `tagPrefix:
  kube-prometheus-stack-` selects the family.
- **Lineage.** Every chart major N has a handful of minors (91.0.0 … 91.8.2) that build on each other
  sequentially, and majors are bumped for *any* breaking change (the README: "CRDs update lead to a
  major version bump"). Major N's minors are never maintained in parallel after N+1 exists — the
  train behaves like `lineage: minor` with the chart major as the semver major.
- Chart tags are cut by Renovate-style PRs merged to `main`; the tag points at the merge commit.
  Each tag also gets a GitHub release (see §3).

## 2. The aggregation topology (the point of this onboarding)

`charts/kube-prometheus-stack/Chart.yaml` at `kube-prometheus-stack-91.8.2`:

```yaml
version: 91.8.2
appVersion: v0.94.1          # prometheus-operator, renovate-pinned per release
kubeVersion: ">=1.25.0-0"
dependencies:
  - name: crds                      # local subchart (charts/crds), version "0.0.0" placeholder
  - name: kube-state-metrics        version: "8.6.0"   repository: oci://ghcr.io/prometheus-community/charts
  - name: prometheus-node-exporter  version: "4.59.0"  repository: oci://ghcr.io/prometheus-community/charts
  - name: grafana                   version: "13.2.7"  repository: oci://ghcr.io/grafana-community/helm-charts
  - name: prometheus-windows-exporter version: "0.12.*"  # a RANGE, not a version
```

Per-release component pins (this is what an upgrade *means* for this product):

| Component | Where pinned | Era |
|---|---|---|
| prometheus-operator (the operator the chart deploys) | `appVersion` (vX.Y.Z) | `v` prefix since 44.0.0; before: `0.61.1` style (43.x and older) |
| kube-state-metrics chart | `dependencies[kube-state-metrics].version` | exact pins since **74.0.0**; ranges (`"5.33.*"`) before |
| prometheus-node-exporter chart | `dependencies[prometheus-node-exporter].version` | exact since **72.0.0** |
| grafana chart | `dependencies[grafana].version` | exact since **73.0.0**; repository `grafana.github.io` → `grafana-community.github.io` → **oci://ghcr.io/grafana-community since 90.0.0** |
| prometheus-windows-exporter chart | `dependencies[...].version` | **still a range today** (`"0.12.*`) — not a version, dependency added in 48.0.0 |
| prometheus, alertmanager, thanos images | `values.yaml` defaults (`prometheus.prometheusSpec.image.tag: v3.15.0-distroless`, `alertmanager.alertmanagerSpec.image.tag: v0.34.1`, `thanos` twice: `v0.42.4`) | quay.io, `registry: quay.io` + `repository:` next to the tag |
| kube-webhook-certgen image | `values.yaml` (`jkroepke/kube-webhook-certgen: 1.8.9`, docker.io) | |

`Chart.lock` (exact resolved versions for the range era) exists up to ~47.x and is gone from 48.0.0
(the crds-subchart restructure); the modern era pins exactly in `Chart.yaml`, annotated with renovate
comments (`# renovate: github=prometheus-operator/prometheus-operator`).

`appVersion` = "refers to the prometheus-operator's version with matching CRDs" (README §Upgrading):
the operator version is the join between this product and the `prometheus-operator` product
(wave 3), and CRD content follows it.

No existing construct can read any of these pins as an *artifact version*: `lookup` searches an
index for entries whose `field` matches the **release** version (the mirror direction, used by
vault / external-secrets / ingress-nginx for "the chart whose appVersion is my release"); here the
release *is* the chart and the component version must be **read out of a file at the release tag**.
A generic `version.strategy: field` was added (see the onboarding record).

## 3. Release notes

- **GitHub release per chart tag** (`github.com/prometheus-community/helm-charts/releases/tag/kube-prometheus-stack-91.8.2`,
  created by github-actions): body = chart description + "**What's Changed**" bullets with exactly
  this chart's PRs ("[kube-prometheus-stack] Update grafana Docker tag to v13.2.7 by @renovate[bot]
  in …/pull/7336") + a "Full Changelog" compare link to the previous chart tag. Assets:
  `kube-prometheus-stack-91.8.2.tgz` and `.tgz.prov` (Helm provenance). One API request per
  release via `GET /repos/…/releases/tags/{tag}`; listing all releases instead pages through
  ~3500 of them (~35 requests) — the per-tag fetch is the affordable shape.
- `git-log {{.PrevTag}}..{{.Tag}}` covers the same commits without the API **but interleaves
  sibling-chart commits** between two chart tags (`[prometheus-redis-exporter] …`,
  `[prometheus] …` between 91.7.1 and 91.8.2): the git-log locator has no path filter and classify
  rules cannot express "not `[kube-prometheus-stack]`" without lookahead. Rejected as a source;
  noted as a discovery/gap item.
- No CHANGELOG.md for this chart (the `prometheus-cloudwatch-exporter` chart keeps one; not KPS).

## 4. Upgrade docs

- `charts/kube-prometheus-stack/UPGRADE.md` (in-tree, read at the release tag): cumulative, one
  `## From NN.x to NN+1.x` section per chart major, newest first, back to `From 6.x to 7.x`.
  Written for the transition *into* major N. Exists since **70.0.0**; before that the same sections
  lived in the chart README (`### From NN.x to NN+1.x` under `### Upgrading an existing Release to a
  new major version`), same heading text, back to `From 5.x to 6.x`.
- Content shape: "This version upgrades Prometheus-Operator to v0.94.0", manual `kubectl apply` CRD
  update commands pinned to the operator version, RBAC/behaviour changes, `helm upgrade --set`
  migrations (e.g. renamed values keys), and notes when a major only bumps dependencies.
- The README `### Upgrading Chart` section adds the standing rule: CRDs are not updated by Helm 3
  and majors signal breaking changes; `crds.upgradeJob.enabled` (since 68.4.0) automates it.

## 5. Compatibility

- `Chart.yaml` `kubeVersion`: `>=1.16.0-0` (≤ 49.x) → `>=1.19.0-0` (50.0.0) → `>=1.25.0-0`
  (75.0.0). Read by the existing `contents: chart-metadata` construct (constraint kind
  `chart-kubeVersion`); no separate support matrix is published.
- Prometheus Operator's own k8s support matrix (upstream repo) is only implied via `appVersion`.

## 6. CRDs

- 10 CRD files (`crd-alertmanagerconfigs`, `crd-alertmanagers`, `crd-podmonitors`, `crd-probes`,
  `crd-prometheusagents`, `crd-prometheuses`, `crd-prometheusrules`, `crd-scrapeconfigs`,
  `crd-servicemonitors`, `crd-thanosrulers` — `prometheusagents` is the newest).
- Location moved twice: `charts/kube-prometheus-stack/crds/*.yaml` (< 48.0.0) → subchart
  `charts/kube-prometheus-stack/charts/crds/crds/*.yaml` (≥ 48.0.0; the subchart also ships
  `files/crds.bz2` + an `upgrade` Job for in-cluster upgrades).
- CRD content tracks `appVersion` (operator), not the chart version.

## 7. Security

- No product-level CVE feed. `SECURITY.md` points to `https://prometheus.io/docs/operating/security/`
  (community policy page, not a per-chart channel). Sibling-repo `github-advisories` is declared as
  the security role (near-empty for a chart monorepo but it is the only upstream channel); component
  CVEs (prometheus, operator, grafana) live in the components' own advisories — unreachable from
  this product without joining on component versions (see record `gaps`).

## 8. Publication channels (all verified reachable)

| Channel | Coordinate | Notes |
|---|---|---|
| Helm repository | `https://prometheus-community.github.io/helm-charts` (index 200; 1148 KPS entries) | canonical; per-entry `created`, `digest`, appVersion, download URL |
| Helm repository (mirror) | `https://raw.githubusercontent.com/prometheus-community/helm-charts/gh-pages` | same bytes via RAW |
| git tags | `kube-prometheus-stack-N.N.N` | canonical version list; dates via tagger |
| OCI chart | `ghcr.io/prometheus-community/charts/kube-prometheus-stack` (+ per-sub-chart repos) | anonymous token + multi-Accept |
| operator image | `quay.io/prometheus-operator/prometheus-operator:v0.94.1` | 200 |
| prometheus image | `quay.io/prometheus/prometheus:v3.15.0-distroless` | 200 |
| alertmanager image | `quay.io/prometheus/alertmanager:v0.34.1` | 200 |
| grafana chart | `ghcr.io/grafana-community/helm-charts/grafana:13.2.7` | since 90.0.0 (was grafana.github.io) |

## 9. Ambiguities worth recording

- Dependency *ranges* before the exact-pin era (and `prometheus-windows-exporter` today): the pin
  is a constraint, not a version — not representable as an artifact version.
- `appVersion` format change at 44.0.0 (`0.61.1` → `v0.62.0`): quay operator tags for the old era
  are not probed here (artifact availability starts at 44.0.0).
- Index vs tags (computed): every index entry has a tag; tags `14.6.3`, `18.0.4`, `19.2.0` have **no
  index entry** (1148 index versions vs 1151 stable tags) — releases that were tagged but never
  published to the Helm repository. They fail a `helm-repo` probe honestly (absent), and the
  `helm-git`/OCI channels still see them.
- `files/crds.bz2` inside the crds subchart duplicates the CRD YAMLs (packed); the plain YAMLs are
  the canonical snapshot input.
