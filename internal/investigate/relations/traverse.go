package relations

import (
	"context"
	"fmt"
	"sort"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

// Algorithm is the traversal identifier recorded in TraversalResult.
const Algorithm = "graph.dfs@v1"

type traverser struct{}

// NewTraverser returns the bounded, deterministic DFS (DESIGN.md §2.5).
func NewTraverser() investigate.Traverser { return traverser{} }

type walk struct {
	g        *domain.RelationshipGraph
	bound    domain.TraversalBound
	out      map[string][]domain.GraphEdge // outgoing edges, sorted (kind, to, id)
	incoming map[string][]domain.GraphEdge // overrides edges into a node
	edges    map[string]domain.GraphEdge

	visited map[string]bool
	paths   []domain.ImpactPath
	depth   int
	trunc   string // first truncation reason
	stop    bool   // MaxPaths reached: nothing more can be emitted
	item    string
	ctxErr  error
}

func (t traverser) Traverse(ctx context.Context, g *domain.RelationshipGraph, roots []domain.ExposureRoot, bound domain.TraversalBound) (*domain.TraversalResult, error) {
	if g == nil {
		return nil, fmt.Errorf("relations: nil graph: %w", investigate.ErrRejected)
	}
	if bound.MaxDepth <= 0 || bound.MaxPaths <= 0 || bound.MaxNodes <= 0 {
		return nil, fmt.Errorf("relations: every traversal bound must be positive, got %+v: %w", bound, investigate.ErrRejected)
	}
	nodes := map[string]bool{}
	for _, n := range g.Nodes {
		nodes[n.ID] = true
	}
	w := &walk{g: g, bound: bound, out: map[string][]domain.GraphEdge{}, incoming: map[string][]domain.GraphEdge{}, edges: map[string]domain.GraphEdge{}, visited: map[string]bool{}}
	for _, e := range g.Edges {
		w.edges[e.ID] = e
		w.out[e.From] = append(w.out[e.From], e)
		if e.Kind == domain.EdgeOverrides {
			w.incoming[e.To] = append(w.incoming[e.To], e)
		}
	}
	for _, es := range w.out {
		sort.Slice(es, func(i, j int) bool {
			if es[i].Kind != es[j].Kind {
				return es[i].Kind < es[j].Kind
			}
			if es[i].To != es[j].To {
				return es[i].To < es[j].To
			}
			return es[i].ID < es[j].ID
		})
	}

	// Roots sorted by (change item, node id); one per pair, the first wins.
	rs := append([]domain.ExposureRoot(nil), roots...)
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].ChangeItem != rs[j].ChangeItem {
			return rs[i].ChangeItem < rs[j].ChangeItem
		}
		if rs[i].Node != rs[j].Node {
			return rs[i].Node < rs[j].Node
		}
		return rs[i].Rule < rs[j].Rule
	})
	var uniq []domain.ExposureRoot
	for i, r := range rs {
		if !nodes[r.Node] {
			return nil, fmt.Errorf("relations: root node %s of %s is not in graph %s: %w", r.Node, r.ChangeItem, g.ID, investigate.ErrRejected)
		}
		if i > 0 && rs[i-1].ChangeItem == r.ChangeItem && rs[i-1].Node == r.Node {
			continue
		}
		uniq = append(uniq, r)
	}

	for _, r := range uniq {
		if w.stop {
			break
		}
		w.item = r.ChangeItem
		if !w.enter(r.Node) {
			continue
		}
		w.dfs(ctx, []string{r.Node}, nil, map[string]bool{r.Node: true})
		if w.ctxErr != nil {
			return nil, w.ctxErr
		}
	}

	res := &domain.TraversalResult{
		Graph: g.ID, Algorithm: Algorithm, Bound: bound, Roots: uniq,
		DepthReached: w.depth, Paths: w.paths, Truncated: w.trunc != "", TruncationReason: w.trunc,
	}
	if res.Roots == nil {
		res.Roots = []domain.ExposureRoot{}
	}
	if res.Paths == nil {
		res.Paths = []domain.ImpactPath{}
	}
	res.Visited = make([]string, 0, len(w.visited))
	for n := range w.visited {
		res.Visited = append(res.Visited, n)
	}
	sort.Strings(res.Visited)
	unresolved := map[string]bool{}
	anyUnresolved := false
	for _, p := range res.Paths {
		if !p.Status.Resolved() {
			anyUnresolved = true
		}
		for _, id := range p.Edges {
			if !w.edges[id].Status.Resolved() {
				unresolved[id] = true
			}
		}
	}
	res.Unresolved = sortedKeys(unresolved)
	res.Complete = !res.Truncated && !anyUnresolved && len(res.Unresolved) == 0
	res.ID = res.DerivedID()
	return res, nil
}

// enter marks a node visited, honouring MaxNodes. It reports false (and
// truncates) when the node cannot be entered.
func (w *walk) enter(n string) bool {
	if w.visited[n] {
		return true
	}
	if len(w.visited) >= w.bound.MaxNodes {
		w.truncate(fmt.Sprintf("max nodes %d reached", w.bound.MaxNodes))
		return false
	}
	w.visited[n] = true
	return true
}

func (w *walk) truncate(reason string) {
	if w.trunc == "" {
		w.trunc = reason
	}
}

// dfs extends the current simple path. A path is emitted when no outgoing
// edge leads to a node off the path (maximal), or when a bound cuts it.
func (w *walk) dfs(ctx context.Context, nodes, edges []string, onPath map[string]bool) {
	if w.stop || w.ctxErr != nil {
		return
	}
	if err := ctx.Err(); err != nil {
		w.ctxErr = err
		return
	}
	cur := nodes[len(nodes)-1]
	var next []domain.GraphEdge
	for _, e := range w.out[cur] {
		if !onPath[e.To] {
			next = append(next, e)
		}
	}
	extended := false
	for _, e := range next {
		if w.stop {
			return
		}
		if len(edges) >= w.bound.MaxDepth {
			w.truncate(fmt.Sprintf("max depth %d reached", w.bound.MaxDepth))
			break
		}
		if !w.enter(e.To) {
			continue
		}
		extended = true
		onPath[e.To] = true
		w.dfs(ctx, append(nodes, e.To), append(edges, e.ID), onPath)
		delete(onPath, e.To)
	}
	if !extended {
		w.emit(nodes, edges)
	}
}

func (w *walk) emit(nodes, edges []string) {
	if len(w.paths) >= w.bound.MaxPaths {
		w.truncate(fmt.Sprintf("max paths %d reached", w.bound.MaxPaths))
		w.stop = true
		return
	}
	p := domain.ImpactPath{
		ChangeItem: w.item,
		Nodes:      append([]string(nil), nodes...),
		Edges:      append([]string(nil), edges...),
		Status:     domain.EdgeEstablished,
	}
	if len(p.Edges) == 0 {
		p.Edges = nil
	}
	ev := map[string]bool{}
	for _, id := range edges {
		e := w.edges[id]
		if p.Status == domain.EdgeEstablished || e.Status == domain.EdgeMissing {
			p.Status = e.Status
		}
		for _, x := range e.Evidence {
			ev[string(x)] = true
		}
	}
	for _, x := range sortedKeys(ev) {
		p.Evidence = append(p.Evidence, domain.EvidenceID(x))
	}
	mit := map[string]bool{}
	for _, n := range nodes {
		for _, e := range w.incoming[n] {
			for _, m := range e.Mitigates {
				if m == w.item {
					mit[e.ID] = true
				}
			}
		}
	}
	p.Mitigations = sortedKeys(mit)
	p.ID = domain.ImpactPathID(p.ChangeItem, p.Nodes, p.Edges)
	if len(edges) > w.depth {
		w.depth = len(edges)
	}
	w.paths = append(w.paths, p)
}
