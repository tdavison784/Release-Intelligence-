# Lane `knowledge` — semantic proposals for the milestone products that have none — Sonnet

Branch `p4ei/knowledge`, worktree `.claude/worktrees/p4ei-knowledge`. Read FLEET.md, `docs/SEMANTIC.md`,
`docs/KNOWLEDGE.md`, `docs/phase3/learning-loop/DESIGN.md` (entity lifecycle) and the Phase 3 run notes in
`docs/phase3/learning-loop/runs/semantic-v4/README.md` (how a multi-model proposal run was done without API
keys, and its cache/binary-drift warnings).

## Why

The product owner will human-verify the controller-default facts in the review UI so OverrideInspection can use
them as pin sources (DESIGN §3 R2.4). `knowledge/` has facts for cert-manager, but **none for ingress-nginx or
kube-prometheus-stack** (and prometheus-operator only partially), so nothing for them can reach the review
queue.

## Do

1. Run the existing learning-loop pipeline unchanged (`ri semantic candidates|propose`, prompt v2, the
   documented multi-model setup with `claude -p`: claude-opus-5-5 and claude-sonnet-5-5; no new prompts) on the
   release edges ingress-nginx 1.11→1.12 (chart 4.11.5→4.12.0) and kube-prometheus-stack 89.2.4→90.2.0, and
   import the proposals into `knowledge/` so they become **review items** (not facts). Use the warm cache
   (`-state /Users/tommydavison/repos/Release-Intelligence-/.ri`), live fetches with
   `GITHUB_TOKEN=$(gh auth token)` only if needed.
2. Do **not** run any proxy review, consensus auto-acceptance or anything that verifies them — verification is
   the product owner's job in the review UI.
3. Integrity: proposals are release-level; never read `eval/investigations/*` (inputs or expectations) or tune
   anything to a case.
4. Record in your status file: commands, models, token use, number of candidates / proposals / review items per
   product, and which of them the review UI's blocking signal will surface (run the reviewfilter view counts
   after import).

Done: proposals imported, store validates (`ri knowledge` checks), build/vet/test green, status, cleanup,
`LANE DONE:`.
