# Go toolchain release-channel research (verified 2026-10-01)

Legend: [200] = fetched with HTTP 200 / successful git operation in this sandbox. UNVERIFIED = could not be checked here (egress policy or inferred).
`RAW` = `https://raw.githubusercontent.com/golang/go`. go.googlesource.com is the canonical git host (UNVERIFIED: not needed, the GitHub mirror carried every ref used here); `github.com/golang/go` is the mirror the definition uses.

## 0. Executive summary (what a declarative definition MUST know)

1. **Two eras of tag scheme, one pattern.** Minor releases were tagged `go1.N` through **go1.20** (go1.20 IS 1.20.0); from **go1.21.0** on the first release of a line is tagged `go1.N.0` — upstream's own statement (go1.21 release notes): *"Go 1.21 introduces a small change to the numbering of releases. In the past, we used Go 1.N to refer to both the overall Go language version and release family as well as the first release in that family. Starting in Go 1.21, the first release is now Go 1.N.0... tools like `go version` will report `go1.21.0`."* One component-group pattern `^go(?P<major>1)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?$` (missing patch = 0, the PostgreSQL construct) covers both eras: go1.20 → 1.20.0, go1.21.0 → 1.21.0. 496 tags total (a fresh `git clone --filter=blob:none` lists 496, not the feared 2500+): 269 three-part stable, 20 two-part minors (go1..go1.20), 99 `weekly.*` snapshots, 65 rc, 29 alpha/beta, plus strays (`go1`, `go1.9.2rc2`).
2. **Release notes left the repository at go1.21 — and are HTML only.** Through go1.20 each minor tag carries `doc/go1.N.html` in-tree (an HTML file, still stamped "DRAFT RELEASE NOTES ... not yet released" — the in-tree copy is never finalized). From 1.21 the notes exist only as `doc/next/*.md` **markdown on master during development** (emptied by a "doc/next: delete" commit at each cut, so the files exist at no tag) and as the published **HTML page** `https://go.dev/doc/go1.N` [200]. There are **no GitHub releases at all** (`/releases/latest` 404, `/releases` empty list — the "release body mirrors the HTML" hypothesis is false). HTML is not readable by the docbook/rst/adoc extractors → new generic `format:html` (see §3).
3. **Patch releases have notes in two channels.** `https://go.dev/doc/devel/release` [200] is a release history with one `h2` per minor (`go1.26.0 (released 2026-02-10)`) and one `h3` **per patch** (`go1.26.1 (released 2026-03-05) includes security fixes to the crypto/x509, html/template, net/url, and os packages`), covering every release from `go1` to `go1.27.1`. And the release branch git log between two patch tags is the curated backport list (15 subjects for go1.27.0→go1.27.1, `[release-branch.go1.27] database/sql: fix ...`), readable with the existing `git-log` locator (argo-cd's construct).
4. **Two-minor support policy, verbatim and reachable.** go.dev/doc/devel/release [200]: *"Each major Go release is supported until there are two newer major releases"* (SECURITY.md: "We support the past two Go releases"). Today: 1.27 (1.27.0 2026-08-19, 1.27.1 2026-09-01) and 1.26 (1.26.0 2026-02-10, latest 1.26.8) are supported; **< 1.26.0 is end-of-life since 1.27.0 (2026-08-19)**. Represented as lifecycle statements citing the policy page (traefik/redis precedent).
5. **Artifacts are URL-templatable on the tag.** `https://go.dev/dl/?mode=json` [200] is a machine-readable manifest (version, stable, files[] with kind/os/arch/sha256/size); `?include=all` covers 365 entries back to `go1` with reduced per-era platform sets. The download URLs behind it are `https://dl.google.com/go/{{.Tag}}.src.tar.gz` etc. [206 on range GET — reachable]; the tag IS the published version string in both eras. **storage.googleapis.com/golang/ answers 403** from this sandbox (the raw bucket; dl.google.com works). Docker official image `docker.io/library/golang` with **three-part tags in both eras** (`1.20.0` exists for the go1.20 minor; oldest exact tag 1.2.0): docker.io answered 200 anonymous (token + manifest) today, while **mirror.gcr.io lags the newest patch** (`1.27.1` and bare `1.27` absent, `1.27.1-alpine` present) — the inverse of the usual Hub-429 situation, recorded in the record.
6. **GOTOOLCHAIN era (1.21+)**: the `go` command auto-downloads and switches toolchains per `go`/`toolchain` directives in go.mod — a compatibility/behavioral dimension of upgrades (noted in classify: "now requires Go 1.N" items are compatibility statements, and the toolchain section of the notes is where behavior changes land). `gopls` and `dlc`/`gofix` tooling are separate products (noted, not modeled).

## 1. Canonical versions

`git clone --filter=blob:none` of github.com/golang/go [200]: 496 tags (discovery saw the same 496 and finished in 8.3 s; the "~2500+ tags" lead is wrong for the tag namespace — there are 58 release-branch refs on the side).

| Shape | Count | Example | Meaning |
|---|---|---|---|
| `go1.N.M` | 269 | `go1.27.1`, `go1.20.14` | stable (incl. 7 era-B minors `go1.21.0`..`go1.27.0`) |
| `go1.N` | 20 | `go1.20`, `go1.5` | era-A minors = `1.N.0` |
| `go1.NrcM` / `go1.NbetaM` / `go1.NalphaM` | 94 | `go1.27rc3`, `go1.10beta2` | pre-releases of a minor |
| `go1.N.MrcM` | 2 | `go1.9.2rc2` | pre-releases of a patch (rc attaches after the patch) |
| `weekly.*` | 99 | `weekly.2023.01.12` | pre-1.0-era snapshots |
| strays | ~12 | `go1`, `go1.4-bootstrap-*` | Go 1.0, bootstrap source snapshots |

- Pattern (component groups, PostgreSQL's construct): `^go(?P<major>1)\.(?P<minor>\d+)(?:\.(?P<patch>\d+))?$`; missing patch = 0. Lineage `minor` (each 1.N is a line; 1.N.0 the minor). 289 stable releases go1.2.2 → go1.27.1 (go1.NrcM and the 2 patch rcs excluded: the rc suffix attaches after the minor **or** after the patch, and one named `(?P<prerelease>)` group cannot sit in both alternation positions — recorded as a gap, not bent around).
- Release dates (release-history page [200]): go1.27.0 2026-08-19, go1.27.1 2026-09-01; go1.26.0 2026-02-10, go1.26.8 latest of its line; cadence ~monthly patches, minors each Feb/Aug.
- `go1` (Go 1.0) does not match the pattern (`go1\.` requires the dot): its dl entry lists only `go1.4-bootstrap-*` files; out of scope.

## 2. Release notes

| Channel | Era | Shape | Reachable |
|---|---|---|---|
| `doc/go1.N.html` at the tag | minors ≤ 1.20 | HTML, one file per line at the release tag; still carries the "DRAFT RELEASE NOTES" banner in-tree; **go1.19 deleted the file at the final tag** (present at go1.19rc2, absent at go1.19 and on release-branch.go1.19 — the doc/next era began mid-1.19) | RAW [404 probes on `doc/go1.24.html` first suggested worse; cat-file at tags is authoritative] |
| `https://go.dev/doc/go1.N` | all minors (1.1–1.27) | the published HTML: `h2` Introduction/Language/Ports/Tools/Runtime/Compiler/Linker/Standard library; per-package `h3`/`h4`; era-A pages use `dl/dt/dd` (44 dd in go1.20), era-B pages are headings + `p` + occasional `ul` (0 dd in 1.21/1.26/1.27) | [200] 64 KB per page |
| `doc/next/*.md` on master | next release only | the markdown sources (`1-intro.md`, `2-language.md`, ..., `7-ports.md`); emptied at each cut ("doc/next: delete" commits), so they exist **at no tag** — git archaeology, not addressable by any locator | in-repo [verified via cat-file] |
| GitHub releases | none | golang/go publishes no releases (empty list, `/releases/latest` 404) | [200 for the API] |
| `https://go.dev/doc/devel/release` | **every release** go1 → go1.27.1 | release history: `h2` per minor with date (`go1.26.0 (released 2026-02-10)`), then a shared `h3` "Minor revisions" under which each patch is a **paragraph addressed only by its `<p id="go1.26.1">` anchor**: `go1.26.1 (released 2026-03-05) includes security fixes to the crypto/x509, ... packages` — the per-patch security summaries are real but no locator construct selects them (markdown-section matches headings, of which there is one per MINOR line, not per patch) | [200] 147 KB |
| `git log` on the release branch | patches | curated backport subjects, `[release-branch.go1.N] area: summary` (15 commits go1.27.0→go1.27.1) | git [200] |

- Chosen model: minors read `go.dev/doc/go1.N` (canonical, mutable page — digest pins the read) with the in-tree `doc/go1.N.html` at the tag as the ref-stable era-A fallback (exceptions: 1.19.0; skip rule for the draft banner); patches read the release-branch git log (`{{.PrevTag}}..{{.Tag}}`), the only addressable per-patch channel. All HTML channels need the new `format:html` extraction.
- Upgrade guidance: there is no per-version migration guide (the Go 1 compatibility promise is the policy: "We expect almost all Go programs to continue to compile and run as before" — go1.21 notes); the notes' "Introduction" carries the compatibility statement per minor, and port/tool removals land under "Ports". `go fix` (1.27) automates migrations noted in the notes.

## 3. The HTML problem and `format:html`

The notes channels of §2 are HTML everywhere the content exists (era A in-tree at the tag, era B only on go.dev). The existing format slot (docbook/rst/adoc, all line-preserving markdown renderers in `internal/normalize`) gets a fourth member: `html` — `normalize.HTMLToMarkdown` renders `h1`..`h6` → `#`-headings (written back onto the heading's opening line), `li` → `- ` bullets (nested indent), `dt` → `**term**` lines, `dd` → own indented blocks, `pre` → 4-space indent, `code` → `` ` ``, `a href` → `[text](url)`, entities decoded, comments and `head`/`style`/`script` dropped, all other tags stripped; every other input line maps to its output line so evidence line ranges are original. Cross-product justification: Tomcat (per-line `changelog.html` only), Kafka (per-release "Notable changes" only on the downloads page), Maven (`docs/history.html`), Mozilla/Firefox release notes — all HTML-only, all well-known; the postgresql record's format:docbook justification already names the slot as the place for "the other non-markdown note formats".

## 4. Security

- SECURITY.md (repo, master): "We support the past two Go releases" + reporting policy at go.dev/security/policy.
- `github-advisories golang/go` [200, API]: **0 published repository advisories** — the Go project publishes to the Go vulnerability database `https://vuln.go.dev` [200, JSON API] and announces on the golang-announce list (groups.google.com [200] but JS-rendered; no adapter reads either — recorded as gaps). Toolchain security fixes surface deterministically through the release-history patch sections ("includes security fixes to ...") — the security classification rides that source.
- CVE-2023-39325-class stdlib/toolchain CVEs have `fixed` versions in the vuln DB that map onto patch releases; not modeled (no advisory adapter), noted as gap.

## 5. Support policy and lifecycle

- Policy (go.dev/doc/devel/release [200]): "Each major Go release is supported until there are two newer major releases. For example, Go 1.5 was supported until the Go 1.7 release..." Critical problems (incl. security) are fixed in the two newest majors only.
- Statements declared: `< 1.26.0` end-of-life (since 2026-08, when go1.27.0 landed; the last 1.25.x was go1.25.14), citing the policy page as evidence source; the page's per-release dates (go1.26.0 2026-02-10) corroborate. No "deprecated" state for 1.26 (upstream has no such language; it is simply the older supported line).

## 6. Artifacts

| Artifact | URL / registry | Version relation | Window | Verified |
|---|---|---|---|---|
| source tarball | `https://dl.google.com/go/{{.Tag}}.src.tar.gz` (+`.sha256`) | tag = filename version | go1.2.2 → | [206 range GET] |
| linux-amd64 archive | `.../{{.Tag}}.linux-amd64.tar.gz` | same | go1.2.2 → | [206] |
| darwin-arm64 archive | `.../{{.Tag}}.darwin-arm64.tar.gz` | same | **>= 1.16.0** (M1 era) | [206 for 1.27.1] |
| windows-amd64 msi | `.../{{.Tag}}.windows-amd64.msi` (+`.zip` twin) | same | go1.2.2 → | [206] |
| image | `docker.io/library/golang:{{.Version}}` (+ mirror.gcr.io fallback) | three-part tags in both eras (`1.20.0` exists; oldest exact 1.2.0) | 1.2.0 → | docker.io [200 anonymous token+manifest for 1.27.1]; mirror.gcr.io [200 tags/list but **lags**: `1.27.1`/`1.27` absent while `1.27.1-alpine` is present] |
| `-alpine` image | same, `{{.Version}}-alpine` | same | present for current lines | [200 via mirror list] |

- `https://go.dev/dl/?mode=json` [200] is a machine-readable manifest (files, sha256, size, kind per version; `include=all` = 365 entries) — evidence/research channel; not declarable as a version index (no adapter) and not needed: URLs template on `{{.Tag}}` exactly.
- Era windows from the dl JSON: platform set 46 files in the 1.21+ era, 19 in the 1.20 era, 15–17 pre-1.10; darwin-arm64 from go1.16; `.msi`/`.zip` windows twins back to go1.2.2.
- storage.googleapis.com/golang/ (the canonical object bucket behind dl.google.com) answers **403** from this sandbox [recorded unreachable]; dl.google.com serves the same objects.

## 7. Related products, out of scope

`golang.org/x/...` modules (x/net, x/sys, ...: independent versioning, module proxy), `gopls`, `dlc`, `go fix` tooling, the `golang.org/toolchain` module (what GOTOOLCHAIN downloads — same versions, different artifact class), distro packages (homebrew `go`, apt `golang-1.27`). Noted, not modeled.

## 8. Hosts

| Host | Verdict |
|---|---|
| go.dev (docs, dl JSON) | reachable [200] |
| dl.google.com (tarballs/msi/pkg) | reachable [206 range] |
| storage.googleapis.com/golang/ | **403** (unreachable; dl.google.com equivalent used) |
| github.com / raw.githubusercontent.com / api.github.com | reachable (API quota-limited; releases endpoints empty, advisories 0) |
| docker.io (registry-1 + auth.docker.io) | **200 anonymous today** (token + manifest); Hub rate limits are the historical norm — mirror declared |
| mirror.gcr.io | reachable but **stale for the newest patch** (1.27.1 absent 2026-10-01) |
| groups.google.com (golang-announce) | reachable [200], no adapter (JS-rendered) |
| vuln.go.dev | reachable [200], no adapter |
| proxy.golang.org | reachable [200] (not needed: stdlib has no module list) |
