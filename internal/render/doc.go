// Package render is the rendering-validation layer (docs/RENDER.md,
// docs/phase3/learning-loop/RENDER-MISSION.md): it renders a product's source
// and target release artifacts with the customer's configuration (Helm
// `helm template`, Kustomize `kustomize build`), normalizes the rendered
// manifests into semantic Kubernetes objects, and diffs them — objects, not
// text — into typed rendered changes (resource-added, image-changed,
// rbac-permission-removed, …). Those changes are first-class evidence:
//
//   - release-level renders (chart defaults, scope "release") back the
//     `rendered-diff` validator (validator.go), which confirms or refutes the
//     subject and change aspects of prose-derived proposals;
//   - environment renders (the customer's values, scope "environment") back
//     the `rendered-change` condition predicate (evaluate.go) and the
//     `ri impact --render` / `ri render diff` views. They never enter
//     knowledge/ or a prompt.
//
// Renderers are external binaries behind the Renderer port (no Helm or
// Kustomize Go SDK). Every failure is explicit (Failure) and becomes an
// evidence gap, never "no change". Every render records its full provenance
// and is cached by a content address over that provenance.
//
// A render difference alone never produces ACTION REQUIRED: rendering proves
// the structural change, not its operational consequence.
package render
