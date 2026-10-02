# Research notes: transfer environment plant-edge (Cilium v1.16.1 → v1.17.0)

Transfer environment for `cilium-1.16-1.17` (same release transition,
independently designed cluster). Researched blind (no Release Intelligence
output consulted) on 2026-10-01; the base case's items, semantics and
environment were read but not modified.

## Sources read

- Version-specific notes: "1.17 Upgrade Notes" of
  https://github.com/cilium/cilium/blob/v1.17.0/Documentation/operations/upgrade.rst
  (Removed Options / Deprecated Options / Helm Options subsections).
- Chart values at both endpoints:
  https://github.com/cilium/cilium/blob/1.16.0/install/kubernetes/cilium/values.yaml
  https://github.com/cilium/cilium/blob/1.17.0/install/kubernetes/cilium/values.yaml
- TLS-visibility CNP shape (terminatingTLS secret in kube-system under the
  default `local` secrets backend):
  https://github.com/cilium/cilium/blob/v1.16.0/examples/kubernetes-tls-inspection/l7-visibility-tls.yaml
- BGP control-plane CRD shapes:
  https://github.com/cilium/cilium/blob/v1.16.0/Documentation/network/bgp-control-plane/bgp-control-plane-v2.rst
- CRD file sets at both tags (for the notExpectedFinding):
  https://github.com/cilium/cilium/tree/v1.16.0/pkg/k8s/apis/cilium.io/client/crds
  https://github.com/cilium/cilium/tree/v1.17.0/pkg/k8s/apis/cilium.io/client/crds
  — identical file sets under v2/ and v2alpha1/.

## Cluster design (and how it differs from the base environment)

plant-edge is a small bare-metal edge cluster at a manufacturing site
(Kubernetes 1.30; base is 1.29), Cilium v1.16.1 via Helm. It was designed
from the upgrade notes, deliberately different from the base case's
mid-size production cluster:

- **Encryption on** (base: none): WireGuard transparent encryption with the
  userspace fallback, because two of three nodes run Ubuntu 20.04 kernels
  without in-tree WireGuard.
- **ClusterMesh + external workloads** (base: none): peered with a central
  cluster; two historian VMs joined as external workloads.
- **BGP via the control plane** (base: the removed legacy metallb-bgp
  `bgp.*` block): LB IPs announced with CiliumBGPClusterConfig /
  CiliumBGPPeerConfig / CiliumBGPAdvertisement — the replacement path.
- **TLS secrets**: neither `tls.secretsBackend` nor the new key set; the
  TLS-aware CNP keeps its secret in kube-system under the 1.16 default
  `local` backend (base sets `tls.secretsBackend: k8s`).
- **Hubble certificates**: `method: cronJob` with `certValidityDuration`
  unset (base: `method: helm`, pinned 1095).
- **dnsProxy.endpointMaxIpPerHostname** unset (base: pinned 50).
- Hubble Relay without UI (base: UI enabled); operator replicas 1 (base: 2).

Every pinned key is grounded in the two values.yaml files: 1.16.0 has
`encryption.wireguard.userspaceFallback: false` ("Enables the fallback to
the user-space implementation (deprecated)"), `externalWorkloads.enabled:
false`, `hubble.tls.auto.method: helm` with `certValidityDuration: 1095`
(method comment lists `cronJob`), `dnsProxy.endpointMaxIpPerHostname: 50`,
`bgpControlPlane.enabled: false` beside the legacy `bgp.enabled` block;
1.17.0 drops `userspaceFallback` and the whole `bgp:` block, defaults
`certValidityDuration: 365` and `endpointMaxIpPerHostname: 1000`, and
deprecates `tls.secretsBackend: ~` in favour of
`tls.readSecretsOnlyFromSecretsNamespace: ~`.

## Link judgements

- **E11 → action-required** (affected). `userspaceFallback: true` is set
  (values.yaml L26-L34). Upstream: "The previously deprecated built-in
  WireGuard userspace-mode fallback (Helm ``wireguard.userspaceFallback``)
  has been removed. Users of WireGuard transparent encryption are required
  to use a Linux kernel with WireGuard support." Canonical exposure for
  helm-value · removed: `values-key{Path, set}`.
- **E9 → review** (affected). `externalWorkloads.enabled: true` (L43-L46).
  Upstream: "The External Workloads feature has been deprecated and will be
  removed in v1.18." Deprecation consequence → review (works in v1.17).
  Exposure `values-key{Path, set}` — the canonical deprecated shape minus a
  replacedBy (E9 has none).
- **E5 → review** (affected). `hubble.tls.auto.certValidityDuration` unset
  with `method: cronJob` (L56-L67). Upstream: "The default value of
  ``hubble.tls.auto.certValidityDuration`` has been lowered from 1095 days
  to 365 days". Canonical exposure for default-changed: `values-key{Path,
  unset}` → the 365-day default applies to certificates generated after the
  upgrade. Deliberate difference from the base link's semantics statement,
  which describes the `helm` generation method: here the quarterly certgen
  CronJob regenerates certificates, so the lowered default applies at the
  next rotation rather than at `helm upgrade`; the class (review) is
  unchanged.
- **E6 → informational** (affected). `dnsProxy.endpointMaxIpPerHostname`
  unset (L68-L71); the CNP's toFQDNs rule puts the DNS proxy in the path.
  Upstream: the default "has been increased from 50 to 1000 to reflect
  improved scaling of toFQDNs policies". Canonical `values-key{Path,
  unset}` → consequence `none`/informational.
- **E4 → not-affected.** Neither `tls.secretsBackend` nor
  `tls.readSecretsOnlyFromSecretsNamespace` is set (values.yaml L52-L55);
  the CNP's terminatingTLS secret lives in kube-system
  (manifests/historian-scrape-cnp.yaml L21-L24), matching the 1.16 default
  `local` backend (upstream example keeps such secrets in kube-system).
  Canonical exposure for deprecated + replacedBy — `all[old set, new
  unset]` — evaluates false (old unset), so not-affected. Nuance recorded
  honestly: the SDS rework also adds `tls.secretSync` defaults for upgraded
  clusters; that is a new-feature default, not the E4 deprecation item, and
  upstream states the upgraded-cluster default keeps
  `readSecretsOnlyFromSecretsNamespace: true` (the same namespace-restricted
  behaviour as today's `local`).
- **E3 → not-affected.** BGP announcements run on the control plane
  (values.yaml L47-L51: `bgpControlPlane.enabled: true`;
  manifests/bgp-advertisement.yaml L5-L49: the three v2alpha1 CRDs plus the
  LoadBalancer Service). Upstream: the `bgp.enabled`,
  `bgp.announce.podCIDR`, `bgp.announce.loadbalancerIP` options "have been
  removed. Users are now required to use Cilium BGP control plane options
  available under ``bgpControlPlane``". E3 bundles three removed values, so
  the item-level exposure is `any[values-key set]` across the three paths —
  the per-subject canonical `values-key{Path, set}` generalized over the
  bundle; it evaluates false (values supplied, none of the keys present).
- **E10 → undecided** (environment-visibility-gap). The cluster runs
  ClusterMesh with a central peer (values.yaml L35-L42), the deployment
  profile whose runbook may have tuned the removed agent flag. Upstream:
  "The previously deprecated ``clustermesh-ip-identities-sync-timeout`` flag
  has been removed in favor of ``clustermesh-sync-timeout``." The flag lives
  in the agent's container args, and the fixture contains no workload
  manifest (nor a live-release values dump) that shows them; the checked-in
  values file has no `extraArgs`, but install-time `--set` flags would not
  appear in it. The cli-flag condition (`set` on
  `--clustermesh-ip-identities-sync-timeout`) is therefore honestly
  UNKNOWN; needed: the live DaemonSet spec or `helm get values --all`.

## expectedFindings / notExpectedFindings

- F1 (`encryption.wireguard.userspaceFallback`): the key is verifiably gone
  from the 1.17.0 chart values, so a values-set-on-removed-key finding is
  expected.
- No `impact:crd-removed` finding: the CRD file sets at v1.16.0 and v1.17.0
  are identical (checked both trees), including the v2alpha1 BGP resources
  this cluster uses.

## Base-label observations (reported, not changed)

- E11's semantics path `encryption.wireguard.userspaceFallback` matches the
  chart nesting (1.16.0 values.yaml: `encryption:` → `wireguard:` →
  `userspaceFallback`), even though the upgrade note spells the Helm value
  as `wireguard.userspaceFallback`; the label is chart-accurate and I kept
  the same path in my exposure. No other base label looked wrong to me.
