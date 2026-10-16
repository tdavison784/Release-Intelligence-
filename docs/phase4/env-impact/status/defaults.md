# Lane `defaults` — status

Branch `p4ei/defaults` (from p4-env-impact @ f467a54d). Brief: `briefs/defaults.md`. **State: done.**

## Done
- `internal/upgrade/values_defaults.go`: `FromDefaults(change)` recovers each subject's source default
  (JSON) from the Detail `diffValues` writes (removed, section-removed, default-changed); `EmptyDefault`
  (`null`, `""`, `{}`, `[]`). Round-trip tests against the real diff (separators inside values, an arrow
  inside a default-changed value, truncated section listings).
- **Join (step 1)** `internal/impact/removed_default.go`: new rule `impact:values-removed-default`
  (review-required, severity medium, confidence medium; `absence` matches citing the values files that do
  not set the key = environment chain) for a removed key the environment leaves unset whose source
  default is non-empty, or not recorded in a truncated section listing. Empty defaults keep
  `impact:values-unset` (not-affected). Keys the environment sets keep today's logic. Never
  action-required from this rule; ACTION REQUIRED contract untouched. `valuesAbsence` in condition.go
  refactored into a free function so the builder reuses it (behaviour unchanged).
- **Correlation (step 2)** `internal/render/correlate_default.go`, wired into `match()` as the
  `values-default` rule (already a strong correlation for `validate.render-diff` and §2.2.1): a rendered
  change whose before holds, under a field named like the key's last segment, the key's source default
  (value, or redaction digest for credential-named fields) more often than its after does, and whose
  object the key's top-level section names (object name, kind or a rendered path segment).
  Minimum-specificity guard: booleans, null, empty values, strings < 4 chars, numbers with |n| < 10,
  maps/lists encoding < 8 chars never link.
- Tests: `internal/impact/removed_default_test.go` (unset ⇒ review; empty default ⇒ not-affected
  regression; set key ⇒ as today; sections incl. truncation; default-changed stays PO-3),
  `internal/render/correlate_default_test.go` (unit cases for every guard incl. location, overridden
  value, default still present, default-changed analogue, hand-built change; live-helm end-to-end on new
  synthetic chart `testdata/charts/dflt-{1.0.0,1.1.0}`: exporter ServiceMonitor change correlated, a
  sibling ServiceMonitor hard-coding the same path not, empty-default key stays not-affected, pinned
  customer keeps values-removed).
- Docs: IMPACT.md (values table), RENDER.md (correlation + values-unset note), INVESTIGATE.md (new
  section "Removed and changed defaults the environment relied on").

## Decisions
- Default-changed keys left unset stay with PO-3 (product-owner decision, ACTION_CLASSIFICATION.md):
  the counterfactual render decides (`values-default-no-effect` is a counterfactual proof, not render
  silence); without a render `values-default-unrendered` stays not-affected in the five-class report,
  and the investigation's `validate.join` already treats that verdict as a gap (never a clearing pass).
  So for default-changed only the correlation half of the brief applies.
- A change that records no source default at all (hand-built, e.g. `eval/adversarial` fixtures) keeps
  today's verdict: no default is known to have been relied on. Every real values diff records it.
- The new link reuses the rule name `values-default` (not a new name) so it is a strong correlation
  without touching `internal/investigate/validate` (validate lane's package).
- Defaults are parsed from the Detail text rather than adding a field to `domain.Change`: no domain /
  schema change, stored edges and goldens unaffected; the parser lives next to the writer and a
  round-trip test pins the format.

## Files touched outside ownership
- `internal/upgrade/values_defaults.go` + `_test.go`: new, additive.

## Contract changes
- none (new impact rule id is in `internal/impact/api.go`, like `RuleValuesUnset`).

## Phase 3 regression check (`ri -state <shared .ri> eval`, eval/gates.yaml untouched)
before = f467a54d, after = this branch. No gate worse; no new regressions vs eval/results.

| metric | before | after |
|---|---|---|
| applicabilityAccuracy (gate ✗ both, min 0.80) | 0.525 | **0.535** |
| impactLinksHit / impactAccuracy | 23/71 (0.324) | **24/71 (0.338)** |
| confusion REVIEW→REVIEW / REVIEW→NOTAFF | 7 / 11 | **9 / 9** |
| weightedMiss | 978 | **972** |
| falseActionRate (gate ✗ both, pre-existing) | 0.059 (1/17) | 0.059 (1/17) |
| criticalRecall / importantRecall | 1.000 / 0.970 | same |
| actionFindingEvidence / unsupported / pipelineFailures | 1.000 / 0 / 0 | same |
| classification accuracy / unknown rate | 0.40 / 0.80 | same |
| vs stored results | no regressions | no regressions; karpenter-0.37.8-1.0.0--ci-buildfarm impactLinksHit 2→3 better |

With `-render`: applicabilityAccuracy 0.564 → 0.574, links 28 → 29, weightedMiss 882 → 876,
render-verifiable hit 7 → 8; the 2 regressions vs eval/results (cilium-1.15-1.17 findingsFound,
cilium-1.16-1.17--plant-edge notAffectedViolations) are identical before and after (pre-existing).
Changed lines: karpenter E7 now hit by `values-removed-default`; kyverno E9 gains a second finding.
`eval/results` NOT regenerated (`-update` not run): the only movement is an improvement; commander's call.

## Motivating case, pipeline output only (`ri investigate` on obs-north / obs-south input.yaml + env/, no agent, cloned state)
- obs-north: before 20 not-affected (20 eligible) + 6 potentially; after 17 not-affected (17 eligible)
  + 5 potentially + **1 affected**. `coreDns.serviceMonitor.bearerTokenFile` → **affected**
  (validated-exposure: env change `rc-566829f890c0` on the coredns ServiceMonitor now correlated by
  values-default, previously anchored to the unexplained item `ci-1871449ec07f`, which is gone — its
  exposure is attributed to the key). All four diagnostic items' join verdicts are now
  `values-removed-default` review-required (no longer values-unset).
- `kubeDns` / `kubeProxy` / `kubeEtcd` `…bearerTokenFile` stay not-affected **eligible** via
  `validate.render-diff` on a complete environment render: obs-north's values disable kubeEtcd and
  kubeProxy, and kubeDns is off by chart default — the FROM render holds no kube-dns/kube-etcd/kube-proxy
  ServiceMonitor, so the default was never used in this environment (the brief's "used by the chart"
  precondition does not hold). At release scope (chart defaults) the kube-etcd and kube-proxy
  ServiceMonitor changes now correlate precisely to their own removed keys. obs-south: same shape.

## Open question (commander)
- If the eval's ground truth scores these three per-key items against a per-note expectation (B3
  "File-based ServiceMonitor values removed", the known one-note-several-items granularity), they may
  still count in unsafeEligibilityRate. Per key they are correctly not-affected (component not deployed,
  proven by a complete render). I did not tune anything toward the case; a granularity decision belongs
  to the commander/eval lane.

## Test status
- `go build ./... && go vet ./... && go test -count=1 ./...` green (43 packages ok).

## Cleanup
- bin/ removed; scratch state clones and eval outputs in the session scratchpad removed; no kind
  clusters, no servers started.
