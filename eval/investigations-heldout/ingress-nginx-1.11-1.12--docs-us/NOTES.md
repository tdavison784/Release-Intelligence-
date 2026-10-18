# ingress-nginx-1.11-1.12--docs-us — notes (ground truth; not for systems under test)

Pair `ingress-nginx-1.11-1.12--docs-paths` with `--docs-eu`. The two environments differ only in the
pathType of the `/.well-known` path of Ingress `docs/docs`: **ImplementationSpecific** here, Prefix in
`--docs-eu`.

## Upstream sources

- Changelog controller-v1.12.0,
  [`changelog/controller-1.12.0.md#L139-L149`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/changelog/controller-1.12.0.md#L139-L149)
  (one entry covers all four scope items B1-B4):
  > ⚠️ Enable security features by default (#11819) ⚠️ This changes the default of the following CLI
  > arguments: `--enable-annotation-validation` gets enabled by default. It also changes the default of
  > the following configuration options: `allow-cross-namespace-resources` gets disabled by default.
  > `annotations-risk-level` gets lowered to "High" by default. `strict-validate-path-type` gets enabled by
  > default.
- Defaults in source, controller-v1.12.0 (`8ee43842`) vs controller-v1.11.5 (`795b6964`):
  `internal/ingress/controller/config/config.go` — v1.12.0 L754 `AllowCrossNamespaceResources: false`,
  L757 `AnnotationsRiskLevel: "High"`, L897 `StrictValidatePathType: true` (v1.11.5 L782 `true`, L785
  `"Critical"`, L930 `false`); `pkg/flags/flags.go` v1.12.0 L162 `enable-annotation-validation` default
  `true` (v1.11.5 L161 `false`).
- Strict path validation,
  [`internal/ingress/inspector/rules.go#L31-L37`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/internal/ingress/inspector/rules.go#L31-L37):
  > validPathType enforces alphanumeric, -, _ and / characters. ... `validPathType = regexp.MustCompile(`(?i)^/[[:alnum:]\_\-/]*$`)`

  applied to every path whose pathType is not `ImplementationSpecific`
  ([`inspector.go#L45-L64`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/internal/ingress/inspector/inspector.go#L45-L64)),
  and only in the admission path: `CheckIngress`
  ([`controller.go#L313-L357`](https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/internal/ingress/controller/controller.go#L313-L357):
  `if cfg.StrictValidatePathType { ... return fmt.Errorf("ingress contains invalid paths: %w", err) }`).
  The controller's sync loop only runs `DeepInspect` (`internal/ingress/controller/store/store.go#L441-L447`,
  `inspector/ingress.go`: host, path and TLS regex checks, no path-type check), so an Ingress stored before
  the upgrade keeps being served.
- The same bug is described from the other side in the cert-manager 1.18 release notes
  ([`release-notes-1.18.md#L36-L44`](https://github.com/cert-manager/website/blob/2e32a5efc7a4ef284e271f54ac1b15e800e66fcc/content/docs/releases/release-notes/release-notes-1.18.md#L36-L44)):
  > `strict-validate-path-type` configuration option which, when enabled, disallows `.` (dot) in the path
  > value. This is a bug ... the buggy validation is enabled by default in `ingress-nginx >= 1.12.0`.
- Chart: 4.11.5 `templates/_params.tpl` renders `--enable-annotation-validation=true` only when
  `controller.enableAnnotationValidations` is true (default false); 4.12.0 renders `=false` only when it is
  false (default true). With default values neither renders a flag: B4 has no rendered counterpart.

## Reasoning in terms of the fixture

- `apps/docs/base/ingress.yaml`: `/` (Prefix, valid) and `/.well-known` with pathType
  **ImplementationSpecific**, which `ValidatePathType` skips. **B3 is not affected.**
- B1: only `ssl-redirect` (and in the observed change `proxy-read-timeout`) — below High. B2: no
  cross-namespace references. B4: valid annotation values. Not affected.

## Observed outcome

`observed/run.sh` (kind v0.24.0, Kubernetes v1.31.0, helm v3.16.1; charts from
kubernetes.github.io/ingress-nginx; shared by the pair), log `observed/run.log`. ingress-nginx is
installed with the Application's two value files in order (helm without `--wait`: in kind the
LoadBalancer Service never gets an address; the controller rollout is waited for). The docs Application
is reproduced with `kubectl apply --server-side` of the rendered overlay — its `ServerSideApply=true`
sync option, under which Argo CD sends every resource of the app to the API server on each sync.
Requests go through the controller Service with `Host: docs.example.com` from a probe pod.

| Step (run.log) | `/` | `/.well-known/security.txt` | sync / write of Ingress `docs` |
|---|---|---|---|
| baseline v1.11.5 (L111) | 200 | 200 | sync OK |
| control v1.11.5: re-sync, annotation change (L115) | 200 | 200 | OK, OK |
| after upgrade to v1.12.0 (L194) | 200 | 200 | |
| next sync, repository unchanged (L198) | | | sync OK |
| annotation change (L210) | | | admitted (exit 0) |
| controller restarted (L215) | 200 | 200 | |
| Ingress deleted, Application re-synced (L228) | 200 | 200 | sync OK |

`observed/render.diff`: identical to `--docs-eu`; no rendered change is breaking.
