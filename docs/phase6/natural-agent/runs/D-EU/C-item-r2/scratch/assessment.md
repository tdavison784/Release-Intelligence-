# ingress-nginx 1.11.5 -> 1.12.0: "Enable security features by default" (#11819)

## Verdict
Three of the four changed defaults have no consumer in this environment. The fourth, `strict-validate-path-type`, is the only real risk, because of the `/.well-known` path in `apps/docs`. I could not settle it from the repo. Do not call the upgrade safe until the server-side dry-run below passes.

## Scope of what I checked
- Files read: `platform/ingress-nginx/values.yaml`, `values-prod.yaml`, `clusters/prod/ingress-nginx.yaml`, and `apps/docs` (base and prod overlay).
- Rendered output: `rendered/manifest.diff`, `to.yaml`, `from.yaml`.
- Upstream: the 1.12.0 changelog.
- The repo declares exactly one Ingress, `docs/docs`. It is the only object these defaults can act on.
- Nothing sets any of the four options, so all of them take the new upstream defaults.
  - No `controller.config` key sets `annotations-risk-level`, `allow-cross-namespace-resources` or `strict-validate-path-type`.
  - No `controller.extraArgs` sets `--enable-annotation-validation`.
  - The rendered ConfigMap holds only `hsts-max-age`, `proxy-body-size` and `use-forwarded-headers`.
  - The rendered args show no validation flag in either version.

## Per-change analysis

| Change | Affected? | Evidence |
|---|---|---|
| `--enable-annotation-validation` on | No (low risk) | The only annotation in the repo is `nginx.ingress.kubernetes.io/ssl-redirect: "false"`. It is a known, low-risk annotation with a boolean value, so it should validate. |
| `annotations-risk-level` lowered to High | No | The Ingress has no snippet or other Critical-risk annotation. |
| `allow-cross-namespace-resources` off | No | The Ingress references only same-namespace Services (`docs-site`, `wellknown`, both in `docs`). It has no TLS secret, no auth-secret annotation and no cross-namespace reference of any kind. |
| `strict-validate-path-type` on | **Unresolved** | See below. |

### Path validation
- The Ingress has two paths, both `pathType: Prefix`: `/` and `/.well-known`.
- Strict validation checks path characters, and rejects anything outside its allowed set unless the pathType is `ImplementationSpecific`.
- `/` is fine.
- `/.well-known` contains a `.`. The upstream docs describe the allowed set as alphanumerics, `-`, `_` and `/`. The regex in the source may also allow `.`.
- Neither the changelog nor anything else in `upstream/` shows the regex, so I could not confirm that `.` is accepted.
- If `.` is rejected, the admission webhook (`ingress-nginx-admission`, enabled in `values.yaml`) would refuse the Ingress on its next create or update.
- The consequence depends on whether the controller also re-checks existing Ingresses at startup or sync. I did not verify that.
  - If it does, `/.well-known` would be dropped or the sync would fail. The rest of the docs host could be affected too.
  - If it does not, the failure would appear only on the next change to the Ingress, for example an Argo CD selfHeal or resync.
- The affected route serves `security.txt`, which is RFC 9116 content.
- The nginx pod serves `/usr/share/nginx/html/.well-known/security.txt` from the `wellknown-content` ConfigMap, so the backend itself is fine.

## Why an unchanged manifest is not proof
The rendered diff shows no change to the Ingress or to any validation setting. These defaults are applied inside the controller binary and webhook, so the rendered manifests cannot show them. The diff therefore proves nothing about this change.

## Related changes seen in the diff (outside #11819)
- `allow-snippet-annotations: "false"` is no longer rendered. The 1.12 controller still defaults to false, and no snippet is in use, so this has no effect. Confirm the controller default.
- `--enable-metrics=false` is no longer passed. Metrics are now off by default (#12153), so behavior is the same.
- New `runAsGroup` values on the controller (82) and on the certgen jobs (65532). The certgen image goes from v1.5.2 to v1.5.0, which is a downgrade. These are separate review items.

## Missing information and tests before proceeding
1. **Confirm path validation.** Read the 1.12.0 source for the strict path regex and check whether it accepts `.`. Better: with a 1.12.0 controller in a staging cluster, run `kubectl apply --dry-run=server -f` on the rendered Ingress (`rendered/to.yaml`, `docs/docs`). Also apply it for real, and confirm `curl https://docs.example.com/.well-known/security.txt` returns 200.
2. **Fallback if rejected.** Change `/.well-known` to `pathType: ImplementationSpecific`, or set `strict-validate-path-type: "false"` in `controller.config`. Prefer the pathType change, since it keeps the protection for other Ingresses.
3. **Check the live cluster.** The repo is declared to be complete, but I only had the repo. Check the live cluster for Ingresses of class `nginx` outside this repo, and for annotations that annotation validation would reject. For example: `kubectl get ingress -A -o yaml`.
4. **Find out what existing objects do at controller start.** Run the staging controller against the existing Ingress and watch the controller logs for validation errors.
5. **Staged rollout.** Argo has `selfHeal` and automated sync on. Bump `targetRevision` in staging first.
6. Check the `kube-webhook-certgen` v1.5.0 image and the new `runAsGroup` values separately.
