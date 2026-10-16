# Investigation

Upgrade: kube-prometheus-stack kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0
Environment: obs-prod-north (Kubernetes 1.31)
Bundle: bnd-7cbbf08745dc · snapshot snap-1cc1e34e3220

## Agenda (26 items, in priority order)

Close every one of these in `submit_findings` (raises-concern, claims-mitigation, explains,
examined-no-concern or not-examined). Items not listed were already decided by a failing deterministic
validation; you may still comment on them.

1. ci-0a5e2fb2331c [agenda:rendered-unexplained] field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-controller-manag…
2. ci-1871449ec07f [agenda:rendered-unexplained] field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-coredns spec.endpoint…
3. ci-194d5b487655 [agenda:behaviour] From 89.x to 90.x: The control-plane ServiceMonitors (kubelet, kube-apiserver, kube-controller-manager, kube-scheduler, …
4. ci-1e94c8421137 [agenda:rendered-unexplained] field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-scheduler spec.e…
5. ci-1f035001bfe6 [agenda:rendered-unexplained] field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-etcd spec.endpoi…
6. ci-51933c83f5d4 [agenda:rendered-unexplained] field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-proxy spec.endpo…
7. ci-555bf9fefdf5 [agenda:rendered-unexplained] field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-apiserver spec.endpoi…
8. ci-903ca29715e3 [agenda:rendered-unexplained] field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kubelet spec.endpoint…
9. ci-a56630d1b23c [agenda:rendered-unexplained] field-changed: core/v1/ConfigMap/kube-prometheus-stack/kube-prometheus-stack-grafana data["grafana.ini"] (not mentioned …
10. ci-aaeb79bed47e [agenda:rendered-unexplained] container-arg-changed: apps/v1/Deployment/kube-prometheus-stack/kube-prometheus-stack-operator spec.template.spec.contai…
11. ci-ac26042a3a1c [agenda:rendered-unexplained] resource-added: core/v1/Secret/kube-prometheus-stack/kube-prometheus-stack-prometheus-token (not mentioned by any change…
12. ci-0f8475f72a68 [agenda:pass-to-scrutinise] Helm value `kubeDns.serviceMonitor.bearerTokenFile` removed
13. ci-186eefc98ab9 [agenda:pass-to-scrutinise] Helm value `kubeEtcd.serviceMonitor.certFile` removed
14. ci-1b14cf0134ba [agenda:pass-to-scrutinise] Helm value `kubeControllerManager.serviceMonitor.insecureSkipVerify` removed
15. ci-1d8a464ef1b7 [agenda:pass-to-scrutinise] Helm value `kubeProxy.serviceMonitor.bearerTokenFile` removed
16. ci-2ecd3769e47d [agenda:pass-to-scrutinise] Helm value `kubeEtcd.serviceMonitor.caFile` removed
17. ci-337251f802e5 [agenda:pass-to-scrutinise] Helm value `kubelet.serviceMonitor.insecureSkipVerify` removed
18. ci-4fafc7794273 [agenda:pass-to-scrutinise] Helm value `kubeScheduler.serviceMonitor.serverName` removed
19. ci-5df55229fb90 [agenda:pass-to-scrutinise] Helm value `kubeEtcd.serviceMonitor.serverName` removed
20. ci-63ebfdbffcaf [agenda:pass-to-scrutinise] Helm value `coreDns.serviceMonitor.bearerTokenFile` removed
21. ci-779e8207da5f [agenda:pass-to-scrutinise] Helm value `kubeControllerManager.serviceMonitor.serverName` removed
22. ci-93f70f86bcd3 [agenda:pass-to-scrutinise] Helm value `kubeEtcd.serviceMonitor.insecureSkipVerify` removed
23. ci-9d2d9680733c [agenda:pass-to-scrutinise] Default of Helm value `prometheusOperator.secretFieldSelector` changed: "type!=kubernetes.io/dockercfg,type!=kubernetes.…
24. ci-b798ed2dd1af [agenda:pass-to-scrutinise] Helm value `kubeScheduler.serviceMonitor.insecureSkipVerify` removed
25. ci-f08ea0fa28f6 [agenda:pass-to-scrutinise] Helm value `kubeEtcd.serviceMonitor.keyFile` removed
26. ci-f8350fd0b224 [agenda:pass-to-scrutinise] Helm value `kubeEtcd.serviceMonitor.bearerTokenFile` removed

## Change items in scope (26)

### ci-0a5e2fb2331c — field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-controller-manager spec.endpoints (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-controller-manager
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-2d1f987381da validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-5e7be2ffe523 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-5588b7e42bf6 validate.static-reference@v1 → not-run: the item has no impact paths to check

### ci-0f8475f72a68 — Helm value `kubeDns.serviceMonitor.bearerTokenFile` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeDns.serviceMonitor.bearerTokenFile
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-a1420a8da933 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-d3acf5071839 impact:values-unset (not-affected): Your values do not set `kubeDns.serviceMonitor.bearerTokenFile` checked values: 15 facts against…
  - vr-88ef672b62b1 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-3171950fb423 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-912181bebb2f validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-186eefc98ab9 — Helm value `kubeEtcd.serviceMonitor.certFile` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeEtcd.serviceMonitor.certFile
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-56992233a181 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-842ec7a324e5 impact:values-unset (not-affected): Your values do not set `kubeEtcd.serviceMonitor.certFile` checked values: 15 facts against kubeE…
  - vr-878fd7ff68aa validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-2a120f64042c validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-a5821e39629f validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-1871449ec07f — field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-coredns spec.endpoints (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-coredns
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 1
  - rc-566829f890c0 field-changed monitoring.coreos.com/v1/ServiceMonitor/kube-system/kube-prometheus-stack-coredns spec.endpoints
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-517355c710e1 validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-566829f890c0 field-changed monitoring.coreos.com/v1/Servi…
  - vr-a07b4c234050 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-6acba2d263c5 validate.static-reference@v1 → not-run: an unresolved relationship decides nothing: - ip-d3c38621873f: the path is its root alone: there is no reference on it to check

### ci-194d5b487655 — From 89.x to 90.x: The control-plane ServiceMonitors (kubelet, kube-apiserver, kube-controller-manager, kube-scheduler, kube-etcd, kube-proxy, coredns…

- family `procedure`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; action required
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.0.0/charts/kube-prometheus-stack/UPGRADE.md L3-L76 (From 89.x to 90.x): "## From 89.x to 90.x The control-plane ServiceMonitors (kubelet, kube-apiserver, kube-controller-manager, kube-scheduler, kube-etcd, kube-proxy, coredns, kube-dns) no longer reference credentials on the scraper's filesystem. `bearerTokenFil…"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-56366468c177 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-b8f4d8658e50 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-9231d9c36192 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-1b14cf0134ba — Helm value `kubeControllerManager.serviceMonitor.insecureSkipVerify` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeControllerManager.serviceMonitor.insecureSkipVerify
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-2b6706e4877f validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-0bbcb6f50f24 impact:values-unset (not-affected): Your values do not set `kubeControllerManager.serviceMonitor.insecureSkipVerify` checked values:…
  - vr-e24b3ba07a85 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-8564ca929939 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-bf3b856ff34a validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-1d8a464ef1b7 — Helm value `kubeProxy.serviceMonitor.bearerTokenFile` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeProxy.serviceMonitor.bearerTokenFile
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-176d30af80e2 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-564f6d99d7e7 impact:values-unset (not-affected): Your values do not set `kubeProxy.serviceMonitor.bearerTokenFile` checked values: 15 facts again…
  - vr-ef225981aaf5 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-4284121fe3e5 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-9035c07a857c validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-1e94c8421137 — field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-scheduler spec.endpoints (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-scheduler
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-a478f0c5e34a validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-74df3b228fd4 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-557cb2382d88 validate.static-reference@v1 → not-run: the item has no impact paths to check

### ci-1f035001bfe6 — field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-etcd spec.endpoints (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-etcd
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-46eeab440470 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-a289ca6df0fa validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-b3a2802c9047 validate.static-reference@v1 → not-run: the item has no impact paths to check

### ci-2ecd3769e47d — Helm value `kubeEtcd.serviceMonitor.caFile` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeEtcd.serviceMonitor.caFile
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-320c01145ae2 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-da21fe42c2a6 impact:values-unset (not-affected): Your values do not set `kubeEtcd.serviceMonitor.caFile` checked values: 15 facts against kubeEtc…
  - vr-8623e53002e1 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-6f7ff43baf46 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-d52fdc921de7 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-337251f802e5 — Helm value `kubelet.serviceMonitor.insecureSkipVerify` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubelet.serviceMonitor.insecureSkipVerify
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-d7ca79bca72c validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-a7b0f4e5ca4f impact:values-unset (not-affected): Your values do not set `kubelet.serviceMonitor.insecureSkipVerify` checked values: 15 facts agai…
  - vr-cca97f4a1b68 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-3c33056d9bae validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-96a533dee905 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-4fafc7794273 — Helm value `kubeScheduler.serviceMonitor.serverName` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeScheduler.serviceMonitor.serverName
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-773032afd6d4 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-e6abf318cdfa impact:values-unset (not-affected): Your values do not set `kubeScheduler.serviceMonitor.serverName` checked values: 15 facts agains…
  - vr-29b322ef0e0d validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-6d4e0a09ef81 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-5fdf8cba2ca4 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-51933c83f5d4 — field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-proxy spec.endpoints (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kube-proxy
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-8a870f473a35 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-2c714e0a351b validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-fca84ae7ff2e validate.static-reference@v1 → not-run: the item has no impact paths to check

### ci-555bf9fefdf5 — field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-apiserver spec.endpoints (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-apiserver
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 1
  - rc-3dd4a1054b1d field-changed monitoring.coreos.com/v1/ServiceMonitor/default/kube-prometheus-stack-apiserver spec.endpoints
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-6d8adb065456 validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-3dd4a1054b1d field-changed monitoring.coreos.com/v1/Servi…
  - vr-bab86689105c validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-46d7af930dba validate.static-reference@v1 → not-run: an unresolved relationship decides nothing: - ip-e0bfa00dc275: the path is its root alone: there is no reference on it to check

### ci-5df55229fb90 — Helm value `kubeEtcd.serviceMonitor.serverName` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeEtcd.serviceMonitor.serverName
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-faadc2944bc3 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-4bbd88b1105a impact:values-unset (not-affected): Your values do not set `kubeEtcd.serviceMonitor.serverName` checked values: 15 facts against kub…
  - vr-b1d1f39940f9 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-3ccfedb14831 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-def5b2915d85 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-63ebfdbffcaf — Helm value `coreDns.serviceMonitor.bearerTokenFile` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: coreDns.serviceMonitor.bearerTokenFile
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-cee1bc638c85 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-306f4c101613 impact:values-unset (not-affected): Your values do not set `coreDns.serviceMonitor.bearerTokenFile` checked values: 15 facts against…
  - vr-51c5c7ba8707 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-7646b714c452 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-997b37ad05bc validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-779e8207da5f — Helm value `kubeControllerManager.serviceMonitor.serverName` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeControllerManager.serviceMonitor.serverName
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-575b8a3f2d0c validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-158872a559c4 impact:values-unset (not-affected): Your values do not set `kubeControllerManager.serviceMonitor.serverName` checked values: 15 fact…
  - vr-45cc6f9eeea4 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-bfd10d690d65 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-c6336cfc5a55 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-903ca29715e3 — field-changed: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kubelet spec.endpoints (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: monitoring.coreos.com/v1/ServiceMonitor/kube-prometheus-stack/kube-prometheus-stack-kubelet
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 1
  - rc-0b14ccf76614 field-changed monitoring.coreos.com/v1/ServiceMonitor/kube-system/kube-prometheus-stack-kubelet spec.endpoints
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-2df01a774116 validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-0b14ccf76614 field-changed monitoring.coreos.com/v1/Servi…
  - vr-49869a72a4f6 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-554e989e4e9f validate.static-reference@v1 → not-run: an unresolved relationship decides nothing: - ip-2ab8023afec7: the path is its root alone: there is no reference on it to check

### ci-93f70f86bcd3 — Helm value `kubeEtcd.serviceMonitor.insecureSkipVerify` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeEtcd.serviceMonitor.insecureSkipVerify
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-367b1b499708 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-4d8448a818bc impact:values-unset (not-affected): Your values do not set `kubeEtcd.serviceMonitor.insecureSkipVerify` checked values: 15 facts aga…
  - vr-15e587283f25 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-92c9bcd10c74 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-6e344f57f410 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-9d2d9680733c — Default of Helm value `prometheusOperator.secretFieldSelector` changed: "type!=kubernetes.io/dockercfg,type!=kubernetes.i… → "type!=kubernetes.io/dockercfg,typ…

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:computed-breaking
- subjects: prometheusOperator.secretFieldSelector
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-8b8ed0f9dca2 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-fc1248c49f1f validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-75db3ab0cfb4 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: review-required (impact:values-default-applies)

### ci-a56630d1b23c — field-changed: core/v1/ConfigMap/kube-prometheus-stack/kube-prometheus-stack-grafana data["grafana.ini"] (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: core/v1/ConfigMap/kube-prometheus-stack/kube-prometheus-stack-grafana
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-90ac69eb3dcb validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-07f5c920ef10 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-e78d70bc05c7 validate.static-reference@v1 → not-run: the item has no impact paths to check

### ci-aaeb79bed47e — container-arg-changed: apps/v1/Deployment/kube-prometheus-stack/kube-prometheus-stack-operator spec.template.spec.containers[name=kube-prometheus-stack].args[=--secret-field-selector] (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: apps/v1/Deployment/kube-prometheus-stack/kube-prometheus-stack-operator
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 1
  - rc-edf56bd91ceb container-arg-changed apps/v1/Deployment/monitoring/kube-prometheus-stack-operator spec.template.spec.containers[name=kube-prometheus-stack].args[=--secret-field-selector]
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-467d744251fa validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-edf56bd91ceb container-arg-changed apps/v1/Deployment/mon…
  - vr-5841646a3b01 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-c6383330736d validate.static-reference@v1 → not-run: an unresolved relationship decides nothing: - ip-e12a6d558d3c: references resolve, but that does not show the unexplained rendered change harmless

### ci-ac26042a3a1c — resource-added: core/v1/Secret/kube-prometheus-stack/kube-prometheus-stack-prometheus-token (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: core/v1/Secret/kube-prometheus-stack/kube-prometheus-stack-prometheus-token
- upstream evidence:
  - ri:render@release/chart-defaults kube-prometheus-stack-89.2.4 → kube-prometheus-stack-90.2.0: "10 rendered changes between the chart defaults of kube-prometheus-stack-89.2.4 and kube-prometheus-stack-90.2.0"
- environment rendered changes correlated to it: 1
  - rc-49f24e98534f resource-added core/v1/Secret/monitoring/kube-prometheus-stack-prometheus-token
- exposure roots: 1; impact paths: 1 (0 unresolved)
- deterministic validations already run:
  - vr-cf94baf9643f validate.render-diff@v1 → not-run: the item is not breaking (scope:rendered-unexplained), but its effect reaches the rendered deployment; a render cannot say whether that is harmless, nor name a concrete failure: - rc-49f24e98534f resource-added core/v1/Secret/monitoring/kub…
  - vr-deb4d3c54952 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-af99d911ecdd validate.static-reference@v1 → not-run: an unresolved relationship decides nothing: - ip-51b78fd93122: the path is its root alone: there is no reference on it to check

### ci-b798ed2dd1af — Helm value `kubeScheduler.serviceMonitor.insecureSkipVerify` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeScheduler.serviceMonitor.insecureSkipVerify
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-4137bde047c7 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-e7a83f279426 impact:values-unset (not-affected): Your values do not set `kubeScheduler.serviceMonitor.insecureSkipVerify` checked values: 15 fact…
  - vr-63768232bcba validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-2bc295bf8840 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-6205fc6b9bd2 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-f08ea0fa28f6 — Helm value `kubeEtcd.serviceMonitor.keyFile` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeEtcd.serviceMonitor.keyFile
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-d59b8efb8b77 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-87e0adc88ce1 impact:values-unset (not-affected): Your values do not set `kubeEtcd.serviceMonitor.keyFile` checked values: 15 facts against kubeEt…
  - vr-a07cd90c7d93 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-3ad1103be79d validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-d62f05257206 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-f8350fd0b224 — Helm value `kubeEtcd.serviceMonitor.bearerTokenFile` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: kubeEtcd.serviceMonitor.bearerTokenFile
- upstream evidence:
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-90.2.0/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
  - https://github.com/prometheus-community/helm-charts/blob/kube-prometheus-stack-89.2.4/charts/kube-prometheus-stack/values.yaml charts/kube-prometheus-stack/values.yaml: "# Default values for kube-prometheus-stack."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-9d97978322dd validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-6657744080c0 impact:values-unset (not-affected): Your values do not set `kubeEtcd.serviceMonitor.bearerTokenFile` checked values: 15 facts agains…
  - vr-4696d77b2f1b validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-b07ffcc7cd65 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-6cbc9731c692 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

## Renders

- environment `argocd:clusters/prod/kube-prometheus-stack.yaml#0/$.spec.sources[0]`: ok, 5 changes, 0 uncorrelated
- environment `kustomize:apps/checkout/overlays/prod`: not-applicable, 0 changes, 0 uncorrelated — version-independent: the overlay neither pins nor deploys kube-prometheus-stack (no image or remote resource carries its version, no object runs its images), so the upgrade does not change what it renders: overlay apps/checkout/overlays/pro…
- release `chart-defaults`: ok, 10 changes, 0 uncorrelated

## Uncorrelated environment rendered changes (0)

- none

## Unresolved relationships (0)

- none

15 further change items are out of scope (routine or not breaking); they are not classified.

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

Budget: 168 tool calls, 6016 KiB returned, 88 turns, 1860 s.
