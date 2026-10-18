# Phase 5 — common preamble (every lane reads this first)

- `docs/phase5/learning-loop/FLEET.md` is binding. Read `MISSION.md` (the scope note at the top binds you: this
  is a controlled experiment on the existing Phase 4 architecture, **not** a new subsystem — the smallest change
  that is correct and fail-closed wins), then `PROTOCOL.md` (pre-registered; §8 is your contract), then
  `docs/phase4/env-impact/SUMMARY.md` and the parts of `docs/phase4/env-impact/DESIGN.md` and `docs/INVESTIGATE.md`
  your brief names.
- Generic code only: no product names, case names or case-shaped conditionals in Go, prompts or tests. Test
  fixtures are **synthetic** (invent a product/CRD if that keeps you honest). Never open
  `eval/investigations/*/{expected.yaml,NOTES.md,observed/}`, and never fetch or read branch `p5ll/groundtruth5`.
- Fail closed: anything your code cannot decide ⇒ no clearance, with a recorded reason. Every behaviour change
  you make gets a test that would have caught the opposite (fail-open) implementation.
- Historical reproducibility: stored Phase 4 run records (`eval/investigations/results/`) must still load and
  re-verify their digests; new fields are `omitempty`; if you bump a producer/policy version, say so in status.
- Done = `go build ./... && go vet ./... && go test ./...` green on your branch, docs (your section of
  `docs/INVESTIGATE.md`, add a heading if new), `status/<lane>.md` current, cleanup per FLEET.md, final message
  `LANE DONE:` with ≤15 lines.
