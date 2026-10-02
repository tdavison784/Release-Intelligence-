# Discovery improvements (G3): measured against the saved baselines

Re-runs of `ri discover` (deterministic, no LLM, default 4 validation
releases) on three already-onboarded repositories, compared with the saved
baseline proposals in this directory and with the final hand-authored
definitions in `products/`. Nothing in `products/` or in
`docs/onboarding/records/` was changed.

"Coverage of the final definition" counts a final source/artifact as
proposed only when the proposal is materially correct (locator / registry /
version relation), not merely present. "Wrong" counts proposals that had to
be replaced by a human (wrong train, wrong registry, wrong relation, wrong
file) — items kept-but-unverified (blocked hosts) are counted separately in
the status columns.

| Product | Elements proposed (before → after) | Final elements covered (before → after) | Wrong proposals (before → after) | Statuses after (validated / discovered / inferred / unverified / exception) |
|---|---|---|---|---|
| external-secrets | 7 → 7 | sources 4/6 → 4/6, artifacts 0/6 → 3/6 | 3 → 0 | 4 / 4 / 0 / 1 / 0 |
| crossplane | 6 → 7 | sources 3/9 → 3/9, artifacts 2/5 → 3/5 | 2 → 2 | 5 / 3 / 0 / 0 / 0 |
| ingress-nginx | 12 → 14 | sources 3/7 → 6/7, artifacts 4/12 → 6/12 | 4 → 2 | 9 / 4 / 0 / 2 / 0 |
| postgresql (bonus) | 3 → 3 | tags 0/4 → 1/4 (listable + validated), rest manual | 1 → 0 | 0 / 2 / 1 / 0 / 0 |

## What changed per product

### external-secrets

Before → after (statuses from the new report):

- `helm-chart`: `version: independent` on 1 unreachable channel (unverifiable)
  → **`lookup appVersion == {{.Tag}}`** — the final definition's exact
  relation — grounded in `deploy/charts/external-secrets/Chart.yaml`
  (appVersion `v2.10.0` at tag `v2.11.0`: the chart lags one release; the
  relation still reads) and **historically-validated** on 4 releases through
  the gh-pages index on raw.githubusercontent.com. Channels 1 → 4
  (`external-secrets.github.io`, `charts.external-secrets.io`,
  raw gh-pages index, `helm-git` at `helm-chart-*` tags); 2 of the final
  definition's 3 channels found (the ghcr OCI chart channel is not).
- `controller-image`: `docker.io/external-secrets/external-secrets` (wrong;
  that namespace is empty) → **`ghcr.io/external-secrets/external-secrets`**
  (the values default via `global.imageRegistry` joined with the hostless
  `image.repository`), tag `{{.Tag}}` from the chart appVersion default,
  historically-validated.
- `crds`: `config/crds/bases` repo-dir → **`deploy/crds/bundle.yaml`** (the
  file the install documentation references), historically-validated. This
  is the final definition's channel.
- `stability-support` is still proposed from the tagged tree and still fails
  validation (rows appear only on `main` after the release); the human fix
  (read from `main`, wider key match) remains manual.

### crossplane

- `image`: absent before (ghcr.io rejected as "snapshot-only") →
  **`ghcr.io/crossplane/crossplane:{{.Tag}}`**, historically-validated. The
  evidence is the manual *Promote* workflow (dispatch input
  `version: v1.18.0`) whose `promote-images … ghcr.io/crossplane/crossplane
  "$VERSION"` line now counts as a release publication. 1 of the final
  definition's 4 registries (xpkg.* and docker.io appear only as logins, not
  image pushes, in the tree).
- `helm-chart`: the development channel `charts.crossplane.io/master` is no
  longer proposed (dropped when a stable sibling exists); version relation
  `{{.Version}}` was already right. The S3 origin channel remains
  undiscovered (found by human research, not in the repository).
- `whats-new` per-minor and the generic `upgrade-crossplane.md` are still the
  deterministic picks; the human corrections (restrict to the v2.0.0 major,
  prefer `upgrade-to-crossplane-v2.md`) remain manual — recorded as the 2
  remaining wrong proposals.

### ingress-nginx

The saved baseline followed the **wrong tag train** (125 `helm-chart-*` tags
beat 101 `controller-v*` tags) and every relation inherited the error. The
new tag-train disambiguation (the chart's `appVersion` equals the newest
`controller-v` version) switches the train, re-checks out
`controller-v1.15.1` and re-scans, so each downstream inference sees the
right ref:

- `versioning.tagPrefix`: `helm-chart-` → **`controller-v`** with a strict
  `^controller-v(?P<version>…)$` pattern.
- release notes: chart changelog → **`changelog/controller-{{.Version}}.md`**
  (the final definition's `controller-changelog`), historically-validated.
- `helm-chart`: `template {{.Version}}` on the chart train → **`lookup
  appVersion == {{.Version}}`** with exactly the final definition's three
  channels (`kubernetes.github.io`, raw gh-pages index, `helm-git`),
  historically-validated.
- `controller-image`: tag `{{.Tag}}` (= `helm-chart-4.15.1`) →
  **`v{{.Version}}` on `registry.k8s.io/ingress-nginx/controller`**, the
  final definition's relation, historically-validated (through the install
  manifests; the registry itself is blocked).
- compatibility: not-found → **the README "Supported Versions table"**
  (key column after the status icon column),
  historically-validated on 4 releases.
- Still wrong: `defaultbackend-amd64` (fake registry coordinates, unverified)
  and the goreleaser `kubectl` plugin binary (no release carries it,
  unverified) — both now visibly flagged `unverified` instead of passing
  silently. 5 of the 9 provider manifests remain unfound (`deploy-cloud` is
  referenced only from the blocked docs site).

### postgresql (bonus, not part of the required three)

The recorded failure mode was "a tag scheme matching 0 of 693 tags". The new
component-group fallback mines the tag list and proposes
`^REL_?(?P<major>\d+)(?:_(?P<minor>\d+))?_(?P<patch>\d+)$`, **matching 554 of
693 tags** (554 stable, latest `REL_18_6`), with the drop-middle digit roles
derived from the tag ordering itself (every 2-group major is newer than every
3-group major: `REL9_6_24` = 9.6.24, `REL_17_2` = 17.0.2). Validation runs
against real releases (`REL_17_0`, `REL_17_11`, `REL_18_0`, `REL_18_6`); the
versioning element is honestly classified `inferred` (the digit-role reading
is an assumption the report asks the reviewer to confirm). The scheme matches
the pattern the human authored; everything else (DocBook release notes,
Docker images, tarball) remains manual as recorded.

## Where the improvements come from

| Change | Files | Failure mode it removes |
|---|---|---|
| Tag families + evidence-based train selection (release triggers, chart appVersion), re-checkout + re-scan on switch | `tags.go`, `train.go`, `pipeline.go`, `resolve.go` | following the most numerous tag family (ingress-nginx proposed the chart train) |
| Component-group tag schemes mined from the tag list, tested against it before being proposed; drop-middle digit roles from major ordering | `tags.go` | "0 stable, 693 junk": a scheme matching zero tags (postgresql) |
| Chart relations from Chart.yaml: appVersion lookup, lag-tolerant matching when the chart is cut after the tag | `detect_helm.go`, `resolve_artifacts.go` | `version: independent` for charts that package the release (external-secrets) |
| chart-releaser channels: raw gh-pages index + helm-git at the chart tag family | `detect_ci.go`, `resolve_artifacts.go` | chart relationships unverifiable (blocked `*.github.io` hosts) |
| Values registry joins (`global.imageRegistry` + hostless `repository`; `registry`+`image`+`tag`) | `detect_helm.go` | image channels falling back to the implicit Docker Hub namespace (external-secrets) |
| Release promote/push lines with a version argument in tag-triggered or version-dispatch workflows | `detect_ci.go` | "no release image" although a promote workflow publishes it (crossplane) |
| Compatibility-table key column after a status column | `detect_docs.go` | README support tables not found (ingress-nginx) |
| Docs-referenced CRD bundle preferred over the CRD source directory; `<placeholder>` refs in raw URLs | `resolve_artifacts.go`, `detect_build.go` | CRDs modelled from the wrong path (external-secrets) |
| Development chart channels dropped when a stable sibling exists | `resolve_artifacts.go` | `charts.*.io/master` proposed as a release channel (crossplane) |
| Family latest by version, not listing order | `tags.go` | lexicographic ls-remote made v1.9.0 "newer" than v1.15.0 |
| Status vocabulary + per-element evidence in the report | `resolve.go`, `propose.go`, `report.go`, `types` in `resolve.go` | proposals that passed silently (see below) |

## Honesty: the status vocabulary

Every proposed element now carries one of `historically-validated`
(observed by the real relationship checker for ≥3 sampled releases),
`discovered` (deterministic, grounded in the cited file, not checked),
`inferred` (heuristic assumption or AI answer — AI-sourced elements are
capped at `inferred` even when they validate), `unverified` (checks ran but
the channel was unreachable or samples insufficient) and `exception` (holds
only with validation-derived exceptions/availability). The report's new
"Proposed elements" table lists status, rule and the grounding evidence per
element, plus a status-count line — e.g. ingress-nginx now shows 9
historically-validated / 4 discovered / 2 unverified, where the baseline
report could only say "unverifiable" in free-text notes.

Wrong-proposal rate over the three products: 9 of 25 baseline proposals were
materially wrong; 4 of 28 new proposals are (crossplane's two documentation
choices, ingress-nginx's two unverified artifacts), and all four are flagged
`unverified`/open rather than asserted.
