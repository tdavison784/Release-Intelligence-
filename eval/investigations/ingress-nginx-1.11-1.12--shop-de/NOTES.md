# ingress-nginx controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) — shop-prod-de (group `security-defaults`, affected half)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and an
observed upgrade in a throwaway kind cluster. No Release Intelligence pipeline, agent or eval output
was run or read. Siblings: `--shop-fr` (identical except one value), `--shop-pl` (value files withheld).

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

Neither value file sets `annotations-risk-level`.

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

- B1: the storefront Ingress, after the prod overlay, carries `configuration-snippet` (Critical). On
  1.11.5 the default level is Critical, so it is accepted (snippets are allowed by the values). On
  1.12.0 the default is High and nothing in the value files raises it, so `CheckAnnotationRisk` fails,
  `Extract` fails for the whole Ingress, and `syncIngress` drops it: `shop.example.com` stops being
  served, not just the headers. catalog-api only uses Low-risk annotations.
- B2: no annotation in either Ingress references a Secret/ConfigMap/Service in another namespace.
- B3: paths are `/` and `/v1` with `pathType: Prefix`; they satisfy the strict rule.
- B4: the validated annotation values are well-formed (`16m`, `30`, `true`); the snippet is not
  value-validated, and its rejection is the risk check (B1), which runs regardless of B4.

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
| 0 chart 4.11.5 | controller `v1.11.5`; `shop.example.com/ -> 200 OK, body: storefront, Strict-Transport-Security…, X-Frame-Options: DENY`; `api.shop.example.com/v1 -> 200 OK, body: catalog-api`; nginx.conf has server blocks for both hosts and the two `more_set_headers` lines |
| 1 chart 4.12.0 | controller `v1.12.0`; controller log `E… store.go:938] annotation group ConfigurationSnippet contains risky annotation based on ingress configuration`; **`shop.example.com/ -> 404 Not Found`** (default backend); `api.shop.example.com/v1 -> 200 OK`; nginx.conf has **no `shop.example.com` server block** |
| 2 re-sync | storefront: **`admission webhook "validate.nginx.ingress.kubernetes.io" denied the request: annotation group ConfigurationSnippet contains risky annotation based on ingress configuration`**; catalog-api: `replaced (server dry run)` |

Earlier runs the same day (script versions with an nginx backend and server-side apply) showed the
same storefront outcome; the backend was switched to http-echo because nginx's own 404 on `/v1`
made the catalog-api probe ambiguous.

`observed/render.diff` (helm template 4.11.5 vs 4.12.0 with both value files): controller and
certgen images, `runAsGroup` additions, and `- --enable-metrics=false` disappearing from the
controller args (the chart now passes the flag only when metrics are enabled, matching the new
controller default). **Nothing about the risk level, cross-namespace resources, path validation or
annotation validation appears in the rendered manifests**: all four B items are binary defaults; the
rendered controller ConfigMap is identical before and after.

## Judgement calls

- B1 `affected`, observed. The break is invisible in the rendered diff and in the app manifests'
  base files; it needs (overlay patch → Critical annotation) + (value files lack a risk level) +
  (upstream default change). That is the intended difficulty.
- The `--enable-metrics=false` removal is a rendered change with no behaviour change (metrics were
  off and stay off). It is not an in-scope item; a system flagging it as breaking for this
  environment is wrong, but the dataset does not score it.
- B2–B4 are `not-affected`; B3/B4 for the storefront Ingress rely on the shop-fr run, where the same
  Ingress passes the 1.12 path and value checks once B1 is neutralised (here it is dropped by B1
  before those checks matter).
