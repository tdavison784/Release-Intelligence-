# Traefik Proxy release-channel map (verified 2026-10-01)

Scope: product `traefik` = Traefik Proxy, github.com/traefik/traefik ("REPO").
NOT in scope: Traefik Hub / Traefik Enterprise (the `hub-manager` chart in the
chart repository, ghcr.io/traefik/traefik-hub, traefik.io products).
Method: blobless clone (581 tags), raw.githubusercontent.com, api.github.com
(anonymous, rate-limited to 60/h on the shared egress IP), release-asset HEAD
probes (`-L` through objects.githubusercontent.com), registry tag lists
(anonymous token for ghcr.io), `curl` HEAD/GET. Snapshots: tags up to v3.7.13
(2026-09-04), master, chart repository tags up to v41.6.1.

Reachable: github.com (git + release downloads), raw.githubusercontent.com,
codeload, api.github.com (anonymous until the hourly quota), ghcr.io (anonymous
token + tags/list with Link pagination), mirror.gcr.io (200 but only what it
has cached), traefik.github.io (chart index), doc.traefik.io (rendered docs of
the same in-repo content).
Blocked / degraded: registry-1.docker.io (anonymous 401/rate-limited; the
canonical Docker Hub tags could not be enumerated from the sandbox),
mirror.gcr.io/traefik/traefik (200 with an EMPTY list — the mirror does not
carry the non-library repository).

## 0. What a definition has to know (the surprises)

1. **Two live majors for 2.5 years.** v3.0.0 (2024-04-29) did not end v2: the
   v2.11 line kept receiving patches in parallel, interleaved in the tag list,
   through v2.11.57 (2026-09-04) — released the same day as v3.7.13. The
   support policy page (in-repo `docs/content/deprecation/releases.md`) says
   the last minor of a major is security-supported for 2 years: v2.11 security
   support **ended 2026-09-07**, so all of v2 is end-of-life as of today.
2. **Canceled releases are real tags.** 15 stable tags (v2.4.4 ... v3.7.2)
   plus v2.0.3, v2.1.5, v1.7.27 have a tag and a CHANGELOG section that says
   only "Release canceled.", no release assets and no image tag.
3. **The changelog is one cumulative in-repo file, present at every tag**,
   v1.0.0-rc2 (2016) to v3.7.13, newest first, one `## [vX.Y.Z](url) (date)`
   section per release (407 sections at master; dual-major sections
   interleaved). Group labels are **bold paragraphs**, not headings and not
   vault-style uppercase paragraphs.
4. **GitHub release bodies exist for v2 and v3** (atom feed confirms releases
   for v2.11.5x) and are assembled from the changelog section plus a
   `**CVE fixed:**` GHSA list and a migration-guide pointer — but
   api.github.com rate limits make it a fallback, not the primary channel.
5. **Migration documentation has three layers** (section 3): a v2→v3 major
   guide, a v1→v2 major guide, and per-minor within-major guides
   (`v2.md`, `v3.md`) whose heading dialects changed twice. The docs
   directory `docs/content/migration/` was renamed to `docs/content/migrate/`
   at v3.5.1.
6. **The published image is the Docker official image**: goreleaser builds no
   images; `script/deploy.sh` bumps `traefik/traefik-library-image` (which
   builds docker.io/library/traefik) and a daily workflow mirrors
   `crane ls traefik` → ghcr.io/traefik/traefik. docker.io/traefik/traefik is
   the legacy containous repository (excluded by discovery as test-only).
7. **The Helm chart lives in a separate repository with independent versions**
   (github.com/traefik/traefik-helm-chart, chart under `traefik/`, tags
   `vX.Y.Z`, now v41.6.1) and its `appVersion` (with `v` prefix) covers only
   63 of 311 stable releases.
8. **CRDs**: ten provider CRDs, API group `traefik.containo.us` (v2) renamed to
   `traefik.io` (v3; v2.11.x ships both). Combined install file
   `kubernetes-crd-definition-v1.yml` from v2.5.0 (v2.0-v2.4:
   `kubernetes-crd-definition.yml`, v1beta1 API). No `TraefikProxy` CRD exists
   in the OSS repository (that name belongs to the Traefik Hub product line;
   the chart repository's `hub-manager` chart is a separate product).

## 1. Canonical versions

`git ls-remote --tags` = 581 tags: 311 stable `vX.Y.Z`, 180 prereleases
(`-alpha\d+` ×8, `-beta.\d+` ×71 + `-beta\d+` ×6, `-rc.\d+` ×3 + `-rc\d+` ×92,
`-ea.\d+` ×3 — "early access" since 3.7.0), 87 junk (v1.0, the 2015-2016
`v1.0.alpha.<sha|N>` family, ...).
- Regex: `^v(?P<version>\d+\.\d+\.\d+(?:-(?:alpha\d+|beta\.\d+|beta\d+|rc\.\d+|rc\d+|ea\.\d+))?)$`
  (discovery's, plus `ea.\d+`). Prereleases are parsed but stay out of the
  canonical list (`includePrereleases` unset).
- Latest stable v3.7.13 (2026-09-04); v2 line ends at v2.11.57 (2026-09-04);
  v1 line ends at v1.7.34 (2021-12-10).
- GitHub Releases exist for both majors (releases.atom lists v2.11.57,
  v2.11.56, ... next to v3.7.x) with 20 assets each; the release workflow's
  `gh release create --notes ${VERSION}` is evidently followed by a body
  update (bodies carry the changelog section + GHSAs).
- Lineage: `minor` (X.Y.z lines; the release workflow triggers on `v*.*.*`).

## 2. Release notes (CHANGELOG.md, canonical)

- `CHANGELOG.md` at every stable tag (and master); own section present at
  every tag checked (v1.0.0, v1.7.34, v2.0.0, v2.5.0, v3.0.0, v3.5.0,
  v3.7.13). Unlike vault, the file at the tag IS current — read at the
  default ref `{{.Tag}}`.
- Section anatomy: `## [v3.7.13](tree-url) (2026-09-04)`, an
  `[All Commits](compare-url)` link line, then **bold label paragraphs**
  with bullets `- **[area]** Description ([#PR](url) @author)`:
  `**Bug fixes:**` ×339, `**Documentation:**` ×267, `**Misc:**` ×126,
  `**Enhancements:**` ×93, `**Merged pull requests:**` ×18, `**Closed
  issues:**` ×14, `**Fixed bugs:**` ×13, `**Implemented enhancements:**` ×10,
  `**Enhancement:**` ×2, `**Important:**` ×1, `**CVE's fixed:**` ×1 (counts
  over the whole file; v1-era sections use the GitHub-generated labels).
  No `**Security fixes:**` label exists — security fixes are ordinary bug-fix
  bullets, and the CVE linkage lives in GitHub release bodies / GHSA
  advisories.
- Upstream defect: the v3.7.0 heading is `##  [v3.7.0]...` (two spaces;
  harmless for an ATX parser).
- `labelParagraphs: '^\*\*[A-Za-z][^*]*:\*\*$'` promotes the labels; the
  generic heading rows then classify Bug fixes → bugfix, Enhancements →
  feature, Misc/Documentation → other, CVE's fixed → security. Dependency
  bumps ("Bump github.com/x to vY") need a product rule (vault-style).
- GitHub release bodies (fallback channel): identical bullets plus
  `**Important:** Please read the migration guide (doc.traefik.io .../migrate/v3/#v3713)`
  and `**CVE fixed:**` GHSA bullets. Verified for v3.7.13, v3.7.12, v3.7.9,
  v3.6.25, v3.0.0, v2.11.57.

## 3. Migration / upgrade guides (in-repo `docs/content/`)

doc.traefik.io serves the same content (reachable, but in-repo raw is
preferred). The directory was `migration/` until v3.5.0 and `migrate/` from
v3.5.1 on.

| Guide | Path (at tag) | Applies to | Availability |
|---|---|---|---|
| v1 → v2 | `docs/content/migration/v1-to-v2.md` | 2.0.0 | every tag v2.0.0 → master |
| v2 → v3 | `docs/content/migration/v2-to-v3.md` | 3.0.0 | every tag v3.0.0 → master (also `v2-to-v3-details.md`, config detail companion, from v3.3.x) |
| within v2 | `docs/content/migration/v2.md` | every v2 minor 2.1-2.11 | file exists from v2.2.0's tag; **the own-minor section is added AFTER the minor ships** (at v2.4.0's tag the file stops at "v2.2 to v2.3"), so it must be read at the final v2 tag (v2.11.57), where all sections exist |
| within v3 | `docs/content/{migration,v3.5.0|migrate,≥v3.5.1}/v3.md` | every v3 minor ≥ 3.1.0 | file exists from v3.1.0's tag; own-minor section present at the own tag (3.1-3.4 as `## v3.X-1 to v3.X`, from 3.5.0 on as `## v3.X.0`); patch sections exist only for some patches |

Heading dialects of the per-minor guides: `## v3.0 to v3.1` (3.1-3.4),
`## v3.5.0` (3.5+), plus historical patch sections `## v3.1.0 to v3.1.1`,
`## v2.2.2 to v2.2.5`, and in the final v2.md short forms `## v2.8`, `## v2.9`,
`## v2.10`, `## v2.11` and patch sections `## v2.11.N`. No `## v2.7` / `## v2.6
to v2.7` section exists anywhere in the final v2.md (2.7.0 has no within-v2
guide). Upstream defect: inside `v3.md` some sub-topics are written as `##`
(the same level as the version sections, e.g. `## Dynamic configuration` under
v3.7.13), so those sub-items fall out of the extracted version section.
`nginx-to-traefik.md` is an ingress-nginx adoption guide, not release-specific.

## 4. Compatibility / support lifecycle

- `docs/content/deprecation/releases.md` (master; added 2026, PR #13627): the
  support table ("3.7 Active Yes Yes ... 2.11 Ended Apr 29 2025 / Ends Sept 07
  2026") and the rules "every minor supported 6 months; the last minor after a
  new major supported 2 years (starting with v3)". This is the evidence for
  the v2 end-of-life statement.
- `docs/content/deprecation/features.md`: a 3-row feature-deprecation table
  (networking.k8s.io/v1beta1, apiextensions.k8s.io/v1beta1 removed in 3.0;
  underscoreHeadersStrategy deprecated 3.7.12). Not modelled (its content is
  a subset of the migration guides).
- No Kubernetes version matrix page exists. The chart's `Chart.yaml`
  `kubeVersion: '>=1.25.0-0'` (v41.x) is the only machine-readable floor, and
  only for chart-shipping releases (chart-metadata content).

## 5. Helm chart (separate repository)

- `github.com/traefik/traefik-helm-chart`, chart directory `traefik/`
  (Chart.yaml, values.yaml), repository tags `vX.Y.Z` = chart versions
  (397 tags, latest v41.6.1; discovery's `^traefik-...` tagPattern matched 0
  of 397 — wrong). The `hub-manager` chart in the same repository is the
  Traefik Hub product, out of scope.
- Published to `https://traefik.github.io/charts` (index.yaml reachable, 350
  entries of chart `traefik`).
- Relationship: `appVersion: v3.7.13` (v-prefixed) == the product tag; lookup
  `appVersion == {{.Tag}}`, `select: latest`, `optional: true` — only 63
  distinct stable appVersions exist (48 v3, 15 v2); e.g. no chart ships
  appVersion v2.11.57, v3.5.6 or v3.6.25 (argo-cd / vault precedent).
- values.yaml notes: `image.registry`/`repository` are nullable and default
  at render time (`docker.io` + `traefik`, or ghcr.io + traefik/traefik-hub
  when `hub.enabled`) — source-tree defaults differ from rendered defaults.

## 6. Images and release assets

- **Image** (the one artifact): `docker.io/library/traefik` (Docker official
  image, built through github.com/traefik/traefik-library-image; deploy.sh in
  the release workflow), tags `vX.Y.Z` (+ floating `v3.7`, `v3`, `latest`).
  Daily mirror: `ghcr.io/traefik/traefik` (2605 tags incl. every stable tag
  v1.0.0 → v3.7.13 except 20, see below). `mirror.gcr.io/library/traefik`
  has 309 tags (a partial cache; no v2.0.0). `mirror.gcr.io/traefik/traefik`
  is empty. docker.io itself was not enumerable from the sandbox (401).
  ghcr.io missing stable tags (20): 15 canceled releases (v2.4.4, v2.4.10,
  v2.6.4, v2.6.5, v2.8.6, v2.9.2, v2.9.3, v2.11.23, v2.11.39, v2.11.47,
  v3.5.5, v3.6.3, v3.6.18, v3.7.2 + unpublished-no-mark v2.0.3, v2.1.5),
  v1.7.27, and three published releases whose image never reached the mirror
  (v2.2.3, v2.10.2, v3.4.2 — v3.4.2 is a documentation-only release).
- **Binaries**: goreleaser archives on the GitHub release:
  `traefik_vX.Y.Z_<os>_<arch>.tar.gz` (17 os/arch; windows zip),
  `traefik_vX.Y.Z_checksums.txt`, `traefik-vX.Y.Z.src.tar.gz` (20 assets per
  release, verified 200 from v2.0.0 on; v1.x used different asset names —
  v1.7.34's `traefik_linux_amd64` is 404). Assets are absent for exactly the
  17 unpublished/canceled versions above (v2.2.3, v2.10.2, v3.4.2 have them).
- goreleaser config is generated from `.goreleaser.yml.tmpl` by
  `go run ./internal/release <os-arch>`; `release: disable: true` (the
  workflow runs `gh release create` itself), no docker builds.

## 7. CRDs

- `docs/content/reference/dynamic-configuration/`: combined install file
  `kubernetes-crd-definition-v1.yml` (v3: 10 `traefik.io` CRDs; v2.11: 9
  `traefik.io` + 9 `traefik.containo.us`; the migrate/v3.md guide tells users
  to `kubectl apply -f` its raw URL) and per-CRD files
  `traefik.io_*.yaml` / `traefik.containo.us_*.yaml` (IngressRoute,
  IngressRouteTCP, IngressRouteUDP, Middleware, MiddlewareTCP,
  ServersTransport, ServersTransportTCP (v3 only), TLSOption, TLSStore,
  TraefikService). Present from v2.5.0; v2.0-v2.4 shipped
  `kubernetes-crd-definition.yml` (apiextensions v1beta1) instead.
- Gateway API CRDs are third-party (integration fixtures pin experimental
  v1.5.1/v1.6.1) — excluded, as discovery did.

## 8. Security

- SECURITY.md: report via GitHub Security Advisories → `github-advisories`
  (api.github.com; the feed lists traefik/traefik GHSAs such as
  GHSA-qqjf-53cj-pwvv, fixed in v3.7.13/v2.11.57). Reachable while the
  anonymous quota lasts.
- GitHub release bodies carry `**CVE fixed:**` GHSA lists per release
  (duplicates the advisory feed; only a fallback here).
- No separate machine-readable bulletin channel (traefik.io/blog is rendered).

## 9. Historical validation summary (see docs/onboarding/checks/traefik.json)

- Canceled releases (tag + changelog section, no assets, no image):
  v2.0.3, v2.1.5, v2.4.4, v2.4.10, v2.6.4, v2.6.5, v2.8.6, v2.9.2, v2.9.3,
  v2.10.2*(assets exist), v2.11.23, v2.11.39, v2.11.47, v3.5.5, v3.6.3,
  v3.6.18, v3.7.2 (asterisk: not canceled — published, image mirror gap).
  v3.6.26 exists as a changelog section ("Release canceled.") but not as a
  tag at all.

## UNVERIFIED / blocked list

- registry-1.docker.io (canonical library/traefik tag list; anonymous 401 —
  image presence on Docker Hub itself is inferred from the daily ghcr sync).
- api.github.com beyond 60 requests/hour (shared IP); all GitHub-API-dependent
  verification was done through github.com HTML/atom, raw and HEAD probes.

## Suggested declarative definition hints

- `versions`: git-tags (strict pattern incl. `-ea.\d+`); github-releases as
  priority-1 fallback.
- `release-notes`: CHANGELOG.md at `{{.Tag}}`, `markdown-section` +
  `labelParagraphs: '^\*\*[A-Za-z][^*]*:\*\*$'`; product rule: `Bump ... to`
  → dependency. GitHub release body as fallbackGroup alternative.
- `upgrade-guide`: v1-to-v2 and v2-to-v3 whole-document sources
  (releaseKinds [major]); `v3.md` split by availability at the 3.5.1 dir
  rename, `releaseKinds [minor]`, heading `^v({{.Major}}\.{{.PrevMinor}} to
  {{.Major}}\.{{.Minor}}|{{regexQuote .Version}})$`; `v2.md` read at the
  final v2 tag, heading `^v2\.(?:{{.PrevMinor}} to )?{{.Minor}}$`,
  exception 2.7.0.
- artifacts: image (library/traefik → ghcr → mirror.gcr, exceptions = the 20
  ghcr gaps), CLI tarball (>= 2.0.0, exceptions = the 17 unpublished), CRDs
  (>= 2.5.0, combined file channel + per-CRD content glob `traefik.*.yaml`),
  chart (lookup appVersion, optional, helm-repo + helm-git at `traefik/`).
- lifecycle: v2 end-of-life statement citing the support-policy source
  (`deprecation/releases.md` on master).
