# Lane `rerun` status

Branch `p3ll/rerun` (worktree `.claude/worktrees/rerun`). Writes only under `docs/rerun/` + this file.

## Done
- Live `ri check` for all 28 products (`docs/rerun/checks/<id>.json`), fresh per-product state dir + `-refresh`,
  same release sets as the baseline (argo-cd 3.4.0 -> 3.4.1, see REPORT).
- `docs/rerun/compare.py` (baseline vs live outcome multiset diff).
- Targeted retries (`docs/rerun/checks-retry/`), direct probes (`docs/rerun/raw/probes.txt`).
- `ri stats` over the live checks (`docs/rerun/stats.{json,md}`).

## In progress / next
- Discovery re-run (`docs/rerun/discovery/`), `ri drift` against baseline and live checks (`docs/rerun/drift/`).
- Research-claim probes (`docs/rerun/probe-claims.sh` -> `claims.tsv`), OUTCOMES.md re-measurement, `REPORT.md`.

## Incident: ENOSPC (disk full) mid-run
The first parallel discovery + drift batch filled the disk (state dirs / clones). Anything produced by that batch
(discovery + drift outputs, kube-prometheus-stack discovery stderr) was DISCARDED and is being re-run serially with
shallow clones and per-product state cleanup. The `ri check` reports (committed before the incident) were
verified as complete JSON. Nothing that failed due to disk is counted as unavailable.

## Decisions
- Separate state dir per product so results are live fetches, never the warm cache.
- argo-cd: baseline release 3.4.0 is not a GitHub release (git tag only), `ri check -versions` aborts; 3.4.1 substituted.

## Files outside ownership
None.
