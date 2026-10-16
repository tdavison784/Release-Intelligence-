# Lane `values` — per-release layered Helm values, done right (redo of reverted fix 1) — Opus

Branch `p4ei/values`, worktree `.claude/worktrees/p4ei-values`. Read FLEET.md, `_wave2-common.md`, then the
revert commit `73bb58ba` (its message lists the defects) and the reverted change itself
(`git show a638c39a`) so you do not repeat it. You own `internal/env` and the values handling in
`internal/investigate/bundle` / `internal/impact` for this. Never open
`eval/investigations/*/{expected.yaml,NOTES.md,observed/}`; every behaviour change has its own synthetic test.

## Problem

An environment's values are currently one repo-wide inventory, with a warning when several values files exist
(fail-closed: the values dimension is `partial`, so no values item can clear). The reverted fix merged *every*
values source in the repo into one global set — fail-open: per-cluster or per-release values overwrote each
other and the dimension counted as complete while effective values could be wrong.

## Required design

1. **Values belong to a release** (a deployment target: Argo CD Application, Flux HelmRelease, helmfile
   release, or a values-files group the renderer already detects). For each release build its own ordered
   values stack exactly as the tool applies it:
   - Argo CD: `helm.valueFiles` in **declared order** (plain paths and `$ref/…` alike), then `helm.values` /
     `valuesObject`, then `helm.parameters` (Argo's documented precedence — cite it).
   - Flux: `valuesFrom` in order (unreadable cluster objects ⇒ that release's values `partial` with reason),
     then `spec.values`.
   - helmfile: release `values:` in order.
   - Explicit `--values` files given to `ri`: their documented precedence must hold (re-read `env.go`'s
     `Inputs.Repo` doc and keep it true, or change doc + tests deliberately and say why).
   Helm merge semantics: maps deep-merge, lists and scalars replace, **an empty map `{}` does NOT delete keys**
   (Helm keeps them), `null` deletes a key. Per-key provenance (file + line that set the effective value).
2. **Never merge across releases.** Two releases of the same product (two clusters, dev/prod) are two
   environments' worth of values: the investigation must evaluate the release(s) that actually deploy the
   product under investigation; if several releases deploy it with different effective values, each is
   evaluated (or the ambiguity is a gap with a reason) — never one collapsed value no release has.
3. A values file that no release references and name-matching cannot attribute to exactly one release keeps
   the old warning ⇒ `partial` for that product (fail-closed).
4. Collection status: the values dimension is `ok` for a release only when its whole stack was read.

## Done

Tests for each tool's ordering, `{}` vs `null`, deep-merge vs list replace, cross-release isolation (the
reverted change's counter-examples from the revert message must now be correct), unattributable files stay
`partial`; golden changes explained line by line in your status file; build/vet/test green; `LANE DONE:`.
