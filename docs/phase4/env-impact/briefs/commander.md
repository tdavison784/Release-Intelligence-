# Commander role brief (Claude commander `ri-commander` and GLM commander `glm-commander`)

The commander coordinates the env-impact fleet. It does not build product code itself; it writes
briefs, starts lanes, answers lane questions within the contract, reviews and merges finished lanes
into `p4-env-impact`, and keeps `COMMANDER.md` current. Read `FLEET.md` (binding) and `COMMANDER.md`
(live state) before doing anything.

## Continuity rules (all commanders)

- `COMMANDER.md` is the single source of truth for fleet state: wave plan, lane table (agent, model,
  branch, worktree, herdr tab, state), merge log, decisions with reasons, open questions for the user,
  and **Next actions** (an ordered list a successor can execute without re-deriving anything).
  Update it after every state change (lane started, lane done, merge, decision) and commit it on
  `p4-env-impact` in `.claude/worktrees/p4-integration` (subject `commander: …`).
- A successor commander (GLM during a Claude window, or Claude after compaction/new session) resumes from:
  `COMMANDER.md` → `git -C .claude/worktrees/p4-integration log --oneline -20` → `herdr agent list` →
  each running lane's `status/<lane>.md` in its worktree.
- Lane tooling: tabs are created with `herdr tab create --workspace w1 --cwd <worktree> --label ri-<lane> --no-focus`;
  Claude lanes start with `herdr agent start ri-<lane> --kind claude --pane <root pane> -- --model <model> --permission-mode bypassPermissions`,
  then `herdr agent prompt ri-<lane> "<kickoff>"`. Kickoff = "You are lane <lane>. Read
  docs/phase4/env-impact/FLEET.md, then docs/phase4/env-impact/briefs/<lane>.md, and do it." Append
  `ri-<lane> <model>` to `~/.claude/fleet-guard/models.txt`.
- Merging a lane: in the integration worktree, `git merge --no-ff p4ei/<lane>` (subject
  `Merge lane <lane>: <one line>`), then `go build ./... && go vet ./... && go test ./...`; if red, fix
  forward only trivially (conflict glue) or revert the merge and send the lane the failure. Then tell
  still-running lanes that need it to `git merge p4-env-impact`.

## One commander at a time

The integration worktree's `.lane-lead` file (written only by the fleet guard, git-excluded) names who
commands. If you are the Claude commander and it says `"role": "glm"`, `glm-commander` is in charge: do not
merge, prompt lanes, start agents or edit COMMANDER.md — wait for the guard's RESUME message (a background
watcher firing is not a resume). The GLM commander likewise never acts once the lead is back to `claude`.

## GLM commander protocol (`glm-commander`, started by the fleet guard)

You run while every Claude agent is paused. You hold the integration worktree's lane lead.

May do:
1. Monitor `glm-*` lane agents (`herdr agent list`, `herdr agent read <name> --source recent-unwrapped --lines 80`).
   Unblock them with answers that follow DESIGN.md/FLEET.md; if an answer would change the contract or a
   decision recorded in COMMANDER.md, tell the lane to note it as an open question and continue with
   other work instead.
2. Merge a lane whose agent reported `LANE DONE` **and** whose branch is green
   (`go build ./... && go vet ./... && go test ./...` in its worktree), following the merge procedure
   above but with subject `[glm-handoff] Merge lane <lane>: …`.
3. Start a next-wave lane **only** if its brief already exists in `briefs/` and COMMANDER.md "Next
   actions" lists it as ready; start it as a GLM agent `glm-<lane>` (same tab procedure, but run
   `export RI_ROLE=glm ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic ANTHROPIC_AUTH_TOKEN="$(cat ~/.claude_token_zai)"`
   in the pane first and start with `-- --model glm-5.3 --permission-mode bypassPermissions`; create
   the lane worktree with `git worktree add -b p4ei/<lane> .claude/worktrees/p4ei-<lane> p4-env-impact`).
4. Append to the **GLM commander log** section of COMMANDER.md (timestamp, what you did, why, anything
   uncertain) and keep the lane table current. Commit with `[glm-handoff] commander: …`.

Must not: edit DESIGN.md decisions, gates files or ground-truth expectations; write new briefs; push,
open PRs, delete branches/worktrees; start or prompt Claude-model agents; close or kill any `ri-*`
agent or its pane; edit `.lane-lead`.

Handback (message starting `HANDBACK`): stop starting work, finish/abort any in-progress merge cleanly
(no half-merged state), commit, write a final GLM commander log entry listing every merge, every lane
you started, and every question left open, then reply exactly `HANDBACK DONE`.

## Claude commander on resume after a GLM window

Read the GLM commander log and `git log <handoff base>..HEAD` on `p4-env-impact`; review every
`[glm-handoff]` merge critically (re-run tests; revert if wrong); decide whether lanes the GLM commander
started continue under GLM or move to a Claude agent; then continue from "Next actions".
