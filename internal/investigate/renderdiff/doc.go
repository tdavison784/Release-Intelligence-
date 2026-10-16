// Package renderdiff turns the environment's own before/after renders into
// indexed, citable evidence (DESIGN.md §2.4, the Phase 4 centerpiece) and
// serves the diff queries the investigation agent's tools call.
//
// RenderEnvironment (investigate.EnvironmentRenderer) renders the product's
// from and to releases with every deployment of it the environment declares
// (Argo CD / Flux / Helmfile / values files / Kustomize overlays, as
// internal/render detects them), diffs and correlates them with the upgrade
// edge, and returns one domain.ManifestPair per target:
//
//   - the from/to renders (normalized, sensitive values replaced by digest
//     markers) and the diff are stored as artifacts;
//   - a complete pair (both renders succeeded, values complete, chart
//     coverage holds) indexes one domain.RenderedChange per render.Change,
//     correlated to change items (render.Correlate links, plus the items
//     anchored on the matching release-scope rendered change);
//   - an incomplete pair carries its reason and NO changes: an incomplete
//     render is an evidence gap, never "no change";
//   - every rendered change cites a rendered-diff evidence record whose
//     content digest is the diff artifact's and whose locator
//     (changes[id=<rc>]) names its entry, so it resolves to the bytes.
//
// Query, Object and Summaries read a bundle's pairs for the agent: filtered,
// deterministically ordered, bounded pages with a continuation cursor.
package renderdiff
