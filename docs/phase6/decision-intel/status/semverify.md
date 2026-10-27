# Lane `semverify` status (Milestone 4 E4–E5, ledger D-0023)

Branch `p6di/semverify`, worktree `.claude/worktrees/p6di-semverify`. Brief `briefs/semverify.md`, plan `PLAN-M4.md`
(lane `semverify` table).

## Done

1. **Semantic check** (`internal/investigate/verify/semantic`, pure Go): value tokens a claim asserts (defaults,
   allowed/enumerated values, character classes and single characters, before/after values, booleans), each checked
   against the cited passages with polarity (accepts/rejects, true/false, enabled/disabled, valid/invalid, negators).
   Missing token or reversed polarity ⇒ `unsupported`; undecidable ⇒ `undetermined` (human); else `supported`.
2. **Proof method `semantic-check`** in `ri verify hypotheses` (`verify/semantics.go`): runs over the passages the
   citation check established; fail refutes the proof, inconclusive is a limitation for the human reviewer, a pass
   never replaces citation-check or rule-exercise.

## In progress / next

- Report-only check of agent `behaviour` entries sourced `upstream` → `internal/synthesize` precompute finding
  `behaviour-unsupported-by-citation`; CLI `ri investigate semcheck <inv>`.
- Runtime exercise `admission-dry-run` (catalog action, runtime runner, kind fixture test, CLI).
- Docs: `docs/INVESTIGATE.md` (verification, runtime validation).

## Decisions

- **Optional method.** `ProofOutcomeOf` ignores an optional method that neither passed nor failed. Why: the brief says
  semantic-check counts only when it passes and is routed to a human when undecidable; promotion needs a confirmed
  proof *and* a human accept, so blocking confirmation on "undetermined" would make the human routing impossible.
  A failure still refutes. Historical proofs (no semantic-check) derive the same outcome.
- **Structured tokens = change before/after + statement tokens.** Applicability-condition values are not compared:
  they scope the environment (consumer roles, shielding alternatives), are not what an upstream passage states, and
  are tested by the rule exercise arms and the applicability lane's checks. Found by the existing consumer-observation
  test, whose valid rule would otherwise have been refuted.
- **Narrow extraction.** Quoted spans count as values only in value positions (after is/to/of/accepts/…, or
  continuing such a list); lowerCamel, dotted, flag-like and versions are names, not values. So "unsupported" means a
  value is really absent or reversed, not that a setting name was paraphrased.
- **Exhaustive sets.** A regex character class, or a list in a clause with "only", is an exhaustive allowed set: a
  claimed *rejection* of a character outside it is supported; a claimed *acceptance* of it is unsupported.

## Domain changes (additive; schemas regenerated)

- `domain.ProofSemanticCheck` (`semantic-check`), `domain.AllProofMethodKinds` (schemagen enum), the optional-method
  rule in `ProofOutcomeOf`, `Validate` accepts the method (deterministic level only).
- `domain.JFBehaviourUnsupported` (`behaviour-unsupported-by-citation`, not hard) in `JudgementFindingKinds`.

## Files touched outside ownership

- `internal/domain/judgement.go` (finding kind, additive), `internal/domain/schemagen/meta.go` (one enum source).

## Test status

- `go test ./internal/investigate/verify/... ./internal/domain/...` green (full suite pending).
