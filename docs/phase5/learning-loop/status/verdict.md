# Lane `verdict` — status

Branch `p5ll/verdict`. Brief: `briefs/verdict.md`. Docs: `docs/INVESTIGATE.md` "Item dossier, provisional verdicts
and mitigation hypotheses (prompt v6)".

## Done (all committed; `go build ./... && go vet ./... && go test ./...` green)

1. **Contract** (PROTOCOL §8) — `internal/domain/verdict.go`, additive hunks in `investigation.go`:
   - `AgentVerdict` (`avd-…`): item, class (the four `FindingClass`es, so eval5 scores verdicts with the policy's
     matcher), confidence, mechanism, environmentReasoning, citations, verifiedFacts (clm-/vr-), assumptions,
     mitigations (hyp-), uncertainty, proposedValidations, wouldOverturn, droppedCitations,
     droppedVerifiedFacts, dropped, dossier, statementKind, provenance (ai). Stored as
     `Investigation.AgentVerdicts`, at most one per item. `Validate` / `ValidateAgainst` resolve the item,
     hypotheses, claims, citations and the dossier artifact.
   - `Hypothesis.Mitigation *MitigationSpec` and `Hypothesis.MitigationRejected`.
     `MitigationSpec{item, proposedRule{assertion (complete SemanticAssertion), release{from,to}},
     environmentBasis, requestedVerification}`. `ReleaseBounds`: from exclusive, to inclusive (agreed with lane
     verify). `ProposedRule.Digest()`. `ParseReleaseVersion` accepts tags like `controller-v1.12.0`.
   - `domain.HypothesisRule` / `ParseHypothesisRule`: the provenance `rule` `hypothesis:<hyp>@<inv>` of an
     exported proposal. This is lane verify's link to the run, agreed by message; verify parses it locally.
   - `AgentRun.{Sessions, WallMillis, DroppedCitations, DroppedVerifiedFacts}` (cost for eval5).
     `AgendaCoverage.{Dossier, Verdict}`. Artifact kind `item-dossier`. Everything is omitempty; schemas are
     regenerated.
2. **Item dossier** — `internal/investigate/agent/dossier.go`: deterministic and bounded (48 KiB), stored as a
   run artifact, inlined into the v6 item task (so the prompt digest pins it). It is served by `get_dossier`,
   and `ri investigate dossier <inv> <item>` rebuilds it read-only and compares it with the stored one.
   - Settings come from attached facts (subject + exposure leaves), join matches, item subjects and upstream
     code spans.
   - Each setting has a state per object: explicit / inherited-default / unknown, plus where. An unset field
     seen only in a file is **unknown**, never default.
   - Gaps are contradictory / missing / partial.
3. **Provisional verdicts** — `verdict.go`: required on every v6 closing entry except not-examined.
   - A malformed shape rejects the call.
   - Citations, verified facts and mitigations resolve one by one; what does not resolve is dropped, counted
     and explained.
   - A non-insufficient verdict needs at least one resolving citation.
   - **Never a classifier input.** `run/pipeline.go` stores them in `inv.AgentVerdicts`, and `agentOutcome()`
     omits them. `TestVerdictsNeverReachThePolicy` shows findings, decision and eligibility are byte-identical
     with and without verdicts, including an adversarial `not-affected` verdict on an item a failing validation
     makes affected.
4. **Mitigation hypotheses** — `mitigation.go`, checked at record time:
   - the grammar is complete and decidable (no undecidable leaf);
   - the subject is in the product and in the bundle;
   - the bounds parse and lie within the run's FROM→TO;
   - the basis refs (clm / vr / resolvable object) resolve;
   - at least one citation is upstream.

   A malformed spec ⇒ free-text hypothesis + `mitigationRejected`, and the model is told. Nothing is wired into
   pins.
5. **Export** — `export.go` + `ri investigate export-hypotheses <inv> -knowledge <dir>`.
   - Each well-formed mitigation becomes a `SemanticProposal`: task full, AI provenance, rule
     `hypothesis:<hyp>@<inv>`, upstream-only citations appended to the change's candidate via
     `ExtendCandidateEvidence`.
   - It also becomes a pending whole-fact `fact-review` item whose `Proposed` is exactly the rule
     (`item.Proposed.Digest()` = the assertion digest verify keys proofs on).
   - Never a fact or a decision. Idempotent.
   - Skips, with reasons: no or malformed mitigation; no upstream change; candidate release outside the bounds;
     no upstream citation; a second mitigation from the same model call for the same candidate (ProposalID has no
     content part).
6. **Prompt v6** (default; v1–v5 still embedded and replayable). It is generic: MISSION §2's eight questions,
   start from the dossier, mitigation only with upstream citations, say what would overturn. A test asserts the
   system prompt and tools name no product or case word.
   - Static digest sha256(system.md ‖ tools.json ‖ meta.json) =
     `sha256:d9b926aeadb8d433ca4295d12157a35e15c2b308b9496af40d9566082f666bff`. item.md =
     `sha256:94b7d44f9b6e…`.
   - Per run, `AgentRun.PromptDigest` = the digest of the item digests (each item digest includes its dossier).
7. **Sample** (synthetic fixture): `docs/phase5/learning-loop/runs/verdict-fixture-sample/` has a dossier, two
   hypotheses (one exported, one rejected with "release bounds (1.5.0, 1.6.0] are not within this run's
   transition"), a verdict, the export result and the knowledge records written.

## In progress (exact next step)
- Live smoke run 1 (`ingress-nginx-1.11-1.12--shop-de`, inputs only, `claude-sonnet-5-5`, prompt v6) in a scratch
  state dir. Next: copy the transcript and outcome to `docs/phase5/learning-loop/runs/verdict-live-1/`, run
  export-hypotheses and dossier on it, check the replay, and write the README.

## Decisions
- Verdict class = `FindingClass` (same matcher as findings for eval5).
- The dossier counts as `get_change_item` for the depth rule (it holds the same content). The artifacts it quotes
  count as retrieved in that session.
- Code spans in the upstream text name settings (generic: release notes put flags and keys in code spans). Found
  on a real bundle where the dossier otherwise named no setting; the fixture covers it. Not case-specific.
- Mitigation bounds must lie **within** the run's transition (runFrom ≤ from < to ≤ runTo).
- A verdict citation that does not resolve is dropped (counted), not fatal; a verdict with no resolving citation
  (except insufficient-evidence) is rejected so the model fixes it.

## Findings for the commander (outside my ownership, not fixed)
- **Run hijack via stage memo** (`internal/investigate/run/keys.go`): `build-graph`'s key is
  `graph:`+hash(bundle, builder), and its memoised output carries the first run's `Refs.Investigation`. So a
  second `ri investigate` with another config (e.g. `-agent`) on the same bundle in the same store creates its
  record, which stays `running` after assemble, and then continues every later stage on the **first** run's
  investigation (prints its id, "agent disabled").
  - Workaround: a fresh store/workspace per arm and run.
  - eval5 was told (message 2026-10-17). It matters for A0/B0/C1 if the arms share a store.

## Files touched outside ownership
- `internal/investigate/run/pipeline.go`: +3 lines (store `out.Verdicts`; a comment that `agentOutcome` omits
  them).
- `README.md`: two command lines.
- `docs/phase5/learning-loop/runs/verdict-*`: lane-verdict run records (the brief asks for the live transcript
  there).

## Contract changes
- All additive (`CONTRACT-CHANGE(verdict)`): domain types above; `investigate.AgentOutcome.Verdicts`,
  `HypothesisInput.Mitigation` / `MitigationInput`, `ItemSummary.Verdict` / `VerdictInput`, `ToolGetDossier`.
- Prompt default bumped to `investigate-agent/v6`, so agent runs get a new config digest and investigation id.
  The producer `investigate.agent@v1` is unchanged; the dossier producer `investigate.dossier@v1` is new.

## Open questions
- none blocking. Possible follow-up: the report UI / `classify.WriteText` do not show verdicts yet (classify and
  reportui are not my ownership).

## Tests
- `internal/domain`: verdict/mitigation shape, bounds and tags, the rule parser.
- `internal/investigate/agent`:
  - dossier: reproducible; states; unknown without a complete render; contradictions; bounded;
  - v6: prompt, full session (dossier, claim, two mitigations, verdict with drops), replay byte-identical;
  - verdict and mitigation rejection paths; `get_dossier`;
  - policy independence (adversarial verdict);
  - export (idempotent, never a fact, environment-only citations refused).
