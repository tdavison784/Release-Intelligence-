# Research notes: external-secrets v0.15.0 → v0.16.0 (with environment)

Researched blind (no Release Intelligence output, results, adjudications or
testdata consulted) on 2026-10-01, from GitHub release bodies, the repository
at the two tags and the two chart tags, and upstream issues/PRs.

## Why this transition

Surveyed the x.y.0 release bodies from v0.10.0 to v2.0.0. v0.10.0 does not
remove v1alpha1 (its breaking change is the webhook provider/generator label).
v0.16.0 is the release that removes `v1alpha1` (ExternalSecret, SecretStore,
ClusterSecretStore), removes the conversion webhooks, promotes `v1`, removes
the v1 template engine and the fake provider's ValueMap, and ships a
pre-upgrade storedVersions migration. v0.17.0 only unserves v1beta1 (plus the
Kubernetes 1.32→1.33 row), so 0.15→0.16 carries far more operator work. The
pipeline's `lineage: minor` makes the edge v0.15.0..v0.16.0 (it contains
v0.15.1, whose release body is images only).

## Sources read

- v0.16.0 release body (https://github.com/external-secrets/external-secrets/releases/tag/v0.16.0):
  the "Guide to Promoting to 0.16" (v1alpha1 manifests, storedVersions check
  and patch), Helm vs separately-installed CRDs, troubleshooting (conversion
  webhook 404, `webhookClientConfig: Forbidden` → use 0.16.1), and the
  BREAKING CHANGES list. Source of E1–E5, E9.
- v0.15.0 / v0.15.1 / v0.17.0 release bodies: v0.15.1 is images only; v0.17.0
  confirms v1beta1 was still served in 0.16 (it "Stops serving `v1beta1`").
- Commit range v0.15.0...v0.16.0 (compare API): the commit subjects the
  commit-log fallback channel would ingest; surfaced #4596 (Vault auth), #4572
  (RBAC), #4571, #4561, #4638 (webhook annotations), #4635 (promote v1).
- Issue #4633 (promotion checklist: remove v1alpha1, remove conversion
  webhooks, remove Template/v1, remove ValueMap) and PR #4635.
- Issue #4662 (the upgrade tracking issue the release links): the conversion
  404 error text, the storedVersions "must appear in spec.versions" error, the
  maintainer's "specifically, are you setting `crds.conversion.enabled=true`?"
  and "if that config is kept, things would break even harder".
- CRD bundles at v0.15.0 / v0.16.0 / v0.16.1 (parsed): ExternalSecret,
  SecretStore, ClusterSecretStore lose v1alpha1 and gain v1 (storage moves
  v1beta1→v1; v1beta1 served, not deprecated); ClusterExternalSecret had only
  v1beta1 at v0.15.0 (so "cluster counterparts" adds nothing for it); the
  engineVersion enum is `[v1, v2]` → `[v2]`; `fake.data[].valueMap` disappears;
  the v0.16.0 bundle still has `conversion.strategy: Webhook`, v0.16.1 has
  none (E9). The CRD name set is identical (notExpectedFindings).
- Chart values at helm-chart-0.15.0 / 0.16.0 / 0.16.1: only three changes —
  `crds.conversion.enabled` true→false (new comment "Conversion is disabled by
  default as we stopped supporting v1alpha1."), new `openshiftFinalizers: true`,
  new `webhook.annotations: {}`; 0.16.0 and 0.16.1 values are identical. Every
  key in environment/values.yaml exists at helm-chart-0.15.0.
- Chart templates diff helm-chart-0.15.0..0.16.0: rbac.yaml gates
  clustersecretstores/clusterexternalsecrets/pushsecrets/clusterpushsecrets on
  the process* values (controller and aggregate-to-view/edit/admin roles);
  validatingwebhook.yaml moves to v1 paths. hack/helm.generate.sh at v0.16.0:
  the CRD `conversion:` block is rendered only `if .Values.crds.conversion.enabled`.
- Code at the tags: pkg/template/engine.go (v0.16.0 has only v2: "unsupported
  template engine version"), cmd/controller/webhook.go (v1alpha1 webhooks gone,
  webhooks registered for v1 only), pkg/provider/vault/auth_kubernetes.go
  (TokenRequest first, legacy Secret second, `iss` comment),
  apis/externalsecrets/v1beta1/secretstore_fake_types.go at v0.15.0 (ValueMap
  deprecation comment), cmd/controller/root.go + secretstore/client_manager.go
  (ClusterSecretStoreEnabled is passed to the ExternalSecret reconciler, so
  the RBAC narrowing in E8 matches what the controller already does).
- docs/introduction/stability-support.md at main (the pipeline's compatibility
  source): "0.16.x | 1.32" (same as 0.15.x). At the v0.16.0 tag the table only
  goes to 0.14.x (rows land after the tag, as products/external-secrets.yaml says).

## Judgement calls

- **Classification convention.** Per the groundtruth-lane correction, every
  linked item's `classification` equals its link in this environment
  (E1/E5/E8/E9 `not-affected`, E7 `review-required`, the rest
  `action-required`); the generic reading is kept as a YAML comment.
- **E1 vs E2 split.** The release bullet bundles all v1alpha1 kinds; split by
  kind so the fixture can be affected for SecretStore (payments-aws at
  v1alpha1) and not-affected for ExternalSecret (all v1beta1). E2 bundles
  SecretStore + ClusterSecretStore (two subjects, `any` exposure).
- **E3 is separate from E1.** The CRDs still record v1alpha1 in
  status.storedVersions even though no ExternalSecret manifest uses v1alpha1:
  E1 is not-affected, E3 is action-required. The failure statement uses the
  Kubernetes validation the issue shows ("status.storedVersions[...] must
  appear in spec.versions"); the quoted comment shows it for "v1" on a
  rollback, the same rule applies to "v1alpha1" on the upgrade. The crds/
  fixture is a trimmed `kubectl get crd -o yaml` export (schemas reduced to
  their root, so it is not a full CRD); storedVersions, versions and
  conversion are as a cluster first installed at v0.3.x would carry them.
  clusterexternalsecrets records only v1beta1 (that CRD never had v1alpha1).
- **E4** labelled `validation-tightened` with `before: '"v1"'` (the value that
  stops being accepted) and `resource-rejected`; the controller-side failure
  ("unsupported template engine version") is in the statement. The v0.16.0
  templating-v1 doc still says "deprecated … removed in the future" (doc lag);
  the release note, issue #4633 and the code are authoritative.
- **E6 is the deliberate trap.** The values diff alone reads as a changed
  default with a pin (canonical: pinned → informational). Here the pinned OLD
  value is what breaks: chart 0.16.0 renders a Webhook conversion when it is
  true, and v0.16 serves no /convert. Exposure is therefore
  `values-key equals true`, consequence `workload-failure`. Grounded in the
  values comment at both tags, helm.generate.sh, webhook.go, and the issue
  #4662 maintainer comments; the exact runtime error is the one users posted.
- **E7** needs the legacy token Secret to still be listed on the
  ServiceAccount (otherwise v0.15 already used TokenRequest and nothing
  changes), hence the `ref` into ServiceAccount `secrets[]`. The cluster is
  1.32, but a token Secret that ESO keeps using is not "unused", so legacy
  token cleanup would not have removed it. Review, not action: whether Vault
  rejects the new JWT depends on Vault's auth config (issuer /
  disable_iss_validation, role audience), which no fixture input carries.
  Vault v1.18.3 (≥ 1.9) is recorded in the inventory for context only (the
  code comment's "<1.9 will likely fail" does not apply); no item is labelled
  on it because the 1.9 note predates this release.
- **E8** labelled `behavior-change` (review) rather than `permission-lost`:
  with process*=false the controller does not reconcile those kinds anyway
  (root.go passes ClusterSecretStoreEnabled to the ExternalSecret reconciler),
  so the narrowing is intended; what is lost is RBAC on the aggregated
  view/edit/admin roles. Not-affected here because all four values are
  explicitly true. openshiftFinalizers (new, default true = previous grants)
  is part of the same PR family and carries no consequence; not an item.
- **E9** exposure uses `installCRDs equals false` as the Helm-side signal for
  "CRDs installed separately"; this environment sets installCRDs true, the
  path the release notes call safe → not-affected.
- **Kubernetes compatibility** is not an expected item: 0.15.x and 0.16.x both
  list 1.32, so nothing changes. It is an expectedFinding
  (`impact:kubernetes-in-range`, informational) as in cert-manager-1.17-1.18.
- **Not items:** refreshPolicy (#4594) and AWS prefix/tags (#4612, #4538) are
  new optional fields; GCP metadata lookup (#4575) makes fields optional;
  #4561 avoids no-op CRD/webhook updates; webhook.annotations is a new
  optional key. None needs operator work.
- No undecidedImpact links: every link is decided by the supplied values,
  manifests or CRDs.

## G22 categories exercised

- gvk removed: E1 (not-affected), E2 (action-required).
- migrations: E3 (CRD storedVersions; scoped `field` over installed CRDs), E9.
- provider-specific removals/behaviour changes: E5 (fake ValueMap removed,
  not-affected), E7 (Vault Kubernetes auth, review; `ref` across resources).
- validation tightening / behaviour change: E4 (engineVersion v1).
- Helm values changes: E6 (default flip where the pin is the hazard), E8
  (RBAC gated on process* values — RBAC category).
- compatibility hidden in prose: E9 (known-broken v0.16.0 manifests, "use
  0.16.1"); Kubernetes table via F1.
- prose-only defaults: E6's consequence and E3's precondition exist only in
  release prose and issue comments, not in any schema.

## Semantic labels

- E1/E2: `gvk` subjects, `removed` with `replacedBy` the v1 GVK (the release
  "Promotion of `ExternalSecret/v1`  and `SecretStore/v1`"); v1beta1 would also
  work for 0.16 but is unserved in 0.17. Exposure is the canonical
  `gvk-in-use` (manifest use, not CRD presence).
- E3: `migration` family (`remove-v1alpha1-from-crd-stored-versions`); the
  exposure is a `resource` scope over `apiextensions.k8s.io/CustomResourceDefinition`
  with `status.storedVersions[]` equals `"v1alpha1"`, evaluated on crds/.
- E4: `crd-field` on ExternalSecret `spec.target.template.engineVersion`;
  exposure also covers ClusterExternalSecret's embedded
  `spec.externalSecretSpec.target.template.engineVersion` (none in the fixture).
- E5, E7: two subjects each (SecretStore + ClusterSecretStore) and an `any`
  of two `resource` scopes, because the same spec exists on both kinds.
- E6: `helm-value` `default-changed` true→false (JSON `true`/`false`), with a
  non-canonical exposure (`equals true`) explained in the item's `note`.
- E8: four `helm-value` subjects (`behavior-changed`: the key now also gates
  RBAC) sharing one consequence (YAML anchor), exposure `any` of
  `values-key equals false`.
- E9: `migration` family with `upgrade-blocked` (the separately-installed CRD
  apply fails) rather than `migration-required`, because the operator's step
  (use 0.16.1) avoids a blocked install rather than migrating state.
