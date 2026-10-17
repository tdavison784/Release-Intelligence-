# Lane `defaults` — removed/changed values keys whose old default was in effect (root cause of the unsafe eligibility) — Opus

Branch `p4ei/defaults`, worktree `.claude/worktrees/p4ei-defaults`. Read FLEET.md, `_wave2-common.md`,
`docs/IMPACT.md` (values join), `docs/RENDER.md` (correlation rules), DESIGN §2.4/§3, and the pipeline-output
diagnostic `git show p4ei/eval:docs/phase4/env-impact/runs/unsafe-eligibility-diagnostic.md` (it contains no
ground truth). Never open `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`. You own `internal/impact`
(values join) and `internal/render` (correlation) for this; minimal edits elsewhere.

## Defect (generic)

For a computed `values:removed` (and `values:default-changed`) change whose FROM-version default is **non-empty
and used by the chart**, an environment that does **not** set the key was **relying on that default** — it is
exactly the exposed environment. Today:
1. the deterministic join emits `impact:values-unset` → not-affected ("your values do not set X, so nothing about
   this change takes effect on you"), and
2. the render correlator does not link the environment rendered changes in which that default value disappears or
   changes (values path ≠ rendered path), so `validate.render-diff` finds "nothing attributable" and passes, and the
   rendered changes become unrelated `scope:rendered-unexplained` items.
Together they let such items reach `not-affected` + approval-eligible.

## Fix

1. Join: for removed / default-changed keys, "not set by the environment" with a non-empty FROM default ⇒ the
   environment **relied on the default**: never not-affected; classify as the existing review-level signal (or
   UNKNOWN with a precise `unknownReason`) per docs/ACTION_CLASSIFICATION.md — never ACTION REQUIRED from this
   rule alone, and the ACTION REQUIRED contract is untouched. An empty/null FROM default keeps today's verdict.
   Keys the environment explicitly sets keep today's logic.
2. Correlation: extend the `values-default` rule (or add `values-removed-default`) so a rendered change whose
   **before** value equals the removed/changed key's FROM default (and whose after differs or is absent) is
   correlated to that key's item — by value and object location, generic, with a minimum-specificity guard (no
   linking on trivially common values like `true`, `1`, `""`). Correlated changes then need a passing harmless
   validation under R2.2, which render silence cannot give.
3. Tests: synthetic chart where a key with a non-empty default is removed (env unset ⇒ not not-affected; env
   overriding the consumer path ⇒ as today), default-changed analogue, the specificity guard, and a regression test
   that a removed key with empty default stays not-affected.
4. **Phase 3 regression check (mandatory):** run `GITHUB_TOKEN=$(gh auth token) bin/ri eval -state
   /Users/tommydavison/repos/Release-Intelligence-/.ri` before and after (the Phase 3 eval, `eval/gates.yaml`, never
   edited) and report every metric delta in your status file. If any Phase 3 gate gets worse, stop and report.
   Do not touch `eval/` or goldens beyond regenerating ones your change legitimately moves (explain line by line).

Done: build/vet/test green, Phase 3 eval delta table, status, `LANE DONE:`.
