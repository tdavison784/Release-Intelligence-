# ingress-nginx controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) — shop-prod-fr (group `security-defaults`, not-affected half)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and an
observed upgrade in a throwaway kind cluster. No Release Intelligence pipeline, agent or eval output
was run or read. Siblings: `--shop-de` (identical except one value), `--shop-pl`.

## The environment

A GitOps repo (Argo CD app-of-apps, `env/clusters/prod`) for a web shop. ingress-nginx comes from
the upstream chart with two value files in order (`platform/ingress-nginx/values.yaml`, shared:
`allowSnippetAnnotations: true`, a few `controller.config` keys; `values-prod.yaml`: replicas,
resources, `proxy-read-timeout`). Two apps are exposed:

- `apps/storefront`: Ingress `shop.example.com` with `proxy-body-size` in the base; the **prod overlay
  patch `ingress-headers.yaml` adds a `configuration-snippet`** setting HSTS and X-Frame-Options.
- `apps/catalog-api`: Ingress `api.shop.example.com/v1` with `proxy-read-timeout` and `enable-cors`.

The demo backends are `hashicorp/http-echo` (answers 200 with the app name on every path), so a
404 can only come from the controller's default backend.

**Difference from shop-de** (`diff -r` of the env trees shows only this plus names):
`platform/ingress-nginx/values-prod.yaml` sets `controller.config.annotations-risk-level: Critical`.
On 1.11.5 that equals the default (a no-op); the chart renders it into the controller ConfigMap.

## Upstream sources

1. Release controller-v1.12.0, release body (the "⚠️ Enable security features by default (#11819)"
   entry and its four bullets): https://github.com/kubernetes/ingress-nginx/releases/tag/controller-v1.12.0
2. PR #11819 "Enable security features by default", merge commit 7b4e4e2f:
   https://github.com/kubernetes/ingress-nginx/pull/11819 ("Lowers the acceptable annotation risk from
   Critical to High / Disables cross namespace consumption / Enables strict path validation / Also makes
   validation enabled by default"); its diff flips `AllowCrossNamespaceResources`, `AnnotationsRiskLevel`
   and `StrictValidatePathType` in `internal/ingress/controller/config/config.go` and the
   `--enable-annotation-validation` flag default in `pkg/flags/flags.go`.
3. Source at tag controller-v1.12.0 (https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/...):
   - `internal/ingress/controller/config/config.go` L97-L106 (semantics of
     allow-cross-namespace-resources and annotations-risk-level), L726-L730 (strict-validate-path-type),
     L753-L757 and L897 (the new defaults).
   - `internal/ingress/annotations/snippet/main.go` L36 (configuration-snippet is
     `AnnotationRiskCritical`), L67-L68 (checked against the configured level).
   - `internal/ingress/annotations/parser/validators.go` L246-L255 (`CheckAnnotationRisk`: "annotation
     %s is too risky for environment" — runs whether or not value validation is enabled) and L234-L240
     (value validators run only `if EnableAnnotationValidation`).
   - `internal/ingress/annotations/annotations.go` L185-L188 (`Extract` returns an error for the whole
     Ingress when any annotation group fails `Validate`) and
     `internal/ingress/controller/store/store.go` L936-L940 (on that error `syncIngress` returns
     without storing the Ingress: the controller stops serving it).
   - Risk of the other annotations used: `proxy/main.go` (proxy-read-timeout Low, proxy-body-size
     Medium), `cors/main.go` (enable-cors Low).
4. Chart 4.12.0 vs 4.11.5 (`helm show values`, templates): `controller.enableAnnotationValidations`
   default false → true; `templates/_params.tpl` renders `--enable-annotation-validation` only when it
   deviates from the chart default (4.11: `=true` when enabled; 4.12: `=false` when disabled);
   `controller-configmap.yaml` renders `allow-snippet-annotations` and `controller.config` verbatim and
   sets no risk level of its own.
5. Field reports of the same mechanism after upgrading to 1.12 (whole Ingress dropped by a failing
   annotation check): https://github.com/kubernetes/ingress-nginx/issues/13092 (validation) and
   https://github.com/kubernetes/ingress-nginx/issues/12764 (auth-signin value rejected).

Chart tarballs (https://kubernetes.github.io/ingress-nginx): ingress-nginx-4.11.5.tgz sha256
`7035b6656a07f85792d0d8e103b9bff84510db0e81d11db4e322709b07783ffa` (controller v1.11.5),
ingress-nginx-4.12.0.tgz sha256 `6445180ad70a12ed91d1430c7b54ed0cc65be6adfc534d23d5cb01c79734cff9`
(controller v1.12.0).
## Reasoning (before the run)

- B1: the storefront Ingress still carries the Critical `configuration-snippet`, but the configured
  level is Critical, so `CheckAnnotationRisk` passes on 1.12.0 as on 1.11.5. To see this a system
  has to follow the Application's second value file (values files merge in order) into the rendered
  ConfigMap.
- B2–B4 as in shop-de.

## Observed outcome (kind, 2026-10-16) — `observed/run.log`, script `observed/run.sh`

Commands: `observed/run.sh <case> ri-groundtruth-in-<x> <charts>`; kind v0.24.0, Kubernetes v1.31.0.
Argo CD is not run: the ingress-nginx Application is resolved into `helm upgrade --install` with
the chart and both value files in order, the app Applications into `kubectl apply -k` of their
paths. For kind only, requests go from an in-cluster curl pod to the controller Service (no external
LoadBalancer) and helm runs without `--wait`. Phase 2 stands in for Argo CD's next re-sync: `kubectl
replace --dry-run=server` sends each Ingress through the admission webhook without persisting it.
Cluster deleted at the end.

| Phase | Observation (excerpts from run.log) |
|---|---|
| 0 chart 4.11.5 | as shop-de: both hosts 200, storefront with both headers |
| 1 chart 4.12.0 | controller `v1.12.0`; ConfigMap data `"annotations-risk-level":"Critical"`; **`shop.example.com/ -> 200 OK, body: storefront`, both headers present**; `api.shop.example.com/v1 -> 200 OK`; nginx.conf still has the `shop.example.com` server block and the `more_set_headers` lines |
| 2 re-sync | both Ingresses `replaced (server dry run)` — admitted |

`observed/render.diff` is identical to shop-de's except context: the risk-level key is rendered in
the ConfigMap in both versions (no diff line), so the mitigation is visible in the rendered *state*,
not in the diff.

## Judgement calls

- B1 `not-affected` via an established override (values-prod.yaml), observed.
- With the default flipped, the override now carries a real security trade-off (it keeps accepting
  arbitrary snippets). That is a decision for the operator, already recorded in the repo, not an
  impact of the upgrade on this environment.
- B2–B4 `not-affected`, observed for both Ingresses (served and admitted on 1.12.0 with strict path
  validation and annotation validation on by default).
