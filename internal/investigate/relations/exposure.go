package relations

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/catalog"
	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Exposure rule ids.
const (
	RuleRenderedChange = "exposure:rendered-change"
	RuleJoinMatch      = "exposure:join-match"
	// RuleReconciledKind roots a controller-behaviour (or unknown) item at
	// every environment object of a kind the product's controller
	// reconciles (the declarative reconciles: construct): a change compiled
	// into the controller reaches each of them.
	RuleReconciledKind = "exposure:product-reconciles"
)

type exposure struct {
	// reconciles: product id → "<group>|<kind>" set
	reconciles map[string]map[string]bool
}

// ExposureOption configures the exposure mapper.
type ExposureOption func(*exposure)

// WithReconciles declares the kinds a product's controller reconciles.
func WithReconciles(product string, kinds ...catalog.ObjectType) ExposureOption {
	return func(e *exposure) {
		if e.reconciles[product] == nil {
			e.reconciles[product] = map[string]bool{}
		}
		for _, k := range kinds {
			e.reconciles[product][k.Group+"|"+k.Kind] = true
		}
	}
}

// WithReconcilesFrom declares the reconciled kinds of every product in the
// catalog (products/<id>.yaml reconciles:).
func WithReconcilesFrom(c *catalog.Catalog) ExposureOption {
	return func(e *exposure) {
		if c == nil {
			return
		}
		for _, d := range c.List() {
			WithReconciles(d.ID, d.ReconciledKinds()...)(e)
		}
	}
}

// NewExposure returns the ExposureMapper (DESIGN.md §2.5): the roots of a
// change item are first the objects of the environment rendered changes
// correlated to it, then the graph nodes the deterministic join's matches
// name, and — for a controller-behaviour or unknown item, whose effect no
// render shows — every environment object of a kind the product's
// controller reconciles (WithReconciles). An item with none has no root (and
// an honest roots-only traversal).
func NewExposure(opts ...ExposureOption) investigate.ExposureMapper {
	e := exposure{reconciles: map[string]map[string]bool{}}
	for _, o := range opts {
		o(&e)
	}
	return e
}

func (x exposure) Exposure(ctx context.Context, b *domain.EvidenceBundle, g *domain.RelationshipGraph, report *domain.ImpactReport) ([]domain.ExposureRoot, error) {
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

	// Tertiary: controller-behaviour / unknown items reach every object
	// the product's controller reconciles (not placeholders: an object the
	// environment does not declare is not exposed by it).
	if kinds := x.reconciles[string(b.Change.Product.ID)]; len(kinds) > 0 {
		var reconciled []string
		for _, n := range g.Nodes {
			parts := strings.SplitN(n.Key, "/", 5)
			if n.Placeholder || len(parts) != 5 || parts[4] == "" {
				continue
			}
			group := parts[0]
			if group == "core" {
				group = ""
			}
			if kinds[group+"|"+parts[2]] {
				reconciled = append(reconciled, n.ID)
			}
		}
		sort.Strings(reconciled)
		for _, it := range b.Items() {
			if !it.InScope || it.Manifestation.RenderDecisive() {
				continue
			}
			for _, node := range reconciled {
				k := pair{it.ID, node}
				if _, dup := roots[k]; !dup {
					roots[k] = domain.ExposureRoot{ChangeItem: it.ID, Node: node, Rule: RuleReconciledKind}
				}
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
