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
   alternatives that contradict the consumer role become **outside-role arms**: predicted "the consumer never
   verifies"; a consumer that verifies at either version refutes the rule; otherwise the arm is recorded and never
   counts toward a pass on its own.

## Done

- (nothing yet)

## In progress (exact next step)

- Implementing 1–4.

## Files touched outside ownership

- (none yet)

## Contract changes (additive)

- (none yet)

## Open questions

- (none)

## Test status

- not run yet
