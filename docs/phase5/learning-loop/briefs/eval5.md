# Lane `eval5` — controlled arms, knowledge snapshots, agent-verdict scoring, the Phase 5 report — Sonnet

Branch `p5ll/eval5`, worktree `.claude/worktrees/p5ll-eval5`. Read `_common.md`, PROTOCOL.md (all of it — you run
it), `eval/investigations/FORMAT.md`, `internal/eval/investigations_*.go`, `cmd/ri/eval_investigations.go`. You
own those files, `scripts/phase5/`, `eval/investigations/results/` (append only) and
`docs/phase5/learning-loop/{EVAL-REPORT.md,runs/}`. You are an evaluation lane: you may read `expected.yaml` of the
**Phase 4** cases for scoring code, never the held-out branch before the commander unseals it.

## Build now (wave 1)

1. `knowledgeDigest` in run meta; `ri eval -investigations` refuses to run without an explicit `-knowledge` that is
   not the live `knowledge/` tree unless `-allow-live-knowledge` (prevents silent contamination).
2. `scripts/phase5/snapshot.sh <sha> <out>` (K0/K1 via `git archive`), `scripts/phase5/kbad.sh` (the PROTOCOL §5
   S3 mutations from K1, each documented).
3. **Agent-verdict scoring** (ungated, separate table): per expected item, the agent's provisional class vs
   expected → correct / false-safe (agent not-affected, expected affected) / false-alarm / hedged
   (potentially/insufficient); matched with the same matcher as policy findings. Code against the PROTOCOL §8
   `AgentVerdict` shape; stub the field locally until lane `verdict` merges (commander tells you to merge).
4. Report tables: policy-class before/after per item (A0 vs C1), safety rows, clearances, cost, human-review items
   needed (K0 vs K1), hypotheses by outcome; accuracy and coverage/risk-detection in separate tables.
5. **Baseline now**: A0 on the 8 Phase 4 cases with K0 = `03b6ff43` (the product owner's verifications included).
   Store it, and write in EVAL-REPORT §A0 per item what still blocks the cert-manager `rotationPolicy` items.

Later (commander-triggered, after the code freeze): B0, C1, Kbad-*, held-out. Never edit gates or expectations.

## Done (wave 1)

Per `_common.md`; A0 stored with its knowledge digest; `LANE DONE:` but keep your worktree — you run waves 2–3.
