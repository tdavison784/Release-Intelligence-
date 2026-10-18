# ingress-nginx controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0): impact assessment

## Verdict

**Do not upgrade as-is.** One breaking change very likely affects this environment, and the rendered diff does not show it. The storefront Ingress uses `configuration-snippet`. v1.12.0 changes the default `annotations-risk-level` to `High`, and snippet annotations are `Critical`. I expect the controller to reject or ignore that annotation even though `allowSnippetAnnotations: true` is set. Everything else in the changelog is low risk or not applicable. The conclusion on the snippet is inferred from upstream behavior I know of. It is not proven by the files supplied (see "Missing information").

## Affected

### 1. Storefront `configuration-snippet` vs. the new security defaults (HIGH)

Evidence:
- `env/apps/storefront/overlays/prod/ingress-headers.yaml` sets `nginx.ingress.kubernetes.io/configuration-snippet` on the storefront Ingress. The snippet sets HSTS and `X-Frame-Options: DENY`, and the file comment says the PCI review requires these headers.
- `env/platform/ingress-nginx/values.yaml:7` sets `allowSnippetAnnotations: true`. The rendered ConfigMap carries `allow-snippet-annotations: "true"` in both versions.
- `changelog/controller-1.12.0.md`, "Enable security features by default (#11819)", lists these changes:
  - `--enable-annotation-validation` is enabled by default.
  - `annotations-risk-level` is lowered to "High" by default.
  - `allow-cross-namespace-resources` is disabled by default.
  - `strict-validate-path-type` is enabled by default.
- Snippet annotations are classified Critical. The "High" level therefore excludes them regardless of `allow-snippet-annotations`. That setting only gates whether snippets are permitted at all.
- The rendered manifests (`rendered/manifest.diff`) show no change to the ConfigMap. They also show no `--enable-annotation-validation` argument in either version. The change is therefore a **binary default**, not a manifest change. An unchanged manifest here does not show unchanged behavior.
- The chart value `enableAnnotationValidations` goes from `false` (4.11.5, line 20) to `true` (4.12.0, line 24). It produces no visible arg in the render, so the effective behavior comes from the new controller default.

Likely consequences:
- The admission webhook has `failurePolicy: Fail` on Ingress CREATE and UPDATE. A sync of the storefront Ingress, including an Argo self-heal, could be rejected.
- The existing Ingress is re-evaluated when the new controller pods start. The storefront host may lose its configuration, or lose only the security headers. That would be a PCI regression and possibly an outage.
- Argo CD sync will show errors or drift. The controller rollout itself can still succeed.

Fix options, to be decided by the owner (the PCI requirement makes the first one a security-owner decision):
- Set `controller.config.annotations-risk-level: Critical`. This keeps the snippet working and keeps snippets enabled.
- Replace the snippet with a safer mechanism. Options are `add-headers` / `custom-headers` (ConfigMap-based) or `more_set_headers` in a controller-level `server-snippet` / `location-snippet`. This needs its own review.

## Not affected, with reasons

| Change | Why it does not apply here |
|---|---|
| `--enable-metrics` now defaults to false (#12153). The rendered diff removes `--enable-metrics=false`. | `values.yaml` already has `metrics.enabled: false`. Behavior is the same. Nothing in `env/` has a ServiceMonitor or Prometheus config. Check for out-of-repo scrape configs and alerts (see below). |
| `allow-cross-namespace-resources` off | Neither Ingress references resources in another namespace. They use no auth, TLS or secret annotations. Each backend Service is in the Ingress's own namespace (kustomize sets the namespace). |
| `strict-validate-path-type` on | Paths are `/` and `/v1`, both `Prefix`, and valid. |
| Removed global-rate-limit options and annotations | Not used anywhere in `env/`. |
| Removed `plugins` (3rd-party Lua) | Not configured. |
| `ingress_upstream_latency_seconds` metric removed | Metrics are disabled. Check external dashboards. |
| OpenTelemetry init container removed | The module is built into the image. OTel is not configured here. |
| Pod Security Policy removed from the chart | Kubernetes 1.31 has no PSP, and no PSP resources appear in the render. |
| `s390x` dropped | Architecture of the nodes is not stated. Likely amd64 or arm64; confirm. |
| CORS "allow any protocol" (#11153) | The catalog-api sets `enable-cors: "true"` with no `cors-allow-origin`, so the origin list is not customised. This is a relaxation of validation, not a restriction. A behavioral check is still advisable (see tests). |
| Rendered label and version changes | Cosmetic. `app.kubernetes.io/version` and `helm.sh/chart` change on every object. |

## Noted in the render (low risk, verify)

- The controller `securityContext` gains `runAsGroup: 82`. The image already runs as uid 101 / gid 82 (upstream comment, `values.yaml:39`). The jobs gain `runAsGroup: 65532`. No `fsGroup`, PSA label or admission policy in `env/` conflicts with this. Cluster-level policy (Kyverno, Gatekeeper, PSA namespace labels) is not visible in the repo.
- `kube-webhook-certgen` goes from **v1.5.2 to v1.5.0**, a downgrade, although the changelog says "Bump Kube Webhook CertGen". It is the chart's pinned version and works with Kubernetes 1.31, but note that it is a regression in version and look for known issues. The jobs are Helm hooks (`pre-install,pre-upgrade`). Under Argo CD, hooks are mapped to its own hook phases. Confirm the Job completes and the cert Secret and webhook caBundle are consistent.
- `maxUnavailable` / PDB / rollout: `replicaCount: 2` and the default rolling update apply. During the rollout, old (1.11.5) and new (1.12.0) pods serve at the same time. Pods have `terminationGracePeriodSeconds: 300`.
- The `server-tokens` value in `values.yaml` is redacted in the supplied files, so I could not validate it.

## Missing information / tests before proceeding

1. **Staging test of the storefront Ingress on 1.12.0 with the production values.** Apply the real overlay and check (a) that the webhook admits it, (b) the generated nginx config for `shop.example.com`, and (c) that `curl -I` returns HSTS and X-Frame-Options. Repeat after a controller pod restart to cover the re-evaluation path. This confirms or refutes finding 1. I could not run anything here (Bash was denied) and the controller source is not in the provided files.
2. Confirm the exact risk classification of `configuration-snippet` and the behavior of existing Ingresses in 1.12.0. The upstream annotation docs and the `annotations-risk-level` ConfigMap docs are not in `upstream/`.
3. Check the controller logs of the staging 1.12.0 pods for "risky annotation" or validation messages.
4. Decide on the header strategy (risk-level change vs. other mechanism) with the security/PCI owner.
5. Check for anything outside this repo that relies on controller metrics: ServiceMonitors, Prometheus scrape jobs, alerts, dashboards. The cluster is described as fully declared in this repo, but monitoring may be in another one.
6. Check the other shop clusters that share `values.yaml`. The shared file affects them all, and only prod is covered by the renders.
7. Test CORS preflight on `api.shop.example.com/v1` before and after.
8. Check that no other Ingress outside the two apps uses the ingress class. The orientation says only two apps use it.
9. Confirm the node architecture, any cluster admission policy on `runAsGroup`, and that the digest-pinned images are mirrored if you use a private registry.
10. Run the upgrade in two steps: change the chart version plus the risk-level setting together in a PR, and keep a rollback path (revert `targetRevision` to 4.11.5). Argo auto-sync with `selfHeal` will apply it immediately on merge.
11. Specific to Argo CD: `selfHeal` and `prune` are on, and the webhook has `failurePolicy: Fail`. A rejected Ingress update during sync can leave the app OutOfSync/Degraded. Watch the first sync closely.

## Files reviewed

`ORIENTATION.md`; `env/platform/ingress-nginx/*`; `env/clusters/prod/*`; `env/apps/{storefront,catalog-api}/**`; `rendered/{manifest.diff,to.yaml}`; `upstream/.../controller-1.12.0.md`; chart `values.yaml` for 4.11.5 and 4.12.0 (via targeted search, not a full read).
