# Research notes: kube-prometheus-stack 90.2.0 → 91.0.0

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- The chart's own upgrade guide, "From 90.x to 91.x" section:
  https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-91.0.0/charts/kube-prometheus-stack/UPGRADE.md
- The chart release body:
  https://github.com/prometheus-community/helm-charts/releases/tag/kube-prometheus-stack-91.0.0
  (major 91 = "Bump prometheus-operator to v0.94.0")
- The operator's v0.94.0 release notes (the CHANGE items the chart inherits):
  https://github.com/prometheus-operator/prometheus-operator/releases/tag/v0.94.0

## Judgement calls

- The chart guide is short (operator bump + RBAC tightening + CRD update
  procedure); the operator's [CHANGE] entries supply the behavioural detail.
  E2 (wildcard verbs) appears in BOTH sources, which is why it is a separate
  expected item.
- E3 is critical: skipping the CRD step is the classic broken kube-prometheus
  upgrade.
- E4/E5 come from the operator's CHANGE log entries that materially change
  how Alertmanager/ScrapeConfig resources are accepted.
- notExpected: the helm-charts repository publishes releases for every chart
  in the monorepo; entries for other charts and renovate/CI chore entries
  are the known false-positive surface for this product.
