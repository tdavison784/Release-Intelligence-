# Lane `relations` — lightweight relationship graph + bounded DFS (secondary) — Sonnet

Branch `p4ei/relations`, worktree `.claude/worktrees/p4ei-relations`. Read `_wave1-common.md` first.

Secondary per the product owner (render diffs are the centerpiece): keep it light and deterministic.
Implement DESIGN §7 "relations" and §2.5 exactly (owns `internal/investigate/relations/`, plus an
additive declarative `relationships:` catalog construct only if DESIGN requires it): `GraphBuilder`
(nodes from rendered objects + env resources; edges from references — Secret/ConfigMap refs, volumes,
Ingress backends/TLS/class, serviceAccountName, selectors, CR → API type), `ExposureMapper`, and the
`Traverser` (§2.5 DFS exactly: stable ordering, bound, truncation reason, unresolved edges recorded).
Its main job for the agent: answer "which consumers reference this changed object?" with evidence.

Done: DFS determinism + bound + truncation tests; a graph that `Validate`s for each groundtruth `env/`;
docs `docs/INVESTIGATE.md` section "Relationships".
