# Lane `advisories` status (D-0008, Milestone 2 A2)

Branch `p6di/advisories` from `p6-decision-intel` @ 44f33a8f. Brief: `briefs/advisories.md`; plan: `PLAN-M2.md` §3
"Lane `advisories`" changes 1–8, tests in §4. **State: done**, ready for the commander to merge.

## Done (PLAN-M2 changes → commits)

| # | Change | Commit | Tests |
|---|---|---|---|
| 1 | `github-global-advisories` locator kind (`ecosystem`, `package`, validated); `Advisories.ListGlobal` pages `GET /advisories?ecosystem&affects&type=reviewed`, keeps only the located package's entries | c75b61bc | `internal/github/global_advisories_test.go` (httptest, cursor pagination, package filtering, offline replay, offline miss = unavailable, throttling), `internal/catalog/globaladvisories_test.go` |
| 2 | Package identity for 10 products (cert-manager, cilium, external-secrets, grafana, ingress-nginx, istio, kyverno, prometheus-operator, vault, terraform-provider-aws), each module path read from the repo's `go.mod`; rule documented | 28906840 | `ri validate`: 28/28 valid |
| 3 | Ranges: explicit lists as an OR of exact versions; ranges that are not one interval (two upper or lower bounds, exact+comparison mix) and Go pseudo-version bounds are not machine-readable (→ unassessed). Prerelease lower bounds match the stable release. Cross-source de-dup merges copies (machine-readable wins; both readable and different → union; all evidence, aliases and sources kept, `Advisory.Sources`) | c75b61bc, c91737c9 | `TestConvertVersionRangeListsAndBounds`, `TestListGlobal`, `internal/ingest/advisories_test.go` |
| 4 | `advisory:regression` (target affected, current not, action required); `affects-target` keeps the persisting case; policy `isAdvisoryAffecting` covers both (additive hunk) | d1f96bc7 | `internal/policy/regression_test.go`: same tier and invariant as affects-target under the default and an auto-pass-everything policy, with or without findings, routine or not |
| 5 | `UpgradeEdge.Security` (`ri.dev/edge-security/v1alpha1`): regressions, persisting, fixed and unassessed advisories, status, gaps; candidates `target-line` / `any-line` / `clean` from `Input.Versions`, each with removes / leaves / unassessed and evidence | d1f96bc7 | `internal/upgrade/security_test.go` |
| 6 | Chart mapping: `Ingester.ChartVersions` evaluates the definition's helm-chart relation for a candidate (lookup → index entry as evidence); `app.Upgrade` passes `vl.Versions` and the lookups (`upgrade.CandidateVersions`) | c87bcac7 | `TestChartVersions`, `TestSecurityCandidateCharts` |
| 7 | `ImpactReport.Security` (copied, evidence cited, joined to findings by `changeId`); advisory findings keep their class, and their `neededToDetermine` names the security section; text section in `ri upgrade`/`ri impact`; `Dossier.Security` summary (absent when the edge has none) | c87bcac7, 7448930e | `internal/impact/advisory_security_test.go`, `internal/investigate/agent/dossier_security_test.go` |
| 8 | An unread source (error, throttling, 404, offline miss) is a non-ok status, an edge warning and a `security.gaps` entry. Status is `incomplete` / `unavailable`, and no candidates are computed with nothing read | c91737c9, d1f96bc7 | `TestSecurityUnavailableSources`, `TestAdvisoriesUnreadableSource`, `TestListGlobalOfflineReplayAndMiss` |

Docs: `docs/SECURITY_ADVISORIES.md` (new: sources, identity rule, range reading, merge, rules, section, candidates),
`docs/ARCHITECTURE.md`, `docs/POLICY.md`, `docs/IMPACT.md` (9b3d144d). No CLI flag changed, so the README is unchanged.

## Decisions (with why)

- **"Removes every regression" = affected by no assessed advisory the current version is free of.** This is
  stronger than "free of the target's regressions": a candidate must not introduce its own regressions. It may
  keep advisories the current version already has (listed in `leaves`).
- **Candidates are the lowest qualifying release above the current version**, never "the newest". They are only
  computed when the target is affected by an assessed advisory and at least one source was read.
- **Pseudo-version bounds are not machine-readable.** The brief requires that they match no ordinary release.
  They also must not read as "no release affected" (false-safe), so the advisory is unassessed: a warning plus a
  gap, and no candidate can be cleared of it.
- **Ranges with two upper bounds are rejected.** A real repository advisory states per-line fixes as
  `<v1.12.14,<v1.15.4,<v1.16.2`. Read as an intersection, that is `< 1.12.14`, which would call 1.15.0
  unaffected.
- **Both copies readable and different → union.** Fail-closed: no "fixed" claim rests on the narrower copy.
- **Existing advisory tests unchanged.** The regression change keeps the old title format ("X affects target
  T: …"); "Security regression" is in the detail and the rule. Unassessed advisories' evidence is not pulled
  into the edge (as before), and their entries are located by URL and source.
- **Impact classification unchanged.** The section adds no finding. Advisory changes keep the
  `impact:not-joined` UNKNOWN finding (making `advisory:fixed` informational would have lowered review → auto-pass).
- **Package identities left out:** `/vN` modules (argo-cd, crossplane, flux, loki, traefik), modules whose ranges
  are not in product release numbers (minio, linkerd2), and modules versioned apart from the product
  (actions-runner-controller). Chart products and non-Go products are also out.
- **Text reports render the section only when a source was read.** The existing warnings already state an
  unavailable source.

## Files touched outside ownership (minimal, additive)

- `internal/app/app.go` (wiring: `Versions`, `candidateCharts`); `internal/app/testdata/e2e/golden/*` regenerated
  (`-update`). The diff is the new global-advisories source row and warning (offline: not in cache), the edge's
  `security` block, and one source-table column re-aligned by the longer kind name.
- `internal/policy/evaluate.go`: one predicate (`isAdvisoryAffecting` also matches `advisory:regression`).
- `internal/domain/impact.go` (`ImpactReport.Security` + validation), `internal/domain/release.go`
  (`Advisory.UpdatedAt`, `Advisory.Sources`), `internal/domain/schemagen/meta.go` (2 enums, descriptions),
  `schemas/*.schema.json` regenerated, `schemas/product-definition.schema.json` (hand-written: kind + fields).
- `internal/upgrade/render.go`, `internal/impact/render.go`: one section call each (shared renderer in
  `internal/upgrade/security_render.go`).

## Contract changes (additive)

`UpgradeEdge.security`, `ImpactReport.security`, `Advisory.updatedAt/sources`, `Locator.ecosystem/package`,
`Dossier.security`, rule `advisory:regression`, `upgrade.Input.Versions/Charts`, `upgrade.CandidateVersions`,
`ingest.Ingester.ChartVersions`.

## Regression comparison (requirement 5)

`ri -offline eval -investigations -rescore <run> -o json` with the base binary (44f33a8f) and this branch, on
every B0 and C1 run (B0-K0-20261017-{6d2ae5bf, d262210d, edb21ed9-v8}, C1-K0-20261021-{3634f501, 6f5de003,
e59f89c8}-v9-r{1,2,3}). The outputs were written to scratch and compared by digest only, never opened.

- **12/12 runs byte-identical** (stdout and stderr). Exit codes are equal (1 on both, the runs' existing gate
  status).
- **Findings added by the new security rules: 0.** A count over the stored inputs in each run's `cases.tar.gz`
  found 0 changes with rule `advisory:{fixed,affects-target,regression}` and 0 unread-advisory warnings. The
  runs carry no edge artifact.
- Limitation: rescore re-classifies stored records and does not rebuild edges, so it cannot exercise the new
  advisory code. That needs a new recorded state with `api.github.com/advisories` responses (the commander's
  recording step).

Also checked: the impact-enrichment offline replay has 4 pending prompts on base and on this branch (no recorded
prompt moved).

## Known gaps / open questions

- No end-to-end app test with advisory data: recorded e2e states lack the new `/advisories` responses (offline
  they read as unavailable, which the goldens now show). The pieces are unit-tested.
- `docs/ONBOARDING.md` (generated by `ri stats`) was already stale before this lane (e.g. 164 vs 263 constructs).
  It was not regenerated, to avoid unrelated churn; regenerate it when the product changes merge.
- Fixtures use a fictional package (`example.com/acme`) with the real response shape, so no development-case
  data is in the tests.

## Test status

`go build ./... && go vet ./... && go test ./...`: green (46 packages ok, 0 FAIL). Schemas regenerated, and
`TestSchemasUpToDate` passes.

## Cleanup

No kind clusters or servers were started. `bin/` and scratch binaries/outputs were removed (see the final
message).
