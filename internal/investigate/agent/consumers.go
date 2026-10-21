package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tdavison784/release-intelligence/internal/domain"
	"github.com/tdavison784/release-intelligence/internal/investigate"
	"github.com/tdavison784/release-intelligence/internal/investigate/relations"
)

// Consumer tracing (prompt feature traceConsumers; docs/phase6/decision-intel/
// PLAN-M1.md changes 3, 4 and 7). The session builds the consumer-mode graph
// of the bundle (relations.WithConsumers) beside the stored graph: the stored
// graph stays what every other tool, the validators and the policy see, so
// earlier prompt versions replay unchanged and no policy input moves. The
// consumer graph is stored as a run artifact; trace_consumers, the dossier's
// consumer chains and the completion check read it.

// ConsumerGraphProducer names the run artifact holding a consumer graph.
const ConsumerGraphProducer = "investigate.agent.consumers@v1"

// Trace bounds.
const (
	maxTraceDepth = 6
	maxTraceHops  = 200
	maxTraceGaps  = 60
)

// consumerKinds are the edges a trace follows: what an object produces, what
// references it (mounts, env, *Ref fields) and the configuration that names
// its files.
var consumerKinds = map[domain.EdgeKind]bool{domain.EdgeProduces: true, domain.EdgeReferences: true, domain.EdgeReadsFile: true}

// ConsumerGraphs builds the consumer-mode graph of a bundle at one side.
type ConsumerGraphs func(ctx context.Context, side relations.Side) (*domain.RelationshipGraph, error)

// cgraph is a consumer graph indexed for traces.
type cgraph struct {
	g     *domain.RelationshipGraph
	art   domain.ArtifactRef
	nodes map[string]domain.GraphNode
	byKey map[string]string
	out   map[string][]domain.GraphEdge // by From, sorted by edge id
	files map[string][]domain.GraphEdge // reads-file edges by To (the configuration), sorted by edge id
}

func indexGraph(g *domain.RelationshipGraph, art domain.ArtifactRef) *cgraph {
	cg := &cgraph{g: g, art: art, nodes: map[string]domain.GraphNode{}, byKey: map[string]string{}, out: map[string][]domain.GraphEdge{}, files: map[string][]domain.GraphEdge{}}
	for _, n := range g.Nodes { // sorted by id: the first id of a key wins
		cg.nodes[n.ID] = n
		if _, ok := cg.byKey[n.Key]; !ok {
			cg.byKey[n.Key] = n.ID
		}
	}
	for _, e := range g.Edges { // sorted by id
		if consumerKinds[e.Kind] {
			cg.out[e.From] = append(cg.out[e.From], e)
		}
		if e.Kind == domain.EdgeReadsFile {
			cg.files[e.To] = append(cg.files[e.To], e)
		}
	}
	return cg
}

func (cg *cgraph) node(ref string) (string, bool) {
	if _, ok := cg.nodes[ref]; ok {
		return ref, true
	}
	id, ok := cg.byKey[ref]
	return id, ok
}

// consumerGraph returns the session's consumer graph at a side, building and
// storing it on first use (deterministic: same bundle, same graph and
// artifact). Caller holds s.mu or runs before the sessions start.
func (s *Session) consumerGraph(ctx context.Context, side relations.Side) (*cgraph, error) {
	if cg := s.cgraphs[side]; cg != nil {
		return cg, nil
	}
	if s.cfg.ConsumerGraphs == nil {
		return nil, rejectf("this investigation has no consumer graph (prompt %s)", s.cfg.Prompt.Version)
	}
	g, err := s.cfg.ConsumerGraphs(ctx, side)
	if err != nil {
		return nil, fmt.Errorf("agent: consumer graph (%s): %w", side, err)
	}
	data, err := json.Marshal(g)
	if err != nil {
		return nil, err
	}
	ref, err := s.cfg.Store.Put(ctx, investigate.ArtifactSpec{Kind: domain.ArtifactGraph, Path: "derived/agent/consumers/" + string(side) + "/" + g.ID + ".json",
		Origin: domain.OriginDerived, MediaType: "application/json", URI: "ri:" + ConsumerGraphProducer, Producer: ConsumerGraphProducer,
		Tags: []string{"consumer-graph", string(side)}}, data)
	if err != nil {
		return nil, fmt.Errorf("agent: store consumer graph: %w", err)
	}
	s.addRunArtifact(ref)
	cg := indexGraph(g, ref)
	s.cgraphs[side] = cg
	return cg, nil
}

// LoadConsumers builds the TO-side consumer graph before the item sessions
// (the dossiers read it). A no-op without feature traceConsumers.
func (s *Session) LoadConsumers(ctx context.Context) (*domain.RelationshipGraph, error) {
	if !s.cfg.Prompt.Features.TraceConsumers {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cg, err := s.consumerGraph(ctx, relations.SideTo)
	if err != nil {
		return nil, err
	}
	return cg.g, nil
}

// --- the trace ---------------------------------------------------------------------------

// ConsumerTrace is trace_consumers' result: every hop from the start objects
// along produces / references / reads-file edges, each with the artifact and
// line that state it, the consumers reached, and the gaps — what the snapshot
// does not establish.
type ConsumerTrace struct {
	Side      string      `json:"side"`
	Graph     string      `json:"graph"` // the consumer graph's artifact
	Start     []TraceNode `json:"start"`
	Hops      []TraceHop  `json:"hops"`
	Consumers []TraceNode `json:"consumers"`
	Gaps      []TraceGap  `json:"gaps"`
	Truncated bool        `json:"truncated,omitempty"`
}

// TraceNode is one object of a trace.
type TraceNode struct {
	Node        string          `json:"node"`
	Key         string          `json:"key"`
	Kind        domain.NodeKind `json:"kind"`
	Placeholder bool            `json:"placeholder,omitempty"`
}

// TraceHop is one relationship of a chain: From provides what To consumes.
type TraceHop struct {
	Edge     string            `json:"edge"`
	Kind     domain.EdgeKind   `json:"kind"`
	From     string            `json:"from"`
	To       string            `json:"to"`
	Status   domain.EdgeStatus `json:"status"`
	Rule     string            `json:"rule"`
	Detail   string            `json:"detail,omitempty"`
	Reason   string            `json:"reason,omitempty"`
	Artifact string            `json:"artifact,omitempty"`
	Line     int               `json:"line,omitempty"`
	Text     string            `json:"text,omitempty"`
	Mount    *domain.EdgeMount `json:"mount,omitempty"`
	File     *domain.EdgeFile  `json:"file,omitempty"`
}

// TraceGap is something the trace cannot establish from the snapshot.
type TraceGap struct {
	Subject string `json:"subject"`
	Detail  string `json:"detail"`
}

// trace walks the consumer edges breadth-first from starts (node ids of cg).
func trace(cg *cgraph, starts []string) *ConsumerTrace {
	t := &ConsumerTrace{Graph: cg.art.ID, Start: []TraceNode{}, Hops: []TraceHop{}, Consumers: []TraceNode{}, Gaps: []TraceGap{}}
	gap := func(subject, format string, args ...any) {
		if len(t.Gaps) < maxTraceGaps {
			t.Gaps = append(t.Gaps, TraceGap{Subject: subject, Detail: fmt.Sprintf(format, args...)})
		} else {
			t.Truncated = true
		}
	}
	tn := func(id string) TraceNode {
		n := cg.nodes[id]
		return TraceNode{Node: n.ID, Key: n.Key, Kind: n.Kind, Placeholder: n.Placeholder}
	}
	isStart := map[string]bool{}
	seen := map[string]bool{}
	var frontier []string
	for _, id := range uniqSorted(starts) {
		isStart[id], seen[id] = true, true
		t.Start = append(t.Start, tn(id))
		frontier = append(frontier, id)
		if len(cg.out[id]) == 0 {
			gap(cg.nodes[id].Key, "no consumer relationship (produces, references, reads-file) leaves this object in the snapshot")
		}
	}
	consumers := map[string]bool{}
	var mountHops []domain.GraphEdge
	for d := 0; d < maxTraceDepth && len(frontier) > 0 && !t.Truncated; d++ {
		var next []string
		for _, id := range frontier {
			for _, e := range cg.out[id] {
				if len(t.Hops) == maxTraceHops {
					t.Truncated = true
					break
				}
				from, to := cg.nodes[e.From], cg.nodes[e.To]
				h := TraceHop{Edge: e.ID, Kind: e.Kind, From: from.Key, To: to.Key, Status: e.Status, Rule: e.Rule, Detail: e.Detail,
					Reason: e.Reason, Mount: e.Mount, File: e.File}
				switch {
				case e.Line != nil:
					h.Artifact, h.Line, h.Text = e.Line.Artifact, e.Line.Line, e.Line.Text
				case len(e.Artifacts) > 0:
					h.Artifact = e.Artifacts[0]
					gap(e.ID, "the line stating %s %s → %s is not located in artifact %s", e.Kind, from.Key, to.Key, h.Artifact)
				default:
					gap(e.ID, "%s %s → %s cites no artifact", e.Kind, from.Key, to.Key)
				}
				t.Hops = append(t.Hops, h)
				if e.Status != domain.EdgeEstablished {
					gap(e.ID, "%s %s → %s is %s: %s", e.Kind, from.Key, to.Key, e.Status, e.Reason)
				}
				if e.Kind != domain.EdgeProduces && !to.Placeholder && !isStart[to.ID] {
					consumers[to.ID] = true
				}
				if e.Mount != nil && to.Kind == domain.NodeWorkload {
					mountHops = append(mountHops, e)
				}
				if !seen[to.ID] {
					seen[to.ID] = true
					next = append(next, to.ID)
					if to.Placeholder && e.Status == domain.EdgeEstablished {
						gap(to.Key, "not an object of the snapshot (created at runtime by its producer): its content cannot be read")
					}
				}
			}
		}
		frontier = next
	}
	if len(frontier) > 0 && !t.Truncated {
		t.Truncated = true
	}
	if t.Truncated {
		gap("trace", "the trace hit its bound (%d hops, depth %d, %d gaps): consumers beyond it are not listed", maxTraceHops, maxTraceDepth, maxTraceGaps)
	}
	// A configuration reached names files the snapshot cannot resolve.
	reached := make([]string, 0, len(seen))
	for id := range seen {
		reached = append(reached, id)
	}
	sort.Strings(reached)
	for _, id := range reached {
		for _, e := range cg.files[id] {
			if e.Status != domain.EdgeEstablished && e.File != nil {
				gap(cg.nodes[id].Key, "data[%s] names %s, read by container %s of %s: %s (%s)", e.File.ConfigKey, e.File.Path, e.File.Container, e.File.Workload, e.Reason, e.Status)
			}
		}
	}
	// A mounted object no configuration names a file of: what the container
	// reads from it is not established. A mounted configuration that names
	// files is the container's configuration itself.
	for _, e := range mountHops {
		from, to := cg.nodes[e.From], cg.nodes[e.To]
		for _, m := range e.Mount.Mounts {
			read := false
			for _, r := range cg.out[e.From] {
				read = read || (r.Kind == domain.EdgeReadsFile && r.File != nil && r.File.Workload == to.Key && r.File.Container == m.Container)
			}
			for _, r := range cg.files[e.From] {
				read = read || (r.File != nil && r.File.Workload == to.Key && r.File.Container == m.Container)
			}
			if !read {
				gap(to.Key, "container %s mounts %s at %s; no configuration in the snapshot names a path under it: what the container reads from it is not established", m.Container, from.Key, m.MountPath)
			}
		}
	}
	ids := make([]string, 0, len(consumers))
	for id := range consumers {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return cg.nodes[ids[i]].Key < cg.nodes[ids[j]].Key })
	for _, id := range ids {
		t.Consumers = append(t.Consumers, tn(id))
	}
	return t
}

// itemStarts are the item's exposure roots present in the consumer graph.
func (s *Session) itemStarts(cg *cgraph, item string) []string {
	if s.cfg.Traversal == nil {
		return nil
	}
	var out []string
	for _, r := range s.cfg.Traversal.RootsOf(item) {
		if _, ok := cg.nodes[r.Node]; ok {
			out = append(out, r.Node)
		}
	}
	return out
}

// --- trace_consumers ------------------------------------------------------------------------

func (s *Session) traceConsumers(ctx context.Context, r investigate.TraceRequest) (*outcome, error) {
	side := relations.Side(r.Side)
	if side == "" {
		side = relations.SideTo
	}
	if side != relations.SideTo && side != relations.SideFrom {
		return nil, rejectf("side %q: want to or from", r.Side)
	}
	if (r.Node == "") == (r.ChangeItem == "") {
		return nil, rejectf("give exactly one of node (a node id or key) or changeItem (trace every exposure root of the item)")
	}
	cg, err := s.consumerGraph(ctx, side)
	if err != nil {
		return nil, err
	}
	var starts []string
	if r.Node != "" {
		id, ok := cg.node(r.Node)
		if !ok {
			return nil, rejectf("unknown node %q (not a node id or key of the %s-side consumer graph)", r.Node, side)
		}
		starts = []string{id}
	} else {
		if _, err := s.item(r.ChangeItem); err != nil {
			return nil, err
		}
		starts = s.itemStarts(cg, r.ChangeItem)
	}
	t := trace(cg, starts)
	t.Side = string(side)
	if len(starts) == 0 {
		t.Gaps = append(t.Gaps, TraceGap{Subject: r.ChangeItem, Detail: "the item has no exposure root in the relationship graph: its consumers are not known from the graph"})
	}
	var arts []string
	var nodes []string
	for _, h := range t.Hops {
		arts = append(arts, h.Artifact)
	}
	for _, n := range append(append([]TraceNode(nil), t.Start...), t.Consumers...) {
		nodes = append(nodes, n.Node)
	}
	return &outcome{value: t, commit: s.retrieve(func() {
		for _, id := range nodes {
			s.traced[id] = true
		}
		if f := s.inspected(r.ChangeItem); f != nil {
			f()
		}
	}, arts...)}, nil
}

// TraceConsumers is the typed form of trace_consumers.
func (s *Session) TraceConsumers(ctx context.Context, r investigate.TraceRequest) (*ConsumerTrace, error) {
	if !s.cfg.Prompt.Features.TraceConsumers {
		_, _, err := s.invoke(ctx, investigate.ToolTraceConsumers, canonicalArgs(r), nil)
		return nil, err
	}
	return typed[*ConsumerTrace](s, ctx, investigate.ToolTraceConsumers, r, func(ctx context.Context) (*outcome, error) { return s.traceConsumers(ctx, r) })
}

// --- the traced-consumer log (completion check) ----------------------------------------------

// tracedObject records, in a session with consumer tracing, that a tool
// returned an object of the consumer graph (get_rendered_object).
func (s *Session) tracedObject(key string) {
	if cg := s.cgraphs[relations.SideTo]; cg != nil && s.traced != nil {
		if id, ok := cg.byKey[key]; ok {
			s.traced[id] = true
		}
	}
}

// tracedArtifact records that read_artifact returned part of an environment
// file: the objects that file defines count as retrieved.
func (s *Session) tracedArtifact(a domain.ArtifactRef) {
	cg := s.cgraphs[relations.SideTo]
	if cg == nil || s.traced == nil || a.Kind != domain.ArtifactEnvironmentFile {
		return
	}
	for _, n := range cg.g.Nodes {
		if !n.Placeholder && contains(n.Artifacts, a.ID) {
			s.traced[n.ID] = true
		}
	}
}

// untracedConsumers are the consumers of the item's exposed objects (TO side)
// no tool of this session returned, by key.
func (s *Session) untracedConsumers(ctx context.Context, item string) ([]string, error) {
	cg, err := s.consumerGraph(ctx, relations.SideTo)
	if err != nil {
		return nil, err
	}
	starts := s.itemStarts(cg, item)
	if len(starts) == 0 {
		return nil, nil
	}
	var out []string
	for _, c := range trace(cg, starts).Consumers {
		if !s.traced[c.Node] {
			out = append(out, c.Key)
		}
	}
	return out, nil
}

func listKeys(keys []string, n int) string {
	if len(keys) <= n {
		return strings.Join(keys, ", ")
	}
	return strings.Join(keys[:n], ", ") + fmt.Sprintf(" … and %d more", len(keys)-n)
}
