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

## 2026-10-02 — groundtruth lane, review of the hand-off window

| Case · item | Field | Before | After | Why |
|---|---|---|---|---|
| loki-2.9-3.0 · E5, E10 / E7 / E6 | `classification` | action-required / review-required / action-required | not-affected / informational / unknown | Internal consistency (eval/FORMAT.md "Expected classifications"): the case was authored before the env-class convention was fixed; the links (not-affected, informational, undecided) were already right. Generic class kept in a comment. |
| traefik-2.11-3.0 · E3, E5, E7, E9 / E6, E10 | `classification` | review/action-required / action/review-required | not-affected / unknown | Same. |
| crossplane-1.20-2.0 · E4 | citation `url` | cmd/crossplane/main.go at v1.20.1 | internal/xpkg/name.go at v1.20.1 | The quoted `DefaultRegistry string = "xpkg.crossplane.io"` lives in internal/xpkg/name.go (line 64); main.go does not contain it. |
| cert-manager-1.17-1.18 · E3 | citation `quote` (PR 11819) | a paraphrase written as a quote | the PR's own words | Quotes must be verbatim. |

Raised by the blind transfer authors and verified upstream:

| Case · item | Field | Before | After | Why |
|---|---|---|---|---|
| karpenter-0.37.8-1.0.0 · E7 | `semantics` subjects, link `exposure`, `environmentEvidence` | helm-value `assumeRoleARN` / `assumeRoleDuration`; evidence values.yaml#L5-L11 | `settings.assumeRoleARN` / `settings.assumeRoleDuration`; evidence values.yaml#L5-L10 | charts/karpenter/values.yaml at v0.37.8 lines 178–182: both keys sit under `settings:`. The fixture's top-level `assumeRoleARN: ""` (L11) was never a chart key (not honoured even on 0.37.8), so it is no longer cited; the link stays action-required through `logConfig` (L5–L10). The fixture is unchanged. |
| karpenter-0.37.8-1.0.0 · sources | added | — | compatibility.yaml on `main` | The F4 `why` relies on the 1.0.x record (min 1.25 / max 1.30), which the v1.0.0 tag does not contain (it ends at 0.37.0); the definition falls back to `main`. The finding itself is unchanged. |

Not acted on: cert-manager-1.17-1.18 F1 cites the website releases README support table, which on
`master` no longer lists 1.18 (the table rolls forward with releases); the 1.29 → 1.33 window for 1.18
was recorded when the case was authored. A pinned source would make it reproducible.

`TestDatasetLabelConsistency` now enforces the convention (and that an affected link's relevance is
the exposed class of one of the item's consequences).

Observation (pre-existing citations, not changed): an automated verbatim check of every quote in the
eight environment cases flags ten more quotes that are paraphrases or condensed renderings of the
cited document (YAML structure written inline, markdown link brackets dropped, ellipsis-joined
fragments): argo-cd-2.14-3.0 E5, cert-manager-1.16-1.17 E3, cilium-1.15-1.17 E8 (1.16.0 values),
cilium-1.16-1.17 E4 (1.16.0 values) and E9, istio-1.23-1.24 E2 and E6, karpenter-0.37.8-1.0.0 E7
(both values.yaml citations), strimzi-0.45-0.46 E4. Their substance was not re-verified in this pass; their wording
should be made verbatim (or re-checked) in a later one. All 216 quotes of the seven new cases
verify (after the crossplane URL fix above).

## 2026-10-02 (b) — verbatim quotes in the environment cases (commander follow-up 1)

The ten pre-existing paraphrased quotes listed above now carry the cited document's exact text
(re-fetched at the cited ref). No label, matcher or URL changed; editorial insertions moved to YAML
comments.

| Case · item | Before (paraphrase) | After (verbatim source text) |
|---|---|---|
| argo-cd-2.14-3.0 · E5 | markdown link to the Dex scope dropped | full sentence incl. the `[scope](…)` link, upgrading/2.14-3.0.md L98–101 |
| cert-manager-1.16-1.17 · E3 | the two gate names joined into the sentence | sentence + the two list items as written (release-notes-1.17.md L75–78) |
| cilium-1.15-1.17 · E8, cilium-1.16-1.17 · E4 | `tls: secretsBackend: local` (YAML path flattened) | `secretsBackend: local` (values.yaml L2360 at 1.16.0, under `tls:`, noted in a comment) |
| cilium-1.16-1.17 · E9 | RST `:ref:` role dropped | the line as written (upgrade.rst L377 at v1.17.0) |
| istio-1.23-1.24 · E2 | editorial "[impacted]" inserted | "While Istio's control plane is not impacted by this, a popular third-party CA implementation, [`istio-csr`](…) is." … |
| istio-1.23-1.24 · E6 | markdown link dropped, two paragraphs merged | both sentences as written, joined by "…" |
| karpenter-0.37.8-1.0.0 · E7 (v0.37.8 values) | `logConfig: {…} / assumeRoleARN: "" / assumeRoleDuration: 15m` | the chart's own lines (incl. the "Logging configuration will be dropped by v1" comment); `settings:` nesting noted in a comment |
| karpenter-0.37.8-1.0.0 · E7 (v1.0.0 values) | editorial "(the logConfig and assumeRole keys of 0.37.8 are gone)" inside the quote | the `logOutputPaths` lines only; the observation moved to a comment |
| strimzi-0.45-0.46 · E4 | markdown link targets dropped | the bullet as written (CHANGELOG.md L34 at 0.46.0) |

With this, every quote of the fifteen environment cases verifies against its source (automated check,
markup-normalised; the remaining crossplane code-comment and flux HTML-list quotes were checked by
hand). Observation: the edge-only cases (argo-cd-3.0-3.1, cert-manager-1.15-1.16,
ingress-nginx-1.11-1.12, istio-1.28-1.29, postgresql ×2, terraform-provider-aws, vault) have about 30
quotes the same check cannot match verbatim; not touched here.

## 2026-10-02 (c) — crossplane-1.20-2.0 E1/E3 matchers — **motivated by pipeline output (trustfix)**

Disclosure: unlike every correction above, this one was motivated by reading pipeline output. The
trigger is docs/phase3/learning-loop/TRUSTFIX.md §1a: a correct ACTION on the computed Composition
schema change was attributed to E3 (not-affected) through a substring match. The product owner
decided to fix the matchers, disclosed. Each matcher below is justified from an upstream source (the
quotes added to the items). No relevance, classification, semantics or fixture changed. kyverno E9
(TRUSTFIX §1b) is deliberately untouched, pending a decision.

| Case · item | Field | Before | After | Upstream justification |
|---|---|---|---|---|
| crossplane-1.20-2.0 · E3 | matcher | `text: (?i)StoreConfig` (any substring, so it caught Composition `spec.publishConnectionDetailsWithStoreConfigRef`) | `text: (?i)\bStoreConfigs?\b` (the kind / its CRD plural only) | E3's title names "the StoreConfig API": `cluster/crds/secrets.crossplane.io_storeconfigs.yaml` exists at v1.20.1 and is absent at v2.0.0 (citation added). |
| crossplane-1.20-2.0 · E1 | matchers added | three text matchers (prose only) | `{subject: spec.mode, text: \bComposition\b}`, `{subject: spec.resources[], …}`, `{subject: spec.patchSets[], …}` | E1's own semantic subjects. The Composition CRD at v1.20.1: "Resources is a list of resource templates …" (citation added), and "All Compositions should use Pipeline mode. Resources mode is deprecated."; at v2.0.0 `default: Pipeline`, with no `resources`/`patchSets`. |

Note: `spec.publishConnectionDetailsWithStoreConfigRef` is itself an external-secret-store field
(E3's area). The computed change is an umbrella over three removed Composition fields, and the
evaluator attributes a change to the last matching item (D13). After the fix only E1 selects it, which
matches the dominant subjects (`spec.resources[]`, `spec.patchSets[]`). The environment sets neither
the store-config field nor a StoreConfig, so E3 stays not-affected.

What the narrowed / added matchers now select (fresh edge): E3 additionally matches
"CRD `storeconfigs.secrets.crossplane.io` (StoreConfig) removed" (its own subject). E1 additionally
matches the Composition schema changes "3 fields removed: `spec.patchSets[]`, `spec.publish…`,
`spec.resources[]`", "allowed values changed: `spec.mode`" and "1 default changed: `spec.mode`".

**Gate panel BEFORE** (full `ri eval`, 2026-10-02, branch p3ll/groundtruth after merging
p3-learning-loop with trustfix; state = primary checkout's warm cache):

| Gate | Actual | Pass | Detail |
|---|---|---|---|
| criticalRecall | 0.98 | ✓ | 50 critical items, 1 missed |
| importantRecall | 0.95 | ✓ | 100 important items, 5 missed |
| applicabilityAccuracy | 0.467 | ✗ | 105 decisions (affected 73: 18 hit; not-affected 32: 1 violation) |
| falseActionRate | 0.133 | ✗ | 15 ACTION findings, 2 wrong |
| actionFindingEvidence | 1.00 | ✓ | 0 unresolving |
| unsupported | 0 | ✓ | |
| pipelineFailures | 0 | ✓ | |

**Gate panel AFTER** (same run conditions, only this matcher change):

| Gate | Actual | Pass | Detail |
|---|---|---|---|
| criticalRecall | 0.98 | ✓ | 50 critical items, 1 missed |
| importantRecall | 0.95 | ✓ | 100 important items, 5 missed |
| applicabilityAccuracy | 0.486 | ✗ | 105 decisions (affected 73: 19 hit; not-affected 32: 0 violations) |
| falseActionRate | 0.067 | ✗ | 15 ACTION findings, 1 wrong (kyverno E9, pending) |
| actionFindingEvidence | 1.00 | ✓ | 0 unresolving |
| unsupported | 0 | ✓ | |
| pipelineFailures | 0 | ✓ | |

crossplane-1.20-2.0 alone: links 0/5 → 1/5 (E1 hit by the two ACTIONs plus the default-change
finding), not-affected violations 1 → 0 (E3), recall 9/10 unchanged (E8 still missed),
classificationAccuracy unchanged overall (0.438 over 112). No stored-result regressions in either run.


## 2026-10-02 (d) — evaluator fix: duplicate groups keyed on CRD identity (lane `eval-dup`)

**Evaluator change, no label changed.** The duplicate-conclusion audit (`internal/eval` `findDuplicates`,
non-gated `duplicateGroups` / `duplicateRate`) keyed its subject comparison on category + subject set. A CRD
schema-path diff carries bare paths as subjects, so the same path on *different* CRDs (`spec.resources[]` newly
required on CiliumEnvoyConfig and CiliumClusterwideEnvoyConfig, `status.conditions[]` added to several CRDs) was
counted as a duplicate. The subject key of a `crd:*` change now includes the CRD identity (group/kind, read from
the differ's deterministic title/detail wording; the CRD name when the title labels it by name). Every other
change is keyed exactly as before. Consequence by design: one kind's two served versions (v1 and v1beta2)
stating the same schema change still group, so a former cross-CRD group can split into one group per kind.

Live run (offline, warm cache), `duplicateGroups` per case (unlisted cases unchanged; `-update` not run):

| Case | Before | After |
|---|---|---|
| cert-manager-1.15-1.16 | 4 | 3 |
| cert-manager-1.16-1.17 | 2 | 0 |
| cert-manager-1.17-1.18 | 2 | 0 |
| cilium-1.15-1.17 | 10 | 1 |
| cilium-1.16-1.17 | 6 | 0 |
| crossplane-1.20-2.0 | 12 | 8 |
| external-secrets-0.15-0.16 | 4 | 0 |
| flux-2.6-2.7 | 7 | 10 (four per-kind v1/v1beta2 groups replace one cross-kind group) |
| kube-prometheus-stack-90-91 | 1 | 0 |
| kyverno-1.12-1.13 | 10 | 15 (same: per-kind v1/v2beta1 or v2/v2beta1 groups) |
| prometheus-operator-0.85-0.86 | 3 | 0 |
| strimzi-0.45-0.46 | 3 | 1 |
| **aggregate** | **75** (rate 0.028) | **49** (rate 0.018) |

No other aggregate metric moved (only `duplicateGroups`, `duplicateRate`, `duplicatedChanges`); no gate reads
these. `ri eval` reports flux and kyverno as "worse" against `eval/results` until the stored snapshots are
accepted with `-update` (commander).

## 2026-10-02 (d) — three rendered effects on not-affected links — **motivated by pipeline output (render-4)**

Disclosure: triggered by the render lane's finding (status/render.md; eval/render/results/2026-10-02-po3.md)
that `ri eval -render` attributes real rendered changes to three links labelled not-affected. Each
was judged from upstream sources only. Two links are relabelled and one stands.

| Case · item | Field | Before | After | Upstream judgement |
|---|---|---|---|---|
| cilium-1.16-1.17--plant-edge · link E4 | `relevance`, `why`, `exposure` | not-affected ("the new tls.secretSync default … is a new-feature default, not this deprecation item") | **review**; exposure: `tls.secretSync.enabled` unset ∧ `upgradeCompatibility` unset ∧ a CiliumNetworkPolicy with `terminatingTLS`/`originatingTLS` | The item (title "… replaced by tls.readSecretsOnlyFromSecretsNamespace (+ tls.secretSync)") is one upstream bullet. upgrade.rst v1.17.0: "The defaults for **new** clusters enable SDS via `tls.readSecretsOnlyFromSecretsNamespace: true` and `tls.secretSync.enabled: true`. The defaults for **upgraded** clusters (where `upgradeCompatibility` is `v1.16`) do not enable SDS." tls-visibility.rst v1.17.0: with SDS, Secrets referenced in Network Policy "are copied into a configured namespace (`cilium-secrets` by default) by the Cilium Operator". This cluster sets no `upgradeCompatibility` and runs a TLS-intercepting policy whose secret lives in kube-system. Its key material will be copied into a new namespace and served via SDS, which an operator should verify. |
| cilium-1.16-1.17--plant-edge · `environment/values.yaml` L52–55 | fixture comment | "the 1.16 chart default `local` backend reads them from kube-system" | states only which keys are unset | tls-visibility.rst v1.17.0: `local` meant "only read from the Secrets namespace". The comment contradicted upstream. No value changed. |
| cilium-1.16-1.17 · E4, cilium-1.15-1.17 · E8 | `semantics` (added a subject) | `tls.secretsBackend` deprecated only | + helm-value `tls.secretSync.enabled` added (default on unless `upgradeCompatibility` <= 1.16), consequence behavior-change → review | Same upstream bullet; release-level, so both cases that carry the item get it. Their links' relevance (review) is unchanged. |
| kyverno-1.12-1.13 · E7 | `classification`, link `relevance`, `why`, + `overlap` | not-affected | **informational** | kyverno website upgrading.md: "Kyverno 1.13 drops deprecated API versions for its managed CustomResourceDefinitions. The migration is handled automatically through Helm hook." The fixture stores two PolicyExceptions as `kyverno.io/v2beta1` (policyexceptions.yaml L4, L26), and the v1.13.0 chart's `crds.migration.resources` lists `policyexceptions.kyverno.io`. The cluster touches the subject and is shielded by the hook: DESIGN.md §1.3 overlap → INFORMATIONAL ("applies to you, you appear safe"). The exposure (hook disabled) is unchanged and still false. |
| cilium-1.16-1.17--plant-edge · link E3 | — | not-affected | **unchanged (stands)** | The rendered effect comes from the new key `bgpControlPlane.statusReport.enabled` (v1.17.0 values: "Status reporting settings (BGPv2 only) … if you have any issue such as high API server load, you can disable it"). It is not in the upgrade notes and not part of E3's subject: E3 is the metallb-bgp removal ("The metallb-bgp integration Helm options bgp.enabled, bgp.announce.podCIDR, and bgp.announce.loadbalancerIP have been removed"), and this cluster sets none of those keys. The effect is real but belongs to another change (BGPv2 status reporting, at most an informational API-load note). It reaches E3 only through the base case's broad matcher `(?i)bgpControlPlane` (D12 / TRUSTFIX §5). Narrowing that matcher is a separate product-owner decision and was not done here. |

**Gate panel BEFORE** (branch p3ll/groundtruth-4 = p3-learning-loop @ 6eb9cd4; state = primary
checkout's warm cache):

| Gate | plain `ri eval` | `ri eval -render` |
|---|---|---|
| criticalRecall | 0.98 ✓ | 0.98 ✓ |
| importantRecall | 0.95 ✓ | 0.95 ✓ |
| applicabilityAccuracy | 0.495 ✗ (affected 20/73 hit; not-affected 32, 0 violations) | 0.495 ✗ (affected 23/73; not-affected 32, **3 violations**: plant-edge E3, E4, kyverno E7) |
| falseActionRate | 0.059 ✗ (17 ACTION, 1 wrong) | 0.059 ✗ (17, 1) |
| actionFindingEvidence | 1.00 ✓ | 1.00 ✓ |
| unsupported | 0 ✓ | 0 ✓ |
| pipelineFailures | 0 ✓ | 0 ✓ |

**Gate panel AFTER** (same conditions, only these label changes):

| Gate | plain `ri eval` | `ri eval -render` |
|---|---|---|
| criticalRecall | 0.98 ✓ | 0.98 ✓ |
| importantRecall | 0.95 ✓ | 0.95 ✓ |
| applicabilityAccuracy | 0.476 ✗ (affected 20/75 hit; not-affected 30, 0 violations) | 0.514 ✗ (affected 25/75; not-affected 30, **1 violation**: plant-edge E3) |
| falseActionRate | 0.059 ✗ (17, 1) | 0.059 ✗ (17, 1) |
| actionFindingEvidence | 1.00 ✓ | 1.00 ✓ |
| unsupported | 0 ✓ | 0 ✓ |
| pipelineFailures | 0 ✓ | 0 ✓ |

Reading: in the render-free run the two relabelled links move from "clean not-affected" to "affected,
not hit", so plain applicabilityAccuracy drops (0.495 → 0.476). That is the honest cost of the
correction. With rendering both are hit (plant-edge E4 review, kyverno E7 informational).
classificationAccuracy is unchanged (0.438 over 112); recall is unchanged.

## 2026-10-02 (e) — stage-f links (LOOP-DIAGNOSIS.md) — **motivated by pipeline output (loop-diagnosis)**

Disclosure: triggered by LOOP-DIAGNOSIS.md stage f, which lists five links answered by a deterministic
finding on a computed change the item's matchers do not select. Product-owner policy: add a matcher
for the computed change **only where the item's own upstream quote names that subject verbatim**. Each
item's evidence quotes were checked against the computed change's subject. One qualifies; four do not
and were left unchanged.

| Case · item (link) | Computed change (subject) | Item's own quote names it? | Action |
|---|---|---|---|
| karpenter-0.37.8-1.0.0 · E7 | "Helm values section `logConfig.*` removed (7 keys)" (`logConfig.*`) | **Yes**: the v0.37.8 charts/karpenter/values.yaml quote, "# -- Log configuration (Deprecated: Logging configuration will be dropped by v1, use logLevel instead) logConfig: …" | matcher added: `{category: helm-values, text: (?i)\blogConfig\b}`. On the fresh edge it selects only that change. |
| cilium-1.15-1.17 · E6, cilium-1.16-1.17 · E3 | "Helm values section `bgp.*` removed (3 keys)" (`bgp.enabled`, `bgp.announce.*`) | **No**: the quote is "Support for ``metallb-bgp``, deprecated since 1.14, has been removed." | **left unchanged** (D1 stands). Note for the PO: the same cited document (upgrade.rst v1.17.0, "Helm Options") says verbatim "The metallb-bgp integration Helm options ``bgp.enabled``, ``bgp.announce.podCIDR``, and ``bgp.announce.loadbalancerIP`` have been removed". That sentence is recorded in these items' `semantics`/NOTES since 2026-10-01, but it is not one of their evidence quotes. Adding it as a quote and then a matcher would need a separate decision. |
| karpenter-0.37.8-1.0.0--ci-buildfarm · E7 (base item karpenter E7) | "Helm value `settings.featureGates.drift` removed" | **No**: the quote names the environment variable "`FEATURE_GATES.DRIFT=true` was dropped and promoted to Stable", not the Helm value `settings.featureGates.drift`. | **left unchanged**. Also note: the transfer link is `review` while the computed finding is ACTION, so a matcher here would count that finding as a false action unless the link were re-judged first. Both are decisions for the PO. |
| strimzi-0.45-0.46--edge-retail · E2 (base item strimzi E2) | "CRD `kafkamirrormakers.kafka.strimzi.io` (KafkaMirrorMaker) removed" | **No**: the quote is "Support for MirrorMaker 1 has been removed. Please make sure to migrate to MirrorMaker 2 …" and names neither the `KafkaMirrorMaker` kind nor its CRD. | **left unchanged**. (The base case's F3 `why`, authored blind, cites the CRD file removal, but that is not this item's quote.) |

**Gate panel BEFORE** (plain `ri eval`; branch p3ll/groundtruth-5 = p3-learning-loop @ 62afad6; state =
primary checkout's warm cache):

| Gate | Actual | Pass | Detail |
|---|---|---|---|
| criticalRecall | 0.98 | ✓ | 50 critical items, 1 missed |
| importantRecall | 0.95 | ✓ | 100 important items, 5 missed |
| applicabilityAccuracy | 0.476 | ✗ | affected 20/75 hit; not-affected 30, 0 violations |
| falseActionRate | 0.059 | ✗ | 17 ACTION findings, 1 wrong |
| actionFindingEvidence | 1.00 | ✓ | |
| unsupported | 0 | ✓ | |
| pipelineFailures | 0 | ✓ | |

**Gate panel AFTER** (same conditions, only the karpenter E7 matcher):

| Gate | Actual | Pass | Detail |
|---|---|---|---|
| criticalRecall | 0.98 | ✓ | 50 critical items, 1 missed |
| importantRecall | 0.95 | ✓ | 100 important items, 5 missed |
| applicabilityAccuracy | 0.486 | ✗ | affected 21/75 hit (+ karpenter E7); not-affected 30, 0 violations |
| falseActionRate | 0.059 | ✗ | 17 ACTION findings, 1 wrong (unchanged) |
| actionFindingEvidence | 1.00 | ✓ | |
| unsupported | 0 | ✓ | |
| pipelineFailures | 0 | ✓ | |

classificationAccuracy (0.438 over 112) and recall are unchanged. There are no stored-result regressions.

