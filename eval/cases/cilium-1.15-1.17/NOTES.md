# Research notes — cilium 1.15.6 → 1.17.0 (multi-hop)

Authored blind on 2026-10-01 from the upstream documents below.

## Why multi-hop

Cilium's upgrade notes are written per minor and the project supports
upgrading one minor at a time; an operator on 1.15.6 moving to 1.17.0 walks
two documented hops. The edge therefore spans BOTH the "1.16 Upgrade Notes"
section (in upgrade.rst at v1.16.0) and the "1.17 Upgrade Notes" section (in
upgrade.rst at v1.17.0). The case measures whether the pipeline surfaces the
union of two hops' upgrade work — not just the target minor's notes.

## Sources read

- upgrade.rst at v1.16.0 — the 1.16 hop: toFQDNs version gate (E1), LB IP
  pool cidrs removal + allowFirstLastIPs default (E2), GRPCRoute API move
  (E3), Envoy DS default (E4), enableRuntimeDeviceDetection deprecation +
  bpf.enableTCX default (E5).
- upgrade.rst at v1.17.0 — the 1.17 hop: metallb-bgp removal (E6),
  proxy-visibility removal (E7), tls.secretsBackend replacement (E8),
  wireguard.userspaceFallback removal (E9 — deprecated in the 1.16 notes,
  removed in the 1.17 notes: the multi-hop lifecycle of one key).
- Chart values at 1.15.6, 1.16.0 and 1.17.0, diffed for the environment
  findings: 1.15.6→1.16.0 removes the `containerRuntime` section and flips
  `enableRuntimeDeviceDetection` false→true; 1.16.0→1.17.0 removes the `bgp:`
  section and empties `tls.secretsBackend` (`local` → `~`).

## Judgement calls

- E4 is review-required, not action-required: the impact (evicting one Pod
  per node) depends on node headroom the fixture does not state; the
  upgradeCompatibility escape hatch keeps old clusters unaffected.
- E5 informational: the fixture explicitly pins the old default
  (enableRuntimeDeviceDetection: false), so the pin keeps winning.
- The 1.16 notes' etcd/KVStoreMesh/clustermesh items are real but the case
  caps at nine expectations; the picked items cover removal, CRD field,
  API-version, default-change and multi-hop-lifecycle shapes.
- The environment's F1/F3 findings span BOTH hops — a removed-by-1.16 key and
  a removed-by-1.17 key — which is exactly what a single-hop-only pipeline
  would miss.

## Semantic labels and corrections (2026-10-01, groundtruth lane)

Authored blind from the upstream sources above (no pipeline output read); corrections are listed with before/after values in eval/CHANGELOG.md.

- E1 corrected to not-affected: the edge itself starts at v1.15.6, so the "v1.15.6 or newer before v1.16" precondition is met (exposure: `edge-from-version` out of `>=1.15.6`, evidence `from`).
- E8 corrected to review: the v1.17.0 chart's `_helpers.tpl` still maps `tls.secretsBackend` onto `readSecretsOnlyFromSecretsNamespace` while the new key is unset (D8). The label uses `deprecated` + `replacedBy`, with the canonical exposure "old set ∧ new unset" and overlap "new set".
- E6 uses the three Helm keys the 1.17 notes name verbatim ("The metallb-bgp integration Helm options bgp.enabled, bgp.announce.podCIDR, and bgp.announce.loadbalancerIP have been removed"); the matchers were not changed (D1).
- E9: the real chart path is `encryption.wireguard.userspaceFallback` (values.yaml at 1.15.6/1.16.0), which the notes abbreviate.

## 2026-10-02 — E1 matcher narrowed (judgement call, CHANGELOG (h))

LOOP-DIAGNOSIS-2 §8.5 reported that E1's bare `(?i)toFQDNs` alternative let an unrelated dnsProxy
finding reach this link's not-affected label at the knowledge view. The item's own evidence quote is
about one thing — "run Cilium v1.15.6 or newer before upgrading to v1.16" — so the matcher now
requires that subject (the `toFQDNs.*1\.15\.6|1\.15\.6 or newer` alternative, unchanged) and the bare
`toFQDNs` alternative is gone. Verified on the current store: plain metrics unchanged except this
case's matched changes (26 → 24; the stored snapshot is left for the commander to accept); at the proxy view the dnsProxy finding no longer
violates the not-affected label (applicability 0.571 → 0.581, cilium-1.15 E1 `!` → `✗`), and the item
still recalls on chg-23a43a125719, which carries the quote verbatim.

Standing PO question (same shape as the bgpControlPlane note in cilium-1.16-1.17): this narrows a
matcher on a computed change; CHANGELOG 2026-10-02 (c) approved that class for crossplane. Veto
welcome — reverting restores the bare alternative.
