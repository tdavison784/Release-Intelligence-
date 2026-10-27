# Security advisories and version intelligence

RI decides deterministically, from advisory evidence, which advisories an upgrade introduces, keeps or removes,
and which known releases would avoid them. No model is involved: the agent reads the result (its dossier's
`security` summary), it never produces it. Mission §4 A2, PLAN-M2 lane `advisories`.

## Sources

A product declares its advisory sources with role `security`. Two locator kinds read GitHub:

| kind | fields | what it reads |
|---|---|---|
| `github-advisories` | `repository: owner/name` | the repository's published security advisories (`GET /repos/{o}/{r}/security-advisories`) |
| `github-global-advisories` | `ecosystem`, `package` | the reviewed advisories of GitHub's global advisory database that affect one package (`GET /advisories?ecosystem=…&affects=…&type=reviewed`, every page) |

The repository source alone is not enough: many projects publish no repository advisories, so their advisories
exist only in the global database. An empty repository listing is an answer about the repository, not about the
product. Declare both where both apply; the repository source first.

Only the vulnerability entries whose package equals the locator's (ecosystem compared case-insensitively, name
exactly) contribute ranges and patched versions. An advisory whose entries all name other packages is dropped.
Withdrawn advisories are skipped. Each advisory keeps its source, its `publishedAt` and `updatedAt`, and one
`advisory` evidence record per copy (the advisory page, the digest of its JSON, the retrieval time).

### Choosing the package identity

The identity is catalog data in `products/*.yaml`, never Go logic. The rule:

1. **Ecosystem** is the GitHub advisory ecosystem of the shipped binary (`go` for a Go program).
2. **Package** is the canonical name of the binary's own package in that ecosystem. For Go, it is the module path
   of the main module, as `go.mod` declares it at the release tags (`k8s.io/ingress-nginx`, not the repository
   URL).
3. Declare it only when it is **unambiguous across the upgrades RI analyses**:
   - The module path must not change between majors. A `/vN` suffix (`github.com/argoproj/argo-cd/v3`) names a
     different package for every major, so one identity would miss the other majors' advisories.
   - The advisory ranges must be written in the product's own release numbers. Products released under date tags
     or edge/stable channels (minio, linkerd2) get only pseudo-version ranges, and products whose Go module is
     versioned apart from the product (actions-runner-controller) are left out.
4. Check the identity against the database (a read-only `GET /advisories?ecosystem=…&affects=…`) when adding it,
   and record what was checked in the source's `notes`.

## Reading ranges

`vulnerable_version_range` becomes a semver constraint (`internal/github/ranges.go`). Comma-separated comparisons
must all hold (`>= 1.0.0, < 1.17.2`), and several ranges of one advisory are united with `||`. The following
shapes are handled specially:

| shape | example | reading |
|---|---|---|
| explicit version list | `v1.18.0,v1.18.1,v1.19.0` | an OR of exact versions. Read as an intersection it would match nothing |
| prerelease lower bound | `>= 1.12.0-beta.0, < 1.12.1` | matches the stable 1.12.0, as well as later prereleases of 1.12.0 |
| two upper (or two lower) bounds | `<v1.12.14,<v1.15.4,<v1.16.2` | **not machine-readable**: the intersection is narrower than the author meant (per-line fixes written as one list) |
| exact versions mixed with comparisons | `1.2.0, < 1.3.0` | **not machine-readable** |
| Go pseudo-version bound | `< 0.0.0-20260319175635-5183b7d86137` | **not machine-readable**: the fix is a commit, not a release. Compared as semver it matches no release, but which releases contain the commit is unknown |

If any range of an advisory is not machine-readable, the advisory has no constraint. It is then **unassessed**: a
warning asks for a manual check, and no version can be cleared of it. A partial constraint could turn
"affected" into "not affected", so RI never uses one.

**Copies of one advisory from several sources** (the same GHSA id in the repository and the global database) are
merged (`internal/ingest/advisories.go`):

- If only one copy has a machine-readable range, that range is used, whichever source listed it first.
- If both are machine-readable and they differ, their union is used, so a version either copy calls affected
  stays affected.
- Evidence, aliases, patched versions and sources of both copies are kept. `sourceId` names the source whose
  range is used, and `sources` lists every source of the advisory.

## Rules on the edge

`internal/upgrade/advisories.go` matches every advisory with a constraint against the current version (From) and
the target (To):

| rule | condition | change |
|---|---|---|
| `advisory:fixed` | From affected, To not | informational security change ("Fixes …") |
| `advisory:affects-target` | both affected (persisting) | action required |
| `advisory:regression` | To affected, From not: the upgrade introduces the vulnerability | action required. Policy treats it at least as strictly as `affects-target` (`security-advisory-affects-target` invariant, docs/POLICY.md) |

## The security section (`UpgradeEdge.Security`)

`ri.dev/edge-security/v1alpha1`, additive on the edge (`internal/domain/edgesecurity.go`). It is carried into the
impact report (`ImpactReport.Security`) and summarised in the agent dossier.

- **`status`**:
  - `complete`: every security source was read.
  - `incomplete`: some source could not be read.
  - `unavailable`: no source was read, or none is declared.

  A source that could not be read (an error, throttling, a 404, an offline cache miss) becomes an edge warning and
  a `gaps` entry. The advisories it lists are unknown, not absent. An unread source is never an empty advisory
  list.
- **`regressions`, `persisting`, `fixed`**: the advisories by the table above. Each entry has its range, patched
  versions, sources, timestamps, evidence and the `changeId` of its edge change (that is how it joins the
  report's findings).
- **`unassessed`**: advisories without a machine-readable range, each also a gap.
- **`candidates`**: computed from every known stable release above From (the version listing), when the target
  is affected by an assessed advisory and at least one source was read:

  | kind | what it is |
  |---|---|
  | `target-line` | the lowest release on To's line that removes every regression, or the statement that none does |
  | `any-line` | the same on any line |
  | `clean` | the lowest release affected by no assessed advisory, or the statement that none is |

  "Removes every regression" means the candidate is affected by no assessed advisory that From is free of. So it
  removes To's regressions and introduces none of its own. It may still carry advisories that already affect
  From (listed in `leaves`).

  Each candidate also has:
  - `removes`: advisories affecting To that do not affect the candidate;
  - `leaves`: advisories affecting the candidate;
  - `unassessed`: advisories it cannot be cleared of;
  - `charts`: the chart versions that ship it, where the product ships as a chart.

  Every advisory reference cites the evidence of its range.

  Chart versions come from the definition's own helm-chart artifact relation, evaluated for the candidate
  (`Ingester.ChartVersions`; for a `lookup`, the chart index entry whose `appVersion` equals the release, which is
  also the evidence). A chart version that only follows a version template is stated in `chartGaps`, not cited.
  Recommending the lowest qualifying release is deliberate: newest is not assumed safest.

A candidate is evidence for a decision, not a decision. Moving to it is a different upgrade (From → candidate),
which needs its own analysis (mission §6 C2).

## Where it shows

- `ri upgrade` and `ri impact` text output: a "Security advisories" section when at least one source was read
  (the warnings state an unavailable one).
- The impact report: the section is version-based (the same for every environment on the upgrade). It adds no
  finding and changes no classification. Advisory changes keep their `impact:not-joined` UNKNOWN finding, whose
  `neededToDetermine` says that the range decides version applicability and that environment exposure stays
  undecided.
- The agent dossier: `security`, a bounded summary with evidence ids. It is absent when the edge has none.

## Offline evaluation

Recorded states carry only the API responses that were recorded. A global listing missing from an offline cache
is `unavailable`, so the section says so and computes no candidates. The commander's recording step captures the
responses (live GitHub calls belong to fixture capture and that step only).
