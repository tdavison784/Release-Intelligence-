# Status — lane `agent` (Claude Opus 5.5, agent ri-agent)

Branch `p4ei/agent`, worktree `.claude/worktrees/p4ei-agent`. Brief: `briefs/agent.md` (+ `_wave2-common.md`,
`_wave1-common.md`, DESIGN §4 / §7 "agent"). Built against merged ports at `p4-env-impact` @ adf4fe08
(bundle, store, renderdiff, relations merged; validate NOT merged yet → validators are injected, faked in tests).
State: **IN PROGRESS** (2026-10-16). Resume from: this file + brief + `git log`.

## Plan

- `internal/investigate/agent/`
  - `tools.go` — `Session` implements `investigate.Tools` for one investigation: id/citation validation
    (`investigate.QuoteOccurs`), budgets (tool calls, read bytes), transcript, records with AI provenance.
  - `dispatch.go` — JSON dispatch by MCP tool name (strict decoding) used by MCP + exchange + replay + scripted.
  - `mcp.go` — minimal MCP stdio server (JSON-RPC 2.0: initialize, tools/list, tools/call, ping).
  - `prompts/investigate-agent/v1/{system.md,task.md,tools.json}` (embedded); digest = sha256(system ‖ task ‖ tools).
  - drivers: `claude-cli` (claude -p + --mcp-config bridge over a unix socket to the in-process session),
    `exchange` (llm.Exchange, one tool round per request file), `replay` (re-issues a transcript, checks
    result digests), `scripted` (tests).
  - `agent.New(st, opts...) investigate.Investigator`.
- `cmd/ri/investigate_mcp.go` — `ri investigate mcp --run <inv-id> [--attach <socket>]`.
- Live run on one groundtruth `input.yaml` → `docs/phase4/env-impact/runs/agent-live-1/`.

## Done

- (nothing committed yet)

## Decisions (with why)

- (filled as I go)

## Files touched outside ownership

- (none yet)

## Contract changes

- (none yet)

## Open questions

- (none yet)

## Tests

- (not run yet)
