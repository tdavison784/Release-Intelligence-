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
eval/results/        # stored results (snapshots), committed after review;
                     # `ri eval -update` rewrites them, tools never touch
                     # case.yaml
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
