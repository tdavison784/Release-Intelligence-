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
    images.txt       #   optional mirror list, one image ref per line
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
    kind: behaviour-change     # breaking | removal | deprecation | behaviour-change | helm-values | crd-schema | api | compatibility | security | artifact | migration-step
    importance: critical       # critical (would break/surprise prod) | important (should know/act) | minor (nice to know)
    actionRequired: true       # an operator must do something (or consciously decide)
    # How an evaluator recognises a generated change as covering this item.
    # A change matches when ANY matcher matches (each matcher: all given fields must match).
    match:
      - text: '(?i)rotationPolicy'
      - subject: 'spec.privateKey.rotationPolicy'
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
```
