# Discovery report: karpenter-provider-aws

- Repository: `github.com/aws/karpenter-provider-aws` at `v1.14.1` (commit `bde00654cd31…`)
- Generated: 2026-10-02T12:35:34Z
- LLM: not used (deterministic resolver only)
- Tags: 272 tags: prefix "v", 265 stable, 6 prereleases (rc.N×6), 1 junk; latest stable v1.14.1; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\.\d+))?)$` (junk sample: v1.7.3-hash-fix-backport)
- Scanned `github.com/aws/karpenter-provider-aws@v1.14.1` (source profile): 1071 files listed, 599 read
- Validation: done against v1.12.0, v1.12.2, v1.13.0, v1.13.1, v1.14.0, v1.14.1

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 6 | git-tags github.com/aws/karpenter-provider-aws; github-releases aws/karpenter-provider-aws |
| release-notes | found | 0 | github-releases aws/karpenter-provider-aws  @{{.Tag}} |
| changelog | not-found | 0 |  |
| helm-charts | found | 8 | karpenter-crd via oci:public.ecr.aws/karpenter/karpenter-crd; karpenter via oci:public.ecr.aws/karpenter/karpenter, oci:public.ecr.aws/karpenter, helm-repo:https://charts.karpenter.sh |
| registries | candidates-only | 3 |  |
| images | candidates-only | 3 |  |
| upgrade-docs | candidates-only | 4 |  |
| compatibility | not-found | 0 |  |
| security | found | 0 | github-advisories aws/karpenter-provider-aws |
| version-relations | found | 3 | karpenter-crd-chart: lookup appVersion == {{.Version}}; karpenter-chart: independent; crds: version = {{.Tag}} |

## Proposed elements

Every proposal carries a status from a closed vocabulary: `historically-validated` (the real relationship checker observed it for at least 3 historical releases), `discovered` (deterministic, grounded in the cited file, not yet checked), `inferred` (heuristic or AI answer, assumptions made), `unverified` (checks ran but the channel was unreachable or samples insufficient), `exception` (holds only with the recorded exceptions/availability).

| Element | Status | Rule | Evidence |
|---|---|---|---|
| artifact:crds | historically-validated | crd.in-repo | https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/pkg/apis/crds/karpenter.k8s.aws_ec2nodeclasses.yaml#L3 (+2) |
| source:release-notes-github | historically-validated | notes.hosted-release-body | https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/release.yaml#L14 |
| artifact:controller | unverified | image.tag-assumed-release | https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter/README.md#L53 |
| artifact:karpenter-chart | unverified | helm.independent-version | https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter/Chart.yaml#L2 (+2) |
| artifact:karpenter-crd-chart | unverified | helm.appversion-lookup | https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter-crd/Chart.yaml#L2 (+2) |
| source:advisories | discovered | security.github-hosted-default |  |
| source:github-releases | discovered | versions.github-releases-fallback | https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/release.yaml#L14 |
| source:tags | discovered | versions.git-tags | https://github.com/aws/karpenter-provider-aws/releases/tag/v1.14.1#refs/tags/v1.14.1 |
| source:upgrade-guide | unverified | upgrade.versioned-doc | https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.13/upgrading/compatibility.md# |
| versioning | discovered | tags.scheme | https://github.com/aws/karpenter-provider-aws/releases/tag/v1.14.1#refs/tags/v1.14.1 (+2) |

Statuses: 2 historically-validated, 4 discovered, 4 unverified.

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: karpenter
name: karpenter-provider-aws
homepage: https://github.com/aws/karpenter-provider-aws
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\.\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/aws/karpenter-provider-aws
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\.\d+))?)$
  - id: github-releases
    roles:
      - versions
    locator:
      kind: github-releases
      repository: aws/karpenter-provider-aws
    priority: 1
  - id: release-notes-github
    roles:
      - release-notes
    locator:
      kind: github-releases
      repository: aws/karpenter-provider-aws
      ref: '{{.Tag}}'
    fallbackGroup: release-notes
    validatedAgainst:
      - 1.12.0
      - 1.12.2
      - 1.13.0
      - 1.13.1
      - 1.14.0
      - 1.14.1
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: aws/karpenter-provider-aws
artifacts:
  - id: karpenter-crd-chart
    type: helm-chart
    name: karpenter-crd
    version:
      strategy: lookup
      field: appVersion
      match: '{{.Version}}'
      select: latest
    channels:
      - kind: oci
        repository: public.ecr.aws/karpenter/karpenter-crd
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/aws/karpenter-provider-aws
          path: charts/karpenter-crd/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/aws/karpenter-provider-aws
          path: charts/karpenter-crd/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 1.12.0 unverifiable (no version index answered: oci index public.ecr.aws/karpenter/karpenter-crd does not expose appVersion); 1.12.2 unverifiable (no version index answered: oci index public.ecr.aws/karpenter/karpenter-crd does not expose appVersion); 1.13.0 unverifiable (no version index answered: oci index public.ecr.aws/karpenter/karpenter-crd does not expose appVersion); 1.13.1 unverifiable (no version index answered: oci index public.ecr.aws/karpenter/karpenter-crd does not expose appVersion); …).'
  - id: karpenter-chart
    type: helm-chart
    name: karpenter
    version:
      strategy: independent
    channels:
      - kind: oci
        repository: public.ecr.aws/karpenter/karpenter
      - kind: oci
        repository: public.ecr.aws/karpenter
      - kind: helm-repo
        url: https://charts.karpenter.sh
        chart: karpenter
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/aws/karpenter-provider-aws
          path: charts/karpenter/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/aws/karpenter-provider-aws
          path: charts/karpenter/Chart.yaml
    notes: 'Unverified by discovery (unverifiable: 1.12.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.12.2 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.13.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.13.1 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); …).'
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/aws/karpenter-provider-aws
        path: pkg/apis/crds
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 1.12.0
      - 1.12.2
      - 1.13.0
      - 1.13.1
      - 1.14.0
      - 1.14.1
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-02"
  validatedReleases:
    - v1.12.0
    - v1.12.2
    - v1.13.0
    - v1.13.1
    - v1.14.0
    - v1.14.1
  notes: Proposed by automated discovery; relationship checks run against v1.12.0, v1.12.2, v1.13.0, v1.13.1, v1.14.0, v1.14.1.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 4 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (main-chart): 2 charts are published; which one installs the product?
- Ambiguity (chart-version): How do the versions of chart(s) karpenter relate to the release version?

## Validation matrix

| Element | Verdict | v1.12.0 | v1.12.2 | v1.13.0 | v1.13.1 | v1.14.0 | v1.14.1 |
|---|---|---|---|---|---|---|---|
| artifact:controller | failing |  |  |  |  |  |  |
| artifact:crds | validated |  |  |  |  |  |  |
| artifact:karpenter-chart | unverifiable |  |  |  |  |  |  |
| artifact:karpenter-crd-chart | unverifiable |  |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |  |
| content:karpenter-chart/chart-metadata | validated |  |  |  |  |  |  |
| content:karpenter-chart/helm-values | validated |  |  |  |  |  |  |
| content:karpenter-crd-chart/chart-metadata | validated |  |  |  |  |  |  |
| content:karpenter-crd-chart/helm-values | validated |  |  |  |  |  |  |
| source:release-notes-github | validated |  |  |  |  |  |  |
| source:upgrade-guide | failing |  |  |  |  |  |  |

## Dropped elements

- `source:upgrade-guide` (deterministic): failed validation: 1.12.0 fail (fetch https://raw.githubusercontent.com/aws/karpenter-provider-aws/v1.12.0/website/content/en/v1.12/upgrading/compatibility.md: not found (HTTP 404)); 1.12.2 not-applicable (not applicable to 1.12.2: release kinds minor,major (this is a patch release)); 1.13.0 fail (fetch https://raw.githubusercontent.com/aws/karpenter-provider-aws/v1.13.0/website/content/en/v1.13/upgrading/compatibility.md: not found (HTTP 404)); 1.13.1 not-applicable (not applicable to 1.13.1: release kinds minor,major (this is a patch release)); …
- `artifact:controller` (deterministic): failed validation: 1.12.0 fail (absent from public.ecr.aws/karpenter/controller:v1.12.0); 1.12.2 fail (absent from public.ecr.aws/karpenter/controller:v1.12.2); 1.13.0 fail (absent from public.ecr.aws/karpenter/controller:v1.13.0); 1.13.1 fail (absent from public.ecr.aws/karpenter/controller:v1.13.1); …

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 272 tags: prefix "v", 265 stable, 6 prereleases (rc.N×6), 1 junk; latest stable v1.14.1; lineage minor; strict tagPattern excludes junk tags such as v1.7.3-hash-fix-backport |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (272 tags: prefix "v", 265 stable, 6 prereleases (rc.N×6), 1 junk; latest stable v1.14.1; lineage minor). |
| source:github-releases | include | versions.github-releases-fallback | heuristic | Hosted GitHub releases are published (release tooling or release-asset downloads found); used as a fallback version list. |
| source:release-notes-github | include | notes.hosted-release-body | heuristic | Hosted GitHub releases exist (assets are downloaded from them); their body may carry notes. |
| source:upgrade-guide | include | upgrade.versioned-doc | heuristic | One upgrade document per release line (4 instances, newest website/content/en/v1.13/upgrading/compatibility.md). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:karpenter-crd-chart | include | helm.appversion-lookup | heuristic | Chart.yaml at the scanned release still packages the previous chart (appVersion "1.13.0", 3 release(s) behind): chart releases are cut after the product tag; the chart for a release is looked up by appVersion == {{.Versi… |
| artifact:karpenter-chart | include | helm.independent-version | heuristic | Chart has its own version "1.13.0" unrelated to the release. Published via 3 channel(s); OCI locations first. |
| artifact:crds | include | crd.in-repo | heuristic | 4 CRDs of the product's API groups in pkg/apis/crds at the release tag. |
| artifact:controller | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| source:upgrade-guide | drop | validate.failed | computed | Relationship check failed: 1.12.0 fail (fetch https://raw.githubusercontent.com/aws/karpenter-provider-aws/v1.12.0/website/content/en/v1.12/upgrading/compatibility.md: not found (HTTP 404)); 1.12.2 not-applicable (not ap… |
| artifact:karpenter-crd-chart | annotate | validate.unverifiable | computed | Kept unverified: 1.12.0 unverifiable (no version index answered: oci index public.ecr.aws/karpenter/karpenter-crd does not expose appVersion); 1.12.2 unverifiable (no version index answered: oci index public.ecr.aws/karp… |
| artifact:karpenter-chart | annotate | validate.unverifiable | computed | Kept unverified: 1.12.0 unverifiable (artifact version is independent of the release version (strategy independent); not resolvable); 1.12.2 unverifiable (artifact version is independent of the release version (strategy … |
| artifact:controller | drop | validate.failed | computed | Relationship check failed: 1.12.0 fail (absent from public.ecr.aws/karpenter/controller:v1.12.0); 1.12.2 fail (absent from public.ecr.aws/karpenter/controller:v1.12.2); 1.13.0 fail (absent from public.ecr.aws/karpenter/c… |

<details><summary>6 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| upgrade-guide `website/content/en/v{{.Major}}.{{.Minor}}/getting-started/migrating-from-cas/_index.md` | upgrade.lower-ranked-template | Another upgrade-guide path template ranks higher or this one does not cover the newest release lines. |
| upgrade-guide `website/content/en/v{{.Major}}.{{.Minor}}/upgrading/_index.md` | upgrade.lower-ranked-template | Another upgrade-guide path template ranks higher or this one does not cover the newest release lines. |
| upgrade-guide `website/content/en/v{{.Major}}.{{.Minor}}/upgrading/upgrade-guide.md` | upgrade.lower-ranked-template | Another upgrade-guide path template ranks higher or this one does not cover the newest release lines. |
| crd `pkg/apis/crds/autoscaling.x-k8s.io_capacitybuffers.yaml` | crd.not-dedicated | CRDs bundled in another manifest, test data, or of third-party API groups. |
| image `public.ecr.aws/karpenter/karpenter` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `public.ecr.aws/eks-distro/kubernetes/pause` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |

</details>

## Candidates


### build-tool (1)

- `ko` (medium; ko.config) — [aws/karpenter-provider-aws:.ko.yaml L1](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.ko.yaml#L1): `defaultBaseImage: public.ecr.aws/eks-distro-build-tooling/eks-distro-minimal-base`

### crd (5)

- `pkg/apis/crds/autoscaling.x-k8s.io_capacitybuffers.yaml` (high; crd.file) — [aws/karpenter-provider-aws:pkg/apis/crds/autoscaling.x-k8s.io_capacitybuffers.yaml L3](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/pkg/apis/crds/autoscaling.x-k8s.io_capacitybuffers.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/apis/crds/karpenter.k8s.aws_ec2nodeclasses.yaml` (high; crd.file) — [aws/karpenter-provider-aws:pkg/apis/crds/karpenter.k8s.aws_ec2nodeclasses.yaml L3](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/pkg/apis/crds/karpenter.k8s.aws_ec2nodeclasses.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/apis/crds/karpenter.sh_nodeclaims.yaml` (high; crd.file) — [aws/karpenter-provider-aws:pkg/apis/crds/karpenter.sh_nodeclaims.yaml L3](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/pkg/apis/crds/karpenter.sh_nodeclaims.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/apis/crds/karpenter.sh_nodeoverlays.yaml` (high; crd.file) — [aws/karpenter-provider-aws:pkg/apis/crds/karpenter.sh_nodeoverlays.yaml L3](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/pkg/apis/crds/karpenter.sh_nodeoverlays.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/apis/crds/karpenter.sh_nodepools.yaml` (high; crd.file) — [aws/karpenter-provider-aws:pkg/apis/crds/karpenter.sh_nodepools.yaml L3](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/pkg/apis/crds/karpenter.sh_nodepools.yaml#L3): `kind: CustomResourceDefinition`

### helm-chart (2)

- `karpenter-crd@charts/karpenter-crd` (high; helm.chart) — [aws/karpenter-provider-aws:charts/karpenter-crd/Chart.yaml L2](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter-crd/Chart.yaml#L2): `name: karpenter-crd`
- `karpenter@charts/karpenter` (high; helm.chart) — [aws/karpenter-provider-aws:charts/karpenter/Chart.yaml L2](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter/Chart.yaml#L2): `name: karpenter`

### helm-oci (3)

- `public.ecr.aws/karpenter` (medium; docs.oci-ref, script.oci-ref) — [aws/karpenter-provider-aws:charts/karpenter/README.md.gotmpl L16](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter/README.md.gotmpl#L16): `karpenter oci://public.ecr.aws/karpenter/{{ template "chart.name" . }} \`
- `public.ecr.aws/karpenter/karpenter` (medium; docs.oci-ref, make.oci-ref, script.oci-ref) — [aws/karpenter-provider-aws:Makefile L181](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/Makefile#L181): `helm upgrade --install karpenter oci://public.ecr.aws/karpenter/karpenter --version ${KARPENTER_VERSION} --nam…`
- `public.ecr.aws/karpenter/karpenter-crd` (medium; docs.oci-ref) — [aws/karpenter-provider-aws:website/content/en/docs/upgrading/upgrade-guide.md L72](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/docs/upgrading/upgrade-guide.md#L72): `helm upgrade --install karpenter-crd oci://public.ecr.aws/karpenter/karpenter-crd --version x.y.z --namespace …`

### helm-repo (3)

- `https://charts.karpenter.sh` (medium; script.helm-repo-add) — [aws/karpenter-provider-aws:hack/image_canary.sh L33](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/hack/image_canary.sh#L33): `helm repo add karpenter https://charts.karpenter.sh/`
- `https://grafana.github.io/helm-charts` (medium; script.helm-repo-add) — [aws/karpenter-provider-aws:website/content/en/docs/getting-started/getting-started-with-karpenter/scripts/step09-add-prometheus-grafana.sh L1](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/docs/getting-started/getting-started-with-karpenter/scripts/step09-add-prometheus-grafana.sh#L1): `helm repo add grafana-charts https://grafana.github.io/helm-charts`
- `https://prometheus-community.github.io/helm-charts` (medium; script.helm-repo-add) — [aws/karpenter-provider-aws:website/content/en/docs/getting-started/getting-started-with-karpenter/scripts/step09-add-prometheus-grafana.sh L2](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/docs/getting-started/getting-started-with-karpenter/scripts/step09-add-prometheus-grafana.sh#L2): `helm repo add prometheus-community https://prometheus-community.github.io/helm-charts`

### image (3)

- `public.ecr.aws/karpenter/controller` (high; docs.image-ref, helm.values-image, script.image-ref) — [aws/karpenter-provider-aws:charts/karpenter/README.md L53](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter/README.md#L53): `| controller.image.repository | string | '"public.ecr.aws/karpenter/controller"' | Repository path to the cont…`
- `public.ecr.aws/eks-distro/kubernetes/pause` (medium; docs.image-ref, manifest.image, script.image-ref) — [aws/karpenter-provider-aws:examples/workloads/arm64.yaml L20](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/examples/workloads/arm64.yaml#L20): `- image: public.ecr.aws/eks-distro/kubernetes/pause:3.2`
- `public.ecr.aws/karpenter/karpenter` (medium; docs.image-ref) — [aws/karpenter-provider-aws:charts/karpenter/README.md L30](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter/README.md#L30): `cosign verify public.ecr.aws/karpenter/karpenter:1.13.0 \`

### manifest (9)

- `charts/karpenter/crds/karpenter.k8s.aws_awsnodetemplates.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/docs/upgrading/upgrade-guide.md L586](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/docs/upgrading/upgrade-guide.md#L586): `kubectl replace -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.14.0/charts/karpenter/crds/…`
- `charts/karpenter/crds/karpenter.sh_provisioners.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/docs/upgrading/upgrade-guide.md L562](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/docs/upgrading/upgrade-guide.md#L562): `kubectl replace -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.16.2/charts/karpenter/crds/…`
- `pkg/apis/crds/karpenter.k8s.aws_awsnodetemplates.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/v1.0/upgrading/v1beta1-migration.md L79](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.0/upgrading/v1beta1-migration.md#L79): `kubectl apply -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.32.10/pkg/apis/crds/karpenter…`
- `pkg/apis/crds/karpenter.k8s.aws_ec2nodeclasses.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/v1.0/upgrading/v1beta1-migration.md L82](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.0/upgrading/v1beta1-migration.md#L82): `kubectl apply -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.32.10/pkg/apis/crds/karpenter…`
- `pkg/apis/crds/karpenter.sh_machines.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/v1.0/upgrading/v1beta1-migration.md L78](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.0/upgrading/v1beta1-migration.md#L78): `kubectl apply -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.32.10/pkg/apis/crds/karpenter…`
- `pkg/apis/crds/karpenter.sh_nodeclaims.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/v1.0/upgrading/v1beta1-migration.md L81](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.0/upgrading/v1beta1-migration.md#L81): `kubectl apply -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.32.10/pkg/apis/crds/karpenter…`
- `pkg/apis/crds/karpenter.sh_nodepools.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/v1.0/upgrading/v1beta1-migration.md L80](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.0/upgrading/v1beta1-migration.md#L80): `kubectl apply -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.32.10/pkg/apis/crds/karpenter…`
- `pkg/apis/crds/karpenter.sh_provisioners.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/v1.0/upgrading/v1beta1-migration.md L77](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.0/upgrading/v1beta1-migration.md#L77): `kubectl apply -f https://raw.githubusercontent.com/aws/karpenter-provider-aws/v0.32.10/pkg/apis/crds/karpenter…`
- `website/content/en/preview/getting-started/getting-started-with-karpenter/cloudformation.yaml` (medium; docs.raw-url) — [aws/karpenter-provider-aws:website/content/en/v1.0/upgrading/v1-migration.md L427](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.0/upgrading/v1-migration.md#L427): `curl -fsSL https://raw.githubusercontent.com/aws/karpenter-provider-aws/${VERSION_TAG}/website/content/en/prev…`

### registry (3)

- `public.ecr.aws` (medium; workflow.registry-login) — [aws/karpenter-provider-aws:.github/workflows/image-canary.yaml L23](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/image-canary.yaml#L23): `registry: public.ecr.aws`
- `021119463062.dkr.ecr.us-east-1.amazonaws.com/karpenter/snapshot` (low; script.registry-ref) — [aws/karpenter-provider-aws:hack/release/common.sh L8](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/hack/release/common.sh#L8): `SNAPSHOT_REPO_ECR="${SNAPSHOT_ECR}/karpenter/snapshot/"`
- `public.ecr.aws/karpenter` (low; script.registry-ref) — [aws/karpenter-provider-aws:hack/release/common.sh L5](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/hack/release/common.sh#L5): `RELEASE_REPO_ECR="public.ecr.aws/${ECR_GALLERY_NAME}/"`

### release-publisher (1)

- `marvinpinto/action-automatic-releases` (high; workflow.release-upload) — [aws/karpenter-provider-aws:.github/workflows/release.yaml L14](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/release.yaml#L14): `contents: write # marvinpinto/action-automatic-releases@v1.2.1`

### release-trigger (4)

- `v[0-9]+.[0-9]+.[0-9]+` (high; workflow.tag-trigger) — [aws/karpenter-provider-aws:.github/workflows/release.yaml L6](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/release.yaml#L6): `- 'v[0-9]+.[0-9]+.[0-9]+'`
- `v[0-9]+.[0-9]+.[0-9]+-alpha.[0-9]+` (high; workflow.tag-trigger) — [aws/karpenter-provider-aws:.github/workflows/release.yaml L8](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/release.yaml#L8): `- 'v[0-9]+.[0-9]+.[0-9]+-alpha.[0-9]+'`
- `v[0-9]+.[0-9]+.[0-9]+-beta.[0-9]+` (high; workflow.tag-trigger) — [aws/karpenter-provider-aws:.github/workflows/release.yaml L9](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/release.yaml#L9): `- 'v[0-9]+.[0-9]+.[0-9]+-beta.[0-9]+'`
- `v[0-9]+.[0-9]+.[0-9]+-rc.[0-9]+` (high; workflow.tag-trigger) — [aws/karpenter-provider-aws:.github/workflows/release.yaml L7](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/.github/workflows/release.yaml#L7): `- 'v[0-9]+.[0-9]+.[0-9]+-rc.[0-9]+'`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-(?:rc\.\d+))?)$` (high; tags.ls-remote) — [aws/karpenter-provider-aws/releases/tag/v1.14.1 refs/tags/v1.14.1](https://github.com/aws/karpenter-provider-aws/releases/tag/v1.14.1#refs/tags/v1.14.1): `bde00654cd316fddb3dccfba3f5ac07039865429 refs/tags/v1.14.1`

### upgrade-guide (4)

- `website/content/en/v{{.Major}}.{{.Minor}}/getting-started/migrating-from-cas/_index.md` (high; paths.versioned-upgrade-guide) — [aws/karpenter-provider-aws:website/content/en/v1.13/getting-started/migrating-from-cas/_index.md ](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.13/getting-started/migrating-from-cas/_index.md): `file website/content/en/v1.13/getting-started/migrating-from-cas/_index.md`
- `website/content/en/v{{.Major}}.{{.Minor}}/upgrading/_index.md` (high; paths.versioned-upgrade-guide) — [aws/karpenter-provider-aws:website/content/en/v1.13/upgrading/_index.md ](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.13/upgrading/_index.md): `file website/content/en/v1.13/upgrading/_index.md`
- `website/content/en/v{{.Major}}.{{.Minor}}/upgrading/compatibility.md` (high; paths.versioned-upgrade-guide) — [aws/karpenter-provider-aws:website/content/en/v1.13/upgrading/compatibility.md ](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.13/upgrading/compatibility.md): `file website/content/en/v1.13/upgrading/compatibility.md`
- `website/content/en/v{{.Major}}.{{.Minor}}/upgrading/upgrade-guide.md` (high; paths.versioned-upgrade-guide) — [aws/karpenter-provider-aws:website/content/en/v1.13/upgrading/upgrade-guide.md ](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/website/content/en/v1.13/upgrading/upgrade-guide.md): `file website/content/en/v1.13/upgrading/upgrade-guide.md`

### version-relation (3)

- `binary.version = {{.Tag}}` (high; make.ldflags-version) — [aws/karpenter-provider-aws:Makefile L4](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/Makefile#L4): `LDFLAGS ?= -ldflags=-X=sigs.k8s.io/karpenter/pkg/operator.Version=$(shell git describe --tags --always | cut -…`
- `chart.appVersion = {{.Version}}` (high; helm.chart-appversion-lags-release) — [aws/karpenter-provider-aws:charts/karpenter-crd/Chart.yaml L6](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter-crd/Chart.yaml#L6): `appVersion: 1.13.0`
- `chart.version = {{.Version}}` (high; helm.chart-version-lags-release) — [aws/karpenter-provider-aws:charts/karpenter-crd/Chart.yaml L5](https://github.com/aws/karpenter-provider-aws/blob/v1.14.1/charts/karpenter-crd/Chart.yaml#L5): `version: 1.13.0`
