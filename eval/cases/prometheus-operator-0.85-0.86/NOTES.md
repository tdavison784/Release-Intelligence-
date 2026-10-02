# Research notes: prometheus-operator v0.85.0 → v0.86.2 (with environment)

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Choosing the transition

I surveyed the CHANGELOG sections for 0.70–0.89 (main and tag copies). Candidates:
0.78→0.79 (Prometheus v3 becomes the default), 0.80→0.81 (`PrometheusShardRetentionPolicy` gate,
OpenStack role removal) and 0.85→0.86. 0.85→0.86 has the widest spread of operator-facing work:
two RBAC changes (status subresources behind a feature gate, and the `events.k8s.io` switch), a
default operand version bump that the CHANGELOG never mentions, two Prometheus-version-gated
behaviour changes stated only in prose (">= 3.0.0", ">= 3.4"), tightened CRD validation, and the
compatibility page's Kubernetes floor. `to` is **v0.86.2**, not v0.86.0: the events RBAC change
shipped broken in v0.86.0 (issue #8076) and the CHANGELOG states the permission requirement only
in the 0.86.2 section; v0.86.2 is the release an operator upgrading today would install. The case
id keeps the minor pair (`0.85-0.86`).

## Sources read

- CHANGELOG.md at v0.86.2 (sections 0.86.2, 0.86.1, 0.86.0) and on main (survey of 0.70–0.94).
  The product definition ingests exactly these sections (`changelog` source).
- `Documentation/getting-started/compatibility.md` at v0.85.0 and v0.86.0: the Kubernetes section
  (the ingested source) is unchanged ("requires Kubernetes >= v1.16.0", client-go v0.33.3 → v0.34.1);
  the Prometheus section adds v3.6.0 to the e2e matrix and moves the e2e default v3.5.0 → v3.6.0.
- `pkg/operator/defaults.go` at both tags: `DefaultPrometheusVersion` is the last entry of
  `PrometheusCompatibilityMatrix`; v0.86.0 appends "v3.6.0". Alertmanager (v0.28.1) and Thanos
  (v0.39.2) defaults are unchanged — the Alertmanager in the fixture is a deliberate look-alike.
- `example/rbac/prometheus-operator/prometheus-operator-cluster-role.yaml` at v0.85.0, v0.86.0,
  v0.86.2: v0.86.0 adds `scrapeconfigs/status`, `podmonitors/status`, `probes/status`; v0.86.2
  switches the events rule from `""` to `events.k8s.io`. `example/rbac/prometheus` (unchanged).
- `bundle.yaml` at v0.85.0 (operator Deployment the fixture copies: args, image, securityContext)
  and v0.86.2.
- `pkg/operator/feature_gates.go` (v0.85.0), `pkg/operator/config.go` (v0.86.2),
  `Documentation/platform/operator.md` (v0.86.2): `StatusForConfigurationResources` exists since
  v0.85.0 and defaults to `enabled: false`; the status-subresource proposal says how to enable it.
- `cmd/operator/main.go` (v0.86.2): startup SelfSubjectAccessReview for `events.k8s.io` events
  (create, patch); on failure it logs "missing permission to emit events" and disables events.
- `pkg/prometheus/server/operator.go` (v0.86.2): `updateConfigResourcesStatus` returns an error
  ("failed to update PodMonitor ... status") when a binding update fails — the concrete failure
  for E2.
- CRDs at both tags: ServiceMonitor already had the status subresource at v0.85.0; PodMonitor,
  Probe and ScrapeConfig gain it at v0.86.x. ScrapeConfig: Eureka `server` gains
  `pattern: ^http(s)?://.+$`, Kuma `server` gains `pattern: ^https?://.+$`, the Hetzner SD `port`
  gains `minimum: 0` / `maximum: 65535`. CRD file set identical at both tags (ten CRDs).
- PRs/issues: #8077 + #8076 (events RBAC), #7867/#7953 (events API switch), #7893 + #7889
  (metadata-wal-records), #7637/#7985 (UTF-8), #7939 (managed-by label), #7856/#7823/#7966
  (ScrapeConfig validations; #7835/#7838 are test-only).

## Items and G22 categories

| Item | What | Class | G22 category |
|---|---|---|---|
| E1 | events.k8s.io events permission now required | action-required | RBAC (+ migration) |
| E2 | `*/status` permissions for PodMonitor/Probe/ScrapeConfig when the gate is on | action-required | RBAC + feature gate |
| E3 | default Prometheus v3.5.0 → v3.6.0 | review-required | prose-only default (changed default not in the CHANGELOG) |
| E4 | metadata-wal-records no longer injected for Prometheus >= 3.4 | not-affected here (generic review-required) | behaviour change + cross-product version dependency (prose ">= 3.4") |
| E5 | UTF-8 names for Prometheus >= 3.0.0; webhook `--name-validation-scheme` | review-required | cross-product version dependency hidden in prose + new CLI flag |
| E6 | ScrapeConfig validation tightened | not-affected here (generic action-required) | CRD validation (deprecation/removal analogue) |
| E7 | Kubernetes >= v1.16.0 restated | informational | compatibility hidden in prose |
| E8 | managed-by label on all managed resources | informational | behaviour change (benign) |

There is no CRD-field or CLI-flag deprecation/removal in this edge (the "Deprecated:" CRD
descriptions differ only by case/wording between the two bundles); the closest are the removal of
an operator-injected Prometheus flag (E4) and validation tightening (E6).

## Judgement calls

- **E1/E2 are action-required only because RBAC is hand-maintained.** Anyone applying the
  v0.86.2 `bundle.yaml` gets the fixed ClusterRole. The environment description states the
  ClusterRole is an in-house copy of the v0.85.0 example that the upgrade does not touch, which is
  the common GitOps/kustomize setup and what issue #8076 hit through a chart.
- **E1 consequence kind** `permission-lost`: the operator keeps reconciling, but the Events that
  surface invalid objects silently stop — a loss of intended behaviour with a concrete log line.
- **E2's subject is the permissions, the gate is the exposure.** The gate pre-dates v0.86 and is
  off by default, so a gate-off cluster is not affected even with the v0.85.0 ClusterRole. The
  fixture turns it on (the team adopted it for ServiceMonitor status at v0.85.0) and selects a
  PodMonitor and a ScrapeConfig. `probes/status` is missing too but no Probe exists, so the
  exposure pairs each permission with use of its kind.
- **E3 is review, not action:** v3.5 → v3.6 is a Prometheus minor bump the operator e2e-tests; the
  cluster should notice its Prometheus is rolled, but nothing is known to break. Labelled as a
  `crd-field` default change on `Prometheus.spec.version` (the knob the operator reads) rather
  than an `image` value change; `spec.image` also shields, so both must be unset.
- **E4 not-affected** (looks similar: Prometheus >= 3.4, remote write configured): the operator
  only added the flag for `messageVersion: V2.0` endpoints (PR #7893 diff), and the fixture's
  remote write sets no messageVersion.
- **E6 not-affected** (looks similar: a ScrapeConfig is in use): it uses only `staticConfigs`.
  Patterns are written `^"?https?://.+` to be robust to whether the evaluator
  matches JSON-encoded or raw scalars.
- **E7 informational:** the floor did not move and the cluster (1.32) is far inside it. Per
  docs/ACTION_CLASSIFICATION.md "a cluster version inside the supported range" is informational,
  so the link carries the out-of-range exposure (false) plus an in-range overlap (true). The item
  exists because the product definition ingests this section and classifies "requir… Kubernetes"
  as action-required — a correct system must not.
- **Item classification follows the link** (groundtruth-lane convention: the scorer compares
  `classification` with the class the engine outputs in this case's environment). E4 and E6 are
  therefore `not-affected`; their generic readings — review-required for E4, action-required for
  E6 (an invalid value is rejected outright) — are kept in YAML comments and in the semantics'
  `exposedClass`. E7 is informational both generically and here.
- **notExpected:** the 0.86.1 event-formatting bugfix and the config-reloader port-name bugfix
  must not be raised as action items.
- **No undecided link:** every link is decided by the fixture (RBAC, args, CRs, inventory).

## Semantic labels

- E1/E2 use `rbac-permission` with `name` = the resource (`events`, `podmonitors/status`, …),
  `group` = its API group and `component: prometheus-operator` (whose ServiceAccount needs it);
  change `now-required` (the permission was not needed by v0.85.0 for these kinds).
- RBAC exposure is a `resource` scope on the ClusterRole **by name** with
  `not(field rules[].apiGroups[] / rules[].resources[] equals …)`. The name scope matters: the
  Prometheus server's ClusterRole (`prometheus`) also lacks `events.k8s.io` but does not need it.
- E2's `feature-gate` leaf uses `component: prometheus-operator` (the container carrying
  `--feature-gates`); the Prometheus pods use `--enable-feature`, a different mechanism.
- E4's subject is a `cli-flag` on the Prometheus container
  (`--enable-feature=metadata-wal-records`, component `prometheus`) with `behavior-changed`: the
  flag still exists in Prometheus, the operator just stops injecting it. Its exposure combines the
  cross-product `product-version prometheus in-range >=3.4.0` leaf with a scoped
  `messageVersion equals "V2.0"` over Prometheus and PrometheusAgent.
- E5 has two subjects: a `protocol-behavior` (UTF-8 name handling, review) and the new
  admission-webhook `cli-flag` (`added`, consequence `none`). Exposure:
  `product-version prometheus in-range >=3.0.0` ∧ a Prometheus/PrometheusAgent in use.
- `prometheus` and `alertmanager` are not catalog products; the inventory lists them under those
  ids because the environment description states their versions and E4/E5 need the Prometheus one.
- E7 uses `compatibility-boundary` / `requirement-changed` with `after: '>=1.16.0'` and no
  `before` — the floor is restated, not moved; `requirement-changed` is the only change type that
  carries a platform range.
- E8 uses `protocol-behavior` (`managed-by-label`), consequence `none`.
