# Validation dataset format

Each case is a real historical upgrade `product A → B` with ground truth
authored **blind** from upstream material: migration and upgrade docs,
release notes, GitHub issues and PRs, community reports. It is written
**without looking at Release Intelligence output**, so the dataset can measure
the tool instead of mirroring it.

```
eval/cases/<case-id>/
  case.yaml          # the ground truth (format below)
  NOTES.md           # how it was researched: sources read, judgement calls
  environment/       # optional: a realistic customer configuration
    values.yaml      #   Helm values as a user would set them
    manifests/*.yaml #   custom resources / workloads the user applies
    crds/*.yaml      #   optional: installed CustomResourceDefinitions
    images.txt       #   optional mirror list, one image ref per line
    inventory.yaml   #   optional product inventory: list of {product, version, note?};
                     #   only facts already stated in case.yaml's environment.description
                     #   (or NOTES.md), each entry citing the sentence in a comment;
                     #   the form {complete: true, products: [...]} declares that
                     #   nothing else runs there (only when the sources say so)
eval/results/        # stored results (snapshots), committed after review;
                     # `ri eval -update` rewrites them, tools never touch
                     # case.yaml
eval/adjudications/  # optional: sample-based human verdicts on output changes
                     # (<case>.yaml; input to labeledPrecision — see below)
eval/adversarial/    # offline join-fooling fixtures with contract-derived
                     # assertions (run by `go test ./internal/eval`, not eval
                     # cases; see eval/adversarial/README.md)
eval/gates.yaml      # pre-registered hard-gate thresholds (`ri eval` exits
                     # non-zero when a gate fails)
```

## case.yaml

The values below are illustrative; URLs and quotes in real cases must be
copied from the upstream documents themselves.

```yaml
id: cert-manager-1.17-1.18
product: cert-manager          # definition id in products/
from: v1.17.0                  # tags as published
to: v1.18.0
researchedAt: "2026-10-01"
sources:                       # every upstream document consulted
  - https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md
expected:                      # the upgrade work a competent operator must know about
  - id: E1
    title: Certificate private key rotationPolicy default changed from Never to Always
    kind: behaviour-change     # breaking | removal | deprecation | behaviour-change | helm-values | crd-schema | api | compatibility | security | artifact | migration-step | dependency
    importance: critical       # critical (would break/surprise prod) | important (should know/act) | minor (nice to know)
    actionRequired: true       # an operator must do something (or consciously decide)
    classification: review-required  # G9: the class a correct system should output for
                               # this item — action-required | review-required |
                               # informational | not-affected | unknown (optional;
                               # legacy cases predate it and score on presence only)
    # How an evaluator recognises a generated change as covering this item.
    # A change matches when ANY matcher matches (each matcher: all given fields must match).
    match:
      - text: '(?i)rotationPolicy'          # regex over the change title + detail
      - subject: 'spec.privateKey.rotationPolicy'  # exact subject (values key path, CRD, flag, ...)
      # more optional matcher fields (all given fields must match):
      #   category: helm-values     # change category, exact
      #   release: '^1\.18'         # regex over the release the change is attributed to
      #   evidence: 'upgrading-1\.17'  # regex over the URIs of the change's evidence
      #   breaking: true            # change flag, exact
      #   actionRequired: false
    evidence:                  # authoritative upstream statements (ground truth citations)
      - url: https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md
        quote: "The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`"
    references:                # issues/PRs showing real-world impact, when found
      - https://github.com/<org>/<repo>/pull/<number>
notExpected:                   # things that a tool might flag but that are NOT upgrade-relevant (optional)
  - title: test-only dependency bumps
    match: [{text: '(?i)bump .* in /test'}]
    classification: action-required  # optional G9: the class such output should NOT have carried
                                     # ("must NOT false-alarm"); feeds the confusion matrix and
                                     # the false-action rate
environment:                   # optional, only when environment/ exists
  description: Small production cluster, Kubernetes 1.31, ACME HTTP01 via ingress-nginx, Prometheus ServiceMonitor enabled
  kubernetes: "1.31"
  expectedImpact:              # which expected items affect THIS environment, and why
    - expected: E1
      relevance: review        # action-required | review | informational | not-affected
      why: Certificates do not set rotationPolicy, so they inherit the new default
  expectedFindings:            # impact findings the join should emit for THIS environment
    - id: F1
      match:                   # finding matcher: all set fields must match
        subject: prometheus.servicemonitor.targetPort  # exact subject of a finding's matches
        rule: impact:values-pinned                      # exact join rule (optional)
        classification: informational                   # exact class (optional)
      why: the pinned value keeps winning over the changed default
  notExpectedFindings:         # findings that must NOT appear (optional)
    - title: no CRD removed by this upgrade
      match: {rule: impact:crd-removed}
      why: the CRD sets of the two endpoints are identical
```

## Environment expectations (extension of the original format)

The impact join speaks its own vocabulary — findings are keyed by join rule
(`impact:values-removed`, `impact:values-pinned`, …) and environment subject
(key paths, image refs), not by upstream titles. The original format could
only tie environments to `expected[]` items (`expectedImpact`); that alone
cannot express "this pinned values key must surface as an informational
finding" or "no CRD-removed finding may appear". `expectedFindings` and
`notExpectedFindings` fill that gap; both are part of the format since the
evaluator (G9) landed. `crds/` was added to the fixture directory for the
same reason (the join consumes installed CRDs).

`expectedImpact` with `relevance: not-affected` asserts the opposite: no
finding may join a change covering that item.

## Semantic labels (G22) and undecided links

Added for the learning loop (docs/phase3/learning-loop/DESIGN.md §9). All of
it is optional and additive; the existing scoring is unchanged. Labels are
authored **blind from upstream sources** (never from pipeline output), and
they are **never read by knowledge authoring** — they score the semantic
stage (per-model subject / change / applicability accuracy, G15) and the
transfer of verified facts (G12).

Per expected item, `semantics:` (one mapping, or a list when one upstream
item bundles several subjects):

```yaml
  - id: E1
    # … the usual fields …
    semantics:
      subject: {family: crd-field, product: cert-manager, group: cert-manager.io, kind: Certificate, path: spec.privateKey.rotationPolicy}
      change: {type: default-changed, before: '"Never"', after: '"Always"'}
      consequence: {kind: behavior-change, exposedClass: review-required, statement: "…"}
      note: optional one-line judgement (the rationale belongs in NOTES.md)
```

Per `expectedImpact` link, the applicability condition for THIS environment
and the fixture evidence that decides it:

```yaml
    - expected: E1
      relevance: review
      why: …
      exposure:                      # domain condition language, verbatim (DESIGN.md §1.3)
        op: resource
        group: cert-manager.io
        kind: Certificate
        of: [{op: field, path: spec.privateKey.rotationPolicy, state: unset}]
      overlap: {…}                   # optional: touches the subject but shielded (→ informational)
      environmentEvidence: manifests/certificate.yaml#L7-L18   # or a list
```

Links whose honest answer is UNKNOWN (the fixture lacks or withholds the
deciding input) go to `environment.undecidedImpact`, never to
`expectedImpact`:

```yaml
  undecidedImpact:
    - expected: E3
      reason: environment-visibility-gap   # a domain UnknownReason
      needed: the argocd-cm ConfigMap (resource.exclusions override)
      why: …
      exposure: {…}                        # optional
      environmentEvidence: [manifests/]    # optional: what was examined
```

Rules (enforced at load time):

- Field names are the domain JSON names (`replacedBy`, `exposedClass`, …) and
  the blocks decode **strictly** into `domain.Subject` / `ChangeSpec` /
  `Consequence` / `Condition`: an unknown key is an error, and each block
  must pass the domain `Validate()` (one spelling per subject family, the
  class follows the consequence kind, scoped leaves only inside a scope, …).
  `before` / `after` / `values` are JSON-encoded strings: write `'"Never"'`,
  `'"30"'` or `'30'`, never a bare number.
- A label is complete: `subject`, `change` and `consequence` are all
  required, and subjects belong to the case's own product (other products
  appear through `product-relationship` / conditions).
- A link with `exposure` requires the item's `semantics` and at least one
  `environmentEvidence` locator: `<path under environment/>[#L<n>[-L<m>]]`
  (the file, and the lines, must exist), or `environment.kubernetes` (the
  declared cluster version) / `from` (the edge's from-version, what runs
  today).
- An expected item is linked at most once per environment (decided or
  undecided).
- `undecidedImpact` links are **recorded, not scored**: counting them in
  `applicabilityAccuracy` (correct = no affected and no not-affected finding
  joins the item) is a scoring change and must be pre-registered first.

## Transfer environments (G12)

A verified release-level fact must carry over to other clusters without
re-review. To measure that, a case can have **transfer cases**: sibling
directories whose `case.yaml` declares `transferOf: <base-case-id>` and only
an environment — a second, independently authored cluster for the same
transition (some links affected, some not, some undecided):

```yaml
id: cert-manager-1.17-1.18--edge-cluster
transferOf: cert-manager-1.17-1.18
researchedAt: "2026-10-01"
sources: [ … fixture grounding only … ]
environment:
  description: …
  expectedImpact: [ … ]
  undecidedImpact: [ … ]
```

Product, from/to, `expected` and `notExpected` are inherited from the base
case at load time and must not be restated (one copy of every expected item
and its `semantics`). The runner runs the impact join against the transfer
environment, and the entry contributes **only environment numbers**
(links, findings, confusion cells, action/unknown accounting) — the shared
edge is scored once, by the base entry. Adjudications of the base case
apply. This was chosen over a second `environment:` block per case because
it needs no change to the edge scoring, gives every environment its own
entry id (the environment context label the transfer subset is computed
from) and its own stored result.

## Vocabulary

- `kind` values (open set, but the evaluator enforces the known ones so
  typos fail loudly): `breaking`, `removal`, `deprecation`,
  `behaviour-change`, `helm-values`, `crd-schema`, `api`, `compatibility`,
  `security`, `artifact`, `migration-step`, `dependency`. `dependency` was
  added with the evaluator for bundled-toolchain bumps (Argo CD's Helm and
  Kustomize, kube-prometheus-stack's operator): they are upgrade knowledge,
  but neither a product behaviour change nor a migration step.
- `importance`: `critical` | `important` | `minor`.
- `relevance`: `action-required` | `review` | `informational` |
  `not-affected` (mirrors `domain.ImpactClass` plus the negative case).

## Loading rules the evaluator enforces

- ids unique, kinds/importances/relevances known, every regex compiles,
  semantic labels and conditions valid (see "Semantic labels"),
  every expected item has at least one matcher (an unmatchable expectation
  can never be found and would silently inflate the miss count),
  every expected item carries at least one evidence citation,
  `expectedImpact[].expected` references an existing item,
  an `environment:` block requires at least one file in `environment/`.

## Stored results (`eval/results/`)

`ri eval` compares every run against the committed snapshots
(`<case-id>.json`: per-entry metrics plus the missed/false-positive ids) and
exits non-zero on any regression (fewer found items, new misses, more false
positives, more duplicate groups, more unsupported conclusions, worse
environment numbers). `ri eval -update` rewrites the snapshots **after human
review**; nothing ever rewrites `case.yaml` — expectations are data, not
pipeline output.

## Expected classifications (G9)

`expected[].classification` states the class a correct system should output
for the item (in a case with an environment, the class for THAT environment —
the scorer compares it with the class observed there, so a linked item's
classification matches its link: action-required ↔ action-required, review ↔
review-required, informational ↔ informational, not-affected ↔ not-affected,
undecidedImpact ↔ unknown; transfer cases do not score item classes) (five-class vocabulary: `action-required`, `review-required`,
`informational`, `not-affected`, `unknown`). It is optional — cases predating
it score on presence only — but new cases should declare it whenever the
fixture can defend the claim. `notExpected[].classification` and
`notExpectedFindings[].classification` state the class such output should NOT
have carried (usually `action-required`: "must NOT false-alarm").
`expectedFindings[].match.classification` doubles as a match clause (the
finding must carry that class to count as found) and as the finding's label
in the confusion matrix and the suggestion scoring.

An item's observed class is the strongest class among the findings joining
its matched changes (unknown outranks not-affected: a miss that denies
required action is worse than a checked-and-clear record); without a
joinable finding, the change-level action flag provides the only fallback
signal. Items with an expected class and no observable signal score nothing
— silently counting unobservable classes as misses would manufacture
failures the fixture cannot support.

## Confusion matrix and severity weighting

The aggregate report contains the expected×actual matrix over labelled
units (one cell per expected-item × change pair, strongest finding class).
Off-diagonal cells are weighted per docs/ACTION_CLASSIFICATION.md:
ACTION → NOT AFFECTED ×10 (catastrophic), ACTION → UNKNOWN and
NOT AFFECTED → ACTION ×5 (serious), ACTION → REVIEW ×1 (tolerable but
imperfect); the remaining pairs follow the same logic and are tabulated in
`internal/eval/classification.go`. The report prints the weighted miss.

## Metrics (G10)

Aggregate fields of `ri eval -o json` (`aggregate`):

- `recall`, `criticalRecall`, `importantRecall` — found/expected overall and
  per importance.
- `precision` — deprecated alias of the raw labeled precision (matched
  changes / (matched + notExpected-matched)); kept one release.
- `labeledPrecision` — precision over adjudicated output: dataset labels
  plus the human verdicts in `eval/adjudications/<case>.yaml`
  (true-positive / false-positive / uncertain; uncertain moves a change out
  of both sides). `adjudicatedTrue`/`adjudicatedFalse` are its counts.
- `falseActionRate` — wrong action-required findings / all action-required
  findings; wrong = unsupported, matching a notExpected entry (change or
  finding level), joining an item whose ground-truth class is softer, or
  joined to an adjudicated-false change. `actionFindingEvidence` = 1 −
  unsupported ACTION findings / ACTION findings (the both-chain gate).
- `applicabilityAccuracy` — (affected links hit + clean not-affected links)
  / all applicability decisions; `impactAccuracy` remains the affected-only
  view.
- `classificationAccuracy` — expected classes whose observed class matched,
  over observable class expectations (`classificationScored`).
- `unknownRate` — UNKNOWN findings / all findings over environment entries.
- `evidenceCoverage` — matched changes with resolving evidence / matched
  changes. `unsupported` counts conclusions whose evidence does not resolve.
- `duplicateRate` — changes inside a duplicate group / all changes.
- `pipelineFailures` — entries whose pipeline could not run (execution
  errors, never conflated with reasoning misses; the entry still scores
  zero found).
- `suggestionPrecision`, `suggestionRecall` (enriched runs) — the AI layer's
  `suggestedClassification` judged against the fixture labels: correct when
  the label is review-required or unknown; recall counts labelled findings
  whose correct class IS unknown that carry a suggestion. Unlabelled
  suggestions are counted (`unlabeled`) but score nothing.

## Hard gates (G11)

`eval/gates.yaml` holds the pre-registered thresholds (defaults per the
phase plan; a missing file means the defaults). `ri eval` renders the gate
panel and exits non-zero when a gate fails. A gate whose metric observed
nothing (no environment decisions, no ACTION findings) is **vacuous**: it
passes, and the panel says so — a vacuous pass proves nothing. Thresholds
are never tuned after reading gate outcomes; failures are findings.

## Adjudications

`eval/adjudications/<case>.yaml`:

```yaml
case: <case-id>
reviewedAt: "2026-10-01"
method: how the sample was drawn (audit trail)
adjudications:
  - changeId: chg-…
    verdict: false-positive   # true-positive | false-positive | uncertain
    reason: one line, grounded in the case's cited sources
```

Verdicts fold into `labeledPrecision` (and the false-action rate) at scoring
time; they are data, committed next to the expectations, and every verdict
without a reason fails validation.
