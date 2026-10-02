# Lane `rerun` — re-run Phase 1 / Phase 2 checks with real network access

**Why.** Phase 1 and much of Phase 2 were executed by Claude in a cloud sandbox where GitHub and other
upstream endpoints (quay.io, registries, project websites) were often unreachable. Results recorded there
are contaminated: many `unavailable` / `unverifiable` records, "UNVERIFIED live" notes and "unreachable"
findings reflect the sandbox, not upstream reality. This machine has full network access
(`GITHUB_TOKEN=$(gh auth token)`; warm cache under `/Users/tommydavison/repos/Release-Intelligence-/.ri`
— but use a SEPARATE state dir for live re-runs, see below).

Read first: `docs/phase3/learning-loop/FLEET.md` (binding), `docs/ONBOARDING.md`, `docs/onboarding/PROTOCOL.md`,
`docs/DRIFT.md`, `docs/FINDINGS.md`, `docs/phase2/{PLAN,OUTCOMES}.md`, `docs/research/*.md`, README
command list. Branch `p3ll/rerun`, worktree `.claude/worktrees/rerun`. You own only new files under
`docs/rerun/` (+ your status file). **Never overwrite or edit the original artifacts** (`docs/onboarding/checks/*.json`,
`records/`, `discovery/`, `docs/research/*.md`, `docs/FINDINGS.md`, products/*.yaml, eval/*) — those are the
historical record; the commander/product owner decides afterwards what to replace.

## Do

1. **Inventory the contaminated artifacts.** For each Phase 1/2 artifact, find what was recorded as
   unavailable / unverifiable / unreachable / "UNVERIFIED live" / blocked, with the file and line. Use git
   history (`git log --follow`) to date them and to find the exact commands that produced them (commit
   messages, PROTOCOL.md, status notes).
2. **Re-run the same things live**, with a fresh state dir (`-state /tmp/…` or under the scratchpad;
   not the shared warm cache, so the results reflect live fetches; `-refresh` where the CLI offers it):
   - `ri check <product>` for all 28 onboarded products → `docs/rerun/checks/<id>.json` (same format as the
     baseline) and `ri drift` where it uses those baselines (report-only).
   - `ri discover …` for the products whose discovery records cite unreachable sources →
     `docs/rerun/discovery/`.
   - `ri stats` (onboarding effort/validation measurements) → `docs/rerun/stats.*`.
   - The Phase 1 research claims marked "UNVERIFIED live" / "unreachable": verify each live (curl / gh /
     oci manifest fetch / helm repo index) and record the outcome with the URL and what it returned.
   - Any Phase 2 measurement in `docs/phase2/OUTCOMES.md` that depended on network reachability: re-run
     its command and record the new numbers.
3. **Compare, honestly.** `docs/rerun/REPORT.md`: per product and per artifact, baseline vs live —
   counts of verified / unavailable / unverifiable / drift; and classify every difference as
   (a) **sandbox artifact** (unreachable then, reachable and consistent now),
   (b) **genuinely unavailable upstream** (still unreachable / 404 / gone),
   (c) **upstream changed since** (reachable now but different — new releases, moved channels), or
   (d) **real defect revealed** (the definition or pipeline is wrong once data actually arrives).
   (d) matters most — list each with evidence; don't fix product definitions yourself, propose the
   change. Also list which FINDINGS.md / OUTCOMES.md conclusions no longer hold.
4. **Recommendations**: which baselines should be replaced by the live versions (and the drift-baseline
   consequence of doing so), which definitions need changes, which findings need correcting.

Rate limits: GitHub API with token is 5000/h — pace if needed; registries may throttle (the fetch client
does not retry 429 — note any throttling as such, not as unavailable). Commit as you go; status file
`docs/phase3/learning-loop/status/rerun.md`; end with `LANE DONE: rerun` + a ≤15-line summary.

Model: Sonnet.
