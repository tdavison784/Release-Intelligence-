# Research notes: Kyverno v1.12.6 → v1.13.0 (with environment)

Researched blind on 2026-10-01. No Release Intelligence output was consulted:
no `ri` runs, nothing under `eval/results/`, `eval/adjudications/` or any testdata.

## Endpoints

- `to: v1.13.0` (published 2024-10-29).
- `from: v1.12.6` (2024-09-27), the last 1.12 patch released before v1.13.0, and
  what a cluster upgrading at that time ran. v1.12.7 (2025-01-17) came later and
  backported 1.13 fixes such as the mutate-existing managed-by label change (#11267),
  so it would muddy the edge. Both tags match the definition's `tagPattern`. The
  chart at the tags is kyverno 3.2.7 (appVersion v1.12.6) and 3.3.0 (appVersion v1.13.0).

## Sources read

- **Upgrade guide** (website `main`, `src/content/docs/docs/installation/upgrading.md`,
  section "Upgrading to Kyverno v1.13"). This is the definition's `upgrade-guide`
  channel. It has two numbered breaking changes (wildcard view removal → E1; default
  exception settings → E2) and "Dropped API versions" (→ E7).
- **Release blog** (website `main`, `src/content/blog/announcing-kyverno-release-1.13/index.md`).
  This is the `release-notes-blog` channel. "Security Hardening" restates E1/E2, and
  "API Version Management" gives the deprecations (E3, E4) and the v2 graduation (E6).
- **GitHub release body v1.13.0**. This is the `release-notes-github` channel. It has a
  curated "Breaking Changes" header (E1, E2) and "Changed" lines for E3/E4 and
  E9 (cleanupJobs, cleanup cronjobs, reports chunking). "Updated default metrics in the
  Helm chart (#10459)" leads to E8. "chore: bump bitnami/kubectl to 1.30.2 (#10496)"
  leads to E5.
- **Chart values / Chart.yaml at v1.12.6 and v1.13.0** (`charts/kyverno/`), diffed in
  full:
  - The wildcard `get/list/watch` entry was removed from `admissionController`,
    `backgroundController` and `reportsController` `.rbac.coreClusterRole.extraResources`.
    The background controller's core role also lost `secrets` from its core-group
    create/update/patch/delete entry.
  - `*.rbac.createViewRoleBinding: true` and `viewRoleName: view` were added.
  - `features.policyExceptions.enabled` changed from `true` to `false`.
  - `metricsConfig.metricsExposure` changed from `~` to a map with
    `disabledLabelDimensions` on six metrics.
  - The whole `cleanupJobs` block was removed.
  - `features.reports.chunkSize` was removed (Chart.yaml artifacthub change).
  - The `webhooksCleanup` / `policyReportsCleanup` image tag changed from `1.28.5` to
    `1.30.2`.
  - `kubeVersion` is `>=1.25.0-0` at both tags.
- **CRDs at both tags** (`config/crds/kyverno/`), parsed per version:
  - The `ClusterPolicy` v1 descriptions now read "Deprecated, use … under the validate /
    generate / mutate rule / webhookConfiguration instead".
  - The rule-level `validate.failureAction` (enum `Audit|Enforce` only, case-sensitive),
    `validate.failureActionOverrides`, `generate.generateExisting` and
    `mutate.mutateExistingOnPolicyUpdate` are **new in 1.13**. They do not exist at
    v1.12.6.
  - PolicyException and (Cluster)CleanupPolicy storage moved from v2beta1 to v2, and
    v2beta1 is now `deprecated: true` but still served.
  - UpdateRequest storage moved from v1beta1 to v2.
  - The unserved v2alpha1 versions were dropped.
  - The AdmissionReport / BackgroundScanReport CRDs (and their cluster variants) were
    removed.
- **Code at both tags**:
  - `cmd/internal/flag.go`: the `--enablePolicyException` default changed from true to
    false. The `--exceptionNamespace` help now says "If it is set to '*', exceptions are
    allowed in all namespaces."
  - `cmd/internal/engine.go`: at v1.12.6 an empty namespace meant the cluster-wide
    lister. At v1.13.0 it logs "the flag --exceptionNamespace cannot be empty" and
    returns **no** exception selector. This is the basis for E2's namespace subject.
  - Chart `templates/hooks/`: post-upgrade-migrate-resources (gated by
    `crds.migration.enabled` and `not templating.enabled`) and
    post-upgrade-clean-reports (gated by `policyReportsCleanup.enabled`, image
    `policyReportsCleanup.image`).
- **PRs**:
  - #10785 (wildcard removal) explicitly names "custom resources, or … security related
    resources" and gives the `coreClusterRole` restore snippet. The upgrade guide's
    snippet uses `clusterRole` instead, and both keys exist.
  - #10459 (metrics defaults).
  - #10760 (cleanup cronjobs removed because "1.12.5 adds circuit breakers").
  - #11242 (cleanupJobs keys).
  - #10496 (kubectl bump).
  - #10515 (webhookTimeoutSeconds/failurePolicy deprecation; considered, see below).
- **Customization docs** (website `main`, RBAC section): the view RoleBindings, the
  aggregation labels, and the statement that default permissions cover "security
  non-critical resources".
- **Kubernetes RBAC docs**: the `view` role "does not allow viewing Secrets". Together
  with the chart diff, this proves that Secret cloning is exposed.
- **Website `release-1-13-0` branch**, `content/en/docs/installation/_index.md`: the
  compatibility matrix (1.12.x: 1.26–1.29, 1.13.x: 1.28–1.31). The `main` matrix no
  longer lists these lines.
- GHSA-qjvc-p88j-j9rm (CVE-2024-48921), summary "PolicyException objects can be created
  in any namespace by default", severity medium.

## Items and G22 categories

| Item | What | Class (this env) | G22 category |
|---|---|---|---|
| E1 | Wildcard view removed and view RoleBinding added; background controller loses its Secret write grant | action-required | RBAC (the Secret part is only in the values diff) |
| E2 | Exceptions off by default; empty namespace no longer means all namespaces | action-required | prose-only/changed default + container flag + security |
| E3 | `spec.validationFailureAction(Overrides)` deprecated, replaced by per-rule `failureAction(Overrides)` | review-required | deprecation with replacedBy (crd-field) |
| E4 | `spec.generateExisting` / `spec.mutateExistingOnPolicyUpdate` deprecated, replaced by rule level | not-affected (generic: review-required) | deprecation with replacedBy (crd-field) |
| E5 | Hook image bitnami/kubectl 1.28.5 → 1.30.2 | action-required | artifact / hidden in a "chore: bump" line |
| E6 | PolicyException / CleanupPolicy v2beta1 deprecated, replaced by v2 | review-required | API deprecation (gvk) |
| E7 | CRD storage-version migration (automatic via Helm hook) | not-affected (generic: action-required) | migration |
| E8 | Metrics label dimensions dropped by default | review-required | changed default behind vague prose |
| E9 | `cleanupJobs` and `features.reports.chunkSize` removed | review-required | Helm values removed |
| E10 | Tested Kubernetes range 1.28–1.31 | informational (generic: review-required) | compatibility hidden in prose |

Per the lane's convention correction, each linked item's `classification` is the
class for **this environment's** link. The generic reading is in a YAML comment
and in `semantics.consequence.exposedClass`.

## Judgement calls

- **E1 is action-required here, not review.** The upgrade guide says "may impact".
  Exposure becomes certain because of the chart diff: the background controller's core
  role loses `secrets` create/update/patch/delete, and the `view` role never covered
  Secrets. The environment's `sync-registry-credentials` clones and syncs a Secret, sets
  no `*.rbac.*` values, and has no aggregated ClusterRole. So regcred silently stops
  appearing in new namespaces. The secrets subject is a fourth subject on E1, not a
  separate item, because upstream presents one change (#10785).
- **E2 is the "pin looks safe but isn't" case.** The values pin
  `features.policyExceptions.enabled: true`. The canonical default-changed overlap
  (values-key set, so informational) would wrongly clear it. The deciding fact is the
  empty `namespace`, verified in v1.13.0 `engine.go`. The exposure is therefore an `any`
  over enabled unset/false and namespace unset/"". I chose `setting-ignored`
  (PolicyExceptions stop being honoured) over `resource-rejected`. Rejection only
  happens for exempted resources under Enforce policies. Here both exceptions target
  Enforce policies, and the statement says so.
- **E3 has no "already on the per-rule field" policy.** The brief suggested one, but
  the per-rule fields do not exist in the v1.12.6 CRD: a structural schema would prune
  them, so a 1.12.6 cluster cannot hold them. The not-affected deprecation direction is
  covered by E4 instead. That cluster also has generate policies, the natural home of
  `generateExisting`, but none sets the policy-level fields. I noted in E3's
  remediation that the per-rule enum is case-sensitive (`Audit|Enforce`), while the
  policy-level enum also accepted lowercase. This bites a mechanical migration of
  `enforce` / `audit`.
- **E5 replaced a webhookTimeoutSeconds/failurePolicy deprecation item.** That item
  (#10515: "deprecates both `spec.webhookTimeoutSeconds` and `spec.failurePolicy` …
  under `spec.webhookConfiguration`") was dropped to stay within 10 items. It was the
  same shape as E3/E4, while E5 adds an artifact/upgrade-blocked item. The
  `disallow-privileged-containers` policy still sets both fields, so a tool that reports
  that deprecation is correct and should not be counted as a false positive. It is
  simply not labelled.
- **E5 is `upgrade-blocked`.** `policyReportsCleanup.enabled` defaults to true, and its
  Job is a `post-upgrade` hook. With every image pulled from the mirror
  (`global.image.registry`) and only `bitnami/kubectl:1.28.5` mirrored, the hook pod
  cannot pull 1.30.2 and `helm upgrade` fails on the hook. Upstream prose is a
  "chore: bump" line, which the definition's GitHub classifier skips, so the
  values-path subject matchers are the realistic way to find it.
- **E7 is not-affected, not informational.** The item's subject is the manual
  storage-version migration. The environment upgrades with Helm and leaves
  `crds.migration.enabled` / `templating.enabled` at their defaults, so the hook does
  the work and the exposure (hook disabled) is false. There is no overlap condition,
  because nothing in the environment touches the manual step.
- **E8 is behaviour-change (review).** Metrics still flow, but the default now drops
  `resource_namespace` from `kyverno_policy_results_total`, and the PrometheusRule
  groups by it. The v1.13.0 values comment still says "by default all metrics and all
  labels are exported", which is stale and contradicts the new default. I trusted the
  value, not the comment.
- **E9 is review, not setting-ignored (action).** The keys are removed and silently
  ignored, but upstream replaced their function: "1.12.5 adds circuit breakers for
  updaterequests and ephemeralreports". AdmissionReports are gone entirely. Nothing
  breaks if the operator does nothing; they should verify the circuit-breaker
  thresholds. Honest call: a reviewer could argue ACTION from loss of the configured
  5000 threshold.
- **E10 is outside the ingested channels.** The tested range lives in the per-release
  website docs, not in the blog, release body or upgrade guide, and the chart
  `kubeVersion` did not change. It is kept as a G22 "compatibility hidden in prose"
  probe and will likely be a recall miss today. For 1.29 the link is informational
  (exposure out-of-range is false, overlap in-range is true), per
  ACTION_CLASSIFICATION ("a cluster version inside the supported range").
  `behavior-change` is used for clusters outside the range because the matrix is a
  tested range ("no guarantees"), not a hard install gate.
- I considered and did not label the following:
  - #10033 (updates to pre-existing violating resources allowed): a fix restoring
    documented behaviour.
  - New features such as `--enableReporting`, reports for mutate/generate, and
    `emitWarning`.
  - Removed AdmissionReport/BackgroundScanReport CRDs: internal intermediate types, and
    the environment does not use them.

## Environment grounding

- `values.yaml` uses only keys present in `charts/kyverno/values.yaml` at v1.12.6.
  The rbac, metricsExposure, crds and templating keys are deliberately absent.
- The policies use only fields served by the v1.12.6 ClusterPolicy v1 schema.
  PolicyExceptions use `kyverno.io/v2beta1`, the v1.12.6 storage version.
- Exception rule names include the `autogen-` rule produced for DaemonSets.
- The PrometheusRule uses real `kyverno_policy_results_total` labels
  (`rule_result`, `policy_name`, `resource_namespace`).
- `images.txt` is the mirror list. Image refs carry the mirror prefix because
  the chart's `kyverno.image` helper renders `<global.image.registry>/<repository>:<tag>`.
- `inventory.yaml` lists only Kyverno v1.12.6 and kube-prometheus-stack (version not
  stated).
- `expectedFindings` F1 is grounded in the values diff: the pinned key's default flips
  true→false.

## Semantic labels

- E1 subjects are `rbac-permission` with `name: '*'`, `group: '*'` and the controller
  as `component`, one per controller. The Secret grant is
  `{name: secrets, component: background-controller}` (core group, so `group` is
  omitted). The verbs (get/list/watch; create/update/patch/delete) are in the
  statements, because the family has no verb field.
- E2 bundles three subjects:
  - the Helm value (`default-changed true→false`);
  - the container flag `--enablePolicyException` (same change, the non-Helm view);
  - `features.policyExceptions.namespace` (`behavior-changed`, before `""`).

  The namespace subject has no `after`: the new meaning of `""` is "no exceptions",
  which is not a value.
- E3/E4 use `crd-field` with `replacedBy` of the same family. The `[]` marks list
  elements (`spec.rules[].validate.failureAction`). Only ClusterPolicy is labelled. The
  namespaced `Policy` kind carries the same deprecations, but the environment has none.
- E6 uses `gvk` deprecated with `replacedBy` gvk v2, one subject per kind.
- E7 uses the `migration` family (`crd-storage-version-migration`) with
  `migration-required`, which the validator pairs with that family.
- E8's `after` is abbreviated to the `kyverno_policy_results_total` entry (see the
  label note). The full v1.13.0 default covers six metrics.
- E10's `before`/`after` are bare semver constraints (`>=1.26.0, <1.30.0` →
  `>=1.28.0, <1.32.0`) encoding the matrix's min/max minors.
- Link conditions:
  - E1 uses a `not(resource ClusterRole …)` negation for "no aggregated role grants
    secrets". `manifests/` is declared complete for Kyverno-related RBAC in the
    description, so the negation is decided by examined manifests, not by absence of
    input.
  - E4, E5 and E7 are environment-wide `values-key` / `image-in-use` leaves, or
    resource-scoped `field` leaves, never mixed inside one scope.
- No `undecidedImpact` links. Every link is decidable from the fixture as written.

## 2026-10-02 relabel (groundtruth-4, motivated by pipeline output: render-4)

- E7 not-affected → informational: the cluster stores kyverno.io/v2beta1 PolicyExceptions (a version 1.13
  drops) and is shielded by the Helm post-upgrade migrate-resources hook (DESIGN.md §1.3 overlap). The hook
  runs only once E5's kubectl image is mirrored. See eval/CHANGELOG.md "2026-10-02 (d)".
