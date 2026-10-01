# Published artifacts and representation provenance (G8)

The pipeline can read facts from **published artifacts** — the things users
actually install — instead of only from the source tree, and every such fact
records **which representation of the artifact produced it**. This document
explains the representations, the provenance convention and the definition
surface. Architecture context: `docs/ARCHITECTURE.md`.

## Why

The source tree at a release tag and the published artifact of that release
are not always equal. Publishers rewrite content at packaging time:

- Istio's chart values in the repository nest defaults under
  `_internal_defaults_do_not_set` and carry development image placeholders
  (`gcr.io/istio-testing:latest`) that release-builder rewrites to the real
  hub/tag when packaging (`products/istio.yaml` notes this; hence its
  `ignoreKeys`).
- kube-prometheus-stack ships its operator CRDs inside a packaged *subchart*
  (`charts/crds/crds/*.yaml`) that a naive "crds/ of the repo" reading misses.

Facts read from the wrong representation are not wrong, but they are about a
different artifact than the one users get. G8 makes the choice explicit and
auditable instead of silent.

## Representations

A **representation** is the published form a fact's bytes came from. The
vocabulary (closed set, `domain.Representation`):

| Value | Meaning | Read via |
|---|---|---|
| `source-tree` | a file in the source repository at a ref | `repo-file` / `repo-dir` contents and channels |
| `published-chart-tgz` | a packaged Helm chart tarball from a chart repository | `helm-repo` (index entry URL) or `chart-tgz` (direct URL) |
| `published-oci-chart` | a Helm chart published as an OCI artifact (chart content layer) | `oci` on a chart |
| `release-asset` | a file attached to a release (install manifests, chart tarballs, archives) | `http` / `chart-tgz` URLs under `/releases/download/` |
| `http-document` | a plain HTTP document that is none of the above | `http` |
| `registry-manifest` | an OCI manifest or config blob (digests, labels, config) | `oci` on a container image |

The representation is **derived from the locator kind** (and, for http URLs,
the URL shape), never guessed from content. It is orthogonal to
`Provenance.Method`: the method states *how* knowledge was derived
(`declared` / `computed` / `heuristic`), the representation states *which
bytes* it was derived from. A kubeVersion read from a packaged Chart.yaml is
`declared` knowledge from the `published-chart-tgz` representation.

## The provenance convention

Every fact, snapshot and evidence record produced from a content carries the
representation:

1. **Evidence**: `Evidence.representation` (see `schemas/release.schema.json`).
   The evidence id hashes the representation too, so records that differ only
   in representation stay distinct. `Evidence.ContentDigest` pins the *whole
   retrieved package* (the archive digest: the helm index digest for
   `published-chart-tgz`, the layer digest for `published-oci-chart`, the
   manifest digest for `registry-manifest`); `Evidence.Locator` names the
   member inside it (`acme/values.yaml`, `L5`, ...).
2. **Facts**: every content snapshot fact has the attribute
   `representation`. Compatibility constraints derived from chart metadata
   point at evidence that carries the representation.
3. **Divergence**: when a definition declares two representations of the same
   content (`compareWith`, below) and they differ, a
   `representation.divergence` fact records what diverged (counts plus the
   keys/fields/names), citing evidence from both sides. The status detail
   says `representations agree` or `representations differ (...)`. A
   definition's choice of primary representation is explicit and the
   divergence stays visible — never a silent preference.

## Reading published packages

`helm.ReadArchive` (internal/helm) unpacks a chart tarball: `Chart.yaml`,
`values.yaml`, CRD manifests from any `crds/` directory in the archive
(including packaged subcharts, e.g. `charts/crds/crds/*.yaml`), and raw
`templates/**`. It is pure (`io.Reader` in, members out), bounds member count
and size, and rejects path traversal, absolute paths, second top-level
directories and archives without a `Chart.yaml`.

Two adapters serve it through the `sources.ChartPackageReader` port:

- **`helm-repo`** (internal/helm `PackageAdapter`): looks the chart version
  up in the repository's index (the index you already parse), downloads the
  entry's tarball URL and **verifies the download against the index digest**
  (helm index digests are the archive's sha256; a mismatch is an error).
  Evidence: the index entry plus the archive.
- **`oci`** (internal/oci): resolves the manifest, requires the Helm config
  media type (`application/vnd.cncf.helm.config.v1+json`), downloads the
  layer with media type `application/vnd.cncf.helm.chart.content.v1.tar+gzip`
  as a digest-addressed blob, verifies the digest and unpacks it. Evidence:
  the manifest plus the layer.
- **`chart-tgz`** (internal/helm): a direct tarball URL — the channel kind
  for chart archives attached to GitHub releases. It is also probed like an
  http asset (HEAD with GET fallback). A URL containing
  `/releases/download/` is labelled `release-asset`; other URLs
  `published-chart-tgz`.

Container images: `sources.ImageManifestReader` (oci adapter) resolves the
manifest and config blob of an image, so an `image-refs` content of a
`container-image` artifact is backed by the registry itself — the reference
with its manifest digest and the config labels (`org.opencontainers.image.*`)
as fact attributes — representation `registry-manifest`.

All downloads go through `fetch.Client`: cached, offline-replayable, token
exchanges never persisted.

## Definition surface

```yaml
artifacts:
  - id: chart
    type: helm-chart
    version: {strategy: template, template: "{{.Version}}"}
    channels:
      - kind: helm-repo                      # or oci, or chart-tgz
        url: https://prometheus-community.github.io/helm-charts
        chart: kube-prometheus-stack
    contents:
      # no locator: the artifact's first readable channel is used — here the
      # packaged chart from the helm-repo index (representation
      # published-chart-tgz)
      - kind: helm-values
      - kind: chart-metadata
      - kind: crds
      # explicit package locator and a material-difference guard:
      - kind: helm-values
        locator: {kind: repo-file, repository: github.com/o/r, ref: "{{.Tag}}", path: charts/acme/values.yaml}
        compareWith: {kind: helm-repo, url: https://charts.example, chart: acme}
```

- A content **locator of kind `helm-repo`, `oci` or `chart-tgz`** reads the
  packaged chart and selects the member(s) per content kind (`values.yaml`,
  `Chart.yaml`, `crds/**`, or values+templates+CRDs for `image-refs`). A
  content **without a locator** falls back to the artifact's first channel
  that can serve contents — document channels as before, now also packaged
  chart channels.
- **`compareWith`** declares the alternate representation (validated like any
  locator; comparing a content with itself is a validation warning). Content
  transforms (`stripPrefix`, `ignoreKeys`) are applied to both sides, so only
  real divergence shows — istio's packaging wrapper and image-rewrite keys do
  not produce noise if the definition already excludes them.
- **`chart-tgz`** is a valid channel kind for `helm-chart` artifacts,
  addressing the gap kube-prometheus-stack and ingress-nginx recorded (".tgz
  release assets have no channel kind"; cilium attaches its chart tarball to
  releases the same way).

Products that can use this today: kube-prometheus-stack (helm-repo index and
the gh-pages raw mirror; subchart CRDs only appear in the package),
cilium/istio (source-tree values with `compareWith` against the published
package to retire the `ignoreKeys` caveat), ingress-nginx (controller chart
tarball release assets).

## What is deliberately not covered

- **No template rendering.** Helm templates are scanned for *static* image
  references only; `{{ }}`-templated values are not rendered (that would need
  a helm engine and cluster assumptions). The values.yaml defaults carry the
  authoritative image pins.
- **`.prov` provenance files** are not verified (they need upstream signing
  keys); the index/manifest digest verification above is the integrity check.
- **Version relations** (`version.from`) still read documents (repo-file /
  http); a `field` read from a package member would be a small extension of
  the same reader if a product needs it (none did yet).
- Release-asset **YAML manifest bundles** (install.yaml) were already
  readable as `http` contents; G8 adds their representation label
  (`release-asset`) rather than a new mechanism.
