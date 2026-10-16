# Investigation

Upgrade: cert-manager v1.17.2 → v1.18.0
Environment: payments-prod-eu (Kubernetes 1.31)
Bundle: bnd-b44c21b921e1 · snapshot snap-ebf65ee8eec1

## Agenda (14 items, in priority order)

Close every one of these in `submit_findings` (raises-concern, claims-mitigation, explains,
examined-no-concern or not-examined). Items not listed were already decided by a failing deterministic
validation; you may still comment on them.

1. ci-043695bb193e [agenda:rendered-unexplained] field-changed: apps/v1/Deployment/cert-manager/cert-manager-webhook spec.template.spec.containers[name=cert-manager-webh…
2. ci-145265d231e5 [agenda:rendered-unexplained] resource-added: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-http01-controller-challenges (not mentioned by an…
3. ci-25e3f7619968 [agenda:behaviour] We have changed the default value of `Certificate.Spec.RevisionHistoryLimit` from `nil` to `1`. > 📖 Read Release 1.18…
4. ci-28c673fd340e [agenda:rendered-unexplained] resource-added: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-dns01-controller-challenges (not mentioned…
5. ci-2a481c9a7ff3 [agenda:rendered-unexplained] resource-added: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-http01-controller-challenges (not mentione…
6. ci-3211afab6562 [agenda:rendered-unexplained] field-changed: apps/v1/Deployment/cert-manager/cert-manager-webhook spec.template.spec.containers[name=cert-manager-webh…
7. ci-4f7919826dd5 [agenda:rendered-unexplained] resource-added: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-dns01-controller-challenges (not mentioned by any…
8. ci-4ff47b85d497 [agenda:behaviour] GHSA-8rvj-mm4h-c258 (CVE-2026-62290) affects target v1.18.0: Direct ACME Challenge resources can bypass Issuer DNS01 sol…
9. ci-5571bb810111 [agenda:behaviour] ACME HTTP01 challenge paths now use `PathType` `Exact` in Ingress routes: We have changed the `PathType` for ACME HTTP01…
10. ci-791a7adbcb62 [agenda:behaviour] Remove deprecated feature gate `ValidateCAA`. Setting this feature gate is now a no-op which does nothing but print a wa…
11. ci-86548e9a2300 [agenda:behaviour] The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`: We have changed the default value of …
12. ci-93299c596271 [agenda:behaviour] We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`. > 📖 Read …
13. ci-f86a125e0d17 [agenda:behaviour] The default value of `Certificate.Spec.RevisionHistoryLimit` is now `1`: The default value for the `Certificate` resourc…
14. ci-a731eaaa4a97 [agenda:pass-to-scrutinise] Default of Helm value `prometheus.servicemonitor.targetPort` changed: 9402 → "http-metrics"

## Change items in scope (16)

### ci-043695bb193e — field-changed: apps/v1/Deployment/cert-manager/cert-manager-webhook spec.template.spec.containers[name=cert-manager-webhook].readinessProbe.httpGet.port (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: apps/v1/Deployment/cert-manager/cert-manager-webhook
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-4af333c330fb field-changed apps/v1/Deployment/cert-manager/cert-manager-webhook spec.template.spec.containers[name=cert-manager-webhook].readinessProbe.httpGet.port
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-8b65265556ee validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-4af333c330fb field-changed apps/v1/Deployment/cert-manage…
  - vr-fd68548c11cb validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-37b58e922348 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-145265d231e5 — resource-added: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-http01-controller-challenges (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-http01-controller-challenges
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-3b606494070d resource-added rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-http01-controller-challenges
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-3b81645aaa3d validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-3b606494070d resource-added rbac.authorization.k8s.io/v1/…
  - vr-a0affc378a99 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-254ae9cfc5d7 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-1e5016bda672 — resource-removed: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-controller-challenges (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-structural
- subjects: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-controller-challenges
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-26ea4d5b0c6f resource-removed rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-controller-challenges
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-c6c82d67e49b validate.render-diff@v1 → fail: breaking item (scope:rendered-structural) whose effect reaches the environment's rendered deployment: - rc-26ea4d5b0c6f resource-removed rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-controller-challenges [correlation release-ancho…
  - vr-e0cc89738f9d validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-b7fd50fe0072 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-25e3f7619968 — We have changed the default value of `Certificate.Spec.RevisionHistoryLimit` from `nil` to `1`. > 📖 Read Release 1.18 notes for more information.

- family `procedure`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md L12-L14: "1. We have changed the default value of `Certificate.Spec.RevisionHistoryLimit` from `nil` to `1`. > 📖 Read [Release 1.18 notes](../release-notes/release-notes-1.18.md) for more information.."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-fabcf2340453 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-29c3bee4c760 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - fact vf-b577957e2615 is verified only at level consensus: a consensus or proxy fact is ne…
  - vr-b1eefc373438 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: unknown (impact:not-joined)

### ci-28c673fd340e — resource-added: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-dns01-controller-challenges (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-dns01-controller-challenges
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-66c914d5675c resource-added rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-dns01-controller-challenges
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-214f34c644a3 validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-66c914d5675c resource-added rbac.authorization.k8s.io/v1/…
  - vr-41d80fdac109 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-49cc3f62e307 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-2a481c9a7ff3 — resource-added: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-http01-controller-challenges (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-http01-controller-challenges
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-953b8a1062c8 resource-added rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-http01-controller-challenges
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-f7084be20faa validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-953b8a1062c8 resource-added rbac.authorization.k8s.io/v1/…
  - vr-76d074d6a0c7 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-13f0e6c489b3 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-3211afab6562 — field-changed: apps/v1/Deployment/cert-manager/cert-manager-webhook spec.template.spec.containers[name=cert-manager-webhook].livenessProbe.httpGet.port (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: apps/v1/Deployment/cert-manager/cert-manager-webhook
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-20475b41f0be field-changed apps/v1/Deployment/cert-manager/cert-manager-webhook spec.template.spec.containers[name=cert-manager-webhook].livenessProbe.httpGet.port
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-65a96f3daa0c validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-20475b41f0be field-changed apps/v1/Deployment/cert-manage…
  - vr-6d4fc91352f0 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-c0062a12fe8d validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-4f7919826dd5 — resource-added: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-dns01-controller-challenges (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-dns01-controller-challenges
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-e3ab468fedd6 resource-added rbac.authorization.k8s.io/v1/ClusterRole//cert-manager-dns01-controller-challenges
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-ca659de474ce validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-e3ab468fedd6 resource-added rbac.authorization.k8s.io/v1/…
  - vr-a25b2ce03907 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-22c8bc68a135 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-4ff47b85d497 — GHSA-8rvj-mm4h-c258 (CVE-2026-62290) affects target v1.18.0: Direct ACME Challenge resources can bypass Issuer DNS01 solver policy and use ClusterIssuer DNS…

- family `behavior`, manifestation `unknown` (manifest:unknown), scope scope:breaking; action required
- subjects: GHSA-8rvj-mm4h-c258
- upstream evidence:
  - https://github.com/cert-manager/cert-manager/security/advisories/GHSA-8rvj-mm4h-c258 GHSA-8rvj-mm4h-c258: "GHSA-8rvj-mm4h-c258 (high): Direct ACME Challenge resources can bypass Issuer DNS01 solver policy and use ClusterIssuer DNS credentials; vulnerable: >= 1.18.0, <= 1.20.2; patched: 1.19.6, 1.20.3"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-b9eac50e5c2b validate.render-diff@v1 → not-run: the item manifests as unknown (manifest:unknown): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an observed run decides it
  - vr-fac787386f59 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-99625d9f5eff validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: unknown (impact:not-joined)

### ci-5571bb810111 — ACME HTTP01 challenge paths now use `PathType` `Exact` in Ingress routes: We have changed the `PathType` for ACME HTTP01 Ingress-based challenges to `Exact`.…

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking
- upstream evidence:
  - https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md L28-L89 (Major Themes › ACME HTTP01 challenge paths now use `PathType` `Exact` in Ingress routes): "### ACME HTTP01 challenge paths now use `PathType` `Exact` in Ingress routes > ⚠️ Breaking change We have changed the `PathType` for ACME HTTP01 Ingress-based challenges to `Exact`. This security feature ensures that the challenge path …"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-44f060ad99ca validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-34899e97634d validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-2d76722ed3bb validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: unknown (impact:not-joined)

### ci-791a7adbcb62 — Remove deprecated feature gate `ValidateCAA`. Setting this feature gate is now a no-op which does nothing but print a warning log line

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:category
- upstream evidence:
  - https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md L367-L367 (v1.18.0 › Other (Cleanup or Flake)): "- Remove deprecated feature gate `ValidateCAA`. Setting this feature gate is now a no-op which does nothing but print a warning log line ([`#7553`](https://github.com/cert-manager/cert-manager/pull/7553), [`@SgtCoDFish`](https://github.com/…"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-89a03f717258 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-be0e5ce8334e validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-de5928e0b6a0 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: unknown (impact:not-joined)

### ci-86548e9a2300 — The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`: We have changed the default value of `Certificate.Spec.PrivateKey.RotationPo…

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking
- upstream evidence:
  - https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md L109-L148 (Major Themes › The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always`): "### The default value of `Certificate.Spec.PrivateKey.RotationPolicy` is now `Always` > ⚠️ Breaking change We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`. Why? Because the old …"
- environment rendered changes correlated to it: 28
  - rc-023028c51138 field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//clusterissuers.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.acme.properties.solvers.items.properties.dns01.properties.webhook.properties.config.description
  - rc-227e1ad33c34 field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//challenges.acme.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.solver.properties.dns01.properties.webhook.properties.solverName.description
  - rc-23e0e7936eb7 field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//certificates.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.privateKey.properties.rotationPolicy.description
  - rc-2ad54386f428 field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//orders.acme.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.status.properties.authorizations.items.properties.challenges.items.properties.type.description
  - rc-31092bd52ae8 field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//issuers.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.acme.properties.solvers.items.properties.dns01.properties.azureDNS.properties.managedIdentity.properties.tenantID.description
  - rc-3423418f7078 field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//certificates.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.additionalOutputFormats.description
  - rc-3f0d2b48a94c field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//issuers.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.acme.properties.solvers.items.properties.dns01.properties.webhook.properties.solverName.description
  - rc-40349ec55489 field-changed apiextensions.k8s.io/v1/CustomResourceDefinition//issuers.cert-manager.io spec.versions[name=v1].schema.openAPIV3Schema.properties.spec.properties.acme.properties.solvers.items.properties.dns01.properties.azureDNS.properties.managedIdentity.properties.resourceID.description
  - … 20 more (query_diff changeItem=ci-86548e9a2300)
- exposure roots: 5; impact paths: 11 (4 unresolved)
- deterministic validations already run:
  - vr-1b5deeb5a7ab validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-b4afd75c737a validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - fact vf-9b9cfa5c9f5f is verified only at level consensus: a consensus or proxy fact is ne…
  - vr-6e18c13e39d3 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: unknown (impact:not-joined)

### ci-93299c596271 — We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`. > 📖 Read Release 1.18 notes for more information.

- family `procedure`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/cert-manager/website/blob/master/content/docs/releases/upgrading/upgrading-1.17-1.18.md L8-L10: "1. We have changed the default value of `Certificate.Spec.PrivateKey.RotationPolicy` from `Never` to `Always`. > 📖 Read [Release 1.18 notes](../release-notes/release-notes-1.18.md) for more information.."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-2c89021fde8d validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-755f90566950 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - fact vf-9b9cfa5c9f5f is verified only at level consensus: a consensus or proxy fact is ne…
  - vr-f882c23f129f validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: unknown (impact:not-joined)

### ci-a731eaaa4a97 — Default of Helm value `prometheus.servicemonitor.targetPort` changed: 9402 → "http-metrics"

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:computed-breaking
- subjects: prometheus.servicemonitor.targetPort
- upstream evidence:
  - https://github.com/cert-manager/cert-manager/blob/v1.17.2/deploy/charts/cert-manager/values.yaml deploy/charts/cert-manager/values.yaml: "# +docs:section=Global"
  - https://github.com/cert-manager/cert-manager/blob/v1.18.0/deploy/charts/cert-manager/values.yaml deploy/charts/cert-manager/values.yaml: "# +docs:section=Global"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-e2b7f47f69a8 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-ab82bdfca0b5 impact:values-default-no-effect (not-affected): You leave `prometheus.servicemonitor.targetPort` unset; rendering shows its new defa…
  - vr-663697110810 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/cert-manager.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-9c37c87575c8 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-02ff92f660f6 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: not-affected (impact:values-default-no-effect)

### ci-d1e0b4eaba71 — resource-removed: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-controller-challenges (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-structural
- subjects: rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-controller-challenges
- upstream evidence:
  - ri:render@release/chart-defaults v1.17.2 → v1.18.0: "14 rendered changes between the chart defaults of v1.17.2 and v1.18.0"
- environment rendered changes correlated to it: 1
  - rc-b5d7b1375d97 resource-removed rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-controller-challenges
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-2d6a7f8ddfcd validate.render-diff@v1 → fail: breaking item (scope:rendered-structural) whose effect reaches the environment's rendered deployment: - rc-b5d7b1375d97 resource-removed rbac.authorization.k8s.io/v1/ClusterRoleBinding//cert-manager-controller-challenges [correlation releas…
  - vr-e65676a92214 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-6dc816fadd85 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known

### ci-f86a125e0d17 — The default value of `Certificate.Spec.RevisionHistoryLimit` is now `1`: The default value for the `Certificate` resource's `revisionHistoryLimit` field is now…

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking
- upstream evidence:
  - https://github.com/cert-manager/website/blob/master/content/docs/releases/release-notes/release-notes-1.18.md L151-L160 (Major Themes › The default value of `Certificate.Spec.RevisionHistoryLimit` is now `1`): "### The default value of `Certificate.Spec.RevisionHistoryLimit` is now `1` > ⚠️ Potentially breaking change The default value for the `Certificate` resource's `revisionHistoryLimit` field is now set to 1. This ensures that old `Certifi…"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-4bd4c68efac5 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-e1d71b313e62 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - fact vf-b577957e2615 is verified only at level consensus: a consensus or proxy fact is ne…
  - vr-6d108bd02fa9 validate.static-reference@v1 → not-run: the traversal was truncated (max depth 8 reached): not every path is known
- deterministic join findings: unknown (impact:not-joined)

## Renders

- environment `argocd:clusters/prod/cert-manager.yaml#0/$.spec.sources[0]`: ok, 49 changes, 0 uncorrelated
- environment `kustomize:apps/payments/overlays/prod`: not-applicable, 0 changes, 0 uncorrelated — version-independent: the overlay neither pins nor deploys cert-manager (no image or remote resource carries its version, no object runs its images), so the upgrade does not change what it renders: overlay apps/payments/overlays/prod does no…
- environment `kustomize:platform/pki/overlays/prod`: not-applicable, 0 changes, 0 uncorrelated — version-independent: the overlay neither pins nor deploys cert-manager (no image or remote resource carries its version, no object runs its images), so the upgrade does not change what it renders: overlay platform/pki/overlays/prod does not…
- release `chart-defaults`: ok, 14 changes, 0 uncorrelated

## Uncorrelated environment rendered changes (0)

- none

## Unresolved relationships (1)

- ge-7408a9792f35 references core/v1/Secret/cert-manager/<redacted sha256:465e95e7be32> → cert-manager.io/v1/ClusterIssuer//mesh-ca (inferred): Secret "<redacted sha256:465e95e7be32>" is produced in namespace "cert-manager", but the namespace of the referrer is not known

49 further change items are out of scope (routine or not breaking); they are not classified.

## Your job

1. Work down the agenda. For each item, call `get_change_item` and read the upstream text: where does
   it take effect, and which explicit setting controls it?
2. Examine the environment's rendered changes for the item (`query_diff`, `get_rendered_object`) and
   the environment's own files (`search_manifest`, `read_artifact`). For `controller-behaviour` and
   `unknown` items, render silence proves nothing: find the setting that pins the old behaviour, or cite
   that the environment does not set it.
3. Record what you find (`record_hypothesis`, `record_assertion`, with verbatim citations) and request
   the validations that would decide the item (`request_validation`, naming subjects as the evidence
   does; `request_evidence` for what is missing).
4. Finish with `submit_findings`, closing every agenda item with a disposition.

Budget: 96 tool calls, 4096 KiB returned, 52 turns, 1200 s.
