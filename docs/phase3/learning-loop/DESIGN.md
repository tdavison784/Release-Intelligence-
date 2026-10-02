# The learning loop — design and lane contract

> Owner: `contract` lane. Normative for every wave-1 lane. Traces to
> [MISSION.md](MISSION.md) goal numbers (G1…G25). Types live in
> `internal/domain/semantic.go`; ports live in `internal/knowledge/api.go`.
> Changes to either go through the `contract` owner (FLEET.md).

## 0. The idea in one paragraph

Today a prose change ("The default `rotationPolicy` is now `Always`") reaches the
join as an opaque string, so it becomes `impact:not-joined · unknown` for every
environment. The loop turns that string — once per **release**, not per
environment — into a **verified semantic fact**: a typed subject, a typed change,
a declarative **applicability condition** the deterministic engine can evaluate
against `env.Environment`, and a typed consequence. Models only *propose* those
four parts; deterministic validators and engineers *verify* them, aspect by
aspect; the impact engine then evaluates verified facts against each
environment with the same evidence discipline as the computed join. Nothing a
model says reaches a classification until a validator or a human has verified it
(G2, G20, G21).

```text
UpgradeEdge change ──► SemanticCandidate ──► SemanticProposal × N models  (AI, never collapsed)
   (prose/computed)        (sc-…)                (sp-…)
                               │                    │
                               ▼                    ▼
                      ValidationResult (val-…)  ──►  ReviewItem (ri-…)  ──►  ReviewDecision (rd-…)
                      deterministic, per aspect      routed by agreement     human | proxy, labelled
                               │                                                   │
                               └───────────────►  VerifiedFact (vf-…)  ◄───────────┘
                                                  release-level, per-aspect verification
                                                        │
             env.Environment ──► condition evaluation ──┤  (internal/impact, deterministic)
                                                        ▼
                                           ImpactFinding  rule impact:knowledge-*
                                           + knowledge ref + unknownReason
                                                        │
                                       ri eval (per verification level) ──► feedback dataset ↺
```

## 1. The semantic knowledge model (G1)

All of it is one value type, `domain.SemanticAssertion`, used identically by
proposals (possibly partial), corrections and facts (complete):

```go
type SemanticAssertion struct {
    Subject       *Subject        // what thing changed
    Change        *ChangeSpec     // how it changed (before/after)
    Applicability *Applicability  // which environments are exposed (declarative)
    Consequence   *Consequence    // what happens to an exposed environment that does nothing
    Statement     string          // one-line human rendering, never parsed
}
```

The four parts are the four **aspects** (`subject`, `change`, `applicability`,
`consequence`). Verification, review questions, agreement metrics and
corrections are all per aspect. Classification is **derived** (§4), never
asserted — the "classification" review question type verifies the consequence
kind, from which the class follows.

### 1.1 Subject — a generic, declarative vocabulary

`Subject{Family, Product, Group, Version, Kind, Path, Name, Component}`. One
struct for all families; each family declares which fields identify it.
`Subject.Key()` renders a canonical, comparable identity
(`crd-field:cert-manager/cert-manager.io/Certificate#spec.privateKey.rotationPolicy`).
`Product` is the catalog id that owns the subject (the fact's product, except for
`product-relationship`, where `Name` is the related product).

| Family | Required | Optional | Example |
|---|---|---|---|
| `crd-field` | Group, Kind, Path | Version | cert-manager.io / Certificate / `spec.privateKey.rotationPolicy` |
| `helm-value` | Path | Name (chart) | `webhook.timeoutSeconds` |
| `gvk` | Version, Kind | Group (`""` = core) | `networking.k8s.io/v1beta1 Ingress` |
| `feature-gate` | Name | Component | `ServerSideApply` (controller) |
| `image` | Name (repository) | | `quay.io/jetstack/cert-manager-controller` |
| `config-key` | Path | Component (file/binary) | `controller.config: kubernetesAPIQPS` |
| `rbac-permission` | Name (resource) | Group, Component | `certificaterequests` in `cert-manager.io` |
| `cli-flag` | Name | Component | `--enable-certificate-owner-ref` |
| `env-var` | Name | Component | `SSL_CERT_DIR` |
| `compatibility-boundary` | Name (platform) | | `kubernetes` |
| `api-endpoint` | Name (method + path) | Component | `GET /api/v1/applications` |
| `protocol-behavior` | Name | Component | `acme-http01-solver` |
| `terraform-attribute` | Kind (resource type), Path | | `aws_s3_bucket` / `acl` |
| `product-relationship` | Name (related product id) | | `ingress-nginx` |
| `migration` | Name (step identifier) | Component | `cmctl upgrade migrate-api-version` |

Families are data. Adding a family is an enum value + a row in the required-field
table in `semantic.go`; no evaluator changes unless a new **predicate** is needed
(§1.3) — predicates, not families, are what the engine executes.

### 1.2 Change — `ChangeSpec{Type, Before, After, RenamedTo}`

| Type | Meaning | Before / After |
|---|---|---|
| `added` | subject now exists | – / value (optional) |
| `removed` | subject no longer exists / no longer honoured | value (optional) / – |
| `renamed` | moved to `RenamedTo` (a Subject) | – |
| `default-changed` | default value changed | **both** required |
| `value-changed` | a non-default fixed value changed (image tag, pinned version) | both required |
| `behavior-changed` | same configuration, different runtime semantics | optional prose-free tokens |
| `deprecated` | still works, scheduled for removal | – |
| `now-required` | previously optional, now mandatory | – / value (optional) |
| `validation-tightened` | values previously accepted are now rejected | optional |
| `requirement-changed` | a version/compatibility requirement moved (`compatibility-boundary`, `product-relationship`) | ranges (e.g. `>=1.29`), After required |
| `migration-required` | an explicit operator step must run (`migration` family) | – |

`Before`/`After` are strings: a JSON-encoded scalar for values (`"\"Never\""`,
`"30"`), a semver constraint for ranges. Pointer-typed so "unset" ≠ `""`.

### 1.3 Applicability — the condition language

`Applicability{Exposure Condition, Overlap *Condition}`:

- **Exposure**: the environment state under which the consequence happens.
- **Overlap** (optional): the environment touches the subject but is shielded
  (e.g. it pins the old value explicitly) → INFORMATIONAL, the "applies to you,
  you appear safe" class.

A `Condition` is a small tree: combinators `all` / `any` over `Of`, and leaf
predicates. Evaluation is **three-valued** (Kleene): each leaf yields `true`
(exposed — always with environment evidence), `false` (checked against a
**supplied, healthy** dimension and clear — always with an `ImpactCheck`), or
`unknown` (with an `UnknownReason` and a `neededToDetermine` string). `all`: any
false → false; all true → true; else unknown. `any`: any true → true; all false →
false; else unknown. There is deliberately no `not`: negation lives in the leaf
states (`unset` vs `set`), so "absence of evidence" can never be negated into
evidence.

| Op | Fields | Evaluated against | `true` evidence | `false` requires |
|---|---|---|---|---|
| `values-key` | Path, State, Values | `Environment.ValuesKeys` | the values line (or, for `unset`, every supplied values file's digest record) | values supplied, health ok |
| `resource-field` | Group, Kind, Version?, Path, State, Values | per-resource field inventory of manifests (see note) | each matching resource document (`unset`: the resource's apiVersion/kind line) | manifests supplied, health ok |
| `gvk-in-use` | Group, Version?, Kind? | `Environment.GVKUsage` (manifest use, not CRD presence) | using documents | manifests supplied, ok |
| `image-in-use` | Name (repository), State? + Range (tag constraint) | `Environment.Images` | image reference lines | an image-bearing input supplied |
| `cli-flag` | Name, State, Values, Component? | container `args`/`command` in manifests | the arg line | manifests supplied, ok |
| `env-var` | Name, State, Values, Component? | container `env` in manifests | the env line | manifests supplied, ok |
| `feature-gate` | Name, State (`enabled`/`disabled`/`unset`), Path? (values key carrying gates) | `--feature-gates` args in manifests + the values key at Path | the arg/values line | manifests (and values when Path set) supplied, ok |
| `product-version` | Name (product id), State, Range | product inventory (`envinv` lane) | the inventory entry | inventory dimension supplied, ok |
| `cluster-version` | Name (platform), State, Range | `Environment.Kubernetes` (platform `kubernetes`) | the `--kubernetes` input | the platform's version supplied |
| `undecidable` | Reason, Needed | — | never | never — always `unknown` with Reason |

States: `unset`, `set`, `equals` (any of `Values`), `not-equals` (set and none of
`Values`); for `feature-gate` `enabled` / `disabled` / `unset`; for the versioned
leaves (`product-version`, `cluster-version`, `image-in-use` with a Range)
`in-range` / `out-of-range` (present and its version inside / outside `Range`).
No range complement is ever computed: "requires ≥ X" is `out-of-range` of `>=X`. `Values` are
JSON-encoded like `Before`/`After`. `Range` uses the repository's existing range
semantics (Masterminds semver constraints, `upgrade.versionRangeOf` conventions).

Rules the evaluator (applicability lane) must keep:

1. **Absence is not knowledge.** A leaf is `false` only when its deciding
   dimension was supplied **and** `Environment.Health` is `ok`. `partial` health
   turns a would-be `false` into `unknown` (`environment-visibility-gap`); a `true`
   with evidence stands under partial health.
2. **Scope.** `resource-field` is quantified "some resource of the GVK": `true`
   if ≥1 resource matches (the finding lists all matching resources — "11
   Certificates with rotationPolicy unset"), `false` if manifests are supplied and
   none match (including zero resources of the kind — the same convention as
   `impact:crd-field-unset`).
3. **No product-specific code.** Every predicate is generic over its fields;
   products appear only as data in facts.

**Environment assumptions** (to be provided additively in `internal/env`,
coordinated through status files):

- `resource-field`, `cli-flag`, `env-var`, `feature-gate` need a per-resource view
  — today `GVKUsage.FieldPaths` is aggregated per GVK and leaf values are not kept.
  The `applicability` lane adds `func (e *Environment) Resources(group, kind string)
  []ResourceInstance` (`{Name, Namespace, Version, Fields map[path]valueJSON,
  Containers []{Name, Image, Args, Env}, Evidence}`) built from the documents the
  loader already decodes.
- `product-version` needs the `envinv` lane's accessor, assumed as
  `func (e *Environment) Product(id string) (ProductInstance, bool)` with a
  `products` dimension in `Health` (`absent|ok|partial`). Until it lands the
  evaluator returns `unknown` / `cross-product-context-gap` for every
  `product-version` leaf. `domain.DimensionProducts` (`"products"`) is added now
  for the evaluation records.

#### Canonical applicability (deterministically verifiable)

For some (family, change type) pairs the exposure condition is fixed by the
semantics of the subject, not by judgement. A validator may confirm the
**applicability** aspect when the asserted condition equals the canonical one:

| Subject family · change | Canonical Exposure | Canonical Overlap |
|---|---|---|
| `helm-value` · `default-changed` | `values-key{Path, unset}` | `values-key{Path, set}` |
| `helm-value` · `removed` / `renamed` | `values-key{Path, set}` | – |
| `crd-field` · `default-changed` | `resource-field{G,K,Path, unset}` | `resource-field{G,K,Path, set}` |
| `crd-field` · `removed` / `validation-tightened` | `resource-field{G,K,Path, set}` | – |
| `gvk` · `removed` | `gvk-in-use{G,V,K}` | – |
| `image` · `removed` / `value-changed` | `image-in-use{Name}` | – |
| `cli-flag` / `env-var` · `removed` / `renamed` | `cli-flag`/`env-var{Name, set}` | – |
| `product-relationship` · `requirement-changed` | `product-version{Name, out-of-range, Range: After}` | – |
| `compatibility-boundary` · `requirement-changed` | `cluster-version{Name, out-of-range, Range: After}` | – |

This table is data in the validate lane (`canonical:<family>/<change>` rule
ids). It is why most facts need a human only for the **consequence** aspect.

### 1.4 Consequence — `Consequence{Kind, Statement, Remediation, Severity}`

| Kind | Action-eligible | Class when exposed (trusted fact) | Typical severity |
|---|---|---|---|
| `upgrade-blocked` | yes | action-required | critical |
| `resource-rejected` | yes | action-required | critical/high |
| `setting-ignored` | yes | action-required | high |
| `behavior-change` | yes | action-required | high |
| `permission-change` | yes | action-required | high |
| `migration-required` | yes | action-required | high |
| `deprecation` | no | review-required | medium |
| `none` | no | informational | low |

`Statement` must answer ACTION_CLASSIFICATION.md's question "what exactly will
fail if I do nothing?" — it is required for every action-eligible kind.

### 1.5 UNKNOWN reasons (G18)

`domain.UnknownReason` on `ImpactFinding.unknownReason` (unknown-only):

| Reason | Meaning | Routes to |
|---|---|---|
| `release-knowledge-gap` | no trusted fact says what the change means (prose, unmapped diff rule, proxy-only knowledge) | candidate generation / review |
| `environment-visibility-gap` | the deciding dimension was not supplied or is partial | the user (supply `--values`, …) |
| `cross-product-context-gap` | depends on other products' presence/versions and the inventory is absent | the user (`--inventory`) |
| `runtime-behavior-gap` | depends on runtime state no static input carries (live traffic, stored objects) | stays UNKNOWN; documented |
| `evidence-gap` | upstream evidence is insufficient or unstructured (constraint not machine-readable; reviewer marked insufficient) | upstream research |
| `semantic-ambiguity` | the change's identity cannot be pinned (unparseable CRD identity; reviewers disagree) | review |

Assignment by the existing join (applicability lane implements; until then the
field is optional and `Validate()` checks only its value and class):

| Existing rule / situation | Reason |
|---|---|
| `impact:not-joined`, note-derived change | `release-knowledge-gap` |
| `impact:not-joined`, computed rule without join rule; subject-less computed change | `release-knowledge-gap` |
| `impact:not-joined`, CRD change whose API identity cannot be parsed | `semantic-ambiguity` |
| `impact:insufficient-visibility`, dimension not supplied / platform unsuppliable | `environment-visibility-gap` |
| `impact:insufficient-visibility`, constraint not machine-readable | `evidence-gap` |
| `impact:knowledge-undecided` | the reason of the first `unknown` leaf (precedence: cross-product → environment-visibility → runtime-behavior → evidence → semantic-ambiguity), or `release-knowledge-gap` when only proxy-level knowledge exists |

Once the join assigns reasons everywhere, the applicability lane makes the field
**mandatory** on unknown findings in `Validate()` and regenerates the goldens in
the same commit (stating why they changed).

## 2. Entities, lifecycle, identity

All IDs are content-derived (`domain.ShortHash`), prefix-typed, and stable
across re-runs. Every entity has a `Validate()` in `semantic.go`.

| Entity | ID | Derived from | Mutable? |
|---|---|---|---|
| `SemanticCandidate` | `sc-` | product, anchor (change id, release) | no |
| `SemanticProposal` | `sp-` | candidate, task, provider, model, modelVersion, promptDigest | no |
| `ValidationResult` | `val-` | candidate, validator producer, checked assertion digest | no |
| `ReviewItem` | `ri-` | candidate, question type, asked assertion digest | status only |
| `ReviewDecision` | `rd-` | review item, reviewer, decidedAt | no (append-only) |
| `VerifiedFact` | `vf-` | product, anchor change id, release, assertion digest | status only (active → retracted/superseded) |

### 2.1 SemanticCandidate (from an UpgradeEdge change)

`{ID, Product, Release, Anchor, Category, Title, Text, Evidence []Evidence,
Hints, Producer, CreatedAt}`. One per **change**, environment-independent
(generated from the edge, not from a report). Candidates come from changes that
the join leaves `unknown` by construction: note-derived, non-routine changes that
are not `impact:security-fix`, and computed changes of diff rules without a join
rule. `Evidence` is a **snapshot of the full records** (not only ids), so the
knowledge store is self-contained and auditable without the edge. `Hints` are
deterministic pre-extractions (backticked tokens, key-path-shaped strings,
GVK-shaped strings, `--flags`) — inputs to prompts, never conclusions.

**ChangeAnchor** — how knowledge re-attaches to a change in a future run (G12):

```go
type ChangeAnchor struct {
    ChangeID     string   // the edge's content-derived change id ("chg-…")
    Release      string   // introducing release as the change states it ("" for endpoint diffs)
    EvidenceKeys []string // digest-free keys of the evidence: ShortHash(kind, uri, locator, excerpt)
}
```

`anchor.Matches(c Change, ev func(EvidenceID) (Evidence, bool)) bool` is true when
(release-compatible: either side empty, or equal) and (`c.ID == ChangeID` or any of
`c`'s evidence has an `EvidenceKey` in `EvidenceKeys`). Change ids are already
content-derived (`"note"` + normalized text; rule + artifact + subjects for diffs),
so the same change in any edge that traverses the release — any environment, any
re-ingestion — attaches the same fact. Evidence keys drop `ContentDigest`, so an
upstream document edited elsewhere keeps the anchor; an edit of the item itself
changes the change id *and* the excerpt, and the fact correctly stops attaching
(a re-review is then a new candidate whose "previous related decisions" panel
shows the old fact).

### 2.2 SemanticProposal (one per model × task; never collapsed — G2, G3)

`{ID, CandidateID, Task, Provider, Assertion, Undetermined []Aspect,
UndeterminedReason, SuggestedClass, Citations, Provenance}`.

- `Task`: `semantic-mapping` (subject+change), `applicability`, `consequence`,
  `relationship`, `duplicate`, `full` (all aspects in one answer).
- `Provider`: who served the model (`anthropic`, `zai`, `typesafe`, …) — required;
  `Provenance` is the existing AI provenance (method `ai`, model, modelVersion,
  promptVersion, promptDigest, inputEvidence, generatedAt, confidence), validated
  by the existing `Provenance.Validate()`. Confidence is capped at `medium`.
- `Assertion` may be partial; an aspect the model would not commit to goes in
  `Undetermined` (abstention is a first-class answer, scored separately).
- `SuggestedClass` ∈ {`""`, `review-required`, `informational`, `unknown`} —
  never `action-required` or `not-affected` (enforced).
- `Citations ⊆ Provenance.InputEvidence ⊆ candidate evidence` (+ the snapshot
  evidence the prompt showed).

### 2.3 ValidationResult (deterministic validators — G5)

`{ID, CandidateID, ProposalID?, Validator, Assertion, Checks []AspectCheck,
Evidence []Evidence, CheckedAt}`; `AspectCheck{Aspect, Outcome, Rule, Detail}`,
`Outcome` ∈ `confirmed | refuted | inconclusive`. A validator confirms only what
an artifact proves:

| Validator | Inputs (already ingested) | Can confirm | Can refute |
|---|---|---|---|
| `crd-schema` | `CRDSnapshot` of from/to (schemas, versions, defaults, required) | subject (field/GVK exists), change (removed/added/default before→after/now-required/version served) | wrong path, wrong default, wrong version |
| `helm-values` | `ValuesSnapshot` from/to, `values.schema.json` when published | subject, change (default before→after, removed, added, type) | same |
| `compatibility` | `CompatibilityConstraint` rows from/to, chart `kubeVersion` | subject, change for `compatibility-boundary` / `product-relationship` | wrong range |
| `image` | `image-refs` snapshots, artifact instances | subject, change (tag moved, repo moved/removed) | same |
| `canonical-applicability` | the asserted subject/change/condition | applicability, when it equals the canonical condition (§1.3) and subject+change are confirmed | – |

Consequence is never deterministically confirmed (it is a judgement about
impact); `none` for `added` subjects is the one exception (`canonical` rule).
Validators fill what snapshots lack only by extending snapshots additively (e.g.
CRD field defaults) — never by fetching new sources at validation time.

### 2.4 ReviewItem (G6, G8, G10)

`{ID, CandidateID, Product, Release, QuestionType, Question, Aspects, Proposed,
Proposals []ID, Validations []ID, Routing, Context, Status, CreatedAt}`.

- `QuestionType` (G10) and the aspects its decision verifies:

| Question type | Verifies aspects | Concrete question (example) |
|---|---|---|
| `semantic-mapping` | subject, change | "Does 'default rotationPolicy is now Always' mean `Certificate.spec.privateKey.rotationPolicy` default Never → Always?" |
| `applicability` | applicability | "Does a Certificate with rotationPolicy **unset** become affected by this default change?" |
| `consequence` | consequence | "If an exposed Certificate does nothing, what happens? (behavior-change: keys rotate on every renewal)" |
| `classification` | consequence | "Is this action-eligible (behavior-change) or a deprecation/no-op?" |
| `relationship` | subject, change, applicability | "Does v1.18 require ingress-nginx ≥ 1.12.6 for HTTP01?" |
| `duplicate` | – (links to a fact) | "Is this the same change as vf-…?" |
| `evidence-sufficiency` | – (gate) | "Is the cited upstream text enough to decide?" |

- `Proposed`: the assertion the reviewer is asked to accept/correct (the
  consensus where models agree on an aspect; the highest-ranked proposal otherwise
  — all proposals stay visible).
- `Context`: optional environment illustration shown to the reviewer
  (`{Label, Digest}`); **review items are environment-free by default**; when
  context is used it is recorded so the eval can separate transfer (§7).
- `Status`: `pending | needs-evidence | deferred | decided | superseded`.

### 2.5 ReviewDecision (G8, G9, G11, G13)

`{ID, ReviewItemID, Action, Labels, Original, Corrected, Reviewer, ReviewerKind,
ProxyProvenance, Reason, StartedAt, DecidedAt, ResultingFact, DuplicateOf}`.

- `Action`: `accept | reject | correct | need-more-evidence | defer`.
- `Labels` (G13): `accepted, rejected, corrected, insufficient-evidence,
  wrong-subject, wrong-change-type, wrong-applicability, wrong-consequence,
  wrong-classification, duplicate`. Consistency enforced: accept ⇒ `accepted`;
  correct ⇒ `corrected` + ≥1 `wrong-*`; reject ⇒ `rejected` or `duplicate`;
  need-more-evidence ⇒ `insufficient-evidence`; defer ⇒ no outcome label.
- **Correction is first-class (G9)**: `Original` is always the assertion shown;
  `Corrected` is set iff `Action == correct` and must differ from `Original`.
- `ReviewerKind` `human | proxy`. A proxy decision **must** carry complete AI
  `ProxyProvenance` and a `Reviewer` naming the model; a human decision must not
  carry AI provenance. There is no field through which a proxy can claim to be
  human (and the fact basis check re-verifies it, §2.6).
- Duration = `DecidedAt − StartedAt` (G24); both required for accept/correct/reject.
- A rejected decision is the retained negative example (G11); no fact results.

### 2.6 VerifiedFact (G11, G12)

`{ID, Product, Release, Anchor, CandidateID, Assertion, Verification
[]AspectVerification, Evidence []Evidence, Status, Supersedes, CreatedAt}`;
`AspectVerification{Aspect, Level, Basis []string}`; `Level` ∈ `deterministic |
human | proxy`.

Invariants (`Validate()`):

- The assertion is **complete** (all four aspects valid).
- Every one of the four aspects has exactly one verification entry with ≥1 basis
  record id; `deterministic` ⇒ basis ids are `val-…`; `human`/`proxy` ⇒ `rd-…`.
- Evidence is **release-level**: upstream kinds only — `local-file` and `input`
  evidence (environment) are rejected. A fact never names an environment.
- `fact.Level()` = the weakest aspect level (deterministic ≡ human > proxy).

Cross-record check `ValidateFactBasis(fact, validations, decisions)`: every basis
id resolves; a `val-` basis has a `confirmed` check for that aspect on an
assertion whose aspect digest equals the fact's; an `rd-` basis is an
accept/correct decision whose `ReviewerKind` **equals** the claimed level, whose
item verifies that aspect, and whose final assertion (Corrected, else Original)
has the same aspect digest. This is where "proxy can't masquerade as human" is
proved from records rather than trusted from a field.

## 3. Lifecycle

```text
candidate ─► proposals (N models × task) ─► validations ─► route ─┬─► all aspects deterministic ─► fact (level deterministic)
                                                                  └─► review item(s) ─► decision ─┬─ accept/correct ─► fact
                                                                                                   ├─ reject ─► negative example
                                                                                                   ├─ need-more-evidence ─► item status needs-evidence
                                                                                                   └─ defer ─► item status deferred
```

A fact is minted when every aspect has a verification at some level. Aspects
verified by a validator need no human; the review item asks only for the rest
(typically consequence, often applicability). Retraction: a later decision
(question on an existing fact) sets the fact `retracted`; a corrected re-review
mints a new fact that `Supersedes` the old one. Only `active` facts are used.

## 4. Trust ladder → classification (G20, G21)

Inputs per (fact F, environment E): `L` = F.Level(), the consequence kind `K`,
`X` = Exposure evaluated on E, `O` = Overlap evaluated on E (false if absent).
"Trusted" = `deterministic` or `human`.

| X | O | L trusted | L proxy |
|---|---|---|---|
| true | – | `K` action-eligible ⇒ **action-required** (confidence high); `deprecation` ⇒ review-required; `none` ⇒ informational | min(class, **review-required**), confidence medium, flagged `verification: proxy` |
| false | true | informational | informational |
| false | false/unknown | **not-affected** with checks | **unknown** (`release-knowledge-gap`: a proxy may never clear) |
| unknown | – | unknown (leaf reason) | unknown (leaf reason) |

Hard rules, each enforced by `ImpactReport.Validate()` on findings that carry a
knowledge reference (`ImpactFinding.Knowledge`, rule prefix `impact:knowledge-`):

1. **ACTION REQUIRED** needs: `Knowledge.Verification` trusted (so subject, change,
   applicability **and** consequence are each deterministic- or human-verified);
   `X` true with environment matches + environment evidence (both chains — the
   existing affected-class rule); confidence `high` (existing demotion rule).
2. **not-affected** from knowledge needs a trusted verification and checks.
3. A **proxy**-verified knowledge finding is review-required, informational or
   unknown — never action-required, never not-affected.
4. **Model proposals alone never change a classification.** They are not inputs
   to the engine at all: `impact.Build` receives facts (`[]VerifiedFact`), never
   proposals. The existing `SuggestedClassification` path (impactenrich) is
   unchanged and remains `unknown`-only.
5. `Knowledge != nil` ⇔ rule starts with `impact:knowledge-`; `unknownReason` is
   unknown-only.
6. Supersession: a change with a knowledge finding carries **no other unknown
   finding** (the knowledge finding replaces the `not-joined` /
   `insufficient-visibility` record — no double counting).

**Composition with the existing join** (applicability lane, `internal/impact`):

- Facts are matched to changes via `ChangeAnchor.Matches`. Several facts may
  attach to one change (e.g. one per CRD kind it touches); each yields a finding.
- Per change, deterministic findings are **kept as they are**. A knowledge
  finding is emitted when its class is not `unknown`, or when the change's
  existing verdict is `unknown` (then the knowledge `unknown` replaces it with a
  sharper reason). It **replaces** the change's `unknown` records and is added
  next to any deterministic affected/not-affected findings only if it is
  **stronger** (action > review > informational > not-affected). It never removes
  or downgrades a deterministic finding (a knowledge not-affected never hides a
  computed action-required).
- Rules: `impact:knowledge-exposed` (affected per the table),
  `impact:knowledge-overlap` (informational via Overlap),
  `impact:knowledge-clear` (not-affected), `impact:knowledge-undecided` (unknown).
  Provenance `method: computed`, producer `impact.knowledge@v1`: the *evaluation*
  is deterministic; the fact's own verification is carried in `Knowledge`.
- Upstream chain = the change's evidence ∪ the fact's evidence (ids must resolve in
  the report's `evidence`; the engine copies fact evidence records into the pool).
- Without facts, `Build` is byte-identical to today (goldens unchanged).

## 5. Cross-product knowledge (G19)

No special construct: a cross-product requirement is a fact whose subject is
`product-relationship{Name: <other product>}` (or a `compatibility-boundary`),
change `requirement-changed` with `After` the required range, and whose Exposure
is `product-version{Name: <other product>, State: out-of-range, Range: <required>}`
— e.g. "v1.18.0 needs ingress-nginx ≥ 1.12.6 for HTTP01" ⇒ Exposure
`all[ product-version{ingress-nginx, out-of-range, ">=1.12.6"}, resource-field{acme.cert-manager.io, Issuer, spec.acme.solvers.http01.ingress, set} ]`.
Evaluated against the `envinv` lane's product inventory. Product absent from a
**supplied** inventory ⇒ `false` (the requirement is irrelevant); inventory not
supplied ⇒ `unknown / cross-product-context-gap`. "Argo CD manages cert-manager
resources" is likewise `product-version{argo-cd, in-range, "*"}` in an Exposure.

## 6. Routing (G16 — hypotheses, measured, not assumed)

Signals computed per aspect from proposals and validations; the route is recorded
on the item (`Routing{Route, Priority, Signals}`) so outcomes can later test the
hypothesis.

| Signals | Route | Priority |
|---|---|---|
| validation `confirmed` for every aspect | `auto-verify` (no review item; fact at `deterministic`) | – |
| models agree on the open aspects + validation confirmed the rest | `review` | low |
| models agree, no validation possible | `review` | normal |
| models disagree on any aspect, or a validation `refuted` a proposal | `review` | normal (high if `high-impact`) |
| every model abstained / undetermined | `missing-evidence` | low |
| any proposal's consequence is action-eligible (`high-impact`) | `review` | high |

"Agree" = equal aspect digests (§2) across ≥2 proposals from distinct models.
Single-model proposals carry the `single-model` signal and never count as
agreement.

## 7. Measuring the loop honestly

**Per verification level.** `ri eval -knowledge <dir> -min-verification <level>`
runs the same dataset three ways and reports each separately (G21, FLEET):
`none` (today's baseline), `deterministic` (facts whose every aspect is
deterministic), `human` (deterministic ∪ human — **the gate number**), `proxy`
(all active facts — reported, labelled proxy, never presented as the gate).
Proxy facts can only reach review-required, but review-required is an affected
class and so counts as a link hit — that is exactly why the proxy number must
never be the headline.

**Blind authoring.** Fact authors and reviewers never see `eval/cases/*/case.yaml`
expectations. Prompts are built from the edge only. Review items are
environment-free by default. A knowledge-lane test fails if any file under
`knowledge/` mentions `eval/cases` or an expected-item id pattern in reasons.

**Transfer (G12).** Each review item records its `Context` (environment label +
digest, or none). The eval reports, per level, `applicabilityAccuracy` overall and
on the **transfer subset**: links whose deciding facts were never reviewed with
that case's environment as context. A fact that only works where it was reviewed
is visible as a gap between the two numbers.

**Metrics each entity must support** (computed by the knowledge lane; G4, G15, G24):

| Metric | From |
|---|---|
| agreement rate, pairwise agreement per aspect and per task | proposals (aspect digests) |
| per model: accepted as-is / after correction / rejected / insufficient | decisions ↔ proposals (aspect digest of the proposal vs the decision's Original/Corrected) |
| per model: FP / FN per aspect vs final facts | proposals vs facts |
| review volume per release / product / question type | review items |
| median and p90 time to decision | decisions (`DecidedAt − StartedAt`) |
| auto-validation rate | facts with all aspects deterministic / all facts |
| acceptance / correction / rejection rate | decisions |
| repeat-pattern rate | decisions whose (family, change type, consequence kind) matches an earlier fact |
| grounding quality per model | proposals: citations ⊆ input, citation reuse by the accepted fact |
| UNKNOWN → verified → class transitions | eval runs per level (G17) |

## 8. Storage

**Committed JSON files under a top-level `knowledge/` directory**, one file per
entity, wrapped in a self-describing envelope (`domain.KnowledgeRecord`,
`schemaVersion: ri.dev/knowledge/v1alpha1`, schema
`schemas/knowledge-record.schema.json`):

```text
knowledge/<product>/<release>/candidates/sc-….json
                              proposals/sp-….json
                              validations/val-….json
                              reviews/ri-….json
                              decisions/rd-….json
                              facts/vf-….json
```

`<release>` is the anchor's release (`_endpoint` for endpoint diffs). Records
other than candidates are placed by resolving their candidate (put the candidate
first). Why files, not SQLite: no new dependency; every decision is a reviewable
diff in a PR; git history is the audit log (who decided what, when); content IDs
make writes idempotent and merges conflict-free (two lanes writing different
entities never touch the same file); the volume (hundreds to low thousands of
records per product) loads into memory in milliseconds. Decisions are
append-only; review items and facts change only their `status` field. If volume
ever outgrows this, the `knowledge.Store` port admits a database without touching
any consumer.

The dashboard (`ri review serve`) is stdlib only: `net/http`, `html/template`,
`embed`; it reads and writes through the same port.

## 9. Lane interface map (wave 1)

Ports are in `internal/knowledge/api.go` (interfaces + their data types only).
Every lane depends on `internal/domain` and may import `internal/knowledge` for
the ports. No lane edits another lane's package.

### `semantic` — candidates + multi-model proposals (G1–G3)

Package `internal/semantic` (new). Files: `candidates.go`, `prompt.go`,
`proposer.go`, `answer.go` (typed answer schema), tests, `docs/SEMANTIC.md`.

```go
func Candidates(edge *domain.UpgradeEdge, now time.Time) []domain.SemanticCandidate
// LLMProposer implements knowledge.Proposer over llm.Client (Anthropic Messages API,
// Anthropic-compatible gateways such as Z.AI for GLM, the file exchange, the cache).
func NewLLMProposer(client llm.Client, provider, model string, opts ProposerOptions) *LLMProposer
func AnswerSchema(task domain.ProposalTask) json.RawMessage // enumerated, typed; every enum from domain
func ProposeAll(ctx context.Context, cands []domain.SemanticCandidate, ps []knowledge.Proposer,
    tasks []domain.ProposalTask) ([]domain.SemanticProposal, []knowledge.ProposalFailure)
```

Requirements: provider-agnostic (`knowledge.Proposer` is the only seam; a typed
"System One" model is another implementation returning the same typed answer);
the answer schema is enumerated (families, change types, ops, states, consequence
kinds, `undetermined`) — free text only in `Statement`/`Detail`; values never
sent; prompts never include eval expectations; each proposal validated with
`SemanticProposal.Validate()` before storing; failures recorded, never dropped.

### `validate` — deterministic validators (G5)

Package `internal/semvalidate` (new). Files: `crd.go`, `values.go`, `compat.go`,
`images.go`, `canonical.go`, tests.

```go
func Validators() []knowledge.Validator // crd-schema, helm-values, compatibility, image, canonical-applicability
// each: Name() string; Validate(ctx, knowledge.ValidationInput) ([]domain.ValidationResult, error)
```

Inputs are the from/to `domain.Release` snapshots (via `internal/store`) — no new
fetches. Additive snapshot fields (e.g. CRD field defaults/required) are allowed
in `internal/normalize` + `domain/release.go`; list them in the status file.

### `knowledge` — store, queue, decisions → facts, dataset, metrics (G6, G11–G16, G24)

Package `internal/knowledge` (implements `api.go`). Files: `filestore.go`,
`route.go`, `decide.go`, `dataset.go`, `metrics.go`, `pipeline.go`, tests; CLI
`ri knowledge candidates|propose|validate|route|export|metrics` in
`cmd/ri/knowledge.go`.

```go
func NewFileStore(dir string) knowledge.Store
func NewQueue(s knowledge.Store, now func() time.Time) knowledge.Queue
func Route(c domain.SemanticCandidate, ps []domain.SemanticProposal, vs []domain.ValidationResult) ([]domain.ReviewItem, *domain.VerifiedFact)
func FactFromDecision(item domain.ReviewItem, d domain.ReviewDecision, prior knowledge.Snapshot) (*domain.VerifiedFact, error)
func ExportDataset(s knowledge.Snapshot, w io.Writer) error // JSONL of domain.FeedbackExample
func ComputeMetrics(s knowledge.Snapshot) knowledge.LoopMetrics
```

Every minted fact passes `VerifiedFact.Validate()` **and**
`domain.ValidateFactBasis`. The integrity test (§7) lives here.

### `applicability` — fact × environment in the engine (G17, G18, G20, G21)

Package `internal/impact` (owns `knowledge.go`, `condition.go`, their tests,
`render.go` additions) + `internal/env` per-resource accessor (`resources.go`)
+ `internal/eval` knowledge wiring + `cmd/ri` flags.

```go
// impact.Input gains (additive): Facts []domain.VerifiedFact; MinVerification domain.VerificationLevel
func EvaluateCondition(c domain.Condition, e *env.Environment) ConditionResult // {Value: True|False|Unknown, Matches, Checks, Reason, Needed}
func ClassifyKnowledge(f domain.VerifiedFact, exposure, overlap ConditionResult) (domain.ImpactClass, domain.Confidence, domain.UnknownReason)
func (e *env.Environment) Resources(group, kind string) []env.ResourceInstance
```

Also assigns `unknownReason` across the existing join (§1.5) and then makes it
mandatory; adds `-knowledge`/`-min-verification` to `ri impact` and `ri eval`,
and the per-level + transfer reporting (§7). `go test ./internal/eval` must show
no regression with `-knowledge` unset.

### `dashboard` — `ri review serve` (G7, G8)

Package `internal/reviewui` (new; `server.go`, `templates/*.html` via `embed`,
`static/`), `cmd/ri/review.go`.

```go
func NewServer(q knowledge.Queue, s knowledge.Store, opts Options) http.Handler
// routes: GET / (inbox counts + filters), GET /items/{id} (knowledge.ReviewContext),
//         POST /items/{id}/decision (accept|reject|correct|need-more-evidence|defer)
```

Inbox counts and filters exactly as G7; item page exactly the G8 list (all from
`knowledge.ReviewContext`). Records `StartedAt` when the item page is served and
`DecidedAt` on submit. Reviewer identity is a required free-text field +
`ReviewerKind` fixed to `human` in the UI (proxy decisions come only from the
CLI with provenance). No auth (non-goal). stdlib only.

### `groundtruth` — dataset expansion (G22)

`eval/cases/**` new cases and additive expectation fields; `eval/FORMAT.md`;
`internal/eval/case.go` (additive types + validation only). New optional block per
expected item, authored blind from upstream sources:

```yaml
semantics:                         # optional, per expected item
  subject: {family: crd-field, group: cert-manager.io, kind: Certificate, path: spec.privateKey.rotationPolicy}
  change: {type: default-changed, before: '"Never"', after: '"Always"'}
  consequence: behavior-change
```

and per `expectedImpact` link:

```yaml
    exposure:                      # the condition, in the domain condition language
      op: resource-field
      group: cert-manager.io
      kind: Certificate
      path: spec.privateKey.rotationPolicy
      state: unset
    environmentEvidence: manifests/certificates.yaml   # where in the fixture it is decided
```

Field names mirror the domain JSON names, so `internal/eval` decodes them
directly into `domain.Subject` / `domain.ChangeSpec` / `domain.Condition`. These
labels score the *semantic* stage (subject/change/applicability accuracy per
model, G15); they are never read by knowledge authoring.

## 10. The required demonstration, mapped

| Step (MISSION) | Entity / mechanism |
|---|---|
| 1 discover the prose change | `chg-…` in the edge → `SemanticCandidate` (`semantic.Candidates`) |
| 2 GLM: plausibly `Certificate.rotationPolicy` | `SemanticProposal` (provider `zai`, model `glm-5.3-flash`, task `full`) |
| 3 Sonnet: undetermined | `SemanticProposal` with `Undetermined: [subject, change, …]` — stored, not collapsed |
| 4 artifact inspection: field exists, default changed | `ValidationResult` `crd-schema` (subject, change `confirmed`); `canonical-applicability` confirms Exposure `resource-field{…, unset}` |
| 5 applicability not fully provable | consequence open → route `review` (models disagree ⇒ normal; action-eligible ⇒ high) |
| 6 "Does a Certificate with rotationPolicy unset become affected …?" | `ReviewItem` (`applicability`/`consequence`) in the dashboard |
| 7 engineer: ACCEPT | `ReviewDecision{accept, human, labels: [accepted]}` |
| 8 rule stored | `VerifiedFact` (subject+change+applicability deterministic, consequence human) |
| 9 11 Certificates with rotationPolicy unset | `resource-field` leaf `true`, 11 matches with evidence |
| 10 ACTION REQUIRED | trusted fact + exposure true + `behavior-change` ⇒ `impact:knowledge-exposed` · action-required · high |
| 11 reuse without re-review | `ChangeAnchor.Matches` in any edge traversing v1.18.0, any environment |

> Machines propose. Evidence constrains. Engineers resolve ambiguity. Verified knowledge compounds.
