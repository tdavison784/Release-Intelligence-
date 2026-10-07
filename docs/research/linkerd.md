# Linkerd release-channel research (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** the checks and probes were re-run live; the live `ri check` outcomes are unchanged. Any unreachable host recorded below was a transient anonymous rate limit or an anonymous-auth refusal, not a block of this product's channels (details: docs/rerun/REPORT.md, per-product table). The original text below is kept as the record of 2026-10-01.

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could
not be checked here. Product: Linkerd, the ultralight service mesh (Rust `linkerd2-proxy` data plane
+ Go control plane + `linkerd` CLI). Canonical repository `github.com/linkerd/linkerd2`. CNCF
graduated. Scope of this definition: the **stable control plane** (`linkerd-control-plane` +
`linkerd-crds` charts, control-plane images, CLI, the pinned proxy) — NOT the in-repo extensions
(`viz/`, `multicluster/`, `jaeger/` charts and their images) and not the `linkerd2-cni` /
`proxy-init` / `linkerd2-proxy` projects (modelled as per-release pins instead; see artifacts).

## 0. Executive summary (what a declarative definition MUST know)

1. **Three tag families, and the "stable channel" changed meaning in February 2024.**
   - `stable-2.X.N` — 56 tags, 2018 → **stable-2.14.10 (2024-02-20), the last open-source stable
     release with artifacts** (charts, images, CLI binaries; GitHub release with a hand-curated
     body). One `stable-2.12.0-rc2` rc tag exists and is excluded by the pattern.
   - `version-2.Y` — 6 tags (2024-02-20 → 2026-06-22): since 2.15, a "stable release" is a
     **designation marker** placed on an edge commit: `version-2.15` = `edge-24.2.4`,
     `version-2.16` = `edge-24.8.2` (website data), `version-2.17` = `edge-24.11.8`,
     `version-2.18` = `edge-25.4.4`, `version-2.19` = `edge-25.10.7` (same commit, double-tagged
     [verified with `git tag --points-at`]), `version-2.20` (2026-06-22). The mapping is published
     in the website repo as `linkerd.io/data/versions.yaml` [200] — upstream data, not a template
     relation.
   - `edge-YY.M.N` — 378 tags, weekly CalVer (year.month.Nth-of-month), 2019 → **edge-26.9.3
     (2026-09-16), still active**. This is now the only channel that publishes open-source release
     artifacts.
   The 2.14.10 release body states it in upstream's own words: *"NOTE about 2.15 and beyond: As of
   February 2024, the Linkerd project is no longer producing open source stable release artifacts"*,
   and the 2.15 announcement's "A new model for stable releases" section: *"as of Linkerd 2.15,
   producing stable Linkerd releases will be in the hands of the vendor community … We'll continue
   publishing edge releases to the GitHub repo as usual."* [200, linkerd/website]
   Consequences: `run.linkerd.io/install` defaults to `edge-26.9.3` [200]; the stable Helm index
   (`helm.linkerd.io/stable`) is frozen at 2024-02-20; `version-2.Y` tags have no GitHub release and
   no assets (expanded_assets page empty [200]); ghcr image tags for the stable family stop at
   `stable-2.14.10`.
2. **Canonical channel chosen: the stable lineage `stable-2.X.N ∪ version-2.Y`** (one family per
   definition, ingress-nginx / actions-runner-controller family-selection precedent; the edge CalVer
   family is the weekly prerelease stream and is excluded, istio-style channel discipline — but with
   the twist that since 2024 the edge family is also the *carrier* of stable artifacts). One
   pattern reads both eras: `^(?:stable|version)-(?P<major>\d+)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?$`
   (component-group assembly, PostgreSQL precedent: `version-2.15` → 2.15.0, `stable-2.14.10` →
   2.14.10). Upstream says stable minors may be skipped within `2.x`… actually the opposite: docs
   "Upgrade paths" (2.15–2.17) say same-"major" (their major = our 2.x minor line) patch skipping is
   safe but skipped "major" upgrades are NOT supported — one line at a time, which is exactly the
   model's `lineage: minor` path.
3. **Helm charts live in the main repository** (`charts/linkerd-control-plane`,
   `charts/linkerd-crds`, plus extensions `charts/linkerd2-cni`, `viz/charts/linkerd-viz`, …) with
   placeholder metadata (`version: 0.0.0-undefined`, `appVersion: edge-XX.X.X`) rewritten by CI at
   packaging — the Strimzi placeholder trap. Published ≤2.14 to `https://helm.linkerd.io/stable`
   [200, index frozen 2024-02-20] where **chart versions are independent of releases**
   (`linkerd-control-plane` 1.9.x–1.16.11, `linkerd2-cni` 30.x, sparse `linkerd-crds` 1.4–1.8 with
   NO appVersion at all) and the join is `appVersion == stable-2.X.N` → **lookup by appVersion
   == {{.Tag}}** (Argo CD precedent). The current edge index (`helm.linkerd.io/edge` [200]) uses
   CalVer chart versions (`2026.9.3`, appVersion `edge-26.9.3`) and belongs to the edge channel.
4. **Images on `ghcr.io/linkerd/*`, tagged with the full release tag** (`stable-2.14.10` etc.) —
   verified for `controller`, `policy-controller`, `proxy`, `web`, `debug` (stable-2.12.0/2.13.0/
   2.14.0/2.14.10 all present [200]; tags exist from the stable-2.9 era back to 2020). `cr.l5d.io`
   is an **alias of ghcr.io**: its 401 advertises `WWW-Authenticate: Bearer
   realm="https://ghcr.io/token"` and the ghcr token lists identical tags through `cr.l5d.io` [200].
   **`registry.k8s.io/linkerd/*` does NOT exist** (404 "repository does not exist" for every name
   probed [200]) — the hypothesised k8s.io mirror is fiction. The pre-2020 registry
   `gcr.io/linkerd-io` is out of scope.
5. **The proxy version split is real but lives one level up than the image tag.** Every release pins
   the data-plane proxy SOURCE version in `.proxy-version` at the tag (plain text): v2.185.0
   (2.12.0) → v2.207.0 (2.14.0) → v2.210.4 (2.14.10) → v2.246.0 (2.16) → v2.326.0 (2.19) →
   v2.359.0 (2.20) [200, git show]. The `linkerd2-proxy` repository publishes **tags
   `release/v2.NNN.N`** (625 tags; NO GitHub release objects — the release page 404s [200]); the
   built `proxy` IMAGE is tagged with the *control-plane* release tag, not the proxy version
   (ghcr.io/linkerd/proxy has no `v2.NNN` tags [200]). So the split is a **`version:pattern` pin on
   `.proxy-version`** (Flux construct), probed at the pinned tag via
   `raw.githubusercontent.com/linkerd/linkerd2-proxy/release/<v>/Cargo.toml` [200].
6. **Two more per-release pins sit in the charts' own values.yaml** (kube-prometheus-stack
   `version:field` precedent): `charts/linkerd-control-plane/values.yaml`
   `proxyInit.image.version` (v2.0.0 at 2.12.0 → v2.4.3 at 2.19; **removed in 2.20** — native
   sidecars became the default, announced in the 2.20 blog) and `charts/linkerd2-cni/values.yaml`
   `image.version` (v1.1.0 at 2.13.0 → v1.6.8 at 2.20; no pin before 2.13). Both images are probe
   able on `ghcr.io/linkerd/{proxy-init,cni-plugin}` [200, all pinned versions present].
7. **Release notes**: ≤2.14 the GitHub release body is a hand-curated changelog ("This stable
   release back-ports bugfixes and improvements from recent edge releases. Introduced support for …
   (#11222; fixes #11175) …") [200]. ≥2.15 there is no release at all; the curated channel is the
   website announcement blog per logical version (`linkerd.io/content/blog/<YYYY>/<MMDD>-announcing
   -linkerd-2.X/index.md` in github.com/linkerd/website, all six exist [200]) — the date-prefixed
   path is NOT derivable from the version, so each is declared as its own availability-pinned
   source. The in-repo `CHANGES.md` is dead (newest section v18.9.1, 2018 CalVer era).
8. **Upgrade guidance** is per-docs-version `tasks/upgrade.md` in the website repo
   (`linkerd.io/content/2.12 … 2.19`, plus generic `2-edge` [200]): up to 2.14 it carries
   `### Upgrade notice: stable-X.Y.0` sections per minor (e.g. 2.14.0: relink multicluster clusters
   after upgrading) — a `markdown-section` source; 2.15–2.17 carry the "Upgrade paths" policy
   section; 2.18+ are generic procedures. The install/upgrade path is `linkerd upgrade | kubectl
   apply` (or Helm); one minor line at a time.
9. **Kubernetes compatibility is `kubeVersion` in Chart.yaml at the tag** — a per-release minimum:
   `>=1.21.0-0` (2.12–2.14.10) → `>=1.22.0-0` (2.15–2.17) → `>=1.23.0-0` (2.18–2.19) → `>=1.31.0-0`
   (2.20) [200, git show]. No support-status table exists anywhere upstream; the chart-metadata
   content construct turns this into `chart-kubeVersion` constraints automatically.
10. **CRDs** live in-tree in the `linkerd-crds` chart (`charts/linkerd-crds/templates/*.yaml` at
    every tag, all eras [200]) — read as a `repo-dir` + `crds` contents snapshot. The published
    `linkerd-crds` chart cannot be related to releases (independent sparse versions, no
    appVersion).
11. **CLI binaries** are bare per-OS/arch assets of the GitHub release (no archive extension;
    `run.linkerd.io/install` downloads `linkerd2-cli-${LINKERD2_VERSION}-${OS}`):
    `linkerd2-cli-stable-2.14.10-{darwin,darwin-arm64,linux,linux-arm64,…}[.sha256]` [200] — ≤2.14
    only.

## 1. Channels and artifacts

| Channel | Kind | Relation to release | Era | Verified |
|---|---|---|---|---|
| git tags `stable-2.X.N`, `version-2.Y` | versions (canonical) | tag IS the release | 2018→ | [200] ls-remote |
| GitHub releases (stable tags) | versions fallback + release-notes bodies | per release | ≤2.14.10 | atom feed [200]; api.github.com rate-limited (403) |
| `helm.linkerd.io/stable` index | helm-repo, chart `linkerd-control-plane` | lookup `appVersion == {{.Tag}}` | 2.12.0–2.14.10 | [200] index frozen 2024-02-20 |
| `ghcr.io/linkerd/{controller,policy-controller,web,debug}` | oci image | tag `{{.Tag}}` | ≤2.14.10 (tags from 2.9 era) | [200] tag lists |
| `ghcr.io/linkerd/proxy` | oci image | tag `{{.Tag}}` (NOT the proxy version) | ≤2.14.10 | [200] |
| `cr.l5d.io/linkerd/*` | oci (alias of ghcr.io) | same | same | [200] via ghcr token realm |
| GitHub release assets `linkerd2-cli-<tag>-<os>-<arch>` | http binary | template `{{.Tag}}` | ≤2.14.10 | [200] expanded_assets |
| `.proxy-version` at tag | repo-file text pin | `version:pattern` → linkerd2-proxy tag `release/vX` | ≥2.9 | [200] git show + raw 200 |
| `charts/linkerd-control-plane/values.yaml` `proxyInit.image.version` | repo-file YAML pin | `version:field` → ghcr proxy-init tag | 2.12.0–2.19 | [200] |
| `charts/linkerd2-cni/values.yaml` `image.version` | repo-file YAML pin | `version:field` → ghcr cni-plugin tag | ≥2.13.0 | [200] |
| `charts/linkerd-crds/templates/*.yaml` | repo-dir | `crds` contents at `{{.Tag}}` | all eras | [200] |
| `charts/linkerd-control-plane/Chart.yaml` `kubeVersion` | chart-metadata content | per-release k8s minimum | all eras | [200] |
| website `content/<X.Y>/tasks/upgrade.md` | repo-file | `markdown-section` "Upgrade notice: stable-X.Y.0" | 2.9–2.14 docs | [200] |
| website announcement blogs | repo-file (per version, availability-pinned) | release notes | 2.15–2.20 | [200] all six |
| GitHub advisories `linkerd/linkerd2` | github-advisories | security | all | SECURITY.md [200]; API rate-limited here |

## 2. The stable ↔ edge relationship (the part the model cannot express as one relation)

`version-2.Y` releases (2.15–2.20) have no artifacts of their own; the artifacts of the commit they
 designate are published under the sibling `edge-YY.M.N` tag (charts/images/CLI). Joining them
requires the upstream mapping table (`linkerd.io/data/versions.yaml`), which is data, not a
derivable template — recorded as the definition's central representability note. The stable-channel
artifacts in this definition therefore end at 2.14.10 (availability `>= 2.12.0, < 2.15.0`), and the
2.15+ era is covered by source-tree evidence (values, CRDs, kubeVersion, pins) plus the announcement
blogs. An alternative canonicalization (edge CalVer as the version stream) would have live published
artifacts for every release but would describe weekly prerelease hops, not the stable product users
upgrade through; it was rejected for the same reason istio tracks only the semver stream.

## 3. Related products (out of scope, recorded)

- `linkerd2-proxy` (github.com/linkerd/linkerd2-proxy) — the Rust data plane, own tag family
  `release/v2.NNN.N`, pinned per control-plane release by `.proxy-version`.
- `proxy-init` (github.com/linkerd2/proxy-init) and `cni-plugin` (github.com/linkerd/cni-plugin) —
  init/CNI images with their own releases, pinned per control-plane release in chart values.
- Extensions shipped from the same repository: `linkerd-viz` (viz/), `linkerd-multicluster`
  (multicluster/), `linkerd-jaeger` (jaeger-inline era) — own charts with CalVer/30.x version
  trains in the /edge index, own images (`metrics-api`, `tap`, `web` at ≥2.15 eras).
- `linkerd-smi` (github.com/linkerd/linkerd-smi), `linkerd-await`, `linkerd2-proxy-init` — adjacent.
- Buoyant Enterprise for Linkerd — the vendor stable channel since 2.15 (out of scope by design).

## 4. Hosts unreachable / misbehaving from this sandbox (2026-10-01)

- `api.github.com` — 403 anonymous rate limit (shared IP quota; discovery + release-body and
  advisory checks hit it). git/atom/raw hosts unaffected; transient, recorded per protocol.
- `registry.k8s.io` — reachable but `linkerd/*` repositories do not exist (404 body "repository
  does not exist"); NOT a Linkerd mirror. (The 307-redirect caveat for other products does not
  arise: there is nothing to probe.)
- `gcr.io/linkerd-io` — legacy registry (pre-2020 images), not probed; out of scope.
- `build.l5d.io` — proxy CI binaries, not needed for the stable channel.
- Docker Hub anonymous pulls — rate-limited (per prior products); not used here (linkerd images are
  ghcr/cr.l5d only).

## 5. Discovery baseline (what `ri discover` proposed, 2026-10-01)

Discovery ran BEFORE this research (protocol step 2). It picked the **edge** family
(`tagPrefix: edge-`, pattern `^edge-(?P<version>…)$`, latest edge-26.9.3) — mechanically "newest
tag wins", blind to the stable/edge semantics, the same lineage trap as actions-runner-controller
(order 20) but inverted: there the live family was invisible, here the artifact-bearing-but-
prerelease family won. It kept 4 charts/images proposals as "unverified" then DROPPED them for
failing against the /stable index and cr.l5d.io (it probed edge tags at stable channels — the
channel split speaking). Useful and kept: the git-tags/github-releases versions pair, the
github-advisories source (SECURITY.md evidence), the helm.linkerd.io/stable + /edge candidates, and
the placeholder-chart observation (`helm.placeholder-version-follows-release` correctly saw Chart.yaml
is rewritten at release time). Its proposal needed no LLM; everything above the tag scheme came from
research.
