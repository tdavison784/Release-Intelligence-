# Lane applicability: status (M4 E1–E3; ledger D-0022)

Branch `p6di/applicability` (from `p6-decision-intel` @ 971636bc). Brief: `briefs/applicability.md`; plan:
`PLAN-M4.md` "Lane `applicability`" (changes 1–4).

## Design (decided 2026-10-27)

1. **Shared three-valued helper**: new package `internal/applicability` (plain Go, imports only `domain`). It holds
   what pins.go had privately (`schemaDefault`, `defaultAt`) and the one rule both evaluators now use for a value
   predicate (`equals`, `not-equals`, `matches`, `has-token`, `has-token-key`) on a field an object leaves unset:
   - a CRD schema default (every CRD of the input that describes the kind/version agrees) decides it;
   - else an **evidenced** `Condition.Default` (below) decides it;
   - else **unknown** (`semantic-ambiguity`), with the reason and what would settle it ("a schema default, or a
     rule default citing upstream evidence").
   A schema default and a rule default that disagree are unknown (contradiction), never either one.
2. **`domain.Condition.Default *FieldDefault`** (additive, `omitempty`, so every existing assertion digest is
   unchanged): `{value (JSON), source: crd-schema | controller, citations: [evidence ids]}`. Allowed on value
   leaves (`field`, `cli-flag`, `env-var`). `Condition.Validate` checks shape (value JSON, known source, at least one
   citation). `VerifiedFact.Validate` additionally rejects a default whose citations do not resolve to the fact's
   evidence (kind `structured`/`document` for crd-schema; upstream kinds for controller). Evaluators use a default
   only when its citations resolve in the evidence pool they are given (the fact's evidence); otherwise it is
   ignored and the leaf stays unknown.
3. **Impact join** (`impact/condition.go` `valueLeaf`): unset + value comparison is no longer false; it goes through
   the shared helper. The join gets CRD schemas from the environment's CRDs (`env.CRDVersion.Schema`, additive,
   `json:"-"`), and the fact's evidence as the citation pool (`ConditionOptions.Evidence`).
   Kleene connectives are monotone in the information order, so turning a false leaf into unknown can only move
   results toward unknown; only an evidenced default adds information.
4. **Pins** (`validate/pins.go` scope eval): same helper, with the fact's evidence as pool.
5. **Exact coverage (R2.2)**: an override-inspection pin pass lists the item's rendered changes it covers: those on
   an object of the pinned kind at exactly the pinned path, and `crd-description-changed` changes whose documented
   field path (from the schema path) is exactly the pinned path in the CRD serving the pinned kind
   (`PinnedSetting.CRD`, additive). The policy re-checks each claim (`EvidenceBundle.PinCoversChange`) and counts
   only confirmed ones; everything else stays a gap.
6. **Boundary fixtures**: the rule exercise adds an **omitted-field arm** per field a rule compares by value; the
   arm's prediction is the rule evaluated with the shared helper. An undecidable prediction makes the exercise
   **fail** before any cluster runs (a rule that only works with every field explicit is refuted, E2). Shielded
   alternatives that contradict the consumer role become **outside-role arms** (same consumers): a consumer that
   verifies at FROM and fails at TO refutes the rule; otherwise the arm is recorded and never
   counts toward a pass on its own.

## Done

1. **Shared helper** `internal/applicability` (SchemaDefault, DefaultAt moved from pins.go, RuleDefault, Resolve,
   Unset, ParentPath/ParentAbsent). Commit 810d2b24.
2. **`domain.Condition.Default`** (`FieldDefault{value, source, citations}`; `DefaultSources` enum; schemas
   regenerated). `Condition.Validate` checks shape and placement (value leaves of field/cli-flag/env-var only);
   `VerifiedFact.Validate` rejects a default whose citations do not resolve to the fact's upstream evidence
   (`FieldDefault.Unevidenced`).
3. **Impact join** (`impact/condition.go` rule 6, `ConditionOptions.Evidence`, `knowledge.go` passes `f.Evidence`):
   unset + value predicate → schema default (environment CRDs, parent present) or evidenced rule default, else
   unknown `semantic-ambiguity`; list elements that omit the field hold the default too.
4. **Pin scope** (`validate/pins.go`) on the same helper, with the fact's evidence as pool. Commit 3d8bc9d3.
5. **Exact coverage**: `PinnedSetting.CRD`, `EvidenceBundle.PinCoversChange`, `domain.SchemaFieldPath`;
   `override.go` `pinCoverage` records the item's rendered changes on the pin's own field; `classify/policy.go`
   re-checks every rendered change a pin pass claims. Commit 5c785576.
6. **Boundary fixtures** (`verify/boundary.go`): every arm predicted by the rule (three-valued, evidenced rule
   defaults only); omitted-field arms; undecidable or self-contradicting ⇒ exercise `fail` before any cluster runs;
   role-contradicting shielded alternatives exercised as outside-role arms (`consumer.go`). `ExerciseInput.Evidence`
   carries the rule's citations. Commit b75e7669.
7. **No fail-open evidence**:
   - `TestUnsetRuleNeverFailsOpen`: 3672 evaluations without an established default vs the previous semantics
     (test-only switch `legacyUnsetFalse`): 2052 moved to unknown, none moved toward a decision.
   - Stored runs re-classified (`cmd/ri/m4_rescore_test.go`, tag `m4rescore`, reads only cases.tar.gz + run.json):
     19 runs (3 B0, 13 C1, 3 D1), 50 cases, 1004 items: **1004 identical** (class and rule); no rule id changed.
   - Join re-run on the 8 cases those runs use (base 971636bc vs this branch, `ri impact -render -knowledge K0`,
     state cloned into scratch): 878 findings, **878 identical** (16 knowledge findings evaluated). K0 has 35
     active facts with value-predicate leaves; none changes on these environments.
   - Not re-run: the override-inspection validator on stored bundles (their CAS artifacts are not in the run
     archives). Its changes are stricter (parent-absent schema defaults) or need an evidenced default, which no K0
     fact states.
8. **Docs**: `docs/IMPACT.md` (unset rule, condition defaults), `docs/INVESTIGATE.md` (pin scope, exact coverage,
   boundary fixtures, outside-role arms).

## In progress (exact next step)

- Nothing. Lane done; merged `p6-decision-intel` (semverify) at 20f7d02f (conflict in docs/INVESTIGATE.md: both
  sections kept). Next is the commander's: merge, then the M4 demo.

## Decisions

- A schema default counts as established (brief: "as pins already do"); the no-fail-open property is stated for
  inputs without an established default.
- Schema defaults apply only inside a present parent (stricter than pins before).
- A flag/env/config-key pin covers no rendered change (a container-args change is never provably only the flag).
  One existing test (`TestR2_4BehaviourItems`) used a flag pin to cover an args change; it now uses a field pin on
  that exact field.
- Boundary failure is recorded on the `rule-exercise` method (fixtures are this lane's; proof methods are
  semverify's), with the detail "decided before any cluster ran". It runs only under `-runtime`, as the exercise
  does; without it the proof stays inconclusive, never confirmed.
- The Phase 5 human-authored rule shape (`isCA equals true` without a default) now fails its boundary fixtures; the
  same rule with an evidenced controller default for the compared field is exercised and can be confirmed.

## Files touched outside ownership

- `internal/env/env.go`: `CRDVersion.Schema` (additive, `json:"-"`), filled from the CRD document.
- `internal/domain/schemagen/meta.go`: enum registration; `schemas/*.json` regenerated.
- `internal/investigate/classify/rules_test.go`: one test adjusted (see Decisions).
- `cmd/ri/m4_rescore_test.go`: build-tagged check, not in the normal suite.

## Contract changes (additive)

- `domain.Condition.Default`, `domain.FieldDefault`, `domain.DefaultSource(s)`, `FieldDefault.Validate/Unevidenced`,
  `Condition.Defaults`.
- `domain.PinnedSetting.CRD`, `EvidenceBundle.PinCoversChange`, `domain.SchemaFieldPath`.
- `impact.ConditionOptions.Evidence`, `impact.SchemasOf`; `verify.ExerciseInput.Evidence`.

## Open questions

- None blocking. For the demo: the formalizer (or a human) must state the compared field's default with an upstream
  citation (e.g. the controller's default for the role field) for a scope rule to decide objects that omit it.

## Test status

- `go build ./... && go vet ./... && go test ./...` green after the merge with `p6-decision-intel`; the m4rescore
  check still 1004/1004 identical after the merge.
