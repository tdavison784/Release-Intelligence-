# ingress-nginx release-channel map (research date 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** registry.k8s.io (307 → Artifact Registry → 200), k8s.gcr.io, kubernetes.github.io (Helm index), kubernetes.io (retirement blog, official CVE feed), api.osv.dev and api.github.com (0 advisories) answer; `gcr.io/k8s-staging-ingress-nginx` still answers 401. **Correction:** `controller-chroot:v1.10.0` was never published (404 at the registry backend; tags jump from the 1.9 line to v1.10.1), so "controller-chroot since v1.2.0" has one exception, now in the definition. The original text below is kept as the record of 2026-10-01.

Scope: `github.com/kubernetes/ingress-nginx` (UP). One repository, two release trains (controller and Helm chart) and
a project that is **retired and archived**. "RAW" = `https://raw.githubusercontent.com/kubernetes/ingress-nginx/<ref>/<path>`
(HTTP 200 verified unless stated). Not to be confused with `nginx/nginx-ingress` (F5/NGINX Inc.'s `kubernetes-ingress`, a
different project that shows up in Docker Hub searches for "ingress-nginx").

Reachable from this sandbox: `git ls-remote`/clones of github.com, `raw.githubusercontent.com` (also other repos such as
`kubernetes/k8s.io`, `kubernetes-sigs/krew-index`, and the `gh-pages` branch), GitHub release asset downloads, Docker Hub API.
Blocked (proxy 403 / curl 000, not retried): `api.github.com`, `github.com` HTML pages, `registry.k8s.io`, `k8s.gcr.io`
(302 to registry.k8s.io), `kubernetes.github.io`, `kubernetes.io`, `quay.io`, `api.osv.dev`; `gcr.io/k8s-staging-ingress-nginx`
answers 401/DENIED anonymously. The repository's archived flag was read through the GitHub search tool of the session.

## 1. Canonical versions: two tag families in one repo

`git ls-remote --tags` → 275 tags: `controller-v*` 101, `helm-chart-*` 125, `ingress-nginx-*` 49 (legacy chart tags 2.x/3.x).

- **Product = controller.** Tag `controller-vX.Y.Z` (94 stable + 7 pre-releases `-alpha.N/-beta.N`, first `controller-v0.34.0`).
  `versioning.tagPrefix: controller-v` selects it; the other two families do not match and are ignored. Discovery picked
  `helm-chart-` instead (see below): it takes the most numerous prefix.
- **Chart = artifact.** Tag `helm-chart-A.B.C` (4.x), independent semver, **appVersion has no `v`** (`appVersion: 1.15.1`).
- Both trains are cut together: `controller-v1.15.1` and `helm-chart-4.15.1` point at the same commit (`0a5901f3c`, 2026-03-19);
  patch releases of up to three lines are released the same day (1.15.1 / 1.14.5 / 1.13.9, PR #14738).
- Lines are maintained in parallel on `release-1.13`, `release-1.14`, `release-1.15` (`lineage: minor`).
- Latest release: **controller-v1.15.1 / helm-chart-4.15.1, 2026-03-19**. `main` has no commit after 2026-03-20, `gh-pages`
  index `generated: 2026-03-19T21:16:17Z`, repository `pushed_at` 2026-03-23.

## 2. Lifecycle (retired)

- `README.md` on `main` opens with "# Ingress NGINX Retirement / ## Retiring": best-effort maintenance until March 2026,
  afterwards "no further releases, no bugfixes, and no updates to resolve any security vulnerabilities"; existing deployments
  and published charts/images keep working. Announcement: kubernetes.io blog 2025-11-11 (blocked, not fetched).
- The GitHub repository is **archived** (`archived: true`, search API, 2026-10-01): read-only, `main` is frozen. That makes
  every `ref: main` document stable.
- The README also warns "If you are not already using ingress-nginx, you should not be deploying it" (Gateway API successor).
- No existing construct could say this about the product as a whole (see the onboarding record): a `lifecycle` statement
  with an evidence source was added.

## 3. Release notes

- **`changelog/controller-X.Y.Z.md`** (RAW at `main`; also at the tag for ≥ 1.9.5): generated per release. Structure:
  `# Changelog` / `### controller-vX.Y.Z` / `Images:` (digest-pinned image refs) / `### All changes:` (PR titles) /
  `### Dependency updates:` / `**Full Changelog**` compare link. Breaking items are wrapped in warning signs with an indented
  explanation: `* ⚠️ Metrics: Disable by default. (#12153) ⚠️` (1.11.0 to 1.12.0 only; none in 1.13+). Files exist for
  1.6.4 … 1.15.1 except **1.8.4 and 1.8.5**; 1.6.4–1.9.4 exist at `main` only (back-filled), not at their tags.
  ~80% of the bullets are contributor mechanics (467 `Images:`, 219 `Go:`, 143 `Docs:`, 131 `CI:`, 110 `Tests:` over 1.6.4–1.15.1).
- `charts/ingress-nginx/changelog/helm-chart-A.B.C.md`: one line, mostly "Update Ingress-Nginx version controller-vX.Y.Z"
  plus the chart PRs, which the controller changelog repeats. Not used (redundant; its version needs a chart lookup).
- Root `Changelog.md` is a legacy pointer (1.5.1 and "All New change are in ./changelog").
- GitHub release bodies (API blocked) carry the same text; declared as the fallback for versions without a changelog file.

## 4. Upgrade docs

None per release. `docs/deploy/upgrade.md` (53 lines, same structure at v1.5.1, v1.11.0 and main) says "change the image tag" /
`helm upgrade --reuse-values`; the chart README has `## Upgrading Chart` with only the stable/nginx-ingress migration.
Breaking changes exist only as the warning-sign bullets above.

## 5. Compatibility

- README "Supported Versions table" (`main`, rows v1.3.1 … v1.15.1): columns `Supported | Ingress-NGINX version |
  k8s supported version | Alpine Version | Nginx Version | Helm Chart Version`. Version cells are `**v1.15.1**` / `v1.12.8`.
  Rows exist only for some patch releases: none for 1.7.0, 1.8.0, 1.8.1, 1.8.2, 1.8.5. The `Supported` column (a refresh
  emoji for 1.13.0–1.15.1) is the project's own support window; k8s lists are the e2e-tested versions
  (1.15.1: 1.35 … 1.31).
- Chart `Chart.yaml` `kubeVersion: '>=1.21.0-0'` (1.15.1), read at the chart tag.
- Alpine/NGINX versions per release are only in that table and in "Images: Bump NGINX/Alpine" bullets.

## 6. Helm chart

- Source `charts/ingress-nginx/` in the same repo; tag `helm-chart-A.B.C`; assets `ingress-nginx-A.B.C.tgz` on the GitHub
  release `helm-chart-A.B.C` (HTTP 200 for 4.15.1).
- **Helm repo index** `https://kubernetes.github.io/ingress-nginx/index.yaml` is blocked; the same file is
  `gh-pages/index.yaml` (RAW, 188 KB, 177 `ingress-nginx` entries, `urls:` = the release assets above).
- Version relation (verified against the index for all 94 stable controller tags): every controller tag has a chart whose
  `appVersion` equals the version, except `1.0.0-alpha.2`. 1:1 and `4.<minor>.<patch>` for ≥ 1.10.0; shifted for 1.9
  (1.9.0→4.8.0, 1.9.1→4.8.1, 1.9.3→4.8.2, 1.9.4→4.8.3, 1.9.5→4.9.0, 1.9.6→4.9.1); several charts per controller in 4.0.x
  (1.1.0 → 4.0.10–4.0.13). So: `lookup appVersion == {{.Version}}`, not a template.
- OCI: the k8s.io promotion list has `charts/ingress-nginx` with tags `v4.12.5 … v4.15.1` (note the `v`, unlike the chart version);
  `registry.k8s.io` is blocked and a channel cannot carry a tag template, so it is not declared.
- Chart `values.yaml` at `helm-chart-A.B.C` is the user-facing surface; `controller.image.tag/digest/digestChroot` and the
  certgen `tag/digest` are rewritten on every release (they duplicate the image diff).

## 7. Images and manifests

- `registry.k8s.io/ingress-nginx/controller:vX.Y.Z` (+ `@sha256:` in manifests) and `controller-chroot:vX.Y.Z` (since v1.2.0).
  Independent versions: `kube-webhook-certgen` (v1.6.9), `custom-error-pages`, NGINX base images, test images.
  Before the registry move the manifests name `k8s.gcr.io/ingress-nginx/controller` (≤ 1.2.1).
- No Docker Hub / ghcr / quay presence of the project (Docker Hub hits are third-party mirrors).
- **Authoritative mapping reachable without the registry**: `kubernetes/k8s.io`
  `registry.k8s.io/images/k8s-staging-ingress-nginx/images.yaml` (RAW, 56 KB) lists `controller` digest → tag
  (`sha256:594ceea7…: ["v1.15.1"]`, identical to the digest in `deploy/static/provider/cloud/deploy.yaml` and in the changelog).
  Not expressible as a channel of a container-image artifact (oci only), so not declared.
- **Static manifests** `deploy/static/provider/<p>/deploy.yaml` at the tag, generated from the chart: `cloud`, `baremetal`,
  `aws`, `aws/nlb-with-tls-termination` (≥ 1.1.2), `do`, `exoscale`, `kind`, `oracle` (≥ 1.8.1), `scw`. They reference the
  controller image and certgen image. **Anomaly**: on 8 tags (0.47.0, 0.51.0, 1.6.4, 1.7.1, 1.8.2, 1.8.5, 1.9.0, 1.9.3) the
  manifest at the tag still names the previous release's image (the bump commit came after the tag); consistent since 1.9.4.
- kubectl plugin: `.goreleaser.yaml` builds `kubectl-ingress-nginx_<os>_<arch>.tar.gz` but no release (controller-v* or
  helm-chart-*, probed 1.1.0 … 1.15.1) has the asset, and krew-index is at v0.31.0: not a current artifact.
- No CRDs of its own (uses Ingress / IngressClass).

## 8. Security

`SECURITY.md` points to `kubernetes-security-announce` and Kubernetes' disclosure page; supported versions "see Kubernetes
version skew policy" (no per-release statement). The changelogs name security fixes without ids ("Controller: Several security
fixes. (#13069)" in 1.11.5 and 1.12.1; "Security: Harden socket creation ..." in 1.14.0; only 1.8.0/1.9.3/1.9.5 mention CVE ids), so
advisories matter: GitHub security advisories (API blocked here, UNVERIFIED) and the Kubernetes official CVE feed (kubernetes.io,
blocked).

## 9. Reading guide for a definition

| Need | Where |
|---|---|
| versions | `git-tags` with prefix `controller-v` (github-releases first when the API is reachable) |
| notes | RAW `main/changelog/controller-{{.Version}}.md`, ≥ 1.6.4, minus 1.8.4/1.8.5 |
| compat | RAW `main/README.md` table, ≥ 1.3.1, minus 5 patch versions |
| chart | `gh-pages` index / `helm-git` over `helm-chart-*` tags, lookup `appVersion` |
| manifests | RAW at `controller-v<ver>`, minus 8 stale tags |
| lifecycle | README banner (frozen), `lifecycle:` statement |
