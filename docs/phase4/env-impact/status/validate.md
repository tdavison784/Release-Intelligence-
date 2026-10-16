# Status — lane `validate` (Claude Opus 5.5, agent ri-validate)

Branch `p4ei/validate`, worktree `.claude/worktrees/p4ei-validate`. Brief: `briefs/validate.md`.
State: **IN PROGRESS**. Resume from: this file + `git log` + DESIGN.md §2.6 / §3.1 / §7 "validate".

## Plan

Package `internal/investigate/validate/` — five `investigate.Validator`s, every record at level
`deterministic`, outcome relative to the safety property, "cannot decide ⇒ never pass":

| Constructor | Validator | Decides |
|---|---|---|
| `JoinVerdicts(report, opts…)` | `validate.join@v1` | §2.6 wrap of decisive join verdicts (≤1 record per item) |
| `RenderDiff(opts…)` | `validate.render-diff@v1` | env-scope renders: fail = strongly correlated change of a breaking item; pass = complete render, nothing attributable, rendered-config item only |
| `OverrideInspection(opts…)` | `validate.override-inspection@v1` | explicit environment setting pinning the old behaviour (PinnedSetting), from declarative pin rules |
| `SchemaCompat(opts…)` | `validate.schema-compat@v1` | platform version vs target range; objects vs target CRD schema (served version, fields, enum, required) |
| `StaticReference(opts…)` | `validate.static-reference@v1` | references along the item's paths survive the upgrade (dangling ⇒ fail; unresolved ⇒ not-run) |

Pin-rule sources for OverrideInspection (the knowledge "setting S pins behaviour B" — declarative and
release-level, never product Go code, never an eval case):

1. `pin:join-pinned@v1` — the deterministic join's own pinned verdicts on the item's change
   (`impact:values-pinned`, `impact:crd-default-pinned`): the upstream change record is a computed
   default change, the join found the environment's explicit setting (environment evidence).
2. `pin:knowledge-default-changed@v1` — a TRUSTED (deterministic/human) active verified fact attached to
   the item whose assertion is `change.type: default-changed` of a settable subject (crd-field,
   cli-flag, env-var, feature-gate, config-key): an explicit setting pins the behaviour.
3. `pin:knowledge-exposure@v1` — a trusted fact whose exposure condition is a conjunction containing a
   settable leaf in state `unset` / `not-equals`: the explicit complement falsifies exposure.

## Done

- `internal/investigate/validate/`: all five validators implemented (common.go, join.go, renderdiff.go,
  override.go + pins.go + envview.go, schema.go, reference.go).
- Test harness (`harness_test.go`): fictional product `gizmo` (never a case), in-memory store, bundle /
  graph / traversal builders; every validator output is run through `Investigation.ValidateAgainst`.
- Tests green: JoinVerdicts, RenderDiff, OverrideInspection (incl. every not-run path).

## In progress / next step

- SchemaCompat + StaticReference tests; determinism test; docs/INVESTIGATE.md "Validators".

## Decisions (with why)

- (filled as I go)

## Files touched outside ownership

- none yet (`docs/INVESTIGATE.md` "Validators" section is this lane's).

## Contract changes

- none yet.

## Open questions

- none yet.

## Tests

- not run yet.
