# Validator audit (both directions) — `validate-4`

Scope: the six `semvalidate` validators plus the render lane's `rendered-diff`, as run by `ri knowledge validate`
over `.ri/semantic-run-v2/knowledge` (519 candidates, 1122 proposals, 1366 results; a scratch copy, the original
untouched). Method, tooling and every number below are reproducible with `scripts/validator-audit/`.

**Headline.** The first run contained **4 false confirmations** (a values key reported *added* that had only moved
root) and **8 ambiguous ones** (a key in several charts confirmed from one), all from one capture defect — Istio's
1.23 chart values are snapshotted under a `defaults.` wrapper — plus **18 validator gaps** among the 37 CRD/values change
refutations (and 13 more in the subject aspect) that rejected true or unprovable claims. All are fixed generically with regression tests. After the fix every
automatically checkable confirmation (407 of 414) re-checks as true against the raw artifacts; the other 7 were read by
hand and are true. The price is **32 true confirmations downgraded to inconclusive** (caution, not error).

## Method

1. **Refutations** (all 71 refuted checks): each read against the raw ingested release JSON
   (`.ri/store/<product>/releases/*.json`) and the candidate's upstream text; classified *model error* (the proposal is
   wrong), *edge scope* (true upstream, but not a change of this edge's endpoints — a correct refutation), or
   *validator gap* (the artifact does not contradict the claim, the validator was wrong or over-eager).
2. **Confirmations, population re-check.** `scripts/validator-audit/verify.py` is an independent implementation (it reads
   the raw store JSON, shares no code with `internal/semvalidate`, and applies its own truth rules including the
   `defaults.` normalisation). It re-checks **every** confirmed values / crd / restatement / canonical result
   (438 of 445 confirmed checks; the other 7 — 3 compat, 1 image, 3 rendered-diff subjects — were read by hand and are true).
3. **Confirmations, stratified manual sample.** 61 confirmations, stratified by validator × aspect (all 15
   canonical-applicability confirmations, all rendered-diff / compat / image confirmations, 5–6 from every other
   stratum, seeded `random.seed(20261002)`), each printed next to the raw from/to artifact (`sample.py`) and judged by
   reading it.

## Confirmations (the dangerous direction)

Original run, before the fixes. `sample` = manually read; `pop` = confirmed results of that stratum, all re-checked
automatically; *gap* = true but under-specified (the claim holds in one chart of several).

| validator | aspect | pop | sample n | correct | wrong | gap | pop wrong | pop gap |
|---|---|---:|---:|---:|---:|---:|---:|---:|
| rendered-diff | subject | 3 | 3 | 3 | 0 | 0 | 0 | 0 |
| canonical | applicability | 15 | 15 (all) | 15 | 0 | 0 | 0 | 0 |
| canonical | consequence (`none`) | 40 | 6 | 5 | 0 | 1 | 0 | 2 |
| compat | subject | 3 | 3 (all) | 3 | 0 | 0 | 0 | 0 |
| crd | subject | 68 | 5 | 5 | 0 | 0 | 0 | 0 |
| crd | change | 33 | 6 | 6 | 0 | 0 | 0 | 0 |
| image | subject | 1 | 1 (all) | 1 | 0 | 0 | 0 | 0 |
| restatement | subject | 97 | 5 | 5 | 0 | 0 | 0 | 0 |
| restatement | change | 76 | 6 | 6 | 0 | 0 | **2** | 3 |
| values | subject | 64 | 5 | 5 | 0 | 0 | 0 | 0 |
| values | change | 45 | 6 | 6 | 0 | 0 | **2** | 3 |
| **total** | | 445 | **61** | 60 | **0** | 1 | **4** | 8 |

**The stratified sample did not hit the 4 wrong confirmations** — they are 4 of 438 and all sit in one product — and was
only complemented by the full re-check. A sample alone would have under-reported. The wrong ones:

- `values` + `restatement`, `helm-value global.platform added`, Istio 1.23.4 → 1.24.0 (2 + 2 results). Raw:
  1.23.4 `chart-base` holds `defaults.global.platform`; 1.24.0 holds `global.platform`. The key was **not added**, its
  root moved. The validators saw it absent from the source and present in the target and confirmed. A deterministic fact
  "global.platform added" would have been minted from a snapshot artifact.
- Same defect, ambiguous: `seLinuxOptions added` (new in `chart-cni`, already `defaults.seLinuxOptions` in
  `chart-ztunnel`), `env added` (new in cni/istiod, present elsewhere): confirmed from whichever chart came first
  (values 3, restatement 3, canonical consequence `none` 2).

Root causes: (1) `products/istio.yaml` does not strip the `defaults.` wrapper for ≤ 1.23 — *a capture defect, reported
below*; (2) the validators compared key sets without noticing that two snapshots of one chart were rooted differently;
(3) a key present in several charts was decided by the first chart.

### Canonical applicability — all confirmations

All 15 (18 after the fixes) were read against the raw artifact: `helm-value default-changed` (6: kvstoremesh ×3,
hubble certValidityDuration ×2, dnsProxy…, envoy.enabled) — source/target values hold exactly the asserted before/after,
condition = `values-key unset` / overlap `set`; `helm-value removed` (9: bgp section, remoteNodeIdentity,
endpointStatus, containerRuntime.integration ×2, proxy.prometheus.enabled, proxy.sidecarImageRegex ×2) — key
(or whole section) in the source, gone in the target, condition = `values-key set`. **0 wrong.** All sit on Cilium
`cilium-chart` (one chart, no re-rooting). The residual risk is semantic, not factual: the canonical table says a
removed key is exposed when the environment *sets* it; the artifact proves removal, not that the chart ignores the value.

## Refutations

Original run (1366 results; 71 refuted checks; counts are checks, proposals that assert the same thing included).

| validator · aspect | refuted | model error | edge scope (correct) | validator gap |
|---|---:|---:|---:|---:|
| crd · change | 21 | 6 | 2 | **13** |
| crd · subject | 5 | 1 | 0 | **4** |
| values · change | 16 | 7 | 4 | **5** |
| values · subject | 17 | 7 | 1 | **9** |
| compat · change | 2 | 2 | 0 | 0 |
| restatement · change | 6 | 1 | 0 | **5** |
| image · change | 1 | 1 | 0 | 0 |
| rendered-diff · change | 3 | 0 | 1 | **2** (render lane) |

Examples of each class (raw evidence in `.ri/store`):

**Validator gaps (fixed):**
- `NodePool.spec.disruption` default `{"consolidateAfter":"0s"}` → none (3 crd + 3 restatement results): the source
  schema states that default, the target states none. The model sent the object as a JSON *string* and `null` as the
  after; both were read literally. Now confirmed.
- `EC2NodeClass.spec.metadataOptions` default (object as string): refuted, now confirmed.
- `CiliumLoadBalancerIPPool.*` ("group cilium.io ships no kind …"): the CRD snapshot of Cilium does not contain
  runtime-generated CRDs, so "no such kind" was a snapshot gap, not a fact. A missing kind is now inconclusive.
  Same for `gateway.networking.k8s.io GRPCRoute` (another project's group).
- `NodePool` `ExpireAfter` moved `spec.disruption` → `spec.template.spec` and `WhenUnderutilized` →
  `WhenEmptyOrUnderutilized`: real, but **between API versions** (v1beta1 keeps the old path/enum, v1 has the new);
  neither version's schema changes. Per-schema refutation was wrong; now inconclusive with that explanation.
- Istio `istio_cni`, `pilot.enabled`, `base.enableCRDTemplates` (values/restatement): keys that existed under
  `defaults.` in 1.23 — the same re-rooting that produced the false confirmations, in the refuting direction. The values
  validator now compares with the wrapper removed (`istio_cni removed`, `pilot.enabled removed` confirmed; the
  restatement validator marks the unreliable computed diff).
- Optional keys the defaults file omits (`podDisruptionBudget.minAvailable`, `serviceAccount.annotations`,
  `etcd.managed`): absent from both snapshots next to their siblings is not proof they do not exist → inconclusive.

**Model errors (correct refutations):** `azureDNS.tenantID` added (the new field is `managedIdentity.tenantID`),
`spec.consolidationPolicy.renameFrom` (nonsense path), `httpPutResponseHopLimit` 2 → 1 (the *object* default changed,
the property's own default did not), `ciss` short name as "GVK added", `wireguard.userspaceFallback` (real key
`encryption.wireguard.userspaceFallback`), `waypoint.affinity` (real `global.waypoint.affinity`), Kafka requirement
sets `!=3.8.0,!=3.8.1` and `>=4.0.0` (the table admits {3.9, 4.0}), Argo CD cosign "image added".

**Edge scope (correct):** `EC2NodeClass.spec.kubelet` and the `v1` API "added" — `v1` already shipped in 0.37.8;
`webhook.extraEnv` — already in cert-manager 1.16.0; `etcd.operator.kvstore-opt` — gone before 1.15.

**Not mine, reported:** `rendered-diff` refutes the true storage-version moves of `NodePool`/`NodeClaim`
(`v1beta1 → v1`) as "the rendered value did not change": the render compares served objects, not the storage flag
(the crd validator confirms the move from the schema). Render lane gap, 2 results.

After the fixes the same 71 checks: 10 → confirmed (true), 35 → inconclusive (gaps), 26 stay refuted (model error /
edge scope).

## What changed (generic, with regression tests)

| fix | test |
|---|---|
| `defaults.`-style root drift between releases: compare with the wrapper removed when the key sets then coincide (≥ half); otherwise never confirm/refute; a path naming the wrapper is inconclusive | `TestValuesRootedDifferentlyAtTheTwoReleases` |
| a key in several charts needs the charts to agree unless the subject names its chart; a chart that cannot be compared makes an unnamed change ambiguous | `TestValuesMultiChartKeyNeedsAgreement`, `TestValuesUnreadableChartMakesAnUnnamedChangeAmbiguous` |
| optional keys next to existing siblings, absent from both releases: inconclusive, not refuted | `TestValuesOptionalKeyNextToSiblingsIsNotRefuted` |
| object/array defaults sent as JSON strings; `null` ≡ "no schema default" at the target (not provable at the source) | `TestCRDDefaultEncodedAsStringAndRemovedDefault`, `TestValuesDefaultAsStringEncodedList`, `TestRestatementDefaultRemovedIsNull` |
| a CRD kind missing from the snapshot is inconclusive (the snapshot may not hold every kind) | `TestCRDFieldValidator`, `TestGVKValidator` |
| moves between API versions (field / enum) are inconclusive, never refuted or confirmed | `TestCRDCrossVersionMovesAreInconclusive` |
| unversioned CRD claims must hold for every version (removed) / not be a whole new version's fields (added); attribute claims use the target storage version | `TestCRDUnversionedClaimsNeedEveryVersionToAgree` |
| restatement: a computed removal/addition of keys *below* a path never restates the parent section; only the key itself or a vanished/new section does | `TestRestatementDoesNotWidenBelowAKeyIntoItsParent` |
| restatement: a chart rooted differently at the two releases confirms existence only; its diff cannot restate a change | `TestRestatementIgnoresReRootedCharts` |
| canonical `added → none` is withheld for a new **required** CRD field (existing resources are rejected) | `TestCanonicalNoneIsNotConfirmedForANewRequiredField` |

## After the fixes (same store, `ri knowledge validate` on a fresh copy)

| validator · aspect | confirmed | refuted | inconclusive |
|---|---:|---:|---:|
| canonical · applicability | 18 | 0 | 356 |
| canonical · consequence | 30 | 0 | 63 |
| compat · subject / change | 3 / 0 | 0 / 2 | 20 / 21 |
| crd · subject / change | 68 / 37 | 1 / 8 | 17 / 41 |
| image · subject / change | 1 / 0 | 0 / 1 | 2 / 2 |
| restatement · subject / change | 93 / 64 | 0 / 0 | 80 / 109 |
| values · subject / change | 66 / 31 | 5 / 6 | 13 / 47 |
| rendered-diff · subject / change | 3 / 0 | 0 / 3 | 576 / 576 |

Re-check of the new confirmations: **all 407 checkable ones true** (`runverify.py` on the new store). Confirmations that
disappeared (`transitions.py`): 4 wrong, 8 ambiguous, **32 true** (canonical consequence 8, restatement 15, values 9) —
the cost of refusing to confirm what a re-rooted or multi-chart artifact cannot prove. Confirmations gained: 4 crd +
4 restatement changes, 3 canonical applicabilities, 2 values subjects (all true).

## Reported to other lanes (not changed here)

1. **capture / `products/istio.yaml`:** the 1.23.x Helm values snapshots are rooted under `defaults.` (every key of
   all five charts), 1.24 is not. Computed `values:*` diffs for Istio 1.23→1.24 are therefore bogus (e.g. "New Helm
   values section `global.*` (62 values)"); the validators now guard against it, the declarative fix (an
   `ignoreKeys`/`stripPrefix` era for ≤ 1.23) belongs in the product definition.
2. **capture:** the Cilium CRD snapshot lacks runtime-generated CRDs (`CiliumLoadBalancerIPPool`, …).
3. **render lane:** the `rendered-diff` validator refutes storage-version moves; it renders served objects only.
4. Optional (commented-out) values keys are invisible to values snapshots; they stay inconclusive by design.

## Reproduce

```text
ri -offline -state <state> knowledge validate -dir <copy-of-knowledge> -edge <p:from:to,…>
export KV=<that dir> STORE=<state>/store EDGEMAP=<candidate-id → "product:from:to" json>
python3 scripts/validator-audit/runverify.py     # independent re-check of every confirmation
python3 scripts/validator-audit/sample.py        # the stratified sample next to raw artifacts
KV_AFTER=<dir validated by newer validators> python3 scripts/validator-audit/transitions.py
```

`EDGEMAP` maps each candidate id to the first requested edge producing it (regenerate the ids with
`semantic.BuildCandidates` per edge, as `ri knowledge validate` does).
