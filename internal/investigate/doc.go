// Package investigate is the Phase 4 environment-aware breaking-change
// investigation: given a dependency upgrade and a target environment, collect
// change and environment evidence into a content-addressed bundle, build the
// relationship graph, traverse it with a bounded deterministic DFS, let an
// AI agent inspect the bundle and request evidence and validations, run
// deterministic validators, and classify every in-scope change item into the
// four MISSION classes with a deterministic, fail-closed policy
// (docs/phase4/env-impact/DESIGN.md).
//
// This package holds the PORTS (api.go): the stage interfaces the
// orchestration layer calls, the artifact/bundle store, the agent's tool
// surface and driver, and the error classes that map to retry policies.
// Implementations live in the wave-1/2 packages named in DESIGN.md §7
// (investigate/bundle, investigate/graph, investigate/validate,
// investigate/agent, investigate/classify), each depending on this package
// and internal/domain, never on each other's internals.
//
// Boundary rules:
//   - No go.temporal.io import here or in any implementation package
//     (MISSION §A7; FLEET.md "Temporal stays at the edge"). Every port takes
//     a context and plain, JSON-serialisable values, so internal/orchestrate
//     can wrap each one in an activity and tests can call them directly.
//   - Large content never travels through a port as bytes: artifacts are put
//     into the ArtifactStore and referenced by domain.ArtifactRef (Temporal
//     payloads stay small; investigations stay reproducible).
//   - AI output enters only through the Tools surface and is recorded as
//     agent records with AI provenance; nothing an agent says is a
//     ValidationRecord or an InvestigationFinding.
package investigate
