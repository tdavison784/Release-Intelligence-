# Discovery report: cilium

- Repository: `github.com/cilium/cilium` at `v1.20.2` (commit `e0dc92bd6dac…`)
- Generated: 2026-10-01T13:26:06Z
- LLM: not used (deterministic resolver only)
- Tags: 885 tags: prefix "v", 340 stable, 137 prereleases (pre.N×30, rc.N×16, rcN×91), 408 junk; latest stable v1.20.2; lineage minor
- Strict tag pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:pre\.\d+|rc\.\d+|rc\d+))?)$` (junk sample: 0.10.1, 1.0.4, 1.0.5, 1.0.6, 1.0.7, 1.1.0)
- Scanned `github.com/cilium/cilium@v1.20.2` (source profile): 7666 files listed, 2640 read
- Scanned `github.com/cilium/cilium.io@main` (docs profile): 3003 files listed, 69 read
- Scanned `github.com/cilium/docsearch-scraper-webhook@master` (docs profile): 107 files listed, 3 read
- Validation: done against v1.18.0, v1.18.14, v1.19.0, v1.19.8, v1.20.0, v1.20.2

## Coverage of the discovery targets

| Target | Status | Candidates | In definition |
|---|---|---|---|
| release-source | found | 3 | git-tags github.com/cilium/cilium |
| release-notes | not-found | 0 |  |
| changelog | found | 1 | repo-file github.com/cilium/cilium CHANGELOG.md |
| helm-charts | found | 10 | cilium via oci:quay.io/cilium/charts/cilium, helm-repo:https://helm.cilium.io |
| registries | found | 12 | quay.io/cilium |
| images | found | 86 | quay.io/cilium/cilium:{{.Tag}}; quay.io/cilium/clustermesh-apiserver:{{.Tag}}; quay.io/cilium/hubble-relay:{{.Tag}}; quay.io/cilium/operator:{{.Tag}} |
| upgrade-docs | not-found | 0 |  |
| compatibility | not-found | 0 |  |
| security | found | 1 | github-advisories cilium/cilium |
| version-relations | found | 2 | cilium-chart: version = {{.Version}}; crds: version = {{.Tag}}; cilium: version = {{.Tag}}; clustermesh-apiserver: version = {{.Tag}}; hubble-relay: version = {{.Tag}}; operator: version = {{.Tag}} |

## Proposed definition

```yaml
apiVersion: ri.dev/v1alpha1
kind: ProductDefinition
id: cilium
name: cilium
homepage: https://github.com/cilium/cilium
versioning:
  scheme: semver
  tagPrefix: v
  tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:pre\.\d+|rc\.\d+|rc\d+))?)$
  lineage: minor
sources:
  - id: tags
    roles:
      - versions
    locator:
      kind: git-tags
      repository: github.com/cilium/cilium
      tagPattern: ^v(?P<version>\d+\.\d+\.\d+(?:-(?:pre\.\d+|rc\.\d+|rc\d+))?)$
  - id: changelog
    roles:
      - changelog
    locator:
      kind: repo-file
      repository: github.com/cilium/cilium
      path: CHANGELOG.md
    extract:
      type: markdown-section
      heading: ^\[?v?{{regexQuote .Version}}\]?(\s|$)
    validatedAgainst:
      - 1.18.0
      - 1.18.14
      - 1.19.0
      - 1.19.8
      - 1.20.0
      - 1.20.2
  - id: advisories
    roles:
      - security
    locator:
      kind: github-advisories
      repository: cilium/cilium
    notes: 'Security policy: SECURITY.md → https://github.com/cilium/community/blob/…/roles/Security-Team.md https://docs.cilium.io/en/latest/security/threat-model/'
artifacts:
  - id: cilium-chart
    type: helm-chart
    name: cilium
    version:
      strategy: template
      template: '{{.Version}}'
    channels:
      - kind: oci
        repository: quay.io/cilium/charts/cilium
      - kind: helm-repo
        url: https://helm.cilium.io
        chart: cilium
    contents:
      - kind: helm-values
        locator:
          kind: repo-file
          repository: github.com/cilium/cilium
          path: install/kubernetes/cilium/values.yaml
      - kind: chart-metadata
        locator:
          kind: repo-file
          repository: github.com/cilium/cilium
          path: install/kubernetes/cilium/Chart.yaml
    validatedAgainst:
      - 1.18.0
      - 1.18.14
      - 1.19.0
      - 1.19.8
      - 1.20.0
      - 1.20.2
  - id: crds
    type: crd
    name: CustomResourceDefinitions
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: repo-dir
        repository: github.com/cilium/cilium
        path: pkg/k8s/apis/cilium.io/client/crds/v2
        glob: '*.yaml'
    contents:
      - kind: crds
    validatedAgainst:
      - 1.18.0
      - 1.18.14
      - 1.19.0
      - 1.19.8
      - 1.20.0
      - 1.20.2
  - id: cilium
    type: container-image
    name: cilium
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/cilium/cilium
    validatedAgainst:
      - 1.18.0
      - 1.18.14
      - 1.19.0
      - 1.19.8
      - 1.20.0
      - 1.20.2
  - id: clustermesh-apiserver
    type: container-image
    name: clustermesh-apiserver
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/cilium/clustermesh-apiserver
    validatedAgainst:
      - 1.18.0
      - 1.18.14
      - 1.19.0
      - 1.19.8
      - 1.20.0
      - 1.20.2
  - id: hubble-relay
    type: container-image
    name: hubble-relay
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/cilium/hubble-relay
    validatedAgainst:
      - 1.18.0
      - 1.18.14
      - 1.19.0
      - 1.19.8
      - 1.20.0
      - 1.20.2
  - id: operator
    type: container-image
    name: operator
    version:
      strategy: template
      template: '{{.Tag}}'
    channels:
      - kind: oci
        repository: quay.io/cilium/operator
    validatedAgainst:
      - 1.18.0
      - 1.18.14
      - 1.19.0
      - 1.19.8
      - 1.20.0
      - 1.20.2
provenance:
  method: discovery
  author: discovery.resolve@v1
  updated: "2026-10-01"
  validatedReleases:
    - v1.18.0
    - v1.18.14
    - v1.19.0
    - v1.19.8
    - v1.20.0
    - v1.20.2
  notes: Proposed by automated discovery; relationship checks run against v1.18.0, v1.18.14, v1.19.0, v1.19.8, v1.20.0, v1.20.2.
```

## Open questions for the reviewer

- Ambiguity (registry-roles): 11 registry hosts/namespaces are referenced for product images; which are release channels?
- Ambiguity (docs-sources): Documentation lives in another repository; no release-notes, upgrade-guide, compatibility source was derived from it.

## Validation matrix

| Element | Verdict | v1.18.0 | v1.18.14 | v1.19.0 | v1.19.8 | v1.20.0 | v1.20.2 |
|---|---|---|---|---|---|---|---|
| artifact:certgen | failing |  |  |  |  |  |  |
| artifact:cilium | validated |  |  |  |  |  |  |
| artifact:cilium-chart | validated |  |  |  |  |  |  |
| artifact:cilium-envoy | failing |  |  |  |  |  |  |
| artifact:clustermesh-apiserver | validated |  |  |  |  |  |  |
| artifact:crds | validated |  |  |  |  |  |  |
| artifact:hubble-relay | validated |  |  |  |  |  |  |
| artifact:hubble-ui | failing |  |  |  |  |  |  |
| artifact:hubble-ui-backend | failing |  |  |  |  |  |  |
| artifact:operator | validated |  |  |  |  |  |  |
| artifact:startup-script | failing |  |  |  |  |  |  |
| artifact:ztunnel | failing |  |  |  |  |  |  |
| content:cilium-chart/chart-metadata | validated |  |  |  |  |  |  |
| content:cilium-chart/helm-values | validated |  |  |  |  |  |  |
| content:crds/crds | validated |  |  |  |  |  |  |
| source:changelog | validated |  |  |  |  |  |  |

## Dropped elements

- `artifact:certgen` (deterministic): failed validation: 1.18.0 fail (absent from quay.io/cilium/certgen:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/certgen:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/certgen:v1.19.0); 1.19.8 fail (absent from quay.io/cilium/certgen:v1.19.8); …
- `artifact:cilium-envoy` (deterministic): failed validation: 1.18.0 fail (absent from quay.io/cilium/cilium-envoy:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/cilium-envoy:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/cilium-envoy:v1.19.0); 1.19.8 fail (absent from quay.io/cilium/cilium-envoy:v1.19.8); …
- `artifact:hubble-ui` (deterministic): failed validation: 1.18.0 fail (absent from quay.io/cilium/hubble-ui:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/hubble-ui:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/hubble-ui:v1.19.0); 1.19.8 fail (absent from quay.io/cilium/hubble-ui:v1.19.8); …
- `artifact:hubble-ui-backend` (deterministic): failed validation: 1.18.0 fail (absent from quay.io/cilium/hubble-ui-backend:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/hubble-ui-backend:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/hubble-ui-backend:v1.19.0); 1.19.8 fail (absent from quay.io/cilium/hubble-ui-backend:v1.19.8); …
- `artifact:startup-script` (deterministic): failed validation: 1.18.0 fail (absent from quay.io/cilium/startup-script:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/startup-script:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/startup-script:v1.19.0); 1.19.8 fail (absent from quay.io/cilium/startup-script:v1.19.8); …
- `artifact:ztunnel` (deterministic): failed validation: 1.18.0 fail (absent from quay.io/cilium/ztunnel:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/ztunnel:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/ztunnel:v1.19.0); 1.19.8 fail (absent from quay.io/cilium/ztunnel:v1.19.8); …

## Decisions

| Element | Action | Rule | Method | Rationale |
|---|---|---|---|---|
| versioning | include | tags.scheme | computed | 885 tags: prefix "v", 340 stable, 137 prereleases (pre.N×30, rc.N×16, rcN×91), 408 junk; latest stable v1.20.2; lineage minor; strict tagPattern excludes junk tags such as 0.10.1, 1.0.4, 1.0.5, 1.0.6 |
| source:tags | include | versions.git-tags | heuristic | Canonical versions come from the repository's git tags (885 tags: prefix "v", 340 stable, 137 prereleases (pre.N×30, rc.N×16, rcN×91), 408 junk; latest stable v1.20.2; lineage minor). |
| source:changelog | include | changelog.file | heuristic | Changelog with 5 version sections, newest 1.20.2 (heading "## v1.20.2"). |
| source:advisories | include | security.github-hosted-default | heuristic | The repository is hosted on GitHub, whose security advisories are the default advisory feed. |
| artifact:cilium-chart | include | helm.version-from-build | heuristic | The build sets the chart version from the release tag (chart.version = {{.Version}}). Published via 2 channel(s); OCI locations first. |
| artifact:crds | include | crd.in-repo | heuristic | 17 CRDs of the product's API groups in pkg/k8s/apis/cilium.io/client/crds/v2 at the release tag. |
| artifact:certgen | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:cilium | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:cilium-envoy | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:clustermesh-apiserver | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:hubble-relay | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:hubble-ui | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:hubble-ui-backend | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:operator | include | image.tag-from-release | heuristic | Image tags equal the release tag in the build/publish configuration. |
| artifact:startup-script | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:ztunnel | include | image.tag-assumed-release | heuristic | No explicit tag evidence; assuming images are tagged with the release tag. |
| artifact:certgen | drop | validate.failed | computed | Relationship check failed: 1.18.0 fail (absent from quay.io/cilium/certgen:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/certgen:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/certgen:v1.19.0); 1.19.8 fail (abse… |
| artifact:cilium-envoy | drop | validate.failed | computed | Relationship check failed: 1.18.0 fail (absent from quay.io/cilium/cilium-envoy:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/cilium-envoy:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/cilium-envoy:v1.19.0); 1.… |
| artifact:hubble-ui | drop | validate.failed | computed | Relationship check failed: 1.18.0 fail (absent from quay.io/cilium/hubble-ui:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/hubble-ui:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/hubble-ui:v1.19.0); 1.19.8 fail… |
| artifact:hubble-ui-backend | drop | validate.failed | computed | Relationship check failed: 1.18.0 fail (absent from quay.io/cilium/hubble-ui-backend:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/hubble-ui-backend:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/hubble-ui-backe… |
| artifact:startup-script | drop | validate.failed | computed | Relationship check failed: 1.18.0 fail (absent from quay.io/cilium/startup-script:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/startup-script:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/startup-script:v1.19.… |
| artifact:ztunnel | drop | validate.failed | computed | Relationship check failed: 1.18.0 fail (absent from quay.io/cilium/ztunnel:v1.18.0); 1.18.14 fail (absent from quay.io/cilium/ztunnel:v1.18.14); 1.19.0 fail (absent from quay.io/cilium/ztunnel:v1.19.0); 1.19.8 fail (abse… |

<details><summary>88 excluded candidates</summary>

| Candidate | Rule | Rationale |
|---|---|---|
| helm-chart `cilium-datapath-plugin@examples/datapath-plugin/deploy/helm` | helm.test-or-library | Chart lives in a test/sample directory or is a library chart. |
| helm-chart `prometheus@examples/kubernetes/addons/prometheus` | helm.test-or-library | Chart lives in a test/sample directory or is a library chart. |
| helm-chart `cnp-second-namespaces@test/k8s/manifests/cnp-second-namespaces` | helm.test-or-library | Chart lives in a test/sample directory or is a library chart. |
| chart-repo `github.com/cilium/charts` | chart-repo.no-product-chart | No chart path for this product is referenced in that repository. |
| manifest `.github/actions/cl2-modules/fqdn/dns-prober.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `.github/actions/cl2-modules/netpol/deployment.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `.github/actions/generic-external-targets/nginx-external.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `.github/actions/setup-aks-cluster/dummy-cni-daemonset.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `contrib/containerlab/auto-discovery/default-gateway/service/service.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `contrib/containerlab/pod-ip-pool/bgp.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `contrib/containerlab/service/service.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| manifest `contrib/k8s/clear-kubeproxy-iptables.yaml` | manifest.not-release-tagged | No product image tagged with the release in this manifest. |
| image `gcr.io/k8s-staging-perf-tests/dnsperfgo` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/pause` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/cilium-runtime` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `docker.io/library/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/image-tester` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/docs-builder` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `quay.io/cilium/helm-toolbox` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `quay.io/cilium/cilium-ci` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `quay.io/cilium/kindest-node` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/cilium-builder` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned,unresolved). |
| image `quay.io/cilium/cilium-cli` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/cilium-bpftool` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/image-compilers` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/cilium-llvm` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/image-maker` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: snapshot). |
| image `quay.io/cilium/cilium-envoy-builder` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/clustermesh-apiserver-ci` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `quay.io/cilium/hubble-relay-ci` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `quay.io/cilium/operator-generic` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `mcr.microsoft.com/oss/cilium/cilium` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `mcr.microsoft.com/oss/cilium/operator` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `quay.io/cilium/operator-generic-ci` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `quay.io/cilium/standalone-dns-proxy` | image.docs-only | Only mentioned in documentation; no build or publish configuration produces it. |
| image `docker.io/golangci/golangci-lint` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/cilium-dev` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `docker.io/renovate/renovate` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/etcd-development/etcd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/goswagger/swagger` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/mikefarah/yq` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/cilium-checkpatch` | image.no-release-evidence | No reference with the release tag and no release publication config (tag classes: pinned). |
| image `docker.io/frrouting/frr` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/nicolaka/netshoot` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/curlimages/curl` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/lvh-images/kind` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/cilium-datapath-plugin-example` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/cilium/hubble` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/cilium/json-mock` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/pires/docker-elasticsearch-kubernetes` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/cilium/esclient` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/cilium/demo-httpd` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/cilium/cc-grpc-demo` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/mccutchen/go-httpbin` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/dns/k8s-dns-node-cache` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/coreos/etcd` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/grafana/grafana` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/prom/prometheus` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/nginx` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/cuelang/cue` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/alpine-curl` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/cilium/demo-client` | image.test-only | Only referenced from test, sample or documentation files. |
| image `registry.k8s.io/gateway-api/echo-basic` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/library/external-authz-test` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/k8s-staging-gateway-api/echo-basic` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/projectcontour/yages` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/istio-testing/app` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/istio/examples-helloworld-v1` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/istio/examples-helloworld-v2` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/fortio/fortio` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/coredns/coredns` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `gcr.io/k8s-staging-ingressconformance/echoserver` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/starwars` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/casey_callendrello/cni-plugins` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/cilium-envoy-dev` | image.dev-registry | Registry path denotes a development/testing repository. |
| image `ghcr.io/spiffe/spire-server` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `ghcr.io/spiffe/spire-agent` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `registry.k8s.io/coredns/coredns` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `docker.io/cilium/netperf` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/kindest/node` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `quay.io/cilium/echoserver-udp` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/cilium/demo-client` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/cilium/echoserver` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/cilium/log-gatherer` | image.test-only | Only referenced from test, sample or documentation files. |
| image `quay.io/cilium/netperf` | image.test-only | Only referenced from test, sample or documentation files. |
| image `docker.io/mfenwick100/cyclonus` | image.third-party | Image is not named after the product (dependency or tooling image). |
| image `cgr.dev/chainguard/busybox` | image.third-party | Image is not named after the product (dependency or tooling image). |

</details>

## Candidates


### changelog (1)

- `CHANGELOG.md` (high; docs.changelog) — [cilium/cilium:CHANGELOG.md L3](https://github.com/cilium/cilium/blob/v1.20.2/CHANGELOG.md#L3): `## v1.20.2`

### chart-repo (1)

- `github.com/cilium/charts` (medium; docs.chart-repo-ref) — [cilium/cilium:Documentation/installation/k8s-install-talos-linux.rst L29](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/installation/k8s-install-talos-linux.rst#L29): `.. _'Cilium Helm chart': https://github.com/cilium/charts`

### crd (22)

- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpadvertisements.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpadvertisements.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpadvertisements.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpclusterconfigs.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpclusterconfigs.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpclusterconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpnodeconfigoverrides.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpnodeconfigoverrides.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpnodeconfigoverrides.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpnodeconfigs.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpnodeconfigs.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgpnodeconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgppeerconfigs.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgppeerconfigs.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumbgppeerconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumcidrgroups.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumcidrgroups.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumcidrgroups.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwideenvoyconfigs.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwideenvoyconfigs.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwideenvoyconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwidenetworkpolicies.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwidenetworkpolicies.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumclusterwidenetworkpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumegressgatewaypolicies.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumegressgatewaypolicies.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumegressgatewaypolicies.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumendpoints.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumendpoints.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumendpoints.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumenvoyconfigs.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumenvoyconfigs.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumenvoyconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumidentities.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumidentities.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumidentities.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumloadbalancerippools.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumloadbalancerippools.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumloadbalancerippools.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumlocalredirectpolicies.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumlocalredirectpolicies.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumlocalredirectpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnetworkpolicies.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnetworkpolicies.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnetworkpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnodeconfigs.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnodeconfigs.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnodeconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnodes.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnodes.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2/ciliumnodes.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumdatapathplugins.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumdatapathplugins.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumdatapathplugins.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumendpointslices.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumendpointslices.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumendpointslices.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumgatewayclassconfigs.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumgatewayclassconfigs.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumgatewayclassconfigs.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliuml2announcementpolicies.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliuml2announcementpolicies.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliuml2announcementpolicies.yaml#L3): `kind: CustomResourceDefinition`
- `pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumpodippools.yaml` (high; crd.file) — [cilium/cilium:pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumpodippools.yaml L3](https://github.com/cilium/cilium/blob/v1.20.2/pkg/k8s/apis/cilium.io/client/crds/v2alpha1/ciliumpodippools.yaml#L3): `kind: CustomResourceDefinition`

### docs-repo (3)

- `github.com/cilium/cilium.io` (medium; docs.docs-repo-ref) — [cilium/cilium:CONTRIBUTING.md L11](https://github.com/cilium/cilium/blob/v1.20.2/CONTRIBUTING.md#L11): `Please see the [cilium.io website contributing guide](https://github.com/cilium/cilium.io/blob/main/CONTRIBUTI…`
- `github.com/cilium/docsearch-scraper-webhook` (medium; docs.docs-repo-ref) — [cilium/cilium:Documentation/contributing/docs/docsframework.rst L324](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/contributing/docs/docsframework.rst#L324): `.. _DocSearch scraper: https://github.com/cilium/docsearch-scraper-webhook`
- `github.com/cilium/docs-builder` (low; docs.docs-repo-ref) — [cilium/cilium:Documentation/contributing/docs/docstest.rst L13](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/contributing/docs/docstest.rst#L13): `real-time preview. It relies on the ''cilium/docs-builder'' Docker container.`

### helm-chart (4)

- `cilium@install/kubernetes/cilium` (high; helm.chart) — [cilium/cilium:install/kubernetes/cilium/Chart.yaml L2](https://github.com/cilium/cilium/blob/v1.20.2/install/kubernetes/cilium/Chart.yaml#L2): `name: cilium`
- `cilium-datapath-plugin@examples/datapath-plugin/deploy/helm` (low; helm.chart) — [cilium/cilium:examples/datapath-plugin/deploy/helm/Chart.yaml L2](https://github.com/cilium/cilium/blob/v1.20.2/examples/datapath-plugin/deploy/helm/Chart.yaml#L2): `name: cilium-datapath-plugin`
- `cnp-second-namespaces@test/k8s/manifests/cnp-second-namespaces` (low; helm.chart) — [cilium/cilium:test/k8s/manifests/cnp-second-namespaces/Chart.yaml L4](https://github.com/cilium/cilium/blob/v1.20.2/test/k8s/manifests/cnp-second-namespaces/Chart.yaml#L4): `name: cnp-second-namespaces`
- `prometheus@examples/kubernetes/addons/prometheus` (low; helm.chart) — [cilium/cilium:examples/kubernetes/addons/prometheus/Chart.yaml L2](https://github.com/cilium/cilium/blob/v1.20.2/examples/kubernetes/addons/prometheus/Chart.yaml#L2): `name: prometheus`

### helm-oci (2)

- `quay.io/cilium/charts/cilium` (medium; docs.oci-ref) — [cilium/cilium:Documentation/installation/k8s-install-helm.rst L35](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/installation/k8s-install-helm.rst#L35): `''oci://quay.io/cilium/charts/cilium''.`
- `quay.io/jetstack/charts/cert-manager` (low; workflow.oci-ref) — [cilium/cilium:.github/workflows/conformance-clustermesh.yaml L657](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/conformance-clustermesh.yaml#L657): `cert-manager oci://quay.io/jetstack/charts/cert-manager \`

### helm-repo (3)

- `https://charts.jetstack.io` (medium; docs.helm-repo-add) — [cilium/cilium:Documentation/network/servicemesh/gateway-api/grpc.rst L55](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/network/servicemesh/gateway-api/grpc.rst#L55): `$ helm repo add jetstack https://charts.jetstack.io`
- `https://grafana.github.io/helm-charts` (medium; docs.helm-repo-add) — [cilium/cilium:.github/actions/cl2-modules/l7/README.md L122](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/cl2-modules/l7/README.md#L122): `helm repo add grafana https://grafana.github.io/helm-charts`
- `https://helm.cilium.io` (medium; docs.helm-repo-add, script.helm-repo-add) — [cilium/cilium:Documentation/installation/k8s-install-download-release.rst L15](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/installation/k8s-install-download-release.rst#L15): `helm repo add cilium https://helm.cilium.io/`

### image (86)

- `docker.io/library/busybox` (high; helm.values-image, make.image-ref, manifest.image) — [cilium/cilium:.github/actions/setup-aks-cluster/dummy-cni-daemonset.yaml L31](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/setup-aks-cluster/dummy-cni-daemonset.yaml#L31): `image: docker.io/library/busybox:1.37.0`
- `ghcr.io/spiffe/spire-agent` (high; helm.values-image, make.image-ref) — [cilium/cilium:install/kubernetes/Makefile.values L69](https://github.com/cilium/cilium/blob/v1.20.2/install/kubernetes/Makefile.values#L69): `export SPIRE_AGENT_REPO?=ghcr.io/spiffe/spire-agent`
- `ghcr.io/spiffe/spire-server` (high; helm.values-image, make.image-ref) — [cilium/cilium:install/kubernetes/Makefile.values L64](https://github.com/cilium/cilium/blob/v1.20.2/install/kubernetes/Makefile.values#L64): `export SPIRE_SERVER_REPO?=ghcr.io/spiffe/spire-server`
- `quay.io/cilium/certgen` (high; docs.image-ref, helm.values-image, make.image-ref) — [cilium/cilium:CHANGELOG.md L262](https://github.com/cilium/cilium/blob/v1.20.2/CHANGELOG.md#L262): `* Update quay.io/cilium/certgen Docker tag to v0.4.9 (v1.20) (cilium/cilium#47657, @cilium-renovate[bot])`
- `quay.io/cilium/cilium` (high; docs.image-ref, helm.values-image, make.image-ref) — [cilium/cilium:Documentation/configuration/verify-image-signatures.rst L35](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/configuration/verify-image-signatures.rst#L35): `"quay.io/cilium/cilium:${TAG}" | jq`
- `quay.io/cilium/cilium-envoy` (high; docs.image-ref, helm.values-image, make.image-ref, script.image-ref) — [cilium/cilium:CHANGELOG.md L107](https://github.com/cilium/cilium/blob/v1.20.2/CHANGELOG.md#L107): `* chore(deps): update quay.io/cilium/cilium-envoy docker tag to v1.37.6-1787987562-ac0b61a4c0a45670a3654448d53…`
- `quay.io/cilium/clustermesh-apiserver` (high; docs.image-ref, helm.values-image, make.image-ref) — [cilium/cilium:Documentation/helm-values.rst L670](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/helm-values.rst#L670): `- ''{"digest":"","override":null,"pullPolicy":"IfNotPresent","repository":"quay.io/cilium/clustermesh-apiserve…`
- `quay.io/cilium/hubble-relay` (high; docs.image-ref, helm.values-image, make.image-ref, script.image-ref) — [cilium/cilium:Documentation/helm-values.rst L2310](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/helm-values.rst#L2310): `- ''{"digest":"","override":null,"pullPolicy":"IfNotPresent","repository":"quay.io/cilium/hubble-relay","tag":…`
- `quay.io/cilium/hubble-ui` (high; docs.image-ref, helm.values-image, make.image-ref) — [cilium/cilium:Documentation/helm-values.rst L2646](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/helm-values.rst#L2646): `- ''{"digest":"sha256:049a80a03585c043d0c3121bdc518c72b0fd42bae07e4bcce0fdb8f1e88041d0","override":null,"pullP…`
- `quay.io/cilium/hubble-ui-backend` (high; docs.image-ref, helm.values-image, make.image-ref) — [cilium/cilium:Documentation/helm-values.rst L2614](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/helm-values.rst#L2614): `- ''{"digest":"sha256:83b3fc763f3d49948c306bf49b08277383e195904b9ca5c0c0022b2112628787","override":null,"pullP…`
- `quay.io/cilium/operator` (high; docs.image-ref, helm.values-image, make.image-ref, script.image-ref) — [cilium/cilium:Documentation/contributing/development/introducing_new_crds.rst L372](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/contributing/development/introducing_new_crds.rst#L372): `--set operator.image.repository=quay.io/cilium/operator \`
- `quay.io/cilium/startup-script` (high; docs.image-ref, helm.values-image, make.image-ref) — [cilium/cilium:Documentation/contributing/development/images.rst L96](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/contributing/development/images.rst#L96): `|                               | images/startup-script/Dockerfile            | quay.io/cilium/startup-script …`
- `quay.io/cilium/ztunnel` (high; docs.image-ref, helm.values-image, make.image-ref) — [cilium/cilium:CHANGELOG.md L1466](https://github.com/cilium/cilium/blob/v1.20.2/CHANGELOG.md#L1466): `* Switch default ztunnel image from 'docker.io/istio/ztunnel' to 'quay.io/cilium/ztunnel:v1.0.0' (cilium/ciliu…`
- `docker.io/curlimages/curl` (medium; manifest.image) — [cilium/cilium:contrib/containerlab/auto-discovery/default-gateway/service/service.yaml L59](https://github.com/cilium/cilium/blob/v1.20.2/contrib/containerlab/auto-discovery/default-gateway/service/service.yaml#L59): `image: curlimages/curl`
- `docker.io/frrouting/frr` (medium; manifest.image) — [cilium/cilium:contrib/containerlab/auto-discovery/default-gateway/multi-homing/topo.yaml L7](https://github.com/cilium/cilium/blob/v1.20.2/contrib/containerlab/auto-discovery/default-gateway/multi-homing/topo.yaml#L7): `image: frrouting/frr:v8.4.0`
- `docker.io/golangci/golangci-lint` (medium; make.image-ref) — [cilium/cilium:Makefile L461](https://github.com/cilium/cilium/blob/v1.20.2/Makefile#L461): `docker.io/golangci/golangci-lint:$(GOLANGCILINT_WANT_VERSION)@$(GOLANGCILINT_IMAGE_SHA) \`
- `docker.io/library/nginx` (medium; manifest.image) — [cilium/cilium:.github/actions/generic-external-targets/nginx-external.yaml L100](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/generic-external-targets/nginx-external.yaml#L100): `image: nginx:latest`
- `docker.io/mikefarah/yq` (medium; make.image-ref, script.image-ref) — [cilium/cilium:Makefile.defs L265](https://github.com/cilium/cilium/blob/v1.20.2/Makefile.defs#L265): `YQ_IMAGE ?= "docker.io/mikefarah/yq@sha256:32be61dc94d0acc44f513ba69d0fc05f1f92c2e760491f2a27e11fc13cde6327" #…`
- `docker.io/nicolaka/netshoot` (medium; manifest.image) — [cilium/cilium:contrib/containerlab/auto-discovery/default-gateway/multi-homing/topo.yaml L104](https://github.com/cilium/cilium/blob/v1.20.2/contrib/containerlab/auto-discovery/default-gateway/multi-homing/topo.yaml#L104): `image: nicolaka/netshoot:v0.11`
- `docker.io/renovate/renovate` (medium; make.image-ref) — [cilium/cilium:Makefile L697](https://github.com/cilium/cilium/blob/v1.20.2/Makefile#L697): `docker.io/renovate/renovate:latest \`
- `gcr.io/etcd-development/etcd` (medium; make.image-ref) — [cilium/cilium:Makefile.defs L67](https://github.com/cilium/cilium/blob/v1.20.2/Makefile.defs#L67): `ETCD_IMAGE=gcr.io/etcd-development/etcd:$(ETCD_IMAGE_VERSION)@$(ETCD_IMAGE_SHA)`
- `gcr.io/k8s-staging-perf-tests/dnsperfgo` (medium; manifest.image) — [cilium/cilium:.github/actions/cl2-modules/fqdn/dns-prober.yaml L22](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/cl2-modules/fqdn/dns-prober.yaml#L22): `image: gcr.io/k8s-staging-perf-tests/dnsperfgo:v1.4.0`
- `quay.io/cilium/cilium-bpftool` (medium; docs.image-ref) — [cilium/cilium:Documentation/contributing/development/images.rst L88](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/contributing/development/images.rst#L88): `| github.com/cilium/image-tools | images/bpftool/Dockerfile                   | quay.io/cilium/cilium-bpftool …`
- `quay.io/cilium/cilium-builder` (medium; docs.image-ref, script.image-ref) — [cilium/cilium:Documentation/contributing/development/images.rst L66](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/contributing/development/images.rst#L66): `| github.com/cilium/cilium      | images/builder/Dockerfile                   | quay.io/cilium/cilium-builder …`
- `quay.io/cilium/cilium-checkpatch` (medium; make.image-ref) — [cilium/cilium:bpf/Makefile.bpf L52](https://github.com/cilium/cilium/blob/v1.20.2/bpf/Makefile.bpf#L52): `CHECKPATCH_IMAGE := quay.io/cilium/cilium-checkpatch:1755701578-b97bd7a@sha256:f1332fa6edbbd40882a59ceae4a7843…`
- … 61 more

### manifest (14)

- `.github/actions/cl2-modules/fqdn/dns-prober.yaml` (medium; manifest.workloads) — [cilium/cilium:.github/actions/cl2-modules/fqdn/dns-prober.yaml L4](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/cl2-modules/fqdn/dns-prober.yaml#L4): `kind: DaemonSet`
- `.github/actions/cl2-modules/netpol/deployment.yaml` (medium; manifest.workloads) — [cilium/cilium:.github/actions/cl2-modules/netpol/deployment.yaml L5](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/cl2-modules/netpol/deployment.yaml#L5): `kind: Deployment`
- `.github/actions/generic-external-targets/nginx-external.yaml` (medium; manifest.workloads) — [cilium/cilium:.github/actions/generic-external-targets/nginx-external.yaml L63](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/generic-external-targets/nginx-external.yaml#L63): `kind: DaemonSet`
- `.github/actions/setup-aks-cluster/dummy-cni-daemonset.yaml` (medium; manifest.workloads) — [cilium/cilium:.github/actions/setup-aks-cluster/dummy-cni-daemonset.yaml L2](https://github.com/cilium/cilium/blob/v1.20.2/.github/actions/setup-aks-cluster/dummy-cni-daemonset.yaml#L2): `kind: DaemonSet`
- `contrib/containerlab/auto-discovery/default-gateway/service/service.yaml` (medium; manifest.workloads) — [cilium/cilium:contrib/containerlab/auto-discovery/default-gateway/service/service.yaml L41](https://github.com/cilium/cilium/blob/v1.20.2/contrib/containerlab/auto-discovery/default-gateway/service/service.yaml#L41): `kind: Deployment`
- `contrib/containerlab/pod-ip-pool/bgp.yaml` (medium; manifest.workloads) — [cilium/cilium:contrib/containerlab/pod-ip-pool/bgp.yaml L98](https://github.com/cilium/cilium/blob/v1.20.2/contrib/containerlab/pod-ip-pool/bgp.yaml#L98): `kind: DaemonSet`
- `contrib/containerlab/service/service.yaml` (medium; manifest.workloads) — [cilium/cilium:contrib/containerlab/service/service.yaml L41](https://github.com/cilium/cilium/blob/v1.20.2/contrib/containerlab/service/service.yaml#L41): `kind: Deployment`
- `contrib/k8s/clear-kubeproxy-iptables.yaml` (medium; manifest.workloads) — [cilium/cilium:contrib/k8s/clear-kubeproxy-iptables.yaml L15](https://github.com/cilium/cilium/blob/v1.20.2/contrib/k8s/clear-kubeproxy-iptables.yaml#L15): `kind: DaemonSet`
- `examples/hubble/hubble-cli.yaml` (medium; docs.raw-url) — [cilium/cilium:Documentation/observability/hubble/configuration/tls.rst L342](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/observability/hubble/configuration/tls.rst#L342): `$ kubectl apply -n kube-system -f https://raw.githubusercontent.com/cilium/cilium/main/examples/hubble/hubble-…`
- `examples/kubernetes-es/es-sw-policy.yaml` (medium; docs.raw-url) — [cilium/cilium.io:src/posts/17-05-2018-es-security/index.md L70](https://github.com/cilium/cilium.io/blob/18a067b89f7cd2f92f1322977a0e70b406943bfa/src/posts/17-05-2018-es-security/index.md#L70): `([View full policy YAML](https://raw.githubusercontent.com/cilium/cilium/v1.1/examples/kubernetes-es/es-sw-pol…`
- `examples/kubernetes/servicemesh/ingress-path-types-ingress.yaml` (medium; docs.raw-url) — [cilium/cilium:Documentation/network/servicemesh/path-types.rst L30](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/network/servicemesh/path-types.rst#L30): `$ kubectl apply -f https://raw.githubusercontent.com/cilium/cilium/main/examples/kubernetes/servicemesh/ingres…`
- `examples/kubernetes/servicemesh/ingress-path-types.yaml` (medium; docs.raw-url) — [cilium/cilium:Documentation/network/servicemesh/path-types.rst L28](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/network/servicemesh/path-types.rst#L28): `$ kubectl apply -f https://raw.githubusercontent.com/cilium/cilium/main/examples/kubernetes/servicemesh/ingres…`
- `examples/minikube/demo.yaml` (medium; docs.raw-url) — [cilium/cilium.io:src/posts/08-07-2017-tutorial-applying-http-security-rules-with-kubernetes/index.md L49](https://github.com/cilium/cilium.io/blob/18a067b89f7cd2f92f1322977a0e70b406943bfa/src/posts/08-07-2017-tutorial-applying-http-security-rules-with-kubernetes/index.md#L49): `$ kubectl create -f https://raw.githubusercontent.com/cilium/cilium/master/examples/minikube/demo.yaml`
- `examples/minikube/l3_l4_l7_policy.yaml` (medium; docs.raw-url) — [cilium/cilium.io:src/posts/08-07-2017-tutorial-applying-http-security-rules-with-kubernetes/index.md L98](https://github.com/cilium/cilium.io/blob/18a067b89f7cd2f92f1322977a0e70b406943bfa/src/posts/08-07-2017-tutorial-applying-http-security-rules-with-kubernetes/index.md#L98): `$ kubectl create -f https://raw.githubusercontent.com/cilium/cilium/master/examples/minikube/l3_l4_l7_policy.y…`

### registry (12)

- `docker.io` (high; workflow.registry-login) — [cilium/cilium:.github/workflows/build-images-releases.yaml L78](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/build-images-releases.yaml#L78): `uses: docker/login-action@dbcb813823bdd20940b903addbd779551569679f # v4.6.0`
- `quay.io` (medium; make.registry-variable) — [cilium/cilium:Makefile.docker L68](https://github.com/cilium/cilium/blob/v1.20.2/Makefile.docker#L68): `DOCKER_REGISTRY ?= quay.io`
- `docker.io/cilium` (low; make.registry-ref, make.registry-variable, script.registry-ref) — [cilium/cilium:Makefile.docker L71](https://github.com/cilium/cilium/blob/v1.20.2/Makefile.docker#L71): `IMAGE_REPOSITORY := $(DOCKER_DEV_ACCOUNT)`
- `docker.io/rhysd/actionlint` (low; workflow.registry-ref) — [cilium/cilium:.github/workflows/lint-workflows.yaml L268](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/lint-workflows.yaml#L268): `uses: docker://docker.io/rhysd/actionlint:1.7.12@sha256:b1934ee5f1c509618f2508e6eb47ee0d3520686341fec936f3b793…`
- `quay.io/api/v1/repository/cilium/cilium-envoy/tag` (low; script.registry-ref) — [cilium/cilium:images/scripts/update-cilium-envoy-image.sh L29](https://github.com/cilium/cilium/blob/v1.20.2/images/scripts/update-cilium-envoy-image.sh#L29): `tags=$(curl -s "https://quay.io/api/v1/repository/cilium/${repo}/tag/?onlyActiveTags=true&filter_tag_name=like…`
- `quay.io/cilium` (low; docs.registry-ref, make.registry-ref, script.registry-ref) — [cilium/cilium:Documentation/contributing/development/images.rst L185](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/contributing/development/images.rst#L185): `''quay.io/cilium/*-ci''. They will be available there for 1 week before they are`
- `quay.io/cilium/docs-builder` (low; workflow.registry-ref) — [cilium/cilium:.github/workflows/documentation.yaml L68](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/documentation.yaml#L68): `uses: docker://quay.io/cilium/docs-builder:e6773ed21ab03066c0f24e68f342f18804a8ee0a@sha256:d3beff6d6408c09f6e1…`
- `quay.io/cilium/image-maker` (low; workflow.registry-ref) — [cilium/cilium:.github/workflows/lint-images-base.yaml L78](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/lint-images-base.yaml#L78): `- uses: docker://quay.io/cilium/image-maker:7de7f1c855ce063bdbe57fdfb28599a3ad5ec8f1@sha256:dde8500cbfbb6c4143…`
- `quay.io/cilium/scruffy` (low; workflow.registry-ref) — [cilium/cilium:.github/workflows/ci-images-garbage-collect.yaml L28](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/ci-images-garbage-collect.yaml#L28): `uses: docker://quay.io/cilium/scruffy:v0.0.3@sha256:ca997451b739cbf03c204cb2523a671c31c61edc606aa5d20dc3560bc7…`
- `quay.io/oauth2/federation/robot/token` (low; workflow.registry-ref) — [cilium/cilium:.github/workflows/build-images-ci-v1.20.yaml L167](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/build-images-ci-v1.20.yaml#L167): `"https://quay.io/oauth2/federation/robot/token" \`
- `quay.io/organization/cilium` (low; docs.registry-ref) — [cilium/cilium:SECURITY-INSIGHTS.yml L27](https://github.com/cilium/cilium/blob/v1.20.2/SECURITY-INSIGHTS.yml#L27): `- https://quay.io/organization/cilium`
- `quay.io/repository/cilium/cilium` (low; docs.registry-ref) — [cilium/cilium:Documentation/installation/k8s-install-helm.rst L78](https://github.com/cilium/cilium/blob/v1.20.2/Documentation/installation/k8s-install-helm.rst#L78): `* **Browse the registry:** 'Quay.io tags <https://quay.io/repository/cilium/cilium?tab=tags>'_`

### release-trigger (2)

- `v1.20.[0-9]+` (high; workflow.tag-trigger) — [cilium/cilium:.github/workflows/build-images-releases.yaml L6](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/build-images-releases.yaml#L6): `- v1.20.[0-9]+`
- `v1.20.[0-9]+-*` (high; workflow.tag-trigger) — [cilium/cilium:.github/workflows/build-images-releases.yaml L7](https://github.com/cilium/cilium/blob/v1.20.2/.github/workflows/build-images-releases.yaml#L7): `- v1.20.[0-9]+-*`

### security-policy (1)

- `SECURITY.md` (high; docs.security-policy) — [cilium/cilium:SECURITY.md L1](https://github.com/cilium/cilium/blob/v1.20.2/SECURITY.md#L1): `# Security Policy`

### tag-scheme (1)

- `^v(?P<version>\d+\.\d+\.\d+(?:-(?:pre\.\d+|rc\.\d+|rc\d+))?)$` (high; tags.ls-remote) — [cilium/cilium/releases/tag/v1.20.2 refs/tags/v1.20.2](https://github.com/cilium/cilium/releases/tag/v1.20.2#refs/tags/v1.20.2): `e0dc92bd6dac33d7ba3b5eace82a0448bbb13aac refs/tags/v1.20.2`

### version-relation (2)

- `chart.version = {{.Version}}` (high; helm.chart-version-matches-ref) — [cilium/cilium:install/kubernetes/cilium/Chart.yaml L5](https://github.com/cilium/cilium/blob/v1.20.2/install/kubernetes/cilium/Chart.yaml#L5): `version: 1.20.2`
- `VERSION file = {{.Version}}` (medium; version-file.matches-ref) — [cilium/cilium:VERSION L1](https://github.com/cilium/cilium/blob/v1.20.2/VERSION#L1): `1.20.2`
