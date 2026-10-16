# Unsafe-eligibility diagnostic — kube-prometheus-stack obs-north (pipeline output only)

Source: stored run records only (`eval/investigations/results/A-20261016T213500Z-cf761e89e5/cases/kube-prometheus-stack-89-90--obs-north/`; arm A before `fixes`: `eval/investigations/results/A-20261016T201359Z-294a49e7a9`; arm C: `eval/investigations/results/C-20261016T213533Z-cf761e89e5`). Contains pipeline output only: no expected classes, expected item text, NOTES or observed content. Environment: `obs-prod-north`; investigation `inv-7ed47e100661`, bundle `bnd-d440b5569088`.

The four findings below are the not-affected, approval-eligible findings of arm A′ on this case that the evaluation's unsafe-eligibility metric counted. (Other eligible not-affected findings exist in the same run; they are not in scope here.)

## `ci-0f8475f72a68` — finding `inf-8584d1f22d6d`

- scope rule `scope:breaking`; manifestation `rendered-config` (`manifest:computed-values`); family `values`; anchor change `chg-c9e1bc5adb08`; breaking=False, actionRequired=True; subjects ['kubeDns.serviceMonitor.bearerTokenFile']
- summary (bundle title): Helm value `kubeDns.serviceMonitor.bearerTokenFile` removed
- upstream change detail (edge): Values set at this key no longer take effect after the upgrade (or are rejected when the chart validates values against a schema). Default in kube-prometheus-stack-89.2.4: "/var/run/secrets/kubernetes.io/serviceaccount/token". Documentation in kube-prometheus-stack-89.2.4: File containing bearer token to be used when scraping targets Empty value do not send any bearer token.
- item upstream evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
- class `not-affected`, rule `classify:validated-clear`, basis `deterministic`, approval eligible=True, requiresHumanReview=False

**Correlated rendered changes (release + environment scope):** none — no rendered change in either scope lists this item; the finding cites `renderedChanges: []`.

**Traversal:** 0 exposure root(s), 0 path(s) for the item; traversal complete=True.

**R2 rule trace** (classifier reasoning, verbatim):

> R2 classify:validated-clear → not-affected. VALIDATED by 2 passing validation(s): validate.render-diff@v1 [vr-88ef672b62b1, deterministic]; validate.join@v1 [vr-f4394492666f, deterministic]. R2.1 every source and input deciding a values item is collected and the environment render is complete; R2.2 0 correlated environment rendered change(s) shown harmless; R2.3 the traversal is complete for the item; R2.4 no exposure root, and a passing validation of the item exists; R2.5 no failing or erroring validation; R2.6 every agent concern answered by a validation; R2.7 no unvalidated assumption; R2.8 no action-required join verdict.

| R2 condition | held | why (from the record) |
|---|---|---|
| R2.1 collection | yes | values family: repo input and environment render complete (no collection gap on the finding) |
| R2.2 exposure accounted for | yes (vacuous) | 0 correlated environment rendered changes |
| R2.3 traversal complete for item | yes | 0 roots / 0 paths; no unresolved edge reachable |
| R2.4 paths/roots covered | yes (no-roots branch) | no exposure root ⇒ "at least one passing validation of the item exists": `vr-88ef672b62b1` pass, `vr-f4394492666f` pass; item is rendered-config so render-silence validations count |
| R2.5 no fail/error | yes | outcomes: pass, pass |
| R2.6 concerns answered | yes (vacuous) | agent not run: 0 assertions on the item |
| R2.7 no assumption | yes | assumptions [] |
| R2.8 no action-required join verdict | yes | join findings on the change: `imp-d3acf5071839` not-affected impact:values-unset |

Join finding `imp-d3acf5071839` (impact:values-unset): "Your values do not set `kubeDns.serviceMonitor.bearerTokenFile`" — The target release of kube-prometheus-stack-90.2.0 changes these keys; your supplied values files set 15 keys in total and none of them is the changed key or under it, so nothing about this change takes effect on you. Checks: [{"dimension": "values", "facts": 15, "subjects": ["kubeDns.serviceMonitor.bearerTokenFile"]}]

**Cited validation records:**

- `vr-88ef672b62b1` `validate.render-diff@v1` kind `render-diff` level `deterministic` → **pass**
  - property: The item's effect does not reach the environment's rendered configuration (Helm value `kubeDns.serviceMonitor.bearerTokenFile` removed)
  - detail: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - subjects: none; paths none; rendered changes none; impact finding —; pin none
  - inputs: `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/to.yaml`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/from.yaml`
  - evidence: `ev-407fb5c90d8a` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-0f8475f72a68, none an effect on rendered configuration"
- `vr-f4394492666f` `validate.join@v1` kind `override-inspection` level `deterministic` → **pass**
  - property: The deterministic join (five-class impact report) finds that the change does not reach or break the environment
  - detail: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-d3acf5071839 impact:values-unset (not-affected): Your values do not set `kubeDns.serviceMonitor.bearerTokenFile` checked values: 15 facts against kubeDns.serviceMonitor.bearerTokenFile
  - subjects: none; paths none; rendered changes none; impact finding imp-d3acf5071839; pin none
  - inputs: `derived/change/upgrade-edge.json`, `derived/snapshot/environment-facts.json`
  - evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
  - evidence: `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."

Finding evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-407fb5c90d8a` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-0f8475f72a68, none an effect on rendered configuration"; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."

## `ci-1d8a464ef1b7` — finding `inf-8bd4c3f5b149`

- scope rule `scope:breaking`; manifestation `rendered-config` (`manifest:computed-values`); family `values`; anchor change `chg-1b377ed0ad5e`; breaking=False, actionRequired=True; subjects ['kubeProxy.serviceMonitor.bearerTokenFile']
- summary (bundle title): Helm value `kubeProxy.serviceMonitor.bearerTokenFile` removed
- upstream change detail (edge): Values set at this key no longer take effect after the upgrade (or are rejected when the chart validates values against a schema). Default in kube-prometheus-stack-89.2.4: "/var/run/secrets/kubernetes.io/serviceaccount/token". Documentation in kube-prometheus-stack-89.2.4: File containing bearer token to be used when scraping targets Empty value do not send any bearer token.
- item upstream evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
- class `not-affected`, rule `classify:validated-clear`, basis `deterministic`, approval eligible=True, requiresHumanReview=False

**Correlated rendered changes (release + environment scope):** none — no rendered change in either scope lists this item; the finding cites `renderedChanges: []`.

**Traversal:** 0 exposure root(s), 0 path(s) for the item; traversal complete=True.

**R2 rule trace** (classifier reasoning, verbatim):

> R2 classify:validated-clear → not-affected. VALIDATED by 2 passing validation(s): validate.join@v1 [vr-955ac1995aac, deterministic]; validate.render-diff@v1 [vr-ef225981aaf5, deterministic]. R2.1 every source and input deciding a values item is collected and the environment render is complete; R2.2 0 correlated environment rendered change(s) shown harmless; R2.3 the traversal is complete for the item; R2.4 no exposure root, and a passing validation of the item exists; R2.5 no failing or erroring validation; R2.6 every agent concern answered by a validation; R2.7 no unvalidated assumption; R2.8 no action-required join verdict.

| R2 condition | held | why (from the record) |
|---|---|---|
| R2.1 collection | yes | values family: repo input and environment render complete (no collection gap on the finding) |
| R2.2 exposure accounted for | yes (vacuous) | 0 correlated environment rendered changes |
| R2.3 traversal complete for item | yes | 0 roots / 0 paths; no unresolved edge reachable |
| R2.4 paths/roots covered | yes (no-roots branch) | no exposure root ⇒ "at least one passing validation of the item exists": `vr-955ac1995aac` pass, `vr-ef225981aaf5` pass; item is rendered-config so render-silence validations count |
| R2.5 no fail/error | yes | outcomes: pass, pass |
| R2.6 concerns answered | yes (vacuous) | agent not run: 0 assertions on the item |
| R2.7 no assumption | yes | assumptions [] |
| R2.8 no action-required join verdict | yes | join findings on the change: `imp-564f6d99d7e7` not-affected impact:values-unset |

Join finding `imp-564f6d99d7e7` (impact:values-unset): "Your values do not set `kubeProxy.serviceMonitor.bearerTokenFile`" — The target release of kube-prometheus-stack-90.2.0 changes these keys; your supplied values files set 15 keys in total and none of them is the changed key or under it, so nothing about this change takes effect on you. Checks: [{"dimension": "values", "facts": 15, "subjects": ["kubeProxy.serviceMonitor.bearerTokenFile"]}]

**Cited validation records:**

- `vr-955ac1995aac` `validate.join@v1` kind `override-inspection` level `deterministic` → **pass**
  - property: The deterministic join (five-class impact report) finds that the change does not reach or break the environment
  - detail: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-564f6d99d7e7 impact:values-unset (not-affected): Your values do not set `kubeProxy.serviceMonitor.bearerTokenFile` checked values: 15 facts against kubeProxy.serviceMonitor.bearerTokenFile
  - subjects: none; paths none; rendered changes none; impact finding imp-564f6d99d7e7; pin none
  - inputs: `derived/change/upgrade-edge.json`, `derived/snapshot/environment-facts.json`
  - evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
  - evidence: `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
- `vr-ef225981aaf5` `validate.render-diff@v1` kind `render-diff` level `deterministic` → **pass**
  - property: The item's effect does not reach the environment's rendered configuration (Helm value `kubeProxy.serviceMonitor.bearerTokenFile` removed)
  - detail: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - subjects: none; paths none; rendered changes none; impact finding —; pin none
  - inputs: `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/to.yaml`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/from.yaml`
  - evidence: `ev-18df1bf202bc` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-1d8a464ef1b7, none an effect on rendered configuration"

Finding evidence: `ev-18df1bf202bc` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-1d8a464ef1b7, none an effect on rendered configuration"; `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."

## `ci-63ebfdbffcaf` — finding `inf-b5bc4b4cbd78`

- scope rule `scope:breaking`; manifestation `rendered-config` (`manifest:computed-values`); family `values`; anchor change `chg-94ed9ea0dd7f`; breaking=False, actionRequired=True; subjects ['coreDns.serviceMonitor.bearerTokenFile']
- summary (bundle title): Helm value `coreDns.serviceMonitor.bearerTokenFile` removed
- upstream change detail (edge): Values set at this key no longer take effect after the upgrade (or are rejected when the chart validates values against a schema). Default in kube-prometheus-stack-89.2.4: "/var/run/secrets/kubernetes.io/serviceaccount/token". Documentation in kube-prometheus-stack-89.2.4: File containing bearer token to be used when scraping targets Empty value do not send any bearer token.
- item upstream evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
- class `not-affected`, rule `classify:validated-clear`, basis `deterministic`, approval eligible=True, requiresHumanReview=False

**Correlated rendered changes (release + environment scope):** none — no rendered change in either scope lists this item; the finding cites `renderedChanges: []`.

**Traversal:** 0 exposure root(s), 0 path(s) for the item; traversal complete=True.

**R2 rule trace** (classifier reasoning, verbatim):

> R2 classify:validated-clear → not-affected. VALIDATED by 2 passing validation(s): validate.render-diff@v1 [vr-51c5c7ba8707, deterministic]; validate.join@v1 [vr-8a38130de336, deterministic]. R2.1 every source and input deciding a values item is collected and the environment render is complete; R2.2 0 correlated environment rendered change(s) shown harmless; R2.3 the traversal is complete for the item; R2.4 no exposure root, and a passing validation of the item exists; R2.5 no failing or erroring validation; R2.6 every agent concern answered by a validation; R2.7 no unvalidated assumption; R2.8 no action-required join verdict.

| R2 condition | held | why (from the record) |
|---|---|---|
| R2.1 collection | yes | values family: repo input and environment render complete (no collection gap on the finding) |
| R2.2 exposure accounted for | yes (vacuous) | 0 correlated environment rendered changes |
| R2.3 traversal complete for item | yes | 0 roots / 0 paths; no unresolved edge reachable |
| R2.4 paths/roots covered | yes (no-roots branch) | no exposure root ⇒ "at least one passing validation of the item exists": `vr-51c5c7ba8707` pass, `vr-8a38130de336` pass; item is rendered-config so render-silence validations count |
| R2.5 no fail/error | yes | outcomes: pass, pass |
| R2.6 concerns answered | yes (vacuous) | agent not run: 0 assertions on the item |
| R2.7 no assumption | yes | assumptions [] |
| R2.8 no action-required join verdict | yes | join findings on the change: `imp-306f4c101613` not-affected impact:values-unset |

Join finding `imp-306f4c101613` (impact:values-unset): "Your values do not set `coreDns.serviceMonitor.bearerTokenFile`" — The target release of kube-prometheus-stack-90.2.0 changes these keys; your supplied values files set 15 keys in total and none of them is the changed key or under it, so nothing about this change takes effect on you. Checks: [{"dimension": "values", "facts": 15, "subjects": ["coreDns.serviceMonitor.bearerTokenFile"]}]

**Cited validation records:**

- `vr-51c5c7ba8707` `validate.render-diff@v1` kind `render-diff` level `deterministic` → **pass**
  - property: The item's effect does not reach the environment's rendered configuration (Helm value `coreDns.serviceMonitor.bearerTokenFile` removed)
  - detail: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - subjects: none; paths none; rendered changes none; impact finding —; pin none
  - inputs: `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/to.yaml`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/from.yaml`
  - evidence: `ev-46008633f302` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-63ebfdbffcaf, none an effect on rendered configuration"
- `vr-8a38130de336` `validate.join@v1` kind `override-inspection` level `deterministic` → **pass**
  - property: The deterministic join (five-class impact report) finds that the change does not reach or break the environment
  - detail: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-306f4c101613 impact:values-unset (not-affected): Your values do not set `coreDns.serviceMonitor.bearerTokenFile` checked values: 15 facts against coreDns.serviceMonitor.bearerTokenFile
  - subjects: none; paths none; rendered changes none; impact finding imp-306f4c101613; pin none
  - inputs: `derived/change/upgrade-edge.json`, `derived/snapshot/environment-facts.json`
  - evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
  - evidence: `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."

Finding evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-46008633f302` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-63ebfdbffcaf, none an effect on rendered configuration"; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."

## `ci-f8350fd0b224` — finding `inf-9d2274eda209`

- scope rule `scope:breaking`; manifestation `rendered-config` (`manifest:computed-values`); family `values`; anchor change `chg-ce0173e6b9b1`; breaking=False, actionRequired=True; subjects ['kubeEtcd.serviceMonitor.bearerTokenFile']
- summary (bundle title): Helm value `kubeEtcd.serviceMonitor.bearerTokenFile` removed
- upstream change detail (edge): Values set at this key no longer take effect after the upgrade (or are rejected when the chart validates values against a schema). Default in kube-prometheus-stack-89.2.4: "/var/run/secrets/kubernetes.io/serviceaccount/token". Documentation in kube-prometheus-stack-89.2.4: File containing bearer token to be used when scraping targets Empty value do not send any bearer token.
- item upstream evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
- class `not-affected`, rule `classify:validated-clear`, basis `deterministic`, approval eligible=True, requiresHumanReview=False

**Correlated rendered changes (release + environment scope):** none — no rendered change in either scope lists this item; the finding cites `renderedChanges: []`.

**Traversal:** 0 exposure root(s), 0 path(s) for the item; traversal complete=True.

**R2 rule trace** (classifier reasoning, verbatim):

> R2 classify:validated-clear → not-affected. VALIDATED by 2 passing validation(s): validate.render-diff@v1 [vr-4696d77b2f1b, deterministic]; validate.join@v1 [vr-ad54f5151c95, deterministic]. R2.1 every source and input deciding a values item is collected and the environment render is complete; R2.2 0 correlated environment rendered change(s) shown harmless; R2.3 the traversal is complete for the item; R2.4 no exposure root, and a passing validation of the item exists; R2.5 no failing or erroring validation; R2.6 every agent concern answered by a validation; R2.7 no unvalidated assumption; R2.8 no action-required join verdict.

| R2 condition | held | why (from the record) |
|---|---|---|
| R2.1 collection | yes | values family: repo input and environment render complete (no collection gap on the finding) |
| R2.2 exposure accounted for | yes (vacuous) | 0 correlated environment rendered changes |
| R2.3 traversal complete for item | yes | 0 roots / 0 paths; no unresolved edge reachable |
| R2.4 paths/roots covered | yes (no-roots branch) | no exposure root ⇒ "at least one passing validation of the item exists": `vr-4696d77b2f1b` pass, `vr-ad54f5151c95` pass; item is rendered-config so render-silence validations count |
| R2.5 no fail/error | yes | outcomes: pass, pass |
| R2.6 concerns answered | yes (vacuous) | agent not run: 0 assertions on the item |
| R2.7 no assumption | yes | assumptions [] |
| R2.8 no action-required join verdict | yes | join findings on the change: `imp-6657744080c0` not-affected impact:values-unset |

Join finding `imp-6657744080c0` (impact:values-unset): "Your values do not set `kubeEtcd.serviceMonitor.bearerTokenFile`" — The target release of kube-prometheus-stack-90.2.0 changes these keys; your supplied values files set 15 keys in total and none of them is the changed key or under it, so nothing about this change takes effect on you. Checks: [{"dimension": "values", "facts": 15, "subjects": ["kubeEtcd.serviceMonitor.bearerTokenFile"]}]

**Cited validation records:**

- `vr-4696d77b2f1b` `validate.render-diff@v1` kind `render-diff` level `deterministic` → **pass**
  - property: The item's effect does not reach the environment's rendered configuration (Helm value `kubeEtcd.serviceMonitor.bearerTokenFile` removed)
  - detail: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - subjects: none; paths none; rendered changes none; impact finding —; pin none
  - inputs: `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/to.yaml`, `derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/from.yaml`
  - evidence: `ev-446c3aebe8f7` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-f8350fd0b224, none an effect on rendered configuration"
- `vr-ad54f5151c95` `validate.join@v1` kind `override-inspection` level `deterministic` → **pass**
  - property: The deterministic join (five-class impact report) finds that the change does not reach or break the environment
  - detail: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-6657744080c0 impact:values-unset (not-affected): Your values do not set `kubeEtcd.serviceMonitor.bearerTokenFile` checked values: 15 facts against kubeEtcd.serviceMonitor.bearerTokenFile
  - subjects: none; paths none; rendered changes none; impact finding imp-6657744080c0; pin none
  - inputs: `derived/change/upgrade-edge.json`, `derived/snapshot/environment-facts.json`
  - evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."
  - evidence: `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."

Finding evidence: `ev-38c25d818334` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."; `ev-446c3aebe8f7` rendered-diff `ri:renderdiff@v1/derived/render/env/argocd-clusters-prod-kube-prometheus-stack-yaml-0-spec-sources-0-412bebba/diff.json` @ `target argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]` — "complete environment render argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]: 5 rendered changes, 0 correlated to ci-f8350fd0b224, none an effect on rendered configuration"; `ev-8df1be07ae9d` structured `https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml` @ `charts/kube-prometheus-stack/values.yaml` — "# Default values for kube-prometheus-stack."

## Same items in arm A (before `fixes`, run `A-20261016T201359Z-294a49e7a9`)

| item | class | rule | blocking gap(s) |
|---|---|---|---|
| `ci-0f8475f72a68` | insufficient-evidence | `classify:insufficient-evidence` | collection: `gitops-repo:.` — environment input gitops-repo . that decides a values item is partial: 1 loader warning(s): repo mode found 2 values files; all are applied, the last one wins per key; agent: `agent` — the investigation agent did not complete (not-run: agent disabled (--agent not given)) |
| `ci-1d8a464ef1b7` | insufficient-evidence | `classify:insufficient-evidence` | collection: `gitops-repo:.` — environment input gitops-repo . that decides a values item is partial: 1 loader warning(s): repo mode found 2 values files; all are applied, the last one wins per key; agent: `agent` — the investigation agent did not complete (not-run: agent disabled (--agent not given)) |
| `ci-63ebfdbffcaf` | insufficient-evidence | `classify:insufficient-evidence` | collection: `gitops-repo:.` — environment input gitops-repo . that decides a values item is partial: 1 loader warning(s): repo mode found 2 values files; all are applied, the last one wins per key; agent: `agent` — the investigation agent did not complete (not-run: agent disabled (--agent not given)) |
| `ci-f8350fd0b224` | insufficient-evidence | `classify:insufficient-evidence` | collection: `gitops-repo:.` — environment input gitops-repo . that decides a values item is partial: 1 loader warning(s): repo mode found 2 values files; all are applied, the last one wins per key; agent: `agent` — the investigation agent did not complete (not-run: agent disabled (--agent not given)) |

In arm A the snapshot input `gitops-repo .` had status `partial` ("1 loader warning(s): repo mode found 2 values files; all are applied, the last one wins per key"); in arm A′ it is `ok`.

## Arm C observed-affected findings and these items (ids only)

| C finding | change item | anchor rendered change | correlated to one of the 4 items? | same item's class in A′ |
|---|---|---|---|---|
| `inf-fbaa11af76f1` | `ci-1871449ec07f` | `rc-20bddabd3ec5` | no | potentially-affected eligible=False |
| `inf-d73bf8ce1d03` | `ci-555bf9fefdf5` | `rc-bb1a77e29889` | no | potentially-affected eligible=False |
| `inf-fc2c9f9dee54` | `ci-903ca29715e3` | `rc-35e7b3f173b5` | no | potentially-affected eligible=False |

The four items above, in arm C: `ci-0f8475f72a68` not-affected, `ci-1d8a464ef1b7` not-affected, `ci-63ebfdbffcaf` not-affected, `ci-f8350fd0b224` not-affected.
