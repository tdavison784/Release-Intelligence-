# Lane `orchestrate2` — Temporal production integration (P5, MISSION §A7, AC7) — Sonnet

Branch `p4ei/orchestrate2`, worktree `.claude/worktrees/p4ei-orchestrate2`. Read `_wave2-common.md` first,
then `TEMPORAL.md` and `status/orchestrate.md` (the wave-0 spike, merged).

Implement DESIGN §5 and §7 "orchestrate" exactly (owns `internal/orchestrate/`, `internal/investigate/run/`,
`cmd/ri/investigate.go`): replace the spike's placeholders with the real ports and `domain.Stage` names; the
in-process runner (`ri investigate <product> <from> <to> --repo … [--agent] [--temporal]`) and `ri worker`;
§5 retry classes / timeouts / idempotency keys (content digests; large evidence travels as artifact refs,
never inline payloads); bounded execution; cancellation; the human-review signal → `ReviewApplier` with a
timeout that fails closed; a state query; the run record's append-only audit trail and §5.4 audit events.

Note: a pre-existing `temporal server start-dev` (not ours, pid 46289) and a `./bin/worker` hold :7233 —
never touch them; for a live check start your own dev server on another port
(`temporal server start-dev --port 7299 --ui-port 8299 --db-filename <scratch>`) and stop it when done.

Done: testsuite tests for retry, non-retryable failure, cancel, review approve/reject/more-evidence/timeout,
replay determinism; the in-process runner and the Temporal path produce the same run-record digest for the
same inputs (with the scripted/replay agent driver); one live end-to-end run on a groundtruth `input.yaml`
via your dev server with the human-review signal sent from the CLI, transcript in status; docs (README
command list, `docs/INVESTIGATE.md` section "Running an investigation").
