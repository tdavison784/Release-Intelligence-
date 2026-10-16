# Investigation

Upgrade: ingress-nginx controller-v1.11.5 → controller-v1.12.0
Environment: shop-prod-de (Kubernetes 1.31)
Bundle: bnd-d5289346bfdb · snapshot snap-9362a47b67db

## Change items in scope (27)

### ci-0e8570d62156 — ⚠️ Enable security features by default (#11819) ⚠️ This changes the default of the following CLI arguments: * `--enable-annotation-validation` gets enabled by…

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L139-L149 (Changelog › All changes:): "* ⚠️ Enable security features by default (#11819) ⚠️ This changes the default of the following CLI arguments: * `--enable-annotation-validation` gets enabled by default. It also changes the default of the following configuration opt…"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-aa56ba345f71 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-e1fbdbcf1f93 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-9207f9ff3b42 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-171205270104 — ⚠️ Metrics: Remove `ingress_upstream_latency_seconds`. (#11795) ⚠️ This metric has already been deprecated and is now getting removed

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L161-L163 (Changelog › All changes:): "* ⚠️ Metrics: Remove `ingress_upstream_latency_seconds`. (#11795) ⚠️ This metric has already been deprecated and is now getting removed."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-3252411b5cdd validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-47dcc729639a validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-1beddeea20f1 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-2324efa027c3 — Helm values section `podSecurityPolicy.*` removed (1 key)

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; declared breaking, action required
- subjects: podSecurityPolicy.enabled
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-2f174098a38e validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-585640787682 impact:values-unset (not-affected): Your values do not set `podSecurityPolicy.enabled` checked values: 13 facts against podSecurityP…
  - vr-5fa8e02a01f1 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-cf3502946c83 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-350afaa22e53 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-31d1b82d7ff3 — Default of Helm value `controller.admissionWebhooks.patch.image.tag` changed: "v1.5.2" → "v1.5.0"

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:computed-breaking
- subjects: controller.admissionWebhooks.patch.image.tag
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-675c735a0067 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-99ae4fb393c1 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-c6c5022f5f46 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: review-required (impact:values-default-applies)

### ci-384b14645d77 — Default of Helm value `controller.enableAnnotationValidations` changed: false → true

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:computed-breaking
- subjects: controller.enableAnnotationValidations
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-3669b2e01df4 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-0a80f59e4935 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-712015f73442 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: review-required (impact:values-default-applies)

### ci-3ef136dfe385 — ⚠️ Metrics: Disable by default

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L51-L55 (Changelog › All changes:): "* ⚠️ Metrics: Disable by default. (#12153) ⚠️ This changes the default of the following CLI arguments: * `--enable-metrics` gets disabled by default."
- environment rendered changes correlated to it: 1
  - rc-bdff3c8a666f container-arg-removed apps/v1/Deployment/ingress-nginx/ingress-nginx-controller spec.template.spec.containers[name=controller].args[=--enable-metrics]
- exposure roots: 1; impact paths: 2 (0 unresolved)
- deterministic validations already run:
  - vr-6845ce3579a7 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-eb85534684f9 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-13603a1ddc25 validate.static-reference@v1 → pass: 2 paths: every edge established, no placeholder, the complete environment renders (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) remove no provider on them
- deterministic join findings: unknown (impact:not-joined)

### ci-4488c165e878 — ⚠️ Remove 3rd party lua plugin support (#11821) ⚠️ This removes the following configuration options: * `plugins` It also removes support for user provided Lua…

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L152-L158 (Changelog › All changes:): "* ⚠️ Remove 3rd party lua plugin support (#11821) ⚠️ This removes the following configuration options: * `plugins` It also removes support for user provided Lua plugins in the `/etc/nginx/lua/plugins` directory."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-0562083857bb validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-98befa9f9ae9 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-15ce336e787c validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-48ef6809f0a2 — Helm value `controller.admissionWebhooks.patch.image.registry` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: controller.admissionWebhooks.patch.image.registry
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-85dc4ce42a2e validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-6cc4a830c221 impact:values-unset (not-affected): Your values do not set `controller.admissionWebhooks.patch.image.registry` checked values: 13 fa…
  - vr-5280115f9eb5 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-f168ee25db0a validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-4168def78a09 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-4c65407896f6 — Lua: Remove plugins from `.luacheckrc` & E2E docs

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:category
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L113-L113 (Changelog › All changes:): "* Lua: Remove plugins from `.luacheckrc` & E2E docs. (#11872)"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-d95ca8733168 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-f720cc275922 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-583d275af633 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-4c7ec4dac2e0 — ⚠️ Chart: Remove Pod Security Policy

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L92-L94 (Changelog › All changes:): "* ⚠️ Chart: Remove Pod Security Policy. (#11971) ⚠️ This removes Pod Security Policies and related resources from the chart."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-be1f78ab13ca validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-ff481903e627 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-222073b2b342 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-4de5841e5544 — Helm value `controller.admissionWebhooks.existingPsp` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: controller.admissionWebhooks.existingPsp
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-5bd722b4ad83 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-e9fa9b2991b9 impact:values-unset (not-affected): Your values do not set `controller.admissionWebhooks.existingPsp` checked values: 13 facts again…
  - vr-17bf9e11fc58 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-e3e96be6c62d validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-67d0ab4b00d2 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-60f8ebbe6d41 — NGINX: Remove inline Lua from template

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:category
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L98-L98 (Changelog › All changes:): "* NGINX: Remove inline Lua from template. (#11806)"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-cd76b6c9b800 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-8c3e612fec2d validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-9221736eee5e validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-63c266806f08 — ⚠️ Images: Remove OpenTelemetry

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L78-L80 (Changelog › All changes:): "* ⚠️ Images: Remove OpenTelemetry. (#12024) ⚠️ OpenTelemetry is still supported, but since the module is built into the controller image since v1.10, we hereby remove the init container and image which were used to install it upon c…"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-976d2a6f5f0c validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-f7ad88b5e90c validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-e962b9e83d22 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-6b97fdbe134e — remove modsecurity coreruleset test files from nginx image

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:category
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L198-L198 (Changelog › All changes:): "* remove modsecurity coreruleset test files from nginx image (#11617)"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-f5dc90fde33c validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-55e4e5f70768 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-9b01108a2983 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-7c156aebc9ef — Dashboard: Remove `ingress_upstream_latency_seconds`

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:category
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L114-L114 (Changelog › All changes:): "* Dashboard: Remove `ingress_upstream_latency_seconds`. (#11878)"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-0f802ec96a2f validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-16071a526abb validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-ac4a70c21ef9 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-8aeec1ddc400 — Default of Helm value `controller.admissionWebhooks.patch.image.digest` changed: "sha256:e8825994b7a2c7497375a9b945f386506ca6a3eda… → "sha256:aaafd456bda110628…

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:computed-breaking
- subjects: controller.admissionWebhooks.patch.image.digest
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-f16970b2c917 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-38effe3e8ae7 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-3b24a8313943 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: review-required (impact:values-default-applies)

### ci-8d404c73508c — ingress-nginx controller-v1.12.0 is end-of-life since 2026-03

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/README.md L7-L7 (Retiring): "* Best-effort maintenance will continue until March 2026."
  - https://github.com/kubernetes/ingress-nginx/blob/main/README.md L8-L8 (Retiring): "* Afterward, there will be no further releases, no bugfixes, and no updates to resolve any security vulnerabilities that may be discovered."
  - https://github.com/kubernetes/ingress-nginx/blob/main/README.md L9-L10 (Retiring): "* Existing deployments of Ingress NGINX will not be broken. * Existing project artifacts such as Helm charts and container images will remain available."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-3ba99f2f41ac validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-8bce9e6543ca validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-98055d350d79 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-9809e2b921b9 — Helm value `defaultBackend.image.registry` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: defaultBackend.image.registry
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-a174eb2ceacd validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-ee2f232ff7cd impact:values-unset (not-affected): Your values do not set `defaultBackend.image.registry` checked values: 13 facts against defaultB…
  - vr-94b726c65e32 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-d1d80010bc2b validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-96d682096b97 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-a69a9cd8d642 — Helm value `controller.existingPsp` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: controller.existingPsp
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-d295d7a2a5ca validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-4f4534eadc4e impact:values-unset (not-affected): Your values do not set `controller.existingPsp` checked values: 13 facts against controller.exis…
  - vr-79fca5632e09 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-2e8f9911b484 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-35075d65f5f6 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-b1cb053e0a10 — Helm value `defaultBackend.existingPsp` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: defaultBackend.existingPsp
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-1a6faabdf4e1 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-034f85368243 impact:values-unset (not-affected): Your values do not set `defaultBackend.existingPsp` checked values: 13 facts against defaultBack…
  - vr-9b38d1ec2b36 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-5fea04c2b257 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-2bdf4b42e206 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-be38f85ce33a — Helm value `controller.image.registry` removed

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: controller.image.registry
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-ce4b080789bf validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-e84d0eb358e5 impact:values-unset (not-affected): Your values do not set `controller.image.registry` checked values: 13 facts against controller.i…
  - vr-360248c9d456 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-4ed9a0adc8ec validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-b01fdfee78b5 validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-d39c9c8701fd — Kubernetes (supported): 1.26–1.30 → 1.28–1.32; drops 1.26–1.27, adds 1.31–1.32

- family `compatibility`, manifestation `rendered-config` (manifest:computed-compat), scope scope:breaking; action required
- subjects: kubernetes
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/README.md L83: "| Supported | Ingress-NGINX version | k8s supported version | Alpine Version | Nginx Version | Helm Chart Version | | | v1.11.5 | 1.30, 1.29, 1.28, 1.27, 1.26 | 3.21.3 | 1.25.5 | 4.11.5 |"
  - https://github.com/kubernetes/ingress-nginx/blob/main/README.md L78: "| Supported | Ingress-NGINX version | k8s supported version | Alpine Version | Nginx Version | Helm Chart Version | | | v1.12.0 | 1.32, 1.31, 1.30, 1.29, 1.28 | 3.21.0 | 1.25.5 | 4.12.0 |"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-255d1febbfea validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-d55868f8741c impact:compatibility-satisfied (not-affected): Cluster Kubernetes 1.31 satisfies the kubernetes chart-kubeVersion constraint (≥ 1.…
  - vr-35733051eb44 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-6981c9dd6dd1 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-77f35ace5fa4 validate.schema-compat@v1 → pass: cluster kubernetes 1.31 is inside the target's chart-kubeVersion range ≥ 1.21
  - vr-90c07b015297 validate.static-reference@v1 → not-run: the item has no impact paths to check

### ci-d952797413b3 — Helm values `controller.opentelemetry.*` removed (14 keys)

- family `values`, manifestation `rendered-config` (manifest:computed-values), scope scope:breaking; action required
- subjects: controller.opentelemetry.containerSecurityContext.allowPrivilegeEscalation, controller.opentelemetry.containerSecurityContext.capabilities.drop, controller.opentelemetry.containerSecurityContext.readOnlyRootFilesystem, controller.opentelemetry.containerSecurityContext.runAsNonRoot, controller.opentelemetry.containerSecurityContext.runAsUser, controller.opentelemetry.containerSecurityContext.seccompProfile.type, controller.opentelemetry.enabled, controller.opentelemetry.image.digest, controller.opentelemetry.image.distroless, controller.opentelemetry.image.image, controller.opentelemetry.image.registry, controller.opentelemetry.image.tag, controller.opentelemetry.name, controller.opentelemetry.resources
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.12.0/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
  - https://github.com/kubernetes/ingress-nginx/blob/helm-chart-4.11.5/charts/ingress-nginx/values.yaml charts/ingress-nginx/values.yaml: "## nginx configuration"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-5bc8e83984c0 validate.join@v1 → pass: NOT AFFECTED by the deterministic join (evaluated against the environment, with checks): - imp-dfca6b897117 impact:values-unset (not-affected): Your values do not touch `controller.opentelemetry.containerSecurityContext.allowPrivilegeEscala…
  - vr-c96afd60d4ee validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-04955707d209 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-e00fbcf7eeef validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: not-affected (impact:values-unset)

### ci-da8be15fb3f7 — ⚠️ Remove global-rate-limit feature (#11851) ⚠️ This removes the following configuration options: * `global-rate-limit-memcached-host` *…

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L117-L133 (Changelog › All changes:): "* ⚠️ Remove global-rate-limit feature (#11851) ⚠️ This removes the following configuration options: * `global-rate-limit-memcached-host` * `global-rate-limit-memcached-port` * `global-rate-limit-memcached-connect-timeout` * `global-…"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-4a39c2c603a0 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-857466ac3b4f validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-d7f78f1f364f validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-e5c06d26ee3a — field-removed: core/v1/ConfigMap/ingress-nginx/ingress-nginx-controller data (not mentioned by any change)

- family `rendered`, manifestation `rendered-config` (manifest:rendered-anchor), scope scope:rendered-unexplained
- subjects: core/v1/ConfigMap/ingress-nginx/ingress-nginx-controller
- upstream evidence:
  - ri:render@release/chart-defaults controller-v1.11.5 → controller-v1.12.0: "8 rendered changes between the chart defaults of controller-v1.11.5 and controller-v1.12.0"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-806cd2e95df8 validate.render-diff@v1 → pass: every environment render is complete (argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]) and nothing in it is attributable to the item
  - vr-1854beef9145 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-3c8f8e838e10 validate.static-reference@v1 → not-run: the item has no impact paths to check

### ci-ec5418d923ab — Replace deprecated queue method

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:category
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L138-L138 (Changelog › All changes:): "* Replace deprecated queue method (#11853)"
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-cd82f93e5d6a validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-b775a33e7ea1 validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-cf067696bf2f validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

### ci-f22db82d8154 — ⚠️ Images: Drop `s390x`. (#12137) ⚠️ Support for the `s390x` architecture has already been removed from the controller image

- family `behavior`, manifestation `controller-behaviour` (manifest:declared-behaviour), scope scope:breaking; declared breaking, action required
- upstream evidence:
  - https://github.com/kubernetes/ingress-nginx/blob/main/changelog/controller-1.12.0.md L59-L61 (Changelog › All changes:): "* ⚠️ Images: Drop `s390x`. (#12137) ⚠️ Support for the `s390x` architecture has already been removed from the controller image. This also removes it from the NGINX base image and CI relevant images."
- environment rendered changes correlated to it: 0
- exposure roots: 0; impact paths: 0 (0 unresolved)
- deterministic validations already run:
  - vr-490f9d9f43e7 validate.render-diff@v1 → not-run: the item manifests as controller-behaviour (manifest:declared-behaviour): its effect is compiled into the shipped software or applied server-side, so render silence proves nothing — an explicit pinned setting (override inspection) or an o…
  - vr-af886eb9b42f validate.override-inspection@v1 → not-run: no declarative pin rule names a setting that pins the old behaviour of this item (consulted: the join's pinned verdicts and trusted verified facts): - no verified fact is attached to the item
  - vr-f259fb77607b validate.static-reference@v1 → not-run: the item has no impact paths to check
- deterministic join findings: unknown (impact:not-joined)

## Renders

- environment `argocd:clusters/prod/ingress-nginx.yaml#0/$.spec.sources[0]`: ok, 7 changes, 0 uncorrelated
- environment `kustomize:apps/catalog-api/overlays/prod`: not-applicable, 0 changes, 0 uncorrelated — version-independent: the overlay neither pins nor deploys ingress-nginx (no image or remote resource carries its version, no object runs its images), so the upgrade does not change what it renders: overlay apps/catalog-api/overlays/prod doe…
- environment `kustomize:apps/storefront/overlays/prod`: not-applicable, 0 changes, 0 uncorrelated — version-independent: the overlay neither pins nor deploys ingress-nginx (no image or remote resource carries its version, no object runs its images), so the upgrade does not change what it renders: overlay apps/storefront/overlays/prod does…
- release `chart-defaults`: ok, 8 changes, 0 uncorrelated

## Uncorrelated environment rendered changes (0)

- none

## Unresolved relationships (0)

- none

48 further change items are out of scope (routine or not breaking); they are not classified.

## Your job

1. For each in-scope item, call `get_change_item` and read the upstream text. Decide where it takes
   effect and which explicit setting controls it.
2. Examine the environment's rendered changes for the item (`query_diff`, `get_rendered_object`) and
   the environment's own files (`search_manifest`, `read_artifact`). For `controller-behaviour` and
   `unknown` items, render silence proves nothing: find the setting that pins the old behaviour, or cite
   that the environment does not set it.
3. Explain the uncorrelated rendered changes if you can: which change item, if any, they belong to.
4. Record what you find (`record_hypothesis`, `record_assertion`, with verbatim citations) and request
   the validations that would decide each item (`request_validation`; `request_evidence` for what is
   missing).
5. Finish with `submit_findings`: one closing summary per item you investigated.

Budget: 80 tool calls, 4096 KiB returned, 40 turns, 1200 s.
