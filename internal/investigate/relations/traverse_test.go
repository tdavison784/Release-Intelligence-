package relations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
)

type te struct {
	from, to string
	kind     domain.EdgeKind
	status   domain.EdgeStatus
	mitigate []string
}

// graphOf builds a valid graph over named nodes; edges are given by name.
func graphOf(t testing.TB, names []string, edges []te) (*domain.RelationshipGraph, map[string]string) {
	t.Helper()
	ids := map[string]string{}
	g := &domain.RelationshipGraph{Bundle: "bnd-t", Builder: "test@v1"}
	for _, n := range names {
		node := domain.GraphNode{Kind: domain.NodeResource, Key: "g/v/Kind/ns/" + n}
		node.ID = domain.NodeID(node.Kind, node.Key)
		ids[n] = node.ID
		g.Nodes = append(g.Nodes, node)
	}
	for i, e := range edges {
		kind := e.kind
		if kind == "" {
			kind = domain.EdgeReferences
		}
		st := e.status
		if st == "" {
			st = domain.EdgeEstablished
		}
		ge := domain.GraphEdge{Kind: kind, From: ids[e.from], To: ids[e.to], Status: st, Rule: "test", Detail: fmt.Sprint(i), Artifacts: []string{"art-x"}, Mitigates: e.mitigate}
		if st != domain.EdgeEstablished {
			ge.Reason = "because"
		}
		ge.ID = domain.EdgeID(ge.Kind, ge.From, ge.To, ge.Detail)
		g.Edges = append(g.Edges, ge)
	}
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ID < g.Nodes[j].ID })
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].ID < g.Edges[j].ID })
	g.ID = g.DerivedID()
	if err := g.Validate(); err != nil {
		t.Fatal(err)
	}
	return g, ids
}

func rootsOf(ids map[string]string, item string, names ...string) []domain.ExposureRoot {
	var out []domain.ExposureRoot
	for _, n := range names {
		out = append(out, domain.ExposureRoot{ChangeItem: item, Node: ids[n], Rule: RuleRenderedChange, RenderedChange: "rc-" + n})
	}
	return out
}

func traverse(t testing.TB, g *domain.RelationshipGraph, roots []domain.ExposureRoot, b domain.TraversalBound) *domain.TraversalResult {
	t.Helper()
	res, err := NewTraverser().Traverse(context.Background(), g, roots, b)
	if err != nil {
		t.Fatal(err)
	}
	if err := res.Validate(); err != nil {
		t.Fatalf("result does not validate: %v", err)
	}
	if err := res.ValidateAgainst(*g); err != nil {
		t.Fatalf("result does not validate against its graph: %v", err)
	}
	return res
}

func pathNames(g *domain.RelationshipGraph, res *domain.TraversalResult) []string {
	name := map[string]string{}
	for _, n := range g.Nodes {
		name[n.ID] = n.Key[strings.LastIndex(n.Key, "/")+1:]
	}
	var out []string
	for _, p := range res.Paths {
		var ns []string
		for _, n := range p.Nodes {
			ns = append(ns, name[n])
		}
		out = append(out, strings.Join(ns, ">"))
	}
	sort.Strings(out)
	return out
}

func eq(t *testing.T, got, want []string) {
	t.Helper()
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got  %v\nwant %v", got, want)
	}
}

func TestTraverseMaximalSimplePaths(t *testing.T) {
	//   a → b → d
	//   a → c → d → e
	g, ids := graphOf(t, []string{"a", "b", "c", "d", "e"}, []te{{from: "a", to: "b"}, {from: "a", to: "c"}, {from: "b", to: "d"}, {from: "c", to: "d"}, {from: "d", to: "e"}})
	res := traverse(t, g, rootsOf(ids, "ci-1", "a"), domain.DefaultTraversalBound)
	eq(t, pathNames(g, res), []string{"a>b>d>e", "a>c>d>e"})
	if !res.Complete || res.Truncated || res.DepthReached != 3 || len(res.Visited) != 5 {
		t.Errorf("unexpected result: complete=%v truncated=%v depth=%d visited=%d", res.Complete, res.Truncated, res.DepthReached, len(res.Visited))
	}
	if res.Algorithm != "graph.dfs@v1" {
		t.Errorf("algorithm = %q", res.Algorithm)
	}
	// Every edge lies on some emitted path.
	onPath := map[string]bool{}
	for _, p := range res.Paths {
		for _, e := range p.Edges {
			onPath[e] = true
		}
	}
	for _, e := range g.Edges {
		if !onPath[e.ID] {
			t.Errorf("edge %s on no path", e.ID)
		}
	}
}

func TestTraverseRootOnlyIsEstablishedAndComplete(t *testing.T) {
	g, ids := graphOf(t, []string{"a", "b"}, nil)
	res := traverse(t, g, rootsOf(ids, "ci-1", "a"), domain.DefaultTraversalBound)
	if len(res.Paths) != 1 || len(res.Paths[0].Nodes) != 1 || res.Paths[0].Status != domain.EdgeEstablished || !res.Complete {
		t.Fatalf("root-only path should be established and complete: %+v", res)
	}
	// No roots at all: nothing to walk, still complete (the light layer never weakens a conclusion).
	res = traverse(t, g, nil, domain.DefaultTraversalBound)
	if len(res.Paths) != 0 || !res.Complete {
		t.Fatalf("no roots: %+v", res)
	}
}

func TestTraverseCyclesAreSimplePaths(t *testing.T) {
	g, ids := graphOf(t, []string{"a", "b", "c"}, []te{{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "c", to: "a"}, {from: "c", to: "b"}})
	res := traverse(t, g, rootsOf(ids, "ci-1", "a"), domain.DefaultTraversalBound)
	eq(t, pathNames(g, res), []string{"a>b>c"})
}

func TestTraverseUnresolvedEdgesMakeItIncomplete(t *testing.T) {
	g, ids := graphOf(t, []string{"a", "b", "c", "d"}, []te{
		{from: "a", to: "b"},
		{from: "b", to: "c", status: domain.EdgeInferred},
		{from: "a", to: "d", status: domain.EdgeMissing},
	})
	res := traverse(t, g, rootsOf(ids, "ci-1", "a"), domain.DefaultTraversalBound)
	if res.Complete || res.Truncated {
		t.Fatalf("complete=%v truncated=%v", res.Complete, res.Truncated)
	}
	if len(res.Unresolved) != 2 {
		t.Fatalf("unresolved = %v, want both non-established edges", res.Unresolved)
	}
	status := map[string]domain.EdgeStatus{}
	for i, p := range res.Paths {
		status[pathNames(g, res)[i]] = p.Status
	}
	if res.CompleteFor("ci-1") {
		t.Error("CompleteFor must be false for an item with unresolved paths")
	}
	if !res.CompleteFor("ci-other") {
		t.Error("an item with no paths is unaffected by another item's unresolved edges")
	}
	// weakest edge status wins: missing > inferred > established
	for _, p := range res.Paths {
		last := p.Edges[len(p.Edges)-1]
		for _, e := range g.Edges {
			if e.ID == last && p.Status != e.Status {
				t.Errorf("path status %s, last edge %s", p.Status, e.Status)
			}
		}
	}
}

func TestTraverseBounds(t *testing.T) {
	// a chain of 6 edges
	g, ids := graphOf(t, []string{"a", "b", "c", "d", "e", "f", "g"}, []te{{from: "a", to: "b"}, {from: "b", to: "c"}, {from: "c", to: "d"}, {from: "d", to: "e"}, {from: "e", to: "f"}, {from: "f", to: "g"}})
	roots := rootsOf(ids, "ci-1", "a")

	res := traverse(t, g, roots, domain.TraversalBound{MaxDepth: 3, MaxPaths: 10, MaxNodes: 100})
	if !res.Truncated || !strings.Contains(res.TruncationReason, "max depth 3") || res.Complete {
		t.Errorf("depth bound: truncated=%v reason=%q complete=%v", res.Truncated, res.TruncationReason, res.Complete)
	}
	if res.DepthReached != 3 || len(res.Paths) != 1 || len(res.Paths[0].Edges) != 3 {
		t.Errorf("depth bound path: %+v", res.Paths)
	}
	if res.CompleteFor("ci-1") {
		t.Error("a truncated traversal is never complete for any item")
	}

	res = traverse(t, g, roots, domain.TraversalBound{MaxDepth: 8, MaxPaths: 10, MaxNodes: 4})
	if !res.Truncated || !strings.Contains(res.TruncationReason, "max nodes 4") || len(res.Visited) != 4 {
		t.Errorf("node bound: reason=%q visited=%d", res.TruncationReason, len(res.Visited))
	}

	// exactly at the bound is not truncated
	res = traverse(t, g, roots, domain.TraversalBound{MaxDepth: 6, MaxPaths: 1, MaxNodes: 7})
	if res.Truncated || !res.Complete || len(res.Paths) != 1 {
		t.Errorf("at-bound traversal should be complete: %+v", res)
	}

	// path bound: a fan of 5 leaves
	g2, ids2 := graphOf(t, []string{"r", "l1", "l2", "l3", "l4", "l5"}, []te{{from: "r", to: "l1"}, {from: "r", to: "l2"}, {from: "r", to: "l3"}, {from: "r", to: "l4"}, {from: "r", to: "l5"}})
	res = traverse(t, g2, rootsOf(ids2, "ci-1", "r"), domain.TraversalBound{MaxDepth: 8, MaxPaths: 3, MaxNodes: 100})
	if !res.Truncated || !strings.Contains(res.TruncationReason, "max paths 3") || len(res.Paths) != 3 || res.Complete {
		t.Errorf("path bound: truncated=%v reason=%q paths=%d", res.Truncated, res.TruncationReason, len(res.Paths))
	}
}

func TestTraverseRejectsBadInput(t *testing.T) {
	g, ids := graphOf(t, []string{"a"}, nil)
	for name, call := range map[string]func() error{
		"zero bound": func() error {
			_, err := NewTraverser().Traverse(context.Background(), g, nil, domain.TraversalBound{MaxDepth: 1})
			return err
		},
		"unknown root": func() error {
			_, err := NewTraverser().Traverse(context.Background(), g, []domain.ExposureRoot{{ChangeItem: "ci-1", Node: "gn-nope", Rule: "x"}}, domain.DefaultTraversalBound)
			return err
		},
		"nil graph": func() error {
			_, err := NewTraverser().Traverse(context.Background(), nil, nil, domain.DefaultTraversalBound)
			return err
		},
	} {
		if err := call(); !errors.Is(err, investigate.ErrRejected) {
			t.Errorf("%s: err = %v, want ErrRejected", name, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewTraverser().Traverse(ctx, g, rootsOf(ids, "ci-1", "a"), domain.DefaultTraversalBound); !errors.Is(err, context.Canceled) {
		t.Errorf("cancelled: err = %v", err)
	}
}

func TestTraverseMitigations(t *testing.T) {
	g, ids := graphOf(t, []string{"a", "b", "ovr"}, []te{
		{from: "a", to: "b"},
		{from: "ovr", to: "b", kind: domain.EdgeOverrides, mitigate: []string{"ci-1"}},
		{from: "ovr", to: "a", kind: domain.EdgeOverrides, mitigate: []string{"ci-2"}},
	})
	res := traverse(t, g, rootsOf(ids, "ci-1", "a"), domain.DefaultTraversalBound)
	if len(res.Paths) != 1 || len(res.Paths[0].Mitigations) != 1 {
		t.Fatalf("want exactly the override that mitigates ci-1: %+v", res.Paths)
	}
}

func TestTraverseSeparatesChangeItems(t *testing.T) {
	g, ids := graphOf(t, []string{"a", "b", "c"}, []te{{from: "a", to: "b"}, {from: "c", to: "b", status: domain.EdgeInferred}})
	roots := append(rootsOf(ids, "ci-2", "c"), rootsOf(ids, "ci-1", "a")...)
	roots = append(roots, rootsOf(ids, "ci-1", "a")...) // duplicate (item, node): one root
	res := traverse(t, g, roots, domain.DefaultTraversalBound)
	if len(res.Roots) != 2 || res.Roots[0].ChangeItem != "ci-1" {
		t.Fatalf("roots must be sorted and unique: %+v", res.Roots)
	}
	if !res.CompleteFor("ci-1") || res.CompleteFor("ci-2") {
		t.Errorf("completeness is per item: ci-1=%v ci-2=%v", res.CompleteFor("ci-1"), res.CompleteFor("ci-2"))
	}
	if res.Complete {
		t.Error("overall traversal is incomplete when any item's paths are unresolved")
	}
}

func randomGraph(t testing.TB, r *rand.Rand, n, m int) (*domain.RelationshipGraph, map[string]string, []string) {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("n%02d", i)
	}
	var edges []te
	kinds := []domain.EdgeKind{domain.EdgeReferences, domain.EdgeSelects, domain.EdgeConsumesAPI}
	sts := []domain.EdgeStatus{domain.EdgeEstablished, domain.EdgeEstablished, domain.EdgeInferred, domain.EdgeMissing}
	for i := 0; i < m; i++ {
		a, b := r.Intn(n), r.Intn(n)
		if a == b {
			continue
		}
		edges = append(edges, te{from: names[a], to: names[b], kind: kinds[r.Intn(len(kinds))], status: sts[r.Intn(len(sts))]})
	}
	g, ids := graphOf(t, names, edges)
	return g, ids, names
}

// Same graph, roots and bound ⇒ byte-identical result, however the inputs are ordered.
func TestTraverseIsDeterministic(t *testing.T) {
	for seed := int64(1); seed <= 25; seed++ {
		r := rand.New(rand.NewSource(seed))
		g, ids, names := randomGraph(t, r, 9, 18)
		roots := append(rootsOf(ids, "ci-1", names[0], names[3]), rootsOf(ids, "ci-2", names[5])...)
		bound := domain.TraversalBound{MaxDepth: 5, MaxPaths: 40, MaxNodes: 8}
		first, _ := json.Marshal(traverse(t, g, roots, bound))
		for i := 0; i < 3; i++ {
			g2 := *g
			g2.Edges = append([]domain.GraphEdge(nil), g.Edges...)
			r.Shuffle(len(g2.Edges), func(a, b int) { g2.Edges[a], g2.Edges[b] = g2.Edges[b], g2.Edges[a] })
			g2.Nodes = append([]domain.GraphNode(nil), g.Nodes...)
			r.Shuffle(len(g2.Nodes), func(a, b int) { g2.Nodes[a], g2.Nodes[b] = g2.Nodes[b], g2.Nodes[a] })
			roots2 := append([]domain.ExposureRoot(nil), roots...)
			r.Shuffle(len(roots2), func(a, b int) { roots2[a], roots2[b] = roots2[b], roots2[a] })
			res, err := NewTraverser().Traverse(context.Background(), &g2, roots2, bound)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := json.Marshal(res)
			if string(got) != string(first) {
				t.Fatalf("seed %d: result depends on input order\n%s\n%s", seed, first, got)
			}
		}
	}
}

// Untruncated: every edge reachable from a root lies on an emitted path, paths are simple and maximal.
func TestTraverseCoversEveryReachableEdge(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		r := rand.New(rand.NewSource(seed))
		g, ids, names := randomGraph(t, r, 7, 12)
		roots := rootsOf(ids, "ci-1", names[0])
		res := traverse(t, g, roots, domain.TraversalBound{MaxDepth: 8, MaxPaths: 100000, MaxNodes: 5000})
		if res.Truncated {
			t.Fatalf("seed %d: unexpectedly truncated: %s", seed, res.TruncationReason)
		}
		reach := map[string]bool{ids[names[0]]: true}
		for changed := true; changed; {
			changed = false
			for _, e := range g.Edges {
				if reach[e.From] && !reach[e.To] {
					reach[e.To], changed = true, true
				}
			}
		}
		onPath := map[string]bool{}
		for _, p := range res.Paths {
			seen := map[string]bool{}
			for _, n := range p.Nodes {
				if seen[n] {
					t.Fatalf("seed %d: path revisits %s", seed, n)
				}
				seen[n] = true
			}
			for _, e := range p.Edges {
				onPath[e] = true
			}
			last := p.Nodes[len(p.Nodes)-1]
			for _, e := range g.Edges {
				if e.From == last && !seen[e.To] {
					t.Fatalf("seed %d: path %v is not maximal (edge %s continues)", seed, p.Nodes, e.ID)
				}
			}
		}
		for _, e := range g.Edges {
			if reach[e.From] && simplyReachable(g, ids[names[0]], e) && !onPath[e.ID] {
				t.Errorf("seed %d: edge %s is enterable by a simple path from the root but is on no emitted path", seed, e.ID)
			}
		}
	}
}

// simplyReachable reports whether some simple path from root reaches e.From without passing through e.To,
// i.e. whether the edge can be taken at all under "a node already on the path is not re-entered".
func simplyReachable(g *domain.RelationshipGraph, root string, e domain.GraphEdge) bool {
	if root == e.To {
		return false
	}
	var dfs func(cur string, on map[string]bool) bool
	dfs = func(cur string, on map[string]bool) bool {
		if cur == e.From {
			return true
		}
		for _, x := range g.Edges {
			if x.From == cur && !on[x.To] && x.To != e.To {
				on[x.To] = true
				ok := dfs(x.To, on)
				delete(on, x.To)
				if ok {
					return true
				}
			}
		}
		return false
	}
	return dfs(root, map[string]bool{root: true})
}

func BenchmarkTraverseDefaultBound(b *testing.B) {
	r := rand.New(rand.NewSource(7))
	g, ids, names := randomGraph(b, r, 40, 70)
	roots := rootsOf(ids, "ci-1", names[0])
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := NewTraverser().Traverse(context.Background(), g, roots, domain.DefaultTraversalBound); err != nil {
			b.Fatal(err)
		}
	}
}

// chain builds nodes n0..n<steps+1> with edges n0→n1→…: a straight chain of
// the given edge count.
func chain(t testing.TB, steps int) (*domain.RelationshipGraph, map[string]string) {
	t.Helper()
	var names []string
	for i := 0; i <= steps; i++ {
		names = append(names, fmt.Sprintf("n%d", i))
	}
	var edges []te
	for i := 0; i < steps; i++ {
		edges = append(edges, te{from: names[i], to: names[i+1]})
	}
	return graphOf(t, names, edges)
}

// The default depth must cover the chains real GitOps environments produce:
// a straight relationship chain as deep as the bound still traverses
// completely (the measured deepest real chain is 9 edges — cert-manager's
// two-tier internal PKI plus the workload→service tail).
func TestDefaultDepthCoversRealChains(t *testing.T) {
	g, ids := chain(t, domain.DefaultTraversalBound.MaxDepth)
	res := traverse(t, g, rootsOf(ids, "ci-1", "n0"), domain.DefaultTraversalBound)
	if res.Truncated || res.DepthReached != domain.DefaultTraversalBound.MaxDepth || !res.Complete {
		t.Fatalf("a chain within the bound must traverse completely: truncated=%v reason=%q depth=%d complete=%v",
			res.Truncated, res.TruncationReason, res.DepthReached, res.Complete)
	}
}

// The bound stays a bound: one edge deeper than the default truncates
// honestly — reason recorded, never complete, so not-affected stays blocked.
func TestDefaultDepthStillTruncatesHonestly(t *testing.T) {
	g, ids := chain(t, domain.DefaultTraversalBound.MaxDepth+1)
	res := traverse(t, g, rootsOf(ids, "ci-1", "n0"), domain.DefaultTraversalBound)
	if !res.Truncated || res.Complete {
		t.Fatalf("a chain past the bound must truncate and never be complete: %+v", res)
	}
	if res.TruncationReason != fmt.Sprintf("max depth %d reached", domain.DefaultTraversalBound.MaxDepth) {
		t.Fatalf("reason = %q", res.TruncationReason)
	}
}
