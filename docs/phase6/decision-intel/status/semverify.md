# Lane `semverify` status (Milestone 4 E4–E5, ledger D-0023)

Branch `p6di/semverify`, worktree `.claude/worktrees/p6di-semverify`. Brief `briefs/semverify.md`, plan `PLAN-M4.md`
(lane `semverify` table, changes 1–3).

## Done

1. **Semantic check** (`internal/investigate/verify/semantic`, pure Go, no I/O): extracts the value tokens a claim
   asserts (defaults, allowed/enumerated values, character classes and single characters, before/after values,
   booleans) and checks each against the cited passages with polarity (accepts/rejects, true/false,
   enabled/disabled, valid/invalid; negators flip). Missing token or reversed polarity ⇒ `unsupported`; no comparable
   token, or a polarity the passage does not let us read ⇒ `undetermined` (a human decides, never a model); else
   `supported`.
2. **Proof method `semantic-check`** in `ri verify hypotheses` (`verify/semantics.go`): runs over the passages the
   citation check established. Fail refutes the proof; inconclusive adds a limitation for the human reviewer; a pass
   never replaces citation-check or rule-exercise (optional method).
3. **Report-only check of agent behaviour** sourced `upstream`: `semantic.CheckBehaviours`;
   `internal/synthesize` precompute adds the disagreement `behaviour-unsupported-by-citation` (not hard) per
   unsupported entry (shown in the judgement section / report UI; never a classification input);
   `ri investigate semcheck [-store DIR] [-json] <key>…` prints supported / unsupported / undetermined for stored runs
   (store or evaluation runs, keys `<run-id>/<case>`).
4. **Runtime exercise `admission-dry-run`** (`internal/investigate/runtime/admission.go`,
   `runtime.admission-dry-run@v1`): controller at the target version installed in a throwaway cluster
   (`ri-admission-*`, same preflight/budget/apply/readiness/cleanup as the isolated upgrade), covering validating
   webhooks found (rules + selectors) and waited for, one object server-side dry-run. Outcome `observed`:
   `accepted` (only with an effective covering webhook), `rejected` (by webhook `<name>` or by the API server's own
   validation, with the message) or `undetermined`. Catalog action `admission-dry-run` (no reaction), schema enum,
   ingress-nginx `ingress-admission` declared. CLI `ri investigate admission -product -version -object [-setup]
   [-values] …` (opt-in). The isolated upgrade never runs admission exercises.
5. **Docs**: `docs/INVESTIGATE.md` (verification: semantic check, outcome rule, report-only check; synthesis
   precompute list; runtime: admission dry run), README command list, `ri verify` usage.

## For the commander (demo step 7)

- M2 misread: `ri investigate semcheck -store eval/investigations/results <run-id>/<case>` on the stored M2 run.
  I did **not** run it on any development case (blinding).
- D-EU Ingress: `ri investigate admission -product ingress-nginx -version <1.12.x> -object <the D-EU Ingress>.yaml
  [-setup <its IngressClass etc.>] [-values <the environment's controller values>]`. The ingress-nginx webhook only
  validates Ingresses of its own class, so pass the environment's controller values (or `-setup` the class) so the
  object is in the webhook's scope; otherwise an `accepted` may only mean the webhook ignored it. The record lists
  the covering webhooks; acceptance requires one that is effective.

## Decisions

- **Optional method.** `ProofOutcomeOf` ignores an optional method that neither passed nor failed. Why: the brief says
  semantic-check counts only when it passes and is routed to a human when undecidable; promotion needs a confirmed
  proof *and* a human accept, so blocking confirmation on "undetermined" would make the human routing impossible. A
  failure still refutes. Historical proofs (no semantic-check) derive the same outcome.
- **Structured tokens = change before/after + statement tokens.** Applicability-condition values are not compared:
  they scope the environment (consumer roles, shielding alternatives), are not what an upstream passage states, and
  are tested by the rule exercise arms and the applicability lane's checks. Found by the existing consumer-observation
  test, whose valid rule would otherwise have been refuted.
- **Narrow extraction.** Quoted spans are values only in value positions (after is/to/of/accepts/…, or continuing such
  a list); lowerCamel, dotted, flag-like names and versions are names, not values. So "unsupported" means a value is
  really absent or reversed, not that a setting name was paraphrased.
- **Exhaustive sets.** A regex character class, or a list in a clause with "only", is an exhaustive allowed set: a
  claimed *rejection* of a character outside it is supported; a claimed *acceptance* of it is unsupported.
- **Only unsupported becomes a disagreement** in the synthesis input; undetermined entries (most free prose) would
  flood it. All three verdicts are printed by `ri investigate semcheck`.
- **Admission "accepted" must be meaningful**: at least one covering webhook that is effective (ready endpoint, or
  failure policy Fail). `matchConditions` (CEL) are not evaluated, so such webhooks never count (fail closed);
  `sideEffects` other than None/NoneOnDryRun ⇒ undetermined. Webhook-call failures are retried, never read as a
  rejection. A run that did not complete (incl. a cluster that could not be deleted) is undetermined.
- **Admission is its own command**, not part of `ri investigate --runtime` or `ri verify -runtime`: it needs an
  environment object chosen by the engineer, and stays opt-in and bounded.
- **Kind test is opt-in** (`RI_ADMISSION_KIND=1`) besides skipping without kind/docker, like the groundtruth runtime
  test: it creates two clusters, which `go test ./...` in every lane must not do.

## Domain changes (additive; schemas regenerated)

- `domain.ProofSemanticCheck` (`semantic-check`), `domain.AllProofMethodKinds` (schemagen enum source), the
  optional-method rule in `ProofOutcomeOf`, `ProofRecord.Validate` accepts the method (deterministic level only).
- `domain.JFBehaviourUnsupported` (`behaviour-unsupported-by-citation`, not hard) in `JudgementFindingKinds`.

## Files touched outside ownership (additive hunks)

- `internal/domain/judgement.go` (finding kind), `internal/domain/schemagen/meta.go` (one enum source).
- `internal/synthesize/precompute.go` (+ `precompute_test.go`): the disagreement (brief: additive hunk).
- `internal/catalog/exercises.go` (+ test), `schemas/product-exercises.schema.json`: the `admission-dry-run` action.
- `cmd/ri/verify.go` (usage text), `README.md`.

## Test status

- `go build ./... && go vet ./... && go test ./...` green.
- Kind fixture test (`RI_ADMISSION_KIND=1 go test ./internal/investigate/runtime -run AdmissionKind -v`), run
  2026-10-27 on kind v0.24.0 / node v1.31.0: **accepted** (`/docs/a`, 34 s) and **rejected** (`/docs/*`, message
  `admission webhook "validate.fixture.ri.dev" denied the request: data.path contains *: only [A-Za-z0-9._/-] is
  allowed`, 33 s); both clusters `ri-semverify-*` deleted and verified gone, fixture image removed.
- No live model runs; nothing run on the development cases.

## Open questions

- None blocking. For the gate: whether the commander wants `admission-dry-run` also declared for cert-manager (its
  webhook validates Certificates); not added, as no M4 question needs it and its references would need fresh
  verbatim upstream quotes.
