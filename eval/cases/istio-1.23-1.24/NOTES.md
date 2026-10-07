# Research notes — istio 1.23.4 → 1.24.0

Authored blind on 2026-10-01 from the upstream documents below.

## Sources read

- Upgrade notes for 1.24 (istio.io announcing-1.24/upgrade-notes): ambient
  DNS-capture ordered upgrade (E1), istio-csr gRPC/ALPN breakage with the
  GRPC_ENFORCE_ALPN_ENABLED workaround (E2), istiod-remote chart merge (E3),
  Helm-templated CRDs (E4), Sidecar scoping unification (E5), peer-metadata
  attribute standardization (E6).
- Change notes for 1.24 (announcing-1.24/change-notes): the 1.20
  compatibilityProfile removal (E7) and the general "**Added**/**Improved**"
  feature bullets behind the notExpected entry.
- supportStatus.yml (the definition's support-status source): Istio 1.24's
  row lists k8sVersions ["1.28","1.29","1.30","1.31"] — grounds F1 at the
  1.28 boundary for the fixture's cluster.

## Judgement calls

- E1/E2 are action-required: both name concrete failure modes ("DNS
  resolution failures", gRPC handshake failures) the upgrade notes say WILL
  happen without remediation, and the fixture has both preconditions
  (dnsCapture: true; istio-csr v0.12.0 in the cluster story).
- E6 is review-required despite naming an API break: whether the telemetry
  values actually disappear depends on the deployed Telemetry CRs (the
  fixture does carry a legacy wasm.* attribute, but proving the metric tag
  breaks needs runtime evidence the join cannot produce).
- The environment values file keeps the OLD defaults.pilot layout keys —
  istio-discovery's values between 1.23 and 1.24 restructure
  `defaults.pilot.*` into `_internal_defaults_do_not_set.*` (chart diff
  verified at both tags); the join sees the moved keys through the chart
  values diff, though the fixture makes no explicit finding claims about
  them (the chart is vendored by istioctl upgrades in most meshes, so the
  operator impact of the internal rename is deliberately left unclaimed).

## Correction after the first live run

E1's original matchers included the bare word "Ztunnel", which matches ~29
ambient-related release notes (the structured-notes channel emits one change
per upstream note file) rather than the one upgrade-notes section the
expectation was authored from ("## Ambient upgrade with DNS proxy"). The
matchers now cite that exact heading and the `cni.ambient.dnsCapture` value;
the change is grounded in the upstream document's own wording, not in
pipeline output (the pipeline's per-note decomposition was already visible in
the cited channel).

## Semantic labels and corrections (2026-10-01, groundtruth lane)

Authored blind from the upstream sources above (no pipeline output read); corrections are listed with before/after values in eval/CHANGELOG.md.

- E2 is a `product-relationship` to istio-csr: the fix (istio-csr PR 422, merged 2024-10-25) first shipped in v0.13.0 (2024-11-25), so the required range is `>=0.13.0`; the exposure also names the documented workaround (`meshConfig.defaultConfig.proxyMetadata.GRPC_ENFORCE_ALPN_ENABLED: "false"`).
- E6 corrected to action-required: upstream says the expressions "must" use the standard attributes and the old ones no longer hold the peer metadata; the fixture's Telemetry CR uses exactly the old attribute. The exposure path uses `tagOverrides.*.value` — a map wildcard the condition path syntax does not define yet (raised with the contract owner).
- E4 is labelled on the istio-base chart value `base.enableCRDTemplates` (default false → true), which the notes name as the opt-out.

## E6 exposure path fix (2026-10-02, groundtruth-6; LOOP-DIAGNOSIS-2 §8.2)

The map wildcard `tagOverrides.*.value` was never part of the condition path
syntax, so the exposure could only evaluate false ("no such literal key `*`").
Narrowed to the concrete key the fixture's Telemetry CR actually overrides
(`peer_namespace`, telemetry.yaml#L18-L21). A real map wildcard remains a
contract request (status file); this fix makes the label decidable with the
language that exists. See eval/CHANGELOG.md "2026-10-02 (f)".
