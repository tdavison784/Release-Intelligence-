# ingress-nginx controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) — shop-prod-nl (group `chart-cleanup`, diff-visible but harmless)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and an
observed upgrade in a throwaway kind cluster. No Release Intelligence pipeline, agent or eval output
was run or read. Purpose: a false-positive trap for diff-driven investigation — the environment's
rendered controller Deployment changes visibly, and none of it matters.

## The environment

Same shop layout as shop-de/fr (storefront, catalog-api; no snippets, no cross-namespace
references), plus:

- `platform/ingress-nginx/values.yaml`: OpenTelemetry tracing on (`controller.opentelemetry.enabled:
  true`, `controller.config.enable-opentelemetry: "true"`, collector host/port, service name,
  AlwaysOn sampler) and metrics on (`controller.metrics.enabled: true`, scraped by an external
  Prometheus through `prometheus.io/*` pod annotations).
- `platform/otel-collector/`: an in-cluster OpenTelemetry collector (OTLP gRPC receiver, debug
  exporter) that receives the controller's spans; Application `otel-collector` (sync wave -1).

## Upstream sources

1. Release controller-v1.12.0, release body:
   https://github.com/kubernetes/ingress-nginx/releases/tag/controller-v1.12.0
   - "⚠️ Images: Remove OpenTelemetry. (#12024) ⚠️ OpenTelemetry is still supported, but since the
     module is built into the controller image since v1.10, we hereby remove the init container and
     image which were used to install it upon controller startup."
   - "⚠️ Metrics: Disable by default. (#12153) ⚠️ … `--enable-metrics` gets disabled by default."
2. PR #12024 (https://github.com/kubernetes/ingress-nginx/pull/12024): "This removal will not change
   the way OpenTelemetry works."; touches `charts/ingress-nginx/values.yaml` and the controller
   Deployment/DaemonSet templates.
3. nginx template at both tags, `rootfs/etc/nginx/template/nginx.tmpl` L33:
   `load_module /etc/nginx/modules/otel_ngx_module.so;` (identical in controller-v1.11.5 and
   controller-v1.12.0; neither references `/modules_mount`), L64 `opentelemetry_config …`.
4. `pkg/flags/flags.go`: controller-v1.11.5 L176-L177 `flags.Bool("enable-metrics", true, …)`;
   controller-v1.12.0 L177-L178 `flags.Bool("enable-metrics", false, …)`.
5. Chart templates `_params.tpl` L57-L58: 4.11.5 `{{- if not .Values.controller.metrics.enabled }}
   - --enable-metrics=…`; helm-chart-4.12.0 `{{- if .Values.controller.metrics.enabled }}
   - --enable-metrics=…` — the chart compensates for the controller default flip.
6. Controller default `AllowSnippetAnnotations: false` in both versions (config.go L781 at
   controller-v1.11.5, L753 at controller-v1.12.0) — relevant to the third rendered change below.

Chart tarballs: as in the shop-de case (sha256 7035b665… for 4.11.5, 6445180a… for 4.12.0).

## Reasoning (before the run)

- B1: the rendered diff removes the init container and the `modules` volume. The only consumer
  would be an nginx `load_module` from `/modules_mount`; neither template has one, so the init
  container's copy was already unused. Tracing should keep working.
- B2: metrics stay on: 4.11 relied on the 1.11 default (true), 4.12 passes `--enable-metrics=true`.
- The ConfigMap loses `allow-snippet-annotations: "false"` (4.12 renders the key only when true);
  the controller default is false in both versions. Not an in-scope item; listed in
  `renderedChanges` as harmless.

## Observed outcome (kind, 2026-10-16) — `observed/run.log`, script `observed/run.sh`

kind v0.24.0, Kubernetes v1.31.0. Argo CD resolved by hand (otel-collector `kubectl apply -k`,
ingress-nginx `helm upgrade --install` with both value files, apps `kubectl apply -k`); requests
from an in-cluster client pod to the controller Service; cluster deleted at the end.

| Check | chart 4.11.5 / controller 1.11.5 | chart 4.12.0 / controller 1.12.0 |
|---|---|---|
| controller pods | `init=[opentelemetry] volumes=[modules webhook-cert …] metrics-flag=[]` | `init=[] volumes=[webhook-cert …] metrics-flag=[enable-metrics=true]` |
| nginx.conf | `load_module /etc/nginx/modules/otel_ngx_module.so;`, `opentelemetry_config …`, `opentelemetry on;` per location | identical lines |
| spans at the collector for the 4 probe requests | 4 × `service.name: Str(ingress-nginx)` | 4 × `service.name: Str(ingress-nginx)` |
| `:10254/metrics` from a pod (annotation scraper's view) | HTTP 200, `nginx_ingress_controller_requests` series: 2 | HTTP 200, series: 2 |
| `shop.example.com/`, `api.shop.example.com/v1` | 200 / 200 | 200 / 200 |
| re-sync (`kubectl replace --dry-run=server`) | — | both Ingresses admitted |

`observed/render.diff` (the environment's FROM/TO render): L45 ConfigMap `allow-snippet-annotations`
removed, L179 `--enable-metrics=true` added, L191-L224 init container + volume + mount removed,
plus image digests and `runAsGroup` additions. Every one of these is labelled in
`expected.yaml#renderedChanges`.

## Judgement calls

- Both items `not-affected`, observed. A diff-driven system that reports "init container removed" or
  "metrics flag changed" as breaking for this environment is producing false positives; one that
  reports them as rendered changes with no impact, citing why, is right.
- B2's mitigation is labelled `override` (`controller.metrics.enabled: true`): with metrics left at
  the chart default (false) the diff would instead show `--enable-metrics=false` disappearing — also
  harmless, as seen in the shop-de/fr renders.
- The security-default items of the same release (#11819) also apply to this cluster; they are out
  of scope for this case (no snippets, no cross-namespace references, plain paths — see shop-de/fr
  for their ground truth).
