# Dataset changelog

Every change to an existing ground-truth label (case.yaml expectation, link relevance, fixture fact)
is recorded here with the value before and after, and why. Corrections are made only where an
upstream source or the fixture itself contradicts the label; the rationale is grounded in the
upstream document, never in pipeline output (FLEET.md "Evaluation integrity"). Thresholds in
`eval/gates.yaml` are never touched.

## 2026-10-01 — groundtruth lane (learning loop, MISSION G22)

Context: the dataset observations D1–D14 of
`docs/phase3/learning-loop/UNKNOWN-ANALYSIS.md` §4. Each correction below was re-verified against
the upstream source quoted.

| Case · item | Field | Before | After | Why (upstream) | Obs. |
|---|---|---|---|---|---|
| cilium-1.15-1.17 · E1 | `classification` | action-required | not-affected | The case's own edge starts at `from: v1.15.6`. upgrade.rst at v1.16.0: "it is required to run Cilium v1.15.6 or newer before upgrading to Cilium v1.16" — the precondition is already met for this transition, whatever the cluster. | D2 |
| cilium-1.15-1.17 · link E1 | `relevance` | action-required | not-affected | Same: the environment runs v1.15.6 (edge from-version; images.txt). The link's own `why` said "exactly the minimum". | D2 |
| cilium-1.15-1.17 · E8 | `classification` | action-required | review-required | `install/kubernetes/cilium/templates/_helpers.tpl` at v1.17.0 (`readSecretsOnlyFromSecretsNamespace`): when the new key is unset, `tls.secretsBackend: local` → true, anything else (`k8s`) → false. values.yaml at v1.17.0: "This value obsoletes `tls.secretsBackend`, with `true` == `local` in the old setting, and `false` == `k8s`." The key is deprecated and still honoured — no concrete failure, a deprecation. | D8 |
| cilium-1.15-1.17 · link E8 | `relevance`, `why` | action-required ("a key the v1.17.0 chart deprecates/empties") | review | Same. | D8 |
| cilium-1.16-1.17 · link E4 | `relevance`, `why` | action-required ("a key that no longer exists in the v1.17.0 chart") | review | Same; the key does exist in the v1.17.0 values (`secretsBackend: ~`) and is still read by the templates. | D8 |
| strimzi-0.45-0.46 · E6 | `classification` | review-required | not-affected | The fixture's Kafka CR has no `spec.kafka.authorization` and the environment description mentions no OPA; the link `why` ("the fixture's authorization configuration references OPA") had no fixture support. Relabelled rather than inventing an OPA block the description never stated. | D3 |
| strimzi-0.45-0.46 · link E6 | `relevance`, `why` | review | not-affected | Same. | D3 |
| strimzi-0.45-0.46 · link E5 | `why` | "the fixture's Kafka CR pins metadataVersion 3.8-IV0 … the brokers must step to 3.9/4.0" | re-grounded on `KafkaMirrorMaker2.spec.version: 3.8.0` | kafka-versions.yaml at 0.45.0: 3.8.0/3.8.1/3.9.0 supported, default 3.9.0; at 0.46.0: 3.8.x `supported: false`, 3.9.0 and 4.0.0 supported. deploying.html (0.46): "`Kafka.spec.kafka.version`, which defaults to the latest supported Kafka version … if not specified" and "If you upgrade the Cluster Operator to a version that does not support the current version of Kafka you are using, you get an unsupported Kafka version error." The Kafka CR sets no version (brokers run 3.9.0; 3.8-IV0 is a metadata version), the MM2 pins 3.8.0. Relevance unchanged (action-required). | D10 |
| strimzi-0.45-0.46 · E5 | `classification` | review-required | action-required | Internal consistency: the scorer compares the item class with the class observed in this case's environment, and the case's own link says action-required (generic reading, review-required, kept in a comment). | D10 |
| strimzi-0.45-0.46 · environment | `description`, `inventory.yaml` | "pinned to Kafka 3.8 (metadataVersion 3.8-IV0)"; inventory `kafka "3.8"` | brokers on the 0.45 default 3.9.0 with metadataVersion 3.8-IV0; MirrorMaker 2 pinned to 3.8.0; inventory: `kafka 3.9.0` (brokers) and `kafka 3.8.0` (MM2) | Same sources; the description restated the fixture wrongly (metadata version ≠ broker version). The two inventory entries deliberately disagree — the cluster runs two Kafka versions. | D10 |
| cert-manager-1.16-1.17 · E2 | `classification` | action-required | review-required | upgrading-1.16-1.17.md: "If you're manually enabling this feature gate, it's advisable to stop." `internal/controller/feature/features.go` at v1.17.0: `ValidateCAA: {Default: false, PreRelease: featuregate.Alpha}` (still functional); at v1.18.0: "Removed: v1.18 … this is a no-op and only prints a log line if added" (no crash). Nothing fails on this transition; it is a deprecation (docs/ACTION_CLASSIFICATION.md, DESIGN.md §1.4). | D7 |
| cert-manager-1.16-1.17 · link E2 | `relevance`, `why` | action-required | review | Same. | D7 |
| cert-manager-1.16-1.17 · link E1 | moved | `expectedImpact` relevance review | `undecidedImpact` (environment-visibility-gap) | cert-manager PR 7368 (the change): "The algorithm is decided by the **signer**. For SelfSigned, that's the same as the cert being issued … but for the CA issuer it can be easy to forget." Release notes 1.17: "The CA and SelfSigned issuers now use SHA-512 when signing with RSA keys 4096 bits and above … If you were previously using a larger RSA key as a CA". The fixture's 4096-bit leaf is signed by the CA issuer `internal-ca`, whose key size (Secret `internal-ca-key`) the fixture does not show — the old `why` reasoned from the leaf's key. | new |
| cert-manager-1.16-1.17 · E1 | `classification` | review-required | unknown | Same (the class a correct system outputs for this environment). | new |
| istio-1.23-1.24 · E6 | `classification` | review-required | action-required | Upgrade notes 1.24: "CEL expressions in the telemetry API **must** use the standard Envoy attributes … Peer metadata is now stored in `filter_state.downstream_peer` and `filter_state.upstream_peer` instead of `filter_state["wasm.downstream_peer"]`". The fixture's Telemetry CR overrides `peer_namespace` with exactly the old attribute, so the tag stops carrying the peer namespace (loss of intended behaviour). The item's own `actionRequired: true` comment already said "stop producing values"; the review class was justified in NOTES by what the join could prove, not by what is true. | new |
| istio-1.23-1.24 · link E6 | `relevance`, `why` | review | action-required | Same. | new |

Net effect on the environment links: 21 affected links → 18 affected + 2 not-affected (decided) + 1
undecided (recorded, not scored). No matcher was changed.

### Considered and deliberately not changed

- **D1** (cilium E6/E3 matchers cannot see the computed `bgp.*` values change): left as is. A
  `subject: bgp.enabled` matcher would be motivated by pipeline output; the restatement link is the
  knowledge layer's job. The upstream note that names the keys verbatim is now recorded in the items'
  `semantics` (helm-value `bgp.enabled` / `bgp.announce.*` removed), which is where it belongs.
- **D4–D6** (cross-product facts only in descriptions): the `envinv` lane already added
  `inventory.yaml` files from the descriptions. D6: the cert-manager-1.17 E3 link stays
  action-required — the environment description states the default ingress-nginx configuration; the
  exposure label names the `strict-validate-path-type` override, so an engine that cannot see the
  ingress-nginx ConfigMap must answer below ACTION, which is a visibility limit, not a label error.
- **D9** (karpenter E7 matchers on env vars, link evidence in Helm values): not changed; the item's
  `semantics` list both the env-var and the Helm-value subjects, so the semantic stage can be scored
  on either.
- **D11** (rotationPolicy review vs the MISSION demo): stands as decided (behavior-change → REVIEW).
- **D12** (loose matchers): not tightened. Which incidental changes a matcher catches can only be
  learned from pipeline output.
- **D13** (last-write-wins attribution): evaluator behaviour, not a dataset label.
- **D14** (`spec.acme.solvers` on a Certificate in cert-manager-1.17's fixture): cosmetic; the
  E3 link's `environmentEvidence` cites the ClusterIssuer's solvers, which are the valid evidence.

### Observations recorded for the commander (not acted on)

- strimzi-0.45-0.46: because `spec.kafka.version` is unset, 0.46 will move the brokers to its default
  Kafka 4.0.0, which makes E8 (Log4j2) relevant to this cluster's Reload4j-era logging ConfigMap
  (deploying.html 0.46: "Custom Log4j configuration must be placed under the log4j2.properties key").
  karpenter-0.37.8-1.0.0: the fixture's NodePool (WhenUnderutilized, no consolidateAfter) and
  EC2NodeClass (no amiSelectorTerms) are also exposed to E5. argo-cd-2.14-3.0's ApplicationSet has
  no nested selectors (E7 does not apply). These would be new links; adding them changes denominators,
  so they are left for an explicit decision.
