# Discovery report: cert-manager

- Repository: `github.com/cert-manager/cert-manager` at `v1.21.2` (commit `922a06aa49ee…`)
- Generated: 2026-10-01T05:51:55Z
- LLM: not used (deterministic resolver only)
- Tags: 299 tags: prefix "v", 161 stable, 100 prereleases (alpha.N×61, beta.N×39), 38 junk; latest stable v1.21.2; lineage minor
- Scanned `github.com/cert-manager/cert-manager@v1.21.2` (source profile): 1231 files listed, 155 read
- Scanned `github.com/cert-manager/website@master` (docs profile): 2568 files listed, 909 read
- Validation: skipped (no relationship checker configured)

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 10 | git-tags github.com/cert-manager/cert-manager; github-releases cert-manager/cert-manager |
| release-notes | found | 1 | repo-file github.com/cert-manager/website content/docs/releases/release-notes/release-notes-{{.Major}}.{{.Minor}}.md @master; github-releases cert-manager/cert-manager  @{{.Tag}} |
| changelog | not-found | 0 |  |
| helm-charts | found | 16 | cert-manager via oci:quay.io/jetstack/charts/cert-manager, helm-repo:https://charts.jetstack.io |
| registries | found | 9 | quay.io/jetstack |
| images | found | 32 | quay.io/jetstack/cert-manager-acmesolver:{{.Tag}}; quay.io/jetstack/cert-manager-cainjector:{{.Tag}}; quay.io/jetstack/cert-manager-controller:{{.Tag}}; quay.io/jetstack/cert-manager-startupapicheck:{{.Tag}}; quay.io/jet… |
| upgrade-docs | found | 2 | repo-file github.com/cert-manager/website content/docs/releases/upgrading/upgrading-{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md @master |
| compatibility | found | 3 | repo-file github.com/cert-manager/website content/docs/releases/README.md @master |
| security | found | 2 | github-advisories cert-manager/cert-manager |
| version-relations | found | 4 | cert-manager-chart: version = {{.Tag}}; cert-manager-crds: version = {{.Tag}}; cert-manager-manifest: version = {{.Tag}}; cert-manager-acmesolver: version = {{.Tag}}; cert-manager-cainjector: version = {{.Tag}}; cert-man… |

## Open questions for the reviewer

- Run the relationship checks (validation) before adopting this definition.
- Ambiguity (registry-roles): 9 registry hosts/namespaces are referenced for product images; which are release channels?

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 299 tags: prefix "v", 161 stable, 100 prereleases (alpha.N×61, beta.N×39), 38 junk; latest stable v1.21.2; lineage minor |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (299 tags: prefix "v", 161 stable, 100 prereleases (alpha.N×61, beta.N×39), 38 junk; latest stable v1.21.2; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-docs | include | notes.per-line-file-with-release-sections | heuristic | One notes file per release line (38 instances) with one section per release (e.g. "## `v1.21.2`"). |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:upgrade-guide | include | upgrade.versioned-doc | heuristic | One upgrade document per release line (35 instances, newest content/docs/releases/upgrading/upgrading-1.20-1.21.md); the file name names the previous and the new line, so {{.PrevMajor}}.{{.PrevMinor}} (the previous exist… |
| source:compatibility | include | compat.markdown-table | heuristic | Support matrix keyed by release line (28 rows; key column Release; Kubernetes columns Compatible Kubernetes versions,Supported Kubernetes / OpenShift Versions,Tested Kubernetes Versions). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:cert-manager-chart | include | helm.version-from-build | heuristic | The build sets the chart version from the release tag (chart.version = {{.Tag}}). Published via 2 channel(s); OCI locations first. |
| artifact:cert-manager-crds | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:cert-manager-manifest | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:cert-manager-acmesolver | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:cert-manager-cainjector | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:cert-manager-controller | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:cert-manager-startupapicheck | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |
| artifact:cert-manager-webhook | include | image.tag-from-chart-appversion | heuristic | The chart leaves the image tag empty so it defaults to the chart appVersion, which follows the release. |

86 candidates were found; 43 were excluded by resolver rules (see the full report).
