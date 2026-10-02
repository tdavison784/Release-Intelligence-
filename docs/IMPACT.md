# Environment-aware impact (deterministic)

`ri upgrade` answers *what changed between A and B*. `ri impact` answers
*which of those changes matter to THIS environment*:

```
ri impact <product> <from> <to> \
  --kubernetes 1.28 \
  --values values.yaml \
  --manifests ./manifests \
  --crds ./crds \
  --images images.txt \
  --inventory inventory.yaml
```

or, pointing at a whole customer repository instead of individual files:

```
ri impact <product> <from> <to> --repo ./customer-repo --kubernetes 1.28
```

Every environment input is optional; at least one is required. The join applies
whatever is supplied — and says so when what was supplied is not enough to
decide (a run with `--kubernetes` only reports Helm-values changes as UNKNOWN,
never as silently unaffected).

The whole path is deterministic: local file parsing plus a pure join, no
cluster access, no network beyond what `ri upgrade` already needs, no LLM.
Enrichment (`ri upgrade -enrich`) is a separate optional layer and is never
part of it.

## The action-classification contract

Every verdict uses the five-class vocabulary of
[ACTION_CLASSIFICATION.md](ACTION_CLASSIFICATION.md) — the normative contract
the implementation enforces (`domain.ImpactReport.Validate()` and
`schemas/impact-report.schema.json`):

| Class | Meaning |
|---|---|
| `action-required` | the environment must change to avoid concrete failure; needs upstream + environment evidence and a deterministic relationship; confidence below `high` is prohibited here |
| `review-required` | credible overlap, applicability not deterministically provable |
| `informational` | evidenced overlap, no action implied (a pin that keeps winning, an in-range cluster) |
| `not-affected` | evaluated against a **supplied** environment dimension and clear; carries the evaluation record (`checks`) |
| `unknown` | applicability cannot safely be determined; carries `neededToDetermine` |

Applicability is decided **first** (`AFFECTED` / `NOT_AFFECTED` / `UNKNOWN`),
then — only for affected units — the action class. Severity
(`critical`/`high`/`medium`/`low`, where determinable) and confidence
(`provenance.confidence`) are separate axes. The engine never assumes
`no match = not affected`: a missing match with the dimension unsupplied is
UNKNOWN, with the missing input named.

## Provenance for every verdict

1. **Upstream chain**: the evidence ids of the joined change (or of the
   compatibility constraint / artifact move), copied from the edge. Required
   for every class.
2. **Environment chain**: the evidence ids of the environment facts that
   matched, resolving inside the report's `environmentEvidence` pool. Required
   for the affected classes; each record has kind `local-file` (URI = the path
   exactly as supplied, locator `$.dotted.key (L12)` or `L6`, excerpt = the
   verbatim line) or kind `input` (a directly supplied value such as
   `--kubernetes 1.28`), plus the sha256 digest of the parsed file.

For `not-affected` verdicts the environment chain is replaced by the
**evaluation record**: which dimension was consulted, how many facts were
compared, which upstream subjects were compared against them — enough to
answer "why do you think this doesn't affect me?" without a model call. For
`unknown` verdicts the report records `neededToDetermine` (e.g. "installed
CustomResourceDefinitions (--crds) not supplied").

`domain.ImpactReport.Validate()` enforces all of it: class-specific chains,
the demotion rule (`action-required` requires `high` confidence), unknown-only
`neededToDetermine`, check-carrying `not-affected`, and summary counts that
match the findings. No unexplained statement survives.

## The funnel

The text renderer opens with the five-line funnel; every verdict is counted
explicitly and unknowns are never folded into "not affected":

```
51 upstream changes analyzed
ACTION REQUIRED:    1
REVIEW REQUIRED:    3
INFORMATIONAL:    1
NOT AFFECTED:    3
UNKNOWN:   50
```

Per-finding sections print ACTION REQUIRED / REVIEW REQUIRED / INFORMATIONAL /
UNKNOWN. NOT AFFECTED appears in the funnel always and as a findings section
only in verbose mode (`ri impact --show-not-affected`), where every record
shows its evaluation.

## The environment model (`internal/env`)

Parsed from local files only, deterministically, with per-fact evidence:

| Input | Extracted facts |
|---|---|
| `--kubernetes 1.28` | the cluster version (evidence kind `input`) |
| `--values` (comma-separated files) | every key the user sets, flattened with exactly the values-snapshot path syntax (lists are leaves, odd keys are `["quoted.key"]`); plus images set through the `repository`+`tag`, `hub`+`tag` or `image:` conventions |
| `--manifests` (files or directories) | one fact per document (`apiVersion`/`kind`, with the document's line); flattened field paths of each resource (`spec.secretTemplate.labels`, …); every `image:` scalar |
| `--crds` (files or directories) | installed CustomResourceDefinitions: name, group, kind, versions with served/storage/deprecated/deprecationWarning, `spec.preserveUnknownFields`; their group/version pairs also feed the apiVersion inventory |
| `--images` (list, or a file with one reference per line, `#` comments) | explicit image references (mirror lists) |
| `--inventory` (YAML list) | the declared product inventory: which other products run here and at which versions (see below) |
| `--repo` (directory) | all of the below, discovered by convention (see the repo-mode section) |

Multi-document YAML streams are decoded with a real stream decoder
(`yaml.Decoder`), so a `---` separator inside a block scalar (literal `|` /
folded `>`) no longer splits a document, and every fact keeps the absolute
line number it was found at — locators point into the original file even
inside block scalars. A stream that stops parsing midway keeps the documents
decoded so far and warns about the rest; nothing is silently dropped. Other
robustness rules: a `---` inside a `--values` file is a warning (Helm values
are one document; only the first is read), duplicate mapping keys are
last-wins with a warning naming the repeat, non-mapping values roots and
unparsable documents are warnings, empty documents are skipped, BOM and CRLF
inputs parse. CRD documents inside `--manifests` are picked up too; a
non-CRD document inside `--crds` is a warning, not an error. A supplied
input directory that contains no `.yaml`/`.yml`/`.json` files warns as well.
Digests of all parsed files are recorded. Caps (10 000 values keys, 2 000
documents, 8 000 field paths, 5 cited occurrences per apiVersion) bound
hostile inputs; hitting one is a warning in the report.

Which inputs were **supplied** is recorded independently of what they yielded
(`env.Environment.Supplied`), so the join can tell "checked and clear" from
"never looked".

### Input health (`env.Environment.Health` / `Statuses`)

Every input dimension — `kubernetes`, `values`, `manifests`, `crds`,
`images` — carries a status: `absent` (not supplied), `ok` (supplied and
completely parsed) or `partial` (supplied with warnings: parse failures,
truncation, caps). Absence is **not knowledge**: an environment with no
`--crds` input has no facts about CRDs, and "nothing matched" must never be
read as "not affected". The join consults the statuses before drawing any
conclusion from missing facts; `ri impact` renders each dimension's warnings
in the Warnings section.

Beyond the per-rule facts above, every manifest document feeds two
cross-cutting inventories:

- **GVK usage inventory** (`Environment.GVKUsage`): one entry per distinct
  group/version/kind the manifests (or installed CRDs) touch, with the
  apiVersion split into `Group` (`""` for the core group) and `Version`, the
  `Names` (metadata name/namespace) of the using resources, the flattened
  `FieldPaths` those documents set, raw `Documents` refs (file + start line,
  so a matcher can re-inspect the original manifest) and the evidence chain.
  This is the contract a GVK-scoped CRD matcher consumes: "does this
  environment use *this exact* API, and where". The compact `APIVersions`
  list the impact join matches on is kept alongside.
- **Installed products** (`Environment.Installed`): best-effort
  identification of *what is installed and at what version*, from Helm
  release annotations (`meta.helm.sh/release-name`, `helm.sh/chart` — the
  version segment is peeled off the chart annotation), the
  `app.kubernetes.io/{name,instance,version}` labels, Argo CD Application
  specs (`spec.source[].chart`/`path`, `targetRevision` as the version), Flux
  HelmRelease specs (`.spec.chart.spec.chart` + `.version`, which may be a
  range such as `1.*`) and Helmfile `releases:` entries (repo mode). Every
  entry records the mechanism that grounded it and cites the exact fields —
  a guess is a guess, never stated as a fact. Argo/Flux inline values
  (`spec.source.helm.values`/`valuesObject`/`parameters`,
  `.spec.values`) are flattened into the values inventory with the same path
  syntax as parsed values files — they ARE the customer's values, and the
  values rules join on them like any other file.

### Product inventory (`Environment.Products`, `--inventory`)

"Which products, at which versions, run here" is a first-class dimension
(`products`), so release knowledge can state cross-product conditions
("requires ingress-nginx >= 1.12.6", "Argo CD manages cert-manager
resources") and the deterministic engine can evaluate them. Entries
(`env.ProductInstance`) carry the product id (the `products/<id>.yaml` id when
the name maps to the catalog, else the lower-cased name), the version
(normalized — `v1.12.1` → `1.12.1`; a bare release line such as `1.16` stays
a line and is never padded into a patch-level claim — with the raw text always
kept), the source and per-entry evidence (file, locator, excerpt, digest):

| Source | Grounded by |
|---|---|
| `declared` | `--kubernetes` (product `kubernetes`) and the inventory file |
| `detected-helm` / `-argo` / `-flux` / `-helmfile` | the existing installed-product detection (`Environment.Installed`, unchanged); the version is a **chart** version (`versionOf: chart`), which may differ from the app version |
| `detected-label` | `app.kubernetes.io/{name,version}` labels (app version) |
| `detected-image` | an image whose repository is an OCI channel of a catalog product's `container-image` artifact (generic, driven by `products/*.yaml`; a mirror that keeps the path but changes the registry host matches by path and says so in `note`), at its tag |

The inventory file is a YAML list of `{product, version, note?}`, given with
`--inventory`, or picked up as `inventory.yaml` in `--repo` mode and in an
eval environment fixture (`eval/cases/<id>/environment/inventory.yaml`).

Sources are **never merged away**. When entries of one product state different
versions (declared 1.12.6 vs an image at v1.12.1), all are kept, each names the
others in `conflict`, the report renders `CONFLICT with …`, and the dimension is
`partial` with a warning. A version that does not parse (`latest`, `1.*`, a
branch) is kept as stated, warns and can never satisfy or violate a range.

The products dimension is `absent` unless an inventory file was given or
something was detected (`--kubernetes` alone does not make it supplied):
absent means "no inventory supplied", **not** "no other products run here".
Likewise, with a supplied inventory a product that is not listed is "not
listed", not "not installed" — the engine decides what that licenses.

Accessors for the engine: `Environment.Product(id)` (declared entry wins),
`ProductInstances(id)` (all entries) and `ProductInRange(id, constraint)`,
which evaluates "product present with version in range" through the repo's
range representation (`domain.CompatibilityConstraint`, i.e.
`upgrade.EvaluatePlatformConstraint`), at full semver precision when the
entry has a full version and the constraint patch-level bounds. Its result
separates `Present`, `Computable` (false for an unparsable/unstated version, an
unevaluable constraint, or conflicting entries that disagree on the verdict)
and `InRange`.

The report carries the inventory additively (`environment.products`,
`environment.productsHealth`, omitted when the dimension is absent; evidence
ids resolve in `environmentEvidence`) and the text output lists it in the
environment block. `Statuses()` deliberately does not list the dimension (it
feeds the enrichment prompts and their committed answer caches); read it with
`Health(env.DimProducts)` / `ProductsStatus()`.

### Resource facts and the query API (`Environment.Resources`)

Beyond the path inventories above, every manifest document is also kept as a
`Resource` (group, version, kind, name, namespace, document ref, document
evidence) with three kinds of fact for the deterministic applicability engine.
They live in their own store: `ManifestFields`, `GVKUsage`, the report counts and
the enrichment prompts are unchanged, so these facts add no finding and move no
prompt digest.

- **Field facts** (`FieldFact`): the scalar **value** of each leaf, JSON-encoded
  exactly like `ValuesKey.Value` (`"Always"`, `true`, `null`), plus container
  facts for mappings and sequences so "is this subtree set" is answerable.
  Sequences are descended with `[]` markers — the syntax of CRD `SchemaPaths`
  (`spec.acme.solvers[].http01.ingress.class`) — and each fact also records the
  indexed `Element` path (`spec.acme.solvers[0]…`), which the evidence locator
  carries with the file line and an excerpt.
- **Text blocks** (`TextBlock`): multi-line string scalars and every ConfigMap
  `data` value, line by line. Each `TextLine` has line-level evidence; for
  literal block scalars `FileLine` is the true file line (scalar start +
  offset; `Exact`), for folded/quoted multi-line scalars it is the line the
  scalar starts on. A `---` inside a block scalar never splits the document.
- **References** (`Ref`): any mapping under a key ending in `Ref` (or elements
  under a key ending in `Refs`) that carries a `name` (+ optional
  `kind`/`group`/`namespace`) — `issuerRef`, `parentRefs[]`, … — recorded
  generically and resolved by `ResolveRef` against the supplied manifests.
  Group/kind constrain the match only when stated; no namespace means the
  referrer's namespace (or a resource declaring none). Outcomes are
  `resolved`, `ambiguous` or `unresolved`; unresolved is **not** "does not
  exist": `ManifestsComplete` tells whether the manifests dimension was
  healthy.

Query API (all deterministic, load-ordered):

| Call | Answers |
|---|---|
| `ResourcesOfKind(group, kind)` / `Select(GVKSelector)` | the resources of an API identity (`Group ""` = core only, `"*"` = any; `Version ""` = any) |
| `FieldValues(sel, path)` | per matching resource: `Set` (false = the resource does not state it) and the `FieldFact`s; `path` with `[]` covers all elements, `[0]` one |
| `TextBlocks(sel, pathPrefix)` + `TextBlock.Match(re)` | per resource, the text blocks under a path and the lines matching a regex (`Withheld` counts lines a "no line matches" predicate cannot see) |
| `References(sel, path)` / `ResolveRef(from, ref)` | the references at a path, each with its resolution |

**Secrets.** Values are withheld (`Withheld: "sensitive"`, no `Value`, no value
in the evidence excerpt) for `Secret` resources, for keys naming a credential
(`password`, `token`, `apiKey`, `clientSecret`, `privateKey`, …) and for
anything containing a private key; text lines assigning such a key, or inside
a private-key block, are withheld the same way (and never match). Oversize
leaves (> 4 KiB) are withheld as `oversize`. Resource values never reach an
LLM prompt (the prompt carries paths only; pinned by
`TestPromptNeverSendsManifestValues`). Caps (100 000 field facts, 50 000 text
lines, 5 000 per block) are warnings that make the manifests dimension
`partial`.

## Repository mode (`--repo`)

`--repo ./customer-repo` walks a directory tree (bounded depth 10, ≤ 5 000
files; `.git` and other hidden directories except `.github`, `vendor`,
`node_modules`, `.terraform`, `dist`, `target`, `tmp` are skipped, and a
directory containing `Chart.yaml` is skipped whole — a vendored chart's
values are chart defaults, not customer values) and classifies each file by
convention:

| Convention | Classified as | What is taken |
|---|---|---|
| `values.yaml`, `values-prod.yaml`, `prod-values.yaml` (by name) | `values` | Helm values file |
| YAML whose documents have `apiVersion`+`kind` | `manifests` | loaded like `--manifests`; a file is skipped silently when no document is k8s-shaped (compose files, CI configs, chart templates) |
| document with kind `Application` in `argoproj.io` | `argocd` | loaded as a manifest; chart identity, `targetRevision` and inline Helm values become installed-product facts and values keys |
| document with kind `HelmRelease` in `helm.toolkit.fluxcd.io` | `flux` | loaded as a manifest; chart + version range + `.spec.values` likewise; `valuesFrom` (cluster objects) is a warning |
| `kustomization.yaml` | `kustomization` | referenced `resources`/`bases`/patches files are inventoried as plain manifests **without applying patches or transformers** (warning); `images:` overrides become pinned image references |
| `helmfile.yaml` | `helmfile` | `releases:` chart+version become installed-product guesses; referenced values files become values inputs; templating (go templates, environments) is not evaluated (warning) |
| `.github/workflows/*.yaml` | `workflow` | scalar `image:` references |
| `*.tf` | `terraform` | literal `image = "…"` assignments only; HCL structure, variables and templating are not evaluated |

Every classification is recorded on `Environment.Discovered` (path, kind,
detail, evidence at the file:line that grounded it) and every ambiguity is a
warning in the report: two values files are both applied (with a warning),
not silently ranked.

**Precedence.** Explicit flags compose with discovery: discovered files are
loaded first, the explicit `--values`/`--manifests`/`--crds` entries after
them — so in an ordered application (Helm values) an explicit file wins
per-key over a discovered one. A file reached through both routes is loaded
once. `--kubernetes` has no repository source and remains flag-only.

## The join (`internal/impact`)

The join runs over the edge's computed diff rules (the `values:*`, `crd:*`,
`images:*` rules and `CompatibilityConstraint`s), because those are the
changes whose subjects are machine-comparable. Every such change gets a
verdict; every other change — note-derived (declared/heuristic) — gets an
explicit UNKNOWN record ("no machine-comparable subject") instead of silence.
The one exception is a note-derived security remediation (the change cites a
CVE/GHSA/advisory — the same predicate as the routine detector's security
carve-out) that ships with the target without a stronger signal (not
breaking, no operator directive): it classifies `impact:security-fix` ·
informational · low, because its applicability is universal by construction —
the fix ships with the upgrade (ACTION_CLASSIFICATION.md §5).

Each join rule declares the environment dimensions that must be supplied to
decide (values / manifests+CRDs / cluster version / images); a missing
deciding dimension means UNKNOWN with `neededToDetermine`. The tables below
show the affected-path behaviour once the dimension is supplied.

Paths are compared **segment-wise**, never by string prefix (`a.bb` is not
under `a.b`), using the same escaping syntax on both sides: the environment
side flattens values through `normalize.FlattenValues`, which produces exactly
the syntax of the chart-side snapshots.

### 1. Helm values (`--values`)

For each upstream values change (`values:removed`, `values:section-removed`,
`values:default-changed`, `values:added`) and each key the environment sets:

| Overlap | Meaning | Finding |
|---|---|---|
| removed key, exact or adjacent | the user's value stops taking effect | `impact:values-removed` · action-required · high |
| default changed, exact key set | the user's pin keeps winning | `impact:values-pinned` · informational · low |
| default changed, ancestor/descendant overlap | Helm merge semantics need a look | `impact:values-adjacent` · review-required · medium |
| new key, already set (or adjacent) | a previously ignored key becomes live | `impact:values-new-key` · review-required · medium |
| no overlap, values supplied | checked and clear | `impact:values-unset` · not-affected (evaluation record) |
| no values supplied | nothing to compare | `impact:insufficient-visibility` · unknown (`--values` missing) |

The upstream side lists subjects as flattened leaves (a removed section lists
its removed keys), so a user key matches whether the chart removed one leaf or
a whole section.

### 2. CRDs and API versions (`--manifests`, `--crds`)

CRD/schema changes are matched by **API identity, never by path alone**. The
join pins each upstream change to CRD name / group / version / kind — parsed
from the differ's own deterministic output, never guessed — and checks it
against the environment's GVK usage inventory (`Environment.GVKUsage`):

- `crd:removed` subjects are CRD names (`certificates.cert-manager.io`);
- `crd:version-*` subjects are `name/version`;
- a `crd:fields-removed` change's subjects are bare schema paths; its identity
  lives in the title (`Certificate v1alpha2 schema: 2 fields removed: …`, the
  label being the CRD's kind when the snapshot knows it, else its name) and
  the detail (`… in the cert-manager.io/v1alpha2 schema of
  certificates.cert-manager.io are pruned …`).

A change that yields no API group this way is UNKNOWN
(`impact:not-joined`, `neededToDetermine` names the gap) — matching its paths
by name alone would flag every unrelated resource that sets a same-named path
(a Deployment's `spec.foo` is not a cert-manager impact).

On the environment side, "usage" means a **resource manifest** of the
group/version/kind: an installed CRD document stages its served versions in
the inventory too, but sets no field paths there, so a non-empty `FieldPaths`
list is the marker of manifest use — "the CRD is installed" never
masquerades as "a resource of this kind exists". The confidence ladder:

| Ladder step | Requirement | Confidence ceiling |
|---|---|---|
| exact GVK (+ exact field) | group/version/kind pinned by the change and in manifest use; `crd:fields-removed`: the removed path is set at/below it under that GVK | `high` → `action-required` when the upstream change is a removal (`crd:*-removed` critical, `crd:field-removed` high) |
| known CRD + compatible kind | the CRD name matches an installed CRD and manifests use its `names.kind` (version unpinned for `crd:removed`); kind only *inferred* (installed CRD states none) → `medium` | `high`/`medium` (medium is demoted to review per the contract) |
| same API group (/version) only | the group matches but the kind is not pinned (CRD not installed) or does not match | `medium` → `review-required` at most, never action |
| field path only | the change states no parsable identity (or a `name/version` subject without version) | — → `impact:not-joined` · unknown (`neededToDetermine`) |
| no match, deciding dimension supplied | installed CRDs / manifests were checked | `impact:crd-unused` / `impact:crd-version-unused` / `impact:crd-field-unset` · not-affected (evaluation record) |

Why-blocks of GVK-scoped findings name the matched resources —
`Environment: Certificate/istio-system/example-com
(manifests/certificate.yaml:L1) sets spec.privateKey (L8)` — and group-only
findings say exactly what is missing to decide (the kind, via the installed
CRD's `names.kind`).

| Upstream rule | Environment fact matched | Finding |
|---|---|---|
| `crd:removed` | installed CRD with that name **and** manifest usage of its group (+kind when the CRD states it); group-only usage → review | `impact:crd-removed` · action-required · critical (high confidence only with usage; installed-but-unused → not-affected when manifests were supplied, review otherwise; group-only → review) |
| `crd:version-removed` / `crd:version-unserved` | exact GVK in manifest use (kind pinned via the installed CRD); group/version with unpinnable kind → review; installed CRD declares the version but no manifest uses the GVK → not-affected (with manifests) / review (without) | `impact:crd-version-removed` · action-required · critical |
| `crd:version-deprecated` | same matching; every affected verdict is review (a deprecation breaks nothing today) | `impact:crd-version-deprecated` · review-required · medium |
| `crd:fields-removed` | the removed path (array markers stripped) set at/below it **within the change's GVK**; a same-named path under another GVK never matches | `impact:crd-field-removed` · action-required · high (exact GVK) / review (kind or version unpinned, or a set section above the path) |
| unparseable upstream identity | — | `impact:not-joined` · unknown |
| no overlap, deciding dimension supplied (`--crds` for the CRD/version rules, `--manifests` for fields) | — | `impact:crd-unused` / `impact:crd-version-unused` / `impact:crd-field-unset` · not-affected |

### 3. Kubernetes compatibility (`--kubernetes`)

The cluster version is evaluated against every `kubernetes` constraint of the
target release with the same range semantics the edge diff uses — both go
through one shared representation (`upgrade.versionRangeOf`), so the diff and
the join can never disagree. A bare line ("1.31") is admitted when any patch
of the line is. Bound kinds have directional meaning: `minimum: 1.30` means
`>= 1.30` (the bound line and everything above), `maximum: 1.33` means
`<= 1.33`; a cluster exactly at a bound is admitted. `tested` lists are
enumerations: outside one is "untested" (informational), never "unsupported".
kubeVersion constraints with prerelease suffixes (`>=1.25.0-0`) compare
exactly like Helm's semver check, so line 1.25 is admitted.

| Outcome | Finding |
|---|---|
| in a `supported` range | `impact:kubernetes-in-range` · informational · low |
| below/above a `supported` range or below a `minimum` | `impact:kubernetes-below-range` / `impact:kubernetes-above-range` · action-required · high (worded as "narrowed under you" when the source release still admitted the cluster) |
| above a `maximum` | `impact:kubernetes-above-range` · action-required · high |
| outside `chart-kubeVersion` | `impact:kubeversion-blocked` · action-required · critical (Helm refuses the install) |
| outside `tested` | `impact:kubernetes-untested` · informational · low |
| constraint satisfied (minimum/kubeVersion/tested) | `impact:compatibility-satisfied` · not-affected |
| cluster version not supplied | `impact:insufficient-visibility` · unknown (platform named) |
| constraint not machine-readable | `impact:insufficient-visibility` · unknown |

Constraints of platforms with no environment input (OpenShift today) are
always UNKNOWN — never assumed fine.

### 4. Images (`--manifests`, `--values`, `--images`)

An image the environment references matches when its **repository** equals the
subject of an `images:removed` / `images:moved` / `images:tag-changed` change
(`impact:image-changed` · review-required · medium) or of a `container-image`
artifact that moved between the endpoints (review-required when the
environment pins the old tag or another tag; informational · low when it
already references the target tag). Unreferenced repositories →
`impact:image-not-referenced` · not-affected; no image-bearing input at all →
unknown.

## Example (from the checked-in fixture)

`internal/app/testdata/e2e/env/cert-manager` against the recorded
cert-manager v1.17.0 → v1.18.0 edge (phase 2 read
"51 upstream changes · 5 affect this environment"; the same run now states
what it could and could not decide):

```
51 upstream changes analyzed
ACTION REQUIRED:    1
REVIEW REQUIRED:    3
INFORMATIONAL:    1
NOT AFFECTED:    3
UNKNOWN:   50

Action required (1)
  1. Cluster Kubernetes 1.28 is below the supported range 1.29–1.33 of v1.18.0 [imp-7e9a11c2834d]
     v1.18.0 requires Kubernetes 1.29–1.33; the cluster runs 1.28. Plan the cluster upgrade at or above 1.29 before upgrading v1.18.0.
     environment: kubernetes 1.28  (ev-7ed6c5e2142e)
     upstream evidence: ev-94a8abc4e147, ev-904e0ae133bd
```

(chain 1: the support-matrix rows of both endpoints; chain 2: the
`--kubernetes` input.) The 50 UNKNOWN records are the honest change from
phase 2: 49 of the 51 edge changes are note-derived (declared release-note
items without a machine-comparable subject) or outside the join's rule set,
plus the OpenShift constraint no input can supply — previously they were
silently unmentioned, which read as "not affected". `-o json` prints the
`domain.ImpactReport`, validated by `schemas/impact-report.schema.json`.

Repo mode is exercised the same way end to end:
`internal/env/testdata/customer-repo` (values files per cluster, base
manifests with Helm metadata, an Argo CD Application, a Flux HelmRelease, a
kustomization, a helmfile, a vendored chart, a workflow and a Terraform file)
joined with the same edge produces the same funnel plus the repo-mode
warnings — see `TestE2EImpactRepo` and the `cert-manager_repo_*` goldens.

## Honest limitations

- **Note-derived changes are unknown, not evaluated.** Declared breaking
  changes from release notes have no machine-comparable subjects; the join
  counts them in the funnel as UNKNOWN (with the reason) instead of silently
  skipping them. Turning them into real verdicts needs subjects the snapshots
  do not carry (see `docs/ARCHITECTURE.md` on honest gaps).
- **CRD matching is GVK-scoped; the identity is parsed from the differ's output.** The edge's CRD snapshots are per-release summaries, so a `crd:fields-removed` change carries its CRD identity only in its title and detail text; the join parses those deterministic strings and treats a change that yields no API group as UNKNOWN rather than guessing. Resource *clients* outside the supplied manifests (operators, controllers, API consumers) stay invisible.
- **Group-only CRD matches stay below action.** Without an installed CRD of the changed name, a group (or group/version) match cannot distinguish kinds of the same group — another CRD could serve the kind in use — so they are review-required at most (medium confidence may not carry ACTION REQUIRED, the contract's demotion rule).
- **Values semantics are syntactic.** The join reasons about key paths, not
  Helm's full merge/type-coercion behaviour; `review-required` is the honest
  outcome wherever merge behaviour could surprise.
- **No cluster access.** The cluster version comes from a flag; nothing is
  read from a live cluster (CRs actually stored, served versions of the
  apiserver). The environment is what the files say.
- **Kustomize is not built.** Repo mode inventories the files a
  kustomization references and its `images:` overrides; strategic-merge/JSON
  patches, configMap/secret generators and name transforms are not applied,
  so a key that only a patch introduces is invisible (warned).
- **Helmfile templating is not evaluated.** Releases, versions and
  values-file references are taken from the literal `helmfile.yaml`;
  go-template values and `environments:` state resolution would answer "what
  does the staging environment actually set" — recorded gap.
- **Terraform: image refs only.** Literal `image = "…"` assignments are
  extracted; modules, variables and `templatefile` rendering stay invisible
  (no HCL library). Anything beyond the trivially deterministic part is a
  recorded gap.
- **Installed-product identification is a guess.** Annotations, labels and
  GitOps specs can disagree or lie (a stale `helm.sh/chart`); every
  `Installed` entry therefore names the mechanism that grounded it. Nothing
  downstream may treat it as authoritative.
- **Digest pinning detail.** Image matches compare repositories (and tags
  when the environment states one); digests are carried through to the
  finding text but do not change the matching.
