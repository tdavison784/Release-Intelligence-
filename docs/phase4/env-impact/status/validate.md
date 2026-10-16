# Status — lane `validate` (Claude Opus 5.5, agent ri-validate)

Branch `p4ei/validate`, worktree `.claude/worktrees/p4ei-validate`. Brief: `briefs/validate.md`.
State: **DONE** (2026-10-16). Resume from: this file + `git log` + DESIGN.md §2.6 / §3.1 / §7 "validate" +
`docs/INVESTIGATE.md` "Validators". Merged `p4-env-impact` (bundle, store, relations, renderdiff, groundtruth,
§1.3 amendment adf4fe08) into this branch.

## Done

Package `internal/investigate/validate/` — five `investigate.Validator`s, all records level `deterministic`,
outcome relative to the safety property, "cannot decide ⇒ never pass":

| Constructor | Validator | Decides |
|---|---|---|
| `JoinVerdicts(report, opts…)` | `validate.join@v1` | §2.6 wrap of decisive join verdicts (≤1 record per item, item-wide) |
| `RenderDiff(opts…)` | `validate.render-diff@v1` | fail = breaking item + strongly correlated env change; pass = complete silent render (or CRD description text only), rendered-config only |
| `OverrideInspection(opts…)` | `validate.override-inspection@v1` | explicit setting pinning the old behaviour → `PinnedSetting` (the only deterministic clear of a behaviour item) |
| `SchemaCompat(opts…)` | `validate.schema-compat@v1` | cluster version vs target range; visible objects vs the computed CRD change |
| `StaticReference(opts…)` | `validate.static-reference@v1` | references along impact paths survive the upgrade (dangling ⇒ fail) |

Files: `common.go` (options, record construction, coverage, artifact reads), `join.go`, `renderdiff.go`,
`override.go` + `pins.go` (pin rules) + `envview.go` (environment files and renders, field lookup),
`schema.go`, `reference.go`. Docs: `docs/INVESTIGATE.md` "Validators".

**Pin-rule sources** ("setting S pins behaviour B" — declarative, release-level, cited; never product Go code,
never a model, never an eval case):

1. `pin:join-pinned@v1` — the join's pinned verdict on a computed default change (`impact:values-pinned`,
   `impact:crd-default-pinned`) when no other non-clear finding of the change exists; values: every changed key
   pinned; fields: objects only the environment render shows must set it too (needs complete renders).
2. `pin:knowledge-default-changed@v1` — a TRUSTED (deterministic/human), active fact attached to the item,
   `change.type: default-changed` of a crd-field / cli-flag / env-var / feature-gate / config-key subject; value =
   old default, or stated identically before and after the upgrade.
3. `pin:knowledge-exposure@v1` — a trusted fact whose exposure is a conjunction with a settable leaf in state
   `unset` / `not-equals V` (top level or directly in a resource scope).

The setting must hold in every complete env render target and on every visible object of the kind.

**Tests** (`go test ./internal/investigate/validate`, 70+ cases): synthetic bundles of a fictional product
(`gizmo`, never a case), in-memory store; every output of every test runs through
`Investigation.ValidateAgainst`. Per validator incl. every not-run path; table "absent/partial/changed setting
never passes" (14 shapes); "nothing to decide on ⇒ no decisive record" over every family × manifestation ×
validator × store; determinism (byte-identical output); rendered-unexplained items; multi-rule correlations.
Opt-in real-bundle test `RI_VALIDATE_STATE=<warm .ri> go test ./internal/investigate/validate -run Groundtruth -v`:
bundle + renderdiff + relations + impact report + knowledge over all 8 groundtruth `input.yaml` (never
`expected.yaml`); every record conforms. Outcome counts over the 8 cases (generic diagnostics, not scored):
render-diff 68 pass / 4 fail / 84 not-run; join 53 pass / 1 fail; override-inspection 0 pass (see note 3);
static-reference 3 pass / 153 not-run (mostly "no impact paths"); schema-compat 4 not-run.

## In progress / next step

- Nothing. Commander: review + merge `p4ei/validate`.

## Decisions (with why)

- **≤1 join record per item, item-wide (no subjects).** The record id has no finding field; merging per item
  avoids collisions, and a mixed join verdict (one key not-affected, another review) is not decisive for the item.
- **Fail = "breaking" only** (§1.3); `scope:category` and `scope:rendered-unexplained` are never breaking here
  (amendment: affected only with a concrete failure named). Render-diff therefore never fails them.
- **Strong correlation** for a render-diff fail: `subject`, `values-default`, `artifact`, `release-anchor` (any of
  the comma-joined rules); `mentions-*` decide nothing. CRD `…description` paths are harmless text.
- **Override: universal pins only.** No per-object partial passes: an unpinned object the traversal missed could
  be cleared by a partial pass (fail-open), so a partly pinned item is not-run with pinned/unpinned listed.
  Universal pin record: `Pin.Subject` = first pinned object (sorted), `Pin.Evidence` = all of them, detail lists all.
- **Override needs complete env renders** for every pin (also consumer CR fields): only then is every object of
  the kind (incl. chart-rendered ones) visible. Values keys are decided through the join (it knows which values
  files belong to the product); a values subject in a fact is not a pin rule.
- **Value acceptance**: old default, or the same explicit value before and after; an explicit new value set only
  after the upgrade changes the behaviour (not a pin). Environment files state one value for both sides.
- **Not-applicable env pairs** (targets that do not deploy the product) are ignored for completeness.
- **Platform anchors** `<platform>` or `<platform>/<kind>` (bundle lane) both resolve; outside a *tested* range
  is not-run (not a demonstrated incompatibility).
- **Static-reference** never passes a root-only path (vacuous) nor a rendered-unexplained item.
- `JoinVerdicts.Kind()` returns `override-inspection` (DESIGN §0 lists the join wrap under override inspection);
  each record's kind follows the evidence that decided it.
- Validator errors only for invalid input (`ErrNotFound`/`ErrRejected`) and cancellation; every other problem is
  a record with the reason.

## Files touched outside ownership

- `internal/impact/crd_gvk.go`: additive exported `CRDIdentity(change)` (wraps the join's unexported
  `identityFromSchemaChange` / `parseCRDName*` / `kindFromCRDTitle`) so schema-compat scopes CRDs exactly like
  the join instead of duplicating the parser.

## Contract changes

- None.

## Open questions / notes for the commander

1. **Bundle → knowledge facts.** OverrideInspection reads trusted facts from the bundle's `knowledge-fact`
   artifacts (`ChangeItems` set), which makes pin rules reproducible from the bundle. The bundle lane does not
   store them yet; until it does, the wiring must pass `validate.WithFacts(app.LoadKnowledge(...).Facts...)`
   (as the groundtruth test does). Suggest a small bundle follow-up.
2. **Contract: `ValidateAgainst` and not-applicable pairs.** It requires *every* env render pair `ok` for
   auto-approve-eligible; renderdiff emits `not-applicable` pairs for overlays that don't deploy the product, so
   any such repo can never be eligible. Probably should exempt `not-applicable` (commander-owned).
3. **Behaviour items stay uncleared on the real bundles**: the facts attached to them are consensus-verified, so
   (correctly) never pin rules. A behaviour item becomes clearable only once a fact is human/deterministically
   verified or an observed run exists — expected, worth knowing for the eval (notAffectedConfirmation on
   behaviour items will be low by design).
4. Classify: a static-reference pass covers paths for *reference* breakage only; a join pass covers no
   subjects/paths (useful for R2.4's no-roots case only).

## Tests

`go build ./... && go vet ./... && go test ./...` green on this branch after merging p4-env-impact (39 packages ok, 0 fail). Real-bundle test green offline (8 cases, 24 s). No kind clusters, servers, bin/ or scratch output created.
