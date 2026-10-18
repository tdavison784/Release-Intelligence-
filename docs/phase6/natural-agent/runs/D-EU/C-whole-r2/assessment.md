# ingress-nginx 4.11.5 (v1.11.5) -> 4.12.0 (v1.12.0): assessment for this environment

## Verdict

**No documented breaking change appears to break this environment, based on static evidence. The upgrade is not recommended as framed, though, because it moves backwards in patch level.**

1. **It is a security regression.** v1.11.5 is newer than v1.12.0. The 1.11.5 changelog contains "Controller: Several security fixes (#13070)" (`changelog/controller-1.11.5.md:13`). The 1.12.0 changelog has no such entry.
   - The README support table lists v1.12.1 through v1.12.8 as later releases.
   - From memory, not from the repo: those #13070 fixes are the March 2025 admission-webhook CVE batch (CVE-2025-1974 and related). Verify this against the advisory.
   - `admissionWebhooks.enabled: true` is set in `values.yaml`, so the exposure would apply here.
   - Per the README, the project is retired (best-effort maintenance only until March 2026). Today is 2026-10-18, so no further fixes are coming at all.
2. **The chart also downgrades its helper image.** `kube-webhook-certgen` goes from v1.5.2 to v1.5.0 (`rendered/manifest.diff:87,126`). The 4.11.5 chart carries the later certgen bump (#13067, #12697) and 4.12.0 predates it.

If the goal is to move forward, the better target is a later 1.12.x or 1.13/1.14 release, or a move off ingress-nginx. Everything below applies to 1.12.0 specifically, and I expect it to carry over to those releases, but I haven't checked them.

## Documented breaking changes versus this environment

| Change in 1.12.0 | Affects this env? | Why / evidence |
|---|---|---|
| Strict path-type validation on by default (`strict-validate-path-type`) | **Probably not; needs a dry-run.** | The only Ingress (`apps/docs/base/ingress.yaml`) uses `/` Prefix and `/.well-known` Prefix. As I remember the validation, it only allows alphanumerics and `. _ - /` in Exact/Prefix paths. `.well-known` contains only those characters. The validator source is not in the repo and I haven't confirmed the exact regex. Also confirm `/` is accepted; it is ubiquitous, so I expect so. |
| Annotation validation on, risk level lowered to High | **No.** | The only annotation is `ssl-redirect: "false"`, which is low risk. No snippet or other high or critical annotations exist anywhere in `env/`. |
| `allow-cross-namespace-resources` off | **No.** | No auth-secret, auth-tls or similar cross-namespace references exist. The Ingress and both backends are all in `docs`. |
| Metrics disabled by default (`--enable-metrics`) | **No behaviour change.** | The chart rendered `--enable-metrics=false` for 4.11.5 because `controller.metrics.enabled` defaults to false (4.11.5 `values.yaml:858`). 4.12.0 drops the flag (`manifest.diff:40`), which is now equivalent. No ServiceMonitor or metrics values exist in `env/`. Whether anything outside the repo scrapes it is unknown (see below). |
| `ingress_upstream_latency_seconds` removed | **Unknown.** | Nothing in the repo references it. External dashboards or alerts could. |
| Global-rate-limit options and annotations removed | **No.** | None are used in `config:` or the Ingress. |
| Lua `plugins` option removed | **No.** | Not set. |
| Chart: Pod Security Policy removed | **No.** | The cluster is Kubernetes 1.31, where PSP no longer exists, and no PSP values are set. |
| Init container and image for OpenTelemetry removed | **No.** | OTel is not enabled in the values. |
| `s390x` dropped | **Probably not.** | The node architecture is not declared in the repo. Confirm the nodes are not s390x. |
| Kubernetes version support | **No.** | The README matrix lists 1.31 as supported for v1.12.x (1.28 to 1.32). The 4.12.0 render shows no API changes. |

## Rendered diff review (`rendered/manifest.diff`)

Apart from the version labels, the diff has only these functional changes:

- Controller image: v1.11.5 -> v1.12.0 (digest pinned).
- `--enable-metrics=false` argument removed. Equivalent, per above.
- `allow-snippet-annotations: "false"` removed from the ConfigMap. The 1.12 chart template only emits the key when true, and `allowSnippetAnnotations` is still false in `values.yaml`. The Ingress uses no snippets either way, so it has no effect here.
- `runAsGroup: 82` added to the controller, and `runAsGroup: 65532` to the certgen jobs.
- certgen image downgrade (above).

Unchanged elements:

- Service: LoadBalancer with `externalTrafficPolicy: Local`.
- IngressClass `nginx` (default).
- Webhook configuration.
- ConfigMap keys `use-forwarded-headers`, `proxy-body-size` and `hsts-max-age`.
- RBAC.

**Unchanged manifests do not prove runtime compatibility.** The image contents change (Alpine 3.20 -> 3.21 per the changelog, NGINX base 1.1.0, Lua inlined into the template removed, a new `runAsGroup`), and the defaults baked into the binary change.

## The `/.well-known` path and its consumers

This was the one relationship worth tracing.

- The Ingress sends `/.well-known` to the `wellknown` Service and `/` to `docs-site`.
- The wellknown pod serves a ConfigMap mounted at `/usr/share/nginx/html/.well-known`. It holds only `security.txt`.
- The ingress-nginx path matching itself is not touched by the 1.12.0 changelog. The only risk is the new strict validation, covered above.
- If validation did reject the path, the failure would be silent and public-facing. With the webhook enabled, a changed Ingress would be rejected at apply time. An existing Ingress that is not re-applied would instead be rejected or ignored when the controller syncs it. Either way the `/.well-known` URLs (`security.txt`, which is a security-contact document) could stop working or fall through to `docs-site`.
- Separate from the upgrade: `security.txt` has `Expires: 2027-06-30`, which is still valid.

## Operational risks

- **Controller `runAsGroup: 82`.** This is new. Files the nginx user (UID 101) must write at runtime are normally owned by 101. I believe the group change is harmless, but it is untested here. The image is non-root with `readOnlyRootFilesystem: false`.
- **Rollout.** The controller runs 2 replicas, so a rolling update is possible. With `externalTrafficPolicy: Local`, check that no node briefly has zero ready controller pods while the load balancer still sends it traffic.
- **Argo CD wiring.** The certgen Jobs are Helm hooks, which Argo treats as PreSync hooks. Check that they re-run cleanly with the older certgen image against the existing webhook secret.
- **Rollback.** Pinning `targetRevision: 4.11.5` again is straightforward, but ConfigMap, flags and the certgen image would flip again.

## Missing information and tests needed before proceeding

1. **Confirm the security status.** Check the advisory for the #13070 fixes and the real fix versions. Decide whether to target a later 1.12.x or newer release, or a replacement, instead of 1.12.0. This is the main blocker.
2. **Strict path validation.** On a non-prod cluster running v1.12.0, `kubectl apply --dry-run=server` the rendered docs Ingress, then curl `/`, `/.well-known/security.txt` and `/index.html`. Look at the controller logs for validation errors.
3. **Pre-flight on prod.** Run `kubectl get ingress -A -o yaml` and verify that `apps/docs` really is the only Ingress, as the orientation states. The repo is declared as complete, but one stray Ingress using a snippet, cross-namespace secret or odd path would change this assessment.
4. **Metrics consumers.** Check for external Prometheus, dashboards or alerts that depend on the controller metrics or on `ingress_upstream_latency_seconds`. Also check whether anything scrapes the controller and expects metrics to be on.
5. **Node architecture** (s390x) and the cluster's scanner or admission policy for the older certgen image.
6. **Rollout rehearsal.** Run the full Argo sync in staging, including the certgen hooks and webhook CA patch. Watch for 5xx errors during the controller rollout.
7. **Post-upgrade smoke tests.** Check Host-based routing for `docs.example.com`, HSTS and forwarded-header behaviour (`use-forwarded-headers`), the 8m body limit, and the admission webhook's response to a deliberately invalid Ingress.

## Evidence sources

- `ORIENTATION.md`
- `env/platform/ingress-nginx/values*.yaml`
- `env/clusters/prod/ingress-nginx.yaml`
- `env/apps/docs/base/*`
- `rendered/manifest.diff`
- `upstream/.../changelog/controller-1.11.5.md` and `controller-1.12.0.md`
- The 4.11.5 and 4.12.0 chart `values.yaml` files
- The upstream `README.md`

No cluster access was available, so no runtime behaviour was verified.
