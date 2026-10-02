# crossplane-1.20-2.0: research notes

Transition: **v1.20.1 → v2.0.0**. Authored blind on 2026-10-01 from upstream material only. No `ri`
output, stored results, adjudications or testdata were consulted.

## Tags

- `gh api repos/crossplane/crossplane/releases`: v1.20.1 was published 2025-08-08 and v2.0.0 on
  2025-08-14. v1.20.1 was the newest v1.20 patch when v2.0.0 shipped, and the v2.0.0 release body
  ends with `Full Changelog: …/compare/v1.20.1...v2.0.0`. Both tags match the definition's
  `tagPattern`.
- The v2.0.0 notes recommend the latest 2.0 patch (v2.0.2 at the time). The case still targets
  v2.0.0, the release the docs and notes describe. None of the expected items changes in 2.0.1 or 2.0.2.

## Sources and what each contributed

| Source | Used for |
|---|---|
| v2.0.0 GitHub release body | Removed-features list (E1–E5), sequential upgrade (E9), metric renames (E10), PR titles (#6520 controllerConfigRef, #6473 XR connection secrets, #6548 default registry, #6640 metrics). Go module path, `xpkg login/logout` and namespaced-MR `deletionPolicy` were read but left out (see below). |
| docs `v2.0-archive:content/v2.0/whats-new/_index.md` | The "Backward compatibility" section. This is the page the definition's `whats-new-major-archive` source reads. `master` no longer has `content/v2.0`. |
| docs `v2.0-archive:content/v2.0/guides/upgrade-to-crossplane-v2.md` | Removed features with migration commands, the prerequisites, the v1.20-only upgrade path, provider upgrade step 4 (E7), and legacy resource behaviour. |
| docs `v2.0-archive:content/v2.0/get-started/install.md` | Prerequisites: only "an actively supported Kubernetes version" and Helm ≥ 3.2.0. No hard Kubernetes floor, so the case has no cluster-version item. |
| Chart `values.yaml` at both tags | The diff only **adds** `image.ignoreTag`, `provider.defaultActivations` and `extraEnvVarsCrossplaneInit`, plus comment changes. No key is removed, which grounds the `impact:values-removed` notExpectedFinding. `args` is passed verbatim to `core start` in `templates/deployment.yaml` at both tags, which makes values `args` the place where the core flags of E3/E4/E8 live. |
| `cluster/crds` listing at both tags | Removed: `pkg.crossplane.io_controllerconfigs`, `secrets.crossplane.io_storeconfigs`. Added: MRD/MRAP, `ops.crossplane.io` (Operation, CronOperation, WatchOperation), `protection.crossplane.io` Usage/ClusterUsage. |
| Composition CRD at both tags | `spec.mode` default goes from `Resources` to `Pipeline` and its enum from `[Resources, Pipeline]` to `[Pipeline]`. `spec.resources`, `spec.patchSets` and `spec.publishConnectionDetailsWithStoreConfigRef` are gone (E1, E3). |
| Provider/Function CRDs at both tags | `spec.controllerConfigRef` is removed (E2). `spec.package` gains the CEL rule "must be a fully qualified image name…" (E4). |
| XRD CRD at v2.0.0 | v1 is `deprecated: true` with its warning (E6). v1 `scope` defaults to `LegacyCluster`, v2 defaults to `Namespaced`. The `connectionSecretKeys` description says "Only LegacyCluster XRs support connection secrets" (E5). |
| `cmd/crossplane/core/core.go` at both tags; commit 8bf617ac | Flag handling. At v2.0.0, `--enable-composition-webhook-schema-validation`, `--enable-external-secret-stores` and `--registry` are hidden and `Run` returns an error when they are set. The hidden GA no-op flags `--enable-composition-revisions/-functions/-functions-extra-resources` of v1.20.1 are deleted outright (E8, E3, E4). |
| `cmd/crossplane/main.go` at v1.20.1 | `DefaultRegistry = "xpkg.crossplane.io"`. This is how the unqualified Function package resolved before the upgrade (E4). |
| `internal/controller/apiextensions/{composite,definition}/reconciler.go` at v2.0.0 | Confirms that legacy XRs still get a connection-secret publisher (E5). |
| provider-upjet-aws and function-patch-and-transform releases | Real versions for the fixture: provider v1.23.0 (2025-06-13), v2.0.0 (2025-08-12), and function-patch-and-transform v0.9.0 (2025-05-12). |

## Items and G22 categories

| Id | Class (this env) | Generic | G22 category |
|---|---|---|---|
| E1 P&T `mode: Resources` removed | action-required | action-required | migration; CRD default change (`Resources`→`Pipeline`) |
| E2 ControllerConfig removed | not-affected | action-required | removal / migration (`replacedBy` DRC, `runtimeConfigRef`) |
| E3 External secret stores removed | not-affected | action-required | feature gate (alpha flag now fatal), removal |
| E4 Default registry removed | action-required | action-required | compatibility hidden in prose (validation tightened; dependencies too) |
| E5 XR connection details removed (modern XRs only) | not-affected | action-required | prose that overstates, narrowed by the CRD and code (behaviour) |
| E6 XRD v1 deprecated | review-required | review-required | deprecation |
| E7 Providers to v2 for namespaced MRs | informational | informational | cross-product version (`product-relationship` + `product-version`) |
| E8 Retired core flags break startup | action-required | action-required | feature gates (beta gate removed, GA no-op flags deleted); compatibility found in code |
| E9 Upgrade only from v1.20 | not-affected | action-required | migration precondition (`edge-from-version`) |
| E10 Metric renames | unknown (undecided) | review-required | behaviour change |

RBAC: the only RBAC changes in the transition are internal to the RBAC manager (#6677 "move crd view
rbac rules to provider system role", #6521 "Don't label RBAC roles with the XRD"). There is also the
"grant Crossplane access to compose resources that aren't Crossplane resources" tip for new v2 usage.
Neither creates an upgrade obligation for an existing v1.20 cluster, so the case has no RBAC item.

## Judgement calls

- **E1 classification and kind.** The upgrade guide says "migrate to composition functions *before
  upgrading*", so the consequence is `migration-required`. The two semantic entries are `spec.mode`
  (`validation-tightened`, `before: "Resources"`: the value that was accepted and was the default at
  v1.20) and `spec.resources` (`removed`, `replacedBy spec.pipeline`). The exposure uses `any` of
  "mode equals Resources" and "resources set" because a v1.20 Composition that omitted `mode` defaulted
  to Resources. The fixture's Pipeline copy is the scoped distractor. It must not be enough on its own,
  and the `resource` scope picks the P&T Composition. Exposure, overlap and transfer all happen inside
  one link per item, so the "not-affected" Pipeline Composition cannot be a link of its own.
- **E2 not-affected.** The environment has migrated to DeploymentRuntimeConfig (`runtimeConfigRef`).
  No ControllerConfig is applied and no package sets `controllerConfigRef`.
- **E3 not-affected.** The values `args` hold four flags, none of them `--enable-external-secret-stores`.
  No StoreConfig is applied and no Composition sets `publishConnectionDetailsWithStoreConfigRef`. The
  generic reading is action-required, because for anyone who enabled ESS the v2 pod refuses to start
  (core.go).
- **E4 affected.** The Function package `crossplane-contrib/function-patch-and-transform:v0.9.0` has no
  registry host. The upgrade guide uses this same shape (`crossplane-contrib/provider-aws-s3:v1.23.0`)
  as its ❌ example. The pattern `^"?[^./"]+/` flags a first path segment without a dot, which is what
  the v2 CEL rule `^[^\.\/]+(\.[^\.\/]+)+…` rejects. The optional leading quote tolerates matching
  against the JSON-encoded value. The upstream note that dependencies of Configurations must also be
  qualified cannot be checked from manifests. The fixture has no Configuration, so that part is not
  needed here.
- **E5 not-affected, against the headline prose.** The release notes say "Composite resources no longer
  have native connection details support". At v2.0.0 the XRD CRD ("Only LegacyCluster XRs support
  connection secrets") and the reconcilers (a secret publisher is wired only for `LegacyCluster`) show
  that the removal applies only to modern Namespaced/Cluster XRs. A v1.20 cluster only has v1 XRDs.
  They default to `LegacyCluster`, so its claim connection Secret keeps being written. The exposure is
  "a v2 XRD is in use, or an XRD sets scope Namespaced/Cluster", which is false here. This item tests
  whether a system follows the prose or the shipped behaviour.
- **E6 review.** A deprecation warning on apply. Nothing breaks in v2.0.
- **E7 informational.** The guide's step 4 is a recommendation ("Your existing cluster-scoped MRs
  continue working unchanged"). The consequence is `none`, so it is informational. The `>=2.0.0` bound
  for `provider-aws-s3` comes from the guide's own `provider-aws-s3:v2.0.0` example. The product id
  `provider-aws-s3` is not in `products/`. It is the package name the guide uses.
- **E8 affected, sourced from code.** No prose mentions this item. The v2.0.0 `core.go` returns
  "Crossplane now uses CEL to validate Compositions…" when
  `--enable-composition-webhook-schema-validation` is set. That flag defaulted to true in v1.20, so
  only clusters that pass it explicitly are exposed, which is the fixture's situation (left over from
  older releases). `--enable-composition-functions` was a hidden no-op in v1.20.1, kept "to avoid
  breaking folks who are passing them", and the flag is deleted in v2.0.0 (commit 8bf617ac).
  - That an unknown flag makes `core start` fail is inferred from Crossplane's Kong command-line
    parser, which rejects undeclared flags. The upstream text does not state it, so this is the least
    certain sub-claim. The schema-validation flag alone makes the link action-required.
- **E9 not-affected.** `edge-from-version` out-of-range of `>=1.20.0, <1.21.0` is false for v1.20.1.
- **E10 undecided.** Metrics are enabled (`metrics.enabled: true`), so the series are scraped. The
  dashboards and alert rules that would break are "maintained outside this repository" (stated in the
  description), so the reason is `environment-visibility-gap`. Subject family `api-endpoint` (component
  `metrics`) was chosen because the catalog has no metric family. A metric series is part of the
  `/metrics` interface. The consequence is `behavior-change` and not `setting-ignored`: nothing in the
  product stops working, only external queries go empty.
- **values-key on `args`.** In values flattening, `args` is a sequence leaf whose value is the encoded
  list. The `matches` patterns end in a delimiter class (`$`, `"`, `,`, `]`, space) so they hold whether
  the engine matches the whole encoded list or individual `args[]` elements.
  - The error-returning flags (`--enable-external-secret-stores`,
    `--enable-composition-webhook-schema-validation`) only count bare or `=true` spellings, because
    `=false` does not trigger the error.
  - The deleted GA no-op flags count any spelling (`=` is in the delimiter class), because the parser
    rejects the flag whatever its value.
- **Left out.**
  - The Go module path `/v2` affects only code that imports Crossplane.
  - `crossplane xpkg login/logout` is a CLI-only change.
  - `deletionPolicy` was removed for namespaced MRs, but only on new v2 MRs: "Existing cluster scoped
    managed resources are not affected".
  - The apiextensions `Usage` deprecation (CRD `deprecationWarning` "migrate to protection.crossplane.io
    Usage or ClusterUsage") was dropped to stay within 10 items. It is a good candidate for a transfer
    case.
- **notExpected.** Only Renovate CI-action digest bumps from the release body are listed. The Go module
  path change is real release information for developers, so it is not listed as a false positive.

## Semantic labels: non-obvious choices

- Several items bundle subjects:
  - E1: `spec.mode` and `spec.resources`.
  - E2: the ControllerConfig GVK and `Provider.spec.controllerConfigRef`.
  - E3: the flag and the StoreConfig GVK.
  - E4: `spec.package` on Function, Provider and Configuration, plus the `--registry` flag.
  - E8: one cli-flag per flag in the fixture.
  - E10: one rename per metric prefix.
- The cli-flag `component` is `crossplane`, the chart's container name (`{{ .Chart.Name }}`).
- E9 uses the `migration` family (`sequential-minor-upgrade-via-v1.20`, `migration-required`). The
  upgrade-from requirement is a precondition on the edge and not a peer product's version, so
  `requirement-changed` on `product-relationship` did not fit.
- E5's subject is `XRD.spec.connectionSecretKeys` `removed`. The narrowing to modern scopes lives in the
  exposure condition and the `note`, not in the subject.
