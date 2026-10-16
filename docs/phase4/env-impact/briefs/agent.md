# Lane `agent` — the investigation agent (P3, MISSION §A4) — Opus

Branch `p4ei/agent`, worktree `.claude/worktrees/p4ei-agent`. Read `_wave2-common.md` first.

Implement DESIGN §4 and §7 "agent" exactly (owns `internal/investigate/agent/` incl. embedded prompts,
`cmd/ri/investigate_mcp.go`): `investigate.Tools` (id/citation validation, budgets, transcript), the MCP
stdio server `ri investigate mcp`, drivers `claude-cli` / `exchange` / `replay` / `scripted`, and
`agent.New(...) investigate.Investigator`.

The product owner's centerpiece is the render **diffs**: the agent works primarily from `query_diff` /
`get_rendered_object` / `list_renders` (renderdiff) and `get_change_item` (upstream change text, so it can
name the deciding setting for controller-behaviour items), with relations as supporting context; it never
receives the whole bundle in context (MISSION §A4: searchable change-scoped manifest). The prompt frames
the task per DESIGN §4.6: hypotheses → evidence → request validation; it may raise concerns and request
validations/evidence, never decide a class. Prompts are generic (never tuned to a case), versioned and
digested.

Data handling: the agent stage is **opt-in** (`--agent`) and Secret values are redacted before anything
reaches a model (DESIGN §2.3); the user has not yet confirmed provider data sharing beyond that — keep it
opt-in and say so in the docs.

Done: scripted-driver tests for every rejection path (unknown id, uncited claim, budget exceeded, invalid
output); a replay reproduces result digests; **one live `claude -p` run** (claude-sonnet-5-5 is fine; no API
keys — the CLI is logged in) on one groundtruth `input.yaml`, transcript + result committed under
`docs/phase4/env-impact/runs/agent-live-1/` — you must not read that case's expected.yaml and you report
what the agent did, not whether it was "right" (the eval lane scores it).
