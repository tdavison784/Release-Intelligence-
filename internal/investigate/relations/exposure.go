package relations

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Exposure rule ids.
const (
	RuleRenderedChange = "exposure:rendered-change"
	RuleJoinMatch      = "exposure:join-match"
)

type exposure struct{}

// NewExposure returns the ExposureMapper (DESIGN.md §2.5): the roots of a
// change item are first the objects of the environment rendered changes
// correlated to it, then the graph nodes the deterministic join's matches
// name. An item with neither has no root (and an honest roots-only
// traversal).
func NewExposure() investigate.ExposureMapper { return exposure{} }

func (exposure) Exposure(ctx context.Context, b *domain.EvidenceBundle, g *domain.RelationshipGraph, report *domain.ImpactReport) ([]domain.ExposureRoot, error) {
	if b == nil || g == nil {
		return nil, fmt.Errorf("relations: bundle and graph are required: %w", investigate.ErrRejected)
	}
	byKey := map[string]string{}
	for _, n := range g.Nodes {
		byKey[n.Key] = n.ID
	}
	inScope := map[string]bool{}
	for _, it := range b.Items() { // record and environment-scope items (DESIGN §1.3 amendment)
		if it.InScope {
			inScope[it.ID] = true
		}
	}
	pool := b.EvidenceByID()

	type pair struct{ item, node string }
	roots := map[pair]domain.ExposureRoot{}

	// Primary: environment rendered changes, in id order so the first
	// rendered change of an (item, node) is the one recorded.
	var changes []domain.RenderedChange
	for _, p := range b.Renders {
		changes = append(changes, p.Changes...)
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].ID < changes[j].ID })
	for _, c := range changes {
		node, ok := byKey[c.Object]
		if !ok {
			continue
		}
		for _, item := range c.ChangeItems {
			k := pair{item, node}
			if !inScope[item] {
				continue
			}
			if _, dup := roots[k]; dup {
				continue
			}
			roots[k] = domain.ExposureRoot{ChangeItem: item, Node: node, Rule: RuleRenderedChange, RenderedChange: c.ID, Evidence: resolving(c.Evidence, pool)}
		}
	}

	// Secondary: the join's matches. A match subject names a node when it is
	// a node key, or the "Kind/name" or "namespace/Kind/name" tail of one.
	if report != nil {
		tails := tailIndex(g)
		findings := append([]domain.ImpactFinding(nil), report.Findings...)
		sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
		for _, f := range findings {
			if f.ChangeID == "" || len(f.Matches) == 0 {
				continue
			}
			item := domain.ChangeItemID(f.ChangeID, "", "")
			if !inScope[item] {
				continue
			}
			for _, m := range f.Matches {
				node := byKey[m.Subject]
				if node == "" {
					node = tails[m.Subject]
				}
				if node == "" {
					continue
				}
				k := pair{item, node}
				if _, dup := roots[k]; dup {
					continue
				}
				roots[k] = domain.ExposureRoot{ChangeItem: item, Node: node, Rule: RuleJoinMatch, ImpactFinding: f.ID, Evidence: resolving(m.Evidence, pool)}
			}
		}
	}

	out := make([]domain.ExposureRoot, 0, len(roots))
	for _, r := range roots {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ChangeItem != out[j].ChangeItem {
			return out[i].ChangeItem < out[j].ChangeItem
		}
		return out[i].Node < out[j].Node
	})
	return out, nil
}

// resolving keeps the evidence ids that resolve in the bundle's pool.
func resolving(ids []domain.EvidenceID, pool map[domain.EvidenceID]domain.Evidence) []domain.EvidenceID {
	var out []domain.EvidenceID
	for _, id := range ids {
		if _, ok := pool[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

// tailIndex maps "Kind/name" and "namespace/Kind/name" to the node, when the
// tail identifies exactly one node.
func tailIndex(g *domain.RelationshipGraph) map[string]string {
	count := map[string]int{}
	node := map[string]string{}
	for _, n := range g.Nodes {
		parts := strings.SplitN(n.Key, "/", 5)
		if len(parts) != 5 {
			continue
		}
		for _, t := range []string{parts[2] + "/" + parts[4], parts[3] + "/" + parts[2] + "/" + parts[4]} {
			count[t]++
			node[t] = n.ID
		}
	}
	out := map[string]string{}
	for t, c := range count {
		if c == 1 {
			out[t] = node[t]
		}
	}
	return out
}
