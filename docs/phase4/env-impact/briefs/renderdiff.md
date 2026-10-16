# Lane `renderdiff` — environment render diffs as indexed evidence (CRITICAL PATH) — Opus

Branch `p4ei/renderdiff`, worktree `.claude/worktrees/p4ei-renderdiff`. Read `_wave1-common.md` first.

This is the product owner's centerpiece: the before/after rendered **diffs** of the environment's own
configuration are the primary evidence the investigation agent works from. Implement DESIGN §7
"renderdiff" and §2.4 exactly (owns `internal/investigate/renderdiff/`): `renderdiff.New(app, st)`
satisfying `investigate.EnvironmentRenderer` over `app.RenderDiffEdge` / `EnvironmentPairs`,
`render.Correlate` and chart coverage — one `ManifestPair` per render target (every Argo CD / Flux /
helmfile / Kustomize target found in the GitOps repo), full before/after manifests and the diff stored as
artifacts, one `RenderedChange` per `render.Change` with its correlation to change items, sensitive values
withheld, incomplete pairs carry reasons and **no** changes (never "no change"). Plus the diff query
engine the agent tools call (`Query`, `Object`, `Summaries`): filter by kind / namespace / name / path /
class / change item / correlated-or-not, deterministic ordering, stable ids, bounded result sizes with
continuation.

Quality bar: a query result is something an agent can cite; every `RenderedChange` resolves back to the
artifact bytes (digest + locator). Measure and record (status file) render coverage across the
groundtruth environments: targets found, rendered ok / partial / unavailable with reasons, rendered
changes, uncorrelated rendered changes. Tests skip with a message when `helm`/`kubectl` are missing.
Docs: `docs/INVESTIGATE.md` section "Render diffs" (skeleton exists; edit your section only).
