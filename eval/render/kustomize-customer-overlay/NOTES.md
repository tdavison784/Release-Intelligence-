# NOTES — kustomize-customer-overlay (render case)

## How this was researched

Read the fixture (`internal/env/testdata/customer-repo/kustomize/kustomization.yaml`
and the `base/` manifests it references) and kustomize's resource/load-restrictor
documentation. This case is fixture-derived on purpose: it evaluates the renderer's
kustomize path, which needs a customer overlay as input, and the repository fixture is
the honest available one (no upstream product material is involved).

## Judgement calls

- The overlay references `../base/*.yaml` — outside the overlay directory. kustomize's
  default (root) load restrictor refuses such resources, so any faithful `kustomize
  build` of this overlay fails; the renderer keeps default restrictions (a lane
  decision recorded in the lane status: fidelity to how the customer's deployer
  builds). Hence K1: the failure must be explicit (`kustomize-dependency`), an
  evidence gap, never "no change" (R13).
- K2 checks the target-level reporting contract (per-target verdict + why record), not
  a specific wording.
- The successful kustomize path (a valid overlay with the product version substituted
  into `images: newTag`, both sides rendering) is covered by the unit tests in
  `internal/render/kustomize_test.go` (skipped without the tool) rather than by an
  eval case here: building a second, valid overlay fixture is testdata work, not eval
  authoring, and the unit tests already pin the substitution semantics.

## Blind-authoring caveat

Same as the main case: the lane status file already records a live run of this
fixture ("Kustomize overlay: explicit kustomize-dependency failure"). The expectation
was re-derived from the fixture files and kustomize's documentation above.
