# Lane `fixes5` — two generic fixes before the code freeze F1 — Opus

Branch `p5ll/fixes5`, worktree `.claude/worktrees/p5ll-fixes5`. Read `_common.md`, then `status/verdict.md`
("Findings for the commander") and `runs/verdict-live-1/README.md` ("Observation: a schema-usability cost").
Ledger items W-0024 and W-0025.

1. **W-0024 — run hijack.** A second `ri investigate` on the same bundle and state dir with different settings
   (e.g. without then with `-agent`) silently continues the first run's investigation. Root cause per lane
   verdict: `internal/investigate/run/keys.go` — the build-graph stage memo key covers only the bundle and its
   cached output carries the first run's investigation id. Fix so every stage memo key covers exactly the inputs
   that stage depends on (and nothing run-identity-specific leaks across runs), on both runners (in-process and
   Temporal activities; parity test). Constraint: ids and digests of stored Phase 4 runs and the A0 run must not
   change — prove it by re-verifying `eval/investigations/results/*` (`-rescore`, no scoring changes). Tests:
   both orders (agent-off then on, on then off) produce two distinct investigations with the right config each.
2. **W-0025 — verdict citation friction.** In every live session the first `submit_findings` was rejected because
   `verdict.citations` was empty while the closing entry's citations were present. Make the verdict inherit the
   closing entry's **already verified** citations when it states none (never unverified ones; never invent), and
   state that in the tool schema text. Bump the prompt/tool version (v7); v6 transcripts must still replay
   byte-identically under v6. No other prompt change.

Done per `_common.md`; no live model runs needed (fixture + replay tests).
