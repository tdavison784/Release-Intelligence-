# Flux (flux2) release-channel map (research date 2026-10-01)

Scope: **`github.com/fluxcd/flux2`** — the user-facing Flux v2 product: the `flux` CLI
(goreleaser), the generated install manifests that bundle the controllers, and the release
metadata that pins them. Flux v2 is an **aggregator**: the six (since 2.7, seven) controllers
live in separate repositories under `fluxcd`, each with its own tags, images on
`ghcr.io/fluxcd/*` and changelogs; a flux2 release pins one version of each. The umbrella is
canonical: users install/upgrade `flux` and the pins move with the `v2.x` tags.

"RAW" = `https://raw.githubusercontent.com/fluxcd/flux2/<ref>/<path>`. Verified reachable
2026-10-01: github.com clones/ls-remote, raw.githubusercontent.com, codeload tarballs,
api.github.com anonymous (60/h; exhausted once during discovery, recovered within the hour —
transient), `ghcr.io` OCI manifests via the anonymous token endpoint (fluxcd + fluxcd-community
namespaces), `https://fluxcd-community.github.io/helm-charts/index.yaml` (200), fluxcd.io
docs pages. **Not reachable**: the OCI artifact `ghcr.io/flux-system/gotk-components.yaml`
(anonymous token request answers `DENIED` — the package is not anonymously pullable from this
sandbox) and `registry-1.docker.io` (anonymous 401, the mirror.gcr.io fallback pattern of
postgresql applies).

## 1. Canonical versions: `v2.x` tags of flux2

216 tags: `v0.0.1` … `v0.41.2` (0.x era, incl. alpha/beta/rc pre-releases), then **v2.0.0
(2022-05)** through **v2.9.6 (2026-10-01)** — there is no v1.x (the jump 0.x → 2.x aligned the
product with "Flux v2"). 206 stable tags, 10 pre-releases. `lineage: minor`; the project
maintains release branches for the most recent three minors (`release/v2.7.x`, `v2.8.x`,
`v2.9.x`) and supports the last three minors (website releases page). Tag dates: v2.7.0
2025-09-30, v2.8.0 2026-02-24, v2.9.0 2026-06-30, v2.9.6 2026-10-01.

## 2. The umbrella-pins-controllers topology

- **Where the pins live.** `manifests/bases/<controller>/kustomization.yaml` at the tag carries
  each controller as a kustomize *remote resource* whose URL embeds the pinned version —
  identical shape in every era (checked v0.20.1, v0.41.2, v2.0.0, v2.6.4, v2.9.6):
  `resources: [https://github.com/fluxcd/source-controller/releases/download/v1.9.6/source-controller.crds.yaml, ...deployment.yaml]`.
  v2.9.6 pins: source-controller **v1.9.6**, kustomize-controller **v1.9.6**,
  notification-controller **v1.9.4**, helm-controller **v1.6.5**, image-reflector-controller
  **v1.2.5**, image-automation-controller **v1.2.5**, source-watcher **v2.2.4**.
- **No clean YAML field exists**: `go.mod` pins the same versions
  (`github.com/fluxcd/source-controller/api v1.9.6`) but is not YAML (karpenter recorded the
  same gap for its libs pin); the install kustomization has `images:` with `newName` only (no
  `newTag` — ever); the release body lists the pins as *markdown links* under
  `## Components changelog`.
- **source-watcher** is the seventh controller: base added 2025-09-15, first release tag
  v2.7.0 (checked absent at v2.6.4). image-reflector/image-automation are opt-in
  (`--components-extra`) but always pinned.
- **Controller images**: `ghcr.io/fluxcd/<controller>:<pin>` (anonymous token 200 for
  v1.9.6/v0.17.1 spot checks); mirrors on `docker.io/fluxcd/*` are not anonymously pullable.
  The controllers are separate products in their own right; here they are *pinned sub-components*
  — the kube-prometheus-stack `version:field` shape, except the pin is inside a text string, not
  a YAML field (→ new construct `version:pattern`, see record).

## 3. Generated manifests (the install artifact)

`kustomize build manifests/install` → release asset **`install.yaml`** (verified at v2.9.6,
v2.3.0, v2.0.0, v0.41.2): one multi-doc YAML with the 7 controller Deployments — `image:
ghcr.io/fluxcd/source-controller:v1.9.6` — plus **15 CRDs** (GitRepository, OCIRepository,
Bucket, HelmRepository, HelmChart, HelmRelease, Kustomization, Receiver, Alert, Provider,
ImagePolicy/ImageRepository/ImageUpdateAutomation, ArtifactGenerator, ExternalArtifact —
`*.source.extensions.fluxcd.io` for the 2.9 additions). Also released: `manifests.tar.gz`,
`crd-schemas.tar.gz`, `flux_<v>_{linux,darwin,windows}_*` archives, `flux_<v>_sbom.spdx.json`,
checksums + sigs, `provenance.intoto.jsonl` (since v2.3.0).

OCI artifact twins (workflow `release-flux-manifests`, added between v2.0.0 and v2.1.0):
`ghcr.io/fluxcd/flux-manifests:<v>` (verified 200 at v2.1.0+) and
`ghcr.io/flux-system/gotk-components.yaml:<v>` (`flux install --export` output; **DENIED**
anonymously). In-repo CRDs are remote refs (`manifests/crds/kustomization.yaml` only), so the
CRD snapshot must come from the published install manifest, not the source tree.

## 4. CLI images and the flux-cli naming

goreleaser pushes per-arch `ghcr.io/fluxcd/flux-cli:<tag>-{amd64,arm64,arm}` plus manifest
lists `ghcr.io/fluxcd/flux-cli:<tag>` — same names at v0.20.1, v0.41.2, v2.0.0, v2.9.6 (200 at
v0.20.1 and v2.9.6). Docker Hub mirror `docker.io/fluxcd/flux-cli` exists (anonymous 401).
No other naming era for flux2 (flux v1's `fluxcd/flux` is a different product).

## 5. The community Helm chart (lookup, not template)

The chart is **not** in the flux2 repo: `github.com/fluxcd-community/helm-charts`, chart
`flux2`, index https://fluxcd-community.github.io/helm-charts (109 entries since 2021-11) and
OCI `ghcr.io/fluxcd-community/charts/flux2` (200). **Chart version ≠ app version**: the chart
has its own majors (chart 2.19.1 ↔ appVersion 2.9.5; chart 2.9.x ↔ appVersion 2.0.0); the
relation is *the chart whose `appVersion` equals the flux2 release* — Argo CD's `lookup`
shape, `select: latest` (several chart versions can share one appVersion). appVersion is
unprefixed. Not every release gets a chart: 50 of ~70 stable releases 0.20.1–2.9.6 have one;
minor `.0` releases often never do (2.7.0/2.8.0/2.9.0 have none), and 2.9.6 (today) not yet —
`optional: true` (argo-cd precedent).

## 6. Release notes (GitHub release body, hand-written + appended)

One release per tag. Structure (release-notes-template.md + verified bodies):
`## Highlights` (prose + "Overview of the new features:" bullets + upgrade-procedure pointer),
label paragraphs `Fixes:` / `Improvements:` (each bullet suffixed with the responsible
controller), `### Kubernetes compatibility` — **a per-release support table**, in every minor
body since v2.0.0, absent in patch bodies: `| Kubernetes version | Minimum required |` rows
`v1.34 | >= 1.34.1` … (v2.9.0: 1.34/1.35/1.36); `## Components changelog` (the pin list as
links); `## CLI changelog` (github-native PR bullets: dependabot/CI noise + real CLI PRs);
`**Full Changelog**:` link. v0.x bodies are plainer (`Components Changelog` capital-C, no
Kubernetes table). `labelParagraphs` + section classify rules apply.

## 7. Upgrade guidance and compatibility

- Website `github.com/fluxcd/website` `content/en/flux/installation/upgrade.md` (main only;
  not at flux2 tags): the canonical generic procedure (CLI, bootstrap/Git reconciliation,
  automation) — "any v2.x to any v2.x". Since v2.7 bodies also point at the "Upgrade Procedure
  for Flux v2.7+" GitHub discussion 5572 (HTML, not declaratively readable — noted).
- `content/en/flux/releases/_index.md`: support = last three minors; controllers compatible
  within one minor of each other; Kubernetes = "all versions supported upstream", tested on
  the latest minor in CI (per-release truth = the body's Kubernetes table, §6).
- No machine-readable compatibility matrix file exists (unlike karpenter); the release-body
  table is the only per-release k8s statement.

## 8. Security

GitHub advisories per repository: flux2 (5, includes ecosystem-wide controller advisories,
e.g. "Helm Controller DoS" published under flux2), source-controller (2), kustomize-controller
(1), helm-controller / notification / image-* / source-watcher (0). Ranges are mostly absent
(`vulnerable_version_range` null on the public endpoint), so the edge reports them as
"check manually" warnings. Controller-CVE→flux-release joins need the pin map (KPS gap shape).
