# Redis release-channel research (verified 2026-10-01)

> **Corrected 2026-10-02 after live re-run (docs/rerun/REPORT.md):** download.redis.io and api.github.com (47 advisories) answer; `git.redis.io` still does not connect (the GitHub mirror remains the declared source). The original text below is kept as the record of 2026-10-01.

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here (egress policy or inferred).
`RAW` = `https://raw.githubusercontent.com/redis/redis`. Canonical git is `git.redis.io/redis.git` (UNVERIFIED: host blocked, `000` on CONNECT); `github.com/redis/redis` is the mirror the definition uses (mirror semantics like PostgreSQL).

## 0. Executive summary (what a declarative definition MUST know)

1. **Not Kubernetes, plain semver.** Tags are bare `X.Y.Z` (no `v` prefix) plus `X.Y-rN`/`X.Y.Z-rN`/`X.Y-mNN` pre-release/milestone shapes. A line is `X.Y`: 8.10 is a minor line (GA 2026-07-29), patches are 8.10.1, 8.10.2. **Dual live majors**: 7.4 (latest 7.4.11, 2026-08-17) is maintained in parallel with the 8.x lines — 7.2 and 7.4 are five-year "extended" lines (EOL 2029-12-01), 8.0 is a "standard" line with published EOL 2026-12-01, 8.2 is extended (EOL 2030-09-01), 8.4/8.6/8.8/8.10 are standard with EOL "TBD" (= 6 months after the next minor).
2. **Release notes are in the repo**, one cumulative file per line: `00-RELEASENOTES` at the release's own tag, one setext-H1 section per release (`Redis 8.10.2    Released Thu 17 Sep 2026 18:00:00 IST` underlined with `====`). Subsections (`### Security fixes`, `### Bug fixes`, ...) are H3 in 8.x, setext-H2 in 5.x–7.x. Every section opens with an `Update urgency: `LEVEL`` line. The GitHub release body is the same section published by the release workflow (`.github/workflows/post-release-automation.yml`, `gh release upload`), and `redis/docs` mirrors it per line (`content/operate/oss_and_stack/stack-with-enterprise/release-notes/redisce/redisos-8.x-release-notes.md`, `redisce-7.4-...`); the docs copy is incomplete (no 7.2 or older), so the repo file is primary and the GitHub body the fallback.
3. **GA headings vary** — the X.Y.0 section is titled: `Redis 8.10 GA` / `Redis 8.8 GA  (v8.8.0)` / `8.2 GA (v8.2.0)` (no "Redis"!) / `Redis 8.6 GA (8.6.0)` / `Redis 7.2.0 GA` / `Redis Community Edition 7.4.0 GA`; **8.0.0 has no section at all** (its notes are the file intro; covered by the GitHub body fallback). Patch sections are uniform `Redis X.Y.Z … Released` back to 5.0 (`Redis 5.0.0     Released`, `Redis 4.0.0     Released`).
4. **License changed twice**: BSD-3 through 7.2; RSALv2 OR SSPLv1 from 7.4; RSALv2 OR SSPLv1 OR AGPLv3 from 8.0 (`LICENSE.txt` [200]). No technical effect; the 8.0 GA notes carry the "License change" bullets.
5. **Artifacts**: source tarballs `https://download.redis.io/releases/redis-X.Y.Z.tar.gz` (**reachable** [200], 285 tarballs 0.091 → 8.10.2, every 7.4/8.x release present); a `redis-full.tar.gz` **GitHub release asset** on 8.10.x only (>= 8.10.0); the Docker official image `docker.io/library/redis` (`X.Y.Z`, `X.Y.Z-trixie`, `X.Y.Z-alpine`; mirror.gcr.io fallback [200] for both default and `-alpine`, tags present from 5.0.0 on; `4.0.0` answers 404 on the mirror). Snap/brew/RPM/APT are official but per-distro packaging (`redis/redis-snap`, `redis/homebrew-redis`, `redis/redis-rpm`, `redis/redis-debian`) — recorded as a gap, like PostgreSQL's PGDG. **No upstream Helm chart, no CRDs, no manifests** (Bitnami's chart is third-party; `helm.redis.io` serves Redis Enterprise/Software products, not the server).

## 1. Canonical versions

`git ls-remote --tags https://github.com/redis/redis` [200]: 416 tags.

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `X.Y.Z` | 269 | `8.10.2`, `7.4.11` | stable releases |
| `X.Y-rN` / `X.Y.Z-rN` | 42 | `7.4-rc1`, `8.10-rc2` | pre-releases (beta/rc) |
| milestone | ~30 | `8.10-m03-int`, `8.8-m02`, `8.0-rc1-int2` | internal milestones |
| junk | 105 | `2.2-alpha0`, `2.6.7-1`, `2.2.105-scripting`, `3.0-alpha0` | ignored |

- Strict pattern (kept from discovery): `^(?P<version>\d+\.\d+\.\d+(?:-(?:beta\d+|rc\d+))?)$`. No tag prefix; lineage `minor` (each X.Y is a line, X.Y.0 is the minor/GA release).
- Latest stable: **8.10.2** (2026-09-17). Live lines on 2026-10-01: 8.10, 8.8, 8.6, 8.4, 8.2, 7.4 (7.2.11 last, 2025-10-03, still inside its extended window). The synchronized security release of 2026-09-17 shipped 8.10.2, 8.8.3, 8.6.7, 8.4.7 and 8.2.10 on the same day — and 7.4.10 on 2026-07-23.
- Release names in the notes use "GA"/"RC" (`Redis 8.10 GA`, `Redis 8.10-RC2 (v8.9.241)`); the internal RC versioning (`8.9.24x`) never appears as a git tag.

## 2. Release notes (the central source) [200]

`RAW/<tag>/00-RELEASENOTES`, cumulative per line, read **at the release's own tag**:

```
================================================================================
Redis 8.10.2    Released Thu 17 Sep 2026 18:00:00 IST     <- setext H1 (==== underline)
================================================================================

Update urgency: `SECURITY`: There are security fixes in the release.

### Security fixes                                        <- H3 (8.x) / setext H2 (<= 7.x)

- #15673 Commands queued in a transaction could ...       <- one bullet per change
```

- File preamble (title, "Upgrade urgency levels:" key, PR-repository key `#n`/`#Qn`/`#Jn`/`#Tn`/`#Pn`) sits before the first section and is excluded by section selection.
- Patch sections: uniform `Redis X.Y.Z    Released <date>` (spacing varies, 2–5 spaces) from 8.10.2 back to 4.0.0. GA sections: the six shapes above; 8.0.0 and 3.0.0 have no per-release section (intro prose instead).
- Subsection vocabulary: `Security fixes` / `Security and privacy fixes`, `Bug fixes (compared to X)`, `New Features (compared to X)`, `Major changes compared to X.Y`, `Headlines`, `Performance and resource utilization improvements`, `Modules API`, `Configuration parameters`, `Metrics`, `CLI tools`, `Binary distributions`, `Operating systems we test Redis X.Y on`, `Known bugs and limitations`. No `Upgrade notes` subsections and **no `<-- N >--` markers exist** in any current channel (checked 5.0–8.10 files and the docs mirror; that lead is stale).
- Module bundles: from 8.0 the notes cover the bundled modules too (Query Engine, JSON, TimeSeries, probabilistic, Vector Sets) with prefixed PR ids.
- GitHub release bodies [200 via api.github.com, gh CLI]: identical content (the per-release section without its header line), available from 6.2.0 on (4.0.0/5.0.0 have no GitHub release). Assets: only 8.10.x carries `redis-full.tar.gz`.
- `redis/docs` mirror [200]: `redisos-8.0…8.10-release-notes.md`, `redisce-7.4-release-notes.md`; headings `## Redis Open Source X.Y.Z (Month YYYY)`; no 7.2-or-older files, so it cannot be the primary channel. redis.io/releases is 404 (page retired); redis.io itself is reachable [200].

## 3. Security

- **GitHub advisories of redis/redis** [200, 47 advisories]: the machine-readable channel the `github-advisories` adapter reads; includes the 2023–2024 Lua/RCE ground-truth set (CVE-2023-41044/41056, CVE-2024-31227/31228/31449/46981, CVE-2025-21605/32023/46817-46819/48367/49844, CVE-2026-23479/23631/25243 …).
- `SECURITY.md` [200] (repo): "we generally backport security issues to a single previous major version" + a supported-versions table (8.6.x/8.4.x checked at 8.10.2; the docs-site table below is kept more current); vulnerabilities are reported per the Redis responsible-disclosure policy (redis.io page).
- No per-CVE advisory pages exist in redis/docs (no `bulletin` path in the 10k-file tree); the historical `SA-xxxxxx` bulletins are published as website posts, not machine-readable. Every security fix also appears in the release notes (`### Security fixes`, CVE ids inline, e.g. `(CVE-2026-62356)`).

## 4. Artifacts

| Artifact | Location | Version relation | Verified |
|---|---|---|---|
| source tarball | `https://download.redis.io/releases/redis-X.Y.Z.tar.gz` | `X.Y.Z` exactly | [200] HEAD (8.10.2: 21.6 MB, 7.4.11, 5.0.14); directory listing [200]: every 7.4/8.x release, 285 files back to 0.091 |
| full source asset | `https://github.com/redis/redis/releases/download/X.Y.Z/redis-full.tar.gz` | `X.Y.Z`; exists only on 8.10.0/8.10.1/8.10.2 (no assets on 8.8.0 and earlier, checked 7.0.15–8.8.0) | [200] via gh api asset listing |
| Docker official image | `docker.io/library/redis:X.Y.Z` (+ `X.Y.Z-trixie`, `X.Y.Z-alpine`, `X.Y.Z-alpine3.2x`) | `X.Y.Z` exactly | hub.docker.com tag API [200] (1196 tags, live lines only); mirror.gcr.io/library/redis [200] for 5.0.0–8.10.2 default and `-alpine` (4.0.0: 404) |
| Snap | `snapcraft.io/redis` (source: `redis/redis-snap`) | snap revisions, not semver | [200] page; not modelled (gap) |
| brew / RPM / APT | `redis/homebrew-redis`, `redis/redis-rpm`, `redis/redis-debian` | upstream + packaging revision | UNVERIFIED (not modelled, gap) |
| Helm chart / CRDs / manifests | none upstream | | Bitnami `redis` chart is third-party (non-goal, like PostgreSQL's PGDG); `helm.redis.io` is Redis Enterprise/Software, not the server |

## 5. Compatibility and lifecycle

- **Modules API**: `src/redismodule.h` `#define REDISMODULE_APIVER_1 1` — the constant is `1` at 7.2.11, 7.4.11 and 8.10.2 alike (the API surface grows, the declared version does not). NOT a usable per-release compatibility constraint; the machine-readable signal a module depends on is the module's own `RedisModule_Init` version, which lives in third-party modules. Recorded as a gap for the compatibility role.
- **Support matrix**: `redis/docs@main content/operate/oss_and_stack/install/version-mgmt.md` [200] — markdown table (Version / Release type / Status / EOL Date): 8.10/8.8/8.6/8.4 standard TBD; **8.2 extended EOL 2030-09-01; 8.0 standard EOL 2026-12-01; 7.4 extended EOL 2029-12-01; 7.2 extended EOL 2029-12-01; 6.2 extended EOL 2027-04-01**. Model policy: standard lines get fixes for 6 months after the next minor; extended lines 5 years. Cited by the definition's lifecycle statements (its items are reported through them).
- Versioning/scheme docs: `version-mgmt.md` (above) and `SECURITY.md`'s table. No machine-readable upgrade-source compatibility (nothing like pg_upgrade's range); upgrade procedures (docs `install/upgrade/{standalone,cluster}.md`) are version-generic, not per-release — no per-version upgrade-guide source exists (gap; the GA notes' "Major changes compared to X.Y" sections are the per-version summary).

## 6. Hosts

Reachable: github.com (git), raw.githubusercontent.com, api.github.com (anonymous 60/h today; discovery's 6 validation calls and a handful of research calls succeeded; the `gh` CLI is authenticated), download.redis.io, hub.docker.com (tag API), registry-1.docker.io via mirror.gcr.io, redis.io (200; /releases retired → 404), snapcraft.io. Blocked/000: git.redis.io (canonical git host; GitHub mirror used). Rate-limited as usual: registry-1.docker.io anonymous pulls (mirror.gcr.io declared as second channel).
