# Prometheus Operator release-channel research (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the checks and probes were re-run live; the live `ri check` outcomes are unchanged. Any unreachable host recorded below was a transient anonymous rate limit or an anonymous-auth refusal, not a block of this product's channels (details: docs/rerun/REPORT.md, per-product table). The original text below is kept as the record of 2026-10-01.

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here.
`RAW` = `https://raw.githubusercontent.com/prometheus-operator/prometheus-operator`. Product:
Prometheus Operator — the Kubernetes operator for the `monitoring.coreos.com` API group
(Prometheus/PrometheusAgent/Alertmanager/AlertmanagerConfig/ThanosRuler/ServiceMonitor/PodMonitor/
Probe/ScrapeConfig/PrometheusRule). Canonical repository `github.com/prometheus-operator/prometheus-operator`
(formerly `coreos/prometheus-operator`). CNCF... no: a Prometheus community project (not CNCF-sponsored
itself; it sits in the Prometheus ecosystem).

## 0. Executive summary (what a declarative definition MUST know)

1. **Semver 0.x, `v`-prefixed, minor lines.** 381 tags total: **175 stable root tags** `v0.0.1` →
   **v0.94.1** (2026-09-23), 3 pre-releases (`v0.51.1-rc.0`, `v0.64.0-rc.0/1`), and **203 module
   tags** — `pkg/apis/monitoring/v0.NN.N` (104) and `pkg/client/v0.NN.N` (99), cut for the two Go
   submodules on every release (RELEASE.md), which never match a `v`-prefixed pattern. **6-week
   release cadence** (RELEASE.md schedule table; v0.95 planned 2026-10-14, v0.96 2026-11-25).
   Release branches `release-0.NN`; the newest line is patched, older lines "best effort" (no dated
   EOL policy exists — see §7).
2. **Release notes = `CHANGELOG.md` in-repo, one `## X.Y.Z / YYYY-MM-DD` section per release**
   (41 sections, oldest kept `0.50.0`-era shape is the same). Bullets are labelled
   `[CHANGE]` / `[FEATURE]` / `[ENHANCEMENT]` / `[BUGFIX]` (also combined `[CHANGE/BUGFIX]`), each
   closing with PR numbers; `> **Note:**` blockquote callouts carry feature-gate promotions
   (0.92.0's Beta-by-default note). The **GitHub release body is a byte-identical paste of the
   changelog section** (RELEASE.md: "paste in changes made to CHANGELOG.md"; verified identical for
   v0.94.0 and v0.80.0) — same text, two channels; the changelog is the quota-free one.
3. **No version-to-version upgrade guide exists** — not in-repo (`Documentation/` has no upgrading
   page; grep finds only incidental mentions) and not on the website (§8: the website mirrors the
   in-repo docs). The `[CHANGE]` bullets + Note callouts are the de facto migration guidance;
   the chart-side guidance lives in kube-prometheus-stack's UPGRADE.md (the other product of the
   join).
4. **Three container images per release, on TWO registries** (`scripts/push-docker-image.sh`,
   `publish.yaml` on tag push, multi-arch amd64/arm64/arm/ppc64le/s390x, cosign-signed):
   `prometheus-operator`, `prometheus-config-reloader`, `admission-webhook`.
   **quay.io is canonical** (tags back to the earliest era, manifests [200] at every probed tag);
   **ghcr.io is a mirror since v0.49.0** (404 at v0.48.0, 200 from v0.49.0). The
   **admission-webhook image starts at v0.55.0** (404 at v0.54.1, 200 from v0.55.0 on both
   registries; the webhook itself shipped in ~0.53 with the image publishing following at 0.55).
   Caveat: quay's `/tags/list` caps at the 100 oldest tags for anonymous clients (pagination
   ignored) — per-tag manifest probes work fine and are what matters.
5. **CRDs: 10 `monitoring.coreos.com` CRDs in two in-tree representations.**
   `example/prometheus-operator-crd/` (controller-gen over `./v1` + `./v1alpha1`) — the set that
   `bundle.yaml`, `stripped-down-crds.yaml` and the jsonnet lib are generated from; and
   `example/prometheus-operator-crd-full/` (over `./...`, adds the `v1beta1` AlertmanagerConfig
   served version for the conversion-webhook path). 8 CRDs at v0.60.0 (`scrapeconfigs` and
   `prometheusagents` missing), 10 from v0.65.0 on, identical file names in both dirs.
6. **Release assets: exactly two YAMLs per release** (release.yaml on release created):
   `bundle.yaml` (RBAC + the non-full CRD set + operator Deployment — the all-in-one install
   document, also committed in-tree) and `stripped-down-crds.yaml` (same CRDs minus `description`
   fields, for Argo-style serverside apply; upload step exists since the v0.60.0 era). **No binary
   assets**: the operator/webhook/reloader binaries ship only inside images; `cmd/` also holds
   `po-docgen` and `po-rule-migration`, published nowhere. **`po-lint` was deleted** (PR #6474,
   "removed the po-lint code and doc part"; before that it had been folded into a `po-tooling`
   image that no longer ships).
7. **Operand compatibility is the product's second dimension.** Machine-readable per-release
   source: `pkg/operator/defaults.go` at the tag — `DefaultAlertmanagerVersion` (`"v0.34.0"`),
   `DefaultThanosVersion` (`"v0.42.4"`), and `DefaultPrometheusVersion` = **last element of
   `PrometheusCompatibilityMatrix`** (`"v3.14.0"`), with base images
   `quay.io/prometheus/{prometheus,alertmanager}` and `quay.io/thanos/thanos`. The matrix-list
   form holds back to at least v0.40.0 (verified v0.40.0–v0.94.1), so the "last entry" pattern has
   one era. The full supported-versions MATRIX (all Prometheus versions the e2e suite verifies,
   v2.45.0 → v3.14.0 today) is set-valued and lives only in that Go slice + the generated
   compatibility page. `Documentation/getting-started/compatibility.md` at the tag states the
   Kubernetes requirement in prose: ">= v0.84.0 requires Kubernetes >= v1.25.0 (CEL in CRDs);
   releases before v0.84.0 require >= v1.16.0" — **that sentence was retro-corrected** (landed
   2025-12-18, PR #8187; first tag carrying it is v0.88.0; at the v0.83.0/v0.84.0 tags the page
   still said v1.16.0), so the per-tag page records what was believed true at each release.
   Support policy for operator lines: RELEASE.md's "maintaining the release branches for older
   minor releases happens on a best effort basis" — no dates, no per-line statements (nothing to
   model as lifecycle).
8. **No Helm chart in this repository.** `helm/` contains a tombstone README: the charts built
   here moved to `prometheus-community/helm-charts` and were combined into **kube-prometheus-stack**
   (order 6 in this catalog). The distribution charts that pin THIS product by `appVersion` are:
   - **`kube-prometheus-stack`** (1148 index entries): appVersion == the operator tag, `v`-prefixed
     from chart 44.0.0 (operator v0.62.0; unprefixed `0.38.1`–`0.61.1` before). **Not every
     operator release gets a chart**: v0.64.x, v0.84.0 and v0.90.0 have no entry (v0.90.1 and
     v0.91.0 do). v0.94.1 → chart 91.8.1/91.8.2.
   - **`prometheus-operator-crds`** (62 entries, CRD-only chart): appVersion tracks the operator
     from 0.60.1 on, roughly one chart per operator release (v0.94.1 → 32.0.1; v0.69.0 skipped).
   This is the **cross-product join designed into the phase-2 plan**: from the chart side it is
   `version.strategy: field` reading `appVersion` (kube-prometheus-stack's record); from THIS side
   it is `version.strategy: lookup` finding the chart whose appVersion == {{.Tag}} — the exact
   relation argo-cd introduced and kube-prometheus-stack's record pre-credited to this onboarding.
   Prefix eras: KPS appVersion is unprefixed (`0.38.1`–`0.61.1`) and **v-prefixed from chart 44.0.0
   (operator v0.62.0)**; the crds chart is unprefixed `0.60.1`–`0.67.0` and v-prefixed from chart
   6.0.0 (operator v0.68.0). Neither chart covers every operator release (KPS skips v0.64.x,
   v0.84.0, v0.90.0 — v0.90.1 and v0.91.0 DO exist; crds-chart skips v0.69.0), so both lookups are
   `optional`: absence is the chart cadence, not a broken relationship.
9. **Security**: GitHub repository security advisories (`github-advisories` adapter; SECURITY.md →
   prometheus.io process page, private GitHub vulnerability reporting). api.github.com answered
   anonymously [200] but **exhausted its 60/h quota during discovery's own validation run**
   (transient; recovered minutes later) — the standing tax of any notes source behind that API,
   which the in-repo CHANGELOG avoids entirely.
10. **Website**: `prometheus-operator.dev` = `github.com/prometheus-operator/website` (Hugo,
    netlify). `synchronize.sh` mirrors the in-repo `Documentation/` tree; `data/prometheusOperator.json`
    holds the current version pointer (mutable, not per-release — not a source).

Related products (NOT this definition): **kube-prometheus-stack** (order 6; the chart that pins
this operator), **prometheus-community/helm-charts** monorepo (its index serves the two lookup
charts), **Prometheus**, **Alertmanager**, **Thanos** (operands whose defaults this operator pins;
Thanos is also pinned by kube-prometheus-stack's values), `kube-prometheus` (the jsonnet mixin
distribution in a sibling repo).

## 1. Versions channel

- `git-tags` of the repository [200]: 381 tags; root `v*` = 175 stable + 3 rc; `pkg/apis/monitoring/*`
  (104) + `pkg/client/*` (99) submodule tags excluded by the `v` prefix.
- `github-releases` [200] as fallback (release per tag, `0.94.1 / 2026-09-23` naming).
- Pattern `^v(?P<version>0\.\d+\.\d+(?:-rc\.\d+)?)$` over the root family.

## 2. Notes channels

- `CHANGELOG.md` at `{{.Tag}}` [200] (markdown-section `## {version} / date`): canonical,
  historically validated by discovery 6/6 (0.92.0–0.94.1).
- GitHub release body [200]: byte-identical to the changelog section (verified v0.94.0, v0.80.0);
  modeled only as the versions fallback, not as a second notes source (would duplicate every item;
  api.github.com quota makes it flaky anyway).

## 3. Artifacts and their version relations

| Artifact | Relation | Channels | Notes |
|---|---|---|---|
| operator image | `{{.Tag}}` | oci quay (canonical) + oci ghcr (mirror ≥0.49.0) | references cross-check inside bundle.yaml |
| config-reloader image | `{{.Tag}}` | same | pinned `v{version.Version}` in defaults.go too |
| admission-webhook image | `{{.Tag}}`, **availability ≥0.55.0** | same | first tag v0.55.0 on both registries |
| bundle.yaml (install manifest) | `{{.Tag}}` | repo-file in-tree (canonical) + http release asset | image-refs contents |
| CRDs | `{{.Tag}}` | repo-dir `example/prometheus-operator-crd` (canonical) + http `stripped-down-crds.yaml` asset (≥0.60.0 era) | crds contents; `-full` twin noted, not modeled |
| prometheus default image | pattern on `pkg/operator/defaults.go` @{{.Tag}} | oci quay.io/prometheus/prometheus | last matrix entry |
| alertmanager default image | pattern on defaults.go | oci quay.io/prometheus/alertmanager | `DefaultAlertmanagerVersion` literal |
| thanos default image | pattern on defaults.go | oci quay.io/thanos/thanos | `DefaultThanosVersion` literal |
| kube-prometheus-stack chart | **lookup appVersion == {{.Tag}}**, select latest, optional | helm-repo index + oci ghcr | availability ≥0.62.0 (v-prefixed appVersion era); skips v0.64.x/v0.84.0/v0.90.0 |
| prometheus-operator-crds chart | lookup appVersion == {{.Tag}}, optional | helm-repo index + oci ghcr | availability ≥0.68.0 (v-prefix era); skips v0.69.0 |

## 4. Hosts verified from this sandbox (2026-10-01)

- `quay.io` [200]: anonymous auth token + per-tag manifest probes (three repos); `/tags/list`
  capped at the oldest 100 for anonymous (pagination ignored) — an index caveat, not an outage.
- `ghcr.io` [200]: anonymous token, manifest probes.
- `prometheus-community.github.io` [200]: index.yaml (the two lookup charts).
- `api.github.com` [200] then **403 rate limit during discovery validation** (transient, recovered);
  release-body and asset fetches that avoid the API are unaffected.
- `github.com` / `raw.githubusercontent.com` / `codeload` [200] (git clone + repo-file reads).
- `prometheus-operator.dev` — not needed (website mirrors in-repo docs; not a source).

## 5. Discovery baseline (what `ri discover` got right / wrong)

Right: tag scheme + junk exclusion, CHANGELOG section selector (historically validated 6/6), the
three quay images (all historically validated), bundle.yaml in-tree and as release asset, advisories,
github-releases versions fallback. Wrong/incomplete: (a) it annotated release-notes-github
`unverifiable` because the API quota died mid-validation — the channel itself is fine, the text is
the changelog anyway; (b) it proposed `bundle` and `bundle-manifest` as two artifacts (one content,
two channels); (c) the CRD candidates were excluded by its `crd.not-dedicated` rule (CRDs "bundled
in another manifest") — for this product the example/ dirs ARE the canonical CRD source; (d) it
missed: the ghcr mirror, the stripped-down-crds asset, the operand default pins (defaults.go), the
Kubernetes compatibility page, and the entire chart join (out-of-repo channels are invisible to the
repo scanner). The VERSION-file and Makefile ldflags candidates are true but not artifacts (no
published channel).
