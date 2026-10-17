# Lane verify — status

Branch `p5ll/verify`. Brief: `briefs/verify.md`. Contract: PROTOCOL §7–§8. Docs: `docs/INVESTIGATE.md`
"Verifying mitigation hypotheses (Phase 5)".

## Done

- **Export→verify link agreed with lane verdict** (cross-session messages, 2026-10-17; verdict commit 86e9ceb3):
  - exported `SemanticProposal.Provenance.Rule = "hypothesis:<hyp-id>@<inv-id>"`, Task full, Producer
    `investigate.agent@v1`; the fact-review item lists it and proposes exactly `MitigationSpec.ProposedRule.Assertion`;
  - run record JSON `hypotheses[].mitigation.proposedRule.{assertion, release.{from,to}}` (FROM exclusive, TO
    inclusive), `hypotheses[].mitigationRejected` when not stored;
  - proposal citations = the hypothesis' upstream citations, appended to the candidate's evidence.
  - verify does **not** import verdict's types: it parses the rule with `knowledge.ParseHypothesisRule` and reads the
    run through a local JSON view (`verify.StateRuns`), so both branches compile alone. After both merge, the
    commander may switch to `domain.ParseHypothesisRule` (same format) — optional.
- `internal/domain/proof.go`: `ProofRecord` (+ `ProofMethod`, `ProofArm`, `ProofCheck`, `ProofBounds`),
  `ProofOutcomeOf`, `Validate` (outcome must follow from the methods; level observed ⇔ exercise ran; passing
  exercise needs both arms answered at FROM and TO). Record kind `proof` (`pf-` ids), stored under
  `<product>/<release>/proofs/`; `FileStore.checkProof` requires the item to propose exactly the proof's rule digest.
- `ReviewDecision.Supersedes` / `Coexists` (additive): the reviewer's choice per active same-subject fact.
- **Promotion gate** `internal/knowledge/promote.go`, enforced in `validateDecisions` (every `Decide` path: UI,
  bulk, CLI): human only, whole-fact item, all four aspects, confirmed proof of the decided rule's digest, no
  refuted proof of it, bounds contain the item's release, every active same-subject fact named in
  supersedes/coexists. `FactFromDecision` supersedes the named facts. Routing (`RouteWith`) ignores hypothesis
  proposals (no consensus / auto-approval / items from them).
- `internal/investigate/verify`: citation check (bundle CAS + fetch cache resolvers, passage, version bounds,
  subject named), fixtures (`products/exercises/fixtures/<product>.yaml`, kind `ProductFixtures`), two-arm
  synthesis from the rule, rule exercise (`EvaluateArms`: the four combinations + every never-pass path),
  orchestrator `Hypotheses` (skips decisive proofs unless `-again`; skips items whose rule ≠ the run's spec).
- `internal/investigate/runtime/fixture.go`: `Validator.RunFixture` (additive; same session, `ri-verify-*`
  clusters; environment and fixture runs refuse while a cluster of either prefix exists).
- `ri verify hypotheses -knowledge DIR [-store STATE] [-runtime] [-kubernetes V] [-again] [-json]` (cmd/ri/verify.go).
- Review UI: hypothesis section (rule in plain English, agent reasoning labelled AI, proofs with methods/levels/
  arms/evidence/limitations, gate state), supersede/coexist radios, default view hides agent rules until a proof
  is confirmed (then pin source). Fixed triage: a whole-fact item proposing another rule is no longer hidden as
  "verified" by the candidate's existing fact (every exported hypothesis item would have been hidden).
- Docs: INVESTIGATE.md section, README command line.

## In progress

- Nothing. Lane done. Real `-runtime` smoke run recorded in `runs/verify-smoke.md`: `rotationPolicy: Never`
  **confirmed** (207 s), an incorrect mitigation **refuted** (194 s); both `ri-verify-*` clusters deleted and verified;
  the gate refused the refuted rule and a proxy, and promoted the confirmed one for a human (scratch copy).

## Decisions (with why)

- Outcome: the condition-present arm changing ⇒ `fail` (refuted) even when the control arm did not change — the
  prediction for the mitigation is directly contradicted; both are refusals for the gate anyway.
- Without `-runtime` a proof is `inconclusive` (PROTOCOL §7.1 needs both methods); inconclusive proofs are
  re-verified on the next pass, decisive ones only with `-again`.
- A refuted proof of a rule blocks promotion of that rule forever (a later confirmation does not outweigh it).
- A citation must name ≥1 version and none outside the bounds; a minor-only token (`1.18`) names a release line.
  An unversioned document (e.g. a PR link) fails the check — fail closed.
- Fixtures live in their own directory (`products/exercises/fixtures/`) so the exercise files' digests (recorded by
  every Phase 4 runtime run) do not change.
- Default view: agent rules are hidden until confirmed ("awaiting a confirmed proof"): before that, no decision can
  change a classification.

## Files touched outside ownership (minimal, additive)

- `internal/domain/semantic.go` (record kind `proof`, `ReviewDecision.Supersedes/Coexists` + validation),
  `internal/domain/schemagen/meta.go` (enums), `schemas/knowledge-record.schema.json` (regenerated).
- `internal/knowledge/api.go` (`Snapshot.Proofs`, `ReviewContext.Promotion`; CONTRACT-CHANGE(verify) notes),
  `internal/knowledge/route.go` (hypothesis proposals ignored), `decide.go`, `queue.go`, `filestore.go`.
- `internal/investigate/runtime/session.go` (cluster prefix, `prepared`, `allConsumed` hooks; preflight/cleanup
  now consider `ri-verify-*` clusters too).
- `cmd/ri/main.go` (dispatch + usage line), `README.md`.

## Contract changes

- Additive only: `ProofRecord` and record kind `proof`; `ReviewDecision.supersedes/coexists`;
  `knowledge.Snapshot.Proofs`, `knowledge.ReviewContext.Promotion`. No producer/policy version bumped. Stored
  Phase 4 knowledge and run records load unchanged (`go test ./...` covers the realrecord/integrity tests).

## Open questions

- None blocking. For wave 2 (PROTOCOL §6 step 4): run `ri verify hypotheses -knowledge <K0 copy> -store <B0 store>
  -runtime -kubernetes 1.31` after export; the fixture cluster needs ~4 min per item.

## Tests

- `go build ./... && go vet ./... && go test ./...` green (2026-10-17, after the last commit).
- New: `internal/domain/proof_test.go`, `internal/knowledge/promote_test.go` (every gate refusal path, proxy,
  partial, refuted-then-confirmed, digest mismatch after correction, bounds, silent coexistence, routing ignores
  hypotheses), `internal/investigate/verify/*_test.go` (citations incl. outside bounds / unresolvable / tampered
  cache; arm synthesis; the four arm combinations; never-pass paths; end-to-end with the gate),
  `internal/investigate/runtime/fixture_test.go`, `internal/reviewui/hypothesis_test.go`.

## Cleanup

- Deleted: both `ri-verify-*` clusters (by the run itself, verified), `bin/`, the scratch smoke dir, the temporary
  smoke program. Remaining kind clusters `ri-groundtruth5-*` belong to lane groundtruth5 (not touched).
