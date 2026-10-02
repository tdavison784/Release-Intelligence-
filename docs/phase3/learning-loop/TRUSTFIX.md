# TRUSTFIX — why the knowledge-free engine fails falseActionRate (0.133)

> Lane `trustfix` (branch `p3ll/trustfix`, base `p3-learning-loop` @ 823b44c). Every number below was
> recomputed with `bin/ri -state <primary>/.ri eval -o json` (warm cache), plus `ri impact` /
> `ri upgrade -o json` per environment case with exactly the inputs `internal/eval/run.go` assembles
> (transfer cases resolved through `transferOf`). No case, gate, result or threshold was edited.

## Bottom line

- **15 ACTION REQUIRED findings across the 28 cases; the evaluator judges 2 wrong** (2/15 = 0.133).
  Contrary to the brief's framing, **neither of the 2 is an engine bug**:
  - **crossplane-1.20-2.0:** the ACTION is correct. The upstream guide requires migrating
    `mode: Resources` Compositions, and the fixture's selected Composition sets `spec.resources`. It is
    scored as wrong because a loose *matcher* on a **different, not-affected** item (E3,
    `(?i)StoreConfig`) selects the same bundled schema change. **Root cause: (b) — the label is right,
    the matcher scope is wrong.**
  - **kyverno-1.12-1.13:** `impact:values-removed` on the user's tuned `cleanupJobs.*` keys. The
    dataset labels the item review-required ("upstream replaced the function with circuit breakers").
    The engine applies the normative contract: a removed key whose value stops being honoured is
    `setting-ignored` → ACTION (DESIGN §1.4; IMPACT.md values table). The case's own NOTES concede "a
    reviewer could argue ACTION from loss of the configured 5000 threshold". **Root cause: (b) — the
    label disagrees with the contract.** Needs a commander/groundtruth decision; generic engine logic
    cannot tell "function superseded upstream" from a values diff.
- **The other 13 ACTIONs were all checked against fixtures and upstream: all justified** (table §2).
- **New join rules (applicability lane):** only `impact:crd-enum-value-removed` produced ACTIONs (2:
  crossplane `spec.mode: Resources`, external-secrets `engineVersion: v1`), and both are justified.
  The crd storage-version, default, required and type rules and the operand support-set rule produced
  **no** ACTION (§3).
- **Engine fixes (generic, each with a regression test):** two trust bugs in `crd-field-removed` were
  found and fixed (§4). Classes changed only in the dangerous direction's favour: external-secrets E5
  is no longer a not-affected violation. applicabilityAccuracy 0.457 → 0.467, classificationAccuracy
  0.429 → 0.438, notAffectedViolations 2 → 1. **falseActionRate is unchanged at 0.133, by design**:
  fixing it requires the two dataset decisions in §1, not engine changes.
- **Bigger trust finding outside the falseActionRate gate (§5):** the knowledge-free values join says
  **NOT AFFECTED** for a changed chart default the user does *not* set. That is backwards, because the
  new default is exactly what reaches them. This produces 8 of the 26 catastrophic
  ACTION → NOT-AFFECTED cells (kyverno E1 RBAC defaults, E5 hook image tag). Proposed fix and blast
  radius are below; it is a contract-level change, so it was left for the commander.

## Gate panel — before / after (knowledge-free run, same warm cache)

| Gate | Threshold | Before (823b44c) | After (this branch) | Verdict |
|---|---|---|---|---|
| criticalRecall | ≥ 0.95 | 0.980 | 0.980 | PASS |
| importantRecall | ≥ 0.90 | 0.950 | 0.950 | PASS |
| applicabilityAccuracy | ≥ 0.80 | 0.457 | **0.467** | FAIL |
| falseActionRate | < 0.05 | 0.133 (2/15) | 0.133 (2/15) | FAIL — both wrong ACTIONs are dataset issues (§1) |
| actionFindingEvidence | ≥ 1.00 | 1.000 | 1.000 | PASS |
| unsupported | ≤ 0 | 0 | 0 | PASS |
| pipelineFailures | ≤ 0 | 0 | 0 | PASS |

Other changes: classificationAccuracy 0.429 → 0.438, notAffectedViolations 2 → 1 (external-secrets
E5). Every other per-case metric is identical; there are no regressions against `eval/results`.

**Projection, if groundtruth accepts both §1 corrections:** 0/15 wrong → falseActionRate 0.00.
Narrowing the crossplane E3 matcher alone gives 1/15 = 0.067, which still fails. Relabelling kyverno
E9 alone gives 1/15 = 0.067, which also still fails.

## 1. The two ACTIONs judged wrong

### 1a. crossplane-1.20-2.0 — `imp-6aa16c4ffb9c` · `impact:crd-field-removed` · high

- **Change:** `chg-5937653bdbb9`, computed `crd:fields-removed`: "Composition v1 schema: 3 fields
  removed: `spec.patchSets[]`, `spec.publishConnectionDetailsWithStoreConfigRef`, `spec.resources[]`".
- **Env fact:** `manifests/compositions.yaml` L4 (Composition `xbuckets-aws-pt`, `mode: Resources`)
  sets `spec.resources` (L17–18). The Pipeline-mode copy does not, and the engine does not flag it
  (checked by running it alone → `crd-field-unset`, not-affected).
- **Why it is scored wrong:** the change is attributed to **E3** (external secret stores,
  relevance `not-affected`). E3's matcher `text: (?i)StoreConfig` matches the substring inside
  `spec.publishConnectionDetailsWithStoreConfigRef`. An ACTION joined to an item whose ground truth is
  softer counts as a false action. The same artefact is the case's not-affected violation (E3 "hit").
  E1's matchers (`native patch.and.transform`, `mode: ?Resources`, `convert pipeline-composition`)
  select neither this change nor the enum change `chg-8d886563fa13`. So **E1 is reported as missed
  even though the engine emits two correct ACTIONs for it**.
- **Upstream evidence that the ACTION is correct** (already cited in the case's E1):
  - "If you're using `spec.mode: Resources` in your Compositions, migrate to composition functions
    before upgrading." — crossplane/docs v2.0 `guides/upgrade-to-crossplane-v2.md`
  - "Native patch and transform within composition (`mode: Resources`)" listed as removed —
    crossplane v2.0.0 release
  - E1's own semantics: "spec.resources (and spec.patchSets) are no longer part of the v2 Composition
    schema, so the patch-and-transform templates of an unmigrated Composition are dropped" →
    `exposedClass: action-required`.
- **Classification: (b) — matcher scope, not label.** E3's label (`not-affected`) is right for E3's
  subjects: the env sets no `publishConnectionDetailsWithStoreConfigRef` and no StoreConfig. E1's
  label (`action-required`) is right too.
- **For groundtruth (do not edit here):**
  1. Anchor E3's matcher to the StoreConfig kind rather than any substring, e.g.
     `(?i)\bStoreConfig\b`, which does not match inside
     `publishConnectionDetailsWithStoreConfigRef`. The upstream quotes for E3 name the StoreConfig API.
  2. Give E1 matchers for its own semantic subjects (`subject`/text on `spec.mode` and
     `spec.resources`), justified by E1's cited CRD quotes ("All Compositions should use Pipeline mode.
     Resources mode is deprecated.", "default: Pipeline").
  3. Note that the evaluator attributes a change to the **last** matching item (`expIDForChange`), so
     (2) without (1) would still score against E3.

### 1b. kyverno-1.12-1.13 — `imp-9aaa41cb6bb5` · `impact:values-removed` · high

- **Change:** `chg-ac2801895465`, computed `values:section-removed` "Helm values section
  `cleanupJobs.*` removed (138 keys)". It is matched by E5 (`(?i)bitnami/kubectl`, incidental: the
  removed keys' defaults mention the image) and by E9 (`(?i)cleanupJobs`). Last-wins attributes it to
  **E9, relevance review**.
- **Env fact:** `values.yaml` L44–54 sets `cleanupJobs.{ephemeralReports,clusterEphemeralReports}.
  {schedule,threshold}` and `cleanupJobs.updateRequests.{enabled,threshold}`, "tuned after an etcd
  size incident on v1.12.1" (fixture comment).
- **Upstream:**
  - "Removed cleanupJobs keys from Helm chart (#11242)" — kyverno v1.13.0 release notes
  - "1.12.5 adds circuit breakers for updaterequests and ephemeralreports, we can remove these cleanup
    cronjobs in 1.13.0." — kyverno PR #10760
  - The v1.13.0 chart (`charts/kyverno` at v1.13.0) ships **no `values.schema.json`** (verified via the
    GitHub contents API: `.helmignore, Chart.yaml, README.md, README.md.gotmpl, charts, ci, templates,
    values.yaml`). The keys are silently ignored, not rejected.
- **The two readings:**
  - Engine / contract: DESIGN §1.4 defines `setting-ignored` = "configured values stop being honoured
    (removed/renamed key, unserved field)" → **action-required**. IMPACT.md: removed key → "the user's
    value stops taking effect" → action-required · high. The user's 5000 threshold stops applying.
  - Dataset (E9 note and NOTES.md): "removed key whose function upstream replaced (circuit breakers),
    so behavior-change (REVIEW) rather than setting-ignored (ACTION)". NOTES.md also says: "Honest
    call: a reviewer could argue ACTION from loss of the configured 5000 threshold."
- **Classification: (b) — the label conflicts with the normative contract.** It is not an engine bug.
  No generic signal in the values diff distinguishes "replaced by another mechanism" from "simply gone",
  and a product-specific carve-out is forbidden. Making `values-removed` review-only would break
  correct ACTIONs elsewhere (cilium `bgp.*`, `containerRuntime.*`, karpenter `logConfig.*`, karpenter
  `settings.featureGates.drift: false`).
- **Decision needed (commander + groundtruth/contract):** either
  (i) relabel E9's link as action-required under DESIGN §1.4 `setting-ignored` (the user's configured
  bound is lost and must be re-expressed for the circuit breaker), or
  (ii) amend the contract with a consequence kind such as "superseded-by-upstream-mechanism" →
  REVIEW. That kind is knowledge-only, so the deterministic engine would still say ACTION until a
  verified fact refines it, and DESIGN currently forbids knowledge from downgrading a stronger
  deterministic class. Under (ii) the knowledge-free gate stays failing on this unit by construction.

## 2. All 15 ACTION findings (knowledge-free run)

"Attributed" = the expected item the evaluator scores the change against (last matching item).

| Case | Rule | Change | Env fact | Attributed (relevance) | Verdict |
|---|---|---|---|---|---|
| cilium-1.15-1.17 | values-removed | `chg-8d1c442dccb7` `bgp.*` section removed | values.yaml sets `bgp.enabled`, `bgp.announce.loadbalancerIP` | — (matcher gap, D1 of UNKNOWN-ANALYSIS) | justified (upstream names `bgp.enabled` as removed; case F3 expects it) |
| cilium-1.15-1.17 | values-removed | `chg-6f7cb18704dc` `containerRuntime.*` removed | `containerRuntime.integration` | — | justified (case F1 expects action-required) |
| cilium-1.16-1.17 | values-removed | `chg-8d1c442dccb7` | `bgp.enabled`, `bgp.announce.*` | — | justified |
| cilium-1.16-1.17--plant-edge | values-removed | `chg-f65c27b0d85b` `encryption.wireguard.userspaceFallback` removed | key set | E11 (action) | justified |
| crossplane-1.20-2.0 | **crd-enum-value-removed** | `chg-8d886563fa13` `spec.mode` enum drops `Resources` | Composition `xbuckets-aws-pt` `spec.mode = "Resources"` | — (E1 matcher gap) | justified (E1 quotes) |
| crossplane-1.20-2.0 | crd-field-removed | `chg-5937653bdbb9` | `spec.resources` on `xbuckets-aws-pt` | **E3 (not-affected)** | justified — **scored wrong** (§1a) |
| external-secrets-0.15-0.16 | **crd-enum-value-removed** | `chg-47de02df1a69` `engineVersion` drops `v1` | ExternalSecret `billing/billing-grafana-datasource` `engineVersion: v1` | E4 (action) | justified |
| external-secrets-0.15-0.16 | crd-version-removed | `chg-40c4904676dd` `external-secrets.io/v1alpha1` | SecretStore at v1alpha1 | E2 (action) | justified |
| flux-2.6-2.7 | crd-version-removed | `chg-d799c76e2d5b` `helm.toolkit.fluxcd.io/v2beta1` | HelmRelease at v2beta1 | E1 (action) | justified |
| flux-2.6-2.7 | crd-version-removed | `chg-91b04a39187c` `kustomize.toolkit.fluxcd.io/v1beta1` | Kustomization at v1beta1 | E1 (action) | justified |
| flux-2.6-2.7 | kubernetes-below-range | `chg-b1b92effeaaa` (≥ 1.32) | `--kubernetes 1.31` | E3 (action) | justified |
| karpenter-0.37.8-1.0.0 | values-removed | `chg-4d6b2184ebf1` `logConfig.*` removed | `logConfig.*` set | — (E7 matcher is env-var text) | justified (case NOTES: "values file sets exactly the dropped logConfig keys") |
| karpenter-0.37.8-1.0.0--ci-buildfarm | values-removed | `chg-c1d179b14c56` `settings.featureGates.drift` removed | `drift: false` (the user opts *out*; upstream: drift "cannot be disabled") | — | justified (the opt-out is lost) |
| kyverno-1.12-1.13 | values-removed | `chg-ac2801895465` `cleanupJobs.*` removed | 6 tuned keys | **E9 (review)** | contract says ACTION — **scored wrong** (§1b) |
| strimzi-0.45-0.46--edge-retail | crd-removed | `chg-45b1663f05f1` `kafkamirrormakers.kafka.strimzi.io` | `KafkaMirrorMaker` `legacy-price-feed` in use | — | justified (case E2 link action-required) |

Seven of the 13 justified ACTIONs are unattributed, because item matchers do not select the computed
changes that carry them (the measurement artefact named in UNKNOWN-ANALYSIS D1). They are correct
findings, invisible to applicability scoring.

## 3. New join rules (applicability lane) — what they produced

| Rule | Findings (class) | ACTIONs justified? |
|---|---|---|
| `impact:crd-enum-value-removed` | 2 action-required | yes, both (§2) |
| `impact:crd-storage-migration` | 12 review-required | n/a, no ACTION. REVIEW is the contract class for a storage move (v1beta1 still served) |
| `impact:crd-default-applies` / `-pinned` | 2 review / 1 informational | n/a |
| `impact:crd-field-now-required`, `impact:crd-field-type-changed` | 0 | n/a |
| `impact:crd-attribute-clear` | 34 not-affected | the 4 ACTION-labelled ones (external-secrets E4 × 3, sibling kinds the env does not use) were checked and are correct |
| operand support sets (`impact:platform-*`) | 1 informational (in range) | n/a, no ACTION |

## 4. Engine fixes on this branch (generic; each with a regression test)

1. **`c804c91` — `crd-field-removed` why-block names only the exposed resources.** A removed list
   field arrives with every removed sub-path as a subject. Each sub-path related to the same set list
   leaf, so the explanation repeated "spec.resources (L18)" once per sub-path (×62 in crossplane). It
   also named every resource of the GVK, including the converted Composition that does not set the
   field. The evidence chain was right, but the prose blamed an unexposed resource. Test:
   `TestCRDFieldRemovedWhyNamesOnlyTheExposedResources`. Classes and metrics are unchanged.
2. **`3b42c1e` — removed CRD paths below a set list are decided element by element** (design and tests
   by the Claude agent; the GLM handoff agent finished it and added the CRD-definition-document
   exemption, which I reviewed and kept). Flattened manifest paths stop at sequences, so a removed
   `spec.provider.fake.data[].valueMap` below a set `spec.provider.fake.data` could only yield
   "section above" → review, on a link labelled not-affected. The full-depth resource facts now decide
   it:
   - any element sets the removed path → action-required, exactly like an exact match;
   - no element sets it → `crd-field-unset`, not-affected with an evaluation record.

   It refuses to decide (keeps the review) on partial manifests, an uncovered usage document, or an
   oversize-withheld subtree. Test: `TestCRDFieldRemovedBelowSetListIsDecidedPerElement` (4 subtests,
   including the partial-inventory refusal and the CRD-supplied shape). Effect: external-secrets E5
   becomes a clean not-affected link; there are no new ACTIONs in the dataset.

`go build ./... && go vet ./... && go test ./...` green; full eval: no regression against
`eval/results`, recall unchanged.

## 5. The dangerous direction: catastrophic ACTION → NOT-AFFECTED cells (26)

These cells are not part of falseActionRate, but they carry ×10 weight and decide whether NOT AFFECTED
can be trusted. Breakdown (per change × item):

| Group | Cells | Reading |
|---|---|---|
| Sibling kinds/versions the env does not use (flux v1beta1 × 10, external-secrets E2 × 1, E4 × 3) | 14 | per-change verdict **correct**. Item-level labels cover a family; the env uses only some members, and those get ACTION |
| Matcher artefacts (cilium `bgpControlPlane` new key × 2, istio `defaults.*` × 1) | 3 | correct per change. These are UNKNOWN-ANALYSIS §2.2, unchanged |
| **values default-changed / added, key unset → `impact:values-unset` not-affected** (kyverno E1: `*.rbac.coreClusterRole.extraResources` defaults × 3 and the new `*Controller.rbac.createViewRoleBinding`/`viewRoleName` keys × 3, the view-role replacement that ships enabled; kyverno E5: `policyReportsCleanup.image.tag` / `webhooksCleanup.image.tag` × 2) | 8 | **engine semantics wrong**: see below |
| Image mirror under another registry (kyverno E5 `images:removed bitnami/kubectl:1.28.5` vs env `registry.corp.example/bitnami/kubectl:1.28.5` → `image-not-referenced`) | 1 | repository equality is registry-exact; a mirrored copy reads as "not referenced" |

**The values-unset bug.** For `values:default-changed` (and `values:added`), "your values do not set
the key" is reported as "nothing about this change takes effect on you" → NOT AFFECTED. For a default
change that is backwards: the user who does not pin the key is exactly the one the new default
reaches. The CRD equivalent added this round already does it right (`impact:crd-default-applies` →
review). Kyverno shows the cost: the background controller silently loses its Secret write grant
(E1, action-required), and the post-upgrade hook pulls an unmirrored image tag (E5, action-required).
Both are shown as checked-and-clear.

**Proposed fix (not applied; needs a contract decision):** `default-changed`/`added` + key unset →
**never NOT AFFECTED**. Either UNKNOWN (`release-knowledge-gap`: "the new default applies to you;
whether it matters is not determinable from the diff"), or REVIEW like `crd-default-applies`. Measured
blast radius on the dataset: 73 `default-changed` and 251 `added` verdicts are `values-unset` today.
Most are routine image-tag/digest bumps of the product's own images, so REVIEW would bury the signal.
UNKNOWN is the honest knowledge-free class, and verified facts (or the rendered-diff validator) can
lift the ones that matter. Expect unknownRate to rise: 0.775 → 0.803 if only `default-changed` moves, → 0.899 if `added` moves too and some
not-affected item classifications to shift to unknown.

**Mirror matching (proposed, not applied):** when the registry differs but the repository path
matches (`bitnami/kubectl`), report a possible mirror at REVIEW at most. Never claim not-referenced.

## 6. Open decisions for the commander

1. Kyverno E9: relabel to action (contract-consistent) **or** amend the contract (§1b).
2. Crossplane E3 matcher narrowing + E1 matchers (§1a), for groundtruth with upstream justification.
3. Values `default-changed`/`added` + unset → UNKNOWN (or REVIEW) instead of NOT AFFECTED (§5). This
   is the biggest remaining trust hole in the knowledge-free engine.
4. Registry-agnostic mirror matching for images (§5).
