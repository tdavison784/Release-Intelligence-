# Actions Runner Controller release-channel research (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the checks and probes were re-run live; the live `ri check` outcomes are unchanged. Any unreachable host recorded below was a transient anonymous rate limit or an anonymous-auth refusal, not a block of this product's channels (details: docs/rerun/REPORT.md, per-product table). The original text below is kept as the record of 2026-10-01.

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. Product: Actions
Runner Controller (ARC), the Kubernetes operator for self-hosted GitHub Actions runners. Canonical
repository `github.com/actions/actions-runner-controller`, default branch **`master`** (there is no
`main`). Docs live partly in-repo (`docs/`, legacy) and partly on docs.github.com (current;
generic, not release-specific).

All hosts needed for the final definition answered: ghcr.io (anonymous token + `/v2` manifests),
api.github.com (anonymous, 60/h), raw.githubusercontent.com / github.com / codeload (release
assets, tag archives), `actions-runner-controller.github.io` and `actions.github.io` (legacy Helm
indexes). Nothing was unreachable. Registry quirk: ghcr `/tags/list` returns a **truncated** list
by default (`ghcr.io/actions/gha-runner-scale-set-controller` showed only 0.2.0–0.5.0 numeric
until `?n=1000`; every 0.6.0–0.15.0 manifest still answers 200 to HEAD) — `ri` probes manifests,
so verification is unaffected.

## 0. Executive summary (what a declarative definition MUST know)

1. **One repository, two release lineages with colliding version numbers.** The controller-era
   `v0.X.Y` tags (61 stable, `v0.1.0` 2020 → **v0.27.6, 2023-10-20**) stopped when the
   runner-scale-set mode took over: since then releases are tagged **`gha-runner-scale-set-X.Y.Z`**
   (26 stable, `gha-runner-scale-set-0.2.0` 2023-02-23 → **0.15.0, 2026-10-01 — cut today**).
   Both families number from 0.x, so `0.9.0` exists twice (as `v0.9.0` and as
   `gha-runner-scale-set-0.9.0`) and the families mean *different artifacts*: a single version
   channel must select one family. Precedent: ingress-nginx's `tagPrefix: controller-v` among
   `helm-chart-*`/`ingress-nginx-*` tags. **Canonical = the `gha-runner-scale-set-` family**
   (the only live channel; the v-family is dead since 2023-10, see 7). Remaining 62 non-release
   tags: legacy chart releases `actions-runner-controller-0.1.2…0.23.7` (GitHub releases that
   carry the chart tgz), ancient `0.2.0`/`0.3.0`, no pre-releases.
2. **The release unit of the current era is the gha-runner-scale-set chart release.** Tag
   `gha-runner-scale-set-X.Y.Z` → GitHub release of the same name → workflow
   `gha-publish-chart.yaml` (workflow_dispatch) builds and pushes:
   `ghcr.io/actions/gha-runner-scale-set-controller:X.Y.Z` (plus `X.Y.Z-<shortsha>` and canary),
   and both charts packaged from the tree to OCI
   `ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set-controller` and
   `…/gha-runner-scale-set`, plus the two `-experimental` variants (same versions, "not supported
   for production workloads"). `hack/check-gh-chart-versions.sh` **fails the release when binary
   version ≠ chart version ≠ appVersion**, and the tag is cut on the "Prepare X.Y.Z release"
   bump commit: **chart version == appVersion == release version** at every tag checked (0.6.0,
   0.9.0, 0.14.2, 0.15.0; no karpenter-style CI lag). At 0.2.0 the runner chart was still named
   `charts/auto-scaling-runner-set`; `charts/gha-runner-scale-set{,-controller}` exist from 0.3.0
   on. All 26 versions verified present as OCI chart tags [200] and image manifests [200].
3. **Release notes = the GitHub release body** (no in-repo changelog for this era). Shape: three
   to five artifact-link bullets (image + charts), a warning line about experimental charts,
   `## What's Changed` with one generated PR-title bullet per merged PR (dependabot and
   github-actions[bot] "Updates: runner to v2.3xx.x" lines included), `## New Contributors`,
   `Full Changelog` compare link. No breaking-change section; breaking changes surface as PR
   titles ("Remove …", "Fix …") like the legacy bodies. The legacy-era curated notes
   (`docs/releasenotes/0.2X.md`, H1 `# actions-runner-controller v0.X.Y` with `## Upgrading`,
   `## BREAKING CHANGE : …`, `## ENHANCEMENT : …`, `## FIX : …` sections) cover only lines
   0.22–0.27 of the dead v-family — wrong lineage for the canonical channel.
4. **Assets: exactly one per release, `actions-runner-controller.yaml`** (every release checked,
   both eras). It is the kustomize build of the *legacy* install path; at 0.15.0 it references
   `summerwind/actions-runner-controller:gha-runner-scale-set-0.15.0` (Docker Hub tag exists
   [200]; a legacy-repo image carrying the full release tag). Legacy v-era releases were signed
   (`actions-runner-controller.yaml.asc`, hack/signrel) — no signature assets in the current era.
5. **CRDs in-tree, not as release assets**: `config/crd/bases/` at every tag carries the 4
   `actions.github.com` CRDs of the scale-set mode (AutoscalingListener,
   AutoscalingRunnerSet, EphemeralRunner, EphemeralRunnerSet; `charts/gha-runner-scale-set-controller/crds/`
   ships the same four) plus the 5 legacy `actions.summerwind.dev` CRDs (Runner, RunnerDeployment,
   RunnerReplicaSet, RunnerSet, HorizontalRunnerAutoscaler). Helm does not manage crds/ on
   upgrade, which is why the whole upgrade procedure exists (see 6).
6. **Upgrade guidance is generic, not per-version, for the current era.** docs.github.com
   ("About ARC" checked: no Kubernetes-support statement; no versioned upgrade page), the README
   and discussion #2775 point at docs.github.com; the only in-repo upgrading doc is the legacy
   chart's `charts/actions-runner-controller/docs/UPGRADING.md` (helm + `kubectl replace -f crds`
   procedure, versionless). Community guidance repeats "CRDs must be applied manually"
   (community discussions #160446/#160717). So no upgrade-guide source is declarable for the
   canonical lineage; per-release intelligence is the release body + CRD/values diffs.
7. **Project status: ACTIVE, with a dead legacy lineage.** Releases continue on the
   `gha-runner-scale-set-` family (0.13.1 2025-12-23, 0.14.0 2026-03-19, 0.14.1 2026-04-15,
   0.14.2 2026-05-22, 0.15.0 2026-10-01; master had commits the same day). But the v-tagged
   controller lineage ended at v0.27.6 (2023-10-20) and the README (present since the v0.26/v0.27
   era) states: "With the introduction of autoscaling runner scale sets, the existing autoscaling
   modes are now legacy. The legacy modes … will continue to be maintained by the community
   only." (discussion #2775). Two lifecycle statements: `deprecated` — legacy autoscaling modes /
   v0.x lineage, evidence README; `end-of-life` — the v-tagged release lineage, no release since
   2023-10, evidence README ("legacy") + the tag record itself.
8. **Security**: GitHub repository advisories feed (api.github.com) — **0 published GHSA** for
   this repository today; SECURITY.md is GitHub's coordinated-disclosure policy (email), not a
   channel. The feed is declared anyway (it is the machine-readable channel should one appear).
9. **Legacy-era channels (declared nowhere, recorded for the drift/scope story)**: controller
   image `docker.io/summerwind/actions-runner-controller:0.X.Y` and the ghcr twin
   `ghcr.io/actions-runner-controller/actions-runner-controller:v0.X.Y` (v-prefixed; only
   v0.25.0–v0.27.6 verified present, 11 numeric tags); chart `actions-runner-controller`
   (version stream 0.1.2 → **0.23.7**, appVersion = controller version, e.g. 0.23.7 → 0.27.6)
   on the chart-releaser gh-pages index `https://actions-runner-controller.github.io/actions-runner-controller`
   [200, 60 entries, frozen 2023-11-27; identical copy at `https://actions.github.io/actions-runner-controller`]
   and as tgz assets on the `actions-runner-controller-X.Y.Z` chart-release tags;
   `docs/releasenotes/app-version-mapping.md` is the upstream mapping table
   (0.27.0→0.22.0 … 0.18.2→0.11.0).

## 1. Versions

- `git ls-remote --tags` [200]: 149 tags. Release families: `^v(?P<version>\d+\.\d+\.\d+)$`
  (61) and `^gha-runner-scale-set-(?P<version>\d+\.\d+\.\d+)$` (26). Excluded: 60
  `actions-runner-controller-*` chart tags, `0.2.0`/`0.3.0` unprefixed, `users` (branch-like tag
  present in the tag namespace). No pre-release tags in either family.
- Lineage: `minor` (0.2 → 0.3 → … → 0.15; patches 0.6.1, 0.8.1–3, 0.9.1–3, 0.10.1, 0.12.1,
  0.13.1, 0.14.1–2 exist alongside).
- Cadence 2025–2026: roughly every 1–5 months; today's 0.15.0 spans 0.14.2 (2026-05-22) →
  2026-10-01.

## 2. Release notes

- Canonical: GitHub release body (`github-releases` locator, `ref: {{.Tag}}`) [200, api.github.com
  anonymous]. Generated "What's Changed" bullets; classify rules must skip dependabot/action
  churn and keep "Updates: runner to …" as the runner-pin signal (the runner images are a
  separate product's cadence, but the pin per ARC release is a fact of this release).
- Legacy only: `docs/releasenotes/0.2{2..7}.md` (H1 per release, sections `## Upgrading`,
  `## BREAKING CHANGE :`, `## ENHANCEMENT :`, `## FIX :`) — belongs to the dead v-family.

## 3. Charts (current era)

- `gha-runner-scale-set-controller`: OCI
  `ghcr.io/actions/actions-runner-controller-charts/gha-runner-scale-set-controller` — 26 numeric
  tags (0.2.0…0.15.0) [200]; source `charts/gha-runner-scale-set-controller/` (≥ 0.3.0). CRD-only
  chart ("A Helm chart for install actions-runner-controller CRD"), image
  `ghcr.io/actions/gha-runner-scale-set-controller`, tag defaults to appVersion. No `kubeVersion`.
- `gha-runner-scale-set`: OCI `…/gha-runner-scale-set` — 26 tags [200]; source
  `charts/gha-runner-scale-set/` (≥ 0.3.0; `auto-scaling-runner-set` at 0.2.0). Deploys an
  AutoscalingRunnerSet; default runner image `ghcr.io/actions/actions-runner` (separate product).
- Experimental twins at the same tags ("may be modified or republished without notice") — not
  declared.
- Version relation: template `{{.Version}}` (== appVersion == image tag), enforced upstream by
  `hack/check-gh-chart-versions.sh`.

## 4. Images

- `ghcr.io/actions/gha-runner-scale-set-controller` — bare-semver tags for all 26 releases
  [HEAD manifests 200 at 0.2.0/0.6.0/0.9.0/0.14.2/0.15.0]; plus `<version>-<shortsha>` and canary.
- Runner images (`ghcr.io/actions/actions-runner`, `…-dind`, `…-dind-rootless`, ubuntu-tagged
  variants like `v2.335.1-ubuntu-22.04`): **a separate product's cadence** (runner binaries);
  ARC releases only pin them ("Updates: runner to v2.33x.y" bullets) and the gha-runner-scale-set
  values default to a floating `latest`. Not modeled as artifacts of this product.
- Legacy: `docker.io/summerwind/actions-runner-controller` (0.x tags, and oddly
  `gha-runner-scale-set-0.15.0` — the manifest build pushes the full tag name), ghcr twin under
  the old org with v-prefixed tags. Not declared (dead lineage).

## 5. CRDs

- `config/crd/bases` repo-dir at `{{.Tag}}`: 9 files at 0.15.0 (4 `actions.github.com` + 5
  `actions.summerwind.dev`); 4 `actions.github.com` already at 0.2.0. Chart `crds/` dir carries
  the 4 current-mode CRDs only.

## 6. Upgrade guidance / compatibility

- No per-version upgrade document for the current era (checked docs.github.com About-ARC page:
  no Kubernetes support statement; no upgrade page; README points to docs.github.com quickstart).
  Legacy `charts/actions-runner-controller/docs/UPGRADING.md`: versionless procedure.
- No compatibility matrix in-repo (k8s or runner versions). Runner compatibility arrives as the
  release-body "Updates: runner to vX" bullets; k8s compatibility is only CI test matrices.

## 7. Security

- `github-advisories` feed: 0 published advisories (checked 2026-10-01). SECURITY.md → GitHub
  coordinated disclosure (email), out of scope for a channel.

## 8. Hosts

| Host | Status | Use |
|---|---|---|
| ghcr.io | [200] anonymous token + manifests | controller image, OCI charts |
| api.github.com | [200] anonymous (60/h) | release bodies, versions fallback, advisories |
| raw.githubusercontent.com / github.com / codeload | [200] | repo files, release asset, tag archive |
| actions-runner-controller.github.io | [200] | legacy gh-pages chart index (frozen 2023-11) |
| actions.github.io | [200] | identical legacy index copy |
| hub.docker.com / registry-1.docker.io | [200] / not needed | legacy image only; no channel declared |

## 9. Definition consequences

- `tagPrefix: gha-runner-scale-set-`, `tagPattern: ^gha-runner-scale-set-(?P<version>\d+\.\d+\.\d+)$`
  (canonical, ingress-nginx `controller-v` precedent); github-releases versions fallback.
- Sources: release body (release-notes, classify for generated bullets); README "Getting
  Started" section on master (lifecycle evidence for the legacy-modes deprecation); advisories.
- Artifacts: controller image (ghcr OCI), both charts (ghcr OCI) with repo-file chart-metadata +
  helm-values contents (files at the tag ARE the release's), CRDs repo-dir, install-manifest
  http asset (image-refs), source archive http.
- Lifecycle: `deprecated` (legacy autoscaling modes / v0.x lineage, README evidence) and
  `end-of-life` (v-tagged releases, ended 2023-10, README + tag record).
- Represented as gaps: no per-version upgrade guide (era), no k8s compatibility matrix
  (upstream), runner-image pins only as release-body bullets.
