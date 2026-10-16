# Lane `validate` — deterministic validators (P4) — Opus

Branch `p4ei/validate`, worktree `.claude/worktrees/p4ei-validate`. Read `_wave1-common.md` first.

Implement DESIGN §7 "validate" and §2.6 exactly (owns `internal/investigate/validate/`):
`JoinVerdicts`, `RenderDiff`, `OverrideInspection`, `SchemaCompat`, `StaticReference`, each an
`investigate.Validator` returning `pass` / `fail` / `error` / `not-run` (with reason) at level
`deterministic`; **cannot decide ⇒ never `pass`**.

Why this lane is Opus: `OverrideInspection` is the **only** deterministic way to clear a breaking item
whose manifestation is controller-behaviour (a default compiled into the new binary — invisible in render
diffs; see DESIGN §3 R2 and the manifestation rule). It must establish, from environment evidence, an
explicit setting that pins the safe behaviour (values key, CR field, annotation, controller flag/config),
and show where it lands in the environment render. Where does the knowledge "setting S pins behaviour B"
come from? It must be **declarative and release-level** (the change record / upstream evidence, a
verified knowledge fact in `knowledge/`, or a product-definition construct) — never product Go code and
never derived from an eval case. Design that source explicitly (propose any additive contract/catalog
change as CONTRACT-CHANGE) and keep it auditable: the validation record cites the rule, the env evidence
and the render location. A setting that is merely *absent* is never a pass for a changed default.

Tests: per validator incl. every not-run path and "cannot decide ⇒ never pass"; table tests over
synthetic bundles; generic fixtures, not case answers. Docs: `docs/INVESTIGATE.md` section "Validators".
