# Phase 6 fleet: rules every lane follows

`docs/phase5/learning-loop/FLEET.md` applies unchanged (branches and worktrees, commits, ownership, conventions,
reporting, cleanup), with these substitutions:
- Integration branch `p6-natural-agent` (base: `p5-learning-loop` @ e2ef81ee, PR #7 open), worktree
  `.claude/worktrees/p6-integration`. Lanes work on `p6na/<lane>` in `.claude/worktrees/p6na-<lane>`.
- The fleet directory is `docs/phase6/natural-agent/` (status files, briefs, ledger). The commander updates
  `LEDGER.yaml`; lanes only read it.
- Phase 0 changes **no RI code** (MISSION §2, PROTOCOL §10). Lanes write scripts under `scripts/phase6/` and
  records under `docs/phase6/natural-agent/`.

**Evaluation integrity for Phase 6** (in addition to Phase 5's rules):
- Never open `docs/phase6/natural-agent/RUBRIC.md`, any case's `expected.yaml`, `NOTES.md` or `observed/`, or the
  Phase 5 `EVAL-REPORT.md`/`SUMMARY.md` (which discuss these cases' labels).
- Investigator sessions see only their workspace (PROTOCOL §3, §5). Nothing a lane writes into a workspace may
  come from RI's derived outputs or from any label-bearing file.
