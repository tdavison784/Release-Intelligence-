// Package relations is the lightweight relationship layer of the Phase 4
// investigation (docs/phase4/env-impact/DESIGN.md §2.5, §7; MISSION §A3).
// It is secondary context: the rendered before/after diff is the centerpiece
// evidence, and this package only answers "which consumers reference this
// changed object?" — with evidence, deterministically.
//
//   - NewBuilder builds the RelationshipGraph of a bundle from the rendered
//     objects of its environment renders and the environment's manifest
//     files (Secret/ConfigMap/ServiceAccount references of pod templates,
//     Ingress backends/TLS/class, Service selectors, CR → API type, GitOps
//     and Kustomize structure).
//   - NewExposure maps change items to exposure roots: the objects of the
//     environment rendered changes correlated to each item first, then the
//     subjects of the deterministic join's matches.
//   - NewTraverser runs the bounded depth-first search "graph.dfs@v1".
//
// Everything is generic: relationships come from Kubernetes field semantics
// and from declarative ProducesRule values, never from product-specific
// code. The package depends on internal/investigate (ports), internal/domain
// and internal/render (object identity and parsing) only.
package relations
