# ingress-nginx controller-v1.11.5 → v1.12.0 (chart 4.11.5 → 4.12.0) — shop-prod-pl (group `security-defaults`, insufficient-evidence case)

Authored blind on 2026-10-16 by the groundtruth lane (Claude Opus 5.5) from upstream sources and the
observed outcomes of the sibling cases. No Release Intelligence pipeline, agent or eval output was
run or read.

## The environment

Same applications as shop-de/shop-fr (storefront with the overlay-added `configuration-snippet`,
catalog-api with Low-risk annotations), but **the ingress-nginx value files are not in this
repository**: the `ingress-nginx` Application takes them from a second source,
`https://git.example.com/platform/ingress-values.git` (`shop/common.yaml`, `shop/prod-pl.yaml`),
which `input.yaml` lists under `withheld`. There is no `platform/` directory.

## Upstream sources

1. Release controller-v1.12.0, release body (the "⚠️ Enable security features by default (#11819)"
   entry and its four bullets): https://github.com/kubernetes/ingress-nginx/releases/tag/controller-v1.12.0
2. PR #11819 "Enable security features by default", merge commit 7b4e4e2f:
   https://github.com/kubernetes/ingress-nginx/pull/11819 ("Lowers the acceptable annotation risk from
   Critical to High / Disables cross namespace consumption / Enables strict path validation / Also makes
   validation enabled by default"); its diff flips `AllowCrossNamespaceResources`, `AnnotationsRiskLevel`
   and `StrictValidatePathType` in `internal/ingress/controller/config/config.go` and the
   `--enable-annotation-validation` flag default in `pkg/flags/flags.go`.
3. Source at tag controller-v1.12.0 (https://github.com/kubernetes/ingress-nginx/blob/controller-v1.12.0/...):
   - `internal/ingress/controller/config/config.go` L97-L106 (semantics of
     allow-cross-namespace-resources and annotations-risk-level), L726-L730 (strict-validate-path-type),
     L753-L757 and L897 (the new defaults).
   - `internal/ingress/annotations/snippet/main.go` L36 (configuration-snippet is
     `AnnotationRiskCritical`), L67-L68 (checked against the configured level).
   - `internal/ingress/annotations/parser/validators.go` L246-L255 (`CheckAnnotationRisk`: "annotation
     %s is too risky for environment" — runs whether or not value validation is enabled) and L234-L240
     (value validators run only `if EnableAnnotationValidation`).
   - `internal/ingress/annotations/annotations.go` L185-L188 (`Extract` returns an error for the whole
     Ingress when any annotation group fails `Validate`) and
     `internal/ingress/controller/store/store.go` L936-L940 (on that error `syncIngress` returns
     without storing the Ingress: the controller stops serving it).
   - Risk of the other annotations used: `proxy/main.go` (proxy-read-timeout Low, proxy-body-size
     Medium), `cors/main.go` (enable-cors Low).
4. Chart 4.12.0 vs 4.11.5 (`helm show values`, templates): `controller.enableAnnotationValidations`
   default false → true; `templates/_params.tpl` renders `--enable-annotation-validation` only when it
   deviates from the chart default (4.11: `=true` when enabled; 4.12: `=false` when disabled);
   `controller-configmap.yaml` renders `allow-snippet-annotations` and `controller.config` verbatim and
   sets no risk level of its own.
5. Field reports of the same mechanism after upgrading to 1.12 (whole Ingress dropped by a failing
   annotation check): https://github.com/kubernetes/ingress-nginx/issues/13092 (validation) and
   https://github.com/kubernetes/ingress-nginx/issues/12764 (auth-signin value rejected).

Chart tarballs (https://kubernetes.github.io/ingress-nginx): ingress-nginx-4.11.5.tgz sha256
`7035b6656a07f85792d0d8e103b9bff84510db0e81d11db4e322709b07783ffa` (controller v1.11.5),
ingress-nginx-4.12.0.tgz sha256 `6445180ad70a12ed91d1430c7b54ed0cc65be6adfc534d23d5cb01c79734cff9`
(controller v1.12.0).
## Why the honest answer is insufficient-evidence (B1)

- The storefront Ingress is exposed: it carries a Critical-risk annotation, and the 1.12 default
  rejects Critical annotations.
- Whether it breaks depends only on `controller.config.annotations-risk-level` (and on
  `controller.allowSnippetAnnotations`) in the withheld value files. The two siblings show that both
  outcomes are real for byte-identical workloads: shop-de (no override) → storefront dropped, 404,
  admission denied; shop-fr (`Critical`) → served unchanged.
- MISSION Principle 7 / §A6: missing context must prevent an automatic safe determination; the
  finding must name the gap and the next step (fetch those two files, or the live controller
  ConfigMap). `insufficient-evidence` is the primary label. `potentially-affected` is listed as
  `acceptable` because the definitions overlap here ("a plausible impact exists but cannot yet be
  confirmed or dismissed"); both are fail-closed. The scored label is `insufficient-evidence`; `acceptable` is
  informational (FORMAT.md, groundtruth additive section). `not-affected` or `affected` would be unsupported
  claims and count as wrong.
- The fact that snippets are used at all hints that the withheld values allow them; it says nothing
  about the risk level. Guessing from it is exactly the unsupported assumption the policy forbids.

## B2–B4: determinable despite the gap

No Ingress references another namespace (B2), all paths are plain Prefix paths (B3), and every
validated annotation value is well-formed (B4). None of these depends on the withheld value files,
so they are `not-affected` here too. A system that marks every item insufficient because one input
is missing is over-cautious and scores misses on them (not a safety failure, but a coverage one).

## Observed outcome

Not run separately: the withheld input is exactly the variable, and running the case would require
inventing it. Verification level `observed-alternatives`: the two possible states of the withheld
setting were each observed on identical workloads in `../ingress-nginx-1.11-1.12--shop-de/observed/`
and `../ingress-nginx-1.11-1.12--shop-fr/observed/`.
