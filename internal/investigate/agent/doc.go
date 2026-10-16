// Package agent is the investigation agent stage (MISSION §A4, P3; DESIGN.md
// §4): a change-scoped tool surface (Session, implementing investigate.Tools)
// served to a model over MCP, and the drivers that run a model against it.
//
// The agent can look (render diffs first — query_diff, get_rendered_object,
// list_renders — then the upstream text of each change item, the manifest,
// the relationship graph), record hypotheses and assertions with verbatim
// citations, and request evidence and deterministic validations. It cannot
// classify: every record it makes carries AI provenance and can only raise
// concern (DESIGN.md §3.3). Every tool call is validated before anything is
// recorded (unknown ids, uncited claims, quotes that do not occur in the
// cited artifact are rejected), counted against the budget, and written to
// the agent transcript, which a replay re-issues to reproduce every result
// digest.
//
// Drivers: claude-cli (`claude -p` with an MCP config that bridges to the
// in-process session), exchange (the -llm-exchange directory protocol, one
// tool round per request), replay (a recorded transcript) and scripted
// (tests). The stage is opt-in (AgentConfig.Enabled, CLI --agent): with it,
// environment configuration — Secret values redacted at collection — is sent
// to the configured model provider.
package agent
