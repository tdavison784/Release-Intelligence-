# External Secrets Operator release-channel map (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** api.github.com (7 advisories), charts.external-secrets.io, external-secrets.github.io, external-secrets.io and api.osv.dev answer; the canonical chart repository (declared first) validates for all 19 checked releases. The original text below is kept as the record of 2026-10-01.

Scope: product `external-secrets` (github.com/external-secrets/external-secrets, "ESO"). One repository, three
independently tagged release streams (operator, Helm chart, esoctl CLI).
Method: `git ls-remote`, blobless clone read at tags (`git show <tag>:<path>`), raw.githubusercontent.com (incl. the
`gh-pages` branch), GitHub release-asset downloads (HEAD -L), ghcr.io token + Distribution v2 API, hub.docker.com API.

Abbreviations: `RAW@<ref>` = https://raw.githubusercontent.com/external-secrets/external-secrets/<ref>;
`REL@<tag>` = https://github.com/external-secrets/external-secrets/releases/download/<tag>.

Blocked, not retried (proxy answers 403 to CONNECT or the API refuses the repository): api.github.com (HTTP 403 "GitHub
access to this repository is not enabled"), charts.external-secrets.io, external-secrets.github.io, external-secrets.io
(docs site), artifacthub.io, api.osv.dev, github.com HTML pages (releases.atom, security/advisories).
Reachable but not used by any adapter: proxy.golang.org (Go module version list), raw `github/advisory-database`.

---

## 1. Canonical versions

`git ls-remote --tags` = 272 refs in five families:

| Family | Pattern | Count | Notes |
|---|---|---|---|
| operator | `vX.Y.Z` | 136 stable | v0.3.x .. v2.11.0; `-rcN` on v0.6.0, v0.7.0, v0.17.1, v0.18.0 (4) |
| Helm chart | `helm-chart-X.Y.Z` | 126 | includes `-rc` forms; created by chart-releaser after the operator tag |
| esoctl CLI | `vX.Y.Z-esoctl` | 3 | v0.1.0 / v0.2.0 / v0.2.1; `v0.1.0-render` is its predecessor name |
| legacy junk | `v.0.3.6`, `v0.9.15-2` | 2 | must not match |

- Strict operator pattern: `^v(?P<version>\d+\.\d+\.\d+(?:-rc\d+)?)$` (excludes the other families).
- Latest stable on 2026-10-01: **v2.11.0** (tag 2026-09-18). Cadence is fast: v2.0.0 on 2026-02-06 to v2.11.0 on
  2026-09-18, a minor roughly every 2-3 weeks.
- Release process (`docs/contributing/release.md`, `.github/workflows/release.yml`): a manually dispatched "Create
  Release" action tags from `main`, creates the GitHub Release with `generate_release_notes: true`, promotes the
  already-built image, attaches SBOM/provenance and the rendered install manifest. The chart is released separately
  (section 5).
- History is linear: every tag is an ancestor of the next except v0.7.3 -> v0.8.0 and v0.8.17 -> v0.9.0 (old release
  branches). The newest minor is the only supported one, so patches are rare (v2.0.1, v2.4.1, v1.1.1, v1.2.1, v1.3.1,
  v1.3.2).
- Aborted releases: v0.20.0 and v0.20.1 tag the same commit (assets only on v0.20.1); v1.3.0 failed in the image-promote
  job (fixed by "fix: ignore the in-toto manifest when promoting the docker build", released as v1.3.1); v0.12.0 and
  v0.9.15 have no chart. These are the only anomalies found in 77 releases since v0.9.0.

## 2. Release notes

- No CHANGELOG file (there is a `changelog.json` config for a tool, not a log), no per-release docs in the repo.
- GitHub Release body = generated "What's Changed" list of PR titles plus three `Image:` lines (`release.yml`);
  `.github/release.yml` only groups General / Dependencies. **Only reachable through the GitHub API (blocked)**.
- Fallback that carries the same information: `git log vA..vB` subjects. History is squash-merged, so subjects are the PR
  titles, mostly conventional commits (`chore:` 1274, `fix:` 358, `feat:` 271, `docs:` 127 of ~2500 since v0.9.0).
  Breaking changes are almost never marked: only two `!:` subjects and one `BREAKING:` prefix in 2500 commits.
  Breaking changes (unserved `v1beta1`, removed stores) are visible only in the CRD / chart diffs and in a few
  `remove`/`unserve` subjects.
- Chart changes land in the operator commit range but the chart is cut 1-3 commits after the operator tag, so the range
  can show a chart feature that is reverted before the chart ships (v2.11.0: `feat(charts): add global.imageRegistry`
  #6983 was reverted by #6996 after the tag; helm-chart-2.11.0 does not have it). The computed values diff (read at chart
  tags) is authoritative.

## 3. Upgrade / migration docs

- There is **no per-release upgrade guide**. Topical guides only: `docs/guides/v1beta1.md` ("Upgrading CRD versions",
  v1alpha1 -> v1beta1, v0.5.0 era), `docs/guides/templating.md#migrating-from-v1`, `DEPRECATING.md` /
  `docs/introduction/deprecation-policy.md` (Kubernetes-style alpha/beta/GA policy; Helm chart, images and release
  process are explicitly out of scope).
- `stability-support.md` advises one-minor-at-a-time upgrades.

## 4. Compatibility

`docs/introduction/stability-support.md` (`RAW@main/docs/introduction/stability-support.md`): table
`| ESO Version | Kubernetes Version | Release Date | End of Life |`, newest first, 35 rows from 0.3.x to 2.11.

- Key cells: `2.11`, `2.4.1`, `2.4`, `0.20.x`, `1.0`. Kubernetes cells: `1.36`, `1.34-1.35`, `1.19 → 1.31`.
- **The row of a release is added late.** At tags v2.8.0, v2.9.0 and v2.10.0 the table has no row for the release itself
  (top row is the previous line); only v2.11.0 contains its own row. The page is therefore read from `main`.
- Policy text: only the newest minor is supported; older minors are "automatically deprecated" when the next appears.
  Release Date / End of Life columns carry the support window (not representable, see gaps).
- Chart `kubeVersion: ">= 1.19.0-0"` in Chart.yaml (constant), provider stability tables (alpha/beta/stable per provider)
  are on the same page (not modelled).

## 5. Helm chart

- Source `deploy/charts/external-secrets` (same repo). Released **after** the operator from branch
  `release-chart-X.Y.Z` (`chore: release helm chart for X.Y.Z`), tag `helm-chart-X.Y.Z` (chart-releaser,
  `CR_RELEASE_NAME_TEMPLATE: helm-chart-{{ .Version }}`).
- Chart version == operator version, appVersion = `vX.Y.Z` (verified against the gh-pages index: 133 entries, all
  `appVersion == "v" + version` except the very first 0.1.1). The chart lags the operator by 1-3 days (v2.11.0:
  tag 2026-09-18, chart 2026-09-21).
- **At the operator tag the chart directory still declares the previous chart** (`Chart.yaml` at v2.11.0 says
  `version: "2.10.0"`); chart values and metadata must be read at the chart tag.
- Chart tags missing for operator tags v0.12.0, v0.20.0, v1.3.0 (and v0.9.15, shipped as chart 0.9.15-2 with
  appVersion v0.9.15-2).
- Published to four places:
  1. `https://charts.external-secrets.io` (canonical in docs; **blocked**);
  2. `RAW@gh-pages/index.yaml` (chart-releaser index, 112 KB, 200; `urls` point to release assets);
  3. GitHub release asset `REL@helm-chart-X.Y.Z/external-secrets-X.Y.Z.tgz` (200);
  4. OCI `ghcr.io/external-secrets/charts/external-secrets:X.Y.Z` (tags from 0.9.14, cosign-signed; manifests reachable).
- Dependency: `bitwarden-sdk-server` (v0.6.0, `oci://ghcr.io/external-secrets/charts`, optional) - not modelled.
- Values: `image.repository: external-secrets/external-secrets`, `global.imageRegistry` (reverted before chart 2.11.0,
  default registry otherwise ghcr.io), `image.tag: ""` = chart appVersion, `image.flavour: ""|ubi|ubi-boringssl`.
  The controller, webhook and cert-controller all run the same image. `crds.unsafeServeV1Beta1`, `crds.conversion.enabled`
  document the API transition.

## 6. Container images

- **Only registry: `ghcr.io/external-secrets/external-secrets`** (tags `vX.Y.Z`, `vX.Y.Z-ubi`, `vX.Y.Z-ubi-boringssl`;
  all three flavours answer 200 for v0.9.20, v0.20.4, v1.3.2, v2.11.0 manifests). Docker Hub namespace
  `external-secrets` is empty (hub.docker.com API: 404/0 results); no quay.io image. Discovery proposed
  `docker.io/external-secrets/external-secrets` (from the chart's `repository:` value, ignoring `global.imageRegistry:
  ghcr.io`) - wrong.
- `pr-N`, `sha-*` and `helm-chart.*` tags exist for CI builds; not release artifacts.
- Provenance: SBOM `REL@vX.Y.Z/sbom.vX.Y.Z.spdx.json`, `provenance.vX.Y.Z.intoto.jsonl`,
  `provenance.vX.Y.Z-ubi.intoto.jsonl` (all 200 for v0.9.20 .. v2.10.0 sampled); images cosign-signed (not verified).

## 7. Release assets (operator)

- `REL@vX.Y.Z/external-secrets.yaml` = `make manifests` output (rendered chart with default values, includes CRDs,
  Deployments with `ghcr.io/external-secrets/external-secrets:vX.Y.Z`). Present for every release since v0.9.0
  except v0.20.0 and v1.3.0.
- `esoctl` (CLI) is released by a separate workflow (`release_esoctl.yml`, tag `vX.Y.Z-esoctl`, goreleaser archives
  `esoctl_<os>_<arch>.tar.gz`, `esoctl_checksums.txt`); `esoctl_checksums.txt` is 404 on operator releases.

## 8. CRDs

- `RAW@vX.Y.Z/deploy/crds/bundle.yaml` (generated single-file bundle, what non-Helm users apply; 200 at every sampled
  tag since v0.9.0) and `config/crds/bases/*.yaml` (25 CRD files at v2.11.0, one per CRD).
- API groups `external-secrets.io` (SecretStore, ClusterSecretStore, ExternalSecret, ClusterExternalSecret, PushSecret,
  ClusterPushSecret) and `generators.external-secrets.io` (19 generators at v2.11.0).
- **Version lifecycle** (read from `spec.versions[].served/storage` in `config/crds/bases`):
  v0.9.20: v1alpha1 + v1beta1 (storage v1beta1); v0.15.1 -> v0.16.0: v1alpha1 removed, v1 added and made storage;
  v0.16.2 -> v0.17.0: v1beta1 `served: false` (still in the CRD, "unserve v1beta1 and mark it as deprecated");
  v0.20.4 .. v2.11.0: v1 served + stored, v1beta1 unserved. The generic CRD diff reports all of these (removed /
  no longer served / new / storage version change) with evidence.
- v2.0.0 ("remove unmaintained secret stores") removed `spec.provider.alibaba` and `spec.provider.device42` from the v1
  SecretStore/ClusterSecretStore schemas: the only signal besides the major bump is the CRD diff.

## 9. Security

- `SECURITY.md`: report by email to the CNCF maintainers list; advisories on GitHub (GHSA) - API blocked here.
  `SECURITY_RESPONSE.md` describes the incident process. The repo mentions CVE/GHSA ids in commit subjects
  (`fix: update grpc for CVE-2026-33186`), which the generic classifier turns into security items.

## 10. Historical validation (ri check, 77 releases v0.9.0 .. v2.11.0)

Everything held except two upstream release failures (v0.20.0: no assets; v1.3.0: no image, no assets) and the missing
charts listed above; all are recorded as `exceptions`. See `docs/onboarding/checks/external-secrets.json`.

## UNVERIFIED / blocked

- GitHub release bodies and advisories (API); charts.external-secrets.io (covered by the gh-pages index and the OCI
  chart, which carry the same entries); cosign signatures; docs site rendering; whether chart-releaser re-publishes
  identical content to charts.external-secrets.io.
