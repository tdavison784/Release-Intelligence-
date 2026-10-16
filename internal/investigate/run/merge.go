package run

import (
	"fmt"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
)

const proposedRule = "agent.proposed-edge@v1"

// MergeProposed returns the graph extended with the relationships the agent
// proposed (DESIGN.md §5.1). A proposed edge is always `inferred` and names
// its assertion; an end that is not a node of the graph becomes a placeholder
// external node, so the edge can only add unresolved paths — it reaches the
// policy and can never complete a traversal. Edges that would make the graph
// invalid are skipped and reported in notes. Without proposals the same graph
// (same id) comes back.
func MergeProposed(g *domain.RelationshipGraph, assertions []domain.AgentAssertion) (*domain.RelationshipGraph, []string) {
	as := append([]domain.AgentAssertion(nil), assertions...)
	sort.Slice(as, func(i, j int) bool { return as[i].ID < as[j].ID })
	cur := g
	var notes []string
	for _, a := range as {
		if a.ProposedEdge == nil {
			continue
		}
		next := withEdge(cur, a)
		if err := next.Validate(); err != nil {
			notes = append(notes, fmt.Sprintf("proposed edge of %s skipped: %v", a.ID, err))
			continue
		}
		cur = next
	}
	return cur, notes
}

func withEdge(g *domain.RelationshipGraph, a domain.AgentAssertion) *domain.RelationshipGraph {
	out := &domain.RelationshipGraph{
		Bundle: g.Bundle, Builder: g.Builder,
		Nodes: append([]domain.GraphNode(nil), g.Nodes...), Edges: append([]domain.GraphEdge(nil), g.Edges...),
	}
	known := map[string]bool{}
	for _, n := range out.Nodes {
		known[n.ID] = true
	}
	end := func(id string) string {
		if known[id] {
			return id
		}
		n := domain.GraphNode{Kind: domain.NodeExternal, Key: "agent-proposed:" + id, Placeholder: true}
		n.ID = domain.NodeID(n.Kind, n.Key)
		if !known[n.ID] {
			known[n.ID] = true
			out.Nodes = append(out.Nodes, n)
		}
		return n.ID
	}
	e := *a.ProposedEdge
	e.From, e.To = end(e.From), end(e.To)
	e.Status, e.Assertion = domain.EdgeInferred, a.ID
	e.Mitigates = nil // a proposal never mitigates
	if e.Rule == "" {
		e.Rule = proposedRule
	}
	if e.Reason == "" {
		e.Reason = "proposed by agent assertion " + a.ID
	}
	e.ID = domain.EdgeID(e.Kind, e.From, e.To, e.Detail)
	dup := false
	for _, x := range out.Edges {
		dup = dup || x.ID == e.ID
	}
	if !dup {
		out.Edges = append(out.Edges, e)
	}
	sort.Slice(out.Nodes, func(i, j int) bool { return out.Nodes[i].ID < out.Nodes[j].ID })
	sort.Slice(out.Edges, func(i, j int) bool { return out.Edges[i].ID < out.Edges[j].ID })
	out.ID = out.DerivedID()
	return out
}
