package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/tdavison784/release-intelligence/internal/app"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate/agent"
	"github.com/tdavison784/release-intelligence/internal/investigate/store"
)

// investigateMCP is `ri investigate mcp`: the investigation agent's tools
// over MCP stdio (docs/INVESTIGATE.md "Investigation agent").
//
//	ri investigate mcp --attach <socket> --token <t>  bridge to a running agent session; the
//	                                                 claude-cli driver writes this into its MCP config
//	ri investigate mcp --run <inv-id>                 serve a stored investigation read-only
//	                                                 (query tools; records and requests are refused)
func (c *cli) investigateMCP(args []string) error {
	fs := c.flags("investigate mcp", "")
	run := fs.String("run", "", "investigation id (inv-...) to serve read-only from -state")
	attach := fs.String("attach", "", "unix socket of a running agent session")
	token := fs.String("token", "", "session token for --attach")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	if len(pos) != 0 || (*attach == "") == (*run == "") {
		fs.Usage()
		return fmt.Errorf("%w: give exactly one of --attach or --run", app.ErrUsage)
	}
	if *attach != "" {
		return agent.Attach(c.ctx, *attach, *token, os.Stdin, c.out)
	}
	s, err := readOnlySession(c.ctx, c.g.state, *run)
	if err != nil {
		return err
	}
	return agent.ServeMCP(c.ctx, os.Stdin, c.out, s)
}

// readOnlySession serves a stored investigation: its bundle, graph,
// traversal, validations and run artifacts.
func readOnlySession(ctx context.Context, state, run string) (*agent.Session, error) {
	st, err := store.New(state)
	if err != nil {
		return nil, err
	}
	inv, err := st.GetInvestigation(ctx, run)
	if err != nil {
		return nil, err
	}
	b, err := st.GetBundle(ctx, inv.Bundle)
	if err != nil {
		return nil, err
	}
	var graph *domain.RelationshipGraph
	if inv.Graph != "" {
		for _, a := range inv.Artifacts {
			if a.ID != inv.Graph {
				continue
			}
			rc, err := st.Open(ctx, a.Digest)
			if err != nil {
				return nil, err
			}
			raw, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return nil, err
			}
			graph = &domain.RelationshipGraph{}
			if err := json.Unmarshal(raw, graph); err != nil {
				return nil, fmt.Errorf("investigation %s: graph artifact: %w", run, err)
			}
		}
	}
	version := agent.DefaultPromptVersion
	if inv.Agent != nil && inv.Agent.PromptVersion != "" {
		version = inv.Agent.PromptVersion
	}
	p, err := agent.LoadPrompt(version)
	if err != nil {
		return nil, err
	}
	return agent.NewSession(agent.SessionConfig{Bundle: b, Store: st, Graph: graph, Traversal: inv.Traversal, Validations: inv.Validations,
		RunArtifacts: inv.Artifacts, Budget: agent.Budget(domain.AgentBudget{}), Prompt: p, ReadOnly: true})
}
