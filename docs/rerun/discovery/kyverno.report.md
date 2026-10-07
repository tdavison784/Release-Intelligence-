# Discovery report: kyverno

- Repository: `github.com/kyverno/kyverno` at `v1.19.1` (commit `40ec788d48bb…`)
- Generated: 2026-10-02T12:36:24Z
- LLM: not used (deterministic resolver only)
- Tags: 733 tags: prefix "v", 125 stable, 181 prereleases (alpha.N×10, beta.N×8, rc.N×81, rcN×82), 427 junk; latest stable v1.19.1; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\.\d+|beta\.\d+|rc\.\d+|rc\d+))?)$` (junk sample: 1.3.0-rc10, 1.6-dev, 1.7-dev, 1.8-dev, 1.9-dev, helm-chart-v.2.2.1)
- Scanned `github.com/kyverno/kyverno@v1.19.1` (source profile): 9490 files listed, 7410 read
- Scanned `github.com/kyverno/website@main` (docs profile): 946 files listed, 110 read
- Validation: done against v1.17.0, v1.17.2, v1.18.0, v1.18.2, v1.19.0, v1.19.1

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 14 | git-tags github.com/kyverno/kyverno; github-releases kyverno/kyverno |
| release-notes | found | 1 | repo-file github.com/kyverno/website src/content/blog/announcing-kyverno-release-{{.Major}}.{{.Minor}}/index.md @main; github-releases kyverno/kyverno  @{{.Tag}} |
| changelog | candidates-only | 1 |  |
| helm-charts | candidates-only | 8 |  |
| registries | found | 2 | ghcr.io/kyverno |
| images | found | 108 | ghcr.io/kyverno/readiness-checker:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 3 | github-advisories kyverno/kyverno |
| version-relations | found | 1 | install-manifest: version = {{.Tag}}; crds: version = {{.Tag}}; readiness-checker: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:crds | historically-validated | crd.in-repo | https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_clusterresources.yaml#L3 (+2) |
| artifact:install-manifest | historically-validated | asset.hosted-release-download | https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/docs/docs/installation/installation.mdx#L144 |
| artifact:kyverno-policies-chart | historically-validated | helm.appversion-lookup | https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno-policies/Chart.yaml#L3 (+2) |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/release.yaml#L301 (+2) |
| source:release-notes-line | historically-validated | notes.per-line-file | https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/blog/announcing-kyverno-release-1.19/index.md# |
| artifact:kyverno-chart | unverified | helm.independent-version | https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno/Chart.yaml#L3 (+2) |
| artifact:kyverno-cli-with-arch-if-eq-amd64-x86-64-else-if-eq-386-i386 | unverified | asset.hosted-release-download | https://github.com/kyverno/kyverno/blob/v1.19.1/.goreleaser.yml#L47 |
| artifact:readiness-checker | exception | image.tag-assumed-release | https://github.com/kyverno/kyverno/blob/v1.19.1/Makefile#L30 |
| source:advisories | discovered | security.advisories-referenced | https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/docs/docs/guides/security.md#L23 (+2) |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/release.yaml#L301 (+2) |
| source:tags | discovered | versions.git-tags | https://github.com/kyverno/kyverno/releases/tag/v1.19.1#refs/tags/v1.19.1 |
| versioning | discovered | tags.scheme | https://github.com/kyverno/kyverno/releases/tag/v1.19.1#refs/tags/v1.19.1 (+2) |

Statuses: 5 historically-validated, 4 discovered, 2 unverified, 1 exception.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: kyverno
name: kyverno
homepage: https://github.com/kyverno/kyverno
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\.\d+|beta\.\d+|rc\.\d+|rc\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/kyverno/kyverno
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\.\d+|beta\.\d+|rc\.\d+|rc\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: kyverno/kyverno
    priority: 1
  - id: release-notes-line
    roles:
      - release-notes
    locator:
      kind: repo-file
      repository: github.com/kyverno/website
      ref: main
      path: src/content/blog/announcing-kyverno-release-{{.Major}}.{{.Minor}}/index.md
    releaseKinds:
      - minor
      - major
    fallbackGroup: release-notes
    validatedAgainst:
      - 1.17.0
      - 1.18.0
      - 1.19.0
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: kyverno/kyverno
      ref: '{{.Tag}}'
    priority: 1
    fallbackGroup: release-notes
    validatedAgainst:
      - 1.17.0
      - 1.17.2
      - 1.18.0
      - 1.18.2
      - 1.19.0
      - 1.19.1
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: kyverno/kyverno
    notes: 'Security policy: SECURITY.md → https://github.com/kyverno/community/blob/…/SECURITY.md. https://kyverno.io/docs/guides/security/'
artifacts:
  - id: install-manifest
    type: manifest
    name: install.yaml
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: http
        url: https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/install.yaml
    contents:
      - kind: image-refs
    validatedAgainst:
      - 1.17.0
      - 1.17.2
      - 1.18.0
      - 1.18.2
      - 1.19.0
      - 1.19.1
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/kyverno/kyverno
        path: cmd/cli/kubectl-kyverno/data/crds
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 1.17.0
      - 1.17.2
      - 1.18.0
      - 1.18.2
      - 1.19.0
      - 1.19.1
  - id: readiness-checker
    type: container-image
    name: readiness-checker
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: ghcr.io/kyverno/readiness-checker
    notes: Failed validation for one release (1.17.0 fail (absent from ghcr.io/kyverno/readiness-checker:v1.17.0)); possibly published late or elsewhere.
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - v1.17.0
    - v1.17.2
    - v1.18.0
    - v1.18.2
    - v1.19.0
    - v1.19.1
  notes: Proposed by automated discovery; relationship checks run against v1.17.0, v1.17.2, v1.18.0, v1.18.2, v1.19.0, v1.19.1.
```

## Open questions for the reviewer

- Release workflow triggers on tags "kyverno-chart-v*", which does not match the tag prefix "v".
- Release workflow triggers on tags "kyverno-policies-chart-v*", which does not match the tag prefix "v".
- Release workflow triggers on tags "kyverno-chart-*", which does not match the tag prefix "v".
- Release workflow triggers on tags "kyverno-policies-chart-*", which does not match the tag prefix "v".
- CHANGELOG.md stops at 1.13.0; confirm it is abandoned (release notes come from other sources).
- artifact:readiness-checker failed validation for one release and was kept: Holds for 5 releases but failed for one (1.17.0 fail (absent from ghcr.io/kyverno/readiness-checker:v1.17.0)); kept with a comment for review.
- Ambiguity (registry-roles): 3 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.
- Ambiguity (main-chart): 2 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) kyverno relate to the release version?

## Validation matrix

| Element | Verdict | v1.17.0 | v1.17.2 | v1.18.0 | v1.18.2 | v1.19.0 | v1.19.1 |
|---|---|---|---|---|---|---|---|
| artifact:crds | validated |  |  |  |  |  |  |
| artifact:install-manifest | validated |  |  |  |  |  |  |
| artifact:kyverno-chart | unverifiable |  |  |  |  |  |  |
| artifact:kyverno-cli-with-arch-if-eq-amd64-x86-64-else-if-eq-386-i386 | unverifiable |  |  |  |  |  |  |
| artifact:kyverno-policies-chart | validated |  |  |  |  |  |  |
| artifact:readiness-checker | failing |  |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |  |
| content:install-manifest/image-refs | validated |  |  |  |  |  |  |
| content:kyverno-chart/chart-metadata | validated |  |  |  |  |  |  |
| content:kyverno-chart/helm-values | validated |  |  |  |  |  |  |
| content:kyverno-policies-chart/chart-metadata | validated |  |  |  |  |  |  |
| content:kyverno-policies-chart/helm-values | validated |  |  |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |  |  |
| source:release-notes-line | validated |  |  |  |  |  |  |

## Dropped elements

- `artifact:kyverno-policies-chart` (deterministic): static validation: error: artifacts[0].channels[0]: template "ghcr.io/${{/kyverno-policies": template: :1: unexpected "/" in command
- `artifact:kyverno-chart` (deterministic): static validation: error: artifacts[0].channels[1]: template "ghcr.io/${{/kyverno": template: :1: unexpected "/" in command
- `artifact:kyverno-cli-with-arch-if-eq-amd64-x86-64-else-if-eq-386-i386` (deterministic): static validation: error: artifacts[1].channels[0]: template "https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/kyverno-cli_{{.Tag}}_linux_\n{{- with .Arch -}}\n{{- if eq . \"amd64\" -}}x86_64{{- else if eq . \"386\" -}}i386{{- else -}}{{- . -}}{{- end -}}\n{{- end -}}\n{{- with .Arm -}}\n{{- if eq . \"6\" -}}hf{{- else -}}v{{- . -}}{{- end -}}\n{{- end -}}.tar.gz": template: :2:9: executing "" at <.Arch>: can't evaluate field Arch in type catalog.RenderContext

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 733 tags: prefix "v", 125 stable, 181 prereleases (alpha.N×10, beta.N×8, rc.N×81, rcN×82), 427 junk; latest stable v1.19.1; lineage minor; strict tagPattern excludes junk tags such as 1.3.0-rc10, 1.6-dev, 1.7-dev, 1.… |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (733 tags: prefix "v", 125 stable, 181 prereleases (alpha.N×10, beta.N×8, rc.N×81, rcN×82), 427 junk; latest stable v1.19.1; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-line | include | notes.per-line-file | heuristic | One notes document per release line (8 instances, newest src/content/blog/announcing-kyverno-release-1.19/index.md), used for X.Y.0 releases. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:advisories | include | security.advisories-referenced | heuristic | The security policy states that advisories are published as GitHub Security Advisories. |
| artifact:kyverno-policies-chart | include | helm.appversion-lookup | heuristic | Chart.yaml sets appVersion "v1.19.1", which equals a release tag; the chart release for a product release is looked up by appVersion == {{.Tag}} (the chart's own version moves independently). Published via 2 channel(s); … |
| artifact:kyverno-chart | include | helm.independent-version | heuristic | Chart has its own version "3.9.1" unrelated to the release. Published via 3 channel(s); OCI locations first. |
| candidate:cand-a158dd63f398 | annotate | asset.integrity-file | heuristic | Checksum/signature/provenance asset; recorded but not modelled as an artifact. |
| artifact:install-manifest | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (1 reference(s)). |
| artifact:kyverno-cli-with-arch-if-eq-amd64-x86-64-else-if-eq-386-i386 | include | asset.hosted-release-download | heuristic | Downloaded from the hosted release of the tag (3 reference(s)). |
| artifact:crds | include | crd.in-repo | heuristic | 20 CRDs of the product's API groups in cmd/cli/kubectl-kyverno/data/crds at the release tag. |
| artifact:readiness-checker | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:kyverno-chart | annotate | validate.unverifiable | computed | Kept unverified: 1.17.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.17.2 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:kyverno-cli-with-arch-if-eq-amd64-x86-64-else-if-eq-386-i386 | annotate | validate.unverifiable | computed | Kept unverified: 1.17.0 unverifiable (not verifiable: http channel (http: error)); 1.17.2 unverifiable (not verifiable: http channel (http: error)); 1.18.0 unverifiable (not verifiable: http channel (http: error)); 1.18.… |
| artifact:readiness-checker | annotate | validate.single-failure-kept | computed | Holds for 5 releases but failed for one (1.17.0 fail (absent from ghcr.io/kyverno/readiness-checker:v1.17.0)); kept with a comment for review. |
| artifact:kyverno-policies-chart | drop | proposer.static-validation | computed | error: artifacts[0].channels[0]: template "ghcr.io/${{/kyverno-policies": template: :1: unexpected "/" in command |
| artifact:kyverno-chart | drop | proposer.static-validation | computed | error: artifacts[0].channels[1]: template "ghcr.io/${{/kyverno": template: :1: unexpected "/" in command |
| artifact:kyverno-cli-with-arch-if-eq-amd64-x86-64-else-if-eq-386-i386 | drop | proposer.static-validation | computed | error: artifacts[1].channels[0]: template "https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/kyverno-cli_{{.Tag}}_linux_\n{{- with .Arch -}}\n{{- if eq . \"amd64\" -}}x86_64{{- else if eq . \"386\" -}}i386{{-… |

<details><summary>178 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| changelog `CHANGELOG.md` | changelog.stale | Changelog is not maintained: newest entry 1.13.0 while the latest release is v1.19.1. |
| helm-chart `crds@charts/kyverno/charts/crds` | helm.unpublished | No publication channel (OCI push/reference or Helm repository) mentions this chart. |
| helm-chart `grafana@charts/kyverno/charts/grafana` | helm.unpublished | No publication channel (OCI push/reference or Helm repository) mentions this chart. |
| crd `cmd/cli/kubectl-kyverno/_testdata/apply/test-3/crd/crd.yml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `cmd/cli/kubectl-kyverno/_testdata/apply/test-4/crd/crds.yml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_tests.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_tests.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `config/crds/policyreport/wgpolicyk8s.io_clusterpolicyreports.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `config/crds/policyreport/wgpolicyk8s.io_policyreports.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `config/install-latest-testing.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-context-apicall-dpol/crds/computeclass-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-context-apicall-gpol/crds/computeclass-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-context-apicall-mpol/crds/computeclass-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-context-apicall-vpol/crds/computeclass-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-context-apicall/crds/computeclass-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-generating-policy/missing-metadata-uid/crds/myresource-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-gpol-custom-crd/crds/widget-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/cli/test-mutating-policy/mutate-custom-crd/crds/widget-crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/events/clusterpolicy/generate-events-upon-fail-generation/crd-as…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/events/clusterpolicy/generate-events-upon-fail-generation/crd.ya…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/generate/validation/clusterpolicy/permissions/same-kind/chainsaw…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/generate/validation/clusterpolicy/permissions/same-kind/chainsaw…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/generate/validation/clusterpolicy/target-namespace-scope/chainsa…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/generate/validation/policy/target-namespace-scope/chainsaw-step-…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/generating-policies/template/generate-ciliumnetworkpolicy/crd-as…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/generating-policies/template/generate-ciliumnetworkpolicy/crd.ya…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/mutate/refactor/k10-minimum-retention/crd-assert.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/mutate/refactor/k10-minimum-retention/crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/policy-validation/cluster-policy/crd-non-exist/crd-ready.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/policy-validation/cluster-policy/crd-non-exist/crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/reports/admission/label/crd-definition.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/cornercases/external-metrics-deprecated/k…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/cornercases/external-metrics-deprecated/k…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/cornercases/external-metrics/keda-ready.y…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/cornercases/external-metrics/keda.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel-deprecated/parameter-resourc…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/clusters…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/clusters…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/namespac…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/namespac…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/namespac…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/namespac…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/namespac…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/cel/parameter-resources/namespac…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk-deprecated/crd-1.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk-deprecated/crd-ready-1.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk-deprecated/crd-ready.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk-deprecated/crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk/crd-1.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk/crd-ready-1.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk/crd-ready.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/clusterpolicy/standard/gvk/crd.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/e2e/lowercase-kind-crd-deprecated/postgresqls-ready.yam…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/e2e/lowercase-kind-crd-deprecated/postgresqls.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/e2e/lowercase-kind-crd/postgresqls-ready.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/validate/e2e/lowercase-kind-crd/postgresqls.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-complex-key…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-complex-key…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-complex/crd…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-complex/crd…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-none/crd-re…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-none/crd.ya…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-simple/crd-…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| crd `test/conformance/chainsaw/verify-images/clusterpolicy/standard/imageExtractors-simple/crd.…` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| image `public.ecr.aws/aquasecurity/trivy-db` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `cgr.dev/chainguard/static` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/kyvernopre` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating). |
| image `ghcr.io/kyverno/kyverno` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: floating,pinned). |
| image `ghcr.io/kyverno/kyverno-cli` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `ghcr.io/kyverno/cleanup-controller` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `ghcr.io/kyverno/reports-controller` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `ghcr.io/kyverno/background-controller` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: ). |
| image `ghcr.io/kyverno/sbom` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/woot` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `reg.kyverno.io/kyverno/kyvernopre` | image.test-only | Only referenced from test, sample or documentation files. |
| image `reg.kyverno.io/kyverno/kyverno` | image.test-only | Only referenced from test, sample or documentation files. |
| image `reg.kyverno.io/kyverno/background-controller` | image.test-only | Only referenced from test, sample or documentation files. |
| image `reg.kyverno.io/kyverno/cleanup-controller` | image.test-only | Only referenced from test, sample or documentation files. |
| image `reg.kyverno.io/kyverno/reports-controller` | image.test-only | Only referenced from test, sample or documentation files. |
| image `registry.k8s.io/kwok/kwok` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/node` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/solr` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/test-verify-image` | image.test-only | Only referenced from test, sample or documentation files. |
| image `registry.k8s.io/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/liveness` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/busybox-selinux` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/redis` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/nickchase/rss-php-nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/dummy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/pause` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `myregistry.corp.com/busybox1` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.digitalocean.com/runlevl4/nginxasdfasdf` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.digitalocean.com/runlevl4/bbbbbbbbbb-ccccc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/test/test3.2` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/node-exporter` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/kubesphere/kube-rbac-proxy` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/bbbbbbbbbb-ccccc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/hashicorp/http-echo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/example/initializer` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/example/myapp` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `myregistry.io/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/httpd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/mysql` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/python` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/test-busybox` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/dlorenc/hello-ko` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghrc.io/kyverno/test-busybox` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `corp.img.io/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `img.corp.com/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/myregistry/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/ngnix` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/dummyimagename` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `images.my-company.example/app` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/test-webserver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/test-webserver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io8s.io/test-webserver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/jimbugwadia/demo-java-tomcat` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/busyboxasdfasdf` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `foo.io/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/myorg/whatever` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/not-kyverno/kyverno` | image.test-only | Only referenced from test, sample or documentation files. |
| image `public.ecr.aws/docker/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `public.ecr.aws/docker/library/python` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/czjunkfoo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `public.ecr.aws/docker/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `public.ecr.aws/docker/library/redis` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `public.ecr.aws/docker/library/alpine` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `some.registry/istio/proxyv2` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `public.ecr.aws/docker/library/ubuntu` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `corp.reg.com/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/arm64v8/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/test-images/cosign` | image.test-only | Only referenced from test, sample or documentation files. |
| image `public.ecr.aws/docker/library/cassandra` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/test-verify-images` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/my-private-registry/docker/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.corp.com/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/abc` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/bcd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/test-image` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.corp.com/infrastructure/vault-init` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/nirmata/github-signing-demo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/bash` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `bar.io/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kedacore/keda-metrics-apiserver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kedacore/keda` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/test/foo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/test/bar` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/nothingherenginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `staging.example.com/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `example.com/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `prod.example.com/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/test-nginx` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/datadog/agent` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/someimagename` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/sigstore/cosign/cosign` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/zulu` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/kyverno/test-verify-image-private` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/kyverno/test-verify-image-rollback` | image.test-only | Only referenced from test, sample or documentation files. |
| image `ghcr.io/vishal-chdhry/artifact-attestation-example` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/stefanprodan/podinfo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/ghost` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/nginxinc/nginx-unprivileged` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/google-samples/node-hello` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/nirmata/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/kyverno/signatures` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `ghcr.io/kyverno/manifests/kyverno` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |

</details>

## Candidates


### build-tool (3)

- `cosign` (high; workflow.cosign) — [kyverno/kyverno:.github/workflows/helm-release.yaml L136](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/helm-release.yaml#L136): `cosign sign --yes \`
- `goreleaser` (high; goreleaser.config, workflow.goreleaser) — [kyverno/kyverno:.github/workflows/release.yaml L301](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/release.yaml#L301): `uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3`
- `ko` (medium; ko.config) — [kyverno/kyverno:.ko.yaml L1](https://github.com/kyverno/kyverno/blob/v1.19.1/.ko.yaml#L1): `defaultBaseImage: ghcr.io/wolfi-dev/static:alpine`

### changelog (1)

- `CHANGELOG.md` (high; docs.changelog) — [kyverno/kyverno:CHANGELOG.md L1](https://github.com/kyverno/kyverno/blob/v1.19.1/CHANGELOG.md#L1): `## v1.13.0`

### crd (112)

- `cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_clusterresources.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_clusterresources.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_clusterresources.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_contexts.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_contexts.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_contexts.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_userinfoes.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_userinfoes.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_userinfoes.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_values.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_values.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/config/crds/cli.kyverno.io_values.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_clusterresources.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_clusterresources.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_clusterresources.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_contexts.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_contexts.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_contexts.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_userinfoes.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_userinfoes.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_userinfoes.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_values.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_values.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/cli.kyverno.io_values.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/kyverno.io_cleanuppolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/kyverno.io_cleanuppolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/kyverno.io_cleanuppolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/kyverno.io_clustercleanuppolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/kyverno.io_clustercleanuppolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/kyverno.io_clustercleanuppolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/kyverno.io_clusterpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/kyverno.io_clusterpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/kyverno.io_clusterpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/kyverno.io_policies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/kyverno.io_policies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/kyverno.io_policies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/kyverno.io_policyexceptions.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/kyverno.io_policyexceptions.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/kyverno.io_policyexceptions.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_deletingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_deletingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_deletingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_generatingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_generatingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_generatingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_imagevalidatingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_imagevalidatingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_imagevalidatingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_mutatingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_mutatingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_mutatingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespaceddeletingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespaceddeletingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespaceddeletingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedgeneratingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedgeneratingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedgeneratingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedimagevalidatingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedimagevalidatingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedimagevalidatingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedmutatingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedmutatingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedmutatingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedvalidatingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedvalidatingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_namespacedvalidatingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_policyexceptions.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_policyexceptions.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_policyexceptions.yaml#L3): `kind: CustomResourceDefinition`
- `cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_validatingpolicies.yaml` (high; crd.file) — [kyverno/kyverno:cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_validatingpolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/cmd/cli/kubectl-kyverno/data/crds/policies.kyverno.io_validatingpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `config/crds/kyverno/kyverno.io_cleanuppolicies.yaml` (high; crd.file) — [kyverno/kyverno:config/crds/kyverno/kyverno.io_cleanuppolicies.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/config/crds/kyverno/kyverno.io_cleanuppolicies.yaml#L3): `kind: CustomResourceDefinition`
- … 87 more

### docs-repo (1)

- `github.com/kyverno/website` (medium; docs.docs-repo-ref) — [kyverno/kyverno:.github/PULL_REQUEST_TEMPLATE.md L31](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/PULL_REQUEST_TEMPLATE.md#L31): `- [ ] I have sent the draft PR to add or update [the documentation](https://github.com/kyverno/website) and th…`

### helm-chart (4)

- `crds@charts/kyverno/charts/crds` (high; helm.chart) — [kyverno/kyverno:charts/kyverno/charts/crds/Chart.yaml L2](https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno/charts/crds/Chart.yaml#L2): `name: crds`
- `grafana@charts/kyverno/charts/grafana` (high; helm.chart) — [kyverno/kyverno:charts/kyverno/charts/grafana/Chart.yaml L2](https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno/charts/grafana/Chart.yaml#L2): `name: grafana`
- `kyverno-policies@charts/kyverno-policies` (high; helm.chart) — [kyverno/kyverno:charts/kyverno-policies/Chart.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno-policies/Chart.yaml#L3): `name: kyverno-policies`
- `kyverno@charts/kyverno` (high; helm.chart) — [kyverno/kyverno:charts/kyverno/Chart.yaml L3](https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno/Chart.yaml#L3): `name: kyverno`

### helm-oci (2)

- `ghcr.io/${{` (high; workflow.helm-push) — [kyverno/kyverno:.github/workflows/helm-release.yaml L134](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/helm-release.yaml#L134): `helm push .dist/${chart}-*.tgz oci://ghcr.io/${{ github.repository_owner }}/charts |& tee .digest`
- `ghcr.io/kyverno/charts/kyverno` (medium; docs.oci-ref) — [kyverno/website:src/content/blog/kyverno-1.8-released/index.md L133](https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/blog/kyverno-1.8-released/index.md#L133): `A couple things of which to be aware prior to upgrading. First, the Helm chart registry URL has changed to 'gh…`

### helm-repo (2)

- `https://kyverno.github.io/kyverno` (medium; docs.helm-repo-add) — [kyverno/kyverno:charts/kyverno-policies/README.md L56](https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno-policies/README.md#L56): `$ helm repo add kyverno https://kyverno.github.io/kyverno/`
- `https://prometheus-community.github.io/helm-charts` (medium; script.helm-repo-add) — [kyverno/kyverno:docs/perf-testing/provision/akamai-setup-cluster.sh L54](https://github.com/kyverno/kyverno/blob/v1.19.1/docs/perf-testing/provision/akamai-setup-cluster.sh#L54): `helm repo add prometheus-community https://prometheus-community.github.io/helm-charts`

### image (108)

- `ghcr.io/kyverno/readiness-checker` (high; helm.values-image, make.image-ref) — [kyverno/kyverno:Makefile L30](https://github.com/kyverno/kyverno/blob/v1.19.1/Makefile#L30): `REPO_READINESS       := $(REGISTRY)/$(REPO)/$(READINESS_IMAGE)`
- `public.ecr.aws/aquasecurity/trivy-db` (high; workflow.image-ref) — [kyverno/kyverno:.github/workflows/images-publish.yaml L59](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/images-publish.yaml#L59): `TRIVY_DB_REPOSITORY: 'public.ecr.aws/aquasecurity/trivy-db:2'`
- `registry.k8s.io/kwok/kwok` (high; kustomize.image) — [kyverno/kyverno:scripts/config/kwok/kustomization.yaml L5](https://github.com/kyverno/kyverno/blob/v1.19.1/scripts/config/kwok/kustomization.yaml#L5): `newTag: v0.2.0`
- `docker.io/library/busybox` (medium; manifest.image) — [kyverno/kyverno:charts/kyverno/ci/extraContainers-values.yaml L4](https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno/ci/extraContainers-values.yaml#L4): `image: busybox`
- `ghcr.io/kyverno/background-controller` (medium; make.image-ref) — [kyverno/kyverno:Makefile L29](https://github.com/kyverno/kyverno/blob/v1.19.1/Makefile#L29): `REPO_BACKGROUND      := $(REGISTRY)/$(REPO)/$(BACKGROUND_IMAGE)`
- `ghcr.io/kyverno/cleanup-controller` (medium; make.image-ref) — [kyverno/kyverno:Makefile L27](https://github.com/kyverno/kyverno/blob/v1.19.1/Makefile#L27): `REPO_CLEANUP         := $(REGISTRY)/$(REPO)/$(CLEANUP_IMAGE)`
- `ghcr.io/kyverno/kyverno` (medium; docs.image-ref, make.image-ref, manifest.image) — [kyverno/kyverno:DEVELOPMENT.md L238](https://github.com/kyverno/kyverno/blob/v1.19.1/DEVELOPMENT.md#L238): `The resulting image should be available remotely, named 'ghcr.io/kyverno/kyverno' (by default, if 'REGISTRY' e…`
- `ghcr.io/kyverno/kyverno-cli` (medium; docs.image-ref, make.image-ref) — [kyverno/kyverno:DEVELOPMENT.md L256](https://github.com/kyverno/kyverno/blob/v1.19.1/DEVELOPMENT.md#L256): `The resulting image should be available remotely, named 'ghcr.io/kyverno/kyverno-cli' (by default, if 'REGISTR…`
- `ghcr.io/kyverno/kyvernopre` (medium; docs.image-ref, make.image-ref, manifest.image) — [kyverno/kyverno:DEVELOPMENT.md L220](https://github.com/kyverno/kyverno/blob/v1.19.1/DEVELOPMENT.md#L220): `The resulting image should be available remotely, named 'ghcr.io/kyverno/kyvernopre' (by default, if 'REGISTRY…`
- `ghcr.io/kyverno/manifests/kyverno` (medium; docs.image-ref) — [kyverno/website:src/content/docs/docs/guides/security.md L100](https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/docs/docs/guides/security.md#L100): `COSIGN_EXPERIMENTAL=1 cosign verify ghcr.io/kyverno/manifests/kyverno:<release_tag> | jq`
- `ghcr.io/kyverno/reports-controller` (medium; make.image-ref) — [kyverno/kyverno:Makefile L28](https://github.com/kyverno/kyverno/blob/v1.19.1/Makefile#L28): `REPO_REPORTS         := $(REGISTRY)/$(REPO)/$(REPORTS_IMAGE)`
- `ghcr.io/kyverno/sbom` (medium; docs.image-ref) — [kyverno/kyverno:README.md L145](https://github.com/kyverno/kyverno/blob/v1.19.1/README.md#L145): `- 👉 ['ghcr.io/kyverno/sbom'](https://github.com/orgs/kyverno/packages?tab=packages&q=sbom)`
- `ghcr.io/kyverno/signatures` (medium; docs.image-ref) — [kyverno/website:src/content/docs/docs/guides/security.md L80](https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/docs/docs/guides/security.md#L80): `Kyverno container images and manifests are signed using Cosign and the [keyless signing feature](https://docs.…`
- `ghcr.io/kyverno/test-nginx` (medium; docs.image-ref, manifest.image) — [kyverno/kyverno:test/conformance/chainsaw/validate/clusterpolicy/standard/enforce/enforce-validate-existing-pss/bad-deploy-update-comply.yaml L26](https://github.com/kyverno/kyverno/blob/v1.19.1/test/conformance/chainsaw/validate/clusterpolicy/standard/enforce/enforce-validate-existing-pss/bad-deploy-update-comply.yaml#L26): `image: ghcr.io/kyverno/test-nginx:dontpull`
- `ghcr.io/kyverno/test-verify-image` (medium; docs.image-ref, manifest.image) — [kyverno/kyverno:test/cli/registry/resources.yaml L36](https://github.com/kyverno/kyverno/blob/v1.19.1/test/cli/registry/resources.yaml#L36): `image: ghcr.io/kyverno/test-verify-image:signed`
- `bar.io/busybox` (low; manifest.image) — [kyverno/kyverno:test/conformance/chainsaw/validate/clusterpolicy/cornercases/ephemeral-containers-deprecated/chainsaw-step-04-apply-1-1.yaml L8](https://github.com/kyverno/kyverno/blob/v1.19.1/test/conformance/chainsaw/validate/clusterpolicy/cornercases/ephemeral-containers-deprecated/chainsaw-step-04-apply-1-1.yaml#L8): `- image: bar.io/busybox:1.35`
- `cgr.dev/chainguard/static` (low; workflow.image-ref) — [kyverno/kyverno:.github/workflows/tests-conformance.yaml L143](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/tests-conformance.yaml#L143): `DIGEST=$(crane digest cgr.dev/chainguard/static)`
- `corp.img.io/busybox` (low; manifest.image) — [kyverno/kyverno:test/cli/test-validating-policy/restrict-image-registries/resource.yaml L25](https://github.com/kyverno/kyverno/blob/v1.19.1/test/cli/test-validating-policy/restrict-image-registries/resource.yaml#L25): `image: corp.img.io/busybox:1.35`
- `corp.reg.com/busybox` (low; manifest.image) — [kyverno/kyverno:test/conformance/chainsaw/generating-policies/matchconditions/cel-libs/triggers.yaml L9](https://github.com/kyverno/kyverno/blob/v1.19.1/test/conformance/chainsaw/generating-policies/matchconditions/cel-libs/triggers.yaml#L9): `image: corp.reg.com/busybox:1.35`
- `docker.io/arm64v8/busybox` (low; manifest.image) — [kyverno/kyverno:test/conformance/chainsaw/image-validating-policies/context/imagedata/image-data-arch/pod-bad.yaml L10](https://github.com/kyverno/kyverno/blob/v1.19.1/test/conformance/chainsaw/image-validating-policies/context/imagedata/image-data-arch/pod-bad.yaml#L10): `image: arm64v8/busybox@sha256:fa8dc70514f29471fe118446e3f84040b19791531ec197836a4f43baf13d744b`
- `docker.io/busybox` (low; manifest.image) — [kyverno/kyverno:test/cli/test-validating-policy/restrict-image-registries/resource.yaml L40](https://github.com/kyverno/kyverno/blob/v1.19.1/test/cli/test-validating-policy/restrict-image-registries/resource.yaml#L40): `image: docker.io/busybox:1.35`
- `docker.io/datadog/agent` (low; manifest.image) — [kyverno/kyverno:test/conformance/chainsaw/validate/clusterpolicy/standard/psa-deprecated/test-deletion-request/manifests.yaml L103](https://github.com/kyverno/kyverno/blob/v1.19.1/test/conformance/chainsaw/validate/clusterpolicy/standard/psa-deprecated/test-deletion-request/manifests.yaml#L103): `image: datadog/agent:7.36.0`
- `docker.io/example/initializer` (low; manifest.image) — [kyverno/kyverno:test/cli/test-mutating-admission-policy/with-apply-configuration/patched-resource.yaml L8](https://github.com/kyverno/kyverno/blob/v1.19.1/test/cli/test-mutating-admission-policy/with-apply-configuration/patched-resource.yaml#L8): `image: example/initializer:v1.0.0`
- `docker.io/example/myapp` (low; manifest.image) — [kyverno/kyverno:test/cli/test-mutating-admission-policy/with-apply-configuration/patched-resource.yaml L11](https://github.com/kyverno/kyverno/blob/v1.19.1/test/cli/test-mutating-admission-policy/with-apply-configuration/patched-resource.yaml#L11): `image: example/myapp:v1.0.0`
- `docker.io/hashicorp/http-echo` (low; manifest.image) — [kyverno/kyverno:test/cli/test-mutate/karpenter-annotations-to-nodeselector/patched.yaml L17](https://github.com/kyverno/kyverno/blob/v1.19.1/test/cli/test-mutate/karpenter-annotations-to-nodeselector/patched.yaml#L17): `image: hashicorp/http-echo:0.2.3`
- … 83 more

### manifest (1)

- `index.yaml` (low; docs.raw-url) — [kyverno/kyverno:docs/dev/releases/create-a-release.md L51](https://github.com/kyverno/kyverno/blob/v1.19.1/docs/dev/releases/create-a-release.md#L51): `- create a copy of [index.yaml](https://raw.githubusercontent.com/kyverno/kyverno/gh-pages/index.yaml), add it…`

### registry (2)

- `ghcr.io` (high; make.registry-variable, workflow.registry-login) — [kyverno/kyverno:.github/workflows/release.yaml L357](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/release.yaml#L357): `registry: ghcr.io`
- `ghcr.io/kyverno` (low; docs.registry-ref) — [kyverno/kyverno:SECURITY-INSIGHTS.yml L41](https://github.com/kyverno/kyverno/blob/v1.19.1/SECURITY-INSIGHTS.yml#L41): `- 'https://ghcr.io/kyverno'`

### release-asset (5)

- `https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/checksums.txt` (high; goreleaser.checksum) — [kyverno/kyverno:.goreleaser.yml L63](https://github.com/kyverno/kyverno/blob/v1.19.1/.goreleaser.yml#L63): `checksum:`
- `https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/kyverno-cli_{{.Tag}}_darwin_
{{- with .Arch -}}
{{- if eq …` (high; goreleaser.archive) — [kyverno/kyverno:.goreleaser.yml L47](https://github.com/kyverno/kyverno/blob/v1.19.1/.goreleaser.yml#L47): `name_template: |-`
- `https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/kyverno-cli_{{.Tag}}_linux_
{{- with .Arch -}}
{{- if eq .…` (high; goreleaser.archive) — [kyverno/kyverno:.goreleaser.yml L47](https://github.com/kyverno/kyverno/blob/v1.19.1/.goreleaser.yml#L47): `name_template: |-`
- `https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/kyverno-cli_{{.Tag}}_windows_
{{- with .Arch -}}
{{- if eq…` (high; goreleaser.archive) — [kyverno/kyverno:.goreleaser.yml L47](https://github.com/kyverno/kyverno/blob/v1.19.1/.goreleaser.yml#L47): `name_template: |-`
- `https://github.com/kyverno/kyverno/releases/download/{{.Tag}}/install.yaml` (medium; docs.release-asset-url) — [kyverno/website:src/content/docs/docs/installation/installation.mdx L144](https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/docs/docs/installation/installation.mdx#L144): `kubectl create -f https://github.com/kyverno/kyverno/releases/download/v1.16.2/install.yaml`

### release-notes (1)

- `src/content/blog/announcing-kyverno-release-{{.Major}}.{{.Minor}}/index.md` (high; paths.versioned-release-notes) — [kyverno/website:src/content/blog/announcing-kyverno-release-1.19/index.md ](https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/blog/announcing-kyverno-release-1.19/index.md): `file src/content/blog/announcing-kyverno-release-1.19/index.md`

### release-publisher (3)

- `goreleaser` (high; goreleaser.release, workflow.goreleaser) — [kyverno/kyverno:.github/workflows/release.yaml L301](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/release.yaml#L301): `uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3`
- `svenstaro/upload-release-action` (high; workflow.release-upload) — [kyverno/kyverno:.github/workflows/release.yaml L341](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/release.yaml#L341): `uses: svenstaro/upload-release-action@29e53e917877a24fad85510ded594ab3c9ca12de # 2.11.5`
- `github-releases` (medium; openssf.distribution-point) — [kyverno/kyverno:SECURITY-INSIGHTS.yml L14](https://github.com/kyverno/kyverno/blob/v1.19.1/SECURITY-INSIGHTS.yml#L14): `release-process: 'https://github.com/kyverno/kyverno/releases'`

### release-trigger (5)

- `kyverno-chart-*` (high; workflow.tag-trigger) — [kyverno/kyverno:.github/workflows/helm-release.yaml L16](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/helm-release.yaml#L16): `- kyverno-chart-*`
- `kyverno-chart-v*` (high; workflow.tag-trigger) — [kyverno/kyverno:.github/workflows/helm-release.yaml L14](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/helm-release.yaml#L14): `- kyverno-chart-v*`
- `kyverno-policies-chart-*` (high; workflow.tag-trigger) — [kyverno/kyverno:.github/workflows/helm-release.yaml L17](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/helm-release.yaml#L17): `- kyverno-policies-chart-*`
- `kyverno-policies-chart-v*` (high; workflow.tag-trigger) — [kyverno/kyverno:.github/workflows/helm-release.yaml L15](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/helm-release.yaml#L15): `- kyverno-policies-chart-v*`
- `v*` (high; workflow.tag-trigger) — [kyverno/kyverno:.github/workflows/assign-milestone.yaml L20](https://github.com/kyverno/kyverno/blob/v1.19.1/.github/workflows/assign-milestone.yaml#L20): `- v*`

### security-advisories (1)

- `kyverno/kyverno` (high; docs.security-advisories-link) — [kyverno/website:src/content/docs/docs/guides/security.md L23](https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/docs/docs/guides/security.md#L23): `## Security Advisories`

### security-policy (2)

- `SECURITY.md` (high; docs.security-policy) — [kyverno/kyverno:SECURITY.md L1](https://github.com/kyverno/kyverno/blob/v1.19.1/SECURITY.md#L1): `# Security Policy`
- `src/content/docs/docs/guides/security.md` (high; docs.security-policy) — [kyverno/website:src/content/docs/docs/guides/security.md L1](https://github.com/kyverno/website/blob/77f297fdd586483469c5a963d749383c22d6634f/src/content/docs/docs/guides/security.md#L1): `---`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\.\d+|beta\.\d+|rc\.\d+|rc\d+))?)$` (high; tags.ls-remote) — [kyverno/kyverno/releases/tag/v1.19.1 refs/tags/v1.19.1](https://github.com/kyverno/kyverno/releases/tag/v1.19.1#refs/tags/v1.19.1): `40ec788d48bb28d83dbf85538e962a59db9d45c6 refs/tags/v1.19.1`

### version-relation (1)

- `chart.appVersion = {{.Tag}}` (high; helm.chart-appversion-matches-ref) — [kyverno/kyverno:charts/kyverno-policies/Chart.yaml L5](https://github.com/kyverno/kyverno/blob/v1.19.1/charts/kyverno-policies/Chart.yaml#L5): `appVersion: v1.19.1`
