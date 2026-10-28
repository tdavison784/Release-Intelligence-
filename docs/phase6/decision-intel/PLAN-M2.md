# Milestone 2: behavioural and security evidence (Workstream A), plan

Mission §10 M2, §4 A1–A3. Development cases are scored by the commander; their expected outcomes are in `GATE-M2.md`,
which lanes never read. Code references are to `p6-decision-intel`.

## 1. Problem (from a code read)

**Security advisories (A2).**
- **Only one source is read.** `internal/github/advisories.go` calls only
  `GET /repos/{o}/{r}/security-advisories`. Many projects publish no repository advisories: their advisories exist
  only in GitHub's global database (`GET /advisories?ecosystem=…&affects=…`). The catalog has no field for a
  product's package identity (ecosystem and package name).
- **Some ranges cannot be read.** Some repository advisories state affected versions as an explicit list
  (`v1.2.0,v1.2.1,…`) rather than a range. `ConvertVersionRange` cannot read that, so `Vulnerable` becomes empty and
  the advisory degrades to a "check manually" warning. De-duplication by GHSA id keeps the first copy seen, even when
  another copy is machine-readable.
- **A security regression has no rule of its own.** `internal/upgrade/advisories.go` has `advisory:fixed` and
  `advisory:affects-target`. The case "the target is affected and the current version is not" is only a title word.
- **No safer release is computed.** `upgrade.Input` carries only the path up to the target, so releases above it
  (later patches on its line) are invisible. Chart versions are mapped app → chart only per ingested release.
- **Advisory changes reach the impact report only as unjoined `unknown` findings.**
- **Offline evaluation gets no advisory data.** Recorded states have no `api.github.com` responses. A cache miss
  silently means "no advisories", which is the missing-advisory-equals-none error the mission forbids.

**Behavioural evidence (A1, A3, B6).**
- **Evidence requests are never fulfilled.** `request_evidence` only records a request: "nothing is fetched during this
  session". So when release notes state a change of behaviour without its exact semantics (for example, a validation
  enabled by default), the agent has no way to read the implementation, docs or tests at the target tag. It falls
  back to model memory, which v9 forbids but cannot replace.
- **The fetchers already exist:**
  - `gitsrc.Files` reads one file at an immutable tag;
  - `gitsrc.Dirs` reads a directory at a tag, with caching and offline replay;
  - `request_validation` already adds run artifacts mid-session without touching the immutable bundle.

## 2. Expected behaviour (generic)

**Advisories.** For any upgrade current → target, RI deterministically reports:
1. **Regressions:** advisories affecting the target but not the current version.
2. **Persisting exposures:** advisories affecting both.
3. **Fixes:** advisories affecting the current version but not the target.
4. **Candidate versions, each with the evidence that establishes it:**
   - the lowest release on the target's line that removes every regression, or the statement that none exists;
   - the lowest release that removes every regression on any line;
   - the lowest release free of every known advisory.
5. **The matching chart version** for each candidate, where the product is shipped as a chart.

Every statement carries source provenance: the advisory id, which source it came from, its timestamp and the range
evidence. An advisory source that could not be read is reported as unavailable, never as "no advisories".

**Behaviour.** An investigation can retrieve, bounded and driven by its question, a file or a regex search over a
directory of the product's own source repositories at the **target's immutable tag**. Whatever it retrieves is
recorded as a cited run artifact with provenance. When retrieval is impossible (offline cache miss, not found, out of
bounds), the question becomes an explicit gap of the form "behaviour at <tag> unavailable: <reason>". It is never
answered from memory.

## 3. Lanes and minimal implementation

### Lane `advisories` (D-0008): no prompt change

| # | Change | Where |
|---|---|---|
| 1 | **Global advisory source:** locator kind `github-global-advisories` with `ecosystem` + `package` (additive `Locator` fields, validation); `Advisories.ListGlobal` calls `GET /advisories?ecosystem&affects` (paginated), filters `vulnerabilities[].package`, and reads `vulnerable_version_range` and `first_patched_version` | `internal/catalog/definition.go`, `validate.go`, `internal/github/advisories.go`, `github.go` |
| 2 | **Catalog data:** add the package identity to products whose ecosystem package is unambiguous. This is declarative YAML, not Go logic. Document the rule for choosing the package | `products/*.yaml` |
| 3 | **Ranges:** accept explicit version lists (`v1.2.0,v1.2.1` as an OR of exact versions). When copies of the same GHSA id disagree, prefer the machine-readable one and record both sources | `internal/github/ranges.go`, `internal/ingest/advisories.go` |
| 4 | **Regression rule** `advisory:regression` (target affected, current not), with action required; policy treats it at least as strictly as `affects-target`. The other rules are unchanged | `internal/upgrade/advisories.go`, `internal/policy/evaluate.go` |
| 5 | **Version intelligence:** `Input.Versions` (every known stable release) yields the candidate versions of §2 item 4, each with the advisories it removes and those it leaves. It sits in an additive `UpgradeEdge.Security`, which is schema-versioned | `internal/upgrade/api.go`, `advisories.go`, `internal/app/app.go`, `internal/domain/upgrade.go` |
| 6 | **Chart mapping:** for each candidate, look up the chart version through the existing app → chart lookup (`appVersion`) | `internal/ingest/artifacts.go`, `internal/helm/repo.go` |
| 7 | **Surface:** the impact report gets a security section (regressions, persisting, candidates), joined rather than `unknown`; the agent dossier gets the edge's security summary | `internal/impact/build.go`, `internal/investigate/agent/dossier.go` |
| 8 | **Unavailability:** a source that could not be read (including an offline cache miss) becomes a warning and a gap, never "no advisories" | `internal/ingest/advisories.go` |

### Lane `upstream` (D-0009): starts after lane `scoped` merges (prompt v11 builds on v10)

| # | Change | Where |
|---|---|---|
| 1 | **`request_evidence` fulfilment** for `kind: upstream-file` (`{repo, ref, path}` or a blob URL) and `kind: upstream-search` (`{repo, ref, dir, pattern}`). Allowed only on immutable refs (`IsImmutableRef`) and only in the product's own code repositories (`CodeReposOf`). Runs through `gitsrc.Files` and `gitsrc.Dirs` plus an in-memory regex | `internal/investigate/agent/tools_record.go`, `session.go`, `investigator.go` |
| 2 | **Recording:** an `upstream-document` run artifact (`OriginUpstream`, URI = blob URL at the tag, digest, producer `evidence-fetch@v1`); request `RequestFulfilled` with `Fulfilled`; normal verbatim citation. The bundle is never changed | as above, `run/pipeline.go` |
| 3 | **Unavailable:** `RequestUnavailable` with the reason, recorded by the tool layer in the assessment's `unknown` as "behaviour at <tag> unavailable: <reason>" | `agent/assessment.go` |
| 4 | **Bounds:** a per-item fetch budget (count and bytes) within `MaxReadBytes`; search result caps | `session.go`, prompt v11 `meta.json` |
| 5 | **Prompt v11** = v10 + the tool's new behaviour + "when upstream behaviour decides the outcome and the change text does not state it, fetch it at the target tag before concluding; cite it or record it unavailable". Generic wording | `prompts/investigate-agent/v11/` |
| 6 | **Reproducibility:** fetches go through the existing caches. Replay must return identical bytes; offline with no cache gives unavailable | tests |

## 4. Tests

- **`advisories`:**
  - an httptest global endpoint with pagination and package filtering;
  - explicit-version-list ranges;
  - de-duplication preferring the machine-readable copy;
  - regression versus persisting versus fixed on fixtures, including prerelease bounds (`>= X-beta.0, < Y`);
  - candidate computation: none on the target's line, one on another line, a fully clean one;
  - chart mapping;
  - offline cache miss reported as unavailable;
  - policy never weaker than today;
  - existing advisory tests unchanged.
- **`upstream`:**
  - file and search at a tag, with fixtures and caches;
  - a mutable ref or a foreign repository is refused;
  - fetch budget exhaustion;
  - fulfilled request → citable artifact;
  - unavailable → gap in `unknown`;
  - replay digests;
  - v1–v10 prompts frozen;
  - policy isolation.

## 5. Guardrails

- **Blinding:** lanes never read `GATE-M*.md`, `M*-RESULT.md`, any `expected.yaml`/`NOTES.md`/`observed/`, or run
  reports and scores (FLEET.md).
- **Generic code:** no product names in Go logic. Catalog YAML is data.
- **Network:** live GitHub calls only in the commander's recording step; lanes use fixtures. `GITHUB_TOKEN` comes from
  `gh auth token` and is never printed or committed.
- **Policy:** security changes may make classification stricter, never weaker.

## 6. Decision after M2

The commander writes `M2-RESULT.md` against GATE-M2. M3 (synthesis) starts on a met gate or by the product owner's
decision.
