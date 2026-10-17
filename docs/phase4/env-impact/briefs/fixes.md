# Lane `fixes` — generic pipeline defects found by eval arm A — Opus

Branch `p4ei/fixes`, worktree `.claude/worktrees/p4ei-fixes`. Read `_wave2-common.md` (+ `_wave1-common.md`),
FLEET.md, then `docs/phase4/env-impact/status/eval.md` ("Routed to the commander") and EVAL-REPORT.md's
root-cause section. You own, for these fixes only: `internal/env`, `internal/investigate/{bundle,validate,
relations,run}`, `internal/impact` (minimal), `internal/render` (minimal).

Integrity (binding): never open `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`. Every fix is a
**generic** correctness fix with its own synthetic test — never a change that targets a case. Do not edit
gates, eval results or the scorer. Arm A's stored run stays as it is; the eval lane re-scores after you merge.
A fix that makes something clear (not-affected) must be justified on its own semantics, not by a score.

## Fixes

1. **Layered Helm values files** (`internal/env/env.go:428` warning): an environment that layers several
   values files for one release (Argo `helm.valueFiles` list, Flux `valuesFrom`, helmfile) is normal. Merge
   them in the order Helm applies them (later overrides earlier; maps deep-merge, lists replace) with per-key
   provenance (which file/line set the effective value), instead of warning. The input stays `partial` only
   when a referenced values file is genuinely missing or unparseable (with reason).
2. **Run-record reproducibility**: the stored ImpactReport artifact embeds `retrievedAt` timestamps, so
   identical runs give different record digests. Keep timestamps out of identified content (provenance /
   audit detail instead), as was done for bundle CreatedAt. Test: two identical runs ⇒ identical record digests.
3. **CRD description-only changes**: rendered changes that only alter CRD schema `description` text are
   documentation, not behaviour. Classify them as such at render/correlation time (a declared, generic render
   class) and let a deterministic validator pass them as harmless; they must not flood an item with noise.
   A description change never *clears* the item's behaviour change itself (manifestation rule unchanged).
4. **Traversal truncation at depth 8** (cert-manager): find out why — legitimately long chains, fan-out, or
   a cycle/self-reference artefact. Fix artefacts; if the bound is simply too low for real GitOps graphs,
   raise the default with a measured justification (record graph sizes). Truncation must stay honest
   (reason recorded, blocks not-affected).

Done: each fix with tests, build/vet/test green, status file listing every behaviour change and its
justification, cleanup, `LANE DONE:`. Do not run the eval yourself.
