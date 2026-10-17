# Lane verify — status

Branch `p5ll/verify`. Brief: `briefs/verify.md`. Contract: PROTOCOL §7–§8.

## Done
- Read FLEET, _common, brief, MISSION, PROTOCOL, DESIGN §2.6, runtime/knowledge/reviewui code.
- Agreed the export→verify link with lane verdict (cross-session message, 2026-10-17):
  - exported `SemanticProposal.Provenance.Rule = "hypothesis:<hyp-id>@<inv-id>"` (verdict adds
    `domain.HypothesisRule`/`ParseHypothesisRule`; verify parses the same format locally until both merge);
  - the fact-review item proposes exactly `MitigationSpec.ProposedRule.Assertion`; proof digest = `item.Proposed.Digest()`;
  - `ProposedRule.Release = ReleaseBounds{From (exclusive), To (inclusive)}` read from the run record;
  - proposal citations = the hypothesis' upstream citations, appended to the candidate's evidence.

## In progress
- Exact next step: domain `ProofRecord` + knowledge record kind `proof`.

## Plan
1. domain `ProofRecord` (internal/domain/proof.go), record kind `proof` (store dir `proofs/`).
2. `internal/investigate/verify`: citation check (deterministic), rule exercise (observed, two arms), outcome.
3. `internal/investigate/runtime`: additive `RunFixture` (synthetic fixture, cluster prefix `ri-verify-`).
4. `products/exercises/fixtures/<product>.yaml` (kind `ProductFixtures`): declarative subject templates.
5. knowledge promotion gate; reviewui hypothesis view; `ri verify hypotheses`.
6. tests, docs (INVESTIGATE.md "Verifying mitigation hypotheses"), one real -runtime run.

## Decisions
## Files touched outside ownership
## Contract changes
## Open questions
## Tests
