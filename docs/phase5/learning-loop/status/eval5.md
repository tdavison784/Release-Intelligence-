# Lane eval5 — status

## Done (wave 1, in progress)
- `knowledgeDigest` in run meta (`InvRunMeta.KnowledgeDigest`, plus `knowledgeLive`); `ri eval -investigations` now
  requires an explicit `-knowledge` that is not the live tree (`./knowledge`, the work tree's, the primary checkout's)
  unless `-allow-live-knowledge`; `-append` refuses a different knowledge digest. `cmd/ri/eval_investigations_knowledge.go` + tests.
- `scripts/phase5/snapshot.sh <sha> <out>` (git archive; prints a digest identical to the Go one).
- `scripts/phase5/kbad.sh` + `scripts/phase5/kbad/main.go`: S3 mutations a–h from K1 (documented in both).
- Agent-verdict scoring: `internal/eval/investigations_verdicts.go` (ungated, separate table; outcomes correct /
  false-safe / false-alarm / hedged / overstated / no-verdict). Input is `InvRun.Verdicts`; the cmd side reads it from
  the stored investigation through the STUB `agentVerdictInputs` (returns nil) until lane `verdict` merges.

## In progress / next
- Report tables (`-compare`), A0 baseline run with K0 = 03b6ff43, EVAL-REPORT §A0.

## Decisions
- Verdict outcome "overstated" (agent affected, expected potentially/insufficient) added beside the four named classes
  so it is not miscounted as a false alarm.
- kbad mutants that change a fact are minted as well-formed record chains (loader accepts them); (a) is proxy-level
  (consensus-only needs forged model proposals, which would be fake provenance).
- Mutation (b) models the *state after a refused promotion* (no fact, item pending); the refusal itself is lane verify's gate.

## Files touched outside ownership
- none (cmd/ri/eval_investigations.go, internal/eval/investigations_*.go are owned).

## Contract changes
- `InvRunMeta.KnowledgeDigest` / `KnowledgeLive` (omitempty). `InvScore.Verdicts` (omitempty). Stored Phase 4 runs unaffected.

## Test status
- go build / vet / test ./... green.
