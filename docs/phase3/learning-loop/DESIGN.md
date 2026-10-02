# The learning loop — design and lane contract

> Owner: `contract` lane. Normative for every wave-1 lane. Traces to
> [MISSION.md](MISSION.md) goal numbers (G1…G25) and to
> [UNKNOWN-ANALYSIS.md](UNKNOWN-ANALYSIS.md) (§x.y / Dnn / Xn / Vn references).
> Types live in `internal/domain/semantic.go` (+ two additive fields in
> `internal/domain/impact.go`); ports live in `internal/knowledge/api.go`.
> Changes to either go through the `contract` owner (FLEET.md).

## 0. The idea in one paragraph

Today a prose change ("The default `rotationPolicy` is now `Always`") reaches the
join as an opaque string, so it becomes `impact:not-joined · unknown` for every
environment. The loop turns that string — once per **release**, not per
environment — into a **verified semantic fact**: a typed subject, a typed change,
a declarative **applicability condition** the deterministic engine evaluates
against `env.Environment`, and a typed consequence that carries the class an
exposed environment receives. Models only *propose* those four parts;
deterministic validators and engineers *verify* them, aspect by aspect; the
impact engine evaluates verified facts against each environment with the same
evidence discipline as the computed join. Nothing a model says reaches a
classification until a validator or a human has verified it (G2, G20, G21).

```text
UpgradeEdge change ──► SemanticCandidate ──► SemanticProposal × N models  (AI, never collapsed)
   (prose/computed)        (sc-…)                (sp-…)
                               │                    │
                               ▼                    ▼
                      ValidationResult (val-…)  ──►  ReviewItem (ri-…)  ──►  ReviewDecision (rd-…)
                      deterministic, per aspect      routed by agreement     human | proxy, labelled
                               │                                                   │
                               └───────────────►  VerifiedFact (vf-…)  ◄───────────┘
                                                  release-level, per-aspect verification,
                                                  anchored to every restatement
                                                        │
             env.Environment ──► condition evaluation ──┤  (internal/impact, deterministic)
                                                        ▼
                                           ImpactFinding  rule impact:knowledge-*
                                           + knowledge ref + unknownReason
                                                        │
                                       ri eval (per verification level) ──► feedback dataset ↺
```

## 1. The semantic knowledge model (G1)

One value type, `domain.SemanticAssertion`, is used by proposals (possibly
partial), corrections and facts (complete):

```go
type SemanticAssertion struct {
    Subject       *Subject        // what thing changed
    Change        *ChangeSpec     // how it changed (before/after, replacedBy)
    Applicability *Applicability  // which environments are exposed (declarative)
    Consequence   *Consequence    // what happens to an exposed environment + its class
    Statement     string          // one-line human rendering, never parsed or compared
}
```

The four parts are the four **aspects** (`subject`, `change`, `applicability`,
`consequence`). Verification, review questions, agreement metrics and
corrections all work per aspect. `AspectDigest(aspect)` is the comparison key:
two assertions agree on an aspect iff their digests are equal. Prose (`Statement`,
`Consequence.Statement/Remediation`) is excluded from digests, so different
wording never counts as disagreement.

### 1.1 Subject — a generic, declarative vocabulary

`Subject{Family, Product, Group, Version, Kind, Path, Name, Component}`. One
struct serves all families. Each family declares which fields identify it, and
every other field must be empty, so a subject has exactly one spelling.
`Subject.Key()` renders the canonical identity, for example
`crd-field:cert-manager|group=cert-manager.io|kind=Certificate|path=spec.privateKey.rotationPolicy`.

| Family | Required | Optional | Example |
|---|---|---|---|
| `crd-field` | Group, Kind, Path | Version | cert-manager.io / Certificate / `spec.privateKey.rotationPolicy` |
| `helm-value` | Path | Name (chart) | `tls.secretsBackend` |
| `gvk` | Version, Kind | Group (`""` = core) | `karpenter.sh/v1beta1 NodePool` |
| `feature-gate` | Name | Component | `ValidateCAA` |
| `image` | Name (repository) | | `quay.io/jetstack/cert-manager-controller` |
| `config-key` | Path | Component (file/binary) | `server.rbac.log.enforce.enable` |
| `rbac-permission` | Name (resource) | Group, Component | `applications/update` |
| `cli-flag` | Name | Component | `--enable-certificate-owner-ref` |
| `env-var` | Name | Component | `LEADER_ELECT` |
| `compatibility-boundary` | Name (platform/operand) | | `kafka` |
| `api-endpoint` | Name (method + path) | Component | `GET /api/v1/applications` |
| `protocol-behavior` | Name | Component | `grpc-alpn` |
| `terraform-attribute` | Kind (resource type), Path | | `aws_s3_bucket` / `acl` |
| `product-relationship` | Name (related product id) | | `ingress-nginx` |
| `migration` | Name (step identifier) | Component | `toFQDNs-selector-migration` |

Families are data. A new family is an enum value plus a row in the table in
`semantic.go`. The engine runs **predicates**, not families, so a new family
needs no evaluator change unless it needs a new predicate (§1.3).

### 1.2 Change — `ChangeSpec{Type, Before, After, ReplacedBy}`

| Type | Meaning | Before / After | ReplacedBy |
|---|---|---|---|
| `added` | subject now exists | – / value (opt.) | – |
| `removed` | subject no longer exists / honoured | value (opt.) / – | optional |
| `renamed` | moved to another identity | – | **required** |
| `default-changed` | default changed | **both** | – |
| `value-changed` | a fixed value changed (image tag, pinned version) | **both** | – |
| `behavior-changed` | same configuration, different semantics | optional | – |
| `deprecated` | still works, scheduled for removal | – | optional |
| `now-required` | previously optional, now mandatory | – / value (opt.) | – |
| `validation-tightened` | values accepted before are now rejected | optional | – |
| `requirement-changed` | a version requirement moved (`compatibility-boundary`, `product-relationship` only) | ranges; After **required** (semver constraint) | – |
| `migration-required` | an operator step must run (`migration` family only) | – | – |

`Before`/`After` hold a JSON-encoded scalar for values (`"\"Never\""`, `"30"`) or
a semver constraint for ranges. They are pointers, so "unset" differs from `""`.
**`ReplacedBy`** (commander decision 4) names the subject that takes over. It
must be of the same family. With it, "old set ∧ new unset" is expressible, and
that fixes the confidently wrong `values-pinned` / `values-unset` answers on
deprecated-and-replaced keys (§2.2 of the analysis: the two current hits and two
of the five dangerous cells).

### 1.3 Applicability — the condition language (commander decision 2)

`Applicability{Exposure Condition, Overlap *Condition}`:

- **Exposure**: the environment state in which the consequence happens.
- **Overlap** (optional): the environment touches the subject but is shielded
  (for example, it pins the old value explicitly). The result is INFORMATIONAL,
  the "applies to you, you appear safe" class.

A `Condition` is a tree evaluated with **three-valued (Kleene) logic**. Each node
yields `true` (exposed, always with environment evidence), `false` (checked
against a supplied, healthy dimension and clear, always with an `ImpactCheck`),
or `unknown` (with an `UnknownReason` and a `neededToDetermine` string).

**Combinators**

| Op | Semantics |
|---|---|
| `all` (conjunction) | any false → false; all true → true; else unknown |
| `any` | any true → true; all false → false; else unknown |
| `not` (exactly one operand) | not(true) = false; not(unknown) = unknown; not(false) = true **only if** the operand examined ≥1 environment fact (its `false` carries the examined evidence, which becomes the `true` evidence). If it examined nothing, the result is unknown (`environment-visibility-gap`). This is the "no line matches" negation; absence of input can never be negated into evidence. |
| `resource` | Group, Kind (+Version, Name): **some single resource** of that kind satisfies all operands, evaluated against that resource. False iff manifests are supplied and healthy and every resource of the kind evaluates false (zero resources of the kind → false, the `impact:crd-field-unset` convention). |

**Scoped leaves** (only inside `resource` or `ref`; the validator enforces this)

| Op | Fields | Bound to (`internal/env/resources.go`) |
|---|---|---|
| `field` (inside `resource` this is the analysis's "resource-field") | Path (`[]` = any list element), State, Values / Pattern / Separator | `FieldValues(sel, path)` → `FieldResult{Set, Facts}` per resource. `unset` ⇔ `!Set` (evidence: `Resource.Evidence`, the document line). `set` counts container facts too (the subtree is set). `equals`/`not-equals`/`matches`/`has-token*` use scalar facts only (`!FieldFact.Container`). |
| `text-line` | Path (values-style, e.g. `data["policy.csv"]`), State `exists` \| `none`, Pattern (RE2) | `TextBlocks(sel, path)` + `TextBlock.Match(re)`. `exists`: true when a line matches (line-level evidence). `none`: true when no line matches (evidence: the block's resource), **unknown** (`environment-visibility-gap`) when `TextBlock.Withheld > 0` or `Truncated` — a withheld line could be the match. The same rule applies to `not(text-line exists)`. |
| `ref` (cross-resource reference, e.g. `Certificate.spec.issuerRef` → Issuer) | Path (the `*Ref` object), Kind?, Group?, operands | `References(sel, path)` / `ResolveRef`: `resolved` → evaluate the operands on `Target`; `ambiguous` → unknown (`semantic-ambiguity`); `unresolved` → false only when `ManifestsComplete`, else unknown (`environment-visibility-gap`). Kind/Group, when given, must match the target. |

**Environment-wide leaves** (never inside a scope)

| Op | Fields | Bound to | `false` requires |
|---|---|---|---|
| `values-key` | Path, State, Values / Pattern / Separator | `Environment.ValuesKeys` (`ValuesKey.Value`) | values supplied, health ok |
| `gvk-in-use` | Group?, Version?, Kind | `Environment.GVKUsage` (manifest use, not CRD presence) | manifests supplied, ok |
| `image-in-use` | Name (repository), State? + Range (tag) | `Environment.Images` | an image-bearing input supplied |
| `cli-flag` | Name, State, …, Component? (container name) | container `args`/`command` field facts (`spec…containers[].args`) of workload resources | manifests supplied, ok |
| `env-var` | Name, State, …, Component? | container `env[]` field facts (name/value of the same element) | manifests supplied, ok |
| `feature-gate` | Name, State (`enabled`/`disabled`/`unset`), Path? (values key carrying the gate list) | `--feature-gates` arg tokens + the values key at Path | manifests (and values when Path is set) supplied, ok |
| `product-version` | Name (product id), State, Range | product inventory (`envinv`, see below) | see the table below |
| `cluster-version` | Name (platform), State, Range | `Environment.Kubernetes` (platform `kubernetes`) | that platform's version supplied |
| `edge-from-version` | State, Range | the edge's **from-version** (`UpgradeEdge.From`), the version the environment runs today (an `input` evidence record of `ri impact <from>`): "precondition already met" | always decidable |
| `rendered-change` | Group?, Kind, Path, Name?, State `changed`\|`unchanged`\|`added`\|`removed`, Values? (To value ∈ Values) | the environment's From vs To render with the customer's values: `internal/render.EvaluateRenderedChange` (render lane; the applicability lane calls it) | both renders succeeded with complete values; render unavailable ⇒ unknown (`environment-visibility-gap`) |
| `undecidable` | Reason, Needed | — (explicit "not statically decidable") | never: always unknown with Reason |

**States**: `unset`, `set`, `equals` (any of `Values`, so equals/in), `not-equals`
(set, to none of `Values`), `matches` (RE2 `Pattern`; a value, or a line for
`text-line`), `has-token` / `has-token-key` (a `Separator`-delimited list, default
`,`, contains one of `Values` exactly / a `k=v` token whose key is one of
`Values`: the `--feature-gates=ValidateCAA=true` shape), `enabled` / `disabled`
for `feature-gate`, `in-range` / `out-of-range` for the versioned leaves,
`exists` / `none` for `text-line`, `changed` / `unchanged` / `added` / `removed` for
`rendered-change`. No range
complement is ever computed: "requires ≥ X" is `out-of-range` of `>=X`.

**Rules the evaluator (applicability lane) must keep**

1. **Absence is not knowledge.** A leaf is `false` only when its deciding
   dimension was supplied **and** `Environment.Health` is `ok`. Under `partial`
   health a would-be `false` becomes `unknown` (`environment-visibility-gap`). A
   `true` backed by evidence stands under partial health.
2. **Withheld values decide nothing.** A predicate that needs a value marked
   `FieldFact.Withheld` (`sensitive` / `oversize`), or a withheld text line, is
   `unknown` (`environment-visibility-gap`, needed: "value withheld (sensitive) at
   <file>:<line>"). `set`/`unset` stay decidable, because presence is not the value.
3. **Scope is per resource.** Operands of one `resource` node are evaluated
   against the same resource, so "a Certificate that sets X **and** references a
   CA Issuer" never pairs two different Certificates.
4. **Every true carries evidence and every false carries a check**, so an
   ACTION finding always has both evidence chains, and a NOT AFFECTED finding
   always has its evaluation record.
5. **No product-specific code.** Every predicate is generic over its fields;
   products appear only as data in facts.

**`product-version` semantics** (envinv: `Environment.Products`,
`Health(env.DimProducts)`, `Product`, `ProductInstances`, `ProductInRange`). These
rules are normative:

| Inventory state for product `Name` | Leaf value |
|---|---|
| dimension `absent` | unknown · `cross-product-context-gap` (needed: "product inventory (--inventory) not supplied") |
| listed (any entry, any kind), `Range == "*"`, State `in-range` (takes precedence) | true: presence is what is asked |
| listed, ≥1 **app-kind** entry (`VersionOf == app`) with a parsable version, and all app-kind entries agree on in/out of `Range` | true/false per State; evidence = those entries |
| listed, app-kind entries disagree (`Conflict`) | unknown · `cross-product-context-gap` (both entries named) |
| listed, only **chart-kind** entries (Helm/Argo/Flux detections) or no parsable version | unknown · `cross-product-context-gap`: a chart version is never read as the app version |
| **not listed**, inventory not declared complete | unknown · `cross-product-context-gap`: absence from a detected or partial inventory is not proof of absence |
| **not listed**, inventory declared complete (`complete: true` in `inventory.yaml`) and health ok | false, with a `products` check citing the declaration |

#### Environment facts this assumes (being built now; commander decision 6)

| Need | Provider | Status |
|---|---|---|
| X1 scalar values on resource fields; X2 `[]` sequence descent; X3 line-addressable embedded text with line evidence; X5 `*Ref` resolution | `envparse`: `Environment.Resources`, `FieldValues`, `TextBlocks`, `References`/`ResolveRef` | merged (cbc80a9) |
| product inventory | `envinv`: `Environment.Products`, `DimProducts` | merged (b9fec21) |
| inventory `complete: true` declaration | `envinv` follow-up (`Environment.InventoryComplete` + evidence) | **requested** |
| CRD per-path `default` / `enum` / `required` in `CRDVersionInfo` (V3) | `capture` | in progress |
| OCI chart values (V4); operand-version tables as compat sources (V5) | `capture` | in progress |
| generic `crd:storage-changed` join rule (V2); `crd:fields-added` → unknownReason | `applicability` | assigned |
| environment From/To renders + `EvaluateRenderedChange`; release-level chart-default renders for the `rendered-diff` validator; `RenderProvenance` | `render` (docs/RENDER.md) | assigned |

**Rename / API suggestions for the env lanes** (follow-up, non-blocking):
(1) export the withheld reasons (`env.WithheldSensitive`, `env.WithheldOversize`)
so the evaluator can name them in `neededToDetermine`; (2) add
`func (e *Environment) ManifestsComplete() bool`, shared by `ResolveRef` and the
evaluator, so both use the same notion; (3) add
`ProductInRangeSemver(id, constraint string, versionOf string) ProductRangeCheck`,
which filters to app-kind entries and takes the condition's `Range` string
directly instead of a `CompatibilityConstraint`; (4) add `InventoryComplete` (above);
(5) `FieldResult.Facts` mixes container and scalar facts: a `Scalars()` helper
would make "equals ignores containers" impossible to forget. Naming in this
document maps onto the analysis's §3.5-2 list as: resource-field = `resource` +
`field`; regex = state `matches`; token in `k=v` list = `has-token(-key)`;
line regex with negation = `text-line` `exists`/`none`; product-version-in-range
= `product-version`; edge from-version = `edge-from-version`; cross-resource
reference = `ref`; conjunction = `all`; not statically decidable = `undecidable`.

#### Canonical applicability (deterministically verifiable)

For some (family, change) pairs the exposure condition follows from the subject's
semantics, not from judgement. A validator may confirm the **applicability**
aspect when the asserted condition equals the canonical one (rule ids
`canonical:<family>/<change>`, data in the validate lane):

| Subject family · change | Canonical Exposure | Canonical Overlap |
|---|---|---|
| `helm-value` · `default-changed` | `values-key{Path, unset}` | `values-key{Path, set}` |
| `helm-value` · `removed` / `renamed` | `values-key{Path, set}` | – |
| `helm-value` · `deprecated` + ReplacedBy | `all[values-key{Path, set}, values-key{ReplacedBy.Path, unset}]` | `values-key{ReplacedBy.Path, set}` |
| `crd-field` · `default-changed` | `resource{G,K, [field{Path, unset}]}` | `resource{G,K, [field{Path, set}]}` |
| `crd-field` · `removed` / `validation-tightened` | `resource{G,K, [field{Path, set}]}` | – |
| `gvk` · `removed` | `gvk-in-use{G,V,K}` | – |
| `image` · `removed` / `value-changed` | `image-in-use{Name}` | – |
| `cli-flag` / `env-var` · `removed` / `renamed` | `cli-flag`/`env-var{Name, set}` | – |
| `product-relationship` · `requirement-changed` | `product-version{Name, out-of-range, Range: After}` | – |
| `compatibility-boundary` · `requirement-changed` | `cluster-version{Name, out-of-range, Range: After}` (platforms) | – |

This table is why most facts need a human only for the **consequence** aspect.

### 1.4 Consequence — `Consequence{Kind, ExposedClass, Statement, Remediation, Severity}`

Commander decisions 3 and C: **the reviewer picks the kind; the class follows,
and the fact carries the mapping explicitly.** `ExposedClass` must equal
`Kind.ExposedClass()`. It is part of the consequence aspect, so it is proposed,
reviewed, corrected and verified with it, and stored in the fact (if the mapping
table is ever revised, old facts still say what they were verified as). The trust
ladder (§4) still caps it. The kinds follow docs/ACTION_CLASSIFICATION.md
("must change … to avoid concrete failure, incompatibility, or loss of intended
behavior"):

| Kind | Meaning | ExposedClass |
|---|---|---|
| `upgrade-blocked` | the upgrade/install itself fails (kubeVersion, a hook, a required precondition) | action-required |
| `resource-rejected` | existing or applied resources are rejected (validation, removed API) | action-required |
| `setting-ignored` | configured values stop being honoured (removed/renamed key, unserved field) | action-required |
| `permission-lost` | a component loses access it relies on (RBAC narrowed) | action-required |
| `workload-failure` | a running workload breaks (incompatible peer, protocol enforcement) | action-required |
| `migration-required` | a mandatory operator step must run before/after the upgrade | action-required |
| `behavior-change` | it still works, but differently (a new default applies); the operator should verify | review-required |
| `deprecation` | works today, scheduled to break later | review-required |
| `none` | no operational consequence | informational |

Action-eligible kinds need a `Statement` that answers "what exactly fails if I do
nothing?". "Precondition satisfied" is not a kind: it is Exposure = false → NOT
AFFECTED. For deprecated + replacedBy with old-set ∧ new-unset, the reviewer
picks `deprecation` (REVIEW) or, when the old key is no longer honoured,
`setting-ignored` (ACTION).

**D11 (commander decision 5).** The dataset labels rotationPolicy on
cert-manager-1.17-1.18 as `review`. That is `behavior-change` → REVIEW REQUIRED,
with nothing special-cased; the demonstration shows REVIEW there (§10).

### 1.5 UNKNOWN reasons (G18)

`domain.UnknownReason` on `ImpactFinding.unknownReason` (unknown-only):

| Reason | Meaning | Routes to |
|---|---|---|
| `release-knowledge-gap` | no trusted fact says what the change means (prose, an unmapped diff rule, proxy-only knowledge) | candidate generation / review |
| `environment-visibility-gap` | the deciding dimension is absent or partial, a value is withheld, or a reference is unresolved in incomplete manifests | the user (supply `--values`, …) |
| `cross-product-context-gap` | depends on other products' presence or versions, and the inventory cannot decide it | the user (`--inventory`) |
| `runtime-behavior-gap` | depends on runtime state no static input carries (live traffic, stored objects) | stays UNKNOWN, documented |
| `evidence-gap` | upstream evidence is insufficient or unstructured (constraint not machine-readable; reviewer said need-more-evidence) | upstream research |
| `semantic-ambiguity` | identity cannot be pinned (unparseable CRD identity; an ambiguous reference; reviewers disagree) | review |

How the existing join assigns them (the applicability lane implements this; until
then the field is optional and `Validate()` checks only its value and class):

| Existing rule / situation | Reason |
|---|---|
| `impact:not-joined`, note-derived change | `release-knowledge-gap` |
| `impact:not-joined`, computed rule without a join rule (incl. `crd:fields-added`); subject-less computed change | `release-knowledge-gap` |
| `impact:not-joined`, CRD change whose API identity cannot be parsed | `semantic-ambiguity` |
| `impact:insufficient-visibility`, dimension not supplied / platform unsuppliable | `environment-visibility-gap` |
| `impact:insufficient-visibility`, constraint not machine-readable | `evidence-gap` |
| `impact:knowledge-undecided` | the reason of the deciding `unknown` leaf (precedence: cross-product → environment-visibility → runtime-behavior → evidence → semantic-ambiguity), or `release-knowledge-gap` when only proxy-level knowledge exists |

Once the join assigns a reason everywhere, the applicability lane makes the
field **mandatory** on unknown findings in `Validate()` and regenerates the
goldens in the same commit, stating why they changed.

## 2. Entities, lifecycle, identity

All IDs are content-derived (`domain.ShortHash`), prefix-typed and stable across
re-runs. Every entity has a `Validate()` in `semantic.go`. A
`domain.KnowledgeRecord` envelope wraps each one on disk.

| Entity | ID | Derived from | Mutable fields |
|---|---|---|---|
| `SemanticCandidate` | `sc-` | product, release, sorted member identities (statement keys for prose members, change ids for computed members) | none |
| `SemanticProposal` | `sp-` | candidate, task, provider, model, modelVersion, promptDigest | none |
| `ValidationResult` | `val-` | candidate, validator, checked assertion digest | none |
| `ReviewItem` | `ri-` | candidate, question type, proposed assertion digest | `status` |
| `ReviewDecision` | `rd-` | review item, reviewer, decidedAt | none (append-only) |
| `VerifiedFact` | `vf-` | product, introducing release, assertion digest (which includes the subject) | `status`, `anchors`, `candidates` |

### 2.1 SemanticCandidate = a restatement cluster (commander review A)

One upstream change is restated 3–6 times across sources (cm-1.17 E1: the
upgrade guide, the release notes and the GitHub release). A candidate is therefore
a **restatement cluster**: `{ID, Product, Release, Members []CandidateMember,
Grouping, Category, Title, Text, Evidence []Evidence, Hints, Producer, CreatedAt}`
with `CandidateMember{ChangeID, Anchor?, Computed}`. **One proposal set and one
review per cluster; the resulting fact attaches to every member.** Otherwise
review volume multiplies and the restatement cells stay UNKNOWN.

The semantic lane builds the clusters deterministically, recording the rule in
`Grouping`:

1. `same-statement`: changes sharing a statement key (see `ChangeAnchor` below).
2. `title-jaccard` / `same-title` / `same-subjects`: the duplicate shapes shared
   by `internal/impactenrich` clusters and the `internal/eval` duplicate audit
   (identical normalized title; identical category + subject set; ≥ 0.75
   title-token Jaccard on the same non-empty subjects).
3. `subject-named`: a computed diff joins the cluster of a note that names its
   subject verbatim (a values key path, a CRD field path with its kind, an image
   repository).

Clusters are **never formed across distinct subjects**: if two members name
different key paths, fields or images, they are separate candidates. They
**never include an umbrella** (`domain.IsUmbrella` for notes; for computed
diffs, a change whose subjects reach outside the cluster's subject). A singleton
is `Grouping: single`.

Members are the edge changes the join leaves `unknown` by construction:
non-routine note-derived changes that are not `impact:security-fix`, and computed
changes of diff rules without a join rule. A computed diff that already has a
join rule may also join a cluster as a `subject-named` member, so the fact can
explain it. Candidates come from the **edge**, not from a report, so they are
environment-independent. `Evidence` snapshots the members' full records plus
any artifact context, and proposals may cite only it. `Hints` are deterministic
token pre-extractions, never conclusions.

**`ChangeAnchor{Release, EvidenceKeys, StatementKeys, ChangeIDs}`** is the
statement-level identity of a prose change:

- `EvidenceKey` = hash(kind, URI, locator, excerpt) (digest-free).
- `StatementKey` = hash(kind, URI, excerpt reduced to lowercase alphanumerics):
  it survives lines shifting above the statement and markup edits.
- `ChangeIDs` are recorded for humans and **never matched on** (commander
  decision 1: `chg-` ids are content-derived from note text and shift with it).

Every prose member carries a `ChangeAnchor`. `anchor.Matches(c)` holds iff `c` is **note-derived**, the releases are
compatible (equal, or either unstated), and one of `c`'s evidence records has an
anchored evidence or statement key. Computed changes never match an anchor:
they share structured evidence (one `values.yaml` record backs every values
diff), so they attach by subject (§2.6).

**Umbrellas** — `domain.IsUmbrella(c)`: a note-derived change that quotes two or
more *different* statements of the same document, or whose detail lists two or
more items. Facts never attach to umbrellas (D13 shadowing).

### 2.2 SemanticProposal (one per model × task; never collapsed — G2, G3)

`{ID, CandidateID, Task, Provider, Assertion, Undetermined, UndeterminedReason,
DuplicateOf, SuggestedClass, Citations, Provenance}`.

- `Task`: `semantic-mapping` (subject + change), `applicability`, `consequence`,
  `relationship` (subject + change + applicability), `duplicate`, `full`. An
  answer must address every aspect of its task, either asserted or listed in
  `Undetermined` with a reason. Abstention is a first-class answer.
- `Provider` (`anthropic`, `zai`, `typesafe`, …) is required. `Provenance` is the
  existing complete AI provenance; confidence is capped at `medium`.
- `SuggestedClass` ∈ {`""`, `review-required`, `informational`, `unknown`}: a
  model can never suggest `action-required` or `not-affected`.
- `Citations ⊆ Provenance.InputEvidence`, and `ValidateAgainst(candidate)` checks
  that citations are candidate evidence.

### 2.3 ValidationResult (deterministic validators — G5)

`{ID, CandidateID, ProposalID?, Validator, Assertion, Checks []AspectCheck,
Evidence, CheckedAt}`; `AspectCheck{Aspect, Outcome confirmed|refuted|inconclusive,
Rule, Detail}`. A confirmation must cite the artifact evidence it rests on.
Validators never confirm a consequence (that is a judgement); the one exception is
kind `none` + `informational`.

**Rendered evidence.** A rendered object field cites the template file (Helm's
`# Source:` comment) as URI/locator, plus `Evidence.Render`
(`domain.RenderProvenance{Scope, Tool, ToolVersion, ChartDigest, ValuesDigest,
Flags}`). Release-level (chart-default, `scope: release`) renders may back facts.
Environment renders (`scope: environment`, the customer's values) may **never**
enter `knowledge/`: every knowledge entity's evidence validation rejects them.

| Validator | Inputs (ingested snapshots only) | Confirms | Analysis ref |
|---|---|---|---|
| `helm-values` | `ValuesSnapshot` from/to (+ OCI chart values, V4) | subject, change (removed / added / default before→after / renamed when the replacement appears) | V1, V4 |
| `crd-schema` | `CRDSnapshot` from/to with per-path default/enum/required (V3) | subject, change (field/version removed, default before→after, enum value renamed, now-required, storage version moved) | V3, V6 |
| `compatibility` | `CompatibilityConstraint` rows (+ operand tables, V5), chart `kubeVersion` | `compatibility-boundary` / `product-relationship` subject + change | V5 |
| `image` | `image-refs` snapshots, artifact instances | image subject + change | – |
| `rendered-diff` | release-level renders of the From and To charts with **chart defaults** (render lane) | subject + change for `rbac-permission`, `cli-flag`, `feature-gate`, `env-var`, `image` and resource shape (fields added/removed in rendered objects); can also **refute** them | – |
| `canonical-applicability` | the asserted subject/change/condition | applicability, when it equals the canonical condition (§1.3) **and** subject + change are confirmed | – |

### 2.4 ReviewItem (G6, G8, G10)

`{ID, CandidateID, Product, Release, QuestionType, Question, Proposed, Proposals,
Validations, Routing, Context?, Status, CreatedAt}`.

| Question type | Verifies aspects | Example |
|---|---|---|
| `semantic-mapping` | subject, change | "Does this note mean `Certificate.spec.privateKey.rotationPolicy` default Never → Always?" |
| `applicability` | applicability | "Does a Certificate with rotationPolicy **unset** become affected?" |
| `consequence` | consequence (incl. ExposedClass) | "If an exposed Certificate does nothing, what happens, and is that review or action?" |
| `classification` | consequence | "Action-eligible failure, deprecation, or no-op?" |
| `relationship` | subject, change, applicability | "Does v1.18 require ingress-nginx ≥ X for HTTP01?" |
| `duplicate` | – (adds the candidate's anchor to a fact) | "Is this the same change as vf-…?" |
| `evidence-sufficiency` | – (a gate) | "Is the cited text enough to decide?" |

`Proposed` must state every aspect its question verifies. `Context` (optional
`{Label, Digest}`) records an environment illustration. Items are
environment-free by default (§7). `Status`: `pending | needs-evidence | deferred |
decided | superseded`.

### 2.5 ReviewDecision (G8, G9, G11, G13)

`{ID, ReviewItemID, Action, Labels, Original, Corrected, Reviewer, ReviewerKind,
ProxyProvenance, Reason, StartedAt, DecidedAt, ResultingFact, DuplicateOf}`.

- Actions `accept | reject | correct | need-more-evidence | defer`. Labels (G13)
  must fit the action: accept ⇒ exactly `accepted`; correct ⇒ `corrected` + ≥1
  `wrong-*`; reject ⇒ `rejected` or `duplicate` (+ `wrong-*`); need-more-evidence
  ⇒ exactly `insufficient-evidence`; defer ⇒ no label. Reject, correct and
  need-more-evidence require a reason.
- **Correction is first-class (G9):** `Original` is the assertion shown;
  `Corrected` is set iff the action is correct, must change ≥1 aspect digest, and
  may not drop an aspect.
- `ReviewerKind human | proxy`: a proxy **must** carry complete AI
  `ProxyProvenance`, and a human must not. No field lets a proxy claim to be
  human, and the fact basis check re-proves this from records (§2.6).
- `StartedAt`/`DecidedAt` are required for accept/correct/reject (time to
  decision, G24). Rejections are retained negative examples (G11).
- **Bulk review** (product-owner request): decisions submitted together by one
  dashboard bulk action share `BatchID` (`rb-…`, `domain.BatchID(itemIDs,
  reviewer, decidedAt)`) and `BatchSize`. There is still **one decision per
  item**. Invariants: `BatchID` set ⇔ `BatchSize ≥ 2`, and `correct` is never
  batched (corrections are individual-only).

### 2.6 VerifiedFact and attachment (G11, G12; commander decision 1)

`{ID, Product, Release, Anchors []ChangeAnchor (all prose members of its clusters), Candidates, Assertion,
Verification []AspectVerification, Evidence, Status, Supersedes, CreatedAt}`;
`AspectVerification{Aspect, Level deterministic|human|proxy, Basis []id}`.

The identity key is product + introducing release + assertion (which contains
the subject); the evidence locators live in `Anchors`. Invariants (`Validate()`):
the assertion is complete; each of the four aspects has one verification with
≥1 basis record (`deterministic` ⇒ `val-…`; `human`/`proxy` ⇒ `rd-…`); evidence
is upstream only (`local-file`/`input` are rejected, so a fact never names an
environment); anchors share the fact's release. `fact.Level()` is the weakest
aspect level.

`domain.ValidateFactBasis(fact, validations, decisions, items)` re-proves each
aspect from its records. A `val-` basis must confirm that aspect on an assertion
with the same aspect digest. An `rd-` basis must be an accept/correct whose
`ReviewerKind` **equals** the claimed level, whose item's question verifies that
aspect, and whose final assertion has the same aspect digest.

**Attachment: a fact attaches to every change of an edge that restates it.**
The applicability lane implements this algorithm; the domain provides
`AttachesByAnchor` and `IsUmbrella`. In an edge `E` of the fact's product:

1. **Anchor matches.** Take every note-derived, non-umbrella change of `E` in the
   fact's release whose statement matches one of the fact's anchors
   (`fact.AttachesByAnchor`). The anchors are the prose members of every
   candidate cluster that produced or was marked duplicate of the fact.
2. **Subject restatements.** Add every computed change of `E` whose subject *is*
   the fact's subject, using the identity parsing the join already does:
   `helm-value` ↔ `values:*` changes whose subjects lie at/under `Subject.Path`;
   `crd-field`/`gvk` ↔ `crd:*` changes whose parsed GVK (`crd_gvk.go`) and path
   match; `image` ↔ `images:*` / image artifacts by repository. A computed change
   with subjects **outside** the fact's subject subtree is an umbrella for that
   fact and is skipped.
3. **Restatement grouping.** Close the set under the deterministic duplicate
   grouping (identical normalized title; identical category + subject set; ≥ 0.75
   title-token Jaccard on the same non-empty subjects — the shape shared by
   `internal/eval` and `impactenrich`), again skipping umbrellas.
4. Each attached change gets its own knowledge finding (§4). A reviewer's
   `duplicate` decision adds the duplicate candidate's anchor to
   `fact.Anchors`, so the restatement is remembered across runs.

## 3. Lifecycle

```text
candidate ─► proposals (N models × task) ─► validations ─► route ─┬─► every aspect confirmed ─► fact (deterministic)
                                                                  └─► review item(s) ─► decision ─┬─ accept/correct ─► fact
                                                                                                   ├─ reject ─► negative example
                                                                                                   ├─ duplicate ─► anchor added to the fact
                                                                                                   ├─ need-more-evidence ─► item needs-evidence
                                                                                                   └─ defer ─► item deferred
```

A fact is minted when each of the four aspects has a verification at some
level. Validator-confirmed aspects need no human, so the review item asks only
for the rest (typically the consequence, often the applicability). To retract,
a later decision on a fact-review item sets the fact `retracted`. A corrected
re-review mints a new fact that `Supersedes` the old one. Only `active` facts
are used.

## 4. Trust ladder → classification (G20, G21)

Inputs per (fact F, environment): `L` = F.Level(); `C` = F's
`Consequence.ExposedClass`; `X` = Exposure evaluated; `O` = Overlap evaluated
(false if absent). "Trusted" means `deterministic` or `human`.

| X | O | L trusted | L proxy |
|---|---|---|---|
| true | – | **C** (action-required only with the confidence `high` the evaluation earns; see rule 1) | min(C, **review-required**), confidence medium, flagged `verification: proxy` |
| false | true | informational | informational |
| false | false/unknown | **not-affected** with checks | **unknown** (`release-knowledge-gap`: a proxy may never clear) |
| unknown | – | unknown (leaf reason) | unknown (leaf reason) |

Hard rules. `ImpactReport.Validate()` and the JSON Schema enforce them on findings
that carry `ImpactFinding.Knowledge` (rule prefix `impact:knowledge-`):

1. **ACTION REQUIRED** requires all of: `Knowledge.Verification` trusted, so
   subject, change, consequence (with its ExposedClass) **and** applicability are
   each verified by a human or deterministically (this is stricter than the
   commander's minimum, on purpose: an unverified condition must not decide
   mandatory work); `X` true with matches and environment evidence (both chains,
   the existing affected-class rule); and confidence `high` (the existing
   demotion rule).
2. **not-affected** from knowledge requires a trusted verification and checks.
3. A **proxy**-verified knowledge finding is review-required, informational or
   unknown: never action-required, never not-affected, never high confidence.
4. **Model proposals alone never change a classification.** `impact.Build`
   receives facts (`[]VerifiedFact`), never proposals. The existing
   `SuggestedClassification` path (impactenrich) is unchanged and stays
   `unknown`-only.
5. `Knowledge != nil` ⇔ the rule starts with `impact:knowledge-`; `unknownReason`
   is unknown-only.
6. **Supersession:** a change with a knowledge finding carries **no other unknown
   finding**. The knowledge finding replaces the `not-joined` /
   `insufficient-visibility` record, so nothing is counted twice.

**Composition with the existing join** (applicability lane, `internal/impact`):

- Deterministic findings are kept as they are. Per attached change, a knowledge
  finding **replaces** the change's unknown records. Next to any deterministic
  affected or not-affected finding, it is **added only if stronger** (action >
  review > informational > not-affected). It never removes or downgrades a
  deterministic finding: a knowledge not-affected never hides a computed
  action-required.
- Rules: `impact:knowledge-exposed` (affected, from Exposure),
  `impact:knowledge-overlap` (informational, from Overlap),
  `impact:knowledge-clear` (not-affected), `impact:knowledge-undecided` (unknown).
  Provenance is `method: computed`, producer `impact.knowledge@v1`: the evaluation
  is deterministic, and the fact's own verification travels in `Knowledge`.
- The upstream chain is the change's evidence ∪ the fact's evidence (the engine
  copies the fact's records into the report pool).
- `impact.Input` gains `Facts []domain.VerifiedFact` and `MinVerification`.
  Without facts, `Build` output is byte-identical to today, so the goldens do not
  change.

## 5. Cross-product knowledge (G19)

No special construct. A cross-product requirement is a fact whose subject is
`product-relationship{Name: <other product>}` (or a `compatibility-boundary`),
whose change is `requirement-changed` with `After` the required range, and whose
Exposure uses `product-version{Name, out-of-range, Range}`. For example, "HTTP01
paths are now PathType Exact; ingress-nginx ≥ 1.12 rejects them under strict
path validation" becomes:

```yaml
exposure:
  op: all
  of:
    - {op: product-version, name: ingress-nginx, state: in-range, range: ">=1.12.0"}
    - op: resource
      group: acme.cert-manager.io   # Issuer / ClusterIssuer
      kind: Issuer
      of: [{op: field, path: "spec.acme.solvers[].http01.ingress", state: set}]
```

It evaluates with the `product-version` semantics of §1.3: only app-kind
versions decide, and a product missing from an inventory that is not declared
complete is `unknown / cross-product-context-gap`, never "not installed". "Argo
CD manages cert-manager resources" is likewise `product-version{argo-cd,
in-range, "*"}` inside an Exposure.

## 6. Routing (G16: hypotheses, measured, not assumed)

Signals are computed per aspect from proposals and validations. The route is
recorded on the item (`Routing{Route, Priority, Signals}`), so outcomes can test
each hypothesis later.

| Signals | Route | Priority |
|---|---|---|
| validation `confirmed` for every aspect | `auto-verify` (no review item; fact at `deterministic`) | – |
| models agree on the open aspects + validation confirmed the rest | `review` | low |
| models agree, no validation possible | `review` | normal |
| models disagree on any aspect, or a validation `refuted` a proposal | `review` | normal (high if `high-impact`) |
| every model abstained | `missing-evidence` | low |
| any proposal's consequence kind is action-eligible (`high-impact`) | `review` | high |

"Agree" means equal aspect digests across ≥2 proposals from **distinct models**.
A single-model proposal carries `single-model` and never counts as agreement.

## 7. Measuring the loop honestly

**Per verification level.** `ri eval -knowledge <dir> -min-verification <level>`
runs the dataset at each level and reports each separately (G21, FLEET):

| Level | Facts used | Role |
|---|---|---|
| `none` | no facts | today's baseline |
| `deterministic` | facts whose every aspect is deterministic | reported |
| `human` | deterministic ∪ human | **the gate number** |
| `proxy` | all active facts | reported, labelled proxy, never presented as the gate |

A proxy fact can reach review-required, which is an affected class, so it counts
as a link hit. That is exactly why the proxy number must never be the headline.

**Blind authoring.** Fact authors and reviewers never see `eval/cases/*/case.yaml`
expectations. Prompts are built from the edge only, and review items are
environment-free by default. A knowledge-lane test fails if any file under
`knowledge/` mentions `eval/cases` or cites expected-item ids.

**Transfer (G12).** Each review item records its `Context`. Per level, the eval
reports `applicabilityAccuracy` overall and on the **transfer subset**: links
whose deciding facts were never reviewed with that case's environment as
context.

**Ceilings to expect** (analysis §3.3, labels unchanged): deterministic only
0.24 (0.38 with capture V3–V5); + human-verified facts 0.76; + cross-product
inventory 0.90. **0.80 needs the inventory tier.** UnknownRate barely moves
(~0.70); its lever is UNKNOWN reasons + routing, not link closure. Class quality
(classificationAccuracy) is a separate axis: several links land on a softer
class than their label by design (D11).

**Metrics each entity must support** (computed by the knowledge lane; G4, G15, G24):

| Metric | From |
|---|---|
| agreement rate, pairwise agreement per aspect and per task | proposals (aspect digests) |
| per model: accepted as-is / after correction / rejected / insufficient | decisions ↔ proposals (proposal aspect digest vs Original/Corrected) |
| per model: FP / FN per aspect vs final facts | proposals vs facts |
| review volume per release / product / question type | review items |
| median and p90 time to decision | decisions |
| batch vs individual: decision counts, accept/correct/reject rates, timing, number of batches — always reported **separately** (bulk accepts must not inflate the per-item acceptance rate or deflate the per-item time) | decisions (`BatchID`, `BatchSize`) |
| auto-validation rate | facts with all aspects deterministic / all facts |
| acceptance / correction / rejection rate | decisions |
| repeat-pattern rate | decisions whose (family, change type, consequence kind, exposed class) matches an earlier fact |
| grounding quality per model | proposals: citations ⊆ input, citation reuse by the accepted fact |
| UNKNOWN → verified → class transitions | eval runs per level (G17) |

## 8. Storage

**Committed JSON files under a top-level `knowledge/` directory**, one file per
entity, each wrapped in `domain.KnowledgeRecord` (`schemaVersion:
ri.dev/knowledge/v1alpha1`, schema `schemas/knowledge-record.schema.json`):

```text
knowledge/<product>/<release>/candidates/sc-….json
                              proposals/sp-….json
                              validations/val-….json
                              reviews/ri-….json
                              decisions/rd-….json
                              facts/vf-….json
```

`<release>` is the candidate's/fact's release (`_endpoint` when there is none).
Records other than candidates and facts are placed by resolving their candidate,
so the candidate is written first. Why files and not SQLite:

- no new dependency;
- every decision is a reviewable diff in a PR, and git history is the audit log;
- content IDs make writes idempotent and merges conflict-free;
- hundreds to low thousands of records per product load in milliseconds.

Decisions are append-only. Review items change only `status`; facts change only
`status`, `anchors` and `candidates`. The `knowledge.Store` port admits a
database later without touching any consumer. The dashboard (`ri review serve`)
is stdlib only (`net/http`, `html/template`, `embed`) and goes through the same
port.

## 9. Lane interface map (wave 1)

Ports are in `internal/knowledge/api.go` (interfaces plus their data types:
`Store`, `Proposer`, `Validator`, `Queue`, `Snapshot`, `ReviewContext`,
`LoopMetrics`). Every
lane depends on `internal/domain` and may import `internal/knowledge` for the
ports. No lane edits another lane's package.

### `semantic` — candidates + multi-model proposals (G1–G3)

Package `internal/semantic` (new): `candidates.go`, `prompt.go`, `proposer.go`,
`answer.go`, tests, `docs/SEMANTIC.md`.

```go
func Candidates(edge *domain.UpgradeEdge, now time.Time) []domain.SemanticCandidate // restatement clusters (§2.1)
// LLMProposer implements knowledge.Proposer over llm.Client (Anthropic Messages API,
// Anthropic-compatible gateways such as Z.AI for GLM, the file exchange, the cache).
func NewLLMProposer(client llm.Client, provider, model string, opts ProposerOptions) *LLMProposer
func AnswerSchema(task domain.ProposalTask) json.RawMessage // enumerated, typed; every enum from domain
func ProposeAll(ctx context.Context, cands []domain.SemanticCandidate, ps []knowledge.Proposer,
    tasks []domain.ProposalTask) ([]domain.SemanticProposal, []knowledge.ProposalFailure)
```

`knowledge.Proposer` is the only seam, so the lane stays provider-agnostic: a
typed "System One" model is just another implementation returning the same typed
answer. The answer schema is enumerated (families, change kinds, ops, states,
consequence kinds, exposed classes, `undetermined`); free text appears only in
`Statement`/`Detail`. Values are never sent, and prompts never include eval
expectations. Each proposal passes `ValidateAgainst(candidate)` before it is
stored, and failures are recorded, never dropped. Clusters never include
umbrellas or cross distinct subjects (§2.1).

### `validate` — deterministic validators (G5)

Package `internal/semvalidate` (new): `values.go`, `crd.go`, `compat.go`,
`images.go`, `canonical.go`, tests.

```go
func Validators() []knowledge.Validator // helm-values, crd-schema, compatibility, image, canonical-applicability
// rendered-diff is implemented by the render lane against the same port.
```

Inputs are the from/to `domain.Release` snapshots (via `internal/store`), with no
new fetches. It consumes the `capture` lane's V3–V5 snapshot additions as they
land.

### `knowledge` — store, queue, decisions → facts, dataset, metrics (G6, G11–G16, G24)

Package `internal/knowledge`: `filestore.go`, `route.go`, `decide.go`,
`dataset.go`, `metrics.go`, `pipeline.go`, tests; CLI
`ri knowledge candidates|propose|validate|route|export|metrics`
(`cmd/ri/knowledge.go`).

```go
func NewFileStore(dir string) knowledge.Store
func NewQueue(s knowledge.Store, now func() time.Time) knowledge.Queue
func Route(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult) knowledge.RouteResult
func ExportDataset(s *knowledge.Snapshot, w io.Writer) error // JSONL of domain.FeedbackExample
func ComputeMetrics(s *knowledge.Snapshot) knowledge.LoopMetrics
```

Every minted fact passes `VerifiedFact.Validate()` **and**
`domain.ValidateFactBasis`. The integrity test (§7) lives here.

### `applicability` — fact × environment in the engine (G17, G18, G20, G21)

`internal/impact` (`knowledge.go`, `condition.go`, tests, `render.go`
additions), `internal/eval` knowledge wiring, `cmd/ri` flags.

```go
// impact.Input gains (additive): Facts []domain.VerifiedFact; MinVerification domain.VerificationLevel
func EvaluateCondition(c domain.Condition, e *env.Environment, edge *domain.UpgradeEdge) ConditionResult
    // {Value True|False|Unknown, Matches, Checks, Examined []EvidenceID, Reason, Needed}
func ClassifyKnowledge(f domain.VerifiedFact, exposure, overlap ConditionResult) (domain.ImpactClass, domain.Confidence, domain.UnknownReason)
func AttachedChanges(f domain.VerifiedFact, edge *domain.UpgradeEdge) []domain.Change // §2.6 steps 1–3
```

This lane also assigns `unknownReason` across the existing join (§1.5), then
makes it mandatory. It owns V2 (`crd:storage-changed` join rule). It adds
`-knowledge` / `-min-verification` to `ri impact` and `ri eval`, with per-level
and transfer reporting (§7). `go test ./internal/eval` must show no regression
when `-knowledge` is unset.

### `dashboard` — `ri review serve` (G7, G8)

Package `internal/reviewui` (new): `server.go`, `templates/*.html` via `embed`;
`cmd/ri/review.go`.

```go
func NewServer(q knowledge.Queue, opts Options) http.Handler
// GET / (inbox counts + filters) · GET /items/{id} (knowledge.ReviewContext)
// POST /items/{id}/decision (accept|reject|correct|need-more-evidence|defer)
```

The inbox counts and filters are exactly those of G7, and the item page shows
exactly the G8 list, all from `knowledge.ReviewContext`. In the consequence form
the reviewer picks the kind, and the form shows the class that follows
(`Kind.ExposedClass()`). `StartedAt` is recorded when the item page is served and `DecidedAt` on
submit. Reviewer identity is required; `ReviewerKind` is fixed to `human` in the
UI (proxy decisions come only from the CLI, with provenance). Bulk actions submit
one decision per selected item with a shared `BatchID` through one
`Queue.Decide` call; correction is offered per item only. stdlib only, no auth
(non-goal).

### `render` — rendered-manifest diffs (product-owner addendum)

`internal/render` + docs/RENDER.md (the render lane fills in the details):
`EvaluateRenderedChange(cond domain.Condition, …) ConditionResult` for the
`rendered-change` predicate (environment renders: From vs To with the customer's
values), and the `rendered-diff` `knowledge.Validator` over release-level
chart-default renders. Every rendered field's evidence carries
`domain.RenderProvenance`.

### `groundtruth` — dataset expansion (G22)

Owns `eval/cases/**` (new cases + additive expectation fields), `eval/FORMAT.md`,
and `internal/eval/case.go` (additive types + validation only). It adds an
optional block per expected item, authored blind from upstream sources:

```yaml
semantics:
  subject: {family: crd-field, product: cert-manager, group: cert-manager.io, kind: Certificate, path: spec.privateKey.rotationPolicy}
  change: {type: default-changed, before: '"Never"', after: '"Always"'}
  consequence: {kind: behavior-change, exposedClass: review-required}
```

and per `expectedImpact` link:

```yaml
    exposure:                      # domain condition language, verbatim
      op: resource
      group: cert-manager.io
      kind: Certificate
      of: [{op: field, path: spec.privateKey.rotationPolicy, state: unset}]
    environmentEvidence: manifests/certificates.yaml
```

Field names mirror the domain JSON names, so `internal/eval` decodes them
directly into `domain.Subject` / `ChangeSpec` / `Consequence` / `Condition`.
These labels score the *semantic* stage (per-model subject/change/applicability
accuracy, G15) and are never read by knowledge authoring. Dataset observations
D1–D13 of the analysis are this lane's (or the commander's) to decide.

## 10. The required demonstration, mapped (commander review D)

**Path 1: the MISSION demo on rotationPolicy, which ends at REVIEW.**

| Step (MISSION) | Entity / mechanism |
|---|---|
| 1 discover the prose change | the note, upgrade-guide and GitHub-release restatements → **one** `SemanticCandidate` cluster (`same-statement` / `title-jaccard`) |
| 2 GLM: plausibly `Certificate.rotationPolicy` | `SemanticProposal` (provider `zai`, model `glm-5.3-flash`, task `full`) |
| 3 Sonnet: undetermined | `SemanticProposal` with `Undetermined: [subject, change, …]`, stored and not collapsed |
| 4 artifact inspection: field exists, default changed | `ValidationResult` `crd-schema` (subject + change confirmed; needs V3 per-path defaults, or the schema must state the default); `canonical-applicability` confirms `resource{Certificate, [field{rotationPolicy, unset}]}` |
| 5 applicability not fully provable | the consequence is open → route `review` (models disagree ⇒ normal) |
| 6 "Does a Certificate with rotationPolicy unset become affected …?" | one `ReviewItem` for the cluster (`consequence`, with the canonical applicability shown) |
| 7 engineer: ACCEPT | `ReviewDecision{accept, human, [accepted]}` on consequence `behavior-change` |
| 8 rule stored | `VerifiedFact` (subject, change, applicability deterministic; consequence human), anchored to every member |
| 9 11 Certificates with rotationPolicy unset | the `resource` scope is true for 11 resources, each with document evidence |
| 10 classification | trusted fact + exposure true + `behavior-change` ⇒ **REVIEW REQUIRED** on every restating change (the dataset label stands, D11; ACTION would need the user to decide otherwise) |
| 11 reuse without re-review | `AttachedChanges` in any edge traversing v1.18.0, any environment, any re-ingestion |

**Path 2: the ACTION path end to end, with a failure-kind fact.** Example:
cert-manager v1.18 HTTP01 solver paths become `PathType: Exact`, and
ingress-nginx ≥ 1.12 rejects them under strict path validation.

| Step | Entity / mechanism |
|---|---|
| candidate | the cluster of the restating notes (upgrade guide + release notes) |
| proposals | subject `product-relationship{ingress-nginx}`, change `requirement-changed`, consequence `workload-failure` ("HTTP01 challenges fail: ingress-nginx rejects the Exact-path Ingress the solver creates") |
| validation | none can prove a prose cross-product claim → review items `relationship` + `consequence` |
| decisions | human accepts both → fact: subject/change/applicability/consequence all `human`, `ExposedClass: action-required` |
| exposure | `all[product-version{ingress-nginx, in-range, ">=1.12.0"}, resource{Issuer, [field{spec.acme.solvers[].http01.ingress, set}]}]` |
| environment | the fixture inventory declares ingress-nginx v1.12.1 (app-kind, declared) **and** the manifests contain an Issuer with an HTTP01 ingress solver → both leaves true, with evidence |
| classification | trusted fact + exposure true **deterministically on both chains** + action-eligible kind ⇒ **ACTION REQUIRED** (`impact:knowledge-exposed`, confidence high) |
| guard | if either leaf is unknown (no inventory, only a chart-kind version, an undeclared-complete inventory without the product, partial manifests) ⇒ UNKNOWN with the leaf's reason. If the fact were proxy-verified ⇒ REVIEW at most. |

> Machines propose. Evidence constrains. Engineers resolve ambiguity. Verified knowledge compounds.
