# external-secrets-0.18-0.19--orders-eu: notes (ground truth; never read by a system under test)

Pair `external-secrets-0.18-0.19--crd-delivery` with `external-secrets-0.18-0.19--orders-br`. Upgrade:
External Secrets Operator v0.18.2 → v0.19.2 (chart 0.18.2 → 0.19.2, `https://charts.external-secrets.io`),
delivered by an Argo CD Application in this repository. Coverage (PLAN-M6 §2): 1 (a product RI has not
been tuned on; `products/external-secrets.yaml` defines it, tags since 0.9.0).

## Why the two environments differ

Same values, stores and consumers. This repository **declares how the add-on is delivered**: an Argo CD
Application with `syncOptions: [CreateNamespace=true]` (client-side apply, Argo CD's default). In
orders-br the add-on is delivered by the platform team's ApplicationSet in another repository, so the
deciding input is absent.

## B1: CRDs too large for client-side apply (decisive) — affected, observed

Upstream:
- Release v0.19.0, https://github.com/external-secrets/external-secrets/releases/tag/v0.19.0 (published
  2025-08-02): "🔴 🔴 BREAKING CHANGE 🔴 🔴 Please note that this a breaking change because our CRDs are now
  too big. Meaning a simple kubectl apply or Argo's default client side apply WILL NOT WORK! You have to add
  `--server-side` to kubectl apply and in argo add: … syncOptions: - CreateNamespace=true -
  ServerSideApply=true".
- Getting started at the chart tag,
  https://github.com/external-secrets/external-secrets/blob/helm-chart-0.19.2/docs/introduction/getting-started.md#L12:
  "Note: Our CRDs have reached the 256KB limit! You have to use server-side-apply in all locations to install
  them correctly."
- Mechanism: a client-side apply stores the full object in the `kubectl.kubernetes.io/last-applied-configuration`
  annotation, and a Kubernetes object's annotations may total at most 262144 bytes.

In this environment: `clusters/prod/external-secrets.yaml` (chart 0.18.2, values
`platform/external-secrets/values.yaml` with `installCRDs: true`) has no `ServerSideApply=true`; the
`argocd` Application next to it has it, so the cluster's Argo CD supports it and simply is not told to
use it for this Application.

Observed (kind v0.24.0, kindest/node v1.31.0; a real Argo CD v3.1.5 installed from argo-cd chart 8.5.1 with
`platform/argocd/values.yaml`, the env's AppProject; `observed/run.sh … env`, raw log `observed/run.log`).
Substitution: the Application's multi-source `$values` reference is replaced by the same values inlined as
`helm.valuesObject` (no Git server in kind); sync policy and options are the environment's.
- L58–L62: in the environment's render the store CRDs serialise to 259438/259391 bytes at 0.18.2 and
  276398/276351 bytes at 0.19.2.
- L153–L176 (FROM 0.18.2): sync Succeeded; last-applied annotations of 259439 and 259392 bytes (just under
  the limit); a fake-provider probe ExternalSecret syncs.
- L201–L214 (TO 0.19.2): `operation: Failed | one or more objects failed to apply, reason: error when
  patching …: CustomResourceDefinition.apiextensions.k8s.io "clustersecretstores.external-secrets.io" is
  invalid: metadata.annotations: Too long: must have at most 262144 bytes` (same for secretstores);
  Deployments `configured`; condition `SyncError`; the Application stays OutOfSync.
- L215–L223: all three Deployments run v0.19.2; both store CRDs still have the 0.18.2 schema (schema hash
  compared with the two renders); `externalsecrets` reached 0.19.2.
- L226–L236: the probe ExternalSecret still syncs (`to-0.19.2`). The environment's own AWS store cannot be
  exercised from kind (no AWS credentials; the controller log at L238–L250 shows the expected STS errors).

So the proposed change does not apply: Argo CD reports a failed sync on every attempt, the operator runs
0.19.2 controllers against 0.18.2 store CRDs (new 0.19 store fields would be pruned), and the GitOps state
of the add-on stays broken until the sync options change. That is the class `affected` here, with the
remediation below; secret delivery itself was not interrupted in the observation.

Render: `observed/render.diff` — L258–L861 and L929–L1532 grow the two store CRDs by ~550 lines each (new
provider auth options), past the limit (B1, breaking under client-side apply); a new `sshkeys` generator
CRD and RBAC/selectable-field additions are harmless.

Remediation: add `ServerSideApply=true` to the Application's syncOptions before the bump (observed to work
in orders-br/observed/run-ssa.log), or manage the CRDs separately with server-side apply.

## Other 0.18.2 → 0.19.2 changes checked (not scored)

Release notes v0.19.0–v0.19.2: removal of the JWT-token auth option of the STSSessionToken generator (no
generators used); otherwise fixes and features. No security regression: GitHub global advisory database,
`github.com/external-secrets/external-secrets`, read 2026-10-27: 4 advisories affect v0.18.2, 3 affect
v0.19.2, none introduced.
