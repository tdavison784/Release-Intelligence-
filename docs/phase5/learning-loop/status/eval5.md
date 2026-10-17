# Lane eval5 — status

## Done (wave 1)
- `InvRunMeta.KnowledgeDigest` (+ `knowledgeLive`); `ri eval -investigations` requires an explicit `-knowledge` that is not
  the live tree (`./knowledge`, the work tree's, the primary checkout's) unless `-allow-live-knowledge`; `-append` refuses a
  different digest. `cmd/ri/eval_investigations_knowledge.go` + tests.
- `scripts/phase5/snapshot.sh <sha> <out>` (git archive; prints a digest identical to the Go one — cross-checked on K0).
- `scripts/phase5/kbad.sh` + `scripts/phase5/kbad/main.go`: S3 mutations a–h from K1, each documented. Tried on a K0 stand-in
  fact (all eight build and load as designed); not yet run on K1 (does not exist). No unit test for the tool.
- Agent-verdict scoring `internal/eval/investigations_verdicts.go` (+ tests): ungated, separate table, never read by
  policy metrics. Input `InvRun.Verdicts`; cmd side reads it through the STUB `agentVerdictInputs` in
  `cmd/ri/eval_investigations_knowledge.go` (returns nil).
- `ri eval -investigations -compare run[,label=run…]` → Markdown tables (`internal/eval/investigations_compare.go`).
  `RunScore` gained `reviewFindings`, `agentTokens`, `hypotheses` (omitempty).
- **A0 baseline stored**: `eval/investigations/results/A0-K0-20261017-bfebde39` (binary bfebde39, clean; K0 = 03b6ff43, digest
  sha256:672a7ce0…74ff5). Identical 24 item decisions to Phase 4's last A run. EVAL-REPORT.md §A0 lists what blocks the
  `rotationPolicy` items (no pin that can fire; render silence; unresolved relationships/traversal incomplete; 12+1+1 / 8+1
  uncovered paths; identical record shape for -us and -eu).
- docs: `docs/INVESTIGATE.md` (Phase 5 subsection + corrected command examples), README command line.

## Next (waves 2–3; commander-triggered)
1. After code freeze F1 and the `verdict` merge: replace the `agentVerdictInputs` stub with a loop over
   `inv.AgentVerdicts` (item, class, confidence, mechanism).
2. Re-run A0 on the frozen binary (PROTOCOL §6 step 2); then B0 (`-agent`, keep transcripts), export hypotheses, verify,
   K1, C1, Kbad-* (`kbad.sh <K1> <fact> <out>`), held-out after unseal. Append each to EVAL-REPORT.md.

## Decisions
- Verdict outcome `overstated` (agent affected, expected potentially/insufficient) beside the four named, so it is not
  counted as a false alarm.
- kbad mutants that change a fact are minted as well-formed record chains (loader accepts them; the pipeline must refuse to
  clear). (a) is proxy-level: a consensus-only mutant would need forged model proposals (fake provenance).
- Mutation (b) is the knowledge state after a refused promotion (no fact, item pending); the refusal is lane verify's gate.
- The first A0 run (binary 19baddab) was deleted before commit and redone on bfebde39: the code gained report fields after it,
  and it was never committed or cited.
  It showed the same as the kept run: identical 24 item decisions and gate states, same digest `sha256:672a7ce0…74ff5`;
  the only difference was the missing review/token/hypothesis fields in its score. Deleted by me (`rm -rf` of its own
  uncommitted results dir) on 2026-10-17; nothing was edited to move a number.

## Files touched outside ownership
- `docs/INVESTIGATE.md` (appended subsection, edited 3 example lines) and `README.md` (one command line): docs only.

## Contract changes
- `InvRunMeta.KnowledgeDigest`/`KnowledgeLive`, `InvScore.Verdicts`, `RunScore` fields above: all omitempty; stored Phase 4
  runs load, rescore and compare unchanged (verified with `-compare` against the Phase 4 A run).

## Open questions
- None blocking. A0/B0 F1 runs superseded (W-0027); -work guard added (refuses non-empty -work unless -append). Note for commander: every existing invocation of `ri eval -investigations` now needs `-knowledge <snapshot>`.

## Test status
- `go build ./... && go vet ./... && go test ./...` green (before the final docs commit; docs only since).

- F2 (8286e36f): A0-K0-20261017-6d2ae5bf and B0-K0-20261017-6d2ae5bf stored, 8 cases each; B0 1,056,794 tokens, 0 hypotheses, verdicts 11 correct / 12 hedged / 1 no-verdict. F1 runs superseded. Idle; bin/ri = 6d2ae5bf.
