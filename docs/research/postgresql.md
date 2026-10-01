# PostgreSQL release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here (egress policy or inferred).
`RAW` = `https://raw.githubusercontent.com/postgres/postgres` (GitHub mirror of the canonical `git.postgresql.org` repository).

## 0. Executive summary (what a declarative definition MUST know)

1. **Not semver, not Kubernetes.** Since 10 a version is `MAJOR.MINOR` (17.2 is the 2nd minor release of major 17); before 10 it was `MAJOR.MINOR.PATCH` (9.6.24, where 9.6 is the major). A *major* is the release line; minors are bug-fix/security-only and never need a dump/restore. Tags: `REL_17_2`, `REL9_6_24`, pre-releases `REL_17_BETA1`, `REL_17_RC1`, `REL9_6_BETA3`; branches `REL_17_STABLE`, `REL9_6_STABLE`.
2. Latest stable today: **18.6** (2026-08-13); 17.11, 16.15, 15.19, 14.24 (same day); 19 is in beta (`REL_19_BETA4`, branch `REL_19_STABLE`) and `master` is the 20 development line. **18.5 was never released** (tags and release notes jump 18.4 -> 18.6; Docker Hub has no `18.5`).
3. Release notes are DocBook **SGML** in the source tree, one `<sect1>` per release, and are the single source for release notes, upgrade guidance and CVEs (section 2). Evidence of the topology needing a construct: markdown section selection finds nothing in them.
4. Support policy: five years per major (UNVERIFIED on postgresql.org, which is blocked). Confirmed indirectly by release dates: 13 (2020-09-24) got its last tag 13.23 on 2025-11-13; 12 (2019-10-03) ended 12.22 on 2024-11-21; 11 ended 11.22 (2023-11-09); 10 ended 10.23 (2022-11-10); 14 (2021-09-30) is still maintained (14.24, 2026-08-13).
5. Artifacts upstream: source tarballs on `ftp.postgresql.org` (blocked) and the Docker official image `docker.io/library/postgres`. **No Helm chart, no CRDs, no manifests upstream** (the popular charts and operators belong to other projects: Bitnami, CloudNativePG, Zalando, Crunchy).

## 1. Canonical versions

`git ls-remote --tags https://github.com/postgres/postgres` [200]: 693 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `REL_N_M` | 173 | `REL_17_2` | N >= 10, M = minor release (17.2); `REL_10_0` is the first release of 10 |
| `RELN_M_P` | 371 | `REL9_6_24` | 7.x-9.x three-part versions |
| `REL_N_BETAn` / `REL_N_RCn` | 35 / 10 | `REL_17_RC1` | pre-releases of majors >= 10 |
| `RELN_M_BETAn/RCn/ALPHAn` | 43 / 16 / 12 | `REL9_6_BETA3` | pre-releases of 9.x and older |
| `RELN_M` | 10 | `REL7_4`, `REL6_5` | **two-part tags of ancient first releases (<= 7.4)**: `REL7_4` is 7.4.0 while `REL_17_2` is 17.2, so one pattern cannot read both; out of scope |
| junk | 23 | `PG95-1_01`, `Release_2_0_0`, `release-6-3`, `REL8_3_0BETA1` | ignored |

- **Mapping used** (new construct, see below): `REL_17_2` -> `17.0.2` (tag `major=17, patch=2`), `REL9_6_24` -> `9.6.24`, `REL_17_RC1` -> `17.0.0-RC1`. With it every major is a line `N.0` (`10.0`, `17.0`, `18.0`; `9.6`), the first release of a major is `N.0.0` (a "major" release kind), 9.6.0 is a "minor" kind, everything else is a "patch". `PrevTag`/`PrevLine` then do the right thing: 17.0 follows 16.0, 10.0 follows 9.6.0.
- Result: 473 releases in 21 lines (8.0 ... 18.0) from 693 tags (120 non-matching, 100 pre-releases, 0 duplicates).
- Branches [200] (`git ls-remote --heads`): `REL_10_STABLE` ... `REL_19_STABLE`, `REL9_0_STABLE` ... `REL9_6_STABLE`, `master` (= 20).
- Release dates (from the notes, `Release date:`): 18 2025-09-25; 17 2024-09-26; 16 2023-09-14; 15 2022-10-13; 14 2021-09-30; minors are released in Feb/May/Aug/Nov (second Thursday), e.g. 17.11 and 18.6 on 2026-08-13; out-of-cycle releases exist (18.3 on 2026-02-26 after 18.2 on 2026-02-12).
- No GitHub Release objects (UNVERIFIED: api.github.com blocked); the project does not use them.

## 2. Release notes (the central source) [200]

`RAW/<tag>/doc/src/sgml/release-NN.sgml` (`release-9.6.sgml` and older: `release-MAJOR.MINOR.sgml`; ancient 7.x use `release.sgml`). The file is cumulative per major and is read **at the release's own tag** (`REL_17_2` has 17.2 at the top); on the stable branch it also gets the next minors.

```
 <sect1 id="release-17-2">            one per release, newest first; id = release-17-2, release-9-6-24, release-17
  <title>Release 17.2</title>          "Release 17" for 17.0, "Release 9.6" for 9.6.0
  <formalpara><title>Release date:</title><para>2024-11-21</para></formalpara>
  <sect2 id="release-17-2-migration"><title>Migration to Version 17.2</title>   prose: "A dump/restore is not required ...", REINDEX hints
  <sect2 id="release-17-2-changes"><title>Changes</title> <itemizedlist><listitem><para>... (Author)
        <ulink url="&commit_baseurl;3b0a0c2d5">&sect;</ulink></para> ...
```

- A major (`release-17`): `Overview` (highlights list), **`Migration to Version 17`** (opens with "A dump/restore using pg_dumpall or use of pg_upgrade ... is required", then an itemized list of the incompatibilities: 11 in 18, one `<listitem>` each), `Changes` (sect3 `Server`, `Utility Commands`, ... with sect4 children, one `<listitem>` per change), `Acknowledgments` (names).
- A minor: `Migration to Version 17.11` (prose; names required actions: "you may need to reindex indexes made with contrib/btree_gist", "see 17.6 if upgrading from an older version") and `Changes` (a flat list of fixes; **CVEs appear inline as "(CVE-2026-6471)"**, 41 of the 169 items between 17.9 and 17.11 cite one).
- Comments before each item carry commit metadata (dropped); `&sect;` links are per-item commit links (dropped).
- Old files (9.x, 8.x) use SGML shorthand (`</>` ends the innermost element, `<xref linkend="x">` has no end tag) and still parse; verified for 8.4.22, 9.4.26, 9.5.25, 9.6.0, 9.6.24.
- The heading to select: `Release 17.2` / `Release 17` (>= 10), `Release 9.6.24` / `Release 9.6` (< 10). The "Migration to Version X" heading is unique per release.
- Public HTML renderings (`postgresql.org/docs/release/17.2/`) are UNVERIFIED (blocked). The in-repo SGML is the machine-readable equivalent.

Other doc pages (not release specific, not ingested): `doc/src/sgml/ref/pgupgrade.sgml` ("pg_upgrade supports upgrades from 9.2.X and later" [200]), the "Upgrading a PostgreSQL Cluster" chapter (`runtime.sgml`/`backup.sgml`), `release.sgml` (index; includes `&release-20;`).

## 3. Security

- Canonical: `https://www.postgresql.org/support/security/` (per-CVE pages, affected/fixed versions) and the pgsql-announce list. UNVERIFIED: postgresql.org is blocked. The project is its own CNA.
- GitHub mirror: `.github/SECURITY.md` [200] only says "see https://www.postgresql.org/support/security/". `api.github.com` advisories are blocked (and the mirror is not expected to publish any), so discovery's `github-advisories` proposal was rejected.
- What works offline: every CVE of a minor release is cited in the "Changes" list of that release's notes; classified `security` with CVE references.

## 4. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| source tarball | `https://ftp.postgresql.org/pub/source/v17.2/postgresql-17.2.tar.bz2` (+ .gz, .xz, .sha256) | `17.2` (>= 10), `9.6.24` (before) | UNVERIFIED: ftp.postgresql.org blocked; `github.com/postgres/postgres/archive/refs/tags/REL_17_2.tar.gz` and codeload answer 403 here |
| Docker official image | `docker.io/library/postgres:17.2` (also `17.2-bookworm`, `17.2-trixie`, `17.2-alpine`) | same string as the tarball; `17.0` for the first release | [200] manifests via registry-1.docker.io (anonymous token; **429** when the shared egress address is rate limited), `mirror.gcr.io/library/postgres` [200] is a reachable alternative; hub.docker.com tag API [200] |
| Debian/RPM packages (PGDG) | `apt.postgresql.org`, `yum.postgresql.org` | upstream version + packaging revision (`17.11-1.pgdg13+2`) | UNVERIFIED (blocked); not modelled |
| Helm chart / CRDs / manifests | none upstream | | |

Docker anomalies found while validating (all [200]/404 on hub.docker.com): the default (Debian) tags of `10.22`, `10.23`, `11.19`-`11.22` were removed (their `-alpine` tags remain); `-alpine` exists from 9.6.1 (`9.6.0-alpine`, `9.5.0-alpine` absent; only some 9.4/9.5 patch releases have one); `18.5` was never published. `docker-library/postgres` `versions.json` [200] lists the maintained majors with Debian/Alpine suites and exact package versions (`18.6-1.pgdg13+2`), a possible cross-check not modelled.

## 5. Compatibility

No machine-readable matrix. Support status (supported/EOL per major, five-year policy) is on `postgresql.org/support/versioning/` (blocked). `pg_upgrade` source-version range is prose. The Docker image's base OS (`debian: trixie`, `alpine: 3.24`) is in `versions.json` but is not a product compatibility statement.

## 6. Hosts

Reachable: github.com (git), raw.githubusercontent.com, registry-1.docker.io + auth.docker.io (GET), hub.docker.com API, mirror.gcr.io, gitlab.com. Blocked (000/403): ftp.postgresql.org, www.postgresql.org, git.postgresql.org, apt/download.postgresql.org, github archive/codeload tarballs, api.github.com, endoflife.date, api.osv.dev, Debian mirrors.
