# Lane `verdict` — status

Branch `p5ll/verdict`. Brief: `briefs/verdict.md`.

## Done
- **Contract (PROTOCOL §8), committed first so eval5/verify can code against it** — `internal/domain/verdict.go`:
  - `AgentVerdict` (`avd-…`): item, class (the four FindingClasses), confidence, mechanism, environmentReasoning,
    citations, verifiedFacts (clm-/vr-), assumptions, mitigations (hyp-), uncertainty, proposedValidations,
    wouldOverturn, droppedCitations/droppedVerifiedFacts/dropped, dossier, statementKind, provenance (ai).
    Stored as `Investigation.AgentVerdicts` (one per item; run-record validation resolves items, hypotheses,
    claims, citations, dossier artifact).
  - `Hypothesis.Mitigation *MitigationSpec` + `Hypothesis.MitigationRejected` (a spec is stored only when
    well-formed). `MitigationSpec{item, proposedRule{assertion (complete SemanticAssertion), release{from,to}},
    environmentBasis, requestedVerification}`. `ReleaseBounds`: from exclusive, to inclusive (agreed with lane
    verify). `ProposedRule.Digest()`. Undecidable applicability leaves are malformed.
  - `domain.HypothesisRule(hyp, inv)` / `ParseHypothesisRule`: the provenance `rule` of an exported proposal
    (`hypothesis:<hyp>@<inv>`) — agreed with lane verify as its link to the run.
  - `AgentRun.{Sessions, WallMillis, DroppedCitations, DroppedVerifiedFacts}`, `AgendaCoverage.{Dossier, Verdict}`,
    artifact kind `item-dossier`. All additive/omitempty; schemas regenerated.

## In progress (next step)
- Step 1: item dossier (`internal/investigate/agent/dossier.go`), tool `get_dossier`, `ri investigate dossier`.

## Decisions
- Verdict class reuses `FindingClass` so eval5 scores verdicts with the same matcher as policy findings.
- Release bounds `{from, to}` (from exclusive, to inclusive), per lane verify's proposal.

## Files touched outside ownership
- (none yet) — planned: `internal/investigate/run/pipeline.go` (store `out.Verdicts` in `inv.AgentVerdicts`, one
  additive hunk), `cmd/ri` new subcommands (brief-sanctioned).

## Contract changes
- See Done (all additive, CONTRACT-CHANGE(verdict) comments).

## Open questions
- none

## Tests
- `go test ./internal/domain/...` green.
