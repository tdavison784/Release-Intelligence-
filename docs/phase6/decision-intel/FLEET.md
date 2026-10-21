# Phase 6 decision-intel fleet: rules every lane follows

`docs/phase5/learning-loop/FLEET.md` applies unchanged (branches and worktrees, commits, ownership, engineering
conventions, running things, reporting, cleanup), with these substitutions:

- **Integration branch** `p6-decision-intel` (base: `p6-natural-agent` @ 1e72d145; PRs #7 and #8 are open),
  worktree `.claude/worktrees/p6di-integration`. Lanes work on `p6di/<lane>` in `.claude/worktrees/p6di-<lane>`.
  Herdr agents are named `ri-<lane>`.
- **The fleet directory** is `docs/phase6/decision-intel/`: mission, milestone plans, briefs, status, ledger,
  commander log. The commander updates `LEDGER.yaml`; lanes only read it.
- **This phase changes RI code** (`internal/`, `cmd/`), milestone by milestone, per `MISSION.md` §10 and the
  milestone's `PLAN-M<n>.md`. Do not start work belonging to a later milestone (mission §13).

**Evaluation integrity** (in addition to Phase 5's rules):

- **Never open:**
  - `docs/phase6/decision-intel/GATE-M*.md` and `M*-RESULT.md` (the commander's label-bearing gates and results);
  - any case's `expected.yaml`, `NOTES.md` or `observed/`;
  - `docs/phase6/natural-agent/RUBRIC.md`, `SCORES.md`, `GATE.md`, or anything under `docs/phase6/natural-agent/runs/`;
  - `report.txt` or `score.json.gz` of any `eval/investigations/results/*` run (they print expected classes);
  - the Phase 5 `EVAL-REPORT.md` / `SUMMARY.md`.

  Transcripts and bundles inside a run's `cases.tar.gz` are allowed: they are inputs, not labels.
- **Development cases** (cert-manager payments-us/eu, ingress-nginx docs-eu) are scored by the commander only.
  Lanes may smoke-test on other cases, unscored. MISSION.md necessarily names these cases' expected outcomes. That
  is why the code must stay generic and the scoring stays with the commander.
- **Generic logic only.** No rule that names a product, a file such as `ledger.conf`, or a key such as `ca.crt`.
  Reviewers reject such rules.

**Host safety (lessons from fleetpilot):**

- Never answer a Claude Code trust or permission dialog for any directory. If one blocks you, stop and tell the
  commander.
- Set `DO_NOT_TRACK=1` before running any third-party CLI.
- Never write to `~/.claude*`, or to the shared `.ri` cache except by reading it (clone it into a scratch state
  directory).
