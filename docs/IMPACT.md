# Environment-aware impact (deterministic)

`ri upgrade` answers *what changed between A and B*. `ri impact` answers
*which of those changes matter to THIS environment*:

```
ri impact <product> <from> <to> \
  --kubernetes 1.28 \
  --values values.yaml \
  --manifests ./manifests \
  --crds ./crds \
  --images images.txt
```

Every environment input is optional; at least one is required. The join applies
whatever is supplied and silently skips the rules whose input is missing
(a run with `--kubernetes` only performs the compatibility check).

The whole path is deterministic: local file parsing plus a pure join, no
cluster access, no network beyond what `ri upgrade` already needs, no LLM.
Enrichment (`ri upgrade -enrich`) is a separate optional layer and is never
part of it.

## The two provenance chains

Every finding — the atomic unit of the report, `domain.ImpactFinding` — cites
**both** chains:

1. **Upstream chain**: the evidence ids of the joined change (or of the
   compatibility constraint), copied from the edge. They resolve inside the
   report's `evidence` pool and point at upstream sources (values.yaml of the
   chart, the support-matrix row, the release manifest).
2. **Environment chain**: the evidence ids of the environment facts that
   matched, resolving inside the report's `environmentEvidence` pool. Each
   record has kind `local-file` (URI = the path exactly as supplied, locator
   `$.dotted.key (L12)` or `L6`, excerpt = the verbatim line) or kind `input`
   (a directly supplied value such as `--kubernetes 1.28`), plus the sha256
   digest of the parsed file.

`domain.ImpactReport.Validate()` enforces this: a finding without either
chain, or citing an id that does not resolve within the document, is invalid.
No unexplained statements survive.

Findings are classified `action-required` (the environment must change or the
upgrade fails / silently misbehaves), `review` (plausible impact that depends
on intent the files cannot show) or `informational` (confirmed overlap, no
action implied — e.g. a pinned value that keeps winning, a cluster inside the
supported range). The summary line is the funnel:

```
51 upstream changes · 5 affect this environment · 1 action required · 3 review · 1 informational
```

A finding can exist without an upstream *change*: the cluster-version check
joins the environment against the target's compatibility *constraints*, which
exist even when support did not change.

## The environment model (`internal/env`)

Parsed from local files only, deterministically, with per-fact evidence:

| Input | Extracted facts |
|---|---|
| `--kubernetes 1.28` | the cluster version (evidence kind `input`) |
| `--values` (comma-separated files) | every key the user sets, flattened with exactly the values-snapshot path syntax (lists are leaves, odd keys are `["quoted.key"]`); plus images set through the `repository`+`tag`, `hub`+`tag` or `image:` conventions |
| `--manifests` (files or directories) | one fact per document (`apiVersion`/`kind`, with the document's line); flattened field paths of each resource (`spec.secretTemplate.labels`, …); every `image:` scalar |
| `--crds` (files or directories) | installed CustomResourceDefinitions: name, group, kind, versions with served/storage/deprecated/deprecationWarning, `spec.preserveUnknownFields`; their group/version pairs also feed the apiVersion inventory |
| `--images` (list, or a file with one reference per line, `#` comments) | explicit image references (mirror lists) |

Multi-document YAML streams are split on `---` separators with line tracking,
so locators point into the original file. CRD documents inside `--manifests`
are picked up too; a non-CRD document inside `--crds` is a warning, not an
error. Unparsable documents are warnings. Digests of all parsed files are
recorded. Caps (10 000 values keys, 2 000 documents, 8 000 field paths, 5
cited occurrences per apiVersion) bound hostile inputs; hitting one is a
warning in the report.

## The join (`internal/impact`)

The join runs over the edge's computed diff rules (the `values:*`, `crd:*`,
`images:*` rules and `CompatibilityConstraint`s), because those are the
changes whose subjects are machine-comparable. Note-derived changes
(declared/heuristic) are counted in `upstreamChanges` but not joined — the
plan records this as a gap (below).

Paths are compared **segment-wise**, never by string prefix (`a.bb` is not
under `a.b`), using the same escaping syntax on both sides: the environment
side flattens values through `normalize.FlattenValues`, which produces exactly
the syntax of the chart-side snapshots.

### 1. Helm values (`--values`)

For each upstream values change (`values:removed`, `values:section-removed`,
`values:default-changed`, `values:added`) and each key the environment sets:

| Overlap | Meaning | Finding |
|---|---|---|
| removed key, exact or adjacent | the user's value stops taking effect | `impact:values-removed` · action-required |
| default changed, exact key set | the user's pin keeps winning | `impact:values-pinned` · informational |
| default changed, ancestor/descendant overlap | Helm merge semantics need a look | `impact:values-adjacent` · review |
| new key, already set (or adjacent) | a previously ignored key becomes live | `impact:values-new-key` · review |

The upstream side lists subjects as flattened leaves (a removed section lists
its removed keys), so a user key matches whether the chart removed one leaf or
a whole section.

### 2. CRDs and API versions (`--manifests`, `--crds`)

| Upstream rule | Environment fact matched | Finding |
|---|---|---|
| `crd:removed` | installed CRD with that name; or manifest usage of the CRD's API group (kind checked when the installed CRD is known) | `impact:crd-removed` · action-required |
| `crd:version-removed` / `crd:version-unserved` | manifest `apiVersion` equal to group/version; or an installed CRD declaring the version | `impact:crd-version-removed` · action-required |
| `crd:version-deprecated` | same | `impact:crd-version-deprecated` · review |
| `crd:fields-removed` | manifest field path equal to, or below, the removed path (`[]` array markers are stripped on the upstream side) | `impact:crd-field-removed` · action-required (exact/below) or review (the manifest sets a section above it) |

Group-only matches (manifests use the group, no installed CRD confirms the
kind) are reported with medium confidence.

### 3. Kubernetes compatibility (`--kubernetes`)

The cluster version is evaluated against every `kubernetes` constraint of the
target release with the same range semantics the edge diff uses
(`upgrade.EvaluatePlatformConstraint`): a bare line ("1.31") is admitted when
any patch of the line is. In range → `impact:kubernetes-in-range`
(informational, `supported` kind only); below/above a `supported` range or a
`minimum` → action-required (worded as "narrowed under you" when the source
release still admitted the cluster); outside a `chart-kubeVersion` →
`impact:kubeversion-blocked` (Helm refuses the install); outside `tested` →
informational. Uncomputable constraints are skipped.

### 4. Images (`--manifests`, `--values`, `--images`)

An image the environment references matches when its **repository** equals the
subject of an `images:removed` / `images:moved` / `images:tag-changed` change
(`impact:image-changed` · review) or of a `container-image` artifact that
moved between the endpoints (review when the environment pins the old tag or
another tag; informational when it already references the target tag).

## Example (from the checked-in fixture)

`internal/app/testdata/e2e/env/cert-manager` against the recorded
cert-manager v1.17.0 → v1.18.0 edge:

```
51 upstream changes · 5 affect this environment · 1 action required · 3 review · 1 informational

Action required (1)
  1. Cluster Kubernetes 1.28 is below the supported range 1.29–1.33 of v1.18.0 [imp-7e9a11c2834d]
     v1.18.0 requires Kubernetes 1.29–1.33; the cluster runs 1.28. Plan the cluster upgrade at or above 1.29 before upgrading v1.18.0.
     environment: kubernetes 1.28  (ev-7ed6c5e2142e)
     upstream evidence: ev-94a8abc4e147, ev-904e0ae133bd
```

(chain 1: the support-matrix rows of both endpoints; chain 2: the
`--kubernetes` input.) The same run reports the pinned
`prometheus.servicemonitor.targetPort` default change as informational, the
already-set `global.rbac.disableHTTPChallengesRole` as review, and the old
controller/webhook image pins as review. `-o json` prints the
`domain.ImpactReport`, validated by `schemas/impact-report.schema.json`.

## Honest limitations

- **Note-derived changes are not joined.** Declared breaking changes from
  release notes have no machine-comparable subjects; only the computed
  contents diffs and compatibility constraints join. A declared removal that
  the snapshots do not capture will not match (the edge itself may also miss
  it — see `docs/ARCHITECTURE.md` on honest gaps).
- **CRD field removal matches by path only.** The upstream change lists
  schema paths without the CRD's identity (the edge's CRD snapshots are
  per-release summaries); a removed `spec.x` matches any manifest setting
  `spec.x`. The CRD name is visible in the change title for the human.
- **Group-only API matches are medium confidence.** Without an installed CRD,
  a group/version match cannot distinguish kinds of the same group.
- **Values semantics are syntactic.** The join reasons about key paths, not
  Helm's full merge/type-coercion behaviour; `review` is the honest outcome
  wherever merge behaviour could surprise.
- **No cluster access.** The cluster version comes from a flag; nothing is
  read from a live cluster (CRs actually stored, served versions of the
  apiserver). The environment is what the files say.
- **Terraform is not parsed.** No HCL library is in go.mod; infrastructure
  managed in Terraform is invisible to the join. Recorded gap; the inputs are
  designed so a `tf` source can be added without changing the join.
- **Digest pinning detail.** Image matches compare repositories (and tags
  when the environment states one); digests are carried through to the
  finding text but do not change the matching.
