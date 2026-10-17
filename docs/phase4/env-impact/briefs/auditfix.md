# Lane `auditfix` — run-record audit ordering with agent requests — Sonnet

Branch `p4ei/auditfix`, worktree `.claude/worktrees/p4ei-auditfix`. Read FLEET.md and `status/eval.md` (the arm B
abort). You own `internal/investigate/run` for this.

Defect (found by eval arm B, case payments-eu): `internal/investigate/run/pipeline.go`, `Pipeline.Run`, case
`hasRecord:` — `done := ev("completed", …)` (≈ line 138) is stamped with `p.now()` **before** `res.apply` runs; the
investigate stage's apply then appends one `requested` event per agent evidence/validation request with later stamps;
`done` is appended after them with the earlier time ⇒ `Investigation.Validate` rejects the record ("audit: event N
goes back in time"). Every agent run that makes ≥1 request fails; scripted runs without requests never hit it.

Fix: stamp the stage-completed event when it is appended (after apply), and check every other stage for the same
pattern. Test: a scripted agent run that makes evidence AND validation requests produces a record that validates,
with monotonic audit times, on both the in-process runner and the Temporal path (parity test). Keep run-record
digests reproducible (times are not identity). build/vet/test green, status, `LANE DONE:`.
