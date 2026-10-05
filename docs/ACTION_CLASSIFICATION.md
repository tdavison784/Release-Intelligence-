# Action classification — the contract

This document is **normative** for `ri impact` (package `internal/impact`,
types in `internal/domain/impact.go`). The implementation enforces it:
`domain.ImpactReport.Validate()` rejects a report that violates it, and the
schema (`schemas/impact-report.schema.json`) mirrors the same rules. Where
this document and older prose disagree, this document wins.

Everything below is deterministic: no LLM participates in this path (the AI
layer is a separate workstream and may never write findings — it attaches
suggestions and notes with AI provenance next to the findings; see
docs/IMPACT-ENRICHMENT.md).

## 1. The three axes

A finding answers three independent questions, and they must never be folded
into one another:

| Axis | Question | Field |
|---|---|---|
| Classification | *What should I do?* | `classification` |
| Severity | *How bad is it if it bites?* | `severity` (where determinable) |
| Confidence | *How certain is the engine?* | `provenance.confidence` |

Severity never changes a classification; confidence never changes a
classification **except** through the demotion rule of §3.

## 2. Applicability comes first (the engine's shape)

For **every** analyzed unit — every upstream change, every compatibility
constraint, every moved container-image artifact — the engine determines
applicability **before** any action class exists:

- **AFFECTED** — the unit intersects the environment with evidence. Only an
  AFFECTED unit receives an action class (`action-required`,
  `review-required`, `informational`).
- **NOT_AFFECTED** — the environment dimension(s) the unit must be checked
  against were **actually supplied**, the subject was checked against them,
  and they show it does not apply. Claimable **only** under full visibility
  for the unit's deciding dimensions (§5); a missing match alone never
  implies NOT_AFFECTED.
- **UNKNOWN** — applicability cannot safely be determined. Every UNKNOWN
  record states **what evidence was missing** in `neededToDetermine`
  (e.g. "Helm values files (--values) not supplied"). UNKNOWN must never
  silently become NOT_AFFECTED or ACTION REQUIRED.

The engine never assumes `no match = not affected`: a missing match may mean
insufficient environment visibility, and insufficient visibility is UNKNOWN.

## 3. The five action classes

Exact identifiers (JSON values of `ImpactFinding.classification`):

### `action-required` (ACTION REQUIRED)

The environment must change before/during the upgrade to avoid concrete
failure, incompatibility, or loss of intended behavior.

REQUIRES: upstream evidence + environment evidence + a deterministic or
strongly-validated relationship between them. A finding in this class must
answer **"what exactly will fail if I do nothing?"** with evidence; if that
cannot be answered, it must not be here. Low confidence is **prohibited** in
this class: any finding whose provenance confidence is below `high` is
demoted to `review-required` (enforced in the builder and by `Validate()`,
and mirrored in the JSON Schema). Ambiguity must never be promoted into
mandatory action.

### `review-required` (REVIEW REQUIRED)

Credible evidence the change intersects the environment, but applicability or
necessity cannot be deterministically proven. The finding must explain what
overlaps, why it may matter, and what to inspect. This is also the receiving
class for demoted ACTION REQUIRED findings.

### `informational` (INFORMATIONAL)

Relevant to the environment — a real, evidenced overlap — while the evidence
indicates no action is needed ("applies to you, you appear safe": a changed
default the customer explicitly overrides, a cluster version inside the
supported range). One shape applies without consulting any environment
dimension: a security remediation that ships with the target release (the
`impact:security-fix` rule) reaches every environment that upgrades, so its
applicability is universal by construction — it cites its upstream evidence
and states that no environment-specific action is required beyond upgrading.
A security item that also carries a stronger signal (breaking, or an
operator directive in the edge) keeps that stronger class and is never
downgraded to informational.

### `not-affected` (NOT AFFECTED)

Evaluated against the environment, with sufficient evidence that it does not
apply. The finding carries the **evaluation record** (`checks`): which
environment dimension was consulted, how many facts were compared, which
upstream subjects were compared against them — enough to answer "why do you
think this doesn't affect me?" without a model call. NOT AFFECTED findings
are counted in the summary and rendered only in verbose mode
(`ri impact --show-not-affected`); they carry no `matches` (nothing matched —
that is the conclusion).

### `unknown` (UNKNOWN / INSUFFICIENT EVIDENCE)

Applicability cannot safely be determined. The finding carries the
`neededToDetermine` list naming the missing evidence. It never carries
`matches` (nothing was established). UNKNOWN findings are printed in the
normal output, grouped by what is missing.

## 4. Migration from the phase-2 vocabulary

| Phase 2 (`impact-report` ≤ v1alpha1, 3 classes) | Phase 3 |
|---|---|
| `action-required` (both chains) | `action-required` — unchanged |
| `review` | `review-required` (identifier renamed, semantics unchanged) |
| `informational` (confirmed overlap, no action implied) | `informational` — unchanged |
| *(silently absent: change checked, no overlap, full visibility)* | `not-affected` |
| *(silently absent: dimension not supplied / no join rule / note-derived change)* | `unknown` |

What the old engine left silent is now stated: a checked-and-clear subject is
`not-affected`; anything the join could not look at is `unknown` with the
reason. Summary numbers therefore shift by design.

## 5. Visibility scoping (when NOT_AFFECTED is claimable)

Each join rule declares the environment dimensions that must be **supplied**
to decide applicability. If a deciding dimension is absent, the verdict is
`unknown` and the dimension appears in `neededToDetermine`:

| Upstream unit | Deciding dimensions | Note |
|---|---|---|
| `values:*` diff rules | `values` | values are a flat mapping: absence of a key is a fact, not a blind spot |
| `values:default-changed` / `values:added`, key **unset** (PO-3) | `values` + `render` | the new default reaches whoever does not pin the key, so "not set" is not "not affected". A render attributing a change to the key → `impact:values-default-applies` · review-required; both renders succeed and nothing attributable changes → `impact:values-default-no-effect` · not-affected (render check as evidence); no render → `impact:values-default-unrendered` · not-affected with a visible `render unavailable` check (the product owner's choice, docs/phase3/learning-loop/DECISIONS.md) |
| `crd:removed`, `crd:version-*` | `crds` | installed CRDs are authoritative for what the cluster runs; manifests refine affected-path confidence |
| `crd:fields-removed` | `manifests` | field paths are manifest facts |
| `images:*`, moved image artifacts | `images` | visible when any image-bearing input was supplied (`--images`, `--values`, `--manifests` all yield image facts) |
| compatibility constraints | `cluster-version` of the constraint's platform | `--kubernetes` supplies the `kubernetes` platform; an unsuppliable platform (e.g. OpenShift today) is always `unknown` |
| note-derived security remediation (cited CVE/GHSA/advisory, no stronger signal) | none — applies by construction | the fix ships with the target, so every environment that upgrades receives it: AFFECTED as `informational` (`impact:security-fix`) with the upstream chain only; no environment dimension is consulted and none is claimed |
| note-derived changes (declared/heuristic — no machine-comparable subject) | none | `unknown` by construction: the join cannot evaluate what it cannot compare |
| computed diff rules without a join rule (e.g. `crd:fields-added`) | none | `unknown` by construction: applicability of an added (possibly required) schema field is not determinable here |

Provenance for every verdict:

- **`action-required`**: both evidence chains are mandatory (upstream chain
  resolving in the report's `evidence`, environment chain resolving in
  `environmentEvidence`) — enforced since phase 2, kept.
- **`review-required` / `informational`**: both chains likewise (an AFFECTED
  finding without an environment fact is a contradiction) — the one exception
  is the `impact:security-fix` rule above, whose universal applicability needs
  no environment fact, so it carries the upstream chain only.
- **`not-affected`**: the upstream chain is kept, plus the evaluation record
  (`checks`, with the supplied-input evidence where a direct input record
  exists, e.g. the `--kubernetes` flag).
- **`unknown`**: the upstream chain is kept (the change exists and cites its
  sources), plus `neededToDetermine`; any checks that were possible with
  partial visibility are recorded too.

## 6. Severity (where determinable)

`severity` is optional and takes `critical` / `high` / `medium` / `low`:

- `critical` — the upgrade fails outright (Helm refuses the install via
  `kubeVersion`; resources of a removed CRD/API stop being served).
- `high` — concrete degradation (a removed values key stops taking effect, a
  pruned CRD field is dropped/rejected, the cluster leaves the supported
  range).
- `medium` — needs a look, outcome depends on intent (adjacent values
  sections, new live keys, deprecations, image reference changes).
- `low` — confirmed no-action overlaps (a pin that keeps winning, in-range
  cluster).

`not-affected` and `unknown` records carry no severity.

## 7. The funnel

The text renderer summarises the report as a five-line funnel that counts
**every verdict explicitly**:

```
51 upstream changes analyzed
ACTION REQUIRED:   1
REVIEW REQUIRED:   3
INFORMATIONAL:     1
NOT AFFECTED:     41
UNKNOWN:            9
```

The counts are per finding/verdict record (a change evaluated in two ways —
an exact pin and an adjacent section — contributes one finding to each
class, so the sum can exceed the analyzed change count). `NOT AFFECTED`
appears in the summary and in verbose mode only; per-finding sections print
`action-required`, `review-required`, `informational` and `unknown`.

## 8. Findings from verified knowledge (learning loop)

Findings whose rule starts with `impact:knowledge-` come from evaluating a
**verified, release-level semantic fact** against the environment
(docs/phase3/learning-loop/DESIGN.md). They follow every rule above, plus the
trust ladder, which `ImpactReport.Validate()` and the schema enforce:

- They carry `knowledge: {fact, verification}`, and only they do.
- `not-affected` requires a fact verified at `deterministic` or `human` level.
- `action-required` requires either:
  - a fact verified at `deterministic` or `human` level (**verified**), or, by
    product-owner decision PO-2 (docs/phase3/learning-loop/DECISIONS.md),
  - a `consensus` fact with `consensusAction` (**model consensus**): every
    aspect at consensus or better; an action-eligible consequence that every
    agreeing separate model call requested as action-required; no aspect
    refuted.

  Both paths still need the deterministic environment match and both evidence
  chains. A model-consensus finding carries `knowledge.verification:
  consensus`, is rendered "ACTION REQUIRED · model consensus · unaudited"
  until a human accepts its audit item (PO-6), and its fact is always sampled
  into human review. A single dissenting call on the consequence (one that
  requested review-required, informational or unknown) blocks the consensus
  path while the consequence rests on consensus (PO-5).
- A `proxy`-verified fact (an AI acting as reviewer), or a consensus fact
  without `consensusAction`, yields at most `review-required`, never at `high`
  confidence.
- A change with a knowledge finding carries no other `unknown` finding: the
  knowledge finding supersedes it.
- **Refinement (PO-4)** is the one way a fact changes a deterministic
  finding's class. Only a `deterministic`/`human` fact may do it, and only when
  its subject covers the finding's matches on the same change. The deterministic
  finding is replaced by `impact:knowledge-refined`, which carries
  `refinedFrom: {classification, rule}`, keeps both evidence chains, and stays
  action-required, review-required or informational, never not-affected. Example:
  a removed values key whose function was `superseded-upstream` moves from ACTION
  to REVIEW.
- `unknownReason` (unknown-only) names why a finding is unknown:
  `release-knowledge-gap`, `environment-visibility-gap`,
  `cross-product-context-gap`, `runtime-behavior-gap`, `evidence-gap`,
  `semantic-ambiguity`.

Model proposals never reach the engine; only facts do. A proposal's
`action-required` is a request that takes effect only through such a consensus
fact.
