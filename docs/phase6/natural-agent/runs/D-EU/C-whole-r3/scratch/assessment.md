# ingress-nginx upgrade assessment: controller-v1.11.5 (chart 4.11.5) -> controller-v1.12.0 (chart 4.12.0)

Environment: prod docs cluster, K8s 1.31, Argo CD. Values come from `platform/ingress-nginx/values.yaml` and then `values-prod.yaml`. One Ingress (`apps/docs`) routes `docs.example.com`.

## Verdict

**No documented breaking change is proven to hit this environment. One item could still block the rollout and has to be tested first: the new default `strict-validate-path-type=true` against the `/.well-known` path.** Several other changes are either neutral here or are rendered differences that need a conscious decision.

I could not run anything. This assessment rests on the changelog, the chart values, the rendered diff and the repo manifests. The controller source and the live cluster were not available.

## Things that could affect this environment

### 1. Strict path-type validation (#11819), the main risk
- 1.12.0 turns `strict-validate-path-type` on by default. The changelog line is at `upstream/.../changelog/controller-1.12.0.md:149`. Nothing in `values.yaml` or `values-prod.yaml` overrides it, so it applies here.
- `env/apps/docs/base/ingress.yaml` has two `Prefix` paths: `/` and `/.well-known`. The second is the only unusual one, because it starts with a dot segment.
- Upstream's validation restricts path characters for `Exact` and `Prefix`. From memory, dots, hyphens and underscores are allowed, so `/.well-known` should pass. **I could not confirm this from the supplied documents**, because the changelog doesn't give the regex. Treat it as unverified.
- Failure modes if it is rejected:
  - The admission webhook is enabled (`admissionWebhooks.enabled: true`), and the rendered `ingress-nginx-admission-patch` job uses `--patch-failure-policy=Fail`. The `docs` Ingress would then be refused at the next Argo sync or update.
  - It is also unclear whether an Ingress that already exists is dropped from the generated config when the new controller starts. If so, it would be an outage rather than a refused sync.
  - If `/.well-known` were dropped, security.txt and other RFC 8615 documents would fall through to `docs-site`, which has no such content, so they would return 404. This would be quiet, because the main docs site would keep working.
- `/.well-known` has to keep working: `security.txt` has `Expires: 2027-06-30`, so it is live content.

### 2. Annotation validation and risk level (#11819)
- `--enable-annotation-validation` becomes the default, and `annotations-risk-level` drops to High.
- The only annotation in use is `nginx.ingress.kubernetes.io/ssl-redirect: "false"`. It is a plain boolean and low risk. **Not affected.**
- Snippet annotations were already off in 1.11.5 (`allowSnippetAnnotations: false`). **Not affected.**
- Other Ingresses outside this repo are not visible. The orientation says the repo declares everything that runs, so I assume there are none, but I haven't checked the cluster.

### 3. Rendered diff items (`rendered/manifest.diff`)
- **`--enable-metrics=false` disappears from the controller args.** The 1.12 default is already false (#12153), so the behaviour is unchanged. No ServiceMonitor, Prometheus scrape config or `controller.metrics` value exists in the repo. Port 10254 is still used for health probes, and the diff doesn't touch that.
  - Missing information: an out-of-repo Prometheus scraping the pods by annotation would already be getting nothing today.
- **`allow-snippet-annotations: "false"` is removed from the ConfigMap.** The chart value is still `false` in both versions, so this looks like only the rendering changing. Confirm that the 1.12 controller default is also false. The changelog doesn't say, and I assume it is. Run a test that applies an Ingress with a `configuration-snippet` and check that it is rejected.
- **`runAsGroup: 82` is added to the controller and `65532` to the certgen jobs.** `runAsUser` and `runAsNonRoot` are unchanged. This could matter only if files the controller writes or reads depend on group ownership, or if a Pod Security or policy admission rule in the cluster restricts `runAsGroup`. No such policy is in the repo, but cluster-level policy (Kyverno, Gatekeeper, PSA labels) is not visible here.
- **`kube-webhook-certgen` goes from v1.5.2 to v1.5.0.** This is a downgrade of the certgen image, which looks odd next to the changelog entry "Chart: Bump Kube Webhook CertGen (#12606)". v1.5.0 creates and patches the admission certificate on a fresh install. Check what it does when the secret `ingress-nginx-admission` already exists. Also check whether your registry mirror or allow-list has the new digest `sha256:aaafd4…`.
- **Controller image digest changed to `v1.12.0@sha256:e6b8de17…`.** It matches the digest in the 1.12.0 changelog, so the pin is consistent.

## Documented breaking changes that do not apply

| Change | Why it doesn't affect this env |
|---|---|
| Global rate limit removed (#11851) | No `global-rate-limit*` config or annotations in values or Ingress |
| 3rd-party Lua plugins removed (#11821) | No `plugins` config set; no custom plugin mounts in the controller |
| `ingress_upstream_latency_seconds` metric removed (#11795) | Metrics are not enabled and nothing in the repo scrapes them or uses a dashboard |
| Pod Security Policy removed from chart (#11971) | K8s 1.31 has no PSP; the render doesn't contain any |
| `allow-cross-namespace-resources` disabled | The Ingress's backends are in its own `docs` namespace |
| OpenTelemetry init container removed (#12024) | OpenTelemetry isn't configured here |
| s390x images dropped (#12137) | No sign of s390x nodes. The node architecture isn't in the repo. |

## Argo CD and rollout mechanics
- `clusters/prod/ingress-nginx.yaml` pins `targetRevision: 4.11.5`, with automated sync, prune and self-heal. The upgrade is a one-line change to `4.12.0`, and it deploys immediately on merge.
- The controller is a Deployment with `replicaCount: 2` and `externalTrafficPolicy: Local`. A rolling update should keep traffic up only if the load balancer health checks work with `Local` and the two pods land on different nodes. That isn't confirmed from the repo.
- The admission jobs are Helm hooks in the chart. Check how Argo treats them on this sync: a leftover completed job from the 4.11.5 sync could conflict.

## Missing information and tests needed before proceeding
1. **Dry-run the actual Ingress against a 1.12.0 controller** in a non-prod cluster with the same values (the quickest way to settle item 1). Apply `apps/docs` and confirm that admission accepts it and that `/.well-known/security.txt` and `/` both return 200 from the right backends.
2. Do this a second way as well: start the 1.12.0 controller with the Ingress already present, since existing-object handling differs from admission. Check the controller logs for rejected-Ingress messages and look at the generated `nginx.conf` for the `/.well-known` location.
3. Look at the controller source or docs for the exact path regex behind `strict-validate-path-type`. Only if it rejects the path, set `strict-validate-path-type: "false"` in `values.yaml` as a stopgap. Do not do this pre-emptively.
4. Check the 1.12 default for `allow-snippet-annotations` and the snippet-rejection behaviour (see item 3 above).
5. Run the certgen jobs on a staging cluster that already has the admission secret from 4.11.5, to see how v1.5.0 behaves.
6. Get cluster-side facts that aren't in this repo:
   - admission policies on `runAsGroup`;
   - any Prometheus scraping of the controller;
   - the registry allow-list for the new digests;
   - other Ingresses using this class (this controller is the default class);
   - the node architecture.
7. Make a rollback plan: revert `targetRevision` to 4.11.5 and note that Argo will sync it automatically. Confirm that a downgrade works with the certgen secret.
8. Check whether the TLS/HSTS config is affected. `hsts-max-age` is set, but nothing in the repo shows TLS being served. The Ingress has `ssl-redirect: "false"` and no `tls:` section, and `use-forwarded-headers` is true, so TLS is probably terminated at the load balancer. Confirm this with a request through the real path.

## Evidence index
- Upstream: `upstream/.../changelog/controller-1.12.0.md`; chart values at `helm-chart-4.11.5` and `helm-chart-4.12.0`.
- Env: `env/apps/docs/base/ingress.yaml`, `env/platform/ingress-nginx/values*.yaml`, `env/clusters/prod/ingress-nginx.yaml`, `env/apps/docs/base/wellknown.yaml`.
- Render: `rendered/manifest.diff`, with the Ingress at `rendered/to.yaml:909`.
