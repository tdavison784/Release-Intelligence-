# Environment-aware investigation (`ri investigate`, Phase 4)

Design: [docs/phase4/env-impact/DESIGN.md](phase4/env-impact/DESIGN.md). Each section below is owned by
one lane; edit only your own section.

## Evidence bundle

An investigation starts from an **evidence bundle**: an immutable, content-addressed record of one
upgrade (`product from → to`) against one environment. Code: `internal/investigate/bundle` (collectors,
assembler) and `internal/investigate/store` (stores). Contract: DESIGN.md §2.

| Piece | Built by | What it holds |
|---|---|---|
| `ChangeEvidenceRecord` | `bundle.NewChangeCollector(app, store)` | the `UpgradeEdge` (one `upgrade-edge` artifact), a `ChangeItem` per non-routine change and per moved compatibility constraint, the upstream documents, the release-scope (chart-default) render, and a status for **every** change source |
| `EnvironmentSnapshot` | `bundle.NewSnapshotBuilder(store)` | every environment file (secrets redacted), the parsed `env.Environment` (`environment-facts`), a status per input, the repo pinned by commit, dirty flag and a tree digest |
| `EvidenceBundle` | `bundle.NewAssembler(store)` | the two above plus the environment renders (from the `renderdiff` lane), the manifest of all artifacts and the evidence pool; `Validate()` resolves every id |

**Change sources.** Each of `release-notes`, `migration-guide`, `source-diff`, `api-schema`, `chart`,
`kustomize`, `rendered-manifests`, `compatibility`, `advisories` is stated exactly once:
`ok` (with the collected artifacts), `partial`, `unavailable` or `not-applicable`, always with a reason
unless `ok`. Upstream documents are fetched through the fetch cache (a GitHub blob URL is read as its raw
content) and compared with the digest recorded when they were ingested; a document that changed since is
`partial`. A product with no upgrade guide (ingress-nginx) is `unavailable`, one that publishes no CRD
bundle is `not-applicable`; source diffs and release-side Kustomize are not collected in milestone 1
and say so.

**Scope and manifestation.** Every item carries `InScope` + `ScopeRule` (DESIGN §1.3) and
`Manifestation` + `ManifestationRule` (§2.2.1), decided by declared rules over the upgrade edge
(`internal/investigate/bundle/scope.go`), never by product code. A behaviour default compiled into a
controller (`manifest:declared-behaviour`, `manifest:crd-default`) is not render-decisive: a silent
render never clears it. Every release-scope rendered change that no change explains becomes its own
item, anchored on it (DESIGN §1.3, first match wins): a class the default tier policy blocks (removed
RBAC permission, removed resource, API-version or CRD storage/served move) is in scope
(`scope:rendered-structural`); the version bump of an image the product definition itself publishes
(`render.SelfArtifactBump`: same repository, exactly the edge artifact's from/to tags) is out of scope
(`scope:routine-self-artifact`) — so it is correlated, never "uncorrelated"; CRD schema documentation
text (class `crd-description-changed`) that no change documents is out of scope (`scope:documentation`,
see "CRD schema documentation" under Render diffs); anything else is in scope
(`scope:rendered-unexplained`, manifestation `rendered-config`).

**Knowledge facts.** `bundle.NewChangeCollector(app, st, bundle.WithKnowledge(ks.Facts...))` (the facts
`app.LoadKnowledge` proves) stores every fact attached to a change of the edge (`impact.AttachedChanges`,
the join's rule) as a `knowledge-fact` artifact `derived/knowledge/<product>/<fact-id>.json`, related to
that change's item (`ArtifactRef.ChangeItems`, `ChangeItem.Artifacts`). Validators read facts from the
bundle, so an investigation is reproducible from its bundle alone (Principle 6); trust is still decided by
the fact's own verification level (consensus/proxy facts never pin).

**Layout** (`<state>/investigations/`, `store.New(<state>)`):

```
cas/sha256/<hex[0:2]>/<hex>   artifact bytes — immutable, written once, digest verified on every read
bundles/<bnd-id>.json         EvidenceBundle — immutable (re-writing the same bundle is a no-op)
runs/<inv-id>.json            Investigation — rewritten as it progresses; its audit trail may only grow
```

Writes are atomic (temp file + rename). Artifact paths are logical: `upstream/<product>/<host>/<path>`,
`env/<path inside the repo>` (explicit inputs: `env/inputs/<file>`), `derived/<stage>/…`.

**Redaction** happens before hashing: Kubernetes `Secret` `data`/`stringData` values and values whose key
names a credential (`password`, `token`, `apiKey`, `privateKey`, `credentials`, …) become
`"<redacted sha256:…>"` (keys kept); the artifact is marked `Redacted`. Credential values are also
withheld from the facts artifact and from evidence excerpts. Files that are not YAML/JSON are stored
verbatim.

### One redaction rule (redaction@v2, Phase 5 lane redact)

One rule (`internal/redaction`) covers everything taken from an environment: the collected environment
files, release-scope renders and facts artifact (`bundle.snapshot@v2`, `bundle.change@v2`, marker
`<redacted sha256:…>`), and the render model (`internal/render`; stored env renders `renderdiff@v2`, marker
`<sensitive sha256:…>`). Renders reach the agent, so they follow the same rule as collected files:

- (a) Secret `data`/`stringData` values are withheld.
- (b) A string holding a PEM private-key block is withheld under any key.
- (c) A scalar whose own key names a credential is withheld unless it is a boolean or null
  (`createSecret: true`). **Exception: object-reference keys.** A key that is exactly `name` or `namespace`,
  or that ends in `Name`/`Namespace`, holds an object name, not secret material, so it stays readable
  (`secretName`, `privateKeySecretRef.name`, `imagePullSecrets[].name`). `key` is a reference key only in the
  selector shape `{name, key}` (SecretKeySelector, ConfigMapKeySelector: `tokenSecretRef.key`): its mapping
  must also have a `name`. A lone `key` (`credentials: {key: …}`) is withheld. The graph keeps resolving
  references.
- (d) Below a credential-named key holding a mapping or list (`privateKey:`, `credentials:`), only
  booleans, nulls, numbers and reference keys stay readable; every other string is withheld. v1 wiped such
  a mapping in collected files, so `privateKey.size` and `privateKey.rotationPolicy` were invisible. The
  render model kept every string there, including `credentials.secretAccessKey`.
- (e) The `value` of an environment variable whose name names a credential (`- name: DB_PASSWORD`) is
  withheld.
- Credential names are the union of the two former expressions plus `accessKey`: `password`, `passwd`,
  `passphrase`, `secret`, `token`, `api[-_]?key`, `private[-_]?key`, `access[-_]?key`, `credential`, `bearer`
  (so `secretAccessKey` and `clientSecret` are covered).
- A `CustomResourceDefinition` is a schema: its property names (`…properties.privateKey`) name fields, so
  only (b) applies in it. Otherwise its descriptions, types and enum values would be withheld, and the
  read-back below and schema defaults need them.
- The render model leaves existing markers alone (a collected file parses unchanged). A re-encoded collected
  document keeps its `---` separator; v1 dropped it, merging the document into the one before.

**Read-back** (`redaction.Schemas.Recover`): a marker (either form) below a credential-named mapping or list
is replaced by a string enum value of the bundle's CRDs only when all of these hold:

- the field's own key names no credential;
- the object is not a core Secret;
- exactly one enum value declared for that exact group/version/kind and field path has that marker.

The CRDs are the upstream CRD artifacts plus those the environment files and renders declare. So only a
constant the schema itself lists is ever shown; free text and anything without a declared enum stay
withheld. Stored bytes never change. The read-back runs:

- in the validators' effective view (`validate.override-inspection@v3`, `validate.schema-compat@v2`). A pin
  decided on a read-back value says so in its detail ("stored redacted as …: the digest of "Never", an enum
  value its CRD declares");
- in the item dossier (the object's `reason`);
- in `read_artifact`. The text stays verbatim, so quotes still verify; `readBack` lists the markers in the
  window that were read back (object, path, marker, value).

A marker that is not read back is **undecidable**, never compared as a literal value. A pin scope condition
on it is unknown (the object is never "provably out of scope"), and a schema-compat check whose value is
withheld is `not-run` (`schema:withheld`), never a pass.

**Dossier objects (`investigate.dossier@v3`).** The dossier reads the pin validator's effective object view
(`validate.EffectiveView`): complete product renders, version-independent overlay renders (a kustomize base
merged with its patches), and the files no render consumed. It keeps one entry per object, a render's before
a file's. An object whose consuming render is unavailable is `unknown` whatever it states. A setting named
by Go field names in upstream text (`Certificate.Spec.PrivateKey.RotationPolicy`) is resolved to the JSON
field path of the view's CRD schema. Without a confirming CRD it is lowerCamel-cased and an unset field is
`unknown`, never the inherited default.

**Reproducibility.** The bundle id is derived from content only. Wall-clock fields (`generatedAt`,
`retrievedAt`) are zeroed in the stored edge copy and `CreatedAt` is not part of the id, so two runs over
the same upstream evidence, repository state and renders yield the same bundle id; a changed file, a
different commit or a dirty tree yields a different one. The run record follows the same rule: the stored
impact report is its identified content (`ImpactReport.IdentifiedContent`, no `generatedAt` /
`retrievedAt`). The stamps are kept on the record as `reportStamps` (report `generatedAt`, `retrievedAt` per
evidence id). They are excluded from the record digests together with `createdAt`, audit times and every
validation's `checkedAt`, so two identical runs give the same record digest (measured: 8/8 eval inputs). An
enrichment's AI `generatedAt` is the model answer's own time, replayed unchanged from the answer cache, and
stays in the report.

**Known gaps (milestone 1).** Live-cluster observation is not implemented (a `live` request records an
`unavailable` input). Routine changes are summarised in the edge, not itemised. Render rules that name
an artifact version bump (rule `artifact`) carry no change id; the collector attributes them to the
computed image items of that repository.

## Environment values (per release)

Lane `values`. The snapshot (`bundle.SnapshotBuilder`) loads the environment with
`env.Inputs.Product` = the product under investigation, so the values that decide a `values`/`image`
item are those of the **release(s) deploying that product** — each release's stack applied as Argo CD,
Flux or helmfile applies it, merged with Helm semantics, with per-key provenance (docs/IMPACT.md
"Values are per release"). Consequences for collection status (R2.1):

- the `gitops-repo` input is no longer `partial` merely because the repository holds several values
  files: a product whose release references them all, and reads them all, is complete;
- it is `partial`, with the reason, when the product's stack has an unreadable layer, when several
  releases deploy the product with different effective values (each keeps its own keys; nothing is
  merged across them), or when a values file no release references cannot be attributed by name;
- another product's releases (their files, their cluster-only `valuesFrom`) are not this product's gap.

The facts artifact (`environment-facts`) carries every release (`ValuesReleases`: layers in
application order, what was applied or ignored and why, effective keys, health) and the scope
(`ValuesScope`).

## Render diffs

The centerpiece evidence: the product's from and to releases rendered with the environment's own
configuration, diffed, and indexed so every rendered change is a citable record
(`internal/investigate/renderdiff`, DESIGN.md §2.4). `renderdiff.New(app, store)` implements the
`EnvironmentRenderer` port over `internal/render` (target detection, helm/kustomize renders, the semantic
diff, changelog correlation, chart coverage).

**One pair per render target.** Every deployment of the product the snapshot's repository (or explicit
values files) declares becomes a `ManifestPair` named by the target, with paths repo-relative so ids do
not depend on where the repository is checked out: `argocd:clusters/prod/app.yaml#0/$.spec.sources[0]`,
`flux:…`, `helmfile:…`, `values:<file>`, `kustomize:<dir>`. A repo-discovered values file that an Argo
CD / Flux / Helmfile target already layers is not rendered on its own (`render.DetectTargets`: that
configuration is not deployed anywhere); a note says so.

**Status** (what decides whether an absence of change is evidence):

| Status | When | Changes indexed |
|---|---|---|
| `ok` | both renders succeeded, values complete, and the chart (from or to, or a declared dependency) defines every top-level values key the customer sets | yes — `[]` means "rendered, nothing changed" |
| `partial` | `values-incomplete: …` (valuesFrom / value files outside the repo), `chart-coverage: …` (customer keys the chart never reads), or one side rendered and the other failed (e.g. `invalid-values: the target chart rejects…`) | **no** |
| `unavailable` | neither side rendered (`chart-unavailable`, `renderer-unavailable`, `template-error`, …), or a Kustomize overlay runs the product's images without pinning their version | **no** |
| `not-applicable` | `version-independent:` a Kustomize overlay (a consumer app) that neither pins nor runs the product: the upgrade does not change what it renders; its `from` render is kept for inspection | no |

An incomplete pair is an evidence gap, never "no change". `RenderResult.Notes` records what target
detection skipped, so "no pairs" is never silent.

**Artifacts** (`derived/render/env/<target-slug>/`): `from.yaml` / `to.yaml` are the *normalized* objects
the diff compared, one document per object headed by `# Object: <group>/<version>/<kind>/<ns>/<name>`
(Secret data and credential-named values are `<sensitive sha256:…>` digest markers; the artifact is then
`redacted`); `diff.json` is the full diff (`ri.dev/render-diff/v1alpha1`: provenance of both renders
without timestamps, every change with its id, correlation links and change items; `indexed: false` for an
incomplete pair).

**Rendered changes.** One `RenderedChange` per `render.Change` of an ok pair, id
`RenderedChangeID(environment, target, object, path, class)`. The path is `render.RenderedPath`, shared
by both render scopes: container args and RBAC rules are sets addressed by one path, so their changes
carry a member selector — `…args[=--flag]`, `rules[=apps/deployments:get]`. Each cites one `rendered-diff` evidence record whose `contentDigest` is
the diff artifact's digest and whose locator is `changes[id=<rc>]`, so it resolves to the bytes.
Sensitive changes record no values.

**Correlation** to change items (`Correlation` = the rules, comma-joined):

- a `render.Correlate` link to an edge change → that change's item (links to routine changes, which have
  no item, are dropped);
- an `artifact` link (no change id) → the computed image items (`manifest:computed-image`) whose title or
  subjects name the repository — the collector's release-scope rule;
- `release-anchor` → the item anchored on the matching release-scope rendered change (same class, API
  group, kind, object name and path with the release name normalized);
- otherwise `rendered-anchor` → the change's **own environment item** (`RenderResult.Items` →
  `EvidenceBundle.EnvironmentItems`, anchored on the environment-scope rendered change):
  `scope:routine-self-artifact` (out of scope) for a self-artifact version bump, `scope:documentation`
  (out of scope) for CRD schema documentation text, else `scope:rendered-unexplained` (in scope, so it
  gets a finding: the deployment changes and nothing upstream explains it — exactly what the
  investigation must look at).

**CRD schema documentation** (lane fixes2). The `description` keyword of a CRD's OpenAPI schema — text
or absent on both sides, not a property *named* `description` (that is a field, and keeps the field-*
classes) — renders as the class `crd-description-changed`. It documents the API and configures nothing:

- it correlates only by `documents-field`: to a change whose text names the property it documents
  (`render.DocumentedField`, through `items` / `additionalProperties`; an identifier of 6+ characters,
  the `mentions-field` bar). The word "description" never links anything. So the reworded docs of the
  field a release note changes stay part of that note's exposure (its roots and paths);
- such a link never makes the item render-decided: `manifest:rendered-correlated` needs a correlated
  change of another class (DESIGN §2.2.1). A behaviour default whose only rendered trace is its field's
  documentation stays `controller-behaviour`, and RenderDiff passes the description as harmless without
  ever clearing the item;
- documentation no change names becomes an out-of-scope `scope:documentation` item at both render
  scopes. It is accounted for (never "uncorrelated", so it does not block approval) and is never an
  item to review. The bundle rejects `scope:documentation` on any other class.

`EvidenceBundle.Items()` / `Item()` / `InScope()` cover record and environment items alike. With every
change anchored, `Uncorrelated()` is empty for a renderdiff-built bundle; it still blocks automated
approval when non-empty (DESIGN §3.4).

**Queries** (what the agent's diff tools call): `renderdiff.QueryPage(bundle, DiffQuery)` filters by
scope (default environment), target, change item, uncorrelated, classes, kinds, namespaces (`-` =
cluster-scoped) and path prefix; results are ordered by (target, object, path, class), at most 200 per
page (default 50, ≤256 KiB), with `Next` → `DiffQuery.After` to continue. Unknown targets, items,
classes, scopes or cursors are rejected (`ErrRejected`), never answered with an empty page.
`renderdiff.Object` reads one object or field (`spec.template.spec.containers[name=x].args[=--flag]`,
`metadata.annotations["a/b"]`, `ports[port=443/TCP]`) from either side, as YAML whose text occurs in the
stored render (cite it with `read_artifact`-style quotes); an absent object or path is `found: false`; a
side that was not rendered is `ErrNotFound` with the pair's reason; objects over 64 KiB must be read by
field. `renderdiff.Summaries` lists every pair with counts by class and uncorrelated.

Coverage on the groundtruth environments (8 cases) is recorded in
`docs/phase4/env-impact/status/renderdiff.md`; reproduce with
`RI_RENDERDIFF_STATE=<warm .ri> go test ./internal/investigate/renderdiff -run Groundtruth -v`.

## Validators

Package `internal/investigate/validate` (DESIGN.md §2.6, §3.1, §7). Five `investigate.Validator`s turn the
bundle, the traversal and the deterministic join into `ValidationRecord`s. Every record is at level
`deterministic` and states its outcome relative to the **safety property** of (item, subjects): `pass` =
demonstrably not reached / not broken, `fail` = demonstrably reached **and** breaking, `error` = the validator
broke, `not-run` = it could not decide, with the reason. **A validator that cannot decide never passes.** No
validator calls a model; agent assertions are never inputs. An undecided record (`not-run`, `error`) echoes
the coverage it was asked about exactly as requested — subjects, paths, rendered changes — next to its own
coverage (`ValidationRecord.EchoRequested`, used here and by the runtime validator). The request stays
linkable to its record, and the audit keeps what was asked. The run record rejects an undecided record that
does not cover its request's subjects. Undecided records never count as clearing coverage.

| Constructor | Name | Kind | Applies | Decides |
|---|---|---|---|---|
| `JoinVerdicts(report, opts…)` | `validate.join@v1` | per verdict | all | wraps the five-class join's decisive verdicts |
| `RenderDiff(opts…)` | `validate.render-diff@v2` | render-diff | all | the environment render pairs: exposure (inconclusive) vs a concrete failure |
| `OverrideInspection(opts…)` | `validate.override-inspection@v2` | override-inspection | all | an explicit setting pinning the old behaviour |
| `SchemaCompat(opts…)` | `validate.schema-compat@v1` | schema-compat | api-schema, compatibility | cluster version range; objects vs the CRD change |
| `StaticReference(opts…)` | `validate.static-reference@v1` | static-reference | all | references in the TO render vs FROM, and along the impact paths |

Options: `WithStore(st)` (read artifact bytes — renders, environment files, the upgrade edge, knowledge facts;
without it a validator that needs bytes is `not-run`), `WithFacts(facts…)` (verified facts the wiring loaded from
`knowledge/`), `WithReportArtifact(id)` (cite the impact report as an input), `WithRelations(relations.WithCatalog(cat))`
(the declared relationship rules the reference checks use; `ri investigate` passes the catalog), `WithClock`. Invalid input (no
bundle, unknown item, a requested path or rendered change that is not the item's) is an error wrapping
`investigate.ErrNotFound` / `ErrRejected`, never a record. Items are looked up with `EvidenceBundle.Item`, so
environment-scope items (`EnvironmentItems`, `scope:rendered-unexplained` anchored on an environment rendered
change) are validated like record items.

**Coverage.** A record speaks for the requested subjects/paths, or — with none requested — for every root and
path of the item. An item-wide result (a silent complete render, a universal pin, an in-range cluster) claims all
of them; an object-level result (schema-compat, static-reference) claims the nodes and paths of the objects it
checked. Render pairs with status `not-applicable` (a target that does not deploy the product) are no render of
it and are ignored; any `partial`/`unavailable` pair makes the render incomplete.

**JoinVerdicts** — at most one record per item, for the item as a whole (no subjects): any `action-required`
finding of the item's change (incl. `impact:render-target-rejects`; a model-consensus fact is labelled as such)
→ `fail`; every finding `not-affected`, deterministic, with checks, from a trusted fact when one is involved →
`pass`; all not-affected but one rests on no render (`impact:values-default-unrendered`, or a render check
`unavailable`) → `not-run`; review-required / informational / unknown or a mix → no record. Record kind follows the
evidence that decided it (render → render-diff, CRD/API/cluster → schema-compat, else override-inspection).

**RenderDiff** — exposure is not failure (DESIGN R1 vs R3; see "Reference and schema checks" below). An
environment rendered change correlated to the item by a rule that ties it to the item's effect (`subject`,
`values-default`, `artifact`, `release-anchor`, or the item is that change), not CRD description text, is
**exposure**: `error` (inconclusive — "exposure: <change> reaches <object>"), never a pass, never a fail by itself,
whatever the item's breaking flag. `fail` only when such a change causes a **concrete failure** in the TO render that
the FROM render did not have: a reference of or to the changed object that dangles, a custom resource invalid
against its rendered CRD schema, an apiVersion the rendered CRD no longer serves — each named in the detail.
`pass` only for a `rendered-config` item when every pair is complete and nothing is attributable — or only CRD
description text, which is shown harmless and cited. For a `controller-behaviour`/`unknown` item render silence
proves nothing: the only pass is **documentation-only coverage** (rule `render-diff:documentation-only`, record field
`documentationOnly: true`, @v2) — every pair complete and every rendered change correlated to the item is CRD
description text in a CRD that changes nothing else in that pair (no schema, default, enum, required, served or
storage change, correlated to anything or not; `EvidenceBundle.DocumentationOnlyChange`). It names those rendered
changes and nothing else (no subject, path or answered hypothesis) and never clears behaviour; a description change
beside any other change of its CRD is `not-run`, naming why. Text-mention correlations decide nothing (`not-run`).

**OverrideInspection** — establishes an explicit environment setting that pins the old behaviour and records
it as the pass's `PinnedSetting` (kind, subject, path, value, the rendered object it lands in, environment
evidence) — the only deterministic validation that clears a behaviour item (`ClearsBehaviour`). Which setting
pins which behaviour is never guessed; it comes from a declarative, release-level **pin rule**, cited in the
record (rule id in the provenance, source in the detail):

| Rule | Source | Setting |
|---|---|---|
| `pin:join-pinned@v1` | the join's own pinned verdict on a computed default change (`impact:values-pinned`, `impact:crd-default-pinned`), with no other non-clear finding of the change | the values keys (every changed key) / the field on every resource; objects only the environment render shows must set it too |
| `pin:knowledge-default-changed@v1` | a **trusted** (deterministic/human), active verified fact attached to the item that states **no applicability**: `change.type: default-changed` of a crd-field, cli-flag, env-var, feature-gate or config-key subject | that subject, set to the old default — or to a value stated identically before and after the upgrade |
| `pin:knowledge-exposure@v2` | a trusted fact whose exposure condition is a conjunction with a settable leaf in state `unset` / `not-equals V` — top level, or inside a `resource` scope (directly or under `all`/`any`) | the explicit setting / one of `V`, on the objects of the scope (below); a leaf on the subject's own setting of a default change also accepts the old default |

A fact that states an applicability is decided by it: its default change alone is never a pin (the fact may say
objects are exposed whatever the setting).

The setting must hold **everywhere it matters**: in every complete environment render target (controller flags,
env vars, feature gates, ConfigMap keys — via `render.Inventory`) and on every object of the kind in the effective
object view that the rule's scope does not provably exclude. A setting that is absent, set only on some objects or installs, set only
after the upgrade to a non-old value, or changed across it is never a pass; neither is any rule from a consensus
or proxy fact, an incomplete render, a partially collected input, or an unparsable environment file that may
declare objects of the kind. Facts come from the bundle's `knowledge-fact` artifacts related to the item and from
`WithFacts` (attached through the report's knowledge findings or `impact.AttachedChanges`).

### Pin rule applicability and the effective object view (Phase 5)

**Scope.** An exposure leaf keeps the `resource` scope it sits in (e.g. "Widgets whose `spec.role` equals
`root` and leave `spec.rotation` unset"). Each object of the kind is decided **three-valued** against the whole
scope, with the pin leaf taking the value the setting gives it: *false* with the setting pinning it → a pinning
record; *false* although the setting does not pin it → **provably out of scope**, excluded from the requirement
and listed (with evidence) in the pass's detail; *true* → a miss (the object is in scope, or in scope through
another condition); *unknown* → undecidable, never excluded. Leaves follow the impact join's semantics
(`internal/impact`): field states over what the object states, Kleene `all`/`any`/`not`, `text-line`, and `ref`
resolving to exactly one object of the view (by name and the kind/group/namespace it states, else the referring
object's namespace). A value predicate on a field the object leaves unset is decided by the rule the impact join
shares (`internal/applicability`, Phase 6 M4): the **schema default** the CustomResourceDefinitions of the bundle
state (upstream `crd` artifacts, environment files, renders; every CRD for the kind must describe the path and
agree, and the parent object must be present), or the rule's own **condition default** (`Condition.Default`:
value, source `controller` | `crd-schema`, citations) when every citation resolves to the fact's upstream evidence;
the two must agree. With neither — no such CRD, a CRD that states no default (the controller may default the field
in code) and no evidenced condition default, an ancestor default, or a schema that does not describe the path — it
is undecidable. A rule may also say what an unset field means with an explicit `unset`/`set` leaf
(`any{F equals X, F unset}`, `all{F not-equals X, F set}`). One deliberate difference keeps exclusion fail-closed: a
reference resolving to nothing (or to several objects) is undecidable.

**Exact coverage (Phase 6 M4, R2.2).** A field pin's pass names the CRD serving the pinned kind
(`PinnedSetting.crd`) and lists the item's environment rendered changes on **its own field**: a change of an
object of the pinned kind at the pinned path, and the `crd-description-changed` text of that field in that CRD (a
field-scoped documentation-only check, `domain.SchemaFieldPath`). The policy re-checks every such claim
(`EvidenceBundle.PinCoversChange`) and counts only confirmed ones: another field's description in the same CRD, a
schema or default change on the field, another CRD version, another resource, or any rendered change claimed by a
flag/env/config-key pin stays a gap. An `undecidable` leaf is unknown. If every visible
object is out of scope, nothing is pinned (whether the environment is exposed at all is the join's decision).

**Effective object view.** The objects are what the environment actually applies: the to side of every complete
render of the product (from side for the value before the upgrade), the render of every **version-independent
overlay** (a `not-applicable` kustomize pair: it renders the same objects at both versions), and the environment
files **no render consumed**. Which files a kustomize render consumed is read from the bundle's own kustomization
files (`resources`, `bases`, `components`, `crds`, `patchesStrategicMerge`, `patches[].path`,
`patchesJson6902[].path`, recursively) — so a base object and the overlay patch that sets the pinned field are seen
as one merged object, never as an unpatched base plus a patch. A file consumed by a render whose objects are not
available (render failed) is undecidable. Every hit, miss and exclusion names the view the object came from
(`environment render <target> (to)`, `environment render <target>, version-independent overlay`,
`environment file <path>, no render consumed it`); a pass lists the files it saw through renders.

**Trust and lifecycle.** Only active facts whose weakest aspect is deterministic or human pin. A fact pins only
when its `release` lies inside the transition (`from < release ≤ to`, same product); a fact that states no release
never pins. When an active fact inside the transition about the same subject (any level, attached or not) states a
different change or applicability than a pin-source fact, nothing pins for the item: `pin:contradicting-facts`.

Reason codes of a `not-run` override inspection (provenance rule): `pin:no-rule` (no rule names a setting —
includes untrusted, inactive and out-of-transition facts, each noted), `pin:not-established` (a rule's setting
does not hold everywhere), `pin:applicability-undecidable` (a rule failed only on objects whose scope membership
cannot be decided), `pin:contradicting-facts`.

**SchemaCompat** — compatibility items: the snapshot's Kubernetes version against the target constraint
(`upgrade.EvaluatePlatformConstraint`; anchors `<platform>` or `<platform>/<kind>`): inside → `pass`; outside a
supported/minimum/chart-kubeVersion range → `fail`; outside a *tested* range, other platforms, no cluster version →
`not-run`. API-schema items anchored on a computed CRD change (`impact.CRDIdentity` scopes the kind exactly as
the join): every visible object of the kind (environment files + environment to-renders) against the change —
removed CRD/version still used, removed field set, removed enum value used, newly required field omitted (parent
present), value of the old type → `fail`; all compatible with complete inputs → `pass` for those objects. Default,
storage and additive changes → `not-run`; no visible object → `not-run` (absence in a repository is not proof).

**StaticReference** — references the upgrade breaks. In the renders: a reference of or to an object the item's
rendered changes touch (`subject`/`values-default`/`artifact`/`release-anchor` correlations, the anchoring change,
or the requested ones) that resolved at FROM and dangles at TO, or that is new at TO with nothing providing its
target → `fail`, naming referrer, field path and target kind/namespace/name. On the item's paths (the graph): an
edge whose provider the TO render no longer renders while its consumer survives unchanged → `fail`; every edge
established, no placeholder, no provider removed, and no reference of a node on the path newly dangling or
undecidable → `pass` for those paths. Unresolved edges, placeholders, an undecidable reference on the path, a dangle
caused by no change of this item, a truncated traversal, an incomplete render or a root-only path → `not-run`. The
property is references only: it never clears a behaviour item and never passes a `scope:rendered-unexplained`
item.

### Reference and schema checks (exposure vs failure)

Lane `refcheck`: two environments with the same rendered change must get different classes when the
consequences differ. Files `validate/refcheck.go`, `validate/crdschema.go`; both RenderDiff and StaticReference
use them (they need `WithStore`; without it RenderDiff's exposure stays inconclusive and says the checks did not
run).

- **Views.** `relations.NewViewer(store, opts…).View(ctx, bundle, side)` reads one side of the upgrade — the FROM
  or TO render of every complete environment pair plus the environment files (one commit: the same at both
  sides) — with the graph builder's reference extraction and namespace semantics, and resolves every reference:
  `resolved` (an object of the side, a declared producer's object, or `ServiceAccount default` /
  `ConfigMap kube-root-ca.crt`, which Kubernetes provides in every namespace), `dangling` (nothing at the side
  provides it), `unknown` (a name-only match, a namespace that is not known, a controller only presumed by
  convention to create it, an `optional: true` selector or an image pull secret, a name the render withheld because
  its key looks like a credential). A view has no nodes for objects of rendered changes, so a reference to a
  removed object dangles there.
- **Namespace semantics.** A name resolves in the referrer's effective namespace (declared, else kustomize's or
  the deploying Application's); a cluster-scoped kind (built-in, declared `scope: cluster`, or a CRD with
  `scope: Cluster`) in none; an explicit namespace where it says (`namespace/name` formats, a `*Ref`'s
  `namespace`). A Secret of the same name in another namespace does not provide it.
- **FROM vs TO.** A dangling TO reference is judged by what the FROM render says about its **target**
  (kind/namespace/name — not the field or list position, which an upgrade that removes a list element shifts):
  some FROM reference resolved it → *regressed*; FROM references to it dangled too → *pre-existing* (the
  environment provides it outside the snapshot — a hook Job or a controller creates it at runtime — or it was
  already missing; not the upgrade's doing, never a failure); no FROM reference names it → *new*; undecidable at
  either side → *unknown* (decides nothing). Regressed and new dangles are failures.
- **CRD schema.** A custom resource the item's changes touch — or every custom resource of a CRD they touch —
  is checked against the CRD the same side renders (or the environment files define), `apiextensions.k8s.io/v1`
  only: its apiVersion defined and served; its body against the version's structural schema (types incl.
  `x-kubernetes-int-or-string`, `enum`, `required`, fields the schema does not know — rejected under strict field
  validation, pruned otherwise; `additionalProperties`, `x-kubernetes-preserve-unknown-fields` and embedded
  resources respected; subtrees under `anyOf`/`oneOf`/`allOf` without properties are not checked). Only
  violations the same object did not have at FROM count. A kind whose CRD is not in the view decides nothing.
- **Classify and `impact:not-joined`.** The join's `impact:not-joined` verdict (`release-knowledge-gap`: a declared
  change, a diff rule without a join rule, a change without subjects) says what the join does not evaluate, not what
  the environment lacks. It no longer blocks R2 by itself: an item whose own evidence meets R2.1–R2.8 (for a behaviour
  item, pinned settings or observed passes over every path, root and rendered change) is cleared; on any finding
  that is not cleared the gap and its next step stay listed. Every other unknown join verdict still blocks R2.

Tests: `go test ./internal/investigate/validate` (synthetic bundles of a fictional product; every output is
checked with `Investigation.ValidateAgainst`). Real bundles from the groundtruth inputs (bundle + renderdiff +
relations, offline): `RI_VALIDATE_STATE=<warm .ri> go test ./internal/investigate/validate -run Groundtruth -v`
(`RI_VALIDATE_VERBOSE=1` logs every record).

## Removed and changed defaults the environment relied on

A removed (or default-changed) values key the environment leaves **unset** is not "nothing takes effect on
you" when the source chart's default for it was non-empty: that default was in effect for exactly this
environment. Two generic rules keep such an item away from `not-affected` (lane `defaults`):

- **Join** (`internal/impact`, `impact:values-removed-default`): a removed key left unset whose source
  default (`upgrade.FromDefaults`, read from the values diff) is non-empty — or not recorded because a
  section listing was truncated — is review-required, with the values files' absence of the key as the
  environment chain. Never action-required from this rule; an empty default (`null`, `""`, `{}`, `[]`)
  keeps `impact:values-unset`. Default-changed keys left unset stay with PO-3 (the counterfactual render
  decides; `values-default-unrendered` is a gap for `validate.join`, never a clearing pass).
- **Correlation** (`internal/render`, rule `values-default`): an environment rendered change whose before
  holds the key's source default (by value or redaction digest, under a field named like the key, in an
  object the key's top-level section names) and whose after no longer does is correlated to the key's
  item. It is a strong correlation: `validate.render-diff` cannot pass the item on render silence, and the
  change is no longer an unrelated `scope:rendered-unexplained` item.

## Runtime validation

Package `internal/investigate/runtime` (MISSION §A5 last bullet, DESIGN §2.6 / §3.1 R1–R2). **Opt-in only**
(`--runtime` in the wiring; never part of a default run). One `investigate.Validator`,
`runtime.New(opts…)`, name `runtime.isolated-upgrade@v1`, kind `runtime`, level **observed**. It exists for the
breaking changes a render cannot show — behaviour defaults compiled into the new controller binary — which
nothing deterministic can confirm as affected, and which only a trusted pinned setting can otherwise clear.

**What one run does** (once per bundle; every item's record is derived from it; one cluster at a time):

1. **Manifests.** Only complete render pairs are usable: any `partial`/`unavailable` environment pair ⇒ every
   record `not-run`. `ok` pairs deploy the product (FROM and TO renders); `not-applicable` pairs are
   version-independent consumer overlays whose FROM render is applied at both versions. The stored renders are
   normalized (empty maps dropped, credential-like values replaced by digest markers) — right for diffs, wrong for
   `kubectl apply` — so the validator **re-renders** each target through the same path (`runtime.NewAppRenderer(app)`:
   `app.RenderTargets` + the render engine and its cache) and **proves** the raw re-render normalizes to exactly the
   stored artifact (`render.ParseObjects` vs the artifact, object by object; nondeterministic objects tolerated)
   before applying anything. The raw bytes go to the throwaway cluster and are never stored.
2. **Preflight** (each failure ⇒ `not-run` with the reason, no cluster): `kind`, `kubectl`, `docker` on PATH and
   the daemon reachable; free disk ≥ 10 GiB (`WithMinFreeDisk`); a pinned node image for the environment's
   Kubernetes version (`kind` v0.24 images for 1.25–1.31, `WithNodeImage` overrides; no version stated ⇒ kind's
   default); **no other `ri-runtime-*` cluster exists** (one at a time, also across processes; a leftover is named
   with the command to delete it).
3. **Cluster** `ri-runtime-<8 hex derived from the bundle id>` (kind, own kubeconfig file; `~/.kube/config` untouched).
   **Deleted on every exit path** — success, error, timeout, cancellation (a fresh bounded context) — and verified
   absent with `kind get clusters`; a cluster that could not be deleted turns the run into `error` with the
   `kind delete cluster` command in every record.
4. **FROM**: namespaces, CRDs (until established), then — in Helm's install order, coarsened: service accounts,
   RBAC, config, services before workloads, jobs, custom resources and webhooks — every object by server-side apply (field manager
   `ri-runtime`, conflicts forced — a GitOps sync), retrying transient errors (webhooks not serving yet, CRDs not
   established) within a bound; wait for readiness (workloads, jobs, CRDs, objects with a Ready/Available
   condition); settle; **baseline observation**. The product's own workloads must be ready, else the run is
   `error` ("no baseline").
5. **Exercises at FROM** (see below) on every target; settle; observation.
6. **TO — the upgrade**: objects the new release no longer renders are pruned (as `helm upgrade` does), Helm
   hooks re-created, everything applied; after readiness **every object is applied once more** (a sync of the
   unchanged desired state: admission webhooks of the new controller re-validate existing objects); settle;
   observation. The upgrade *completed* when every object of the product's targets applied and every product
   workload is ready.
7. **Exercises at TO**; settle; **after observation**.
8. Bounded wall clock: `WithTimeout` (default 10 min, cluster creation to the last observation; deletion has its
   own bound); the run record states the duration. Pulled images stay in the node's cache only.

**Observations are generic** (no product Go code), per object: existence and apply/admission errors, status
conditions, readiness and pod problems (waiting reasons, restarts), generation vs observedGeneration,
Secret/ConfigMap data **digests per key (never values)** — HMAC-SHA256 under a random per-run key that is never
stored, so digests compare within a run and cannot be matched against a guessed value —, Warning events since the window started, and
error-level log lines of the product's controller pods that name the object (`<ns>/<name>` with name
boundaries). Observed objects are every applied object plus every Kubernetes object the relationship graph names
(e.g. the Secret a Certificate's controller produces); graph keys whose name is a redaction marker are resolved
against live names by recomputing the marker.

**Degradations** (value-free *signatures*: `rejected`, `deleted`, `condition:<type>`, `not-ready`,
`not-reconciled`, `restarts`, `data:<key>`, `warning:<reason>`, `log:<line>`):

- **upgrade window** — the last FROM observation → TO. An object the new release no longer renders being gone
  is the upgrade itself, not a degradation; a data change the render declares (the object's rendered data differs
  FROM → TO) is the render diff's business; data changes count only under a consumer (an outgoing graph edge);
  warnings/log lines already seen at FROM are explained.
- **exercise window** — what the exercises did at TO (TO → TO-exercised) that **the same exercises did not do at
  FROM** (FROM → FROM-exercised). A renewal that rotates a certificate at both versions is explained; one that also
  rotates the private key only at TO is a difference (`data:tls.key` under the consuming Deployment).
- **A data change is not a failure by itself** (commander decision 2026-10-16, replacing the brief's "referenced
  data changed under a consumer ⇒ fail": MISSION's Affected needs evidence of an incompatible impact, and a `fail`
  must name a concrete failure). Data differences are marked *inconclusive* and name the consumers (graph
  successors) and whether any of them degraded. An item whose only degradations are such data changes gets
  `error` / `runtime:inconclusive-change` ("data changed under consumer X; no degradation observed"): a plausible-impact
  signal (R3), never a clearance. It fails only through a concrete degradation — e.g. the consumer itself becoming
  not ready — which then lists the data change as context.
- an exercise the TO version **refuses** on an object the FROM version accepted it on (an admission webhook
  denying the update) is a degradation of that object (`exercise-rejected:<exercise>`).
- an exercise the TO controller accepts **without the declared reaction** it showed at FROM is an inconclusive
  difference (`exercise-unanswered:<exercise>`: a controller that stopped acting on the object, or a slow one).
- an image that cannot be pulled is the sandbox's problem (`Infrastructure`), never a failure.

**Exercises** — `products/exercises/<product>.yaml` (kind `ProductExercises`, `internal/catalog/exercises.go`,
schema `schemas/product-exercises.schema.json`): declared, upstream-documented triggers, each with a target kind,
an action (`status-condition` through the status subresource, `annotate`, `rollout-restart`), the change-item
`families` it exercises (**required**: an exercise covers only the families it names), the controller's observable
**reaction** (`field-changed` + path, or `event` + reason), an optional settle window and ≥1 https upstream reference with a verbatim quote (never
eval data). They live beside the product definitions, not inside them, so the definition digest that pins ingested
releases is untouched. All files load catalog-wide (`catalog.LoadExercises` → `runtime.FromSets`); an exercise
runs when the environment has an object of its target kind. **An exercise counts as run on an object only when the
controller showed the declared reaction there** (checked after the settle window): an accepted patch proves only that
the API server stored it. An exercise without a declared reaction can reveal failures (a refusal, a rejection Event)
but never supports a pass. Declared today: cert-manager `certificate-renewal` (Issuing=True, ManuallyTriggered —
what `cmctl renew` does; reaction: `status.revision` changes, set on issuance), ingress-nginx `ingress-update` (an
Ingress update the admission webhook re-validates; reaction: the controller's `Sync` event), prometheus-operator
`servicemonitor-update` / `podmonitor-update` / `prometheusrule-update` (the operator re-reconciles and rejects
invalid resources with an Event; **no reaction declared** — upstream documents none for an update that leaves the
generated configuration unchanged — so they never support a pass).

**Per item** (rule = provenance rule; coverage = the requested subjects/paths — subjects only with a graph to check them against, else the request is rejected — or every root and path of the item):

| Outcome | Rule | When |
|---|---|---|
| `not-run` | `runtime:run-not-run` | the run had a missing precondition (above) |
| `error` | `runtime:run-error` | the run broke (cluster creation, no baseline, timeout, cluster not deleted) |
| `error` | `runtime:unreached` | nothing on the item's roots/paths is a cluster object, but an exercise declared for its family degraded its targets at TO only — a plausible impact the graph does not connect to any one item (never a fail, never a pass) |
| `not-run` | `runtime:no-object` | nothing on the item's roots/paths is a Kubernetes object of the cluster (and nothing above) |
| `fail` | `runtime:degraded` | a concrete degradation on the item's paths **attributed to it** (`attribution:own-rendered-change` or `attribution:unique-path`, below); names each concrete failure, cites the observation artifact and the item's rendered changes on the degraded objects |
| `error` | `runtime:inconclusive-change` | its only (attributable) degradations are inconclusive differences — data changed under consumers with no consumer degrading, or an exercise left unanswered at TO |
| `error` | `runtime:sandbox` | the sandbox could not run an object on the paths (image pull) |
| `error` | `runtime:unattributed` | a degradation on its paths not attributed to it (`attribution:shared-own-change`, `shared-path` or `reconciled-only`) — inconclusive (R3), never a fail |
| `not-run` | `runtime:other-families` | its objects degraded only under an exercise declared for other families |
| `error` | `runtime:upgrade-incomplete` | the upgrade did not complete (a product object rejected, a product workload not ready) |
| `not-run` | `runtime:no-baseline` | an object on its paths was not observed healthy in every observation — FROM, after the FROM exercises, TO, after the TO exercises (absent before, removed by the upgrade, not ready, unserved) |
| `not-run` | `runtime:no-exercise` | **no exercise declared for its family with a target on its paths: a quiet window is never a pass** |
| `not-run` | `runtime:exercise-not-run` | its exercise was not applied, or the controller showed no declared reaction, on its objects at both versions (or the exercise declares no reaction) |
| `not-run` | `runtime:reconciled-only` | everything above held, but the item reaches its objects only through the `exposure:product-reconciles` rule (no rendered change or join match of its own): the exercises show the controller still handles its objects, not that this particular change was exercised |
| `pass` | `runtime:healthy-exercised` | the upgrade completed, every object on its paths was healthy in each observation (the record lists them), and its exercise was applied with the controller's declared reaction observed at both versions, degrading nothing; covers the item's rendered changes on those objects |

**Attribution precedence** (generic; the rule id is recorded on **every** record after `+` in its provenance rule,
e.g. `runtime:degraded+attribution:own-rendered-change`; `runtime.OutcomeRule` / `runtime.AttributionRule` split it).
For each degradation on an item's objects, among the in-scope items that reach the object (for an exercise-window
degradation: whose exercise ran on their paths):

1. `attribution:own-rendered-change` — the item has its **own rendered change** (environment or release scope,
   correlated to it) on the degraded object, the degradation concerns that object (rejected, refused exercise,
   deleted, not ready, not reconciled, a failure condition, a warning event — not a data change, restarts or a log
   line), and no other reaching item has one ⇒ `fail` for it. With **two or more** such items: all
   `attribution:shared-own-change` (inconclusive, the items named — no guessing); every other reaching item:
   `attribution:shared-path` naming the item it is attributed to.
2. otherwise, items reaching the object through **relationship paths** (roots from a rendered change or a join match):
   the only one ⇒ `attribution:unique-path` (`fail`, or `inconclusive-change` for a data-only difference); several ⇒
   `attribution:shared-path` (inconclusive).
3. items reaching it **only as a reconciled kind** (`exposure:product-reconciles` roots) ⇒ `attribution:reconciled-only`,
   always inconclusive.

Also: `attribution:other-families`, `attribution:unreached`, `attribution:sandbox`, and `attribution:none` (no
degradation on the item's objects, e.g. every pass).

A runtime pass is level `observed`, so it `ClearsBehaviour()` (R2.4); classify still applies R2.1–R2.8. Records cite
the applied render artifacts and the exercise declarations (`derived/runtime/<bundle>/exercises.json`) as inputs,
and `run.json` (phases, apply errors, exercises, upgrade problems, degradations, cleanup) and `observations.json`
(the four observations) as outputs (`validation-output` artifacts, returned in `ValidationOutput.Artifacts`).

**Wiring** (`ri investigate --runtime`, `ri worker --runtime`, `ri eval -investigations -runtime`;
`cmd/ri/investigate.go`): `runtime.New(runtime.WithStore(st), runtime.WithRenderer(ar),
runtime.WithExercises(runtime.FromSets(sets…)…))` with `ar := runtime.NewAppRenderer(app)`, `ar.Repo` = the
environment checkout (the snapshot is checkout-independent), `sets` from
`catalog.LoadExercises("<products>/exercises")`, and `runtime.isolated-upgrade@v1` added to `Config.Validators`
(so the option is part of the investigation id). `Run(ctx, bundle, graph)`
performs the run explicitly (e.g. as its own activity, heartbeating); `Validate` reuses it per item, so an agent's
`request_validation` of kind `runtime` is answered from the same run.

Tests: `go test ./internal/investigate/runtime` (a fake driver and an in-memory cluster simulating a fictional
product: every outcome path, "quiet window without exercise ⇒ never pass", cleanup on every exit path, observation
diffing). Real runs on groundtruth inputs (one kind cluster at a time, minutes per case):
`RI_RUNTIME_STATE=<warm .ri> RI_RUNTIME_CASES=<case>[,<case>] [RI_RUNTIME_TIMEOUT=20m] go test
./internal/investigate/runtime -run Groundtruth -v -timeout 60m`.

### Admission dry run (`runtime.admission-dry-run@v1`, Phase 6 Milestone 4)

Does the **target version's validating admission webhook** accept one environment object? Static evidence can leave
this open (a parser's actual character class, a validation the documentation does not state); the API server's answer
settles it. **Opt-in**: its own command, never part of an investigation run.

```
ri investigate admission -product ingress-nginx -version 1.12.1 -object ingress.yaml \
    [-setup refs.yaml] [-values controller-values.yaml] [-namespace ns] [-exercise ID] [-kubernetes 1.31] [-timeout 15m] [-json]
```

1. The product's controller is rendered at the target version (chart defaults; the product's verification fixture's
   `controller.values` when declared; `-values` instead) and installed with the isolated upgrade's apply and readiness
   rules into a throwaway cluster `ri-admission-<8 hex>` (same preflight: tools, free disk, the pinned node image, one
   isolated cluster at a time; same budget; deleted on every exit path, deletion verified). `-setup` objects (what the
   submitted object refers to) are applied for real; the object's namespace is created.
2. The validating webhooks whose rules cover a `CREATE` of the object's resource (from discovery) are listed with their
   selectors evaluated (`namespaceSelector`, `objectSelector`: matchLabels and In/NotIn/Exists/DoesNotExist;
   `matchConditions` are not evaluated, so such a webhook is not counted) and waited for until their service has a
   ready endpoint.
3. The object is submitted once as `kubectl apply --server-side --dry-run=server` (nothing is stored), retried while
   the webhook cannot be called (connection refused, certificate not yet injected, …).

The record (`AdmissionRun`, level **observed**, stored as `derived/runtime-admission/<id>/admission.json`):

| outcome | when |
|---|---|
| `accepted` | the API server admitted the dry run with at least one **effective** covering webhook in its path: its service had a ready endpoint, or its failure policy is `Fail` (a failed call rejects, it never silently admits) |
| `rejected` | `admission webhook "<name>" denied the request: …` (`rejectedBy: webhook <name>`), or the API server's own validation (`is invalid`, `Invalid value`; `rejectedBy: api-server`, before any validating webhook), with the full message |
| `undetermined` | no effective covering webhook (acceptance would show only the API server's own validation), a covering webhook whose `sideEffects` refuse dry runs, an answer that is neither a decision nor a validation failure after the retries, or a run that did not complete |

Exercises of action `admission-dry-run` are declared in `products/exercises/<product>.yaml` (target kind, families,
upstream references; no reaction — the outcome is the API server's answer); the isolated upgrade never runs them.
ingress-nginx declares `ingress-admission` (Ingress). Tests: fake driver paths in `admission_test.go`; real kind
fixtures (a stdlib webhook compiled into a `FROM scratch` image, clusters `ri-semverify-*`, accepted and rejected):
`RI_ADMISSION_KIND=1 go test ./internal/investigate/runtime -run AdmissionKind -v -timeout 20m`.

## Relationships

Secondary supporting context (the rendered diff is the centerpiece). Package `internal/investigate/relations`
answers one question for the agent and the policy: **which consumers reference this changed object, and how well
do we know it?** Design: DESIGN.md §2.5.

**Graph** (`relations.NewBuilder(store, opts...)`, producer `relations.builder@v1`). Nodes come from the TO
render of every complete environment pair, the environment's YAML files, and every object an environment rendered
change names (so a removed object can still be a root). Edges point in the direction impact propagates (provider →
consumer):

| Edge | From → to | Source |
|---|---|---|
| `references` | Secret / ConfigMap / ServiceAccount → workload | pod template volumes (incl. projected), `env`/`envFrom`, `serviceAccountName`, `imagePullSecrets` |
| `references` | Service / Secret / IngressClass → Ingress | backends, `defaultBackend`, `tls[].secretName`, `ingressClassName` / class annotation |
| `references` | referenced object → custom resource | generic `*Ref` fields `{name, kind[, group, namespace]}`; Flux `valuesFrom` |
| `selects` | workload → Service | `spec.selector` ⊆ pod template labels |
| `serves-api` / `consumes-api` | CRD → api-type → custom resource | CRD `spec.versions`, object `apiVersion`/`kind` |
| `produces` | custom resource → object it creates | declarative `ProducesRule`, else the `spec.secretName` convention |
| `renders-to` / `deploys` / `configures` | Application, kustomization, render source → objects; values file → Application | Argo `source.path`, `$values/…` value files, kustomize `resources`, generators, render targets |

**Consumer mode** (`relations.WithConsumers()` / `relations.BuildConsumerGraph(ctx, store, bundle, side, opts…)`,
producer `relations.builder@v1+consumers@v<rules>`; Phase 6 Milestone 1, `docs/phase6/decision-intel/M1-IMPLEMENTATION.md`).
The same graph plus: on every Secret/ConfigMap volume `references` edge, `mount` (volume, projected source, items
key → path, every container mount with `mountPath`/`subPath`); on `references` and `produces` edges, `line` (the
artifact line that states the field); and **`reads-file`** edges, mounted Secret/ConfigMap → the ConfigMap whose
configuration (mounted, or read through `configMapKeyRef`/`envFrom`) names an absolute path under that mount in the
same container (`file`: config key, path, workload, container, mount, the key it maps to; `line` + text). Generic
path matching only; a path under no mount, under a volume that is not a Secret/ConfigMap, or to a key the object
does not hold is an unresolved `reads-file` edge (an `external` placeholder `file:<workload>#<container>:<path>`),
never dropped. Generated ConfigMaps are read from their `configMapGenerator` `files`/`literals`/`envs`.
**Consumer rules** (`relations.WithConsumerRules`): v3 (the default) also matches the absolute paths a container's
own spec names (each `command` and `args` string, shell scripts passed to `sh -c` included, and literal `env[].value`
strings; Kubernetes `$(VAR)` references are masked like `${VAR}`) against the same container's mounts: `reads-file`
edges provider (or unresolved placeholder) → the **workload**, with `file.via` `pod-spec`, `file.field` (e.g.
`spec.template.spec.containers[0:app].command[2]`) and the `line` that names the path; rule
`graph.reads-file@v3:pod-spec:{command,args,env}`. `valueFrom` entries stay `references` edges; `command[0]` under no
mount (the program, from the image) is not taken for a file. v2 also takes a generated ConfigMap's data from the
generated object of a render of the same side when the snapshot lacks a generator source (the snapshot collects
YAML files only): the render of an overlay whose kustomization tree includes the generator, the object named as
kustomize names it (tree `namePrefix`/`nameSuffix`, then the content-hash suffix unless disabled), in the namespace;
the edge's `file.dataFrom` names that object and its `line` cites the render. Data no source holds (no render, or
renders that disagree) and a ConfigMap the snapshot does not define become unresolved `reads-file` edges with an
empty `file.path` (`external` placeholder `data:<workload>#<container>:<configmap>[<key>]`) that `trace_consumers`
and the dossier report as gaps. `secretGenerator` keys are known from the generator; a key of a generated Secret
whose keys are unknown is `inferred`. v1 is the Milestone 1 graph; v1 and v2 are kept, byte for byte (golden graphs
in `internal/investigate/relations/testdata/consumer-rules/`), so transcripts recorded under them replay: a
v9 session writes `consumerRules` into its transcript header, and a replay uses the rules its transcript names
(none: v1). The pipeline never builds this mode: its graph, traversal and every policy input are unchanged. Prompt
v9's agent session builds it (below).

Kustomize `namespace:` (outermost wins) and an Application's `destination.namespace` are applied so references
resolve; kustomize patch files are not objects; `configMapGenerator`/`secretGenerator` outputs are nodes. The same
extraction and resolution, per side and without the graph, is `relations.NewViewer(…).View` (see "Reference and
schema checks" under Validators).

**Status = how well a relationship is known.** `established` is an explicit name reference or resolved selector in
the snapshot. A reference to something the snapshot does not contain becomes a **placeholder node** and a `missing`
edge (reason stated) — never dropped. A name-only match (a namespace unknown on either side) and the generic
`spec.secretName` produces-convention are `inferred`. A declared `produces` relationship (below) makes the produces edge and the reference through it `established`, so a
path Certificate → Secret → Deployment can be complete.
Unresolved (inferred/missing) edges can only add paths that need validation: a path through one leaves the
traversal incomplete, which blocks `not-affected` (fail closed).

**Exposure** (`relations.NewExposure()`): the roots of an in-scope change item are the graph nodes of the
environment rendered changes correlated to it (rule `exposure:rendered-change`), then the nodes the deterministic
join's matches name (`exposure:join-match`; a subject is a node key, `Kind/name` or `namespace/Kind/name`), then —
for a `controller-behaviour` or `unknown` item only, whose effect no render can show — every environment object
(not a placeholder) of a kind the product's controller **reconciles** (`exposure:product-reconciles`, option
`relations.WithReconcilesFrom(catalog)`). One root per (item, node), the first rule wins; an item with no signal
has no root.

**Reconciled kinds** — `reconciles:` in `products/<id>.yaml` (`internal/catalog/reconciles.go`, schema
`product-definition.schema.json`): entries `{id, kinds: [{group, kind}], summary, references}`, each citing
upstream documentation with a verbatim quote (https, never eval data; a kind is declared once per product).
Declared: ingress-nginx (Ingress), cert-manager (Certificate, CertificateRequest, Issuer, ClusterIssuer),
prometheus-operator and kube-prometheus-stack (the operator's ten custom resource kinds). Release-level facts —
"the controller this product ships acts on every object of these kinds" — so a change compiled into the
controller has an established exposure in every environment that declares such objects; the paths from those
roots follow the ordinary edges (a Certificate's Secret, the Deployment mounting it).

**Traversal** (`relations.NewTraverser()`, `graph.dfs@v1`): roots sorted by (item, node id); outgoing edges by
(kind, to-node id, edge id); simple paths only; a maximal path is emitted when no outgoing edge leads off the
path, so every edge that some simple path from a root can take lies on an emitted path (an edge that would
re-enter a node already on the path is on none). Bound `{MaxDepth 8, MaxPaths 500, MaxNodes 5000}` by default;
hitting any bound sets `Truncated` and a reason (`max depth N reached`, `max paths N reached`, `max nodes N
reached`) and the traversal is not complete. Path status is the weakest edge status; `Unresolved` lists exactly the
non-established edges on paths. Same graph, roots and bound ⇒ byte-identical result (input order does not matter).

**Declared relationships** (`relationships:` in a product definition, `internal/catalog/relationships.go`,
validated by `ri catalog validate`/`catalog.Validate`, schema `schemas/product-definition.schema.json`). Object
kinds relate in ways field semantics cannot show, so the catalog states them, each citing upstream API/CRD
documentation with a verbatim quote (never an environment or an evaluation case):

```yaml
relationships:
- id: certificate-secret
  kind: produces            # produces | references | selects
  from: {group: cert-manager.io, kind: Certificate}
  to: {kind: Secret}
  path: spec.secretName     # dotted keys; a key with dots/slashes in brackets: metadata.annotations[a.b/c];
                            # `[]` after a key = every list element: spec.endpoints[].tlsConfig.keySecret.name
  format: name              # or namespace/name, or '[namespace/]name' (either; default name = namespace of `from`)
  scope: namespaced         # or cluster (scope of `to`)
  whenPath: spec.issuerRef.kind   # optional condition on `from`; with whenEquals
  summary: ...
  references: [{url: https://…, quote: "verbatim, ≤400 chars"}]
- id: servicemonitor-services
  kind: selects             # to: Service (metadata.labels) or Pod (pod-template labels of workloads)
  from: {group: monitoring.coreos.com, kind: ServiceMonitor}
  to: {kind: Service}
  selectorPath: spec.selector                 # a LabelSelector: matchLabels + matchExpressions
  namespaceSelectorPath: spec.namespaceSelector   # {any, matchNames}; default: the selector's own namespace
```

`produces` makes the edge from → to (status `established`; a missing `to` object becomes a placeholder explained by
its producer); `references` links `to` → `from`; `selects` links the selected object → the selector. A field that
does not hold the documented form (e.g. no `namespace/name`) yields no edge. `relations.NewBuilder(store,
relations.WithCatalog(cat))` applies the relationships of every product in the catalog (rules of products the
environment does not use match no object); `WithRelationships(product, rs...)` and `WithProducesRules` add rules
directly. Edges carry the rule id `relationship:<product>/<id>`. Shipped: cert-manager (Certificate secret,
Issuer/ClusterIssuer CA secrets, `issuerRef` to a ClusterIssuer), ingress-nginx (`auth-secret`,
`auth-tls-secret`, `proxy-ssl-secret` annotations — `auth-secret` as `[namespace/]name`, the other two as
`namespace/name`, as the controller parses them), prometheus-operator (ServiceMonitor, PodMonitor selectors; the
Secrets/ConfigMaps of every endpoint — `authorization.credentials`, `bearerTokenSecret`, `basicAuth`
username/password, `tlsConfig` ca/cert/keySecret — in the monitor's own namespace). A ClusterIssuer reads its CA
Secret from the controller's `--cluster-resource-namespace` (`namespaceFrom`, below). A rendered Ingress's
`…-secret` annotation values are withheld by the render (credential-looking key), so those references are
`unknown` in a view (the graph keeps a placeholder); in environment files they resolve. A Certificate whose
`issuerRef` omits `kind` (default Issuer) is not followed (no false dangle; not checked).

**Cluster-scoped name references** (`namespaceFrom`, Phase 5 lane coverage). A cluster-scoped object that names a
namespaced one without a namespace is resolved where its controller documents it, when the rule says so:

```yaml
- id: clusterissuer-ca-secret
  kind: references          # format name only; declaring namespaceFrom makes `from` cluster-scoped
  from: {group: cert-manager.io, kind: ClusterIssuer}
  to: {kind: Secret}
  path: spec.ca.secretName
  namespaceFrom:
    container: cert-manager-controller   # the controller container in the environment's workloads
    arg: --cluster-resource-namespace    # its value may use $(VAR) from the container's env
    default: kube-system                 # documented value when the flag is absent (omit if none)
    configFileArg: --config              # a config file that can also set it (the flag wins)
    references: [{url: https://…, quote: "…"}]   # required
```

The graph (and the per-side reference view) decides the namespace from the environment at that side: every
container of that name in the environment's workloads, the flag's value (`--flag=v` or `--flag v`; `$(VAR)` expanded
from a literal env value or `fieldRef: metadata.namespace` = the workload's namespace), else the default when no
`configFileArg` is given. Decided ⇒ the name resolves in that namespace (`established` when the object or a declared
producer is there, `missing` when not — never a name-only match), and the edge cites the controller render. **Fail
closed**: no citation, no such container, an env var from `configMapKeyRef`/`secretKeyRef`/`envFrom` or undefined,
a config file with no flag, a flag without a value or given twice differently, a non-namespace value, or containers
that disagree ⇒ the reference stays matched by name only (`inferred`), and the edge reason says what was not decided.

Limits (milestone 1): no NetworkPolicy/Gateway-API selectors, no live observation, no values-file `configures` for
Helm values outside Argo `$values/` references; cluster scope of a kind is known only for built-in kinds, declared
`scope: cluster`, a declared `namespaceFrom` (its `from` kind) and CRDs in the snapshot.

### Policy v2: what a behaviour item needs besides its pin (Phase 5, lane coverage)

`classify.policy@v2` changes one thing: a documentation-only render-diff pass (`documentationOnly: true`, see
RenderDiff) accounts for the CRD description changes it names in **R2.2** ("every correlated rendered change shown
harmless"), whatever the item's manifestation. Before, a pinned `controller-behaviour` item whose only correlated
rendered change was the reworded description of its field stayed `potentially-affected` forever (an override
inspection names no rendered changes; render-diff refused behaviour items). The classifier re-checks the claim against
the bundle (`EvidenceBundle.DocumentationOnlyChange`) and the run record rejects a false one. Such a record never
counts for R2.4 (paths/roots/"a passing validation exists"), never answers an agent concern (R2.6) and never clears
behaviour: a pin or an observed outcome is still required. Agent output is still never an input. A run asking for
`classify.policy@v1` is classified as v1 (its validators never emitted documentation-only records); a
documentation-only record under v1 is refused. The other coverage change — cluster-scoped name references resolved
through a documented controller argument (`namespaceFrom`, under Relationships) — needs no policy change: it turns
an inferred edge into an established or missing one only when the environment decides it (R2.3).

## Investigation agent

The agent stage (DESIGN.md §4; code `internal/investigate/agent`, CLI `ri investigate mcp`). It is
**opt-in** (`AgentConfig.Enabled`, `--agent`). When it is on, environment configuration from the bundle is
sent to the configured model provider. Kubernetes `Secret` values and credential-named values are redacted
at collection, before anything is hashed or served (see "Evidence bundle"), and sensitive rendered values
are never recorded. Whether provider data sharing beyond that is acceptable is still an open question for
the user, so keep the stage off for real customer environments until it is answered. When the stage is off,
the run records `agent: not-run` and nothing is sent anywhere.

**What the agent can do.** It works through 13 tools, all scoped to one investigation. It never receives
the bundle itself, only the manifest it can search.

| Tool | |
|---|---|
| `list_renders`, `query_diff`, `get_rendered_object` | the rendered diffs: the primary evidence (lane renderdiff's query engine) |
| `get_change_item` | one item with its upstream change text, evidence, source documents, correlated rendered changes and join findings |
| `search_manifest`, `read_artifact` | artifacts by terms/kind/item plus a bounded RE2 grep; line windows (≤400 lines / 64 KiB) or one YAML node |
| `expand_graph`, `list_paths` | the relationship graph (depth ≤3) and impact paths |
| `record_hypothesis`, `record_assertion` | `raises-concern` / `explains` / `claims-mitigation`, optionally a proposed (always inferred) edge |
| `request_evidence` | recorded for the next collection round; live-cluster reads are rejected in milestone 1. v11: kinds `upstream-file` / `upstream-search` are fetched during the session (below) |
| `request_validation` | runs the configured deterministic validators of that kind now; the validator's record is the result |
| `submit_findings` | v2: closes every agenda item with a disposition (below); v1: one `explains` summary per item. Ends the run |

**What it cannot do.** Classify. Every agent record carries AI provenance (`investigate.agent@v1`, model and
version as reported, prompt version and digest, input evidence, call id). The deterministic policy
(DESIGN §3.3) only lets such records raise concern. Validations come from validators, never from the agent.

**Validation of every call.** Before anything is recorded, the tool layer checks each call:

- Arguments are decoded strictly; unknown fields are rejected.
- Every id must exist in this run: change items, artifacts, rendered changes, nodes (id or object key),
  impact paths, hypotheses.
- Every hypothesis, assertion and summary needs at least one citation `{artifact, locator, quote}`. The quote
  must occur verbatim in the artifact's bytes (`investigate.QuoteOccurs`, whitespace-insensitive). Each
  citation then becomes a `domain.Evidence` (kind by origin, `contentDigest` = artifact digest) in the run
  pool.

A failed check returns `ErrRejected` to the model as a tool error, and nothing is recorded. Every call,
accepted or not, still counts against the budget and goes into the transcript.

**Budgets** (`DefaultAgentBudget`; unset fields take the default): 40 turns, 80 tool calls, 600 000 tokens,
4 MiB returned, 1200 s.

- The tool layer counts tool calls and returned bytes. Once a budget is spent, every further call gets
  `ErrBudgetExhausted`.
- The driver checks turns, tokens and wall clock. Tokens are the envelope's input, cache-write and output
  tokens; cache reads are reported but not counted. The CLI can overshoot `--max-turns`, so turns are also
  checked after the fact. An accepted submission is allowed one closing turn.

Exhaustion is an outcome (`budget-exhausted`). It keeps every record made so far and makes automated
approval ineligible.

**Drivers** (`AgentConfig.Driver`):

| Driver | How |
|---|---|
| `claude-cli` | `claude -p --session-id <uuid> --mcp-config <tmp>/mcp.json --strict-mcp-config --tools "" --allowedTools mcp__ri --permission-mode dontAsk --max-turns N --system-prompt … --setting-sources "" --no-session-persistence --output-format json`, in a neutral temporary directory. The MCP config starts `ri investigate mcp --attach <socket> --token <t>`, a byte bridge back to the in-process session, so validation, budgets and the transcript live in one place. The session id is chosen up front and becomes every record's call id; model and version are read from the envelope's `modelUsage`. No API key: the CLI must be logged in. |
| `exchange` | the `-llm-exchange` directory protocol, one request file per tool round (`{"calls":[{"tool","arguments"}]}`). An unanswered round ends the run as `failed: exchange-pending: <request file>`; re-running replays the answered rounds (deterministic results, same digests) and continues. A response without a `callId` uses `exchange:<request digest>`. |
| `replay` | re-issues a recorded transcript's calls under their recorded call ids and compares every result digest. Any mismatch, or a different prompt digest, fails the run with the first difference named. |
| `scripted` | a fixed list of calls (tests). |

**Transcript** (`agent-transcript` artifact, `derived/agent/<tag>/transcript.jsonl`, format
`ri.dev/agent-transcript/v1`). It has three kinds of line:

- a `session` line: bundle, prompt version and digest, model, budget;
- one `call` line per tool call: call id, tool, canonical arguments, ok/error, `resultDigest` (sha256 of the
  exact bytes returned to the model), counters;
- an `envelope` line: the driver result and the provider envelope.

Results carry no wall-clock fields (`checkedAt`, `generatedAt`, … are stripped), so a replay reproduces the
transcript byte for byte.

**Agenda and coverage (prompt `investigate-agent/v2` and later).** Coverage does not depend on the model's
diligence. `agent.Agenda(bundle, validations)` lists every in-scope item the deterministic pass did not decide.
"Decided" means a failing validation, which makes the item affected by R1 whatever the agent records. Items are
in priority order, and each entry names its declared rule:

| Priority | Rule | Items |
|---|---|---|
| 0 | `agenda:behaviour` | manifestation `controller-behaviour` or `unknown` |
| 0 | `agenda:rendered-unexplained` | `scope:rendered-unexplained` |
| 1 | `agenda:pass-to-scrutinise` | a passing validation exists (R2 still needs every concern answered: a concern here prevents an unsafe clear) |
| 2 | `agenda:undecided` | the rest |

`submit_findings` must close **every** agenda item with a disposition:

- `raises-concern`, `claims-mitigation`, `explains` — an assertion of that stance;
- `examined-no-concern` — an `explains` assertion citing what was inspected. It is **not a mitigation** and
  never lowers a class (DESIGN §3.3 unchanged);
- `not-examined` — the summary states the reason.

Every disposition except `not-examined` needs at least one checked citation. The call is rejected while any
agenda item has no entry. It is accepted once even after the budget is spent, so coverage is always reported,
but the run stays `budget-exhausted`. A run that ends without a submission reports every agenda item as
`not-examined: no closing entry: the run ended <status>`.

`AgentRun.Coverage` holds one entry per agenda item: rule, disposition, summary, closing assertion, every
record about the item, citations.

The budget scales with the agenda (`agenda@v1`, stated in `AgentRun.BudgetRule`). Each field is the larger of
the configured budget and the formula below, which only ever raises the budget:

| Field | Formula (n = agenda size) | Cap |
|---|---|---|
| tool calls | 12 + 6n | 400 |
| turns | 10 + 3n | 200 |
| tokens | 150 000 + 30 000n | 3 000 000 |
| returned bytes | 1 MiB + 192 KiB·n | 16 MiB |
| seconds | 300 + 60n | 3600 |

**Subjects (v2 and later).** The model may name a subject the way the evidence names it. Each subject resolves, by the
first matching rule, to graph nodes:

| Rule | Form |
|---|---|
| `subject:node-id` | `gn-…` |
| `subject:node-key` | `<group>/<version>/<kind>/<namespace>/<name>` |
| `subject:rendered-change` | `rc-…` (its object's node; the change joins the request) |
| `subject:artifact` | `art-…` or an environment file path (the nodes carrying it) |
| `subject:object-ref` | `Kind/name` or `Kind/namespace/name` |
| `subject:values-key` | `a.b.c` or `values:a.b.c`: the values files that set it; else the values files of the product's render targets, noted "not set" |
| `subject:rendered-setting` | a flag, annotation key, env var or config key found as a token in a complete environment render (to side) |

`request_validation` returns the resolution. An unresolvable subject, or one matching more than 20 nodes, is
rejected with the list of accepted forms. Prompt v1 keeps its contract: node ids and keys only.

**Answering concerns (prompt `investigate-agent/v3`).** The classifier lifts a `raises-concern`,
or establishes a `claims-mitigation`, only through a counting passing validation whose `Answers` lists the
**assertion id** (R2.6/R2.7). The validators know only the request's hypothesis, so the tool layer links
assertions into the record. A link needs all of the following:

- **Request:** the agent names the assertions in `request_validation`'s `answers`, or the assertion was
  recorded with the request's `hypothesis`.
- **Stance and item:** the assertion is about the same item, with stance `raises-concern` or
  `claims-mitigation`.
- **Coverage:** the record covers everything the assertion names: its subjects, impact paths and rendered
  changes. An assertion naming none of them needs an item-wide record, covering every root and path of the
  item.

A named assertion the request cannot cover is rejected before anything runs. An implicit one is simply not
linked, and the result says why. `ValidationRequest.Answers` records what was asked.

Some requests duplicate a record the deterministic pass already made. That record is not recorded twice:
`AgentOutcome.AnswerLinks` reports the link, and the runner applies it with
`agent.ApplyAnswerLinks(passRecords, outcome.AnswerLinks)` before classify. The link only says which question a
validation answered. The validator decides the outcome, and the classifier decides whether that pass counts:
for a controller-behaviour item, only a pinned setting or an observed outcome does.

**Depth rules (prompt v3; commander decision 2026-10-16).** The agenda makes the agent close every item, but
coverage alone does not make it examine each one. Live run 2 closed 14/14 items after 4 inspection calls. So v3
enforces a depth floor in the tool layer (`internal/investigate/agent/depth.go`), checked when a record is made:

| Rule | Requirement |
|---|---|
| `depth:examined-after-get-change-item` | an `explains` assertion, or an `explains` / `examined-no-concern` closing entry, only for an item retrieved with `get_change_item` earlier in the session |
| `depth:unshared-citation` | such a record needs at least one citation that no record about another item cites; this includes the other entries of the same `submit_findings`. A citation is the artifact plus the whitespace-normalised quote, so a changed locator does not make a shared quote unique |
| `depth:behaviour-concern-validated-or-reasoned` | `submit_findings` is rejected while a `raises-concern` on a `controller-behaviour` / `unknown` item has neither a recorded `request_validation` for that item nor a `noValidationReason` (`AgentAssertion.NoValidationReason`) |

Coverage entries report depth per item:

- `examined`: `get_change_item` was called on the item;
- `inspections`: accepted `get_change_item`, `query_diff`, `search_manifest` and `list_paths` calls naming the
  item;
- `validationRequests`: recorded requests for the item.

The rules never change a class: they decide only which agent records are accepted.

**Per-item sessions (prompt `investigate-agent/v4`; commander decision 2026-10-16).** In live run 3,
depth was a choice the model declined: 12 of 168 calls. A budget floor could be met with useless calls, so v4
changes the structure instead.

The Investigator runs **one short, focused session per agenda item**, in `agent.ItemAgenda` order:

1. manifestation `controller-behaviour` / `unknown`;
2. `scope:rendered-unexplained`;
3. the rest.

Ties go by item id. Each session has:

- its own task: only that item, plus pointers to its upstream sources, correlated rendered changes, roots,
  paths and validations (`prompts/investigate-agent/v4/item.md`);
- its own budget, `item@v1`: `AgentConfig.ItemBudget`, else 16 calls, 14 turns, 150 000 tokens, 1 MiB, 360 s;
- its own prompt digest and call id.

The tools are unchanged and still bundle-scoped.

- **Cap:** `AgentConfig.MaxItems` (default 10), recorded as `AgentRun.ItemCap`, bounds cost. Agenda items beyond
  it are closed `not-examined: beyond cap N` by the runner, never by the model.
- **Closing:** each session must close its item with exactly one disposition (`submit_findings` with one entry,
  for that item). D1 and D2 still apply.
- **Concerns need the item's own evidence.** A `raises-concern` counts as an item concern only when it is about
  the session's item and cites at least one artifact retrieved in that session. "Retrieved" means:
  `read_artifact`; `get_rendered_object`; `search_manifest` hits with grep lines; the item's upstream sources
  via `get_change_item`; the renders behind `query_diff` results.

  Otherwise it is recorded as an `AgentNote`: investigation-level, AI, labelled with the reason
  (`AgentOutcome.Notes` → `Investigation.AgentNotes`). A closing `raises-concern` without item evidence closes
  the item `not-examined`, naming the note. A note never reaches the classifier, so unexamined items keep their
  deterministic class.
- **Aggregate outcome.**
  - `AgentRun.Status` is the worst session status (failed > budget-exhausted > submitted).
  - `Reason` lists the non-submitted items.
  - `Turns`, `ToolCalls` and `Tokens` are totals.
  - `PromptDigest` is sha256 of the item digests in order.
  - Coverage carries each item's session status, reason, call id, turns, tool calls and tokens.
- **Transcript:** one `item` line per session (item, prompt digest, budget), then its calls and envelope. Replay
  walks the segments in order and must reproduce each byte for byte.

**Setting claims (prompt `investigate-agent/v5` and later; commander decision 2026-10-16).** Live run 4
showed the remaining gap: citations are checked, prose is not — a session closed
`raises-concern` while its summary said the environment does not set a key that
`values-prod.yaml` sets to `true` (fail-safe, but unverified). v5 makes environment facts
structured and checked:

- `record_claim` states a fact — setting S is `absent` / `present` / has `value` V in an
  input (an environment file, an environment render, or `environment-files` for all of them) —
  and the tool layer verifies it **deterministically before recording**
  (`investigate.claim-check@v1`, recorded as the claim's `Verifier` with the artifacts
  `Checked`).
- A setting is searched three ways: as a dotted key path from a YAML document root, as its
  last key anywhere in a document, and as a whole token in the text — so `absent` is verified
  only when the setting occurs nowhere in any form. `value` compares the decoded YAML scalar
  (canonically) at every key match.
- A claim that does not hold is **rejected with the contradicting line** (artifact, path,
  line, text); it is never recorded.
- Recorded claims are `domain.SettingClaim` (`clm-…`, AI provenance: the claim is the agent's,
  the verification is deterministic). Assertions and closing summaries name them in `claims`;
  a claim about another item is rejected. The evidence of a present/value claim joins the
  run's evidence pool.
- Everything the agent writes in free text (assertion / hypothesis / note statements,
  summaries) is labelled `unverified-ai-prose` (`StatementKind` / `SummaryKind`); summaries the
  runner writes (beyond-cap, failed sessions) carry no label.
- The rendered investigation text (`classify.WriteText`):
  - shows such statements as `(AI: <model>, unverified prose)`, with the verified claim ids they rest on;
  - lists the run's verified environment claims (✓) and the agent's investigation-level notes in their own
    sections.

A claim supports the agent's records; it is never a validation and never a policy input —
classification still runs only over validation records.

**Item dossier, provisional verdicts and mitigation hypotheses (prompt `investigate-agent/v6`; v7 is v6 plus
verdict citation inheritance, and v8 revises the mitigation instructions, both below; v9 (the default from the M1 gate, 2026-10-26, until v13 replaced it at the M2 gate,
2026-10-27) is the
consumer-aware investigation further down, and v10 its scoped verdicts; Phase 5,
`docs/phase5/learning-loop/PROTOCOL.md` §8).** v6 keeps
the v5 surface. It adds three things.

*Item dossier* (`internal/investigate/agent/dossier.go`, `ri.dev/item-dossier/v1`, producer
`investigate.dossier@v1`). Each item's session starts from a dossier. The dossier is assembled
deterministically from the bundle and the run, with no new store and no model:

- the change: title, family, manifestation, release, upstream detail and links, evidence excerpts, source
  artifacts, join findings;
- the **settings that decide it**, with their effective state on every exposed object;
- the correlated rendered changes, and the relevant **unchanged** configuration (explicit settings identical in
  the from and to renders);
- the knowledge facts attached to the item, with status, verification level and release;
- the validations already run (with pins, and whether they clear behaviour);
- the relations: roots, paths, unresolved edges;
- an explicit list of **gaps**.

Settings are derived in a fixed order of trust:

1. the subject of an attached active fact, and the settable leaves of its exposure condition;
2. the join's values-key matches;
3. the item's subjects (`Kind.path` → an object field, `a.b` → a values key, `--flag` → a token);
4. code spans in the upstream text.

Each setting has a state per object or input:

| State | Meaning |
|---|---|
| `explicit` | set, with the value and where it comes from (render target or file, line) |
| `inherited-default` | unset in a **complete** render (or absent from every file and complete render): the shipped default applies |
| `unknown` | it cannot be decided, with the reason |

An unset field seen only in a file as declared is `unknown`, never `inherited-default`: an overlay or patch may
set it (fail closed). Summary states are `mixed` (objects differ) and `no-objects`.

Gaps are `contradictory` (two active facts disagreeing on a subject, values set differently in several files,
validations both passing and failing), `missing` (no upstream evidence, no deciding setting, an unknown
setting, an unavailable source or render) or `partial` (a partial render or source, an undecided validation,
unresolved or truncated relations, uncorrelated rendered changes). Not-applicable renders are not gaps.

Same inputs ⇒ byte-identical dossier. It is bounded at 48 KiB: lists are capped, and what was cut is listed in
`truncated`. The dossier is stored as a content-addressed run artifact (`item-dossier`,
`derived/dossier/<item>/<hash>.json`, named in `AgendaCoverage.Dossier`) and inlined into the item task, so the
session's prompt digest pins it. The item counts as examined, and the artifacts it quotes count as retrieved in
that session. `get_dossier` serves any item's dossier. `ri investigate dossier <inv-id> <item>` rebuilds it
read-only from a stored run (the deterministic pass's validations, as the agent saw them) and says whether it
matches the stored one.

*Provisional verdict* (`verdict.go`). Every closing entry except `not-examined` carries the agent's own
assessment: one of the four classes, confidence, mechanism, environment reasoning, citations, verified facts
(`clm-…` / `vr-…`), assumptions, mitigations (`hyp-…`), uncertainty, proposed validations and what would
overturn it.

- A malformed verdict rejects the call (unknown class or confidence, a missing mechanism / reasoning /
  overturn).
- Citations, verified facts and mitigations are resolved **one by one**. What does not resolve is dropped,
  counted (`droppedCitations`, `droppedVerifiedFacts`) and explained (`dropped`).
- A verdict other than `insufficient-evidence` needs at least one citation that resolves.
- **v7** (Phase 5 fix W-0025): a verdict that states **no** citations (omitted or empty) takes its closing
  entry's citations once they are checked, and is marked `citationsFromEntry`. An entry citation that does not
  check rejects the call, as it always did, so nothing unchecked is inherited; citations the verdict does state
  are its own and are never replaced (if none of them resolves, the verdict is still rejected). The tool schema
  says so. In live run 1 (v6) every first `submit_findings` was rejected for an empty `verdict.citations`
  (17% of calls). v7 changes only `meta.json` (`verdictInheritsCitations`) and that schema text; system, task
  and item prompts are v6's byte for byte. A transcript replays only under the version it was recorded with
  (the replay driver refuses another prompt digest): `ri investigate -agent-prompt investigate-agent/v6 …`
  pins an earlier version, and a `ri eval -investigations` replay arm defaults to the replayed run's version.
- **v8** (Phase 5 W-0029, PROTOCOL amendment 2026-10-17 10:55; one revision, then frozen). Run
  `B0-K0-20261017-6d2ae5bf` (v7, 8 cases) made **zero** `record_hypothesis` calls, although many verdicts named,
  in `wouldOverturn` / `proposedValidations`, conditions under which the change would not reach an object (the
  field set to the old value on some objects, a feature disabled, nothing depending on the changed property).
  The v7 trigger ("when you believe a setting or condition in the environment preserves the old behaviour") was
  read as "only when the environment meets it for the whole item". v8 changes `system.md`, `item.md` and the
  `environmentBasis` wording of `tools.json` (the schema is v7's); `meta.json` and `task.md` are v7's byte for
  byte (no tool, validator, policy, budget or verdict schema change). It says:
  - a mitigation is a **release-level rule**: record it whenever upstream text supports a condition under which
    an exposed object is not impacted, whether the environment meets it on all, some or none of the exposed
    objects — the validator decides per object afterwards. **`environmentBasis`** is the verified evidence of
    this environment's state with respect to the rule's conditions, **whether met or not** (commander decision
    2026-10-17, W-0029; documented on `domain.MitigationSpec` and in the v8 tool schema): the claims/objects that
    meet the condition and those that do not; when none meets it the statement says so. It stays non-empty
    (a mitigation with no environment evidence is rejected, unchanged). It is not an applicability decision:
    only the recorder and storage read it — not export, verification, the promotion gate or policy;
  - **change vs consequence**: distinguish the objects whose setting changes from those where a consumer can
    observe the change, and scope the rule's applicability and consequence to the latter as a decidable
    predicate over the subject's fields and the references it states (`field` / `ref` inside `resource`),
    naming when it does not hold (an abstract example, no product);
  - **consistency**: every upstream-supported, environment-checkable condition in `wouldOverturn` is a recorded
    mitigation named in the verdict's `mitigations`, or carries a one-line reason it is not expressible in the
    fact grammar;
  - **consumers**: when the open question is whether anything depends on the changed property, trace what
    references the object (or what it produces) and what it reads, record that as claims and assertions before
    settling on `potentially-affected`; a runtime-produced object absent from the snapshot is said to be absent
    and reasoned about from what references it.

  The revision was made after seeing in-sample v7 results and is reported as such (PROTOCOL amendment).
  Tests: v8 was the default until the M1 gate, and v1–v7 are frozen by a files digest; v8's tools.json differs from v7's only by
  the basis wording; v8's text names no evaluation product, environment, kind or field (a word list plus every
  token of the case directory names); a v8 session that records a mitigation whose basis covers only some (or
  none) of the exposed objects stores it, and an empty basis is still rejected. Smoke (one Phase 4 regression
  case, not cert-manager, `claude-sonnet-5-5`): 10 sessions, 125k tokens, every call well-formed; no hypothesis
  was recorded there, and every verdict's `wouldOverturn` carried a not-expressible reason.
- **v9** (Phase 6 Milestone 1, `docs/phase6/decision-intel/PLAN-M1.md`, `M1-IMPLEMENTATION.md`; the default from the M1 gate, 2026-10-26, to the M2 gate, 2026-10-27; mission §13).
  Per-item sessions kept; `meta.json` turns `mitigation` off (`record_hypothesis` has no `mitigation`) and adds
  `traceConsumers`, `assessment`, `itemCalls: 24`, `itemTurns: 18` (rule `item@v2`; v1–v8 keep `item@v1`).
  - **Consumer graph.** The session builds the consumer-mode graph of the bundle (TO side; FROM on demand) with
    the pipeline's relationship rules (`agent.WithRelations`) and stores it as a run artifact
    (`derived/agent/consumers/<side>/<graph>.json`). Every other tool and the validators keep the stored graph.
  - **`trace_consumers`** `{changeItem | node, side}`: from every exposure root of the item (or one node), the
    `produces` / `references` / `reads-file` hops, each with its artifact, line and text, mount and file detail;
    the consumers reached; and `gaps` — unresolved edges, objects created at runtime, unresolvable paths a reached
    configuration names, mounts no configuration names, the bound (200 hops, depth 6).
  - **Dossier** (producer `investigate.dossier@v4`, only with a consumer graph): `relations.consumers`, the
    chains of each exposed object; their gaps are dossier gaps.
  - **`assessment`** on every closing entry except `not-examined` (`internal/investigate/agent/assessment.go`):
    `whatChanged`, `mechanism`, `consumers`, `effectiveConfiguration` (`explicit` / `inherited-default` /
    `unknown`), `verified` (each statement with citations that must resolve, or the call is rejected),
    `inferred`, `unknown`, `likelyImpact`, `recommendedTests`. Stored as `Investigation.AgentAssessments`
    (`aas-…`, AI provenance, prose labelled), linked to the entry's verdict; never a classifier input.
  - **Completion check.** Consumers of the item's exposed objects that no tool of the session returned
    (`trace_consumers`, `get_rendered_object` of the object, `read_artifact` of the environment file defining it)
    reject a `not-affected` verdict once, listing them; a resubmission is accepted. Every assessment records them
    in `untracedConsumers` and as investigation gaps in `unknown`.
  - The prompt asks the mission's eight questions, consumers first, verified / inferred / unknown kept apart
    (model memory is never evidence), and provisional versus supported conclusions with their proof
    obligations; the mitigation and consistency instructions are gone. Generic wording (tested like v8).
- **v10** (Phase 6, scoped verdicts, `docs/phase6/decision-intel/briefs/scoped.md`, ledger D-0007; product owner
  decision 2026-10-26). v9 with only the verdict rules changed; **not the default** until its check on the
  development pair (`pinnedDefaultPrompt` in `claims_test.go` holds v9). `meta.json` adds `scopedVerdict`.
  - **Scoped verdict.** The agent declares the scope it investigated (e.g. the consumers the environment
    declares, the rendered objects, the cited upstream evidence) and sorts what is open into (a) in-scope
    unknowns the breaking mechanism depends on, which keep the verdict `potentially-affected` or
    `insufficient-evidence`, and (b) preconditions outside the scope that the repository cannot state (reload
    behaviour, pinning by external systems, runtime object contents), which do not. When every traced consumer
    and effective setting in scope is accounted for, it commits to `affected` or `not-affected` within the scope
    and lists the preconditions. Model memory is never evidence for an in-scope fact; untraced consumers stay
    in-scope unknowns.
  - **Assessment fields** (additive): `headline` (one sentence: class, scope, preconditions; required),
    `scope`, `preconditions`. Earlier versions refuse them.
  - **Checks.** `affected` / `not-affected` without a `scope` is rejected (every time). A `not-affected` verdict
    is rejected once while in-scope unknowns remain: the assessment's `unknown` entries and the untraced
    consumers, listed in the rejection; it shares the v9 completion check's once-flag, so an item is rejected at
    most once. The resubmission is accepted, with the untraced consumers recorded as gaps as in v9.
  - Like the rest of the assessment, the scoped fields are AI prose and never a classifier input
    (`TestScopedVerdictsNeverReachThePolicy`, `run/assessment_test.go`).
- **v11** (Phase 6 M2, upstream behavioural evidence, `docs/phase6/decision-intel/briefs/upstream.md`, ledger
  D-0009; design and tests in `docs/phase6/decision-intel/M2-IMPLEMENTATION.md`). v9 plus evidence fetching;
  `scopedVerdict` off; **not the default** (`pinnedDefaultPrompt` holds v9). `meta.json` adds `upstream`, the
  per-item bounds: 6 fetches, 384 KiB stored (at most the item's `MaxReadBytes`), a search scans ≤ 1500 files and
  returns ≤ 30 matching lines with 2 lines of context, stores ≤ 6 matching files; a file result shows its first
  40 lines.
  - **`request_evidence` kinds `upstream-file`** (`{repo, ref, path}`, or a blob URL at the tag in `target`) **and
    `upstream-search`** (`{repo, ref, dir, pattern, glob}`: an RE2 regex matched line by line over every file below
    `dir`, `glob` on base names). Read only in the product's own code repositories (`linkedev.CodeReposOf`) at an
    immutable ref that is the target's tag (`TagFor`, the default) or the current version's tag (comparison).
    Fetches run through the cached `repo-file` / `repo-dir` adapters (`internal/gitsrc`), so `-offline` replays a
    recorded state with identical bytes.
  - **Fulfilled:** each file read becomes an `upstream-document` run artifact (`OriginUpstream`, URI = blob URL at
    the tag, producer `evidence-fetch@v1`), in `EvidenceRequest.Fulfilled`, citable verbatim at once and readable
    with `read_artifact` / `search_manifest`. The bundle never changes.
  - **Unavailable:** another repository or ref (refused, without a fetch), a spent budget, not found, an offline
    cache miss, no match: `RequestUnavailable` with the reason. The item's assessment states each in `unknown` as
    "behaviour at <tag> unavailable: <reason>" and lists the request in `unavailableEvidence` (a refusal whose
    file or search a later request of the item fulfilled is not a gap). Malformed requests are rejected.
  - **Prompt:** one generic rule: when the outcome depends on upstream behaviour (validation, defaulting,
    parsing, a feature gate) the change text does not state exactly, fetch the implementation, documentation
    or tests at the target's tag before concluding; cite them or keep the behaviour unknown. The item task names
    the repositories and tags that may be read. Earlier versions decode `request_evidence` exactly as before.
- **v12** (Phase 6 M2 fix, behaviour dependencies, `docs/phase6/decision-intel/briefs/upstream2.md`, ledger
  D-0013; design and tests in `docs/phase6/decision-intel/M2-IMPLEMENTATION.md`). v11 plus `meta.json`
  `behaviour: true`; **not the default** (`pinnedDefaultPrompt` holds v9). Why: under v11 fetching was a model
  choice, and a not-affected verdict could rest on remembered behaviour that the assessment itself listed as not
  fetched.
  - **Assessment field `behaviour`** (additive): every upstream behaviour the verdict relies on (validation,
    defaulting, parsing, feature gates, API semantics), each `{behaviour, source}`. `source` is `upstream`
    (`citations` of upstream text beyond the item's change text: a fetched run artifact or an upstream source of
    the bundle; verbatim, checked), `change-text` (`quote`: a verbatim quote of the item's change text, which is
    its title, the change's title and detail, and the excerpts of its upstream evidence), `unavailable`
    (`request`: an upstream request of the item recorded unavailable) or `not-checked`.
  - **Checks on `submit_findings`.** Rejected every time: a `not-affected` verdict with any `unavailable` or
    `not-checked` behaviour; a malformed entry (a non-upstream artifact or a quote that does not resolve under
    `upstream`, or citations that all quote the change text; a `change-text` quote that is not in the change
    text; an `unavailable` id that is not such a request). Rejected once (its own once-flag): a `not-affected`
    verdict with no behaviour on a behavioural item, i.e. manifestation `controller-behaviour` / `unknown` or
    family `behavior` (`behaviouralChange`). Each unestablished behaviour is recorded in `unknown` as
    `behaviour "<b>" at <tag>: <source>`. The checks never read the wording. Earlier versions refuse the field.
  - Like the rest of the assessment, `behaviour` is AI prose plus checked citations, never a classifier input
    (`TestBehaviourNeverReachesThePolicy`).
- **v13** (Phase 6 M2 fix, fetch first, `docs/phase6/decision-intel/briefs/upstream3.md`, ledger D-0014; design
  and tests in `docs/phase6/decision-intel/M2-IMPLEMENTATION.md`). v12 plus `meta.json` `fetchFirst: true`;
  **the default since the M2 gate** (product owner, 2026-10-27; v9 before). Why: under v12, with fetching configured and working, the agent often declared
  the deciding behaviour `not-checked` and took a safe verdict without ever calling `request_evidence`.
  - **Check on `submit_findings`**, whatever the verdict, while the session can fetch for the item (a fetch port,
    a known repository of the product, budget left) and the item is behavioural: a `not-checked` behaviour is
    rejected **once** when the item made no upstream request ("Upstream fetching is available for <repos> at
    <tags>: request the behaviour, or state in the entry why upstream source cannot answer it"), and **every
    time** after a request (any upstream request of the item, fulfilled or unavailable): the entry is then
    `upstream` (cited) or `unavailable` (the request id), under v12's rules.
  - **`reason`** (additive, `not-checked` only): why the upstream source cannot answer the behaviour; accepted on
    the resubmission and recorded in the gap, `behaviour "<b>" at <tag>: not-checked (reason: <r>)`.
  - **Prompt:** one sentence on the order (fetch first, then conclude) and the `reason` field. Earlier versions
    refuse a behaviour `reason` with the strict decoder's exact message, so their transcripts replay unchanged.

Verdicts are stored as `Investigation.AgentVerdicts` (`avd-…`, AI provenance, prose labelled
`unverified-ai-prose`) and named in `AgendaCoverage.Verdict`. They are **never** a classifier input: the
pipeline does not pass them to classify. A test shows findings, decision and eligibility are byte-identical with
and without verdicts, including an adversarial `not-affected` verdict on an affected item. The agent may say
`not-affected` where the policy cannot; the two are scored separately (lane eval5).

*Structured mitigation hypotheses* (`mitigation.go`). `record_hypothesis` takes an optional `mitigation`:

- `proposedRule`: a complete `SemanticAssertion` (subject, change, applicability, consequence) plus `release`
  bounds `{from, to}` (from exclusive, to inclusive);
- `environmentBasis`: claim ids, validation ids or object references;
- `requestedVerification`.

It is checked when recorded: the grammar is complete and **decidable** (no `undecidable` leaf); the subject
belongs to the product and its identifying words occur together in a bundle artifact; the bounds parse (tags
like `controller-v1.12.0` too) and lie within the run's FROM→TO; every basis reference resolves in the run; at
least one citation is upstream text. A malformed spec is not stored. The hypothesis stays as free text with
`mitigationRejected` saying why, and the model is told. A stored spec is a proposal only: nothing wires it into
a pin; the agent never chooses the pin rule.

*Export* (`export.go`). `ri investigate export-hypotheses <inv-id> -knowledge <dir>` writes each well-formed
mitigation into a knowledge directory as:

- a `SemanticProposal`: task `full`, AI provenance with rule `hypothesis:<hyp>@<inv>`
  (`domain.ParseHypothesisRule`), citations limited to upstream evidence and appended to the change's
  candidate;
- a whole-fact `fact-review` item proposing exactly the rule, pending, on the candidate of the item's upstream
  change.

It never writes a fact or a decision. Records are content-addressed and an existing review item is left
untouched, so re-export is a no-op. A hypothesis is skipped, with the reason, when it has no well-formed
mitigation, its item has no upstream change, the change's release is outside the rule's bounds, it has no
upstream citation, or another mitigation of the same model call already became the proposal for that
candidate. Lane verify attaches proof records to these items; promotion needs a confirmed proof record and a
human.

*Cost* (`AgentRun`): `sessions`, `wallMillis`, `droppedCitations` and `droppedVerifiedFacts`, beside the
existing turns, tool calls and tokens.

**Environment-scope items.** Items anchored on environment-only rendered changes
(`EvidenceBundle.EnvironmentItems`, e.g. `scope:rendered-unexplained`) are included everywhere the agent
iterates items: the task, the agenda, the per-item tools, coverage and the deterministic pass. These all go
through `EvidenceBundle.Items()` / `Item()`.

**Deterministic pass.** `agent.DeterministicPass(ctx, validators, input)` runs every applicable validator over
every in-scope item; output is deduplicated, in item × validator order. Its `Records`, `Artifacts` and
`Evidence` go into the run record. They are also passed to the agent as `InvestigateInput.Validations`,
`ValidationArtifacts` and `ValidationEvidence`: the agent can read and cite them, but they are not reported as
the agent's outputs.

**Prompts** are embedded, versioned files: `internal/investigate/agent/prompts/investigate-agent/v1/`
(`system.md`, `task.md` template, `tools.json` schemas, and for v2 `meta.json`, the features it switches on).
`PromptDigest = sha256(system ‖ rendered task ‖ tools.json [‖ meta.json])`. v1 has no `meta.json`; v2 adds the
agenda and subject resolution; v3 adds answer links and the depth rules; v4 adds per-item sessions (`item.md`);
v5 adds setting claims (`record_claim`) and the prose labels; v6 adds the item dossier
(`get_dossier`), provisional verdicts and structured mitigations; v7 lets a verdict inherit its entry's checked
citations; v8 records mitigations as release-level rules from named overturning conditions and traces consumers;
v9 adds `trace_consumers`, the consumer dossier, natural assessments and the completion check; v10 adds scoped
verdicts (headline, scope, preconditions); v11 is v9 plus upstream evidence fetched at the upgrade's tags; v12 is v11 plus the assessment's behaviour dependencies; v13 is v12 plus the fetch-first check. `-agent-prompt <version>` (`ri investigate`, `ri worker`, `ri eval -investigations`) selects one; the
version is part of the config digest, hence of the investigation id.
Every version stays embedded, and its tasks and digests are unchanged.

The task lists:

- the in-scope items: id, title, family, manifestation, upstream citations, correlated rendered changes;
- the render summary;
- the uncorrelated rendered changes and unresolved edges;
- the deterministic validations already run.

The system prompt states what the agent's records can and cannot do. It also tells the agent that, for
`controller-behaviour` and `unknown` items, render silence proves nothing: it should name the setting that
pins the old behaviour and request an override inspection. A prompt change is a new version. The prompts
are generic: never tuned to a case.

**`ri investigate mcp --run <inv-id>`** serves a stored investigation read-only to any MCP client: the query
tools work, and record/request/submit tools are refused.

**Wiring (for the runner).**

- Create the stage with `agent.New(store, agent.WithValidators(...), agent.WithDriver(name, d))`;
  `claude-cli` is registered by default.
- `agent.MergeGraph(graph, outcome)` adds the agent's proposed edges (inferred) and placeholder nodes
  (`AgentOutcome.ProposedNodes`) for the post-agent re-traversal (DESIGN §5.1).
- Agent-requested validations are in `AgentOutcome.Validations`. Their evidence and artifacts are in the
  outcome's `Evidence` / `Artifacts`, alongside the transcript.

**Live runs.** Five `claude -p` runs on groundtruth inputs are recorded, each with what the agent did and
its replay check:

- [agent-live-1](phase4/env-impact/runs/agent-live-1/README.md): ingress-nginx, prompt v1;
- [agent-live-2](phase4/env-impact/runs/agent-live-2/README.md): cert-manager, prompt v2, with coverage;
- [agent-live-3](phase4/env-impact/runs/agent-live-3/README.md): kube-prometheus-stack, prompt v3: coverage,
  depth, validation requests and answer links;
- [agent-live-4](phase4/env-impact/runs/agent-live-4/README.md): the same input under v4 (per-item sessions), for
  a v3 vs v4 comparison of depth;
- [agent-live-5](phase4/env-impact/runs/agent-live-5/README.md): the same input under v5 (setting claims), for a
  v4 vs v5 comparison of claim verification.

**Reproducibility caveat.** A bundle's snapshot pins the HEAD commit of the git work tree containing the
environment and the input path as supplied. The groundtruth environments sit inside this repository, so
every commit, and every checkout location, gives a new bundle id. A recorded run then replays only at its
commit and path: a replay elsewhere is refused for a prompt-digest mismatch, never silently accepted.

## Verifying mitigation hypotheses (Phase 5)

An agent may propose a reusable rule — "this setting preserves the previous behaviour across this transition"
(a `MitigationSpec` on a hypothesis, exported by `ri investigate export-hypotheses` as an AI proposal plus a
whole-fact review item). It is never a fact until it is **independently verified and promoted by a human**
(docs/phase5/learning-loop/PROTOCOL.md §7–§8). Package `internal/investigate/verify`, command:

```
ri verify hypotheses -knowledge <dir> [-store <state>] [-runtime] [-kubernetes 1.31] [-again] [-json]
```

For every open hypothesis-sourced fact-review item (`Provenance.Rule = hypothesis:<hyp>@<inv>` on a proposal
of the item's rule) it writes a **`ProofRecord`** (`internal/domain/proof.go`, knowledge record kind `proof`,
stored beside the item under `<product>/<release>/proofs/`):

- **Citation check** (`verify.citations@v1`, deterministic). The rule's release bounds — `(From, To]` from the
  run's mitigation spec — and its upstream citations (the proposal's evidence on the candidate). Each citation
  must resolve in the evidence cache (the investigation store's CAS by content digest, or the fetch cache by
  digest, else by URL; a GitHub blob URL also matches its raw form; a cited digest must match exactly), contain
  its cited passage (whitespace-normalised, as the agent's tools check), and name at least one version and no
  version outside the bounds (versions are read from the URL; a minor-only token like `1.18` names a release
  line). Any failure ⇒ `fail`; no bounds or no citation ⇒ `not-run`; no passing passage naming the subject ⇒
  `inconclusive` (whether the text *states* the behaviour is the reviewer's judgement, listed as a limitation).
- **Semantic check** (`verify.semantic-check@v1`, deterministic, Phase 6 M4; package
  `internal/investigate/verify/semantic`). The citation check proves a passage exists, not that it says what the rule
  says (the M2 finding: correct upstream text, misread). Over the passages that **passed** the citation check, the value
  tokens the rule asserts — its change's before/after values and the values its statement asserts (defaults, allowed or
  enumerated values, character classes and single characters, booleans) — must each occur with the **same polarity**
  ("accepts" vs "rejects", "true" vs "false", "enabled" vs "disabled", "valid" vs "invalid", negators flip). A token the
  passages lack, or one they state only reversed ⇒ `fail` (the rule is unsupported, the proof refuted). A claim with no
  comparable token, or whose token's polarity cannot be read in the passage ⇒ `inconclusive`: it is routed to the human
  reviewer (never to a model) and recorded as a limitation. No established passage ⇒ `not-run`. A regular-expression
  character class, or a list in a clause with "only", is an exhaustive allowed set: claiming a character outside it is
  *accepted* is unsupported; claiming it is *rejected* is supported. Extraction is lexical and generic: quoted spans count
  as values only in value positions (after *is*, *to*, *of*, *accepts*, …, or continuing such a list); dotted, lowerCamel,
  flag-like names and versions are never values. Applicability-condition values are not compared: they scope the
  environment and are tested by the exercise arms and the applicability checks. The method is **optional**: it never
  replaces the citation check or the rule exercise.
- **Rule exercise** (`verify.exercise@v1`, observed, opt-in `-runtime`). A **synthetic** fixture built from the
  rule, never an evaluation environment: the product's controller at FROM (its chart with the values declared
  in `products/exercises/fixtures/<product>.yaml`), the subject kind's minimal object from the same file (plus
  prerequisites, e.g. an issuer) in two arms — **condition-present** satisfies the rule's mitigating (`overlap`)
  condition, **condition-absent** its exposure condition — and nothing else. `runtime.Validator.RunFixture`
  runs the same phases as an environment run (install, exercises at FROM, upgrade to TO, exercises at TO,
  observation, cleanup on every exit path) in a cluster `ri-verify-<8 hex>`; a fixture run and an environment
  run never overlap (each refuses while a cluster of either prefix exists). Objects the controller produces
  from each arm (the product's `produces` relationships) are observed and their data digested per key.

  | condition-present arm | condition-absent arm | exercise outcome |
  |---|---|---|
  | unchanged at TO | changed at TO | `pass` — the behaviour change is demonstrated, and the condition preserves the previous behaviour |
  | changed | changed | `fail` — the condition does not preserve it (refuted) |
  | changed | unchanged | `fail` — the prediction for the condition arm is contradicted |
  | unchanged | unchanged | `inconclusive` — the change itself was not demonstrated, so neither is the mitigation |

  "Changed" is any difference the runtime's degradation rules record on the arm's objects (exercise window:
  what the exercise did at TO that it did not do at FROM; upgrade window). Never a pass: an exercise not answered
  on every arm at both versions (a quiet window), a run that did not complete, a sandbox problem (image pull),
  no stored observation artifacts. `not-run`: no fixture for the subject kind, no declared exercise or one
  without a reaction, or a rule whose conditions cannot be synthesised on the subject object (only `field`
  leaves `unset` / `set` / `equals` / `not-equals` without list paths, inside `resource` of the subject kind;
  `set`/`not-equals` use the change's previous value; a rule without an overlap is mitigated by setting an
  "exposed while unset" field to its previous value) — never approximated.

**Outcome** = `confirmed` only when both required methods (citation check, rule exercise) passed; any `fail` of any
method (the semantic check included) ⇒ `refuted`; otherwise `inconclusive` — an inconclusive or not-run semantic check
does not hold back an otherwise confirmed proof, since a human must accept the item anyway (`domain.ProofOutcomeOf`; `ProofRecord.Validate` rejects an outcome the methods do not support, a passing
exercise without both arms answered, and a level that does not match: `observed` exactly when the exercise
ran). Each proof lists its limitations (one synthetic object per arm, chart-default controller, other consumers
and topologies not covered, applicability to an environment decided by the validators, not by the proof). A
proof whose rule differs from the run's mitigation spec is not written. Items with a confirmed or refuted proof
are skipped unless `-again`; inconclusive ones are re-verified.

**Fixtures** (`products/exercises/fixtures/<product>.yaml`, kind `ProductFixtures`; own directory so the exercise
files' digests, recorded by every runtime run, are untouched): `controller.values` (+ upstream references),
and per subject `target` (group/version/kind), `exercise` (an id from `products/exercises/<product>.yaml`, which
must declare a reaction), `namespace`, `prerequisites`, `template` (`{{name}}` is replaced by the arm object's
name) and upstream `references`. Unknown fields, missing references or an evaluation-data URL are load errors.

### Semantic check of agent behaviour (report-only, Phase 6 M4)

The same check runs over every agent assessment `behaviour` entry sourced `upstream`: its statement against the
passages its citations quote (the cited evidence records' excerpts in the run's pool). Each entry is `supported`,
`unsupported` or `undetermined`. It is **report-only**: never a classification input, never a policy input.

- **Synthesis input and reports.** The upgrade-level pre-computation adds one disagreement
  `behaviour-unsupported-by-citation` (not hard) per unsupported entry, with the reason (the missing token, or the
  reversed polarity) and the cited evidence; the judgement copies it verbatim, so the change-ticket section and the report
  UI show it beside the other disagreements.
- **Stored runs.** `ri investigate semcheck [-store DIR] [-json] <key>…` re-checks stored runs (an investigation store,
  or stored evaluation runs with keys `<run-id>/<case>`) and prints every upstream entry with its verdict and reason.

### Boundary fixtures (Phase 6 M4, lane applicability)

The synthesised arms state every field the rule compares by value; real objects mostly omit them. Before any
controller is rendered, the exercise **predicts every arm with the rule itself**, three-valued
(`internal/applicability`, with only the rule's evidenced condition defaults: the fixture consults no CRD schema, so
a rule relying on a schema default states it with source `crd-schema`). It then adds one **omitted-field arm** per
arm and per field compared by value (`equals`/`not-equals`): the same object without the field (and without parents
left empty), predicted by the rule, named `condition-present-omitted-N` / `condition-absent-omitted-N`; objects
identical to an existing arm are not repeated (at most 6). The exercise **fails** — before any cluster runs — when
the rule cannot predict an arm or an omitted-field object (a rule that only works when every field is explicit, or
whose condition default is not evidenced), or when its exposure contradicts how an arm was built. At runtime an
omitted-field arm predicted to keep the previous behaviour that changes refutes the rule; one that does not confirm
its prediction keeps the exercise from passing.

### Consumer observation (lane verify2)

The produced-object observation above can show that an explicit setting preserves an object's previous
behaviour. It cannot test a rule whose claim is about **consequence** — that an object changes without any
consumer observing it (a certificate's own key rotating is invisible to a client that verifies it through its
issuer; the issuer's key rotating is not). A subject fixture may therefore declare `consumers`:

```yaml
consumers:
- id: ca-trust
  role: [{path: spec.isCA, value: true}]          # the field(s) the subject holds in this role
  objects: [ ...consuming objects, {{name}} = the arm's subject object... ]
  trust:   {secret: "{{name}}-tls", key: ca.crt}  # captured by the consumer before re-issuance
  reissue: ["{{name}}", "{{name}}-leaf"]          # objects of the subject kind, re-issued in this order
  verify:  {secret: "{{name}}-leaf-tls", key: tls.crt}
  references: [ ...upstream documentation... ]
```

**Selection.** A rule whose applicability names a role field distinguishes subjects by that role, so its
exercise is the consumer observation; every other rule keeps the produced-object observation, unchanged (the
Phase 5 verify-smoke proofs still load and re-verify). A rule naming the role fields of two observations is
`not-run` (ambiguous).

**Arms, from the rule, in the role.** Exposure and overlap are expanded into alternatives (`resource` / `all` /
`any` / `field` on the subject kind; at most 16). Every overlap alternative that holds together with the role
fields and does not meet the exposure is a **shielded** arm (`condition-present`, `condition-present-2`, …;
predicted: the consumer still verifies at TO); every exposure alternative that holds in the role and does not
meet the overlap is an **exposed** arm (`condition-absent`, …; predicted: it verifies at FROM and fails at TO).
A **shielded** alternative that contradicts the role is exercised **outside the role** (`condition-present-outside-role-N`,
Phase 6 M4): the alternative's object with the same consuming objects. Its consumer verifying at FROM and failing at
TO refutes the rule (the object plays the role after all and is exposed); verifying at both versions is consistent
with the shield; no relationship at all is recorded and decides nothing. Omitting a role field in a boundary arm
also takes the object outside the role. Exposed alternatives that contradict the role stay limitations. Each arm
also gets the declared consuming objects (e.g. a CA issuer and a leaf it signs).

**The observation** (`runtime.ConsumerCheck`, `internal/investigate/runtime/consumer.go`), at each version right
after the version's baseline observation and before its exercises (inside the exercise window at both
versions, so the upgrade window stays clean): capture the PEM certificates of `trust`; re-issue the `reissue`
objects one after another with the subject's declared exercise, each awaited until its reaction field changes
(bounded by the exercise's settle time); then verify the first certificate of `verify` (the rest as
intermediates) against the captured certificates with Go `crypto/x509`, in process. `verified`; `failed` only
for an authority mismatch (`x509.UnknownAuthorityError`: the chain does not reach the captured trust);
`undetermined` for anything else (an object not re-issued, a Secret or certificate missing, any other x509
error such as validity). Only PEM `CERTIFICATE` blocks are read — a declared key holding any other block (a
private key) is refused — and only the outcome and sha256 digests of the certificates and their public keys
are recorded (`Run.Consumers`, `ProofArm.Consumer`); Secret data in observations stays HMAC-digested.

**Outcome** (fixed by PROTOCOL amendment 2026-10-17 before the method existed): `pass` iff every shielded arm
verifies at FROM and at TO and every exposed arm verifies at FROM and fails at TO; `fail` (refuted) if a
shielded arm fails at either version; otherwise `inconclusive` — an exposed arm that still verifies at TO
(quiet), an exposed arm that fails already at FROM (no baseline), any undetermined arm, or no exposed arm at
all (the rule can then be refuted but never confirmed). Data differences on the arm's objects (e.g. the leaf's
own key rotating) are listed on the arm as "not decisive for the consumer". The cert-manager fixture declares
`ca-trust`: the arm's Certificate is a self-signed CA, a CA `Issuer` signs a leaf with it, the CA and then the
leaf are re-issued, and the leaf's `tls.crt` is verified against the CA's `ca.crt` captured before.

### Human-authored rules (`ri knowledge intake-rule`)

```
ri knowledge intake-rule -dir <knowledge copy> -author "<who wrote it, verbatim>" [-candidate sc-…] <rule.json>
```

The pre-registered fallback (PROTOCOL amendment 2026-10-17) when the agent proposes no mitigation: a rule file
(`docs/phase5/learning-loop/rules/*.json`: `_provenance`, `statement`, `assertion` {subject, change,
applicability, consequence, release {from, to, bounds: "(from, to]"}}, `citations` ["ev-… (locator)"],
`requestedVerification`) becomes exactly what `export-hypotheses` makes of an agent hypothesis — a
`SemanticProposal` and a whole-fact `fact-review` item proposing exactly its rule, never a fact
(`internal/knowledge/intake.go`). The file is encoded **faithfully or refused**: unknown fields, a bounds shape
other than `(from, to]`, an incomplete or undecidable rule, or any part of the assertion that does not survive a
round trip through the fact grammar unchanged is an error. Provenance: rule `human-authored:<hyp-id>` (the
hypothesis id derived from the file's sha256), provider and model version `human-authored`, prompt version
`human-authored/v1`, `model` = the author string verbatim, prompt digest and call id = the file's sha256; method
stays `ai` (the proposal contract admits only AI provenance, the least trusted label). The proposal carries the
rule's bounds (`SemanticProposal.release`, set only by the intake) because there is no run record. The rule is
anchored on the one candidate of the subject's product inside the bounds holding every cited evidence id (or
`-candidate`); an existing item proposing exactly the same rule is reused (`itemExisted`). Routing ignores the
proposal, `ri verify hypotheses` verifies it (bounds from the proposal) and the promotion gate applies
unchanged; the review UI labels it **human-authored — not agent-proposed**. Output: file digest, hypothesis,
candidate, proposal, review item, rule digest (what a proof must name) and proposed-rule digest (with bounds).

### The promotion gate (`internal/knowledge/promote.go`)

`Queue.Decide` refuses, before anything is written, an accept/correct of a hypothesis-sourced item unless:
the reviewer is **human** (a proxy or the agent never promotes); the item is a whole-fact (`fact-review`) item
and the decision verifies **all four aspects**; a **confirmed** proof of exactly the decided rule's digest exists
for the item (a correction that changes the rule needs a new item and a new proof; a prose-only correction keeps
the digest); **no refuted proof** of that rule exists (a refutation is never outweighed by a later confirmation);
the proof's bounds contain the item's release; and every other active fact on the same subject is named in the
decision's `supersedes` (the narrower rule replaces it — it becomes `superseded`) or `coexists` (kept, said
explicitly) — a refining rule never silently coexists. Reject / need-more-evidence / defer are always allowed.
Ordinary (non-hypothesis) items are unchanged.

### In the review UI

A hypothesis item's page shows the rule in plain English, the agent's reasoning labelled **AI — not
verified**, every proof (each method with its level — deterministic / observed — outcome, detail, checks, the
two arms with their predicted and observed behaviour, evidence links and limitations) and the gate (open /
closed, with the reason); the decision form asks, per active same-subject fact, *supersede* or *keep beside*.
The default view lists an agent rule only once its proof is confirmed (then as a pin source: promoting it can
clear investigated environments); until then it is hidden as "awaiting a confirmed proof".

Tests: `go test ./internal/investigate/verify ./internal/knowledge ./internal/reviewui` (synthetic product
`acme-ctl`; every gate refusal path, the four arm combinations, citations outside the bounds, a proof digest
mismatch after a correction; the consumer outcome rule, human-authored intake and its gate) and `go test
./internal/investigate/runtime -run 'Fixture|Consumer'` (the fake cluster, with real x509 certificates for the
consumer observation). Real runs on the cert-manager fixture are recorded in
`docs/phase5/learning-loop/runs/verify-smoke.md` and `runs/verify2-smoke.md` (consumer observation).

## Running an investigation

`ri investigate <product> <from> <to>` runs the whole environment-aware pipeline (DESIGN.md §5):

```
collect-change-evidence → build-snapshot → assemble-bundle → build-graph → traverse → validate
  → investigate (agent) → traverse (graph + the agent's proposed, always-inferred edges) → validate
  → classify → human-review
```

The environment is named like `ri impact` (`--repo`, `--values`, `--manifests`, `--crds`, `--images`,
`--inventory`, `--kubernetes`, `--env-name`); `--render` (default on) renders both releases with the
environment's configuration; `--knowledge DIR` supplies verified facts (a fact clears a change only when
trusted). `--agent` runs the investigation agent (driver `--agent-driver claude-cli|exchange|replay`,
`--agent-exchange DIR`, `--agent-replay FILE`, or `--agent-driver scripted --agent-script FILE`, a JSON list of tool calls played as the model); without it the agent stage records `not-run` and no
run can become auto-approve-eligible. `--runtime` (off by default) adds the observed-level isolated-upgrade
validator ("Runtime validation": a throwaway kind cluster per investigation, minutes, needs docker/kind/kubectl;
`ri worker` takes the same flag). Everything lands in `<state>/investigations/`: the immutable bundle,
content-addressed artifacts and the run record `runs/<inv-id>.json`, whose **hash-chained audit trail**
has `started` / `completed` / `failed` / `retried` per stage (actor `system`), `requested` per agent
request (actor `agent:<model>`) and `decided` for the review (`human:<name>` / `proxy:<model>`). The
deterministic pass's `ValidationRecord`s are in the record itself, so it is complete without re-running
validators.

**In-process** (default): a plain sequential runner. A transient error is retried with the stage's policy
(DESIGN §5.1: collect ×5, snapshot/assemble/validate ×3, agent/classify ×1; exponential backoff), a
permanent one stops the run. Without `--review` it ends `awaiting-review`; then
`ri investigate review <inv-id> approve|reject|more --reviewer NAME [--note …]` records the decision
(only a human approves; a proxy decision is recorded and never approves). `--review approve --reviewer me`
applies a decision inline. `--review-timeout` fails closed: nothing is approved.

**Temporal** (`--temporal`): the same stages as activities of one workflow. Start a worker with
`ri worker` (same wiring flags; `--temporal-addr`, default `localhost:7233`, `--temporal-namespace`), then
`ri investigate … --temporal` (add `--detach` to print the workflow id and return). The workflow id is
content-derived from the request (`invreq-…`), so re-submitting attaches to the existing execution; a
finished result is not silently recomputed (`--rerun N` adds `-rN`). Send the human decision with
`ri investigate review <workflow-id> approve|reject|more --temporal --reviewer NAME` (`more` takes
`--request values:<path>,manifest:<path>,crd:<path>,image:<ref>`), and look at the live state (stage,
round, decision, review, workflow audit) with `ri investigate state <workflow-id> --temporal`. Timeouts:
review default 7 d (`--review-timeout`) ⇒ outcome `review-timed-out`, status `failed`, nothing approved;
cancelling the workflow records a `cancelled` audit event. `request-more-evidence` starts a new round
(snapshot → … → classify; the change evidence is reused; at most 2 rounds). A round whose bundle is
unchanged reuses the run record; one with new inputs is a new bundle and so a new investigation id.
`--search-attrs` upserts `RiDependency`, `RiEnvironment`, `RiClassification`, `RiReviewState` (register
them first; see TEMPORAL.md).

Both paths call the same `run.Pipeline`, so for the same inputs, configuration and a scripted/replay
agent they produce the same run-record digest (`recordDigest`: the record with its audit sequence, not
its timestamps). A stage's idempotency key is a content digest (DESIGN §5.1); a repeated key returns the
first result without repeating side effects, in particular without calling the agent again. Because a
repeated key returns the first result's refs too, each key covers every input its stage reads and every ref
its output carries on (change, snapshot, bundle, investigation, previous round, and the pre-record audit by
content, not time); build-graph's covers the run record it writes, collect's the render setting. So two
runs on one state dir that differ only in their settings (e.g. without and then with `--agent`) are two
investigations, each run to the end with its own config (Phase 5 fix W-0024; before it, the second run
continued the first run's investigation from build-graph on). Keys live only in the workspace memo, never
in a run record: no stored id or digest changed. Memos written under the old keys are simply not hit.

**Snapshot identity.** A repository input is identified by the content of the environment directory (tree digest of its collected files and
their repo-relative paths): the same content in another checkout, path or commit is the same snapshot and bundle (same ids, same bytes). The
enclosing commit, dirty flag, remote and checkout path are provenance recorded in the `build-snapshot` audit event, not in the bundle.
A loader warning that a non-semver version (a branch) was found counts as a collection gap only when it concerns the product under
investigation; for another product's deployment it is recorded as a `note:` on the repo input.

## Investigation report (per upgrade): what was checked, every verdict, the human decision

`ri investigate serve` (and `/investigations` inside `ri review serve`, one link in its top bar) shows every stored
investigation so a human can see what was checked before a release, read every verdict and record the decision.
Package `internal/investigate/reportui` (stdlib only).

```
ri investigate serve                                         # the investigation store in the global -state (reviews are recorded)
ri investigate serve -store eval/investigations/results      # the stored evaluation runs, read-only (packed cases.tar.gz or unpacked cases/)
ri investigate serve -store eval/investigations/results/B-20261017-047cb4ea1c   # one evaluation run
ri review serve -knowledge knowledge/ -investigation-store eval/investigations/results   # both views, one server
```

Default address `127.0.0.1:8486` (`/` redirects to `/investigations`). No authentication: it binds to localhost and
refuses cross-origin POSTs.

**List** (`GET /investigations`): one row per investigation — product, from → to, environment, run date, decision
(needs human review / eligible, not enacted / not classified), review state and reviewer, item counts by class —
newest first; filters `?product=`, `?env=`, `?decision=review|eligible|none` and, for stored runs, `?run=<run-id>`.

**Investigation page** (`GET /investigations/view/<key>`; key = the investigation id, or `<run-id>/<case>` for a stored
evaluation run), top to bottom:

1. The decision in plain English (why it needs review, or why it is eligible and that automated approval is off), the
   policy's own reasons verbatim, and the review state.
2. Release (product, from → to, item counts, change-evidence collection status per source) and environment (repo
   path/URL, commit, tree digest, Kubernetes version, every snapshot input with status).
3. **What was checked**: every render target (environment and chart-default scope) with status, rendered-change counts
   (explained / unexplained) and links to the FROM/TO renders and diff; every validator with pass / fail /
   inconclusive / not-run counts; relationship traversal (complete or not); runtime validation run yes/no; agent run
   yes/no (model, provider, prompt, turns, tool calls, tokens, transcript); knowledge-fact artifacts with their
   verification level when the store holds their bytes (trusted = deterministic or human); pinned settings established.
4. **Items, attention first**: affected → potentially affected → insufficient evidence → not affected (collapsed). Each
   item: class, the rule that decided it (R1–R4 in words), one deterministic sentence of reason (never from AI output),
   the full deterministic reasoning, the upstream change text with links, the rendered before/after values of its
   correlated changes (sensitive fields withheld), every validation (validator, outcome, level, property, detail,
   pinned setting, evidence, inputs/outputs), runtime observations, the join verdicts, gaps, assumptions, remediation,
   next steps and approval blockers.
5. **AI output in its own dashed, labelled box** ("AI OUTPUT — advisory, not a validation"): the agent's per-item
   summary, concerns (with whether a passing validation answered them), explanations, mitigation claims, hypotheses,
   verified setting claims (with the checker), and its validation/evidence requests — each with model, provider,
   prompt version and call id, and "unverified AI prose" where the record says so. Validated results never appear
   inside it; AI records never appear outside it.
6. **Human review**: approve / reject / request more evidence, with reviewer name and a reason (both required; inputs
   wanted for "more"). The decision goes through `classify.NewReviewApplier()` (`ReviewerKind: human` always — the form
   has no other kind; an AI reviewer is recorded by tooling, shown as "AI reviewer — recorded, never approves", and never
   approves) and is written with the store's `PutInvestigation` (validated; the audit trail only grows). As with
   `ri investigate review` on a stored run, a decided review completes the run; a new round of evidence is a new
   `ri investigate`. Runs driven by Temporal are reviewed with `ri investigate review <workflow-id> … --temporal` (the
   signal), not on this page. Then the review history and the full **hash-chained audit trail** (who, what, when, each
   event's previous and own digest; the chain is re-checked on every view).
7. The record: investigation id, status, bundle, configuration digest, record digest (timestamps excluded).

**Shareable record**: "Download report (HTML)" (`GET /investigations/export/<key>?format=html`) is one self-contained
file for a release ticket — stylesheet inlined, no scripts, no links back into the server, every item expanded — that
carries the run record JSON inside it as a download link; `?format=json` is the run record alone.
`GET /investigations/artifact/<key>?digest=sha256:…` serves an artifact's bytes as plain text when the store holds them
(stored evaluation runs keep only the per-case records, so most artifact names are shown without a link there).

Stored evaluation runs are read-only: the page says so and offers no review form. Screenshots:
`docs/phase4/env-impact/review-ui/10-investigations-list.png`, `11-investigation-page.png`, `12-investigation-review.png`.
Tests: `internal/investigate/reportui/reportui_test.go` (list/filter, attention order, AI labelling, human review with
reason, proxy cannot approve, read-only runs, cross-origin refusal, self-contained export).

## Upgrade-level synthesis (`--synthesize`, Phase 6 Milestone 3)

After classification, `--synthesize` makes one committed **upgrade judgement** per upgrade × environment
(`Investigation.judgement`, `domain.UpgradeJudgement`; package `internal/synthesize`; plan
`docs/phase6/decision-intel/PLAN-M3.md`, design and limits in `docs/phase6/decision-intel/M3-IMPLEMENTATION.md`). It is
advisory AI output for an engineer to approve or deny (PD-1): it is never a policy input and never changes the findings,
the decision, the review or knowledge.

- **Deterministic pre-computation** (a pure function of the run record, bundle, impact report, edge and the product's
  release list): the hard findings and disagreements, each with a content-derived id (security regressions, confirmed
  breakage, failed validations, target options still affected, agent-versus-classification disagreements, untraced
  consumers, unestablished upstream behaviour, upstream behaviour its own citations do not support (the report-only
semantic check, Milestone 4), advisory inconsistencies, an incomplete advisory picture); the target
  options of the PD-3 rule (minimum safe release from the edge's candidates; the latest patch on the chosen line when it
  removes every regression and adds no assessed advisory; nothing is safe while a security source is unread); the change
  locations established by deterministic facts (a passing pin, verified setting claims, the GitOps line stating the
  deployed version, with file and line); candidate tests; unresolved items as residual risks; items touching the same
  object or setting; the backout line.
- **One structured model call** (prompt `synthesize/v1`, generic, digest-pinned; default model `claude-opus-5-5`,
  `-synthesize-model`) through the LLM answer cache over `-synthesize-exchange DIR` (answer with
  `scripts/semantic-exchange.sh DIR`) or a stateless `claude -p` call; `-offline` answers from the cache only. A replay
  is a cache hit.
- **Deterministic floor** on the answer: `proceed` is impossible with a hard finding; the target is the proposed one or
  a computed safe option (never one with a regression); `modify-target` names its version in the headline; every id and
  citation resolves; a step's location is a computed location or none (`proposed`). A violating answer is retried
  (at most 3 calls, each told the violations); after that the judgement is recorded as `failed`, never repaired. The
  pre-computed findings are copied verbatim whatever the answer says. A changed target always requires the new
  current → target investigation.

```sh
ri investigate ingress-nginx controller-v1.11.5 controller-v1.12.0 --repo ./env --synthesize   # claude -p
ri investigate … --synthesize --synthesize-exchange /tmp/x    # writes the request; answer it, run again
ri -offline investigate … --synthesize                          # replay from the answer cache
ri investigate … --synthesize --temporal                        # the synthesize activity (ri worker serves it)
ri eval -investigations … -synthesize                           # + the ungated judgement side table
```

`ri investigate` prints the judgement as a change-ticket section (summary and recommendation, target options,
disagreements and hard findings, residual risks, impact with the agent's assessments, implementation steps with their
locations, validation, backout, audit); the report UI page and its HTML export show the same section in an AI box.
Under Temporal the step runs behind `workflow.GetVersion("synthesize")`, so histories recorded before it replay
unchanged. A synthesis that yields no judgement (a pending exchange, an unreachable model) is audited as `failed` and
the run continues to the review.

## Evidence formalization (`ri investigate formalize`, Phase 6 Milestone 4)

Formalization turns one natural agent assessment (prompt v9 onwards; v13 is the default) into testable hypotheses that
the existing verification and review path can check (package `internal/formalize`; plan
`docs/phase6/decision-intel/PLAN-M4.md`, design and limits in `docs/phase6/decision-intel/M4-IMPLEMENTATION.md`). It is a
separate stage after the investigation: the investigator explains naturally, the formalizer extracts the rule.

- **Input:** the assessment, its change item and the run's evidence. The model sees the assessment, the *citable*
  evidence (what the assessment's verified and behaviour entries cite, the item's change text when a behaviour quotes
  it, the evidence of the run's verified setting claims about the item), the *uncertainties* it leaves (unknowns,
  not-checked or unavailable behaviours, settings in state unknown or inherited-default, preconditions), and the
  environment basis (the item's verified setting claims and validation records).
- **Output:** per assessment, a formalization record with zero or more `Hypothesis` records:
  - **rule**: a `MitigationSpec` that passes the agent's own spec checks (`agent.CheckMitigation`: complete and
    decidable, the subject is of the run's product and occurs in the bundle, the bounds lie within the run, the basis
    resolves, an upstream citation);
  - **unrepresentable** (mission D3): the claim, its citations, `MitigationRejected` = the grammar limitation, and
    `proposedVerification` with the method, the intended conditions, the uncertainties it keeps and the rule as far as
    it can be written (undecidable leaves allowed).
- **Non-invention floor** (mission D4), checked after the call: a citation outside the citable evidence, a dropped
  uncertainty, bounds beyond the run, a subject of another product, a rule that treats an unset field as a value (a
  default without evidence), an undecidable rule, a basis outside the run are all violations. A violating answer is
  retried (at most 3 calls, each told the violations); after that the record is `failed`, never repaired.
- **One structured model call** (prompt `formalize/v1`, generic, digest-pinned; default model `claude-opus-5-5`,
  `-formalize-model`) through the LLM answer cache over `-formalize-exchange DIR` or a stateless `claude -p`;
  `-offline` answers from the cache only. A replay is a cache hit.

Records are stored at `<state>/investigations/formalizations/<inv-id>/<fmz-id>.json`. The run record, its findings,
decision and review are never changed, and formalized hypotheses never enter the classifier's input. With
`-knowledge`, the run's formalized hypotheses go through `ExportHypotheses` unchanged (proposals and whole-fact review
items, never facts); `ri verify hypotheses` finds them in the formalization records; a human promotes.

```sh
ri investigate formalize <inv-id>                                  # claude -p, every assessment of the run
ri investigate formalize <inv-id> -formalize-exchange /tmp/x       # writes the requests; answer them, run again
ri investigate formalize <inv-id> -assessment aas-… -knowledge K   # one assessment; export to a knowledge copy
ri verify hypotheses -knowledge K -runtime                         # independent verification, as for agent rules
ri eval -investigations … -agent -formalize                        # formalize each case's assessments (stored as formalizations.json)
```

## Evaluation

`ri eval -investigations` scores the investigation pipeline against the blind ground truth in
`eval/investigations/` (format: `eval/investigations/FORMAT.md`; design: DESIGN.md §6). For every case it
builds the request from `input.yaml` + `env/` only (the pipeline never sees `expected.yaml`), runs the same
in-process pipeline as `ri investigate` with its store in a temporary directory (`-work DIR` keeps it), and
scores the stored run record:

- **Item decisions** — one per (case, expected item). The item's `match` alternatives select the system's
  in-scope change items (subject = substring of the item's subjects; text = RE2 over the title and the
  upstream change detail); several → the strongest class decides and all are listed; none →
  `insufficient-evidence` and an `itemMatchMiss`.
- **Metrics** — every DESIGN §6.2 metric (falseNegativeRate, unsafeEligibilityRate, falsePositiveRate,
  affectedDetection/Confirmation, notAffectedConfirmation, exposureRecall, evidenceCoverage,
  citationResolution, unresolvedRate, observedAgreement, reproducibility, pipelineFailures, human-review and
  eligibility rates), each with numerator/denominator, and split per finding basis
  (`deterministic`/`observed`/`none`), per manifestation and per expected verification.
- **Gates** — `eval/investigations/gates.yaml` (frozen). A gate whose denominator is below its `minSample`
  (or 0/0) is `INSUFFICIENT-DATA`: printed as such, never a pass, and a FAIL for the autonomy criteria. The
  command exits non-zero unless every gate passes.
- **Reproducibility** — the stored records are re-classified (`classify.New()`) and must reproduce the
  findings and decision byte for byte. A run whose config asks for `classify.policy@v1` (Phase 4) is
  re-classified under v1 (same decisions, v1 stamp); see "Policy v2" below.

```
ri -state <cache> -offline eval -investigations -knowledge <snapshot>   # arm A: deterministic only
ri eval -investigations -knowledge <snapshot> -case <id>[,<id>] -o json # a subset, JSON score
ri eval -investigations -knowledge <snapshot> -save                                    # keep results/<run-id>/ (never overwritten)
ri eval -investigations -agent -agent-driver claude-cli -agent-model claude-sonnet-5-5 -save   # arm B
ri eval -investigations -runtime -save                                  # arm C: + observed validation (one
                                                                       # throwaway kind cluster per case, deleted after)
ri eval -investigations -agent -agent-driver replay -agent-replay-dir eval/investigations/results/<B run>   # replay B offline
ri eval -investigations -rescore eval/investigations/results/<run-id>   # re-classify + re-score a stored run offline
```

A stored run (`eval/investigations/results/<run-id>/`) holds, uncompressed, `run.json` (arm, binary commit
and dirty flag, configuration, per-case input/env/expected digests, investigation and bundle ids, record
digests, and every later `-append`), `gates.json` (gate states + autonomy at a glance) and `report.txt`;
compressed, `score.json.gz` (decisions, metrics, splits, gates) and one deterministic `cases.tar.gz` (sorted
entries, zero times, gzip -9) with the per-case records — `request.json`, `investigation.json`, `bundle.json`,
`graph.json`, `impact-report.json`, `changes.json` (+ `transcript.jsonl` when the agent ran) under
`cases/<id>/` — enough to re-score offline. `-rescore` and the replay driver read them transparently (the
legacy uncompressed layout is still read); `ri eval -investigations -pack <run dir>` converts a legacy run,
checking every file byte for byte before removing the original.

`-append <run dir>` adds the selected cases (`-case`) to an existing run — same arm, configuration digest and
binary commit; a case already in the run is refused — and re-scores the run over all its cases. Long arms (the
agent, runtime) are built this way case by case so partial progress is committed. `-rescore` uses the current
classifier: a stored run reproduces byte for byte with the binary of its own commit, and a difference under newer
code shows exactly what a policy change altered.
Scored runs are kept, never overwritten or cherry-picked; the analysis is
`docs/phase4/env-impact/EVAL-REPORT.md`.

### Phase 5: pinned knowledge, agent verdicts, comparison

- **Knowledge is pinned.** `ri eval -investigations` refuses to run without an explicit `-knowledge` naming a snapshot,
  and refuses the live `knowledge/` tree (`./knowledge`, the work tree's, the primary checkout's) unless
  `-allow-live-knowledge`, so a later knowledge change cannot contaminate an arm. `run.json` records
  `knowledgeDigest` (sha256 over the sorted relative paths and bytes of every file in the tree; `none` with
  `-knowledge ""`) and `knowledgeLive`. `-append` refuses a run with a different digest. Runs stored before Phase 5
  have no digest and still load and re-score. `-rescore`, `-pack` and `-compare` read stored runs and need no snapshot.
- **Snapshots.** `scripts/phase5/snapshot.sh <sha> <out>` extracts `knowledge/` of a commit with `git archive` and prints
  the same digest (K0 = `03b6ff43`; K1 = K0 plus the promoted facts, one commit). `scripts/phase5/kbad.sh <K1> <fact-id>
  <out>` builds the PROTOCOL §5 S3 negative-control trees (proxy-only, refused promotion, release outside the
  transition, wrong pin, superseded, retracted, undecidable condition, contradicting active fact) as copies of K1; each
  is documented in `scripts/phase5/kbad/main.go`.
- **Agent verdicts are scored apart from policy.** The agent's provisional class per item (`AgentVerdict`, PROTOCOL §8) is
  matched to expected items with the same matcher as policy findings and scored `correct` / `false-safe` (agent
  not-affected, expected anything else) / `false-alarm` (agent affected, expected not-affected) / `hedged`
  (potentially-affected or insufficient-evidence, not equal to expected) / `overstated` (agent affected, expected
  potentially-affected or insufficient-evidence) / `no-verdict`. Ungated; never read by gates or policy metrics; absent
  from the score when an arm stored no verdicts.
- **`-compare <run>[,label=<run>…]`** prints Markdown tables for stored runs side by side, first = baseline: policy class
  per item, safety rows (false negatives, unsafe eligibility, clearances, unsafe clearances, clearances gained/lost),
  accuracy and coverage/risk detection as separate tables, human-review and cost rows, agent-verdict outcomes. The
  analysis is `docs/phase5/learning-loop/EVAL-REPORT.md`.
- **Upgrade judgements (Phase 6 M3, `-synthesize`)** get an ungated side table (`ScoreJudgements`, `judgements` in the
  score JSON): per case the status, recommendation, chosen / minimum / rule target, the counts of changes (and how many
  have a deterministic location), tests, residual risks, findings (hard) and interactions, and the required parts that
  are missing. It reads no expectation and feeds no gate; it is absent when no run carries a judgement.

### Held-out ground truth (Phase 5)

`eval/investigations-heldout/` holds ten further cases (cert-manager 1.17.2 → 1.18.0, ingress-nginx
4.11.5 → 4.12.0, kube-prometheus-stack 89.2.4 → 90.2.0) in the same format, authored blind by lane
`groundtruth5` on a sealed branch and confirmed by observed upgrades in kind (`README.md` there: case
index and re-run commands; no expected class is stated outside `expected.yaml`). It is the held-out
measurement of `docs/phase5/learning-loop/PROTOCOL.md` §5 S4, scored with `ri eval -investigations -dir
eval/investigations-heldout` once the gates file for it is in place (commander-owned, PROTOCOL §5).
