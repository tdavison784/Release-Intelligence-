# ingress-nginx 1.11.5 -> 1.12.0: "Enable security features by default (#11819)"

## Verdict

**Not expected to break this environment.** Every changed default is either already satisfied by the one Ingress in the repo or is not used at all. I found no config that depends on the old permissive behavior. This is a static review. Two points could not be confirmed offline: the exact path-validation regex, and live cluster state. See "Missing information and tests".

## Scope of the evidence

- `env/platform/ingress-nginx/values.yaml` and `values-prod.yaml`. The Argo app (`env/clusters/prod/ingress-nginx.yaml`) loads them in that order. Together they set only these things:
  - the ingress class
  - `config`: `use-forwarded-headers`, `proxy-body-size`, `hsts-max-age`
  - `admissionWebhooks.enabled: true`
  - replicas, service and resources
- There are no `extraArgs`, no `allow-cross-namespace-resources`, no `annotations-risk-level`, no `strict-validate-path-type`, no `global-allowed-response-headers`, and no `default-ssl-certificate`.
- `env/apps/docs/base/ingress.yaml`. It is the only Ingress, because ORIENTATION states the repo is the complete declaration. It was cross-checked against the rendered Ingress at `rendered/to.yaml:909-936`.
- `env/apps/docs/base/{docs-site,wellknown}.yaml`, `rendered/manifest.diff`, `rendered/to.yaml`, the 1.12.0 changelog, and the 4.11.5 and 4.12.0 chart `values.yaml` files.

## Item by item

### 1. `--enable-annotation-validation` enabled by default: not affected
- The Ingress has one annotation, `nginx.ingress.kubernetes.io/ssl-redirect: "false"`. It is a low-risk annotation with a plain boolean value, which is a valid value.
- No annotation in the repo is rejected by the validation.
- The rendered diff shows no `--enable-annotation-validation` arg added to the controller. Chart 4.12.0 flips `controller.enableAnnotationValidations` to `true` (`values.yaml:24`, was `false` in 4.11.5), but the rendered args don't contain the flag. The effective behavior therefore comes from the 1.12.0 binary default. That is still on, so don't conclude from the unchanged args that nothing changes.
- Validation also takes effect in the validating webhook, which is enabled here. It covers CREATE and UPDATE of ingresses with `failurePolicy: Fail` (`rendered/to.yaml:26-38`).

### 2. `allow-cross-namespace-resources` disabled by default: not affected
- This option only matters for annotations that reference a Secret or ConfigMap in another namespace (`auth-secret`, `auth-tls-secret`, `proxy-ssl-secret` and similar).
- The Ingress has none of these annotations.
- Both backends (`docs-site` and `wellknown`) are Services in the same `docs` namespace as the Ingress. Backends are not subject to this option anyway.
- There is no TLS section and no default SSL certificate arg.
- The controller runs in the `ingress-nginx` namespace and the Ingress in `docs`. That isn't relevant, because no resource is referenced across namespaces.

### 3. `annotations-risk-level` lowered to High: not affected
- Under this default, annotations rated Critical are no longer accepted. Examples are `server-snippet`, `configuration-snippet` and `auth-snippet`.
- None are used. `ssl-redirect` is low risk.
- `allow-snippet-annotations: "false"` disappears from the rendered ConfigMap. That is the chart no longer emitting a value that was already the default (chart `allowSnippetAnnotations: false`). It is not a behavior change, and it is a different change from #11819.

### 4. `strict-validate-path-type` enabled by default: not affected (high confidence, not verified against source)
- This enforces a character whitelist on Exact and Prefix paths in the admission webhook.
- The paths here are `/` and `/.well-known`, both `pathType: Prefix`.
- From my recollection of `internal/ingress/inspector/rules.go`, the allowed set is alphanumerics plus `- . _ ~ /`, so `.` and `-` pass. The upstream folder here doesn't include that source, so please confirm by test.
- The `/.well-known` path is the only one with unusual characters, and it is the most plausible place for a problem. Both of the other path characteristics (`/` root, Prefix type) are plain.
- The `wellknown` service mounts the ConfigMap at `/usr/share/nginx/html/.well-known`, so the request path `/.well-known/security.txt` still maps to the file. Path validation does not change that.

## Other differences in the same upgrade (not part of #11819, noted for completeness)
- `--enable-metrics=false` is removed from the args (#12153: metrics are disabled by default). Metrics stay off, so there is no change. Nothing in the repo enables or scrapes metrics. Check for external monitoring that expects them.
- The `kube-webhook-certgen` image goes from v1.5.2 to v1.5.0, which is a downgrade of the tag. This is what the chart pins, and it is worth a glance.
- `runAsGroup` is added on the controller (82) and on the certgen jobs (65532). The controller `runAsGroup: 82` could matter if the image's filesystem permissions (e.g. mounted volumes) assume another group. Test on startup.
- Dropped Lua plugin support (#11821). Nothing here uses it.
- Other changes were not assessed, since they were outside the question.

## What is not proven by the unchanged manifests
- The Ingress manifest is unchanged, but the controller's validation behavior is not. A new controller would validate any later UPDATE to the Ingress, and all the Ingresses applied on the cluster by anything other than this repo are invisible to me.
- If any Ingress exists in the cluster that is not declared in this repo (ORIENTATION says none), it may carry snippet or cross-namespace annotations that the new defaults would reject or drop.

## Missing information and tests before proceeding
1. On staging (or `kubectl apply --dry-run=server` against a 1.12.0 controller), apply the rendered Ingress and confirm the webhook admits it. This closes the path-regex uncertainty.
2. After the rollout, run `kubectl get ingress -A -o yaml | grep -E 'snippet|auth-|secret'` and confirm no Ingress outside `docs` uses risky or cross-namespace annotations.
3. Check the new controller logs for validation or "dropping ingress" errors. Confirm `curl -H 'Host: docs.example.com' /`, `/.well-known/security.txt` and a 404 path return as before.
4. Check that `ssl-redirect: "false"` is still honored (HTTP returns 200, no 308).
5. Roll out with `externalTrafficPolicy: Local` and 2 replicas, and watch that the admission jobs complete (the webhook has `failurePolicy: Fail`, so a broken webhook blocks every Ingress write in the cluster).
6. Confirm that nothing in the Argo setup (other clusters share `values.yaml`, "all docs clusters") has Ingresses that depend on the old defaults. This repo only shows `docs-prod-eu`.
7. Update `targetRevision` to 4.12.0 in `env/clusters/prod/ingress-nginx.yaml` only after the above. I made no changes to the environment.
