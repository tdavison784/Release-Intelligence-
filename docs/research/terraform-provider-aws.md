# Terraform AWS Provider release-channel map (verified 2026-10-01)

Scope: product `terraform-provider-aws` (github.com/hashicorp/terraform-provider-aws, "UP"). There is no
second repository: guides, changelog, registry manifest and release tooling all live in this one repo
(unlike Vault's vault-helm / web-unified-docs split).
Method: blobless clone (`--filter=blob:none`), `git show` at tags, GitHub REST API (anonymous, used
sparingly), releases.hashicorp.com range probes, registry.terraform.io `/v1` API, proxy.golang.org
`@v/*.info` probes. Snapshot: tags up to v6.67.0 (2026-09-30).

Abbreviations: `RAW` = https://raw.githubusercontent.com/hashicorp/terraform-provider-aws,
`REL` = https://releases.hashicorp.com/terraform-provider-aws, `REG` = https://registry.terraform.io/v1/providers/hashicorp/aws.

Reachable from the sandbox (all verified 2026-10-01): github.com (git + release-asset downloads through
the signed-redirect), raw.githubusercontent.com, api.github.com (anonymous 60/h), releases.hashicorp.com,
registry.terraform.io (`/versions`, `/…/download/{os}/{arch}`), proxy.golang.org, developer.hashicorp.com,
discuss.hashicorp.com. **No blocked host found for this product.**

## 0. What a definition has to know (the surprises)

1. **Extreme release velocity, tiny releases.** 518 stable tags in 9 years; the v5 era alone ran
   v5.0.0 (2023-05-25) → v5.100.0 (2025-06-12), ~100 minors in 2 years; v6.0.0 (2025-06-18) → v6.67.0
   (2026-09-30) with 4–9 days between releases. Only 49 patch releases exist in total: nearly every
   release is an X.Y.0. `lineage: minor` therefore walks exactly one step per release (the line's only
   release is its X.Y.0), which is the right `ri upgrade` ergonomics at this velocity.
2. **The changelog is complete AT THE TAG** (the opposite of Vault). CHANGELOG.md at tag vX.Y.Z contains
   release X.Y.Z's own section; it is never read from main. The file carries only the current major era
   (74 sections at v6.67.0) plus a `## Previous Releases` pointer, so reading at the tag is the only
   correct strategy anyway.
3. **The own section often says `(Unreleased)`.** v4.67.0, v5.100.0, v5.99.1 and the whole v5 era carry
   `## X.Y.Z (Unreleased)` at their own tag (the date is filled in later commits on main); v6-era tags
   (e.g. v6.67.0, dated) fill it before tagging. The heading selector must accept both.
4. **Two label dialects.** v4+ writes HashiCorp labels with a colon (`NOTES:`, `FEATURES:`,
   `ENHANCEMENTS:`, `BUG FIXES:`, `BREAKING CHANGES:`); v3 and earlier writes the same labels WITHOUT a
   colon (77 no-colon labels at v3.74.0; vocabulary there is exactly {BUG FIXES, ENHANCEMENTS, FEATURES,
   NOTES, BREAKING CHANGES, IMPROVEMENTS, INTERNAL}). One `labelParagraphs` pattern with an optional
   colon covers both.
5. **GitHub releases carry NO binaries — and for the v6 era, no assets at all.** The 15 per-OS/arch
   zips named in `.release/terraform-provider-aws-artifacts.hcl` are published to releases.hashicorp.com
   and indexed by registry.terraform.io. GitHub release assets are only `_manifest.json` + `_SHA256SUMS`,
   and only for v4.5.0 – v6.0.0; every release from v6.1.0 on has an empty asset list.
6. **The registry delists some releases.** registry.terraform.io `/versions` lists 506 of the 518 stable
   versions; 12 git tags are missing (3.26.2/.3/.5/.10/.16/.17/.18, 3.64.3, 5.71.0, 6.1.0, 6.57.0 …).
   Spot-checked v6.1.0: present on git, releases.hashicorp.com (dir 200, zip 206) and as a GitHub
   release, but 404 at the registry download endpoint and absent from its version list. Git tags are the
   only complete version source.
7. **One upgrade guide per major, in-repo, accumulating.** `website/docs/guides/version-N-upgrade.html.md`
   (N = 2…6); all five exist at v6 tags. The website extension changed at v5.17.0
   (`.html.md` → `.html.markdown`), splitting the v5 guide's availability window in two.
8. **Terraform Core compatibility is prose, not data.** The machine-readable compatibility fact is the
   provider **protocol version** (`terraform-registry-manifest.json`: `metadata.protocol_versions:
   ["5.0"]`; registry API: per-version protocols `4`, `4.0`, `4.0,5.0`, `5.0`), which says "Terraform
   0.12+" — there is no per-release Terraform Core matrix anywhere. The guides' Prerequisites sections
   ("first upgrade to the latest 5.x…") are prose.

## 1. Canonical versions

Source: `git ls-remote --tags` = 521 tag refs: **518 stable `vX.Y.Z`**, 3 prereleases
(`v6.0.0-beta1/2/3`), no junk (no submodule tags, no `+ent` train, no rebuilds). Strict pattern
`^v(?P<version>\d+\.\d+\.\d+)$` equals the stable set.

- Majors: v1.0.0 (2017-09-27), v2.0.0 (2019-02-27), v3.0.0 (2020-07-31), v4.0.0 (2022-02-10),
  v5.0.0 (2023-05-25), v6.0.0 (2025-06-18). Latest stable v6.67.0 (2026-09-30).
- Patch releases: 49 total (e.g. v5.99.1, v6.57.1); a line almost always has exactly its X.Y.0.
- Lineage `minor`: path 6.64.0 → 6.67.0 = {6.65.0, 6.66.0, 6.67.0}; 5.98.0 → 5.99.1 =
  {5.99.0, 5.99.1}; across a major boundary = {6.0.0} exactly.
- Mirrors: GitHub releases (API, rate-limited), `REG/versions` (509 entries incl. the 3 betas — and 12
  stable tags missing, see surprise 6), `REL/index.json` (HTTP 200, vault-style).

## 2. Release notes

### CHANGELOG.md (canonical, at the tag)
- `RAW/vX.Y.Z/CHANGELOG.md`: one `## X.Y.Z (Date)` section per release — dated, or `(Unreleased)` at
  the release's own tag in the v4/v5 eras (surprise 3). Content is COMPLETE at the tag.
- Labels (v4+): `NOTES:` / `FEATURES:` / `ENHANCEMENTS:` / `BUG FIXES:` / rare `BREAKING CHANGES:`
  (6 in the v6 file, 5 in v5.0.0's own section). v3-era: same labels without colon, plus
  `IMPROVEMENTS:` / `INTERNAL:`.
- Bullets: `* resource/aws_x: Change description ([#50189](…/issues/50189))` — one bullet per PR with
  an issue link. FEATURES uses bold prefixes (`**New Resource:**`, `**New Ephemeral Resource:**`,
  `**New List Resource:**`, `**New Data Source:**`, `**New Function:**`).
- Trimming: at v6.67.0 the file holds 74 v6 sections + `## Previous Releases`; at v5.0.0 it held the
  v5 section + all v4 sections. Per-release `.changes/<major-era>/X.Y.Z.md` snapshots and the
  `.changelog/*.txt` go-changelog entries (218 at v6.67.0, merged at release) are redundant with the
  CHANGELOG section.
- Old eras verified: `## 3.0.0 (July 31, 2020)` with a real BREAKING CHANGES block; v2.x/v1.x/0.1.0
  sections present at their tags.

### GitHub release bodies (same content, behind the API)
The release body for v6.67.0 is byte-for-byte the CHANGELOG section (headings included). Anonymous
api.github.com works (60/h) — a usable fallback, not the canonical channel.

### Docs-site notes
developer.hashicorp.com/terraform/docs/aws-provider/versioning etc. are rendered from this repo's
`website/docs/` (version-index.html.markdown). Not a separate source.

## 3. Upgrade guides (per major)

`website/docs/guides/version-{2,3,4,5,6}-upgrade.html.md` (≤ v5.16.0) / `.html.markdown` (≥ v5.17.0,
rename verified at v5.16.0→v5.17.0):

| Major | Availability (verified at tag) | Content shape |
|---|---|---|
| 2 | v2.0.0 (2019) | prose topics |
| 3 | v3.0.0 (2020) | prose topics |
| 4 | v4.0.0 (2022) | prose topics |
| 5 | v5.0.0 (2023-05, guide created pre-release) | `## Upgrade Guides` sub-sections per area; the v4→v5 boundary: removed provider arguments, `tags` behavioral changes, ~100 resource removals (EC2-Classic/Macie Classic), default_tags |
| 6 | v6.0.0 (2025-06) | H2 per topic: Removed Provider Arguments, Enhanced Region Support, deprecations (Elastic Transcoder, CloudWatch Evidently, S3 Global Endpoint), removals (OpsWorks, SimpleDB, Worklink), then `## Data Source …` / `## Resource …` per-asset sections |

Guides accumulate (all five exist at v6.67.0) and are attached only at major releases
(`releaseKinds: [major]`). Published rendered on developer.hashicorp.com and in the registry docs tab.
Dialect: terraform-docs markdown — front matter, `<!-- TOC -->` comment block with link bullets,
`->`/`~>`/`-> **Note:**` admonition paragraphs, `terraform` code fences (Before/After examples).

## 4. Compatibility

- **Protocol**: `terraform-registry-manifest.json` at every tag ≥ v4.6.0-release era (`version: 1`,
  `metadata.protocol_versions: ["5.0"]`); registry API per version: `4`/`4.0` (≤ 3.x), `4.0,5.0`
  (transition), `5.0` (modern). No extract type reads JSON → gap.
- **Terraform Core**: no machine-readable constraint exists; guides say "upgrade to latest 5.x first,
  run plan, expect no deprecation warnings". Recorded as a gap.
- **Pinned toolchain in go.mod** (machine-readable, per release, `version: pattern` candidates):
  - `github.com/hashicorp/terraform-plugin-sdk/v2`: v2.10.1 (v4.0.0) → v2.26.1 (v5.0.0) → v2.37.0
    (v6.0.0) → v2.40.1 (v6.67.0). v3.x used `terraform-plugin-sdk` v1 (different module); v2.x has no
    go.mod at all.
  - `github.com/hashicorp/terraform-plugin-framework`: first at v4.23.0 (v0.10.0), v1.2.0 (v5.0.0),
    v1.15.0 (v6.0.0), v1.19.0 (v6.67.0).
  - `github.com/aws/aws-sdk-go-v2`: v1.13.0 `// indirect` (v4.0.0) → v1.18.0 (v4.67.0) → v1.47.0
    (v6.67.0); v4.0.0 still had `aws-sdk-go` v1 direct.
  All three probe at proxy.golang.org `<module>/@v/v<version>.info` (200, JSON).

## 5. Artifacts

Declared set: `.release/terraform-provider-aws-artifacts.hcl` — 15 zips
(`terraform-provider-aws_${version}_{darwin,freebsd,linux,openbsd,windows}_{amd64,arm64,arm,386,s390x}.zip`).

| Artifact | Channels (verified) | Version rule | Window |
|---|---|---|---|
| provider zip | `REL/X.Y.Z/terraform-provider-aws_X.Y.Z_linux_amd64.zip` (206 for 1.0.0…6.67.0); registry download JSON points at the same bytes | == release | all eras |
| SHA256SUMS | `REL/X.Y.Z/terraform-provider-aws_X.Y.Z_SHA256SUMS` (206 everywhere) | == release | all eras |
| SHA256SUMS.72D7468F.sig | `REL/…_SHA256SUMS.72D7468F.sig` (206 for 1.0.0, 5.0.0, 6.67.0) | == release | all eras checked |
| registry manifest | `REL/…_X.Y.Z_manifest.json` (404 ≤ 4.5.0, 206 ≥ 4.6.0); also a GitHub release asset v4.5.0–v6.0.0 | == release | ≥ 4.6.0 |
| GitHub release assets | `github.com/…/releases/download/vX.Y.Z/…_SHA256SUMS` + `…_manifest.json` (asset counts: 0 ≤ v4.4.0, 2 for v4.5.0…v6.0.0, **0 for every v6.1.0+ release**) | == release | ≥ 4.5.0, < 6.1.0 |
| registry.terraform.io | `REG/X.Y.Z/download/{os}/{arch}` 200 JSON: filename, download_url (releases.hashicorp.com), shasums_url, signature url, GPG key 34365D9472D7468F | == release | 506 of 518 (12 delisted) |

No images, no CRDs, no charts, no OS packages: the artifact model is `binary` zips + `package`
manifests/Go-module pins, with `http` release-asset channels.

## 6. Security

- GitHub repository advisories (`/repos/hashicorp/terraform-provider-aws/security-advisories?
  state=published`): **0 published today**. Historical provider CVEs (e.g. CVE-2018-15857) predate the
  advisory database or were published outside it; there is no HCSEC-style bulletin category for
  providers (discuss.hashicorp.com reachable but not machine-readable).
- In-band: changelog `NOTES:` bullets carry security-relevant items (e.g. v3.0.0 "built using Go
  1.14.5, including security fixes"), and `sdk: Update` bullets carry AWS SDK security bumps.
- `.release/security-scan.hcl` configures release-time scanning, not an advisory feed.

## 7. Historical validation plan

Default `ri check` window (6 recent releases, all v6 patches-of-new-lines) plus an era run across the
v5 boundary (5.98.0, 5.99.1, 5.100.0, 6.0.0, 6.1.0, 6.2.0 …) to exercise: the guide windows (.html.md
vs .html.markdown), the manifest/asset windows, the SDK/framework pins, the `(Unreleased)` headings and
the registry-delist anomaly (6.1.0). sampleEdge: patch-level hop first (6.64.0 → 6.65.0/6.66.0), then a
slice at the v5→v6 boundary (5.100.0 → 6.0.0).

## UNVERIFIED / blocked list

None for this product. (api.github.com is quota-limited to 60/h anonymous — transient 403s expected under
burst, not a block; all binary/content channels avoided it.)

## Suggested declarative definition hints

- `versions`: `git-tags`, strict pattern, no tagPattern needed beyond versioning; `lineage: minor`.
- `release-notes`: `repo-file` at `{{.Tag}}` + `markdown-section` (`^{{regexQuote .Version}}(\s|$)`),
  `labelParagraphs: '^[A-Z][A-Z0-9 /&-]+:?$'`; classify: FEATURES/ENHANCEMENTS/IMPROVEMENTS → feature,
  BUG FIXES → bugfix, BREAKING CHANGES → breaking + action required, `^sdk: Update` → dependency;
  github-releases body as fallbackGroup twin.
- `upgrade-guide`: five majors × the 5.17.0 extension split = six sources, `releaseKinds: [major]`,
  availability scoped to each major; classify skips TOC link bullets, admonitions and terraform fences.
- artifacts: two zips + checksums + signature + registry manifest (windows!), GitHub release-asset
  checksums (window ≥ 4.5.0 < 6.1.0), and the three go.mod pins as `version: pattern` `package`
  artifacts probed at proxy.golang.org.
- `security`: github-advisories (empty today, still the standard feed).
