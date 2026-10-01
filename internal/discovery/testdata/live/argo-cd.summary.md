# Discovery report: argo-cd

- Repository: `github.com/argoproj/argo-cd` at `v3.5.3` (commit `c9c369efcc5b…`)
- Generated: 2026-10-01T05:52:02Z
- LLM: not used (deterministic resolver only)
- Tags: 596 tags: prefix "v", 458 stable, 133 prereleases (rcN×133), 5 junk; latest stable v3.5.3; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\d+))?)$` (junk sample: gitops-engine/v3.6.0-rc1, stable, v2.1.2-hf1, v2.2.8-1, v0.4.0-alpha1)
- Scanned `github.com/argoproj/argo-cd@v3.5.3` (source profile): 5430 files listed, 2896 read
- Scanned `github.com/argoproj/argo-site@master` (docs profile): 490 files listed, 1 read
- Validation: skipped (no relationship checker configured)

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 15 | git-tags github.com/argoproj/argo-cd; github-releases argoproj/argo-cd |
| release-notes | found | 1 | github-releases argoproj/argo-cd  @{{.Tag}} |
| changelog | candidates-only | 1 |  |
| helm-charts | found | 48 | argo-cd via helm-repo:https://argoproj.github.io/argo-helm, helm-git:github.com/argoproj/argo-helm |
| registries | found | 10 | quay.io/argoproj |
| images | found | 76 | quay.io/argoproj/argocd:{{.Tag}} |
| upgrade-docs | found | 1 | repo-file github.com/argoproj/argo-cd docs/operator-manual/upgrading/{{.PrevMajor}}.{{.PrevMinor}}-{{.Major}}.{{.Minor}}.md |
| compatibility | found | 1 | repo-file github.com/argoproj/argo-cd docs/operator-manual/tested-kubernetes-versions.md |
| security | found | 4 | github-advisories argoproj/argo-cd |
| version-relations | found | 1 | argo-cd-chart: lookup appVersion == {{.Tag}}; manifests-install: version = {{.Tag}}; manifests-ha-install: version = {{.Tag}}; manifests-core-install: version = {{.Tag}}; manifests-install-with-hydrator: version = {{.Tag… |

## Open questions for the reviewer

- CHANGELOG.md stops at 2.4.8; confirm it is abandoned (release notes come from other sources).
- Run the relationship checks (validation) before adopting this definition.
- Ambiguity (registry-roles): 9 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (chart-version): How do the versions of chart(s) argo-cd relate to the release version?

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 596 tags: prefix "v", 458 stable, 133 prereleases (rcN×133), 5 junk; latest stable v3.5.3; lineage minor; strict tagPattern excludes junk tags such as gitops-engine/v3.6.0-rc1, stable, v2.1.2-hf1, v2.2.8-1 |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (596 tags: prefix "v", 458 stable, 133 prereleases (rcN×133), 5 junk; latest stable v3.5.3; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes | include | notes.generated-release-body | heuristic | The release pipeline generates the hosted release body from commit/PR titles. |
| source:upgrade-guide | include | upgrade.versioned-doc | heuristic | One upgrade document per release line (29 instances, newest docs/operator-manual/upgrading/3.4-3.5.md); the file name names the previous and the new line, so {{.PrevMajor}}.{{.PrevMinor}} (the previous existing line) is … |
| source:compatibility | include | compat.markdown-table | heuristic | Support matrix keyed by release line (3 rows; key column Argo CD version; Kubernetes columns Kubernetes versions). |
| source:advisories | include | security.advisories-referenced | heuristic | The security policy states that advisories are published as GitHub Security Advisories. |
| artifact:argo-cd-chart | include | chart.external-lookup-appversion | heuristic | The chart lives in a separate repository with independent versions; the chart release for a product release is looked up by appVersion == release tag (GitHub Pages URL by the chart-releaser convention). |
| artifact:manifests-install | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:manifests-ha-install | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:manifests-core-install | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| artifact:manifests-install-with-hydrator | include | manifest.in-repo-install | heuristic | Static install manifest in the tagged tree referencing the product image with the release tag. |
| candidate:cand-874ad4718852 | annotate | asset.integrity-file | heuristic | Checksum/signature/provenance asset; recorded but not modelled as an artifact. |
| artifact:argocd-binary | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (7 reference(s)). |
| artifact:crds | include | crd.in-repo | heuristic | 3 CRDs of the product's API groups in manifests/crds at the release tag. |
| artifact:argocd | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |

213 candidates were found; 161 were excluded by resolver rules (see the full report).
