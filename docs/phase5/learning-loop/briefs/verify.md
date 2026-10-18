# Lane `verify` — independent verification of mitigation hypotheses and the promotion gate — Opus

Branch `p5ll/verify`, worktree `.claude/worktrees/p5ll-verify`. Read `_common.md`, PROTOCOL §7–§8, DESIGN §2.6
(validators) and the runtime section, `internal/investigate/runtime`, `products/exercises/*.yaml`,
`internal/knowledge` (fact model, `decide.go`, review items), `internal/reviewui`. You own a new package
`internal/investigate/verify`, additive changes in `internal/investigate/runtime` and `products/exercises/`,
`internal/knowledge` (promotion gate + proof storage), `internal/reviewui` (showing hypotheses + proofs), the
`ProofRecord` domain type, and a `cmd/ri` subcommand.

## Build

1. `ri verify hypotheses -knowledge <dir> [-runtime]`: for each open hypothesis-sourced review item, produce a
   **ProofRecord** (PROTOCOL §8) by:
   - **Citation check** (deterministic): each cited upstream text resolves in the evidence cache at a version in
     the rule's release bounds and contains the cited passage; a citation outside the bounds or unresolvable ⇒
     that method fails.
   - **Rule exercise** (observed, opt-in `-runtime`): generate a **synthetic minimal fixture** from the proposed
     rule (only the subject object(s), the condition set vs unset, the controller at FROM), upgrade to TO in a
     throwaway kind cluster, run the product's declared exercise for that subject (`products/exercises/`), and
     observe **both arms** (condition present ⇒ previous behaviour preserved; condition absent ⇒ new behaviour).
     Confirmed only if both arms behave as the hypothesis predicts; a quiet window, a missing exercise, or the
     control arm *not* changing ⇒ inconclusive (the change is not demonstrated, so neither is the mitigation).
     Never uses an evaluation environment's repo. Clusters `ri-verify-*`, always deleted.
   - outcome = confirmed only if every applicable method passed; any fail ⇒ refuted; otherwise inconclusive.
     Limitations listed (what the fixture did not cover: other consumers, other topologies).
2. **Promotion gate** in `internal/knowledge`: a human accept/correct of a hypothesis-sourced fact-review item
   requires a confirmed ProofRecord for the same proposed-rule digest (a correction that changes the rule needs a
   new proof); refuted/inconclusive/missing ⇒ the decision is refused with a reason. Proxy/agent reviewers can
   never promote (already true — re-test). Promotion of a narrower rule **supersedes** the broader fact it refines
   when the reviewer says so (existing `Supersedes`), never silently coexists.
3. **Review UI**: the hypothesis item shows the proposed rule in plain English, the agent's reasoning labelled AI,
   the proof record (each method, outcome, evidence links, limitations) labelled deterministic/observed, and
   the gate state. Default view keeps showing only items whose decision would change a classification.
4. Tests: every gate refusal path; both-arm logic (the four arm combinations); citation outside bounds; proof
   digest mismatch after a correction.

## Done

Per `_common.md`; one real `-runtime` run on a synthetic fixture recorded under
`docs/phase5/learning-loop/runs/` (any product with an exercise), clusters deleted.
