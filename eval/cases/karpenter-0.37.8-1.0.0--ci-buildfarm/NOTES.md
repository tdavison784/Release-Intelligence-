# Research notes — karpenter v0.37.8 → v1.0.0, transfer environment `ci-buildfarm`

Transfer case for `karpenter-0.37.8-1.0.0`. Authored blind on 2026-10-01 from
the upstream documents below (release-level items and their `semantics` are
inherited from the base case and were not re-derived or edited). The cluster
was designed from the v1 upgrade notes first, deliberately different from the
base case's `prod-cluster`, so the links come out differently.

## Cluster design

`ci-buildfarm` is a CI/build EKS cluster: GPU build runners on Karpenter
v0.37.8 (Helm chart, controller image pinned 0.37.8), Kubernetes 1.30.
Differences from the base environment and why:

- **One GPU NodePool, consolidation left at default.** The base pool spells
  `consolidationPolicy: WhenUnderutilized`; this pool leaves it unset (v1beta1
  default `WhenUnderutilized`, per the v0.37.8 CRD) and drives rotation with
  `expireAfter: 336h`. That flips E2 to not-affected and makes E5 bite through
  the two other required-field channels (`consolidateAfter`, `nodeClassRef`).
- **AL2023, not AL2/Ubuntu** → E8 clearly clear.
- **No `metadataOptions` on the EC2NodeClass** → E6 exposed (default hop-limit
  change) rather than shielded.
- **Drift explicitly opted out in values** (`settings.featureGates.drift:
  false`): with AMIs selected by name glob, drift rolled the GPU fleet on
  every upstream AMI release, so the team rotates nodes on expiry instead.
  This exercises E7's feature-gate channel (base exercised the dropped
  `logConfig` keys) at review level.
- **Workloads show both disruption-protection idioms**: the orchestrator
  already uses the replacement `karpenter.sh/do-not-disrupt` annotation
  (E3 clear), while the log-collector DaemonSet tolerates the *old*
  disruption taint `karpenter.sh/disruption=disrupting:NoSchedule` (E4
  affected — the rename strands the toleration).
- **Minimal values file**: no `logConfig`, no `assumeRole*`, no metrics-port
  pin — the base's E7 values-channel exposure is deliberately absent.

## Sources read (all quoted verbatim below)

- upgrade-guide.md at v1.0.0, "Upgrading to `1.0.0`+" changelog.
- v1-migration.md at v1.0.0: procedure, "Changes required before upgrading to
  v1.0.0" (metadataOptions warning, Ubuntu, annotations), "Before upgrading
  to v1.1.0" (v1beta1 support gone), K8s floor ("Validate that you are running
  at least Kubernetes 1.25").
- Chart values at v0.37.8 and v1.0.0 (diffed): `settings.featureGates.drift`
  exists at 0.37.8 ("drift is in BETA and is enabled by default. / Setting
  drift to false disables the drift disruption method") and the `drift` key is
  gone at 1.0.0; `logConfig` block and `settings.assumeRoleARN` /
  `settings.assumeRoleDuration` exist at 0.37.8 and are gone at 1.0.0.
- CRDs at v0.37.8 (karpenter-crd chart templates; the v1 version they carry is
  gated behind `webhook.enabled`, i.e. the conversion-webhook preview shipped
  in the 0.37.x patch line): v1beta1 `consolidationPolicy` enum
  `[WhenEmpty, WhenUnderutilized]`, default `WhenUnderutilized`, CEL
  "consolidateAfter cannot be combined with consolidationPolicy=WhenUnderutilized";
  v1beta1 `nodeClassRef` = `{apiVersion, kind, name}` with only `name`
  required, vs the gated v1 `{group, kind, name}` all required; v1beta1
  EC2NodeClass `metadataOptions` default `httpPutResponseHopLimit: 2` vs the
  gated v1 default `1`; v1beta1 `amiFamily` enum includes `Ubuntu`, gated v1
  enum drops it; v1beta1 AMISelectorTerm has `id/name/owner/tags` only (the
  `alias` field is v1-only: "Alias specifies which EKS optimized AMI to
  select"); v1beta1 EC2NodeClass spec requires `amiFamily` (amiSelectorTerms
  becomes required in v1).
- disruption.md at v0.37.8: "You can block Karpenter from voluntarily
  choosing to disrupt certain pods by setting the
  `karpenter.sh/do-not-disrupt: \"true\"` annotation on the pod." (grounds the
  replacement annotation in the fixture).
- getting-started step12-add-nodepool.sh at v0.37.8: the documented v1beta1
  shape copied for the fixture — `nodeClassRef: {apiVersion:
  karpenter.k8s.aws/v1beta1, kind: EC2NodeClass, name: default}`,
  `disruption: {consolidationPolicy: WhenUnderutilized, expireAfter: 720h}`,
  and AMI name-glob selection (`- name: "amazon-eks-node-${K8S_VERSION}-*"`
  "automatically upgrade when a new AL2 EKS Optimized AMI is released. This is
  unsafe for production workloads.").
- compatibility.md at v1.0.0: matrix ends at karpenter `0.37.0` → Kubernetes
  `1.30`; the v1.0.0 file carries no 1.0.x row (see "base labels" below).
  Kubernetes 1.30 is within what 0.37.x supports and above the migration doc's
  1.25 floor.

## Link judgements

- **E1 action-required** — fixtures are v1beta1 (`karpenter.sh/v1beta1`
  NodePool, `karpenter.k8s.aws/v1beta1` EC2NodeClass). Guide: "Karpenter will
  stop serving the v1beta1 API version at v1.1.0 … Migrate all stored
  manifests to v1 API versions on Karpenter v1.0+." True for every 0.37.8
  cluster with Karpenter resources; the transfer value is that it still holds
  in a differently shaped fleet.
- **E2 not-affected** — the pool never names `WhenUnderutilized`. The rename
  ("API Rename: NodePool's ConsolidationPolicy `WhenUnderutilized` is now
  renamed to `WhenEmptyOrUnderutilized`") only costs work where the removed
  value is spelled out; the v1beta1 CRD default `WhenUnderutilized` maps to
  the v1 CRD default `WhenEmptyOrUnderutilized`, same behavior. Exposure is
  the canonical validation-tightened shape (`field set`) narrowed to the exact
  renamed value (`equals "WhenUnderutilized"`): the other v1beta1 value
  `WhenEmpty` survives unchanged in the v1 enum, so only the spelled-out
  renamed value exposes.
- **E3 not-affected** — the orchestrator pod template carries
  `karpenter.sh/do-not-disrupt: "true"` (the replacement, per disruption.md at
  0.37.8), and neither workload template carries `karpenter.sh/do-not-evict`
  or `karpenter.sh/do-not-consolidate` (removed per the guide: "`karpenter.sh/do-not-consolidate`
  (annotation), `karpenter.sh/do-not-evict` (annotation), and
  `karpenter.sh/managed-by` (tag) are all removed."). Exposure asks for the
  removed annotations on the workload kinds present in the repo; all four
  resource scopes evaluate false (checked and clear).
- **E4 action-required** — the DaemonSet tolerates exactly the old taint.
  Guide: "The taint used to mark nodes for disruption and termination changed
  from `karpenter.sh/disruption=disrupting:NoSchedule` to
  `karpenter.sh/disrupted:NoSchedule`. It is not recommended to tolerate this
  taint, however, if you were tolerating it in your applications, you'll need
  to adjust your taints to reflect this." Exposure matches the toleration key
  on the pod template (`spec.template.spec.tolerations[].key`).
- **E5 action-required** — two channels hit at once: (a) `consolidateAfter`
  unset — guide: "API: ConsolidateAfter is required. Users couldn't set this
  before with ConsolidationPolicy: WhenUnderutilized, where this is now
  required. Users can set it to 0 to have the same behavior as in v1beta1."
  The v1beta1 CEL ("consolidateAfter cannot be combined with
  consolidationPolicy=WhenUnderutilized") shows why a default-policy pool like
  this one *could not* have set it; (b) `nodeClassRef` in the v1beta1 spelling
  `{apiVersion, kind, name}` with no `group` — guide: "API: All
  `NodeClassRef` fields are now all required, and apiVersion has been renamed
  to group". The `amiSelectorTerms` channel of E5 is not exposed (the class
  sets them).
- **E6 review** — class sets no `metadataOptions`; v1beta1 CRD default
  `httpPutResponseHopLimit: 2`, gated-v1 default `1`; guide: "API: Karpenter
  will drop support for IMDS access from containers by default on new
  EC2NodeClasses by updating the default of `httpPutResponseHopLimit` from 2
  to 1."; migration doc: "If you have workload pods that are not using
  `hostNetworking`, the updated default `metadataOptions` could cause your
  containers to break when you apply new EC2NodeClasses on v1." The CI pods
  are non-hostNetwork, so this is the warned case → review, matching the
  consequence's exposedClass (not action: existing nodes keep working; the
  operator decides whether new classes need hop limit 2).
- **E7 review** — drift opted out in values. Guide: "`FEATURE_GATES.DRIFT=true`
  was dropped and promoted to Stable, and cannot be disabled. / Users
  currently opting out of drift, disabling the drift feature flag will no
  longer be able to do so." Chart grounding: 0.37.8 values carry
  `settings.featureGates.drift` ("Setting drift to false disables the drift
  disruption method"); 1.0.0 values carry no `drift` key. Drift disruption
  silently returns and can never be turned off — the Drift subject's
  consequence (`behavior-change`, review-required) decides the link's class;
  the dropped logging/assumeRole subjects evaluate clear (values set none of
  them). Exposure is modelled in the subject's family: `feature-gate Drift
  disabled`, with `path` naming the chart key. NOTE: this chart expresses
  gates as `settings.featureGates.<gate>` booleans rather than a
  `--feature-gates` token list, so the path points at the boolean key.
- **E8 not-affected** — `amiFamily: AL2023`. Guide: "Breaking API (Manual
  Migration Needed): Ubuntu is dropped as a first class supported AMI
  Family". The gated-v1 CRD enum at 0.37.8 (`AL2, AL2023, Bottlerocket,
  Custom, Windows2019, Windows2022`) confirms AL2023 survives. Exposure is
  the canonical `field set` narrowed to `equals "Ubuntu"` — the only value
  the item drops.
- **E9 undecided (runtime-behavior-gap)** — `expireAfter: 336h` puts forceful
  expiration in active use. Guide: "Expiration is now forceful and begins
  draining as soon as it's expired. Karpenter does not wait for replacement
  capacity to be available before draining, but will start provisioning a
  replacement as soon as the node is expired and begins draining." Whether
  that harms this cluster (in-flight builds lost, g-capacity contention at
  peak) or is harmless (reschedulable CI pods) is a runtime property no
  static fixture can decide, so the link is recorded undecided with what
  would decide it.
- **F1 finding** — mirror list pins `public.ecr.aws/karpenter/controller:0.37.8`;
  the tag moves to 1.0.0 with the upgrade (image-changed), same channel as
  the base case's F3.

## Observations on the base labels (not changed)

1. **F4 grounding**: the base's compatibility finding says "the matrix
   declares a minimum for 1.0.x", but `compatibility.yaml` (and the generated
   compatibility page) at v1.0.0 has no 1.0.x row — it ends at `0.37.0` /
   K8s 1.30. The substance (K8s 1.29 is fine for 1.0.0) is still supported by
   the v1-migration doc's floor ("Validate that you are running at least
   Kubernetes 1.25"), but the cited artifact does not contain the claimed row.
2. **E7 helm-value subject paths**: labelled as top-level `logConfig`,
   `assumeRoleARN`, `assumeRoleDuration`, but in the 0.37.8 chart the latter
   two live under `settings.` (`settings.assumeRoleARN`,
   `settings.assumeRoleDuration`); the 1.0.0 chart drops them from `settings`.
   Spelling drift only — the removal itself is correct.
3. **E2 change type** `validation-tightened` fits the spelled-out value, but
   note the default is *renamed* too (WhenUnderutilized →
   WhenEmptyOrUnderutilized) with identical behavior; the label's
   before/after values capture that correctly.

## E3/E4 resource group fixes (2026-10-02, groundtruth-6; LOOP-DIAGNOSIS-2 §8.2)

`resource Deployment/DaemonSet` without `group` reads the *core* API group,
where those kinds do not exist — E4's would-be-affected condition could never
fire on any fixture (it evaluated false from never looking), and E3's four
leaves had the same latent bug (its false was labelled-consistent only by
accident). Both now carry `group: apps`. Labels unchanged; verified with the
engine (E4 true, E3 false, both as labelled). See eval/CHANGELOG.md
"2026-10-02 (f)".
