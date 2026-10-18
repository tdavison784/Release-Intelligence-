# ingress-nginx 1.11.5 (chart 4.11.5) -> 1.12.0 (chart 4.12.0): assessment for docs-prod-eu

## Verdict

No documented 1.12.0 breaking change appears to break this environment's configuration or its one Ingress. Each ⚠️ item was checked against the repo, and none applies or none has a visible effect.

**I do not recommend this specific target.** 1.12.0 is a security regression from 1.11.5, and a better target exists (details below).

Runtime compatibility is not proven. The checks below are static only. They need a staging run before production.

## Biggest issue: 1.12.0 is older than 1.11.5 in security terms

- `changelog/controller-1.11.5.md:13` lists "Controller: Several security fixes. (#13070)".
- The 1.12.0 changelog was cut earlier (its newest entries are from Dec 2024) and has no such entry.
- The 1.11.5 patch was released after 1.12.0 as a backport. Going to 1.12.0 would drop those fixes.
- From my own knowledge, not from the supplied files: the fixes cover the March 2025 ingress-nginx CVEs, including CVE-2025-1974. They landed in 1.11.5 and 1.12.1. I could not confirm that from the repo, so please verify it against the upstream advisory.
- The README support matrix (`upstream/.../main/README.md:70-78`) shows 1.12.x as no longer supported (no 🔄 marker). Lines 52-69 show 1.13.x and later as supported, and they list Kubernetes 1.31.
- This cluster has the admission webhook enabled (`values.yaml`), and it is public-facing.
- Recommendation: if an upgrade is wanted, target at least 1.12.1 (chart 4.12.1). Preferably target a currently supported line, 1.13.x or later. Re-run this review against that target, because 1.13+ has its own breaking changes that I did not review (no changelog was supplied).

## What the rendered diff actually changes (`rendered/manifest.diff`)

Apart from the `app.kubernetes.io/version` and `helm.sh/chart` labels, the full set of changes is:

| Change | Effect here |
|---|---|
| Controller image v1.11.5 -> v1.12.0 | The intended upgrade. |
| `--enable-metrics=false` arg removed | No change in behavior. 1.12.0 defaults `--enable-metrics` to off (#12153). `controller.metrics.enabled` is false in both chart versions, and nothing in `env/` enables it. |
| `allow-snippet-annotations: "false"` removed from the ConfigMap | See "Open item" below. Likely no change in behavior. |
| `runAsGroup: 82` added (controller), `65532` (certgen jobs) | The controller image's `www-data` is already gid 82, per the 4.12.0 values comment. Low risk. Verify the pod starts. |
| kube-webhook-certgen v1.5.2 -> **v1.5.0** | The chart pins an older certgen version, so this is a downgrade. It affects the admission webhook cert jobs only. See "Open item" below. |

There is no change to the Service, the Ingress class, RBAC rules, ports or probes. No PSP resources were rendered before or after, so the PSP removal (#11971) has no effect on the diff.

## Documented breaking changes, assessed against this environment

- **`strict-validate-path-type` now on by default.**
  - The only Ingress (`apps/docs/base/ingress.yaml`) has two Prefix paths: `/` and `/.well-known`.
  - Both use only characters that strict validation allows (letters, `.`, `-`, `/`).
  - Not expected to be rejected. This is the most relevant item because the `.well-known` path contains a dot. I could not test it, because the validation regex is not in the supplied upstream files.
- **`--enable-annotation-validation` on by default, and `annotations-risk-level` lowered to High.**
  - The only annotation is `nginx.ingress.kubernetes.io/ssl-redirect: "false"`. It is a valid boolean and low risk.
  - Not affected.
- **`allow-cross-namespace-resources` disabled by default.**
  - Both backends (`docs-site`, `wellknown`) are in the Ingress's own `docs` namespace. No cross-namespace secrets or auth references exist.
  - Not affected.
- **Global rate-limit removed.** No `global-rate-limit*` config keys or annotations anywhere in `env/`. Not affected.
- **`plugins` / 3rd-party Lua removed.** No `plugins` key and no custom Lua. Not affected.
- **`ingress_upstream_latency_seconds` metric removed, and metrics disabled by default.**
  - Metrics are already off, and there is no ServiceMonitor or PrometheusRule in `env/`.
  - Not affected in this repo. Any dashboards or alerts outside this repo are unknown. See the missing information below.
- **OpenTelemetry init container and image removed.** No OpenTelemetry settings are present. Not affected.
- **s390x dropped.** The platform's node architectures aren't stated. Only an s390x node pool would be affected. Unlikely.
- **PSP removed from the chart.** Kubernetes 1.31 has no PSP, and none rendered. Not affected.

Shared and prod values (`values.yaml`, `values-prod.yaml`) set only `ingressClassResource`, `use-forwarded-headers`, `proxy-body-size`, `hsts-max-age`, the admission webhook, replicas, the Service (LoadBalancer with `externalTrafficPolicy: Local`) and resources. None of these keys was removed or renamed between 4.11.5 and 4.12.0, as far as the supplied files show (I checked the rendered diff and the values files, not an exhaustive values diff).

## Consumers and relationships checked

- **Argo CD** (`clusters/prod/ingress-nginx.yaml`): `targetRevision: 4.11.5`. The upgrade is a one-line change. The value files are unchanged, and automated sync with prune and selfHeal is on. Merging the change rolls it out to prod immediately, with no manual gate.
- **IngressClass** `nginx` (default) and controller `k8s.io/ingress-nginx` are unchanged. The Ingress uses `ingressClassName: nginx`.
- **docs Ingress -> `docs-site` and `wellknown` Services.**
  - Service names, port name `http` and selectors are unchanged and are unaffected by the controller version.
  - `/.well-known` Prefix routes to the `wellknown` service. The mount `/usr/share/nginx/html/.well-known` lines up with the request path.
  - The controller does no rewriting, and none is configured.
- **security.txt** (`Contact`, `Expires: 2027-06-30`) is served through that route. It is a good smoke test, since it is the only content under `/.well-known`.

## Open item: `allow-snippet-annotations`

The key disappears from the rendered ConfigMap even though the chart's `allowSnippetAnnotations: false` is the same in both values files. Either the 1.12.0 chart omits the key when it is false, or the controller default changed. The supplied files don't say which.

If the controller default is not false, snippets would become allowed. Nothing in the repo uses snippets, so there is no functional break today. It would still be a security-posture change, because anyone able to create Ingresses could use snippets. Confirm with the chart template or `kubectl get cm` on staging.

## Open item: webhook certgen downgrade

The certgen image going from v1.5.2 to v1.5.0 is a downgrade. Check whether v1.5.2 fixed anything that matters, and confirm that the create and patch jobs still complete under Argo CD hooks. The webhook is enabled, and a failed job would block or break Ingress admission.

## Missing information and tests needed before proceeding

1. Decide the real target version (at least 1.12.1, or preferably 1.13+) and redo the render and diff for it. The supplied materials cover only 1.12.0.
2. Run the check on the actual cluster state:
   - `kubectl get ingress -A` and `kubectl get ingress -A -o yaml`, to find any Ingress not in this repo.
   - `kubectl get cm,deploy,ds -n ingress-nginx -o yaml`, to look for drift. The statement that nothing is applied by hand is unverified.
   - Compare the live ConfigMap and args to `rendered/from.yaml`.
   - The Ingress may have been applied or altered by another process.
3. Staging or canary run of 1.12.0, checking:
   - The pods become Ready, and the controller log has no admission or validation errors.
   - The admission webhook accepts the docs Ingress and rejects a bad one.
   - `GET /` and `GET /.well-known/security.txt` return 200 with the expected content, and the HSTS header is present.
   - `X-Forwarded-*` headers behave the same, with `use-forwarded-headers` and the `externalTrafficPolicy: Local` client IP.
   - A request with a body over 8m returns 413.
   - The `ssl-redirect: "false"` behavior is unchanged: HTTP is not redirected.
4. Rollout behavior: two replicas on a LoadBalancer with `externalTrafficPolicy: Local` need the LB health checks to pass during the rolling update. Confirm PDB and `maxUnavailable` expectations, since the diff shows no strategy change.
5. External consumers: any monitoring, dashboards or alerts based on removed metrics, and any log parsing that depends on the controller's log format. None is in this repo.
6. Rollback plan: pin back to `4.11.5` in Git. Note that this reverts to the older image, with the CRD and admission pieces unchanged.

## Evidence

- `ORIENTATION.md`, `env/platform/ingress-nginx/values*.yaml`, `env/clusters/prod/ingress-nginx.yaml`, `env/apps/docs/base/ingress.yaml`, `wellknown.yaml`, `docs-site.yaml`
- `rendered/manifest.diff`, `rendered/from.yaml`, `rendered/to.yaml`
- `upstream/.../changelog/controller-1.12.0.md` and `controller-1.11.5.md`
- `upstream/.../helm-chart-4.1{1.5,2.0}/charts/ingress-nginx/{Chart,values}.yaml`
- `upstream/.../main/README.md` (support matrix)

Limits: I had no cluster access and no upstream docs beyond the changelogs and values files. I could not run anything. The contents of the strict path-type regex and the controller's snippet default are from knowledge or inference, not from the supplied files.
