# ingress-nginx 1.11.5 -> 1.12.0: does "Enable security features by default" (#11819) break this environment?

## Verdict

**Probably not, but one item is unverified.** None of the four changed defaults conflicts with anything declared in the repository. The one residual risk is the `strict-validate-path-type` check applied to the `/.well-known` path (a path segment starting with a dot). I could not confirm from the supplied material that the check accepts it. It needs a quick admission test before rollout (see "Required before proceeding").

## What the environment declares (evidence)

- `env/clusters/prod/ingress-nginx.yaml`: chart 4.11.5, value files `platform/ingress-nginx/values.yaml` then `values-prod.yaml`. Argo CD has automated sync, prune and selfHeal on.
- `values.yaml` sets only these controller options:
  - `ingressClassResource` (name `nginx`, default)
  - `config`: `use-forwarded-headers`, `proxy-body-size`, `hsts-max-age`
  - `admissionWebhooks.enabled: true`
- `values-prod.yaml` sets replicas, service type and resources.
- **Nothing overrides any of the four changed settings.** There is no `extraArgs`, no `annotations-risk-level`, no `allow-cross-namespace-resources`, no `strict-validate-path-type`, and no `--enable-annotation-validation`. This environment therefore picks up the new defaults.
- The only Ingress in the repo is `env/apps/docs/base/ingress.yaml` (`docs`, class `nginx`). The overlay and cluster Applications add no other Ingresses.
  - Annotation: `nginx.ingress.kubernetes.io/ssl-redirect: "false"`.
  - Paths: `/` (Prefix) to `docs-site`, and `/.well-known` (Prefix) to `wellknown`.
  - There is no TLS section, no secret reference, and no snippet or auth annotation.
- `rendered/manifest.diff` shows the chart-level changes. The admission webhook stays enabled, which is the component that applies the new validation.

## Per-change analysis

| Change | Affected? | Reasoning |
|---|---|---|
| `--enable-annotation-validation` on by default | **No** | The only annotation is `ssl-redirect: "false"`, a valid boolean. It is a low-risk annotation, so it is not rejected by validation or by the risk level. |
| `annotations-risk-level` lowered to High | **No** | The default only blocks "Critical" annotations, such as `configuration-snippet`, `server-snippet` and `auth-snippet`. None are used. `ssl-redirect` is low risk. |
| `allow-cross-namespace-resources` disabled | **No** | This option governs references to Secrets in other namespaces (`auth-secret`, `auth-tls-secret`, `proxy-ssl-secret` and similar). The Ingress has no such annotations. Its backends `docs-site` and `wellknown` are in the same namespace (kustomize sets `docs`). |
| `strict-validate-path-type` enabled | **Probably no, unverified** | Both paths use `Prefix`. Neither is a regex and neither uses `ImplementationSpecific`. From memory, strict validation rejects characters outside an alphanumeric, `-`, `_`, `.` and `/` class, and regex-like or `//` paths. If so, `/.well-known` passes because `.` is allowed. The supplied upstream files contain only the changelog line, not the implementation, so I could not check this. |

## Runtime compatibility beyond the unchanged manifest

- **The `/.well-known` route works end to end.** `wellknown.yaml` mounts the `wellknown-content` ConfigMap at `/usr/share/nginx/html/.well-known`, and that ConfigMap holds `security.txt`. A request to `/.well-known/security.txt` maps to a real file. The controller does not rewrite the path, so none of the changed defaults affect it.
- **The validation runs at admission time.** If `/.well-known` were rejected, the failure would show up when Argo CD syncs the Ingress, which already exists. Existing Ingresses already in the cluster are not re-admitted on a controller upgrade. If the controller's runtime sync also applies the check, the Ingress could be skipped and the route would drop. Which of these happens cannot be settled from the supplied material. That is why a live test is needed, not just manifest review.
- **Possible hidden consumers.** I only see this repo. Anything created outside it would not appear here: other Ingresses, or annotations added by other teams or controllers. ORIENTATION says nothing is applied by hand. Because the app is a docs site on a shared "docs clusters" values file, other clusters' repos might add annotations. Those are out of scope but should be checked if the shared `values.yaml` is reused.

## Unrelated changes visible in the render (not part of #11819, but worth knowing)

- The `--enable-metrics=false` arg disappears (metrics now off by default, #12153). No metrics, ServiceMonitor or PodMonitor are configured here, so there is no impact. Anything that scraped the controller would already have been failing.
- `allow-snippet-annotations: "false"` is no longer written to the ConfigMap. Snippets are not used, and the effective setting is still "no snippets" because of the new risk level.
- The admission jobs use `kube-webhook-certgen` v1.5.0, which is **older** than the v1.5.2 used by the 1.11.5 render. Check that this is intended.
- `runAsGroup` is newly set: 82 on the controller and 65532 on the jobs. This is low risk, but confirm against any Pod Security or admission policy in the cluster.

## Missing information and tests needed before proceeding

1. **Server-side dry-run in staging on 1.12.0.** Run `kubectl apply --dry-run=server` of `apps/docs` against a cluster with the 1.12.0 webhook. Confirm that the `/.well-known` Prefix path and the `ssl-redirect` annotation are admitted. This resolves the one unverified item.
2. **Inspect the 1.12.0 source or the docs.** Confirm the exact path regex in the controller. Upstream code and configuration docs were not supplied.
3. **Smoke test after the upgrade.** Fetch `https://docs.example.com/` and `/.well-known/security.txt`. Check the controller logs for "invalid" or "skipping ingress" messages. Check that the generated nginx.conf contains the `/.well-known` location.
4. **Inventory of other Ingresses** in the live cluster (`kubectl get ingress -A -o yaml`). Look for snippet, cross-namespace secret, or odd-path annotations that this repo does not declare.
5. **Check the certgen v1.5.0 versus v1.5.2 discrepancy** and the new `runAsGroup` values against cluster policy.
6. **Rollback plan.** Pin chart 4.11.5 in `clusters/prod/ingress-nginx.yaml`. As a mitigation if validation misbehaves, set `controller.config.strict-validate-path-type: "false"`, which affects only this one setting.
