# Research notes: Istio 1.28.3 → 1.29.0

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- Upgrade notes for 1.29 (the page explicitly scopes itself to "upgrading
  from Istio 1.28.x to Istio 1.29.0"):
  https://istio.io/latest/news/releases/1.29.x/announcing-1.29/upgrade-notes/
  (raw copy: istio/istio.io repo, content/en/news/releases/1.29.x/…)
- Cross-checked the 1.28 page
  (…/announcing-1.28/upgrade-notes/) to be sure each item is new in 1.29 and
  not carried over (e.g. seccompProfile, BackendTLSPolicy v1alpha3 removal
  and METRIC_ROTATION_INTERVAL removal are 1.28 items and are NOT in this
  case's expectation list).

## Judgement calls

- Every "##" section of the 1.29 page became one expected item except the
  dry-run/ambient introduction, which is kept (E7) because the page frames it
  as an upgrade-ordering hazard (old ztunnel + new istiod enforces policies).
- E1 is critical: the statsCompression annotation is REMOVED, so anyone who
  used it (or assumed plaintext Prometheus scrapes) changes behaviour without
  doing anything.
- notExpected: the InferencePool items on the 1.28 page are features, not
  1.29 upgrade blockers; flagging them for a 1.28→1.29 edge would be noise.
  (There are no InferencePool items on the 1.29 page itself.)
