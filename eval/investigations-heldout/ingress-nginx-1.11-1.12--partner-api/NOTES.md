# ingress-nginx-1.11-1.12--partner-api — notes (ground truth; not for systems under test)

Not paired. A behavioural break with no rendered counterpart and no error anywhere (B1), next to a
release change that the environment's values neutralise and that shows up as a harmless rendered
change (B2).

## Upstream sources

- Changelog controller-v1.12.0,
  [`changelog/controller-1.12.0.md#L117-L133`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/changelog/controller-1.12.0.md#L117-L133)
  (one entry covers all global-rate-limit options and annotations):
  > ⚠️ Remove global-rate-limit feature (#11851) ⚠️ This removes the following configuration options:
  > `global-rate-limit-memcached-host` `global-rate-limit-memcached-port` `global-rate-limit-memcached-connect-timeout`
  > `global-rate-limit-memcached-max-idle-timeout` `global-rate-limit-memcached-pool-size` `global-rate-limit-status-code`
  > It also removes the following annotations: `global-rate-limit` `global-rate-limit-window` `global-rate-limit-key`
  > `global-rate-limit-ignored-cidrs`
- What the feature did, controller-v1.11.5 (`795b6964`)
  [`docs/user-guide/nginx-configuration/annotations.md#L580-L613`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.11.5/docs/user-guide/nginx-configuration/annotations.md#L580-L613):
  > Global Rate Limiting overcome this by using lua-resty-global-throttle. lua-resty-global-throttle shares
  > its counters via a central store such as memcached. ... In case of an error it will log the error message
  > and **fail open**. ... `nginx.ingress.kubernetes.io/global-rate-limit`: Configures maximum allowed number
  > of requests per window. Required.

  Implementation in v1.11.5: `internal/ingress/annotations/globalratelimit/main.go` (L37-L40 the four
  annotations), `rootfs/etc/nginx/lua/global_throttle.lua`, called from `lua_ingress.lua` L167. In
  controller-v1.12.0 (`8ee43842`) neither the annotation package nor `global_throttle` exists any more
  (`internal/ingress/annotations/`, `rootfs/etc/nginx/`).
- Unknown annotations are not rejected: the admission check
  ([`controller.go#L313-L392`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/internal/ingress/controller/controller.go#L313-L392))
  validates the annotations known to the annotation extractor (risk level, values) and the blocklist;
  the removed ones are simply no longer parsed.
- Metrics, [`changelog/controller-1.12.0.md#L51-L55`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/changelog/controller-1.12.0.md#L51-L55):
  > ⚠️ Metrics: Disable by default. (#12153) ⚠️ ... `--enable-metrics` gets disabled by default.

  `pkg/flags/flags.go` L177 (v1.12.0) `enable-metrics` default `false` (v1.11.5 L176: `true`). Chart 4.12.0
  renders `--enable-metrics={{ .Values.controller.metrics.enabled }}`: with `controller.metrics.enabled:
  true` the flag is passed explicitly (render.diff L102-L109).

## Reasoning in terms of the fixture

- `apps/partner-api/base/ingress.yaml`: `global-rate-limit: "120"`, `-window: 1m`, `-key:
  $http_x_partner_id`; `platform/ingress-nginx/values.yaml` points the controller at
  `memcached.ingress-nginx.svc` (`platform/memcached`), three controller replicas. The README states the
  quota is contractual.
- On v1.12.0 the annotations and configuration keys are still present and still applied, but nothing
  implements them: the quota silently stops being enforced (the documented behaviour on error is
  fail-open as well, but here there is not even an error). **B1 is affected** — a security/contract
  control is lost; there is no rendered change and no failed sync to notice it by.
- B2: `controller.metrics.enabled: true` → the explicit flag keeps the endpoint on; the pod annotations
  keep pointing at it. **Not affected**; its rendered change is harmless.

## Observed outcome

`observed/run.sh` (kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1), log `observed/run.log`. Argo CD's
sync waves reproduced in order (memcached, ingress-nginx with the Application's value file — helm
without `--wait` because a LoadBalancer Service never gets an address in kind —, partner-api). A probe
pod sends bursts through the controller Service with `Host: api.partners.example.com`.

| Step (run.log) | burst (requests from one partner) | status codes | `global_throttle` in nginx.conf | metrics endpoint |
|---|---|---|---|---|
| v1.11.5 (L116) | 130 from partner-a | **120 × 200, 10 × 429** | 4 lines | serving (126 series) |
| v1.11.5 | 20 from partner-b | 20 × 200 | | |
| upgrade to v1.12.0 (L125); ConfigMap still has the `global-rate-limit-*` keys (L191) | | | | |
| v1.12.0 (L193) | 130 from partner-c | **130 × 200** | | |
| v1.12.0, after the window expired (L197) | 130 from partner-a | **130 × 200** | **0** (L208) | serving (121 series) |
| v1.12.0 (L202) | annotation change on the Ingress | admitted | | |

`observed/render.diff`: ingress-nginx release FROM/TO; the ConfigMap's `global-rate-limit-*` keys and the
Ingress are identical at FROM and TO. All rendered changes are harmless.
