# Research notes: Flux v2.6.4 → v2.7.0 (with environment)

Researched blind (no Release Intelligence output consulted) on 2026-10-01.

## Why this transition

Both candidate minors were read. v2.6.0's upgrade notes are light: the OCIRepository v1 GA
bump (manifests only, v1beta2 stays served), Kubernetes minimum 1.31, and a new opt-in
gate (`DisableChartDigestTracking`). v2.7.0 has much more real upgrade work. The v1beta1/v2beta1
APIs are removed from the CRDs, `flux migrate` of stored versions is mandatory, the Kubernetes
minimum goes to 1.32, the image APIs go GA (v1beta2 deprecated), controller flags and template
fields are removed, and new feature gates arrive. `from` is v2.6.4, the last 2.6 patch (2025-07-08,
before v2.7.0 on 2025-09-30), because that is what a well-run cluster runs. The ingested window
is then the v2.7.0 release body only.

## Sources read

- flux2 GitHub release v2.7.0 (ingested channel `release-notes` + `kubernetes-compat`). It has the
  feature list (incl. both new feature gates), the Kubernetes table (`v1.32 >= 1.32.0` first row),
  the upgrade-procedure warning (APIs removed, `flux migrate` required unless Flux Operator), and
  the CLI changelog ("Set Kubernetes 1.32 as min supported version", "Implement `flux migrate`
  command"). Source for E1, E2, E3, E6, E8.
- flux2 GitHub release v2.6.0: the previous minimum (`v1.31 >= 1.31.0`), for E3 `before`.
- Flux 2.7 blog post (fluxcd.io and its markdown source in fluxcd/website). It has the full list of
  removed groups (E1), the image-reflector autologin removal and the IUA template-field removal
  under "Breaking changes" (E4, E5), `--feature-gates=GitSparseCheckout=true` (E8), the
  `spec.proxySecretRef` addition (E7) and the support table (1.32-1.34).
- Discussion #5572 "Upgrade Procedure for Flux v2.7+". It has the step order (migrate Git
  manifests, then `flux migrate` in-cluster, then upgrade) and the beta2 removal schedule (image
  v1beta2 removed in 2.9, which is E6's "removed later"). NB: it has been edited since (it now
  targets 2.9 and CLI v2.9.0); only the version-independent statements are quoted.
- PR #5473 (`flux migrate`). It says the command is a prerequisite for 2.7, gives the roadmap list
  incl. "End support for Kubernetes v1.31.x", and says Flux Operator users are exempt.
  `cmd/flux/migrate.go` at v2.7.0 rewrites `crd.Status.StoredVersions = []string{storageVersion}`,
  so the deciding environment fact for E2 is status.storedVersions.
- Issue #5598: a real failure of exactly this upgrade without migration. The CRD patch is rejected
  with `status.storedVersions[0]: Invalid value: "v2beta1": missing from spec.versions …`. It also
  shows the storedVersions shapes (`["v2beta1","v2beta2","v2"]`, `["v1beta1","v1beta2","v1beta3"]`),
  which the crds/ fixture reproduces.
- `cmd/flux/check.go` at v2.6.4 / v2.7.0: `kubernetesConstraints` goes from `>=1.31.0-0` to
  `>=1.32.0-0` (E3). `flux bootstrap` / `flux install` do not call this check, only `flux check`
  does (see the judgement call below).
- install.yaml at v2.6.4 and v2.7.0. Parsed every CRD's versions and their served/storage/deprecated
  flags. At v2.6.4 all source/kustomize/notification v1beta1 versions and helm v2beta1 are served +
  deprecated, and image v1beta1 is served (IUA v1beta1 deprecated). At v2.7.0 all of those are gone,
  image v1 becomes storage, and image v1beta2 is deprecated ("v1beta2 ImagePolicy is deprecated,
  upgrade to v1"). The IUA messageTemplate description now says "Note: The `Updated` template field
  has been removed". Provider `spec.proxy` is marked "Deprecated: Use ProxySecretRef instead". The
  CRD set is 13 → 14 (adds externalartifacts), with no CRD removed (notExpectedFindings).
- Controller CHANGELOGs at the v2.7.0 pins (`manifests/bases/*/kustomization.yaml`: source v1.7.0,
  kustomize v1.7.0, helm v1.4.0, notification v1.7.1, image-reflector v1.0.1, image-automation
  v1.0.1). From the v2.6.4 pins they cover: per-group removals with "must run `flux migrate`"
  (E1/E2), the auto-login flag removal (E4), image v1 GA plus "manifests in Git … must be updated"
  (E6), and Provider proxy deprecation (E7).
- image-reflector-controller PR #786 and main.go at v0.35.2: the three flag names, and the failure
  mode ("flag provided but not defined").
- image-automation-controller PR #931 and docs/spec/v1/imageupdateautomations.md at v1.0.1. The
  failure mode is "Templates using `Updated` will result in an error and the ImageUpdateAutomation
  will be marked as Stalled".
- `internal/features/features.go` at both pins of kustomize, helm, image-automation,
  image-reflector, source and notification. These are the feature-gate facts. **No gate was
  promoted or changed default in this transition.** New gates are all opt-in (false):
  kustomize-controller `AdditiveCELDependencyCheck`, `ExternalArtifact`,
  `CancelHealthCheckOnNewRevision`; helm-controller `AdditiveCELDependencyCheck`,
  `ExternalArtifact`; image-automation-controller `GitSparseCheckout`. Only the two the release
  body names are expected (E8). The others are the same shape.

## G22 categories exercised

| Item | Category |
|---|---|
| E1 v1beta1/v2beta1 APIs removed | gvk removal (deprecations reaching EOL), mixed API versions in the fixture |
| E2 `flux migrate` (storedVersions) | migration; the deciding fact is installed-CRD status, not manifests |
| E3 Kubernetes ≥ 1.32 | compatibility in a table plus prose/CLI-changelog (`compatibility-boundary` + `cluster-version`) |
| E4 autologin flags removed | removed controller CLI flags (`cli-flag`), not-affected direction |
| E5 IUA `.Updated` / `.Changed.ImageResult` removed | behaviour change / validation tightened, line-level text in a CR field (`text-line`) |
| E6 image v1beta2 deprecated → v1 | deprecation with replacement (gvk `replacedBy`) |
| E7 Provider proxy deprecation | deprecation of a field and of a Secret key, honestly UNDECIDED |
| E8 new opt-in gates | feature gates (`feature-gate`), not-affected direction |

No `product-relationship` item. The release does not state a cross-product version dependency
(the Flux Operator exemption for E2 is a deployment-method condition, not a version
requirement, so it lives in the description, not a condition).

## Judgement calls

- **E1 and E2 are separate items.** They are the same upstream event, but they are two distinct
  pieces of operator work with different deciding facts. E1 is about Git manifests at removed
  apiVersions (gvk-in-use). E2 is about storage versions recorded in the cluster's CRD status, and
  it applies even when every manifest in Git is already on v1. The release body states them in two
  sentences and the upgrade discussion makes them separate steps (Step 1 / Step 2).
- **E2's consequence kind is `migration-required`, not `upgrade-blocked`.** The failure is that the
  CRD apply is rejected, so the upgrade is blocked. But the cure is a mandatory operator step, and
  the `migration` subject family requires `migration-required`. The statement names the concrete
  rejection.
- **E3 is `upgrade-blocked` (ACTION).** Nothing in `flux bootstrap` refuses a 1.31 cluster. What
  fails is the documented pre-flight (`flux check --pre`, constraint `>=1.32.0-0`) and the
  post-upgrade `flux check` the procedure tells you to run. Upstream also states 1.31 is no longer
  supported ("End support for Kubernetes v1.31.x"). "A required precondition" is the closest
  DESIGN §1.4 kind. A reviewer could argue for REVIEW because no hard failure is proven. I kept
  ACTION because upstream's stated precondition is unambiguous and the cluster violates it. The
  fixture is deliberately 1.31, so this link is affected. A 1.32+ cluster would be "in range" and
  would emit an informational finding, which a `not-affected` link would wrongly count as a
  violation.
- **E4 is item class ACTION but link `not-affected`.** The image-reflector Deployment's args
  (L143-L148) contain none of the three flags. This looks similar to an exposed cluster because
  the cluster *does* pull from ECR, but it does so through the replacement
  (`ImageRepository.spec.provider: aws`, image-automation.yaml L10). That is the "uses the
  replacement" direction.
- **E5 is `validation-tightened` on `spec.git.commit.messageTemplate`.** The CRD field is unchanged
  in type, but values accepted before (templates using `.Updated`) now fail at reconcile time. The
  consequence is `workload-failure`: the automation is Stalled and stops committing. It is not
  `resource-rejected`, because admission still accepts the object. The template in the fixture is
  the long-standing docs example (`.Updated.Files`, `.Updated.Images`), so this link is affected.
- **E6 is REVIEW.** v1beta2 is still served in 2.7 (and 2.8). The CHANGELOG's "must be updated" is
  advice for after the upgrade, and the hard removal is 2.9. `actionRequired: true` records that an
  operator should consciously schedule the bump. The class is review-required (`deprecation`).
- **E7 is undecided, not not-affected.** The Provider sets no `spec.proxy` (that leg is false). But
  the deprecation also covers the `proxy` key inside the Secret named by `spec.secretRef`, and that
  Secret is SOPS-encrypted and absent from the export. So the `ref` leg is unresolved in incomplete
  manifests → unknown, and `any(false, unknown)` = unknown. Reason: `environment-visibility-gap`.
- **E8 is a `not-affected` link on an informational item.** The kustomize-controller *does* pass
  `--feature-gates` (L64), which makes it look like a gate-bearing environment, but neither new gate
  is in it. image-automation-controller passes no gates. The gates are opt-in, so nothing changes.
  The link tests that a new gate is not reported as touching a cluster that merely uses feature gates.
- **Mixed versions that must NOT count for E1.** The `apps` Kustomization and the ingress-nginx
  HelmRepository are at `v1beta2`, the image objects at `image…/v1beta2`, and Provider/Alert at
  `notification…/v1beta3`. All of these are still served in v2.7.0 (checked in install.yaml).
  Only `infra-legacy` (kustomize v1beta1, L30-L42) and the ingress-nginx HelmRelease (helm v2beta1,
  L68-L85) are exposed.
- **Items outside the ingested channels.** E4, E5 and E7 are not in the v2.7.0 release body. They
  are in the blog post, the controller CHANGELOGs and the CRD schema descriptions (E5's and E7's
  descriptions change in install.yaml, which the install-bundle CRD contents may surface). They are
  real upgrade work for this cluster, so they stay. Missing them measures channel coverage.
- notExpected: the source-watcher controller and ArtifactGenerator are opt-in
  (`--components-extra=source-watcher`) and must not be raised as ACTION. The CLI changelog's
  CI/dependabot churn should not appear at all.
- expectedFindings F1 (`impact:kubernetes-below-range`) is grounded in the v2.7.0 compatibility
  table (first row `v1.32 >= 1.32.0`) against the declared 1.31. No class is asserted for the
  finding. notExpectedFindings: no CRD is removed (13 → 14 CRDs, only versions inside them drop).

## Fixture grounding

- `manifests/flux-system-controllers.yaml`: Deployment names, container name `manager`, images and
  base args copied from install.yaml at v2.6.4 (pins source v1.6.2, kustomize v1.6.1, helm v1.3.0,
  notification v1.6.0, image-reflector v0.35.2, image-automation v0.41.2). The added args are
  customer patches: `--concurrent=20` and the kustomize/helm `--feature-gates`. Every gate named
  there exists at the v2.6.4 pins (features.go).
- `manifests/gitops-objects.yaml`, `image-automation.yaml`, `notifications.yaml`: every apiVersion is
  one served by the v2.6.4 CRDs (checked against install.yaml), so the cluster is valid today.
- `crds/flux-crds.yaml`: the 13 CRDs of v2.6.4 with their exact version lists and
  served/storage/deprecated flags. Schemas are elided. `status.storedVersions` reflects a 2021
  bootstrap upgraded in place. Every CRD that existed then lists its v1beta1/v2beta1 era. OCIRepository
  (introduced at v1beta2) does not. The shapes match issue #5598.
- `inventory.yaml`: only `flux v2.6.4`, from the description. Kubernetes is not a catalog product
  and is carried by `environment.kubernetes`.

## Item classification convention

Per the groundtruth lane's convention, `expected[].classification` is the class the engine should
output **in this case's environment**, i.e. it equals the link: E1/E2/E3/E5 action-required,
E6 review-required, E4 and E8 not-affected, E7 unknown (undecided link). The generic, environment-free
class is noted in a YAML comment where it differs: E4 action-required (workload-failure), E7
review-required (deprecation), E8 informational (opt-in gates).

## Semantic labels

- E1 and E6 use one `gvk` subject per served kind and removed/deprecated version (12 and 3), each
  with `replacedBy` the stable version of the same kind. Alert/Provider are replaced by `v1beta3`
  because notification has no v1 for them in 2.7. The exposure is the canonical
  `gvk-in-use` per subject under `any`.
- E2: `migration` family, name `flux-migrate-stored-versions`, component `flux-cli`. The exposure
  scopes to ONE CustomResourceDefinition that both belongs to a Flux group
  (`spec.group` matches `\.toolkit\.fluxcd\.io$`) and lists a removed version in
  `status.storedVersions[]`. The scope matters because another product's CRD storing some
  `v1beta1` must not trigger it. The deciding fact is installed-CRD status (crds/), not manifests.
- E3: `compatibility-boundary{kubernetes}` + `requirement-changed` (`>=1.31.0` → `>=1.32.0`).
  The exposure is canonical: `cluster-version{kubernetes, out-of-range, >=1.32.0}`.
- E4: three `cli-flag` subjects with component `image-reflector-controller`. The condition omits
  `component` because every Flux controller's container is named `manager`, so the container name
  would not discriminate. The flag names are unique to that controller.
- E5: `crd-field` + `validation-tightened`. The exposure is narrower than the canonical
  `field set`: a `text-line` over the multi-line `spec.git.commit.messageTemplate` with
  `\.Updated\b|\.Changed\.ImageResult`. Setting a template is not exposure by itself, using the
  removed data is.
- E7: a second subject (`config-key` path `proxy`, component naming the Provider's secretRef
  Secret) carries the Secret-key half of the deprecation. It has no `replacedBy` because the
  replacement is a crd-field (a different family).
- E8: `feature-gate` subjects with component = controller, `added` with `after: 'false'` (the
  default), consequence `none`. The exposure is `feature-gate … enabled`.
