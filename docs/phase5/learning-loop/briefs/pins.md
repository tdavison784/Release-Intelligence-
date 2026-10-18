# Lane `pins` — rule applicability conditions + a correct object view for the pin validator — Opus

Branch `p5ll/pins`, worktree `.claude/worktrees/p5ll-pins`. Read `_common.md`. You own
`internal/investigate/validate/{pins.go,envview.go,override.go}` and their tests (other files in `validate/` only
with minimal additive hunks, listed in status).

## Problem (from the Phase 5 gap analysis)

1. A trusted fact's applicability can only say "objects of kind K where field F is unset/set". There is no way to
   scope a rule to the objects it is about (e.g. "objects of kind K **whose field G equals V**" or "objects that
   some other object references through edge E"). `specsFromExposure` (`pins.go:186-232`) keeps only the
   `unset` leaf of an exposure and drops the rest of the scope, so the pin must hold on **every** object of the
   kind (`selects`/`evalObjects`, `pins.go:326-401`).
2. The env object view reads files as declared (`envview.go:31-33`): a kustomize base object and the overlay patch
   that sets the pinned field are seen as two objects, and the unpatched base counts as a miss. Renders that are
   `not-applicable` are dropped by `envPairs` (`common.go:344-349`), so base+patch are never merged.

## Build

1. **Applicability conditions.** Decide the full applicability expression of a trusted fact (all leaves the fact
   model already supports: field states/values, `all`/`any`, resource scope; and, if the fact model has them,
   relationship leaves) against each object, three-valued: in scope / out of scope / undecidable. An object is
   excluded from the pin requirement **only** when it is provably out of scope (CRD/schema defaults are used only
   when the effective object view is complete and the default is cited by the bundle's schema). Undecidable ⇒
   the pin is not established for that object, with the reason. If the fact model lacks a leaf a reasonable rule
   needs, add it additively in `internal/domain` (CONTRACT-CHANGE note) — keep it to what the existing
   `applicability` grammar can express naturally.
2. **Effective object view.** Use the environment's own rendered objects (the version-independent kustomize/helm
   build the render-diff lane already produces) where a render consumed the file; a raw file is used on its own
   only when no render consumed it. A file that no render consumed and that cannot be attributed keeps today's
   behaviour (fail-closed). Record which view each object came from in the validation detail.
3. **Trust and lifecycle** (already partly there — prove it with tests): only active facts at trusted level (weakest
   aspect); superseded/retracted never pin; a fact whose release bounds do not cover the FROM→TO transition never
   pins; two active facts that contradict on the same subject ⇒ no pin + gap.
4. **Negative-control fixture tests** (PROTOCOL §5 S3 a–h), synthetic: each must leave the item un-cleared and say
   why; plus the positive control: a scoped rule pinned by an overlay patch clears.

## Done

Per `_common.md`; status lists each new reason code and an example validation detail.
