# Research notes: Cilium v1.16.1 → v1.17.0 (with environment)

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Sources read

- Version-specific notes: the "1.17 Upgrade Notes" section of
  https://github.com/cilium/cilium/blob/v1.17.0/Documentation/operations/upgrade.rst
  (Removed Options / Deprecated Options / Helm Options subsections). This is
  the authoritative operator-facing list for the 1.16 → 1.17 transition; the
  same page states the only tested upgrade path is between consecutive minor
  releases, which is exactly this case.
- Chart values at both endpoints (for the helm-values items and the
  environment fixture):
  https://github.com/cilium/cilium/blob/1.16.0/install/kubernetes/cilium/values.yaml
  https://github.com/cilium/cilium/blob/1.17.0/install/kubernetes/cilium/values.yaml

## Judgement calls

- Kept the 11 most operator-relevant items out of a much longer list; the
  page also documents MTU auto-detection changes, cluster-name validation,
  services protocol differentiation, hubble-relay --dial-timeout no-op,
  --k8s-watcher-endpoint-selector, bugtool flags, metrics renames, operator
  QPS defaults. Metrics renames are recorded as notExpected (they are
  monitoring dashboard work, not upgrade blockers).
- The four Helm-values items (E4–E6, and bgp under E3) are stated in the
  upstream notes AND verifiable in the two values.yaml files, which is what
  the environment fixture pins.
- notExpectedFindings (no CRD removed): verified by listing
  pkg/k8s/apis/cilium.io/client/crds at both tags — identical file sets.
- The environment sets tls.secretsBackend: k8s (not local) so the fixture
  exercises the "agent reads secrets cluster-wide" half of the deprecation,
  which is the security-relevant direction upstream calls out.
