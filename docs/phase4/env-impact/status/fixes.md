# Status — lane `fixes` (GLM-5.3, agent glm-fixes; Claude lane lead paused for a usage-limit window)

Branch `p4ei/fixes`. Brief: `briefs/fixes.md` (+ `_wave2-common.md`, `_wave1-common.md`, FLEET.md).
Defect list: EVAL-REPORT.md §5 (arm A) as routed by the commander; read from `p4ei/eval` (not merged into
`p4-env-impact` at lane start; read-only via `git show`, no merge).

## Done
- Lane setup: brief + FLEET + wave preambles + EVAL-REPORT §5 + status/eval.md read; baseline
  `go build ./...` + `go vet` green.

## In progress
- Fix 1 (layered values files, `internal/env/env.go`): replace the "repo mode found N values files; the
  last one wins per key" warning with a real Helm-order merge (later files override earlier ones per key,
  mappings deep-merge, any other leaf replaces the subtree below its path), one effective key per path with
  evidence from the file/line that set it; images extracted from the merged result. Exact next step:
  implement `valuesMerge` in env.go, wire `loadValuesFiles` + inline Argo/Flux/parameter values through it,
  update repo/env tests, add synthetic layering tests.

## Not started
- Fix 2 (run-record reproducibility: ImpactReport embeds evidence `retrievedAt`; two identical runs must
  give identical record digests).
- Fix 3 (CRD description-only rendered changes: declared generic render class + harmless deterministic
  verdict, no noise flood, never clears behaviour).
- Fix 4 (cert-manager traversal truncation at depth 8: diagnose, fix artefacts or raise the default with
  measured justification; truncation stays honest).

## Decisions (with why)
- none yet beyond the brief.

## Files touched outside ownership
- none yet.

## Contract changes
- none.

## Open questions
- none.

## Test status
- baseline green; lane changes not yet tested.

## GLM handoff log
- (session start) Resumed from brief + commander routing (commit 3729ab10); no prior lane status file
  existed. Began with fix 1.
