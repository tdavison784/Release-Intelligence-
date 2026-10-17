# Lane `pins` — status

Branch `p5ll/pins` (from `p5-learning-loop` @ 73d72aef). Brief: `briefs/pins.md`. **State: done.**

## Done

1. **Applicability conditions** (`validate/pins.go`). An exposure leaf (`field` in state `unset` / `not-equals`)
   keeps its `resource` scope; leaves are found directly in the scope or under `all`/`any` (never under `not`/`ref`).
   Each object of the kind is decided three-valued against the whole scope (`scopeEval`), the pin leaf taking the
   value the setting gives it: false+pinned → hit; false+not pinned → **provably out of scope** (excluded, listed
   with evidence); true → miss; unknown → undecidable (never excluded). Leaf semantics mirror `internal/impact`
   (field states, Kleene all/any/not, text-line, ref → exactly one object). Fail-closed differences: a value
   predicate (equals / not-equals / matches / has-token) on a field the object leaves unset is decided **only** by a
   schema default that CRDs of the bundle (upstream `crd` artifacts + env files + renders) state and agree on;
   no CRD, a CRD that states **no** default, disagreeing CRDs, an ancestor default or an undescribed path ⇒
   undecidable; an unresolved/ambiguous ref is undecidable; `undecidable` leaves are unknown. `unset`/`set` leaves
   are decided on what the object states. All objects out of scope ⇒ no pin.
   Also: a fact that states an applicability is decided by it — its default change alone no longer yields a pin
   (found by the `any` test: the Phase 4 default-changed derivation ignored the fact's own exposure, fail-open
   against a trusted rule). The exposure leaf on the subject's own setting inherits the old-default acceptance.
2. **Effective object view** (`validate/envview.go`). Objects = to side of complete product renders (from side for
   the before value) + renders of **version-independent overlays** (`not-applicable` pairs with a from side) +
   environment files **no render consumed**. Consumption is read from the bundle's own kustomization files
   (`kustomizeConsumes`: resources/bases/components/crds/patchesStrategicMerge/patches/patchesJson6902, recursive).
   A file consumed by a render whose objects are unavailable ⇒ its objects are undecidable. Every hit/miss/
   exclusion names its view; a pass lists the files seen through renders. Unparsable files consumed by an
   available render no longer block the kind. Verified on a real Phase 4 bundle (shop-de): 14 base/patch files
   attributed to their two overlay renders, nothing unattributed.
3. **Trust and lifecycle** (`validate/override.go`). Trusted = weakest aspect deterministic/human (existing, now
   tested incl. a single consensus aspect). Superseded/retracted never pin (tested). **Release bounds** (new): a fact
   pins only when `from < release ≤ to` and same product; no release / unparsable ⇒ never. **Contradictions** (new):
   an active fact inside the transition about the same subject key (any level, attached or not; all bundle
   knowledge-fact artifacts + `WithFacts`) whose change or applicability differs ⇒ no pin for the item
   (`pin:contradicting-facts`). Identical restatements, other releases and inactive facts are not contradictions (tested).
4. **Negative controls** (`validate/pins_test.go`, synthetic product `gizmo`/kind `Widget`): S3 a (consensus,
   proxy, weakest aspect), b (unverified), c (release after / equal to from / none / other product), d (wrong
   pinned value), e (superseded), f (retracted), g (undecidable scope), h (contradicting active fact) — each
   un-cleared with its reason code asserted. Positive control: a root-scoped rule pinned by an overlay patch clears;
   the same environment with an unscoped rule, without the overlay render (raw files), or with the overlay render
   unavailable does not. Plus: affected counterpart names the object and scope; CRD-default cases (none / no default
   / default puts it in scope / CRDs disagree / CRD in env files); any/not/ref operators; all-out-of-scope ⇒ no pin;
   rendered product objects use their from side.

## Reason codes (provenance rule of a not-run override inspection)

| Code | When |
|---|---|
| `pin:no-rule` (existing) | no rule names a setting — includes untrusted, inactive and out-of-transition facts (each noted), and a default change whose fact's exposure names no pin leaf |
| `pin:not-established` (existing) | a rule's setting does not hold everywhere (absent, wrong value, in scope through another condition, all out of scope) |
| `pin:applicability-undecidable` (**new**) | a rule failed only on objects whose scope membership (or effective state) cannot be decided |
| `pin:contradicting-facts` (**new**) | two active in-transition facts about a pin source's subject state different change/applicability |

Example pass detail (positive control fixture):

```
pin rule pin:knowledge-exposure@v2, from fact vf-scoped (verification human, release 2.0.0): Widgets of the scope that leave spec.rotation unset now rotate always.
Pinning setting: field spec.rotation on every gizmo.example.io Widget in the rule's scope (Widget where spec.role equals "root" and spec.rotation unset), set explicitly to the old default "Never" (or to a value stated identically before and after the upgrade) — it holds everywhere it matters:
- gizmo.example.io/v1/Widget/apps/w-root spec.rotation (environment render kustomize:apps/w/overlays/prod, version-independent overlay): set to "Never", the old default
Out of the rule's scope (no setting required):
- gizmo.example.io/v1/Widget/apps/w-a (environment render kustomize:apps/w/overlays/prod, version-independent overlay): spec.role is leaf (spec.role equals "root" does not hold)
- gizmo.example.io/v1/Widget/apps/w-b (…): spec.role is leaf (spec.role equals "root" does not hold)
Environment files seen through the renders that consumed them (effective object view):
- env/apps/w/base/widgets.yaml → kustomize:apps/w/overlays/prod
- env/apps/w/overlays/prod/rotation.yaml → kustomize:apps/w/overlays/prod
It lands in the environment render in gizmo.example.io/v1/Widget/apps/w-root.
```

Example not-run (affected counterpart): `pin:not-established` — `… in the rule's scope (Widget where spec.role equals
"root" and spec.rotation unset) … not pinned: gizmo.example.io/v1/Widget/apps/w-root (environment render
kustomize:apps/w/overlays/prod, version-independent overlay): leaves spec.rotation unset`, with the excluded leaf
Widgets listed as `out of scope: …`.

## Decisions

- **Versions bumped**: `validate.override-inspection@v1` → `@v2` (view, scoping, bounds, contradictions);
  `pin:knowledge-exposure@v1` → `@v2` (scoped semantics). `pin:knowledge-default-changed@v1` and `pin:join-pinned@v1`
  unchanged. Nothing outside `validate` keys on these strings (cmd/ri uses the constant). New run configs list v2.
- Stored Phase 4 runs: no `internal/domain` change; all 152 stored investigations in `eval/investigations/results/*`
  re-validate (`ValidateAgainst` their bundles) with this code.
- No new domain leaf. The existing grammar already has `resource`/`field`/`ref`/`not`/`any`; a reverse relationship
  ("objects some other object references") is **not** expressible — see open questions.
- Contradiction pool includes untrusted active facts (fail-closed; a sloppy consensus fact on the same subject blocks a
  human pin until superseded/retracted). Overlap differences count as different applicability.
- Exclusion uses the post-upgrade (to) object only.
- **Commander review fix (2026-10-17): unset field + CRD states no default ⇒ undecidable** for value predicates
  (previously equals → false and not-equals-with-some-elements-present → true, which could exclude an object).
  Why: the absence of a schema default does not prove what the controller does with an absent field — controllers
  default in code (the default-changed pattern this validator exists for). What an unset field means must come from
  the trusted fact itself, via an explicit leaf: `any{F equals X, F unset}` (unset is in scope) or
  `all{F not-equals X, F set}` / `all{F equals X, F set}` (unset is out of scope) — tested, decided without any
  schema. A cited schema default still decides (the API server stores it). Tests:
  `TestPinScopeUnsetFieldNeedsTheSchema` (equals and not-equals with a no-default CRD upstream or in the env ⇒
  `pin:applicability-undecidable`; explicit unset/set leaves ⇒ decided both ways; schema default in/out of scope;
  CRDs disagree). Mutation check (manual, recorded): restoring the old `defaultNone` decisions makes the three
  no-default cases fail with `outcome pass, want not-run` — i.e. the old code cleared them.

## Files touched outside ownership

- `internal/investigate/validate/common.go`: one-line version bump of `NameOverrideInspection`.
- `docs/INVESTIGATE.md`: OverrideInspection table/section, new subsection "Pin rule applicability and the effective
  object view (Phase 5)".

## Contract changes

None (no `internal/domain` types changed; no schema regen needed).

## Open questions (for the commander)

1. **Reverse references.** A rule like "CA objects that some issuer object references" cannot be written: `ref` only
   follows a reference *from* the scoped object. If the verify/verdict lanes' proposed rules need it, a leaf such as
   `referenced-by {kind, path, of}` would be an additive `internal/domain` op (+ impact evaluator support). Not added.
2. A rule whose scope excludes every object yields no pin by design (exposure is the join's decision). If the experiment
   expects a clearance there, it must come from the join, not from override inspection.

## Integration

- 2026-10-17 merged `p5-learning-loop` @ ef6b8d04 (coverage, eval5). Conflicts only in the validator-name line of
  `validate/common.go` and its row in `docs/INVESTIGATE.md`: kept both bumps (`validate.render-diff@v2` from
  coverage, `validate.override-inspection@v2` from this lane).

## Test status

`go build ./... && go vet ./... && go test ./...` green on `p5ll/pins`.

## Cleanup

No kind clusters, no servers. `bin/` removed. Scratch files only in the session scratchpad. One smoke
`ri investigate` (shop-de, K0) wrote new content-addressed artifacts and run record `inv-47fc4f2228ca` into the shared
`.ri` store (additive; nothing deleted).
