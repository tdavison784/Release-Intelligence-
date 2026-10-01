# Discovery report: istio

- Repository: `github.com/istio/istio` at `1.31.1` (commit `190ae921b848…`)
- Generated: 2026-10-01T05:51:57Z
- LLM: not used (deterministic resolver only)
- Tags: 465 tags: prefix "", 312 stable, 142 prereleases (alpha.N×29, beta.N×54, rc.N×59), 11 junk; latest stable 1.31.1; lineage minor
- Strict tag pattern: `^(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\.\d+|beta\.\d+|rc\.\d+))?)$` (junk sample: 1.0.0-snapshot.0, 1.0.0-snapshot.1, 1.0.0-snapshot.2, 1.1.0-snapshot.2, 1.1.0-snapshot.3, 1.1.0-snapshot.4)
- Scanned `github.com/istio/istio@1.31.1` (source profile): 6331 files listed, 3327 read
- Scanned `github.com/istio/istio.io@master` (docs profile): 61228 files listed, 714 read
- Validation: skipped (no relationship checker configured)

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 5 | git-tags github.com/istio/istio; github-releases istio/istio |
| release-notes | found | 7 | repo-file github.com/istio/istio.io content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Version}}/index.md @master; repo-file github.com/istio/istio.io content/en/news/releases/{{.Major}}.{{.Minor}}.x/announci… |
| changelog | not-found | 0 |  |
| helm-charts | found | 18 | base via oci:registry.istio.io/release/charts/base, oci:ghcr.io/istio/release/charts/base, helm-repo:https://istio-release.storage.googleapis.com/charts, helm-repo:https://blob.istio.io/istio-release/charts; gateway via … |
| registries | found | 12 | docker.io/istio; gcr.io/istio-release; registry.istio.io/release |
| images | found | 105 | docker.io/istio/agentgateway:{{.Tag}}; docker.io/istio/install-cni:{{.Tag}}; docker.io/istio/istioctl:{{.Tag}}; docker.io/istio/pilot:{{.Tag}}; docker.io/istio/proxyv2:{{.Tag}}; docker.io/istio/ztunnel:{{.Tag}} |
| upgrade-docs | found | 1 | repo-file github.com/istio/istio.io content/en/news/releases/{{.Major}}.{{.Minor}}.x/announcing-{{.Major}}.{{.Minor}}/upgrade-notes/index.md @master |
| compatibility | found | 1 | repo-file github.com/istio/istio.io data/compatibility/supportStatus.yml @master |
| security | found | 2 | github-advisories istio/istio |
| version-relations | found | 1 | base-chart: version = {{.Version}}; gateway-chart: version = {{.Version}}; cni-chart: version = {{.Version}}; istiod-chart: version = {{.Version}}; ztunnel-chart: version = {{.Version}}; ambient-chart: version = {{.Versi… |

## Open questions for the reviewer

- Security bulletins live in github.com/istio/istio.io:content/en/news/security (55 entries, newest istio-security-2026-006); no advisory adapter reads such pages, so they are not part of the definition.
- Run the relationship checks (validation) before adopting this definition.
- Ambiguity (registry-roles): 14 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (main-chart): 6 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) base, gateway, cni, istiod, ztunnel, ambient relate to the release version?

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 465 tags: prefix "", 312 stable, 142 prereleases (alpha.N×29, beta.N×54, rc.N×59), 11 junk; latest stable 1.31.1; lineage minor; strict tagPattern excludes junk tags such as 1.0.0-snapshot.0, 1.0.0-snapshot.1, 1.0.0-s… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (465 tags: prefix "", 312 stable, 142 prereleases (alpha.N×29, beta.N×54, rc.N×59), 11 junk; latest stable 1.31.1; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-docs | include | notes.per-release-file | heuristic | One notes document per release (253 instances, newest content/en/news/releases/1.30.x/announcing-1.30.5/index.md). |
| source:release-notes-line | include | notes.per-line-file | heuristic | One notes document per release line (31 instances, newest content/en/news/releases/1.31.x/announcing-1.31/change-notes/index.md), used for X.Y.0 releases. |
| source:release-note-files | include | notes.structured-dir | heuristic | Directory of 1834 structured note files (keys: releaseNotes). |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:upgrade-guide | include | upgrade.versioned-doc | heuristic | One upgrade document per release line (31 instances, newest content/en/news/releases/1.31.x/announcing-1.31/upgrade-notes/index.md). |
| source:compatibility | include | compat.yaml-records | heuristic | Support matrix keyed by release line (20 rows; key column version; Kubernetes columns k8sVersions,testedK8sVersions). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:base-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "1.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 4 channel(s); OCI locations first. |
| artifact:gateway-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "1.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 4 channel(s); OCI locations first. |
| artifact:cni-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "1.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 4 channel(s); OCI locations first. |
| artifact:istiod-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "1.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 4 channel(s); OCI locations first. |
| artifact:ztunnel-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "1.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 4 channel(s); OCI locations first. |
| artifact:ambient-chart | include | helm.placeholder-version-follows-release | heuristic | Chart.yaml carries placeholder version "1.0.0" replaced at release time; assumed to equal the release ({{.Version}}). Published via 3 channel(s); OCI locations first. |
| artifact:istio-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (2 reference(s)). |
| artifact:istioctl-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (2 reference(s)). |
| artifact:crds | include | crd.in-repo | heuristic | 15 CRDs of the product's API groups in manifests/charts/base/files at the release tag. |
| artifact:agentgateway | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 3 registries; ordered by evidence strength. |
| artifact:install-cni | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 3 registries; ordered by evidence strength. |
| artifact:istioctl | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 3 registries; ordered by evidence strength. |
| artifact:pilot | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 3 registries; ordered by evidence strength. |
| artifact:proxyv2 | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 3 registries; ordered by evidence strength. |
| artifact:ztunnel | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. Published to 3 registries; ordered by evidence strength. |

165 candidates were found; 118 were excluded by resolver rules (see the full report).
